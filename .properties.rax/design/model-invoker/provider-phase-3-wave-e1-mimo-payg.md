# 第三阶段波次 E1：Xiaomi MiMo按量 Provider设计卡

## 1. 状态与计划修正

- 模块：`model-invoker`
- Route family：`xiaomi.mimo.payg`
- Provider ID / Runtime Adapter ID：`xiaomi-mimo`
- Offering：`xiaomi.mimo.payg`，`allowed_usage=general_api`
- Deployment：`xiaomi.mimo.global`，Region=`global`
- 设计状态：已按 2026-07-11 官方公开合同刷新，可进入离线实施
- 默认主路径：`https://api.xiaomimimo.com/anthropic` 的 Anthropic Messages兼容接口
- 补充路径：`https://api.xiaomimimo.com/v1` 的 OpenAI Chat Completions兼容接口
- 明确不含：Responses、Token Plan、`tp-*` Key、区域 Token Plan域名、Web Search、全模态理解、ASR/TTS、旧 V2模型、真实调用

第三阶段总计划原写“Responses主路径、Chat/Messages降级”。当前官方 `llms.txt`、快速开始与 API Reference只公开 Chat Completions和 Messages，没有公开 Responses Endpoint或字段合同。依据“证据优先、不得伪装兼容”门禁，本卡把当前切片修正为 Messages主路径 + Chat补充路径，不生成 Responses Binding。

MiMo按量 API与 Token Plan是不同 Offering。按量使用普通 `sk-*` API Key与 `api.xiaomimimo.com`；Token Plan使用 `tp-*` Key和中国/新加坡/欧洲独立 `token-plan-*`域名，两者不得互换或自动回退。

## 2. 官方证据

| Source ID | 官方来源 | 本卡使用结论 |
|---|---|---|
| `mimo.index.2026-07-11` | <https://platform.xiaomimimo.com/llms.txt> | 当前官方文档索引只列 OpenAI Chat与 Anthropic Messages |
| `mimo.first-call.2026-07-11` | <https://platform.xiaomimimo.com/static/docs/quick-start/first-api-call.md> | 按量 Endpoint、`MIMO_API_KEY`、两协议 SDK与 reasoning历史要求 |
| `mimo.models.2026-07-11` | <https://platform.xiaomimimo.com/static/docs/quick-start/model.md> | 模型能力、上下文、输出和限流 |
| `mimo.openai.2026-07-11` | <https://platform.xiaomimimo.com/static/docs/api/chat/openai-api.md> | Chat字段、thinking、JSON Object、工具、reasoning_content、流与终态 |
| `mimo.anthropic.2026-07-11` | <https://platform.xiaomimimo.com/static/docs/api/chat/anthropic-api.md> | Messages字段、thinking/signature、工具、parallel控制、流与终态 |
| `mimo.reasoning-history.2026-07-11` | <https://platform.xiaomimimo.com/static/docs/usage-guide/passing-back-reasoning_content.md> | thinking工具历史必须完整回传，否则 HTTP 400 |
| `mimo.payg.2026-07-11` | <https://platform.xiaomimimo.com/static/docs/price/pay-as-you-go.md> | 按量普通 Key、余额计费、Token Plan额度不互通 |
| `mimo.token-plan.2026-07-11` | <https://platform.xiaomimimo.com/static/docs/price/tokenplan/quick-access.md> | `tp-*` Key、三 Region专属 Endpoint及用途隔离 |
| `mimo.deprecation.2026-07-11` | <https://platform.xiaomimimo.com/static/docs/updates/deprecate.md> | V2旧模型名已于 2026-06-30失效，不能进入 callable集合 |
| `mimo.errors.2026-07-11` | <https://platform.xiaomimimo.com/static/docs/quick-start/error-codes.md> | 400/401/402/403/404/421/429/500/503错误语义 |

证据 TTL为7天。虽然模型总览仍展示 V2历史行，官方下线公告明确旧名称已经失效；当前文本 callable模型只允许 `mimo-v2.5-pro`与 `mimo-v2.5`。

## 3. Route与 Credential

| Route ID | Protocol | Endpoint | Auth |
|---|---|---|---|
| `xiaomi.mimo.global.payg.messages` | `messages` | `https://api.xiaomimimo.com/anthropic` | `Authorization: Bearer` |
| `xiaomi.mimo.global.payg.chat_completions` | `chat_completions` | `https://api.xiaomimimo.com/v1` | `Authorization: Bearer` |

两条 Route均引用 `MIMO_API_KEY`秘密引用。配置必须接受 `sk-*`并拒绝 `tp-*`；只允许固定官方 HTTPS host或 loopback测试服务，禁止 userinfo、query、fragment、重定向和跨 host发送 Key。Token Plan控制记录继续使用 `MIMO_TOKEN_PLAN_API_KEY`与三 Region专属 Credential/Endpoint，不因按量 Adapter变为 callable。

## 4. 模型与协议方言

### 4.1 通用模型边界

- `mimo-v2.5-pro`与 `mimo-v2.5`默认开启 thinking，可显式 `enabled/disabled`；portable effort只映射开关，不声称能调节推理深度；
- thinking开启时 Provider强制采样默认值；当前公共 Request没有采样字段，因此 Adapter不注入伪造值；
- 只实现文本、函数工具、流、usage和 Chat JSON Object；`mimo-v2.5`的图像/音频/视频输入、Web Search和其他模态能力保持 slice外；
- Provider返回模型必须与请求模型精确一致，禁止旧模型自动路由或别名静默映射。

### 4.2 Anthropic Messages主路径

- `thinking.type=enabled|disabled`由 MiMo方言显式生成；不发送 Anthropic `adaptive`、`budget_tokens`、`display`或 `output_config.effort`；
- thinking块与 signature通过 Provider continuation原样保存；MiMo响应 `tool_use`没有 Anthropic新增 `caller`字段，内部 State只在服务端输出规范化时补 `caller.type=direct`，发送回 MiMo前移除；
- `tool_choice`只允许 `auto`；`disable_parallel_tool_use`映射 portable parallel开关；工具 schema根必须为 object，`strict`不在当前合同；
- Messages没有 JSON Object/JSON Schema输出合同；Metadata、Prompt Cache创建、hosted tools和多模态不在本 slice；
- `content_filter`归策略拒绝；`repetition_truncation`归 incomplete + other，不当作正常完成。

### 4.3 Chat Completions

- 对 reasoning请求发送 `thinking.enabled`；`ReasoningEffortNone`发送 `thinking.disabled`；其他 effort只转换为开关；
- 非流/流 `reasoning_content`进入 portable reasoning输出/delta；
- thinking工具后续轮必须回传完整 assistant `reasoning_content`，当前 portable Chat输入无法保真表示，因此在 HTTP前拒绝；显式关闭 thinking时可使用普通 function call/result历史；
- `tool_choice`只允许 `auto`，其他值会被服务端静默删除，Adapter必须预调用拒绝；Chat不暴露 parallel控制；
- `response_format=json_object`兼容，严格 JSON Schema不支持；Web Search工具、模态内容和音频输出不进入通用工具数组；
- `repetition_truncation`归 incomplete + other；`content_filter`归策略拒绝。

## 5. 能力合同

| 能力 | Messages | Chat |
|---|---|---|
| 文本、流、usage | compatible | compatible |
| 工具调用 | compatible；auto | compatible；auto |
| parallel tools | compatible；可禁用 | unsupported；无 portable控制合同 |
| reasoning/output | compatible；thinking块与 signature | compatible；reasoning_content |
| structured output | unsupported | partial；JSON Object only |
| continuation | provider continuation compatible | unsupported；thinking历史无法保真输入 |
| prompt cache | 只读取 usage；创建不在公共面 | 只读取 usage；创建不在公共面 |
| vision/audio/video/web search | 当前 slice unsupported | 当前 slice unsupported |
| Responses/server state | 无官方合同 | 无官方合同 |

## 6. 错误、流与安全

- HTTP 400归 invalid request；401归 authentication；402归 billing且不自动切换 Token Plan或其他 Key；
- 403归 permission；404归 model/resource；421归 policy rejected；429归 rate limit可重试；500/503归 provider unavailable可重试；
- transport错误归 provider unavailable；协议畸形、未知终态和不匹配模型 fail closed；
- 只白名单 request ID、retry-after和限流头；SDK错误、原始错误消息、请求对象和 Key不得进入公开 unwrap链；
- Chat reasoning/text与 Messages content block流必须单调，工具参数完整且只有一个终态。

## 7. 离线测试与真实烟测

离线黑盒必须覆盖：

1. OpenAI与 Anthropic Go SDK对本机 HTTP/SSE fake的两条路径、Bearer Header、body和响应；
2. 两个 V2.5文本模型正例，V2旧名、模态模型、未知模型、`tp-*` Key和 Token Plan Endpoint预调用拒绝；
3. 两协议 thinking开关、reasoning输出、工具、parallel边界与 JSON Object；
4. Messages thinking/signature/tool continuation与服务端 direct caller规范化；
5. Chat thinking工具历史不完整拒绝、非流/流 reasoning、专属 repetition终态；
6. 400/401/402/403/404/421/429/500/503、重定向、取消、超时、body limit、Credential脱敏和模型错配；
7. Catalog/Schema/Markdown、公共 SDK签名和随机模型/Key/错误的泄密/错路由 fuzz。

真实烟测只提供 `integration` build tag入口，要求全局确认、MiMo专属确认、`MIMO_API_KEY`和精确 `MIMO_SMOKE_MODEL`。本阶段只编译入口，不执行真实认证或付费调用。

## 8. 完成产物

- `provider/mimo/`独立 Adapter；
- 两条 callable按量 Binding，与六条 Token Plan控制记录并存但不互换；
- `tests/mimo/`黑盒、fuzz和 `tests/integration/`显式烟测入口；
- README、Matrix、module、properties、plan与 memory同步；
- 状态最多为 `implemented_offline`。
