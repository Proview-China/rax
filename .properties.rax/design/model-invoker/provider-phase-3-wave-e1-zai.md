# 第三阶段波次 E1：Z.AI按量 Provider 设计卡

## 1. 状态与边界

- 模块：`model-invoker`
- Route family：`zai.platform`
- Provider ID / Runtime Adapter ID：`zai`
- Offering：`zai.platform.payg`，`allowed_usage=general_api`
- Deployment：`zai.platform.global`
- 设计状态：已按 2026-07-11 官方公开合同刷新，可进入离线实施
- 实现范围：`https://api.z.ai/api/paas/v4` 的 OpenAI Chat Completions兼容文本路径
- 明确不含：GLM Coding Plan、中国 `open.bigmodel.cn` Endpoint、视觉/音频/视频、官方 Web Search/Retrieval、Agent API和真实调用

Z.AI按量 API与 GLM Coding Plan是不同 Offering。按量 Adapter不接受 Coding专属 Endpoint、企业 Coding Key或订阅额度；1309/1311/1313/1315等订阅终态不能触发 PAYG或其他 Key自动回退。

## 2. 官方证据

| Source ID | 官方来源 | 本卡使用结论 |
|---|---|---|
| `zai.introduction.2026-07-11` | <https://docs.z.ai/api-reference/introduction> | 按量 Endpoint、Bearer认证、OpenAI SDK兼容和 Coding Plan专属 Endpoint边界 |
| `zai.chat.2026-07-11` | <https://docs.z.ai/api-reference/llm/chat-completion> | 当前文本模型、thinking、reasoning effort、工具、JSON Object、SSE、usage和 finish reason |
| `zai.thinking.2026-07-11` | <https://docs.z.ai/guides/capabilities/thinking-mode> | interleaved/preserved/turn-level thinking与 `clear_thinking`边界 |
| `zai.function-calling.2026-07-11` | <https://docs.z.ai/guides/capabilities/function-calling> | Function Calling、仅 `tool_choice=auto`和工具结果循环 |
| `zai.errors.2026-07-11` | <https://docs.z.ai/api-reference/api-code> | HTTP+业务错误码、余额、策略、限流、订阅和流异常边界 |

证据 TTL为7天。官方文档当前默认模型为 `glm-5.2`，本实现不使用 `glm-latest`或未列出的别名。

## 3. Route与 Credential

| 维度 | 值 |
|---|---|
| Route ID | `zai.platform.global.payg.chat_completions` |
| Model family | `glm` |
| 当前文本模型 | `glm-5.2`、5.1、5-Turbo、5、4.7/flash/flashx、4.6、4.5/air/x/airx/flash、`glm-4-32b-0414-128k` |
| Protocol | `chat_completions` |
| Endpoint | `https://api.z.ai/api/paas/v4` |
| Credential | `ZAI_API_KEY`秘密引用 |
| Auth | `Authorization: Bearer` |

只允许固定官方 HTTPS host或 loopback测试服务；禁止 userinfo、query、fragment、重定向和跨 host发送 Key。GLM Coding Plan的 `https://open.bigmodel.cn/api/coding/paas/v4`与专属 Key只能由其独立非 callable控制记录识别。

## 4. 方言映射

### 4.1 Thinking

- GLM-4.5及以上文本模型支持 `thinking.type=enabled|disabled`；`glm-4-32b-0414-128k`不声明 reasoning；
- `glm-5.2`支持官方 effort集合；`low/medium`明确转换为 `high`，`xhigh`转换为 `max`，`minimal`因会跳过 thinking而与统一语义冲突，本 slice拒绝；
- 其他 thinking模型只映射启用/禁用，不伪造精确 effort档位；
- 标准按量 Endpoint默认 `clear_thinking=true`。本 slice不启用 preserved thinking；带函数结果的 thinking后续轮因无法回传完整 `reasoning_content`而拒绝；
- 非流和流式 `reasoning_content`进入统一 reasoning output/delta；流 Envelope中的 `request_id`进入统一 Request ID。

### 4.2 工具与输出

- 仅实现 function工具，最多128个，名称最长64；Retrieval、Web Search和 Agent工具不进入通用工具数组；
- 官方只支持 `tool_choice=auto`，其他统一 tool choice在 HTTP前拒绝；
- `tool_stream`独占能力本 slice不启用；普通 SSE工具参数仍由 Chat driver按实际事件拼接；
- `response_format=json_object`映射统一 JSON Object；严格 JSON Schema未获合同支持，拒绝；
- 多模态、user_id/request_id注入、stop数组和采样参数不在当前公共请求面，不通过 ProviderOptions偷渡。

## 5. 能力合同

| 能力 | 判断 |
|---|---|
| 文本、流、usage | compatible |
| 工具调用 | compatible；仅 function + auto |
| parallel tools | partial；由具体模型返回，调用方不可强制非 auto策略 |
| reasoning/output | compatible；4.5+文本模型 |
| JSON Object | compatible |
| JSON Schema | unsupported |
| provider continuation | unsupported；preserved thinking未实现 |
| prompt cache | unknown；不声明 |
| vision/audio/video/file/hosted tools/Agent | 当前 slice unsupported |

所有能力按精确模型查询；Provider级支持不能扩散为所有 GLM模型的能力保证。

## 6. 错误与终态

- 1000/1001/1003/1005 → authentication；1113 → billing且不重试；
- 1210/1213/1214/1215/1261 → invalid request；1211/1212/1220/1221/1222 → permission/model unavailable；
- 1301或 finish reason `sensitive` → policy rejected；
- 1302 → rate limit可重试；1305 → provider unavailable可重试；
- 1308-1321订阅/企业/额度终态 → billing或 permission，不自动切换 Key/Offering；
- finish reason `model_context_window_exceeded` → incomplete + other，不伪装 max output；`network_error` → provider unavailable；
- 只白名单 Request ID、retry-after和限流头；SDK错误、业务响应和 Key不得进入公开 unwrap链。

## 7. 离线测试与真实烟测

离线黑盒必须覆盖：

1. OpenAI Go SDK对本机 HTTP fake的 `/api/paas/v4/chat/completions`、Bearer Header、body和 SSE；
2. 当前文本模型正例、未知/视觉模型、Coding模型/Endpoint错配预调用拒绝；
3. GLM-5.2 effort转换、其他模型 thinking开关、4-32B reasoning拒绝和 preserved thinking拒绝；
4. 非流/流 `reasoning_content`、body `request_id`、工具、JSON Object、usage和单终态；
5. `sensitive`、context window、network error与1000/1113/1211/1301/1302/1305/1315分类；
6. 重定向、取消、超时、body limit、Credential脱敏、Catalog/Schema/Markdown和公共 SDK签名；
7. 随机模型/Key/错误的泄密与错路由 fuzz。

真实烟测只提供 `integration` build tag入口，要求全局确认、Z.AI专属确认、`ZAI_API_KEY`和精确 `ZAI_SMOKE_MODEL`。本阶段只编译入口，不执行真实认证或付费调用。

## 8. 完成产物

- `provider/zai/`独立 Adapter；
- 一条 callable按量 Binding，与 GLM Coding Plan控制记录并存但不互换；
- `tests/zai/`黑盒、fuzz和 `tests/integration/`显式烟测入口；
- README、Matrix、module、properties、plan与 memory同步；
- 状态最多为 `implemented_offline`。
