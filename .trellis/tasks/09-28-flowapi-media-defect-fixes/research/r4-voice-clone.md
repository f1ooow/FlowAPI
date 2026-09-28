# Research: R4 MiniMax 声音克隆链路（上传 → 克隆）

- **Query**: 入站/出站 file_id 格式；上传与克隆的渠道选择；是否有按 purpose 的路由
- **Scope**: mixed（FlowAPI 源码 + keli/Canvas 只读 + MiniMax 公开文档 platform.minimaxi.com/docs）
- **Date**: 2026-09-28

## 1. 入站 file_id

- 解析路径：
  - `relay/channel/minimax/voice_clone.go:20` 定义 `FileID int64 \`json:"file_id,string"\``；
  - `:33-36` 调用 `common.UnmarshalBodyReusable`（`common/gin.go:108-148`）→ `encoding/json`（`common/json.go:9-11`）。
- 探针：在 `/tmp` 用独立 Go 程序测试，与 `encoding/json` 行为相同。

  | 输入 | 结果 |
  |---|---|
  | `{"file_id":123456789012345678}` | `json: invalid use of ,string struct tag, trying to unmarshal unquoted value into int64` |
  | `{"file_id":"123…"}` | 成功 |
  | Decoder 路径（磁盘 body，`gin.go:118-124`）传数字 | 同样报错 |

  → **数字格式会被拒**，返回 400（`voice_clone.go:35`）。
- 执行顺序：middleware 用 gjson `.String()` 取 file_id（`middleware/minimax_resource_route.go:46`），数字也能查到绑定，所以数字 file_id 会**先路由成功，再在 handler 返回 400**。controller 已预扣 9.9（`controller/relay_minimax_extended.go:68-74`），出错后在 defer 中退款（`:75-79`）。
- keli 行为相同。keli `voice_clone.go:21-24` 的注释说明：跨 HTTP 边界用 string，避免 JS 数字精度问题。
- **FlowAPI `/v1/files/upload` 返回给客户端的是字符串**：
  - `MiniMaxFileObject.FileID string`（`files.go:21-27`）；
  - 上游返回数字或字符串都接受（`files.go:29-56`）；
  - `c.JSON(response)`（`files.go:127`）写回；
  - 测试断言 `"file_id":"9223372036854775807"`（`media_test.go:122`），另见 `:175-183`。
- **MiniMax 官方上传返回的是整数**：`FileObject.file_id: integer, int64`（`docs/api-reference/file-management-upload.md` 与 `voice-cloning-uploadcloneaudio.md`）。
- 判断：FlowAPI 自己的“上传 → 克隆”链路（字符串往返）是通的。按 MiniMax 官方形态直接传数字的客户端会得到 400，**属于兼容性缺陷，不是链路断裂**。

## 2. 出站 file_id 与其他字段

- FlowAPI 发出的是**数字**：`voice_clone.go:47-64` 走 struct → map，再执行 `upstream["file_id"] = request.FileID`（int64）。测试 `media_test.go:97-100` 断言 `"file_id":9223372036854775807`。
- 官方 `VoiceCloneReq.file_id` 为 `integer, int64`，示例 `file_id: 123456789`（`docs/api-reference/voice-cloning-clone.md`）。→ **出站格式正确**。
- 附带发现：出站字段与官方文档不一致（FlowAPI 从 keli 继承）。

  | 字段 | FlowAPI | 官方 | 后果 |
  |---|---|---|---|
  | 降噪 | `need_noise_reduce`（`voice_clone.go:24`，keli `:29` 同名） | `need_noise_reduction` | 开关可能被上游忽略（上游是否兼容旧名：未确认）；测试 `media_test.go:100` 固化了当前字段名 |
  | accuracy | `*int`，0..100（`:26`、`:40-42`） | number double [0,1]，默认 0.7，配合 `text_validation` | 客户端传 0.7 时反序列化失败返回 400；传 70 会被转发成越界值 |
  | clone_prompt / text_validation / language_boost | 不在 struct 中 | 官方可选字段 | struct 往返（`:47-55`）时被**静默丢弃**。`clone_prompt.prompt_audio` 也是账号资源（purpose=prompt_audio） |
  | voice_id | 正则 `^[A-Za-z][A-Za-z0-9_-]{7,255}$`（`:30`） | 末位不能是 `-` 或 `_` | 上游拒绝后，`:66` 写入的 pending 绑定不会释放（`model/minimax_resource.go:56-57, 85-87`），该 voice_id 以后无法再用 |
  | 试听 text | ≤10000（`:37`） | ≤1000 字符 | 放行过长文本 |

## 3. 路由

### 渠道事实

- FlowAPI 71 ← keli 源 49，80 ← 源 62，两者都在 `vedio` 组（`.trellis/tasks/09-22-flowapi-pricing-verification/flowapi-disabled-channels.json`）。
- 源渠道的优先级、模型与映射见 `09-22-flowapi-provider-coverage/research/coverage-matrix.json` 的 `source_channels`：
  - 62：priority 10，models `[MiniMax-H3, minimax-files-upload]`，mapping null；
  - 49：priority 0，含 files-upload / voice-clone / music / speech-* / Hailuo。
- PRD 记录生产环境为 80 p10、71 p0。**生产 DB 未直接核实。**

### 上传是否一定落到 80

是，只要 80 可用。选择链路：
1. `middleware/distributor.go:416-428`：model 取 multipart 里的 `model` 字段，缺省为 `minimax-files-upload`；
2. `:147-154` 调用 `service.CacheGetRandomSatisfiedChannel`；`ExcludedChannelIDs` 非 nil，所以进入 `cacheGetResilientChannel`（`service/channel_select.go:168-202`）；
3. `model.GetRandomSatisfiedChannelWithOptions`（`model/channel_resilient_select.go:18-92`）只在**最高优先级档**里加权随机（`:63-71`）。80 与 71 都启用时，每次都选 80。

例外情况：
- 80 被禁用或自动禁用；
- channel affinity 已启用且命中（`distributor.go:117-145`，`service/channel_affinity.go:550+`，默认关闭）；
- token 指定了渠道。

另外，扩展 controller 只调用一次上游，不重试；`miniMaxError` 带 SkipRetry（`relay/channel/minimax/extended.go:21-23`）。如果 MetaSo 拒绝 `voice_clone` 用途的上传（未确认），上传直接失败，也不会回落到 71。

### 文件绑定到 80 之后的克隆选渠道 → 必然失败

- `miniMaxRequestBinding`（`minimax_resource_route.go:22-100`）查到 file 绑定，ChannelID=80。
- 进入 `selectMiniMaxResourceChannel`（`:104-151`）：
  - `GetChannelById(80)` 成功；
  - `IsChannelEnabledForGroupModel(group, "minimax-voice-clone", 80)`（`model/channel_satisfy.go:8-35`）为 false，因为 80 没有挂 clone 模型；
  - 返回错误 `original MiniMax channel is not enabled for this group and model`（`:127-129`）；
  - `distributor.go:110-115` 以 **403** 中止。
- 克隆 body 带试听 `model=speech-*` 时，同样 403（80 没有 speech 模型）。
- token 指定 71 时：`distributor.go:176-180` 返回 **409** `resource belongs to another channel`。
- file_id 不是经 FlowAPI 上传的：该资源 required=true，返回 **409** `MiniMax file is not available to this user`（`:86-93` + `distributor.go:42-46`）。
- 绑定逻辑本身没错：文件属于 MetaSo 账号，71 的官方账号用不了，绑定正确阻止了跨账号误用。**缺陷在于上传落到了错误的账号。**
- keli 对比：keli 没有资源绑定，优先级相同（62 > 49），上传同样落到 MetaSo。克隆发到 49 时带的是 MetaSo 的 file_id，预计上游报错（未实测，未确认）。

### 按 purpose 路由

**没有。**
- purpose 只在 handler 中校验（`files.go:78-86`），那时渠道已经选定；distributor 只看 model。
- 代码层面存在一个绕路方法（由代码推断，未实测）：客户端在 multipart 里传 `model=minimax-voice-clone`。只有 71 挂了该模型，因此会选到 71。
  - controller 按路径判定 RelayMode（`relay_minimax_extended.go:45`），并把 OriginModelName 强制设为 `minimax-files-upload`（`:54-56`），上传仍然免费（`:27-28`）；
  - 发往上游的只有 purpose 和 file（`files.go:98-104`）；
  - 但 token 模型限制会按 `minimax-voice-clone` 检查（`distributor.go:65-83`）。

## 4. 附带发现：上传 purpose 白名单与官方不一致

- `files.go:82-86` 只允许 `voice_clone`、`prompt_audio`、`video_generation`、`t2a_async_input`。
- 官方 enum 为 `voice_clone`、`prompt_audio`、`t2a_async_input`、`video_understanding`、`video_generation_input`（`file-management-upload.md`）：
  - `video_generation` **不是官方值**；
  - `video_generation_input` 才是 H3 用 `mm_file://` 引用素材时的 purpose。
- Canvas 的 H3 驱动发送 `purpose=video_generation_input`、`model=minimax-files-upload`（`keli-canvas/server/lib/drivers/video/minimax-h3.ts:18-19, 125-127`），FlowAPI 会返回 **400 unsupported upload purpose**。keli 不校验 purpose，原样转发（keli `files.go:84-95`）。
- 后果：H3 的 `mm_file` 引用需要一条经 FlowAPI 上传产生的绑定（`relay/channel/task/hailuo/adaptor.go:269-270`，以及 `minimax_resource_route.go:79-80` 中 required=true）。因此在 FlowAPI 上，这条链路在上传一步就断了。**B 实例的 H3 使用会直接受影响。**
- 大小限制也不一致：`MaxUploadBytes` 为 20MB（`extended.go:17`），官方 `video_generation_input` 的上限是图片 30MB、视频 50MB、音频 15MB。

## 5. 结论与修法建议

| # | 问题 | 判定 | 建议 |
|---|---|---|---|
| 1 | 上传落到 80 后克隆 403 | **真缺陷**（缺少按用途路由） | 见下方 a/b/c |
| 2 | purpose 白名单缺 `video_generation_input` / `video_understanding` | **真缺陷**（影响 H3） | 改为官方 enum，`video_generation` 可视需要保留作兼容；加表测试 |
| 3 | 入站数字 file_id 被拒 | 兼容性缺陷 | 自定义解析，同时接受数字和字符串（参照 `MiniMaxFileObject.UnmarshalJSON`，`files.go:29-56`），拒绝非整数和 ≤0；出站保持数字；表测试覆盖 数字 / 字符串 / 1.5 / 负数 / 超 int64 |
| 4 | 出站 file_id | 正确 | 不改 |
| 5 | need_noise_reduction、accuracy、clone_prompt 等字段 | **真缺陷**（静默失效或误拒） | 对齐官方字段名、类型和范围；入站可保留旧名作别名；如果支持 clone_prompt，prompt_audio 也要做归属校验 |

第 1 项的修法选项：
- **a.** 在 distributor 中对 `/v1/files/upload` 按 purpose 选择“路由用模型”：
  - `voice_clone` / `prompt_audio` → 只选挂了 `minimax-voice-clone` 的渠道；
  - `video_generation_input` → 按 H3 能力选渠道；
  - 计费维持“免费上传”（controller 已强制 OriginModelName）。
  - 回归测试：两个渠道优先级不同时，`voice_clone` 上传落到挂 clone 模型的渠道。
- **b.** 拆分公开的上传模型名，例如克隆专用的上传模型只挂在 71，客户端按用途传 model。不改路由，但需要客户端配合。
- **c.** 只调优先级不可行：把 71 调高后，H3 上传会落到 71，80 上的 H3 视频又会因绑定不符而失败。

## Caveats / Not Found

- 未实测以下三点：MetaSo 是否接受 `voice_clone` / `video_generation` 用途、上游是否兼容 `need_noise_reduce`、keli 跨账号 file_id 的具体报错。
- 生产渠道的优先级、分组、模型列表来自 PRD 与导入清单，未直接查询生产 DB。
