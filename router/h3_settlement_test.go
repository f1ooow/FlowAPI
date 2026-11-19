package router

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel/task/hailuo"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type h3Transport func(*http.Request) (*http.Response, error)

func (transport h3Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestH3FinalQuotaHTTP(t *testing.T) {
	setupRelayRouterTestDB(t)
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}, &model.Log{}))
	oldBatch, oldCache, oldLimit, oldFactory := common.BatchUpdateEnabled, common.MemoryCacheEnabled, constant.TaskQueryLimit, service.GetTaskAdaptorFunc
	common.BatchUpdateEnabled, common.MemoryCacheEnabled, constant.TaskQueryLimit = true, false, 10
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(platform) }
	t.Cleanup(func() {
		common.BatchUpdateEnabled, common.MemoryCacheEnabled, constant.TaskQueryLimit, service.GetTaskAdaptorFunc = oldBatch, oldCache, oldLimit, oldFactory
	})
	oldPrices := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"MiniMax-H3":0.1125}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices)) })
	const balance = 10000000
	user := model.User{Username: "h3-owner", Group: "default", Status: common.UserStatusEnabled, Quota: balance}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "h3ownerkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: balance, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	baseURL := "https://h3.fixture.example"
	channel := model.Channel{Type: constant.ChannelTypeMiniMax, BaseURL: &baseURL, Key: "fixture-key", Status: common.ChannelStatusEnabled, Models: hailuo.ModelMiniMaxH3, Group: "default"}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(db))

	var upstreamID, status, resolution string
	var queries int
	service.InitHttpClient()
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	client.Transport = h3Transport(func(request *http.Request) (*http.Response, error) {
		body := ""
		switch request.Method + " " + request.URL.String() {
		case "POST " + baseURL + hailuo.H3CreateEndpoint:
			body = fmt.Sprintf(`{"task_id":%q}`, upstreamID)
		case "GET " + baseURL + "/v2/query/video_generation/" + upstreamID:
			queries++
			body = fmt.Sprintf(`{"result":{"task":{"id":%q,"status":%q,"resolution":%q,"content":{"url":"https://cdn.example/video.mp4"},"usage":{"input_seconds":0,"output_seconds":5}}}}`, upstreamID, status, resolution)
		default:
			return nil, fmt.Errorf("unexpected fixture request %s %s", request.Method, request.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	engine := gin.New()
	SetVideoRouter(engine)
	call := func(t *testing.T, method, path, key, body string, code int) map[string]any {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+key)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, code, response.Code, response.Body.String())
		var result map[string]any
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		return result
	}
	fetch := func(t *testing.T, id string) map[string]any {
		t.Helper()
		response := call(t, "GET", "/v1/video/generations/"+id, token.Key, "", 200)
		require.Equal(t, "success", response["code"])
		data := response["data"].(map[string]any)
		assert.Equal(t, id, data["task_id"])
		assert.Equal(t, hailuo.ModelMiniMaxH3, data["properties"].(map[string]any)["origin_model_name"])
		return data
	}
	var total, requests int
	var refunds int64
	ledger := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.First(&user, user.Id).Error)
		require.NoError(t, db.First(&token, token.Id).Error)
		require.NoError(t, db.First(&channel, channel.Id).Error)
		assert.Equal(t, balance-total, user.Quota)
		assert.Equal(t, balance-total, token.RemainQuota)
		assert.Equal(t, total, user.UsedQuota)
		assert.Equal(t, total, token.UsedQuota)
		assert.Equal(t, int64(total), channel.UsedQuota)
		assert.Equal(t, requests, user.RequestCount)
		var count int64
		require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&count).Error)
		assert.Equal(t, refunds, count)
	}
	for _, tc := range []struct {
		name, upstreamStatus, tier, terminal string
		final                                int
		interrupt                            bool
	}{
		{"unchanged", "succeeded", "1080P", "SUCCESS", 437500, false},
		{"downgrade", "succeeded", "768P", "SUCCESS", 281250, true},
		{"failed", "failed", "", "FAILURE", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstreamID, status, resolution = "provider-"+tc.name, tc.upstreamStatus, tc.tier
			created := call(t, "POST", "/v1/videos", token.Key, `{"model":"MiniMax-H3","prompt":"fixture","size":"1080P","seconds":"5","metadata":{"ratio":"16:9"}}`, 200)
			id := created["id"].(string)
			require.NotEqual(t, upstreamID, id)
			pending := fetch(t, id)
			assert.NotContains(t, pending, "settled_quota")
			assert.NotContains(t, pending, "settlement_pending")
			assert.Equal(t, float64(0), pending["finish_time"])
			reserved := int(pending["quota"].(float64))
			require.Equal(t, 437500, reserved)
			total += reserved
			requests++
			ledger(t)
			stale, exists, err := model.GetByTaskId(user.Id, id)
			require.NoError(t, err)
			require.True(t, exists)
			if tc.interrupt {
				require.NoError(t, db.Exec(`CREATE TRIGGER reject_usage BEFORE UPDATE OF used_quota ON channels BEGIN SELECT RAISE(ABORT, 'fixture accounting failure'); END`).Error)
			}
			service.RunTaskPollingOnce(t.Context(), nil)
			terminal := fetch(t, id)
			assert.Equal(t, tc.terminal, terminal["status"])
			finish := int64(terminal["finish_time"].(float64))
			assert.GreaterOrEqual(t, finish, stale.SubmitTime)
			assert.LessOrEqual(t, finish, time.Now().Unix())
			if tc.interrupt {
				assert.NotContains(t, terminal, "settled_quota")
				assert.Equal(t, true, terminal["settlement_pending"])
				assert.NotContains(t, terminal, "pending_quota")
				ledger(t)
				require.True(t, model.HasUnfinishedSyncTasks())
				require.NoError(t, db.Exec(`DROP TRIGGER reject_usage`).Error)
			}
			before := queries
			var workers sync.WaitGroup
			for range 2 {
				workers.Go(func() { service.RunTaskPollingOnce(t.Context(), nil) })
			}
			workers.Wait()
			assert.Equal(t, before, queries)
			require.NoError(t, service.UpdateVideoTasks(t.Context(), stale.Platform, map[int][]string{channel.Id: {upstreamID}}, map[string]*model.Task{upstreamID: stale}))
			for range 2 {
				data := fetch(t, id)
				assert.Equal(t, float64(tc.final), data["settled_quota"])
				assert.NotContains(t, data, "settlement_pending")
				assert.Equal(t, float64(finish), data["finish_time"])
			}
			total += tc.final - reserved
			if tc.final < reserved {
				refunds++
			}
			ledger(t)
			require.False(t, model.HasUnfinishedSyncTasks())
		})
		if t.Failed() {
			return
		}
	}
	for _, historical := range []model.Task{
		{TaskID: "old-success", Status: model.TaskStatusSuccess, Quota: 437500},
		{TaskID: "old-failure", Status: model.TaskStatusFailure, Quota: 0},
	} {
		historical.UserId, historical.ChannelId = user.Id, channel.Id
		historical.FinishTime = time.Now().Unix() - 3600
		historical.Properties.OriginModelName = hailuo.ModelMiniMaxH3
		require.NoError(t, db.Create(&historical).Error)
		service.RunTaskPollingOnce(t.Context(), nil)
		legacy := fetch(t, historical.TaskID)
		assert.NotContains(t, legacy, "settled_quota")
		assert.NotContains(t, legacy, "settlement_pending")
	}
	call(t, "GET", "/v1/video/generations/old-success", "invalid", "", 401)
	other := model.User{Username: "other-owner", AffCode: "other", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&other).Error)
	otherToken := model.Token{UserId: other.Id, Key: "otherkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&otherToken).Error)
	denied := call(t, "GET", "/v1/video/generations/old-success", otherToken.Key, "", 400)
	assert.Equal(t, "task_not_exist", denied["code"])
}
