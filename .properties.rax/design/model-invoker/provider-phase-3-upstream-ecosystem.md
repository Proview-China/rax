# 第三阶段：完整上游生态设计

## 1. 设计状态

- 模块：`model-invoker`
- 设计版本：`v1`
- 设计日期：2026-07-10
- 当前状态：当前授权范围已完成；A-E1离线验收完成，F未触发，G/H明确延期，总计划已转为陈旧计划
- 阶段变化：2026-07-10 由“计划待审核”进入“离线实施”
- 现有事实：OpenAI、Anthropic、Gemini Developer API 已完成离线实现；所有真实 API 烟测仍延期

本设计把下一阶段从“继续增加若干厂商 Adapter”扩展为“建立能长期覆盖直连 API、官方订阅、云托管和第三方托管的完整上游体系”。用户已授权按配套计划逐波次完成离线实现；真实 API、真实套餐和付费烟测仍需全部构建完成后单独授权。

## 2. 最终目标

Praxis 最终应覆盖有实际用户存量的主流模型上游，并能忠实处理：

1. 厂商原生 API 与原生 SDK；
2. OpenAI Responses 与 Chat Completions 兼容路线；
3. Anthropic Messages 兼容路线；
4. Gemini、Bedrock 等其他正式协议；
5. 官方 Token Plan、Coding Plan、会员权益和企业订阅；
6. AWS Bedrock、Google Vertex AI、Azure AI 等云托管服务；
7. 其他有稳定官方 API 的第三方托管与自托管推理后端；
8. 官方只提供非 Go SDK时的隔离语言执行器；
9. 每条路线独立的认证、区域、配额、条款、能力、错误和真实烟测证据。

“覆盖”不等于把所有服务压成一个请求结构，也不等于只替换 Base URL。Praxis 的统一层提供语义并集，Provider 路由保留完整方言。

## 3. 上游路由模型

```text
UpstreamRoute
├── Model Family
├── Provider
├── Offering
├── Deployment
├── Protocol
├── Endpoint
└── Credential Profile
```

| 维度 | 回答的问题 | 例子 |
|---|---|---|
| Model Family | 使用哪个模型家族 | Claude、GPT、Gemini、GLM、Llama |
| Provider | 谁接收请求并承担服务责任 | Anthropic、AWS、Google Cloud、Z.AI |
| Offering | 使用哪个商业或订阅产品 | 按量 API、Token Plan、Coding Plan、Provisioned Throughput |
| Deployment | 请求落在哪个账号、项目、地域或后端 | AWS Region、GCP Project、Azure Deployment、Workspace |
| Protocol | 线上交换格式是什么 | Responses、Chat Completions、Messages、GenerateContent、Converse |
| Endpoint | 精确入口是什么 | Host、Path、API Version |
| Credential Profile | 如何鉴权及其权限范围 | API Key、OAuth、ADC、SigV4、Entra ID |

### 3.1 不变量

- 模型原厂与托管平台分离；Anthropic Claude、Bedrock Claude 和 Vertex Claude 是三条不同路由；
- 按量 API 与订阅计划分离；Key、Endpoint、配额和允许用途不得混用；
- 协议与 Provider 分离；OpenAI SDK可以调用兼容端点，但不会把该端点变成 OpenAI Provider；
- Endpoint 与 Deployment 分离；相同 Host 下不同 Project、Workspace 或 Deployment 仍是不同运行身份；
- 能力合同绑定完整路由，不能从同模型的另一条路由继承；
- 真实烟测绑定精确路由、模型 ID、区域、账号和核验时间，不能跨路由复用结论。

## 4. 上游类型

### 4.1 厂商直连 API

模型厂商同时运营 API、鉴权、计费和模型服务。当前三家已落地路线属于此类；其余重点厂商需要逐家设计。

### 4.2 官方订阅计划

厂商通过固定周期费用提供 Token/Coding/会员额度。订阅路线不是按量 API 的价格标签，而是独立 Offering：

- 使用独立 Key、Base URL、模型别名和额度窗口；
- 可能限制个人使用、客户端类型、并发、生产用途和账号共享；
- 可能只允许官方列出的编程工具；
- 可能要求保留真实 User-Agent 或客户端标识；
- 到期、额度耗尽或风控后不能无提示切到按量余额；
- 只有官方允许自定义客户端或 SDK调用时，Praxis 才能执行直连。

首批调查结论：

- GLM Coding Plan只允许官方支持工具，Praxis自定义 Runtime直连为 `条款阻塞`；
- Kimi Code只允许个人、交互式 Coding/Agent客户端，必须保留真实客户端身份，产品后端应走开放平台；
- MiniMax Token Plan允许自定义 OpenAI/Anthropic Agent/Coding客户端，但生产 SaaS/backend权利仍需官方确认；
- MiMo Token Plan禁止自动脚本、自定义应用后端和非 Coding API用途，Praxis model-invoker直连为 `条款阻塞`；
- Alibaba Coding Plan与 Token Plan Team同样限制在交互式编程工具/Agent，禁止通用应用后端；Savings Plan只是按量计费优惠，不是新 Provider或协议；
- xAI消费者订阅目前只确认 Grok Build登录、headless与 ACP，没有公开订阅 API Key/Base URL映射，不能建成 HTTP Provider。

每个 Offering必须声明 `allowed_usage`：

- `general_api`：允许应用后端和通用 API；
- `interactive_coding_only`：只允许个人交互式 Coding/Agent客户端；
- `official_client_only`：只允许厂商列出的工具，Praxis不得直连。

### 4.3 云托管 Provider

AWS Bedrock、Google Vertex AI 与 Azure AI 是独立 Provider。它们可能托管来自 Anthropic、Meta、OpenAI、Google 或其他厂商的模型，但认证、地域、配额、数据处理、模型 ID和协议能力由云平台决定。

每个云托管 Provider 至少拆分：

- Serverless/按量与 Provisioned/Dedicated Offering；
- 原生云协议与 OpenAI/Anthropic 兼容协议；
- 区域、跨区域推理与私网 Endpoint；
- API Key、IAM/ADC/Entra ID等鉴权路线；
- 云平台模型目录与实际模型厂商；
- 云端 Guardrail、日志、缓存、状态和托管工具。

当前 Anthropic Go SDK已经提供 Bedrock Mantle与 Vertex认证入口，因此 Claude在这两条云路线不需要仅为 SDK引入 Sidecar；Bedrock Runtime的 Converse/Invoke仍由 AWS Go SDK拥有。Anthropic Message Batches与 AWS/Google各自的云 Batch必须作为不同能力域，不能共用一个 Batch声明。

### 4.4 第三方托管与自托管

第三方推理服务和自托管后端必须各自作为 Provider/Deployment，不以模型品牌代替。候选发现队列包括 Groq、Cerebras、Together、Fireworks、OpenRouter、Cloudflare Workers AI、NVIDIA NIM、Hugging Face Inference 与常见 vLLM 部署；每家只有完成官方来源调查后才能进入设计和计划。

该队列只表示覆盖方向，不代表已核验、已设计或已实现。

## 5. 协议覆盖策略

### 5.1 通用协议驱动

长期维护以下可复用协议驱动：

- OpenAI Responses；
- OpenAI Chat Completions；
- Anthropic Messages；
- Gemini GenerateContent；
- Amazon Bedrock Converse/InvokeModel；
- 经专项设计批准的其他原生协议。

协议驱动位于 `internal/protocol/`，负责统一语义与 wire DTO之间的映射、响应归一化、流状态机、协议 continuation、Raw构造和安全错误提取。Provider 方言负责模型、Endpoint、鉴权、能力合同、扩展策略、限流、厂商错误分类和条款；公共 Provider API与根包不暴露内部驱动。

### 5.2 多协议原则

- 官方同时提供 Responses、Chat 或 Messages 时，分别记录能力，不用“兼容”一词合并；
- Chat Completions 作为最广泛的无状态兼容路径长期保留；
- Messages 作为 Claude Code生态和多家订阅计划的正式兼容路径长期保留；
- Responses 用于官方确实实现状态、工具和事件语义的路线；
- 厂商原生协议能提供兼容协议没有的能力时，必须保留原生路线；
- 同一 Provider 的多协议通过统一语义契约对照，但允许能力差异。

## 6. SDK 与语言执行策略

### 6.1 选择规则

1. 路由官方 Go SDK完整时，优先原生 Go；
2. 官方公开完整 HTTP/SSE/WebSocket/gRPC合同时，可在 Go 内忠实实现；
3. 官方仅在 TypeScript 或 Python SDK中提供关键能力时，通过隔离 Sidecar 执行；
4. 官方明确推荐 OpenAI/Anthropic兼容 SDK时，复用协议方 Go SDK并建立独立 Provider 方言；
5. 官方同时给出多种协议时，按能力价值覆盖多条路径；
6. 社区 SDK默认不能进入正式实现。

### 6.2 语言边界

- Go：统一语义、路由、能力、策略、审计、生命周期与安全所有者；
- TypeScript：首选非 Go Sidecar，承载官方 TS SDK或生态集成；
- Python：仅为确有官方 Python SDK独占能力的获批路线启用；
- Rust：仍只用于经过测量的计算热点，不用于 Provider 网络适配。

所有 Sidecar 使用同一版本化 IPC、同一取消/超时/资源上限和同一 Provider 契约测试。SDK对象、异常与密钥不得穿透到 Go Runtime。

## 7. 路由状态

### 7.1 证据状态

| 状态 | 含义 |
|---|---|
| `fresh` | 官方证据在有效期内且无冲突 |
| `stale` | 证据超期，禁止新实现或发布 |
| `invalidated` | 旧结论已被新官方资料或实测推翻 |
| `unverified` | 缺少足以实施的官方合同 |
| `terms_blocked` | 技术上可访问，但官方条款不允许 Praxis 当前场景 |
| `deprecated` | 官方宣布弃用或下线 |

### 7.2 落地状态

| 状态 | 含义 |
|---|---|
| `research_only` | 只有调查记录 |
| `designed` | 路由详细设计已审核 |
| `planned` | 实施计划已审核并获授权 |
| `implemented_offline` | 代码和离线契约通过 |
| `live_verified` | 指定真实路由烟测通过 |
| `production_approved` | 容量、地域、条款、安全和运维门槛通过 |

证据状态与落地状态必须正交。例如 `fresh + research_only` 表示资料可信但尚未实现；`terms_blocked + research_only` 表示只能建档，不能调用。

## 8. 目录与机器可读目录

第三阶段需要设计一个机器可读 Upstream Catalog，至少包含：

- `provider_id`、`offering_id`、`deployment_kind`、`protocol_id`；
- 官方来源 ID、核验时间、有效期和证据状态；
- SDK语言、包、所有权、版本和许可证；
- Endpoint模板、区域、API Version和鉴权类型；
- 允许用途、客户端限制、个人/团队/生产边界；
- 模型发现方式和稳定别名策略；
- 能力、忽略字段、扩展字段、流事件和错误方言；
- Praxis落地状态、代码位置、测试证据和真实烟测记录。

Markdown矩阵由目录生成或与目录交叉校验。过期、缺少来源、状态冲突、同 ID重复和已实现但无测试证据必须使验证失败。

## 9. 安全与条款

- 凭据只通过 Credential Profile引用，不写入 Catalog、日志、Raw或计划；
- 不伪造客户端身份、不冒充官方工具、不绕过订阅限制；
- 不在未授权情况下自动从订阅切到按量余额或另一账号；
- 云 IAM、Project、Workspace、Region和私网配置必须进入审计结果；
- 按路由记录数据保留、训练使用、请求日志和地域边界；
- 真实烟测必须限定账号、模型、预算、调用次数和可接受数据；
- 只有官方资料不能证明生产可用，生产结论必须另做容量与运维验收。

## 10. 测试原则

每条路线必须独立通过：

1. Route解析、Offering隔离、Credential选择和禁止跨计划回退；
2. 请求序列化、响应解析和流事件顺序；
3. 工具、推理、结构化输出、状态和用量；
4. 配额耗尽、订阅到期、模型不可用、地域错误和条款阻塞；
5. 超时、取消、重试所有权、流关闭和资源上限；
6. Raw、错误、Header和跨事件密钥脱敏；
7. 同一语义跨协议、跨托管方的对照测试；
8. SDK升级、Endpoint变更和官方资料失效回归；
9. 明确授权的最小真实烟测。

同一模型在一条路线通过，不能代替其他订阅、云或协议路线。

## 11. 第三阶段边界

第三阶段计划应包含路由模型、Catalog、矩阵校验器、协议复用层、语言执行器，以及首批直连/订阅/云托管路线的实现波次。每个 Provider/Offering在编码前仍要有独立设计卡和通过审核的实现切片。

本设计不授权：

- 立即实现所有候选 Provider；
- 使用用户现有订阅或真实 Key；
- 付费调用或真实烟测；
- 绕过官方工具或条款限制；
- 把第三方托管模型写成模型原厂直连；
- 把待调查队列写成当前支持。
