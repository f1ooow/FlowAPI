package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPassthroughModelRedirectOutgoingBody(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	previous := settings.PassThroughRequestEnabled
	claudeSettings := model_setting.GetClaudeSettings()
	previousThinking := claudeSettings.ThinkingAdapterEnabled
	claudeSettings.ThinkingAdapterEnabled = true
	t.Cleanup(func() {
		settings.PassThroughRequestEnabled = previous
		claudeSettings.ThinkingAdapterEnabled = previousThinking
	})

	for _, handler := range []struct {
		name, path  string
		channelType int
		request     dto.Request
		run         func(*gin.Context, *relaycommon.RelayInfo) *types.NewAPIError
	}{
		{"openai", "/v1/chat/completions", constant.ChannelTypeOpenAI, &dto.GeneralOpenAIRequest{}, TextHelper},
		{"responses", "/v1/responses", constant.ChannelTypeOpenAI, &dto.OpenAIResponsesRequest{}, ResponsesHelper},
		{"claude", "/v1/messages", constant.ChannelTypeAnthropic, &dto.ClaudeRequest{}, ClaudeHelper},
		{"image", "/v1/images/generations", constant.ChannelTypeOpenAI, &dto.ImageRequest{}, ImageHelper},
		{"rerank", "/v1/rerank", constant.ChannelTypeOpenAI, &dto.RerankRequest{}, RerankHelper},
	} {
		t.Run(handler.name, func(t *testing.T) {
			for _, global := range []bool{true, false} {
				scope := "global"
				if !global {
					scope = "local"
				}
				t.Run(scope, func(t *testing.T) {
					settings.PassThroughRequestEnabled = global
					for _, tc := range []struct {
						name, mapping, wantModel string
						invalid                  bool
					}{
						{"matched", `[{"match_type":"exact","source":"client","target":"upstream"}]`, "upstream", false},
						{"unmatched", `{"other":"upstream"}`, "client", false},
						{"same target", `{"client":"client"}`, "client", false},
						{"thinking target stays literal", `{"client":"claude-opus-4-8-thinking"}`, "claude-opus-4-8-thinking", false},
						{"effort target stays literal", `{"client":"claude-opus-4-8-high"}`, "claude-opus-4-8-high", false},
						{"invalid JSON never dispatched", `{"client":"upstream"}`, "", true},
					} {
						t.Run(tc.name, func(t *testing.T) {
							payload := " {\n" + `"model":"client","messages":[{"role":"user","content":"hello"}],"input":"hello","prompt":"hello","query":"hello","documents":["hello"],"unknown":9007199254740993,"nested":{"model":"keep-me","n":18446744073709551615},"temperature":0,"stream":false` + "\n} "
							if tc.invalid {
								payload = `{"model":`
							}
							received := make(chan []byte, 1)
							upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								body, err := io.ReadAll(r.Body)
								assert.NoError(t, err)
								assert.EqualValues(t, len(body), r.ContentLength)
								received <- body
								w.Header().Set("Content-Type", "application/json")
								w.WriteHeader(http.StatusBadRequest)
								_, _ = io.WriteString(w, `{"error":{"message":"captured upstream request","type":"invalid_request_error"}}`)
							}))
							defer upstream.Close()
							ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
							ctx.Request = httptest.NewRequest(http.MethodPost, handler.path, strings.NewReader(payload))
							ctx.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
							common.SetContextKey(ctx, constant.ContextKeyChannelType, handler.channelType)
							common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
							common.SetContextKey(ctx, constant.ContextKeyChannelKey, "test-key")
							common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "client")
							common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
								PassThroughBodyEnabled: !global, SystemPrompt: "must not be injected", SystemPromptOverride: true,
							})
							common.SetContextKey(ctx, constant.ContextKeyChannelParamOverride, map[string]interface{}{"unknown": "must not override"})
							ctx.Set("model_mapping", tc.mapping)
							defer common.CleanupBodyStorage(ctx)
							info := &relaycommon.RelayInfo{OriginModelName: "client", Request: handler.request,
								RequestURLPath: handler.path, RelayMode: relayconstant.Path2RelayMode(handler.path)}
							apiErr := handler.run(ctx, info)
							require.NotNil(t, apiErr)
							assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
							if tc.invalid {
								assert.Equal(t, types.ErrorCodeConvertRequestFailed, apiErr.GetErrorCode())
								assert.True(t, types.IsSkipRetryError(apiErr))
								assert.Empty(t, received)
								return
							}
							require.Len(t, received, 1, "handler must have reached the upstream")
							actual := <-received
							if tc.wantModel == "client" {
								assert.Equal(t, payload, string(actual), "no-op keeps exact original bytes")
							} else {
								var expectedFields, fields map[string]json.RawMessage
								require.NoError(t, common.Unmarshal([]byte(payload), &expectedFields))
								require.NoError(t, common.Unmarshal(actual, &fields))
								assert.Equal(t, `"`+tc.wantModel+`"`, string(fields["model"]))
								delete(expectedFields, "model")
								delete(fields, "model")
								assert.Equal(t, expectedFields, fields)
							}
							assert.Equal(t, tc.wantModel, info.UpstreamModelName)
							storage, err := common.GetBodyStorage(ctx)
							require.NoError(t, err)
							original, err := storage.Bytes()
							require.NoError(t, err)
							assert.Equal(t, payload, string(original), "next attempt must still see the client body")
						})
					}
				})
			}
		})
	}
}

func TestPassthroughModelRedirectChannelRetryUsesOriginalBody(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	previous := settings.PassThroughRequestEnabled
	settings.PassThroughRequestEnabled = true
	t.Cleanup(func() { settings.PassThroughRequestEnabled = previous })

	payload := ` {"model":"client","unknown":9007199254740993} `
	received := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		received <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"try another channel","type":"invalid_request_error"}}`)
	}))
	defer upstream.Close()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "client")
	defer common.CleanupBodyStorage(ctx)
	info := &relaycommon.RelayInfo{OriginModelName: "client", Request: &dto.GeneralOpenAIRequest{Model: "client"},
		RequestURLPath: "/v1/chat/completions", RelayMode: relayconstant.RelayModeChatCompletions}

	for _, attempt := range []struct{ mapping, wantBody string }{
		{`{"client":"upstream-a"}`, `{"model":"upstream-a","unknown":9007199254740993}`},
		{`{"client":"upstream-b"}`, `{"model":"upstream-b","unknown":9007199254740993}`},
		{"", payload},
	} {
		ctx.Set("model_mapping", attempt.mapping)
		apiErr := TextHelper(ctx, info)
		require.NotNil(t, apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		require.Len(t, received, 1)
		assert.Equal(t, attempt.wantBody, string(<-received))
		assert.Equal(t, "client", info.Request.(*dto.GeneralOpenAIRequest).Model)
	}
}
