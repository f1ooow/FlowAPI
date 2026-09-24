# Video integration contract

Channel send failed: sandbox denies ~/.trellis/channels lock creation. This file is the handoff until main can read.

Owned new DTOs: relaykit/dto/wan.go, dto/wan.go aliases. WanCreatePath and WanProtocolRolldek are source-compatible.

Parent required:
- TaskInfo.OutputVideoDuration float64.
- TaskBillingContext.BillingUnit string, InputVideoDuration/OutputVideoDuration float64; immutable EffectiveRatio and price snapshots preserved.
- relaykit/dto.TaskEndpointOverride { SubmitPath, FetchPath string }; ChannelOtherSettings.TaskEndpointOverride pointer and VideoProtocol string.
- taskcommon.ResolveEndpointURL(baseURL, path string, variables map[string]string) (string,error).
- Poll adaptor Init with saved settings and original upstream model; FetchTask body includes model.
- TaskSubmission migration, middleware and authenticated GET /api/v1/task-submissions/:client_key; source recovery response header X-Keli-Task-Recovery: request-id-v1.
- Before POST persist selected route using forthcoming PrepareTaskSubmissionDispatch; once dispatched never retry POST, preserve reserved public ID, atomic InsertTaskForSubmission and buffer response until durable task insert.
- Register dedicated FastAIToken=103, never blanket task routing for NewAPI=60.

Source reference is read-only /Users/Zhuyu/Documents/Code/new-api, dirty files are not production proof. No paid provider calls.

## Implemented exact APIs

- `model.PrepareTaskSubmissionDispatch(id, userID, tokenID int, route model.TaskSubmissionRoute) error`: call after reservation and before POST. Route contains ChannelID, ChannelType, APIKey, BaseURL, UpstreamModelName, ChannelOtherSettings, BillingContext, ReservedQuota. Durable one-shot CAS. Acceptance checks task user/token/channel/key/upstream model against reservation.
- `service.wanActualQuotaChecked(task *model.Task) (int, *common.QuotaClamp)`: polling settlement must forward clamp. EffectiveRatio is preferred; polling calls the checked API directly.
- `fastaitoken.ResolveModel(name string) (upstream, unit string, ok bool)` returns unit `seconds` / `tokens`. EstimateBilling sets context task_billing_unit and normalizes UpstreamModelName, leaves OriginModelName. Parent must snapshot BillingUnit and require explicit ModelPrice for seconds / token ratio for tokens; time parses intentionally suppress token usage to avoid double settlement. Wan FastAI aliases are provisional flat /v1/videos/generations; no Seedance token table applies.

Important: TaskSubmission.Insert now validates task.PrivateData.TokenId. Tests and caller must populate the exact authenticated token. Source tables were not enough to persist route; new RouteJSON text and DispatchedAt persist it before the POST. DB lockForUpdate used, no dialect-specific JSON/new DB enum.

## Main follow-up (19:46)
- Added requested TaskSubmissionRoute.FundingSource string, SubscriptionID int, Group string.
- FinishTaskSubmission(id, dispatched, definiteFailure ...bool) keeps existing 2-arg API compatible. Middleware passes c.GetBool("task_submission_definite_failure"). Main must set this flag true for its proven rejected 4xx (except 408) branch; task_submission_uncertain=false alone is not enough because successful response followed by persistence failure is still unknown. Dispatched default remains unknown.
- H3 converter now calls minimax.CheckResourceOwnership for every mm_file:// resource; tests for wrong key/channel follow.
- Initial focused tests pass Ali/Hailuo/FastAI/Sora/Doubao/model/service/dto. Controller blocked transiently by other worker viapi/volcvod dto imports. Sandbox blocks TCP listen, so owned HTTP protocol fixtures use in-memory RoundTripper.

## FastAI billing guard

Time-model BuildRequestBody requires PriceData.UsePrice, so absent explicit public-model ModelPrice cannot silently dispatch against token fallback. Main should validate this immediately after pricing and return 400, before reserve. In its controller success branch snapshot c.task_billing_unit as BillingUnit; seconds must be PerCallBilling=true (submitted duration charge) until verified usage contract exists. Token models use token settlement with only the video-input ratio; never the seconds multiplier. FastAI Wan aliases use seconds and no Seedance ratio table. Provider resolution/time retail rates are pricing-owned and remain unverified.

## Final integration status

Main integrated all requested shared fields and hooks, observed in current workspace: endpoint resolver; 103 registration; frozen settings/key/base/OtherInfo; NewTaskBillingContext; PrepareTaskSubmissionDispatch with funding/subscription/group; definite failure flag; FastAI seconds ModelPrice/token ModelRatio guard and BillingUnit snapshot. Focused controller recovery and per-task polling tests now pass. `controller/task_relay_contract_test.go` and `service/task_polling_account_test.go` were explicitly assigned by main as additional ownership and are included.

No provider-side recovery lookup is invented: source only exposes authenticated gateway submission-key lookup. If paid upstream accepted before provider ID is persisted, state stays unknown with original account/funding snapshot and no second POST. Exact upstream lookup remains a provider evidence dependency. Cross-database executable migration checks remain main integration ownership; local tests use SQLite and portable GORM locking/transactions.
