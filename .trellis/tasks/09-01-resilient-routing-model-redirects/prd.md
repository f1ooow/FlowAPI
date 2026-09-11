# CCH-style resilient routing and model redirects

## Goal

Make FlowAPI keep a replay-safe user request alive while any compatible, healthy channel remains, with CCH-style per-channel attempts, soft circuit breaking, and ordered per-channel model redirect rules.

## Requirements

### Request failover

- A request owns an ordered routing history and excludes a channel after that channel exhausts its configured attempts.
- A channel's maximum attempt count includes the first attempt and defaults to `2`.
- Selection always uses the highest-priority tier that still contains an eligible channel; weight only chooses among remaining channels in that tier.
- Exhausting channel A must try same-priority channel B before a lower-priority channel C.
- The request continues until one attempt succeeds, no eligible channel remains, the response is committed, the error is not retryable, or the hard safety cap of 20 distinct channels is reached.
- Retryable provider/network failures and non-retryable client, local billing, request cancellation, and validation failures are classified explicitly.
- Existing affinity and Auto-group routing must use the same exclusion semantics rather than bypassing the route state.
- Every selected route recomputes and reserves route-dependent billing before the upstream attempt.
- The request body and original model are restored before every attempt. Multipart image generation and image editing bodies must remain replayable.
- Routing history records channel ID, attempt number, and terminal outcome for diagnosis.

### Supported request scope

- Apply transparent failover to replay-safe synchronous relay requests, including Claude, OpenAI, and Gemini text APIs, synchronous image generation, and synchronous image editing.
- For streaming responses, fail over only before the first valid upstream content has been committed to the client.
- Do not transparently retry WebSocket sessions, video requests, asynchronous task submission, task polling, or a response after client-visible bytes have been committed.

### Channel automatic ban

- Automatic ban is the only temporary channel-health state; there is no separate circuit-breaker state.
- Each channel has a maximum attempt count, consecutive exhausted-request threshold, and automatic-ban duration. Defaults are 2 attempts, 5 consecutive exhausted requests, and 30 minutes.
- All retryable attempts for one channel in one user request count as exactly one failure only when that channel is exhausted. Individual attempts never increment the threshold separately.
- A successful completed request on a channel resets that channel's consecutive failure count. A later channel succeeding does not clear an earlier failed channel's count.
- Client cancellation/499, local or client errors, non-retryable errors, and channel/group probes neither increment nor clear the count.
- Reaching the threshold persists the normal automatic-disabled channel status and an expiry timestamp. The channel is excluded until that timestamp and then automatically restored with its count cleared.
- Manual disable has no expiry timestamp and is never automatically restored.
- Channel automatic ban is controlled by the channel's own automatic-ban switch and reliability settings. It must not depend on or activate the legacy global automatic-disable status-code, keyword, or response-time rules.

### Model eligibility and redirect rules

- A channel supports ordered rules with match types `exact`, `prefix`, `suffix`, `contains`, and `regex`.
- The first matching rule wins and rewrites the immutable user-requested model to the upstream model for the selected channel only.
- A matching redirect source also makes the channel eligible for the original request model, so future client model names such as an Opus 5 identifier can route to a channel that redirects `contains: opus` to `claude-opus-4-6`.
- Switching channels restores the original model and evaluates the new channel's rules independently.
- Existing object-form exact mappings are accepted and normalized to ordered exact rules during migration.
- Invalid, empty, duplicate, overlong, unsafe, or malformed regex rules are rejected by the backend; frontend validation is additional UX only.
- Billing and user-facing request identity continue to use the original model unless an existing billing contract explicitly requires otherwise.

### Configuration experience

- Channel reliability settings use domain labels that distinguish per-channel attempts from request-wide traversal.
- Model rules are edited as ordered rows with match type, user-requested model pattern, and actual upstream model.
- The editor can test a model against the current ordered rules and show the first matching target.
- Existing JSON editing remains available using the new rule-list format.
- All visible text is internationalized.

## Acceptance Criteria

- [ ] With A and B at the same priority and C lower, exhaustion follows `A -> B -> C`; A is never selected again in the same request after exclusion.
- [ ] A channel configured for two attempts can be invoked at most twice for one request, including its first invocation.
- [ ] The router traverses all remaining compatible healthy channels up to the 20-channel safety cap instead of using priority tier as the retry index.
- [ ] Non-retryable errors stop immediately, while retryable pre-commit failures switch according to the route state.
- [ ] Route-dependent billing is prepared for every selected channel and final settlement uses the successful route.
- [ ] Text, synchronous image generation, and synchronous image editing requests restore their bodies across attempts.
- [ ] A streaming failure before valid content can switch channels; a failure after client-visible content does not replay.
- [ ] One channel exhaustion increments the consecutive failure count once regardless of its configured attempt count; a successful request resets it.
- [ ] The configured threshold automatically disables the channel until its ban window expires, while a manually disabled channel never auto-restores.
- [ ] Automatic ban works while the legacy global "disable on failure" switch is off and does not enable its status-code/keyword/response-time rules.
- [ ] `contains: opus -> claude-opus-4-6` makes the channel eligible for a future Opus model and forwards the replacement only to that channel.
- [ ] Exact, prefix, suffix, contains, and regex rules use ordered first-match semantics; channel switching starts again from the original model.
- [ ] Existing exact mapping objects continue to load and are saved as ordered exact rules without losing mappings.
- [ ] The channel UI clearly configures reliability and model rules, supports rule testing, and passes typecheck, affected tests, and production build.
- [ ] Backend route selection, breaker transitions, model matching, retry classification, body replay, and billing route changes have deterministic regression tests.

## Constraints

- Preserve SQLite, MySQL, and PostgreSQL support.
- Preserve current user changes in the dirty worktree and avoid unrelated refactors.
- Do not add transparent retries to operations that may have been accepted upstream without an idempotency contract.
- Do not claim failover after response commitment; that boundary cannot be made transparent at the gateway.

## Reference

- CCH source snapshot: `ding113/claude-code-hub@cbde16562dfd830060270e265ce9421ecc5a6384`.
- Behavioral reference: provider exclusion plus highest-remaining-priority selection, per-provider inner attempts, soft circuit breaker, and ordered model redirects.
