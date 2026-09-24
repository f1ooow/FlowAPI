# 媒体处理与音频渠道

## 目标与范围

落实父任务 R3。Viapi 三动作、VolcVOD 三动作、VolcCV 图像增强；MiniMax 文件上传/克隆/音乐与六种 speech TTS；火山 `doubao-tts-2.0`/`doubao-asr-2.0`。源渠道 49/50/51/52/54/56，另含 62 的 MiniMax 上传。FastAI 由 video 子任务负责，不在本任务重复实现。

## 依赖

- 父 P0 提供渠道注册、共享路由和轮询/倍率快照接口。
- 本任务先交付 MiniMax 上传/账号一致性供 video 的 H3；先交付 VOD credentials/upload/sign 供自身 VOD/CV/ASR 按实际引用复用。
- 与 images 共享火山 adaptor/constants 的修改由父任务串行整合。
- 单位与语义样例先交 pricing，正式价格与配置依赖 pricing 输出。

## 验收

- [ ] Viapi/VOD/CV 的 channel ID、模型、签名、同步/异步响应及轮询元数据路径完整。
- [ ] 上传→克隆→合成/视频的 file_id/voice_id 归属同一供应商账号；多 key/多渠道情况下不会跨账号误用；int64 ID 无 JS 精度损失。
- [ ] MiniMax TTS 音频/字幕、音乐、克隆的实际路由和错误与客户端合同一致；文件上传免费，克隆/音乐按确定的计费模式结算。
- [ ] 火山 ASR/TTS 的请求头、上传/签名、音频二进制与 content type、错误和 usage/时长校验有 fixtures。
- [ ] 字符/秒/分钟/次的转换、上下界、预扣、失败退款和重复轮询幂等有精确样例；保留 FlowAPI 饱和审计。
- [ ] 临时媒体结果、上传结果和签名 URL 可按契约访问，任务保持原渠道/账号和价格快照。

范围不含新供应商探索、生产部署、付费调用；KeliAPI 全程只读。
