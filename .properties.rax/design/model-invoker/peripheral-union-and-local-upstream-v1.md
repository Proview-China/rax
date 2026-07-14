# 外围能力并集与本地上游 v1 设计

## 1. 状态

- 状态：`v1 implemented_offline`
- 日期：2026-07-14
- 上游事实基线：[上游外围能力官方事实研究](./upstream-peripheral-capability-research-20260714.md)
- 实现位置：`ExecutionRuntime/model-invoker/`

## 2. 设计决定

现有 `modelinvoker.Provider` 和 `Request/Response/Stream` 继续专注 LLM 文本、工具调用和推理。外围能力新增正交包，不向现有请求无限加字段：

```text
modelinvoker          LLM 单次/流式调用
operation             同步或异步外围操作
resource              Files/Store/Collection/Cache 等资源操作
realtime              双向长连接会话
provider/*            厂商方言与传输
```

这些包共享 `ProviderID`、统一错误、脱敏载荷和 Route 身份，但接口生命周期分离。

## 3. Operation 类型体系

### 3.1 OperationKind

首批固定：

- `embedding.create`
- `rerank.create`
- `moderation.create`
- `image.generate`、`image.edit`、`image.variation`
- `video.generate`、`video.edit`、`video.extend`、`video.remix`
- `audio.transcribe`、`audio.translate`
- `speech.generate`
- `music.generate`
- `token.count`
- `batch.create`、`batch.get`、`batch.cancel`、`batch.results`

### 3.2 输入

`OperationRequest` 固定：Route、Kind、Model、Prompt、Inputs、Output、Budget、Metadata、ProviderOptions。

`ArtifactInput` 只允许四种来源：

- 内联字节；
- URL；
- Provider Resource ID；
- 文本。

每个输入必须带媒体类型；字节受调用方预算限制。`ProviderOptions` 只能位于所选 Provider 命名空间，方言必须报告已消费字段，剩余字段一律拒绝。

### 3.3 结果

`OperationResult` 固定：

- `Status`：completed/incomplete/failed/cancelled/in_progress/queued；
- `Artifacts[]`：类型、MIME、字节、URL、Resource ID、大小、Hash、过期时间；
- `Job`：异步作业引用；
- `Vectors[]`、`Rankings[]`、`Transcript` 等稳定结果槽；
- `Usage`、`ProviderMetadata`、`MappingReport`；
- 脱敏的 RawRequest/RawResponse。

短期 URL 不能冒充持久资源；必须携带 `ExpiresAt` 或 `ExpiryUnknown=true`。

## 4. Resource 协议

`ResourceKind` 首批：file、upload、vector_store、collection、cache、voice、video_character。

`ResourceProvider` 提供：Create、Get、List、Delete、Content、Search。Provider 可只实现子集，能力合同精确到 action。

删除是外部状态改变，不由自动重试器盲目重放；Create 只有具备官方 idempotency 证据或调用方 idempotency key 时才允许自动重试。

## 5. Job 协议

统一 Job 状态机：

```text
queued -> validating -> running -> finalizing -> succeeded
   |          |           |            |
   +----------+-----------+------------+-> failed/cancelling/cancelled/expired
```

Provider 原生状态映射到上述状态，同时保留 `NativeStatus`。未知状态不猜测，返回 `unknown` 并保留原文。

结果获取可以返回 Artifacts、ResourceRefs 或结果文件。轮询策略属于调用方/Runtime Policy，不属于 Provider SDK。

所有正常结束的HTTP流必须恰好产生一个`StreamCompleted`。SSE的`[DONE]`显式生成该事件；NDJSON或binary在正常EOF时合成一次该事件；异常EOF只保留`Err`，不得同时伪造成功终态。

## 6. Realtime 协议

`RealtimeProvider.Open(ctx, SessionRequest) (Session, error)` 返回连接对象：

- `Send(ClientEvent)`；
- `Next() bool`、`Event()`、`Err()`；
- `CloseWrite()`；
- `Close()`。

事件固定连接、会话配置、输入音频/视频/文本、输出音频/文本增量、工具调用、usage、error、session end；Provider 原生事件始终可审计。SSE 不允许冒充 Realtime。

WebRTC/SIP 由信令适配器返回受限连接描述；Praxis Go 内核不直接拥有浏览器媒体轨道。

## 7. 通用本地与中转 Route

### 7.1 身份

新增独立身份：

- `local-openai-compatible`
- `ollama-native`
- `ollama-openai-compatible`
- `llamacpp-native`
- `llamacpp-openai-compatible`

它们不能复用 `openai` 或 `third-party-relay` Provider ID。

### 7.2 Endpoint 与认证

- HTTP 只允许 loopback；非 loopback 必须 HTTPS，除非后续 RuntimePolicy 明确加入受信企业网段。
- Base URL、operation path、模型和 capability 都要白名单。
- 支持 `anonymous`、Bearer、API-Key header 三种明确认证模式；空 Key 只有 `anonymous` 模式允许。
- 禁止 caller 通过 Metadata/ProviderOptions 注入 Authorization、Host、Cookie 或转发头。

### 7.3 探测

探测结果是有时效的 `CapabilitySnapshot`，来源可以是：

- `/v1/models`、Ollama tags/show、llama.cpp props/health；
- 服务器版本和启动 capability；
- 管理员显式声明。

探测失败不自动扩大权限；最多回退到管理员显式 allowlist。

## 8. 实施切片

### v1 核心

1. `operation` 公共类型、校验、Registry、Invoker、能力合同和统一 HTTP transport；
2. `resource` 与 `realtime` 公共接口；
3. 通用 OpenAI-family operation 方言，覆盖 Embedding、Images、Audio、Speech、Videos、Files、Vector Stores、Batch、Moderation；
4. `local-openai-compatible`、Ollama 与 llama.cpp 的文本/Embedding/Rerank/Images 能力；
5. 供应商 operation catalog，先允许以官方原生 JSON/multipart 调用未完成高级映射的能力，同时稳定归一化 Job/Artifact/Resource；
6. OpenAI、Anthropic、Gemini、xAI、Kimi、MiniMax、Z.AI、Qwen、MiMo 的显式 operation spec。

### 后续增强

- 每家媒体参数的高级强类型 builder；
- WebRTC/SIP 浏览器/边缘 Sidecar；
- 云平台异步推理与企业私网信任配置；
- 资源迁移和统一资产缓存。

## 9. 验收

必须证明：

1. 未声明能力会在发网前拒绝；
2. endpoint/path/model/auth 不能由请求绕过；
3. anonymous 仅限显式本地 Route；
4. Raw/错误/格式化不泄漏 Key；
5. JSON、multipart、binary、SSE 和 WebSocket 生命周期分别测试；
6. 作业状态、短期 URL、资源 ID 和产物字节不混淆；
7. Ollama/llama.cpp 的 capability 根据配置收缩；
8. 官方、第三方中转和本地 Route 身份不互换；
9. 普通、shuffle、race、vet、fuzz、httptest 黑盒和无公网离线门禁通过；
10. 真实测试只验证用户授权 Route，凭据不落盘，已经通过的文本/Tool Call 不重复消耗。
