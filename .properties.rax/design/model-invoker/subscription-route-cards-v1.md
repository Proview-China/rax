# 官方订阅调用面设计卡v1候选

## 1. 总边界

- 刷新时间：2026-07-11；只使用厂商官方产品文档、使用政策和条款；
- 允许实现的共同条件：官方明确支持第三方编程工具或通用Agent框架、独立API Key与Endpoint、个人/单租户/前台/非生产；客户端身份与Entitlement必须由宿主可信Resolver生成，普通调用方claim不构成证明；
- 禁止：伪造User-Agent或官方工具名、消费者登录逆向、多人共享、后台/批量/SaaS/backend、隐式PAYG回退；
- 当前实现只批准文本、流、function tool与token usage；reasoning控制、State、结构化输出、并行工具、Hosted/MCP工具均继续显式拒绝；
- 用户之后逐Route手动提供真实Key；当前只做fake Key与本机HTTP/SSE。

## 2. Kimi Code会员

| 字段 | 决定 |
|---|---|
| Route | `kimi.code-membership.global.chat_completions`、`kimi.code-membership.global.messages` |
| Adapter | `kimi-code`，`provider/plancompat` |
| 协议/Endpoint | OpenAI Chat：`https://api.kimi.com/coding/v1`；Anthropic Messages：`https://api.kimi.com/coding/` |
| Key/模型 | 会员控制台创建的独立Kimi Code API Key；模型固定`kimi-for-coding` |
| 身份/用途 | 官方允许主流Coding Agent和OpenClaw/Hermes类通用Agent框架；必须发送调用方真实User-Agent，篡改会影响会员权益；个人交互式编程，产品集成走Kimi开放平台 |
| 配额/终态 | 周期额度从订阅日每7天刷新，另有滚动5小时频率窗口；401/403/429分别归一为credential/permission/rate-limit，不切PAYG |
| 流/工具 | Chat与Messages非流/流、function tool；其余候选能力拒绝 |
| 官方证据 | <https://www.kimi.com/code/docs/en/> |

结论：Adapter已`implemented_offline`，默认Catalog为`callable=false + blocked_by_host_trust`；只有可信宿主激活Catalog并注入授权Resolver后才可调用。

## 3. MiniMax Token Plan

| 字段 | 决定 |
|---|---|
| Route | `minimax.token-plan.global.chat_completions`、`minimax.token-plan.global.messages` |
| Adapter | `minimax-token-plan` |
| 协议/Endpoint | OpenAI Chat：`https://api.minimax.io/v1`；Anthropic Messages：`https://api.minimax.io/anthropic` |
| Key/模型 | 独立`sk-cp-*` Token Plan Key；当前文本切片`MiniMax-M3`、`MiniMax-M2.7`、`MiniMax-M2.7-highspeed` |
| 身份/用途 | 官方DIY入口允许把Key用于OpenClaw/Claude Code/Cline或任意OpenAI兼容工具；当前仍按个人、交互、前台、非生产收窄 |
| 配额/终态 | M2.7类按滚动5小时请求窗口；其他模态按日额度。额度耗尽只拒绝/等待，Praxis不自动换按量Key |
| 流/工具 | Chat与Messages非流/流、function tool；Token Plan MCP与多模态不并入model-invoker Tool |
| 官方证据 | <https://platform.minimax.io/docs/token-plan/intro>、<https://platform.minimax.io/docs/token-plan/quickstart>、<https://platform.minimax.io/subscribe> |

结论：官方明确存在DIY/任意兼容工具入口；Adapter已`implemented_offline`，默认Catalog仍`callable=false + blocked_by_host_trust`。

## 4. Xiaomi MiMo Token Plan

| 字段 | 决定 |
|---|---|
| Route | 中国/新加坡/欧洲三Region × Chat/Messages，共6条 |
| Adapter | `mimo-token-plan` |
| 协议/Endpoint | `token-plan-{cn,sgp,ams}.xiaomimimo.com`；OpenAI `/v1`，Anthropic `/anthropic` |
| Key/模型 | 独立`tp-*`；当前`mimo-v2.5`/`mimo-v2.5-pro`文本切片 |
| 身份/用途 | 官方允许支持自定义Provider的其他编程工具；仅编程工具，明确禁止自动脚本、自定义应用backend与非Coding API用途 |
| 配额/终态 | 套餐有效期和共享额度；401/403/429严格拒绝，不跨Region、不换按量`sk-*` |
| 流/工具 | Chat与Messages非流/流、function tool；MCP/其他模态另行设计 |
| 官方证据 | <https://mimo.mi.com/docs/tokenplan/subscription>、<https://mimo.mi.com/docs/en-US/tokenplan/integration/tools-overview> |

结论：Praxis只有在可信宿主证明真实交互式编程客户端模式后才可激活；默认Catalog为`implemented_offline + callable=false + blocked_by_host_trust`。

## 5. Alibaba Model Studio Coding Plan

| 字段 | 决定 |
|---|---|
| Route | 中国/国际 × Chat/Messages，共4条 |
| Adapter | `alibaba-plan` |
| 协议/Endpoint | 中国`coding.dashscope.aliyuncs.com`、国际`coding-intl.dashscope.aliyuncs.com`；OpenAI `/v1`、Anthropic `/apps/anthropic` |
| Key/模型 | 独立`sk-sp-*`；官方exact-string集合为`qwen3.7-plus`、`qwen3.6-plus`、`kimi-k2.5`、`glm-5`、`MiniMax-M2.5`、`qwen3.5-plus`、`qwen3-max-2026-01-23`、`qwen3-coder-next`、`qwen3-coder-plus`、`glm-4.7`，不推断其他版本兼容 |
| 身份/用途 | 官方明确允许任何支持自定义Endpoint的第三方编程工具与OpenClaw类Agent；禁止Dify/n8n/Coze、Postman、自动脚本、自定义应用和backend |
| 配额/终态 | 按计划请求/额度窗口；禁止自动换Model Studio按量Key |
| 流/工具 | Chat与Messages非流/流、function tool；内建Hosted Tools和MCP不进入本候选Tool合同 |
| 官方证据 | <https://help.aliyun.com/en/model-studio/coding-plan>、<https://help.aliyun.com/en/model-studio/more-tools> |

结论：第三方交互式编程工具范围明确；Adapter已实现，但默认Catalog为`callable=false + blocked_by_host_trust`。

## 6. Alibaba Token Plan Team Edition

| 字段 | 决定 |
|---|---|
| Route | `alibaba.token-plan-team.cn-beijing.chat_completions`、`.messages` |
| Adapter | `alibaba-plan` |
| 协议/Endpoint | OpenAI `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1`；Anthropic `.../apps/anthropic` |
| Key/主体 | Team成员席位生成的独立`sk-sp-*` Key；Gateway仍要求一次调用对应个人主体、单租户、前台和真实客户端身份 |
| 模型 | 使用Token Plan Team独立exact-string文本集合，不与Coding Plan做隐式并集；图像生成模型不进入当前文本Route |
| 身份/用途 | 官方列出OpenClaw、Hermes、Claude Code、OpenCode、Cursor、Codex等和“更多工具”；同样禁止自动化平台、API测试器和自定义backend |
| 配额/终态 | Seat/Credits属于订阅控制面；token usage不等于剩余额度，必须提供新鲜EntitlementState |
| 官方证据 | <https://help.aliyun.com/en/model-studio/token-plan-quickstart>、<https://help.aliyun.com/en/model-studio/more-tools> |

结论：允许第三方交互式工具；Adapter已实现，但默认Catalog为`callable=false + blocked_by_host_trust`，必须由可信宿主激活。

## 7. GLM Coding Plan

| 字段 | 决定 |
|---|---|
| Route | `zai.glm-coding-plan.cn.chat_completions` |
| Endpoint | `https://open.bigmodel.cn/api/coding/paas/v4`（Z.AI国际文档另列`api.z.ai/api/coding/paas/v4`，不能混Route） |
| 条款 | 官方政策明确只允许官方支持工具/产品；未授权SDK式或其他第三方集成可能受限；自有应用、bot、网站、SaaS和服务能力均禁止，除非书面协议 |
| Praxis决定 | Praxis尚未进入官方支持工具清单，不能因为协议兼容而冒充Claude Code/OpenCode等身份 |
| 官方证据 | <https://docs.z.ai/devpack/usage-policy>、<https://docs.z.ai/legal-agreement/subscription-terms>、<https://docs.bigmodel.cn/cn/coding-plan/quick-start> |

结论：`official_client_only + research_only + callable=false`。未来只有官方把Praxis列入支持范围或给出书面许可，才可重新设计。

## 8. 共同实现合同

1. 默认Catalog不调用这些订阅Route；可信宿主激活后，Catalog先执行真实Route、evidence和逐Route exact model，再由可信Resolver提供Invocation/Entitlement并完成policy门禁；失败时Binding/Secret/Factory/Provider零触达。
2. Gateway把attested ClientIdentity放入Pool key，身份变化会创建新Adapter；Factory把真实User-Agent固定在最终HTTP transport，Request/ProviderOptions不能覆盖。
3. 每个计划Key只进入对应Credential Profile与官方host/path；Key前缀、Region、Offering和PAYG互换均被拒绝。
4. `plancompat`只实现Chat/Messages文本+function切片；生产、后台、批量、结构化输出、reasoning、State、并行工具和ProviderOptions拒绝。
5. 本机HTTP/SSE覆盖4类计划、2协议、认证头、真实User-Agent、非流/流与Key/host反例；没有真实认证或付费调用。
