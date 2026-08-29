# GPT Image Playground Reference Alignment - Technical Design

## Status

Planning. This revision follows the deployed `9f131ab7` implementation and keeps `/playground` as the product route. It changes the interaction/state model without restoring the removed chat Playground.

## Product Architecture

### Unified request composer

- Remove the visible `generate/edit` tabs and the `mode` state from user-facing composition.
- Keep one prompt composer with a paperclip/upload affordance and an input image strip. Input images can arrive from file picker, drag/drop, clipboard paste, or a generated output's edit action.
- Choose the endpoint from the input image set at submit time: empty set -> `/v1/images/generations`; non-empty set -> `/v1/images/edits`.
- Keep the selected user token and basic image parameters in composer state. Reuse-config restores prompt, parameters, token selection when the token still exists, and the saved input images/mask.

### Task lifecycle

Create a task before resolving the API key or making the network request:

```text
draft -> running card -> done card | error card
```

- `running` stores prompt, requested parameters, token label, start time and no output images. It is inserted at the top of the gallery immediately.
- `done` replaces the same task record with output images, actual response metadata, elapsed time and completion time.
- `error` preserves the prompt, input images, error message, elapsed time and retry action. It must not auto-open a detail view.
- `AbortController` is associated with task ID. Stopping a request updates the card to an explicit interrupted/error state rather than removing it.
- Requests may run concurrently; each task owns its controller and completion update. The composer remains available while other cards are running.

## Data Model

Extend the current local types without introducing a server schema:

- `PlaygroundTask.status`: `running | done | error`.
- `PlaygroundTask.operation`: internal `generation | edit` derived from whether input images existed at submit; never render as a required mode choice.
- `PlaygroundTask.inputImages`, `mask`, `outputImages`, `params`, `tokenId`, `tokenName`, `createdAt`, `startedAt`, `completedAt`, `elapsed`, `error`, `actualParams`, and optional `revisedPrompt`.
- `StoredTask` stores only non-sensitive metadata and image IDs. It must not store the resolved API key, session JWT, headers, raw response, or hidden prompt-injection material.
- `StoredImage` stores Blob/MIME/role/createdAt. Keep source URLs only as a best-effort fallback when Blob materialization fails.

Persist the running task before the request only in React state; do not persist an in-flight task to IndexedDB until it reaches `done` or `error`, avoiding stale forever-running records after refresh.

## Gallery and Details

- Gallery is the primary page content, newest task first, with a 3-column desktop layout and responsive single/two-column narrow layout.
- The visual baseline is the user's supplied light-theme reference screenshots. Reuse its visual language, not its overall page geometry: white/light canvas, soft neutral borders, compact grey parameter chips, split media/info cards, restrained blue action accent, and clear icon operations. Keep Flow API's approved left composer / right gallery layout so the product remains distinct from the reference layout. This is visual/workflow adaptation, not source-code or brand copying.
- Each card follows the reference split composition: fixed media pane on the left, information pane on the right. Use stable aspect ratio and dimensions so loading/content transitions do not resize the grid.
- Running cards show spinner/progress placeholder and elapsed timer while retaining prompt and requested parameter tags.
- Done cards show the first output as cover, a count badge for additional outputs, ratio/size/format/quality/count tags, prompt excerpt and action buttons.
- Error cards show error state, preserved prompt/parameters and retry/delete actions.
- Card controls: preview/details, favorite is out of scope, reuse configuration, edit output, download one/all, delete. Every icon-only action has an accessible label and stops card click propagation.
- Clicking a card/image opens the existing-style detail overlay only on explicit user action. Completion never opens it automatically.
- Detail overlay displays all output slots with next/previous or grid navigation, prompt, actual parameters, source token name, elapsed/time, and actions for reuse config, edit output, download, delete. It never displays the secret token.

## Clipboard and Context Menu

- Register a document-level `paste` listener while the route is mounted. Extract `image/*` clipboard items, convert them to File/Blob, add to the input image strip, and prevent default only when an image was accepted.
- Preserve normal text paste into the prompt textarea. Do not intercept clipboard text globally.
- Add a document-level `contextmenu` handler for result/reference `<img>` elements. Render a positioned menu with Copy, Download, Download all (when applicable), and Edit/add as reference.
- Copy uses `navigator.clipboard.write([new ClipboardItem({ [mime]: blob })])` when supported. If the source is a URL, first fetch/materialize it; on failure show an error, never a false success.
- Menu closes on pointer down outside, scroll, resize or Escape. The handler must keep iOS native long-press behavior where image clipboard APIs are unavailable.

## Reference Preview and Mask Editor

- Input image thumbnails in the composer are clickable and open a preview/lightbox-like overlay. The preview contains an explicit “Edit image” action.
- “Edit image” selects that image as the mask target and opens an independent full-screen overlay root, not a node-local resize or inline drawer.
- Reuse the reference project's proven canvas interaction shape: source canvas, white mask canvas, blue transparent preview, brush/eraser, brush-size control, undo/redo, clear, save/cancel, Escape close, pointer/touch handling and reduced-motion-safe transitions.
- Before canvas setup, call a local `prepareMaskTargetDataUrl` equivalent:
  - decode the image and determine natural dimensions;
  - cap the longest edge at 1920px;
  - floor each edge to a multiple of 16 while keeping at least 16px;
  - convert the working target to PNG when resized or source format is not PNG;
  - use the same working dimensions for source and mask.
- On save, persist the normalized working target and mask, replace the original target input image while preserving input order, then close the overlay. The next edit request sends the working target as the first `image` and the mask as `mask` in multipart form data.
- Surface a non-blocking notice when the image was resized, including original and working dimensions. A malformed/mismatched mask must be rejected before network submission.

## Request and Response Boundaries

- Continue resolving user-owned tokens through `/api/token/` and `/api/token/:id/key`; do not add model probing.
- Use native `fetch` for Image API calls so the selected API Key remains in the `Authorization` header rather than being replaced by the session Axios interceptor.
- Generation body remains JSON. Edit body remains `FormData` with repeated `image`/`image[]` files and optional `mask`; never set the multipart boundary manually.
- Normalize Base64 and URL responses into Blob/object URL records. Preserve partial output slots and revised prompts. Do not fetch arbitrary URLs without validating `http(s)` and do not leak URL contents into error text.
- For response failures, keep the gateway's actionable message and status category. For moderation/quota/auth errors, do not retry automatically or switch tokens.

## Homepage Scope

- Replace the current custom “Internal AI API infrastructure” default branch with the pre-existing New API native `Hero`/`HeroTerminalDemo` composition (or restore its equivalent from the known-good repository revision).
- Retain the existing custom `HomePageContent` URL/HTML/Markdown override branch unchanged.
- Render only the native Hero and its terminal demo in the default branch, sized to one screen. Remove `Stats`, `Features`, `HowItWorks`, `CTA`, long `Footer`, internal-team labels and duplicate marketing sections. Do not invent new artwork, colors, taglines or page sections.
- Keep existing public header/auth behavior unless required by the native hero composition; do not expose the English Playground name on the homepage.

## Compatibility / Rollout

- No backend API or database migration is expected. Existing Image Relay, token ownership, billing, logs and `/v1` routes remain authoritative.
- Keep `/playground` authenticated and keep `/pg/chat/completions` absent.
- Generated TanStack route declarations must be regenerated by the existing build plugin.
- Browser storage migration must accept tasks written by the prior simplified version. Map prior `mode` to internal `operation`, default missing status to `done`, and preserve any available input/output images.
- Rollback is a frontend-only revert to `9f131ab7`; no server data migration is needed.

## Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Running card disappears or result opens unexpectedly | Create/update a task by stable ID and separate card click from completion callback. |
| User confusion from visible mode selector | Derive endpoint exclusively from input image presence; keep copy “添加参考图” instead. |
| Clipboard permission/CORS failure | Materialize Blob when possible, handle permission errors explicitly, preserve native fallback. |
| Mask dimensions differ after preprocessing | Store working dimensions with both image and mask; validate equality before FormData construction. |
| Browser storage fills with large outputs | Persist Blobs once, dedupe by image ID, catch quota errors without discarding in-memory task. |
| Homepage reintroduction conflicts with simplification task | Update that task's homepage acceptance to “native New API hero, one screen, sections deleted”; preserve custom content branch. |
