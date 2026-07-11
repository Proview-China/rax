# 第三阶段波次 E1：xAI 按量 Responses Provider 设计卡

## 1. 状态与边界

- 模块：`model-invoker`
- 波次：第三阶段 E1
- 设计日期：2026-07-11
- 当前状态：设计已刷新，进入离线实施
- 实施对象：xAI API 按量付费直连路线
- 明确排除：Grok 消费者订阅、Grok Build 产品登录、Chat Completions、gRPC、WebSocket、Batch、Deferred、媒体、文件、服务端托管工具、Remote MCP、Context Compaction 与真实烟测

本卡只批准一条 `Responses API + grok-4.5` 文本路线。xAI 消费者产品继续使用已有 `official_client_only + unverified + callable=false` 控制记录；不能把产品登录、订阅权益或 Grok Build 客户端身份当成 `XAI_API_KEY`。

## 2. 官方证据快照

| 主题 | 当前合同 | 官方来源 |
|---|---|---|
| 推荐协议 | Responses API 是推荐路径；Chat Completions 已为 legacy | https://docs.x.ai/developers/model-capabilities/text/comparison |
| Endpoint | REST base 为 `https://api.x.ai`；Responses 创建为 `POST /v1/responses` | https://docs.x.ai/developers/rest-api-reference/inference 与 https://docs.x.ai/developers/rest-api-reference/inference/chat |
| 鉴权 | `Authorization: Bearer <XAI_API_KEY>` | https://docs.x.ai/developers/rest-api-reference/inference |
| 当前模型 | `grok-4.5`，Responses/Chat 均支持；本切片只批准 Responses | https://docs.x.ai/developers/grok-4-5 |
| Reasoning | `grok-4.5` 支持 `low`、`medium`、`high`，默认 `high`，不能禁用 | https://docs.x.ai/developers/model-capabilities/text/reasoning |
| 状态 | 响应默认保存 30 天；`previous_response_id` 可继续会话 | https://docs.x.ai/developers/rest-api-reference/inference/chat |
| 工具 | Responses 支持函数工具；本切片不启用 xAI 托管工具 | https://docs.x.ai/developers/tools/function-calling |
| 缓存 | Responses 使用 body 字段 `prompt_cache_key` 提高缓存命中 | https://docs.x.ai/developers/advanced-api-usage/prompt-caching/maximizing-cache-hits |
| 错误 | 400/401/403/404/405/415/422/429；服务端错误按 5xx 处理 | https://docs.x.ai/developers/debugging |
| 模型淘汰 | 旧 Grok 模型会被重定向；为避免静默模型与价格变化，本切片不接受旧 slug | https://docs.x.ai/developers/migration/may-15-retirement |

证据 TTL 为 7 天。运行时模型可用性仍受账号、地域和团队 ACL 影响；离线实现不声称真实账号一定可调用。

## 3. Route 身份

| 维度 | 固定值 |
|---|---|
| Model Family | `grok`；运行时只允许精确 `grok-4.5` |
| Provider | `xai.api` |
| Offering | `xai.api.payg` |
| Deployment | `xai.api.global` |
| Protocol | `responses` |
| Endpoint | `https://api.x.ai/v1` |
| Credential Profile | `XAI_API_KEY`，Bearer header，绑定本 Provider/Offering/Deployment/Endpoint |

`Config.BaseURL` 只允许 loopback HTTP(S) 测试服务；生产配置不能覆盖到代理、网关、Grok 产品域名或第三方兼容服务。官方没有给出可验证的 API Key 前缀，因此只要求非空、无控制字符，不编造前缀门禁。

## 4. 运行语义

### 4.1 输入与输出

- 复用内部 `openairesponses` driver，但 Provider 身份、错误、能力与字段门禁由 `provider/xai` 独立拥有；
- 支持文本消息、system/developer/user/assistant role、函数工具、函数调用结果、非流与流；
- 支持 typed output、usage、reasoning token、cached token、请求 ID 与 Raw 审计；
- `previous_response_id` 只允许同一 xAI Provider、Responses 协议和本 Route 继续；
- 不启用结构化输出、图像、文件或托管工具，避免把未经本切片验证的字段静默发送。

### 4.2 Reasoning

- 未设置 portable reasoning 时保留服务端默认 `high`；
- 显式设置时只允许 `low`、`medium`、`high`；
- `none`、`minimal`、`xhigh`、`max`、budget token 与 summary-style 控制在 HTTP 前拒绝；
- `reasoning.encrypted_content` 当前公共语义没有保真载体；默认有服务端状态时使用 `previous_response_id`，本切片不请求或伪造加密推理 continuation。

### 4.3 xAI 方言

唯一批准的 Provider Option：

```json
{"xai":{"prompt_cache_key":"stable-conversation-id"}}
```

- `prompt_cache_key` 必须为 1–256 字节可打印 ASCII，不允许控制字符；
- 未提供时不生成；
- 未知 namespace、未知字段、重复语义或畸形 JSON 一律拒绝；
- portable `Metadata` 不发送，因为 xAI 当前只把响应 `metadata` 标为兼容字段，未形成需要本切片保留的业务语义。

## 5. 能力声明

| 能力 | 状态 | 说明 |
|---|---|---|
| text_generation | compatible | Responses 文本切片 |
| streaming | compatible | Responses SSE 事件 |
| tool_calling | compatible | 仅本地函数工具 |
| parallel_tool_calling | compatible | portable bool 原样映射 |
| function_error_result | partial | OpenAI function output 没有 portable `is_error` 标记，需显式 degradation |
| reasoning | compatible | 只允许 low/medium/high |
| reasoning_summary | unsupported | 不把 encrypted reasoning 冒充可读 summary |
| server_state | compatible | 默认存储 30 天并以 `previous_response_id` 继续 |
| prompt_caching | compatible | `prompt_cache_key` 与 cached token usage |
| usage_reporting | compatible | token usage；`cost_in_usd_ticks` 当前只保留 Raw |
| structured_output / hosted_tools / media / batch / background / realtime | unsupported | 不在本切片 |

## 6. 错误与安全

- 400/415/422：`invalid_request`；401：`authentication`；403：`permission`；404：模型或路径映射错误；429：`rate_limit` 且可重试；5xx：`provider_unavailable` 且可重试；
- 对官方常见 `invalid_api_key`、`insufficient_quota`、`model_not_found`、`permission_denied` 等 code 做更精确分类，未知 code 保留但不暴露服务端 message；
- 仅允许 `x-request-id`、`request-id`、`retry-after` 与 `x-ratelimit-*` 进入 ProviderMetadata；
- API Key、错误 body、请求 body 与 SDK error 不进入公开错误链或格式化输出；
- 禁止重定向、限制响应体、尊重取消、超时与统一 Redactor。

## 7. 离线验收

1. loopback fake 证明 `/responses`、Bearer、模型、reasoning、工具、状态与 `prompt_cache_key` 映射；
2. non-stream/stream typed output、reasoning、usage、cached usage 和终态一致；
3. 错 Endpoint、模型、协议、Provider、未知字段与不支持能力均在 HTTP 前拒绝；
4. 401/403/404/429/5xx、取消、重定向、响应体上限与 secret 不泄漏测试通过；
5. Catalog、Schema、Markdown、公共 SDK AST、普通/shuffle/race/fuzz 与 integration 仅编译门禁通过；
6. 新增真实烟测入口，但必须同时满足 `PRAXIS_LIVE_TESTS=1`、`PRAXIS_XAI_LIVE_TESTS=1`、`XAI_API_KEY` 与显式模型；本阶段只编译，不执行。

## 8. 延期项

- 真实认证、真实模型、真实费用、账号 ACL 与 Region 可用性；
- Chat Completions legacy 路线；
- encrypted reasoning 的 stateless continuation；
- hosted web/X/code/MCP/file tools、结构化输出、Context Compaction、WebSocket、Batch、Deferred、Priority、mTLS；
- xAI gRPC 独占能力与 Python SDK，留在 E2/F 触发判断；
- Grok Build/消费者产品、第三方网关和其他 Grok 模型。
