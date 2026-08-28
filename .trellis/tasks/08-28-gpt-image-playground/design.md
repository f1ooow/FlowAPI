# GPT Image Playground - Technical Design

## Status

规划阶段。产品入口已确认使用 `/playground`；结果历史使用当前浏览器 IndexedDB；首版只使用 Image API 的 generations/edits，不恢复旧聊天 Playground 或 `/pg` 协议。

## Architecture Boundaries

### Frontend

- 新增独立 feature 目录 `web/src/features/image-playground/`，避免与正在删除的旧 `web/src/features/playground/` 聊天实现混淆。
- 新增 authenticated route `web/src/routes/_authenticated/playground/index.tsx`，由 `/_authenticated` 既有认证守卫保护。
- 在 `web/src/hooks/use-sidebar-data.ts` 的 `General` 分组增加 `/playground` 入口，使用现有 Hugeicons/Lucide 图标和 i18n；不要恢复旧 Chat 分组。
- 页面复用 `SectionPageLayout` 或等价的现有 `Main` 工作区容器，使用 Flow API 当前浅色语义 token、Public Sans、Tailwind v4 和 Base UI 组件。

### Request Layer

- Token 元数据通过现有 `getApiKeys` / `fetchTokenKey` 读取；列表只展示掩码 Key，真实 Key 仅在提交时按需解析并保存在 React 内存。
- Image 请求不能使用项目 `api` Axios 实例发送，因为其 request interceptor 会把会话 JWT 覆盖到 `Authorization`；必须使用浏览器原生 `fetch`，将所选 API Token 放在 `Authorization: Bearer sk-...`。
- 生成请求：`POST /v1/images/generations`，`Content-Type: application/json`。
- 编辑请求：`POST /v1/images/edits`，`Content-Type: multipart/form-data`，文件字段使用 `image`（多图时可重复 `image[]`），不手动设置 boundary。
- 两条请求都经过当前网关的 TokenAuth、Distribute、模型映射、计费、日志和错误转换；不新增后台 controller、代理路由、数据库表或旁路计费。

### Local History Layer

- 新增轻量 IndexedDB wrapper，数据库名使用 feature 专属前缀，版本从 1 开始，object stores 至少包含 `tasks` 与 `images`。
- `tasks` 保存：任务 ID、`generate|edit` 模式、prompt、非敏感参数、所选 Token ID/名称、固定模型、创建/完成时间、状态、错误文本、输入图片 ID 和输出图片 ID。
- `images` 保存 Blob、MIME、尺寸元数据和角色（input/output）；渲染时用 `URL.createObjectURL`，组件卸载或记录删除时 revoke URL。
- 不保存真实 API Key、JWT、请求头、完整原始响应或可复用的敏感凭据。用户清理站点数据后历史消失，属于已确认的浏览器本地语义。
- IndexedDB 写入失败不能让已经成功的上游结果消失；页面保留当前结果并展示“历史保存失败”的可操作提示。

## Data Flow

```text
user session
  -> getApiKeys (masked metadata)
  -> select token id
  -> fetchTokenKey (full key, memory only)
  -> browser fetch /v1/images/generations or /v1/images/edits
  -> gateway TokenAuth + relay + billing + logs
  -> Image API response data[].b64_json or data[].url
  -> normalize to Blob/object URL for UI
  -> IndexedDB images + task metadata
  -> gallery/detail/re-edit/download
```

## Request Contract

### Common UI state

- `mode`: `generate | edit`.
- `prompt`: required non-empty string; trim before submit.
- `tokenId`: selected user-owned token ID; no model capability probe.
- Fixed `model`: `gpt-image-2` for the first version. A failed model/channel request is surfaced from the gateway rather than pre-validated.
- Basic options only: `size` (`auto`, square, portrait, landscape presets), `quality` (`auto`, `low`, `medium`, `high`), `output_format` (`png`, `jpeg`, `webp`), and bounded `n` (UI maximum 4; backend still enforces `dto.MaxImageN`). Moderation remains `auto` and is not a user-facing safety bypass.

### Generations

```json
{
  "model": "gpt-image-2",
  "prompt": "...",
  "size": "auto",
  "quality": "auto",
  "output_format": "png",
  "n": 1,
  "moderation": "auto"
}
```

Omit optional fields when they are set to the UI default only if doing so keeps compatibility with the gateway and upstream. Explicit `n` is always bounded before sending.

### Edits

Use the same fields in a `FormData` body and append one or more local image Blobs. At least one image is required in edit mode. Do not send a Responses payload, `previous_response_id`, `tools`, or Agent metadata.

## Response Normalization

- Accept the standard `{ data: [{ b64_json, url, revised_prompt, ... }] }` shape.
- Prefer `b64_json` and construct a data URL using the requested output MIME.
- For HTTP `url`, use it for immediate `<img>` display and attempt a same-request `fetch` to materialize a Blob for IndexedDB; if materialization fails, keep the URL as a non-persistent fallback and tell the user that the upstream URL may expire.
- Treat missing/empty `data` as a user-visible error and retain a shortened raw diagnostic only in developer logs, never in local history.
- Parse non-2xx responses from the gateway's OpenAI error envelope (`error.message`) and preserve status categories such as unauthorized, forbidden, insufficient quota, rate limit, moderation, and upstream failure in the UI copy.

## UI Composition

- Header: existing page title and a small mode switch (`Generate` / `Edit`). No Agent tab, settings drawer, or extra product explanation.
- Main workspace: responsive two-region layout. The request composer remains visible while the result gallery occupies the remaining scroll area; on narrow screens it stacks with the composer first.
- Composer: token combobox, prompt textarea, edit-only upload dropzone/preview, compact output controls, primary submit button, and stable loading/error state. The submit button is disabled only for missing required input, missing Token, or an active request.
- Gallery: newest tasks first, with responsive thumbnails, prompt excerpt, mode/model/created time, output count, and icon actions for preview, download, re-edit, and delete. Empty/loading/error states use existing `Empty`, `Skeleton`, and `Alert` primitives.
- Detail dialog: large image preview plus prompt and actual request metadata; actions are download, re-edit, and delete. Do not expose the full API Key or duplicate metadata already shown on the card.
- Re-edit loads the selected stored output into edit mode and focuses the prompt; it creates a new task on submit and does not mutate the original record.

## Security and Privacy

- Never put the resolved API Key in query parameters, route search state, IndexedDB, localStorage, error messages, or analytics payloads.
- Do not log request headers or request bodies containing credentials.
- Token ownership remains enforced by `POST /api/token/:id/key`; a forged token ID cannot retrieve another user's key.
- The selected Token's model limits and status are not probed in advance. Gateway rejection remains the source of truth; the UI must not silently retry with another Token.

## Compatibility and Rollout

- This feature intentionally reuses `/playground` after the concurrent internal simplification task removes the old chat Playground. That task's acceptance criterion must be interpreted as “old chat Playground and `/pg/chat/completions` are gone; the new Image API Playground is present at `/playground`.”
- No database migration is needed. Existing user/token and image relay contracts remain unchanged.
- The route tree is generated by the existing TanStack Router plugin; do not hand-edit generated declarations beyond the normal route generation command.
- If IndexedDB is unavailable (private mode or browser policy), keep an in-memory gallery for the current session and show a non-blocking persistence warning.

## Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Axios session interceptor overwrites the selected API Key | Use a dedicated native `fetch` helper and test the outgoing Authorization header. |
| Upstream returns expiring URLs instead of base64 | Prefer `b64_json`; materialize URL responses to Blob; keep a clear non-persistent fallback. |
| Large images exhaust browser storage | Store Blobs, not duplicated base64 strings; isolate history cleanup and handle quota errors without losing current result. |
| User selects disabled/expired or model-limited Token | Do not preflight capability; send once and show the gateway's actionable error. |
| `/playground` changes collide with simplification WIP | Integrate after reading the latest file state; preserve unrelated deletions and regenerate the route tree. |
| Mobile composer or gallery causes overflow | Use stable grid/aspect-ratio constraints, responsive stacking, keyboard focus checks, and 375px/390px browser validation. |
