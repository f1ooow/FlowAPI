# 渠道与模型清单（2026-09-22）

权威逐模型数据见 [coverage-matrix.json](coverage-matrix.json)。这是配置差集，不是模型可用性的实测结果。价格字段仅表示 KeliAPI 储存的零售配置；目标价格和供应商成本尚未定案。

| Keli 渠道 ID / 类型 | 系列 / 子任务 | 完整模型 ID |
|---|---|---|
| 42 / 54 | Seedance 原生 / video | `doubao-seedance-2-5-260628`<br>`doubao-seedance-2-0-260128`<br>`doubao-seedance-2-0-fast-260128`<br>`doubao-seedance-2-0-mini-260615` |
| 48 / 17 | HappyHorse 原生 / video | `happyhorse-1.0-t2v`<br>`happyhorse-1.0-i2v`<br>`happyhorse-1.0-r2v`<br>`happyhorse-1.0-video-edit` |
| 49 / 35 | MiniMax 扩展与 Hailuo / media+video | `speech-2.8-hd`<br>`speech-2.8-turbo`<br>`speech-2.6-hd`<br>`speech-2.6-turbo`<br>`speech-02-hd`<br>`speech-02-turbo`<br>`minimax-files-upload`<br>`minimax-voice-clone`<br>`minimax-music-generation`<br>`music-2.6`<br>`music-2.6-free`<br>`music-2.5+`<br>`music-2.5`<br>`music-2.0`<br>`music-cover`<br>`music-cover-free`<br>`MiniMax-Hailuo-2.3`<br>`MiniMax-Hailuo-2.3-Fast`<br>`MiniMax-Hailuo-02` |
| 50 / 100 | Viapi / media | `viapi-super-resolve`<br>`viapi-erase-subtitles`<br>`viapi-segment-common` |
| 51 / 101 | VolcVOD / media | `volc-vod-erase-subtitle-auto`<br>`volc-vod-erase-subtitle-locations`<br>`volc-vod-audio-extract` |
| 52 / 102 | VolcCV / media | `volc-lens-lqir` |
| 53 / 45 | Seedream 原生 / images | `doubao-seedream-5-0-260128`<br>`doubao-seedream-4-5-251128`<br>`doubao-seedream-4-0-250828` |
| 54 / 45 | Volcengine TTS / media | `doubao-tts-2.0` |
| 56 / 45 | Volcengine ASR / media | `doubao-asr-2.0` |
| 61 / 17 | HappyHorse Zivv / video | `happyhorse-1.0-t2v`<br>`happyhorse-1.0-i2v`<br>`happyhorse-1.0-r2v`<br>`happyhorse-1.0-video-edit`<br>`happyhorse-1.1-r2v`<br>`happyhorse-1.1-i2v`<br>`happyhorse-1.1-t2v` |
| 62 / 35 | MetaSo H3 与文件上传 / video+media | `MiniMax-H3`<br>`minimax-files-upload` |
| 64 / 60 | FastAI 视频 / video | `doubao-seedance-2-5-260628`<br>`doubao-seedance-2-0-260128`<br>`doubao-seedance-2-0-fast-260128`<br>`doubao-seedance-2-0-mini-260615`<br><del>`wan-3.0-time`</del><br><del>`wan-3.0`</del> |
| 74 / 60 | Seedream Pro / Zivv / images | `doubao-seedream-5-0-pro-260628` |
| 75 / 1 | Rolldek Wan / video | `wan3.0-video`<br>`wan3.0-video-prime` |

共 14 条源媒体渠道、48 个源媒体模型 ID；按本次目标范围排除 FastAI 的两个 Wan ID 后，目标配置覆盖 46 个媒体模型 ID，Wan 只保留 Rolldek 75 的两个模型。FastAI 与原生 Seedance、多个 HappyHorse 渠道、MiniMax 文件上传有重叠，不能把渠道行中的模型数相加当成唯一模型数。

其他差集：`claude-opus-4-5`、`claude-sonnet-4-5`、`gpt-5.2`，来自 Keli 中指向 FlowAPI 的渠道。交由 pricing 子任务核实历史别名/可用性；不反向复制这些渠道，也不凭名称增加旧模型。

具体端点设置：渠道 61 的 submit 为 `/v1/services/aigc/video-generation/video-synthesis`，fetch 为 `/api/v1/tasks/{task_id}`；渠道 75 的 `video_protocol=wan-rolldek-v1`。已从实际 `settings` 列选择性读取，其他列出的媒体渠道没有这两个设置。空 upstream_host 表示渠道使用默认地址，实施时从对应适配器默认地址解析，不表示没有供应商。
