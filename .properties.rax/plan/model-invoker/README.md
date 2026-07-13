# 模型调用器计划索引

> 当前完成计划：[第三方中转站兼容 Route 实施计划 v1](./third-party-relay-compat-v1.md)。独立Relay Provider、显式Factory、四协议离线合同与7/8真实文本/Tool Call已完成；Gemini原生Route保留中转上游429容量复测项。

> 最近完成计划：[执行语义并集第二轮 Review 与测试加固计划 v2](./execution-semantic-union-review-hardening-v2.md)。P0/P1 合同缺口已修复；高价值单元、并发、故障注入与五路真实 Adapter + fake process 集成已通过最终离线门禁，真实凭据和官方二进制继续为`not_run`。

> 前序完成计划：[执行语义并集 Runtime 实施计划 v1](./execution-semantic-union-runtime-v1.md)。`union/profile/effect/execution`、Direct与五条Harness Adapter、白盒、黑盒、N01-N14和六路本地收敛已完成离线验收；计划保留为“陈旧计划（已完成）”，真实 API、OAuth 和订阅联调继续标记为`not_run`。

> 设计收口计划：[全量上游支持与 Profile 并集计划 v1](./upstream-support-and-profile-union-v1.md)。该计划形成时物理模块和 Runtime 实现尚未获授权；现已由上面的独立实施计划承接，不回写或删除这份历史边界。
>
> 历史完成计划：[Factory双层信任闭合](./factory-trust-matrix-v1.md)。18 Factory/14默认活跃Adapter/4订阅Factory的A/B层protocol/profile合同、AST证据、Endpoint/Model/Lifecycle gap与全量离线回归均已完成；[宿主激活计划](./host-activation-and-upstream-revalidation.md)、[信任闭合计划](./route-gateway-trust-closure.md)、[上游调用最终候选计划](./route-gateway-final-candidate.md)与[Route Policy/Audit阶段计划](./route-invocation-facade-v1.md)继续作为历史计划保留。

## 1. 历史第一阶段计划状态

- 模块：`model-invoker`
- 计划版本：`v1`
- 创建时间：2026-07-10
- 当前状态：陈旧计划（第一阶段离线验收已完成）
- 实现位置：`ExecutionRuntime/model-invoker/`
- 授权依据：用户已明确要求把现有设计转换为可实施计划并立即编写代码。

本计划只执行已确认的第一阶段切片，不代表整个多 Provider 设计已经全部落地。实现完成并通过离线验收后，本计划保留并标记为“陈旧计划”，后续 Provider 使用新的阶段计划继续推进。

## 2. 本阶段确定产物

完成本计划后，仓库会得到以下可预见结果：

1. 一个独立、可测试的 Go module：`github.com/Proview-China/rax/ExecutionRuntime/model-invoker`；
2. 不暴露任何厂商 SDK 类型的 Praxis 统一语义 API；
3. Provider 注册、能力契约、确定性映射、统一错误、重试所有权、流事件和审计数据等内核能力；
4. 第一个真实 Provider：OpenAI，使用官方 `openai-go/v3`，同时支持 Responses 与 Chat Completions 两种原生协议；
5. 使用可注入客户端、手写 fake 和 `httptest` 的无密钥离线测试；
6. 面向使用者的模块说明、项目索引和 `.properties.rax/memory` 阶段快照；
7. 真实 API Key 烟雾测试入口和边界说明，但本轮不创建、不读取、不复用真实 Key，也不执行真实联网调用。

## 3. 范围与不做事项

### 3.1 本阶段范围

- Provider、Protocol、Endpoint 和 Model 分离建模；
- 第一版统一语义字段：
  - `Input`：文本消息、函数调用和函数结果；
  - `Instruction`：system/developer 约束；
  - `Tool`：JSON Schema 函数定义、选择策略和并行调用开关；
  - `OutputConstraint`：文本、JSON Object、JSON Schema；
  - `Reasoning`：推理强度和摘要请求；
  - `State`：无状态和 OpenAI `previous_response_id`；
  - `Stream`：统一的文本、工具参数、完成和错误事件；
  - `Budget`：最大输出 Token 和超时；
  - `Metadata`：追踪与业务标签；
  - `ProviderOptions`：按 Provider 命名空间隔离的扩展字段；
- `Native`、`Compatible`、`Partial`、`Unsupported` 四级能力契约和限制说明；
- 精确映射、显式转换、经授权降级和拒绝四种确定性决策；
- OpenAI Responses 非流式、SSE 流式、函数调用和结构化输出映射；
- OpenAI Chat Completions 非流式、`delta` 流式、函数调用和结构化输出映射；
- 请求 ID、用量、原始载荷、Provider 元数据和映射报告保留；
- 认证、权限、无效请求、不支持、限流、超时/取消、暂时故障、策略拒绝、流中断、映射失败和未知错误分类；
- Runtime 单点控制非流式重试；OpenAI SDK 内建重试固定关闭，流式调用不自动重放。

### 3.2 本阶段不做

- Anthropic、Gemini、xAI、DeepSeek、Kimi、MiniMax、Qwen、Meta、GLM、MiMo 的实现或空壳；
- TypeScript Sidecar；因此本阶段不选择 gRPC、Connect 或 JSON-RPC；
- 图片、音频、视频、文件等多模态输入；能力契约必须明确拒绝，而不是静默忽略；
- Hosted Tools、Realtime、Batch、后台执行、缓存控制的完整统一；
- 图像/视频/音乐生成、完整实时语音、微调、评测、部署、账号管理；
- Agent Run Engine、上下文处理、记忆、多 Agent 编排、MCP、资产或自定义工具执行器；
- Rust；
- 真实 API 联调、性能容量结论或生产可用声明。

## 4. 技术决定

### 4.1 Go module 与包边界

- module 根：`ExecutionRuntime/model-invoker/`；
- module path：`github.com/Proview-China/rax/ExecutionRuntime/model-invoker`；
- 根包名：`modelinvoker`；
- OpenAI 适配器位于 `provider/openai`；厂商 SDK 类型只能出现在该目录内部；
- 公共接口只使用 Go 标准库和 Praxis 自有类型。

### 4.2 OpenAI SDK 与协议

- 锁定官方 `github.com/openai/openai-go/v3 v3.41.1`；该版本已在 2026-07-10 通过 Go module 实时查询核验；
- Responses 是默认协议；调用方可显式选择 Chat Completions；
- Responses 流按有类型的 SSE 事件解析；Chat Completions 流按 chunk `delta` 解析；
- Responses 函数调用和结果通过 `call_id` 关联；Chat Completions 保持 assistant tool call 与 tool message 的关联；
- SDK 自动重试通过 `option.WithMaxRetries(0)` 关闭，避免和 Runtime 重试叠加；
- SDK 升级必须显式修改版本，运行全部离线回归和序列化契约测试，再更新 Provider 矩阵与 memory；不自动漂移版本。

### 4.3 能力与降级

- `Native`、`Compatible`：允许执行并记录映射方式；
- `Partial`：只有请求显式设置 `AllowDegradation` 才可执行，并在 `MappingReport` 记录限制和降级；
- `Unsupported`：始终拒绝；
- 未识别字段、错误命名空间或不适用于所选协议的字段必须返回映射错误；
- 禁止静默删除任何调用者表达的语义。

### 4.4 原始数据与脱敏

- 原始请求、响应和流事件保存在受控载荷类型中；
- 普通字符串格式化和 JSON 序列化默认只显示 `[REDACTED]`；
- 需要审计的调用者必须显式取得副本；
- Authorization、API Key 等认证信息不进入 RawRequest、错误文本或日志。

### 4.5 API Key 与集成边界

- 本轮只允许假的测试凭据和本机 `httptest` 地址；
- 普通测试清除 `OPENAI_API_KEY` 后仍必须全部通过；
- 真实 API 烟雾测试只有用户之后提供测试边界并明确要求执行时才启用；
- Mock/`httptest` 通过不能冒充真实 Provider 联调通过。

### 4.6 测试目录约束

- 测试代码不得与生产源码混放；
- 统一语义内核测试位于 `ExecutionRuntime/model-invoker/tests/core/`；
- OpenAI Provider 测试位于 `ExecutionRuntime/model-invoker/tests/openai/`；
- 固定协议样本放在对应测试目录的 `testdata/`；
- `ExecutionRuntime/model-invoker/` 根包和 `provider/openai/` 生产目录不得保留 `*_test.go`；
- 独立测试包只能通过公共 API、可注入的 Provider 契约和本机 `httptest` 观察行为，不为测试向生产 API 暴露厂商 SDK 或内部实现。

## 5. 计划清单

### 阶段 A：控制资产与公共契约

- [x] 读取并核对设计、Provider 矩阵和构建指南；
- [x] 核验 Go 版本、OpenAI SDK 最新稳定版本和官方协议文档；
- [x] 固定第一阶段范围、第一 Provider、无密钥边界和 Sidecar 延期决定；
- [x] 建立 Go module；
- [x] 定义统一请求、响应、输入项、输出项、用量、状态和 Provider 元数据；
- [x] 定义流接口与统一事件；
- [x] 定义统一错误和错误分类；
- [x] 定义受控 Raw 载荷。

### 阶段 B：能力、映射与路由内核

- [x] 定义能力枚举、支持级别、限制和契约；
- [x] 从请求确定性推导能力需求；
- [x] 实现精确、转换、降级、拒绝决策与 MappingReport；
- [x] 实现并发安全的 Provider Registry；
- [x] 实现 Invoker 的校验、路由、超时和非流式重试；
- [x] 保证流式路径不自动重放并可取消、可关闭。

### 阶段 C：OpenAI Provider

- [x] 建立 SDK 窄接口和可注入客户端；
- [x] 实现配置校验、官方 SDK 客户端和禁用 SDK 重试；
- [x] 实现 Responses 请求映射和响应归一化；
- [x] 实现 Responses SSE 事件归一化；
- [x] 实现 Chat Completions 请求映射和响应归一化；
- [x] 实现 Chat Completions chunk 事件归一化；
- [x] 实现工具调用、结构化输出、推理配置、状态、用量和请求 ID；
- [x] 实现 OpenAI 错误到统一错误的分类；
- [x] 保留经过脱敏保护的原始请求、响应和事件。

### 阶段 D：自动化测试

- [x] 请求校验、能力判断、映射报告、Registry、错误、Raw 载荷的表驱动单元测试；
- [x] Invoker 正常、拒绝、取消、超时、重试及不重试测试；
- [x] OpenAI Responses/Chat 请求 Schema 与序列化测试；
- [x] 两种协议的非流式响应、文本、工具调用、用量与请求 ID 测试；
- [x] 两种协议的流顺序、文本增量、工具参数增量、完成、错误、EOF 和关闭测试；
- [x] `httptest` 黑盒测试，验证路径、认证头、请求体和真实 SDK 解码；
- [x] 公共 API 外部包黑盒测试；
- [x] Parser/校验器 fuzz 种子和关键 benchmark；
- [x] 确认普通测试没有读取真实 Key 或访问公网。

### 阶段 E：验收与同步

- [x] `gofmt` 无差异；
- [x] `go mod tidy`、`go mod verify`；
- [x] `go vet ./...`；
- [x] 普通、shuffle、race 和覆盖率测试；
- [x] `git diff --check`；
- [x] 记录实际命令、结果、失败原因和未覆盖项；
- [x] 完成 `module/model-invoker` 说明与项目总索引；
- [x] 写入实现与验收 memory 快照；
- [x] 将本计划状态标记为“陈旧计划”。

## 6. 测试与验收标准

| 类型 | 本阶段做法 | 完成标准 |
|---|---|---|
| 单元测试 | 标准库 `testing`、表驱动、手写 fake | 重要分支、错误路径和边界均有断言 |
| 结构白盒验证 | 外部测试包通过可注入 Provider、确定性 sleeper 和可观察状态覆盖内部契约 | 不依赖睡眠和公网，结果确定 |
| 黑盒测试 | 外部测试包和 `httptest` | 只通过公共 API 使用模块，校验可观察行为 |
| 本地集成测试 | 官方 OpenAI SDK + `httptest` 服务 | 两种协议的 HTTP、SSE、工具和错误链路可运行 |
| 真实集成测试 | 延期 | 用户提供真实 API 与边界后单独执行，不计入本轮通过 |

仓库当前没有既定覆盖率门禁，因此本轮生成并记录覆盖率报告，但不擅自新增百分比门禁。测试必须同时证明：无真实 Key、无公网、无静默降级、无 SDK 类型泄漏、无叠加重试。

## 7. 验证命令

在 `ExecutionRuntime/model-invoker/` 中执行：

```bash
gofmt -w .
go mod tidy
go mod verify
env -u OPENAI_API_KEY GOTOOLCHAIN=local GOPROXY=off go vet ./...
env -u OPENAI_API_KEY GOTOOLCHAIN=local GOPROXY=off go test -count=1 ./...
env -u OPENAI_API_KEY GOTOOLCHAIN=local GOPROXY=off go test -shuffle=on -count=20 ./...
env -u OPENAI_API_KEY GOTOOLCHAIN=local GOPROXY=off go test -race -count=1 ./...
env -u OPENAI_API_KEY GOTOOLCHAIN=local GOPROXY=off go test -covermode=atomic -coverpkg=./... -coverprofile=/tmp/model-invoker.cover ./...
go tool cover -func=/tmp/model-invoker.cover
```

仓库根目录执行：

```bash
git diff --check
git status --short
```

## 8. 风险与回退

| 风险 | 控制 | 回退 |
|---|---|---|
| 统一接口过早固化 | 第一阶段只覆盖 Agent 必需能力，Provider 扩展隔离 | 保留设计与计划，删除未发布 Go module 即可回退 |
| SDK 生成类型变化 | 精确锁版本、窄接口、序列化回归 | 恢复旧版本并记录兼容差异 |
| SDK 与 Runtime 重试叠加 | SDK 重试固定为 0，Runtime 单点负责 | 将 Runtime 最大尝试数设为 1 |
| 流事件丢失或重排 | 不使用异步 channel 转发，按原顺序同步归一化 | 返回原生事件审计副本并中止流 |
| Raw 数据泄漏 | 默认字符串/JSON 脱敏，不保存认证头 | 禁止暴露原始副本的调用路径 |
| Mock 与真实 API 偏差 | 明确标记离线验收，保留真实烟测待办 | 真实联调失败时回到 Provider 映射层修正 |

## 9. 完成条件

只有同时满足以下条件，本阶段才可标记完成：

1. 计划范围内的公共 API、内核和 OpenAI Provider 均已落地；
2. Runtime 公共包没有 OpenAI SDK 类型；
3. Responses 和 Chat Completions 的非流式及流式离线链路均通过；
4. 能力拒绝和显式降级可审计，不存在静默字段丢弃；
5. SDK 自动重试已关闭，Runtime 重试行为有确定性测试；
6. 普通、shuffle、race、vet、coverage 报告和 Git diff 检查均已实际运行并记录；
7. 模块说明、总体索引、memory 和设计状态已经同步；
8. 真实 API 测试明确标记为延期，未被误报为通过。
