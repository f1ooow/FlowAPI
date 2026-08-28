# GPT Image Playground - Implementation Plan

## Preconditions

- Keep the active internal simplification worktree changes intact; inspect overlapping files before each edit.
- Confirm the old chat Playground deletion is still represented by the current route/feature state, then add the new image-only route at the same URL.
- Run the route generator through the existing frontend dev/build tooling rather than hand-authoring `routeTree.gen.ts`.

## Ordered Checklist

### 1. Feature contracts and storage

- [x] Add `web/src/features/image-playground/types.ts` for mode, request parameters, token option, task, image, and normalized response types.
- [x] Add bounded parameter constants and pure request/response normalization helpers.
- [x] Add a small IndexedDB module for task/image CRUD, Blob storage, versioning, and graceful unavailable-storage errors.
- [x] Add unit tests for request payload shape, n bounds, b64/url normalization, OpenAI error extraction, and IndexedDB serialization behavior.

### 2. Gateway request integration

- [x] Add a native-fetch API module that accepts a resolved token key, calls only `/v1/images/generations` or `/v1/images/edits`, supports AbortSignal, and never uses the Axios session instance.
- [x] Implement lazy `fetchTokenKey` resolution through the existing token API; keep the full key in memory only.
- [x] Normalize response data to displayable images and persistable Blobs; preserve partial success when one URL cannot be materialized.
- [x] Add tests that assert Authorization uses the selected API Key, generation uses JSON, edits uses multipart, Responses is never requested, and gateway errors remain actionable.

### 3. Page and interaction UI

- [x] Add the authenticated `/playground` route and export the new `ImagePlayground` feature entry.
- [x] Add the sidebar entry in the existing `General` group and ensure no old Chat navigation is restored.
- [x] Build the token combobox from the current user's token list, including status labels without model probing; handle empty/loading/error states.
- [x] Build mode switching, prompt input, edit-only image upload/preview/removal, compact size/quality/format/count controls, submit/cancel/loading state, and inline/alert errors.
- [x] Build responsive result gallery, task cards, detail dialog, download, delete, and re-edit actions using existing UI primitives and icon conventions.
- [x] Keep visible controls justified by an actual user action; do not add Agent, Responses, complex settings, mask editor, favorites, ZIP export, or server history controls.
- [x] Add i18n keys to `static-keys.ts` and the active `zh.json` locale for all new visible strings; the concurrent simplification task has reduced runtime locales to simplified Chinese.

### 4. Browser history lifecycle

- [x] Hydrate tasks and image Blobs from IndexedDB on page load with skeleton/empty states.
- [x] Save successful tasks and their input/output images after request completion; show a non-blocking warning if storage fails.
- [x] Revoke object URLs on task deletion and component unmount; avoid retaining full API responses in React state after normalization.
- [x] Verify refresh recovery, output-to-edit flow, download, deletion, and private-mode/in-memory fallback.

### 5. Verification and review

- [x] Run focused Vitest tests for the new feature and any touched shared helpers.
- [x] Run `cd web && bun run typecheck`.
- [x] Run focused lint for all touched frontend files; no errors remain.
- [x] Run `cd web && bun run build` and regenerate the typed route tree as required by the build.
- [x] Run `git diff --check`.
- [x] Start the local frontend dev server and use browser validation at 1440x900, 390x844, and 375px widths with deterministic mocked gateway responses.
- [x] Confirm DOM/request evidence: route requires auth, only `/v1/images/generations` or `/v1/images/edits` is called, `/v1/responses` is absent, selected key is sent in Authorization, no key is persisted, and no horizontal overflow exists.
- [x] Run relevant Go relay/request tests; paid live provider calls were not required.

Full `bun run test` was also attempted. It currently fails in unrelated `redemption-codes` and API-key drawer tests because the active simplification worktree's test environment exposes an invalid `localStorage` implementation; the new feature's 7 tests pass independently.

## Validation Commands

```bash
cd web
bun run test -- image-playground
bun run typecheck
bun run lint
bun run build
cd ..
git diff --check
go test ./relay/helper ./relay/channel/openai ./router ./controller
```

Live upstream image generation is intentionally excluded from automated verification. Browser tests should use a deterministic mocked gateway response for success, base64, URL, moderation/error, and quota/error paths; one manual request may be run only when a suitable user-owned Token and provider are available.

## Risky Files / Rollback Points

- `web/src/hooks/use-sidebar-data.ts`: overlaps with the internal simplification's sidebar cleanup; keep only the new image Playground link.
- `web/src/routes/_authenticated/playground/index.tsx`: the old file is deleted by the simplification task; replacement must not reintroduce chat imports.
- `web/src/routeTree.gen.ts`: generated output; regenerate, never manually reconstruct.
- `web/src/features/image-playground/lib/api.ts`: credential boundary; review and test separately.
- `web/src/features/image-playground/lib/storage.ts`: browser persistence and Blob lifecycle; keep isolated so it can be disabled without changing gateway calls.
- `web/src/i18n/locales/*.json`: generated/maintained locale surface; avoid unrelated translation churn.

Rollback can remove the new route/sidebar entry and feature directory without database migration. Existing relay, token, logs, and user data remain untouched.
