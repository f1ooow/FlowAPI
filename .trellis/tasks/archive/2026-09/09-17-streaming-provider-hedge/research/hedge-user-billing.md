# Research: User-paid hedge attempt billing

- Query: Reuse FlowAPI billing for independently reserved and settled hedge attempts, including cancelled losers, charged to the requesting user; assess usage estimates, logs, subscriptions, and failure durability.
- Scope: internal, incremental billing research only. Read `research/flowapi-backend.md` first; no routing/gate redesign, product edits, task-plan edits, or tests.
- Date: 2026-09-17

## Findings

### Requirement correction

The user explicitly rejected operator-funded hedge overhead. This document **supersedes the winner-only billing recommendation** in `research/flowapi-backend.md`. Each actually dispatched attempt owns its own billable usage, route pricing, reservation, settlement/refund, and consume-log row. The winner alone owns downstream output, not all accounting. Loser cancellation is an accounting outcome, not an automatic free request.

Request charge is the sum of attempt charges. With A reserving 100 and B reserving 150, live reservation is 250, not 150. If A later settles for 40 and B for 110, charge is 150 and their unused reservations return independently. Do not sum estimated tokens and evaluate one price expression: A and B may have different route ratios, cache treatment, or nonlinear pricing tiers.

### Current files and reusable boundaries

| File | Relevant current code |
| --- | --- |
| `service/billing.go:21` | `PreConsumeBilling` creates and attaches a new session; checks negative quota and saturation. |
| `service/billing_session.go:26` | Session retains exactly one RelayInfo owner; in-memory mutex/terminal flags. |
| `service/billing_session.go:187` | Token reserve, then funding reserve, compensating token rollback if funding fails. |
| `service/billing_session.go:301` | Trust bypass; `ForcePreConsume` already disables it. |
| `service/billing_session.go:361` | Live factory currently constructs UnlimitedFunding or WalletFunding only. |
| `service/funding_source.go:51` | Wallet initial reserve is atomic `TryReserveUserQuota`. |
| `model/quota_reserve.go:144` | Conditional DB reserve and Redis-backed atomic reserve with persistence/compensation. |
| `service/quota.go:388` | Atomic token initial reserve; playground exemption. |
| `service/funding_source.go:89` | Retained SubscriptionFunding implementation, currently not constructed by live factory. |
| `model/subscription.go:1238` | Subscription preconsume record unique request ID, 64 chars. |
| `service/tiered_settle.go:127` | Route snapshot refresh and reservation; null Billing creates a new session. |
| `service/text_quota.go:268` | Existing ratio/cache/tool/fixed-price quota computation. |
| `service/text_quota.go:434` | Text finalization: counters, settlement, logs, observations/metrics currently coupled. |
| `service/billing_usage.go:20` | Canonical BillingUsage preferred over display usage; estimated/source classifications. |
| `service/usage_helpr.go:22` | Existing local prompt + observed response-text estimation. |
| `model/log.go:491` | Additive consume-log insert; request ID read from Gin context. |
| `model/log.go:715` | User log filtering by shared request ID. |
| `web/src/features/usage-logs/components/dialogs/details-dialog.tsx:649` | Existing request/upstream ID display; billing-source and stream details. |

### Independent sessions, minimal correct reuse

1. On the hedge-enabled path, transfer A's existing initial reservation/session exclusively to A, or branch before controller preconsume so each attempt creates its own session. Do not create a second session for A without retiring the first. `controller/relay.go:176` and its deferred refund at `:183` currently assume one owner and must be adjusted narrowly for the hedge path.
2. Construct each attempt's RelayInfo with no copied Billing or stale subscription/preconsumed fields. Compute its selected route ratio and use the existing `PrepareBillingForSelectedRoute` with `Billing == nil`; it creates a separate reservation for the full target. The attempt snapshot remains associated with that same session through completion.
3. Set `ForcePreConsume = true` for all hedge-enabled paid attempts, including A before its initial reserve, if the desired contract is genuinely additive reserved quota. Otherwise the existing trust bypass can reserve zero on wallet sessions. This flag already exists and avoids inventing another preconsume API.
4. Serialize launches/reservation calls through the coordinator, while using existing atomic wallet/token reserves for cross-request concurrency. If B cannot reserve, B must not dispatch; A continues. Record the skipped hedge separately from an upstream billable attempt.
5. Each dispatched attempt is finalized once on any outcome, including loser cancellation, client disconnect, and upstream failure with observed billable usage. Settlement with `actualQuota = 0` or full refund is used only when that attempt has no charge under the approved usage policy. Retrying the same channel is a new actual attempt and gets its own reservation/settlement identity if its previous attempt already incurred a charge.
6. Never run the parent failure defer to refund all sessions indiscriminately. A winner failure cannot erase a loser charge already settled; a successful winner does not suppress loser accounting. Coordinator joins workers and owns one-time finalization decisions.
7. Reuse the existing pricing calculator and `SettleBilling`; change only the text finalize boundary enough to attach attempt metadata and separate financial accounting from response success. Existing helper methods returning only an error may need an attempt result carrying observed usage even on cancellation/error. Do not build a new general wallet ledger for this feature.

`BillingSession.Settle` is idempotent only for its own funding/token balance changes within that live object. It does not guard `PostTextConsumeQuota`'s counters/logs, which happen separately. The coordinator/attempt finalizer needs its own exactly-once-in-process state covering the entire accounting action. Calling PostText twice would double `used_quota`, logs, and telemetry even if Settle returns early on its second invocation.

### Funding behavior and limits

**Wallet:** initial reserve uses `model.TryReserveUserQuota` (`funding_source.go:55`), whose DB form is `WHERE quota >= amount` (`quota_reserve.go:144`). Reusing a shared BillingSession.Reserve for B is wrong both because it reserves max rather than sum and because its wallet top-up path deliberately allows debt (`billing_session.go:243`). Fresh session preconsume correctly refuses an unaffordable speculative dispatch. Final actual charges can still exceed the estimate and take the wallet negative via `WalletFunding.Settle`; that is existing usage settlement behavior, not full future-cost escrow.

**API token:** every attempt uses `PreConsumeTokenQuota` and `TryReserveTokenQuota` (`service/quota.go:388`, `model/quota_reserve.go:203`). Finite tokens get independent additive reservations. Unlimited tokens skip the limit but still update used/remaining accounting. Playground skips token reserve/settlement. Do not charge wallet twice to emulate token cost; these are two accounting limits on one charge.

**Unlimited user:** current factory creates `UnlimitedFunding` (`billing_session.go:365`), which does not touch wallet but retains usage and token accounting (`funding_source.go:26`). User-paid hedge must respect this existing account entitlement.

**Free route:** preserve effective zero pricing; do not force a wallet debit merely because an attempt exists. A free A and paid B must not copy A's FreeModel or nil billing behavior to B.

**Subscriptions, current-source caveat:** comments claim subscription-first / wallet-first behavior, but the currently inspected factory ends with `return tryWallet()` (`billing_session.go:406`), and a repository search found no construction `SubscriptionFunding{...}`. Subscription funding types and model functions remain. Do not claim subscription routing is active and do not restore it incidentally in hedge work. Coordinate with concurrent product changes; earlier source or comments are insufficient authority.

If a subscription path is restored or another caller uses it, each attempt must get a distinct immutable billing operation key. `SubscriptionPreConsumeRecord.RequestId` is unique (`subscription.go:1240`); repeated A/B keys return A's reservation rather than deducting B (`:1314`). Use a bounded derived/generated attempt ID (max 64 chars), distinct from the public correlation ID. `SubscriptionFunding.requestId` must use it for both reserve and refund; keep Gin's common RequestIdKey as the original client request ID for logs. Do not change outward request IDs just to obtain independent subscription charges.

Subscription finalization adjusts actual usage by delta (`funding_source.go:123`, `subscription.go:1510`). It enforces total limit, so postconsume can fail when usage exceeds the reserve. It is not an unlimited debt wallet. Existing preconsume/idempotency records do not represent a durable settled state: status is consumed/refunded, and settlement delta is not keyed by attempt. Preserve this limitation explicitly; do not assert complete subscription exactly-once billing from the preconsume unique key.

### Truncated usage: existing behavior and necessary extension

The user-paid policy needs an explicit distinction between **observed usage**, **estimated usage**, and **unknown provider cost**. Connection cancellation cannot reveal unreported reasoning tokens. No source change can guarantee matching the upstream invoice when the provider stops before sending usage.

| Protocol | Existing fallback after stream processing |
| --- | --- |
| OpenAI chat | Without valid final usage, estimates prompt tokens and collected response text, plus existing tool token approximation (`relay/channel/openai/relay-openai.go:184`). |
| Claude | Fills missing prompt/completion tokens, preserves cache fields from message_start, and estimates unfinished output (`relay/channel/claude/relay-claude.go:153`). |
| Responses | Estimates output only if collected text is nonempty; adds estimated prompt only when output is nonzero (`relay/channel/openai/relay_responses.go:164`). No-data fallback is not currently the same as OpenAI chat. |
| Gemini | Uses valid usage metadata; without metadata estimates only when text/image exists, otherwise returns empty usage (`relay/channel/gemini/relay-gemini.go:178`). |

Important interactions:

- Most gate errors currently return `nil, streamErr` before adapter fallback. A cancelled loser needs to return observed/estimated usage alongside its non-success status. Do not let an error erase already captured usage.
- The existing precommit buffer delays neutral frame processing. Claude message_start may already contain input/cache usage that has not reached the adapter parser before loss. Collect billing evidence from received frames independently of winner output (without replaying loser frames to the client). This is a billing-observation hook at an existing boundary, not a change to winner semantics.
- `service.ResponseText2Usage` returns prompt estimate plus tokenized observed text and marks local counting (`usage_helpr.go:22`). It does not estimate hidden reasoning or time-based output. Preserve known cache splits and canonical `BillingUsage` rather than overwriting them with a fresh plain Usage.
- **Do not pass nil to mean zero:** `calculateTextQuotaSummary` turns nil usage into estimated prompt tokens (`text_quota.go:286`). Explicit empty `&dto.Usage{}` means zero observed tokens and no tool surcharge means quota zero (`:415`).
- For explicit estimates, pass a nonnil normalized usage with estimated provenance. `PostTextConsumeQuota` evaluates expression billing only when original usage is nonnil (`text_quota.go:449`). A nil fallback risks applying the wrong pricing path. `effectiveBillingUsage` prefers the nested canonical billing payload, so updating only top-level numbers can fail to change the actual charge (`billing_usage.go:20`).
- Existing source provenance is mostly admin-only (`billing_usage.go:57`); user-paid estimated loser charges need a public-safe estimate marker.
- Do not treat the reservation as known consumption, charge max output tokens for an interrupted request, or infer output tokens from elapsed seconds. Tiered evaluation has an existing error fallback to the attempt's reservation (`tiered_settle.go:213`); retain and clearly distinguish that existing billing-error fallback from ordinary unknown-usage policy.

Recommended minimal policy for planning, pending user's explicit no-usage preference: always charge received authoritative usage/tool counts for any dispatched attempt; otherwise use a documented local estimate of prompt plus observed response content when the request is considered accepted/billable, marked estimated. Before dispatch / definite local rejection has zero upstream charge. A request sent but cancelled before any response has uncertain acceptance and unknown hidden cost: whether to charge a prompt estimate is a product rule, not a fact inferable from networking. Parent should resolve this narrow rule; do not quietly keep the operator-paid assumption from the previous design. Even prompt estimation cannot promise exact reimbursement of unreported reasoning.

For fixed-price / per-call models, the existing calculator charges a full per-call price when billable usage exists (`text_quota.go:405`, `:415`). A prompt estimate for a loser therefore may produce a full second call fee. This follows the configured pricing contract but must be disclosed in the final billing policy. For tiered expressions, evaluate each attempt against its independent route snapshot and actual/estimated usage.

### Additive consume logs and user-facing interpretation

SQL log `request_id` has a normal index, not uniqueness (`model/log.go:78`); multiple consume rows for one original request already fit the schema. `RecordConsumeLog` reads shared correlation ID from Gin context and inserts another row (`:498`, `:535`). Per-attempt upstream request IDs should remain isolated. User/admin request-ID filters already retrieve all related rows (`:635`, `:730`).

Minimal metadata in each charged or zero-charge dispatched-attempt row:

- Stable attempt ID / ordinal and shared root request ID.
- Outcome `winner`, `hedge_cancelled`, actual upstream failure, or client cancellation; whether that attempt delivered output.
- Usage basis `upstream`, `estimated`, or `unknown`; cancellation/elapsed timing relevant to interpreting the charge.
- Own model, route group, channel cost ratio snapshot, actual quota, upstream request ID, normal token/cache/tool billing breakdown.

Expose only billing-relevant attempt role/ordinal/estimated status to the user in `other` or a dedicated safe metadata object. Keep channel identity, routing priority, raw upstream diagnostics and full route history under `admin_info`. `formatUserLogs` strips admin_info (`model/log.go:158`), so putting all hedge markers there would leave users seeing unexplained duplicate charges. Do not append an automatic succeeded route history entry for a charged loser; `GenerateTextOtherInfo` currently synthesizes success (`service/log_info_generate.go:135`).

Frontend already displays multiple data rows without keying by request ID (`usage-logs-table.tsx:159`) and shows request/upstream IDs (`details-dialog.tsx:649`). Extend `web/src/features/usage-logs/types.ts`, details dialog, desktop/mobile row summaries, and locales to show attempt outcome and estimate basis. Optional grouping or same-request navigation improves auditability; it does not require a new accounting database table. A success icon must not imply that a cancelled but charged attempt delivered an answer.

**Statistics:** `PostTextConsumeQuota` currently increments request_count per billable finalization (`text_quota.go:484`, `model/user.go:1376`) and records success performance samples (`text_quota.go:585`). Charging losers must not mark them successful replies. Existing `UpdateUserUsedQuota` (`model/user.go:1385`) can add their costs without increasing logical user request_count; designate the logical request's count once through coordinator finalization. Log RPM is currently `count(*)` of consume rows (`model/log.go:773`), so additive rows will change RPM to billed-attempt rate unless queries/labels deliberately distinguish distinct request count. Token/quota sums should include all attempts.

**ClickHouse caution:** logs use ordinary MergeTree, not ReplacingMergeTree (`model/main.go:510`), so same request ID rows are preserved. But current pagination order is `(created_at DESC, request_id DESC)` (`model/log.go:148`), now tying more frequently for multiple attempts in the same second. A deterministic tie-breaker is needed if stable pagination across these rows is an acceptance criterion; do not claim existing IDs are globally unique there (`model/main.go:489` has `id DEFAULT 0`). This does not require broad accounting changes, but it is a concrete log-view limitation.

### Crash / idempotency limits

- Wallet funding/token reserve and rollback are separate operations. Conditional reserves prevent concurrent over-reservation but do not make the whole multi-step lifecycle transactional.
- Wallet refunds use additive non-idempotent increments; source explicitly says not to retry them (`funding_source.go:80`). BillingSession's mutex and `refunded/settled` flags protect one live object, not a process restart.
- Refund is asynchronous (`billing_session.go:83`); process death can lose a pending refund. Quota batching and Redis persistence also retain their existing crash windows (`quota_reserve.go:191`, `model/token.go:419`).
- User/channel used counters, funding settlement, token settlement, consume log insertion, and metrics are not one transaction; logs may use a different database. `PostTextConsumeQuota` logs settlement errors but still proceeds to log insertion (`text_quota.go:488`). Do not blindly retry whole finalization after an ambiguous error.
- Subscription preconsume/refund identity is durable only for that record's retained lifetime (cleanup defaults seven days at `subscription.go:1471`); postconsume deltas have no durable operation key. Current refund implementation also invokes the delta helper through its own DB transaction (`subscription.go:1419`, `:1517`); avoid treating the comment "idempotent" as a full crash-recovery proof.
- Multiple log rows with shared request ID are additive evidence, not a deduplication guard. Per-attempt identity belongs in telemetry and potential future reconciliation, but adding a field does not itself guarantee crash-safe exactly-once charging.

Minimal delivery should promise **one finalize per attempt during a live request**, reuse existing persistence behavior, surface settlement/unknown-usage anomalies, and leave durable ledger/reconciliation redesign as a separately approved effort. This is sufficient to stop deliberately making losers free; it cannot guarantee every cent of unknown upstream consumption is recovered.

### Focused acceptance tests to add during implementation

1. A + B reserve independently; 100 + 150 are both deducted; actual 40 + 110 settle to a 150 total charge and two correlated consume rows.
2. Wallet/token cannot fund B: no B dispatch, no B net debit, A remains reserved and can complete.
3. Trust threshold, finite/unlimited token, unlimited user, free A/paid B preserve their correct semantics.
4. Both return usage then one loses: each charged exactly once; only winner output and logical success metrics; no auto-ban effect from loser cancellation.
5. Loser cache-only Claude message_start, truncated Responses usage, and observed text estimate preserve canonical source, cache/tier pricing and estimated marker.
6. Explicit zero usage produces zero charge; nil is never accidentally used as zero; unknown-before-response policy follows reviewed rule.
7. Client cancellation or both fail still settles billable attempts and returns only unconsumed reservations; parent refund never unwinds settled attempts.
8. Duplicate result/finalize notification changes neither balance nor counters/log rows a second time.
9. Logs grouped by root ID expose all charges to user without channel/routing leaks; cancelled charges do not synthesize succeeded history.
10. If subscription funding is active when implementation starts: A/B distinct billing IDs independently preconsume/refund; own subscription metadata; no shared request-key collision.

Use current `model/quota_reserve_test.go`, `service/route_billing_test.go`, `service/text_quota_test.go`, `model/log_route_history_test.go`, and billing usage tests as fixtures. Run focused package/race tests during implementation, not this research turn. Read `pkg/billingexpr/expr.md` before pricing changes; reuse centralized safe quota conversions and saturation logs.

### Related specs / external references

- `AGENTS.md`: billing safety, separate main/log DB compatibility, meaningful tests, no guessed runtime state.
- `.trellis/spec/backend/billing-ratio-routing.md`: applies independently to each charged attempt; original winner-only request assumption needs a hedge-specific extension.
- `pkg/billingexpr/expr.md`: existing expression contract and normalization; no new expression semantics proposed.
- `research/flowapi-backend.md`: state isolation and prior snapshot; winner-only accounting portions are superseded by this document.
- CCH source matching remains the parent session's research responsibility. This document does not claim CCH estimates or recovers usage it never receives.

## Caveats / Not Found

- Live `NewBillingSession` currently has no subscription construction, despite retained comments/types. Do not undo concurrent edits or restore subscriptions as an incidental hedge change.
- No durable per-attempt wallet ledger, crash-recovery reconciliation, or exactly-once consume-log uniqueness was found.
- No existing uniform cross-protocol policy for cancelled-before-first-content usage was found. Define no-response acceptance/estimation policy explicitly; hidden upstream work is not measurable from local elapsed time.
- No tests run and no product/task-plan files edited. Only this research file was added.
