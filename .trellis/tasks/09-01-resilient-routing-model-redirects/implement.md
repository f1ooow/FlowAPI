# Implementation Plan

## 1. Routing contracts and selection

- [x] Add request route-state and route-attempt types with deterministic tests.
- [x] Change cached and database channel selection to accept exclusions and choose highest remaining priority.
- [x] Make model redirect rules participate in channel eligibility.
- [x] Integrate affinity and Auto-group selection with exclusions.
- [x] Replace the synchronous relay retry-index loop with outer channel traversal and inner per-channel attempts.
- [x] Preserve per-route billing preparation, body restoration, multi-key rotation, and original-model restoration on every attempt.
- [x] Add structured retry decisions and route-chain logging.

Validation: `go test ./model ./service ./controller`

## 2. Replay-safe endpoint coverage and stream gate

- [x] Verify general relay covers synchronous image generation and image editing and add body-replay regressions for JSON and multipart bodies.
- [x] Add shared pre-commit stream state and return retryable errors for provider failure/empty stream before valid output.
- [x] Verify Claude, OpenAI, Gemini, image generation, and image editing routing behavior.
- [x] Keep WebSocket, task/video async submission, polling, and post-commit stream failure outside transparent replay.

Validation: `go test ./controller ./relay/... ./service`

## 3. Channel automatic ban

- [x] Define and validate channel reliability configuration in existing settings JSON.
- [x] Count one failure only after a channel exhausts all attempts; reset the consecutive count on a completed success.
- [x] Persist threshold disable and expiry atomically using the existing automatic-disabled status.
- [x] Restore only expired automatic bans; never restore manual disables.
- [x] Keep channel automatic ban independent from the legacy global automatic-disable rules.
- [x] Add count, reset, threshold, expiry, manual-disable, and selection tests.

Validation: `go test ./service ./model ./controller`

## 4. Ordered model redirects

- [x] Add backend rule types, legacy normalization, validation, and first-match matcher.
- [x] Replace exact-map rewriting with one channel-specific ordered-rule evaluation from the original model.
- [x] Add exact/prefix/suffix/contains/regex, legacy migration, eligibility, and channel-switch tests.
- [x] Update frontend schema and mapping utilities to the ordered rule format.
- [x] Rebuild the editor with match type, ordered rows, reorder/delete controls, JSON view, and rule testing.
- [x] Update missing/exposed model helpers and channel create/update payload normalization.
- [x] Add English base and all locale keys through the repository i18n workflow.

Validation: `go test ./relay/helper ./model ./service`, frontend model-mapping tests, i18n sync, and typecheck.

## 5. Reliability UI and system setting cleanup

- [x] Add per-channel max attempts and automatic-ban controls to channel advanced settings.
- [x] Update global routing reliability UI so it no longer describes a request-wide retry count as the failover mechanism.
- [x] Add validation and user-visible behavior tests for defaults, limits, and save/load normalization.
- [x] Confirm the controls remain usable in create, edit, and tag batch-edit flows where applicable.

Validation: affected channel tests, frontend typecheck, and production build.

## 6. Integration and finish

- [x] Run focused backend tests, then root `go test ./...` once.
- [x] Run frontend affected tests and production build once.
- [x] Review only the new diff for billing, retry safety, response commitment, cache state, i18n, and dirty-worktree overlap.
- [x] Update the routing/billing spec with stable contracts discovered during implementation.
- [x] Record verified scope and explicit exclusions in release notes.

Rollback points: routing state before automatic-ban integration; automatic-ban integration before UI exposure; backend model compatibility before frontend editor migration.
