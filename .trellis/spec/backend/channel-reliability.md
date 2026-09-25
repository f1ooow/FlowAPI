# Channel Reliability Settings

## 1. Scope / Trigger

Apply when changing per-channel attempt, automatic-ban or timeout settings:
the `settings.reliability` JSON, `dto.ChannelReliabilitySettings`
(`relaykit/dto/channel_reliability.go`), `Channel.GetReliabilitySettings` and
`Channel.ValidateSettings` (`model/channel.go`), their readers, any writer of
`channels.settings`, or the channel form reliability fields. Failover before
the first content is governed by
[stream-precommit-gate.md](./stream-precommit-gate.md); retry pricing by
[billing-ratio-routing.md](./billing-ratio-routing.md).

## 2. Signatures

Storage: `channels.settings` → `dto.ChannelOtherSettings.Reliability
*ChannelReliabilitySettings` (`json:"reliability,omitempty"`). The
`channels.setting` column (`dto.ChannelSettings`) has no reliability field.

| JSON field | Accepted values | Missing or 0 |
| --- | --- | --- |
| `max_attempts` | 1–10 | 2 |
| `auto_ban_threshold` | 1–100 | 5 |
| `auto_ban_duration_seconds` | 1–86400 | 1800 |
| `streaming_idle_timeout_seconds` | 0 or 60–600 | global `STREAMING_TIMEOUT` (default 300 s) |
| `non_streaming_timeout_seconds` | 0 or 60–1800 | global `RELAY_TIMEOUT` only (default 0 = unlimited) |

- The timeouts are `*int` with `omitempty`; missing and explicit 0 behave the
  same. The form displays and saves 0 and describes it as the global default.
- `Channel.GetReliabilitySettings()` returns
  `GetOtherSettings().Reliability.WithDefaults()`. Readers: per-channel max
  attempts (`controller/relay.go`), auto-ban threshold and duration
  (`model/channel.go`).
- `RelayInfo.ChannelOtherSettings.Reliability` is filled from
  `channel.GetOtherSettings()` (`SetupContextForSelectedChannel` →
  `InitChannelMeta`). Readers: streaming idle timer
  (`relay/helper/stream_scanner.go`), non-streaming deadline
  (`relay/channel/api_request.go`).
- `Channel.ValidateSettings()` parses `settings` into
  `dto.ChannelOtherSettings` and calls `Reliability.Validate()`; channel add,
  update and copy, and the advanced-custom model preview (`FetchModels`), all
  run it.
- Retired key: `first_content_timeout_seconds` belonged to provider racing,
  removed on 2026-09-25. It is no DTO field.

## 3. Contracts

- `settings.reliability` is the only source. Read it through
  `GetReliabilitySettings()` or `ChannelOtherSettings.Reliability`, never from
  `channels.setting`.
- The server validates `settings.reliability` on every save. Out-of-range,
  negative, fractional or wrongly typed values are rejected with an error that
  names the field. Frontend bounds are a convenience, not the guard.
- `reliability` is a typed field of `dto.ChannelOtherSettings`, so every
  `SetOtherSettings` rewrite (upstream model update check, detect and apply;
  the unsaved advanced-custom model preview) keeps the known fields. Such
  rewrites drop unknown sub-keys; admin form saves keep them, because the form
  spreads the stored object before writing its own fields.
- Max attempts counts attempts on one channel, including the first, before the
  serial router excludes it and selects another. Paths without transparent
  failover (Realtime, audio, specific-channel requests) use 1.
- Automatic ban: with the channel's auto-ban switch on, a request whose
  retryable attempts on the channel are exhausted adds one consecutive failure.
  Reaching `auto_ban_threshold` auto-disables the channel for
  `auto_ban_duration_seconds`.
- Streaming idle timeout: a positive channel value wins; 0 or missing falls
  back to `STREAMING_TIMEOUT` (`constant.StreamingTimeout`); when neither is
  positive there is no timer. The timer starts after the upstream response
  headers and resets on every read that returns bytes. Before commit it fails
  the attempt with a retryable `idle_timeout` (transparent failover); after
  commit it ends the stream (`EndReason=timeout`) and nothing is replayed.
- Non-streaming timeout: a positive channel value bounds the whole upstream
  request, headers and body; `deadlineResponseBody` releases it on close. 0 or
  missing adds no channel deadline and leaves the global `RELAY_TIMEOUT` HTTP
  client timeout, which bounds every relay request and also applies next to a
  channel value.
- A stored `first_content_timeout_seconds` is ignored on read and validation,
  kept by form saves and dropped by `SetOtherSettings` rewrites. No data
  migration.

## 4. Validation & Error Matrix

| Condition | Behavior |
| --- | --- |
| `reliability` missing | 2 attempts, threshold 5, 1800 s ban, global timeouts |
| A field missing or 0 | That field's default; timeouts use the global default |
| `max_attempts` 11, threshold 101, duration 86401 | Reject save |
| Timeout negative, fractional, 1–59 or above its maximum | Reject save |
| Wrong JSON type, e.g. a string | Reject save. If already stored, `GetOtherSettings` resets the whole `settings` column (existing behavior) |
| Reliability stored in `channels.setting` | Ignored |
| `first_content_timeout_seconds` present | Ignored; save allowed |
| Idle timeout before commit | 502 `idle_timeout`, retryable |
| Idle timeout after commit | Stream ends; no error, no replay |
| Non-streaming deadline reached, headers or body | Request fails with a deadline error |

## 5. Good / Base / Bad Cases

- Base: every timeout is 0. Streams use `STREAMING_TIMEOUT` (300 s unless the
  environment changes it); non-streaming requests are bounded only by
  `RELAY_TIMEOUT` (unset in production, so unlimited).
- Good: a channel saves `max_attempts: 3`; a retryable failure is retried twice
  more on that channel before the router moves on.
- Good: an upstream sends `message_start` and then hangs; the idle timeout
  fails the attempt over and the client receives one answer from another
  channel.
- Bad: reading reliability from `channels.setting`. From 09-11 to 09-25 the
  form saved `settings.reliability` while the backend read `setting`, so saved
  values never applied (#26 and #67 stopped after 2 attempts despite
  `max_attempts: 3`).
- Bad: treating 0 as "no idle timer". From 09-18 to 09-25 `STREAMING_TIMEOUT`
  was unread and no channel set a value, so no gateway timer bounded a silent
  stream.
- Bad: re-marshalling `settings` through a struct without `Reliability`; this
  most likely deleted the saved `max_attempts: 3` of #27 and #28.

## 6. Tests Required

- `model/channel_settings_test.go`: values come from `settings.reliability`,
  missing fields default, the legacy `setting` column and the retired key are
  ignored; out-of-range and fractional values are rejected on save.
- `controller/channel_upstream_update_test.go`: the scheduled upstream model
  check keeps `reliability`.
- `controller/relay_test.go`: a retryable failure uses every saved attempt.
- `model/channel_auto_ban_test.go`, `service/channel_auto_ban_test.go`:
  threshold and duration.
- `relay/helper/stream_scanner_test.go`: a channel value wins; 0 or missing
  falls back to the global value; idle before commit is retryable; idle after
  commit ends without replay. Tests that change `constant.StreamingTimeout`
  must not run in parallel.
- `relay/channel/deadline_body_test.go`: the non-streaming deadline covers the
  body.
- `relaykit/dto/channel_reliability_test.go`: bounds. Any relaykit change also
  requires `cd relaykit && GOWORK=off go build ./...`.
- Frontend: `channel-reliability-form.test.ts` (load, save, unknown keys kept,
  bounds) and `channel-reliability-fields.test.tsx` (0 described as the global
  default).

## 7. Wrong vs Correct

### Wrong

```go
setting := channel.GetSetting() // channels.setting: the form never writes here
if setting.Reliability != nil { /* ... */ }

if s := info.ChannelOtherSettings.Reliability; s != nil && s.StreamingIdleTimeoutSeconds != nil {
	streamingTimeout = time.Duration(*s.StreamingIdleTimeoutSeconds) * time.Second // 0 disables the timer
}
```

### Correct

```go
settings := channel.GetReliabilitySettings() // settings.reliability with defaults

streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second
if s := info.ChannelOtherSettings.Reliability; s != nil && s.StreamingIdleTimeoutSeconds != nil && *s.StreamingIdleTimeoutSeconds > 0 {
	streamingTimeout = time.Duration(*s.StreamingIdleTimeoutSeconds) * time.Second
}
```
