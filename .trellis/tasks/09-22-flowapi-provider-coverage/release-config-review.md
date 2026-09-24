# Release configuration review — 2026-09-25

Scope: the two pricing-verification JSON deployment artifacts only. No runtime code, production access, provider requests, or credentials were changed by this reviewer. User authorization to deploy is recorded, while provider verification remains a separate gate. The production importer must record actual state separately from this manifest.

## Fixed

- **FastAI source 64:** target type is **103**, not the source's type 60. `relay/relay_adaptor.go` dispatches only 103 to the FastAI task adapter. Explicit mappings retain the supported Mini and 2.5 token routes.
- **FastAI 2.0 / 2.0 Fast:** removed these two routes from the target channel's `models` and mapped them under `blocked_routes` / pricing `withheld_routes`. Their public names have token `ModelRatio` 23 / 18.5, but the source maps to per-second upstream aliases. `relay/relay_task.go` rejects those incompatible routes before pre-consume. Do not convert ratios into guessed per-second prices or change shared public model prices to `ModelPrice`: the native Seedance source 42 still serves all four public models correctly. Enabling FastAI time variants later requires separately named public models and verified per-second prices.
- **Viapi source 50:** split into two target channels sharing source credentials: `Viapi-Ali-Image` (`image`, segmentation only) and `Viapi-Ali-Video` (`vedio`, video super-resolution / subtitle removal). These are video actions in `relay/channel/task/viapi/action_table.go`, despite the former generic Vision name. **Importer identity must be `(source_channel_id, group)`, not source ID alone.** The deployment now has **15 target channels from 14 source channels**, still **46 unique public models**.
- **VolcCV 52:** fixed the manifest base URL to `https://visual.volcengineapi.com`, matching the adapter's fixed signed CV endpoint. The previous Ark URL was unrelated. VOD explicitly records its fixed `https://vod.volcengineapi.com` endpoint; native Seedance explicitly records its code default `https://ark.cn-beijing.volces.com`. These values are code-backed endpoint resolution, not invented observations of source configuration.
- **Volc TTS 54 / ASR 56:** explicit empty target mappings discard the source's empty-string TTS mapping and the ASR channel's unrelated TTS entry. Keeping `doubao-tts-2.0` selects the Seed TTS V3 protocol; ASR has its own fixed endpoint.
- **Prices:** retained all 34 approved deployment `ModelPrice` baselines and 12 `ModelRatio` baselines. The chosen price is keyed by the public model name. HappyHorse 1.1 is explicitly documented as a 0.5 family baseline adopted from 1.0; current Keli DB has no ModelPrice key for those three models and no code default exists. Their zero tiered expressions are not imported. Added instructions to remove conflicting opposite-mode keys for these imported models only. No supplier-cost evidence was invented.
- All manifest channels remain `disabled_pending_provider_verification` rather than implying a new user approval is needed. The user already authorized deployment. Enable individual routes only according to release verification evidence.

## Import prerequisites owned by main session

- Merge only these 46 model keys into live pricing; preserve unrelated live options and groups. Do not use the still-null independent future `target_price` field as the deployment price. The machine-readable baselines are under `deployment_base_pricing`.
- Apply the target manifest's type, models, mappings and groups rather than wholesale copying the source rows. `blocked_routes` and `withheld_routes` are evidence only and must not create abilities.
- Transfer credentials outside the repository; preserve required account metadata. MiniMax requires account consistency for file/voice resources. Volc ASR additionally needs VOD credentials, space, playback domain and optional signing metadata from channel `OtherInfo`; VOD needs its space/playback metadata. The safe static artifact cannot assert live credential completeness.
- The two Viapi rows reuse source 50 credentials, but create separate target channel IDs and abilities. Endpoints are selected per action by the adapter.
- Source retail values are not verified supplier costs. Provider requests, bill settlement and media retrieval still require live release evidence; no static check certifies those.

## Verification

- `jq empty` passed for both JSON artifacts.
- Root `go test ./...`, independent `relaykit` build, and frontend `bun run typecheck` passed on 2026-09-25; details are in [release-gate-2026-09-25.md](release-gate-2026-09-25.md).
- A deterministic read-only manifest check passed: 15 target identities / 14 source IDs / 46 unique public models; channel models exactly equal price rows and the disjoint 34 `ModelPrice` + 12 `ModelRatio` keys; all model mappings belong to their channel; the retained FastAI aliases are token routes; exactly two time routes are withheld; Viapi groups match action media types; Wan appears only under source 75 with `wan3.0-video` and `wan3.0-video-prime`; credentials are null.
- Runtime Go tests and frontend lint/typecheck are delegated to the other release checker; this reviewer changed JSON/docs only and does not claim their results.
- No exposed Serena tools were available; navigation used read-only terminal search and file reads, and modifications used `apply_patch`.
