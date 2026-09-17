# Design: Streaming Provider Hedge Racing

Status: approved for implementation on 2026-09-17, including user-funded racing and conditional channel-ratio billing.

## Evidence

- `research/cch-timeout-routing.md` pins CCH v0.9.5 source at `dfeb14331cb350f672e92a3684adecf1052dd476` and distinguishes threshold, idle and nonstream deadlines.
- `research/configuration-surface.md` maps the existing channel JSON/form/drawer contracts.
- `research/flowapi-backend.md` records exact backend anchors, adapter capability boundaries, body-reader and cancellation traps, and billing/affinity ownership.
- `research/hedge-user-billing.md` supersedes the earlier winner-only recommendation and records current funding behavior, usage provenance, log/statistics effects and persistence limits.
- Existing FlowAPI resilient-routing PRD and `.trellis/spec/backend/billing-ratio-routing.md` remain authoritative for routing and pricing compatibility.

## Configuration Contract

Extend the existing channel reliability JSON, not database columns, with bounded integer seconds for first-content threshold, streaming idle timeout and nonstream total timeout. All three default to zero: omission and explicit zero disable the respective timer. A manually configured positive value enables it. Independent transport/client safety limits remain separate. Validate on channel create/update independently of frontend validation.

Keep the compact routing settings drawer. Display and save missing timeout fields as zero, without a blank/inheritance state. Bounds follow the reference: first content zero or 1-180 seconds; idle zero or 60-600; nonstream zero or 60-1800. No bulk mutation of existing channel records.

Global setting: `general_setting.bill_hedge_losers` (boolean, default true), exposed in the existing routing reliability settings. Snapshot it at request admission. On winner selection, enable bounded drain only for peers that have already received a protocol-valid response prefix and only if this setting is enabled; headers/heartbeats alone are insufficient. All other losers are canceled and their reservations refunded without charges. The disabled state cancels all losers and charges only the winner. This latest user instruction supersedes unconditional loser retention below.

## Request Ownership

The relay request owns the scheduler, route history, client response and winner decision. Every attempt owns its billing reservation/lifecycle, independent cancellation context, body reader, parsed request/DTO, mutable headers, channel metadata, model mapping, RelayInfo, and stream status. Copying a Gin context or RelayInfo shallowly is insufficient. Do not concurrently seek/read a shared BodyStorage cursor.

Keep the current serial path for unsupported operations and disabled racing. Introduce the race at the existing supported text relay orchestration boundary, preserving provider adapters and the shared stream scanner rather than duplicating protocols.

Use explicit actual-adapter/transport capability checks, not just client endpoint names. AWS event streams and other adapters that bypass the common gate remain serial. Exclude upstream-state-dependent requests such as `previous_response_id` and upstream-executed tools from speculative duplication; preserve ordinary client-executed function-call output. Selection must skip unsupported backup adapters before dispatch.

The coordinator serializes channel selection, attempt admission and winner commitment. Each concurrent attempt independently preconsumes its own route estimate before dispatch, even if it is cheaper than A. Combined reservation is additive, not the serial-retry maximum. Existing A reservation is adopted rather than reserved twice. Pricing snapshots and BillingSession ownership stay attempt-local.

Failure to reserve an optional backup prevents that backup from being dispatched but leaves a funded original attempt eligible to finish. It must not cancel A or trigger a second refund lifecycle.

## State Transitions

1. Select/prepare A using existing routing and billing rules; begin A's monotonic threshold when its upstream attempt starts.
2. Before commitment, A remains active. Threshold expiry requests another eligible channel. If a slot and candidate exist, prepare and dispatch B without cancelling A.
3. At most two active attempts exist. Exclude already-running and exhausted channels from alternative selection. Actual retryable failure may free a slot; preserve existing attempt/traversal caps.
4. The shared semantic stream gate offers a candidate when valid content or legitimate empty completion is ready. The first accepted candidate becomes the sole client writer. Flush its prefix once; eligible prefixed peers become background usage collectors only if global loser billing is enabled; other peers are canceled/refunded.
5. Losing and failed prefixes never reach the client. Losing workers have explicit completion/timeout/cancellation outcomes and a dedicated accounting finalizer; they cannot run delivered-answer success bookkeeping.
6. Before commitment, client cancellation stops all work and launches. After commitment, winner cancellation remains client-bound, while already-detached loser collectors have their own finite lifetime. No candidates alone does not terminate still-live attempts.
7. After commitment, keep the winner for the response lifetime. A later idle timeout ends that stream; it does not replay the user request.

The existing `relay/helper/stream_gate.go` classifies Chat, Responses, Claude and Gemini content. Preserve the distinction between first upstream event telemetry and semantic readiness; do not redefine existing TTFT metrics silently. HTTP headers, role-only events, `response.created`, and keepalive noise cannot win.

## Billing And Side Effects

Preserve the existing group-pricing decision rather than unconditionally multiplying channel cost ratios. `relay/helper/price.go:46` HandleGroupRatio resolves Auto group, legacy overrides and user-group factors, reads `GetIncludeChannelRatio(relayInfo.UsingGroup)`, and multiplies the selected channel ratio only when enabled. `setting/ratio_setting/group_ratio.go:161` defaults a missing include flag to false. Call this existing resolver for each isolated selected attempt and snapshot its effective ratio and components before dispatch; both preconsume and eventual winner/loser settlement use that snapshot. Do not add a second channel-ratio multiplication in the hedge finalizer or read current group settings during background settlement.

For the normal migrated pricing path: effective ratio = base group ratio * user-group ratio * (include channel ratio ? attempt channel ratio : 1). With base 2 and user-group factor 1, channels A=0.5 and B=1.5 both use effective ratio 2 when channel inclusion is off; they use 1 and 3 respectively when it is on. Per-attempt token/cache usage may still differ, so equal effective ratios do not imply equal final charges. Preserve the resolver's legacy early-return override when migration is incomplete.

The user explicitly chose default-on billing for eligible losing work, with a global opt-out that charges only the winner. Use one independent BillingSession per attempt, with immutable route pricing and usage, correlated under one client request. Do not share the initial BillingSession across clones: Reserve and Settle write their owner's RelayInfo. Preserve existing funding preferences and unlimited/token rules; current NewBillingSession chooses wallet or unlimited and this task must not revive dormant subscription selection. If an already-supported alternate funding path is reached, its reservation identity must also distinguish attempts.

Each attempt finalizes exactly once. Refund only that attempt's unused reservation; parent failure cleanup cannot refund settled peers. Keep charges, user/token/channel counters and consume logs behind the same guarded finalizer: BillingSession.Settle idempotence alone does not protect the surrounding counters/logs. Do not blindly retry non-idempotent wallet/subscription delta writes after uncertain errors; retain observable failure state for reconciliation. Preserve quota saturation safeguards and per-attempt billing-expression snapshots. Do not sum raw A/B tokens and price the combined amount as one generation.

For additive reservation, set the existing ForcePreConsume flag before hedge-enabled A's initial preconsume as well as B's; otherwise trusted wallet requests can bypass reservation entirely. Preserve free/unlimited account rules. This is estimated preconsumption, not an escrow guaranteeing coverage of every possible output token. The exactly-once guard covers a live attempt: existing wallet/token/log operations are not one durable transaction, and a new crash-recovery ledger is outside this feature's scope.

Record one correlated charge detail per billable attempt using the existing request_id lookup and attempt identity/role metadata. Keep each channel's usage/cost attribution on its own log row; do not add loser charges to both a summary consume row and a loser consume row. User-visible log details show winner/extra racing attempt, charge and completion state without exposing private channel identity. Summing correlated charges yields the request total; loser completion can add a later entry. Do not use the optional log database as the authoritative wallet ledger.

Separate financial attempt counts from logical request statistics. Both attempts contribute billable tokens/quota, but losing finalization must not increment successful response counts or supply normal success-latency samples. Audit existing consume-row-count RPM queries so extra charge rows do not silently inflate logical request RPM; group by public request identity where appropriate, preserving legacy row behavior. Review stable pagination for same-request charge rows sharing a timestamp, including supported separate-log-database configurations.

For eligible losers under the enabled global billing policy, winner selection transfers loser readers, their buffered usage prefix and immutable billing/auth metadata to bounded background collection. A collector never touches the original Gin writer, request context pool, request-body backing storage after release, or winner affinity. Retain necessary resources under explicit ownership and release them on every exit. Set a finite 120-second collection deadline from detachment, retain provider hard deadlines, and bound collected bytes/global concurrency; admission failure cancels the loser and finalizes only available evidence. A loser without a valid protocol prefix at winner selection is canceled immediately; the collection deadline applies only to admitted prefixed losers. Collect metering data incrementally rather than retaining the losing answer indefinitely.

Transport contexts must support this lifetime from dispatch: an already-issued HTTP request cannot be detached by merely replacing a Gin request pointer later. The coordinator propagates client cancellation to all candidates before commitment and the winner afterward, while loser contexts obey their detached deadlines. Capture request ID, billing identity and required values explicitly; a detached worker must not retain pooled Gin state.

Interrupted collection with no trustworthy usage cannot invent a full-token or per-request charge. Explicit partial usage must preserve completeness/provenance and follow the relevant pricing contract; complete per-request-priced responses may follow existing per-request billing. Missing usage and accounting errors are distinguishable from a known zero charge. This is not a promise of exact external-provider invoice recovery.

Follow the researched CCH rule for an interrupted loser with no reported usage: mark unmetered and do not invent a prompt-only charge. Do not pass nil usage as zero to existing helpers, since nil can trigger estimated prompt billing. Winner estimation behavior remains unchanged. Any future policy to bill estimated losing usage must be explicit rather than inferred from the user's decision about who bears known charges.

Cancellation and race loss are not upstream failure evidence for auto-ban. Real provider errors retain existing classification. Winner channel/model/group must propagate to consume logs, counters, response metadata and affinity. In particular, distributor post-processing must not overwrite the winning affinity with the initially selected channel.

Record request-correlated attempt launch, threshold, winner, losing collection, metering completion, cancellation and failure states using existing route history/admin logging, without exposing credentials. Losing completion is chargeable but does not count as a client-delivered response or overwrite winner latency/affinity. Client-visible usage frames remain the winner's protocol payload; extra attempt charges are exposed in the billing log rather than corrupting its usage fields.

## Hard Timeout Behavior

- Streaming idle override measures upstream inactivity and resets on incoming transport data at the existing scanner boundary. It can fail an uncommitted candidate; committed responses end without fallback.
- Nonstream total override runs from upstream dispatch through body consumption. Bind it to that attempt's request context; only uncommitted failures may use existing retry policy. Do not stop timing when headers arrive.
- Hedge threshold is a soft scheduling event. It must not cancel the original attempt or be implemented by the hard deadline context.
- All timer/cancel/close paths must release response bodies and goroutines, including malformed/empty responses and late completions.

## Compatibility And Rollback

No new database dialect behavior or destructive migration. Absent timeout fields behave as zero and disable the respective timers. Existing priority/group/redirect/affinity policies remain in force. Unsupported operations keep current code paths.

Rollback can disable racing through per-channel configuration; code rollback can ignore the added JSON keys. No production settings or deployment changes are part of this task.

## Validation

Use controlled clocks/signals and real loopback upstreams for deterministic lifecycle tests: A wins before threshold, B starts after threshold, late A wins, B wins, neutral prefix does not win, invalid prefix fails, no backup, slot saturation, candidate failure replacement, client disconnect, and no post-commit replay. Verify one response owner, additive preconsumption, winner plus loser charges with different route prices, exactly-once per-attempt finalization, failed-backup admission, bounded drain before/after headers, incomplete usage, late billing, isolated refunds, affinity, and health outcomes. Run the affected Go packages with the race detector.

UI verification covers zero defaults/bounds, load-save round-trip, field errors, all new keys in the currently shipped zhCN locale, and drawer usability. Live i18n/config.ts supports only zhCN; preserve the runtime rather than restoring removed locales based on stale generic guidance. Follow repository typecheck/lint/build and targeted tests. If relaykit is affected, independently build with `GOWORK=off`.
