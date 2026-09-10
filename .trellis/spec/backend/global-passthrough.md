# Global Passthrough

## Scope / Trigger

Read when modifying global passthrough settings, channel header overrides, relay transports, or channel inheritance UI. Request body and request headers are independent parts of HTTP; one switch must not silently control both.

## Signatures

- `setting/model_setting.GlobalSettings.PassThroughRequestEnabled` / `PassThroughHeadersEnabled` register as `global.pass_through_request_enabled` / `global.pass_through_headers_enabled`.
- `PUT /api/option/`: existing privileged option API, `{ "key": "global.pass_through_headers_enabled", "value": true }`.
- `GET /api/channel/passthrough`: `AdminAuth` plus `authz.ChannelRead`, returning `{ "success": true, "data": { "pass_through_request_enabled": true, "pass_through_headers_enabled": false } }`.
- `relay/common.GetEffectiveHeaderOverride(*RelayInfo) map[string]interface{}` merges the effective source before `relay/channel.ResolveHeaderOverride` resolves it.

## Contracts

- Unset global header option is false. Global true injects `*` into a sanitized copy, never mutating stored channel maps. No channel backfill or database migration.
- Runtime overrides are final: do not reintroduce wildcard defaults after runtime operations. Channel tests must not automatically forward headers from administrator HTTP requests.
- Automatic wildcard/regex headers precede explicit administrator overrides; explicit values win. Automatic forwarding excludes credentials, cookies, transport metadata, `Content-Type`, and `chatgpt-account-id`. The last two preserve adapter body encoding and account selection. Explicit administrator entries are still permitted.
- Common HTTP/form/WebSocket paths use this machinery. `DoTaskApiRequest` and bespoke transports are not implicitly covered. Audit the actual dispatch path before claiming support.
- Body passthrough in supported handlers is global OR local. Original body bytes may bypass adapter conversion, model remapping in serialized JSON, parameter overrides and system prompt injection. Do not describe it as compatible with arbitrary upstream protocols.
- Channel display reads only the two flags, not privileged system options. A globally enabled body switch appears on and disabled without changing the saved local preference; successful option mutation invalidates `['channel-passthrough']`.
- Existing `delete_header` operations remove map entries, not individual headers later expanded from `*`. Do not claim they are wildcard exclusion filters.

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

## Good / Base / Bad Cases

- Good: global header true forwards `session_id` to an ordinary HTTP upstream while channel Authorization remains intact.
- Base: global false preserves channels that already have `{"*": true}`.
- Bad: copying the incoming header map after adapter setup overwrites OAuth account identity or the adapter's required JSON media type.

## Tests Required

- Global/local inheritance, toggling off, explicit precedence, runtime final-map behavior, and channel-test isolation: `relay/channel/global_header_passthrough_test.go`.
- Real adapter setup plus actual captured upstream HTTP headers: `relay/channel/codex/header_passthrough_test.go`.
- Independently updated registered boolean combinations and response shape: `controller/channel_passthrough_test.go`.
- Settings save/invalidation and inherited channel preference restoration: frontend passthrough tests under feature `__tests__/` directories.

## Wrong vs Correct

Wrong: persist `*` onto every channel when a global option is toggled, or set a channel form field true merely because its effective value is inherited.

Correct: derive effective state at request/render time, preserve local preferences, and invalidate the channel inheritance query after successful global saves.
