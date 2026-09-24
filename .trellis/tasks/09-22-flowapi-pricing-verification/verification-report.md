# Pricing and configuration implementation report

## Implemented locally

- Added channel type labels, icons, key hints, and form support for Viapi (100), VolcVOD (101), VolcCV (102), and FastAI Video (103).
- Added media settings for the existing channel editor: `video_protocol` supports only blank or `wan-rolldek-v1`; submit/fetch endpoint overrides are checked as same-origin paths and fetch paths require exactly one `{task_id}`. VOD credentials remain in the existing `other_info` JSON field and are never prefilled from secrets.
- Added a billing-unit catalog (`model/media_billing_units.json`) and exposed `Pricing.billing_unit`. Pricing cards/details display seconds, minutes, characters, images, tiers, tokens, or request according to the catalog. MiniMax clone/music remain per-call; upload is free. FastAI time aliases remain separate from token aliases.
- Usage-log billing details now show the persisted billing unit when task settlement records one.
- Added review-only channel and pricing artifacts. The 34 fixed-price and 12 ratio-price models in the 46-model target scope have deployable `ModelPrice`/`ModelRatio` baselines copied from Keli's retail configuration; FastAI Wan aliases are excluded from the target and Wan remains available only through Rolldek. Commercial `target_price` stays null pending supplier-cost and product approval. No channel is enabled by these artifacts.

## Validation

- `go test ./model` passed.
- `bunx vitest run src/features/channels/lib/__tests__/new-api-channel.test.ts src/features/channels/lib/__tests__/media-settings.test.ts` passed (8 channel/settings behaviors).
- `cd web && bun run typecheck` passed.
- Focused `oxlint` on modified frontend files passed.
- `cd web && bun run build` passed and produced `web/dist`.
- `bun run lint` across the existing frontend remains blocked by unrelated pre-existing errors outside this change; no focused errors remain.
- `bun run i18n:sync` passed. This checkout currently contains only `zh.json`; no absent locale files were fabricated.

## Not verified

No provider calls, credentials, production writes, deployment, or commercial price approval were performed. Target prices and source costs remain pending. The smoke caps in `verification-budgets.md` require a separately approved isolated group/token and real provider execution.
