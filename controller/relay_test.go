package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayRetriesChannelUpToSavedMaxAttempts(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
	oldBatch, oldCache := common.BatchUpdateEnabled, common.MemoryCacheEnabled
	common.BatchUpdateEnabled, common.MemoryCacheEnabled = false, false
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		common.BatchUpdateEnabled, common.MemoryCacheEnabled = oldBatch, oldCache
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"retry-test":1}`))
	service.InitHttpClient()

	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"provider failed","type":"server_error"}}`))
	}))
	t.Cleanup(upstream.Close)

	require.NoError(t, db.Create(&model.User{Id: 7101, Username: "retry-user", Quota: 100000, Group: "default"}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 7101, UserId: 7101, Key: "retry-token", RemainQuota: 100000, Status: common.TokenStatusEnabled}).Error)
	weight, priority := uint(100), int64(10)
	channel := &model.Channel{Id: 7110, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "retry", Key: "up-key", BaseURL: &upstream.URL, Group: "default", Models: "retry-test", Weight: &weight, Priority: &priority, OtherSettings: `{"reliability":{"max_attempts":3}}`}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "retry-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"retry-test","messages":[{"role":"user","content":"hello"}]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	for key, value := range map[string]any{"id": 7101, "token_id": 7101, "token_key": "retry-token", "token_name": "retry-token", "token_group": "default", "group": "default"} {
		ctx.Set(key, value)
	}
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, channel, "retry-test"))
	t.Cleanup(func() { common.CleanupBodyStorage(ctx) })

	Relay(ctx, types.RelayFormatOpenAI)

	assert.Equal(t, int32(3), attempts.Load(), "a retryable failure must use every attempt the channel saved")
	stateValue, exists := ctx.Get("route_state")
	require.True(t, exists)
	state, ok := stateValue.(*service.RouteState)
	require.True(t, ok)
	require.Len(t, state.History, 3)
	for i, attempt := range state.History {
		require.NotNil(t, attempt.Timing)
		assert.Equal(t, i+1, attempt.Attempt)
		assert.GreaterOrEqual(t, attempt.Timing.EndOffsetMS, attempt.Timing.StartOffsetMS)
		assert.GreaterOrEqual(t, attempt.Timing.DurationMS, int64(0))
		if i > 0 {
			assert.GreaterOrEqual(t, attempt.Timing.StartOffsetMS, state.History[i-1].Timing.EndOffsetMS)
		}
	}
	assert.Equal(t, service.RouteAttemptExcluded, state.History[2].Outcome)
	// The reservation is refunded asynchronously; wait so cleanup cannot race it.
	require.Eventually(t, func() bool {
		var user model.User
		var token model.Token
		return db.First(&user, 7101).Error == nil && user.Quota == 100000 &&
			db.First(&token, 7101).Error == nil && token.RemainQuota == 100000
	}, 5*time.Second, time.Millisecond, "an exhausted request must refund its reservation")
}

func TestRelayRouteTimingAcrossProvidersAndSynchronousImages(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, response string
		firstStatus                int
		channels                   int
	}{
		{"text failover", "/v1/chat/completions", `{"model":"retry-test","messages":[{"role":"user","content":"hello"}]}`, `{"id":"completion","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, 500, 2},
		{"terminal failure", "/v1/chat/completions", `{"model":"retry-test","messages":[{"role":"user","content":"hello"}]}`, "", 400, 1},
		{"image failover", "/v1/images/generations", `{"model":"retry-test","prompt":"a tree","n":1,"size":"1024x1024"}`, `{"created":1700000000,"data":[{"url":"https://example.com/image.png"}]}`, 500, 2},
		{"direct image success", "/v1/images/generations", `{"model":"retry-test","prompt":"a tree","n":1,"size":"1024x1024"}`, `{"created":1700000000,"data":[{"url":"https://example.com/image.png"}]}`, 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
			oldBatch, oldCache := common.BatchUpdateEnabled, common.MemoryCacheEnabled
			common.BatchUpdateEnabled, common.MemoryCacheEnabled = false, false
			oldRatio := ratio_setting.ModelRatio2JSONString()
			t.Cleanup(func() {
				common.BatchUpdateEnabled, common.MemoryCacheEnabled = oldBatch, oldCache
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
			})
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"retry-test":1}`))
			service.InitHttpClient()
			require.NoError(t, db.Create(&model.User{Id: 7201, Username: "timing-user", Quota: 10000000, Group: "default"}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 7201, UserId: 7201, Key: "timing-token", RemainQuota: 10000000, Status: common.TokenStatusEnabled}).Error)
			var selected *model.Channel
			for index := 0; index < tc.channels; index++ {
				status := tc.firstStatus
				if index > 0 {
					status = 200
				}
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					if status == 200 {
						_, _ = w.Write([]byte(tc.response))
					} else {
						_, _ = w.Write([]byte(`{"error":{"message":"provider failed","type":"server_error"}}`))
					}
				}))
				t.Cleanup(upstream.Close)
				weight, priority := uint(100), int64(10-index)
				channel := &model.Channel{Id: 7210 + index, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: tc.name, Key: "key", BaseURL: &upstream.URL, Group: "default", Models: "retry-test", Weight: &weight, Priority: &priority, OtherSettings: `{"reliability":{"max_attempts":1}}`}
				require.NoError(t, db.Create(channel).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "retry-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
				if index == 0 {
					selected = channel
				}
			}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			for key, value := range map[string]any{"id": 7201, "token_id": 7201, "token_key": "timing-token", "token_name": "timing-token", "token_group": "default", "group": "default"} {
				ctx.Set(key, value)
			}
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
			require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, selected, "retry-test"))
			t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
			format := types.RelayFormatOpenAI
			if tc.path == "/v1/images/generations" {
				format = types.RelayFormatOpenAIImage
			}
			t.Cleanup(func() {
				require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
			})
			Relay(ctx, format)
			stateValue, exists := ctx.Get("route_state")
			require.True(t, exists)
			state, ok := stateValue.(*service.RouteState)
			require.True(t, ok)
			require.Len(t, state.History, tc.channels)
			for index, attempt := range state.History {
				require.NotNil(t, attempt.Timing)
				assert.Equal(t, 7210+index, attempt.ChannelID)
				assert.GreaterOrEqual(t, attempt.Timing.DurationMS, int64(0))
			}
			if tc.firstStatus == 400 {
				assert.Equal(t, service.RouteAttemptStopped, state.History[0].Outcome)
			} else {
				assert.Equal(t, service.RouteAttemptSucceeded, state.History[len(state.History)-1].Outcome)
				var consumeLogs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&consumeLogs).Error)
				require.Len(t, consumeLogs, 1)
				var other struct {
					AdminInfo struct {
						History []service.RouteAttempt `json:"route_history"`
					} `json:"admin_info"`
				}
				require.NoError(t, common.UnmarshalJsonStr(consumeLogs[0].Other, &other))
				require.Len(t, other.AdminInfo.History, tc.channels)
				assert.Equal(t, state.History[len(state.History)-1].Timing, other.AdminInfo.History[tc.channels-1].Timing)
			}
		})
	}
}
