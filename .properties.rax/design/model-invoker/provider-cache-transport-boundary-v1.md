# Model Invoker Provider 缓存传输边界 v1

## 1. 结论

当前 `model-invoker`只拥有“缓存相关字段如何安全穿过 Provider边界”的传输合同，不拥有缓存策略。v1允许归一化 Provider返回的 cache read/write用量，并允许经严格 Provider命名空间验证的原生传输选项；不负责缓存键生成、内容存储、TTL、路由、命中决策、淘汰、预热或成本优化。

这一区分避免把 Prompt Cache、server continuation、Provider continuation、SDK credential cache和未来 Context Engine缓存混成一个系统。

## 2. 传输面

| 传输位置 | v1允许 | v1禁止 |
|---|---|---|
| `Request`公共字段 | 不新增统一缓存策略字段 | 通用 cache key、TTL、作用域、写入/旁路策略 |
| `ProviderOptions[AdapterID]` | 只传已选 Adapter明确设计并严格验证的 JSON；当前 xAI允许 `prompt_cache_key` | 跨 Provider复用、未知字段、把它当无校验逃生舱 |
| `Usage.CacheReadTokens` | 归一化 Provider报告的缓存读取明细 | 推导命中率、计费或把它再次加进 `TotalTokens` |
| `Usage.CacheWriteTokens` | 归一化 Provider报告的缓存创建/写入明细 | 代表 Praxis已经拥有创建策略 |
| `ProviderMetadata` | 保存受控的 Provider专属缓存计数/标签 | 保存秘密、完整缓存内容或策略状态 |
| `RawResponse` / Native event | 在统一 Raw安全边界内保留必要原生证据 | 普通日志、持久缓存、跨租户复用 |
| `State` | 继续表示 server/provider continuation | 伪装成 Prompt Cache ID或跨 Route缓存引用 |

## 3. 当前协议归一化边界

| 协议/路线 | 当前传输行为 | 未声明能力 |
|---|---|---|
| OpenAI Responses / Chat及兼容路径 | Provider返回的 cached input token进入 `CacheReadTokens` | 通用缓存创建/TTL策略 |
| Anthropic Messages | cache read与cache creation分别进入 read/write；详细 creation分解可留在 Provider metadata | 公共 Request创建策略；continuation中的 `cache_control`注入固定拒绝 |
| Gemini GenerateContent | cached content token进入 `CacheReadTokens`，详细计数保留在 metadata | Cached Content创建和生命周期管理 |
| Bedrock Converse | 服务返回的 cache read/write token进入统一 Usage | 跨 Bedrock/Anthropic Direct/Vertex复用缓存或 State |
| xAI Responses | 严格验证的 `prompt_cache_key`通过 xAI ProviderOptions传输；cached token进入 read | Praxis生成 key、TTL、命中或淘汰策略 |
| MiMo Messages / Chat | 只保留服务返回的缓存用量 | 公共缓存创建面 |
| 其他当前 Route | 只有 Provider合同明确报告时才归一化；否则 unsupported/unknown | 从协议相似性猜测支持 |

AWS SDK的 `CredentialsCache`只缓存签名凭据刷新结果，属于 Credential执行细节，不是模型 Prompt Cache，也不得进入 `Usage`或 `ProviderOptions`语义。

## 4. Route隔离不变量

1. cache相关 ProviderOptions必须以 Route绑定后的 Runtime AdapterID为命名空间。
2. RouteID门面禁止调用方注入 Provider/Protocol/Endpoint，因此缓存选项不能借选择器漂移到另一服务。
3. `State`已经绑定 Provider与 Protocol；缓存相关 continuation同样不得跨 Binding。
4. Anthropic Direct、Bedrock Mantle/Runtime、Vertex与兼容厂商即使承载相似模型或协议，也不能复用 cache key、State、ProviderOptions或原生 continuation。
5. 订阅 Route拒绝时不得因缓存命中、旧 State或备用 Key而绕过 entitlement或切换 PAYG。
6. Raw与metadata继续经过响应体上限、复制和脱敏；缓存内容本身不进入普通审计字段。

## 5. 明确不实现的缓存策略

- Prompt规范化、prefix切片或自动 key派生；
- 租户/用户/会话缓存作用域；
- TTL、过期刷新、预热、写穿、旁路和淘汰；
- 缓存命中驱动的 Provider/Route选择；
- 跨 Provider复用或从 Raw重放缓存；
- 成本/延迟优化器与计费推断；
- Context Engine、Model Profile或 Runtime Kernel中的缓存协调。

这些能力需要独立设计、数据边界、威胁模型和用户授权，不能从当前 `CacheReadTokens`、`CacheWriteTokens`或 xAI `prompt_cache_key`反推出来。

## 6. 代码证据入口

- 统一计数：`ExecutionRuntime/model-invoker/model.go`；
- Provider命名空间校验：`Request.Validate`；
- Anthropic归一化：`internal/protocol/anthropicmessages/normalize.go`；
- Gemini归一化：`internal/protocol/geminigenerate/normalize.go`；
- OpenAI协议归一化：`internal/protocol/openairesponses/normalize.go`与`openaichat/normalize.go`；
- Bedrock归一化：`internal/protocol/bedrock/mapping.go`；
- xAI严格选项：`provider/xai/dialect.go`；
- 跨 Binding State拒绝：`validateState`与`internal/protocol/Binding`测试。

## 7. Route级机器事实交接

机器资产：[Provider缓存事实CSV v1候选](./provider-cache-facts-v1candidate.csv)。它由`cachefacts`包和`cmd/cachefactsgen`从活动Catalog确定性生成，并由`tests/cachefacts`做漂移门禁。

当前资产逐条覆盖39条默认callable Route、14个默认活跃Adapter和6类协议，固定以下事实：

- Catalog声明的`prompt_caching`支持级别；
- Request控制面、cache key所有权和TTL控制是否暴露；
- `State`与Prompt Cache必须保持分离且绑定当前Route/Provider/Protocol；
- cache read/write usage是否只在Provider实际报告时归一化；
- `TotalTokens`保持Provider总数，read/write明细不得再次相加；
- cache失败不新造统一错误类型，继续走当前Route/Provider错误合同；
- Route证据状态、TTL、失效时间、digest和实际归一化代码入口。

只有xAI Responses当前拥有严格Provider命名空间的`prompt_cache_key`传输面；其余38条默认callable Route不暴露统一Request缓存控制。Messages/Bedrock Converse可以归一化Provider实际返回的read/write计数，但这不代表Praxis拥有写入策略或厂商一定会产生缓存。

## 8. `CacheIntent`候选接缝与联合决策

当前不向`modelinvoker.Request`增加`CacheIntent`。未来若联合审核通过，候选接缝应位于上层调用意图与已绑定Route之间：上层只表达意图，Route绑定完成后由Provider事实编译成该Adapter允许的严格选项；不能让意图重新选择Provider、Endpoint、Credential或绕过entitlement。

联合审核必须先决定：

1. 谁拥有key生成与稳定性，调用方、Context Engine还是独立Cache模块；
2. key的租户、用户、会话和Route作用域，以及跨主体数据隔离；
3. TTL、续期、预热、写入、旁路、淘汰和失败回退语义；
4. Evidence过期、模型版本变化、Credential轮换和Region变化时如何失效；
5. cache read/write计数与真实计费、订阅额度、成本优化之间是否存在可验证关系；
6. Prompt Cache与`State`、Provider continuation、Raw内容和SDK credential cache如何保持类型与生命周期分离；
7. 是否允许缓存意图影响Route选择；若允许，必须由独立策略设计和用户授权，不能在model-invoker内隐式实现。

在这些问题完成联合设计、威胁模型和验收标准前，`CacheIntent`只是一条候选接缝，不是公共API承诺。
