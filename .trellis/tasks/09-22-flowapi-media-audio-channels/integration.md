# Media integration (in progress)

Channel send blocked by sandbox EPERM on ~/.trellis/channels lock; this file is the early integration handoff.

- Register task adaptors viapi.TaskAdaptor / volcvod.TaskAdaptor / volccv.TaskAdaptor at 100/101/102.
- MiniMax controller RelayMiniMaxExtended needs /v1/files/upload multipart, /v1/voice_clone, /v1/music_generation under authentication and model distribution. RelayModeMiniMaxFilesUpload/VoiceClone/Music constants. Upload always bypasses price lookup and reservation; clone/music per-call explicit ModelPrice, not token fallback.
- Resource ownership persistence required: provider, kind(file/voice), resource ID string, user ID, channel ID, credential SHA256. Main should implement model lookup/create and distributor selection for consumed resource IDs, or reject a mismatched selected route before forwarding. Upload and clone save binding before success. Worker implements provider glue once main provides interfaces. TTS and H3 also need lookup/validation; unknown IDs cannot silently hop accounts.
- Polling must retain original channel ID, credentials, upstream model/action and channel OtherInfo VOD config. VOD playback domain/scheme/auth key and upload space are needed. Secrets belong only in private task data.
- Task billable units: Viapi video minute, Viapi image call; VOD output-minute erase / input-minute audio extract; CV call. Multipliers will be named minutes, never seconds against minute prices.
- Volcengine shared adaptor audio dispatch patch will be supplied separately; do not overwrite image code.

## Concrete APIs implemented

`relay/channel/minimax/resources.go` owns portable GORM `ResourceBinding` and exports:
- `MigrateResourceBindings(db *gorm.DB) error`: call once during startup after DB init (root main can call; model must not import provider).
- `LookupResourceBinding(userID int, kind, id string) (*ResourceBinding,error)` for distributor/H3.
- `ResourceCredentialHash(baseURL,key string) string` and `CheckResourceOwnership(info,kind,id) error`; bound channel ID and exact base URL + key hash must match.
- `BindResource(info,kind,id) error`: insert only, conflicting account rejected.
- `CheckVoiceOwnership(info,voiceID) error`: known system voices allowed, custom voices require binding.
No secrets stored. Upload ID and clone voice binding written before success; clone reserves voice name before upstream to preserve account after uncertain responses. Main should select matching original channel/key using binding; provider rejects mismatches even before routing integration. H3 must check each consumed file ID with CheckResourceOwnership (and main must make these available on type-60 H3 upload channel if that is target architecture; current extended controller requires MiniMax type).

`volcengine-dispatch.patch` contains exact five audio dispatch changes for main to apply around image work. Adds transcription + SeedTTS V3 conversion, URL, headers, request, response. Dedicated `setupSeedTTSV3Headers` exists. Add models doubao-tts-2.0 / seed-tts-2.0-standard / doubao-asr-2.0 to shared constants.

Provider synchronous responses set context `media_sync_result` to `*relaycommon.TaskInfo`. Main controller must persist SUCCESS + result URL + finish time and skip future polling for this context (Viapi segment-common, CV lens-lqir), preserving normal quota/log path.

## Updated model API (main request at 11:48 incorporated)

DB ownership now in `model/minimax_resource.go` (authorized additional file). Register `&model.MiniMaxResourceBinding{}` in existing auto-migration. Exported:
- `model.LookupMiniMaxResourceBinding(userID int, kind, id string) (*MiniMaxResourceBinding,error)`
- `model.MiniMaxCredentialHash(baseURL,key string) string`
- `model.IsKnownMiniMaxVoice(id string) (bool,error)`
- `model.BindMiniMaxResource(userID int,kind,id string,channelID int,baseURL,key,state string) error`
- `model.CompleteMiniMaxVoice(userID int,id string,channelID int,baseURL,key string) error`
States `pending`/`ready`: clone reserves prior to upstream, duplicate reservation rejected to prevent paid resend; only successful response marks ready; pending cannot synthesize. Custom gateway voice across users rejected. All unknown voice names remain compatible system voices per main request; clients can mark `metadata.voice_source=custom` to require ownership for external/unknown custom names. This cannot identify an externally created custom voice without caller declaration, so do not claim that case is fully proven.

New optional polling interface (implementations on viapi/volcvod):
`AdjustBillingOnCompleteChecked(*model.Task,*relaycommon.TaskInfo) (int,*common.QuotaClamp,bool)`
Main must call before `PerCallBilling` short-circuit, pass clamp to RecalculateTaskQuota, honor a handled zero quota. `media_billing_unit` context is per-minute / per-output-minute / per-input-minute; prices are currency/minute, OtherRatios.minutes=duration/60. VOD actual seconds carried in TaskInfo.OutputVideoDuration (audio-extract semantics = input duration). Viapi only adjusts if upstream supplies Duration; otherwise declared duration stays the billable quantity.

VOD `SnapshotMediaSettings() map[string]string` / `RestoreMediaSettings(map[string]string)` available, store privately and restore AFTER Init when polling. Keys playback domain/scheme/auth key. `volcvod.ResultMediaURL(encoded,track)` decodes current client-compatible envelope for result proxy; track default voice for AudioExtract and video for Erase, bg supported. Playback domain required for playable results. Main must unwrap before download proxy.

Current packages and controller compile with GOCACHE=/tmp/flowapi-media-go-cache. Main-applied audio dispatch confirmed. Focused fixtures being added.

## Verification

Focused provider tests (all pass):
`GOCACHE=/tmp/flowapi-media-go-cache go test ./relay/channel/task/viapi ./relay/channel/task/volcvod ./relay/channel/task/volccv ./relay/channel/minimax ./relay/channel/volcengine ./controller -run 'TestMiniMax|TestVolcengine|TestVOD|TestViapi|TestPreprocess'`.

Full controller/model runs are sandbox-blocked by unrelated tests attempting localhost/miniredis listeners (`operation not permitted`). No provider calls or deployment.
