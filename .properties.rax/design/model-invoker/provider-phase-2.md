# 模型调用器第二阶段 Provider 设计

## 1. 设计状态

- 模块：`model-invoker`
- 阶段：第二阶段，Anthropic 与 Gemini Provider 扩展
- 状态：已完成实施并通过离线验收
- 确认时间：2026-07-10
- 完成时间：2026-07-10
- 授权依据：用户已明确要求继续覆盖各种供应商，并在其审核代码期间继续向下实施
- 第一阶段基线：现有 OpenAI Responses 与 Chat Completions 实现及其离线验收结果

本设计是 `model-invoker` 既有架构的增量，不建立新模块，也不覆盖第一阶段设计和陈旧计划。最终实现、测试与安全终审已按本设计落地；实际命令和结果记录在同模块第二阶段 memory 快照中。

## 2. 本阶段目标

在不泄漏厂商 SDK 类型、不把不同协议伪装成 OpenAI API、且不破坏现有 OpenAI 行为的前提下，新增两个原生 Provider：

1. Anthropic Messages；
2. Gemini GenerateContent。

第二阶段完成后，统一调用层应能以一致的 Runtime 语义表达三家 Provider 的文本、函数工具、结构化输出、推理配置、续接状态、非流式/流式结果、用量、错误和受控原始数据，同时保留每个协议无法无损统一的边界。

## 3. 固定依赖与协议

| Provider | Provider 包 | 原生协议 | 官方 Go SDK | 本阶段默认协议 |
|---|---|---|---|---|
| Anthropic | `provider/anthropic` | Messages | `github.com/anthropics/anthropic-sdk-go v1.56.0` | `messages` |
| Gemini Developer API | `provider/gemini` | GenerateContent / GenerateContentStream | `google.golang.org/genai v1.63.0` | `generate_content` |

版本必须精确锁定。升级 SDK 前必须重新运行请求序列化、响应归一化、流式事件、错误与公开 API 回归测试，并同步 Provider 调查矩阵与 memory。

Gemini 本阶段只面向 Gemini Developer API。Vertex AI 虽可由同一 SDK 访问，但认证、端点、地域、配额和字段支持边界不同，不允许在同一能力声明下静默切换。

## 4. 统一语义增量

### 4.1 Protocol

统一协议增加：

- `ProtocolMessages = "messages"`；
- `ProtocolGenerateContent = "generate_content"`。

`Request.Validate` 与 `Registry.Register` 必须复用同一个协议有效性判断，避免协议白名单在多个位置漂移。Anthropic 默认 `messages`，Gemini 默认 `generate_content`。

本阶段不声明 `interactions` 已支持。Gemini GenerateContent 返回的 `response_id` 是响应标识，不能被解释为服务端会话续接 ID。

### 4.2 工具调用与结果

统一函数结果必须同时容纳“按调用 ID 关联”和“按函数名关联”：

- `FunctionResult` 增加函数名；
- 统一层不再强制所有 `FunctionCall` 都具有厂商调用 ID；
- 函数结果至少提供调用 ID 或函数名之一；
- OpenAI 与 Anthropic Adapter 继续执行各自更严格的 ID 校验；
- Gemini `FunctionResponse` 使用函数名，存在 ID 时同时保留；
- `IsError` 在 Anthropic 中映射为原生 `is_error`，在 Gemini 中映射为结构化 error 结果；不能静默丢弃。

工具参数仍为 JSON Object。工具结果第二阶段继续以统一文本承载；Gemini Adapter 可确定性包装为 `{"output": ...}` 或 `{"error": ...}`。是否引入多模态或任意 JSON 工具结果属于后续独立语义设计。

### 4.3 Reasoning

统一推理配置增加：

- 明确的 thinking/reasoning Token 预算；
- `max` 推理强度；
- 现有 Summary 为空时，语义固定为“不请求统一推理摘要输出”。

映射原则：

- Anthropic 使用 `output_config.effort` 和 `thinking` 配置；显式预算映射为 thinking budget；
- Anthropic 返回的 signed、redacted thinking 必须保留用于续接，但签名和密文不能成为根包厂商字段；
- Gemini 使用 `ThinkingConfig` 的 budget/level；
- Gemini `Part.Thought` 只表示厂商思考内容，不自动等价为 Praxis reasoning summary；不能为了复用现有事件而谎报摘要能力；
- Provider 不支持调用方指定的摘要粒度时，必须拒绝或在调用方允许后显式降级。

### 4.4 State 与连续性

现有只含 `PreviousResponseID` 的 State 是 OpenAI 专属结构，第二阶段改为两类统一状态：

1. `server_continuation`：服务端持有上下文，Praxis 保存续接 ID；
2. `provider_continuation`：Provider 协议要求调用方无损回传的受控原生片段。

State 至少携带：

- State 类型；
- Provider ID；
- Protocol；
- 可选续接 ID；
- 受控、不在普通日志中展开的 Payload。

规则：

- nil State 表示无状态；
- State 不得跨 Provider 或跨 Protocol 使用；
- OpenAI Responses 使用 `server_continuation`；
- Anthropic Messages 与 Gemini GenerateContent 本身不声明服务端会话状态；
- Anthropic thinking signature/redacted block 和 Gemini thought signature 通过 `provider_continuation` 无损往返；
- Provider continuation 使用与 RawPayload 等价的默认脱敏和防御性复制规则；
- 能力契约增加 Provider continuation 能力，并与 Server state 分开判断。

### 4.5 停止原因与响应状态

`ResponseStatus` 继续表示调用生命周期状态；新增统一 StopReason 表示模型停止生成的原因。最小统一集合：

- `end_turn`；
- `max_output_tokens`；
- `stop_sequence`；
- `tool_call`；
- `content_filter`；
- `paused`；
- `other`。

自定义停止序列应单独保留。Anthropic `pause_turn` 映射为 `paused`，Gemini safety/prohibited 类终止映射为策略拒绝或 `content_filter`，厂商更细的原始枚举保留在 RawResponse/ProviderMetadata。

### 4.6 Stream

统一事件族继续覆盖：response started、text delta、function call started、arguments delta、function call completed、reasoning delta、usage、response completed、error、native。

第二阶段增加专用 reasoning delta 字段，不再让 reasoning 事件借用普通文本字段。统一约束：

- Provider 没有原生 sequence 时，由 Adapter 从 1 开始分配严格递增的本地序号；
- 原生顺序不得重排；
- 所有原生事件进入受控 NativeEvents；
- Anthropic signature delta 和 Gemini thought signature 不作为可展示推理文本，必须进入受控续接状态与原生审计；
- 流式调用不自动重放；
- Close 必须幂等并传播取消；
- Gemini SDK 的迭代器没有统一 Stream Close 时，Adapter 必须持有可取消的子 Context 以完成关闭。

### 4.7 Usage

统一 Usage 明确区分：

- Input tokens；
- Output tokens；
- Reasoning tokens；
- Cache read tokens；
- Cache write/creation tokens；
- Total tokens。

归一化口径：

- InputTokens 是本次有效输入总量，包含缓存参与和工具结果输入；
- OutputTokens 是生成总量，包含 reasoning；
- Reasoning、cache read、cache write 是明细，不应在业务侧再次与总量重复相加；
- TotalTokens 优先保留 Provider 权威总量；Provider 没有总量时，Adapter 按已定义口径计算并记录转换；
- Anthropic 的 cache creation/read 必须分别保存；
- Gemini 的 prompt、tool-use prompt、candidate、thought 与 cached-content 计数必须按上述口径归一化，详细模态分解保留在 RawResponse。

### 4.8 根包校验边界

根包只校验跨 Provider 恒定的结构不变量。以下限制下沉到具体 Adapter：

- 工具名字符集和最大长度；
- Metadata 数量及键值长度；
- Provider 对 JSON Schema 子集和 Strict 的限制；
- Provider 对角色、工具调用 ID、输出候选数和模型参数的限制。

禁止把 Anthropic 或 Gemini 的专有请求字段加入统一 Request。厂商特有配置必须通过各 Provider 包提供的类型安全辅助方法编码到命名空间隔离的 ProviderOptions。

## 5. Provider 原生映射

### 5.1 Anthropic Messages

- system/developer 指令映射为 Messages 顶层 system 内容；developer 角色转换必须记录；
- user/assistant 文本和工具块按原顺序组合，不能因为角色合并打乱工具关联；
- client tool、tool choice、并行调用开关按 Messages 原生字段映射；
- tool use ID、JSON input、tool result 与 `is_error` 无损往返；
- JSON Schema 输出按官方 `output_config.format` 映射；仅 JSON Object 而无 Schema 时，不得未经设计擅自合成约束；
- effort、thinking budget 和可展示摘要按实际支持级别映射；
- signed/redacted thinking 进入 Provider continuation；
- 非流式保留 stop reason、stop sequence、request ID、usage 与原始响应；
- 流式覆盖 message/content block start、delta、stop、tool JSON delta、thinking、signature、usage 和 error；
- SDK 内建重试必须关闭，由 Runtime 继续拥有非流式重试策略。

### 5.2 Gemini GenerateContent

- system/developer 指令映射到 `SystemInstruction`，转换必须记录；
- user 映射为 user Content，assistant 映射为 model Content；
- 相邻 Part 必须在不改变语义顺序的前提下组合；
- FunctionCall 名称必需，ID 可选；FunctionResponse 名称必需，存在 ID 时保留；
- JSON Object 使用 `application/json`，JSON Schema 使用 `ResponseJsonSchema`；SDK/API 不支持的 Schema 关键字必须显式拒绝或降级；
- ThinkingConfig budget/level 与统一 reasoning 映射；原始 thought 不自动归一为 summary；
- thought signature 进入 Provider continuation，并在后续工具轮次原样回传；
- 只归一化第一个候选；请求多个候选不在本阶段范围，必须拒绝，不能默认丢弃；
- GenerateContentStream 的每个响应块按收到顺序转换并累计；
- PromptFeedback、FinishReason、ResponseID、UsageMetadata、HTTP 请求 ID 和受控原始数据均须保留；
- Gemini Developer API 与 Vertex AI 的能力不能共用一个未经区分的声明。

## 6. 分层与目录

```text
ExecutionRuntime/model-invoker/
|-- internal/
|   |-- adaptercore/
|   |   |-- audit.go
|   |   |-- capability.go
|   |   |-- endpoint.go
|   |   |-- headers.go
|   |   `-- validate.go
|   `-- testkit/providercontract/
|       `-- suite.go
|-- provider/
|   |-- openai/
|   |-- anthropic/
|   |   |-- adapter.go
|   |   |-- client.go
|   |   |-- config.go
|   |   |-- errors.go
|   |   |-- mapping.go
|   |   |-- normalize.go
|   |   `-- stream.go
|   `-- gemini/
|       |-- adapter.go
|       |-- client.go
|       |-- config.go
|       |-- errors.go
|       |-- mapping.go
|       |-- normalize.go
|       `-- stream.go
`-- tests/
    |-- core/
    |-- openai/
    |-- anthropic/
    |   `-- testdata/
    |-- gemini/
    |   `-- testdata/
    `-- integration/
```

`internal/adaptercore` 只能依赖 Go 标准库和 `modelinvoker` 根包，不允许导入任何厂商 SDK。它只承载已出现至少两个真实消费者的 SDK 无关脚手架：Endpoint 归一化、Raw 审计、标准 Header/Retry-After 解析、基础能力契约和 Provider/Protocol/Options namespace 校验。

消息、工具、推理、响应、错误方言和流状态机归各 Provider 所有。当前不抽象 OpenAI/Anthropic 兼容协议 codec；只有出现第二个真实兼容 Provider 并完成方言对比后才允许提取，避免过早制造错误抽象。

## 7. 错误、重试与安全

- SDK 错误不能越过 Provider 包进入公开 unwrap 链；
- 统一错误必须保留厂商错误码、HTTP 状态、请求 ID、Retry-After 和可重试判断；
- Runtime 是非流式重试的唯一所有者；SDK 自动重试必须关闭或经测试证明未启用；
- 流式调用永不自动重放；
- Authorization、API Key、Google API Key/query credential 不得进入 RawRequest、RawResponse、NativeEvents、错误或普通日志；
- Raw 与 continuation Payload 默认格式化和 JSON 输出只显示 `[REDACTED]`，读取时返回防御性副本；
- 2xx 畸形响应视为不可重试的 Provider 协议错误。

## 8. 明确不做

第二阶段不实现：

- Gemini Interactions、Live、Realtime、Batch；
- Vertex AI 方言、服务账号认证、地域路由；
- Anthropic Message Batches、Token Count、Hosted/Server Tools、Container 执行；
- 图片、音频、视频、文件和多模态工具结果；
- Prompt Cache 的创建、管理和缓存策略统一；本阶段只归一化已返回的缓存用量；
- 多候选统一、logprobs、采样参数统一、停止序列统一配置；
- xAI、DeepSeek、Kimi、MiniMax、Qwen、Meta、GLM、MiMo；
- OpenAI/Anthropic 兼容 Provider codec 抽取；
- TypeScript Sidecar、IPC、Rust、Agent 编排；
- 真实 API 联调和生产可用性声明。

未实现能力必须在 CapabilityContract 中标记为 Unsupported 或 Partial，不创建空壳，不静默忽略字段。

## 9. 测试设计

### 9.1 统一契约

所有 Provider 运行同一套 Provider contract，至少验证：

- ID、默认协议、协议拒绝和 Endpoint 约束；
- 能力契约完整、限制可追踪；
- Provider/Protocol/State/ProviderOptions 不串用；
- context 取消和超时；
- 非流式响应身份、MappingReport、Raw 安全；
- 流顺序、终态、Close 幂等和错误保持；
- SDK 类型不进入根包公共 API。

### 9.2 Provider 白盒与黑盒

- 结构白盒：通过窄客户端、确定性 fake、可观察的流累加器和取消状态覆盖映射、状态机、错误分支与边界；
- 黑盒：外部测试包只调用公共 API，以 `httptest` 验证真实 HTTP 路径、认证头、请求体、SDK 解码、SSE/流响应和可观察结果；
- 固定协议样本：成功、工具、结构化输出、推理、续接、usage、限流、策略拒绝、畸形响应；
- Fuzz：畸形 JSON、SSE、函数参数增量、未知事件和错误载荷；
- 普通测试必须清除全部真实 Key，并在 `GOPROXY=off` 下执行。

### 9.3 真实烟测

本阶段只建立显式 build tag 的真实烟测入口，不读取、不创建、不复用真实 Key，也不执行公网请求。真实烟测等待用户另行批准具体账号、模型、预算和调用次数后执行；未执行的真实烟测不得计入通过。

## 10. 设计完成后的可预见产物

1. 统一语义能够真实表达三家 Provider 的核心 Agent 调用链；
2. Anthropic Messages 和 Gemini GenerateContent 均有独立 Provider 包；
3. 共享代码只存在于 SDK 无关的内部 Adapter 脚手架；
4. 每个 Provider 都有独立黑盒、结构白盒、协议样本和延期真实烟测入口；
5. 现有 OpenAI 行为经过完整回归且不被新协议改变；
6. 未支持能力和真实联调边界在 Capability、说明资产和验收记录中明确可见。
