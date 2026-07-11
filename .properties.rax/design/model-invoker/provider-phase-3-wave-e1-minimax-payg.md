# 第三阶段波次 E1：MiniMax 按量 Provider 设计卡

## 1. 状态与边界

- 模块：`model-invoker`
- Route family：`minimax.platform`
- Provider ID / Runtime Adapter ID：`minimax`
- Offering：`minimax.platform.payg`，`allowed_usage=general_api`
- Deployment：`minimax.platform.global`，Region=`global`
- 设计状态：已按 2026-07-11 官方公开合同刷新，可进入离线实施
- 主路径：`https://api.minimax.io/anthropic` 的 Anthropic Messages 兼容接口
- 补充路径：`https://api.minimax.io/v1` 的 OpenAI Chat Completions 与 Responses 兼容接口
- 明确不含：MiniMax Token Plan、`sk-cp-*` Key、音频/图像/视频生成、Files、MCP、原生旧 `chatcompletion_v2`、多模态输入、优先服务层和真实调用

MiniMax 按量 API 与 Token Plan 是不同 Offering。两者虽然可使用相同协议域名，但 Key、额度、允许用途和计费边界不得互换；按量 Adapter 在配置阶段拒绝 `sk-cp-*` Token Plan Key，也不读取 `MINIMAX_TOKEN_PLAN_API_KEY`。

## 2. 官方证据

| Source ID | 官方来源 | 本卡使用结论 |
|---|---|---|
| `minimax.api.overview.2026-07-11` | <https://platform.minimax.io/docs/api-reference/api-overview> | 按量 Key 与 Token Plan Key分离、当前模型和三种文本调用方式 |
| `minimax.anthropic.2026-07-11` | <https://platform.minimax.io/docs/api-reference/text-anthropic-api> | Anthropic主路径、模型、参数、thinking、工具、流与完整历史要求 |
| `minimax.messages.2026-07-11` | <https://platform.minimax.io/docs/api-reference/text-chat-anthropic> | `/anthropic/v1/messages`、Bearer/x-api-key、请求与响应字段 |
| `minimax.openai.2026-07-11` | <https://platform.minimax.io/docs/api-reference/text-openai-api> | Chat Endpoint、`thinking`、`reasoning_split`、累积流片段与工具历史要求 |
| `minimax.responses.2026-07-11` | <https://platform.minimax.io/docs/api-reference/responses-create> | `/v1/responses`、typed input/output、reasoning、工具、流、`store=false`和无服务器续接合同 |
| `minimax.errors.2026-07-11` | <https://platform.minimax.io/docs/api-reference/errorcode> | 1000/1001/1002/1004/1008/1024/1026/1027/1033/1039/1041/1042错误语义 |

证据 TTL为7天。当前精确文本模型为 `MiniMax-M3`、`MiniMax-M2.7`、`MiniMax-M2.7-highspeed`、`MiniMax-M2.5`、`MiniMax-M2.5-highspeed`、`MiniMax-M2.1`、`MiniMax-M2.1-highspeed`与 `MiniMax-M2`；其他模型、旧别名和模态模型不进入 callable集合。

## 3. Route 与 Credential

| Route ID | Protocol | Endpoint | Auth |
|---|---|---|---|
| `minimax.platform.global.payg.messages` | `messages` | `https://api.minimax.io/anthropic` | `x-api-key`；官方也接受 Bearer |
| `minimax.platform.global.payg.chat_completions` | `chat_completions` | `https://api.minimax.io/v1` | `Authorization: Bearer` |
| `minimax.platform.global.payg.responses` | `responses` | `https://api.minimax.io/v1` | `Authorization: Bearer` |

三条 Route均引用 `MINIMAX_API_KEY`秘密引用。只允许固定官方 HTTPS host或 loopback测试服务；禁止 userinfo、query、fragment、重定向和跨 host发送 Key。Token Plan控制记录继续使用独立 `MINIMAX_TOKEN_PLAN_API_KEY`、`sk-cp-*`前缀和 `interactive_coding_only`用途，不因本按量 Adapter变为 callable。

## 4. 协议与 thinking 方言

### 4.1 Anthropic Messages 主路径

- M3省略 `thinking`时关闭；统一 reasoning为 `none`时发送 `disabled`，其他已支持 effort只转换为 `adaptive`开关，不声称能调节推理深度；
- M2.x thinking无法关闭。`none`在 HTTP前拒绝，其他 effort只表达“保持模型固有 thinking”，不发送 Anthropic `output_config.effort`；
- 不支持 `budget_tokens`与 summary样式控制；thinking块和 signature由 Provider continuation原样保存；
- 官方 tool choice只允许 `auto/none`；不发送未获合同支持的并行开关、JSON Schema、Batch或 hosted tools；
- MiniMax响应 `tool_use`没有 Anthropic新增的 `caller`字段。内部 continuation只在服务端输出归一化时补成显式 `caller.type=direct`，外部注入状态仍必须通过严格字段校验。

### 4.2 Chat Completions

- 始终发送 `reasoning_split=true`，避免把 thinking混入 `<think>`文本；
- M3在无 reasoning或 `none`时显式发送 `thinking.disabled`，其他支持 effort转换为 `thinking.adaptive`；M2.x拒绝 `none`且不发送虚假深度档位；
- 非流 `reasoning_content/reasoning_details`进入统一 reasoning output；官方流字段是累积值，Adapter按已接收前缀只发新增 delta，避免重复文本；
- thinking开启的工具后续轮必须携带完整原响应，但当前 portable Chat输入不能保真携带 reasoning details，因此在 HTTP前拒绝；M3显式关闭 thinking时可使用普通 function call/result历史；
- 工具选择只允许 `auto/none`，不声明结构化输出、并行控制和多模态输入。

### 4.3 Responses

- M3 `none`关闭 reasoning，`minimal/low/medium/high`仅开启 reasoning而不代表深度；M2.x reasoning不能关闭；
- 支持文本、typed function call/result、SSE、usage和 reasoning output；输出格式只允许 `text`；
- 官方响应固定 `store=false`，当前合同没有 `previous_response_id`。Adapter必须清除通用 Responses driver生成的服务器续接 State，并拒绝输入 State；
- thinking开启的工具后续轮因缺少完整 reasoning item保真输入而拒绝；M3关闭 thinking时允许显式 function call/result历史；
- 不启用多模态、prompt cache key、priority service tier或其他 ProviderOptions。

## 5. 能力合同

| 能力 | Messages | Chat | Responses |
|---|---|---|---|
| 文本、流、usage | compatible | compatible | compatible |
| 工具调用 | compatible；auto/none | compatible；auto/none | compatible；auto/none |
| parallel tools | unsupported；调用方不可控制 | unsupported；调用方不可控制 | partial；响应可报告但请求不可控制 |
| reasoning/output | compatible；Messages thinking块 | compatible；split reasoning | compatible；typed reasoning item |
| structured output | unsupported | unsupported | unsupported；当前只声明 text |
| provider/server continuation | provider continuation compatible | unsupported | unsupported；`store=false` |
| prompt cache | 当前 slice unsupported | unsupported | 当前 slice unsupported |
| vision/video/audio/file | 当前 slice unsupported | 当前 slice unsupported | 当前 slice unsupported |

所有能力按精确模型与协议查询；Provider级支持不能扩散为其他 MiniMax模型或 Token Plan能力保证。

## 6. 错误、流与安全

- 1004归 authentication；1008归 billing且禁止自动回退 Token Plan或其他 Key；
- 1002归 rate limit可重试；1000/1001/1024/1033归 provider unavailable可重试；
- 1026/1027归 policy rejected；1039/1042归 invalid request；1041归 rate limit可重试；
- HTTP 400/401/403/404/429/5xx按稳定公共 ErrorKind归一化，transport错误为 provider unavailable；
- 只白名单 request ID、retry-after和限流头；SDK错误、原始错误消息和 Key不得进入公开 unwrap链；
- Chat累积流必须单调去重；三协议均只有一个终态，未知事件 fail closed或只在显式 degradation下保留 Raw。

## 7. 离线测试与真实烟测

离线黑盒必须覆盖：

1. Anthropic与 OpenAI Go SDK对本机 HTTP/SSE fake的三条路径、Header、body和响应；
2. 当前模型正例、未知/模态模型、Token Plan Key和错 Endpoint预调用拒绝；
3. M3与 M2.x在三协议下的 thinking开关、reasoning映射和不支持档位；
4. Messages thinking/signature/tool continuation及服务端 `caller=direct`规范化；
5. Chat非流/累积流 reasoning去重，Responses typed output、`store=false`和无服务器 State；
6. 工具、usage、错误矩阵、重定向、取消、超时、body limit、Credential脱敏与公共 SDK签名；
7. Catalog/Schema/Markdown与随机模型/Key/错误的泄密、错路由 fuzz。

真实烟测只提供 `integration` build tag入口，要求全局确认、MiniMax专属确认、`MINIMAX_API_KEY`和精确 `MINIMAX_SMOKE_MODEL`。本阶段只编译入口，不执行真实认证或付费调用。

## 8. 完成产物

- `provider/minimax/`独立 Adapter；
- 三条 callable按量 Binding，与两条 Token Plan控制记录并存但不互换；
- `tests/minimax/`黑盒、fuzz和 `tests/integration/`显式烟测入口；
- README、Matrix、module、properties、plan与 memory同步；
- 状态最多为 `implemented_offline`。
