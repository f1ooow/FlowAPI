# Design

## Boundaries

All fixes stay in Go test files or the Vitest setup. Each Go test package has its own process, so Gin test mode can be initialized once per affected package before tests run. Individual tests must not write Gin's global mode, including cleanup callbacks that restore it.

The controller fixture waits for the default gopool workers it caused to finish before restoring global flags and closing its SQLite connection. `gopool.WorkerCount()` reaches zero only after queued callbacks return; use a bounded assertion so a stuck callback fails the test instead of hanging teardown.

The video polling test captures upstream ID strings before launching the worker and uses those immutable values for map keys and assertions. The production goroutine continues updating its Task objects as before.

Vitest already creates a jsdom instance and exposes it as `jsdom` in setup files. Override Node 25's `globalThis.localStorage` property with `jsdom.window.localStorage` during setup. This keeps browser-like storage semantics and avoids an OS-specific `--localstorage-file` path.

## Compatibility And Risks

- Gin mode is process-global, so package-level setup must cover every mode write in the affected package; otherwise parallel tests can still race.
- gopool's worker count is global. Controller tests are non-parallel; the bounded wait can reveal unrelated long-running workers instead of racing through fixture cleanup.
- Vitest's jsdom global is specific to the configured `jsdom` environment. The setup file is already tied to DOM tests, and the type-only `vitest/jsdom` declaration carries no runtime import.
- No database migration, API contract, or production deployment is involved. Revert the test-only edits if validation fails.
