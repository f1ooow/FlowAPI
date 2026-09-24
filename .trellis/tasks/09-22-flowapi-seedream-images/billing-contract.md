# Seedream image contract and offline evidence

All fixtures are deterministic mocks; no paid request or production model availability is asserted.

| Client/upstream model ID | Channel/protocol | Endpoint | Reference editing |
| --- | --- | --- | --- |
| doubao-seedream-4-0-250828 | Volcengine native | POST /api/v3/images/generations | JSON image string / array |
| doubao-seedream-4-5-251128 | Volcengine native | POST /api/v3/images/generations | JSON image string / array |
| doubao-seedream-5-0-260128 | Volcengine native | POST /api/v3/images/generations | JSON image string / array |
| doubao-seedream-5-0-pro-260628 | OpenAI/NewAPI compatible | POST /v1/images/generations | JSON image string / array |

`/images/edits` and multipart are not claimed supported for these four models. Such Seedream requests return 400. Use JSON `image` on generations. Layer streaming lacks a verified provider contract and returns 400; unexpected upstream layer SSE returns 502 without retry.

Optional seed / guidance_scale / watermark / stream / layer_decomposition and sequential controls survive DTO deepcopy; omitted values remain omitted, explicit 0/false remain explicit. Pro sequential_image_generation and sequential_image_generation_options are removed from the final outgoing JSON, including passthrough/override paths. Native sequential fields remain. No general merge of unknown Extra fields was introduced. Existing passthrough unknown fields retain their original JSON values.

## Accounting for pricing owner

The reference 0.5 layer factor is implemented, but the final retail price remains subject to parent pricing review. ModelPrice comes from OriginModelName; final upstream model and actual boolean layer mode determine protocol/count behavior. GroupRatioInfo retains the selected route's effective group ratio.

- Ordinary image: existing bounded request n reservation and actual data array count settlement; no layer discount for absent/false. Wrong JSON type for layer_decomposition returns 400, never enables layers. A true layer flag on native Seedream returns 400.
- Native explicit sequential auto with max_images reserves at least that bounded maximum; disabled sequential does not add that multiplier. max_images and n share dto.MaxImageN=128 bound.
- Pro layer: only boolean true and exact supported Pro model, n omitted/0(default)/1. n>1 returns 400 so the allowed response bound is 17 total images. Reserve ratio n=17*0.5=8.5 before upstream dispatch. Settle by REPLACING n with actual generated_images*0.5; retain other existing ratios. Neither original n nor layer count gets multiplied a second time.
- Valid layer usage: integer generated_images 1..17 equal to returned data array length. Missing/null/zero/negative/fraction/string/overflow/out-of-range/mismatched values return 502 before forwarding image data and skip retry. Existing controller error logging and refund apply; this is never a successful zero-price result. Malformed response JSON also skips retry for layers.
- tiered_expr: existing captured expression is the complete pricing source. No implicit layer factor/n is applied, and the original request input remains frozen for param(). Usage validity still applies. Expressions cannot currently read generated_images; pricing must not configure actual-layer-count expressions until an explicit expression variable contract exists.
- Pre-consume uses existing strict quota conversion and selected-route reservation; settlement retains checked decimal quota conversion and saturation audit.

Illustrative ModelPrice=0.10, QuotaPerUnit=500000, effective ratio=2*0.8*1.5=2.4 (no other factors):

| Mode | Output images | n multiplier | Pre-reserve quota | Final quota | Refund quota | Final retail amount |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Pro layer | 1 | 0.5 | 1020000 | 60000 | 960000 | 0.12 |
| Pro layer | 17 | 8.5 | 1020000 | 1020000 | 0 | 2.04 |
| Ordinary | 2 (request n=3) | 2 | 360000 | 240000 | 120000 | 0.48 |
| Expression result 200000 | 1..17 valid | expression only | expression estimate | 240000 | depends on estimate | 0.48 |

Display fields: protocol/model, requested n, actual layer image count, layer factor, selected effective ratio, billing mode, charged quota. Actual layer count is appended to consume log text; existing other_ratios.n is the effective per-model quantity factor, not a raw layer count.

## Tests

- relay/channel/volcengine/seedream_test.go: three native models, text and single/multiple reference fixtures, endpoint and zero/false/omitted fields, native layer/Pro/native quantity rejection.
- relay/channel/openai/seedream_image_test.go: four model response fixtures and GPT/Gemini normal-count regression; Pro output URL/base64, 1/17 usage, malformed/invalid/oversized usage, expression priority and final mode override, unexpected SSE.
- relay/image_seedream_test.go: Pro final payload omission, quantities, native sequential bound, max reservation/retry ratio/mapped alias/override, expression isolation, insufficient funds/overflow, unsupported endpoints.
- service/seedream_image_billing_test.go: exact reservation/final/refund amounts, expression precedence and settlement saturation marker.

Shared registrations/converter wiring are owned by main; see integration-requests.md.
