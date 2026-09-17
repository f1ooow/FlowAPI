# Research: FlowAPI streaming provider hedge feasibility

- Query: Implement per-channel first-response hedge racing: keep A alive when its threshold expires, start an eligible B, publish only the winner, and cancel losers.
- Scope: internal; current workspace product source is authoritative. No production code changed and no runtime tests run during research.
- Date: 2026-09-17

## Findings

### Executive assessment

FlowAPI already has most of the serial failover building blocks, including provider exclusion, body replay, protocol-aware precommit buffering, route-dependent billing reservation, and cancellation propagation. It does **not** currently have a concurrent attempt coordinator. Running the existing relay helper twice with the same Gin context / RelayInfo would corrupt routing state and billing, even if both goroutines were prevented from writing simultaneously.

The best existing ownership handoff is `commitBufferedStream` in `relay/helper/stream_scanner.go:279`, immediately before SSE headers are exposed and `MarkCommitted` is called. Extend that boundary to ask a request-level coordinator for permission to win. Do not treat a response header, heartbeat, or `response.created` event as a winner merely because bytes arrived.

### Files found and current contracts

| File | Responsibility / exact anchor |
| --- | --- |
| `controller/relay.go:71` | Request validation, initial preconsume, serial retry traversal, final error/refund. |
| `controller/relay.go:193` | Initializes request `RouteState` and `RetryParam`; current model is one mutable request state. |
| `controller/relay.go:233` | Prepares selected route's ratio and reservation before upstream call. |
| `controller/relay.go:248` | Per-channel attempts, body reset, blocking helper call, attempt metrics, success/error routing. |
| `controller/relay.go:407` | Transparent failover excludes realtime and audio speech/transcription/translation. This is broader than a safe hedge eligibility list. |
| `controller/relay.go:476` | `getChannel`: selects using request exclusions and populates selected-channel context. |
| `service/channel_select.go:168` | Resilient selection, including Auto groups; accepts exclusion set but not currently a custom hedge-capability predicate. |
| `model/channel_resilient_select.go:18` | Candidate eligibility, model redirect eligibility, advanced-custom request path, highest remaining priority, weighted choice. `CandidateAllowed` is available here. |
| `service/route_state.go:25` | Unsynchronized maps and history slice; 20 distinct-channel limit at line 3. |
| `service/relay_retry.go:17` | Stops retry after commitment, client cancellation, local/billing errors; distinguishes retryable upstream failures. |
| `middleware/distributor.go:437` | Assigns credentials, channel settings, model mapping, cost ratio, selected multikey, provider-specific fields. |
| `relay/common/relay_info.go:84` | Mutable per-attempt and per-request state currently combined. |
| `relay/helper/stream_gate.go:64` | Protocol-aware frame classification. |
| `relay/helper/stream_scanner.go:89` | Stream prebuffer, idle timeout, pings, concurrent scanner / output handler, cleanup. |
| `relay/channel/api_request.go:542` | HTTP cancellation propagation and client dispatch. |
| `common/body_storage.go:14` | Replay storage with independent `NewReader` contract. |
| `common/gin.go:36` | Cached body accessor rewinds shared storage cursor. |
| `service/billing_session.go:26` | One billing lifecycle with mutex and a pointer to its owner RelayInfo. |
| `service/tiered_settle.go:164` | Per-route reservation refresh for ratio and expression billing. |
| `service/text_quota.go:434` | Usage, quota accounting, settlement, consume log, cache metrics. |
| `service/log_info_generate.go:135` | Admin-only routing history read during settlement. |
| `relaykit/dto/channel_reliability.go:14` | Existing channel attempts / auto-ban configuration. |
| `relaykit/dto/channel_settings.go:13` | Shared serialized channel settings DTO; independent relaykit module. |

### Current request flow

1. Middleware selects A and writes its settings into Gin context. Request validation and `GenRelayInfo` happen once (`controller/relay.go:122`, `:133`).
2. Controller estimates tokens, determines pricing, and preconsumes once (`controller/relay.go:160`, `:176`). A deferred failure path refunds the billing session and applies the existing violation-fee policy (`:183`).
3. For each selected channel, controller resolves its ratio and reserves any increase (`:233`). Inner attempts call one helper synchronously (`:282`); the call runs until the entire upstream response has been consumed and settled.
4. A successful helper has **already** settled and written the consume log before control returns to the controller. The controller then records route success and channel health (`:298`).
5. An error is classified only after the helper returns. Retry is allowed while no meaningful output is committed (`:327`). Eligible failures consume per-channel attempts; exhausted channels become excluded (`:366`).

This means first-byte hedging is not obtainable by shortening `STREAMING_TIMEOUT`: today's timeout exits the scanner and closes A, after which serial retry begins. A separate launch timer and concurrent coordinator are needed.

### What constitutes first response / a winner

Existing gate behavior is a useful and established contract:

- OpenAI chat: content, reasoning_content, tool/function arguments, refusal, audio data/transcript (`stream_gate.go:106`). Role-only and empty deltas remain neutral. Note: the classifier checks `reasoning_content`, while rendering also knows alternate reasoning fields; do not promise arbitrary dialect parity without fixtures.
- OpenAI Responses: nonempty delta events, content-bearing output items and tool actions, partial images, text/reasoning/tool done events (`:159`). `response.created`, `response.in_progress`, and `response.queued` remain neutral request echoes (`:201`).
- Claude: text, thinking, partial JSON, signature/citation deltas and content-bearing block starts (`:87`). `message_start` and empty content block starts do not win.
- Gemini: text / media / function-call / code-execution payloads; block reason is an upstream error (`:129`).
- Explicit content-free completion can legitimately win: finish_reason / stop_reason / response.completed / response.incomplete; terminal-only empty stream stays an error, except the existing rule permits a terminal after a neutral prefix (`stream_scanner.go:335`). Reuse this behavior unless the product deliberately changes it.
- Prebuffer caps are 64 counted events and 10 MiB ordinary buffered bytes; request echoes are excluded from the ordinary byte cap but the absolute cap is 20 MiB (`stream_gate.go:10`, `stream_scanner.go:350`). Each concurrent attempt needs its own bounded buffer.
- **Existing TTFT is not the winner signal**: `SetFirstResponseTime` is deliberately called for neutral upstream events too (`stream_scanner.go:369`). Keep metric semantics stable; record a separate hedge-ready / committed timestamp instead of redefining existing TTFT.

Candidate timer should start when the selected upstream attempt is dispatched, before HTTP response headers arrive. Starting only inside `StreamScannerHandlerWithGate` misses providers that stall before sending headers. Winner choice should happen at the established valid-content gate, before any downstream header/prefix flush.

### Protocol coverage is determined by adapter path, not client API name

| Path | Existing gate / feasibility |
| --- | --- |
| OpenAI-compatible chat | `relay/channel/openai/relay-openai.go:128`; suitable. |
| Native Responses | `relay/channel/openai/relay_responses.go:88`; suitable. |
| Chat converted through Responses | `relay/channel/openai/chat_via_responses.go:270`; gated using actual upstream Responses protocol. |
| Responses converted through chat | `relay/channel/openai/responses_via_chat.go:91`; gated using actual upstream chat protocol. |
| Claude HTTP SSE | `relay/channel/claude/relay-claude.go:203`; suitable, including downstream conversion. |
| Gemini HTTP SSE | `relay/channel/gemini/relay-gemini.go:153`; native Gemini (`relay-gemini-native.go:82`) and Responses conversion (`relay_responses.go:126`) reuse it. |
| Vertex HTTP SSE | `relay/channel/vertex/adaptor.go:339` dispatches to Gemini / Claude / OpenAI handlers by subtype; audit subtype capability explicitly. |
| AWS Bedrock event stream | `relay/channel/aws/relay-aws.go:260` bypasses the common gate, immediately invokes Claude handler, and uses an SDK stream. Not automatically covered. |
| Other legacy adapters | Many still use ungated `StreamScannerHandler`; eligibility must be explicit. |
| Images / audio / realtime / async tasks | Exclude from first text-hedge implementation unless separately designed and tested. Responses requests may contain server-side tools/image generation even though endpoint is a text API. |

Do not use only `supportsTransparentFailover` to enable hedging: serial precommit failover is safe for more operations than concurrent duplication. A future capability predicate should consider API mode, actual adapter, stream transport, and any stateful request constraints.

### State isolation requirements

1. One Gin context per attempt, constructed from a baseline captured **before** workers mutate anything. `gin.Context.Copy` (Gin v1.9.1 `context.go:112`) copies the key map shallowly, retains the same `Request`, and clears the writer's underlying response writer. It is not a complete attempt clone. Explicitly clone HTTP request/header/URL with a child context, install an attempt-owned writer, and isolate mutable key values.
2. One RelayInfo per attempt. Existing `InitChannelMeta` assigns new channel metadata and converter cache, then mutates `info.Request.SetModelName` (`relay_info.go:190`, `:249`). Even though Text / Claude / Responses / Gemini helpers deep-copy their request DTOs later, a shared Request still races at initialization. Use an independent DTO per attempt.
3. Isolate `ChannelMeta`, request headers/overrides, `PriceData.OtherRatios`, tiered snapshot/request input, request conversion chains, Claude conversion state, tool counters, stream status, timing fields, and usage. Do not JSON-copy the whole RelayInfo: it includes interfaces, websocket handles, private state, and live Billing references.
4. Never share a Gin writer/header map across contenders. Only the winner can bind/publish to the original response. The common scanner has a local write mutex, but A and B would have different mutexes; those do not protect a shared writer.
5. `RouteState` belongs to the coordinator. Maps/slices have no concurrency protection. Workers report typed events/results rather than mutating route history directly. Publish a stable history snapshot for settlement logging.
6. Each attempt gets a fresh adapter. `relay/relay_adaptor.go:56` already creates adapters per helper call, which is compatible with isolation.

### Request-body replay trap

`BodyStorage.NewReader()` explicitly returns independent read cursors (`common/body_storage.go:23`), for both memory and disk storage. However:

- `common.GetBodyStorage` calls `GetRequestBody`, which seeks the shared storage back to zero (`common/gin.go:36`).
- `NewReplayableBodyReader(storage)` still delegates its initial `Read` to `storage.Read` (`common/body_storage.go:350`). Its `NewReader` is independent, but its initial cursor is not.
- The current controller assigns `io.NopCloser(bodyStorage)` per serial attempt (`controller/relay.go:280`). This is unsafe for two live attempts.
- Passthrough handlers themselves call `GetBodyStorage` and `NewReplayableBodyReader`, so merely assigning separate `c.Request.Body` readers is insufficient (`relay/compatible_handler.go:97`).

Minimal safe contract: share immutable storage bytes/lifecycle only, give each attempt an independent reader/seekable view for every accessor, and keep `GetBody` creating additional independent readers. Close attempt readers on completion, retain backing storage until all contenders have stopped. Preserve ContentLength / HTTP2 replay behavior through `ApplyUpstreamBodyMetadata` (`relay/channel/api_request.go:31`). Avoid copying a 128 MiB request N times when existing disk-backed independent readers can be reused.

### Cancellation and loser handling

- HTTP dispatch already attaches `c.Request.Context()` to the upstream request (`api_request.go:546`). A child cancel context can interrupt headers wait as well as response reads.
- Scanner observes request cancellation (`stream_scanner.go:427`), closes response body, stops timers, and waits for worker goroutines (`:141`). Do not return the original Gin context to its pool until all workers are joined.
- **Important trap:** precommit client cancellation currently sets ClientGone and can return `nil`, because the final precommit error creation excludes ClientGone (`:442`). Providers then may run their final-output and usage/settlement paths. A hedge loser must have a distinct typed cancellation outcome and must return before final output / settlement / success metrics; a context cancellation alone is insufficient.
- A and B can become ready simultaneously. Make winner acquisition atomic and reject late candidates. Check ownership before header publication, every worker finalization, and any fallback path that bypasses the normal gate.
- Once a winner is committed, do not switch on a later idle timeout/error. Replaying after client-visible output would mix conversations.
- A timeout that only launches B is not an upstream failure. A cancelled loser is neither a failed provider request for auto-ban nor a succeeded one. Preserve separate actual-failure health accounting.
- Aborting the connection is best effort at stopping the provider's work; the provider may already have billed prompt processing or reasoning. Do not promise no additional upstream cost.

### Billing / accounting invariants

The core existing contract is `.trellis/spec/backend/billing-ratio-routing.md`: recompute route ratio and reserve the larger target before sending a more expensive route; settle with the actual winner's ratio.

Current code makes naive billing reuse unsafe:

- BillingSession is mutex-protected, but references its original RelayInfo (`billing_session.go:26`); Reserve writes compatibility fields back to that object (`:177`, `:337`) and Settle updates subscription fields there (`:75`). Sharing it across mutable attempt RelayInfo copies can produce stale winner logs and races.
- `PostTextConsumeQuota` updates user/channel quota counters before Settle (`text_quota.go:480`), then writes one consume log (`:563`) and metrics (`:585`). Idempotent BillingSession.Settle does **not** prevent duplicate counters/logs if both attempts call PostTextConsumeQuota.
- Four text helper families settle directly after adapter completion: Text (`compatible_handler.go:213`), Claude (`claude_handler.go:228`), Responses (`responses_handler.go:147` onward), Gemini (`gemini_handler.go:202`); Chat-via-Responses also has a separate early settlement branch (`compatible_handler.go:84`).
- Route history is collected by settlement before the controller's success append; its current implementation synthesizes a final succeeded attempt (`log_info_generate.go:135`). A race coordinator must supply a stable winner-aware history, including hedge launch / cancellation, before that point.

Recommended contract: one request-owned billing lifecycle and serialized reservation; attempt-local route price/snapshot; exactly one successful winner settlement and exactly one final refund if no winner. Losers must not invoke PostText/PostAudio consume, usage-cache observers, request success metrics, final protocol trailers, or violation fees. Reservation for B that cannot be obtained should prevent B dispatch and allow already-running A to continue; do not discard a potentially valid A just because the optional hedge is unaffordable. Adopt winner route metadata and subscription fields before settlement.

Two implementation shapes are possible; prefer the first if keeping helper changes scoped is feasible:

1. Add a small attempt ownership contract to RelayInfo and gate selection. Coordinator owns shared billing; workers execute existing protocol helpers with per-attempt state; every successful-settlement entry checks/adopts ownership. Loser cancellation is an explicit non-success return. Carefully synchronize owner metadata before settling; a billing guard alone cannot protect pre-settle side effects.
2. Split the four text helpers into execute/consume-result and settlement phases, letting coordinator settle the winner itself. This creates a clearer accounting boundary but changes more helper APIs and the Chat-via-Responses/audio branches. Preserve existing callers with deliberate interfaces, not a general-purpose relay rewrite.

### Routing, health, affinity, and metrics

- Reuse highest remaining priority / weight selection and existing model+group eligibility. Add active channel IDs to a temporary selection exclusion set so B cannot equal A, without marking A failed or preventing it from winning. Keep actual exhausted channels separate.
- Preserve 20 distinct channels and configured MaxAttempts for real retry paths; do not increment failure count on launch threshold alone.
- `ClassifyRelayRetry` currently treats a cancelled attempt context as a client abort (`service/relay_retry.go:27`); coordinator must distinguish loser cancellation before calling that classifier.
- Auto-ban should count only genuinely exhausted channel failures. Existing controller records failure once at end of channel attempts (`controller/relay.go:357`) and success when helper succeeds (`:310`). Hedge loser cancellation must bypass both.
- Middleware records affinity after relay returns (`middleware/distributor.go:168`). `RecordChannelAffinity` honors current `channel_id` when SwitchOnSuccess is enabled (`service/channel_affinity.go:721`). Promote winner identity to the original context; preserve existing sticky setting semantics instead of accidentally pinning B or always pinning initial A.
- `RecordChannelAttempt` currently takes boolean success (`controller/relay.go:296`). Do not misreport an intentionally cancelled race loser as an availability failure. Either add a neutral/cancelled outcome or omit that sample and add hedge-specific telemetry, with a documented denominator.
- Keep route history under `other.admin_info` so provider IDs/names and reasons do not leak in ordinary user logs.

### Existing timeouts and configuration

- `RELAY_TIMEOUT` defaults to 0 (`common/init.go:111`) and becomes `http.Client.Timeout` (`service/http_client.go:101`), covering the entire HTTP exchange, including streaming reads. This is not a hedge trigger.
- `STREAMING_TIMEOUT` defaults to 300 seconds (`common/init.go:182`). The scanner starts it after headers and resets it on **every scanned line**, even comments / neutral events (`stream_scanner.go:307`). It is not a semantic first-content timer.
- A per-channel first-content launch threshold should be independent of both. Zero should disable this hedge trigger without changing existing global timeout behavior. Do not tell users that zero guarantees infinite waiting if existing global/server limits still apply.
- Channel reliability settings live in `setting` JSON; extending that object avoids SQL schema migration. Existing WithDefaults replaces zero values for the three old fields (`channel_reliability.go:28`); new hedge zero must remain zero, not be defaulted to an enabled value.
- If all three CCH controls are introduced, first-content deadline should launch B, stream idle should abort stalled reading, and nonstream total should abort a nonstream request. Their semantics and fallback behavior must be separate. For idle/total, nullable values are useful to distinguish inherited current global settings from explicit disabled zero.
- Configuration DTOs are under standalone relaykit; runtime coordination remains in root `controller` / `relay` / `service`. Do not import root runtime packages into relaykit. Changes there require `GOWORK=off go build ./...` from `relaykit/`.

### Recommended minimal delivery architecture

1. Opt-in per-channel threshold, default disabled. Explicit initial scope: gated HTTP SSE text APIs; preserve serial behavior for disabled or unsupported paths.
2. A request-scoped coordinator in controller (or a focused root service with controller-provided attempt callbacks) owns selection, attempt counts, reservation, winner state, cancellation, join, and final result. Avoid importing controller/middleware from service.
3. Launch A using isolated request/RelayInfo/body/writer state. Start a launch-only timer at upstream dispatch. When it expires, choose eligible B excluding active/exhausted IDs, reserve B's required amount, and launch B without cancelling A.
4. Gate hook requests ownership on meaningful event / valid completion. First accepted candidate wins atomically. Only then bind headers/output and release buffered prefix; cancel and await all losers. Coordinator freezes route history and promotes winner metadata before winner accounting.
5. No-candidate / capacity-reached threshold leaves live attempts running under their existing hard limits. Retryable genuine failure consumes current attempt budget and can refill a free racing slot; nonretryable local/client errors preserve existing stop semantics.
6. After winner commitment, stop all launch timers and selection; winner streams normally and is settled once. Client disconnect cancels every attempt and joins all workers.
7. Use an explicit bounded concurrency policy. A cap of two live attempts is a conservative starting recommendation, not yet a user-approved requirement. Define whether B's own timer may queue C, whether C waits for a slot, and how real failures interact with same-channel retry limits.

### Test targets and meaningful verification

Existing fixtures to extend:

- `relay/helper/stream_scanner_test.go:214`, `:234`, `:261`, `:315`, `:449`, `:517`, `:561`: gate silence, valid content, empty completion, TTFT accounting, protocol classifiers, cancellation/body closure.
- `relay/channel/api_request_stream_precommit_test.go:25`: no SSE headers / pings before commitment while waiting for headers.
- `relay/channel/api_request_getbody_test.go:462`, `:497`, `:529`: independent body replay across HTTP2 retries and passthrough.
- `service/route_billing_test.go:30`: larger reservation before dispatch; expand to winner ratio and attempted optional hedge reservation failure.
- `service/relay_retry_test.go:14`, `:57`, `service/route_state_test.go`, `service/channel_select_auto_groups_test.go`: classification / exhaustion / grouping.
- `relaykit/dto/channel_reliability_test.go:10`: default disabled, bounds, round-trip zero/unset semantics.
- `model/log_route_history_test.go`: admin-only history stripping.

New coordinator integration tests should use two httptest upstreams controlled by channels/barriers, and a controlled timer trigger; avoid arbitrary sleeps or wall-clock performance assertions:

1. A wins before threshold: B receives no request.
2. Threshold fires, A still lives; B wins; A receives cancellation; exactly B's headers/body/usage/log and one settlement.
3. Threshold fires, then A wins; B is cancelled; proves original request was not aborted at threshold.
4. A and B report readiness concurrently: one winner, no mixed frames, no duplicate quota/log counters.
5. Header-only, heartbeat, role-only / created events do not win; meaningful reasoning/tool content and valid empty completion do.
6. A errors while B is pending; B can still succeed. B errors while A remains pending; A is not discarded. Both fail; refund once and preserve route retry/health rules.
7. No backup, threshold zero, pinned channel, unsupported adapter, or concurrency cap: no unintended extra dispatch.
8. Same-priority B is selected before lower-priority C; active IDs excluded; original model and per-channel remapping remain isolated.
9. Different group/channel ratios, free-to-paid route, subscription funding, expression snapshots: reserve before dispatch and settle only winner with correct metadata.
10. Client disconnect during headers wait / buffered prefix / concurrent readiness: all upstream work closes and no goroutine accesses pooled Gin context.
11. Large memory and disk-backed passthrough request bodies remain byte-identical under simultaneous reads and transport GetBody calls.
12. Postcommit idle failure never switches provider; cancelled loser does not change auto-ban counters or success/failure denominator incorrectly.

Run focused package tests, then `go test -race` on changed coordinator/scanner/billing packages because concurrency is the feature's central risk. If relaykit DTOs change, run its independent build and DTO tests separately. Broaden only for new failures/changed cross-layer boundaries.

### Related specs and references

- `AGENTS.md`: relaykit independence, common JSON wrapper, billing invariants, meaningful testify tests, DB compatibility.
- `.trellis/spec/backend/billing-ratio-routing.md`: route-dependent reservation/settlement contract; this is a real Go project contract.
- `.trellis/spec/backend/global-passthrough.md`: body/header independence, original raw request semantics, custom transport exclusions.
- `.trellis/tasks/09-01-resilient-routing-model-redirects/prd.md`: earlier serial-failover intent; current source verified above takes precedence.
- Gin dependency: `github.com/gin-gonic/gin v1.9.1` in `go.mod`; inspected installed `Context.Copy` implementation for shallow-copy/writer behavior.
- Go standard `context`, `net/http` request cancellation / ResponseWriter ownership underpin proposed integration; no external CCH assertions were inferred in this backend subtask. Parent session researches CCH source independently.

## Caveats / Not Found

- No hedge coordinator, per-channel first-byte timer, or dedicated cancelled-loser outcome was found in current product source.
- Backend `error-handling.md`, `quality.md`, and shared `code-quality.md` still contain Electron/TypeScript template material; use AGENTS.md and the concrete Go contracts above, not the unrelated npm/Drizzle examples.
- Research did not verify production ingress timeouts, provider billing-after-cancel policies, or live provider behavior. No claims about unchanged cache hit rate / upstream cost can be established from FlowAPI source alone.
- Product decisions for parent planning: two live attempts versus cumulative A/B/C fanout; maximum total attempts; start timer before local conversion vs at transport dispatch; first raw byte vs meaningful content (recommend existing gate); whether to include all three timeout controls now; scope of adapters; stateful Responses `previous_response_id` and server-executed tools; neutral loser metrics / operator-visible cost accounting; optional hedge reservation failure behavior; timer reset semantics and inherited zero/unset rules.
- The user has already specified that A stays alive and B races after A's threshold; that requirement is settled and should not be asked again.
