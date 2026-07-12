# 模型调用器模块说明

## 1. 模块作用

`model-invoker` 是 Praxis Runtime 与模型 Provider 之间的稳定调用边界。它解决四个问题：

1. Runtime 不依赖任何厂商 SDK 类型；
2. 调用前明确判断能力、协议、模型与端点边界；
3. 所有转换、降级和拒绝都可以审计；
4. 超时、重试、流生命周期、错误和 Raw 数据由统一规则控制。

代码位于 `ExecutionRuntime/model-invoker/`。第二阶段、第三阶段A-E1、上游调用最终候选A→F、信任闭合、宿主激活再验证与Factory A/B双层信任闭合均在同一模块内实施。根包 `RouteInvoker`负责Policy/Authorization/Audit；`catalog.ApplyActivationPlan`负责原子激活快照；`routegateway.NewHost`负责宿主依赖、工厂、可信订阅Resolver和Gateway就绪事务。机器资产包括39×20语义矩阵、39条Provider缓存事实和18行Factory双层信任矩阵；16条订阅Route保留实现但默认host-blocked。

## 2. 当前产物

| 产物 | 位置 | 内容 |
|---|---|---|
| 设计 | `.properties.rax/design/model-invoker/` | 架构、统一语义 v1、RouteID门面、缓存传输边界和 Provider调查 |
| 计划 | `.properties.rax/plan/model-invoker/` | 各阶段陈旧计划及已完成的上游调用与统一封装 v1计划 |
| Go module | `ExecutionRuntime/model-invoker/` | 统一内核、十四个 Runtime Provider、上游控制面与 RouteID调用门面 |
| Route Policy/Audit | `ExecutionRuntime/model-invoker/route_invoker.go` | 对预构造 Invoker执行Resolve/Invoke/Stream、选择器所有权、调用时 evidence、Offering/entitlement和 RouteError |
| Route Gateway | `ExecutionRuntime/model-invoker/routegateway/` | HostConfig/NewHost、RouteID策略预检、RuntimeBindingResolver、SecretResolver、18工厂、单飞池/Lease及完整失败报告 |
| Factory信任矩阵 | `ExecutionRuntime/model-invoker/internal/trustmatrix/`、`tests/trustmatrix/` | 18个固定v1candidate builtin的A/B层×protocol/profile机器合同、live Version/Route全集、Go AST代码证据与verification-mode测试证据门禁；不声称支持Factory实例热替换 |
| Route与 Credential | `ExecutionRuntime/model-invoker/upstream/` | 七维身份、Endpoint解析、Credential引用与绑定、使用策略和 MappingReport |
| Catalog与 Schema | `ExecutionRuntime/model-invoker/catalog/` | 39条默认callable Binding、16条host-blocked已实现订阅Route、7条研究/控制记录、证据、严格编解码、版本化 JSON Schema和 Markdown生成/校验 |
| 共享脚手架 | `ExecutionRuntime/model-invoker/internal/adaptercore/` | Endpoint、能力、Raw、Header、无跳转、响应捕获和脱敏 |
| 协议基础 | `ExecutionRuntime/model-invoker/internal/protocol/` | Binding、Driver、Dialect、Failure归一化和强制身份边界 |
| Chat协议驱动 | `ExecutionRuntime/model-invoker/internal/protocol/openaichat/` | Chat mapping、SDK窄缝隙、normalize、安全 Failure和流状态机 |
| Responses协议驱动 | `ExecutionRuntime/model-invoker/internal/protocol/openairesponses/` | typed Items、服务端 continuation、normalize、安全 Failure和独立流状态机 |
| Messages协议驱动 | `ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages/` | signed/redacted thinking、tool continuation、normalize、安全 Failure和 content-block流状态机 |
| GenerateContent协议驱动 | `ExecutionRuntime/model-invoker/internal/protocol/geminigenerate/` | JSON Schema、thought signature continuation、normalize、安全 Failure和迭代流状态机 |
| 核心测试 | `ExecutionRuntime/model-invoker/tests/core/` | 外部包黑盒契约、并发、fuzz 与目录结构门禁 |
| Provider 测试 | `ExecutionRuntime/model-invoker/tests/{openai,anthropic,gemini}/` | 官方 SDK + HTTP/JSON/SSE 离线黑盒与协议测试 |
| 波次 A测试 | `ExecutionRuntime/model-invoker/tests/{upstream,catalog,catalogassets}/` | Route/Credential/Catalog/Schema/Markdown、AdapterID和三项新增 fuzz |
| 波次 B0测试 | `ExecutionRuntime/model-invoker/tests/protocol/` | 非原厂身份、typed-nil、安全 Failure、流生命周期与 AST边界反例 |
| 波次 B1测试 | `ExecutionRuntime/model-invoker/tests/protocol/openaichat/` | 非原厂 driver、映射、错误、流、身份与两项 fuzz |
| 波次 B2测试 | `ExecutionRuntime/model-invoker/tests/protocol/openairesponses/` | typed Items/State、非原厂 driver、native sequence与两项 fuzz |
| 波次 B3测试 | `ExecutionRuntime/model-invoker/tests/protocol/anthropicmessages/` | 非原厂 driver、完整 provider continuation、流终态与两项 fuzz |
| 波次 B4测试 | `ExecutionRuntime/model-invoker/tests/protocol/geminigenerate/` | 非原厂 driver、thought signature State、流去重与两项 fuzz |
| 波次 B5/B6测试 | `ExecutionRuntime/model-invoker/tests/{core,protocol}/` | 共享 Failure提取、context sentinel、Provider旧提取器反例与公共 SDK签名 AST门禁 |
| 波次 C测试 | `ExecutionRuntime/model-invoker/tests/{upstream,catalog,catalogassets}/` | entitlement状态、Key前缀、到期/额度/HTTP拒绝、BillingPlan、22条 Catalog记录和泄密 fuzz |
| 云 Provider | `ExecutionRuntime/model-invoker/provider/{bedrockmantle,bedrockruntime,vertex,azureopenai}/` | Mantle/Runtime、Vertex Gemini/Claude/Chat、Azure v1/legacy及独立 Credential/Deployment边界 |
| Bedrock协议 | `ExecutionRuntime/model-invoker/internal/protocol/bedrock/` | Converse可移植 Agent语义、InvokeModel provider-native Raw、AWS错误和两种流状态机 |
| 波次 D测试 | `ExecutionRuntime/model-invoker/tests/{bedrockmantle,bedrockruntime,vertex,azureopenai}/`、`tests/protocol/bedrock/` | 本机 SDK HTTP fake、SigV4/bearer、ADC/API Key、Entra刷新、错配和流 |
| 兼容 Provider组合 | `ExecutionRuntime/model-invoker/internal/compatprovider/` | 协议 SDK transport、driver组合、身份恢复和凭据脱敏，不拥有厂商能力判断 |
| DeepSeek直连 | `ExecutionRuntime/model-invoker/provider/deepseek/` | Chat/Messages独立 Binding、精确 v4模型、reasoning方言与错误分类 |
| 波次 E1-DeepSeek测试 | `ExecutionRuntime/model-invoker/tests/deepseek/` | 本机 SDK HTTP/SSE fake、模型静默映射反例、reasoning、流、脱敏和 fuzz |
| Kimi按量直连 | `ExecutionRuntime/model-invoker/provider/kimi/` | K2/Moonshot文本模型、thinking方言、按量与 Code会员隔离 |
| 波次 E1-Kimi测试 | `ExecutionRuntime/model-invoker/tests/kimi/` | SDK HTTP/SSE fake、当前/下线模型、preserved thinking拒绝、quota、流和 fuzz |
| Z.AI按量直连 | `ExecutionRuntime/model-invoker/provider/zai/` | GLM文本、thinking/request_id、业务错误、专属终态和 Coding Plan隔离 |
| 波次 E1-Z.AI测试 | `ExecutionRuntime/model-invoker/tests/zai/` | SDK HTTP/SSE fake、模型/Offering、终态、错误矩阵、流和 fuzz |
| MiniMax按量直连 | `ExecutionRuntime/model-invoker/provider/minimax/` | Messages主路径、Chat/Responses、M3/M2.x thinking、累积流和 Token Plan隔离 |
| 波次 E1-MiniMax测试 | `ExecutionRuntime/model-invoker/tests/minimax/` | 三协议 SDK HTTP/SSE fake、continuation、无服务器 State、Offering边界和 fuzz |
| MiMo按量直连 | `ExecutionRuntime/model-invoker/provider/mimo/` | Messages主路径、Chat、V2.5 thinking、专属终态和 Token Plan隔离 |
| 波次 E1-MiMo测试 | `ExecutionRuntime/model-invoker/tests/mimo/` | 两协议 SDK HTTP/SSE fake、continuation、模型/Key/Endpoint边界和 fuzz |
| Qwen/百炼按量直连 | `ExecutionRuntime/model-invoker/provider/qwen/` | 北京/新加坡 Workspace专属 Responses/Chat、thinking、server state与订阅隔离 |
| 波次 E1-Qwen测试 | `ExecutionRuntime/model-invoker/tests/qwen/` | 双 Region、双协议 SDK HTTP/SSE fake、Workspace/Key/模型边界和 fuzz |
| RouteID门面测试 | `ExecutionRuntime/model-invoker/tests/routefacade/` | 39条默认callable解析、selector/static model/evidence、订阅可信claim正反例、流与设计资产门禁 |
| Route Gateway测试 | `ExecutionRuntime/model-invoker/tests/routegateway/` | 默认Route与可信激活订阅Route的真实工厂构造、零触达顺序、轮换、单飞、超时、流Lease、Close/Endpoint/Closer与Secret泄密fuzz |
| 语义矩阵 | `ExecutionRuntime/model-invoker/semanticmatrix/`、`tests/semanticmatrix/` | 780行Route×能力机器合同、确定性CSV和真实Gateway Capabilities逐项漂移门禁 |
| Provider缓存事实 | `ExecutionRuntime/model-invoker/cachefacts/`、`tests/cachefacts/` | 39条Route×14默认活跃Adapter×6协议的传输事实、确定性CSV与零策略所有权门禁 |
| 订阅Adapter测试 | `ExecutionRuntime/model-invoker/tests/plancompat/` | 4类计划、2协议、真实User-Agent、Key/host、HTTP/JSON/SSE与反例 |
| 真实烟测入口 | `ExecutionRuntime/model-invoker/tests/integration/` | Provider直连、十家P0 Gateway与Kimi Code/MiniMax Token两类P1的显式双开关入口；MiMo/Alibaba禁止测试器，仅保留离线验证 |
| 统一离线入口 | `ExecutionRuntime/model-invoker/scripts/verify-offline.sh` | 屏蔽真实凭据后执行格式、模块、静态、普通、shuffle、race、integration仅编译和 Catalog资产门禁 |
| CI | `.github/workflows/model-invoker.yml` | 调用统一离线入口，不执行真实 API或真实套餐 |
| 使用说明 | `ExecutionRuntime/model-invoker/README.md` | 公共 API、示例、安全与验证命令 |

## 3. 输入与输出

统一输入是 `modelinvoker.Request`：Model、消息/函数项、工具、输出约束、推理、状态、流开关、预算、Metadata和 ProviderOptions。生产调用先用`ActivationPlan + HostConfig`构造immutable Gateway；调用时Provider、Protocol和Endpoint必须留空，由`RouteCall.RouteID`唯一绑定。十家直连P0模型使用Catalog exact集合；订阅Route禁止调用方自带Invocation/Entitlement，必须由宿主可信`SubscriptionAuthorizationResolver`生成。

非流式输出是 `RouteResponse{Response, RouteSelection}`；流式输出是带 `Route()`审计投影的 `RoutedStream`和有序 `StreamEvent`。统一 Response仍保留结果、用量、Request ID、Provider Metadata、MappingReport和受控 Raw；RouteSelection并列保留 RouteID、七维身份、evidence digest、Adapter与 PolicyDecision。底层 Provider调用仍可直接使用基础 `Invoker`。

## 4. 关键组成

- `model.go`：统一请求、响应、状态和校验；
- `capability.go`：能力需求、四级支持和映射报告；
- `registry.go`：并发安全的 Provider 注册；
- `invoker.go`：路由、预算、非流式重试和流生命周期；
- `route_invoker.go`：RouteID绑定、离线预检、Offering/entitlement、禁止 PAYG与 Route审计；
- `routegateway/`：完整Route执行组合、具体协议Endpoint回读、秘密/绑定边界、18工厂和实例Lease生命周期；
- `internal/trustmatrix/`：从live Catalog/Factory Registry构建A/B双层信任合同并生成严格18行CSV与展开Markdown；
- `errors.go`：统一错误；
- `raw.go`：默认脱敏的审计载荷；
- `internal/adaptercore/`：所有 HTTP Provider复用的 SDK 无关安全与审计基础；
- `provider/openai/`：配置、SDK transport、Capabilities、Responses/Chat方言验证和错误分类；
- `provider/anthropic/`：Messages配置、SDK transport、Capabilities、方言验证和错误分类；
- `provider/gemini/`：Developer API配置、SDK transport、Capabilities、方言验证和错误分类；
- `internal/testkit/providercontract/`：直连 Provider共同执行的公共契约；
- `upstream/`：Model Family、Provider、Offering、Deployment、Protocol、Endpoint与 Credential七维 Route身份、策略和 MappingReport；
- `catalog/`：机器可读 Catalog、版本化 Schema、证据 TTL/摘要/状态迁移、资产校验和 Markdown当前 Binding生成；
- `internal/protocol/`：SDK中立的 Binding、Driver、Dialect、Failure、错误归一化与 identity-bound stream；
- `internal/protocol/openaichat/`：Chat Completions请求映射、官方 SDK窄客户端、响应归一化、安全 Failure提取和独立流状态机；
- `internal/protocol/openairesponses/`：Responses typed Items、continuation、官方 SDK窄客户端、响应归一化和独立 SSE状态机；
- `internal/protocol/anthropicmessages/`：Messages signed/redacted thinking、tool continuation、官方 SDK窄客户端、响应归一化和 content-block SSE状态机；
- `internal/protocol/geminigenerate/`：GenerateContent JSON Schema、thought signature continuation、官方 SDK窄客户端、响应归一化和迭代流状态机；
- `internal/protocol/bedrock/`：Converse文本/工具/用量/流映射与 InvokeModel受控 Raw边界；
- `provider/bedrockmantle/`：Mantle Responses/Chat/Messages、API Key刷新与 `bedrock-mantle` SigV4；
- `provider/bedrockruntime/`：AWS SDK v2 Converse/Invoke、bearer刷新与 Runtime SigV4；
- `provider/vertex/`：Vertex Gemini、Claude `rawPredict`、OpenAI Chat和 ADC/API Key；
- `provider/azureopenai/`：Azure v1/legacy、deployment name、API Key与 Entra刷新；
- `internal/compatprovider/`：为官方明确兼容路线组合 OpenAI/Anthropic协议 driver，不拥有 Provider方言或商业身份；
- `provider/deepseek/`：DeepSeek Chat/Messages、精确 v4模型、thinking/reasoning和静默模型映射门禁；
- `provider/kimi/`：Kimi开放平台按量 Chat、K2 thinking、Moonshot文本和 Kimi Code Offering隔离；
- `provider/zai/`：Z.AI按量 Chat、GLM thinking、body request ID、业务错误与 Coding Plan隔离；
- `provider/minimax/`：MiniMax按量 Messages/Chat/Responses、M3/M2.x thinking、累积流与 Token Plan隔离；
- `provider/mimo/`：Xiaomi MiMo按量 Messages/Chat、V2.5 thinking、专属终态与 Token Plan隔离；
- `provider/qwen/`：Alibaba Model Studio北京/新加坡按量 Responses/Chat、Workspace专属 Endpoint、Qwen thinking与 Coding/Token Plan隔离；
- `scripts/verify-offline.sh`：本地与 CI共用的统一离线验收入口。

## 5. 依赖关系

- 直接依赖：Go 标准库、官方 `github.com/openai/openai-go/v3 v3.41.1`、`github.com/anthropics/anthropic-sdk-go v1.56.0`、`google.golang.org/genai v1.63.0`、`github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.55.0`，以及官方认证支撑模块；
- 被依赖方：未来的 Praxis Runtime 与 Agent Run Engine；
- 当前不存在：TypeScript Sidecar、矩阵中的其他 Provider、Rust 计算层。

SDK 仅存在于内部 Provider transport与具体协议 driver包；根包公开 API只使用标准库和 Praxis自有类型。

## 6. 已验证行为

- Request 校验、能力排序、全量拒绝报告与显式降级；
- Registry 并发注册/读取和 typed-nil 防护；
- 超时覆盖能力查询与调用，取消可传播；
- 非流式重试单点所有权、Retry-After 最低等待语义；
- 流不重放、EOF 终态、底层 Close 和 Close 幂等；
- OpenAI Responses/Chat、Anthropic Messages、Gemini GenerateContent 的请求路径、认证、请求体、响应、工具、用量和 Request ID；
- 直连与云 Provider的流事件顺序、工具参数、thinking/signature、usage、唯一终态、错误和失败审计；
- Strict 未指定时保留协议默认；
- Raw/State/NativeEvents 默认脱敏且防御性复制；
- 所有 SDK HTTP客户端拒绝 3xx，malformed 2xx保留脱敏原始 body；
- 所有 HTTP响应统一受 8 MiB解压后硬上限保护，超限不可重试；
- 跨 Text/Reasoning/Arguments delta 的 secret 分片会状态化清洗并在终态前安全 flush；
- SDK 错误、Authorization、X-Api-Key、X-Goog-Api-Key 不穿透 Provider 边界；
- Anthropic Prompt Cache 创建为 Unsupported，continuation 不能借原生字段绕过；
- Gemini ID-less/有 ID 工具快照、后到签名、签名冲突与 continuation 多语义校验；
- 所有 `_test.go` 只位于独立 `tests/` 树；
- 39条默认callable Route全部能经 `Resolve`精确绑定；16条订阅Route默认在Binding/Secret/Factory/Provider前受host trust阻塞；
- caller selector、non-callable控制记录、过期 evidence、错误 static model、缺/失效 entitlement均在 Provider前拒绝；有效订阅 fixture保持禁止自动 PAYG；
- `RoutedStream`保留 Route选择、事件顺序、终态错误和关闭语义；统一语义/Route门面版本 ID与三份设计资产有自动门禁；
- 39条默认callable Binding与14个活跃Runtime AdapterID严格对应；Registry另保留4个订阅Adapter，只允许可信宿主激活，云和订阅Credential/Region/Offering/ClientIdentity不互相折叠；
- ActivationPlan按精确RouteID/evidence/adapter原子应用，任一失败零部分生效；HostBuildReport在失败时仍保留完整候选列表、FailureCode和审计摘要；
- 十家直连P0使用官方exact模型集合；DeepSeek/Kimi Code/MiniMax Messages的Catalog认证与真实wire header一致；
- 十家P0与条款允许的Kimi Code/MiniMax Token两类P1已有Gateway级真实烟测入口；MiMo/Alibaba三类没有自动真实smoke，本轮也没有读取Key或执行真实请求；
- DeepSeek/Kimi/MiniMax/Z.AI公共构造器只接受官方远端Endpoint或loopback测试地址；Gateway统一校验流/非流实际响应模型，阻断任意Host外发与服务端静默模型漂移；
- Catalog严格 Schema、证据 TTL/摘要/失效、状态迁移、重复/冲突来源、缺失证据、`terms_blocked`可调用、资产路径与 Markdown漂移反例；
- 统一离线入口实际通过，CI已接入同一入口；覆盖 `go mod verify`、gofmt、tidy diff、`git diff --check`、vet、普通、shuffle、race、integration仅编译和 Catalog资产校验；
- 原有9项 fuzz之外，波次 A新增并完成3秒验收：`FuzzEndpointResolutionNeverReturnsUnsafeURL`、`FuzzCatalogDecodeValidateAndDigest`、`FuzzCatalogArtifactPaths`。
- 波次 B0以非原厂 fake证明 Provider/Protocol/Endpoint由 Binding注入，覆盖请求/State错配预调用拒绝、SDK cause隔离、context sentinel、Raw复制、流事件/终态/Close、红后 stamp顺序、typed-nil和 AST边界；统一离线入口在修复 `Binding.StampError(nil)` 后实际通过。
- 波次 B1以非原厂 fake直接运行 Chat driver，覆盖无状态门禁、完整 HTTP错误矩阵、SDK cause隔离、流顺序/usage/终态/Close、body-limit、Provider ID与密钥重合、Endpoint密钥片段脱敏，以及两项3秒 fuzz；相关合并覆盖率为77.9%。
- 波次 B2以非原厂 fake直接运行 Responses driver，覆盖 typed message/function/reasoning Items、`previous_response_id`、State身份、response.failed分类、native sequence、usage终态和两项3秒 fuzz；相关合并覆盖率为73.4%。
- 波次 B3以非原厂 fake直接运行 Messages driver，覆盖 signed/redacted thinking、direct tool-use continuation、State/Response/MappingReport身份、SDK cause隔离、流顺序/usage/终态/Close和两项3秒 fuzz；相关合并覆盖率为75.1%。
- 波次 B4以非原厂 fake直接运行 GenerateContent driver，覆盖 thought signature、native/语义工具 ID、State/Response/MappingReport身份、SDK cause隔离、重复流快照去重、usage/终态/Close和两项3秒 fuzz；相关合并覆盖率为72.9%。
- 波次 B5/B6统一四协议安全首轮提取，删除三家 Provider旧 SDK错误/Raw实现，新增公共 SDK签名与旧提取器 AST反例；统一离线入口、20项 fuzz和 `-coverpkg=./...`全量覆盖均通过，合并覆盖率为81.0%。
- 波次 C新增 `EntitlementState`、稳定的401/402/403/429拒绝、专属 Key前缀、禁止自动 PAYG与 `BillingPlanReference`；Catalog扩展为22条但 callable仍为4，新增泄密 fuzz与两个扩展 Catalog fuzz均通过。
- 波次 D新增四个云 Adapter、两个 Bedrock协议、21条 callable云 Binding和5条云控制记录；Catalog扩展为48条，其中25条 callable、23条控制记录。本机 SDK fake与全量普通测试已通过，真实烟测只编译。
- 波次 E1首家 DeepSeek新增 Chat/Messages两条 callable Binding；保留 `thinking/reasoning_content`，精确限定当前 v4模型并阻断未知/Claude别名静默映射。Catalog扩展为50条，其中27条 callable、23条控制记录；DeepSeek新增 fuzz 3秒通过27,064次。
- 波次 E1-Kimi新增一条按量 Chat Binding；K2.7强制 thinking、K2.6/K2.5开关、Moonshot文本与 Code会员边界均有离线反例。Catalog扩展为51条，其中28条 callable、23条控制记录；Kimi新增 fuzz 3秒通过25,231次。
- 波次 E1-Z.AI新增一条按量 Chat Binding；GLM effort、body/stream request ID、专属 finish reason与 Coding Plan边界均有离线反例。Catalog扩展为52条，其中29条 callable、23条控制记录；Z.AI新增 fuzz 3秒通过19,575次。
- 波次 E1-MiniMax新增 Messages/Chat/Responses三条按量 Binding；thinking/signature continuation、Chat累积流、Responses `store=false` State与 Token Plan Key边界均有离线反例。Catalog扩展为55条，其中32条 callable、23条控制记录；MiniMax新增 fuzz 3秒通过26,436次。
- 波次 E1-MiMo新增 Messages/Chat两条按量 Binding；Bearer Messages、thinking/signature continuation、Chat reasoning流、专属终态与 Token Plan Key/Region边界均有离线反例。Catalog扩展为57条，其中34条 callable、23条控制记录；MiMo新增 fuzz 3秒通过17,774次，全仓覆盖率76.4%。
- 波次 E1-Qwen新增北京/新加坡各 Responses/Chat四条按量 Binding；Workspace专属 Endpoint、`sk-ws-*`/旧 `sk-*`与 `sk-sp-*`拒绝前缀、typed state、thinking、JSON Object和流均有离线反例。Catalog扩展为61条，其中38条 callable、23条控制记录；Qwen新增 fuzz 3秒通过28,342次，全仓覆盖率76.5%。
- 波次 E1-xAI新增 `grok-4.5` Responses按量 Binding；固定 `api.x.ai/v1`、Bearer `XAI_API_KEY`、low/medium/high reasoning、30天 `previous_response_id`、函数工具、parallel控制、流与 `prompt_cache_key`，并拒绝消费者产品、旧模型、legacy Chat、托管工具和非本切片能力。Catalog扩展为62条，其中39条 callable、23条控制记录；xAI新增 fuzz 3秒通过16,417次，全仓合并语句覆盖率76.7%。

MiMo最终命令与结果记录在[第三阶段波次 E1 MiMo完成快照](../../memory/model-invoker/20260711-033547-第三阶段波次E1-MiMo完成.md)中。

Qwen最终命令与结果记录在[第三阶段波次 E1 Qwen完成快照](../../memory/model-invoker/20260711-035627-第三阶段波次E1-Qwen完成.md)中。

实际最终命令与结果记录在[第三阶段波次 A完成快照](../../memory/model-invoker/20260710-222624-第三阶段波次A完成.md)中。

最新信任闭合实现、机器事实与全量回归记录在[Route Gateway信任闭合修正完成快照](../../memory/model-invoker/20260711-163800-Route-Gateway信任闭合修正完成.md)中。

最新宿主激活、十家上游再验证、两棒审查修正与全量回归记录在[宿主激活与十家上游再验证完成快照](../../memory/model-invoker/20260711-183800-宿主激活与十家上游再验证完成.md)中。

最新Factory A/B双层信任矩阵、证据门禁、迟到Build/Closer/Stream身份闭合与全量回归记录在[Factory双层信任矩阵与gap闭合完成快照](../../memory/model-invoker/20260712-020500-Factory双层信任矩阵与gap闭合完成.md)中。

## 7. 当前限制与风险

1. 所有真实 API、真实云账号与真实套餐烟雾测试延期，当前结果只证明离线协议、安全、生命周期和控制面链路；
2. 模型能力会漂移；需要阻断静默映射的短 TTL Provider方言会内置精确切片并在证据刷新时更新；
3. Gemini Developer与 Vertex已经分离实现；Interactions、Live与多候选仍未实现；
4. 多模态、Hosted Tools、Batch、Realtime、后台执行与 Prompt Cache 创建未实现；
5. Catalog当前含39条默认callable Binding、16条带Adapter但host-blocked的订阅Route和7条研究/控制记录；host-blocked不等于默认可调用；
6. 第三阶段最终记录的全仓合并覆盖率为76.7%，原最终候选为77.8%，信任闭合后为77.5%，宿主激活再验证后为78.0%；Factory双层信任与生命周期gap闭合后`-covermode=atomic -coverpkg=./... ./tests/...`重新实测为79.4%，仍只记录现状，尚未设为仓库门禁。
7. RouteInvoker不负责Credential秘密解析、Provider构造和实例生命周期；这些职责已由`routegateway`完成离线实现。生产宿主的真实Resolver接线仍待Runtime联合审核；Runtime Kernel、Context Engine、Model Profile和缓存策略不在本模块范围。

## 8. 下一阶段入口

Route Policy/Audit阶段[设计](../../design/model-invoker/route-invocation-facade-v1.md)、[陈旧计划](../../plan/model-invoker/route-invocation-facade-v1.md)、已完成的[最终候选计划](../../plan/model-invoker/route-gateway-final-candidate.md)、[信任闭合计划](../../plan/model-invoker/route-gateway-trust-closure.md)与[宿主激活计划](../../plan/model-invoker/host-activation-and-upstream-revalidation.md)继续保留。下一步只能从[最终候选审核清单](../../design/model-invoker/final-candidate-review.md)中的联合审核、用户决策或逐Route真实烟测边界重新立项。

真实认证、付费调用、生产评审、Runtime Kernel、Context Engine、Model Profile和缓存策略仍需独立授权；当前离线结果不代表真实模型可用性或生产支持。
