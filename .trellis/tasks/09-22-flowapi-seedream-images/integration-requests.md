# Images integration requests

Channel send is blocked by sandbox EPERM on `/Users/Zhuyu/.trellis/channels/.../provider-coverage.lock`. These requests are the fallback.

- Images owns `relaykit/dto/openai_image.go` image-only optional fields and new `relaykit/dto/seedream_image.go`; please avoid concurrent edits. DTO must survive existing JSON deep-copy (unknown Extra is deliberately not marshaled).
- Images owns `relay/image_handler.go` and image tests: final outbound JSON validation/sanitizing applies to passthrough and overrides too; reserve model-price layer max 17 × 0.5 before send with `PrepareBillingForSelectedRoute`, settlement replaces the same n ratio. Expression mode remains authoritative. Invalid usage is a non-retryable 502 before response write; existing controller refund and error log remain responsible.
- Main: `volcengine.Adaptor.ConvertImageRequest` generations branch should call `ConvertSeedreamImageRequest(request)` (new image-owned function).
- Main: add native `doubao-seedream-4-5-251128`, `doubao-seedream-5-0-260128` to `volcengine.ModelList`; 4.0 already registered.
- Main: add `doubao-seedream-5-0-pro-260628` to `openai.ModelList` for OpenAI/NewAPI compatible image channels. Keep Pro out of native list.
- Confirmed fixture surface: JSON generations with `image` as URL/data URL or array supports reference editing. Do not claim native or Pro multipart `/images/edits` provider support. Layer streaming has no confirmed contract and is rejected before send.

No real provider calls or deployment.

## First verification

- `GOWORK=off go build ./...` in relaykit passed (2026-09-22).
- OpenAI image/Seedream and relay/helper image tests passed.
- Combined root/volcengine build currently blocked by other workers' unfinished `dto.NewOpenAIVideo`/`VideoStatus*`, `taskcommon.ResolveEndpointURL`, and `relaykitrelaykitdto` symbols in task adapters. Images has not altered those files.
