# Verification budget and smoke plan

This is a review artifact. No credentials, production writes, or paid provider calls were used.

| Family | Minimum smoke | Cost cap per isolated token | Evidence |
|---|---|---:|---|
| Synchronous image/audio/Viapi/CV | one request per protocol and one invalid-input case | 0.50 CNY | HTTP response, media retrieval, usage/log, wallet delta |
| Async video/Hailuo/Seedance | one submit + poll + duplicate poll; one rejected request | 2.00 CNY | task status, final media, immutable pricing snapshot |
| VOD/ASR | one short media fixture and one duration-boundary case | 0.50 CNY | upload/signature, duration, settlement |
| MiniMax file/clone/music | upload (free), then one clone/music call | 1.00 CNY | resource ownership, account affinity, per-call settlement |
| FastAI Seedance aliases | one explicit time model; token alias separately without seconds multiplier | 2.00 CNY | upstream model, billing unit, quota |
| Rolldek Wan | one shortest supported output, one recovery lookup | 2.00 CNY | durable submission, no duplicate paid POST, output seconds |

Before real execution, create a dedicated enabled group and token, set hard wallet/subscription limits below the caps, and approve the exact provider credentials. Unknown accepted submissions must never be retried automatically. A failed or unverified smoke leaves the channel disabled.

## Alias dispositions

- `claude-opus-4-5`: keep only if Flow's upstream mapping is confirmed; otherwise hold.
- `claude-sonnet-4-5`: keep only if Flow's upstream mapping is confirmed; otherwise hold.
- `gpt-5.2`: keep only if Flow's upstream mapping is confirmed; otherwise hold.

These are historical text aliases, not justification for importing Keli channels that already route into FlowAPI.
