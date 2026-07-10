# 模型调用器架构与语义映射

## 1. 核心判断

Praxis 不直接围绕模型品牌构建统一接口，也不把所有厂商假设为同一种 OpenAI API。

模型品牌、服务提供商、API 协议和具体端点是四个不同概念：

- 模型品牌描述模型能力；
- Provider 描述实际提供 API、认证、计费和运维责任的服务方；
- Protocol 描述请求、响应和流式事件的结构；
- Endpoint 描述具体地域、产品计划和 API 版本的访问入口。

Praxis 以 Provider 为原生实现边界，以语义能力为 Runtime 统一边界。

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
Provider Registry
      |
      +--> OpenAI Adapter
      +--> Anthropic Adapter
      +--> Gemini Adapter
      +--> xAI Adapter
      +--> DeepSeek Adapter
      +--> Kimi Adapter
      +--> MiniMax Adapter
      +--> Qwen Adapter
      +--> Meta Adapter
      +--> GLM Adapter
      `--> MiMo Adapter
```

### 2.1 Provider 原生层

每个 Provider Adapter 对本厂商负责，至少控制：

- 认证方式、Base URL、地域和 API 版本；
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

### 2.2 语义映射层

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

### 2.3 Runtime 统一层

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

1. 官方 Go SDK 能完整表达能力时，直接使用官方 Go SDK。
2. 没有官方 Go SDK，但官方 TypeScript SDK明显拥有更完整的厂商语义时，使用 TypeScript Sidecar。
3. 厂商正式声明兼容 OpenAI 或 Anthropic，且兼容 SDK能完整表达目标能力时，复用对应官方 Go SDK。
4. SDK 无法表达厂商扩展时，在 Provider 内增加原生 Go HTTP、SSE 或 WebSocket 实现。
5. 社区 SDK 只有经过维护状态、协议覆盖、安全性和许可证审核后才能引入。

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

每个 Provider 至少具备：

- 文档 Schema 与请求序列化测试；
- 非流式响应解析测试；
- 流式事件顺序和增量拼接测试；
- 工具调用往返测试；
- 能力拒绝与显式降级测试；
- 错误、超时、取消和重试测试；
- Raw 数据保留和脱敏测试；
- 使用真实 API Key 的最小烟雾测试；
- SDK 升级后的兼容回归测试。

统一层必须使用同一组语义契约测试运行所有 Provider，验证相同意图的行为边界。

## 9. 反目标

- 不构建一个包含所有厂商字段的巨大公共请求结构；
- 不以更换 Base URL 作为完整 Provider 适配；
- 不把未报错视为兼容成功；
- 不静默忽略无法映射的字段；
- 不允许 SDK 类型成为 Praxis Runtime 的公共 API；
- 不在缺少测量证据时引入 Rust；
- 不在本阶段实现 Agent 编排。
