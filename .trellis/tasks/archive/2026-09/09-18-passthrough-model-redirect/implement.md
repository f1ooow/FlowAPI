# Implementation Plan

1. Add a shared JSON passthrough model rewrite helper under `relay/helper`, using `common.Unmarshal` and `common.Marshal`.
2. Apply it to OpenAI-compatible, Responses, Claude, image, and rerank JSON passthrough branches while preserving replayability and existing error classification.
3. Keep Gemini URL model handling and non-JSON/multipart paths unchanged.
4. Add deterministic regression tests for the helper and affected body behavior.
5. Run `gofmt`, focused relay/helper tests, affected relay tests, and inspect the final diff for unrelated worktree changes.

Risk points: body replay storage ownership, preserving unknown JSON fields, and ensuring no-op mappings avoid unnecessary serialization.
