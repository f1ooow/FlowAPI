# Design: CCH-style resilient routing and model redirects

## Existing constraints and reusable foundations

- `controller/relay.go` currently loops over a global retry index. The index is passed into selection and incorrectly doubles as a priority-tier index.
- `model/channel_cache.go` already holds eligible channel IDs sorted by priority and can be changed to select from an exclusion-aware candidate set.
- `RelayInfo.InitChannelMeta` restores `OriginModelName` before applying the selected channel, which is the correct boundary for channel-specific redirects.
- `BillingSession` and `PrepareBillingForSelectedRoute` already support reserving a more expensive retry route and settling the successful route.
- `common.GetBodyStorage` provides replayable request data, including multipart image bodies, as long as every attempt restores it before relay conversion.

## Routing state and selection contract

Introduce a request-scoped route state passed through existing channel selection:

```text
RouteState
  excludedChannelIDs: set<int>
  attemptsByChannel: map<int,int>
  distinctChannelsAttempted: int
  history: []RouteAttempt
```

Selection no longer accepts a retry index as a priority selector. It accepts the immutable original model, request path, and exclusions. Candidate evaluation is:

1. enabled channel and group/Auto-group eligibility;
2. exact advertised model, normalized model, or matching redirect rule;
3. request-path compatibility;
4. not excluded by request state;
5. circuit permits a request;
6. maximum priority among remaining candidates;
7. existing weighted random selection within that priority.

The selected channel remains current for its inner attempt loop. When its per-channel maximum is exhausted, it is excluded and the selector runs again. A local/non-retryable error stops without consuming other channels.

Affinity may choose the first channel, but failure cannot keep returning the failed affinity channel. Existing `skip_retry_on_failure` remains an explicit policy that stops traversal. Auto-group selection continues to resolve the group before candidates are ranked, while request exclusions apply across subsequent selections.

## Retry classification and replay boundary

Replace the status-range-only decision with a structured decision containing:

```text
retryCurrentChannel | switchChannel | stop
breakerEligible: bool
reason: stable code
```

- Network connection failures, retryable upstream status codes, and upstream malformed/empty pre-commit responses can retry or switch.
- Authentication/rate-limit/provider capacity errors may switch but are classified separately for channel/key handling.
- Client validation, local quota/billing, body parsing, request cancellation, and explicitly skipped errors stop.
- Retry decisions must consult whether the response has committed client-visible content.

The relay controller restores request body and original model for every raw attempt. Synchronous image generation and image editing use the same loop. Task submission, WebSocket, video, and post-commit streaming are outside transparent replay.

## Stream pre-commit gate

Provider stream readers expose whether valid model content was observed before writing final client-visible output. Headers alone do not count as committed content. Provider errors or an empty stream before the gate opens return a retryable relay error. Once content is written, the attempt owns the response and later failure is returned/logged without switching.

This is implemented at the shared stream response boundary where possible, with protocol-specific validation only where stream formats differ. It must not buffer an entire normal response; the gate retains only the minimal prefix needed to validate the stream.

## Channel automatic ban

Reliability configuration is persisted in the existing channel settings JSON to avoid a cross-database schema migration:

```json
{
  "reliability": {
    "max_attempts": 2,
    "auto_ban_threshold": 5,
    "auto_ban_duration_seconds": 1800
  }
}
```

The consecutive count and automatic-ban expiry are stored in channel status metadata:

```text
auto_ban_failures
auto_ban_until
```

One channel contributes at most one failure for one request: only retryable upstream failures that exhaust all configured attempts increment the count. A completed success clears that channel's count. Client errors, local errors, cancellations/499, and probes do not alter it.

Failure increments, threshold transition, expiry, and status metadata are persisted atomically under a row lock. At threshold the existing automatic-disabled status is used. Expired automatic bans restore to enabled and clear the count; manually disabled channels have no automatic-ban expiry and are never restored automatically.

This state machine is independent of the legacy global automatic-disable switch and its status-code, keyword, and response-time rules. The channel's own automatic-ban switch remains authoritative.

## Model rules

Persist `model_mapping` as an ordered JSON array:

```json
[
  { "match_type": "contains", "source": "opus", "target": "claude-opus-4-6" }
]
```

Backend parsing accepts the legacy object form and converts entries to ordered `exact` rules. Rule matching is case-sensitive to preserve model identifier semantics. Regex uses Go's RE2 engine, inherently avoiding catastrophic backtracking; length and count limits still apply.

The model matcher is shared by channel eligibility and relay rewriting so they cannot disagree. The eligibility check asks whether any redirect rule matches the immutable original model. After selection, the first matching rule determines the upstream model. Exact redirect chaining is removed: ordered first-match semantics produce one target, matching CCH and avoiding channel-specific loops.

## API and UI

- Extend channel DTO validation for reliability settings and rule-list model mapping.
- Replace the mapping editor's map rows with ordered rule rows: match type, user-requested pattern, upstream model, reorder, delete.
- Keep visual and JSON tabs. JSON uses the rule array and legacy data is normalized on load.
- Add an inline rule test action using the same frontend matcher semantics; backend remains authoritative.
- Place reliability controls in the channel advanced settings because the values are channel-specific. Replace the global retry presentation with a traversal safety setting or explanatory status, not a misleading per-request retry count.

## Compatibility and migration

- No database column migration is required; settings and mapping remain text JSON.
- Legacy object mappings are read-compatible and normalized when edited/saved. Runtime supports them during rollout.
- Existing global `RetryTimes` is no longer the primary route budget for supported synchronous relays. A hard distinct-channel cap of 20 prevents loops.
- Unsupported task/WebSocket paths retain their current behavior until a separate idempotency design exists.

## Observability

Route history is logged once at completion and includes channel/attempt/outcome. Circuit transitions log stable structured fields. Final errors retain the last upstream cause and indicate how many distinct channels were attempted without exposing credentials.

## Risks and mitigations

- Duplicate upstream work: only replay pre-commit, replay-safe synchronous paths; exclude accepted async work.
- Billing mismatch: prepare billing after every selection and before sending; settle successful route only.
- Affinity loops: exclusions override affinity after failure unless explicit no-retry policy stops.
- Regex abuse: RE2 plus rule count/text limits and compile validation.
- Concurrent failure/success completion: serialize metadata transitions with the channel row lock and the existing channel status lock.
- Dirty worktree conflicts: edits stay within routing/channel configuration files and preserve unrelated image-playground and ratio-sync work.

## Rollback shape

The routing loop can be reverted independently while legacy model mappings remain readable. Automatic-ban metadata uses the existing `other_info` JSON and requires no destructive database migration.
