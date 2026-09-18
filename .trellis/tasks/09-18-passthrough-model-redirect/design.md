# Design

## Boundary

Use `relay/helper.NewPassthroughBody(storage, info, contentType)` to return a replayable outbound body and an optional owned closer. It reuses original storage for an unchanged model or non-JSON payload and creates isolated storage for a matched JSON redirect.

## Data flow

1. `ModelMappedHelper` resolves `OriginModelName` to `UpstreamModelName` for the selected channel.
2. Each JSON body passthrough handler uses the shared helper; no-op paths keep the original replayable storage without materializing it.
3. Gate rewriting on an actual channel redirect (`IsModelMapped`), not unrelated adapter normalization. Preserve the configured rule target exactly, including any thinking/effort suffix; skip Claude suffix adaptation during passthrough.
4. If it differs, parse the top-level JSON object as `map[string]json.RawMessage` using `common.Unmarshal`, replace only `model` with the target, and marshal with `common.Marshal`. Raw values preserve large-number precision and unknown nested fields.
5. Build an independently replayable outbound body and close only its owned resources. Never replace or mutate the immutable inbound storage shared by retries/hedged attempts. Continue using the current adapter transport.

## Compatibility

The helper preserves unknown fields semantically and only targets the top-level key. It does not affect converted requests, no-op mappings, URL model paths, multipart payloads, or async transports. Invalid JSON/object shapes fail before the upstream call so a wrong client model cannot be silently forwarded.

## Testing

Cover the shared rewrite helper with matching, no-op, unknown-field, nested-field, invalid JSON, and non-object cases. Add a focused passthrough handler regression where practical; the shared contract is the source of truth for all JSON handlers.

## Rollback

The change is isolated to the helper and JSON passthrough body construction. Reverting those changes restores the prior documented bypass behavior without schema or migration changes.
