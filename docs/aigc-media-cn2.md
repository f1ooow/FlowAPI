# AIGC media fetch egress

## Scope

`FETCH_PROXY_ROUTES` configures exact HTTPS origins (port 443) for the protected
media-fetch client. It does not change provider/channel proxies, gateway ingress,
model pricing, or other origins. The initial deployment routes only
`https://aigc.keliagent.cn` through the existing HK-to-CN2 SOCKS listener.

Kling 4 downloads reference images, videos and audio in `ProbeKling4Media` before
submitting to the supplier. An ingress CN2 relay does not cover these outbound
downloads. The supplier still receives the original media URL and must be able
to fetch it independently.

## Configuration and security

```yaml
services:
  app:
    environment:
      FETCH_PROXY_ROUTES: '{"https://aigc.keliagent.cn":"socks5://172.19.0.1:11081"}'
```

- Configuration is read once at startup; malformed origins/proxies fail startup
  without logging proxy credentials. Empty configuration preserves prior behavior.
- HTTPS with omitted port and explicit `:443` are equivalent. There is no suffix,
  wildcard, HTTP, arbitrary-port or cross-origin matching.
- Each routed connection resolves locally, validates all returned IPs, and gives
  SOCKS a validated numeric IP. Public-only DNS checks remain active even when
  global fetch protection or its DNS check is disabled. Other enabled domain/IP/
  port filters still apply. TLS validates the original hostname, even if global
  provider TLS verification is disabled.
- Every redirect is reselected by destination origin and retains existing fetch
  redirect validation. Proxy, TLS or validation failures never fall back to a
  direct connection. Unmatched requests keep their previous transport behavior.
- Both CN2 SSH `PermitOpen` and the key's `authorized_keys` `permitopen` options
  must allow the numeric destination. For the current AIGC
  endpoint this is `39.101.67.52:443`. Existing MiniMax targets stay unchanged;
  no wildcard, public proxy listener, DNS or nginx ingress change is required.
- If the AIGC public IP changes, revalidate the new destination before updating
  the narrow SSH allowlist. A mismatched destination fails closed.

## Verification and rollback

Focused tests cover exact selection, SOCKS destination bytes, TLS bytes and SNI,
redirects, private/mixed DNS, disabled global checks, cancellation, and no fallback:

```sh
GOWORK=off go test ./service ./relay/channel/task/sora ./relay -count=1
```

For production verification, use the existing invalid-signature input URL. A
HEAD returning the application's 404 proves network/TLS reachability only. An
authenticated Kling request referencing that same nonexistent URL must fail with
`invalid_media` / media HTTP 404 before precharge and supplier submission. Neither
check proves valid object download, supplier retrieval, or successful generation.

Before changing SSH/Compose, retain their exact prior files and the running
image. Validate `sshd -t` and Compose configuration before applying. SSH allowlist
changes require a fresh tunnel connection; reconnect only with no current SOCKS
connections. Replace only the FlowAPI app service, not its database/Redis or any
other application. Roll back by restoring the prior Compose file/image and SSH
allowlists (sshd configuration and authorized key), validating both, then
reloading SSH and reconnecting the idle tunnel. Do not change the key material.

Release-specific image identity, checksums, backup paths and observed results
are recorded separately after deployment; source changes alone are not a live
verification result.

## 2026-11-01 deployment evidence

- Source baseline: `8111383be7c707cf70468254d3a523249ce400e8`, task branch
  `fix/aigc-media-cn2`; at deployment, changes were not committed or pushed. The release
  contains a source patch and manifest, rather than claiming an uncommitted
  change is part of the baseline commit.
- Image: `flowapi:hk-aigc-cn2-8111383be-98d64b06-20261101`;
  binary SHA-256: `9a1a010450a4bd9aeddb583d50c3f50c3a98681cf543daad6160f6ce80a96433`.
  Built with Go 1.25.6, linux/amd64, CGO disabled and `greenteagc`; frontend built
  from unchanged baseline sources. Runtime layers reuse the previous live image.
- HK release/rollback directory:
  `/opt/flowapi/releases/hk-aigc-cn2-8111383be-98d64b06-20261101/`.
  It retains `manifest.json`, `source.patch`, `SHA256SUMS`, `deployment.json`, the
  active binary and `docker-compose.before.yml`. Previous image:
  `flowapi:hk-kling4-8111383be-20261101`.
- CN2 rollback directory: `/root/flowapi-aigc-media-cn2-20261101T062235Z/`.
  Both `60-flowapi-egress.conf` and `authorized_keys` are retained; key material,
  source-IP restriction and existing MiniMax destinations are unchanged.
- `sshd -t`, Compose configuration and release checksums passed. The tunnel was
  reconnected only after confirming zero established SOCKS connections. Only
  FlowAPI app was replaced, with a 120-second graceful-stop allowance; a brief
  API interruption occurred during replacement. App is healthy with zero
  restarts, `/api/status` reports the new version, and all other app environment
  entries have the same hash. PostgreSQL, Redis and CCHP were not restarted.
- HK via CN2 reaches the invalid-signature AIGC input with HTTP 404 and TLS in
  about 0.173 seconds. Existing MiniMax endpoint also completes TLS/HTTP through
  its original domain-based allowlist; this is not a paid MiniMax generation test.
- From the actual AIGC Generation Worker, one authenticated validation-only
  `POST /v1/videos` returned HTTP 400 / `invalid_media` /
  `invalid reference video: media download returned HTTP 404` in 915 ms.
  Request ID: `202611010628433187759838268d9d6fHRj0gPO`.
  The bounded `kling-4.0` consume-log window is empty before and after. This
  confirms the deployed gateway reaches the AIGC media HTTP boundary, rather
  than failing at TCP. It does not submit a supplier generation or fetch a real
  OSS object; generation cost is zero.
- Service, Sora task and relay tests passed, as did focused race tests. Independent
  review findings (userinfo route bypass and JSON null configuration) were fixed
  and regression-tested; port `0443` also normalizes to 443. Actual valid signed
  object download, supplier retrieval and full Kling generation remain untested.
- Local build output, temporary release staging and the borrowed dependency
  symlink are removed after verification. The user subsequently authorized branch
  commit/push and PR delivery on 2026-11-01; the PR records the source identity,
  separately from the already-deployed image. Active release and exact rollback artifacts remain
  until a later verified release supersedes this rollback slot.
