# Release gate — 2026-09-25

This is the release record for the authorized HK FlowAPI deployment.

## Checks

| Check | Result |
|---|---|
| `go test ./...` (repository root) | PASS |
| `cd relaykit && GOWORK=off go build ./...` | PASS |
| `cd web && bun run typecheck` | PASS |
| JSON manifests (`jq empty`) | PASS |
| Deployment manifest identity/price/group invariants | PASS: 15 target channels, 14 source IDs, 46 models, 34 ModelPrice and 12 ModelRatio keys |
| HK app image | PASS: `flowapi:hk-media-8156149df-20260925`, health and public status OK |
| Database/cache preservation | PASS: PostgreSQL and Redis container IDs/start times unchanged |
| Production backup | PASS: `/opt/flowapi/backups/pre-provider-coverage-20260925/`, SHA256 and `pg_restore -l` verified |
| Config import | PASS: 46 models, 15 channels, `image`/`vedio`; all target channels enabled after smoke |
| Live smoke | PASS: Seedream image retrieval, MiniMax TTS, Volc TTS, Rolldek Wan submit/poll/retrieval; duplicate Wan poll did not add a charge |
| `git diff --check` | PASS |

The existing full frontend lint limitation remains documented in the pricing verification report: repository-pre-existing errors outside this change block the full lint command, while focused modified-file lint passed earlier.

## Live release evidence

- FlowAPI HK production is serving version `hk-media-8156149df-20260925` at `https://flowapi.robusta.top`. KeliAPI Guangzhou was not modified.
- Imported target channels are IDs 69–83. FastAI time routes remain withheld; Wan is configured only on Rolldek channel 83.
- Smoke billing examples: Seedream quota 110000, MiniMax TTS 2160, Volc TTS 3000, Wan submit quota 360000 with 2-second output.
- Provider response evidence is kept in the root-only temporary release directory; signed URLs and credentials are not committed.

## Release boundaries

- Production deployment and configuration import were authorized by the user on 2026-09-25 and completed by the main session.
- All manifest channels remain disabled until their provider smoke evidence is recorded. FastAI time routes remain withheld because they need separately named public models and verified per-second prices. Wan is Rolldek-only.
- HappyHorse 1.1's 0.5 deployment baseline is an explicit product/configuration decision adopted from the HappyHorse 1.0 family baseline. The current Keli database has no `ModelPrice` key for the three 1.1 models; its zero `tiered_expr` must not be imported or interpreted as free, and no code default exists. This is not supplier-cost evidence.
- The main session owns credentials, backups, production writes, image/build swap, health checks, rollback and live provider verification.
