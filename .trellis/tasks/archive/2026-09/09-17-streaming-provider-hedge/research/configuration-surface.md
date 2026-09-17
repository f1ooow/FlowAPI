# Channel Configuration Surface

Inspected current FlowAPI main at `606927f44`, 2026-09-17. Existing unrelated worktree changes were preserved.

## Existing Contract

- `web/src/features/channels/types.ts:95` defines `ChannelOtherSettings`; its `reliability` property contains `max_attempts`, `auto_ban_threshold`, and `auto_ban_duration_seconds`.
- `web/src/features/channels/lib/channel-form.ts:533` initializes defaults; lines around 563 load reliability JSON; lines around 800 write `settingsObj.reliability`.
- `web/src/features/channels/components/drawers/sections/channel-reliability-fields.tsx` renders bounded numeric fields through React Hook Form, project Input, and safeNumberFieldProps.
- `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx:992` computes routing configuration state from reliability defaults. New values must participate in watches, section error state, and configured summaries.
- Existing regression coverage: `lib/__tests__/channel-reliability-form.test.ts`, `components/drawers/sections/__tests__/channel-reliability-fields.test.tsx`.
- No per-channel first-byte hedge, idle-stream timeout, or nonstream total timeout fields were found in the channel feature.

## Proposed Surface

Reuse channel routing/reliability settings and current numeric-field patterns. Persist timeout configuration in existing channel JSON rather than adding database-specific columns. UI and API must reject negative, fractional, or excessive values, and preserve explicit zero. Do not introduce a separate routing settings page.

If all three timeout controls are included, differentiate the hedge trigger from hard cancellation timeouts. A zero per-channel override must have explicitly documented behavior relative to existing global transport/scanner timeouts; do not describe it as disabling every timeout in the system.

Use compact fields matching the existing admin drawer. Translate all labels and validation messages into all seven locales through the project i18n skill. Numeric seconds must round-trip without accidental minutes/milliseconds conversion.

## Pending Scope

The user requested the hedge feature after showing CCH's three controls. Whether the initial implementation also adds the other two per-channel timeout overrides is a product scope decision to confirm in the proposed plan.
