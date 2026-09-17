package helper

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/tidwall/gjson"
)

// A protocol startup event is enough to retain a loser for accounting, but
// headers, keepalive noise and arbitrary JSON are not evidence of acceptance.
func validHedgePrefix(protocol StreamProtocol, data string) bool {
	switch classifyStreamFrame(protocol, data) {
	case streamFrameContent, streamFrameCompleted:
		return true
	case streamFrameError, streamFrameMalformed, streamFrameTerminal:
		return false
	}
	if !gjson.Valid(data) {
		return false
	}
	root := gjson.Parse(data)
	switch protocol {
	case StreamProtocolOpenAIChat:
		if choices := root.Get("choices"); choices.IsArray() {
			for _, choice := range choices.Array() {
				if choice.Get("delta").IsObject() && choice.Get("delta.role").String() == "assistant" {
					return true
				}
			}
		}
		return hasReportedHedgeUsage(protocol, root.Get("usage"))
	case StreamProtocolOpenAIResponses:
		return isStreamRequestEcho(protocol, data) && root.Get("response").IsObject()
	case StreamProtocolAnthropic:
		switch root.Get("type").String() {
		case "message_start":
			message := root.Get("message")
			return message.IsObject() && (message.Get("type").String() == "message" ||
				message.Get("role").String() == "assistant" || hasReportedHedgeUsage(protocol, message.Get("usage")))
		case "content_block_start":
			block := root.Get("content_block")
			switch block.Get("type").String() {
			case "text":
				return block.Get("text").Type == gjson.String
			case "thinking":
				return block.Get("thinking").Type == gjson.String
			case "tool_use":
				return block.Get("id").Type == gjson.String && block.Get("name").Type == gjson.String && block.Get("input").IsObject()
			}
		}
	case StreamProtocolGemini:
		if response := root.Get("response"); response.IsObject() {
			root = response
		}
		if candidates := root.Get("candidates"); candidates.IsArray() {
			for _, candidate := range candidates.Array() {
				if candidate.Get("content.role").String() == "model" && candidate.Get("content.parts").IsArray() {
					return true
				}
			}
		}
		return hasReportedHedgeUsage(protocol, root.Get("usageMetadata"))
	}
	return false
}

// Object presence alone does not distinguish unknown usage from reported zero.
// Only recognized numeric token counts provide metering evidence.
func hasReportedHedgeUsage(protocol StreamProtocol, value gjson.Result) bool {
	if !value.IsObject() {
		return false
	}
	var paths []string
	switch protocol {
	case StreamProtocolOpenAIChat:
		paths = []string{"prompt_tokens", "completion_tokens", "total_tokens", "prompt_cache_hit_tokens",
			"prompt_tokens_details.cached_tokens", "prompt_tokens_details.cache_write_tokens", "prompt_tokens_details.cached_creation_tokens",
			"prompt_tokens_details.text_tokens", "prompt_tokens_details.audio_tokens", "prompt_tokens_details.image_tokens",
			"completion_tokens_details.reasoning_tokens", "completion_tokens_details.text_tokens", "completion_tokens_details.audio_tokens", "completion_tokens_details.image_tokens"}
	case StreamProtocolOpenAIResponses:
		paths = []string{"input_tokens", "output_tokens", "total_tokens", "input_tokens_details.cached_tokens", "input_tokens_details.cache_write_tokens"}
	case StreamProtocolAnthropic:
		paths = []string{"input_tokens", "output_tokens", "cache_creation_input_tokens", "cache_read_input_tokens",
			"claude_cache_creation_5_m_tokens", "claude_cache_creation_1_h_tokens",
			"cache_creation.ephemeral_5m_input_tokens", "cache_creation.ephemeral_1h_input_tokens"}
	case StreamProtocolGemini:
		paths = []string{"promptTokenCount", "toolUsePromptTokenCount", "candidatesTokenCount", "totalTokenCount", "thoughtsTokenCount", "cachedContentTokenCount",
			"promptTokensDetails.#.tokenCount", "toolUsePromptTokensDetails.#.tokenCount", "candidatesTokensDetails.#.tokenCount"}
	}
	reported := false
	for _, path := range paths {
		result := value.Get(path)
		counts := []gjson.Result{result}
		if strings.Contains(path, ".#.") {
			counts = result.Array()
		}
		for _, count := range counts {
			if !count.Exists() {
				continue
			}
			n, err := strconv.Atoi(count.Raw)
			if count.Type != gjson.Number || err != nil || n < 0 {
				return false
			}
			reported = true
		}
	}
	return reported
}

// Observe before the content gate: message_start can contain billable cache
// usage even if a candidate never produces content. Never estimate loser usage.
func observeHedgeUsage(a *relaycommon.HedgeAttempt, protocol StreamProtocol, data string) {
	verdict := classifyStreamFrame(protocol, data)
	if verdict == streamFrameError || verdict == streamFrameMalformed {
		return
	}
	if verdict == streamFrameCompleted || verdict == streamFrameTerminal {
		a.Completed = true
	}
	root := gjson.Parse(data)
	switch protocol {
	case StreamProtocolOpenAIChat:
		value := root.Get("usage")
		if hasReportedHedgeUsage(protocol, value) {
			var usage dto.Usage
			if common.UnmarshalJsonStr(value.Raw, &usage) == nil {
				a.ReportedUsage = &usage
			}
		}
	case StreamProtocolOpenAIResponses:
		value := root.Get("response.usage")
		if hasReportedHedgeUsage(protocol, value) {
			var usage dto.Usage
			if common.UnmarshalJsonStr(value.Raw, &usage) == nil {
				usage.PromptTokens = usage.InputTokens
				usage.CompletionTokens = usage.OutputTokens
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
				usage.PromptTokensDetails.CachedTokens = int(value.Get("input_tokens_details.cached_tokens").Int())
				usage.PromptTokensDetails.CacheWriteTokens = int(value.Get("input_tokens_details.cache_write_tokens").Int())
				a.ReportedUsage = &usage
			}
		}
	case StreamProtocolAnthropic:
		value := root.Get("usage")
		if root.Get("type").String() == "message_start" {
			value = root.Get("message.usage")
		}
		if hasReportedHedgeUsage(protocol, value) {
			var usage dto.ClaudeUsage
			if a.ReportedUsage != nil && a.ReportedUsage.BillingUsage != nil && a.ReportedUsage.BillingUsage.ClaudeUsage != nil {
				usage = *dto.CloneBillingUsage(a.ReportedUsage.BillingUsage).ClaudeUsage
			}
			if common.UnmarshalJsonStr(value.Raw, &usage) == nil {
				a.ReportedUsage = &dto.Usage{BillingUsage: dto.NewClaudeMessagesBillingUsage(&usage), UsageSemantic: "anthropic"}
			}
		}
	case StreamProtocolGemini:
		if response := root.Get("response"); response.IsObject() {
			root = response
		}
		value := root.Get("usageMetadata")
		if hasReportedHedgeUsage(protocol, value) {
			var usage dto.GeminiUsageMetadata
			if common.UnmarshalJsonStr(value.Raw, &usage) == nil {
				a.ReportedUsage = relayconvert.UsageFromGeminiMetadata(&usage, 0)
			}
		}
	}
}
