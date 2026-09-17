# Streaming Provider Hedge Racing

## 1. Scope / Trigger

Apply when changing text relay racing, channel timeouts, per-attempt billing,
stream gates, or usage-log aggregation. Serial retry pricing remains governed
by [billing-ratio-routing.md](./billing-ratio-routing.md).

## 2. Signatures

Channel `settings.reliability` stores optional integer seconds:

| JSON field | Accepted values |
| --- | --- |
| `first_content_timeout_seconds` | 0 or 1–180 |
| `streaming_idle_timeout_seconds` | 0 or 60–600 |
| `non_streaming_timeout_seconds` | 0 or 60–1800 |

The Go DTO uses `*int` with `omitempty`. Missing and explicit zero disable
these timers; the UI displays and saves zero. Existing transport safety limits
remain independent.

Global option `general_setting.bill_hedge_losers` is boolean and defaults to
`true`. Preserve explicit `false` when loading and saving settings. Capture
the policy for each request.

Consume rows share the original `request_id`. Public `other.hedge` identifies
`attempt_id`, `attempt`, `role`, `usage_source`, `metering_status`, and
`settlement_status`. Provider-private details remain under `admin_info`.
`settlement_status = not_billed` means policy excluded the loser and its
reservation was released; it does not erase known `upstream` metering evidence.
Only `settled` rows display a billed-cost breakdown.

## 3. Contracts

- A first-content threshold launches an eligible backup without canceling the
  original. At most two attempts run concurrently. Route eligibility, distinct
  channel caps, retry restrictions, and replay-safety checks still apply.
- Only one attempt commits downstream output. Semantic content or legitimate
  empty completion can win; headers, heartbeats, and initialization events
  cannot. A committed answer never switches providers.
- A protocol-valid response prefix is distinct from winning content. With
  loser billing enabled, only already-prefixed losers can continue bounded
  background reading after winner selection. Other losers are canceled and
  refunded. Disabled loser billing cancels and refunds every loser.
- Recognize startup frames by supported protocol structure, not arbitrary
  `response.*` or `content_block_*` names. Gemini's `response` wrapper must be
  normalized consistently for both prefix admission and usage observation.
- Each attempt owns its request DTO, body reader, context, writer, cancellation,
  pricing snapshot, and billing session. Never share a seek cursor or retain
  pooled Gin state in detached work.
- Paid attempts reserve independently before dispatch, including trusted
  wallet accounts. Insufficient optional-backup quota leaves the funded
  original running. Unlimited accounts and zero-priced routes retain their
  established semantics.
- Resolve each attempt's group pricing through `HandleGroupRatio`. Apply its
  channel ratio only when its resolved group enables it; delayed settlement
  uses the captured pricing, not current settings.
- Finalization guards settlement, refunds, counters, and log insertion together
  exactly once for a live attempt. This is not a durable crash-recovery ledger.
- Interrupted losers without trustworthy usage are unmetered; never invent a
  prompt estimate or constant-expression charge. A zero charge does not prove
  the external provider incurred no cost.
- An empty, null, or malformed usage object is not reported zero. Require
  numeric metering evidence and preserve explicitly reported zero counts.
  Detached Responses function-call events still contribute configured tool
  surcharges exactly once, without retaining their arguments or emitting output.
- Both charged attempts contribute fees/tokens. A loser is not a delivered
  response, success-latency sample, or additional logical request/RPM count.
  Race loss and client cancellation are not provider auto-ban evidence.
- Before commitment, client cancellation cancels all attempts. Afterward,
  winner cancellation remains client-bound; admitted loser collectors have
  finite time, byte, and concurrency limits and never write to the client.
- Nonstream total timeout covers response-body reads as well as headers.
  Streaming idle timeout ends a committed stream without replaying it.

## 4. Validation & Error Matrix

| Condition | Behavior |
| --- | --- |
| Missing or zero channel timeout | Disabled; no inherited UI value |
| Negative, fractional, or out-of-bounds timeout | Reject channel update |
| No eligible backup / backup reserve fails | Keep funded original running |
| Unsupported transport, upstream state or side-effect tool | Keep serial path |
| Gemini cached content or image/audio output, including accepted DTO aliases | Keep serial path |
| Loser billing false | Cancel losers; refund their reservations |
| Loser billing true, no valid prefix at winner selection | Cancel/refund loser |
| Admitted loser reaches collection limit | Cancel; finalize reliable evidence only |
| Unknown loser usage | Unmetered audit, no fabricated charge |
| Accounting write returns an uncertain error | Expose failure; do not blindly retry |

## 5. Good / Base / Bad Cases

- Base: all three timeouts are zero; no speculative backup launches.
- Good: A exceeds 60 seconds; B starts while A remains eligible to win.
- Good: group ratio 2, channel ratios 0.5/1.5, channel inclusion disabled:
  both attempts use effective ratio 2. Enabled: they use 1 and 3 respectively.
- Bad: multiply combined A/B tokens by the winner's price, or use one shared
  reservation equal to the more expensive attempt.
- Bad: treat a charged loser as another successful answer, or put all attempt
  billing labels under admin-only metadata.

## 6. Tests Required

Use controlled upstream signals for A/B winner arbitration, neutral prefixes,
additive reservations, backup admission failure, cancellation, and no mixed
output. Assert actual wallet/token/log outcomes, not just helper calls. Cover
both switch values and prefix eligibility, conditional group/channel ratios,
unknown usage, bounded drain, and finalization idempotence. Verify logical
statistics separately from additive fees/tokens across supported log dialects.
Run affected lifecycle tests with the race detector. Any relaykit change also
requires `cd relaykit && GOWORK=off go build ./...`.

## 7. Wrong vs Correct

Wrong: copy the original `RelayInfo` and its billing session for B, cancel A
when its soft threshold fires, or bill every dispatched loser unconditionally.

Correct: construct attempt-owned state; independently reserve B before
dispatch; arbitrate one client writer; apply the captured global policy to
losers; finalize each admitted billable attempt using its own usage/pricing.
