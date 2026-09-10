# Global request body and header passthrough

## Goal

Enable request body and header passthrough globally for existing and new channels, removing repetitive per-channel configuration.

## Background

- User authorized implementation and task creation on 2026-09-10. Preserve the unfinished implementation and unrelated dirty worktree changes.
- Body passthrough already uses global OR local flags in supported handlers (`relay/compatible_handler.go:97`, `relay/responses_handler.go:79`). It skips conversion and body parameter overrides.
- Header wildcard processing runs after adapter setup (`relay/channel/api_request.go:334`). Client Content-Type and chatgpt-account-id can undo protocol normalization and channel identity (`relay/channel/codex/adaptor.go:179`).

## Requirements

- R1: Separate global body and header switches, with headers disabled when unset.
- R2: Existing and new channels inherit headers without record rewrites; explicit channel header values win. Turning off global restores local behavior.
- R3: Wildcard/regex rules protect credentials, cookies, transport headers, adapter content type and account identity. Explicit administrator overrides remain supported.
- R4: Channel UI shows inheritance. Inherited body switches cannot misleadingly toggle or change stored local preference.
- R5: Saving options refreshes inheritance. Relevant text is translated in all active locales.
- R6: Explain actual risks and transport coverage. Verify locally; do not deploy or change production configuration.

## Acceptance Criteria

- [x] AC1 (R1/R2): Flags save independently, channels need no template while headers are globally enabled, disabling preserves local templates.
- [x] AC2 (R2/R3): Final upstream requests preserve channel authentication and required adapter headers, forward ordinary metadata, and honor explicit administrator overrides.
- [x] AC3 (R4/R5): New and existing forms reflect inheritance without mutating local preferences; saving settings refreshes them.
- [x] AC4 (R5): Relevant translations, frontend tests, typecheck and changed-file lint pass.
- [x] AC5 (R6): Backend regressions pass; final report states limits accurately.

## Out Of Scope

- Hybrid body rewriting, new protocols, guaranteed cache hits, and per-channel opt-out semantics.
- Bulk database updates, deployment, production traffic tests, and reworking delete_header operation semantics.
