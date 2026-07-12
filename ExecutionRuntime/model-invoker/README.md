# Praxis Model Invoker

`model-invoker` 是 Praxis 的 Go 模型调用内核。它向 Runtime 提供稳定的统一语义、能力检查、Provider 注册、超时与重试所有权、流式事件、统一错误和受控审计载荷；厂商 SDK 类型不会越过 Provider 边界。

## 当前状态

- 第二阶段离线实现：完成；
- 第三阶段波次 A：Route、Credential、Catalog、Schema、Markdown与 CI证据门禁已完成离线实现和验收；
- 第三阶段波次 B0：SDK中立的协议 Binding、Driver、Failure与身份边界已完成离线实现和验收；
- 第三阶段波次 B1：独立 Chat Completions driver已完成离线抽取和验收；
- 第三阶段波次 B2：独立 Responses driver已完成离线抽取和验收；
- 第三阶段波次 B3：独立 Anthropic Messages driver及完整 continuation已完成离线抽取和验收；
- 第三阶段波次 B4：独立 Gemini GenerateContent driver及 thought signature continuation已完成离线抽取和验收；
- 第三阶段波次 B5/B6：共享安全错误提取、Provider旧实现清理、全回归与公共 SDK签名 AST门禁已完成离线验收；
- 第三阶段波次 C：动态订阅 entitlement、专属 Key/Endpoint/Region约束、禁止自动 PAYG和18条非 callable控制记录已完成离线验收；
- 最终候选订阅调用面：Kimi Code、MiniMax/MiMo Token Plan、Alibaba Coding/Token Plan共16条Route已离线实现，但默认`callable=false + blocked_by_host_trust`；可信宿主激活后才可进入个人交互门禁；GLM Coding Plan继续official-client-only；
- 第三阶段波次 D：AWS Bedrock Mantle/Runtime、Google Vertex AI、Azure OpenAI v1/legacy已完成离线实现和验收；
- 第三阶段波次 E1：DeepSeek、Kimi、Z.AI、MiniMax、Xiaomi MiMo、Qwen/百炼与 xAI按量路线已完成离线实现和验收；
- 波次 F未发现 Sidecar触发证据；G第三方首批名单与 H真实烟测按当前授权延期；统一离线总验收已通过，第三阶段计划已转为陈旧计划；
- Route Policy/Audit候选：调用时 evidence、订阅 entitlement、禁止自动 PAYG和Route审计已完成；它依赖预构造 Registry/Invoker，不是完整 Gateway；
- Route Gateway候选：`routegateway`已组合运行绑定、类型化秘密解析、十八个内建工厂、可信订阅授权、单飞/轮换/Lease生命周期、入池前Provider/Closer/Endpoint门禁和Route级Resolve/Capabilities/Invoke/Stream；
- 宿主激活合同：`catalog.ApplyActivationPlan`与`routegateway.NewHost`已提供精确RouteID、evidence/adapter pin、原子启用/禁用、完整失败报告与默认fail-closed构造；
- 十家P0模型门禁：所有直连P0 Route已改为官方exact static集合，未知模型在Authorization/Binding/Secret/Factory前拒绝；
- 上游调用最终候选A→F及信任闭合：39×20默认语义矩阵、16条host-blocked订阅Route、39条Provider缓存事实和最终离线总验收均已完成；
- Factory A/B双层信任闭合：18个Builtin Factory、14个默认活跃Adapter与4个订阅Factory均有按protocol/profile展开的Endpoint/Credential/Model/Lifecycle机器合同和AST证据门禁；
- 已实现 Runtime Provider：OpenAI、Anthropic、Gemini、AWS Bedrock Mantle、AWS Bedrock Runtime、Google Vertex AI、Azure OpenAI、DeepSeek、Kimi、Z.AI、MiniMax、Xiaomi MiMo、Qwen、xAI；
- 已实现协议：Responses、Chat Completions、Messages、GenerateContent、Bedrock Converse、Bedrock InvokeModel；
- 锁定主要 SDK：`openai-go/v3 v3.41.1`、`anthropic-sdk-go v1.56.0`、`go-genai v1.63.0`、`aws-sdk-go-v2/service/bedrockruntime v1.55.0`；
- 测试方式：手写 fake、官方 SDK、本机 `httptest`/TLS server、JSON/SSE/AWS event-stream固定样本与 fuzz；
- 真实 API：未执行成功的认证模型调用，保留显式 build tag 烟雾测试入口；
- 生产结论：尚未做真实账号、具体模型和公网容量验证，不能据此声明生产可用。

## 组成

| 位置 | 作用 |
|---|---|
| 根包 `modelinvoker` | 统一请求、响应、能力、错误、Registry、基础 Invoker、RouteID门面和 Stream 契约 |
| `route_invoker.go` | RouteID唯一选择、离线 Resolve、调用时 evidence/Offering/entitlement门禁及 Route审计 |
| `routegateway` | 完整Route执行组合：HostConfig、RuntimeBinding/Secret Resolver、18工厂、AdapterPool/Lease和具体协议Endpoint审计 |
| `internal/trustmatrix` | 从live Catalog/Factory Registry生成18行A/B双层信任合同、FactoryVersion与protocol/profile证据 |
| `semanticmatrix` | 从Catalog生成39条默认callable Route×20能力的v1候选矩阵、支持级别与映射动作机器合同 |
| `cachefacts` | 从Catalog生成39条默认callable Route的Provider缓存传输事实；只描述字段、usage、证据与隔离，不拥有缓存策略 |
| `provider/openai` | OpenAI配置、SDK transport、Capabilities与两种协议方言 |
| `provider/anthropic` | Anthropic配置、SDK transport、Capabilities与方言验证/错误分类 |
| `provider/gemini` | Gemini Developer API配置、SDK transport、Capabilities与方言验证/错误分类 |
| `provider/bedrockmantle` | Bedrock Mantle的 Responses/Chat/Messages、API Key刷新、SigV4与 Project状态边界 |
| `provider/bedrockruntime` | AWS SDK v2的 Converse/Invoke、bearer刷新、SigV4与 Region/model ref边界 |
| `provider/vertex` | Vertex Gemini、Claude、OpenAI Chat及 ADC/API Key、Project/Location/Deployment边界 |
| `provider/azureopenai` | Azure OpenAI v1/legacy、deployment name、API Key与 Entra刷新 |
| `provider/deepseek` | DeepSeek Chat/Messages、精确 v4模型、thinking/reasoning方言和静默模型映射门禁 |
| `provider/kimi` | Kimi开放平台按量 Chat、当前文本模型、thinking方言和 Kimi Code隔离 |
| `provider/zai` | Z.AI按量 Chat、GLM thinking、request_id、业务错误和 Coding Plan隔离 |
| `provider/minimax` | MiniMax按量 Messages/Chat/Responses、M3/M2.x thinking、累积流与 Token Plan隔离 |
| `provider/mimo` | Xiaomi MiMo按量 Messages/Chat、V2.5 thinking、专属终态与 Token Plan隔离 |
| `provider/qwen` | Alibaba Model Studio北京/新加坡 Workspace专属 Responses/Chat、thinking、server state与订阅隔离 |
| `provider/plancompat` | Kimi/MiniMax/MiMo/Alibaba官方订阅的受限Chat/Messages、真实User-Agent和严格Key/host边界 |
| `internal/compatprovider` | 组合官方兼容协议 driver与 SDK transport，不拥有厂商身份或能力判断 |
| `internal/adaptercore` | SDK 无关的端点、能力、Raw、Header、无跳转、响应捕获与脱敏脚手架 |
| `internal/protocol` | SDK中立的协议 Binding、Driver、Dialect、Failure归一化与强制身份边界 |
| `internal/protocol/openaichat` | Chat Completions映射、官方 SDK窄缝隙、响应归一化、安全 Failure提取和流状态机 |
| `internal/protocol/openairesponses` | Responses typed Items、continuation、响应归一化、安全 Failure提取和独立流状态机 |
| `internal/protocol/anthropicmessages` | Messages映射、signed/redacted thinking、tool continuation、安全 Failure提取和 content-block流状态机 |
| `internal/protocol/geminigenerate` | GenerateContent映射、JSON Schema、thought signature continuation、安全 Failure提取和迭代流状态机 |
| `internal/protocol/bedrock` | Bedrock Converse可移植 Agent语义与 InvokeModel provider-native Raw/流边界 |
| `upstream` | 七维 Route身份、Endpoint安全解析、Credential秘密引用与绑定、使用策略和 MappingReport |
| `catalog` | 39条默认callable Binding、16条host-blocked已实现订阅Route、7条研究/控制记录、exact模型、原子ActivationPlan、证据门禁和生成资产 |
| `tests/core` | 只通过公开 API 验证统一内核 |
| `tests/routefacade` | RouteID绑定、39条默认callable解析漂移、订阅可信claim拒绝、evidence、流与版本化资产门禁 |
| `tests/routegateway` | 39条默认Route与16条可信激活订阅Route的真实内建工厂构造、失败顺序、Secret轮换、并发单飞、超时、流Lease、Close/Endpoint/Closer与泄密fuzz |
| `tests/trustmatrix` | 18/14/4与39/16全集、Go AST代码声明、可执行Test签名、verification-mode allow/required registry及生成资产漂移门禁 |
| `tests/semanticmatrix` | 780行生成资产漂移、6协议/14个默认活跃Adapter覆盖及真实Gateway Capabilities逐项一致性 |
| `tests/cachefacts` | 39条Route缓存事实CSV漂移、唯一xAI严格key传输面和零策略所有权门禁 |
| `tests/plancompat` | 四类订阅、Chat/Messages、真实User-Agent、Key/host、HTTP/JSON与SSE离线闭环 |
| `tests/openai`、`tests/anthropic`、`tests/gemini`、`tests/{bedrockmantle,bedrockruntime,vertex,azureopenai,deepseek,kimi,zai,minimax,mimo,qwen,xai}` | 通过公开 API、官方 SDK 与本机 HTTP fake验证十四个 Runtime Provider |
| `tests/integration` | 显式`integration` build tag下的Provider直连、十家P0 Gateway与Kimi Code/MiniMax Token两类P1真实烟测入口；MiMo/Alibaba套餐只做离线验证 |
| `tests/upstream`、`tests/catalog`、`tests/catalogassets` | 波次 A的 Route/Credential/Catalog/Schema/Markdown、AdapterID和 fuzz门禁 |
| `tests/protocol` | 波次 B0的身份注入、Failure安全、流生命周期、typed-nil与 AST边界反例 |
| `tests/protocol/openaichat` | 波次 B1的非原厂 driver契约、映射、错误、流和 fuzz门禁 |
| `tests/protocol/openairesponses` | 波次 B2的 typed Items/State、非原厂身份、native sequence、流和 fuzz门禁 |
| `tests/protocol/anthropicmessages` | 波次 B3的非原厂身份、完整 provider continuation、流和 fuzz门禁 |
| `tests/protocol/geminigenerate` | 波次 B4的非原厂身份、thought signature State、流去重和 fuzz门禁 |
| `tests/core`、`tests/protocol` | 波次 B5/B6的共享错误安全、Provider旧提取器反例和公共 SDK签名 AST门禁 |
| `scripts/verify-offline.sh` | 本地与 CI共用的统一离线验收入口 |

## 第三阶段波次 A基础

`upstream` 已把 Model Family、Provider、Offering、Deployment、Protocol、Endpoint与 Credential固定为独立 Route维度。Credential Profile只保存秘密引用及 audience、Provider、Offering、Deployment、Region、Endpoint约束，不保存明文秘密；Endpoint模板解析拒绝 scheme/userinfo/通配符、未知占位符、路径穿越、查询与片段注入。所有路由决定可以形成不含 SDK类型的 MappingReport。

`catalog` 使用 `praxis.upstream-catalog/v1` Schema保存 Route、官方来源、TTL、证据摘要、能力、错误/流方言、边界、实现状态与测试路径。波次 A最初固定的四条直连 Binding为：

| Route ID | Catalog Provider | Runtime AdapterID |
|---|---|---|
| `openai.direct.payg.responses` | `openai` | `openai` |
| `openai.direct.payg.chat_completions` | `openai` | `openai` |
| `anthropic.direct.payg.messages` | `anthropic` | `anthropic` |
| `google.gemini-developer.payg.generate_content` | `google.gemini-developer` | `gemini` |

AdapterID由测试直接对照现有 Provider公开 `ProviderID`；这保留了 Google Gemini Developer API的 Catalog Provider身份，同时正确映射到 Runtime Registry中的 `gemini` Adapter。Provider Matrix只把当前 Binding区块交给 Catalog生成和漂移校验，其余研究内容不会因此变成可调用路线。

## 第三阶段波次 B0协议基础

`internal/protocol` 提供协议 driver共用的 `Binding`、`Driver`、`Dialect`、`Base`和结构化 `Failure`。Binding中的 Provider是 Runtime Registry/Adapter ID，不是 Catalog商业 Provider ID；每次请求、响应、State、MappingReport、流事件、终态错误与 Close错误都会由 Binding重新注入 Provider、Protocol和 Endpoint，协议源码不得硬编码原厂身份。

Failure只允许经过校验的分类字段进入统一错误。SDK对象、`http.Request`、Credential、未批准的原生 cause和 Raw都不能进入公开 unwrap链；取消与超时只保留 `context.Canceled`或`context.DeadlineExceeded`。B0审计还修复了 `Binding.StampError(nil)` 的 typed-nil接口陷阱，并用直接反例锁定真正的 nil `error`语义。

## 第三阶段波次 B1 Chat Completions driver

`internal/protocol/openaichat` 已接管 Chat Completions的请求映射、官方 SDK窄客户端缝隙、非流响应归一化、Failure安全提取、Raw/usage/tool/finish reason语义和完整流状态机。它只依赖 Binding、Dialect、Praxis类型与具体协议 SDK，不导入 Provider包，也不硬编码原厂 Provider身份。

OpenAI Adapter保留 API Key、Endpoint、HTTP transport、SDK构造、Capabilities与 OpenAI方言。公开组合顺序为 `driver → Redactor → public Binding`：最后一层恢复 Provider和 Protocol权威身份，但 Endpoint使用已经脱敏的公开副本，避免身份恢复重新引入密钥片段。旧的 Chat mapping、normalize和 stream实现已从 Provider包删除；Responses随后已由 B2独立抽取。

## 第三阶段波次 B2 Responses driver

`internal/protocol/openairesponses` 已接管 Responses typed input/output Items、tool/reasoning/structured output映射、服务端 `previous_response_id` continuation、响应状态、Provider声明失败、安全 Failure提取和独立 SSE状态机。State由 Binding最终注入 Provider/Protocol，native sequence在保证单调事件顺序的同时继续保留。

OpenAI Adapter的 Responses路径也使用 `driver → Redactor → public Binding`。旧的 Responses mapping、normalize与 stream实现已从 Provider包删除；OpenAI Provider现在只拥有配置、SDK transport、Capabilities、方言验证、错误分类和响应头白名单。

## 第三阶段波次 B3 Anthropic Messages driver

`internal/protocol/anthropicmessages` 已接管 Messages请求映射、signed/redacted thinking、direct tool-use provider continuation、非流归一化、安全 Failure提取和 content-block SSE状态机。版本化 continuation只保留必须原样回传的思考与工具块，普通文本和未知原生块不会成为不受控的输入通道；State身份最终由 Binding注入。

Anthropic Adapter使用 `driver → Redactor → public Binding`，只保留 Credential、Endpoint、SDK transport、Capabilities和 Anthropic方言。直接 driver测试以非原厂 Binding完成 thinking → tool result → final text往返，并锁定 SDK cause隔离、流终态和 Close幂等；Provider包内旧 mapping、normalize与 stream实现已删除。

## 第三阶段波次 B4 Gemini GenerateContent driver

`internal/protocol/geminigenerate` 已接管 GenerateContent请求映射、JSON Schema、thought signature provider continuation、非流归一化、安全 Failure提取和迭代流状态机。Continuation继续保存 native/语义工具 ID、已响应索引和必须回传的 model/tool/result/text历史，同时拒绝未知字段、不一致索引、伪造生成 ID和不受控原生 Part；State身份最终由 Binding注入。

Gemini Adapter使用 `driver → Redactor → public Binding`，只保留 Developer API Credential、Endpoint、SDK transport、Capabilities和 Gemini方言。直接 driver测试以非原厂 Binding完成 thought → tool result → next response往返，并锁定重复工具快照去重、SDK cause隔离、流终态和 Close幂等；Provider包内旧 continuation、mapping、schema、normalize与 stream实现已删除。

## 第三阶段波次 B5/B6错误与公共边界

四个协议 driver现在统一通过 `BeginFailureExtraction`、`ExtractCommonFailure`和 `BoundedFailureText`处理 context终态、既有公共错误、重定向、transport与 malformed payload。三个 Provider的旧 SDK错误归一化与 Raw提取已经删除；Provider只保留配置期错误构造和方言分类，SDK对象与原生 cause不会进入公共 unwrap链。

新增 AST门禁遍历所有包外可达函数、导出接收者方法、导出类型/字段和值，拒绝 OpenAI、Anthropic或 Gemini SDK类型出现在公共签名；同时拒绝 Provider `errors.go`重新拥有 SDK失败提取。统一离线入口、20项 fuzz与 `-coverpkg=./...`全量覆盖回归均通过，合并语句覆盖率为81.0%。

## 第三阶段波次 C订阅控制面

`EntitlementState`为 Coding/Token Plan保存不含秘密的账户状态快照，绑定 Offering与 Credential Profile，并检查观察窗口、套餐到期、剩余额度、重置时间、暂停和错绑定。授权前仍需通过显式个人、单租户、前台、非生产与真实客户端身份门禁；401/402/403/429只返回稳定拒绝，永不自动替换成 PAYG Key、Endpoint或 Offering。

波次 C结束时 Catalog共有22条记录：4条直连 callable与18条订阅/消费者控制记录。最终候选D阶段实现Kimi Code、MiniMax/MiMo Token Plan和Alibaba Coding/Token Plan共16条受限Adapter；信任闭合审核后默认Catalog不再把调用方claim当可信证明，这16条固定为`implemented_offline + callable=false + blocked_by_host_trust`。当前总数62条：39条默认callable、16条host-blocked、7条研究/控制记录。Alibaba Savings Plan继续只由`BillingPlanReference`表示。

## 第三阶段波次 D云托管 Provider

AWS拆成 `aws-bedrock-mantle`与 `aws-bedrock-runtime`两个 Runtime Adapter。Mantle复用 Responses、Chat和 Messages协议 driver，同时独立处理 Bedrock API Key、短期刷新、`bedrock-mantle` SigV4签名、Project状态与 `store=false`门禁；Runtime使用 AWS SDK v2实现 Converse/ConverseStream和 InvokeModel/流式 Invoke。Converse映射文本、工具、用量、错误与流，InvokeModel不猜测模型私有 JSON，只保留受控 Raw边界。

Vertex使用独立 `google-vertex-ai`身份：Gemini经 Google Gen AI SDK，Claude经 Anthropic SDK的官方 `rawPredict/streamRawPredict` middleware，OpenAI兼容入口仅声明 Chat。ADC/API Key、Project、Location与 serverless/Provisioned/Model Garden Deployment互不混用。Azure使用独立 `azure-openai`身份：v1 Responses/Chat永不追加 `api-version`，dated legacy Chat单独追加；请求 `model`必须等于 deployment name，API Key与 Entra刷新互斥。

Catalog当前62条记录包含39条默认callable Binding。16条订阅Route保留`implemented_offline` Adapter但默认受可信宿主激活门禁阻塞；Provisioned Throughput、self-deployed Model Garden、Foundry其他模型、Claude Platform on AWS、Claude消费者计划、GLM Coding Plan与xAI消费者产品保持非 callable。没有真实账号证据时不会升级为`live_verified`。

## 第三阶段波次 E1重点厂商直连

DeepSeek使用独立 `deepseek`身份：Chat主路径保留官方 `thinking`、`reasoning_effort`与 `reasoning_content`非流/流语义；Messages兼容路径只接受当前 `deepseek-v4-flash/pro`精确模型 ID，拒绝服务端对 Claude别名、未知模型和即将废弃旧别名的静默映射。两协议共享秘密引用但绑定不同 Endpoint与认证放置，不能被 OpenAI或 Anthropic Provider冒领。

`internal/compatprovider`只组合协议 driver、官方协议 SDK transport、Binding和最终脱敏；模型、Endpoint、能力、错误、条款和扩展仍由 DeepSeek Provider方言拥有。它不是“万能 Base URL客户端”，也不会让后续厂商自动继承 DeepSeek能力。

Kimi使用独立 `kimi`按量 Adapter，只绑定 `api.moonshot.cn/v1`与 `MOONSHOT_API_KEY`。K2.7始终 thinking且禁止禁用开关，K2.6/K2.5按统一 reasoning意图映射 `thinking.type`，非流/流 `reasoning_content`进入统一输出。`api.kimi.com/coding`、`kimi-for-coding`和会员Key属于独立`kimi-code`受限Adapter，只在获准的个人交互式客户端合同下调用；按量与会员Key、Endpoint和Offering不能互换。

Z.AI使用独立 `zai`按量 Adapter，只绑定 `api.z.ai/api/paas/v4`与 `ZAI_API_KEY`。GLM-5.2 effort、其他 thinking模型开关、body/stream `request_id`及 `sensitive`、context window、`network_error`终态均有显式映射；`open.bigmodel.cn/api/coding/paas/v4`与 Coding Plan订阅错误保持独立控制面，不触发隐式余额或 Key回退。

MiniMax使用独立 `minimax`按量 Adapter，Anthropic Messages为默认主路径，同时实现 Chat Completions和 Responses。M3与 M2.x按三协议各自的 thinking默认值映射；Messages保留 thinking signature与工具 continuation，Chat把官方累积 reasoning/text流归一化为增量，Responses清除官方 `store=false`路线不具备的服务器 State。`sk-cp-*`仍被按量Adapter拒绝，只能进入独立`minimax-token-plan`受限Adapter。

Xiaomi MiMo使用独立 `xiaomi-mimo`按量 Adapter，Anthropic Messages为默认主路径，Chat Completions为补充路径；官方未公开 Responses，因此不生成虚假 Binding。两条路线只接受当前 `mimo-v2.5-pro`与 `mimo-v2.5`文本切片，thinking只映射启用/禁用，Messages保留 signature与工具 continuation，Chat保留非流/流 `reasoning_content`。`tp-*` Key、三类 Token Plan域名、旧 V2模型、模态与 Web Search均在 HTTP前隔离。

Qwen使用独立 `qwen`按量 Adapter，Responses为默认主路径，Chat Completions为补充路径；北京和新加坡 Workspace专属域名分别形成四条 Route。Responses保留 typed output、reasoning、工具、7天 server state和流；Chat用 `enable_thinking`/`thinking_budget`映射 reasoning并保留 `reasoning_content`与 JSON Object。Credential Schema的拒绝前缀让旧 `sk-*`按量 Key可用时仍能先拒绝 `sk-sp-*`订阅 Key；共享、Trial、Coding/Token Plan域名和跨 Region/Workspace选择均在 HTTP前隔离。

## Route Policy/Audit Invoker

`RouteInvoker`是 Route策略、授权、选择器绑定和审计层。调用方显式选择 `RouteID`，但必须把语义 `Request`中的 Provider、Protocol和 Endpoint留空；该层从活动 Catalog绑定这些字段，并在任何 Provider方法之前完成 callable、evidence、model、Offering和 entitlement检查。它要求调用方预先构造 Registry/Invoker，不解析秘密、不构造 Adapter、不管理实例生命周期。

```go
routeCatalog, err := catalog.NewDefault(time.Now().UTC())
if err != nil {
    return err // 包括内置 evidence 已过期，需要先刷新 Catalog。
}

baseInvoker, err := modelinvoker.NewInvoker(registry)
if err != nil {
    return err
}
routeInvoker, err := modelinvoker.NewRouteInvoker(routeCatalog, baseInvoker)
if err != nil {
    return err
}

routed, err := routeInvoker.Invoke(ctx, modelinvoker.RouteCall{
    RouteID: "openai.direct.payg.responses",
    Invocation: upstream.InvocationContext{
        Usage: upstream.InvocationGeneralAPI,
        Subject: upstream.SubjectService,
        Tenancy: upstream.TenancyMulti,
        Execution: upstream.ExecutionForeground,
    },
    Request: modelinvoker.Request{
        Model: model,
        Input: []modelinvoker.InputItem{
            modelinvoker.MessageInput(modelinvoker.RoleUser, "Hello"),
        },
    },
})
if err != nil {
    return err
}
fmt.Println(routed.Route.RouteID, routed.Response.Text())
```

`Resolve`执行同一套离线 Route与策略预检，但不会调用 `Provider.Capabilities/Invoke/Stream`。non-callable控制记录、过期 evidence、错误 static model、缺失/失效 entitlement均在 Provider前拒绝；该层不自动切换 Route、Key、账号或 PAYG余额。完整执行组合由同 module的 `routegateway`候选层提供。

## Route Gateway

`routegateway.Gateway`是面向上层的完整Route执行组合。上层只提交`RouteCall`；Gateway固定先执行Policy/Audit预检，再解析非秘密运行绑定和类型化Credential引用，按Route选择内建工厂并通过Lease复用Adapter，最后执行实际`Capabilities`、`Invoke`或`Stream`。任何non-callable、evidence、model、policy或entitlement失败都不会触达BindingResolver、SecretResolver、Factory或Provider。订阅Route必须使用经审核激活的Catalog和宿主注入的`SubscriptionAuthorizationResolver`；调用方自带Invocation或Entitlement claim会在Resolver前拒绝。

生产宿主应使用`NewHost`，不要复制测试中的Document手改逻辑。`ActivationPlan`只接受精确RouteID并钉住当前EvidenceDigest与AdapterID；任一项失败时不会返回部分Catalog或Gateway。`HostBuildReport.Ready`才表示完整宿主事务成功，`Activation.Applied`只表示候选Catalog阶段成功。

Pool key只含Route identity/evidence、Credential非秘密轮换版本、Binding版本、Factory版本和可信客户端身份；不含秘密值、秘密摘要或请求内容。Factory结果只有在Provider身份、生命周期Closer、Endpoint非空且scheme/host/base path安全全部通过后才会入池；轮换与关闭错误以不泄密合同聚合。`SecretResolver`与`RuntimeBindingResolver`必须由宿主注入；本包不读取环境Key、ADC、Entra或AWS默认链。

```go
factories, err := routegateway.NewBuiltinFactoryRegistry()
if err != nil {
    return err
}
gateway, buildReport, err := routegateway.NewHost(routegateway.HostConfig{
    BaseCatalog: routeCatalog,
    ActivationPlan: activationPlan, // nil保持所有订阅Route默认关闭。
    BindingResolver: bindingResolver,
    SecretResolver: secretResolver,
    SubscriptionAuthorizationResolver: subscriptionResolver,
    Factories: factories,
})
if err != nil || !buildReport.Ready {
    return err
}
defer gateway.Close()

result, err := gateway.Invoke(ctx, modelinvoker.RouteCall{
    RouteID: "openai.direct.payg.responses",
    Invocation: invocationContext,
    Request: modelinvoker.Request{Model: model, Input: input},
})
```

## 底层 Provider 用法

```go
openAIAdapter, err := openai.New(openai.Config{APIKey: apiKey})
if err != nil {
    return err
}

registry, err := modelinvoker.NewRegistry(openAIAdapter)
if err != nil {
    return err
}

invoker, err := modelinvoker.NewInvoker(registry)
if err != nil {
    return err
}

response, err := invoker.Invoke(ctx, modelinvoker.Request{
    Provider: openai.ProviderID,
    Protocol: modelinvoker.ProtocolResponses,
    Model:    model,
    Input: []modelinvoker.InputItem{
        modelinvoker.MessageInput(modelinvoker.RoleUser, "Hello"),
    },
    Budget: modelinvoker.Budget{
        MaxOutputTokens: 256,
        Timeout:         30 * time.Second,
    },
})
if err != nil {
    // response 仍可能包含 RawResponse、MappingReport 等失败审计数据。
    return err
}
fmt.Println(response.Text())
```

默认协议是 Responses。调用方也可以显式选择 `ProtocolChatCompletions`。

其他原生 Provider 使用同一注册方式：

```go
anthropicAdapter, err := anthropic.New(anthropic.Config{APIKey: anthropicKey})
geminiAdapter, err := gemini.New(gemini.Config{APIKey: geminiKey})
registry, err := modelinvoker.NewRegistry(openAIAdapter, anthropicAdapter, geminiAdapter)
```

Anthropic 默认协议是 `ProtocolMessages`；Gemini 默认协议是 `ProtocolGenerateContent`。

## 统一语义

当前统一语义支持：

- system/developer 指令与文本消息；
- 函数定义、函数调用、函数结果和并行工具调用；
- 文本、JSON Object、JSON Schema 输出约束；
- 推理强度与 Responses 推理摘要；
- OpenAI 服务端 continuation，以及 Anthropic/Gemini 受控 provider continuation；
- Anthropic signed/redacted thinking 与 Gemini thought signature 的防御性往返；
- Gemini tool → result → text → next-user 多轮 continuation；
- 统一 StopReason、reasoning delta、cache read/write usage；
- Token/时间预算、Metadata 和 ProviderOptions 命名空间；
- 文本、工具参数、推理摘要、用量、终态、错误和 Native 流事件；
- Request ID、用量、Provider Metadata、MappingReport 与受控 Raw 数据。

图片、音频、视频、文件、Hosted Tools、Batch、Realtime、后台执行和缓存创建控制不在当前范围内，能力检查会明确拒绝。Gemini Developer API与 Vertex AI由不同 Adapter、Credential和 Endpoint承载，不能交叉配置。

## 能力与降级

能力级别为 `Native`、`Compatible`、`Partial`、`Unsupported`：

- `Partial` 只有在 `Request.AllowDegradation=true` 时才会执行；
- `Unsupported` 始终拒绝；
- 所有决定进入 `MappingReport`；
- 能力拒绝和调用失败时，统一 `Error.MappingReport` 仍可读取；
- OpenAI 模型是否实际拥有某项能力由 API 在调用时最终校验。离线 CapabilityContract 描述的是适配器对指定模型和协议的映射能力，不是实时模型目录。

Provider 差异通过字段级能力或映射决定单独建模，例如：

- Chat Completions 不返回 Responses 风格的 reasoning summary；
- 两种 OpenAI 协议都没有统一的函数结果 `is_error` 标记；
- Anthropic Prompt Cache 创建策略为 Unsupported，仅归一化服务端返回的缓存用量；
- Gemini 原始 thought 不是统一 reasoning summary，多候选与 Vertex AI 均不伪装为已支持。

它们不会被粗粒度能力声明静默吞掉。

## Strict 的含义

`Tool.Strict` 和 `OutputConstraint.Strict` 是指针：

- `nil`：不写入请求，保留所选协议自身的默认行为；
- `&true`：启用严格 Schema，并在发送前检查关键严格约束；
- `&false`：显式关闭严格模式。

这避免了把“未指定”静默改写成 `true`。

## 流式调用

```go
stream, err := invoker.Stream(ctx, request)
if err != nil {
    return err
}
defer stream.Close()

for stream.Next() {
    event := stream.Event()
    switch event.Type {
    case modelinvoker.StreamEventTextDelta:
        fmt.Print(event.TextDelta)
    case modelinvoker.StreamEventError:
        // event.Response 包含已累计的受控失败审计数据。
    case modelinvoker.StreamEventResponseCompleted:
        final := event.Response
        _ = final
    }
}
if err := stream.Err(); err != nil {
    return err
}
```

流是同步迭代器，不创建后台转发 goroutine，不自动重放。EOF 后进入不可逆终态并关闭底层流；`Close` 可重复调用。

## 错误与重试

统一错误覆盖认证、权限/模型不可用、无效请求、不支持、限流、超时、取消、暂时故障、策略拒绝、流中断、映射失败和未知 Provider 错误。

- OpenAI 与 Anthropic SDK 自动重试固定为 0；Gemini 调用已用单次 HTTP 计数验证不会与 Runtime 叠加重试；
- Runtime 只重试 `Retryable=true` 的非流式调用；
- `Retry-After` 是最低等待时间，不会被本地最大 backoff 截短；
- 流式调用永不自动重放；
- 2xx 畸形载荷视为不可重试的 Provider 协议错误；
- 所有已实现 Provider的 SDK错误对象都不会进入公开 unwrap 链，避免泄露 SDK类型、HTTP Request或认证信息。

## Raw 数据安全

`RawPayload` 的普通字符串、Go 格式化、JSON 和 Text 序列化始终只显示 `[REDACTED]`。只有显式调用 `Bytes()` 才会获得防御性副本。

认证头和 API Key 不进入公开 Response、Error、StreamEvent、State、NativeEvents、Metadata、RawRequest 或 RawResponse。所有 SDK HTTP客户端均拒绝全部 HTTP 3xx，避免认证信息被转发到第二跳；非流响应会捕获并回放原始 body，使畸形 2xx 也能留下脱敏审计证据。

所有 Provider 响应都有 8 MiB 解压后硬上限；超限统一返回不可重试的 `response_body_limit_exceeded`。TextDelta、ReasoningDelta 和 ArgumentsDelta 采用跨事件状态化脱敏，调用方拼接相邻分片也不能还原 API Key；安全后缀会在终态或异常 EOF 前 flush。

调用 `Bytes()` 相当于进入受信审计路径，调用方仍必须自行执行租户、隐私和存储策略。

## 离线验证

在本目录执行：

```bash
bash ./scripts/verify-offline.sh
```

该统一入口先执行 `go mod download`与 `go mod verify`，随后屏蔽全部已知 Provider/云凭据和烟测开关，把外部 HTTP代理指向关闭的 loopback端口，再执行 gofmt、`go mod tidy -diff`、`git diff --check`、`go vet`、普通、shuffle、race、integration仅编译和 Catalog资产校验。依赖获取可能访问配置的 Go module proxy；Provider测试只使用 loopback或自定义 transport，不调用真实 Provider API。GitHub Actions也只调用这一入口。

波次 A还实际执行并通过三项独立3秒 fuzz：

- `FuzzEndpointResolutionNeverReturnsUnsafeURL`：349,949次执行；
- `FuzzCatalogDecodeValidateAndDigest`：18,291次执行；
- `FuzzCatalogArtifactPaths`：13,548次执行。

波次 B1另实际执行并通过两项独立3秒 fuzz：

- `FuzzDriverInvokeNeverPanicsOrLosesIdentity`：59,976次执行；
- `FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence`：56,654次执行。

B1相关 `internal/protocol/openaichat + provider/openai`合并覆盖率实际为77.9%；仓库仍未设定百分比硬门禁。

波次 B2另实际执行并通过两项独立3秒 fuzz：

- `FuzzDriverInvokeNeverPanicsOrLosesTypedState`：58,427次执行；
- `FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence`：63,606次执行。

B2相关 `internal/protocol/openairesponses + provider/openai`合并覆盖率实际为73.4%。

波次 B3另实际执行并通过两项独立3秒 fuzz：

- `FuzzDriverInvokeNeverPanicsOrLosesProviderContinuation`：16,343次执行；
- `FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence`：37,964次执行。

B3相关 `internal/protocol/anthropicmessages + provider/anthropic`合并覆盖率实际为75.1%。

波次 B4另实际执行并通过两项独立3秒 fuzz：

- `FuzzDriverInvokeNeverPanicsOrLosesThoughtSignatureState`：22,743次执行；
- `FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence`：18,419次执行。

B4相关 `internal/protocol/geminigenerate + provider/gemini`合并覆盖率实际为72.9%。

波次 B5/B6实际运行全部20项现有 fuzz并通过；`FuzzCatalogArtifactPaths`在连续高并发批次出现一次停止边界超时，单独5秒复验通过34,593次。全仓 `-coverpkg=./...`合并语句覆盖率为81.0%。

波次 E1-MiniMax的 `FuzzMiniMaxSelectionNeverLeaksOrCallsUnknownModel`独立运行3秒通过26,436次。

波次 E1-MiMo的 `FuzzMiMoSelectionNeverLeaksOrCallsUnknownModel`独立运行3秒通过17,774次；最终全仓 `-coverpkg=./...`合并语句覆盖率为76.4%。

波次 E1-Qwen的 `FuzzQwenSelectionNeverLeaksOrCallsUnknownModel`独立运行3秒通过28,342次；该波次当时的全仓 `-coverpkg=./...`合并语句覆盖率为76.5%。随后 xAI完成后的第三阶段最终记录为76.7%。

最终候选F阶段再次运行统一离线入口并通过：gofmt、tidy diff、module verify、vet、普通、shuffle、全仓race、integration仅编译和生成资产门禁全部成功；29个现有fuzz入口逐项运行1秒全部通过。`go test -covermode=atomic -coverpkg=./... -coverprofile=... ./tests/...`合并语句覆盖率实测为77.8%。覆盖率仍只记录现状，未设百分比门禁。

信任闭合修正完成后再次运行统一离线入口并通过；可信claim、Secret前逐Route精确模型、轮换Close汇聚和Factory Provider/Closer/Endpoint入池门禁均有反例。相关3项fuzz各运行3秒并通过，执行12,498、21,444和1,956次；`go test -count=1 -coverpkg=./... -coverprofile=... ./...`合并语句覆盖率实测为77.5%。

宿主激活再验证完成后，统一离线入口再次通过；Catalog、Route Gateway、Qwen与Z.AI相关5项fuzz各3秒通过，全仓合并语句覆盖率实测为78.0%。第二棒审查发现的两项P1已修正：MiMo/Alibaba不再提供违反禁止脚本/API测试器条款的自动真实smoke；`Gateway.Close()`会等待正在进行的首次Factory Build并汇聚晚到Closer的安全错误。兼容Provider任意远端HTTPS Host与Gateway流/非流响应模型漂移也已加入拒绝门禁；修正后的全仓普通/shuffle/race、integration guard与仅编译均通过。

Factory A/B双层信任闭合后，生成资产保持header+18个Factory数据行、14个默认活跃Adapter、4个host-blocked订阅Factory、39+16 Route；代码证据改为完整`path#symbol`的Go AST精确声明门禁，测试证据限制为`tests/**/*_test.go`可执行签名并按verification mode白名单。18个builtin是固定`Version=v1candidate`的值对象，Registry拒绝替换已注册AdapterID，因此不支持Factory实例热替换；Gateway仍会在每次`prepare`重读自定义`Factory.Version()`并纳入pool key。补齐自定义Factory的Provider-derived Closer、post-build cancellation/deadline、callErr+releaseErr、Gateway二次模型错配Event身份与安全Stream Close因果后，统一离线入口、30项fuzz与`-covermode=atomic -coverpkg=./... ./tests/...`均通过，合并语句覆盖率实测79.4%。覆盖率只记录现状，不设百分比门禁。

## 真实烟雾测试边界

`tests/integration/*_smoke_test.go` 默认不会编译，也不会读取环境变量。每家都要求用户明确批准真实调用并同时设置确认开关、API Key 和模型：

新增的`TestDirectRoutesGatewayLiveSmoke`还要求`PRAXIS_LIVE_TESTS=1`，并复用对应Provider开关/Key/Model；`TestSubscriptionRoutesLiveSmoke`只覆盖Kimi Code与MiniMax Token，要求`PRAXIS_LIVE_TESTS=1`、对应套餐开关、套餐Key、精确RouteID和模型。双开关开启后缺任何参数会失败而不是Skip。MiMo/Alibaba条款禁止脚本/API测试器，因此没有自动真实smoke入口。完整变量名见测试表与[宿主激活再验证设计](../../.properties.rax/design/model-invoker/host-activation-and-upstream-revalidation.md)。

- `PRAXIS_OPENAI_SMOKE=confirmed`
- `OPENAI_API_KEY`
- `OPENAI_SMOKE_MODEL`
- `PRAXIS_ANTHROPIC_SMOKE=confirmed`、`ANTHROPIC_API_KEY`、`ANTHROPIC_SMOKE_MODEL`
- `PRAXIS_GEMINI_SMOKE=confirmed`、`GEMINI_API_KEY`、`GEMINI_SMOKE_MODEL`
- `PRAXIS_BEDROCK_MANTLE_SMOKE=confirmed`及精确 AWS Region、Project Ref和模型
- `PRAXIS_BEDROCK_RUNTIME_SMOKE=confirmed`及精确 AWS Region和模型
- `PRAXIS_VERTEX_SMOKE=confirmed`及精确 Project、Location、Deployment Ref和模型
- `PRAXIS_AZURE_OPENAI_SMOKE=confirmed`及精确 Resource Endpoint、Region、Deployment和 Key
- `PRAXIS_LIVE_TESTS=1`、`PRAXIS_DEEPSEEK_LIVE_TESTS=1`、`DEEPSEEK_API_KEY`和精确 `DEEPSEEK_SMOKE_MODEL`
- `PRAXIS_LIVE_TESTS=1`、`PRAXIS_KIMI_LIVE_TESTS=1`、`MOONSHOT_API_KEY`和精确 `KIMI_SMOKE_MODEL`
- `PRAXIS_LIVE_TESTS=1`、`PRAXIS_ZAI_LIVE_TESTS=1`、`ZAI_API_KEY`和精确 `ZAI_SMOKE_MODEL`
- `PRAXIS_LIVE_TESTS=1`、`PRAXIS_MINIMAX_LIVE_TESTS=1`、`MINIMAX_API_KEY`和精确 `MINIMAX_SMOKE_MODEL`
- `PRAXIS_LIVE_TESTS=1`、`PRAXIS_MIMO_LIVE_TESTS=1`、`MIMO_API_KEY`和精确 `MIMO_SMOKE_MODEL`
- `PRAXIS_LIVE_TESTS=1`、`PRAXIS_QWEN_LIVE_TESTS=1`、`DASHSCOPE_API_KEY`及精确 Region、Workspace和模型

获得单次真实调用授权后，只执行对应 Provider 的测试名：

```bash
go test -tags=integration -run '^TestOpenAIResponsesSmoke$' ./tests/integration
go test -tags=integration -run '^TestAnthropicMessagesSmoke$' ./tests/integration
go test -tags=integration -run '^TestGeminiGenerateContentSmoke$' ./tests/integration
go test -tags=integration -run '^TestBedrockMantleResponsesSmoke$' ./tests/integration
go test -tags=integration -run '^TestBedrockRuntimeConverseSmoke$' ./tests/integration
go test -tags=integration -run '^TestVertexGenerateContentSmoke$' ./tests/integration
go test -tags=integration -run '^TestAzureOpenAIResponsesSmoke$' ./tests/integration
go test -tags=integration -run '^TestDeepSeekLiveSmoke$' ./tests/integration
go test -tags=integration -run '^TestKimiLiveSmoke$' ./tests/integration
go test -tags=integration -run '^TestZAILiveSmoke$' ./tests/integration
go test -tags=integration -run '^TestMiniMaxLiveSmoke$' ./tests/integration
go test -tags=integration -run '^TestMiMoLiveSmoke$' ./tests/integration
go test -tags=integration -run '^TestQwenLiveSmoke$' ./tests/integration
go test -tags=integration -run '^TestDirectRoutesGatewayLiveSmoke$' ./tests/integration
go test -tags=integration -run '^TestSubscriptionRoutesLiveSmoke$' ./tests/integration
```

本阶段只执行了 `go test -tags=integration -run '^$' ./tests/integration`，即仅编译、不运行任何测试；没有执行上面的真实烟测测试名。

## 当前限制

- 没有真实云账号、具体云模型、认证成功调用或公网容量结论；
- 没有执行真实套餐或付费调用；Catalog中的 `implemented_offline`不表示生产支持；
- 十家直连P0 Route由Catalog短TTL exact集合门禁；云部署仍按Deployment或宿主绑定验证，未来动态发现必须另做可信Resolver而不能fallback-open；
- 只实现文本、函数、结构化输出、推理、状态等 Agent 核心语义；
- Gemini Interactions/Live、云 Batch、Hosted Tools与 Prompt Cache创建均未实现；
- RouteID门面不实现自动 Route选择、Credential秘密解析、Provider构造、Context Engine、Model Profile或缓存策略；
- 没有 TypeScript Sidecar、尚未实施的其他矩阵 Provider、Rust 或 Agent 编排；
- 覆盖率用于记录现状，仓库尚未设定百分比门禁。

上游调用与统一封装最终候选A→F、信任闭合及宿主激活再验证已完成离线实现、两棒审查、验收和资产同步。稳定实现、待联合审核、明确延期、需用户决定与未运行真实验证的边界见`.properties.rax/design/model-invoker/final-candidate-review.md`；真实认证、付费调用和生产评审继续延期。
