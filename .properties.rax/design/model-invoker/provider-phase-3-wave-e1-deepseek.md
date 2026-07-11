# 第三阶段波次 E1：DeepSeek 直连 Provider 设计卡

## 1. 状态与边界

- 模块：`model-invoker`
- Route family：`deepseek.direct`
- Provider ID：`deepseek`
- Offering：DeepSeek 开放平台按量 API，`allowed_usage=general_api`
- 设计状态：已按 2026-07-11 官方公开合同刷新，可进入离线实施
- 实现范围：OpenAI Chat Completions 主路径、Anthropic Messages 兼容路径
- 明确不含：Beta Prefix/FIM、消费者订阅、Agent工具配置、真实 Key、付费调用和生产批准

本卡只授权 DeepSeek 直连 Provider。它不得复用 OpenAI或 Anthropic的 Provider身份、Credential、Endpoint、错误归属或能力声明。

## 2. 官方证据

| Source ID | 官方来源 | 本卡使用结论 |
|---|---|---|
| `deepseek.quickstart.20260711` | <https://api-docs.deepseek.com/> | OpenAI Base URL为 `https://api.deepseek.com`，Anthropic Base URL为 `https://api.deepseek.com/anthropic`；Bearer API Key；当前模型为 `deepseek-v4-flash/pro` |
| `deepseek.chat.reference.20260711` | <https://api-docs.deepseek.com/api/create-chat-completion/> | `/chat/completions`、SSE、工具、JSON Object、`thinking`、`reasoning_effort`和 `reasoning_content`合同 |
| `deepseek.anthropic.20260711` | <https://api-docs.deepseek.com/guides/anthropic_api/> | `/anthropic/v1/messages`、`x-api-key`、支持/忽略字段，以及未知模型和 Claude别名会自动映射的行为 |
| `deepseek.errors.20260711` | <https://api-docs.deepseek.com/quick_start/error_codes/> | HTTP状态与认证、余额、限流和服务错误边界 |

证据 TTL为7天。`deepseek-chat`与 `deepseek-reasoner`将在 2026-07-24 15:59 UTC废弃，本实现不把它们加入离线可调用模型集合。

## 3. Route与 Credential

| 维度 | Chat主路径 | Messages兼容路径 |
|---|---|---|
| Route ID | `deepseek.direct.payg.chat_completions` | `deepseek.direct.payg.messages` |
| Model family | `deepseek` | `deepseek` |
| Provider model ref | 仅 `deepseek-v4-flash`、`deepseek-v4-pro` | 同左；禁止 Claude别名和未知模型 |
| Deployment | `deepseek.direct.global` | 同左 |
| Protocol | `chat_completions` | `messages` |
| Endpoint | `https://api.deepseek.com` | `https://api.deepseek.com/anthropic` |
| Credential | `DEEPSEEK_API_KEY`秘密引用 | 同一秘密引用、不同 Endpoint绑定 |
| Auth | `Authorization: Bearer` | `x-api-key` |

配置只允许固定官方 HTTPS host或 loopback测试服务；禁止 userinfo、query、fragment、重定向和跨 host Credential发送。默认 Endpoint不含 `/v1`，由协议 SDK追加 `/chat/completions`或 `/v1/messages`。

## 4. 协议与方言

### 4.1 OpenAI Chat主路径

- 复用内部 `openaichat` driver，但 Binding身份始终为 `deepseek`；
- `Reasoning.Effort`映射为 `reasoning_effort`；非 `none`推理显式补充 `thinking.type=enabled`，`none`补充 `disabled`；
- 非流式 `message.reasoning_content`映射为统一 reasoning output；流式 `delta.reasoning_content`映射为 `reasoning_delta`；
- 支持文本、SSE、工具、并行工具、JSON Object、usage和 reasoning；
- JSON Schema严格结构化输出不声明支持；Provider未知扩展不得静默删除；
- Prefix/FIM要求 Beta Endpoint与专门语义，本波次拒绝。

### 4.2 Anthropic Messages兼容路径

- 复用内部 `anthropicmessages` driver，保留 thinking、工具和完整 provider continuation；
- 只允许两个精确 DeepSeek模型 ID；禁止依赖服务端把 Claude别名或未知模型静默映射为 `deepseek-v4-flash`；
- `anthropic-version`与 `anthropic-beta`被官方忽略，不能据此声明 Anthropic原生版本或 Beta能力；
- image、document、redacted thinking、code execution和 MCP块不支持；
- `is_error`、`budget_tokens`和多项兼容字段会被忽略，只有调用方显式允许降级时才可发送相应可降级语义。

## 5. 能力合同

| 能力 | Chat | Messages |
|---|---|---|
| 文本、流、usage | compatible | compatible |
| 工具调用 | compatible | compatible |
| 并行工具 | compatible | partial；官方忽略 `disable_parallel_tool_use` |
| reasoning | native方言 | compatible |
| provider continuation | unsupported | compatible；完整 content block续接 |
| JSON Object | compatible | unsupported |
| JSON Schema | unsupported | unsupported |
| vision/document | unsupported | unsupported |
| prompt cache | unknown，本卡不声明 | unsupported/ignored |

所有模型能力仍在请求时按精确模型校验；Catalog不能把 Provider级兼容性扩散为未来模型的能力保证。

## 6. 错误、流与安全

- 错误归属固定为 `deepseek`，只保留稳定 kind、code、request ID、retry-after和白名单限流头；
- 401认证、402余额、422映射、429限流、5xx服务不可用分别归一化；
- SDK错误、请求对象、响应 body中的 Key、Endpoint中的秘密和原始 cause不得进入公开 unwrap链；
- SSE必须单调序列化 reasoning、text、tool arguments、usage和唯一终态；未知事件 fail closed，只有显式降级时保留在 `NativeEvents`；
- Redirect被禁用；HTTP只允许 loopback离线测试。

## 7. 离线测试与真实烟测边界

离线黑盒必须覆盖：

1. 两协议真实 SDK对本机 HTTP fake的 path、header、body和 SSE；
2. `deepseek-v4-flash/pro`正例，旧别名、Claude别名和未知模型在发 HTTP前拒绝；
3. Chat `thinking/reasoning_effort/reasoning_content`非流与流映射；
4. Messages thinking、工具续接、ignored/unsupported字段的显式降级；
5. 401/402/422/429/5xx、重定向、body limit、取消、超时和泄密反例；
6. Registry、Catalog、Schema、Markdown和公共 SDK签名门禁。

真实烟测只提供 `integration` build tag入口，还必须同时满足 `PRAXIS_LIVE_TESTS=1`、`PRAXIS_DEEPSEEK_LIVE_TESTS=1`、`DEEPSEEK_API_KEY`和精确 `DEEPSEEK_SMOKE_MODEL`。本阶段只编译该入口，不执行真实认证或付费调用。

## 8. 本卡完成产物

- `provider/deepseek/`独立 Adapter；
- 两条 callable Catalog Binding；
- `tests/deepseek/`离线黑盒和 `tests/integration/`显式烟测入口；
- README、Matrix、module、properties、plan与 memory同步；
- 状态最多标为 `implemented_offline`，不得标 `live_verified`或 `production_approved`。
