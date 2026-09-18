# Global Passthrough

## Scope / Trigger

Read when modifying global passthrough settings, channel header overrides, relay transports, or channel inheritance UI. Request body and request headers are independent parts of HTTP; one switch must not silently control both.

## Signatures

- `setting/model_setting.GlobalSettings.PassThroughRequestEnabled` / `PassThroughHeadersEnabled` register as `global.pass_through_request_enabled` / `global.pass_through_headers_enabled`.
- `PUT /api/option/`: existing privileged option API, `{ "key": "global.pass_through_headers_enabled", "value": true }`.
- `GET /api/channel/passthrough`: `AdminAuth` plus `authz.ChannelRead`, returning `{ "success": true, "data": { "pass_through_request_enabled": true, "pass_through_headers_enabled": false } }`.
- `relay/common.GetEffectiveHeaderOverride(*RelayInfo) map[string]interface{}` merges the effective source before `relay/channel.ResolveHeaderOverride` resolves it.
- `relay/helper.NewPassthroughBody(common.BodyStorage, *RelayInfo, string) (common.ReplayableBody, io.Closer, error)` keeps no-op bodies on original storage and returns caller-owned storage only when rewriting JSON.

## Contracts

- Unset global header option is false. Global true injects `*` into a sanitized copy, never mutating stored channel maps. No channel backfill or database migration.
- Runtime overrides are final: do not reintroduce wildcard defaults after runtime operations. Channel tests must not automatically forward headers from administrator HTTP requests.
- Automatic wildcard/regex headers precede explicit administrator overrides; explicit values win. Automatic forwarding excludes credentials, cookies, transport metadata, `Content-Type`, `chatgpt-account-id`, and everything listed under Limits. `Content-Type` and `chatgpt-account-id` preserve adapter body encoding and account selection. Explicit administrator entries are still permitted.
- Common HTTP/form/WebSocket paths use this machinery. `DoTaskApiRequest` and bespoke transports are not implicitly covered. Audit the actual dispatch path before claiming support.
- Body passthrough in supported handlers is global OR local. It bypasses adapter conversion, parameter overrides and system prompt injection. For OpenAI-compatible text, Responses (including compact), Claude, image JSON, and rerank requests, an actual channel model redirect must still rewrite the top-level `model` to the configured target. Do not describe passthrough as compatible with arbitrary upstream protocols.
- Model rewriting preserves unknown fields and numeric precision through raw JSON values; never round-trip arbitrary payloads through `map[string]any`. Without a matched redirect, retain original bytes and replayable storage. Nested `model` fields are not routing identities and must remain unchanged.
- Rewritten bodies belong to the outbound attempt. Never mutate or replace inbound body storage: retries and hedged attempts must each start from the original client payload. Adapter-only suffix normalization must not change a passthrough model or reinterpret a configured redirect target.
- Multipart payloads, URL-based Gemini model handling, WebSockets and asynchronous tasks keep their existing behavior; this JSON rewrite does not extend their support.
- Channel display reads only the two flags, not privileged system options. A globally enabled body switch appears on and disabled without changing the saved local preference; successful option mutation invalidates `['channel-passthrough']`.
- Existing `delete_header` operations remove map entries, not individual headers later expanded from `*`. Do not claim they are wildcard exclusion filters.

## Limits

- Automatic forwarding never includes client IP and proxy-chain headers (`X-Forwarded-For`, `X-Real-IP`, `X-Forwarded-Host/Proto/Port`, `CF-Connecting-IP`, `True-Client-IP`, `Forwarded`, `Via`), caller-site disclosure (`Referer`, `Origin`), or upstream account scope (`OpenAI-Organization`, `OpenAI-Project`, `x-goog-user-project`, `mj-api-secret`). Account scope is not merely a leak: `processHeaderOverride` runs after `SetupRequestHeader`, so a forwarded client value would replace the adapter-selected organization or project and can redirect a request to another account, producing 403s that trip channel auto-disable or misbilled usage.
- The switch is global, all-or-nothing. There is no per-channel opt-out; a channel cannot exclude itself from forwarding once the option is on, and `delete_header` operations remove map entries rather than filtering names expanded from `*`.
- The skip list only covers well-known headers. Custom or vendor-specific secrets carried in arbitrary header names cannot be recognized and will be forwarded. Do not enable global forwarding for a fleet whose clients carry bespoke credentials in headers.
- Explicit administrator overrides bypass the skip list by design; blocking a header there would remove the only supported way to configure these values per channel.

## Validation & Error Matrix

| Input / condition | Required behavior |
| --- | --- |
| Global header false, no local rules | No automatic forwarding |
| Global header true, no local rules | Forward eligible client headers |
| Client credentials, account ID, content type | Keep adapter-selected values for automatic rules |
| Explicit administrator header value | Override automatic/default value |
| Invalid regex | Existing channel-header-override error |
| Non-string explicit value | Existing sanitization converts it to a trimmed string before resolution |
| Missing request context with active passthrough | Existing missing-context error |
| Unauthorized channel settings read | Existing admin/ChannelRead denial |
| JSON body passthrough with matched redirect | Send configured target in top-level `model`; retain unknown and nested values |
| No matched redirect or identity mapping | Keep original body byte-for-byte |
| Rewrite required but body is invalid or non-object JSON | Fail before contacting upstream; use existing body/conversion error classification |
| Retry or hedge chooses another channel | Rewrite independently from original model and original body |

## Good / Base / Bad Cases

- Good: global header true forwards `session_id` to an ordinary HTTP upstream while channel Authorization remains intact.
- Base: global false preserves channels that already have `{"*": true}`.
- Bad: copying the incoming header map after adapter setup overwrites OAuth account identity or the adapter's required JSON media type.
- Good: redirect `client-alias` to `provider-model` while preserving an unknown integer `9007199254740993` exactly and retaining a nested `model` value.
- Bad: mutate shared inbound storage on the first attempt, causing the next channel to receive that attempt's target.

## Tests Required

- Global/local inheritance, toggling off, explicit precedence, runtime final-map behavior, and channel-test isolation: `relay/channel/global_header_passthrough_test.go`.
- Real adapter setup plus actual captured upstream HTTP headers: `relay/channel/codex/header_passthrough_test.go`.
- Independently updated registered boolean combinations and response shape: `controller/channel_passthrough_test.go`.
- Settings save/invalidation and inherited channel preference restoration: frontend passthrough tests under feature `__tests__/` directories.
- Body redirect regressions: `relay/helper/passthrough_body_test.go` and `relay/passthrough_model_test.go`. Assert captured outbound JSON across the supported handlers, global/local switch inheritance, byte-identical no-op bodies, unknown field precision, nested model preservation, invalid object rejection, replayability and cross-channel isolation.

## Wrong vs Correct

Wrong: persist `*` onto every channel when a global option is toggled, or set a channel form field true merely because its effective value is inherited.

Correct: derive effective state at request/render time, preserve local preferences, and invalidate the channel inheritance query after successful global saves.
