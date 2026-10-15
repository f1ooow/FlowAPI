# Route history timing

## Scope / Trigger

Applies to serial relay attempt timing and admin usage-log rendering. Timing is observability only; do not change retry decisions, billing ordering, or stream success semantics to collect it.

## Signatures

`RouteState.StartAttemptTiming(channelID, attempt int, requestStart, startedAt time.Time)` starts the actual helper invocation. `FinishAttemptTiming(channelID, attempt int, endedAt time.Time) *RouteAttemptTiming` freezes a matching attempt once and returns an independent snapshot.

## Contracts

`other.admin_info.route_history[].timing` is optional. When present, `started_at_ms` and `ended_at_ms` are Unix milliseconds; `start_offset_ms`, `end_offset_ms`, and `duration_ms` are monotonic elapsed milliseconds. Zero is valid. Missing historical data remains absent; no database migration or inferred backfill.

Consume-log generation may precede the relay helper return. The successful log freezes the current attempt's result time; the controller reuses it. This denotes server-side result recording, not client delivery or first byte. Failed helpers freeze before error processing. Exhaustion reuses the last actual attempt's timing.

## Validation & Error Matrix

- Missing start or mismatched attempt identity: no snapshot; never borrow another attempt's timing.
- Repeated finish: same frozen values, independently copied.
- New attempt: reset current timing, preserve historical snapshots.
- Missing or invalid frontend timing: show missing-time copy, not zero or a guessed timestamp.
- Non-admin logs: strip the entire `admin_info`, including timing.

## Good / Base / Bad Cases

Good: channel A fails at +12.30 seconds, channel B records success at +35.80 seconds; each has its own duration.
Base: a direct successful attempt appears in both admin entrypoints; historical records show missing timing.
Bad: treating HTTP 200 as proof a stream ended normally, deriving duration from adjacent nodes, or exposing timing outside `admin_info`.

## Tests Required

Deterministic state tests cover zero, reset, identity, frozen copies, and early success. Controller tests cover retry, failover, terminal failure, and synchronous image consume logs. Model tests assert non-admin redaction. Frontend tests cover both entrypoints, keyboard-accessible timestamp tooltips, missing/invalid data, and zero values.

## Wrong vs Correct

Wrong: capture success only after the helper returns; the consume log may already be written without timing.

Correct: freeze in the successful consume-log snapshot path and reuse the frozen value when the controller records the same attempt.
