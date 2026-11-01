# OCR API 接口文档

通过 FlowAPI 或直连硅基流动调用 OCR 模型。两种入口使用相同的请求体与响应结构，均为 OpenAI Chat Completions 格式。

## 1. 通过 FlowAPI 调用

**POST `https://flowapi.robusta.top/v1/chat/completions`**

请求头：

```http
Content-Type: application/json
Authorization: Bearer <TOKEN>
```

`request.json` 使用第 3 节的共享请求体。

```bash
curl --request POST \
  --url 'https://flowapi.robusta.top/v1/chat/completions' \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer <TOKEN>' \
  --data-binary @request.json
```

## 2. 直连硅基流动

**POST `https://api.siliconflow.cn/v1/chat/completions`**

请求头：

```http
Content-Type: application/json
Authorization: Bearer <TOKEN>
```

`request.json` 使用第 3 节的同一个共享请求体。

```bash
curl --request POST \
  --url 'https://api.siliconflow.cn/v1/chat/completions' \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer <TOKEN>' \
  --data-binary @request.json
```

## 3. 共享请求体

以下为 DeepSeek-OCR 的文字与坐标请求。`data:image/png;base64,...` 为格式占位，必须替换为真实图片的 Base64 Data URL。

```json
{
  "model": "deepseek-ai/DeepSeek-OCR",
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "image_url",
          "image_url": {
            "url": "data:image/png;base64,..."
          }
        },
        {
          "type": "text",
          "text": "<image>\n<|grounding|>OCR this image."
        }
      ]
    }
  ],
  "max_tokens": 4096,
  "temperature": 0,
  "stream": false
}
```

### 请求参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 模型完整 ID，见下表 |
| `messages` | array | 消息列表；图片与提示词放在同一条用户消息中 |
| `messages[].role` | string | 本接口示例使用 `user` |
| `messages[].content` | array | 包含 `image_url` 图片项和 `text` 提示词项 |
| `image_url.url` | string | 图片 URL 或 Base64 Data URL；不可直接传 PDF/PPT 文件 |
| `text` | string | 对应模型及识别模式的提示词，见下表 |
| `max_tokens` | integer | 输出 token 上限；图片输入也占用模型上下文 |
| `temperature` | number | 示例为 `0` |
| `stream` | boolean | 本文使用 `false`，返回完整 JSON 响应 |

### 模型与提示词

| `model` | 纯文字识别 `text` | 文字与坐标 `text` | 建议 `max_tokens` |
| --- | --- | --- | --- |
| `deepseek-ai/DeepSeek-OCR` | `<image>\nFree OCR.` | `<image>\n<\|grounding\|>OCR this image.` | `4096` |
| `PaddlePaddle/PaddleOCR-VL-1.5` | `OCR:` | `Spotting:` | `8192` |

`\n` 表示换行。切换模型时同时替换 `model`、`text` 和 `max_tokens`。DeepSeek 不要直接将输出上限设为 `8192`：图片输入与输出之和可能超过其上下文限制。

## 4. 正常返回结构

以下仅展示 JSON 结构；ID、时间、识别文字与 token 数值均为示意，不是实际识别结果。

```json
{
  "id": "chatcmpl-example",
  "object": "chat.completion",
  "created": 0,
  "model": "deepseek-ai/DeepSeek-OCR",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "示意识别文字"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 20,
    "total_tokens": 120
  }
}
```

| 返回字段 | 说明 |
| --- | --- |
| `id` | 响应 ID |
| `object` | 非流式响应类型，通常为 `chat.completion` |
| `created` | 响应创建时间，Unix 秒级时间戳 |
| `model` | 返回响应对应的模型 |
| `choices[].index` | 候选结果序号 |
| `choices[].message.role` | 本例为 `assistant` |
| `choices[].message.content` | OCR 原始结果字符串；格式取决于模型与提示词 |
| `choices[].finish_reason` | `stop` 表示正常结束；`length` 表示输出被长度限制截断 |
| `usage.prompt_tokens` | 输入 token 数量 |
| `usage.completion_tokens` | 输出 token 数量 |
| `usage.total_tokens` | 总 token 数量 |

## 5. 识别内容与坐标

### 纯文字识别

DeepSeek 使用 `<image>\nFree OCR.` 提示词，Paddle 使用 `OCR:` 提示词。两者均从 `choices[0].message.content` 读取文字字符串，不包含统一的独立 `boxes` 字段。

```text
示意标题
示意正文
```

### DeepSeek-OCR：文字与坐标

使用 `<image>\n<|grounding|>OCR this image.`。以下为格式示意：

```text
<|ref|>示意识别文字<|/ref|><|det|>[[100,200,600,260]]<|/det|>
```

- `det` 是矩形列表，每个矩形为 `[x1, y1, x2, y2]`。
- 坐标按 **999** 归一化：`pixel_x = x / 999 × width`，`pixel_y = y / 999 × height`。
- `ref` 在本页图片 OCR 提示词下可为识别文字；文档转 Markdown 等其他模式下也可能是区域类别，不能统一当作正文。

### PaddleOCR-VL-1.5：文字与坐标

使用 `Spotting:`。以下为格式示意，八个 `LOC` 整数依次为 `x1,y1,x2,y2,x3,y3,x4,y4`：

```text
示意识别文字<|LOC_100|><|LOC_200|><|LOC_600|><|LOC_200|><|LOC_600|><|LOC_260|><|LOC_100|><|LOC_260|>
```

- 四组坐标构成四边形，按 **1000** 归一化：`pixel_x = x / 1000 × width`，`pixel_y = y / 1000 × height`。
- 部分输出包含 `TEXT_START`、`TEXT_END`、`LOC_BEGIN`、`LOC_END` 包装标记。
- `width`、`height` 均指实际提交图片的像素尺寸；保留原始特殊标记再解析，不能先过滤 `LOC` 或 `det`。

## 6. 官方参考

- [硅基流动：多模态请求格式](https://docs.siliconflow.cn/docs/userguide/capabilities/multimodal-vision)
- [PaddleOCR-VL-1.5：模型与 Spotting 提示词](https://huggingface.co/PaddlePaddle/PaddleOCR-VL-1.5)
