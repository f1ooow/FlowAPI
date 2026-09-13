# Research: 前端「表达式错误: Unexpected strict mode reserved word」的根因、是否阻断保存、无 `let` 写法

- **Query**: 用户把 A 条粘进后台表达式编辑器，「Token 估算器」报 JS 错误。回答：会不会阻止保存；无 `let` 等价写法；前端估算器到底认哪些语法
- **Scope**: internal（读前端源码；用 bun 直接调用前端估算器 `evalExprLocally` 与保存链路函数实跑；用 Go 对真实 `pkg/billingexpr` 实跑短路与等价性；**未修改仓库任何代码**，临时模块已清理）
- **Date**: 2026-09-13

---

## 结论

1. **不阻断保存。** 报错来自「Token 估算器」的本地预览求值，它只渲染一个红色提示框，不参与表单校验、不参与提交（证据见下）。`let` 版本可以直接保存并生效，后端引擎认 `let`。
2. **改成无 `let` 也救不了估算器。** 估算器是一个纯 JavaScript `new Function` 求值器，环境里只有 token 变量和 `tier/max/min/abs/ceil/floor`，**没有 `param`、`has`、`header`，也不认 `matches`、`split`、`int`、`float`、`string`、`trim`、`upper`、`first`、`filter`**。任何用 `param()` 判档的表达式在估算器里都必然报错——去掉 `let` 只是把错误从 `Unexpected keyword 'let'` 变成 `param is not defined` / `Unexpected identifier 'matches'`（bun 实跑，见下表）。**估算器只能预览纯 token 表达式，对本任务的 6 条表达式它永远是红的，这是预期行为，不是配置错误。**
3. 因此**建议保留 `let` 写法**（可读、已全矩阵验证）。无 `let` 的等价写法本文也给出并实跑验证（与 `let` 版逐用例一致），仅供将来有其他原因需要时使用。

**后端引擎（`pkg/billingexpr`，expr-lang）和前端估算器（`web/src/features/pricing/lib/tier-expr.ts`，JavaScript）是两套完全独立的解析器：后端认 `let`、`param()`、`matches`、`split` 等；前端只认 JavaScript 表达式语法和固定的 16 个标识符。以后写表达式，后端是真理，前端估算器只对纯 token 表达式有参考价值。**

---

## 1. 为什么不阻断保存（file:line）

| 环节 | 位置 | 行为 |
|---|---|---|
| 估算器求值 | `web/src/features/pricing/lib/tier-expr.ts:271-315` `evalExprLocally` | `new Function(...Object.keys(env), '"use strict"; return (' + exprStr + ');')`（`:305-308`），try/catch 后把异常文本放进返回值的 `error` 字段（`:311-314`），**不 throw** |
| 估算器渲染 | `web/src/features/system-settings/models/tiered-pricing-editor.tsx:1361-1470` `CostEstimator` | `useMemo` 调 `evalExprLocally`（`:1380-1384`），`result.error` 只用于切换红框样式和文案（`:1443-1454`）；组件没有任何 onError/onChange 回调向上传递 |
| 表达式进入表单 | `tiered-pricing-editor.tsx:1683-1695` | Raw 模式下 `effectiveExpr = splitBillingExprAndRequestRules(rawExpr).billingExpr` → `onBillingExprChange`，与估算器结果无关 |
| 提交前校验 | `web/src/features/system-settings/models/model-pricing-sheet.tsx:413-439` `validatePricingValues` | 只校验 per-token 模式的价格字段；`:466-476` `commitDraft` = `form.trigger()` + `validatePricingValues()` + `buildSubmitData()`，`:441-464` 把 `billingExpr` 原样放进提交数据 |
| 写入 options | `web/src/features/system-settings/models/model-ratio-visual-editor.tsx:405-413,591-599` | `JSON.stringify(billingExprMap)` 写 `billing_setting.billing_expr`；`ratio-settings-card.tsx:101` 的校验是「整个 option 值是否合法 JSON」，与表达式内容无关 |
| 后端保存 | `model/option.go:293-301,719-722` | 无表达式编译/冒烟校验（`SmokeTestExpr` 全仓无调用点） |

全链路没有任何地方读取估算器的 `error`。bun 实跑 `splitBillingExprAndRequestRules → combineBillingExpr`：4 条 `let` 版文本保存后与原文 `trim()` 逐字相等（第 4 版文档已记）。

## 2. 前端估算器实际认什么（bun 直接调用 `evalExprLocally`）

估算器环境（`tier-expr.ts:291-304`）：`p, c, len, tier, max, min, abs, ceil, floor, cr, cc, cc1h, img, img_o, ai, ao`。语法 = JavaScript 表达式。

| 表达式 | 估算器结果 |
|---|---|
| `let` 版 A（定稿） | `ERROR: Unexpected keyword 'let'`（浏览器里显示为 `Unexpected strict mode reserved word`，同一错误） |
| 无 `let` 版 A（`matches` + `&&`） | `ERROR: Unexpected identifier 'matches'` |
| `param("size") == "4K" ? tier("4k", 210000) : tier("1k", 120000)` | `ERROR: param is not defined` |
| `has(param("size"), "4096") ? … : …` | `ERROR: has is not defined` |
| `header("x") == "" ? 1 : 0` | `ERROR: header is not defined` |
| `float(int(split(string(trim(upper("2048x1152"))), "x")[0])) > 0 ? 1 : 0` | `ERROR: float is not defined` |
| `first(filter([1,2], # > 1)) == 2 ? 1 : 0` | `ERROR: Invalid character: '#'` |
| `tier("1k", 120000)` | ok cost=120000 tier=1k |
| `len <= 200000 ? tier("s", p * 3 + c * 15) : tier("l", p * 6 + c * 22.5)` | ok cost=10500 tier=s |
| `(p ?? 0) > 10 && c > 0 ? tier("a", 1) : tier("b", 2)` | ok（`??`、`&&`、三元是 JS 语法，可用） |

**白名单**：数字、算术、比较、`&&` `||` `? :` `??`、括号，以及上面 16 个标识符。**黑名单**（后端可用、前端不可用）：`let`、`param`、`header`、`has`、`hour/minute/weekday/month/day`、`matches`、`contains`/`startsWith`/`endsWith`、`split`、`int`、`float`、`string`、`trim`、`upper`/`lower`、`first`/`filter`/`#`、`in`。结论：**只要表达式需要读请求参数，估算器就不可能通过；不存在能让它变绿的改写。**

## 3. 后端 `&&` 是否短路（决定无 `let` 写法安全性）—— 是

对真实 `pkg/billingexpr` 实跑（body `{"size":"auto"}`）：

| 表达式 | 结果 |
|---|---|
| `false && int("auto") > 0 ? 1 : 0` | 0，无错误（右侧 `int("auto")` 未求值） |
| `true \|\| int("auto") > 0 ? 1 : 0` | 1，无错误 |
| `(string(param("size") ?? "") matches "^[0-9]+x[0-9]+$" && float(int(split(string(param("size") ?? ""), "x")[0])) >= 1) ? 1 : 0` | 0，无错误 |

expr-lang 的 `&&` / `||` 是短路求值，`matches` 为假时 `int(split(...))` 不会执行，`auto`/空串不会变成 400。

`param()` 重复调用无副作用：`run.go:94-104` 每次 `gjson.GetBytes(request.Body, path)` 读同一份冻结 body（`price.go:352` 冻结、`tiered_settle.go:208-211` 复用），gjson 单次路径查询是线性扫描，body 只有几百字节，多调几次没有可感知开销。

## 4. 无 `let` 等价写法（已验证，仅供备用）

**A（第 3 版语义）**：

```
(string(param("size") ?? "") matches "^[0-9]+x[0-9]+$" && float(int(split(string(param("size") ?? ""), "x")[0])) * float(int(split(string(param("size") ?? ""), "x")[1])) >= 4194304 ? tier("4k", 210000) : tier("1k-2k", 120000)) * max(float(param("n") ?? 1), 1.0)
```

与 `let` 版 A 在 52 个用例（26 个生产尺寸 + 15 种异常输入 + 11 种张数）上逐一相同；唯一「差异」是 `99999999999999999999x1` 两者都报 `int()` 运行错误、只有错误信息里的位置数字不同（都是预扣 400 拒绝）。

B/C 同理把 `px` 内联两次（`>= 4194304` 与 `> 1572864` 各一次）。D/E/F 的五段 `imageSize` 链无 `let` 写法要把 `first(filter([...], # != nil && trim(string(#)) != ""))` 内联到每个比较里（第 4 版有三处比较 → 三份拷贝，约 1.3k 字符），可行但难读；`first/filter/#` 在后端可用（第 2 版已验证 10 个用例与显式版一致），前端估算器同样不认。**不建议采用。**

## 5. 附带提醒（来自 tuzi 表达式原文的对比）

- tuzi 引擎是 `v2`，`tier("4K", 0.257)` 在它那里就是 $0.257/次；**在我们的 v1 引擎里 `0.257` 会被当成 $/1M，折算后四舍五入为 0 quota，图片免费**。不要照抄上游数字。
- tuzi 原文写 `upper(param("…"))`，我们的引擎里 `upper(nil)` 会报错（实跑：`interface conversion: interface {} is nil, not string`），所以我们的写法必须是 `upper(trim(string(param("…") ?? "")))`。

## Caveats

- 浏览器里的错误文案 `Unexpected strict mode reserved word` 与 bun 里的 `Unexpected keyword 'let'` 来自不同 JS 引擎对同一语法错误的描述，已按 `"use strict"; return (let …)` 的构造方式确认是同一根因。
- 未在真实浏览器里点保存按钮；保存链路的结论来自源码与 bun 对同一函数的实跑。
