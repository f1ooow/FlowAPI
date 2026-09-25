# Research: Why only Anthropic? Neutral-frame gaps in the other gated protocols, and when real pre-content failures happen

- **Query**: Do OpenAI Chat, OpenAI Responses, Gemini and OpenAI Image have frames that show the model is alive or reasoning, but that the gate classifies as neutral, so they can pile up past 64 before first content? Compare with CCH, check production, give a verdict per protocol. Addendum from the coordinator: how late do real pre-content failures happen, and would committing on a deadline or cap overflow break the data handlers when frames are replayed?
- **Scope**: mixed (FlowAPI code, CCH source, HK production logs and database read-only, vendor docs)
- **Date**: 2026-09-25
- **Code base**: FlowAPI working tree at `f3f7a8129`. The gate code went into git in `845a85998` (2026-09-11 13:18). Production ran an uncommitted build of the gate from 2026-09-02. The HK app image today is `flowapi:hk-media-8156149df-20260925`.

## Conclusion

| Protocol | Reasoning or progress frames the gate counts as content | Reasoning or progress frames the gate counts as neutral (toward the 64 cap) | Overflows in production, 09-02 to 09-25 | Verdict |
|---|---|---|---|---|
| Anthropic | `thinking_delta` with non-empty `delta.thinking`, `signature_delta`, text, `input_json`, `citation` (`stream_gate.go:97-99`) | `thinking_delta` with empty text (the redact-thinking beta), `message_start`, `content_block_start` for thinking, JSON `ping` | **364 attempts in 173 requests** on `/v1/messages` (#10: 266, #47: 55, #11: 27, #62: 16) | Needs a change now (this task) |
| OpenAI Chat | `delta.content`, `delta.reasoning_content`, tool/function arguments, refusal, audio (`stream_gate.go:107-111`) | **`delta.reasoning` and `delta.reasoning_details`**, role-only deltas, `choices: []`, usage-only chunks, a first tool-call chunk that has only the name | **4 attempts in 2 requests** (#61 兔子 DeepSeek, 09-14). Both recovered on #59. | The code gap is confirmed and the fix is small. Production impact so far is small. Recommend fixing now in the same change or right after. |
| OpenAI Responses | Any `*.delta` with non-empty `delta`, which covers `reasoning_summary_text.delta` and `reasoning_text.delta`; `*.done` with text; `output_item.*` with a payload (`stream_gate.go:160-182`) | Lifecycle events (`created`, `in_progress`, `queued`), `reasoning_summary_part.added/done`, `output_item.added/done` for a reasoning item that has only `encrypted_content`, `content_part.added`, hosted-tool progress events | **3 attempts in 3 requests** (#67 FAST PRO 订阅, one Codex CLI session on 09-21). The retry on the same channel succeeded each time. The frame types are unknown. | Low risk, defer. No confirmed classifier gap. |
| Gemini | Any part with non-empty `text`, including `thought: true` parts (`stream_gate.go:137-143`) | Parts that have only `thoughtSignature`, usage-only chunks | 0 | No gap |
| OpenAI Image | `image_generation.*` / `image_edit.*` with `b64_json`, `partial_image_b64`, `url` or `result` (`stream_gate.go:151-154`) | Any other `image_generation.*` event | 0 | No gap |

**Why the impact lands almost only on Anthropic (confirmed):** 364 of the 371 `prebuffer_overflow` attempts since 09-02 are Anthropic-protocol `/v1/messages`. Anthropic is the only protocol where a default client setting (Claude Code's `redact-thinking-2026-02-12`) turns every reasoning frame into a neutral frame.

**The one real analog (confirmed in code, weak evidence in production):** in OpenAI Chat, reasoning arrives in `delta.reasoning` / `delta.reasoning_details`. These frames stream at token rate, so a stream would overflow within seconds, not after about 90 s. Current vLLM and OpenRouter both use these fields. No OpenRouter channel exists in production today, and most chat traffic goes to DeepSeek, GLM, Qwen and Kimi relays that send `reasoning_content`, which is why only 4 attempts were hit.

---

## 1. Gated call sites and the protocol each one uses

`grep -rn StreamScannerHandlerWithGate` finds 7 call sites. The gate always classifies the raw **upstream** frames (`stream_scanner.go:401-402`), so the protocol follows the upstream wire format, not the client format.

| Call site | Protocol | Upstream format → client format | Reached from |
|---|---|---|---|
| `relay/channel/claude/relay-claude.go:203` (`ClaudeStreamHandler`) | `StreamProtocolAnthropic` | Anthropic → Claude or OpenAI | Anthropic channels (`claude/adaptor.go:127`), Vertex in Claude mode (`vertex/adaptor.go:343-344`), NewAPI channels (type 60) when the client format is Claude (`newapi/adaptor.go:106-107`) |
| `relay/channel/openai/relay-openai.go:128` (`OaiStreamHandler`) | `StreamProtocolOpenAIChat` | Chat → OpenAI, Claude or Gemini (`openai/helper.go:23-35`) | Default path of the OpenAI adaptor (`openai/adaptor.go:661-666`), which covers OpenAI-compatible types including NewAPI (type 60) and OpenRouter (type 20). Also deepseek, mistral, submodel, jimeng, vertex open-source (`vertex/adaptor.go:351-352`) and others. |
| `relay/channel/openai/responses_via_chat.go:91` (`OaiChatToResponsesStreamHandler`) | `StreamProtocolOpenAIChat` | Chat → Responses | `advancedcustom/adaptor.go:328` |
| `relay/channel/openai/relay_responses.go:88` (`OaiResponsesStreamHandler`) | `StreamProtocolOpenAIResponses` | Responses → Responses | `openai/adaptor.go:653-658`, codex (`codex/adaptor.go:125`), xai, cloudflare |
| `relay/channel/openai/chat_via_responses.go:270` (`OaiResponsesToChatStreamHandler`) | `StreamProtocolOpenAIResponses` | Responses → Chat, Claude or Gemini (`chat_via_responses.go:197-263`) | `relay/chat_completions_via_responses.go:160-161`, `advancedcustom/adaptor.go:323` |
| `relay/channel/gemini/relay-gemini.go:153` (`geminiStreamHandler`) | `StreamProtocolGemini` | Gemini → Gemini, OpenAI or Responses | `gemini/adaptor.go:271,289`, `gemini/relay_responses.go:126`, `vertex/adaptor.go:347-349` |
| `relay/channel/openai/relay_image.go:116` (`OpenaiImageStreamHandler`) | `StreamProtocolOpenAIImage` | image SSE → image SSE | `openai/adaptor.go:645-647` |

`relay/helper/hedge_usage.go:17,119` also calls `classifyStreamFrame`, but only to accept a hedge prefix and observe usage.

Consequence: a Chat-protocol gap affects not only `/v1/chat/completions`. It also affects `/v1/messages` and Gemini clients routed to OpenAI-compatible channels, and Responses clients on the chat-conversion path.

## 2. How each protocol classifies reasoning and progress frames

Shared rules (`relay/helper/stream_gate.go`):
- `[DONE]` is terminal (`:66-68`). Empty data is neutral (`:69-71`).
- Non-JSON data (`data: ping`) is neutral (`:72-77`) and is exempt from the event count (`isStreamKeepAlive` at `:193-199`, used at `stream_scanner.go:439-441`). It still counts toward the byte caps.
- Invalid JSON is malformed (`:78-80`). A non-empty top-level `error` is an error (`:82-84`).
- Emptiness is checked by `streamPathNonEmpty` (`:222-239`): the string must be non-blank, the array must have a non-blank element, and the object must not be `{}`.
- A neutral frame is buffered (`stream_scanner.go:430`). Only JSON frames that are not keepalives increment `bufferedEvents` (`:439-441`). `> 64` means overflow (`:442-447`).
- Responses request-echo frames (`response.created`, `in_progress`, `queued`) are excluded from `bufferedBytes`, but they **do** count as events (`:432-434`, `stream_gate.go:201-211`).

### 2.1 Anthropic (for reference)
- A `content_block_delta` is content only if one of `delta.text | partial_json | thinking | signature | citation` is non-empty (`:97-99`). So a `thinking_delta` with `thinking: ""` is **neutral**. This is the gap the task fixes.
- A `content_block_start` is content only if it carries a payload (`data | content | file_id | fileId | url | result`) (`:101-104`). A `redacted_thinking` block has `data`, so it is content. A thinking block start is neutral.
- A `message_delta` with any `stop_reason` counts as completed and commits (`:91-93`). `message_stop` is terminal (`:94-96`).
- JSON `{"type":"ping"}` is neutral and counted. This is out of scope per the PRD.

### 2.2 OpenAI Chat (`:106-115`)

| Frame | Verdict | Evidence |
|---|---|---|
| `delta.content` non-empty | content | `:108` |
| `delta.reasoning_content` non-empty (DeepSeek, Qwen, GLM, Kimi style) | content | `:108`; same as CCH #1394 |
| **`delta.reasoning`** (current vLLM, OpenRouter) | **neutral** | not in `:107-111` |
| **`delta.reasoning_details`** (OpenRouter array of `reasoning.text` / `reasoning.encrypted` / `reasoning.summary`) | **neutral** | not in `:107-111` |
| role-only `{"delta":{"role":"assistant"}}` | neutral | test `stream_scanner_test.go:529` |
| `{"choices":[],"usage":{...}}` (usage-only or empty choices) | neutral | test `:545` |
| `finish_reason` non-empty | completed, commits | `:113-115`, test `:543` |
| first tool-call chunk with only `function.name` and `arguments: ""` | neutral (the next argument chunk is content) | `:109` |

FlowAPI's own code already treats `delta.reasoning` as reasoning, but the gate does not:
- The DTO has both `reasoning_content` and `reasoning` (`relaykit/dto/openai_response.go:89-93`). `GetReasoningContent()` falls back to `Reasoning` (`:107-115`). The field was added in `aa6c894f5` (2025-03-06), "fix: adapting return format for openrouter think content (#793)".
- Token accounting counts it (`relay/channel/openai/helper.go:93-97`).
- The Responses conversion emits it as reasoning deltas (`relaykit/relayconvert/internal/oai_chat/to_oai_responses_stream_resp.go:93-94`).
- The Claude conversion turns it into thinking (`to_claude_messages_resp.go:195,366`).
- The OpenRouter channel type explicitly asks upstream for reasoning (`relay/channel/openai/adaptor.go:251-293`).

External confirmation:
- vLLM docs (latest, "Reasoning Outputs"): "`reasoning` used to be called `reasoning_content`… your client code could silently read an empty `reasoning_content`, even when reasoning is populated". The streaming example chunk is `"delta": {"role": "assistant", "reasoning": "is"}`. https://docs.vllm.ai/en/latest/features/reasoning_outputs.html
- OpenRouter docs (Reasoning Tokens): "Streaming responses: `reasoning_details` appears in `choices[].delta.reasoning_details` for each chunk". https://openrouter.ai/docs/use-cases/reasoning-tokens

Reasoning chunks arrive at token rate, so a reasoning phase in these formats reaches 65 neutral frames within seconds.

### 2.3 OpenAI Responses (`:116-128`, `streamResponsesContent` at `:159-185`)

| Frame | Verdict | Evidence |
|---|---|---|
| any `*.delta` with non-empty `delta`: `output_text`, `reasoning_summary_text`, `reasoning_text`, `function_call_arguments`, `custom_tool_call_input`, audio, … | content | `:160-162` (a generic suffix rule, broader than CCH's list) |
| `response.reasoning_summary_part.added` / `.done` | neutral | not matched |
| `response.reasoning_summary_text.done` / `reasoning_text.done` / `output_text.done` with text | content | `:167-168` |
| `response.output_item.added` / `.done` for a `reasoning` item with only `encrypted_content` (and `summary: []`) | **neutral** | `item.encrypted_content` is not in `:180` |
| `output_item.*` with `content.#.text`, `summary.#.text`, `arguments`, `input`, `action`, `queries`, `query`, `code`, `command`, `operation`, `result` | content | `:179-180` |
| `response.created` / `in_progress` / `queued` | neutral, count as events, not as bytes | `stream_scanner.go:432-441` |
| `response.content_part.added` (empty part), hosted-tool progress (`web_search_call.in_progress/searching/completed`, `mcp_list_tools.*`) | neutral | not matched |
| `response.completed` with output text or payload | content (`:181-182`); otherwise completed, which commits (`:123-125`) | |
| `response.failed` / `error` / `response.error` / non-empty `response.error` | error | `:117-119` |

With summaries off, a reasoning model emits about 6 neutral frames before the first `output_text.delta`: `created`, `in_progress`, `output_item.added` (reasoning), `output_item.done` (reasoning), `output_item.added` (message), `content_part.added`. That is far below 64. Production agrees: the `sequence_number` values in pre-content Responses error frames cluster at 2, 4 and 5 (`seq=4`: 168, `seq=2`: 168, `seq=5`: 119, `seq=0`: 259, others ≤ 18).

### 2.4 Gemini (`:129-146`)
- An envelope `response` is unwrapped first (`:130-133`).
- `thought: true` parts carry `text`, so `candidates.#.content.parts.#.text` counts them as content whatever the `thought` flag says (`:137-138`).
- A part with only `thoughtSignature` (`relaykit/dto/gemini.go:275`) is neutral, but it is a single frame and does not pile up.
- Any `finishReason` counts as completed (`:144-146`). `promptFeedback.blockReason` is an error (`:134-136`).

### 2.5 OpenAI Image (`:147-154`)
- Content is an `image_generation.*` / `image_edit.*` event with `b64_json`, `partial_image_b64`, `url` or `result`.
- The upstream protocol sends only partial and completed events, so there are no progress frames to pile up.
- In production, image traffic is non-streaming: `/v1/images/edits` has 3,182 successful requests, all with `is_stream=f`.

## 3. Comparison with CCH (main `dfeb1433` v0.9.5, dev `47a915fa` 2026-09-25)

Files: `/tmp/cch-src/src/app/v1/_lib/proxy/stream-gate/frame-classifier.ts` (main) and `git show origin/dev:…`.

**Same in both:**
- Anthropic `content_block_delta` needs a non-empty `delta.thinking` (main `:52-59`), so **CCH has the same empty-thinking gap**. This matches the 14 CCH errors cited in the PRD.
- The OpenAI Chat content paths are identical (main `:103-111`). CCH added `reasoning_content` in `4b634e1d` (2026-08-04), "fix(proxy): recognize DeepSeek reasoning stream content, Fixes #1394". **Neither CCH nor FlowAPI recognizes `delta.reasoning` or `delta.reasoning_details`.**
- The Gemini content paths are identical (main `:206-214`).
- Responses request-echo frames are excluded from bytes but still counted as events (main `:285-304`).
- Default event cap is 64 (`src/lib/config/env.schema.ts:224`).

**CCH has, FlowAPI lacks:**
- Responses: a compaction item (`type: "compaction"` with non-empty `encrypted_content`) in `output_item.done` or `response.completed` counts as content (main `:465-467`, `:484-511`). In FlowAPI such a stream still commits at `response.completed` (completed), just later.
- Anthropic (dev only): `message_delta` with `stop_reason: "refusal"` counts as content (dev `:91-96`). FlowAPI already commits on any `stop_reason` (`stream_gate.go:91-93`).
- Gemini: an abnormal `finishReason` (SAFETY, RECITATION, …, OTHER) counts as an **error**, which triggers failover (main `:222-244`). FlowAPI releases every `finishReason` as completed.
- A gate mode switch (`off`/`shadow`/`enforce`), and on dev a high-concurrency bypass (`isStreamGatePrecommitActive`). This is out of scope for FlowAPI per the PRD.

**FlowAPI has, CCH lacks:**
- Responses: the generic `*.delta` rule (`stream_gate.go:160-162`) instead of CCH's list of 10 event types (main `:124-136`).
- Responses: `item.result` / `response.output.#.result` count as content (`:180,182`).
- A completion verdict for Chat `finish_reason`, Anthropic `stop_reason`, Gemini `finishReason` and Responses `completed`/`incomplete` (`:91-93,113-115,123-125,144-146`), plus terminal after any buffered frame, which commits (`stream_scanner.go:414-428`). CCH fails terminal-before-content as `empty_stream`, except for a clean or incomplete Responses completion (main stream-content-gate `:430-442`).
- Non-JSON data is neutral and **not counted** (`stream_gate.go:72-77`, `stream_scanner.go:439-441`). CCH treats it as malformed, which is a hard failure (frame-classifier main `:362-365`, dev `:370-372`). CCH also counts **every** frame, including JSON pings, toward the cap (stream-content-gate main `:414,448`).
- An OpenAI Image protocol. CCH has no family for it and fails open (`mapProviderTypeToFamily` at main `:260-277`).

## 4. Production evidence (HK, read-only)

**Method.** I read the app logs `/app/logs/oneapi-*.log` in container `flowapi` (09-02 09:34 to 09-25 17:09, UTC+8). I collected every request id with a `stream failed before first valid content` line (1,151 requests, 2,380 attempt-failure lines), pulled all lines for those ids through `grep -F -f /dev/stdin` over ssh, and joined them with Postgres `logs` (`frt`, `request_path`, route history) and `channels` (non-secret columns only). Nothing was written on the server. Local scratch files were in `/tmp/sg-research` and have been deleted.

Totals of `stream ended` lines with a gate reason:
- `upstream_error`: 1,098
- `prebuffer_overflow`: 371
- `empty_stream` with `reason=eof`: 332
- `malformed_event`: 4
- `idle_timeout`: **0**

The `channel error … empty_stream` lines number 533 because they also include client disconnects (`stream ended: reason=client_gone`), which is the logging-label issue listed as out of scope in the PRD.

### 4.1 Every `prebuffer_overflow` not on `/v1/messages`: 7 attempts, 5 requests

| Request | Path / protocol | Channel (type) | Model | Attempts → outcome | Timing | Most likely frames |
|---|---|---|---|---|---|---|
| `202609141410116957025498268d9d6cjKdKTZO` | `/v1/chat/completions` / OpenAIChat | #61 兔子 DeepSeek (60 NewAPI, `api.tu-zi.com`) | deepseek-v4.1-flash | #61 overflow ×2 (8 s, 5 s), then #59 SC DeepSeek succeeded (87 completion tokens, 18 s total) | first frame at `frt` 7.26 s, overflow logged at 8 s, so **65 JSON neutral frames arrived in under about 2 s** | inferred: per-token chunks in an unrecognized reasoning field (`delta.reasoning` / `reasoning_details`) |
| `202609141410298583578848268d9d6zxviI9u5` | same | #61 | deepseek-v4.1-flash | #61 overflow ×2 (8 s, 5 s), then #59 succeeded (4,376 tokens, 38 s) | `frt` 7.00 s, overflow at 8 s | same |
| `202609210325111725203678268d9d6EOEoKhWn` | `/v1/responses` / OpenAIResponses | #67 FAST PRO 订阅 (1 OpenAI, fastaitoken.com) | gpt-6-astra | #67 overflow (71 s), then #67 retry succeeded (146 s total) | `frt` 2.06 s, 65 frames over about 69 s (about 1 per 1.06 s) | unknown slow trickle |
| `202609210336164178467708268d9d6xDKz9bk2` | same | #67 | gpt-6-astra | overflow (29 s), then #67 retry committed (client later disconnected) | `frt` 2.07 s, about 27 s for 65 frames | unknown |
| `202609210339124112126608268d9d6lOcOrXY9` | same | #67 | gpt-6-astra | overflow (23 s), then #67 retry succeeded (61 s total) | `frt` 2.59 s, about 20 s for 65 frames | unknown |

Confirmed context:
- Channel #61 served 893 + 257 + 68 successful streaming DeepSeek requests between 09-14 and 09-18 with no other overflows. In the same agent session (user `keli`, token `auto`), the requests at 22:10:07, 22:11:13 and 22:11:22 committed normally on #61.
- Channel #67 served about 3,850 successful streaming Responses requests between 09-19 and 09-21. All 3 overflows came from one Codex CLI session: channel-affinity `prompt_cache_key` fingerprint `f7c43471`, 09-21 11:25–11:39.

Inferred, not confirmed. The gate does not log frame types on overflow, and the DEBUG `stream scanner data` lines are not enabled.
- **#61**: both requests show 65 JSON frames within about 2 s after a 7 s TTFB, the same request succeeded on a different relay, and the next requests on #61 succeeded. This fits token-rate reasoning chunks in a field the gate does not recognize. It could also be some other per-token frame without `content` or `reasoning_content`.
- **#67**: text or summary deltas cannot be the cause, because any non-empty `*.delta` is content. The candidates are hosted-tool progress events whose `output_item.done` has no non-empty `action`, summary parts with empty text (a relay redacting summaries), or relay heartbeat JSON events. None can be distinguished from the logs.

### 4.2 `empty_stream` / `idle_timeout` on non-Anthropic paths (possibly reasoning-related)
- **`idle_timeout`: 0.** It is structurally disabled. The timer only starts if a channel sets `streaming_idle_timeout_seconds > 0` (`stream_scanner.go:123-133`). Every channel has it at 0 or unset (DB `channels.settings`), and no `streaming idle timeout` line exists in any log.
- **`/v1/responses` `empty_stream`: 408 attempts in total.**
  - **276** are historic (09-06 to 09-11 13:27:57, before the 09-11 13:28 deploy). The first and only frame was `response.completed` (`completed_at - created_at` of about 170 s in the sample), on the fake-streaming relays #26, #27, #28, #24, #45, #48. The model worked 50–321 s (p50 101 s) and then sent one completion frame. The current code releases these (`stream_gate.go:120-125`), so they are not failures today and are not a classifier gap.
  - **125** were client cancels.
  - **6** were real upstream HTTP/2 resets (`scanner error: stream error: stream ID N; INTERNAL_ERROR; received from peer`, `received=0..3`) after 3–5 minutes (p50 200 s, max 279 s). This is the upstream dying during the silent reasoning phase. They are reasoning-related in timing, but not a classifier gap.
  - **0** were plain EOF without a frame.
- `/v1/chat/completions`: 0 `empty_stream`.
- Gemini: 1 historic (the first frame was `finishReason: PROHIBITED_CONTENT`, released today) and 1 client cancel.
- Requests without a `[GIN]` path line: 5 historic `response.completed`-first and 2 client cancels.
- For comparison, `/v1/messages`: 48 attempts where `message_stop` was the first frame (09-02, #10/#11, 3–22 s; still a failure today), 2 plain EOF with no frame (post-fix, 9 s), and 67 client cancels.

### 4.3 Denominators (DB `logs`, since 09-02 00:05, `type=2`, `is_stream`)
- `/v1/responses`: 48,410
- `/v1/messages`: 20,408
- `/v1/chat/completions`: 2,875
- Gemini `streamGenerateContent`: 143

There is no OpenRouter (type 20) channel. The chat relays (type 60) serve DeepSeek, GLM, Qwen, KIMI and MiniMax.

On the ratio for `/v1/messages` overflows: my count is 173 requests, 121 ending in an error log and 52 succeeding. The PRD says 119/54. The small difference comes from how the final outcome is counted, and the logs now run until 17:09 today.

## 5. Timing of real pre-content failures (coordinator addendum)

**Method.**
- Attempt 1 starts at the request's first log line, the pre-consume line `用户 N 需要预扣费` / `额度充足`.
- Each later attempt starts at the previous attempt's `channel error (channel #N, …)` line. Non-gate failures such as HTTP 5xx or 524 are also used as boundaries.
- The failure time is the attempt's `channel error` line. Resolution is 1 s.
- `empty_stream` is split by the attempt's own preceding `stream ended` / `scanner error` line.
- **The duration includes connect time and the wait for response headers.** It is an upper bound on how long the gate was holding frames.
- Hedging never ran: `grep -ci hedge` finds only the 2 startup banners, and no `hedge attempt … launched` line exists. So attempts are sequential.
- In the tables below, "real" means upstream_error, malformed_event, empty_stream (eof, no frame), empty_stream (terminal first) and empty_stream (upstream transport reset).

### 5.1 Duration from attempt start to gate failure, whole window 09-02 → 09-25

| Category | Endpoint | n | p50 s | p90 s | p99 s | max s |
|---|---|---|---|---|---|---|
| upstream_error | /v1/messages | 88 | 4 | 30 | 132 | 132 |
| upstream_error | /v1/responses | 917 | 43 | 133 | 273 | 481 |
| upstream_error | /v1/chat/completions | 79 | 8 | 104 | 199 | 199 |
| upstream_error | gemini | 2 | 19 | 25 | 25 | 25 |
| upstream_error | no path line | 12 | 20 | 21 | 21 | 21 |
| malformed_event | /v1/responses | 4 | 18 | 142 | 142 | 142 |
| empty_stream, `stream ended reason=eof` (coordinator's definition) | /v1/messages | 50 | 3 | 5 | 22 | 22 |
| same | /v1/responses | 276 | 101 | 191 | 289 | 321 |
| same | gemini | 1 | 7 | 7 | 7 | 7 |
| same | no path line | 5 | 116 | 227 | 227 | 227 |
| … of which: eof with no frame (real) | /v1/messages | 2 | 9 | 9 | 9 | 9 |
| … of which: terminal first, `message_stop` (real) | /v1/messages | 48 | 3 | 5 | 22 | 22 |
| … of which: completion first, pre-09-11 build (**released by current code**) | /v1/responses + gemini + no path line | 282 | 102 | 191 | 289 | 321 |
| empty_stream, upstream HTTP/2 reset (`scanner_error`, real, not in the eof definition) | /v1/responses | 6 | 200 | 279 | 279 | 279 |
| idle_timeout | all | 0 | – | – | – | – |
| prebuffer_overflow | /v1/messages | 364 | 87 | 99 | 144 | 201 |
| prebuffer_overflow | /v1/responses | 3 | 29 | 71 | 71 | 71 |
| prebuffer_overflow | /v1/chat/completions | 4 | 5 | 8 | 8 | 8 |

For reference, client cancels that were mislabelled `empty_stream` had p50 60 s, p90 201 s, max 420 s (n=195). They are excluded above.

After the fix (≥ 09-11 13:28), with current classifier semantics:

| Category | Endpoint | n | p50 | p90 | p99 | max |
|---|---|---|---|---|---|---|
| upstream_error | /v1/messages | 27 | 21 | 83 | 132 | 132 |
| upstream_error | /v1/responses | 701 | 37 | 127 | 313 | 481 |
| upstream_error | /v1/chat/completions | 70 | 8 | 104 | 199 | 199 |
| upstream_error | gemini | 2 | 19 | 25 | 25 | 25 |
| malformed_event | /v1/responses | 1 | 142 | 142 | 142 | 142 |
| empty_stream eof, no frame | /v1/messages | 2 | 9 | 9 | 9 | 9 |
| empty_stream upstream reset | /v1/responses | 6 | 200 | 279 | 279 | 279 |
| prebuffer_overflow | /v1/messages | 290 | 86 | 94 | 115 | 201 |

### 5.2 Real failures later than a threshold (duration from attempt start)

Whole window. Real failures exclude the 282 pre-fix completion-first attempts.

| Endpoint | n | > 5 s | > 10 s | > 15 s | > 30 s | > 60 s |
|---|---|---|---|---|---|---|
| /v1/messages | 138 | 38 | 31 | 25 | 8 | 4 |
| /v1/responses | 927 | 792 | 727 | 687 | 607 | 368 |
| /v1/chat/completions | 79 | 44 | 31 | 28 | 19 | 9 |
| gemini | 2 | 2 | 2 | 2 | 0 | 0 |
| no path line | 12 | 8 | 8 | 8 | 0 | 0 |
| **all** | **1,158** | **884 (76%)** | **799 (69%)** | **750 (65%)** | **634 (55%)** | **381 (33%)** |

By category (all endpoints):

| Category | n | > 5 s | > 10 s | > 15 s | > 30 s | > 60 s |
|---|---|---|---|---|---|---|
| upstream_error | 1,098 | 871 | 789 | 741 | 627 | 374 |
| malformed_event | 4 | 3 | 3 | 3 | 2 | 2 |
| empty_stream eof, no frame | 2 | 2 | 0 | 0 | 0 | 0 |
| empty_stream terminal first | 48 | 2 | 1 | 1 | 0 | 0 |
| empty_stream upstream reset | 6 | 6 | 6 | 5 | 5 | 5 |

- After the fix only: n = 809; > 5 s: 652; > 10 s: 577; > 15 s: 535; > 30 s: 444; > 60 s: 244. `/v1/messages` 29 → 28 / 24 / 20 / 8 / 4.
- Using the coordinator's literal eof definition, which includes the 282 historic completion-first attempts: n = 1,434 → 1,160 / 1,074 / 1,026 / 910 / 656.

### 5.3 How long frames were actually held before a real failure (estimate)

`frt` is the time from request start to the first upstream frame of any attempt; buffered frames count (`stream_scanner.go:453`, `service/log_info_generate.go:101`). It is stored only for successful requests, and `relayInfo` is shared across attempts (`controller/relay.go:132`). So for **attempt 1 of requests that later succeeded**: if `frt < duration`, attempt 1 held frames for `duration − frt`; otherwise the error was its first frame.

| Endpoint | attempt-1 real failures | held any frame | hold p50 | hold p90 | hold max | hold > 10 s | hold > 30 s | hold > 60 s |
|---|---|---|---|---|---|---|---|---|
| /v1/messages | 12 | 9 | 10 s | 117 s | 117 s | 4 | 1 | 1 |
| /v1/responses | 504 | 340 | 9 s | 114 s | 432 s | 169 (34%) | 114 (23%) | 61 (12%) |
| /v1/chat/completions | 33 | 1 | 0 | 0 | 0 | 0 | 0 | 0 |
| all | 552 | 350 | 9 s | 114 s | 432 s | 173 (31%) | 115 (21%) | 62 (11%) |

Reading:
- **Chat errors almost always arrive as the first frame** (1 of 33 held anything). A commit deadline measured from the first held frame would not change Chat failover. The long Chat durations are header or first-byte waits.
- On **Responses**, relays usually send `response.created` / `in_progress` early and the error minutes later. With a commit deadline N measured from the first held frame, roughly 34% / 23% / 12% of the sampled Responses real failures would have been committed before the error arrived, for N = 10 / 30 / 60 s. Those would become client-visible errors instead of transparent failover.
- Context: first-frame times for clean single-attempt successes since 09-11 are Responses p50 6.9 s / p90 24.5 s / p99 82.6 s (n = 36,844), `/v1/messages` 3.8 / 8.2 / 16.9 s (n = 17,807) and chat 3.8 / 12.7 / 70.6 s (n = 2,615).

### 5.4 Replaying buffered frames after a deadline or cap commit

The replay path is the same one used by today's content commit. `commitBufferedStream` publishes headers, calls `MarkCommitted`, and pushes every buffered frame through `dataChan` (`stream_scanner.go:315-341`) into the data handler goroutine (`:279-290`). Frames after the commit take the same route (`:463-476`). The buffer can contain non-JSON keepalives: they are appended at `:430` and only exempted from the count at `:439-441`.

How each handler reacts to a replayed non-JSON `data: ping`:

| Handler | Reaction | file:line |
|---|---|---|
| Claude `ClaudeStreamHandler` → `HandleStreamResponseData` | **Fatal.** `UnmarshalJsonStr` fails, it returns `types.NewError(...)`, and the handler calls `sr.Stop(err)`. The stream ends right after the commit and the function returns `nil, err`. | `relay/channel/claude/relay-claude.go:86-92`, `:203-207`, `:212-214`; `relay/helper/stream_result.go:29-35`; `stream_scanner.go:287-289` |
| Gemini `geminiStreamHandler` | **Fatal.** `sr.Stop(fmt.Errorf("unmarshal: %w", err))` | `relay/channel/gemini/relay-gemini.go:153-158` |
| OpenAI Chat `OaiStreamHandler` | Soft error only. For OpenAI clients the frame is passed through raw via `helper.StringData`; for Claude or Gemini clients the conversion fails as a soft error and the frame is dropped. | `relay/channel/openai/relay-openai.go:23-29`, `:128-147`; `relay/channel/openai/helper.go:37-41`, `:60-65` |
| Responses / Responses→Chat / Chat→Responses | Soft error, frame dropped | `relay_responses.go:91-96`; `chat_via_responses.go:276-281`; `responses_via_chat.go:106-111` |
| Image | Passed through raw | `relay_image.go:176-185` |

After a commit, a handler error cannot fail over, because the controller trusts `IsCommitted()` for gated streams (`controller/relay.go:316-327`).

Implications:
- **Confirmed:** a non-JSON data line kills a Claude or Gemini stream **today**, both when it is replayed at content commit and when it arrives after commit. A deadline or cap commit adds no new failure mode for these frames, but on those two protocols it would trigger the same kill in a new case: a buffer holding only proxy pings.
- **Production exposure is zero today:** 0 `error unmarshalling stream response` lines (the Claude `SysLog`), 0 `end_error="unmarshal`, 0 `data: ping` or `keepalive` lines across all logs.
- **JSON neutral frames replay safely in every handler.** This includes 64 or more empty `thinking_delta`, `message_start`, JSON `ping`, `delta.reasoning` chunks and Responses lifecycle events.
- The overflowing frame is already in `buffered` before the cap check (`:430` comes before `:442`), so committing at the cap would not lose it.
- **Hedge interaction (code only; hedging is inactive in production):** `commitBufferedStream` calls `info.Hedge.Commit()` first (`:316-324`). A deadline commit would claim the hedge winner slot on a timer instead of on content. The hedge launch timer is driven by `first_content_timeout_seconds` (`controller/relay_hedge.go:66-68`, `:276-292`), which is set to 60 on #10 and #58.

## 6. Recommendation per protocol

| Protocol | Recommendation | Basis |
|---|---|---|
| Anthropic | **Change now** (this task, R1/R2) | 364 of 371 overflows; p50 87 s to overflow |
| OpenAI Chat | **Change now, or right after this task, low risk**: add `choices.#.delta.reasoning` and `choices.#.delta.reasoning_details` to the content paths at `stream_gate.go:107-111` | Confirmed code gap. FlowAPI's DTO and converters already treat `reasoning` as reasoning (`openai_response.go:91,107-115`). vLLM and OpenRouter docs use these fields. Light production evidence: 4 attempts / 2 requests on #61, 65 frames in under 2 s. |
| OpenAI Responses | **Defer.** The low-risk option is to log the buffered event-type histogram on `prebuffer_overflow` to identify the #67 frames. | Reasoning deltas are already content through the generic `*.delta` rule; summaries-off streams send about 6 neutral frames; 3 overflow attempts in one Codex session, all recovered on retry, out of about 3,850 successes on that channel. |
| Gemini | No change | Thought parts carry text, so they are content; signature-only parts are single frames; 0 overflows; 143 streams since 09-02 |
| OpenAI Image | No change | No progress frames in the protocol; 0 overflows; image traffic is non-streaming |

For the protocol-agnostic "commit after N seconds held" idea (section 5):
- On Anthropic, the thinking fix alone removes the overflow case.
- On Responses, real failures after long holds are common: 169 / 114 / 61 of 504 sampled attempt-1 failures had frames held for more than 10 / 30 / 60 s. A deadline would turn those into client-visible errors.
- Chat errors come as the first frame and are unaffected.
- Claude and Gemini handlers die on replayed non-JSON pings; this is pre-existing behavior, not new.

## Caveats / Not found

- The frame contents of the 7 non-Anthropic overflow attempts are **not recorded anywhere**. The gate logs only the reason, and DEBUG scanner logging is off. The frame types in section 4.1 are inferences.
- The pre-09-11 classifier is not in git (production ran an uncommitted build from 09-02). The "released by current code" label for the 282 completion-first attempts is based on today's `stream_gate.go:120-125` and the 09-11 deploy note, not on replaying them.
- Attempt durations are measured from log lines at 1 s resolution and include time before the response headers. The hold-time estimate (5.3) covers only attempt 1 of requests that later succeeded, which is 552 of 1,158 real failures, and assumes the pre-consume line time is close to `RelayInfo.StartTime`.
- The Gemini vendor docs (`ai.google.dev`) could not be reached from this machine. The Gemini behavior above comes from FlowAPI code (`stream_gate.go:129-146`, `relaykit/dto/gemini.go:272-275`) and has no external confirmation.
- Other vendors said to use `delta.reasoning` (Groq, Ollama, LM Studio and others) were not verified. Only vLLM and OpenRouter were checked against their docs.
- Server access was read-only (log greps through `docker exec`, psql `SELECT` / `\copy … to stdout`). No temp files were written on HK. Local scratch `/tmp/sg-research` has been deleted.

---

## 7. 19:56 flowadmin 4s error

- **Query**: Error row 2026-09-25 19:56:28 CST, flowadmin, channel #10, `claude-opus-5-5`, stream, 4.0 s, `status_code=502, stream failed…`. Was the 64-frame cap reached after only 4 s?
- **Scope**: HK production, read-only (psql `SELECT`, `grep` on `/app/logs/oneapi-20260925010530.log`). Prod image `flowapi:hk-media-8156149df-20260925`. Gate files at `8156149df` are identical to `HEAD` (`f3f7a8129`). Code lines below are from `git show 8156149df:<file>`.

**Verdict: this is a client cancel, not a gate failure.** The gate was holding 4 frames (the cap is 64) when the client closed the connection. The old code labels a disconnect before commit as `empty_stream`. The route history shows the real reason, `client_cancelled`.

### 7.1 DB row (`logs.id` 83056)
- 19:56:28, type 5, token `Auto`, group `auto`, channel #10 ZIVV CCMAX (type 14), `is_stream` true, quota 0, `use_time` 4.
- `request_id` `202609251156240381175088268d9d6umTSs6Mr`. `upstream_request_id` is empty.
- Full `content`: `status_code=502, stream failed before first valid content: empty_stream`
- `other`:
  - `error_code` `bad_response_body`, `error_type` `openai_error`, `status_code` 502, `request_path` `/v1/messages`
  - `route_history`: `[{"channel_id":10,"channel_name":"ZIVV CCMAX","attempt":1,"outcome":"stopped","reason":"client_cancelled","status_code":502,"priority":100}]`
  - `channel_affinity`: rule `claude cli trace`, key `metadata.user_id` (fp `8126ae60`), `selected_group` `【职刻】Claude 官转`, `override_template.applied` true (1 param override key)
  - `stream_status`: not present. Error rows do not carry it, and they have no `frt`, which is why the UI shows first-token N/A.

### 7.2 App log (every line for the request)
```
[INFO] 19:56:24 | 用户 1 需要预扣费 ¥0.873194 (funding=unlimited)
[ERR]  19:56:28 | scanner error: http2: response body closed
[ERR]  19:56:28 | stream ended: reason=client_gone end_error="context canceled", received=4
[ERR]  19:56:28 | channel error (channel #10, status code: 502): stream failed before first valid content: empty_stream
[INFO] 19:56:28 | route history: [{"channel_id":10,"channel_name":"ZIVV CCMAX","attempt":1,"outcome":"stopped","reason":"client_cancelled","status_code":502,"priority":100}]
[INFO] 19:56:28 | 用户 1 请求失败, 返还预扣费（token_quota=¥0.873194, funding=unlimited）
[INFO] 19:56:28 | record error log: … content=status_code=502, stream failed before first valid content: empty_stream
[ERR]  19:56:28 | relay error: stream failed before first valid content: empty_stream
[GIN]  19:56:28 | relay | 502 | 4.865156628s | 36.248.233.213 | POST /v1/messages?beta=true
```

### 7.3 What happened (one attempt, about 4 s)
1. 19:56:24: attempt 1 starts on #10.
2. The upstream returned an SSE response. The gate held 4 `data:` frames, all neutral under the old classifier (`received=4`, counted at `stream_scanner.go:454`). None was content, so nothing was sent to the client.
3. 19:56:28: the client closed the request. The main select took `c.Request.Context().Done()` and set `client_gone` / `context canceled` (`:509-512`).
4. `cleanup()` closed `resp.Body` (`:175-190`), so the scanner stopped with `http2: response body closed` (`:486`). This is FlowAPI closing its own upstream body, not an upstream reset.
5. The stream was never committed, so the scanner's EOF path set the precommit error `empty_stream` (`:490-494`). It was returned as 502 `bad_response_body` (`:525-531`).
6. `ClassifyRelayRetry` saw the cancelled request context and returned `client_cancelled` (`service/relay_retry.go:27-29`). There was no retry and no auto-ban strike (`controller/relay.go:327-341`, `:362`). The pre-consume was refunded.

- **Reason:** `client_gone`, labeled `empty_stream`. It was not an EOF with no frames and not a terminal-first stream. It was not `upstream_error` either, so there is no upstream frame to quote. `prebuffer_overflow` was not reached (4 of 64).
- **Frame types are not logged** (DEBUG scanner logging is off). Inferred, not confirmed: a Claude Code stream on #10 starts with `message_start`, a thinking `content_block_start` and `ping`. The 4th neutral frame is most likely the first empty `thinking_delta`, because a non-empty thinking or text delta would have committed.
- **Context:**
  - Same session (affinity fp `8126ae60`): 6 requests from 19:55:15 to 19:56:23 succeeded on #10 with `frt` 2.4–3.7 s.
  - The next flowadmin request started at 19:58:17 with a different fp (`2a253041`).
  - The same client IP `36.248.233.213` had 9 request bodies cut off mid-upload between 19:57:22 and 20:00:10 (`user 1 | Invalid request: unexpected EOF`, HTTP 400, not written to `logs`). Some of these took 1m18s–1m41s.
  - This points to the client side, either a user interrupt or the client's network (inference).

### 7.4 flowadmin error rows on `claude-opus-5*`, 17:03:08 – 20:03:08 CST
| Time | Request id (tail) | Ch | use_time | GIN | Content | Cause from app log |
|---|---|---|---|---|---|---|
| 17:08:09 | `…67vsrzqBa` | 10 | 0 | 1.00 s | do request failed | `context canceled` before upstream headers |
| 17:08:10 | `…6zRSGsPno` | 10 | 0 | 0.55 s | do request failed | same |
| 18:25:01 | `…6Sto7nKQS` | 10 | 0 | 0.79 s | do request failed | same |
| 18:48:13 | `…6NTX9qyvG` | 10 | 0 | 0.87 s | do request failed | same |
| 19:56:28 | `…6umTSs6Mr` | 10 | 4 | 4.87 s | empty_stream | `client_gone` before commit, received=4 |
| 19:59:54 | `…6VnHR1Odq` | 10 | 0 | 21.48 s | do request failed | `context canceled` before upstream headers |
| 19:59:55 | `…6cwracv4f` | 10 | 1 | 3.95 s | do request failed | same |
| 19:59:55 | `…6IU2Y7cpC` | 10 | 0 | 0.15 s | do request failed | same |
| 19:59:57 | `…6bez93QIk` | 10 | 0 | 6.69 s | do request failed | same |
| 19:59:58 | `…6uy9RCHto` | 10 | 1 | 1.67 s | do request failed | same |
| 20:00:00 | `…67J26l15y` | 10 | 2 | 3.08 s | do request failed | same |

Counts per reason:
- 10: client cancel before the upstream response headers (`do request failed: Post "https://api2.zivv.pro/v1/messages?beta=true": context canceled`, `relay/channel/api_request.go:609-614`). The gate never ran.
- 1: client cancel after headers, before commit (`client_gone`, labeled `empty_stream`).
- 0: `prebuffer_overflow`, `upstream_error`, `malformed_event`, and real `empty_stream`.

All 11 have route history `client_cancelled`, attempt 1, #10. The 17:08–18:48 rows came from IP `112.48.65.152` and the 19:56–20:00 rows from `36.248.233.213`.

In the same window, 688 flowadmin `claude-opus-5*` requests succeeded, all on #10 at attempt 1. Two of them were committed streams that the client left (17:08:08 and 18:21:06, `stream_status.end_reason=client_gone`). Those were billed as consume rows.

The only gate overflows in this window belonged to user `hx`: 4 `prebuffer_overflow` attempts in 2 requests (17:03:26 #10, 17:05:10 #47, 17:07:46 #10, 17:09:10 #10).

### 7.5 Would this task's fix change the outcome?
- **R2 (release at the cap):** no. The gate held 4 frames and the cap is 64.
- **R1 (`thinking_delta` is content):** only if one of the 4 frames was a `thinking_delta`.
  - If one was, the gate would have opened at that frame and sent the headers and the held frames. The 19:56:28 disconnect would then end a committed stream.
  - Current code records a committed stream that the client left as a consume row with `stream_status.end_reason=client_gone` and bills the usage seen so far, as at 17:08:08 and 18:21:06 today. That would replace the 502 error row and its refund.
  - If none was, nothing changes.
- **No retry either way:** the client had already left.
- **The label stays the same:** the fix does not change how a disconnect before commit is labeled `empty_stream`. The working tree `relay/helper/stream_scanner.go:497-505`, `:525` and `:540` match prod. The PRD lists this label as out of scope.
- **The 10 `do request failed` rows:** these fail before the upstream answers, so the gate is not involved and the fix does not affect them.

## 8. Timeouts: upstream new-api vs FlowAPI

- **Query**: "Original New API has no notion of when a request times out. It only treats something as a timeout when the upstream reports one, right?" This section lists every relay timeout in upstream new-api and in FlowAPI (HEAD and the deployed `8156149df`), plus the HK prod env and nginx.
- **Scope**: mixed.
  - Upstream: `upstream/main` = `c2b7a9a9e` (2026-09-25, after `git fetch upstream`).
  - FlowAPI: HEAD `f3f7a8129` and deployed `8156149df`.
  - HK prod, read-only.
  - Go stdlib: local 1.25.6 source. The builder image is `golang:1.26.1` (`Dockerfile:10`).
- **Date**: 2026-09-25

### Short answer

- **Upstream new-api: not quite.** By default it owns one timeout on the stream body: `STREAMING_TIMEOUT` = 300 s.
  - It is an idle timer between SSE lines, and it starts only after the upstream headers.
  - Current upstream main also bounds the header wait at 1800 s (`RELAY_RESPONSE_HEADER_TIMEOUT`). That was added 2026-08-30 in `b518d0033`, first released in `v1.0.0-rc.28`, after FlowAPI forked.
  - There is no first-token timeout and no total timeout (`RELAY_TIMEOUT` = 0).
  - Everything else counts as a timeout only when the upstream reports it (504/524) or the socket fails.
- **FlowAPI prod: yes, in effect.** After connect, no gateway-owned timeout is active on a chat stream.
  - `STREAMING_TIMEOUT` is still parsed, but nothing has read it since `b9692d6f4` (deployed 2026-09-18).
  - Its per-channel replacement is not read in prod, and the saved values are 0 anyway.
  - `RELAY_TIMEOUT` is unset, and there is no header timeout.
  - A silent request is stopped only by nginx `proxy_read_timeout 900s` or by the client leaving.
  - Upstream 408/504/524 are recognized as `upstream_timeout` and retried on another channel.

### 8.1 Upstream new-api (`upstream/main` `c2b7a9a9e`)

| Timeout | Env / default | Where | What it covers |
|---|---|---|---|
| Total client timeout | `RELAY_TIMEOUT`, default 0 = off | `common/init.go:112`. Applied at `service/http_client.go:129-131`, `service/protected_fetch_client.go:94-95`, and the AWS SDK context at `relay/channel/aws/relay-aws.go:43-48` | `http.Client.Timeout` covers connect, redirects **and reading the body**. The timer keeps running after `Do` returns and interrupts `Body.Read` (Go `net/http/client.go:90-96`). If set, it cuts long streams, as upstream notes at `common/constants.go:174-177` |
| Stream idle | `STREAMING_TIMEOUT`, default 300 s | `common/init.go:179`. `relay/helper/stream_scanner.go:94` reads the value. `:99` creates the ticker unconditionally. `:256` calls `ticker.Reset` on every scanned line. `:299-301` fires and sets `EndReason=timeout` | Starts when `StreamScannerHandler` runs, which is after the upstream headers. Covers the gap from headers to the first line and every line-to-line gap. Any line resets it, including blank lines, `: ping` comments and `event: ping` |
| Same env, other readers | same | `relay/responses_websocket.go:363-367` and `:490-494` (Responses WebSocket idle, fallback 300 s). `relay/channel/task/jsplugin/submit_stream.go:30-34` (task-plugin submit stream, fallback 30 s) | Neither file exists in FlowAPI |
| Response header wait | `RELAY_RESPONSE_HEADER_TIMEOUT`, default 1800 s | `common/constants.go:170-178`, `common/init.go:114`, `service/http_client.go:108-116` | `Transport.ResponseHeaderTimeout`: from the request being fully written to the response headers. Added in `b518d0033` (2026-08-30, #6949). Before that the wait was unbounded. That includes FlowAPI's fork point `e468b7391` (2026-08-27, after `v1.0.0-rc.26`) |
| Dial / TLS | 30 s / 10 s (Go defaults) | `service/http_client.go:81-82` clones `http.DefaultTransport` (Go `net/http/transport.go:45-56`: dial 30 s, keepalive 30 s, TLS 10 s, `ExpectContinueTimeout` 1 s). SOCKS dialer 30 s at `:294-297` | Connection setup only |
| Idle pooled connection | `RELAY_IDLE_CONN_TIMEOUT`, default 90 s | `common/init.go:113`, `service/http_client.go:98` | Closes unused keep-alive connections. This is not a request timeout |
| TCP keepalive | idle 30 s, then 9 probes 15 s apart (Go defaults) | Go `net/tcpsock.go:132-147`, `:289-296` | Detects a dead peer after about 165 s. A live but silent upstream answers the probes, so this never fires for it |
| First token / first content | none | Grep of `upstream/main` finds no first-token or first-byte timeout | none |
| Non-stream | nothing besides `RELAY_TIMEOUT` | none | none |
| Server side | none | `main.go:214-217`: `http.Server` has no Read or Write timeouts. The per-write deadline of 30 s is at `relay/helper/stream_scanner.go:33` and `:80` | The 30 s deadline bounds a single blocked write to a slow client. It does not bound the upstream |

- **A client disconnect does not cancel the upstream header wait.**
  - `DoApiRequest` and `DoFormRequest` build the upstream request with `http.NewRequest`, with no context (`relay/channel/api_request.go:319`, `:349`).
  - `doRequest` (`:520-589`) never attaches `c.Request.Context()`.
  - Only task requests use the client context (`:621-626`).
  - After the headers arrive, the scanner does watch `c.Request.Context().Done()` (`stream_scanner.go:304-307`).
- **Keepalive before headers.**
  - For streams, `doRequest` sets the SSE headers and starts a pinger before `Do` (`api_request.go:544-560`). The pinger stops after at most 120 min (`:464`).
  - Ping is off by default (`setting/operation_setting/general_setting.go:28-29`: disabled, 60 s).
  - `SetEventStreamHeaders` only sets headers and never flushes (`relay/helper/common.go:45-59`). So with default settings the client receives nothing until the first upstream data.
- **How upstream-reported timeouts surface.**
  - **Transport errors.** Dial or TLS timeouts, `timeout awaiting response headers` and `Client.Timeout exceeded` all come back from `relayClient.Do`.
    - They become `do_request_failed`, with the detail hidden behind the message `upstream error: do request failed` (`api_request.go:562-566`).
    - The status is 500 by default (`relaykit/types/error.go:257`). 500 is in the default retry ranges (`setting/operation_setting/status_code_ranges.go:26`), so they are retried.
  - **Upstream HTTP 504/524.** These are never retried.
    - `alwaysSkipRetryStatusCodes = {504, 524}` (`status_code_ranges.go:31-34`) is checked in `DecideRelayRetry` (`service/relay_error.go:51-52`), reason `system_retry_exclusion`.
    - 408 is outside the retry ranges (`status_code_ranges.go:24-25`).
    - The client gets the upstream status, unless a channel status-code mapping rewrites it (`service/error.go:140`).
    - The default auto-disable list contains only 401 (`status_code_ranges.go:17`).
  - **Stream idle timeout.** The stream ends with `EndReason=timeout` and a nil error.
    - `IsNormalEnd()` is false for that reason (`relay/common/stream_status.go:209-216`). It is logged as an abnormal end and recorded in the log info (`service/log_info_generate.go:162`).
    - The client just sees the SSE stream stop.

### 8.2 FlowAPI (HEAD `f3f7a8129` and deployed `8156149df`)

`git diff f3f7a8129 8156149df` does not touch any of the files cited in this subsection except the media ones listed at the end. That covers `relay/helper/`, `relay/channel/api_request.go`, `service/http_client.go`, `common/` and `constant/env.go`. The line numbers below are valid for both commits. The deployed diff only adds media-specific timeouts.

**`STREAMING_TIMEOUT` is parsed but never read.**
- `common/init.go:182` still sets it (default 300) into `constant.StreamingTimeout` (`constant/env.go:3`). It is still documented in `.env.example:65-66` and `docker-compose.yml:39`.
- No non-test Go code reads it: not in HEAD, not in `8156149df`, not in the working tree.
  - Six test files still assign it, for example `relay/helper/stream_scanner_test.go:27-28`. The assignment has no effect.
- At the fork point `e468b7391`, its only reader was `relay/helper/stream_scanner.go:88`. Upstream added the Responses WebSocket and jsplugin readers after the fork.
- **It was replaced in `b9692d6f4`** (2026-09-17 23:35, "feat(routing): add provider hedge racing and loser billing").
  - That commit deleted `streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second`, the unconditional `time.NewTicker` and the per-line `ticker.Reset`.
  - In their place it reads `info.ChannelSetting.Reliability.StreamingIdleTimeoutSeconds`.
  - The server skill records the HK deploy of this commit on 2026-09-18: image `hk-hedge-b9692d6f4-20260918`, backup `pre-hedge-20260917-164453` (UTC), which is 09-18 00:44 CST.

**The gated and non-gated scanner are the same function** (`streamScannerHandler`, `relay/helper/stream_scanner.go:104`).
- `:123-126`: the idle value comes only from the per-channel `Reliability.StreamingIdleTimeoutSeconds`.
- `:127-133`: `nil` or `0` means no ticker is created and `idleC` stays nil.
- `:135-138` and `:90-102`: when enabled, the timer resets on every `Read` that returns bytes, not once per line.
- `:501-506`: when the idle case fires, `EndReason` becomes `timeout`. Before commit it also records `StreamPrecommitFailure{Reason:"idle_timeout"}`.
- **There is no fallback to the global env.**
- In the working tree (another agent's edit), the same logic sits at `:124-133` and reads `info.ChannelOtherSettings.Reliability`. It still has no fallback.

**Per-channel reliability timeouts** (`relaykit/dto/channel_reliability.go:14-21`, bounds at `:45-58`):

| Field | Allowed | Reader | Effect |
|---|---|---|---|
| `streaming_idle_timeout_seconds` | 0, or 60–600 | `stream_scanner.go:124-125` | The idle timer described above |
| `non_streaming_timeout_seconds` | 0, or 60–1800 | `relay/channel/api_request.go:549-554`, only when `!info.IsStream`. Released on body close at `relay/channel/deadline_body.go:10-13` and `:34-35` | `context.WithTimeout` on the upstream request. Covers both headers and body |
| `first_content_timeout_seconds` | 0, or 1–180 | `controller/relay_hedge.go:65-67`, `:275-292`, `:368-372` | Only a hedge threshold: when it expires, a backup attempt is launched. It never cancels the primary attempt |

In the deployed build all three fields are read from `info.ChannelSetting`, which is the `channels.setting` column (`relaykit/dto/channel_settings.go:26`). §8.3 shows that none of them is active in prod.

**Other timeout settings and connection behaviour**
- **`RELAY_TIMEOUT` is still honored in code.** It is read at `common/init.go:111` and applied at `service/http_client.go:106-108`, `service/protected_fetch_client.go:94-96` and `relay/channel/aws/relay-aws.go:43-48`. It is unset in prod, so it is 0 (off).
- **Transport** (`service/http_client.go:74-99`).
  - It uses `http.DefaultTransport.Clone()`: dial 30 s, TLS 10 s, `ExpectContinueTimeout` 1 s.
  - `IdleConnTimeout` comes from `RELAY_IDLE_CONN_TIMEOUT` (90 s) at `:93`.
  - **There is no `ResponseHeaderTimeout`.** FlowAPI does not have upstream `b518d0033` and has no `RELAY_RESPONSE_HEADER_TIMEOUT`.
- **The upstream request is bound to the client context.** Since `845a85998` (2026-09-11), `relay/channel/api_request.go:546-548` does `req = req.WithContext(c.Request.Context())`.
  - When the client disconnects, or nginx closes the connection, the upstream wait is cancelled.
  - Upstream new-api does not do this (see 8.1).
- **A gated stream sends nothing to the client before commit.**
  - The pinger before `Do` runs only if the response is already committed (`api_request.go:588-603`; `streamResponseCommitted` at `:529-534`).
  - The scanner's ping goroutine skips every tick until commit (`stream_scanner.go:226-228`).
  - After commit it pings every `PingIntervalSeconds`. It stops 30 min after the scanner starts (`:158-167`, `:219-220`).

**Retry classification of upstream timeouts** (`service/relay_retry.go`, since `845a85998`):
- `:62-63`: 408, 504 and 524 become `upstream_timeout`, with `Retryable: true` and `AutoBanEligible: true`.
- This check runs before the always-skip list. So it overrides the upstream no-retry rule for 504/524, even though that rule is still present at `setting/operation_setting/status_code_ranges.go:31-34`.
- `:27-32`: when the inbound context has expired, the result is `client_cancelled` or `request_deadline`, and traversal stops.

**Other relay paths**
- **Realtime WebSocket.** It uses `websocket.DefaultDialer.Dial` (`api_request.go:437`); gorilla v1.5.0 gives a 45 s handshake timeout (`client.go:140-143`). `relay/channel/openai/relay_realtime.go` sets no read deadline, so there is no idle timeout. The Responses WebSocket relay exists only upstream.
- **Handlers that run their own scanner loop.** These get no idle timer in either codebase:
  - `relay/channel/openai/chat_via_responses.go:90` (buffered Responses→Chat)
  - the cloudflare, cohere, coze, ollama, tencent and zhipu stream handlers
- **Async tasks** (same in both codebases):
  - `TASK_TIMEOUT_MINUTES` defaults to 1440 (`common/init.go:205`). A sweep marks unfinished tasks older than 24 h as failed (`service/task_polling.go:74-84`).
  - The polling pass runs every 15 s (`controller/system_task_handlers.go:146`).
  - Midjourney proxy requests use 60 s, 30 s and 60 s (`relay/mjproxy_handler.go:231`, `:320`, `:542`). The MJ task-list poll uses 15 s (`controller/midjourney.go:112-113`).
- **Media-specific timeouts added in `8156149df`.** None is on the chat path.
  - VOD upload client: 90 s (`relay/channel/task/volcvod/uploader.go:36`).
  - Doubao ASR HTTP client: 20 s (`relay/channel/volcengine/asr.go:78`). ASR polling runs every 2 s for up to 300 s (`:61-62`, `:438-441`).
  - Wan media download: 45 s (`service/wan_video.go:33`, `:39`).

### 8.3 HK prod state (read-only, 2026-09-25)

**Container and environment**
- **Running image.** Container `flowapi` runs `flowapi:hk-media-8156149df-20260925`.
- **No timeout keys are set.** `docker exec flowapi printenv` has no `RELAY_TIMEOUT`, `STREAMING_TIMEOUT`, `RELAY_IDLE_CONN_TIMEOUT`, `RELAY_RESPONSE_HEADER_TIMEOUT`, `TASK_TIMEOUT_MINUTES` or `SHUTDOWN_TIMEOUT_SECONDS`.
- **No related key names either.** No key name contains timeout, idle, ping, poll, deadline, interval, stream or relay.
- **Full key list** (names only): BATCH_UPDATE_ENABLED, ERROR_LOG_ENABLED, GIN_MODE, HOME, HOSTNAME, MAX_REQUEST_BODY_MB, NODE_NAME, PATH, REDIS_CONN_STRING, SESSION_COOKIE_SECURE, SESSION_COOKIE_TRUSTED_URL, SESSION_SECRET, SQL_DSN, TZ.
- **Compose files.** `/opt/flowapi/docker-compose.yml` and `.env` contain no timeout keys either.
- **Effective values.** `RELAY_TIMEOUT` is 0. `STREAMING_TIMEOUT` is 300 but never read. `RELAY_IDLE_CONN_TIMEOUT` is 90. `TASK_TIMEOUT_MINUTES` is 1440.

**Database**
- **`options` table.**
  - `general_setting.ping_interval_enabled` is `true`.
  - `ping_interval_seconds` is not stored, so the default of 60 s applies (`general_setting.go:31`).
  - There are no timeout or hedge option rows.
- **`channels` table (52 rows).**
  - 0 rows have `reliability` in `setting`, which is the column the deployed code reads.
  - 12 rows have `streaming_idle_timeout_seconds` in `settings`, and all 12 values are 0.
  - 0 rows have a non-zero `non_streaming_timeout_seconds`.
  - So the idle and non-stream timeouts are off on every channel. That holds in the deployed build, and it would still hold after the working-tree fix (see `hedge-config.md` §6.1).

**nginx** (`/etc/nginx/sites-enabled/flowapi.robusta.top:29-42`)
- **Directives set:** `proxy_read_timeout 900s` (`:38`), `proxy_send_timeout 900s` (`:39`), `proxy_buffering off` (`:40`), `proxy_request_buffering off` (`:41`), plus WebSocket upgrade headers (`:32-33`).
- **Directives not set:** there is no `proxy_connect_timeout` and no `send_timeout`, so both use the 60 s default. `nginx.conf` has no timeout directives.
- **No CDN in front:** `flowapi.robusta.top` resolves straight to the HK IP `193.134.209.185`.

**App logs** (`/app/logs/oneapi-*.log`, 54 files, 2026-08-27 → 2026-09-25)
- **The global 300 s timer never fired.** There are 0 `reason=timeout` lines and 0 `streaming idle timeout` lines. That includes the period before 09-18, when the timer was still active.
- **Upstream-reported timeouts are common.**
  - There are 663 `channel error … status code: 524` lines and 25 `… 504` lines.
  - 435 route-history lines contain `"reason":"upstream_timeout"`.
  - In 410 of 492 requests, the first 524 arrived 125–134 s after request start. That timing fits a proxy timeout on the upstream relay's side.
- **Deadline errors all came from synthetic group probes.**
  - 99 lines read `do request failed … context deadline exceeded` (09-02 → 09-11 13:14 CST).
  - Every one was a group probe (`分组探测`, token `模型测试`, route reason `request_deadline`) that failed 59–60 s after it started.
  - That probe code is not in any committed Go file, and these errors stop after the 09-11 deploy.
  - No user request ever hit a gateway-side deadline.

**nginx error logs** (retained 09-08 → 09-25, server TZ is UTC)
- There are 3 lines of `upstream timed out (110) while reading response header from upstream`, all on `POST /v1/responses`. Each one matches a FlowAPI `[GIN]` line lasting exactly 15m0.1–0.5 s.
- `202609100337132341811388268d9d68WHqhNNT` (09-10 11:37 → 11:52 CST, 502).
  - Six gated attempts ran on #26, #27 and #28.
  - These are fake-stream relays that return only `response.completed` after about 145–170 s. The pre-09-11 code rejected each as `empty_stream`.
  - The request ended `client_gone` when nginx closed the connection.
- `202609102256030208169658268d9d640iPNqCB` (09-11 06:56 → 07:11 CST, 502).
- `202609170632170757297798268d9d6MhYmKFhx` (09-17 14:32 → 14:47 CST, 500).
  - Ten attempts ran, two per channel, in the order #22→#28→#24→#27→#26.
  - Six of them were upstream 524s, each arriving about 125 s in, and each was retried as `upstream_timeout`.
  - The last attempt ended with `do request failed … context canceled` at 900 s.
- In none of these cases did FlowAPI itself time out. Nothing had been written to the client, so nginx's 900 s budget ran out across all the failover attempts.

### 8.4 What bounds a streaming request

| Phase | Upstream new-api defaults (`upstream/main`) | FlowAPI prod today (`8156149df`, HK) |
|---|---|---|
| Connect | Dial 30 s, TLS 10 s (Go `transport.go:45-56`) | Same |
| Before upstream headers | 1800 s `ResponseHeaderTimeout` (`service/http_client.go:108-116`). Unbounded before rc.28, including at the fork point. A client leaving does **not** cancel the wait (`api_request.go:319`) | **No gateway timeout.** The wait ends only when the client leaves or nginx `proxy_read_timeout 900s` fires; the bound context then cancels the upstream call (`api_request.go:546-548`). Because FlowAPI writes nothing before commit, nginx's 900 s counts from request start and is shared by every failover attempt |
| After headers, before first content | `STREAMING_TIMEOUT` 300 s idle, reset per line (`stream_scanner.go:94-99`, `:256`, `:299-301`). No first-token timeout | No idle timer: `stream_scanner.go:123-133`, per-channel value unread and 0. No first-content timeout either: the hedge threshold is unread, so hedging never runs. Still inside nginx's 900 s pre-commit budget. The gate caps are size-only: 64 events / 10 MiB (`stream_gate.go:11-12`) |
| Between chunks, after commit | 300 s idle, reset per line | No gateway idle timer. FlowAPI's own 60 s pings reset nginx's timer until 30 min after scanner start (`stream_scanner.go:219-220`). After that, nginx cuts the connection after 900 s of silence. Any upstream byte (for example SSE keepalive comments) keeps the stream open indefinitely |
| Total | None (`RELAY_TIMEOUT=0`) | None (`RELAY_TIMEOUT` unset) |
| Dead TCP peer | About 165 s via keepalive (Go defaults) | Same |
| Upstream 504/524 | Returned to the client and never retried (`status_code_ranges.go:31-34`, `relay_error.go:51-52`) | Classified `upstream_timeout`, retried on the next channel and counted toward auto-ban (`relay_retry.go:62-63`) |

**The change that removed the global streaming timeout** is `b9692d6f4` (2026-09-17, deployed to HK 2026-09-18). Since then no gateway idle timeout has been active in prod, for two separate reasons:
- `845a85998` (2026-09-11) reads reliability from `setting`, but the UI writes it to `settings`, so the per-channel value is never read (see `hedge-config.md`).
- The saved values are 0 anyway.

### 8.5 Caveats

- **Go version.** The Go stdlib line numbers come from the local Go 1.25.6 source. The image is built with `golang:1.26.1` (`Dockerfile:10`). `DefaultTransport` and the keepalive defaults have not changed for years, but I did not check the 1.26.1 source.
- **Keepalive figure.** The 165 s figure is computed as idle 30 s plus 9 probes × 15 s, using Go's defaults on Linux. It was not measured.
- **Source of the 524 cluster.** I read the 125–134 s 524 cluster as the upstream relays' proxy timeout; the 524 bodies are HTML error pages. I did not identify which CDN sits behind each channel.
- **Group-probe deadline.** The 60 s deadline of the pre-09-11 group probes is inferred from timing only: request-id timestamp against error time, 98 of 99 at exactly 60 s. The probe code is not in git.
- **Read-only exceptions.**
  - `git fetch upstream` updated local remote-tracking refs and tags only.
  - Inside the prod container I wrote a scratch id list to `/tmp/.dl_ids_readonly` and deleted it in the same command; the deletion was verified.
  - Nothing else was written on the server.
