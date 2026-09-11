package helper

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

const (
	streamGateEventCap = 64
	streamGateByteCap  = 10 << 20
)

type StreamProtocol string

const (
	StreamProtocolAnthropic       StreamProtocol = "anthropic"
	StreamProtocolOpenAIChat      StreamProtocol = "openai_chat"
	StreamProtocolOpenAIResponses StreamProtocol = "openai_responses"
	StreamProtocolGemini          StreamProtocol = "gemini"
	StreamProtocolOpenAIImage     StreamProtocol = "openai_image"
)

type StreamPrecommitFailure struct {
	Reason string
	Frame  string
}

func (e *StreamPrecommitFailure) Error() string {
	if e == nil {
		return "stream failed before first valid content"
	}
	if e.Frame == "" {
		return fmt.Sprintf("stream failed before first valid content: %s", e.Reason)
	}
	return fmt.Sprintf("stream failed before first valid content: %s: %s", e.Reason, e.Frame)
}

type streamFrameVerdict int

const (
	streamFrameNeutral streamFrameVerdict = iota
	streamFrameContent
	streamFrameError
	// streamFrameCompleted marks an explicit upstream completion signal
	// (finish_reason, finishReason, stop_reason, response.completed). A
	// generation that legitimately produces no content — content filter,
	// immediate stop sequence, empty tool turn — ends this way, so it must be
	// released to the client instead of being treated as a broken stream.
	streamFrameCompleted
	streamFrameTerminal
	streamFrameMalformed
)

func truncateStreamFrame(data string) string {
	const limit = 2000
	if len(data) <= limit {
		return data
	}
	return data[:limit]
}

func classifyStreamFrame(protocol StreamProtocol, data string) streamFrameVerdict {
	data = strings.TrimSpace(data)
	if data == "[DONE]" {
		return streamFrameTerminal
	}
	if data == "" {
		return streamFrameNeutral
	}
	if data[0] != '{' && data[0] != '[' {
		// Non-JSON payloads are proxy/keepalive noise (`data: ping`), not a
		// broken upstream. Only a payload that claims to be JSON and fails to
		// parse indicates a corrupted frame.
		return streamFrameNeutral
	}
	if !gjson.Valid(data) {
		return streamFrameMalformed
	}
	frameType := gjson.Get(data, "type").String()
	if streamPathNonEmpty(data, "error") {
		return streamFrameError
	}

	switch protocol {
	case StreamProtocolAnthropic:
		if frameType == "error" || frameType == "response.error" {
			return streamFrameError
		}
		if frameType == "message_delta" && streamPathNonEmpty(data, "delta.stop_reason") {
			return streamFrameCompleted
		}
		if frameType == "message_stop" {
			return streamFrameTerminal
		}
		if frameType == "content_block_delta" && streamAnyPathNonEmpty(data,
			"delta.text", "delta.partial_json", "delta.thinking", "delta.signature", "delta.citation") {
			return streamFrameContent
		}
		if frameType == "content_block_start" && streamAnyPathNonEmpty(data,
			"content_block.data", "content_block.content", "content_block.file_id", "content_block.fileId",
			"content_block.url", "content_block.result") {
			return streamFrameContent
		}
	case StreamProtocolOpenAIChat:
		if streamAnyPathNonEmpty(data,
			"choices.#.delta.content", "choices.#.delta.reasoning_content",
			"choices.#.delta.tool_calls.#.function.arguments", "choices.#.delta.function_call.arguments",
			"choices.#.delta.refusal", "choices.#.delta.audio.data", "choices.#.delta.audio.transcript") {
			return streamFrameContent
		}
		if streamPathNonEmpty(data, "choices.#.finish_reason") {
			return streamFrameCompleted
		}
	case StreamProtocolOpenAIResponses:
		if frameType == "error" || frameType == "response.error" || frameType == "response.failed" || streamPathNonEmpty(data, "response.error") {
			return streamFrameError
		}
		if streamResponsesContent(frameType, data) {
			return streamFrameContent
		}
		if frameType == "response.completed" || frameType == "response.incomplete" {
			return streamFrameCompleted
		}
		if frameType == "response.done" {
			return streamFrameTerminal
		}
	case StreamProtocolGemini:
		payload := data
		if gjson.Get(data, "response").IsObject() {
			payload = gjson.Get(data, "response").Raw
		}
		if streamPathNonEmpty(payload, "error") || streamPathNonEmpty(payload, "promptFeedback.blockReason") {
			return streamFrameError
		}
		if streamAnyPathNonEmpty(payload,
			"candidates.#.content.parts.#.text", "candidates.#.content.parts.#.inlineData.data",
			"candidates.#.content.parts.#.fileData.fileUri", "candidates.#.content.parts.#.functionCall.name",
			"candidates.#.content.parts.#.functionResponse.name", "candidates.#.content.parts.#.executableCode.code",
			"candidates.#.content.parts.#.codeExecutionResult.output") {
			return streamFrameContent
		}
		if streamPathNonEmpty(payload, "candidates.#.finishReason") {
			return streamFrameCompleted
		}
	case StreamProtocolOpenAIImage:
		if frameType == "error" || frameType == "upstream_error" {
			return streamFrameError
		}
		if (strings.HasPrefix(frameType, "image_generation.") || strings.HasPrefix(frameType, "image_edit.")) &&
			streamAnyPathNonEmpty(data, "b64_json", "partial_image_b64", "url", "result") {
			return streamFrameContent
		}
	}
	return streamFrameNeutral
}

func streamResponsesContent(frameType string, data string) bool {
	if strings.HasSuffix(frameType, ".delta") && streamPathNonEmpty(data, "delta") {
		return true
	}
	if frameType == "response.image_generation_call.partial_image" && streamPathNonEmpty(data, "partial_image_b64") {
		return true
	}
	switch frameType {
	case "response.output_text.done", "response.reasoning_text.done", "response.reasoning_summary_text.done":
		return streamPathNonEmpty(data, "text")
	case "response.audio.transcript.done":
		return streamAnyPathNonEmpty(data, "transcript", "text")
	case "response.refusal.done":
		return streamPathNonEmpty(data, "refusal")
	case "response.function_call_arguments.done", "response.mcp_call_arguments.done":
		return streamPathNonEmpty(data, "arguments")
	case "response.custom_tool_call_input.done":
		return streamPathNonEmpty(data, "input")
	case "response.code_interpreter_call_code.done":
		return streamPathNonEmpty(data, "code")
	case "response.output_item.added", "response.output_item.done":
		return streamAnyPathNonEmpty(data, "item.content.#.text", "item.summary.#.text", "item.arguments", "item.input", "item.action", "item.queries", "item.query", "item.code", "item.command", "item.operation", "item.result")
	case "response.completed":
		return streamAnyPathNonEmpty(data, "response.output.#.content.#.text", "response.output.#.summary.#.text", "response.output.#.arguments", "response.output.#.input", "response.output.#.action", "response.output.#.queries", "response.output.#.query", "response.output.#.code", "response.output.#.command", "response.output.#.operation", "response.output.#.result")
	}
	return false
}

// isStreamKeepAlive reports whether a buffered payload is proxy/keepalive noise
// rather than an upstream protocol event. Some third-party proxies emit
// `data: ping` every few seconds; counting those against streamGateEventCap
// would make a slow-first-token reasoning request fail with prebuffer_overflow
// purely as a function of elapsed time. They still count against the byte caps
// so an endless heartbeat cannot grow the buffer without bound.
func isStreamKeepAlive(data string) bool {
	data = strings.TrimSpace(data)
	if data == "" {
		return true
	}
	return data[0] != '{' && data[0] != '['
}

func isStreamRequestEcho(protocol StreamProtocol, data string) bool {
	if protocol != StreamProtocolOpenAIResponses {
		return false
	}
	switch gjson.Get(data, "type").String() {
	case "response.created", "response.in_progress", "response.queued":
		return true
	default:
		return false
	}
}

func streamAnyPathNonEmpty(data string, paths ...string) bool {
	for _, path := range paths {
		if streamPathNonEmpty(data, path) {
			return true
		}
	}
	return false
}

func streamPathNonEmpty(data string, path string) bool {
	result := gjson.Get(data, path)
	if !result.Exists() || result.Type == gjson.Null {
		return false
	}
	if result.IsArray() {
		for _, item := range result.Array() {
			if item.Type != gjson.Null && strings.TrimSpace(item.String()) != "" && item.Raw != "[]" && item.Raw != "{}" {
				return true
			}
		}
		return false
	}
	if result.IsObject() {
		return result.Raw != "{}"
	}
	return strings.TrimSpace(result.String()) != ""
}
