package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/minimax"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMiniMaxResourceRouteKeepsOriginalEnabledAccountAndPermissions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousCache := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() { model.DB, common.MemoryCacheEnabled = previousDB, previousCache; sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	require.NoError(t, minimax.MigrateResourceBindings(db))
	base := "https://minimax.example"
	ch := &model.Channel{Id: 62, Type: constant.ChannelTypeMiniMax, Status: common.ChannelStatusEnabled, Group: "media", Models: "MiniMax-H3,minimax-voice-clone,speech-2.8-hd", Key: "other-account\noriginal-account", BaseURL: &base, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{}}}
	require.NoError(t, db.Create(ch).Error)
	info := &relaycommon.RelayInfo{UserId: 21, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 62, ChannelBaseUrl: base, ApiKey: "original-account"}}
	require.NoError(t, minimax.BindResource(info, "file", "9223372036854775807"))
	require.NoError(t, minimax.BindResource(info, "voice", "my-voice-name"))
	tests := []struct{ name, path, model, body string }{
		{"clone", "/v1/voice_clone", "minimax-voice-clone", `{"file_id":"9223372036854775807","voice_id":"another-voice"}`},
		{"H3", "/v1/video/generations", "MiniMax-H3", `{"metadata":{"reference_videos":["mm_file://9223372036854775807"]}}`},
		{"voice", "/v1/audio/speech", "speech-2.8-hd", `{"voice":"system-voice","metadata":{"voice_setting":{"voice_id":"my-voice-name"}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("id", 21)
			bound, err := miniMaxRequestBinding(c, tt.model)
			require.NoError(t, err)
			require.NotNil(t, bound)
			selected, err := selectMiniMaxResourceChannel(c, bound, tt.model, "media")
			require.NoError(t, err)
			require.Nil(t, SetupContextForSelectedChannel(c, selected, tt.model))
			assert.Equal(t, "original-account", common.GetContextKeyString(c, constant.ContextKeyChannelKey))
			assert.Equal(t, 1, common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex))
			_, err = selectMiniMaxResourceChannel(c, bound, tt.model, "another-group")
			require.Error(t, err)
			common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
			common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"unrelated-model": true})
			_, err = selectMiniMaxResourceChannel(c, bound, tt.model, "media")
			require.ErrorContains(t, err, "token")
			common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, false)
			ch.ChannelInfo.MultiKeyStatusList = map[int]int{1: common.ChannelStatusManuallyDisabled}
			require.NoError(t, db.Save(ch).Error)
			_, err = selectMiniMaxResourceChannel(c, bound, tt.model, "media")
			require.ErrorContains(t, err, "credential")
			ch.ChannelInfo.MultiKeyStatusList = map[int]int{}
			ch.Status = common.ChannelStatusManuallyDisabled
			require.NoError(t, db.Save(ch).Error)
			_, err = selectMiniMaxResourceChannel(c, bound, tt.model, "media")
			require.ErrorContains(t, err, "unavailable")
			ch.Status = common.ChannelStatusEnabled
			require.NoError(t, db.Save(ch).Error)
			c.Set("id", 22)
			_, err = miniMaxRequestBinding(c, tt.model)
			if tt.name != "voice" {
				require.Error(t, err)
			} // unknown voice is checked against provider builtins before dispatch
		})
	}
}
