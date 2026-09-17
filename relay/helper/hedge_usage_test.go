package helper

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHedgeUsageObservesNeutralClaudeCacheAndMergesDelta(t *testing.T) {
	a := &relaycommon.HedgeAttempt{}
	observeHedgeUsage(a, StreamProtocolAnthropic, `{"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":90,"cache_creation_input_tokens":30}}}`)
	require.NotNil(t, a.ReportedUsage)
	require.NotNil(t, a.ReportedUsage.BillingUsage)
	observeHedgeUsage(a, StreamProtocolAnthropic, `{"type":"message_delta","usage":{"output_tokens":7}}`)
	usage := a.ReportedUsage.BillingUsage.ClaudeUsage
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 90, usage.CacheReadInputTokens)
	assert.Equal(t, 30, usage.CacheCreationInputTokens)
	assert.Equal(t, 7, usage.OutputTokens)
}

func TestHedgeUsageNeverEstimatesFromContent(t *testing.T) {
	a := &relaycommon.HedgeAttempt{}
	observeHedgeUsage(a, StreamProtocolOpenAIChat, `{"choices":[{"delta":{"content":"unmetered"}}]}`)
	assert.Nil(t, a.ReportedUsage)
	observeHedgeUsage(a, StreamProtocolOpenAIChat, `{"usage":{"prompt_tokens":0,"completion_tokens":0}}`)
	require.NotNil(t, a.ReportedUsage, "reported zero is different from missing usage")
	assert.Zero(t, a.ReportedUsage.TotalTokens)
}

func TestHedgeUsageRequiresReportedNumericCounts(t *testing.T) {
	for _, protocol := range []struct {
		name     string
		protocol StreamProtocol
		frame    string
		count    string
	}{
		{"chat", StreamProtocolOpenAIChat, `{"usage":%s}`, "prompt_tokens"},
		{"responses", StreamProtocolOpenAIResponses, `{"type":"response.completed","response":{"usage":%s}}`, "input_tokens"},
		{"claude", StreamProtocolAnthropic, `{"type":"message_start","message":{"usage":%s}}`, "input_tokens"},
		{"gemini", StreamProtocolGemini, `{"usageMetadata":%s}`, "promptTokenCount"},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			for _, invalid := range []string{`{}`, `null`, `[]`, `{"unknown_count":0}`,
				fmt.Sprintf(`{"%s":null}`, protocol.count),
				fmt.Sprintf(`{"%s":"0"}`, protocol.count),
				fmt.Sprintf(`{"%s":false}`, protocol.count),
				fmt.Sprintf(`{"%s":[0]}`, protocol.count),
				fmt.Sprintf(`{"%s":0.5}`, protocol.count),
			} {
				t.Run(invalid, func(t *testing.T) {
					a := &relaycommon.HedgeAttempt{}
					observeHedgeUsage(a, protocol.protocol, fmt.Sprintf(protocol.frame, invalid))
					assert.Nil(t, a.ReportedUsage, "unknown usage must not become an authoritative zero")
					observeHedgeUsage(a, protocol.protocol, fmt.Sprintf(protocol.frame, fmt.Sprintf(`{"%s":12}`, protocol.count)))
					require.NotNil(t, a.ReportedUsage)
					previous := a.ReportedUsage
					observeHedgeUsage(a, protocol.protocol, fmt.Sprintf(protocol.frame, invalid))
					assert.Same(t, previous, a.ReportedUsage, "invalid updates must preserve earlier reported usage")
				})
			}
			a := &relaycommon.HedgeAttempt{}
			observeHedgeUsage(a, protocol.protocol, fmt.Sprintf(protocol.frame, fmt.Sprintf(`{"%s":0}`, protocol.count)))
			require.NotNil(t, a.ReportedUsage, "an explicit numeric zero is reported usage")
			assert.Zero(t, a.ReportedUsage.TotalTokens)
		})
	}
}

func TestHedgeClaudeCachePartialUpdatesPreserveEvidence(t *testing.T) {
	a := &relaycommon.HedgeAttempt{}
	observeHedgeUsage(a, StreamProtocolAnthropic, `{"type":"message_start","message":{"usage":{"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20},"cache_read_input_tokens":30}}}`)
	require.NotNil(t, a.ReportedUsage)
	require.NotNil(t, a.ReportedUsage.BillingUsage)
	initial := a.ReportedUsage
	// Decoding can fail after mutating nested fields; the prior snapshot must survive.
	observeHedgeUsage(a, StreamProtocolAnthropic, `{"type":"message_delta","usage":{"cache_creation":{"ephemeral_5m_input_tokens":999},"server_tool_use":"invalid","output_tokens":7}}`)
	assert.Same(t, initial, a.ReportedUsage)
	assert.Equal(t, 10, initial.BillingUsage.ClaudeUsage.CacheCreation.Ephemeral5mInputTokens)
	observeHedgeUsage(a, StreamProtocolAnthropic, `{"type":"message_delta","usage":{"cache_creation":{"ephemeral_1h_input_tokens":25},"output_tokens":7}}`)
	usage := a.ReportedUsage.BillingUsage.ClaudeUsage
	assert.Equal(t, 10, usage.CacheCreation.Ephemeral5mInputTokens)
	assert.Equal(t, 25, usage.CacheCreation.Ephemeral1hInputTokens)
	assert.Equal(t, 30, usage.CacheReadInputTokens)
	assert.Equal(t, 7, usage.OutputTokens)
	assert.Equal(t, 20, initial.BillingUsage.ClaudeUsage.CacheCreation.Ephemeral1hInputTokens)
}

func TestHedgeGeminiWrappedUsageMatchesUnwrapped(t *testing.T) {
	payload := `{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"toolUsePromptTokenCount":2,"candidatesTokenCount":3,"thoughtsTokenCount":4,"cachedContentTokenCount":5,"totalTokenCount":19}}`
	plain := &relaycommon.HedgeAttempt{}
	wrapped := &relaycommon.HedgeAttempt{}
	observeHedgeUsage(plain, StreamProtocolGemini, payload)
	observeHedgeUsage(wrapped, StreamProtocolGemini, `{"usageMetadata":{"promptTokenCount":999},"response":`+payload+`}`)
	require.NotNil(t, wrapped.ReportedUsage)
	assert.Equal(t, plain.ReportedUsage, wrapped.ReportedUsage)
	assert.Equal(t, 12, wrapped.ReportedUsage.PromptTokens)
	assert.Equal(t, 7, wrapped.ReportedUsage.CompletionTokens)
	assert.Equal(t, 5, wrapped.ReportedUsage.PromptTokensDetails.CachedTokens)
	assert.True(t, wrapped.Completed)
	unknown := &relaycommon.HedgeAttempt{}
	observeHedgeUsage(unknown, StreamProtocolGemini, `{"usageMetadata":{"promptTokenCount":999},"response":{"usageMetadata":{}}}`)
	assert.Nil(t, unknown.ReportedUsage, "the wrapper, as in the stream gate, takes precedence")
}

func TestHedgeResponsesUsageKeepsCacheDetails(t *testing.T) {
	a := &relaycommon.HedgeAttempt{}
	observeHedgeUsage(a, StreamProtocolOpenAIResponses, `{"type":"response.completed","response":{"usage":{"input_tokens":100,"output_tokens":5,"input_tokens_details":{"cached_tokens":80,"cache_write_tokens":10}}}}`)
	require.NotNil(t, a.ReportedUsage)
	assert.Equal(t, 100, a.ReportedUsage.PromptTokens)
	assert.Equal(t, 5, a.ReportedUsage.CompletionTokens)
	assert.Equal(t, 80, a.ReportedUsage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 10, a.ReportedUsage.PromptTokensDetails.CacheWriteTokens)
}

func TestHedgePrefixRequiresAcceptedProtocolData(t *testing.T) {
	for _, test := range []struct {
		name     string
		protocol StreamProtocol
		data     string
		eligible bool
	}{
		{"chat startup", StreamProtocolOpenAIChat, `{"choices":[{"delta":{"role":"assistant"}}]}`, true},
		{"responses startup", StreamProtocolOpenAIResponses, `{"type":"response.created","response":{}}`, true},
		{"claude startup", StreamProtocolAnthropic, `{"type":"message_start","message":{"usage":{"input_tokens":2}}}`, true},
		{"gemini usage", StreamProtocolGemini, `{"usageMetadata":{"promptTokenCount":2}}`, true},
		{"wrapped gemini usage", StreamProtocolGemini, `{"response":{"usageMetadata":{"promptTokenCount":2}}}`, true},
		{"wrapped gemini content", StreamProtocolGemini, `{"response":{"candidates":[{"content":{"parts":[{"text":"answer"}]}}]}}`, true},
		{"wrapped gemini completion", StreamProtocolGemini, `{"response":{"candidates":[{"finishReason":"STOP"}]}}`, true},
		{"claude block startup", StreamProtocolAnthropic, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, true},
		{"chat content", StreamProtocolOpenAIChat, `{"choices":[{"delta":{"content":"answer"}}]}`, true},
		{"chat completion", StreamProtocolOpenAIChat, `{"choices":[{"finish_reason":"stop"}]}`, true},
		{"responses content", StreamProtocolOpenAIResponses, `{"type":"response.output_text.delta","delta":"answer"}`, true},
		{"responses completion", StreamProtocolOpenAIResponses, `{"type":"response.completed","response":{}}`, true},
		{"claude content", StreamProtocolAnthropic, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"answer"}}`, true},
		{"claude completion", StreamProtocolAnthropic, `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`, true},
		{"heartbeat", StreamProtocolOpenAIChat, `ping`, false},
		{"arbitrary json", StreamProtocolOpenAIChat, `{"status":"waiting"}`, false},
		{"provider error", StreamProtocolOpenAIChat, `{"error":{"message":"rejected"}}`, false},
		{"failed response", StreamProtocolOpenAIResponses, `{"type":"response.failed","response":{"error":{"message":"rejected"}}}`, false},
		{"unknown response event", StreamProtocolOpenAIResponses, `{"type":"response.custom_keepalive","response":{}}`, false},
		{"missing response startup payload", StreamProtocolOpenAIResponses, `{"type":"response.created"}`, false},
		{"null response startup payload", StreamProtocolOpenAIResponses, `{"type":"response.created","response":null}`, false},
		{"missing claude startup payload", StreamProtocolAnthropic, `{"type":"message_start"}`, false},
		{"empty claude startup payload", StreamProtocolAnthropic, `{"type":"message_start","message":{}}`, false},
		{"unknown claude block event", StreamProtocolAnthropic, `{"type":"content_block_ping"}`, false},
		{"missing claude block payload", StreamProtocolAnthropic, `{"type":"content_block_start","index":0}`, false},
		{"empty chat choices", StreamProtocolOpenAIChat, `{"choices":[]}`, false},
		{"invalid chat choice", StreamProtocolOpenAIChat, `{"choices":[{}]}`, false},
		{"chat choices object", StreamProtocolOpenAIChat, `{"choices":{"delta":{"role":"assistant"}}}`, false},
		{"empty chat usage", StreamProtocolOpenAIChat, `{"usage":{}}`, false},
		{"invalid chat count", StreamProtocolOpenAIChat, `{"usage":{"prompt_tokens":[0]}}`, false},
		{"null chat count", StreamProtocolOpenAIChat, `{"usage":{"prompt_tokens":null}}`, false},
		{"empty gemini candidates", StreamProtocolGemini, `{"candidates":[]}`, false},
		{"gemini candidates object", StreamProtocolGemini, `{"candidates":{"content":{"role":"model","parts":[]}}}`, false},
		{"empty gemini usage", StreamProtocolGemini, `{"response":{"usageMetadata":{}}}`, false},
		{"wrapped gemini error", StreamProtocolGemini, `{"response":{"error":{"message":"rejected"},"usageMetadata":{"promptTokenCount":2}}}`, false},
		{"bare terminal", StreamProtocolOpenAIChat, `[DONE]`, false},
		{"malformed", StreamProtocolOpenAIChat, `{"choices":`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, resp, info := setupStreamTest(t, strings.NewReader("data: "+test.data+"\n\n"))
			resp.StatusCode = http.StatusOK
			resp.Header = http.Header{"Content-Type": []string{"text/event-stream"}}
			info.Hedge = &relaycommon.HedgeAttempt{}
			StreamScannerHandlerWithGate(c, resp, info, test.protocol, func(_ string, _ *StreamResult) {})
			assert.Equal(t, test.eligible, info.Hedge.ValidPrefix.Load())
		})
	}
}
