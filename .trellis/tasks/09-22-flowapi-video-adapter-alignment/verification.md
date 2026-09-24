# Video child verification

Status: bounded implementation and focused local verification complete, 2026-09-22. No paid supplier calls, SSH, production changes, commits or pushes. Source reference remains read-only; source-sample.md records hashes. Integration calls are documented in integration.md.

## Delivered contracts

- Ali HappyHorse 1.0/1.1 + Zivv 61 explicit/default-empty overrides, original account and model, UNKNOWN nonterminal, metadata duration bounds, false/zero controls preserved; existing Wan2.7 tests retained.
- Doubao four Seedance versions, native submit/fetch, token usage and input-video ratios, top-level and metadata duration bounds and media count limits. Duration integer and seconds string use the same bounded wire field.
- Hailuo resolution/duration tiers use the normalized final wire payload; H3 4–15 seconds, 768P/2K, v2 submit/query envelopes and errors, full int64 file IDs and original user/channel/credential checks through MiniMax binding.
- Dedicated FastAI 103 adapter, four source Seedance aliases and wan-3.0 -> wan-3.0-time; flat media payload, native /v1/videos/generations and matching fetch, time/token unit separation, no Seedance price ratios on Wan. Time requires explicit ModelPrice; time results cannot trigger token settlement.
- Rolldek Wan profile isolated from legacy Sora; official DTO in relaykit, root aliases; validation/media controls, video-track duration probe with SSRF protected transport and bounded downloads, immutable effective-ratio quota calculation with checked saturation.
- Durable TaskSubmission key/owner/fingerprint and before-POST route/account/funding/billing reservation. Original public ID, atomic task association, definite failure vs unknown, no unknown replay, restart/read and cross-token isolation. Response publication waits for persisted task; repeated terminal polling does not double charge/refund.
- Polling fixtures prove frozen original key/base/model/protocol/endpoint/OtherInfo, including empty snapshots and legacy fallback.

## Commands and results

Passed all five owned video package tests with:

```sh
GOCACHE=/tmp/flowapi-video-go-cache go test ./relay/channel/task/ali ./relay/channel/task/hailuo ./relay/channel/task/doubao ./relay/channel/task/sora ./relay/channel/task/fastaitoken
```

Passed nine-package focused integration command:

```sh
GOCACHE=/tmp/flowapi-video-go-cache go test ./relay/channel/task/ali ./relay/channel/task/hailuo ./relay/channel/task/doubao ./relay/channel/task/sora ./relay/channel/task/fastaitoken ./dto ./model ./service ./controller -run 'TestWan|TestTaskSubmission|TestTaskResponseIsPublished|TestTaskRetryDoesNot|TestVideoPollingUses|TestConvertToAli|TestSoraBuild|TestTaskAdaptorEndpoint|TestBuildH3|TestDoH3|TestH3|TestHailuo|TestAli|TestParseH3|TestLegacyMetadata|TestProcessAli|TestParseTaskResult|TestGetVideo|TestFastAI|TestSeedance' -count=1
```

Passed static checks:

```sh
GOCACHE=/tmp/flowapi-video-go-cache go vet ./relay/channel/task/ali ./relay/channel/task/hailuo ./relay/channel/task/doubao ./relay/channel/task/sora ./relay/channel/task/fastaitoken ./dto ./model ./service ./controller ./middleware
cd relaykit && GOCACHE=/tmp/flowapi-video-go-cache GOWORK=off go build ./...
```

`gofmt` and `git diff --check` pass. Initial failures from restricted default Go cache, forbidden TCP listeners, and parallel Viapi/VOD imports were resolved using /tmp cache, deterministic RoundTripper fixtures, and the media worker's import corrections. No tests skipped in the stated commands. Test fixtures make no provider network requests.

## Validation boundaries

- FastAI Wan endpoint/media/status contract is provisional, derived from source FastAI Seedance protocol plus configured Wan aliases; provider compatibility and exact price/resolution/usage need parent P4 evidence. No authoritative provider time usage is invented.
- Source recovery has no verified supplier lookup API for the acceptance-before-ID crash window. Such reservations stay unknown with the saved account/funding snapshot; authenticated gateway lookup never invents a task ID or repeats the paid POST.
- SQLite tested; real MySQL/PostgreSQL migration/restart validation and full workspace regression belong to main. GORM transactions/lockForUpdate and text storage preserve dialect portability, but are not claimed as executed database evidence.
- No frontend/production verification claimed.

## Files written

- `relay/channel/task/ali/adaptor.go`
- `relay/channel/task/ali/adaptor_test.go`
- `relay/channel/task/ali/constants.go`
- `relay/channel/task/ali/protocol_test.go`
- `relay/channel/task/hailuo/account_test.go`
- `relay/channel/task/hailuo/adaptor.go`
- `relay/channel/task/hailuo/adaptor_test.go`
- `relay/channel/task/hailuo/constants.go`
- `relay/channel/task/hailuo/models.go`
- `relay/channel/task/hailuo/protocol_test.go`
- `relay/channel/task/doubao/adaptor.go`
- `relay/channel/task/doubao/adaptor_test.go`
- `relay/channel/task/doubao/constants.go`
- `relay/channel/task/doubao/constants_test.go`
- `relay/channel/task/sora/adaptor.go`
- `relay/channel/task/sora/constants.go`
- `relay/channel/task/sora/wan.go`
- `relay/channel/task/sora/wan_test.go`
- `relay/channel/task/fastaitoken/adaptor.go`
- `relay/channel/task/fastaitoken/adaptor_test.go`
- `relay/channel/task/fastaitoken/constants.go`
- `relay/channel/task/fastaitoken/models.go`
- `relaykit/dto/wan.go`
- `dto/wan.go`
- `dto/wan_test.go`
- `service/wan_video.go`
- `service/wan_video_test.go`
- `service/task_polling_account_test.go`
- `model/task_submission.go`
- `model/task_submission_test.go`
- `middleware/task_submission.go`
- `controller/task_submission.go`
- `controller/task_submission_test.go`
- `controller/task_relay_contract_test.go`
