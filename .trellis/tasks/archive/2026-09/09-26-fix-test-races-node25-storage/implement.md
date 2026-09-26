# Implementation

- [x] Remove per-test `gin.SetMode` calls in the affected relay packages; initialize test mode once per package.
- [x] Synchronize controller task-submission fixture teardown with pending gopool work before restoring globals and closing the test DB.
- [x] Capture video polling test upstream IDs before launching the polling worker; assert against immutable IDs.
- [x] Restore jsdom `localStorage` in Vitest setup for Node 25.
- [x] Run race checks for relay, controller, and service; run the full frontend test suite, typecheck, and lint of changed frontend file.
- [x] Review the diff for production-code changes, update task status, and commit the scoped work (`a61c17b5e`).

## Verification Results

- `go test -race ./relay/... -count=1` passed.
- `go test -race ./controller -count=1` passed.
- `go test -race ./service -count=1` passed.
- `cd web && bun run test` passed: 64 files, 289 tests.
- `cd web && bun run typecheck` passed.
- `cd web && bunx oxlint -c .oxlintrc.json src/test-setup.ts` passed.
- `git diff --check` and `gofmt -l` on changed Go files passed.
