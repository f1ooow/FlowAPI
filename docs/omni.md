# Gemini Omni 视频调用方法

通过 FlowAPI 调用 `gemini-omni-1.1-flash` 生成视频。先提交一次任务，保存返回的 `id`，再查询进度并下载成片；支持文字提示词及图片、视频、音频参考素材。

## 1. 接入准备

| 配置项 | 填写内容 |
| --- | --- |
| API 地址 | `https://flowapi.robusta.top` |
| 身份认证 | `Authorization: Bearer <你的 FlowAPI API Key>` |
| 令牌分组 | 创建或编辑令牌时选择 `video` |
| 模型 ID | `gemini-omni-1.1-flash`，大小写与拼写保持一致 |
| 模型限制 | 如开启令牌模型白名单，加入 `gemini-omni-1.1-flash` |
| 视频规格 | 720p；时长为 `4`、`6`、`8`、`10` 秒 |
| 生成数量 | 每次请求固定 `n: 1` |

提交、查询和下载使用同一个有效的 FlowAPI Key。请求体不用另传 `group`，也不需要任何上游供应商的 Key。API Key 只保存在服务端或本地环境变量，不要写入前端网页、公开仓库或 URL。

## 2. 价格与计费

当前 `video` 分组标准价格为 **人民币 0.20 元 / 秒**，按请求中的 `duration` 计算，不是每次请求固定 0.20 元。

| `duration` | 一条视频费用 |
| --- | --- |
| `4` | ¥0.80 |
| `6` | ¥1.20 |
| `8` | ¥1.60 |
| `10` | ¥2.00 |

以上为每次生成一条、720p、无额外账户折扣时的价格。提交时预扣，任务被后台确认失败后退回对应预扣额度；查询已有任务不重复收取生成费。

部分价格目录可能仍将该模型单位显示为“次”，但当前视频接口实际按上述秒数计费。不要把目录中的 `0.20` 当作整条视频价格。

## 3. 快速开始：提交、查询、下载

以下命令适用于 macOS、Linux 等终端，需要 `curl` 和 Python 3。请求参数全部放在 JSON 顶层，时长使用 `duration`，不要同时传 `seconds`，也不要把参数嵌套到 `metadata`。

### 3.1 配置环境变量

```bash
export FLOWAPI_BASE_URL='https://flowapi.robusta.top'
export FLOWAPI_TOKEN='替换为你的_FlowAPI_API_Key'
```

### 3.2 提交一次任务

```bash
curl --silent --show-error --fail-with-body \
  "${FLOWAPI_BASE_URL}/v1/videos" \
  -H "Authorization: Bearer ${FLOWAPI_TOKEN}" \
  -H 'Content-Type: application/json' \
  --data-binary @- \
  --output omni-submit.json <<'JSON'
{
  "model": "gemini-omni-1.1-flash",
  "prompt": "清晨的海边，镜头缓慢推进，海浪轻拍沙滩，自然光，画面平稳。",
  "duration": 4,
  "resolution": "720p",
  "aspect_ratio": "16:9",
  "n": 1
}
JSON

cat omni-submit.json
```

提交成功时，关键字段示意如下；这不是一份实际生成结果，响应可能包含其他字段：

```json
{
  "id": "task_...",
  "status": "queued"
}
```

HTTP 200 和任务 ID 只表示已受理，不代表生成完成。保存返回的 **`id`**，后续始终使用它，不依赖其他 `task_id` 字段：

```bash
TASK_ID="$(python3 - <<'PY'
import json

with open('omni-submit.json', encoding='utf-8') as file:
    response = json.load(file)
task_id = response.get('id')
if not isinstance(task_id, str) or not task_id.startswith('task_'):
    raise SystemExit('未返回有效任务 ID，请检查响应，不要直接重复提交。')
print(task_id)
PY
)" && export TASK_ID

printf '任务 ID：%s\n' "${TASK_ID}"
```

**提交超时或连接中断，不代表任务没有创建。先保留响应和请求记录，核实是否已经受理；不要自动重复 POST，以免重复生成、重复计费。**

### 3.3 查询任务

```bash
curl --silent --show-error --fail-with-body \
  "${FLOWAPI_BASE_URL}/v1/videos/${TASK_ID}" \
  -H "Authorization: Bearer ${FLOWAPI_TOKEN}" \
  --output omni-result.json

cat omni-result.json
```

建议每隔 **15 秒** 查询一次。接口返回后台轮询保存的任务快照，进度可能短暂不变，不要因此重新提交。

| `status` | 处理方式 |
| --- | --- |
| `queued` / `pending` | 排队中，继续查询原任务 |
| `processing` / `in_progress` | 生成中，继续查询原任务 |
| `completed` | 生成完成，可以下载 |
| `failed` / `cancelled` | 已失败或取消，停止轮询，读取 `error.message` |

完成状态的关键字段示意：

```json
{
  "id": "task_...",
  "status": "completed"
}
```

不要假定响应一定带有 `duration`、`seconds`、`video_url` 或 `metadata.url`。下载统一使用下一节的 FlowAPI `/content` 接口。

### 3.4 下载成片

仅在 `status` 为 `completed` 后执行：

```bash
curl --silent --show-error --fail-with-body \
  --retry 3 --retry-delay 5 \
  "${FLOWAPI_BASE_URL}/v1/videos/${TASK_ID}/content" \
  -H "Authorization: Bearer ${FLOWAPI_TOKEN}" \
  --output omni.mp4.part && mv omni.mp4.part omni.mp4
```

此接口返回视频二进制内容，仍需携带 Authorization，不是可直接粘贴到浏览器的公开文件地址。请由自己的服务端下载并转存，不要将 API Key 拼到地址中。下载临时失败时重试同一任务的下载接口，不要重新生成视频。

## 4. 请求参数

提交接口为 **POST `/v1/videos`**，请求类型为 `application/json`。

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填，固定为 `gemini-omni-1.1-flash` |
| `prompt` | string | 必填，非空的视频描述 |
| `duration` | integer | 本接入要求显式填写 `4`、`6`、`8` 或 `10`，单位秒；不要依赖默认值 |
| `resolution` | string | 填写 `720p` |
| `aspect_ratio` | string | 画面比例，例如 `16:9`、`9:16` |
| `n` | integer | 固定填写 `1`；本文不提供批量多条生成合同 |
| `images` | string[] | 可选，参考图片的公开 HTTPS 地址数组，保留输入顺序 |
| `videos` | string[] | 可选，参考视频的公开 HTTPS 地址数组 |
| `audios` | string[] | 可选，参考音频的公开 HTTPS 地址数组 |
| `generateAudio` | boolean | 可选，是否请求生成音频；`false` 可显式传递 |
| `preprocess` | boolean | 可选，人脸预处理开关；`false` 可显式传递，不代表绕过内容审核 |
| `seed` | integer | 可选，随机种子；`0` 可显式传递 |
| `size` | string | 可选像素尺寸或比例；一般省略，由 `resolution` 和 `aspect_ratio` 表达规格 |

素材地址必须无需登录、Cookie 或额外请求头即可访问，并在生成期间保持有效；不要提交本地文件路径。素材数量、组合和内容以模型实际能力及审核结果为准。

`generateAudio` 控制输出音频，`audios` 是输入参考音频，两者不是同一个参数。不要把清晰度或比例拼进模型名；也不要传入其他视频模型的 `1792x1024` / `1024x1792` 尺寸，这不是本模型的 720p 调用规格，并可能触发额外尺寸倍率。

## 5. 使用参考素材

将快速开始中的 JSON 请求体替换为下面的示例即可；地址是占位值，必须换成自己的真实素材地址。

### 5.1 图片参考

```json
{
  "model": "gemini-omni-1.1-flash",
  "prompt": "保持参考图中的主体外观，镜头缓慢推进，人物自然转身微笑。",
  "images": ["https://example.com/subject.jpg"],
  "duration": 6,
  "resolution": "720p",
  "aspect_ratio": "16:9",
  "preprocess": false,
  "seed": 0,
  "n": 1
}
```

### 5.2 视频与音频参考

按需要提供 `videos` 或 `audios`；没有对应素材时直接省略该字段。

```json
{
  "model": "gemini-omni-1.1-flash",
  "prompt": "参考视频中的动作节奏，配合参考音频生成自然连贯的画面。",
  "videos": ["https://example.com/motion.mp4"],
  "audios": ["https://example.com/rhythm.wav"],
  "duration": 8,
  "resolution": "720p",
  "aspect_ratio": "16:9",
  "generateAudio": true,
  "n": 1
}
```

## 6. Python 自动轮询已有任务

完成第 3 节提交并设置 `TASK_ID` 后运行下面的脚本。它只查询已有任务，不会发起新的付费生成；完成后再执行第 3.4 节下载命令。

```python
import json
import os
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

base_url = os.environ.get('FLOWAPI_BASE_URL', 'https://flowapi.robusta.top').rstrip('/')
token = os.environ['FLOWAPI_TOKEN']
task_id = os.environ['TASK_ID']
if not task_id.startswith('task_'):
    raise SystemExit('请使用提交响应中的 FlowAPI id。')

deadline = time.monotonic() + 30 * 60
while time.monotonic() < deadline:
    request = Request(
        f'{base_url}/v1/videos/{task_id}',
        headers={'Authorization': f'Bearer {token}'},
    )
    try:
        with urlopen(request, timeout=60) as response:
            task = json.load(response)
    except HTTPError as error:
        if error.code != 429 and error.code < 500:
            raise SystemExit(f'查询失败 HTTP {error.code}，请检查令牌及任务 ID。')
        print(f'查询暂时失败 HTTP {error.code}，15 秒后继续查询原任务。')
    except (URLError, TimeoutError):
        print('查询连接暂时失败，15 秒后继续查询原任务。')
    else:
        status = task.get('status')
        print(task_id, status, task.get('progress', ''))
        if status == 'completed':
            print(f'生成完成，请下载：{base_url}/v1/videos/{task_id}/content')
            break
        if status in ('failed', 'cancelled'):
            raise SystemExit(f'任务失败：{task.get("error") or "请联系管理员查询原因"}')
    time.sleep(15)
else:
    raise SystemExit('本次轮询已超时；任务未被取消，请保存 TASK_ID，稍后继续查询，不要重复提交。')
```

## 7. 常见问题

- **401 / 403：** 检查 FlowAPI Key 是否有效、令牌是否选择 `video`，以及模型白名单是否允许该模型。不要使用供应商 Key 访问 FlowAPI。
- **找不到任务：** 确认使用提交响应的 `id`，且使用同一账户的 Key；不要用上游任务 ID。
- **进度不变或查询偶发超时：** 继续查询同一个任务，降低轮询频率，不要自动重新提交。
- **状态为 `failed`：** 保留任务 ID 和 `error.message`；后台确认失败后退回对应预扣额度。不要把客户端超时等同于任务失败。
- **已完成但下载失败：** 稍后重试同一个 `/content` 地址。生成完成与下载成功是两个步骤，不需要重新生成。
- **费用与预期不符：** 检查 `duration`、`n` 和 `size`，不要同时发送 `seconds`；本文标准示例为 720p、每次一条、0.20 元/秒。

本文依据当前接入配置、接口实现与供应商协议整理；示例响应为结构示意，不代表已完成该模型的付费生成验收。
