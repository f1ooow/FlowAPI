package channel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalHeaderPassthrough(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := settings.PassThroughHeadersEnabled
	t.Cleanup(func() { settings.PassThroughHeadersEnabled = original })

	for _, tc := range []struct {
		name    string
		enabled bool
		info    relaycommon.RelayInfo
		want    map[string]string
	}{
		{
			name: "disabled preserves existing default",
			info: relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}},
			want: map[string]string{},
		},
		{
			name:    "enabled covers new channel without template",
			enabled: true,
			info:    relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}},
			want:    map[string]string{"session_id": "session-1", "x-feature": "client"},
		},
		{
			name:    "existing channel overrides win",
			enabled: true,
			info: relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				HeadersOverride: map[string]any{"X-Feature": "channel"},
			}},
			want: map[string]string{"session_id": "session-1", "x-feature": "channel"},
		},
		{
			name: "disabled preserves channel template",
			info: relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				HeadersOverride: map[string]any{"*": true},
			}},
			want: map[string]string{"session_id": "session-1", "x-feature": "client"},
		},
		{
			name:    "channel tests do not forward admin request headers",
			enabled: true,
			info:    relaycommon.RelayInfo{IsChannelTest: true, ChannelMeta: &relaycommon.ChannelMeta{}},
			want:    map[string]string{},
		},
		{
			name:    "runtime final map is not repopulated",
			enabled: true,
			info: relaycommon.RelayInfo{
				ChannelMeta:               &relaycommon.ChannelMeta{},
				UseRuntimeHeadersOverride: true,
				RuntimeHeadersOverride:    map[string]any{"x-feature": "runtime"},
			},
			want: map[string]string{"x-feature": "runtime"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings.PassThroughHeadersEnabled = tc.enabled
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			ctx.Request.Header.Set("Session_id", "session-1")
			ctx.Request.Header.Set("X-Feature", "client")
			for _, name := range []string{
				"Authorization", "Api-Key", "X-Api-Key", "X-Goog-Api-Key", "Cookie",
				"Connection", "Host", "Content-Length", "Accept-Encoding",
				"Content-Type", "Chatgpt-Account-Id",
				"Sec-WebSocket-Key", "Sec-WebSocket-Protocol",
			} {
				ctx.Request.Header.Set(name, "must-not-forward")
			}
			headers, err := processHeaderOverride(&tc.info, ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.want, headers)
		})
	}
}

func TestGlobalHeaderPassthroughParamOverride(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := settings.PassThroughHeadersEnabled
	settings.PassThroughHeadersEnabled = true
	t.Cleanup(func() { settings.PassThroughHeadersEnabled = original })

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		HeadersOverride: map[string]any{"X-Static": "channel"},
		ParamOverride: map[string]any{"operations": []any{
			map[string]any{"mode": "set_header", "path": "X-Feature", "value": "runtime"},
		}},
	}}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("Session_id", "session-1")
	ctx.Request.Header.Set("X-Feature", "client")

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"example"}`), info)
	require.NoError(t, err)
	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"session_id": "session-1", "x-feature": "runtime", "x-static": "channel",
	}, headers)
	assert.Equal(t, map[string]any{"X-Static": "channel"}, info.HeadersOverride)
}
