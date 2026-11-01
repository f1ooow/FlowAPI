# Kling 4.0 视频调用方法

通过 FlowAPI 调用 `kling-4.0`，提交一次视频生成任务，再查询进度并下载成片。支持文字、图片、视频和音频参考，生成 3–30 秒、720p 或 1080p 视频。

## 1. 接入准备

| 配置项 | 填写内容 |
| --- | --- |
| API 地址 | `https://flowapi.robusta.top` |
| 身份认证 | `Authorization: Bearer <你的 FlowAPI API Key>` |
| 令牌分组 | 创建或编辑令牌时选择 `video` |
| 模型 ID | `kling-4.0`，不添加分辨率后缀 |
| 模型限制 | 如开启令牌模型白名单，加入 `kling-4.0` |
| 输出时长 | `duration` 为 `3` 至 `30` 的整数，单位秒 |
| 分辨率 | `720p` 或 `1080p` |
| 画面比例 | `aspect_ratio`，例如 `16:9`、`9:16`；不限定为这两个比例 |
| 生成数量 | 每次只生成一条视频，`n` 省略或填写 `1` |

提交、查询和下载使用同一个有效的 FlowAPI Key。请求体不需要另传 `group`，不要使用供应商 Key 访问 FlowAPI。Key 仅保存在服务端或本地环境变量中，不要写入网页、公开仓库或 URL。

## 2. 快速开始：提交、查询、下载

以下命令适用于 macOS、Linux 等终端，需要 `curl` 和 Python 3。

### 2.1 配置环境变量

```bash
export FLOWAPI_BASE_URL='https://flowapi.robusta.top'
export FLOWAPI_TOKEN='替换为你的_FlowAPI_API_Key'
```

### 2.2 提交一次任务

```bash
curl --silent --show-error --fail-with-body \
  "${FLOWAPI_BASE_URL}/v1/videos" \
  -H "Authorization: Bearer ${FLOWAPI_TOKEN}" \
  -H 'Content-Type: application/json' \
  --data-binary @- \
  --output kling4-submit.json <<'JSON'
{
  "model": "kling-4.0",
  "prompt": "清晨的海边，柔和暖光，海浪轻拍沙滩，镜头缓慢前进，无文字。",
  "duration": 10,
  "resolution": "720p",
  "aspect_ratio": "16:9",
  "n": 1
}
JSON

cat kling4-submit.json
```

提交成功时的关键字段示意如下；这是结构示例，不是一次实际生成结果：

```json
{
  "id": "task_...",
  "status": "queued"
}
```

HTTP 200 和任务 ID 只表示已受理，不代表视频已生成。保存 FlowAPI 返回的 **`id`**，不要使用上游任务 ID：

```bash
TASK_ID="$(python3 - <<'PY'
import json

with open('kling4-submit.json', encoding='utf-8') as file:
    response = json.load(file)
task_id = response.get('id')
if not isinstance(task_id, str) or not task_id.startswith('task_'):
    raise SystemExit('未返回有效任务 ID，请检查响应，不要直接重复提交。')
print(task_id)
PY
)" && export TASK_ID
```

**提交超时或连接中断时，先核实任务是否已经受理，不要自动重复 POST。** 带多段参考视频的请求可能比纯文本请求等待更久。

### 2.3 查询任务

```bash
curl --silent --show-error --fail-with-body \
  "${FLOWAPI_BASE_URL}/v1/videos/${TASK_ID}" \
  -H "Authorization: Bearer ${FLOWAPI_TOKEN}" \
  --output kling4-result.json

cat kling4-result.json
```

建议每隔 **15 秒** 查询一次。查询暂时失败或进度不变时，继续查询同一个任务，不要重新提交。

| `status` | 处理方式 |
| --- | --- |
| `queued` / `pending` | 排队中，继续查询 |
| `processing` / `in_progress` | 生成中，继续查询 |
| `completed` | 已完成，可以下载 |
| `failed` / `cancelled` | 停止轮询，读取 `error.message` 并保留任务 ID |

### 2.4 下载成片

仅在状态为 `completed` 后执行：

```bash
curl --silent --show-error --fail-with-body \
  --retry 3 --retry-delay 5 \
  "${FLOWAPI_BASE_URL}/v1/videos/${TASK_ID}/content" \
  -H "Authorization: Bearer ${FLOWAPI_TOKEN}" \
  --output kling4.mp4.part && mv kling4.mp4.part kling4.mp4
```

此接口返回视频文件，仍需 Authorization。请由自己的服务端下载或转存，不要把 Key 放进公开下载地址。下载失败时只重试同一任务的 `/content`，不要重新生成。

## 3. 请求参数

使用 **POST `/v1/videos`** 和 `Content-Type: application/json`。参数放在 JSON 顶层，不使用 multipart 上传。

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填，固定为 `kling-4.0` |
| `prompt` | string | 必填，非空的视频描述 |
| `duration` | integer | 必填，3–30 秒；不要同时传入 `seconds` |
| `resolution` | string | 必填，`720p` 或 `1080p`；不要用 `size` 替代 |
| `aspect_ratio` | string | 可选，画面比例；有参考视频时同样支持 |
| `images` | string[] | 可选，最多 10 张图片，每张不超过 10 MB |
| `videos` | string[] | 可选，最多 5 段 MP4 视频，每段不超过 30 秒、50 MB |
| `audios` | string[] | 可选，最多 3 段 MP3/WAV 音频，每段 5–30 秒、不超过 20 MB |
| `n` | integer | 可选，只允许 `1` |

素材使用可直接读取的 HTTP(S) 文件地址，建议 HTTPS；不能依赖登录、Cookie 或额外请求头，不能使用本地路径、内网地址或 data URL。地址和文件内容应在生成期间保持有效且不变。提交前确认文件格式、数量、大小和时长满足上述限制。

优先使用上述标准字段。兼容单图别名 `image` 或 `input_reference`：只有未提供 `images` 时才回退使用，`images: []` 明确表示不使用图片参考。兼容比例别名 `ratio`，但只要提供 `aspect_ratio` 就以它为准，包括显式空字符串。

不要将时长、分辨率或参考素材放入 `metadata`，也不要发送其他模型的 `seconds`、`size`、`input.media` 等字段。本文只使用已明确列出的参数，不使用未验证的模型扩展字段。

## 4. 使用参考素材

将快速开始的 JSON 替换为下面的内容。示例域名仅为占位，必须换成真实可访问的素材文件地址。

### 4.1 图片参考

```json
{
  "model": "kling-4.0",
  "prompt": "保持参考图中的人物外貌和服装，在纯绿色背景前自然讲解，固定镜头，无文字。",
  "images": ["https://example.com/subject.png"],
  "duration": 10,
  "resolution": "1080p",
  "aspect_ratio": "16:9"
}
```

### 4.2 视频与音频参考

```json
{
  "model": "kling-4.0",
  "prompt": "参考视频中的动作节奏与参考音频，保持主体外貌一致，生成连贯自然的画面。",
  "images": ["https://example.com/subject.jpg"],
  "videos": ["https://example.com/motion.mp4"],
  "audios": ["https://example.com/rhythm.wav"],
  "duration": 10,
  "resolution": "720p",
  "aspect_ratio": "9:16"
}
```

参考音频用于输入参考，不代表输出一定带有音轨。没有对应素材时省略该数组，不要填写占位 URL。

## 5. 常见问题

- **401 / 403：** 检查 FlowAPI Key、`video` 分组及模型白名单。
- **400 素材错误：** 检查素材 URL 是否直接返回文件，以及数量、大小、格式和时长是否符合限制。不要把网页链接当作视频文件地址。
- **找不到任务：** 使用提交响应的 FlowAPI `id` 和同一账户的 Key，不要替换为供应商任务编号。
- **查询超时或进度不变：** 继续查询原任务；轮询停止不会取消任务。
- **已完成但下载失败：** 重试该任务的 `/content` 下载，不要再次提交生成。

本文依据 FlowAPI 任务接口与供应商文档整理；示例响应仅说明结构，不代表已完成生产生成验收。
