# 模型调用器架构与语义映射

## 1. 核心判断

Praxis 不直接围绕模型品牌构建统一接口，也不把所有厂商、订阅计划和托管平台假设为同一种 OpenAI API。

一条可执行上游路由由七个不同概念组成：

- Model Family 描述模型家族与能力来源，例如 Claude、GPT、Gemini、GLM；
- Provider 描述真正接收请求并承担 API、认证、计费和运维责任的服务方；
- Offering 描述商业与使用产品，例如按量 API、Token Plan、Coding Plan、预置吞吐或企业订阅；
- Deployment 描述账号、项目、Workspace、地域、云资源、部署名与实际推理后端；
- Protocol 描述请求、响应和流式事件结构，例如 Responses、Chat Completions、Messages、GenerateContent 或 Bedrock Converse；
- Endpoint 描述精确的 Host、Path、API Version 与区域入口；
- Credential Profile 描述鉴权类型、权限范围和密钥引用，不保存明文密钥。

运行时使用 `UpstreamRoute = Model Family + Provider + Offering + Deployment + Protocol + Endpoint + Credential Profile` 作为完整身份。Praxis 以 Provider 与 Offering 的组合为实现和条款边界，以语义能力为 Runtime 统一边界。

## 2. 分层结构

```text
Praxis Runtime
      |
      v
Unified Semantic Model API
      |
      v
Capability Router
      |
      v
Semantic Mapper
      |
      v
Upstream Route Resolver
      |
      +--> Provider Adapter Registry
      |     +--> Direct vendor APIs
      |     +--> Official subscription plans
      |     `--> Cloud and third-party managed Providers
      |
      +--> Protocol Driver Registry
      |     +--> OpenAI Responses / Chat Completions
      |     +--> Anthropic Messages
      |     +--> Gemini GenerateContent
      |     +--> Bedrock Converse / InvokeModel
      |     `--> Vendor-native protocols
      |
      `--> Language Executor
            +--> Go native
            +--> TypeScript Sidecar
            `--> Evidence-gated Python Sidecar
```

### 2.1 上游路由身份

- 同一个模型家族由不同托管方提供时，必须创建不同 Provider 路由。例如 Anthropic 直连、AWS Bedrock Claude 与 Vertex AI Claude 不能共用认证、Endpoint 或能力声明；
- 同一厂商的按量 API 与 Token/Coding Plan 必须创建不同 Offering，分别维护 Key、Base URL、配额、允许场景和条款；
- 同一 Offering 在不同 Region、Workspace、云账号或实际 serving backend 上必须创建不同 Deployment；
- “OpenAI 兼容”或“Anthropic 兼容”只归类 Protocol，不能替代 Provider Adapter；
- 路由解析结果必须可审计，并能解释为什么选择某个商业计划、部署、协议和语言执行器。

### 2.2 Provider 原生层

每个 Provider Adapter 对本厂商负责，至少控制：

- 商业计划、允许使用场景和订阅状态；
- 认证方式、Base URL、地域、Deployment 和 API 版本；
- 支持的协议和模型；
- 请求字段、默认值和约束；
- 非流式响应与完整流式事件；
- 推理内容和签名字段；
- 工具声明、工具选择和工具结果回传；
- 文本、图片、音频、视频和文件输入；
- 结构化输出；
- 缓存、服务端状态、Batch 和后台执行；
- 错误码、请求 ID、限流信息和用量；
- 厂商独有字段；
- SDK 或协议版本变化。

Provider 原生层允许公开本厂商特有的方法，但不能让第三方 SDK 类型越过该层进入 Runtime。

### 2.3 语义映射层

语义映射层把 Runtime 的意图转换为 Provider 可以无歧义执行的请求，并把 Provider 事件转换为 Praxis 事件。

它必须处理：

- 同一语义在不同协议中的字段差异；
- system、developer、user、assistant 和 tool 角色差异；
- message、content block、input item 和 part 的结构差异；
- 工具调用 ID、参数增量和工具结果关联；
- reasoning、thinking 和 reasoning content 的差异；
- JSON Mode 与 JSON Schema Structured Output 的差异；
- stateless history、previous response ID 和 interaction ID 的差异；
- SSE、WebSocket 和厂商原生流事件的差异；
- unsupported、ignored 和 silently degraded 参数。

### 2.4 Runtime 统一层

Runtime 只依赖 Praxis 自己的稳定语义，不依赖厂商 SDK。

第一版语义域建议包含：

- `Input`：文本和多模态内容；
- `Instruction`：系统或开发者约束；
- `Tool`：工具定义、选择策略和工具结果；
- `OutputConstraint`：文本、JSON Object 或 JSON Schema；
- `Reasoning`：是否启用、强度和是否允许返回摘要；
- `State`：无状态、Provider 会话 ID 或继续执行 ID；
- `Stream`：是否流式和事件订阅要求；
- `Budget`：Token、时间和成本约束；
- `Metadata`：追踪、租户和业务标签；
- `ProviderOptions`：经过命名空间隔离的厂商扩展。

### 2.5 Route Policy/Audit层

`RouteInvoker`把显式 `RouteID`与语义 `Request`候选组合。调用方不得再提供 Provider、Protocol或 Endpoint；该层从活动 Catalog绑定这三个选择器，并按固定顺序检查 callable状态、调用时 evidence、static model、Offering policy、订阅 entitlement、禁止自动 PAYG和 Adapter注册，之后才进入预构造的基础 `Invoker`。

`Resolve`只做离线预检，不调用任何 Provider方法。`RouteSelection`保存七维身份、evidence digest和 PolicyDecision；Provider语义映射仍保存在原有 `Response.MappingReport`。该层不解析秘密、不构造 Adapter、不拥有实例生命周期，因此不是完整 Route Gateway。阶段性合同见[Route Policy/Audit Invoker v1候选](./route-invocation-facade-v1.md)，完整组合设计见[上游调用最终候选设计](./route-gateway-final-candidate.md)。

## 3. 能力并集

每项 Provider 能力必须声明支持级别：

| 级别 | 含义 |
|---|---|
| `Native` | 厂商原生完整支持，Praxis 可无损映射 |
| `Compatible` | 通过兼容协议支持，核心语义可保持 |
| `Partial` | 只支持部分字段、内容类型或行为 |
| `Unsupported` | 厂商不支持，禁止调用 |

能力清单至少覆盖：

- `TextGeneration`
- `Streaming`
- `ToolCalling`
- `ParallelToolCalling`
- `StructuredOutput`
- `Reasoning`
- `VisionInput`
- `AudioInput`
- `VideoInput`
- `FileInput`
- `ServerState`
- `PromptCaching`
- `Batch`
- `BackgroundExecution`
- `Realtime`
- `HostedTools`
- `UsageReporting`

能力声明必须包含限制条件，不能只有布尔值。例如 `VisionInput=Partial` 还应说明支持的模型、格式、大小和协议。

## 4. 智能映射流程

智能映射必须是确定性的能力决策，不是隐式猜测。

```text
Validate Intent
      |
      v
Resolve Provider and Protocol
      |
      v
Check Capability Contract
      |
      v
Build Mapping Plan
      |
      +--> Exact Mapping
      +--> Explicit Downgrade
      `--> Reject Unsupported
      |
      v
Invoke Provider
      |
      v
Normalize Events and Preserve Raw Data
      |
      v
Audit Mapping Result
```

映射计划必须能够回答：

1. Runtime 请求了什么语义；
2. 选择了哪个 Provider、协议和端点；
3. 哪些字段被原样映射；
4. 哪些字段发生转换；
5. 哪些能力发生显式降级；
6. 哪些厂商扩展被使用；
7. 最终返回了哪些原始事件和统一事件。

禁止静默删除参数。只有调用者明确允许降级时，`Partial` 能力才可以执行。

## 5. SDK 与语言策略

### 5.1 选择顺序

SDK 与语言必须按 `UpstreamRoute` 决策，不能只按模型品牌或厂商做一次全局选择：

1. 该路由有官方 Go SDK且能完整表达目标能力时，直接使用并封装；
2. 该路由的官方接入说明指定 TypeScript、Python 或其他官方 SDK，且 SDK包含公开 HTTP 文档无法忠实表达的语义时，使用隔离 Sidecar；
3. 该路由公开了完整原生 HTTP、SSE、WebSocket 或 gRPC 合同，但没有合适 Go SDK时，在 Provider 内实现原生 Go 客户端；
4. 厂商正式支持 OpenAI 或 Anthropic 兼容协议时，复用协议方官方 Go SDK或自有 codec，但必须为该路由建立独立方言和契约测试；
5. 同一 Offering 官方同时开放 OpenAI Chat Completions、Responses 或 Anthropic Messages 时，按官方边界尽量覆盖多协议，不用单一路径代替全部能力；
6. 社区 SDK 只有经过维护状态、协议覆盖、安全性和许可证审核后才能引入，且不能被标为厂商官方 SDK。

“厂商第一方 SDK”和“协议方官方 SDK调用兼容端点”必须分开记录。后者只证明传输与部分协议可复用，不证明 SDK、能力或支持责任属于该 Provider。

### 5.2 Go 的职责

Go 始终负责：

- Runtime 官方调用接口；
- Provider Registry；
- Capability Contract；
- 语义映射与路由；
- 取消、超时、策略和审计；
- 统一事件、错误和用量；
- Sidecar 生命周期管理。

### 5.3 TypeScript Sidecar 的边界

TypeScript 只能作为可替换的 Provider 执行后端：

- 使用版本化的进程通信契约；
- 不向 Go 暴露 SDK 类、异常或运行时对象；
- 支持健康检查、取消、超时和优雅退出；
- 流式事件必须通过统一序列传回 Go；
- 崩溃不能拖垮 Go Runtime；
- 同一 Provider 的 Go 与 TypeScript 实现必须使用同一套一致性测试。

是否采用 gRPC、Connect 或 JSON-RPC，进入 plan 前再确定。

### 5.4 非 Go 官方 SDK 的执行边界

- Go 仍是路由、能力、策略、重试、审计和生命周期的唯一所有者；
- TypeScript 或 Python 只承载已经通过 Provider 证据审核的官方 SDK，不形成第二套 Runtime 语义；
- Sidecar 必须声明运行时、包版本、许可证、协议覆盖、资源上限和供应链锁文件；
- 每种语言执行器使用同一版本化 IPC 和同一 Provider 契约测试，不能按 SDK 临时发明接口；
- 官方 SDK没有独占语义时，不因为“官方提供了某语言示例”就自动增加 Sidecar；
- Python Sidecar 只有在某条已批准路由确实依赖 Python 官方 SDK时才落地，不能预先成为全局依赖。

### 5.5 订阅计划与托管 Provider 门禁

- Token Plan、Coding Plan、会员权益、按量 API、预置吞吐和企业订阅分别建 Offering；
- 每个 Offering必须声明 `allowed_usage`：`general_api`、`interactive_coding_only` 或 `official_client_only`；
- Key、Base URL、模型别名、配额窗口、并发、自动续费和余额回退不得跨 Offering 推断或复用；
- 订阅计划若限制工具、客户端身份、个人使用或生产用途，Praxis 必须保存并执行该限制；
- 需要伪造 User-Agent、客户端标识或冒充受支持工具才能调用的路线一律标记 `条款阻塞`；
- AWS Bedrock、Google Vertex AI、Azure AI 等托管平台是独立 Provider，不能继承模型原厂直连 API 的能力合同；
- 第三方托管同一模型时，模型来源、服务运营方、实际 serving backend、地域和数据边界分别记录；
- “支持某订阅或托管路线”至少要求官方允许该场景、离线协议测试和明确授权的真实烟测，只有目录记录不得标为已实现。

## 6. 原始信息保留

为了覆盖厂商完整细则，每次调用应保留：

- `RawRequest`：发送给 Provider 的最终请求；
- `RawResponse`：Provider 的原始响应；
- `NativeEvents`：未经丢失的流式事件；
- `RequestID`：厂商请求标识；
- `ProviderMetadata`：限流、缓存、地域和计费信息；
- `MappingReport`：语义转换和降级记录。

原始数据必须服从密钥、隐私、租户和日志脱敏策略，不能默认完整写入普通日志。

## 7. 错误与重试

统一错误至少区分：

- 认证失败；
- 权限或模型不可用；
- 请求无效；
- 能力不支持；
- 限流；
- 超时或取消；
- Provider 暂时故障；
- 内容策略拒绝；
- 流中断；
- 映射失败；
- 未知 Provider 错误。

统一错误必须保留厂商错误码、HTTP 状态、请求 ID 和可重试判断。重试策略不能完全交给 SDK，否则 Runtime 和 SDK 可能发生叠加重试。

## 8. 测试要求

每条获批 `UpstreamRoute` 至少具备：

- Provider、Offering、Deployment、Protocol、Endpoint 与 Credential Profile 解析测试；
- 订阅有效期、配额耗尽、条款阻塞和禁止自动跨计划回退测试；
- 文档 Schema 与请求序列化测试；
- 非流式响应解析测试；
- 流式事件顺序和增量拼接测试；
- 工具调用往返测试；
- 能力拒绝与显式降级测试；
- 错误、超时、取消和重试测试；
- Raw 数据保留和脱敏测试；
- 使用真实 API Key 的最小烟雾测试；
- SDK 升级后的兼容回归测试。

统一层必须使用同一组语义契约测试运行所有 Provider 路由，验证相同意图的行为边界。同一模型在直连、订阅计划和云托管路径上也必须分别执行，不能用其中一条路线的通过代替其他路线。

## 9. 反目标

- 不构建一个包含所有厂商字段的巨大公共请求结构；
- 不以更换 Base URL 作为完整 Provider 适配；
- 不把订阅计划当作按量 API 的折扣别名；
- 不把模型原厂与云托管方视为同一 Provider；
- 不通过伪造客户端身份绕过订阅计划的允许场景；
- 不把未报错视为兼容成功；
- 不静默忽略无法映射的字段；
- 不允许 SDK 类型成为 Praxis Runtime 的公共 API；
- 不在缺少测量证据时引入 Rust；
- 不在本阶段实现 Agent 编排。
