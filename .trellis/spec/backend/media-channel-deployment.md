# Media Channel Deployment Contract

## Scenario: Importing Keli media coverage into FlowAPI

### 1. Scope / Trigger

Use this contract when deploying the reviewed Keli media manifest to FlowAPI. It covers channel/model identity, media groups, pricing mode, and withheld routes. It does not certify provider availability.

### 2. Signatures

The importer consumes two files:

```text
flowapi-disabled-channels.json
target-pricing-proposal.json
```

Channel identity is `(source_channel_id, group)`. A source channel may produce multiple target channels when one adapter contains both image and video actions (Viapi source 50).

### 3. Contracts

- Target groups are exactly `image` and `vedio` (spelling is contractual).
- `ModelPrice` models use `deployment_base_pricing.model_price[public_model]`; `ModelRatio` models use the ratio map. Remove the opposite mode only for imported model keys.
- Model price lookup uses the public/origin model name. An upstream mapping must not silently change the customer's price key.
- FastAI source type 60 is imported as FlowAPI type 103. Only its supported token aliases are enabled; Wan is Rolldek type 1 only.
- Viapi video actions (`viapi-super-resolve`, `viapi-erase-subtitles`) belong to `vedio`; image segmentation belongs to `image`.
- Credentials are transferred out of band and never read from repository artifacts.
- HappyHorse 1.1 `tiered_expr=0` is not imported. Its 0.5 per-second deployment baseline is explicitly adopted from the 1.0 family baseline and must remain documented as a product/configuration decision.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| Duplicate `(source_channel_id, group)` | Reject manifest |
| Model appears in neither exactly one price map nor one explicit withheld list | Reject manifest |
| Token upstream mapped to a public ModelPrice, or time upstream mapped to a public ModelRatio | Keep route disabled and require a separately priced public alias |
| Wan model on any channel except Rolldek 75 | Reject manifest |
| Viapi action in the wrong media group | Reject manifest |
| Missing credentials or provider evidence | Keep channel disabled; do not claim live availability |

### 5. Good / Base / Bad Cases

- Good: source 50 is represented by image segmentation and vedio processing target identities.
- Base: an empty source mapping for TTS/ASR is discarded while the public model selects the adapter protocol.
- Bad: import source 64 as type 60, copy FastAI Wan aliases, or treat HappyHorse 1.1's zero expression as free.

### 6. Tests Required

- Parse both JSON files and assert 15 target identities, 46 public models, and disjoint 34/12 price maps.
- Assert every model belongs to its channel and exactly one pricing mode or an explicit withheld route.
- Assert Wan appears only on channel 75 and FastAI contains no Wan model.
- Assert Viapi group/action partition and FastAI type 103 mapping.
- After deployment, separately verify database preservation, health, provider smoke, result retrieval, and billing settlement.

### 7. Wrong vs Correct

#### Wrong

```text
copy source channel 50 as one image channel and import source 64 as type 60
```

#### Correct

```text
identity=(50,image)  -> viapi-segment-common
identity=(50,vedio)  -> viapi-super-resolve, viapi-erase-subtitles
source 64/type 60   -> FlowAPI type 103 token routes only
source 75/type 1    -> Wan 3.0 and Wan 3.0 Prime
```
