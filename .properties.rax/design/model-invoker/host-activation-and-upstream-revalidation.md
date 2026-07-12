# Model Invoker宿主激活与十家上游再验证设计

## 1. 目标与范围

- 对象：`ExecutionRuntime/model-invoker`；不新增模块；
- 目标：十家上游按`P0官方API -> P1合法程序化套餐 -> P2厂商native`分层，确保默认调用面至少有一条来源明确、协议/认证/模型一致、可由Gateway构造的Route；
- 安全基线：P0模型字符级exact；P1默认fail-closed，只能由可信宿主按精确RouteID激活；任何失败不触达Binding、Secret、Factory或Provider；
- 当前不做：真实Key读取、付费/套餐调用、生产批准、Runtime Kernel、Profile、Context、缓存策略、自动Route选择。

## 2. 十家P0模型门禁

本轮不引入只有接口而没有真实Provider发现实现的动态Resolver。所有十家直连P0 Route使用`static_catalog + exact_provider_id`，由7天evidence TTL强制复核。

| 家族 | 本轮exact集合 |
|---|---|
| OpenAI | `gpt-5.5`；`gpt-5.6`仍为受限预览，不进入默认集合 |
| Anthropic | `claude-fable-5`；未来可用官方Models API增强，但不能fallback-open |
| Gemini | `gemini-3.5-flash`；GenerateContent保留为可用legacy P0 |
| xAI | `grok-4.5` |
| Z.AI | `glm-5.2`；Provider直接调用面同步收窄 |
| DeepSeek | `deepseek-v4-flash`、`deepseek-v4-pro` |
| Kimi API | `kimi-k2.7-code[-highspeed]`、`kimi-k2.6`、`kimi-k2.5`、Moonshot V1三档 |
| MiniMax API | `MiniMax-M3`及官方当前M2.7/M2.5/M2.1/M2文本集合 |
| Xiaomi MiMo API | `mimo-v2.5-pro`、`mimo-v2.5` |
| Alibaba Qwen API | `qwen3.7-max`、`qwen3.7-plus`、`qwen3.6-flash` |

静态模型错误必须在SubscriptionAuthorizationResolver、RuntimeBindingResolver、SecretResolver、Factory和Provider之前拒绝。大小写、空白、前缀、通配符和未登记别名均不近似匹配。

## 3. Catalog激活事务

`catalog.ApplyActivationPlan`接收`ActivationPlan`，每项必须钉住精确RouteID、EvidenceDigest和AdapterID。入口复制计划slice，限制最多256项，并只在报告中输出经过校验的Catalog公开事实。

固定语义：

1. 完整预检所有项；
2. 激活只允许`callable=false + trusted_subscription_authorization_resolver + implemented_offline + AdapterID`的订阅Route；
3. `terms_blocked`、`official_client_only`、过期/不可用evidence、研究记录和无Adapter记录没有force override；
4. 任一失败返回`nil Catalog + Applied=false report`，零项生效；
5. 成功产生新immutable Catalog；Disable恢复订阅host requirement；
6. canonical `AuditDigest`不包含SecretMaterial、Credential值、Entitlement或未经校验的pin文本。

GLM Coding Plan继续不可激活。官方只允许列出的工具/产品，Praxis不在名单；当前CN Endpoint没有与global来源闭合，因此记录为`unverified + research_only + official_client_only`。

## 4. Gateway宿主构造事务

新增`routegateway.HostConfig`与`NewHost`，把BaseCatalog、ActivationPlan、BindingResolver、SecretResolver、SubscriptionAuthorizationResolver、FactoryRegistry、Clock和HTTPClient收敛为一个构造边界。

固定顺序：

```text
HostConfig结构/typed-nil检查
  -> 以当前时间重新校验BaseCatalog
  -> ApplyActivationPlan
  -> 完整生成candidate报告
  -> 每条callable Route检查Factory
  -> 每条callable订阅Route检查本次计划审计
  -> 检查可信SubscriptionAuthorizationResolver
  -> 构造Gateway/Policy
  -> Ready=true
```

失败不返回Gateway；`HostBuildReport`始终带稳定`FailureCode`与`AuditDigest`。宿主需要用新Catalog+新Gateway做原子替换，再关闭旧Gateway；本模块不实现Runtime级热切换。

## 5. Wire认证纠正

- DeepSeek Chat：`Authorization: Bearer`；DeepSeek Messages：`x-api-key`，两条Route使用独立Credential Profile；
- Kimi Code Chat：Bearer；Kimi Code Messages：官方`ANTHROPIC_API_KEY`路径，使用`x-api-key`；
- MiniMax Token Plan Chat：Bearer；Messages：`x-api-key`；
- MiMo与Alibaba订阅Messages继续使用官方允许的Bearer路径；
- Catalog Profile、Provider transport和HTTP fixture必须三方一致。

## 6. 真实烟测入口

- 保留现有Provider直连smoke；
- 新增十家P0 Gateway smoke，验证真实RouteID经过Catalog、HostConfig、Binding/Secret Resolver、内建Factory与Gateway；
- 新增Kimi Code与MiniMax Token两类P1 Gateway smoke；它们的官方范围允许真实第三方交互式编码工具；
- MiMo Token、Alibaba Coding与Alibaba Token Team虽然保留离线Adapter、激活合同和Gateway黑盒测试，但官方条款明确禁止自动脚本/API测试器，因此不提供会发真实请求的自动live smoke，也不得把测试器伪装成交互式客户端；
- 只有`PRAXIS_LIVE_TESTS=1`和对应家族开关同时开启后才读取显式Key/Route/Model；双开关已开启但参数缺失必须失败，不能Skip成假通过；
- SecretResolver钉住精确RouteIdentity与CredentialProfile；响应必须包含约定marker；默认离线流程只编译这些入口。

## 7. 明确延期与需用户决定

- Anthropic Agent SDK允许第三方应用消耗订阅额度，但这是独立P1 SDK/sidecar/auth合同，不是Messages API。是否在`model-invoker`新增该bridge需用户确认；当前只在控制资产分轴记录；
- Gemini Interactions已是新项目推荐native面，GenerateContent仍受支持。Interactions的typed steps、server state和SSE作为P2独立设计；
- xAI当前P0只实现Responses文本与client-side function slice；官方Hosted Web/X Search与Code Execution属于P2能力，不得由现有`tool_calling`声明冒充，需独立能力与安全设计；
- Alibaba DashScope native、xAI gRPC/vendor SDK、Z.AI vendor-native SDK均为P2，不把兼容SDK冒充native；
- `kimi-for-coding-highspeed`需要Allegretto及以上tier事实，未把它无条件加入普通会员Route；
- 没有真实账号结果时所有Route继续保持`implemented_offline`，不升级`live_verified`。

## 8. 验收

- 黑盒：Catalog激活、Host构造、exact model、wire auth、blocked Route和gated smoke行为；
- 白盒：原子性、TOCTOU防护、typed-nil、报告完整性/脱敏、零下游触达、Factory/Resolver顺序；
- 自动资产：Provider Matrix、语义矩阵、缓存事实必须由DefaultDocument重生且无漂移；
- 全量：统一离线脚本、shuffle、race、integration仅编译、相关fuzz和全仓覆盖率实测。

## 9. Factory双层信任补充

宿主路径通过不能替代公开Provider直用安全性。18个Builtin Factory、14个默认活跃Adapter与4个受限订阅Factory的A/B层Endpoint、Credential audience、响应Model和生命周期合同，以[Factory双层信任矩阵设计](./factory-trust-matrix-v1.md)为准；该矩阵由live Catalog/Registry生成或核对。
