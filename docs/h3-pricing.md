# MiniMax H3 billing

The October 2026 baseline reverses the confirmed MetaSo 80% prices. Prices
below use the deployment's existing quota currency (CNY); group and user/channel
ratios are applied after the complete baseline amount.

| Item | ModelPrice key / rule | Baseline |
| --- | --- | --- |
| 768P output and reference video | `MiniMax-H3` | 0.1125 / second |
| 2K output and reference video | `MiniMax-H3` × 5/3 | 0.1875 / second |
| First 5 input images | Free | 0 |
| Additional input images | `MiniMax-H3-Input-Image` | 0.0625 / image |
| Input audio | Free | 0 |
| Context IR | `MiniMax-H3-Context-IR` | 0.0625 / task |

`MiniMax-H3-Input-Image` is a configurable meter price, not a callable model.
Images are counted from the validated content actually sent upstream; caller
metadata cannot supply a billing count or price. Input image cost is additive,
outside duration/resolution multipliers, and inside the effective group ratio.
With group ratio 1.2 the corresponding rates are 0.135/s, 0.225/s, 0.075/image,
and 0.075/IR task. A 5-second 2K video with 9 images costs 1.425 before IR,
or 1.5 with one successful IR task.

Video tasks reserve requested output plus 15 reference-video seconds when any
reference video is present. Successful tasks settle against bounded upstream
`usage.output_seconds` and `usage.input_seconds`, refunding unused reservation.
Missing/invalid usage keeps the reservation. Prices, image quantities, and all
effective route ratios are frozen in `TaskBillingContext`; settlement never reads
new settings. New snapshots use `h3_media_v2`; historical snapshots retain their
existing billing rules. Failure refunds use the existing task accounting path.

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
