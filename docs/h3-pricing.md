# MiniMax H3 billing

The existing baseline reverses the confirmed MetaSo 80% prices. The four
resolution factors below were checked against [MetaSo's official price page](https://metaso.cn/minimax-h3)
on 2026-11-11, using its standard-channel rates for videos up to 15 seconds:
480P 0.06, 768P 0.09, 1080P 0.14, and 2K 0.15 CNY per second. The same rates
apply to reference-video seconds. The existing 768P `ModelPrice = 0.1125` stays
unchanged; group and user/channel ratios apply after the complete baseline amount.

| Item | ModelPrice key / rule | Baseline |
| --- | --- | --- |
| 480P output and reference video | `MiniMax-H3` × 2/3 | 0.075 / second |
| 768P output and reference video | `MiniMax-H3` | 0.1125 / second |
| 1080P output and reference video | `MiniMax-H3` × 14/9 | 0.175 / second |
| 2K output and reference video | `MiniMax-H3` × 5/3 | 0.1875 / second |
| First 5 input images | Free | 0 |
| Additional input images | `MiniMax-H3-Input-Image` | 0.0625 / image |
| Input audio | Free | 0 |
| Context IR | `MiniMax-H3-Context-IR` | 0.0625 / task |

`MiniMax-H3-Input-Image` is a configurable meter price, not a callable model.
Images are counted from the validated content actually sent upstream; caller
metadata cannot supply a billing count or price. Input image cost is additive,
outside duration/resolution multipliers, and inside the effective group ratio.
With group ratio 1.2 the four video rates are 0.09/s, 0.135/s, 0.21/s and
0.225/s; additional images and IR remain 0.075/image and 0.075/IR task.
A 5-second 2K video with 9 images costs 1.425 before IR,
or 1.5 with one successful IR task.

Video tasks reserve requested output plus 15 reference-video seconds when any
reference video is present. Successful tasks settle against bounded upstream
`usage.output_seconds` and `usage.input_seconds`, refunding unused reservation.
Missing/invalid usage keeps the reservation. Prices, image quantities, and all
effective route ratios are frozen in `TaskBillingContext`; settlement never reads
new settings. New snapshots use `h3_media_v2`; historical snapshots retain their
existing billing rules. Failure refunds use the existing task accounting path.

A successful response with a known lower `task.resolution` refunds the resolution
premium using the frozen base price and route ratio. All six downward combinations
among 480P, 768P, 1080P and 2K are covered. This applies even when billed seconds
equal the reservation, and combines with unused-second refunds without reducing
the frozen image charge. Missing/unknown resolution keeps the original resolution
factor; unknown historical factors are not assigned a new tier. Bounded usage
is still required, and usage above the reservation is not adjusted. Settlement
only lowers the current task quota. The existing successful-status CAS and
persisted quota prevent repeated/stale polls from issuing the same refund again.

The adapter reads the actual resolution from `task`, `result.task` or a flat
`result` query response and exposes it as `metadata.resolution` in
`GET /v1/videos/{id}`. It never substitutes the requested resolution when this
field is missing. This keeps provider-specific upscaling behavior inside FlowAPI.

Request `size` accepts the four tiers case-insensitively; an omitted size still
defaults to 768P. Metadata cannot override the validated resolution or its price.
This change retains the existing 4–15 second request limit. It does not add 4K,
16–30 second pricing, priority channels, channel cost changes or historical task
recalculation. Provider cost/profitability is separate from these customer rates.

## Rollout status and verification

Prepared for review on 2026-11-11; deployment requires the user's later idle-window
instruction. This branch does not modify live channel enablement, prices or data.
Until deployed, the running older adapter still rejects 480P/1080P and does not
refund a successful resolution downgrade. No new ModelPrice keys or database
migration are required for these two tiers.

Local regression covers four-tier request serialization/reservation, all six
downward resolution adjustments, preserved image/effective-ratio charges, missing
or invalid provider usage, and repeated/stale polling through the real H3 adapter
with an isolated SQLite wallet/token/task ledger and a fixture HTTP transport.
The fixture test reserves 4.50 for a 1080P reference task with four chargeable
images, settles to 1.38 after delivery at 768P, and records one 3.12 refund.
No paid provider calls or production changes are part of these tests.

Verified locally on this change: `go test ./relay/channel/task/hailuo ./relay
./service ./model -count=1`, H3 `go test -race`, the root-module portion of
`make test`, `go vet ./...` and `go build ./...`. The independent relaykit tests,
vet and build also passed with `GOWORK=off` after retrying a timed-out
`golang.org/x/text` download through a command-scoped Go proxy. The root compile
check used the same temporary `web/dist/index.html` placeholder as CI; it was
removed afterward and is not a frontend release build.

After authorized deployment, verify a minimal real 480P and 1080P task and inspect
the submitted tier, provider result, final task quota and refund log. A real
upscaling-failure response and MySQL/PostgreSQL settlement remain unverified by
the local SQLite fixture; do not claim production acceptance from these tests.

## Context IR wire contract

Submit through the existing `POST /v1/videos` endpoint:

```json
{
  "model": "MiniMax-H3-Context-IR",
  "prompt": "Describe the intended shot",
  "seconds": "5",
  "metadata": { "ratio": "16:9" }
}
```

Image/frame/video/audio metadata matches the normal H3 request contract. The
adapter sends native `POST /v2/h3_context_ir` with `model: MiniMax-H3`, content,
duration and ratio. IR is a separate async task charged once regardless of input
media count, resolution, or token usage. Poll `GET /v1/videos/{id}`; completed
responses contain `metadata.task_type = h3_context_ir` and `metadata.prompt`.
The caller then submits a normal H3 video with that enhanced prompt and the same
media. An IR task that succeeds remains charged even if the separate video fails;
a failed IR task follows normal full task refund. No automatic paid IR retry is
added by this adapter.

Deployment must merge the three ModelPrice entries above into the live options,
preserve `vedio = 1.2`, and add only `MiniMax-H3-Context-IR` to the existing MetaSo
H3 channel/token model allowlists. Do not map it to the legacy Hailuo protocol.
Default code prices do not overwrite already persisted production prices.

## Regeneration boundary

MetaSo's undiscounted regeneration rates are 0.075 per output/reference-video
second and 0.05 per image after the first 5. The current FlowAPI/Canvas product
has no regeneration entry point. These are not ordinary 2K generation prices;
do not configure them as the H3 2K rate or claim the native
`/v2/video_regeneration` endpoint is implemented.

Protocol references: MiniMax [IR](https://platform.minimaxi.com/docs/api-reference/video-generation-v2-h3-context-ir),
[query](https://platform.minimaxi.com/docs/api-reference/video-generation-v2-query),
and [regeneration](https://platform.minimaxi.com/docs/api-reference/video-generation-v2-regeneration).
