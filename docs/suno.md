# Suno 调用方法

通过 FlowAPI 生成歌曲与纯音乐。一个任务采用“提交一次，再轮询结果”的调用流程，支持灵感创作、自定义歌词和纯音乐三种模式。

## 1. 接入准备

| 配置项 | 填写内容 |
| --- | --- |
| API 地址 | `https://flowapi.robusta.top`，不添加 `/v1` |
| 身份认证 | `Authorization: Bearer <你的 FlowAPI API Key>` |
| 令牌分组 | 创建或编辑令牌时选择 `video` 分组 |
| 模型限制 | 关闭令牌的“模型限制”开关，提交和查询使用同一个令牌 |
| 计费模型 | `suno_music`，用于识别计费项目，不需要在请求体里填写 `model` |
| 模型版本 | 请求体使用 `mv` 选择版本，推荐 `chirp-v5` |
| 当前价格 | 人民币 **0.60 元 / 提交任务**；查询已有任务不重复收取生成费 |

一次成功任务可能包含多首作品。当前实测一次返回两首，但客户端应遍历实际返回的数组，不要写死作品数量。0.60 元按提交任务计，不是每首分别计费；调用前可在模型广场核对当前价格。

API Key 属于敏感信息，只保存在服务端或本地环境变量中，不要放入前端网页、公开仓库或分享的请求截图。

## 2. 快速开始：提交与查询

以下命令适用于 macOS、Linux 等终端。先将占位值替换为自己的 FlowAPI API Key，再按顺序执行。

### 2.1 配置环境变量

```bash
export FLOWAPI_BASE_URL='https://flowapi.robusta.top'
export FLOWAPI_TOKEN='替换为你的_FlowAPI_API_Key'
```

### 2.2 提交一次创作任务

```bash
curl --silent --show-error --fail-with-body \
  "${FLOWAPI_BASE_URL}/suno/submit/music" \
  -H "Authorization: Bearer ${FLOWAPI_TOKEN}" \
  -H 'Content-Type: application/json' \
  --data-binary @- \
  --output suno-submit.json <<'JSON'
{
  "mv": "chirp-v5",
  "gpt_description_prompt": "写一首简短中文歌，讲述春天傍晚与朋友散步的快乐，温暖轻快的流行风格。",
  "make_instrumental": false
}
JSON

cat suno-submit.json
```

成功提交的回包示例：

```json
{
  "code": "success",
  "message": "",
  "data": "task_..."
}
```

`data` 是任务 ID。收到任务 ID 只代表已受理，不代表歌曲生成完成。保存它，并从刚才的文件中提取：

```bash
export TASK_ID="$(python3 - <<'PY'
import json

with open('suno-submit.json', encoding='utf-8') as file:
    response = json.load(file)
task_id = response.get('data')
if response.get('code') != 'success' or not isinstance(task_id, str) or not task_id:
    raise SystemExit('提交未返回有效任务 ID，请检查 suno-submit.json，不要直接重复提交。')
print(task_id)
PY
)"

printf '任务 ID：%s\n' "${TASK_ID}"
```

如果提交请求超时或连接中断，不能据此判断任务未创建。先保留已有响应和请求记录，核实任务是否已经受理，不要自动重新发送 POST，以免重复创作、重复计费。

### 2.3 查询这个任务

**单任务查询：**

```bash
curl --silent --show-error --fail-with-body \
  "${FLOWAPI_BASE_URL}/suno/fetch/${TASK_ID}" \
  -H "Authorization: Bearer ${FLOWAPI_TOKEN}" \
  --output suno-result.json

cat suno-result.json
```

**批量查询：**

```bash
python3 - <<'PY' > suno-fetch.json
import json
import os

task_id = os.environ.get('TASK_ID')
if not task_id:
    raise SystemExit('请先设置 TASK_ID。')
print(json.dumps({'ids': [task_id]}))
PY

curl --silent --show-error --fail-with-body \
  "${FLOWAPI_BASE_URL}/suno/fetch" \
  -H "Authorization: Bearer ${FLOWAPI_TOKEN}" \
  -H 'Content-Type: application/json' \
  --data-binary @suno-fetch.json
```

批量请求的 `ids` 可以填写多个已经提交的任务 ID。单任务查询的外层 `data` 是一个任务对象，批量查询的外层 `data` 是任务对象数组。

查询后检查任务对象的 `status`：`SUCCESS` 表示完成，`FAILURE` 表示失败，其他状态继续等待。建议每 5–10 秒查询一次。完整的有限等待示例见第 6 节。

## 3. 三种创作模式

三种模式均调用 `POST /suno/submit/music`，认证方式相同。将快速开始中的 JSON 请求体替换为对应示例即可。

### 3.1 灵感模式：描述你想听到的歌曲

填写 `gpt_description_prompt`，描述主题、情绪、语言和音乐风格，由模型创作歌词与音乐。

```json
{
  "mv": "chirp-v5",
  "gpt_description_prompt": "写一首简短中文歌，讲述春天傍晚与朋友散步的快乐，温暖轻快的流行风格。",
  "make_instrumental": false
}
```

灵感模式也可以生成纯音乐：把同一请求的 `make_instrumental` 改为 `true`，并在描述中写清想要的配器、情绪和节奏。

### 3.2 自定义模式：自己写歌词和风格

使用 `prompt` 提供歌词、`tags` 描述音乐风格、`title` 指定作品标题。

```json
{
  "mv": "chirp-v5",
  "prompt": "[Verse]\n晚风吹过熟悉的街角\n灯火照亮回家的路\n\n[Chorus]\n把今天的故事轻轻唱\n让温柔陪在你身旁",
  "tags": "acoustic pop, warm male vocal, gentle guitar, 80 bpm",
  "title": "晚风回家",
  "make_instrumental": false
}
```

歌词可以使用 `[Verse]`、`[Chorus]` 等段落标记。音乐类型、声音特点、乐器和速度写在 `tags`；`title` 只是作品标题，不用于控制风格。

### 3.3 纯音乐模式：只有配器，没有演唱

自定义纯音乐使用 `tags` 描述风格，设置 `make_instrumental: true`。可以完全省略歌词字段 `prompt`。

```json
{
  "mv": "chirp-v5",
  "tags": "instrumental, ambient piano, soft strings, calm, 70 bpm",
  "title": "雨后微光",
  "make_instrumental": true
}
```

灵感模式和自定义模式分别填写，**不要在同一个请求中同时传入 `gpt_description_prompt` 和 `prompt`**。生成结果具有随机性，描述、歌词段落标记和风格标签都是创作指引，不保证每个字词或风格细节逐项实现。

## 4. 参数与版本

| 参数 | 类型 | 使用方式 |
| --- | --- | --- |
| `mv` | string | 选择模型版本，推荐明确填写 `chirp-v5`；它不是音乐风格字段 |
| `gpt_description_prompt` | string | 灵感模式填写自然语言创作描述；不与 `prompt` 同时传入 |
| `prompt` | string | 自定义歌曲的歌词；自定义纯音乐可省略 |
| `tags` | string | 自定义模式的风格描述，例如曲风、乐器、声音特点和 BPM |
| `title` | string | 可选，作品标题；不是风格控制参数 |
| `make_instrumental` | boolean | `true` 生成纯音乐，`false` 生成带人声歌曲；示例均明确传入 |

版本选择：

| `mv` 值 | 对应版本 | 建议 |
| --- | --- | --- |
| `chirp-v5` | Suno v5 | 推荐；本文三种模式均已用此版本验证成功 |
| `chirp-v4-5` | Suno v4.5 | 可选择的其他版本 |
| `chirp-v4` | Suno v4 | 可选择的其他版本 |
| `chirp-v3-5` | Suno v3.5 | 可选择的其他版本 |

切换版本时修改 `mv`；调整风格时修改 `tags` 或灵感描述。`suno_music` 是 FlowAPI 的计费模型名，不要将它填入 `mv`，也不需要添加 `model` 字段。

## 5. 任务结果与音频保存

### 5.1 单任务成功回包

下例展示结构，任务 ID、作品 ID、链接和时长均为示意值：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_...",
    "action": "MUSIC",
    "status": "SUCCESS",
    "progress": "100%",
    "fail_reason": "",
    "data": [
      {
        "id": "1234567890",
        "clipId": "00000000-0000-4000-8000-000000000000",
        "title": "晚风回家",
        "tags": "acoustic pop, warm male vocal, gentle guitar, 80 bpm",
        "prompt": "[Verse]\n晚风吹过熟悉的街角……",
        "makeInstrumental": false,
        "duration": 38.4,
        "cld2AudioUrl": "https://example.com/song.m4a",
        "cld2ImageUrl": "https://example.com/cover.jpeg",
        "status": 30
      }
    ]
  }
}
```

**任务状态看 `data.status`，音频链接取 `data.data[i].cld2AudioUrl`。** 歌曲对象使用 camelCase 字段名，不能把音频字段写成 `audio_url`。

| 字段 | 含义 |
| --- | --- |
| 任务对象 `task_id` | 提交时获得的任务 ID，用于后续查询 |
| 任务对象 `status` | `SUCCESS` 已完成；`FAILURE` 已失败；其他状态继续轮询 |
| 任务对象 `progress` | 进度展示；以任务 `status` 判断是否完成 |
| 任务对象 `fail_reason` | 失败原因，失败时保留该字段与任务 ID 排查 |
| 任务对象 `data` | 实际生成的歌曲数组，遍历全部元素读取结果 |
| 歌曲对象 `id` | 歌曲记录 ID，不是轮询用的任务 ID |
| 歌曲对象 `clipId` | 作品片段 ID，不是轮询用的任务 ID |
| 歌曲对象 `cld2AudioUrl` | 音频地址 |
| 歌曲对象 `cld2ImageUrl` | 封面图片地址 |
| 歌曲对象 `duration` | 时长，单位为秒 |
| 歌曲对象 `prompt` / `tags` / `title` | 歌词、风格标签与标题 |
| 歌曲对象 `makeInstrumental` | 是否为纯音乐 |
| 歌曲对象 `status` | 歌曲内部状态值，不代替外层任务状态判断 |

### 5.2 提取所有歌曲的音频地址

任务完成后，使用单任务查询保存的 `suno-result.json`：

```bash
python3 - <<'PY'
import json

with open('suno-result.json', encoding='utf-8') as file:
    response = json.load(file)
task = response.get('data')
if response.get('code') != 'success' or not isinstance(task, dict):
    raise SystemExit('查询未返回有效任务对象，请检查 suno-result.json。')
if task.get('status') != 'SUCCESS':
    raise SystemExit(f"任务尚未成功：{task.get('status')}；{task.get('fail_reason', '')}")
for index, song in enumerate(task.get('data') or [], start=1):
    print(f"{index}. {song.get('title', '')}")
    print(song.get('cld2AudioUrl', ''))
PY
```

把下面的占位地址替换为结果中的 `cld2AudioUrl`，即可下载音频：

```bash
export AUDIO_URL='替换为结果中的_cld2AudioUrl'
curl -L --fail --show-error \
  -A 'Mozilla/5.0' \
  "${AUDIO_URL}" \
  --output song.m4a
```

音频下载出现 `403` 时，可以使用正常浏览器打开，或像示例一样显式设置 `User-Agent`。Python 下载时同样为请求设置 `User-Agent: Mozilla/5.0`。**访问音频链接不要附带 FlowAPI API Key**。

音频与封面链接可能具有有效期，拿到结果后及时下载，或转存到自己的存储。任务超时后继续查询原任务，不要因为暂时没有音频地址就新建同一创作任务。

## 6. 完整 Python 示例：提交一次，有限等待

只需要 Python 3 标准库，无需安装第三方依赖。将代码保存为 `suno_example.py`，先按第 2 节设置 `FLOWAPI_TOKEN`。

**本节的 Python 提交与第 2.2 节的 curl 提交二选一，不要为同一次创作重复提交。** 已执行快速开始、拿到任务 ID 后，使用 `--task-id` 查询原任务；只有显式传入 `--submit` 才会创建并计费一个新任务，不带参数或任务 ID 为空时不会提交。

脚本最长轮询 10 分钟，每 5 秒查询一次。拿到任务 ID 后立刻写入当前目录的 `suno-task-id.txt`；成功时保存 `suno-result.json` 并输出全部音频地址。

```python
import argparse
import json
import os
import sys
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import quote
from urllib.request import Request, urlopen


BASE_URL = "https://flowapi.robusta.top"
POLL_INTERVAL = 5


def request_json(method, path, token, payload=None, timeout=30):
    body = None if payload is None else json.dumps(payload).encode("utf-8")
    headers = {"Authorization": f"Bearer {token}"}
    if body is not None:
        headers["Content-Type"] = "application/json"
    request = Request(BASE_URL + path, data=body, headers=headers, method=method)
    try:
        with urlopen(request, timeout=timeout) as response:
            return json.load(response)
    except HTTPError as error:
        detail = error.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {error.code}: {detail}") from error


def main():
    parser = argparse.ArgumentParser(description="Suno 提交一次，轮询已有任务")
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--submit", action="store_true", help="明确创建一个新的计费任务")
    mode.add_argument("--task-id", help="只查询这个已有任务，不发起新的创作")
    parser.add_argument("--wait-seconds", type=int, default=600)
    args = parser.parse_args()
    token = os.environ.get("FLOWAPI_TOKEN", "").strip()
    if not token:
        raise SystemExit("请先设置环境变量 FLOWAPI_TOKEN。")
    if args.wait_seconds <= 0:
        raise SystemExit("--wait-seconds 必须大于 0。")

    task_id = None if args.task_id is None else args.task_id.strip()
    if task_id == "":
        raise SystemExit("--task-id 不能为空；不会提交新的创作任务。")
    if task_id is None:
        # 可用第 3 节任意一种 JSON 请求体替换此处，其他调用流程不变。
        payload = {
            "mv": "chirp-v5",
            "gpt_description_prompt": "写一首简短中文歌，温暖轻快，讲述春日散步。",
            "make_instrumental": False,
        }
        try:
            submitted = request_json("POST", "/suno/submit/music", token, payload)
        except (RuntimeError, URLError, OSError, ValueError) as error:
            print(f"提交未获得明确结果：{error}", file=sys.stderr)
            print("脚本不会自动重发 POST。请先核实任务是否已受理。", file=sys.stderr)
            return 1
        task_id = submitted.get("data")
        if submitted.get("code") != "success" or not isinstance(task_id, str) or not task_id:
            print(json.dumps(submitted, ensure_ascii=False), file=sys.stderr)
            print("未得到有效任务 ID，不会自动重新提交。", file=sys.stderr)
            return 1

    print(f"任务 ID：{task_id}", flush=True)
    Path("suno-task-id.txt").write_text(task_id + "\n", encoding="utf-8")
    print(f"恢复查询：python3 suno_example.py --task-id {task_id}", flush=True)
    deadline = time.monotonic() + args.wait_seconds
    path = "/suno/fetch/" + quote(task_id, safe="")

    while time.monotonic() < deadline:
        remaining = deadline - time.monotonic()
        try:
            result = request_json("GET", path, token, timeout=min(30, remaining))
        except (RuntimeError, URLError, OSError, ValueError) as error:
            print(f"查询中断：{error}", file=sys.stderr)
            print(f"任务 ID 已保留：{task_id}；请使用 --task-id 继续查询。", file=sys.stderr)
            return 1
        task = result.get("data")
        if result.get("code") != "success" or not isinstance(task, dict):
            print(json.dumps(result, ensure_ascii=False), file=sys.stderr)
            print(f"请保留任务 ID：{task_id}，不要重新提交。", file=sys.stderr)
            return 1

        status = task.get("status")
        print(f"状态：{status}，进度：{task.get('progress', '')}", flush=True)
        if status == "SUCCESS":
            Path("suno-result.json").write_text(
                json.dumps(result, ensure_ascii=False, indent=2) + "\n",
                encoding="utf-8",
            )
            songs = task.get("data") or []
            for index, song in enumerate(songs, start=1):
                print(f"{index}. {song.get('title', '')}")
                print(f"音频：{song.get('cld2AudioUrl', '')}")
                print(f"封面：{song.get('cld2ImageUrl', '')}")
            print("结果已保存至 suno-result.json，请及时保存音频和封面。")
            return 0
        if status == "FAILURE":
            print(f"任务失败：{task.get('fail_reason', '')}", file=sys.stderr)
            print(f"任务 ID：{task_id}；脚本不会重新提交。", file=sys.stderr)
            return 1

        remaining = deadline - time.monotonic()
        if remaining > 0:
            time.sleep(min(POLL_INTERVAL, remaining))

    print(f"本次等待结束，任务 ID：{task_id}，任务可能仍在生成。")
    print(f"继续查询：python3 suno_example.py --task-id {task_id}")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
```

已有任务、查询中断或等待超时，务必携带原来的任务 ID 恢复，只查询、不重新创作：

```bash
python3 suno_example.py --task-id "${TASK_ID}" --wait-seconds 600
```

如果刚才运行的是 Python 示例，可从文件恢复：

```bash
python3 suno_example.py \
  --task-id "$(cat suno-task-id.txt)" \
  --wait-seconds 600
```

如果尚未提交，并且明确要发起一次新的创作，再执行下面的命令。此命令会创建一个新的计费任务，不要在已经 curl 提交后用它恢复查询：

```bash
python3 suno_example.py --submit
```

恢复查询始终使用 `--task-id`。`--submit` 只用于新的创作，脚本不会自动重发它。

## 7. 常见问题

| 现象 | 检查与处理 |
| --- | --- |
| `401`，令牌无效或缺失 | 使用自己的 FlowAPI API Key；检查 `Authorization: Bearer ...`、令牌是否过期或被禁用，不要把账户登录密码当成 API Key |
| `403`，无权访问或查询被拒绝 | 检查令牌分组为 `video`，关闭“模型限制”开关，并确认查询使用提交时的同一令牌 |
| 余额或额度不足 | 检查账户余额及令牌可用额度，确认足以提交一个 0.60 元任务；查询时保留原任务 ID，不要重复新建任务 |
| 请求参数错误 | 检查 JSON、`mv` 和字段类型；灵感与自定义参数分开填写，`make_instrumental` 使用布尔值而不是字符串 |
| HTTP `200` 或 `code: success`，但还没有歌曲 | 提交成功只表示已受理；继续查询原任务，直到任务对象 `status` 为 `SUCCESS` |
| 任务状态为 `FAILURE` | 查看并保留 `fail_reason`、任务 ID 和请求记录；排查原因后再决定是否重新创作，客户端不要自动重发提交 |
| 等待超时、网络中断或临时服务错误 | 已有任务 ID 时继续查询该 ID；提交结果不明确时先核实是否已受理，不要直接重新 POST |
| 已成功却取不到音频 | 遍历歌曲数组，读取 `cld2AudioUrl`，注意 camelCase 字段名；不要使用 `audio_url` 或把 `clipId` 当成任务 ID |

接入时最重要的四件事：**令牌选 `video` 并关闭模型限制；提交一次后保存任务 ID；以任务 `status` 判断完成；遍历歌曲数组读取 `cld2AudioUrl`。**
