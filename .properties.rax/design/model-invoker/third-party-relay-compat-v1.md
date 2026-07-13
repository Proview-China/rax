# 第三方中转站兼容 Route v1 设计

## 目标

Praxis需要支持用户明确配置的第三方中转站，但不能通过放宽OpenAI、Anthropic、Gemini等官方Provider的主机校验来实现。中转路线必须作为独立七维Route存在，并在统一响应中保留`third-party-relay`实际执行身份。

统一的是`Request/Response/Tool Call/Usage/Error`语义，不宣称中转站与原厂在模型纯净度、系统提示、计费、容量、限流或扩展字段上等价。

## 信任边界

1. 官方Provider Config继续只接受官方host/path或本机测试地址；
2. 中转站使用独立`provider/relaycompat`，AdapterID固定为`third-party-relay`；
3. `routegateway.NewRelayCompatFactory()`是显式opt-in Factory，不进入默认18个Builtin Factory；
4. 每条Route只绑定一个协议、一个Endpoint前缀和一个精确模型集合；
5. 生产Endpoint必须为无凭据、无query/fragment的canonical HTTPS URL，本机测试只允许loopback HTTP；
6. SDK禁止自动跳转和自动重试，Retryable只作为统一错误事实返回Runtime；
7. API Key只由Secret Resolver进入Factory，不写Catalog、测试资产、Raw、错误或日志；
8. ProviderOptions默认拒绝，防止兼容路线注入未审核扩展。

## 协议与Endpoint

| Praxis协议 | Route Endpoint前缀 | 实际操作路径 | SDK/Driver |
|---|---|---|---|
| `chat_completions` | `https://host/v1` | `/v1/chat/completions` | OpenAI SDK + Chat driver |
| `responses` | `https://host/v1` | `/v1/responses` | OpenAI SDK + Responses driver |
| `messages` | `https://host` | `/v1/messages` | Anthropic SDK + Messages driver |
| `generate_content` | `https://host/v1beta` | `/v1beta/models/{model}:generateContent` | Google GenAI SDK + GenerateContent driver |

同一中转站暴露多种协议时必须建立多条Route；不能自动猜协议或失败后跨协议重放。

## 能力声明

- `text_generation`、`streaming`、`tool_calling`、`usage_reporting`：Compatible；
- Messages/GenerateContent的`function_error_result`：Compatible；
- Chat的`function_error_result`：Partial，需要显式允许降级；
- Responses的`server_state`：Compatible，仅在中转返回continuation时成立；
- 结构化输出、推理、多模态、Batch、Hosted Tools等未逐Route确认的能力继续拒绝。

## Profile关系

Relay不是新的模型行为Profile。有效Profile仍为`ModelBehaviorProfile × HarnessCapabilityProfile × RuntimePolicy`。中转API可能在服务端注入系统上下文、工具环境或固定推理策略，因此其Harness能力必须记录为“远端不可见/按实测推断”，不能标记为clean API。Input Tokens显著高于请求文本、输出Token越过预期或不同协议表现不一致，均应成为Route级HarnessDelta证据。

## 验收

- 离线：四协议文本与Tool Call、认证头、路径、归一化、精确模型、协议漂移、Endpoint与Factory opt-in反例；
- 集成：真实凭据只通过环境变量注入，单Route文本与强制工具调用；
- 经济门禁：每条Route最多一次16-token文本和一次64-token工具请求，瞬时429最多重试2次且不跨Route；
- 结果只记录协议、模型、FunctionCall、Usage和统一错误，不记录密钥或完整Raw。
