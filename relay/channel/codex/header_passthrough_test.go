package codex

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeaderPassthroughOutgoingRequest(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := settings.PassThroughHeadersEnabled
	t.Cleanup(func() { settings.PassThroughHeadersEnabled = original })

	for _, tc := range []struct {
		name          string
		globalEnabled bool
		overrides     map[string]any
		wantAuth      string
		wantAccount   string
		wantType      string
		wantFeature   string
	}{
		{
			name:          "global defaults protect adapter headers without channel template",
			globalEnabled: true,
			wantAuth:      "Bearer channel-token", wantAccount: "channel-account",
			wantType: "application/json", wantFeature: "client-feature",
		},
		{
			name:      "local wildcard protects adapter headers",
			overrides: map[string]any{"*": true},
			wantAuth:  "Bearer channel-token", wantAccount: "channel-account",
			wantType: "application/json", wantFeature: "client-feature",
		},
		{
			name:      "local regex protects adapter headers",
			overrides: map[string]any{"regex:.*": true},
			wantAuth:  "Bearer channel-token", wantAccount: "channel-account",
			wantType: "application/json", wantFeature: "client-feature",
		},
		{
			name:          "explicit administrator values win over adapter and client",
			globalEnabled: true,
			overrides: map[string]any{
				"Authorization":      "Bearer explicit-token",
				"Chatgpt-Account-Id": "explicit-account",
				"Content-Type":       "application/vnd.example+json",
				"X-Feature":          "explicit-feature",
			},
			wantAuth: "Bearer explicit-token", wantAccount: "explicit-account",
			wantType: "application/vnd.example+json", wantFeature: "explicit-feature",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings.PassThroughHeadersEnabled = tc.globalEnabled
			received := make(chan http.Header, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- r.Header.Clone()
				w.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"example"}`))
			ctx.Request.Header.Set("Authorization", "Bearer client-token")
			ctx.Request.Header.Set("Chatgpt-Account-Id", "client-account")
			ctx.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
			ctx.Request.Header.Set("Session_id", "client-session")
			ctx.Request.Header.Set("X-Feature", "client-feature")
			ctx.Request.Header.Set("Cookie", "client-cookie")
			ctx.Request.Header.Set("X-Api-Key", "client-key")
			ctx.Request.Header.Set("Connection", "keep-alive")
			info := &relaycommon.RelayInfo{
				RelayMode: relayconstant.RelayModeResponses,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl:  upstream.URL,
					ApiKey:          `{"access_token":"channel-token","account_id":"channel-account"}`,
					HeadersOverride: tc.overrides,
				},
			}
			result, err := (&Adaptor{}).DoRequest(ctx, info, strings.NewReader(`{"model":"example"}`))
			require.NoError(t, err)
			response, ok := result.(*http.Response)
			require.True(t, ok)
			defer response.Body.Close()
			_, err = io.Copy(io.Discard, response.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusNoContent, response.StatusCode)

			headers := <-received
			assert.Equal(t, tc.wantAuth, headers.Get("Authorization"))
			assert.Equal(t, tc.wantAccount, headers.Get("Chatgpt-Account-Id"))
			assert.Equal(t, tc.wantType, headers.Get("Content-Type"))
			assert.Equal(t, tc.wantFeature, headers.Get("X-Feature"))
			assert.Equal(t, "client-session", headers.Get("Session_id"))
			assert.Equal(t, "responses=experimental", headers.Get("OpenAI-Beta"))
			assert.Equal(t, "codex_cli_rs", headers.Get("Originator"))
			assert.Empty(t, headers.Get("Cookie"))
			assert.Empty(t, headers.Get("X-Api-Key"))
			assert.Empty(t, headers.Get("Connection"))
		})
	}
}
