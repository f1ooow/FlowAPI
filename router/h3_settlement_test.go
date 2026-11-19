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
	var posts, queries int
	var onDispatch func()
	var postError error
	var accountTiers map[string]string
	service.InitHttpClient()
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	client.Transport = h3Transport(func(request *http.Request) (*http.Response, error) {
		body := ""
		switch request.Method + " " + request.URL.String() {
		case "POST " + baseURL + hailuo.H3CreateEndpoint:
			posts++
			if onDispatch != nil {
				onDispatch()
			}
			if postError != nil {
				return nil, postError
			}
			body = fmt.Sprintf(`{"task_id":%q}`, upstreamID)
		case "GET " + baseURL + "/v2/query/video_generation/" + upstreamID:
			queries++
			tier := resolution
			if accountTiers != nil {
				tier = accountTiers[request.Header.Get("Authorization")]
				if tier == "" {
					return nil, fmt.Errorf("unrecognized provider account")
				}
			}
			body = fmt.Sprintf(`{"result":{"task":{"id":%q,"status":%q,"resolution":%q,"content":{"url":"https://cdn.example/video.mp4"},"usage":{"input_seconds":0,"output_seconds":5}}}}`, upstreamID, status, tier)
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
	var total, requests, otherTotal int
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
		assert.Equal(t, int64(total+otherTotal), channel.UsedQuota)
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
	other := model.User{Username: "other-owner", AffCode: "other", Status: common.UserStatusEnabled, Group: "default", Quota: balance}
	require.NoError(t, db.Create(&other).Error)
	otherToken := model.Token{UserId: other.Id, Key: "otherkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, RemainQuota: balance}
	require.NoError(t, db.Create(&otherToken).Error)
	denied := call(t, "GET", "/v1/video/generations/old-success", otherToken.Key, "", 400)
	assert.Equal(t, "task_not_exist", denied["code"])
	const payload = `{"model":"MiniMax-H3","prompt":"fixture","size":"1080P","seconds":"5","metadata":{"ratio":"16:9"}}`
	t.Run("acknowledged dispatch recovers delivered resolution", func(t *testing.T) {
		upstreamID, status, resolution = "provider-interrupted", "succeeded", "768P"
		beforePosts, beforeQueries := posts, queries
		var intent model.Task
		onDispatch = func() {
			require.NoError(t, db.Where("status = ?", "DISPATCHING").First(&intent).Error)
			require.NoError(t, db.Exec(`CREATE TRIGGER reject_accept BEFORE UPDATE ON tasks WHEN OLD.status = 'DISPATCHING' AND NEW.status = 'NOT_START' BEGIN SELECT RAISE(ABORT, 'interrupted acceptance'); END`).Error)
		}
		defer func() { onDispatch = nil }()
		result := call(t, "POST", "/v1/videos", token.Key, payload, 202)
		require.Equal(t, intent.TaskID, result["id"])
		require.Equal(t, "acceptance_recovery_pending", result["code"])
		require.NoError(t, intent.MarkH3DispatchUnknown())
		assert.Equal(t, "DISPATCHING", fetch(t, intent.TaskID)["status"])
		oldTimeout := constant.TaskTimeoutMinutes
		constant.TaskTimeoutMinutes = 0
		t.Cleanup(func() { constant.TaskTimeoutMinutes = oldTimeout })
		require.NoError(t, db.Model(&intent).Update("submit_time", time.Now().Add(-2*time.Minute).Unix()).Error)
		require.True(t, model.HasUnfinishedSyncTasks())
		service.RunTaskPollingOnce(t.Context(), nil)
		data := fetch(t, intent.TaskID)
		assert.Equal(t, "SUCCESS", data["status"])
		assert.Equal(t, float64(281250), data["settled_quota"])
		denied := call(t, "GET", "/v1/video/generations/"+intent.TaskID, otherToken.Key, "", 400)
		assert.Equal(t, "task_not_exist", denied["code"])
		require.NoError(t, db.Exec(`DROP TRIGGER reject_accept`).Error)
		assert.Equal(t, beforePosts+1, posts)
		assert.Equal(t, beforeQueries+1, queries)
		total += 281250
		requests++
		refunds++
		ledger(t)
	})
	t.Run("provider acknowledges after dispatch timeout", func(t *testing.T) {
		upstreamID, status, resolution = "provider-late", "succeeded", "768P"
		beforePosts, beforeQueries := posts, queries
		oldTimeout := constant.TaskTimeoutMinutes
		constant.TaskTimeoutMinutes = 1
		t.Cleanup(func() { constant.TaskTimeoutMinutes, onDispatch = oldTimeout, nil })
		var intent model.Task
		onDispatch = func() {
			require.NoError(t, db.Where("status = ?", model.TaskStatusDispatching).First(&intent).Error)
			require.NoError(t, db.Model(&intent).Update("submit_time", time.Now().Add(-time.Hour).Unix()).Error)
			service.RunTaskPollingOnce(t.Context(), nil)
			data := fetch(t, intent.TaskID)
			assert.Equal(t, "DISPATCH_UNKNOWN", data["status"])
			assert.NotContains(t, data, "settled_quota")
		}
		result := call(t, "POST", "/v1/videos", token.Key, payload, 200)
		require.Equal(t, intent.TaskID, result["id"])
		service.RunTaskPollingOnce(t.Context(), nil)
		assert.Equal(t, float64(281250), fetch(t, intent.TaskID)["settled_quota"])
		assert.Equal(t, beforePosts+1, posts)
		assert.Equal(t, beforeQueries+1, queries)
		total += 281250
		requests++
		refunds++
		ledger(t)
	})
	for _, scenario := range []struct{ name, code string }{
		{"ack and promotion unavailable", "provider_acknowledgement_unrecorded"},
		{"connection lost after dispatch", "provider_acceptance_unknown"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			upstreamID = "provider-unresolved"
			beforePosts, beforeQueries := posts, queries
			if scenario.code == "provider_acknowledgement_unrecorded" {
				require.NoError(t, db.Exec(`CREATE TRIGGER reject_ack BEFORE UPDATE OF h3_upstream_id ON tasks BEGIN SELECT RAISE(ABORT, 'cannot save provider id'); END`).Error)
				t.Cleanup(func() { require.NoError(t, db.Exec(`DROP TRIGGER reject_ack`).Error) })
			} else {
				oldRetries := common.RetryTimes
				common.RetryTimes = 2
				postError = io.ErrUnexpectedEOF
				t.Cleanup(func() { common.RetryTimes, postError = oldRetries, nil })
			}
			result := call(t, "POST", "/v1/videos", token.Key, payload, 202)
			require.Equal(t, scenario.code, result["code"])
			id := result["id"].(string)
			require.NotEqual(t, upstreamID, id)
			require.Equal(t, id, result["task_id"])
			oldTimeout := constant.TaskTimeoutMinutes
			constant.TaskTimeoutMinutes = 1
			t.Cleanup(func() { constant.TaskTimeoutMinutes = oldTimeout })
			require.NoError(t, db.Model(&model.Task{}).Where("task_id = ?", id).Update("submit_time", time.Now().Add(-time.Hour).Unix()).Error)
			service.RunTaskPollingOnce(t.Context(), nil)
			data := fetch(t, id)
			assert.Equal(t, "DISPATCH_UNKNOWN", data["status"])
			assert.NotContains(t, data, "settled_quota")
			assert.Equal(t, float64(0), data["finish_time"])
			assert.Equal(t, beforePosts+1, posts)
			assert.Equal(t, beforeQueries, queries)
			total += 437500
			requests++
			ledger(t)
		})
	}
	checkOther := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.First(&other, other.Id).Error)
		require.NoError(t, db.First(&otherToken, otherToken.Id).Error)
		assert.Equal(t, balance-otherTotal, other.Quota)
		assert.Equal(t, balance-otherTotal, otherToken.RemainQuota)
		assert.Equal(t, otherTotal, other.UsedQuota)
		assert.Equal(t, otherTotal, otherToken.UsedQuota)
	}
	t.Run("two keys on one channel share a provider id", func(t *testing.T) {
		upstreamID, status = "account-local-id", "succeeded"
		beforePosts, beforeQueries := posts, queries
		accountTiers = map[string]string{"Bearer fixture-key": "1080P", "Bearer other-account": "768P"}
		t.Cleanup(func() { accountTiers = nil })
		first := call(t, "POST", "/v1/videos", token.Key, payload, 200)["id"].(string)
		require.NoError(t, db.Model(&channel).Update("key", "other-account").Error)
		second := call(t, "POST", "/v1/videos", otherToken.Key, payload, 200)["id"].(string)
		require.NotEqual(t, first, second)
		require.NoError(t, db.Model(&channel).Update("key", "fixture-key").Error)
		service.RunTaskPollingOnce(t.Context(), nil)
		assert.Equal(t, beforePosts+2, posts)
		assert.Equal(t, beforeQueries+2, queries)
		assert.Equal(t, float64(437500), fetch(t, first)["settled_quota"])
		secondData := call(t, "GET", "/v1/video/generations/"+second, otherToken.Key, "", 200)["data"].(map[string]any)
		assert.Equal(t, second, secondData["task_id"])
		assert.Equal(t, float64(281250), secondData["settled_quota"])
		assert.Equal(t, "task_not_exist", call(t, "GET", "/v1/video/generations/"+second, token.Key, "", 400)["code"])
		total += 437500
		otherTotal += 281250
		requests++
		refunds++
		checkOther(t)
		ledger(t)
	})
	t.Run("retired channel and active channel share a provider id", func(t *testing.T) {
		upstreamID, status, resolution = "provider-retired", "succeeded", "768P"
		beforePosts, beforeQueries := posts, queries
		onDispatch = func() { require.NoError(t, channel.Delete()) }
		defer func() { onDispatch = nil }()
		created := call(t, "POST", "/v1/videos", token.Key, payload, 200)
		id := created["id"].(string)
		saved, exists, err := model.GetByTaskId(user.Id, id)
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, upstreamID, saved.GetUpstreamTaskID())
		onDispatch = nil
		active := model.Channel{Type: constant.ChannelTypeMiniMax, BaseURL: &baseURL, Key: "active-account", Status: common.ChannelStatusEnabled, Models: hailuo.ModelMiniMaxH3, Group: "default"}
		require.NoError(t, db.Create(&active).Error)
		require.NoError(t, active.AddAbilities(db))
		activeID := call(t, "POST", "/v1/videos", otherToken.Key, payload, 200)["id"].(string)
		accountTiers = map[string]string{"Bearer active-account": "768P"}
		t.Cleanup(func() { accountTiers = nil })
		require.NoError(t, token.Delete())
		replacement := model.Token{UserId: user.Id, Key: "replacement", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
		require.NoError(t, db.Create(&replacement).Error)
		common.MemoryCacheEnabled = true
		for range 2 {
			service.RunTaskPollingOnce(t.Context(), nil)
		}
		common.MemoryCacheEnabled = false
		assert.Equal(t, beforePosts+2, posts)
		assert.Equal(t, beforeQueries+1, queries)
		activeData := call(t, "GET", "/v1/video/generations/"+activeID, otherToken.Key, "", 200)["data"].(map[string]any)
		assert.Equal(t, "SUCCESS", activeData["status"])
		assert.Equal(t, float64(281250), activeData["settled_quota"])
		otherTotal += 281250
		checkOther(t)
		require.NoError(t, db.First(&active, active.Id).Error)
		assert.Equal(t, int64(281250), active.UsedQuota)
		call(t, "GET", "/v1/video/generations/"+id, token.Key, "", 401)
		data := call(t, "GET", "/v1/video/generations/"+id, replacement.Key, "", 200)["data"].(map[string]any)
		assert.Equal(t, id, data["task_id"])
		assert.Equal(t, "FAILURE", data["status"])
		assert.Equal(t, float64(0), data["settled_quota"])
		assert.NotContains(t, data, "settlement_pending")
		require.NoError(t, db.First(&user, user.Id).Error)
		require.NoError(t, db.Unscoped().First(&token, token.Id).Error)
		assert.True(t, token.DeletedAt.Valid)
		assert.Equal(t, balance-total, token.RemainQuota)
		assert.Equal(t, balance-total, user.Quota)
		assert.Equal(t, total, token.UsedQuota)
		assert.Equal(t, total, user.UsedQuota)
	})
}
