# 第三阶段波次 E1：Qwen/百炼按量 Provider设计卡

## 1. 状态与实施切片

- 模块：`model-invoker`
- Provider ID：`alibaba.model-studio`
- Runtime Adapter ID：`qwen`
- Offering：`alibaba.model-studio.payg`，`allowed_usage=general_api`
- 当前部署：`cn-beijing` 与 `ap-southeast-1` 的 Workspace专属域名
- 默认协议：OpenAI Responses；补充协议：OpenAI Chat Completions
- 设计状态：已按 2026-07-11 官方公开合同刷新，可进入离线实施
- 明确不含：DashScope原生接口、Anthropic兼容接口、内置工具、多模态、Batch、异步 Responses、Trial域名、Coding/Token Plan、真实调用

当前切片只实现中国北京和新加坡两组 Workspace专属按量 Endpoint。两地各自拥有独立 API Key、Workspace、模型范围和数据部署边界，不能跨 Region复用。美国、德国、日本和中国香港虽然已有官方 Endpoint资料，但未在本卡中形成完整的共同模型与测试矩阵，继续保持待设计状态，不用“任意 Base URL”提前放行。

## 2. 官方证据

| Source ID | 官方来源 | 本卡使用结论 |
|---|---|---|
| `alibaba.qwen.reference.2026-07-11` | <https://www.alibabacloud.com/help/en/model-studio/qwen-api-reference> | Qwen文本提供 Chat、Responses、Messages与 DashScope四类独立接口 |
| `alibaba.qwen.responses.2026-07-11` | <https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses> | Responses Endpoint、同步/流、typed output、工具、reasoning与7天 server state |
| `alibaba.qwen.chat.2026-07-11` | <https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-chat-completions> | Chat字段、thinking、reasoning_content、工具、JSON Object、usage与流 |
| `alibaba.base-url.2026-07-11` | <https://help.aliyun.com/en/model-studio/base-url> | Workspace专属、共享、Trial、Coding与 Token Plan域名及套餐隔离 |
| `alibaba.regions.2026-07-11` | <https://help.aliyun.com/en/model-studio/regions/> | Region、部署范围、数据边界、Workspace域名与 API Key不能跨 Region |
| `alibaba.api-key.2026-07-11` | <https://www.alibabacloud.com/help/en/model-studio/get-api-key> | 按量 `sk-ws-*`新 Key、旧 `sk-*`兼容、Workspace权限与模型授权 |
| `alibaba.errors.2026-07-11` | <https://www.alibabacloud.com/help/en/model-studio/error-code> | InvalidApiKey、Arrearage、AccessDenied、ModelNotFound与 Throttling分类 |

证据 TTL为7天。官方文档明确指出 Responses只处理已列出的参数，其他 OpenAI参数会被忽略；Praxis不能把“服务端静默忽略”当成功降级，未获批字段必须在 HTTP前拒绝或产生明确 MappingDecision。

## 3. Route、Region、Workspace与 Credential

| Route前缀 | Region | Host模板 | Protocol |
|---|---|---|---|
| `alibaba.model-studio.cn-beijing.payg` | `cn-beijing` | `{workspace}.cn-beijing.maas.aliyuncs.com` | Responses、Chat |
| `alibaba.model-studio.ap-southeast-1.payg` | `ap-southeast-1` | `{workspace}.ap-southeast-1.maas.aliyuncs.com` | Responses、Chat |

两协议 Base Path均为 `/compatible-mode/v1`，认证均为 `Authorization: Bearer`，秘密引用为 `DASHSCOPE_API_KEY`。配置必须显式提供 Region与 Workspace ID；Workspace只允许安全 DNS label，Host必须由两者确定性生成。测试可用 loopback Base URL覆盖，但生产配置不能接受共享 DashScope、Trial、Coding、Token Plan或任意第三方 Host。

按量 Adapter接受当前 `sk-ws-*`与仍有效的旧 `sk-*`，显式拒绝 `sk-sp-*`订阅 Key。Catalog中既有 Coding/Token Plan控制记录继续 `interactive_coding_only + callable=false`，不得因本 Adapter落地而获得后端调用权，也不得自动切换额度或余额。

## 4. 模型与协议方言

### 4.1 当前共同文本模型切片

北京与新加坡两地当前共同支持并进入 callable白名单的稳定别名为：

- `qwen3.7-max`、`qwen3-max`；
- `qwen3.6-plus`、`qwen3.6-flash`；
- `qwen-plus`、`qwen-flash`；
- `qwen3-coder-plus`、`qwen3-coder-flash`。

日期快照、字符扮演、视觉/音频、OCR和其他厂商模型不进入本切片。Provider返回模型必须和请求模型精确一致；禁止自动追加 Region后缀、替换快照或静默映射未知模型。

### 4.2 Responses主路径

- 保留 typed message、function call/result、reasoning item、usage、SSE顺序和 `previous_response_id`；服务端 Response ID官方有效期为7天；
- portable reasoning effort映射为官方 `reasoning.effort=none|minimal|medium|high`；`low`没有官方枚举，必须拒绝，不能静默改成 `minimal`或 `medium`；
- 只批准同步请求；`background`、异步轮询和未列出的 OpenAI字段在调用前拒绝；
- 当前通用工具只实现 function；web search、code interpreter、web extractor、MCP、文件/图像搜索等 Provider hosted tools不伪装成普通函数；
- `tool_choice`只允许 auto、none，以及恰好一个函数时的 required/指定函数；strict schema、parallel控制和完整 structured output需按文档精确验证，不能依赖服务端忽略；
- `previous_response_id`必须保持同 Provider、Protocol、Endpoint、Model与 Workspace绑定，跨 Region/Workspace continuation拒绝。

### 4.3 Chat Completions

- Qwen Chat使用 `enable_thinking`与可选 `thinking_budget`，不是文档中仅适用于 DeepSeek/GLM的 `reasoning_effort`；portable effort只映射为开关，`BudgetTokens`映射为正整数 `thinking_budget`，不伪造推理深度；
- 非流/流 `reasoning_content`进入 portable reasoning output/delta；thinking工具后续轮要求完整保留历史 reasoning_content，当前 portable Chat输入不能保真表示时在 HTTP前拒绝；
- function tools、auto/none/required/指定函数选择按公开合同验证；工具 schema根必须是 object；
- `response_format=json_object`兼容，提示中必须显式要求 JSON；严格 JSON Schema不在当前切片；
- `stream_options.include_usage=true`由协议 driver拥有；空 choices的最终 usage chunk不能产生第二终态；
- 多候选、logprobs、搜索、多模态、音频与非标准 provider options保持 slice外。

## 5. 能力合同

| 能力 | Responses | Chat |
|---|---|---|
| 文本、流、usage | compatible | compatible |
| function tools | compatible | compatible |
| parallel tool control | unsupported；当前不批准 | unsupported；当前不批准 |
| structured output | unsupported；当前不批准 | partial；JSON Object only |
| reasoning | compatible；官方 effort四态 | compatible；enable_thinking/thinking_budget/reasoning_content |
| continuation | server state compatible；7天且 identity-bound | unsupported；reasoning历史无法保真输入 |
| hosted tools | unsupported于当前通用工具面 | unsupported于当前切片 |
| multimodal/Batch/async | unsupported于当前切片 | unsupported于当前切片 |

## 6. 错误、流与安全

- 400参数错误归 invalid request；`Arrearage`与 `AllocationQuota.FreeTierOnly`归 billing且不自动启用付费或切换套餐；
- 401/`InvalidApiKey`归 authentication；403/`AccessDenied`/`Model.AccessDenied`归 permission；404模型/Workspace错误归 permission或 mapping；
- 429 `Throttling*`、`limit_requests`、`limit_burst_rate`和 `insufficient_quota`归 rate limit并允许上层显式重试；5xx归 provider unavailable；
- Region、Workspace、Key、Endpoint和模型错配均在 HTTP前拒绝；3xx重定向、userinfo、query、fragment和非 loopback HTTP拒绝；
- 只白名单 request ID、retry-after和限流头；Key、Workspace凭据、SDK错误、原始 body与请求对象不进入公开 unwrap链；
- Responses与 Chat流都必须单调、工具参数完整、usage只合并一次且只有一个终态。

## 7. 离线测试与真实烟测

离线黑白盒必须覆盖：

1. 北京/新加坡两 Region、Responses/Chat四条 Binding的确定性 Endpoint、Bearer Header、body与响应；
2. `sk-ws-*`与旧 `sk-*`正例，`sk-sp-*`、错误 Region、Workspace、订阅/Trial/shared Host和跨 Region选择预调用拒绝；
3. 当前共同模型白名单、未知/快照/多模态模型与响应模型不匹配；
4. Responses typed input/output、server state、reasoning、工具、流与未列字段 fail closed；
5. Chat reasoning_content、JSON Object、工具、usage尾块、thinking continuation拒绝与流；
6. 官方错误矩阵、重定向、取消、超时、body limit、Credential/Workspace脱敏；
7. Catalog/Schema/Markdown、公共 SDK签名与随机 Key/Region/Model/错误的泄密和错路由 fuzz。

真实烟测只提供 `integration` build tag入口，要求 `PRAXIS_LIVE_TESTS=1`、`PRAXIS_QWEN_LIVE_TESTS=1`、精确 Region、Workspace、Key与模型。当前阶段只编译入口，不读取 Key、不调用真实模型、不产生费用。

## 8. 完成产物

- `provider/qwen/`按量 Adapter；
- 北京/新加坡各两条 callable Binding，与既有六条 Alibaba订阅控制记录严格隔离；
- `tests/qwen/`黑盒、fuzz和 `tests/integration/qwen_smoke_test.go`显式入口；
- Catalog、Matrix、design、plan、module、properties与 memory同步；
- 状态最多为 `implemented_offline`。
