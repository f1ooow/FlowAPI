# Implementation Plan

- [x] Recover unfinished implementation and inspect current paths.
- [x] Create task after consent and record converged requirements/design.
- [x] Backend worker: finish header protections and final-request tests; verify registration and permissions. Five requested Go package suites passed.
- [x] Frontend worker: finish inherited state, translations and UI tests. Six focused tests and changed-file lint/format passed; active locale is zh only. Final frontend typecheck passed on 2026-09-11 after concurrent monitoring work stabilized.
- [x] Main: inspect transport coverage and update project contract.
- [x] Main: complete production build and browser inspection. Production build passed. Settings save toggles and channel edit inheritance passed against mocked APIs at 1440×1000 and 390×844; screenshots inspected and no horizontal document overflow found.
- [x] Independent check agent: full scoped review passed; fresh typecheck and 11-file lint passed; spec wording corrected.
- [x] Record results/limits and finish without deployment.

## Verification

- Backend: go test ./relay/channel ./relay/channel/codex ./relay/common ./controller ./router; narrow only for documented unrelated failures.
- Frontend: focused Vitest passthrough tests, bun run typecheck, changed-file oxlint/format, bun run build.
- Browser: desktop/mobile settings and inheritance against local mocked APIs or local runtime without production credentials.
- relaykit is outside scope; if its APIs change run GOWORK=off go build ./... inside relaykit.

## Baseline And Coordination

Before task creation, global header tests, controller settings test and frontend save test passed. Actual HTTP header regression tests are now passing. Shared files contain unrelated changes; use targeted edits and never revert other work. User initially requested uncommitted work, then explicitly authorized committing this feature on 2026-09-11. Commit only passthrough changes; preserve unrelated work.

## Browser Verification (2026-09-11)

Local Rsbuild UI with mocked `/api/**` responses; no production credentials or traffic. The browser captured independent header-option PUTs for false/true and confirmed a channel with local body false renders the inherited switch checked and disabled. Both body/header inherited labels render. Temporary script/screenshots are under `/tmp/flowapi-passthrough/` (ephemeral). The existing unrelated `ChannelAuthSection` paragraph/div nesting warning remains; there were no passthrough runtime errors.
