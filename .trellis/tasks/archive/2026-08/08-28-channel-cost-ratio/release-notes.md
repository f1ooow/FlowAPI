# Release Notes

## Channel cost ratios

- Channels now support a root-managed cost ratio. Existing channels without a stored value continue to use `1x`.
- Billing groups can opt into the selected channel ratio. The effective multiplier is the billing-group ratio multiplied by the user-group ratio and, when enabled, the final selected channel ratio.
- Cross-channel retries refresh the effective multiplier and reserve any higher estimated charge before the next upstream attempt.
- Asynchronous tasks persist the complete multiplier snapshot so later configuration changes do not reprice an existing task.

## User-group ratio migration

- Legacy absolute special ratios are migrated once into user-group multipliers by dividing by the target billing-group ratio.
- A zero billing-group ratio combined with a positive legacy special ratio is reported as a blocking conflict and remains on legacy runtime semantics until an administrator resolves it.
- The migration is versioned and idempotent.

## User display

- Authenticated API Key group selection now shows only the final multiplier: a single value, a current enabled-channel range, Auto, or unavailable.
- Anonymous group responses retain the legacy single `ratio` contract and do not expose cost-derived ranges.

## Administrator pricing tools

- Channel row-level upstream-ratio fetching is replaced by explicit channel cost-ratio configuration.
- Global model-price synchronization remains available through `GET /api/ratio_sync/channels`, `POST /api/ratio_sync/fetch`, and the model-pricing sync UI.
- `GET /api/ratio_config` and channel upstream model-update endpoints remain available with their existing authorization behavior.
