# 第三阶段：完整上游生态落地计划

## 1. 计划状态

- 模块：`model-invoker`
- 计划版本：`v1`
- 创建时间：2026-07-10 21:26 CST
- 当前状态：已完成当前授权范围并转为陈旧计划；A-E1离线实现与验收完成，F未触发，G第三方首批名单与 H真实烟测明确延期
- 实现位置：`ExecutionRuntime/model-invoker/`；若未来触发非 Go执行器，将位于该模块的隔离 Sidecar目录
- 设计依据：`design/model-invoker/provider-phase-3-upstream-ecosystem.md`
- 调查依据：`design/model-invoker/provider-matrix.md` v3

用户已于 2026-07-10 明确授权按照本计划逐一完成全部离线构建，并明确本阶段不使用真实 API或真实套餐。真实 Key、消费者登录态、付费调用与认证成功烟测统一留到全部构建完成后单独授权。

本计划是已按 A→H顺序审计和执行的总计划。每条 Route仍必须先具备设计卡与官方证据；`terms_blocked` 或 `unverified` 路线只能完成 Catalog、策略和离线拒绝行为，不能伪装成可调用 Provider。

## 2. 本阶段完成后会产出什么

完整执行本计划后，仓库将得到：

1. 一个稳定的 `UpstreamRoute` 身份模型，分离 Model Family、Provider、Offering、Deployment、Protocol、Endpoint和 Credential Profile；
2. 机器可读 Upstream Catalog、Schema、Markdown矩阵生成/一致性校验和证据失效检查；
3. 可复用但不抹平方言的 OpenAI Responses、Chat Completions、Anthropic Messages、Gemini GenerateContent与 Bedrock协议驱动；
4. Go统一控制面，以及版本化 TypeScript/Python隔离执行器边界；
5. 订阅计划 `allowed_usage`、到期、配额、Key/Endpoint隔离和禁止自动跨计划扣费策略；
6. AWS Bedrock、Google Vertex AI、Azure OpenAI/AI Foundry等云托管路线；
7. xAI、DeepSeek、Kimi、MiniMax、Qwen、Meta、Z.AI、MiMo等重点直连路线的分波次实现；
8. Kimi Code、MiniMax Token Plan等官方允许的交互式订阅路线，以及 GLM/MiMo/Alibaba受条款阻塞路线的不可调用目录表示；
9. 第三方托管和自托管 Provider的统一接入框架与首批获批路线；
10. 每条路线独立的离线黑白盒测试、协议样本、真实烟测入口、模块说明、项目索引和 memory证据。

最终结果不是“一个万能 Base URL客户端”，而是可以持续新增路线、自动发现证据过期、严格隔离凭据并解释每次路由决定的上游系统。

## 3. 范围与边界

### 3.1 本阶段范围

- `UpstreamRoute`、`CommercialEntitlement`、`ProtocolBinding`、`ModelIdentity`和 `CredentialProfile`；
- `allowed_usage = general_api | interactive_coding_only | official_client_only`；
- Region、Project、Workspace、Resource、Deployment name、model ref和 maturity；
- 机器可读 Catalog与证据状态；
- 协议驱动与 Provider方言分离；
- Go、TypeScript和证据门禁下的 Python执行器；
- 直连 API、官方订阅、云托管、第三方托管和自托管五类路线；
- OpenAI Responses/Chat、Anthropic Messages、Gemini GenerateContent、Bedrock Converse/Invoke；
- API Key、OAuth、ADC、Entra ID、SigV4、Bedrock bearer等凭据类型；
- 订阅到期、配额耗尽、条款阻塞、余额隔离和禁止隐式回退；
- Provider能力、错误、流事件、用量、Raw、取消、超时和重试；
- 离线协议/契约测试与显式启用的真实烟测入口；
- 矩阵 TTL、链接/来源、模型兼容表和 SDK版本持续维护。

### 3.2 本计划不授权

- 一次性实现全部候选 Provider；
- 使用任何现有消费者订阅、浏览器登录态或未明确提供的真实 Key；
- 冒充 Claude Code、OpenClaw、官方 CLI或其他受支持工具；
- 绕过 User-Agent、客户端身份、个人使用、生产用途或账号共享限制；
- 自动从订阅额度切换到按量余额、另一 Key或另一账号；
- 对外声明生产支持、SLA、容量或成本结论；
- 图像、视频、音乐、完整实时语音和 Agent编排的统一实现；
- 未经独立设计的社区 SDK、聚合平台或消费者网页登录逆向 API；
- 破坏第一、第二阶段已验证的 OpenAI、Anthropic、Gemini直连行为。

## 4. 目标代码与资产结构

以下结构是计划产物，实施前可在审核中调整：

```text
ExecutionRuntime/model-invoker/
├── catalog/
│   ├── schema/
│   ├── routes/
│   └── validator/
├── upstream/
│   ├── route/
│   ├── entitlement/
│   ├── credential/
│   └── resolver/
├── internal/protocol/
│   ├── openairesponses/
│   ├── openaichat/
│   ├── anthropicmessages/
│   ├── geminigenerate/
│   └── bedrock/
├── provider/
│   └── <provider>/
├── sidecar/
│   ├── contract/
│   ├── typescript/
│   └── python/
└── tests/
    ├── catalog/
    ├── protocol/
    ├── provider/
    ├── route/
    ├── sidecar/
    └── integration/
```

公共 Go API仍只能暴露标准库和 Praxis自有类型。目录名称不表示所有子目录必须在第一个波次一次生成；空壳目录和无测试 Provider均禁止。

## 5. 核心技术决定

### 5.1 Route与能力归属

- 能力合同绑定 `Deployment × Protocol`，不是只绑定 Provider或 Model Family；
- `canonical_family`、`provider_model_ref`与 `provider_deployment_name`分开；
- 路由必须返回选择理由、能力匹配、降级、Offering、Credential和证据版本；
- Preview/GA、Region和模型兼容表是路由条件；
- 同一模型在不同 Provider、计划或协议下必须分别测试和验收。

### 5.2 Catalog与文档

Catalog至少保存：

- Route各身份字段；
- SDK语言、包、所有权、版本和许可证；
- Endpoint模板、鉴权、Region和模型发现；
- `allowed_usage`、Key前缀、到期、配额与生产边界；
- 能力、限制、忽略字段、扩展字段、流事件和错误方言；
- 官方来源 ID、`checked_at`、`valid_until`、证据状态；
- Praxis状态、代码位置、测试与烟测证据。

Markdown矩阵由 Catalog生成或双向一致性校验。验证器必须拒绝缺来源、超期、重复 Route ID、非法状态迁移、已实现但无测试、`terms_blocked`却可调用等情况。

### 5.3 协议驱动复用

- 先用现有三家测试保护行为，再抽取协议层；
- Protocol Driver作为内部实现，拥有统一语义与 wire DTO之间的映射、响应归一化、流状态机、协议 continuation、Raw构造和安全错误提取；
- Provider Adapter拥有 Endpoint、Credential、模型、方言、能力、错误和条款；
- Responses和 Chat使用不同 DTO与状态机；
- 兼容 Provider必须测试未知字段、扩展字段和非标准流事件；
- 任何无法表达的字段都返回稳定错误，不静默删除。

### 5.4 SDK与语言执行器

- 官方 Go SDK完整时使用 Go；
- 完整公开 HTTP/SSE/WebSocket/gRPC合同可由 Go忠实实现；
- 官方 TS/Python SDK有独占语义时使用 Sidecar；
- TypeScript为首选非 Go执行器；Python按 Route证据启用；
- Sidecar统一使用版本化 IPC、健康检查、取消、超时、资源上限、优雅退出和流背压；
- SDK对象、异常、日志与密钥不得跨 IPC泄漏；
- 每种执行器运行相同 Provider公共契约测试。

### 5.5 Credential与订阅策略

- Credential只保存类型和秘密引用，不保存明文；
- Endpoint host allowlist与 Credential audience绑定，防止 OpenAI Key发往 AWS/Azure或反向泄漏；
- Key前缀、Region、Offering和 Endpoint族硬校验；
- `official_client_only`在配置阶段拒绝；
- `interactive_coding_only`必须由调用场景显式声明，禁止服务器批处理和多租户后端；
- 订阅到期、额度耗尽或 HTTP 402/403不自动切换 PAYG；
- 客户端身份必须真实，不允许伪造 User-Agent；
- 所有回退必须由用户配置的显式策略批准并进入 MappingReport。

## 6. 实施波次与详细清单

### 波次 A：Route模型、Catalog与证据门禁

- [x] 审核并固定 Route、Offering、Deployment、Protocol、Credential和 ModelIdentity字段；
- [x] 固定 `allowed_usage`、证据状态和落地状态机；
- [x] 为现有三家直连 Provider的四条协议 Binding建立 Catalog记录，保证生成结果与 live state一致；
- [x] 定义 Catalog Schema、Source ID、TTL与失效记录；
- [x] 实现 Catalog解析、语义校验、重复检测、状态迁移和 Markdown一致性验证；
- [x] 实现超期、断链、缺来源和 `terms_blocked`可调用反例测试；
- [x] 更新 CI入口，但不引入自动付费或真实 API调用；
- [x] 完成 Route解析和 MappingReport设计审核。

实际验收摘要（2026-07-10）：

- Route与 Credential：`upstream/` 已落地 Model Family、Provider、Offering、Deployment、Protocol、Endpoint、Credential七维身份、确定性摘要、`allowed_usage`策略、Endpoint安全解析、Credential audience/作用域绑定和 SDK无关 MappingReport；Credential只保存秘密引用，不保存明文值。
- Catalog、Schema与 Markdown：`catalog/` 已登记 OpenAI Responses、OpenAI Chat Completions、Anthropic Messages、Gemini GenerateContent四条当前 Binding；`catalog/schema/catalog-v1.schema.json` 与嵌入 Schema接受严格一致性校验，证据 TTL、摘要、失效、状态迁移、资产路径和阻塞路线反例均有测试；Provider Matrix中的当前 Binding区块由 Catalog生成并接受漂移校验。
- Runtime AdapterID映射固定为：两条 OpenAI Binding → `openai`、Anthropic Messages → `anthropic`、Gemini GenerateContent → `gemini`；测试直接对照各 Provider公开 `ProviderID`，Catalog Provider身份不被错误压成 Runtime Registry ID。
- 统一离线入口为 `ExecutionRuntime/model-invoker/scripts/verify-offline.sh`，GitHub Actions入口为 `.github/workflows/model-invoker.yml`。脚本实际退出 0，覆盖依赖校验、格式、`go mod tidy -diff`、`git diff --check`、`go vet`、全量、shuffle、race、integration仅编译和 Catalog资产校验；依赖获取之后的测试阶段屏蔽真实凭据并将外部 HTTP代理指向关闭的 loopback端口。
- `go test -count=1 ./tests/upstream ./tests/catalog ./tests/catalogassets`实际通过。三项新增 fuzz各运行 3秒并通过：`FuzzEndpointResolutionNeverReturnsUnsafeURL` 349,949次、`FuzzCatalogDecodeValidateAndDigest` 18,291次、`FuzzCatalogArtifactPaths` 13,548次。
- 本波次未执行真实 API、真实套餐或认证成功调用，未新增 Provider，也不形成生产支持结论。

完成门槛：已达到。现有 OpenAI/Anthropic/Gemini离线回归保持通过；矩阵当前 Binding事实可由机器验证；没有新 Provider注册。下一步仅启动波次 B0。

### 波次 B：协议驱动与现有三家回归（已完成）

- [x] B0：建立 `internal/protocol` 的 Binding、Driver、Failure和 Provider身份注入反例；
- [x] B1：先从 OpenAI Provider抽取无状态 Chat Completions driver，验证内部包边界；
- [x] B2：抽取 Responses driver，保留 typed Items、状态与独立流状态机；
- [x] B3：抽取 Anthropic Messages driver及完整 continuation；
- [x] B4：抽取 Gemini GenerateContent driver及 thought signature continuation；
- [x] B5：统一安全错误提取，删除旧重复实现，SDK错误不得进入公开 unwrap链；
- [x] B6：运行全部 Provider与协议回归，并增加导出签名不含 SDK类型的 AST门禁；
- [x] Provider方言继续拥有能力与扩展，不把兼容层写成模型原厂；
- [x] Protocol Binding必须注入 Provider、Protocol和 Endpoint，驱动源码不得硬编码原厂 Provider ID；
- [x] 固定四协议非流式、流式、工具、推理、结构化输出、状态、用量和错误样本；
- [x] 运行第一、第二阶段全部普通、shuffle、race、fuzz、coverage与 integration compile回归；
- [x] 证明 SDK类型、Credential与 Raw没有穿透公共边界。

B0实际验收摘要（2026-07-10）：

- `internal/protocol` 已建立 SDK中立的 `Binding`、`Driver`、`Dialect`、`Base`、结构化 `Failure`与 identity-bound stream；Binding强制注入 Runtime Provider、Protocol和 Endpoint，并拒绝请求、状态与 Endpoint错配。
- Failure只允许安全分类字段进入统一错误，SDK error、`http.Request`、Credential与自定义取消 cause不能进入公开 unwrap链；Raw保持默认脱敏和防御性复制。
- `tests/protocol` 已覆盖非原厂 Provider身份、typed-nil、无效分类 fail-closed、事件/终态/Close身份、红后 stamp顺序、AST边界和原厂身份硬编码反例。
- 审计捕获并修复 `Binding.StampError(nil)` 的 typed-nil接口陷阱；直接回归证明 nil输入返回真正的 nil `error`。
- `go test -count=1 ./tests/protocol`、同包 race、定向 vet和统一 `scripts/verify-offline.sh`均实际退出 0；统一脚本继续屏蔽真实凭据且未执行真实 API或真实套餐。

B1实际验收摘要（2026-07-10）：

- 新增 `internal/protocol/openaichat`，完整拥有 Chat Completions请求映射、官方 SDK窄客户端缝隙、非流响应归一化、流状态机、Raw/usage/tool/finish reason语义和安全 Failure提取；源码不导入 Provider包，也不硬编码原厂 Provider身份。
- OpenAI Adapter只保留 Credential/Endpoint/SDK transport、Capabilities和 `chatDialect`；Chat调用经 `driver → Redactor → public Binding`，最后一层恢复 Provider/Protocol身份，但只恢复已经脱敏的公开 Endpoint。
- 删除 `provider/openai` 中旧的 Chat mapping、normalize和 stream重复实现；Responses路径保持原位，等待 B2独立抽取。
- 新增非原厂 fake driver契约、Chat完整 HTTP错误矩阵、state预调用拒绝、SDK cause隔离、流顺序/usage/终态/Close、body-limit，以及 Provider ID或 Endpoint与密钥重合的脱敏反例。
- 抽取测试捕获并修复 HTTP 408重试回归：已分类的 HTTP Timeout可保持 Retryable；context取消/截止仍不可重试且只保留安全 sentinel。
- `FuzzDriverInvokeNeverPanicsOrLosesIdentity`运行3秒、59,976次；`FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence`运行3秒、56,654次；相关合并覆盖率实际为77.9%，仓库仍无百分比硬门禁。
- 统一 `scripts/verify-offline.sh`在最终代码上实际退出0，覆盖普通、shuffle、race、vet、integration仅编译与 Catalog资产；没有执行真实 API、真实套餐或付费调用。

B2实际验收摘要（2026-07-10）：

- 新增 `internal/protocol/openairesponses`，完整拥有 Responses typed input/output、tool/reasoning/structured output映射、服务端 continuation、非流归一化、安全 Failure提取和独立流状态机。
- OpenAI Adapter的 Responses路径也改为 `driver → Redactor → public Binding`；Provider包内原 Responses mapping、normalize和 stream重复实现已删除，Chat与 Responses现在是两个独立具体协议包。
- 直接 driver测试使用非原厂 Binding验证 typed message/function/reasoning Items、`previous_response_id`、State身份、Provider metadata、Raw、native sequence、usage终态和 Close幂等。
- 现有 OpenAI黑盒继续覆盖 Responses HTTP/SSE、response.failed、refusal、工具、reasoning、结构化输出和安全边界；抽取中把 transport cause收口为 nil，公开 unwrap继续只允许 context安全 sentinel。
- 新增 `FuzzDriverInvokeNeverPanicsOrLosesTypedState`运行3秒、58,427次；`FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence`运行3秒、63,606次；相关合并覆盖率实际为73.4%，仓库仍无百分比硬门禁。
- 统一 `scripts/verify-offline.sh`在最终代码上实际退出0；没有执行真实 API、真实套餐或付费调用。

B3实际验收摘要（2026-07-11）：

- 新增 `internal/protocol/anthropicmessages`，完整接管 Messages请求映射、signed/redacted thinking、tool-use provider continuation、非流归一化、安全 Failure提取和 content-block SSE状态机；源码不导入 Provider包，也不硬编码原厂 Provider身份。
- Anthropic Adapter改为 `driver → Redactor → public Binding`；Provider包只保留 API Key、Endpoint、SDK transport、Capabilities、Provider方言验证/分类与响应头白名单，旧 mapping、normalize和 stream重复实现已删除。
- continuation使用版本化白名单载荷，只保留可续接的 thinking、redacted thinking与 direct tool-use块；普通 assistant text、未知块、cache-control注入、非 direct caller和跨 Binding State继续在 HTTP前拒绝。
- 新增非原厂 Binding直接 driver契约，覆盖 signed thinking + tool result完整往返、State/Response/MappingReport身份、SDK cause隔离、流顺序/usage/终态/Close和 typed-nil构造反例；现有 Anthropic HTTP/SSE、安全与 continuation黑盒全部保持通过。
- 新增 `FuzzDriverInvokeNeverPanicsOrLosesProviderContinuation`运行3秒、16,343次；`FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence`运行3秒、37,964次；相关合并覆盖率实际为75.1%，仓库仍无百分比硬门禁。
- `go vet ./...`、定向 race、全量普通测试和统一 `scripts/verify-offline.sh`均实际退出0；没有执行真实 API、真实套餐、付费调用或认证成功烟测。

B4实际验收摘要（2026-07-11）：

- 新增 `internal/protocol/geminigenerate`，完整接管 GenerateContent请求映射、JSON Schema、thought signature provider continuation、非流归一化、安全 Failure提取和迭代流状态机；源码不导入 Provider包，也不硬编码原厂 Provider身份。
- Gemini Adapter改为 `driver → Redactor → public Binding`；Provider包只保留 Developer API Credential/Endpoint/SDK transport、Capabilities、Provider方言验证/分类和响应头白名单，旧 continuation、mapping、schema、normalize和 stream重复实现已删除。
- continuation继续区分 native ID与确定性语义 ID，保留 model/tool/result/text历史、thought signature、已响应索引和跨轮防御性复制；未知字段、不一致 call索引、伪造生成 ID、模型/用户角色违规和不受控原生 Part继续在 HTTP前拒绝。
- 新增非原厂 Binding直接 driver契约，覆盖 thought signature + tool result往返、State/Response/MappingReport身份、SDK cause隔离、重复工具快照去重、usage/终态/Close和 typed-nil构造反例；现有 Gemini HTTP/SSE、安全、后到签名和冲突黑盒全部保持通过。
- 新增 `FuzzDriverInvokeNeverPanicsOrLosesThoughtSignatureState`运行3秒、22,743次；`FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence`运行3秒、18,419次；相关合并覆盖率实际为72.9%，仓库仍无百分比硬门禁。
- `go vet ./...`、定向 race、全量普通测试和统一 `scripts/verify-offline.sh`均实际退出0；没有执行真实 API、真实套餐、付费调用或认证成功烟测。

B5/B6实际验收摘要（2026-07-11）：

- 四个协议 driver统一使用 `BeginFailureExtraction`、`ExtractCommonFailure`与 `BoundedFailureText`完成 context、既有公共错误、重定向、transport和 malformed payload的安全首轮提取；Gemini删除最后一套通用重复分支。
- `adaptercore.ContextError`把 Capabilities的 context终态统一收口为 `context.Canceled`或`context.DeadlineExceeded`，未知原生错误 fail-closed且不进入公开 unwrap链。
- 三个 Provider的 `errors.go`已删除旧 `normalizeError`及 SDK Raw提取，只保留 Provider本地错误构造与方言分类；OpenAI SDK错误共享提取仍由两个 OpenAI协议 driver共用的 `internal/protocol/openaierrors`拥有。
- 新增共享 Failure安全/边界测试、Provider旧提取器反例，以及包外可达函数、导出接收者方法、导出类型/字段和值不含三家 SDK类型的 AST门禁。
- `scripts/verify-offline.sh`在最终代码上实际退出0；20项现有 fuzz均实际通过，其中 `FuzzCatalogArtifactPaths`在连续高并发3秒批次出现一次停止边界 `context deadline exceeded`，单独5秒复验通过34,593次，余下目标限制8 workers后稳定通过。
- `go test -count=1 -coverpkg=./... -coverprofile=... ./...`实际通过，合并语句覆盖率为81.0%；仓库仍不设置百分比硬门禁。
- 本波次没有新增 Provider、没有读取真实凭据、没有执行真实 API、真实套餐、付费调用或认证成功烟测。

完成门槛：已达到。现有三家全部回归通过；协议层可复用但没有虚假兼容声明；公共签名与错误边界不泄漏 SDK类型或原生错误。

### 波次 C：订阅与商业计划控制面

- [x] 实现 Offering、CommercialEntitlement、UsageScope和配额状态；
- [x] 实现 Key前缀、Endpoint族、Region和到期校验；
- [x] 实现订阅不自动回退到 PAYG；
- [x] Catalog登记 GLM、Kimi、MiniMax、MiMo、Alibaba和 xAI消费者计划；
- [x] 官方证据刷新已推翻 GLM、MiMo、Alibaba旧全面 `terms_blocked`判断；改为 `interactive_coding_only + planned + callable=false`，后端与非交互场景继续硬拒绝；
- [x] Kimi Code只允许个人交互式 Coding/Agent配置，保留真实 User-Agent；
- [x] MiniMax Token Plan只按官方允许的 Agent/Coding客户端范围实现，生产 backend保持拒绝；
- [x] Alibaba Savings Plan只作为按量路线 BillingPlan，不创建重复 Provider；
- [x] xAI Grok Build仅作为外部 Agent/ACP候选，不伪装 HTTP模型 Provider；
- [x] 测试 402/403、额度耗尽、到期、错误 Key、错误 Region和禁止场景。

波次 C实际验收摘要（2026-07-11）：

- 新增动态 `EntitlementState`，绑定 Offering与 Credential Profile并检查状态观察窗口、套餐到期、剩余额度、额度重置、暂停和错绑定；静态 `CommercialEntitlement`继续约束显式个人、单租户、前台、非生产与真实客户端身份。
- 新增瞬时 Key前缀验证，错误只返回稳定分类且不回显输入；MiniMax `sk-cp-*`、MiMo `tp-*`、Alibaba Coding `sk-sp-*`已进入 Catalog Credential约束。
- 401/402/403/429订阅终态统一返回 Credential/Billing/Access/Quota拒绝，`AllowsAutomaticPAYGSwitch`始终为 false；现有 Route校验继续锁定 Endpoint audience、Offering、Deployment、Region和 Credential。
- 新增 `BillingPlanReference`，Savings Plan只影响现有 Offering结算，不进入七维 Route身份；Schema与防御性复制测试同步更新。
- `catalog.DefaultDocument`由4条扩展为22条：原4条 callable直连 Binding不变，新增18条 GLM/Kimi/MiniMax/MiMo/Alibaba/xAI控制记录均无 Adapter ID且 `callable=false`。
- C定向测试与全量 `go test -count=1 ./...`通过；新增 Credential泄密 fuzz运行3秒通过374,365次，两个 Catalog fuzz在扩展文档上分别通过11,334次和2,592次。
- 本波次只实现离线授权/拒绝与 Catalog控制面，没有实现订阅 HTTP Adapter，没有读取真实 Key或执行真实套餐调用。

完成门槛：已达到。允许路线按官方范围可离线授权或拒绝；计划/阻塞路线无法被 Invoker选择，当前 callable集合仍严格保持原4条直连 Binding。

### 波次 D：云托管 Provider

#### AWS Bedrock

- [x] 分开 `bedrock-mantle`与 `bedrock-runtime`；
- [x] Mantle实现 Responses、Chat和 Messages绑定，兼容能力保持模型级校验而不批量继承；
- [x] Claude Mantle使用当前 `anthropic-sdk-go` 的 `bedrock.NewMantleClient`路径，没有新增 Sidecar；
- [x] Runtime使用 AWS SDK for Go v2实现 Converse/ConverseStream与 InvokeModel；
- [x] 实现 Bedrock bearer、短期 Key刷新与 SigV4路线；
- [x] 禁止 Bedrock使用 OpenAI Key；
- [x] 测试 `store=false`、`previous_response_id`、Project隔离和30天保存边界；
- [x] 结构化输出、缓存、Guardrail和状态差异按 Endpoint保持显式 Unsupported/Unknown或映射拒绝；
- [x] Anthropic Message Batches明确拒绝；AWS云 Batch保持独立能力域；
- [x] Region、model ref、inference profile与配额分别入 Catalog。

#### Google Vertex AI

- [x] 将 Gemini Developer API与 Vertex Gemini分开；
- [x] 使用 Google Gen AI Go SDK实现 Vertex原生 GenerateContent；
- [x] 实现 ADC/API Key及 Project/Location边界；
- [x] 实现 Vertex Claude `rawPredict/streamRawPredict`的 Anthropic SDK路线；
- [x] 使用当前 `anthropic-sdk-go` 的 Vertex Google Auth/credentials路径，没有新增 Sidecar；
- [x] OpenAI兼容 Endpoint只按当前官方 Chat能力设计，禁止推定 Responses；
- [x] 区分 serverless、Provisioned Throughput与 self-deployed Model Garden。
- [x] Anthropic Message Batches明确拒绝；Google Cloud Batch作为独立能力域且未知能力保持 Unknown。

#### Azure OpenAI / AI Foundry

- [x] 实现 Azure OpenAI v1与独立 legacy binding；
- [x] v1拒绝自动追加 `api-version`；
- [x] 使用自定义 deployment name测试，禁止假设 deployment等于模型 ID；
- [x] 支持 API Key与 Entra ID刷新；
- [x] Responses/Chat能力保持 Deployment、Preview/GA和 Region级核验；
- [x] Azure AI Foundry其他模型逐模型建 Route卡，不继承 Azure OpenAI能力。

#### 其他 Claude云产品

- [x] 将 Anthropic-operated、AWS Marketplace计费的 Claude Platform on AWS单独建为 `unverified + research_only`控制记录；
- [x] 明确它不是 Bedrock，不共享 Provider ID、Credential、Endpoint或能力；
- [x] Claude Pro/Max消费者订阅不能转换为 Anthropic API、Bedrock或 Vertex额度。

波次 D实际验收摘要（2026-07-11）：

- 新增 `aws-bedrock-mantle`、`aws-bedrock-runtime`、`google-vertex-ai`和 `azure-openai`四个 Runtime Adapter；Mantle、Runtime、Vertex和 Azure保持独立 Provider/Endpoint/Credential身份。
- 新增 `bedrock_converse`与 `bedrock_invoke_model`协议；Converse完成文本、工具、用量、错误和流映射，InvokeModel保持显式 provider-native Raw边界。
- Mantle完成 Responses、Chat、Messages与 API Key/SigV4组合；Runtime完成 Converse/Invoke与 bearer/SigV4组合；Vertex完成 GenerateContent、Messages、Chat与 ADC/API Key组合；Azure完成 v1 Responses/Chat、dated legacy Chat与 API Key/Entra组合。
- Catalog由22条扩展为48条：25条 callable、23条控制记录；Provisioned Throughput、Model Garden、Foundry其他模型、Claude Platform on AWS和 Claude消费者计划保持非 callable。
- 本机 HTTP/SDK fake实际覆盖 AWS SigV4/bearer、Mantle Key刷新与三协议、Vertex Gemini/Claude官方 middleware、Azure v1/legacy和 Entra刷新；全量普通测试通过。
- 新增四条显式 `integration`烟测入口；统一离线脚本清空云凭据与确认开关后只编译入口，没有执行真实认证或付费调用。

完成门槛：已达到。每个云 Provider至少一条获批 Route离线通过；所有 callable云路线保持 `implemented_offline`，没有冒充 `live_verified`或生产批准。

### 波次 E：重点厂商直连 Provider

按协议复用和风险分组，不以“换 Base URL”批量生成：

#### E1：兼容协议主路径

- [x] DeepSeek：OpenAI Chat主路径、Anthropic Messages兼容路径，保留 reasoning方言并阻断静默模型映射；
- [x] Kimi开放平台：OpenAI Chat，和 Kimi Code订阅完全分离；
- [x] MiniMax按量：Anthropic主路径，Chat/Responses按能力卡实现；
- [x] Z.AI按量：OpenAI Chat主路径，和 Coding Plan完全分离；
- [x] MiMo按量：当前官方仅公开 Chat/Messages，按独立设计卡实现；不生成虚假 Responses Binding，按量与 Token Plan Region/Key隔离；
- [x] Qwen/百炼：Responses与 Chat兼容路径，Region/Workspace明确；
- [x] xAI：Responses主路径与独立方言。

E1-DeepSeek实际验收摘要（2026-07-11）：

- 独立设计卡刷新到当前 `deepseek-v4-flash/pro`合同；旧 `deepseek-chat/reasoner`将在 2026-07-24废弃，未进入 callable模型集合；
- 新增 `deepseek` Runtime Adapter，OpenAI Chat和 Anthropic Messages分别绑定官方 Endpoint与认证放置；
- Chat保留 `thinking`、`reasoning_effort`、非流 `reasoning_content`和流式 reasoning delta；Messages强制精确 DeepSeek模型 ID，拒绝服务端对 Claude别名或未知模型的静默映射；
- 新增内部 `compatprovider`组合层和 Chat方言扩展缝隙；SDK类型仍不进入公共签名，Provider身份、能力、错误和 Credential不被兼容协议原厂吞并；
- Catalog由48条扩展为50条：27条 callable、23条控制记录；DeepSeek两条 Binding状态为 `implemented_offline`；
- DeepSeek本机 SDK HTTP/SSE fake、全量普通、shuffle、定向 race、Catalog资产和 integration仅编译均通过；新增泄密/选择 fuzz运行3秒通过27,064次；
- 未读取真实 Key，未执行真实认证、真实模型或付费调用。下一步只启动 Kimi开放平台按量路线。

E1-Kimi实际验收摘要（2026-07-11）：

- 独立设计卡刷新当前 `kimi-k2.7-code/highspeed`、K2.6/K2.5和 Moonshot V1文本模型；已下线的 K2预览、`kimi-latest`与视觉模型不进入 callable集合；
- 新增 `kimi` Runtime Adapter，只绑定 `https://api.moonshot.cn/v1`与 `MOONSHOT_API_KEY`；Kimi Code的 `api.kimi.com/coding`、`kimi-for-coding`和会员 Key继续作为独立非 callable控制记录；
- K2.7强制 thinking且不发送禁用开关；K2.6/K2.5把统一 reasoning意图转换为 `thinking.type`；非流/流 `reasoning_content`均进入统一 reasoning输出；
- 当前统一 Chat输入无法忠实回传 preserved thinking，thinking工具后续轮在 HTTP前拒绝，不静默丢历史推理；Partial Mode、官方工具、文件、Batch和多模态保持当前 slice外；
- Catalog由50条扩展为51条：28条 callable、23条控制记录；Kimi按量 Binding状态为 `implemented_offline`；
- 本机 SDK HTTP/SSE fake、模型/Offering错配、quota错误、全量普通、shuffle、定向 race、Catalog资产和 integration仅编译通过；新增 fuzz运行3秒通过25,231次；
- 未读取真实 Key，未执行真实认证、真实模型或付费调用。下一步只启动 Z.AI按量路线。

E1-Z.AI实际验收摘要（2026-07-11）：

- 独立设计卡刷新 `api.z.ai/api/paas/v4`、当前 GLM文本模型、thinking、工具、JSON Object、业务错误和专属 finish reason；
- 新增 `zai`按量 Chat Adapter，只绑定 `ZAI_API_KEY`与标准 Endpoint；GLM Coding Plan专属 Endpoint、Key和订阅额度继续作为不同非 callable Offering；
- GLM-5.2 effort显式转换，其他 thinking模型只映射启用/禁用；非流/流 `reasoning_content`与 body `request_id`进入统一输出；
- Chat协议新增 Provider方言 finish reason与流 Envelope元数据扩展，`sensitive`、context window、`network_error`分别归一化，不混入未知终态；
- 当前不启用 preserved thinking，无法保真回传推理历史的 thinking工具后续轮在 HTTP前拒绝；`tool_choice`只接受官方 `auto`；
- Catalog由51条扩展为52条：29条 callable、23条控制记录；Z.AI按量 Binding状态为 `implemented_offline`；
- 本机 SDK HTTP/SSE fake、模型/Offering错配、业务错误矩阵、全量普通、shuffle、定向 race与 Catalog资产通过；新增 fuzz运行3秒通过19,575次；
- 未读取真实 Key，未执行真实认证、真实模型或付费调用。下一步只启动 MiniMax按量路线。

E1-MiniMax实际验收摘要（2026-07-11）：

- 独立设计卡刷新到当前 `MiniMax-M3`与 M2.7/M2.5/M2.1/M2合同；Anthropic Messages、OpenAI Chat与 Responses三条官方兼容路线保持独立状态语义；
- 新增 `minimax` Runtime Adapter，Messages为默认主路径，Chat/Responses为补充路径；三条 Route只引用 `MINIMAX_API_KEY`，`sk-cp-*` Token Plan Key在配置阶段拒绝，原订阅控制记录继续不可调用；
- Messages保留 thinking/signature与工具 continuation，并只在服务端输出规范化时补内部 `caller.type=direct`，发送时恢复 MiniMax文档 wire shape；
- Chat固定 `reasoning_split=true`并把官方累积 reasoning/text字段转换为不重复增量；Responses保留 typed output并清除 `store=false`路线不具备的服务器 continuation State；
- M3与 M2.x按三协议各自默认值处理 thinking；M2.x禁用请求、thinking工具历史不完整、未知模型、错 Endpoint和静默模型映射均在 HTTP前拒绝；
- Catalog由52条扩展为55条：32条 callable、23条控制记录；MiniMax三条按量 Binding状态为 `implemented_offline`；
- MiniMax本机 Anthropic/OpenAI SDK HTTP/SSE fake、全量普通、shuffle、定向 race、Catalog资产和 integration仅编译均通过；新增泄密/选择 fuzz运行3秒通过26,436次；全仓合并语句覆盖率76.2%；
- 未读取真实 Key，未执行真实认证、真实模型或付费调用。下一步只启动 MiMo按量路线。

E1-MiMo实际验收摘要（2026-07-11）：

- 独立设计卡与官方 `llms.txt`刷新确认当前按量 API只公开 Anthropic Messages和 OpenAI Chat，不生成虚假 Responses Binding；只允许 `mimo-v2.5-pro`与 `mimo-v2.5`文本切片；
- 新增 `xiaomi-mimo` Runtime Adapter，Messages为默认主路径，Chat为补充路径；两条 Route只引用 `MIMO_API_KEY`，`tp-*` Key和三类 Token Plan Region域名在配置阶段拒绝，原六条订阅控制记录继续不可调用；
- Messages使用官方支持的 Bearer认证，保留 thinking/signature、工具 continuation和 parallel开关；Chat保留非流/流 `reasoning_content`与 JSON Object，thinking工具历史无法保真时在 HTTP前拒绝；
- Messages协议新增 Provider专属 stop reason扩展；两协议把 `repetition_truncation`归为 incomplete/other，把 `content_filter`归为策略拒绝，不把厂商终态混入未知降级；
- Catalog由55条扩展为57条：34条 callable、23条控制记录；MiMo两条按量 Binding状态为 `implemented_offline`；
- MiMo本机 Anthropic/OpenAI SDK HTTP/SSE fake、全量普通、定向 race、Catalog资产和 integration仅编译均通过；新增泄密/选择 fuzz运行3秒通过17,774次；全仓合并语句覆盖率76.4%；
- 未读取真实 Key，未执行真实认证、真实模型或付费调用。下一步只启动 Qwen/百炼按量路线。

E1-Qwen实际验收摘要（2026-07-11）：

- 独立设计卡刷新当前 Responses、Chat、Region、Workspace、`sk-ws-*`新 Key、旧 `sk-*`兼容和 `sk-sp-*`订阅隔离合同；当前只批准北京与新加坡 Workspace专属按量 Endpoint；
- 新增 `qwen` Runtime Adapter，Responses为默认主路径、Chat为补充路径；两 Region各两条 Binding保持独立 Deployment、Credential Profile与 Endpoint模板；
- 当前共同文本模型只允许八个稳定别名；日期快照、视觉/音频/OCR、未知模型、共享/Trial/订阅域名和跨 Region/Workspace选择均在 HTTP前拒绝；
- Responses保留 typed output、reasoning、工具、7天 `previous_response_id` State和流；Chat把 portable reasoning映射为 `enable_thinking`/`thinking_budget`并保留非流/流 `reasoning_content`与 JSON Object；
- Credential Schema新增可选 `denied_key_prefixes`，允许 Catalog准确表达旧 `sk-*`按量 Key同时拒绝更具体的 `sk-sp-*`订阅 Key，拒绝规则优先于允许规则；
- Catalog由57条扩展为61条：38条 callable、23条控制记录；Qwen四条按量 Binding状态为 `implemented_offline`，原六条 Alibaba Coding/Token Plan控制记录继续不可调用；
- Qwen本机 OpenAI SDK HTTP/SSE fake、全量普通、shuffle、race、Catalog/Schema/Markdown、integration仅编译与统一离线脚本均通过；新增泄密/选择 fuzz运行3秒通过28,342次；全仓合并语句覆盖率76.5%；
- 未读取真实 Key，未执行真实认证、真实模型、真实套餐或付费调用。下一步只启动 xAI Responses按量路线。

E1-xAI实际验收摘要（2026-07-11）：

- 独立设计卡刷新当前 `grok-4.5`、`https://api.x.ai/v1/responses`、Bearer `XAI_API_KEY`、Responses优先、30天状态、reasoning、工具、缓存与错误合同；旧模型、legacy Chat和消费者产品不进入 callable路线；
- 新增 `xai` Runtime Adapter，只允许精确 `grok-4.5 + Responses + api.x.ai`；消费者 Grok Build记录继续为 `official_client_only + unverified + callable=false`；
- 保留 typed output、函数工具、parallel控制、非流/流、usage、cached/reasoning token与 `previous_response_id`；`prompt_cache_key`通过严格 Provider Option映射；
- reasoning仅允许官方 `low/medium/high`且不能禁用；encrypted reasoning、结构化输出、托管工具、gRPC、WebSocket、Batch、Deferred、媒体和其他模型保持当前 slice外；
- Catalog由61条扩展为62条：39条 callable、23条控制记录；xAI API与消费者订阅分别具有不同 Provider、Offering、Endpoint、Credential和实施状态；
- 本机 OpenAI SDK HTTP/SSE fake、错误/取消/重定向/响应上限、普通全量、shuffle、定向 race、Catalog资产和 integration仅编译均通过；新增泄密/选择 fuzz运行3秒通过16,417次；全仓合并语句覆盖率76.7%；
- 未读取真实 Key，未执行真实认证、真实模型、真实消费者订阅或付费调用。E1兼容协议主路径至此完成。

#### E2：原生或复杂路径

- [ ] Meta Llama API：先解决 GA、服务范围和条款证据；原生 `/v1`与兼容 `/compat/v1`分开；
- [ ] Qwen DashScope原生能力；
- [ ] xAI gRPC独占能力；
- [ ] 只有兼容协议无法表达的厂商扩展；
- [ ] 需要非 Go官方 SDK的路线进入 Sidecar波次。

每个 Provider开始编码前必须提交独立设计卡：协议、Endpoint、auth、模型、SDK、能力、限制、错误、流、测试和真实烟测边界。

### 波次 F：TypeScript与 Python执行器（触发审计完成，未触发）

当前结论（2026-07-11）：

- [x] 已逐条审计当前批准的直连、订阅控制面与云托管 Route；全部 callable路线可由 Go协议 driver、官方 Go SDK或审计过的 REST合同完整承载；
- [x] xAI gRPC、Meta原生、Qwen原生及其他可能依赖非 Go SDK的能力仍属于未批准 E2研究路线，不足以触发 Sidecar；
- [x] 依据“没有证据不得为了演示引入 SDK”，本波次不创建 IPC、supervisor、TS/Python包、lockfile或空壳测试；
- [x] 未来只有获批 Route给出非 Go独占能力和官方证据时，才重新开启以下陈旧实施清单。

未来触发后的实施清单：

- [ ] 固定 Go↔Sidecar版本化 IPC与兼容策略；
- [ ] 实现 TypeScript supervisor、健康检查、取消、超时、资源上限、流背压和优雅退出；
- [ ] 实现 Python supervisor的同等契约，但只有获批 Route可引入 Python依赖；
- [ ] 锁定包管理器、lockfile、运行时版本、许可证和供应链扫描；
- [ ] 实现 Sidecar崩溃、卡死、异常退出、畸形事件和版本不匹配测试；
- [ ] 使用 SDK fake与本机协议服务完成黑白盒测试；
- [ ] 证明 SDK对象、异常、认证和日志不穿透到 Go；
- [ ] 同一 Provider的 Go/TS/Python执行结果运行语义对照测试。

完成门槛：至少一条有真实证据需要非 Go SDK的路线完成离线验收；没有证据不得为了演示引入 SDK。

当前门槛结论：未发现触发证据，因此“不实施 Sidecar”是本轮完成状态，不把不存在的执行器冒充产物。

### 波次 G：第三方托管与自托管（按当前授权延期）

当前结论（2026-07-11）：活动 goal明确要求第三方首批名单保持延期；没有用户确认的首批 Route，因此本波次不新增 Provider、Deployment模板或 `callable=true`记录。以下清单原样保留，未来单独审核。

- [ ] 从 Groq、Cerebras、Together、Fireworks、OpenRouter中审核并确认首批 Route；
- [ ] 审核 Cloudflare Workers AI、NVIDIA NIM、Hugging Face Inference；
- [ ] 建立 vLLM、TGI、Ollama等自托管 Deployment模板；
- [ ] 每家核验 Provider、billing owner、实际 serving backend、模型来源、Region、数据和协议；
- [ ] 聚合平台必须保留上游模型与实际 Provider元数据；
- [ ] 自托管必须记录部署者、镜像/版本、硬件、模型权重和 Endpoint所有权；
- [ ] 不使用社区 SDK替代官方合同；
- [ ] 每条路线独立运行协议、错误、流和真实烟测门槛。

完成门槛：首批清单由用户确认；队列中未确认 Provider继续保持 `research_only`。

### 波次 H：真实烟测、生产门槛与持续维护（真实调用按当前授权延期）

- [x] 为每个已实现 Runtime Provider提供 build tag、全局确认与 Provider确认/凭据三重显式启用的烟测入口；
- [ ] 用户逐路批准账号、模型、Region、预算、调用次数和输入数据；
- [ ] 记录精确 Route、模型 ID、Endpoint、时间、响应 ID和结果；
- [ ] 验证非流式、流式、工具与最低错误路径；
- [ ] 真实失败只修正对应 Route，不扩大为全 Provider结论；
- [ ] 生产评审另核验容量、SLA、配额、数据地域、日志、条款与运维；
- [ ] 定期执行 7/14/30/90天 TTL与官方来源变化检查；
- [ ] SDK升级、模型下线、Endpoint迁移和条款变化触发自动失效。

完成门槛：只有满足生产门槛的精确 Route可标 `production_approved`；其他路线保持真实状态。

当前门槛结论（2026-07-11）：全部 smoke入口只完成离线编译；活动 goal未授权任何真实账号、套餐、预算或调用，所有真实烟测、生产评审和 `production_approved`状态明确延期。全部 callable Route保持 `implemented_offline`。

## 7. 测试与验收方法

| 层级 | 方法 | 完成标准 |
|---|---|---|
| Catalog | Schema、生成器、失效和状态反例 | 缺来源、超期、非法状态和阻塞可调用均失败 |
| Route核心 | 表驱动、属性测试、并发与 fuzz | 身份、策略、回退、Credential和 MappingReport确定可复现 |
| 协议驱动 | 固定 JSON/SSE/event-stream样本与 `httptest` | 字段、流、工具、状态、错误和未知扩展均有断言 |
| Provider黑盒 | 外部测试包、官方 SDK/HTTP、假服务 | 只通过公共 API验证最终请求和结果 |
| Sidecar | 真进程、本机 IPC、故障注入 | 生命周期、版本、取消、背压、崩溃和脱敏通过 |
| 云托管 | 本机签名/凭据 fake与官方 SDK | Credential、Region、deployment和协议绑定可验证 |
| 订阅策略 | 到期、额度、402/403、错误 Key、使用场景 | 无隐式回退、无伪装、阻塞路线不可调用 |
| 真实烟测 | 显式 build tag + Route级环境门槛 | 只有用户批准路线运行，结果不冒充生产结论 |

### 7.1 所有波次共同门禁

- 当前 OpenAI、Anthropic、Gemini全部回归；
- `gofmt`、`go mod tidy`、`go mod verify`、`go vet`；
- 普通、shuffle、race、fuzz与 coverage；
- TypeScript/Python formatter、typecheck、unit和集成测试，仅在相关波次存在时执行；
- 无真实 Key的普通测试禁网并通过；
- `git diff --check`与相关文件范围审计；
- README、design、plan、matrix、module、properties和 memory一致；
- 未实际运行的命令不得写成通过。

仓库当前没有统一覆盖率门禁，因此本计划不擅自设定一个百分比。新增高风险分支必须有正反例，最终覆盖率记录实际结果并由审核决定是否设门禁。

## 8. 主要风险与控制

| 风险 | 控制 | 回退 |
|---|---|---|
| 总计划过大导致混乱 | 按波次和 Route设计卡授权 | 未授权波次保持待办，不生成空壳 |
| 协议复用吞掉方言 | Protocol与 Provider职责分离、未知字段测试 | 方言移回 Provider，不回退公共语义 |
| 订阅条款违规 | `allowed_usage`硬门禁、真实身份、`terms_blocked`不可注册 | 禁用路线，只保留 Catalog记录 |
| Credential发错 Host | audience/allowlist绑定、Key前缀和 Endpoint校验 | 在配置阶段拒绝，不发送请求 |
| 自动回退产生付费 | 默认无回退，显式策略与 MappingReport | 关闭策略，返回额度/订阅错误 |
| Sidecar增加故障面 | supervisor、资源上限、版本 IPC、故障注入 | 禁用该执行器，保留其他路线 |
| 云协议快速变化 | 7天 TTL、模型兼容表和 maturity路由 | 标记 `stale`，停止发布 |
| Mock与真实 API偏差 | 明确离线状态、Route级烟测 | 只修正失败 Route，不扩大结论 |
| 第三方实际后端不透明 | 保存 billing owner、model vendor、serving backend | 证据不足标 `unverified`并拒绝生产 |

任何回退只作用于当前波次相关文件，不使用 `git reset --hard`、`git checkout --`，不覆盖用户或其他任务的未提交改动。

## 9. 后续波次启动门槛

用户已经授权按本计划完成离线构建，波次 A与 B已经完成。后续仍需在对应波次用当前官方证据确认：

1. Route七维身份与 `allowed_usage`三态已经由波次 A固定；后续变更必须重新提供设计和测试证据；
2. 第三阶段继续按 A→H波次推进，不一次实现全部 Provider；
3. 波次 C已核验 Kimi Code与 MiniMax Token Plan，并按 `interactive_coding_only + planned + callable=false`建档；
4. 波次 C已确认 GLM、MiMo、Alibaba的旧全面阻塞结论被当前官方资料推翻；替代结论同为仅交互式、禁止后端、无 Adapter且不可调用；
5. 波次 D已按 AWS → Vertex → Azure顺序完成离线实施；
6. 到波次 E时核验重点直连 E1的实施顺序；
7. 到波次 F前确认哪条官方路线的独占能力足以触发 TypeScript或 Python Sidecar落地；
8. 到波次 G前确认第三方托管首批名单；
9. 真实烟测继续逐路单独授权。

波次 A、B、C、D及 E1全部按现有离线授权完成。F触发审计确认当前无非 Go独占能力证据，未生成 Sidecar空壳；G第三方首批名单与 H真实烟测按活动 goal明确延期。最终离线总验收、资产一致性审计和计划收口已经通过；当前无本计划内仍在执行的下一项。

## 10. 完成条件

本第三阶段总计划只有在以下条件全部满足后才可标记完成并转为陈旧计划：

1. Route、Catalog、协议、Credential与 Offering基础完整；Sidecar有触发证据时基础完整，无触发证据时有明确审计结论；
2. 现有三家无回归；
3. 用户批准的重点直连、订阅和云托管 Route全部达到各自计划状态；
4. 条款阻塞路线确实不可调用；
5. 第三方托管首批清单完成或被用户明确延期；
6. 所有相关黑白盒、安全、生命周期和一致性测试实际通过；
7. 真实烟测按用户逐路决定完成或明确延期；
8. Matrix、module、properties与 memory同步为 live state；
9. 没有把 `research_only`、`unverified`、`terms_blocked`或离线结果误写成生产支持；
10. 最终审核确认产物与本计划承诺一致。

## 11. 最终离线验收结论

2026-07-11最终审核确认本计划在当前授权范围内完成并转为陈旧计划：

- [x] A完成 Route、Catalog、Credential、Schema与证据门禁；
- [x] B完成四协议 driver、安全错误与公共 SDK边界；
- [x] C完成订阅/商业计划控制面与条款阻塞行为；
- [x] D完成 AWS Bedrock、Vertex AI与 Azure OpenAI云托管离线路线；
- [x] E1完成 DeepSeek、Kimi、Z.AI、MiniMax、MiMo、Qwen与 xAI已批准直连路线；
- [x] F完成触发审计；当前没有非 Go独占能力证据，依计划不生成 Sidecar空壳；
- [x] G第三方首批名单由活动 goal明确延期，未新增伪造的 research/callable路线；
- [x] H已提供显式 smoke入口并完成离线编译；真实账号、套餐、预算、调用与生产评审由活动 goal明确延期；
- [x] Catalog为62条：39条 `implemented_offline + callable=true`与23条无 Adapter控制记录；
- [x] `scripts/verify-offline.sh`实际通过依赖校验、格式、diff、vet、普通、shuffle、全仓 race、integration仅编译与 Catalog资产门禁；
- [x] xAI新增 fuzz 3秒通过16,417次，全仓合并语句覆盖率76.7%；
- [x] README、design、plan、matrix、module、properties和 memory已同步；
- [x] 没有读取真实 Provider Key，没有执行真实认证、模型、订阅或付费调用，没有把离线结果写成生产支持。

计划中的 E2、未来 Sidecar实施清单、第三方名单、真实烟测和生产批准继续保留为陈旧待办；它们必须由新的设计、证据和用户授权重新启动，不属于本次完成声明。
