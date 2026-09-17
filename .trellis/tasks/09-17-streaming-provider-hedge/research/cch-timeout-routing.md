# Research: CCH Timeout Routing and Hedge Racing

- Query: What exactly happens when provider A reaches its first-byte deadline, and which CCH behavior should inform FlowAPI?
- Scope: external source research, with FlowAPI contract references only
- Date: 2026-09-17
- Source: `ding113/claude-code-hub`, pinned SHA `dfeb14331cb350f672e92a3684adecf1052dd476`; inspected local snapshot `/private/tmp/claude-code-hub-dfeb14331cb350f672e92a3684adecf1052dd476`.

## Findings

### Direct Answer

With legacy Hedge enabled and a concurrency cap above one, A's first-byte threshold starts B **without aborting A**. Both remain eligible. The first eligible response commits as winner. Thus if A has a 60-second threshold, B starts around t=60; A can still win at t=61 before B becomes ready at t=62.

Two material differences from the author's pasted description exist in this pinned source:

1. The default content gate requires a valid semantic content event, not simply HTTP headers or any first byte. A ping or role-only prefix does not win under the gate.
2. Loser billing now defaults to enabled. With this setting and a request record, the loser is retained and drained in the background, and its billable cost is added to the request. Immediate loser cancellation is the setting-disabled behavior.

### Trigger and Scheduling

- `src/app/v1/_lib/proxy/forwarder.ts:4692-4704`: legacy Hedge requires endpoint retry and provider-switch permission, `stream === true`, initial provider `firstByteTimeoutStreamingMs > 0`, and no request-level Hedge disable. [Pinned source](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L4692-L4704)
- `forwarder.ts:5057-5105`: threshold expiry marks `hedge_triggered` and invokes `launchAlternative()`; it does not abort that attempt. `forwarder.ts:5236-5257` sets the transport's first-byte hard timeout to zero when cap > 1, replacing destructive timeout with scheduler control. Timer begins on entering transport preparation and resets at actual dispatch; budget waits pause it (`5139-5171`). [Threshold](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L5057-L5105), [transport override](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L5236-L5257)
- `forwarder.ts:5188-5220`: alternative selection excludes every already-launched provider. It uses the existing provider resolver (`4663-4690`), rather than an A/B static pair. No alternative marks `noMoreProviders` but preserves live attempts. The request only fails for exhaustion when there are no more providers **and** no live attempts (`5181-5185`). [Pinned source](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L5181-L5220)
- Every newly launched provider has its own threshold. Runtime cap is 1-4, default 2 (`forwarder.ts:171-182`; schema `903-904`). Cap 2 means B's threshold does not launch C while A and B are both active. An actual failure frees a slot and immediately permits replacement C. Cap 3 permits A -> B -> C while all three are pending. Cap 1 retains hard timeout and serial fallback. The cap counts concurrent attempts, not total providers over the request. [Cap](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L171-L182)
- `forwarder.ts:5887-5998`: alternatives acquire provider session limits, resolve endpoints and create isolated shadow sessions before dispatch. Endpoint-resolution failures do not count as launched upstream requests. [Pinned source](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L5887-L5998)

### Winner Definition and Stream Commitment

- `forwarder.ts:5325-5409`: enforce mode, replay owners and forced Codex Responses handling run a precommit content gate for supported protocol families. Others use the first nonempty readable body chunk. Headers alone are never a winner. Gate mode defaults to `enforce` in `src/drizzle/schema.ts:1053` and `src/lib/config/env.schema.ts:221`; runtime snapshot takes priority over env (`stream-content-gate.ts:157-166`). [Winner selection](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L5325-L5409), [default](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/drizzle/schema.ts#L1053)
- Timer distinction verified: on response headers, `5312` calls `attempt.clearResponseTimeout()`, which is the transport hard timer installed at `4628-4629`; it does **not** clear `attempt.thresholdTimer`. The Hedge scheduler timer is separately armed at `5123-5135`, remains alive through gate/read-first-chunk, and is cleared on winner commitment at `5732-5734` (or failure/cancellation/pause). The nearby comment at `5358` describes the hard timer and must not be read as saying that headers stop the Hedge threshold. `onFirstByte` at `5355-5357` records a timestamp only. Therefore neutral physical bytes can update TTFB while the threshold still launches B because semantic commitment has not happened. [Header handoff](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L5308-L5359)
- `stream-gate/stream-content-gate.ts:18-31`: neutral prefixes are buffered; errors, malformed frames, early EOF and buffer overflow fail before commitment. A clean completed Responses response is valid even if empty. A failed candidate leaks zero bytes to the client. [Pinned source](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/stream-gate/stream-content-gate.ts#L18-L31)
- `stream-gate/frame-classifier.ts:100-115` accepts OpenAI Chat content, reasoning content, tool argument payload, refusal and audio content; a role-only delta is neutral. Responses rules at `121-191` include output text, reasoning and function argument events. Thus "first content" need not mean visible answer text; real reasoning/tool payload can legitimately win. [Pinned source](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/stream-gate/frame-classifier.ts#L100-L191)
- `forwarder.ts:5704-5884`: atomic `winnerCommitted` guard fences double winners, synchronizes the winner's model/session state, records winner, settles all peers, then returns buffered prefix + remaining reader. Provider/session affinity moves to the winner. There is no later rerace after content has committed. [Pinned source](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L5704-L5884)

### Cancellation and Billing

- `src/drizzle/schema.ts:897-901`: `billHedgeLosers` defaults to true. `forwarder.ts:4989-5054`: a loser with billing enabled and an attributable request record is marked settled but **not aborted**; otherwise its response controller and reader are cancelled and transport resources released. [Setting](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/drizzle/schema.ts#L897-L904), [cancel/drain branch](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L4989-L5054)
- `forwarder.ts:4920-4987`: loser drain carries already-read prefix bytes so initial usage is retained. It uses bounded detached-stream admission and a drain deadline, then finalizes loser cost asynchronously. Default drain timeout is 120 seconds (`env.schema.ts:200-202`). This timeout starts when drain starts; it is not a global pre-winner request deadline. [Drain](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L4920-L4987), [default timeout](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/lib/config/env.schema.ts#L200-L202)
- `response-handler.ts:6743-6772,6825-6854`: parses usage, applies billable-response rules, adds loser cost to original request, records per-provider loser breakdown and updates key/user/provider spend counters using an idempotent event identifier. Truncated drain with no usage is not charged; complete legacy responses can still support per-request pricing. Winner and loser retain their own pricing/model context. [Billing](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/response-handler.ts#L6743-L6772), [cost accounting](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/response-handler.ts#L6825-L6854)
- `forwarder.ts:6070-6104`: client cancellation before winner aborts all attempts and stops selecting providers even when loser billing is enabled. Non-retryable client errors also terminate all peers (`5640-5697`). Mere threshold expiry or losing a race does not itself count as provider circuit failure; actual provider failures do (`5658-5675`).
- Closing an HTTP connection does not prove an upstream stopped model work or waived its charge. The author's no-material-cost-change claim is not verifiable from these code/tests; FlowAPI should not promise it.

### The Three Timeouts Are Different

| Setting | Current CCH behavior |
| --- | --- |
| Streaming first-byte threshold | Soft trigger for additional candidate when legacy Hedge cap > 1; original stays alive. Under enforced gate, timer continues until meaningful content/valid completion, even if neutral bytes arrive. Cap 1 retains a hard timeout. |
| Streaming idle timeout | Before commitment, gate read-gap expiry rejects that attempt and allows another candidate. After commitment, aborts upstream and closes client stream; no silent switch or splice into another generated answer. |
| Non-stream total timeout | Hard abort for the attempt; no Hedge. Failures while still inside forwarding enter serial retry/provider fallback. A later timeout during response-handler body processing is finalized as failure, not rerouted. |

- Bounds/defaults: first byte `0` or 1-180 seconds; idle `0` or 60-600 seconds; non-stream `0` or 60-1800 seconds. All provider defaults are zero. Zero disables that provider timer; it does not override lower-level connection/body transport timeouts. `src/lib/constants/provider.constants.ts:43-68`; transport defaults in `env.schema.ts:195-198`. [Provider bounds](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/lib/constants/provider.constants.ts#L43-L68)
- Hard deadline implementation: `forwarder.ts:3811-3909`; non-stream timer is retained through response body handling (`4618-4638`). Timeout becomes 524 in the forwarding phase (`4207-4227`), and provider errors may retry the same provider/next endpoint up to `maxRetryAttempts`, then switch (`3024-3027,3135-3176`). Do not infer "immediate next provider" from nearby comments; actual loop and test allow retries. [Timer](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L3811-L3909), [serial retry](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L3135-L3176)
- Idle after commit: `response-handler.ts:4620-4679` aborts both client/source; `5575-5595` explicitly says no retry once HTTP 200 was sent. Post-forward non-stream body timeout is finalized at `3697-3767`. [Idle](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/response-handler.ts#L4620-L4679), [no retry](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/response-handler.ts#L5575-L5595), [non-stream late failure](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/response-handler.ts#L3697-L3767)

### Failure and Exhaustion

- Explicit retryable candidate failure releases its slot and launches a replacement; an already-running peer can still win (`forwarder.ts:5450-5698`). Request-scoped non-retryable client failure aborts the entire race. Provider-local model 404 does not automatically terminate healthy peers.
- No alternative is not itself a hard timeout: the existing attempts continue. Legacy Hedge has no dedicated total racing deadline; with no provider idle limit and no lower-level failure, attempts can remain pending. Treat this as a design decision to revisit for FlowAPI.
- This revision also has a separate, default-disabled **Discovery** strategy (`schema.ts:906-914`). `sendInternal` tries it before legacy Hedge (`forwarder.ts:1662-1697`); it has waves, sticky SLA and a total racing deadline (`6122-6142`). Those are not implied by the screenshot's three provider timeout fields and should not silently expand FlowAPI's scope. [Discovery distinction](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/src/app/v1/_lib/proxy/forwarder.ts#L6122-L6142)

### Tests Read

These are source-inspected tests, not executed in this research task.

- `tests/unit/proxy/proxy-forwarder-hedge-first-byte.test.ts:1301`: B starts on A threshold and wins; A is cancelled with billing disabled. `1652`: A can still win after B starts. `1734`: cap 2 blocks C until one fails. `1873`: cap 1 serial fallback. `1942`: cap 3 launches C at B threshold. `2022`: client abort cancels all. `2140`: launcher rejection settles. `2222`: provider-local 404 preserves live peer. `2409`: non-retryable client error stops all. [Pinned tests](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/tests/unit/proxy/proxy-forwarder-hedge-first-byte.test.ts#L1301-L2021)
- `tests/unit/proxy/stream-gate-forwarder-integration.test.ts:559`: early error frame fails over without leaking bytes; `614`: neutral prefix flushed once with first content; `660`: terminal-only stream rejected; `690`: valid empty Responses accepted; `974`: fake JSON response cannot win a Codex race. [Pinned tests](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/tests/unit/proxy/stream-gate-forwarder-integration.test.ts#L554-L755)
- `tests/integration/proxy-hedge-lifecycle.test.ts:961`: no fallback splice after actual Responses content committed. `1255-1372`: real loopback transports, winner charged `0.016`, loser charged `0.011`, both exactly once; loser abort count remains zero with billing enabled. [Pinned billing integration](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/tests/integration/proxy-hedge-lifecycle.test.ts#L1255-L1372)
- `tests/unit/proxy/proxy-forwarder-retry-limit.test.ts:930-997`: 524 failures retry five times when configured, across endpoint pool, before vendor/provider exhaustion. [Pinned test](https://github.com/ding113/claude-code-hub/blob/dfeb14331cb350f672e92a3684adecf1052dd476/tests/unit/proxy/proxy-forwarder-retry-limit.test.ts#L930-L997)

## Files Found

- `src/app/v1/_lib/proxy/forwarder.ts`: eligibility, serial retries, legacy Hedge, Discovery, attempt cloning, selection, winner and loser lifecycle.
- `src/app/v1/_lib/proxy/response-handler.ts`: downstream stream lifecycle, idle timeout, accounting, loser billing.
- `src/app/v1/_lib/proxy/stream-gate/stream-content-gate.ts`: precommit buffering and commit/error rules.
- `src/app/v1/_lib/proxy/stream-gate/frame-classifier.ts`: protocol-specific semantic content signals.
- `src/lib/constants/provider.constants.ts`: provider timeout validation ranges/defaults.
- `src/lib/config/env.schema.ts`: transport, drain and gate defaults.
- `src/drizzle/schema.ts`: persisted settings including loser billing, race concurrency and gate mode.
- Tests listed above provide scheduling, protocol and real-transport lifecycle evidence.

## Related FlowAPI Specs

- `.trellis/spec/backend/billing-ratio-routing.md`: winner selection must preserve route-dependent pricing; preconsume must reserve a higher selected route before dispatch, and settlement must use the winner's own route. Parallel attempts cannot share mutable route/pricing context.
- `.trellis/spec/backend/index.md`: backend guidelines index.
- `.trellis/spec/backend/error-handling.md`: read, but generic Electron/TypeScript template content is not authoritative for this Go relay design.
- Root `AGENTS.md`: billing safety, no negative/overflow charge, request scalar preservation, independent relaykit build if affected.

## Caveats / Not Found

- Findings describe the pinned snapshot, not necessarily the older release discussed by the pasted author statement or the user's deployed CCH settings.
- Current default semantic gate and loser billing are material deviations from "first byte wins and all losers immediately close". Do not copy the old prose as implementation acceptance criteria without a deliberate product choice.
- No measured cache-hit or external provider cost comparison was performed. The integration tests validate local accounting and cancellation, not an upstream provider's charging policy.
- Legacy Hedge does not supply a universal maximum request duration. Zero provider timeouts still coexist with transport defaults, and idle timeouts only bound inactivity rather than overall stream duration.
- This research does not prescribe copying CCH's background loser billing into FlowAPI. A scoped initial implementation can cancel losers, settle only the chosen output, and audit speculative attempts while acknowledging possible upstream spend.
