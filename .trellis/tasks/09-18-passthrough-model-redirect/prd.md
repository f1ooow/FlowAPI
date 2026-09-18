# Preserve model redirects with global request body passthrough

## Goal

When global request-body passthrough is enabled, a channel model redirect must still reach the configured upstream model without losing unknown client fields or changing unrelated passthrough behavior.

## Background

`ModelMappedHelper` resolves the channel-specific upstream model before relay handling, but passthrough branches currently send the original request bytes directly. Channel eligibility can therefore use the redirect while the upstream JSON still contains the client model. The existing passthrough contract documents that serialized model remapping may be bypassed; this task repairs that specific gap for safe JSON requests.

## Requirements

- On supported JSON request-body passthrough paths, rewrite only the top-level `model` field when the selected channel has a matching redirect target.
- Preserve all other top-level and nested fields, including fields unknown to FlowAPI.
- Preserve the original body when no redirect is matched or the target equals the requested model.
- Do not rewrite nested `model` fields.
- Use the repository JSON wrapper APIs for parsing and serialization.
- Keep existing passthrough behavior for parameter overrides, system prompts, and adapter conversion.
- Do not broaden this change to multipart bodies, asynchronous task submission, WebSocket flows, or model names carried in request URLs.

## Acceptance Criteria

- [x] A globally passthrough OpenAI-compatible JSON request with a matching redirect sends the target in its top-level `model` field upstream.
- [x] Responses, Claude, image, and rerank JSON passthrough paths use the same behavior where their body has a top-level model field.
- [x] Unknown fields and nested model values survive the rewrite unchanged in meaning.
- [x] Requests without a matching redirect retain the original body and model.
- [x] Invalid or non-object JSON is rejected with an existing request-body/conversion error instead of silently forwarding the wrong model.
- [x] Focused helper/relay tests pass, and unrelated dirty-worktree changes remain untouched.

## Out of Scope

- Changing channel selection, redirect matching rules, billing identity, or route priority.
- Applying parameter overrides or system prompts while passthrough remains enabled.
- Rewriting multipart bodies or URL path model identifiers.

## Open Questions

None.
