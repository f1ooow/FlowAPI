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
	// The reservation is refunded asynchronously; wait so cleanup cannot race it.
	require.Eventually(t, func() bool {
		var user model.User
		var token model.Token
		return db.First(&user, 7101).Error == nil && user.Quota == 100000 &&
			db.First(&token, 7101).Error == nil && token.RemainQuota == 100000
	}, 5*time.Second, time.Millisecond, "an exhausted request must refund its reservation")
}
