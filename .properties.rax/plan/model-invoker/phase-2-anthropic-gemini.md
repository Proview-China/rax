# 模型调用器第二阶段 Anthropic 与 Gemini 落地计划

## 1. 计划状态

- 模块：`model-invoker`
- 计划版本：`v2`
- 创建时间：2026-07-10
- 当前状态：已完成并转为陈旧计划
- 最终完成时间：2026-07-10
- 验收结论：最终反证审计追加的跨流事件 secret 分片、响应体无界、Gemini continuation 丢失和 Anthropic continuation 绕过均已修复，全部门禁重新通过
- 透明记录：未执行任何认证成功或产生模型输出的真实调用；终审子任务曾错误使用 dummy key 向 OpenAI 公网端点发出一次请求并收到未认证 401，无有效凭据、无模型输出、无计费，发现后已立即禁止后续公网路径
- 实现位置：`ExecutionRuntime/model-invoker/`
- 设计依据：`.properties.rax/design/model-invoker/provider-phase-2.md`
- 第一阶段计划：保留 `.properties.rax/plan/model-invoker/README.md` 为陈旧历史，不覆盖、不续写

## 2. 本阶段完成后产物

本计划完成后应得到：

1. 向后迁移完成的 Provider-neutral 统一语义增量；
2. Anthropic Messages Provider，锁定 `anthropic-sdk-go v1.56.0`；
3. Gemini GenerateContent Provider，锁定 `go-genai v1.63.0`；
4. SDK 无关的内部 Adapter 共享脚手架，且没有厂商 switch 或 SDK 类型；
5. OpenAI、Anthropic、Gemini 共用的 Provider 契约测试；
6. 两个新 Provider 的离线 HTTP/流式黑盒、结构白盒、协议样本和 fuzz；
7. 默认不编译、不运行的真实烟测入口；
8. 更新后的模块说明、项目状态索引和第二阶段 memory 验收快照。

## 3. 范围

### 3.1 本阶段实施

- `messages` 与 `generate_content` 两个统一协议；
- 文本、system/developer 指令和 assistant 历史；
- 函数定义、函数选择、函数调用、函数结果、错误结果与并行工具语义；
- JSON Object 与 JSON Schema 输出约束在各 Provider 的真实支持边界内映射；
- reasoning effort、thinking budget、摘要请求和厂商签名续接；
- server continuation 与 provider continuation 分离；
- 统一停止原因、流 reasoning delta 与 cache read/write usage；
- 非流式、流式、错误、超时、取消、Raw、Request ID、Retry-After 和 MappingReport；
- 现有 OpenAI Provider 的兼容迁移与全量回归。

### 3.2 明确不实施

- Gemini Interactions、Vertex AI、Live、Realtime、Batch、多候选；
- Anthropic Batch、Token Count、Hosted/Server Tools、Container；
- 图片、音频、视频、文件等多模态；
- 缓存创建/管理策略，只保留 usage；
- sampling、logprobs、stop sequences 的统一公共字段；
- 其他 Provider 或兼容协议 codec；
- TypeScript Sidecar、Rust、Agent Run Engine；
- 真实 Key 调用、生产容量或生产可用性结论。

## 4. 技术边界

### 4.1 公共根包

根包只拥有稳定的统一语义、能力、Invoker、Registry、错误、Stream 和 Raw 类型。不得导入 Anthropic、Gemini 或 OpenAI SDK。

本阶段公共语义变更作为一个原子迁移完成，不保留新旧 State 双入口：

- 新增 Messages/GenerateContent Protocol；
- FunctionResult 增加函数名，统一层放宽函数调用 ID；
- Reasoning 增加 Token 预算与 max effort；
- State 改为 server/provider continuation；
- Response 增加统一 StopReason 与停止序列；
- StreamEvent 增加专用 reasoning delta；
- Usage 区分 cache read 与 cache write；
- 增加 Provider continuation capability；
- Provider 特有工具名和 Metadata 限制下沉。

### 4.2 内部共享层

目标目录：`ExecutionRuntime/model-invoker/internal/adaptercore/`。

只允许放入已经具有至少两个真实消费者的 SDK 无关逻辑：

- Endpoint 归一化与一致性检查；
- 受控 JSON/Raw 审计载荷构建；
- 标准 Retry-After 和可配置 Request-ID Header 提取；
- 默认 Unsupported 能力契约和 Query 限制填充；
- Provider、Protocol、Endpoint 和 ProviderOptions namespace 基础校验。

禁止放入：厂商错误码、角色映射、工具映射、Schema 方言、reasoning 映射、usage 映射、原生流事件或 SDK 类型。

### 4.3 Provider 层

```text
provider/anthropic/
|-- adapter.go
|-- client.go
|-- config.go
|-- errors.go
|-- mapping.go
|-- normalize.go
`-- stream.go

provider/gemini/
|-- adapter.go
|-- client.go
|-- config.go
|-- errors.go
|-- mapping.go
|-- normalize.go
`-- stream.go
```

每个 Provider 使用 SDK 窄接口，配置、认证、能力、映射、错误和流生命周期互不串用。Gemini Provider 只配置 Gemini Developer API；Vertex AI 配置必须拒绝。

## 5. 细粒度实施清单

### 阶段 A：控制资产与统一契约

- [x] 固定 Anthropic Messages 与 Gemini GenerateContent 范围；
- [x] 固定 SDK 版本 `v1.56.0` 与 `v1.63.0`；
- [x] 固定统一语义增量、目录、测试和真实烟测边界；
- [x] 新增统一 Protocol 常量；
- [x] 统一 Request 与 Registry 的协议有效性判断；
- [x] 为 FunctionResult 增加函数名；
- [x] 调整 FunctionCall/FunctionResult 的 ID/Name 校验；
- [x] 为 Reasoning 增加 thinking Token 预算；
- [x] 增加 `max` reasoning effort；
- [x] 重构 State 为 server/provider continuation；
- [x] 增加 Provider continuation capability 与能力推导；
- [x] 增加统一 StopReason 和 StopSequence；
- [x] 增加专用 reasoning stream delta；
- [x] 将缓存用量拆分为 read/write；
- [x] 固定 Usage 跨 Provider 归一化口径；
- [x] 将 OpenAI 专有工具名和 Metadata 限制移回 OpenAI Adapter；
- [x] 更新所有公共构造器、克隆、防御性复制和校验测试。

### 阶段 B：OpenAI 兼容迁移与共享脚手架

- [x] 将 OpenAI Responses State 迁移到 server continuation；
- [x] 更新 OpenAI FunctionResult、StopReason、StreamEvent 与 Usage 映射；
- [x] 保证 OpenAI Responses/Chat 原有请求体不发生非预期变化；
- [x] 提取 Endpoint 归一化到 `internal/adaptercore`；
- [x] 提取 Raw 审计构建和 raw fallback；
- [x] 提取标准 Retry-After/可配置 Request-ID Header 读取；
- [x] 提取默认能力契约和基础 Query support 构建；
- [x] 提取 Provider/Protocol/Options namespace 基础校验；
- [x] 保留 OpenAI 错误分类、Schema strict 校验、消息/工具/流映射在 OpenAI 包；
- [x] 运行现有 OpenAI 普通、流式、错误、fuzz 与 race 回归。

### 阶段 C：Anthropic Messages Provider

#### C1. 配置与客户端

- [x] 新建 `provider/anthropic` 七个职责文件；
- [x] 定义 `ProviderID`、默认 Base URL 和 Messages 默认协议；
- [x] Config 校验 API Key、Base URL、HTTP Client 和 Loopback 测试边界；
- [x] 建立官方 SDK 窄接口；
- [x] 明确关闭 Anthropic SDK 自动重试；
- [x] 确认认证头不进入 Raw 或错误文本；
- [x] 实现 Endpoint 与 CapabilityQuery 一致性校验。

#### C2. 能力契约

- [x] 声明文本、流、工具、并行工具、结构化输出、reasoning、reasoning summary、函数错误结果、provider continuation 和 usage；
- [x] 明确 ServerState、Interactions、Realtime、Batch、多模态和 Hosted Tools 的本阶段支持级别；
- [x] 对模型能力只描述 Adapter 映射边界，不维护易漂移静态模型白名单；
- [x] 未支持协议返回统一无效请求/能力错误。

#### C3. 请求映射

- [x] system/developer 指令映射到顶层 system，并记录 developer 转换；
- [x] 保持 user/assistant、tool use、tool result 的原始顺序和角色分组；
- [x] 映射函数定义、Strict、描述和 JSON input schema；
- [x] 映射 auto/none/required/function tool choice；
- [x] 映射并行工具控制；
- [x] 映射 FunctionResult ID、Output 和 IsError；
- [x] 映射最大输出 Token；
- [x] 映射 JSON Schema 输出；
- [x] 对无 Schema 的 JSON Object 明确拒绝或按设计记录兼容转换，禁止静默；
- [x] 映射 effort、thinking budget 和 summary/display；
- [x] 解码并验证 Anthropic provider continuation；
- [x] 序列化最终 RawRequest，排除认证信息。

#### C4. 非流式响应与错误

- [x] 归一化文本、tool use 和 reasoning summary；
- [x] 保存 signed/redacted thinking 为 Provider continuation；
- [x] 映射 stop reason、stop sequence、pause turn 和状态；
- [x] 归一化 input/output/reasoning/cache read/cache write/total usage；
- [x] 保存 request ID、允许的限流/服务元数据和 RawResponse；
- [x] 分类 authentication、permission/not found、invalid request、rate limit、overloaded、policy 和未知错误；
- [x] 解析 Retry-After 并判断 retryable；
- [x] 确保 SDK 错误不进入公开 unwrap 链；
- [x] 将 2xx 畸形载荷分类为不可重试协议错误。

#### C5. 流式响应

- [x] 包装官方 Messages stream；
- [x] 映射 message start/delta/stop；
- [x] 映射 content block start/delta/stop；
- [x] 映射 text、input JSON、thinking 和 usage delta；
- [x] 保留 signature delta 与全部 NativeEvents；
- [x] 累计文本、工具参数、工具调用、reasoning、usage 与 continuation；
- [x] 构造唯一终态 Response；
- [x] 流错误保留已累计 Raw、NativeEvents、Request ID、MappingReport 和 continuation；
- [x] EOF、Close、取消、重复 Close 与终态后停止读取均有断言；
- [x] 证明流式调用不自动重放。

### 阶段 D：Gemini GenerateContent Provider

#### D1. 配置与客户端

- [x] 新建 `provider/gemini` 七个职责文件；
- [x] 定义 ProviderID、Gemini Developer API 默认端点和 GenerateContent 默认协议；
- [x] Config 校验 API Key、Base URL、HTTP Client 和测试 Loopback；
- [x] 明确拒绝 Vertex AI backend/config；
- [x] 建立 `go-genai` 窄接口；
- [x] 现场确认 SDK 重试行为，不允许与 Runtime 叠加；
- [x] 确保 API Key/query credential 不进入 Raw 或错误。

#### D2. 能力契约

- [x] 声明文本、流、工具、并行工具、结构化输出、reasoning、函数错误结果、provider continuation 和 usage；
- [x] 对 reasoning summary 只按真实语义声明，不把 raw thought 当摘要；
- [x] 明确 ServerState、Interactions、Live、Batch、Vertex、多候选和多模态的本阶段支持级别；
- [x] 不把 ResponseID 声明为 server continuation；
- [x] 不维护静态模型白名单。

#### D3. 请求映射

- [x] system/developer 指令映射到 SystemInstruction 并记录转换；
- [x] user/assistant 映射到 user/model Content；
- [x] 按顺序组合 Text、FunctionCall 和 FunctionResponse Part；
- [x] FunctionCall 允许无 ID，Name/Args 必须有效；
- [x] FunctionResponse 使用 Name，存在 ID 时同时保留；
- [x] IsError 映射为明确 error 对象；
- [x] 映射函数声明、JSON Schema 和 tool choice；
- [x] 映射并行工具语义，无法精确控制时拒绝或显式降级；
- [x] 映射最大输出 Token；
- [x] 映射 JSON Object MIME Type 与 JSON Schema；
- [x] 校验 Gemini 支持的 Schema 子集；
- [x] 映射 thinking budget/level；
- [x] 解码并原样回传 thought signature continuation；
- [x] 拒绝 candidate count 大于 1；
- [x] 序列化最终 RawRequest，排除认证信息。

#### D4. 非流式响应与错误

- [x] 要求存在且只归一化一个候选；
- [x] 归一化文本和 FunctionCall；
- [x] 保存 thought text 的真实能力边界和 thought signature continuation；
- [x] 映射 FinishReason、PromptFeedback、策略拒绝和不完整状态；
- [x] 归一化 prompt/tool prompt/candidate/thought/cache/total usage；
- [x] 保留 ResponseID、ModelVersion、HTTP request ID、允许的 ProviderMetadata 和 RawResponse；
- [x] 分类认证、权限、无效请求、限流、暂时故障、策略拒绝和未知错误；
- [x] 解析 Retry-After 并判断 retryable；
- [x] SDK 错误不进入公开 unwrap 链；
- [x] 2xx 畸形或无候选载荷返回不可重试协议错误。

#### D5. 流式响应

- [x] 包装 GenerateContentStream 迭代器；
- [x] 为无原生 sequence 的块分配严格递增序号；
- [x] 按收到顺序映射文本、函数调用、reasoning/Native、usage 和完成；
- [x] 正确处理重复累计字段，禁止重复拼接完整快照；
- [x] 累计 thought signature 和 Provider continuation；
- [x] 持有可取消子 Context，保证 Close 能停止 SDK 迭代；
- [x] 构造唯一终态 Response；
- [x] 错误保留累计 Raw、NativeEvents、Request ID、MappingReport 和 continuation；
- [x] EOF、Close 幂等、取消和终态后停止读取均有断言；
- [x] 证明流式调用不自动重放。

### 阶段 E：测试资产

#### E1. 统一与结构白盒

- [x] 建立 `internal/testkit/providercontract`，不导入厂商 SDK；
- [x] OpenAI、Anthropic、Gemini 各自运行同一套 Provider contract；
- [x] Core 覆盖新增 Protocol、State、FunctionResult、Reasoning、StopReason、StreamEvent 和 Usage 校验；
- [x] 覆盖 State 跨 Provider/Protocol 拒绝；
- [x] 覆盖 continuation 默认脱敏、防御性复制和错误路径保持；
- [x] 通过确定性 fake/窄接口覆盖映射分支、流累加器和取消状态；
- [x] 禁止为测试向生产公共 API 暴露 SDK 或内部状态。

#### E2. Anthropic 黑盒与协议测试

- [x] 使用 `httptest` 验证 `/v1/messages` 路径、认证头、版本头和请求体；
- [x] 非流式文本、工具、错误工具结果、JSON Schema、reasoning、continuation 与 usage；
- [x] SSE 顺序、文本、工具 JSON、thinking、signature、usage、完成与 error；
- [x] 401/403/404/429/5xx、Retry-After、request ID；
- [x] 未知 block、畸形 JSON、畸形工具参数与异常 EOF；
- [x] Raw 中不存在 API Key 或认证头；
- [x] 固定 testdata 与 fuzz 种子。

#### E3. Gemini 黑盒与协议测试

- [x] 使用 `httptest` 验证 GenerateContent 路径、认证方式和请求体；
- [x] 非流式文本、工具、错误工具结果、JSON Object、JSON Schema、thinking、continuation 与 usage；
- [x] 流块顺序、文本、工具、thought signature、usage、完成与 error；
- [x] PromptFeedback、各类 FinishReason、无候选和 malformed function call；
- [x] 400/401/403/404/429/5xx、Retry-After、request ID；
- [x] 多候选、Vertex 配置、Interactions 和不支持字段明确拒绝；
- [x] Raw 中不存在 API Key 或认证 query；
- [x] 固定 testdata 与 fuzz 种子。

#### E4. 真实烟测入口

- [x] Anthropic 烟测使用 `integration` build tag；
- [x] Anthropic 同时要求 `PRAXIS_ANTHROPIC_SMOKE=confirmed`、`ANTHROPIC_API_KEY`、`ANTHROPIC_SMOKE_MODEL`；
- [x] Gemini 烟测使用 `integration` build tag；
- [x] Gemini 同时要求 `PRAXIS_GEMINI_SMOKE=confirmed`、`GEMINI_API_KEY`、`GEMINI_SMOKE_MODEL`；
- [x] 默认测试不编译烟测文件、不读取环境 Key、不访问公网；
- [x] 本阶段只验证烟测入口可编译，不执行真实调用。

### 阶段 F：自动化验收

- [x] `gofmt -l .` 无输出；
- [x] `go mod tidy` 后依赖精确；
- [x] `go mod verify` 通过；
- [x] 清除三家 Key 后 `go vet ./...` 通过；
- [x] 清除三家 Key、`GOPROXY=off` 的普通测试通过；
- [x] shuffle `-count=20` 通过；
- [x] race 测试通过；
- [x] 全模块 coverage 生成并记录；
- [x] integration build tag 以 `-run '^$'` 编译通过；
- [x] `git diff --check` 通过；
- [x] `git status --short` 只包含当前阶段和用户既有改动；
- [x] 记录未执行认证成功的真实模型调用，并如实记录一次 dummy-key 未认证 401 公网请求。

### 阶段 G：说明与同步

- [x] 更新 `ExecutionRuntime/model-invoker/README.md`；
- [x] 更新 `.properties.rax/module/model-invoker/README.md`；
- [x] 更新 `.properties.rax/properties/README.md`；
- [x] 在 `.properties.rax/memory/model-invoker/` 新增第二阶段实现与离线验收快照；
- [x] 记录 SDK 版本、实际命令、结果、覆盖率、失败修复和未执行项；
- [x] 完成后将本计划状态改为陈旧计划，保留全部清单与证据。

### 阶段 H：最终反证审计追加门禁

- [x] Gemini 在 tool → result → text 后保留后续轮次必需的 provider continuation；
- [x] Anthropic continuation 严格拒绝未支持 block、结构层 `cache_control` 与非 direct caller；
- [x] 三家响应体和流式读取具有统一 8 MiB 解压后硬上限，3xx 在读取 body 前拒绝；
- [x] 跨 Text/Reasoning/Arguments delta 分片的 API Key 无法由调用方重新拼接；
- [x] 新增反例后重新运行全部普通、shuffle×20、race、vet、coverage、integration compile 与 9 项 fuzz；
- [x] 最终只读复核无剩余已知 P1/P2 后转为陈旧计划。

## 6. 测试与验收方法

| 层级 | 方法 | 完成标准 |
|---|---|---|
| Core 单元 | 表驱动、fake Provider、确定性 sleeper | 新统一语义所有校验和能力推导均有正反例 |
| 结构白盒 | SDK 窄接口、确定性 fake、可观察累加/取消状态 | 映射、状态机、错误与资源释放分支可确定复现 |
| Provider 黑盒 | 外部测试包、官方 SDK、`httptest` | 只经公共 API 验证真实 HTTP/流协议和可观察结果 |
| 协议回归 | 固定 JSON/SSE/stream testdata、fuzz | 成功、错误、未知字段、畸形载荷和异常 EOF 均有断言 |
| 真实集成 | 显式 build tag 与三重环境门槛 | 本阶段延期，只编译入口，不计入通过 |

普通测试必须同时证明：无真实 Key、无公网、无 SDK 类型泄漏、无静默字段丢弃、无 SDK/Runtime 叠加重试、无流自动重放、Raw/continuation 默认脱敏。

## 7. 验证命令

在 `ExecutionRuntime/model-invoker/` 中执行：

```bash
gofmt -l .
go mod tidy
go mod verify
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY GOTOOLCHAIN=local GOPROXY=off go vet ./...
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY GOTOOLCHAIN=local GOPROXY=off go test -count=1 ./...
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY GOTOOLCHAIN=local GOPROXY=off go test -shuffle=on -count=20 ./...
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY GOTOOLCHAIN=local GOPROXY=off go test -race -count=1 ./...
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY GOTOOLCHAIN=local GOPROXY=off go test -covermode=atomic -coverpkg=./... -coverprofile=/tmp/model-invoker-phase-2.cover ./...
go tool cover -func=/tmp/model-invoker-phase-2.cover
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY GOTOOLCHAIN=local GOPROXY=off go test -tags=integration -run '^$' ./tests/integration
```

仓库根目录执行：

```bash
git diff --check
git status --short
```

不得运行 Anthropic 或 Gemini 真实烟测测试名，除非用户之后明确批准真实账号、模型、预算和调用次数。

## 8. 验收标准

只有同时满足以下条件，第二阶段才可标记完成：

1. 两个 SDK 精确锁定为 `anthropic-sdk-go v1.56.0` 和 `go-genai v1.63.0`；
2. 根包公开 API 不含任何厂商 SDK 类型或厂商专有结构；
3. OpenAI 原有两协议离线行为全部回归通过；
4. Anthropic Messages 文本、工具、结构化输出、reasoning、continuation、流、usage 和错误链路通过；
5. Gemini GenerateContent 同等核心链路通过，且不伪装 Interactions/Vertex/多候选支持；
6. State、StopReason、reasoning stream delta 和 Usage 语义在三家 Provider 中口径一致；
7. Capability、MappingReport 和映射错误证明不存在静默删除或虚假能力；
8. 普通、shuffle、race、vet、coverage、module verify、integration compile 和 diff check 均有实际结果；
9. Raw、continuation、错误和 SDK unwrap 链没有认证信息或 SDK 类型泄漏；
10. 真实烟测明确延期且没有被误报为通过；
11. README、module、properties 与 memory 已同步为第二阶段事实。

## 9. 风险与回退

| 风险 | 控制 | 回退 |
|---|---|---|
| State 原子迁移破坏 OpenAI | 先更新 Core 契约，再迁移 OpenAI 并跑全量回归 | 仅回退统一语义与 OpenAI 对应迁移，不保留双 State |
| SDK 类型或行为漂移 | 精确锁版本、窄接口、固定协议样本 | 恢复锁定版本并回退对应 Provider 包 |
| Usage 口径重复计数 | 固定跨 Provider 不变量和契约测试 | 保留 Raw 权威数据，回退新 Provider usage 映射 |
| signed/thought continuation 丢失 | 受控 Payload、原样往返与工具多轮测试 | 禁用对应 reasoning/tool continuation 能力，不做静默降级 |
| SDK 与 Runtime 叠加重试 | 关闭或验证 SDK 重试，错误路径计数 | Runtime MaxAttempts 设为 1，并禁用对应 Provider 重试能力 |
| Gemini 流无法关闭 | Provider 子 Context、Close/取消/race 测试 | 禁用 Gemini Streaming，保留非流式实现 |
| Provider 方言被错误共享 | adaptercore 禁止 SDK/映射/厂商 switch | 将错误抽取移回具体 Provider，不影响公共契约 |
| Mock 与真实 API 偏差 | 明确离线结论、保留延期烟测 | 真实联调失败时只修正对应 Provider 映射与能力声明 |

回退必须只作用于本阶段相关文件，不使用 `git reset --hard`、`git checkout --`，不覆盖用户或其他任务的未提交改动。Anthropic/Gemini Provider 在未注册时不影响现有 OpenAI 路径；出现单 Provider 阻塞时，可撤回该 Provider 的注册与能力暴露，保留已通过的统一语义和另一 Provider。
