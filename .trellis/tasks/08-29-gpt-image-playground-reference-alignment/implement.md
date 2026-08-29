# GPT Image Playground Reference Alignment - Implementation Plan

## Preconditions

- Work from the clean deployed baseline at `941298f6` / `9f131ab7` and keep unrelated active task changes intact.
- Read the current `image-playground` source before edits; do not copy the reference project's source files.
- Update the internal simplification task's homepage acceptance/documentation when its old “internal one-screen” wording conflicts with the new user decision.

## Ordered Checklist

### 1. State and persistence model

- [ ] Replace visible generate/edit mode state with one composer input-image state; retain an internal operation field only for request/history metadata.
- [ ] Add `running | done | error` task lifecycle and stable task-ID updates.
- [ ] Persist only terminal task bundles to IndexedDB; add migration/default handling for tasks from `9f131ab7`.
- [ ] Add tests for immediate running insertion, success/error update, cancellation, concurrent task isolation and refresh hydration.

### 2. Unified composer and inputs

- [ ] Remove mode tabs and edit-only conditional blocks.
- [ ] Add a persistent attachment affordance and reference-image strip available in the same composer for every request.
- [ ] Support file picker, drag/drop and document clipboard image paste without breaking text paste.
- [ ] Add preview action for each reference image, remove/clear controls and stable input ordering.
- [ ] Add tests for generation vs edits endpoint selection based solely on image presence and paste/file input behavior.

### 3. Request lifecycle and endpoint integration

- [ ] Refactor the native-fetch API module to accept an optional mask and to keep generations JSON / edits multipart contracts.
- [ ] Ensure `n`, size, quality, format and moderation remain bounded/normalized by existing gateway contracts.
- [ ] Surface actionable gateway errors without automatic token fallback; attach response metadata to the terminal task.
- [ ] Test authorization, endpoint/body shape, mask field ordering, URL/Base64 normalization and no Responses calls.

### 4. Reference-style gallery and details

- [ ] Rebuild gallery cards into stable split media/info cards with running/error/done visual states and elapsed timers.
- [ ] Keep composer and gallery visible together; do not replace gallery with a result-only loading screen.
- [ ] Remove automatic detail opening after success.
- [ ] Implement explicit detail overlay with all outputs, prompt, actual parameters, token name, reuse config, edit, delete and download actions.
- [ ] Implement single/all downloads and card action event isolation.
- [ ] Add component tests for card states, no auto-open, all-output detail, action accessibility and responsive layout contracts.

### 5. Clipboard context menu and edit flow

- [ ] Add right-click menu for output/reference images with Copy, Download, Download all and Edit/add reference actions.
- [ ] Implement clipboard image write with permission/CORS failure handling and tests using mocked Clipboard API.
- [ ] Implement output-to-reference reuse and “复用配置” restoration, including selected token when still owned/available.
- [ ] Keep the original task immutable when creating a new edit task.

### 6. Mask editing

- [ ] Add mask preprocessing utility with max-edge 1920, 16px multiple flooring, PNG conversion and explicit original/working dimensions.
- [ ] Add unit tests for square, portrait, landscape, oversized and non-PNG inputs plus mask/source dimension equality.
- [ ] Add independent full-screen mask editor overlay with brush/eraser, undo/redo, clear, brush size, preview, save/cancel and keyboard/touch behavior.
- [ ] Wire preview “编辑图片” to the overlay and save normalized target + mask into composer state and IndexedDB.
- [ ] Add edits request tests asserting `image` and `mask` are sent with matching dimensions.

### 7. Homepage correction

- [ ] Restore the known-good native New API `Hero`/`HeroTerminalDemo` default homepage composition.
- [ ] Delete only the default `Stats`, `Features`, `HowItWorks`, `CTA`, long `Footer` and “Internal AI API infrastructure” content; do not redesign the hero.
- [ ] Preserve custom URL/HTML/Markdown homepage precedence and its isolation behavior.
- [ ] Add/update homepage tests for native hero presence, removed sections, one-screen structure and custom-content override.

### 8. Verification and deployment

- [ ] Run focused feature tests and homepage tests.
- [ ] Run `cd web && bun run typecheck`.
- [ ] Run focused `oxlint` and `oxfmt --check` for touched files.
- [ ] Run `cd web && bun run build`; regenerate `routeTree.gen.ts` through the build plugin.
- [ ] Run `go test ./relay/helper ./relay/channel/openai ./router ./controller`.
- [ ] Run `git diff --check`.
- [ ] Start local frontend and use browser validation at 1440x900, 1280x720, 390x844 and 375x844.
- [ ] Verify with mock network: running card appears before response; success updates same card; no automatic dialog; generation/edits endpoint selection; paste/right-click copy; mask save/preprocessing; details and all outputs; homepage sections removed.
- [ ] Build and deploy a new `linux/amd64` HK image only after tests pass; back up PostgreSQL/Compose, recreate app only, confirm container health, public `/api/status`, `/playground`, and unauthenticated image endpoint behavior.

## Validation Commands

```bash
cd web
bun run test -- src/features/image-playground src/features/home
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/image-playground src/features/home src/routes/_authenticated/playground/index.tsx src/hooks/use-sidebar-data.ts
bunx oxfmt --check src/features/image-playground src/features/home src/routes/_authenticated/playground/index.tsx src/hooks/use-sidebar-data.ts
bun run build
cd ..
go test ./relay/helper ./relay/channel/openai ./router ./controller
git diff --check
```

Paid upstream image calls remain excluded from automated verification. Browser tests use deterministic generations/edits responses and inspect request bodies/headers.

## Risky Files / Rollback Points

- `web/src/features/image-playground/components/image-playground.tsx`: full gallery/composer/details interaction rewrite.
- `web/src/features/image-playground/hooks/use-image-playground.ts`: task lifecycle, concurrency, clipboard/persistence coordination.
- `web/src/features/image-playground/lib/api.ts`: selected-key credential boundary and multipart mask contract.
- `web/src/features/image-playground/lib/storage.ts`: IndexedDB migration and Blob lifecycle.
- New `web/src/features/image-playground/lib/mask-preprocess.ts` and `mask-editor.tsx`: official image dimension handling and canvas state.
- `web/src/features/home/index.tsx`: restore native hero while preserving custom home override.
- `web/src/features/home/components/*`: restore only existing native hero dependencies if missing; avoid new design work.
- `web/src/routeTree.gen.ts`: generated output, never hand-reconstruct.
- `web/src/i18n/locales/zh.json`: write only through the sanctioned i18n script.

Rollback is a frontend/app image revert to the previous deployed tag; no database rollback is required.
