# Model Invoker语义并集矩阵v1候选

## 1. 状态与权威来源

- 状态：`praxis.model-invoker.semantic-matrix/v1candidate`，待Profile、Cache、Context/Runtime消费者联合审核；
- 机器资产：[semantic-matrix-v1candidate.csv](./semantic-matrix-v1candidate.csv)；
- 生成器：`ExecutionRuntime/model-invoker/cmd/semanticmatrixgen`；
- 机器合同：`ExecutionRuntime/model-invoker/semanticmatrix`；
- 运行态复核：`tests/semanticmatrix`用真实Route Gateway、18个已注册内建工厂和39条默认callable Route实际`Capabilities`逐项对照Catalog；
- 当前规模：39 Route × 20项能力 = 780行，覆盖6种协议和14个默认活跃Runtime Adapter；4个订阅工厂只供可信宿主激活后的Catalog使用，不进入默认矩阵。

矩阵不是文档关键词清单。每行锁定Route、Provider、Offering、Adapter、协议、能力、支持级别、映射动作、显式降级要求、evidence digest、代码路径和测试证据。CSV必须由Catalog确定性重生成，手改会被资产门禁拒绝。

## 2. 支持级别与动作

| 支持级别 | 候选动作 | 执行规则 |
|---|---|---|
| Native | exact | 可直接执行并记录精确映射 |
| Compatible | transformed | 可执行，但必须记录协议转换 |
| Partial | degraded | 只有`AllowDegradation=true`才可执行，否则实际动作是rejected |
| Unsupported/Unknown | rejected | 不得静默忽略、猜测或旁路 |

## 3. 当前候选v1可表达面

- 文本Message与Instruction；
- function调用、参数、结果和`is_error`能力判断；
- function Tool、选择方式、并行控制；
- 文本、JSON Object、JSON Schema输出约束；
- reasoning effort/summary/budget；
- server/provider continuation State；
- 同步Response、顺序StreamEvent、统一Error、Usage、MappingReport；
- Adapter命名空间隔离的ProviderOptions与受控RawPayload。

“v1完整”只表示上述Runtime Agent文本切片能无损表达、显式转换、经授权降级或明确拒绝，不表示囊括厂商全部API。

## 4. 公共结构扩展审核

| 当前结构 | 风险判断 | 候选兼容规则 | 当前动作 |
|---|---|---|---|
| `Message.Text` | 直接替换为ContentBlock会破坏现有构造和映射 | 未来保留`Text`，加可选`Content []ContentBlock`；两者互斥或由迁移助手把Text变成单文本块 | v1candidate继续只接受Text；多模态前联合设计，不预造类型 |
| `FunctionResult.Output string` | 直接改成`any`或Block数组会破坏所有协议driver | 保留Output；未来加结构化JSON/Content字段并要求恰好一种表示；旧字符串继续是单文本结果 | 当前不改；结构化工具结果延期 |
| function-only `Tool` | 把现有字段塞入Hosted/Computer/MCP会混淆身份与执行所有权 | 未来增加零值等于function的`Kind`和分型payload；现有字段继续只属于function | Hosted/Computer/MCP/其他动作延期 |
| `OutputItem` | 复用Text承载refusal/citation/multimodal会丢语义 | 只允许新增tag和对应字段；现有Text/FunctionCall/ReasoningSummary不改义 | refusal/citation/structured/multimodal输出延期 |
| `StreamEvent` | 用Native或TextDelta模拟新内容/动作增量会破坏顺序语义 | 新事件类型和分型字段只能加法扩展；`Sequence`与唯一终态不变量保持 | 新内容块/动作增量延期 |
| `State` | 非版本Raw可能被误当跨Route通用状态 | Provider/Protocol身份继续强制；未来payload schema单独版本化，禁止跨Route/Offering复用 | 当前结构足够，无改动 |
| `ProviderOptions` | 可能成为绕过统一语义的逃生舱 | 命名空间必须等于Adapter；每个driver严格验证已知字段；未知字段拒绝 | 当前结构足够，无改动 |
| `Usage` | 新计数直接复用现有字段会造成账务误读 | 只加语义单一的新计数；未知/缺失保持零但不能推导计费结论 | 当前input/output/reasoning/cache-read/cache-write/total足够 |
| `RawPayload` | 可能被误用作普通语义、日志或缓存 | 继续只做受控审计载荷，默认脱敏、有硬上限；不能替代ContentBlock或CacheIntent | 当前结构足够，无改动 |

结论：当前类型没有要求立即进行破坏性修改。确定的防破坏规则是“保留现有字段语义、未来只加tag/可选字段、旧构造继续有效、未知新类型在旧driver中明确拒绝”。在消费者联合审核前提前增加空ContentBlock、Tool Kind或CacheIntent，反而会把未决跨层合同错误冻结，因此本阶段不加公共类型。

## 5. 相邻线程稳定接缝

- Profile线程可稳定消费：`Tool`的function表面、`CapabilityContract`、`MappingReport`和明确拒绝；不得把模型提示词写入ProviderOptions。
- Cache线程可稳定消费：RouteID/Identity、Protocol、AdapterID、Provider原生缓存能力、Usage cache read/write和Provider错误事实；CacheIntent仍需Profile/Context联合决定，model-invoker不生成key或分段。
- Runtime/Context线程可稳定消费：Route Gateway、Request/Response/Stream/Error、State身份与MappingReport；不得依赖Provider SDK类型或SecretMaterial。

## 6. 纳入、延期与决策点

| 能力/路线 | 决定 | 理由 |
|---|---|---|
| 当前六协议文本/工具/reasoning/state/stream/error/usage | 纳入v1候选 | 39条默认callable Route已有Catalog、真实Adapter Capabilities和离线协议测试证据 |
| xAI gRPC | 明确延期 | 当前批准实现是Responses；gRPC没有本轮Go Runtime合同和真实Route证据 |
| Qwen DashScope原生 | 明确延期 | 当前四条Route使用官方OpenAI兼容Responses/Chat；原生语义并集尚未设计 |
| 多模态输入输出 | 明确延期 | 当前公共面是文本切片，需ContentBlock与Profile/Context消费者联合审核 |
| Hosted Tools / Computer / MCP /其他动作 | 明确延期 | 需要分型Tool/Output/Stream以及工具执行所有权，超出model-invoker边界 |
| Batch / Realtime /后台执行 | 明确延期 | 生命周期、调度和异步状态不等于当前Invoke/Stream |
| Meta Llama | 需用户决定 | Meta不是单一可调用Provider；需先选官方/云托管/第三方精确Route、条款和Credential所有者 |
| 跨层`CacheIntent` | 需Profile/Cache/Context联合决定 | 只允许表达上游意图，不得由model-invoker决定key、prefix、分段、存储或淘汰 |

## 7. 已发现并修复的漂移

运行态逐项对照发现并修复：Bedrock InvokeModel误报可移植tool calling与Unknown usage、Azure Responses把部署依赖State误报Compatible、DeepSeek Chat漏记JSON Object部分支持。矩阵门禁现在要求Catalog与实际Provider合同一致。
