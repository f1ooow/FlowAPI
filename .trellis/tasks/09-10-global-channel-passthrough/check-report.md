# Final Passthrough Review

Reviewed on 2026-09-11 against the task PRD, design, implementation plan and global passthrough contract.

## Findings Fixed

- `.trellis/spec/backend/global-passthrough.md`: corrected the error matrix, which claimed non-string explicit header values always fail. The existing sanitizer converts values to strings before resolution. Also clarified that the controller regression verifies registered option updates and response shape, not database persistence itself.
- No production code defects found in the scoped implementation.

## Behavior Reviewed

- The new global header flag defaults to false and participates in the existing registered option export/update mechanism. Body and header options have independent UI fields and save keys.
- Global header inheritance adds a wildcard to a fresh sanitized map, preserving stored channel maps and explicit channel values. Runtime override maps remain final, and channel tests do not automatically forward administrator request headers.
- Shared request dispatch applies adapter setup first, automatic forwarding next, and explicit overrides last. Automatic rules protect common credentials, cookies, transport headers, content type and Codex account identity; explicit administrator values remain supported.
- The channel settings endpoint is behind AdminAuth and ChannelRead and exposes only two boolean flags. Successful option saves invalidate the shared inheritance query.
- New and existing forms derive effective body state without overwriting local preference. The inherited switch is displayed on and disabled; table/card indicators include global state.
- Active frontend localization is Chinese only, matching the current runtime configuration.

## Verification

- Fresh `bun run typecheck`: PASS, exit 0.
- Fresh scoped `bunx oxlint -c .oxlintrc.json` across 11 passthrough TypeScript/TSX files: PASS, exit 0.
- Earlier implementation verification retained: `go test ./relay/channel ./relay/channel/codex ./relay/common ./controller ./router` PASS; six focused frontend tests PASS; scoped formatting and active locale checks PASS. No production edits were made during final review, so these successful suites were not repeated.
- Main session reported production `bun run build`: PASS.
- Main session reported browser checks at 1440x1000 and 390x844: independent header false/true save payloads, inherited body checked and disabled, both inheritance labels and no document horizontal overflow; screenshots visually reviewed.

## Limits / Out Of Scope

- Existing ChannelAuthSection paragraph/div nesting warning remains outside this change.
- Body passthrough can bypass body conversion, model rewriting, parameter overrides and system prompt injection.
- Header inheritance covers shared forwarding machinery; DoTaskApiRequest and bespoke transports are not universally covered.
- Automatic exclusions cannot recognize every custom secret. Ordinary headers can still replace adapter defaults; cache hits are provider-dependent.
- No channel opt-out or wildcard exclusion semantics were added. No production configuration changes, deployment, commits or pushes were performed.
