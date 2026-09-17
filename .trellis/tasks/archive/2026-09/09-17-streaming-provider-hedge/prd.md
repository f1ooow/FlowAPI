# FlowAPI streaming provider hedge racing

## Goal

Implement configurable streaming provider hedge racing in FlowAPI, informed by Claude Code Hub. Reduce long waits for GPT and other streaming models by starting an eligible alternative channel after the initial channel's first-byte threshold while keeping the original request alive.

## Requirements

- Research and explain CCH's three timeout settings, winner selection, cancellation, routing, and cost implications from pinned source.
- Add per-channel configuration and the corresponding relay behavior in FlowAPI, not merely a configuration screen.
- Preserve the initial request when the hedge threshold elapses; forward only the winning attempt. Apply the global loser-billing policy: by default retain only losers with an already valid response prefix for bounded background usage collection and charge their billable consumption; disabling that policy cancels losers and charges only the winner.
- Preserve channel eligibility, request isolation, billing correctness, and cancellation when the client disconnects.
- Determine compatibility with existing routing, stream precommit, and timeout controls before implementation.

## Acceptance Criteria

- [x] CCH behavior is documented with pinned source references and a concrete A/B timeline.
- [x] The agreed hedge policy is implemented and configurable from the channel admin interface.
- [x] Deterministic regression checks cover slow A / winning B, late winning A, no eligible backup, client cancellation, and billing isolation.
- [x] Existing routing and supported non-hedged requests remain compatible.

## Approved Scope

- Add the three per-channel controls shown in CCH: streaming first-content hedge threshold, streaming idle timeout, and non-stream total attempt timeout. The hedge trigger is semantic first content, not a TCP byte, HTTP header, heartbeat, or startup event.
- Initially support replay-safe text requests through OpenAI Chat Completions, OpenAI Responses, Claude Messages, and Gemini generation paths using the existing protocol-aware gate.
- Eligibility must also check the actual upstream adapter/transport. Ungated adapters, upstream-state-dependent requests (including `previous_response_id`), and upstream-executed tool operations remain serial. Client-executed function-call output can participate through the existing content gate.
- Keep at most two upstream attempts active. Start an eligible alternative when the initial threshold expires; preserve the initial attempt. An actual failure may free a slot for another eligible channel within existing traversal limits.
- First valid content or legitimate empty completion wins atomically. Forward its buffered prefix once, apply the global policy to peers, and never switch after response commitment.
- Add a global “Bill provider racing losers” switch, enabled by default. When enabled, only losers that already received a protocol-valid response prefix when the winner is selected may continue bounded background reading and be billed; other losers are canceled and refunded. When disabled, cancel and refund every loser and charge only the winner. HTTP headers and heartbeat noise alone are not a valid response prefix. Snapshot the setting for each request so changing it does not change an in-flight request’s policy.
- Use existing group/model/channel eligibility, priorities, affinity restrictions, per-channel attempt budgets, exclusions, and 20-distinct-channel safety cap. A channel already running cannot be selected as its own backup.
- User-confirmed billing policy: with loser billing enabled, the requesting user pays for both winner and eligible billable losing attempts. Each attempt uses its own usage and existing effective group-pricing snapshot; a losing attempt is not free merely because its output was not delivered. Do not blindly double the winning price or merge tokens across pricing tiers.
- User-confirmed group rule: channel ratio is conditional, not mandatory. Reuse existing HandleGroupRatio for each attempt's actual selected billing group. The normal effective ratio is base group ratio * user-group ratio * (include_channel_ratio ? selected channel ratio : 1). Groups with channel ratio disabled must ignore it for both preconsume and settlement, including losing attempts. Preserve Auto-group resolution, configured zero multipliers, and existing legacy override behavior.
- Collect losing usage in the background with finite time, byte and concurrency limits; the client need not wait for loser completion. Do not emit losing output to the client or store it as a second delivered answer.
- Reserve separately for each billable concurrent attempt before dispatch. Combined reservation must cover concurrent attempts' estimates, not just the more expensive route. Settle/refund each attempt independently and at most once. Preserve the existing funding policy; this feature must not enable dormant subscription selection.
- Record correlated per-attempt charge entries under the same client request ID, with winner/loser role, usage, fee and settlement state visible to the user. The request total is the sum of settled attempt charges. Keep private channel/pricing internals admin-only; late loser charges must remain attributable to the original request.
- Count one logical user request and at most one delivered response; accumulate all billable attempts' tokens/fees without turning extra charge rows into successful response or logical-RPM samples.
- Missing usage after interrupted drain is not fabricated or silently presented as zero provider cost. Settle trustworthy billable evidence under the attempt's pricing contract; mark incomplete/unmetered attempts and settlement errors explicitly. A complete response priced per request can follow existing per-request rules.
- Consistent with the researched CCH behavior, interrupted losing requests with no reported usage are unmetered and are not charged a guessed prompt/full-request amount. This does not assert that their external provider cost was zero.
- Audit launch/winner/loser/background completion/cancellation without counting race loss or threshold expiry as automatic-ban failures or treating loser completion as delivered-answer success.
- If an optional backup's reservation cannot be obtained, do not dispatch it; allow the already-running original attempt to continue.
- All three timeout settings default to zero and require a manually configured positive duration to enable. Missing settings behave as zero; UI fields display and save zero without an inheritance state. Independent transport/client safety limits remain separate.
- Idle timeout before commitment permits existing eligible fallback; after commitment it terminates the current stream without replay. Nonstream total attempt timeout cancels the attempt and uses existing retry rules only while the response is uncommitted.
- No eligible backup means continuing the original live attempt subject to its existing hard deadlines. Before a winner, client disconnect cancels every active attempt and stops launches. After a winner, the delivered stream follows client cancellation while already-detached loser accounting remains bounded; no further upstream requests are launched. Collected billable usage is not erased by cancellation.

## Additional Acceptance Criteria

- [x] Headers, heartbeat and initialization events cannot win; existing valid empty-completion behavior remains supported.
- [x] No more than two upstream requests are live, no channel races itself, and both attempt slots remain eligible to win.
- [x] A losing attempt cannot emit user content, settle/refund another attempt's reservation, reset channel health, or overwrite affinity.
- [x] Concurrent attempts reserve their own estimates before dispatch; both winner and billable losers settle once with their own pricing snapshots. Insufficient backup quota leaves the funded original request alive.
- [x] With include_channel_ratio disabled, different A/B channel ratios do not affect either charge. With it enabled, each attempt applies its own channel ratio exactly once. Each attempt follows its own resolved group when Auto routing changes groups; asynchronous loser settlement uses the captured rule, not current mutable settings.
- [x] The global switch defaults to enabled, persists both true and false, and enforces prefix eligibility when enabled and winner-only billing when disabled.
- [x] Loser drain has bounded resources and cannot delay winner output. User logs identify late loser charges and their relationship to the original request.
- [x] Duplicate completion callbacks, interrupted drain, missing usage and billing failures cannot produce duplicate charges, fabricated usage or cross-attempt refunds.
- [x] Extra billed attempts do not inflate logical request counts, successful-response metrics or logical RPM; their fees/tokens still contribute to consumption totals.
- [x] Idle timeout and nonstream deadline each enforce their documented scope without post-commit replay.
- [x] Old channel JSON remains compatible; missing values display/save as zero, and zero/positive timeout settings round-trip through API and UI with server validation.
- [x] New controls follow the existing admin drawer and current shipped locale (zhCN); typecheck, affected tests, lint and build pass. Live i18n/config.ts supports only zhCN, superseding the stale generic seven-locale assumption.

## Out Of Scope

- CCH Discovery waves, sticky SLA/cooldown strategy, or configurable concurrency above two.
- A new durable wallet ledger or process-crash reconciliation system; live-attempt deduplication must not be advertised as cross-restart exactly-once accounting.
- Racing images, audio, video, tasks, WebSockets, or other operations without the agreed replay and protocol-gate contract.
- Seamlessly joining two generated answers after one has begun streaming.
- Production deployment or changing live channel settings.

## Risks

- Background losers may finish substantial generation, so a single client request may incur two billable attempts; this is the user's explicitly chosen cost policy. Missing upstream usage can still prevent exact charge reconciliation.
- Existing mutable relay context, body readers and billing sessions were designed for serial retry and require explicit isolation.

## Review Status

The user explicitly approved implementation after reviewing user-funded racing and conditional group/channel-ratio billing. These artifacts supersede the previous winner-only accounting proposal. Implementation authorized on 2026-09-17.
