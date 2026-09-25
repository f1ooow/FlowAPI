# Stream Precommit Gate

## 1. Scope / Trigger

Apply when changing frame classification in `relay/helper/stream_gate.go`, the
gated scanner in `relay/helper/stream_scanner.go`, any adaptor that calls
`helper.StreamScannerHandlerWithGate`, or retry classification that consumes
the gate result. The gate is FlowAPI-specific (upstream new-api has none). It
holds the start of an upstream stream so a failure before the first content
can switch channels while the client has received nothing.

## 2. Signatures

- `helper.StreamScannerHandlerWithGate(c, resp, info, protocol, dataHandler) *types.NewAPIError`.
  Gated protocols: `StreamProtocolAnthropic`, `StreamProtocolOpenAIChat`,
  `StreamProtocolOpenAIResponses`, `StreamProtocolGemini`,
  `StreamProtocolOpenAIImage` (including the chat↔responses conversion paths).
- `classifyStreamFrame(protocol, data) streamFrameVerdict`: neutral, content,
  error, completed, terminal, malformed.
- Prefix caps: `streamGateEventCap = 64` JSON frames (non-JSON keepalives from
  `isStreamKeepAlive` are not counted); `streamGateByteCap = 10 << 20` bytes
  excluding Responses request-echo frames; `2 × streamGateByteCap` including
  them.
- Precommit failure: `*StreamPrecommitFailure{Reason}` returned as a 502
  `ErrorCodeBadResponseBody`; `service.ClassifyRelayRetry` maps it to
  `invalid_precommit_upstream_response` (retryable, auto-ban eligible).
- Cap release log (warning): `stream gate opened at prefix cap without content: protocol=%s events=%d bytes=%d`.

## 3. Contracts

- Before commit the gate writes nothing to the client: no headers and no
  FlowAPI ping.
- Commit publishes SSE headers, marks `StreamStatus` committed, replays every
  held frame exactly once in upstream order, then streams through. After
  commit a failure is never replayed on another channel
  (`ClassifyRelayRetry` → `response_committed`).
- Commit points: the first content or completed frame, a terminal frame after
  at least one held frame (a content-free completion), and the moment a prefix
  cap is reached.
- Anthropic content includes any `content_block_delta` whose `delta.type` is
  `thinking_delta`, even with empty text. Claude Code sends
  `redact-thinking-2026-02-12` by default, so thinking arrives as empty
  `thinking_delta` frames (about 1.4 s apart) followed by `signature_delta`.
  `message_start`, the thinking `content_block_start` and `ping` stay neutral.
  `message_delta` with a non-empty `stop_reason` is a completed frame
  (covers refusals without content blocks).
- Unrecognized frames never fail an attempt. Only explicit signals fail over
  before commit: error frame, malformed JSON frame, terminal frame before any
  neutral frame or EOF before commit (`empty_stream`), idle timeout. The idle
  timeout is the channel's `streaming_idle_timeout_seconds`, or the global
  `STREAMING_TIMEOUT` when that is 0 or missing
  ([channel-reliability.md](./channel-reliability.md)).
- No time-based release. Production data (09-02 to 09-25) shows Codex
  pre-content upstream errors arriving p50 43 s / p90 133 s after attempt
  start; releasing after 10 / 30 / 60 s would have turned 34% / 23% / 12% of
  rescued Codex requests into client-visible errors.

## 4. Validation & Error Matrix

| Frame or event before commit | Behavior |
| --- | --- |
| Content or completed frame | Commit |
| Anthropic `thinking_delta`, empty text | Commit |
| Neutral frame under the caps | Hold |
| Event cap or byte cap reached | Commit + warning log |
| Error frame (`error` field or error type) | 502 `upstream_error`, retryable |
| Invalid JSON that claims to be JSON | 502 `malformed_event`, retryable |
| Terminal after held frames | Commit |
| Terminal as the first frame, or EOF | 502 `empty_stream`, retryable |
| Idle timeout elapses (channel value, else `STREAMING_TIMEOUT`) | 502 `idle_timeout`, retryable |
| Non-JSON `data: ping` | Held; excluded from the event cap, counted in byte caps |
| Client disconnect | Stop; no failover; not a channel failure |

## 5. Good / Base / Bad Cases

- Good: Claude Code long thinking (`message_start`, empty thinking start,
  100+ empty `thinking_delta`, signature, text) opens at the first
  `thinking_delta`, within 1–2 s; the client sees the stream immediately.
- Good: a Codex relay sends `response.created`, then an error 40 s later; the
  request fails over and the client receives one clean answer.
- Base: reasoning in a field the classifier does not know (OpenAI Chat
  `delta.reasoning`) fills 64 frames in about 2 s and is released at the cap;
  the request succeeds on the same channel.
- Bad: treating "too many neutral frames" as an upstream failure. From 09-02
  to 09-25, 173 `/v1/messages` requests hit `prebuffer_overflow` around 90 s,
  119 failed, and the failures fed channel auto-ban.
- Bad: adding a time-based release to shorten the client wait; it converts
  late but rescuable Codex errors into client-visible errors.

## 6. Tests Required

In `relay/helper/stream_scanner_test.go`, with testify, deterministic, and
also under `-race`:

- Empty thinking delta opens the gate: the fake upstream blocks after the
  first empty `thinking_delta` until the client has received a frame; the
  remaining stream arrives byte-for-byte in order.
- Prefix cap releases held frames: 65 unrecognized neutral JSON frames, then
  content → no error, output equals upstream, exact frame count.
- Failure before content stays retryable: error after the thinking block
  start, truncated `thinking_delta`, EOF before content, error after more than
  64 heartbeats → 502, not committed, empty body,
  `ClassifyRelayRetry(...).Retryable == true`.
- Idle timeout: before commit → 502 `idle_timeout`, retryable, nothing
  written; after commit → no error, stream ends with `EndReason=timeout`,
  output written once.

## 7. Wrong vs Correct

### Wrong

```go
if bufferedEvents > streamGateEventCap {
	err := &StreamPrecommitFailure{Reason: "prebuffer_overflow"}
	setPrecommitErr(err) // attempt fails, channel failure counted
	return
}
```

Adding one more per-provider field rule on top of this only moves the outage
to the next unrecognized progress frame.

### Correct

```go
if capReached {
	logger.LogWarn(c, "stream gate opened at prefix cap without content: ...")
	if !commitBufferedStream() {
		return
	}
	continue
}
```

Field rules (such as `thinking_delta`) are a fast path for early commit; the
cap release is the safety net that keeps a missed rule from becoming a failure.
