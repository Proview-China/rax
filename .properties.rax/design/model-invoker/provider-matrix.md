# Provider、Offering、协议与 SDK 可用性矩阵

## 1. 矩阵状态

- 矩阵版本：`v3`
- 全量核验时间：2026-07-11 03:55 CST；Qwen/百炼按量路线已刷新
- 证据范围：厂商、云平台和协议方的官方 API 文档、SDK 文档、官方仓库与公开条款
- 本机实现状态来源：`ExecutionRuntime/model-invoker/` 的真实目录、依赖和测试资产
- 真实 API 状态：所有路线均未完成认证成功的当前模型烟雾测试
- 当前性质：第三阶段设计与计划依据，不是已实现 Provider 清单

完整上游身份为：

```text
UpstreamRoute = Model Family
              + Provider
              + Offering
              + Deployment
              + Protocol
              + Endpoint
              + Credential Profile
```

本矩阵不把模型品牌、服务运营方、订阅产品、云部署或兼容协议混成一列。官方文档确认某路线存在，不等于 Praxis 已实现；离线测试通过也不等于公网、账号或生产可用。

## 2. 状态与所有权

### 2.1 证据状态

| 状态 | 含义 | 是否可进入实现 |
|---|---|---|
| `fresh` | 官方来源在有效期内且没有未解决冲突 | 可以继续专项设计 |
| `stale` | 证据超期 | 不可以，必须先刷新 |
| `invalidated` | 旧结论已被新官方资料或实测推翻 | 不可以，必须记录替代结论 |
| `unverified` | 官方合同不足、资料互相冲突或关键字段缺失 | 不可以 |
| `terms_blocked` | 技术路径存在，但官方条款不允许 Praxis 当前使用场景 | 不可以直连实现 |
| `deprecated` | 官方宣布弃用或下线 | 不可以新增实现 |

### 2.2 Praxis 落地状态

| 状态 | 含义 |
|---|---|
| `research_only` | 只有调研记录 |
| `designed` | 路由详细设计已审核 |
| `planned` | 计划已审核并获得实现授权 |
| `implemented_offline` | 代码与离线契约测试通过 |
| `live_verified` | 指定账号、区域、Endpoint和模型烟测通过 |
| `production_approved` | 容量、数据、条款、安全和运维门槛通过 |

### 2.3 SDK 所有权

| 类型 | 含义 |
|---|---|
| `provider_native` | 当前 Provider自己维护并支持的 SDK |
| `model_vendor` | 模型原厂 SDK用于调用云托管路线，但服务责任仍属于云 Provider |
| `protocol_upstream` | OpenAI、Anthropic等协议原厂 SDK调用兼容端点 |
| `cloud_native` | AWS、Google Cloud、Azure维护的云服务 SDK |
| `community` | 社区 SDK；默认不得进入正式实现 |

`protocol_upstream` 不能写成目标 Provider 的“官方 SDK”。它只证明协议客户端可复用，不证明完整字段、事件、模型或支持责任等价。

## 3. 厂商直连 API 路线

| Route family | Provider / Offering | 官方协议 | SDK与 Praxis策略 | Praxis状态 | 证据状态 |
|---|---|---|---|---|---|
| `openai.direct` | OpenAI / 按量 API | Responses、Chat Completions | `provider_native` Go `openai-go/v3`；当前两协议均已落地 | `implemented_offline` | `fresh` |
| `anthropic.direct` | Anthropic / 按量 API | Messages | `provider_native` Go `anthropic-sdk-go`；保留 thinking、签名和 content block | `implemented_offline` | `fresh` |
| `google.gemini-developer` | Google / Gemini Developer API | GenerateContent | `provider_native` Go `go-genai`；不与 Vertex 共用认证和能力 | `implemented_offline` | `fresh` |
| `xai.direct` | xAI / 按量 API | Responses、原生 gRPC；Chat兼容能力按当前文档维护 | 官方 Python `xai-sdk`与公开 proto；Go优先 Responses协议驱动，gRPC独占能力另设计 | `research_only` | `fresh` |
| `deepseek.direct` | DeepSeek / 按量 API | OpenAI Chat、Anthropic Messages | 两条均为 `protocol_upstream`；已保留 `thinking/reasoning_content`并拒绝静默模型映射 | `implemented_offline` | `fresh` |
| `kimi.platform` | Moonshot/Kimi开放平台 / 按量 API | OpenAI Chat；其他协议按产品单独核验 | Chat已离线实现并保留 thinking；Kimi Code会员仍是独立非 callable Offering | `implemented_offline` | `fresh` |
| `minimax.platform` | MiniMax开放平台 / 按量 API | Anthropic Messages、OpenAI Chat、Responses；媒体接口独立 | 三协议已离线实现；Messages为默认主路径，M3/M2.x thinking、累积 Chat流和无服务器 Responses状态均有专属方言 | `implemented_offline` | `fresh` |
| `alibaba.model-studio` | Alibaba Cloud Model Studio / 按量与云资源计划 | OpenAI Responses、Chat、DashScope原生 | 北京/新加坡 Workspace专属 Responses与 Chat已离线实现；DashScope原生能力单独实施 | `implemented_offline` | `fresh` |
| `meta.llama-api` | Meta Llama API / 直连服务 | 原生 `/v1`、OpenAI兼容 `/compat/v1` | Meta官方 Python/TypeScript；未发现官方 Go；公开 GA、价格与服务范围证据仍不足 | `research_only` | `unverified` |
| `zai.platform` | Z.AI开放平台 / 按量 API | OpenAI Chat为公共主路径；其他端点按产品核验 | Chat已离线实现，保留 thinking、request_id与专属终态；Coding Plan继续分离 | `implemented_offline` | `fresh` |
| `xiaomi.mimo` | Xiaomi MiMo / 按量 API | Anthropic Messages、OpenAI Chat；无公开 Responses合同 | 两协议已离线实现；V2.5 thinking、continuation、专属终态和 Token Plan隔离均有专属方言 | `implemented_offline` | `fresh` |

## 4. 官方订阅与 Token/Coding Plan

订阅计划必须与同厂商按量 API建立不同 Offering。Key、Base URL、模型别名、额度窗口、允许客户端、生产用途和余额回退不能互相推断。

| Route family | Offering边界 | 协议 / Endpoint | 使用限制与 Praxis判断 | 状态 |
|---|---|---|---|---|
| `zai.glm-coding-plan` | GLM Coding Plan；独立订阅额度 | 中国专属 Coding Endpoint `https://open.bigmodel.cn/api/coding/paas/v4`；其他区域须另建 Route | 2026-07-11官方文档明确允许指定 Coding工具；Catalog已按真实客户端白名单建档，但没有 Provider Adapter，保持非 callable | `fresh + planned` |
| `kimi.code-membership` | Kimi会员内的 Kimi Code权益；与开放平台按量 API分离 | OpenAI：`https://api.kimi.com/coding/v1`；Anthropic：`https://api.kimi.com/coding/`；模型 `kimi-for-coding` | 只允许个人、交互式 Coding/Agent客户端并要求保持真实 User-Agent；两协议 Route已建档，无 Adapter | `fresh + planned` |
| `minimax.token-plan` | Token Plan；`sk-cp-*` Key与按量 API不可互换 | Global：OpenAI `https://api.minimax.io/v1`、Anthropic `https://api.minimax.io/anthropic` | 官方允许自定义 OpenAI/Anthropic Agent/Coding工具；当前只按交互式个人场景建档，无 Adapter | `fresh + planned` |
| `mimo.token-plan` | Token Plan；`tp-*` Key，与按量 Key隔离 | 中国、新加坡、欧洲独立域名；分别提供 OpenAI兼容 `/v1` 与 Anthropic兼容 `/anthropic` | 官方允许编程工具并禁止自动脚本、自定义应用后端和非 Coding API用途；六条 Region×Protocol Route已建档，无 Adapter | `fresh + planned` |
| `alibaba.coding-plan` | Alibaba Model Studio Coding Plan；`sk-sp-*` Key | 中国 OpenAI `https://coding.dashscope.aliyuncs.com/v1`、Anthropic `.../apps/anthropic`；国际使用 `coding-intl` 域名 | 官方允许编程工具/OpenClaw类 Agent，同时禁止 API测试器、工作流、自动脚本和应用后端；四条 Route已建档，无 Adapter | `fresh + planned` |
| `alibaba.token-plan-team` | Alibaba Token Plan Team Edition；独立订阅与 Endpoint | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` 或 `/apps/anthropic` | 仅支持编程工具/OpenClaw类交互式 Agent；两条 Route已建档，无 Adapter | `fresh + planned` |
| `alibaba.savings-plan` | General-purpose AI Savings Plan | 继续使用按量 API Key与区域 Endpoint | 只是按量费用抵扣，不是新协议或新 Provider；`BillingPlanReference`已固定该边界，具体引用随未来按量 Route落地 | `fresh + designed` |
| `xai.consumer-subscription` | SuperGrok/X Premium+ 与 Grok Build权益 | 官方账号登录的 Grok Build、headless与 ACP；没有公开订阅 API Key/Base URL映射 | 只能作为外部 Agent/ACP backend候选，不能写成 model-invoker HTTP Provider | `unverified + research_only` |
| `anthropic.consumer-plans` | Claude Pro/Max消费者订阅 | Claude.ai产品，不提供 Anthropic API、Bedrock或 Vertex额度 | 只能作为产品账号权益建档，不能转换为 model-invoker Credential或消费计划 | `fresh + research_only` |

订阅计划的“支持”分为两层：Catalog能识别该 Offering；Invoker只有在官方允许 Praxis使用场景、完成专项设计并获授权后才执行。每条路线必须声明 `general_api`、`interactive_coding_only` 或 `official_client_only`；`terms_blocked` 路线不得为了覆盖率写入可调用注册表。

2026-07-11刷新说明：GLM、MiMo与 Alibaba的旧 `terms_blocked`结论已被当前官方 Coding/Token Plan文档推翻，替代结论是 `interactive_coding_only + planned + callable=false`。这只允许控制面准确建档和离线授权/拒绝测试，不等于 Praxis可以伪装工具、运行后端或已经拥有可调用 Adapter。

## 5. 云托管 Provider 路线

| Route family | Provider / Deployment | 协议与 Endpoint | SDK、鉴权与边界 | 状态 |
|---|---|---|---|---|
| `aws.bedrock-mantle` | AWS Bedrock Mantle；Region + Project/Workspace | `https://bedrock-mantle.{region}.api.aws`；OpenAI Responses、Chat Completions、Anthropic Messages | Bedrock API Key或 SigV4；OpenAI/Anthropic SDK属于协议/模型方，当前 Anthropic Go SDK有 Mantle客户端；能力、状态和数据保留按AWS实现 | `fresh + implemented_offline` |
| `aws.bedrock-runtime` | AWS Bedrock Runtime；Region + model/inference profile | Converse、InvokeModel；具体模型支持按AWS目录 | `cloud_native` AWS SDK for Go v2覆盖 Converse/Invoke；SigV4或Bedrock Key；与 Mantle分别维护能力和配额 | `fresh + implemented_offline` |
| `gcp.vertex-gemini` | Google Vertex AI；Project + Location + Publisher Model | Gemini API in Vertex AI / GenerateContent | `cloud_native`/`provider_native` Google Gen AI Go SDK；ADC或官方支持的 API Key；与 Gemini Developer API分离 | `fresh + implemented_offline` |
| `gcp.vertex-claude` | Google Vertex AI Partner Model；Project + Location + Anthropic Model | Vertex Publisher endpoint `rawPredict` / `streamRawPredict`；使用 Anthropic SDK官方 middleware | Anthropic Go SDK当前支持 Google Auth/credentials；服务运营与鉴权属于 Google Cloud；模型版本、区域和 Provisioned Throughput单独记录 | `fresh + implemented_offline` |
| `gcp.vertex-openai-chat` | Google Vertex AI OpenAI兼容 Endpoint；Project + Location | `.../projects/{project}/locations/{location}/endpoints/openapi`；当前只按 Chat Completions建模 | Google Cloud Auth；复用协议 SDK但不能推定 Responses或完整 OpenAI能力 | `fresh + implemented_offline` |
| `azure.openai-v1` | Azure OpenAI / AI Foundry；Resource + Deployment | `https://{resource}.openai.azure.com/openai/v1/`；Responses或Chat能力按 Deployment核验 | OpenAI官方 Go SDK配置 Azure v1端点；API Key或 Microsoft Entra ID；请求 `model` 是 Azure deployment name | `fresh + implemented_offline` |
| `azure.openai-legacy` | Azure OpenAI旧版本 Endpoint | 独立 dated API Version绑定 | 与 v1独立；禁止向 v1自动拼接 `api-version`，迁移前逐 Deployment核验 | `fresh + implemented_offline` |
| `azure.foundry-models` | Azure AI Foundry其他托管模型 | 协议、Endpoint与能力按具体模型和 Deployment | 不能因 Foundry提供 OpenAI形状入口就继承 Azure OpenAI能力；每个模型路线另做证据卡 | `unverified + research_only` |
| `anthropic.platform-on-aws` | Anthropic-operated、AWS Marketplace计费的 Claude Platform on AWS | 协议和资源边界需专项设计 | 不是 Amazon Bedrock；Provider operator、billing owner和 Credential不得与 Bedrock合并 | `unverified + research_only` |

### 5.1 AWS当前必须保留的差异

- `bedrock-mantle` 是新应用的开放协议入口；`bedrock-runtime` 保留云原生 Converse/Invoke；
- Mantle的 OpenAI Responses 默认 `store=true`，AWS当前文档说明响应在源 Region按 Project保存30天；`store=false` 时不能使用 `previous_response_id`；
- Mantle与 Runtime的 Messages能力不完全相同；例如结构化输出等字段必须按 Endpoint核验；
- OpenAI Go SDK已通过 Base URL + Bedrock API Key/SigV4完成本机 HTTP fake离线验收；它仍属于协议方 SDK复用，不冒充 AWS官方 Go Mantle SDK；
- 同一 Bedrock模型可能只支持部分 API家族，必须先读取 AWS模型兼容目录。

### 5.2 Claude云托管必须保留的差异

- Bedrock Mantle Messages、Bedrock Runtime Invoke、Bedrock Converse与 Vertex `rawPredict/streamRawPredict` 是不同 ProtocolBinding；
- Bedrock Invoke body使用 `bedrock-2023-05-31`；Mantle Header使用 `anthropic-version: 2023-06-01`；Vertex body使用 `vertex-2023-10-16`；
- Bedrock Invoke使用 AWS event-stream，Mantle与 Vertex Messages路线使用 SSE；Converse使用 AWS原生结构；
- Anthropic Message Batches不直接出现在 Bedrock/Vertex Messages客户端上；AWS与Google各自的云 Batch是独立能力域；
- Bedrock当前不支持 Anthropic server-side web search/fetch/code execution；Vertex部分模型的 Web Search受组织策略和 VPC-SC限制，必须按模型/Region/Policy声明；
- Cache、State、ProviderOptions与 continuation不能在 Anthropic Direct、Bedrock和 Vertex之间复用。

## 6. 第三方托管与自托管发现队列

| 候选 Provider | 当前处理 |
|---|---|
| Groq、Cerebras、Together、Fireworks、OpenRouter | 有市场存量，进入官方来源调查队列；尚未形成 Praxis路线设计 |
| Cloudflare Workers AI、NVIDIA NIM、Hugging Face Inference | 进入云/边缘/企业托管调查队列；需拆账号、部署和协议 |
| vLLM、TGI、Ollama等自托管后端 | 作为自托管 Deployment类别调查；不能与模型原厂或托管 SaaS共用身份 |

本节只是覆盖队列。没有官方来源、设计卡和测试的候选不能出现在“已支持”或第三阶段确定产物中。

## 7. 协议驱动与语言执行器

| Driver | 可服务路线 | 当前 Praxis事实 | 第三阶段要求 |
|---|---|---|---|
| OpenAI Responses | OpenAI直连，以及官方明确兼容的 xAI、Qwen、MiniMax、MiMo、Bedrock等路线 | 独立协议 driver已离线实现；OpenAI、云路线与 MiniMax各保留独立方言 | 每个新增 Provider仍保留独立方言、能力和错误 |
| OpenAI Chat Completions | OpenAI及大量兼容 Provider/计划/托管路线 | 独立协议 driver已离线实现；当前服务多个直连/云 Adapter，MiniMax累积流使用窄扩展缝隙 | 长期保留最广兼容路径，新增路线继续测试未知字段和流事件 |
| Anthropic Messages | Anthropic直连，以及 MiniMax、MiMo、Kimi Code、Bedrock等正式兼容路线 | 独立协议 driver已离线实现；MiniMax服务端 tool block会先规范化再进入严格 continuation | 保留 thinking、签名、tool block和被忽略字段差异；不能假设完整等价 |
| Gemini GenerateContent | Gemini Developer API、Vertex Gemini | 独立协议 driver已离线实现；Developer API与 Vertex使用不同 Adapter、认证和 Deployment | 后续路线继续禁止把 Developer API和 Vertex合并 |
| Bedrock Converse/Invoke | Bedrock托管模型 | 已由 `aws-bedrock-runtime`离线实现 | 使用 AWS SDK for Go v2；模型能力按AWS目录和 Region核验 |
| 厂商原生协议 | xAI gRPC、Meta native、DashScope及其他独占能力 | 未实现 | 只有兼容驱动无法忠实表达时才新增，需独立批准 |
| TypeScript Sidecar | 官方 TS SDK拥有关键独占语义的路线 | 未实现 | 固定 IPC、版本、资源、安全、取消和契约测试 |
| Python Sidecar | 官方 Python SDK拥有关键独占语义且无等价Go/TS/HTTP路线 | 未实现 | 逐路证据门禁，不能预装为全局依赖 |

OpenAI官方当前仍同时维护 Responses与 Chat Completions；两者在状态、函数调用、结构化输出和流事件上不同。Praxis不能为了复用而把 Responses降成 Chat，也不能把所有兼容端点声明为完整 Responses。

## 8. 当前实现事实

`catalog.DefaultDocument`当前包含62条记录：39条已实现 callable Binding和23条计划/研究控制记录。下表只渲染当前可调用的39条精确 Binding；控制记录不会进入 Runtime Adapter区块。

<!-- BEGIN GENERATED: praxis-model-invoker-current-bindings -->
| Route ID | Provider | Runtime Adapter ID | Offering | Deployment | Protocol | Endpoint | Credential Profile | Evidence | Praxis状态 |
|---|---|---|---|---|---|---|---|---|---|
| `alibaba.model-studio.ap-southeast-1.payg.chat_completions` | `alibaba.model-studio` | `qwen` | `alibaba.model-studio.payg` | `alibaba.model-studio.ap-southeast-1` | `chat_completions` | `https://{workspace}.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1` | `alibaba.model-studio.ap-southeast-1.chat_completions` | `fresh` | `implemented_offline` |
| `alibaba.model-studio.ap-southeast-1.payg.responses` | `alibaba.model-studio` | `qwen` | `alibaba.model-studio.payg` | `alibaba.model-studio.ap-southeast-1` | `responses` | `https://{workspace}.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1` | `alibaba.model-studio.ap-southeast-1.responses` | `fresh` | `implemented_offline` |
| `alibaba.model-studio.cn-beijing.payg.chat_completions` | `alibaba.model-studio` | `qwen` | `alibaba.model-studio.payg` | `alibaba.model-studio.cn-beijing` | `chat_completions` | `https://{workspace}.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `alibaba.model-studio.cn-beijing.chat_completions` | `fresh` | `implemented_offline` |
| `alibaba.model-studio.cn-beijing.payg.responses` | `alibaba.model-studio` | `qwen` | `alibaba.model-studio.payg` | `alibaba.model-studio.cn-beijing` | `responses` | `https://{workspace}.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `alibaba.model-studio.cn-beijing.responses` | `fresh` | `implemented_offline` |
| `anthropic.direct.payg.messages` | `anthropic` | `anthropic` | `anthropic.api.payg` | `anthropic.direct.global` | `messages` | `https://api.anthropic.com/v1` | `anthropic.default` | `fresh` | `implemented_offline` |
| `aws.bedrock-mantle.us-east-1.api-key.chat_completions` | `aws.bedrock-mantle` | `aws-bedrock-mantle` | `aws.bedrock-mantle.payg` | `aws.bedrock-mantle.us-east-1` | `chat_completions` | `https://bedrock-mantle.{region}.api.aws/openai/v1` | `aws.bedrock-mantle.api-key` | `fresh` | `implemented_offline` |
| `aws.bedrock-mantle.us-east-1.api-key.messages` | `aws.bedrock-mantle` | `aws-bedrock-mantle` | `aws.bedrock-mantle.payg` | `aws.bedrock-mantle.us-east-1` | `messages` | `https://bedrock-mantle.{region}.api.aws/anthropic/v1` | `aws.bedrock-mantle.api-key` | `fresh` | `implemented_offline` |
| `aws.bedrock-mantle.us-east-1.api-key.responses` | `aws.bedrock-mantle` | `aws-bedrock-mantle` | `aws.bedrock-mantle.payg` | `aws.bedrock-mantle.us-east-1` | `responses` | `https://bedrock-mantle.{region}.api.aws/openai/v1` | `aws.bedrock-mantle.api-key` | `fresh` | `implemented_offline` |
| `aws.bedrock-mantle.us-east-1.sigv4.chat_completions` | `aws.bedrock-mantle` | `aws-bedrock-mantle` | `aws.bedrock-mantle.payg` | `aws.bedrock-mantle.us-east-1` | `chat_completions` | `https://bedrock-mantle.{region}.api.aws/openai/v1` | `aws.bedrock-mantle.sigv4` | `fresh` | `implemented_offline` |
| `aws.bedrock-mantle.us-east-1.sigv4.messages` | `aws.bedrock-mantle` | `aws-bedrock-mantle` | `aws.bedrock-mantle.payg` | `aws.bedrock-mantle.us-east-1` | `messages` | `https://bedrock-mantle.{region}.api.aws/anthropic/v1` | `aws.bedrock-mantle.sigv4` | `fresh` | `implemented_offline` |
| `aws.bedrock-mantle.us-east-1.sigv4.responses` | `aws.bedrock-mantle` | `aws-bedrock-mantle` | `aws.bedrock-mantle.payg` | `aws.bedrock-mantle.us-east-1` | `responses` | `https://bedrock-mantle.{region}.api.aws/openai/v1` | `aws.bedrock-mantle.sigv4` | `fresh` | `implemented_offline` |
| `aws.bedrock-runtime.us-east-1.bearer.bedrock_converse` | `aws.bedrock-runtime` | `aws-bedrock-runtime` | `aws.bedrock-runtime.payg` | `aws.bedrock-runtime.us-east-1` | `bedrock_converse` | `https://bedrock-runtime.{region}.amazonaws.com` | `aws.bedrock-runtime.bearer` | `fresh` | `implemented_offline` |
| `aws.bedrock-runtime.us-east-1.bearer.bedrock_invoke_model` | `aws.bedrock-runtime` | `aws-bedrock-runtime` | `aws.bedrock-runtime.payg` | `aws.bedrock-runtime.us-east-1` | `bedrock_invoke_model` | `https://bedrock-runtime.{region}.amazonaws.com` | `aws.bedrock-runtime.bearer` | `fresh` | `implemented_offline` |
| `aws.bedrock-runtime.us-east-1.sigv4.bedrock_converse` | `aws.bedrock-runtime` | `aws-bedrock-runtime` | `aws.bedrock-runtime.payg` | `aws.bedrock-runtime.us-east-1` | `bedrock_converse` | `https://bedrock-runtime.{region}.amazonaws.com` | `aws.bedrock-runtime.sigv4` | `fresh` | `implemented_offline` |
| `aws.bedrock-runtime.us-east-1.sigv4.bedrock_invoke_model` | `aws.bedrock-runtime` | `aws-bedrock-runtime` | `aws.bedrock-runtime.payg` | `aws.bedrock-runtime.us-east-1` | `bedrock_invoke_model` | `https://bedrock-runtime.{region}.amazonaws.com` | `aws.bedrock-runtime.sigv4` | `fresh` | `implemented_offline` |
| `azure.openai.eastus.legacy.api-key.chat_completions` | `azure.openai` | `azure-openai` | `azure.openai.payg` | `azure.openai.deployment.eastus` | `chat_completions` | `https://{resource}.openai.azure.com/openai/deployments` | `azure.openai.api-key` | `fresh` | `implemented_offline` |
| `azure.openai.eastus.legacy.entra.chat_completions` | `azure.openai` | `azure-openai` | `azure.openai.payg` | `azure.openai.deployment.eastus` | `chat_completions` | `https://{resource}.openai.azure.com/openai/deployments` | `azure.openai.entra` | `fresh` | `implemented_offline` |
| `azure.openai.eastus.v1.api-key.chat_completions` | `azure.openai` | `azure-openai` | `azure.openai.payg` | `azure.openai.deployment.eastus` | `chat_completions` | `https://{resource}.openai.azure.com/openai/v1` | `azure.openai.api-key` | `fresh` | `implemented_offline` |
| `azure.openai.eastus.v1.api-key.responses` | `azure.openai` | `azure-openai` | `azure.openai.payg` | `azure.openai.deployment.eastus` | `responses` | `https://{resource}.openai.azure.com/openai/v1` | `azure.openai.api-key` | `fresh` | `implemented_offline` |
| `azure.openai.eastus.v1.entra.chat_completions` | `azure.openai` | `azure-openai` | `azure.openai.payg` | `azure.openai.deployment.eastus` | `chat_completions` | `https://{resource}.openai.azure.com/openai/v1` | `azure.openai.entra` | `fresh` | `implemented_offline` |
| `azure.openai.eastus.v1.entra.responses` | `azure.openai` | `azure-openai` | `azure.openai.payg` | `azure.openai.deployment.eastus` | `responses` | `https://{resource}.openai.azure.com/openai/v1` | `azure.openai.entra` | `fresh` | `implemented_offline` |
| `deepseek.direct.payg.chat_completions` | `deepseek` | `deepseek` | `deepseek.api.payg` | `deepseek.direct.global` | `chat_completions` | `https://api.deepseek.com` | `deepseek.default` | `fresh` | `implemented_offline` |
| `deepseek.direct.payg.messages` | `deepseek` | `deepseek` | `deepseek.api.payg` | `deepseek.direct.global` | `messages` | `https://api.deepseek.com/anthropic` | `deepseek.default` | `fresh` | `implemented_offline` |
| `google.gemini-developer.payg.generate_content` | `google.gemini-developer` | `gemini` | `google.gemini-developer.api.payg` | `google.gemini-developer.global` | `generate_content` | `https://generativelanguage.googleapis.com/v1beta` | `google.gemini-developer.default` | `fresh` | `implemented_offline` |
| `google.vertex-ai.us-central1.adc.chat_completions` | `google.vertex-ai` | `google-vertex-ai` | `google.vertex-ai.payg` | `google.vertex-ai.serverless.us-central1` | `chat_completions` | `https://{region}-aiplatform.googleapis.com/v1beta1/projects` | `google.vertex-ai.adc` | `fresh` | `implemented_offline` |
| `google.vertex-ai.us-central1.adc.generate_content` | `google.vertex-ai` | `google-vertex-ai` | `google.vertex-ai.payg` | `google.vertex-ai.serverless.us-central1` | `generate_content` | `https://{region}-aiplatform.googleapis.com/v1` | `google.vertex-ai.adc` | `fresh` | `implemented_offline` |
| `google.vertex-ai.us-central1.adc.messages` | `google.vertex-ai` | `google-vertex-ai` | `google.vertex-ai.payg` | `google.vertex-ai.serverless.us-central1` | `messages` | `https://{region}-aiplatform.googleapis.com/v1/projects` | `google.vertex-ai.adc` | `fresh` | `implemented_offline` |
| `google.vertex-ai.us-central1.api-key.chat_completions` | `google.vertex-ai` | `google-vertex-ai` | `google.vertex-ai.payg` | `google.vertex-ai.serverless.us-central1` | `chat_completions` | `https://{region}-aiplatform.googleapis.com/v1beta1/projects` | `google.vertex-ai.api-key` | `fresh` | `implemented_offline` |
| `google.vertex-ai.us-central1.api-key.generate_content` | `google.vertex-ai` | `google-vertex-ai` | `google.vertex-ai.payg` | `google.vertex-ai.serverless.us-central1` | `generate_content` | `https://{region}-aiplatform.googleapis.com/v1` | `google.vertex-ai.api-key` | `fresh` | `implemented_offline` |
| `kimi.platform.cn.payg.chat_completions` | `kimi` | `kimi` | `kimi.platform.payg` | `kimi.platform.cn` | `chat_completions` | `https://api.moonshot.cn/v1` | `kimi.platform.cn` | `fresh` | `implemented_offline` |
| `minimax.platform.global.payg.chat_completions` | `minimax` | `minimax` | `minimax.platform.payg` | `minimax.platform.global` | `chat_completions` | `https://api.minimax.io/v1` | `minimax.platform.global.chat_completions` | `fresh` | `implemented_offline` |
| `minimax.platform.global.payg.messages` | `minimax` | `minimax` | `minimax.platform.payg` | `minimax.platform.global` | `messages` | `https://api.minimax.io/anthropic` | `minimax.platform.global.messages` | `fresh` | `implemented_offline` |
| `minimax.platform.global.payg.responses` | `minimax` | `minimax` | `minimax.platform.payg` | `minimax.platform.global` | `responses` | `https://api.minimax.io/v1` | `minimax.platform.global.responses` | `fresh` | `implemented_offline` |
| `openai.direct.payg.chat_completions` | `openai` | `openai` | `openai.api.payg` | `openai.direct.global` | `chat_completions` | `https://api.openai.com/v1` | `openai.default` | `fresh` | `implemented_offline` |
| `openai.direct.payg.responses` | `openai` | `openai` | `openai.api.payg` | `openai.direct.global` | `responses` | `https://api.openai.com/v1` | `openai.default` | `fresh` | `implemented_offline` |
| `xai.api.global.payg.responses` | `xai.api` | `xai` | `xai.api.payg` | `xai.api.global` | `responses` | `https://api.x.ai/v1` | `xai.api.global.responses` | `fresh` | `implemented_offline` |
| `xiaomi.mimo.global.payg.chat_completions` | `xiaomi.mimo` | `xiaomi-mimo` | `xiaomi.mimo.payg` | `xiaomi.mimo.global` | `chat_completions` | `https://api.xiaomimimo.com/v1` | `xiaomi.mimo.global.chat_completions` | `fresh` | `implemented_offline` |
| `xiaomi.mimo.global.payg.messages` | `xiaomi.mimo` | `xiaomi-mimo` | `xiaomi.mimo.payg` | `xiaomi.mimo.global` | `messages` | `https://api.xiaomimimo.com/anthropic` | `xiaomi.mimo.global.messages` | `fresh` | `implemented_offline` |
| `zai.platform.global.payg.chat_completions` | `zai` | `zai` | `zai.platform.payg` | `zai.platform.global` | `chat_completions` | `https://api.z.ai/api/paas/v4` | `zai.platform.global` | `fresh` | `implemented_offline` |
<!-- END GENERATED: praxis-model-invoker-current-bindings -->

其余直连、订阅、云托管和第三方路线仍保持本矩阵各节记录的真实状态。波次 C订阅记录虽已进入机器 Catalog，但全部 `callable=false`且没有 Adapter ID，不因可查询而成为可调用 Provider。

本机当前精确依赖：

| SDK | 版本 |
|---|---|
| `github.com/openai/openai-go/v3` | `v3.41.1` |
| `github.com/anthropics/anthropic-sdk-go` | `v1.56.0` |
| `google.golang.org/genai` | `v1.63.0` |

## 9. 持续可用性规则

### 9.1 有效期

| 事实类型 | 最长有效期 | 过期处理 |
|---|---:|---|
| 活跃实现路线的模型、Endpoint、弃用、兼容参数和 Region | 7天 | 标记 `stale`，禁止新增实现和发布 |
| Token/Coding Plan的允许工具、Key、Base URL、配额和条款 | 7天 | 标记 `stale`；条款不清即停止直连 |
| 云托管模型目录、API兼容表、Region和鉴权 | 7天 | 标记 `stale`，禁止部署 |
| 非活跃路线的协议、SDK与 Endpoint族 | 30天 | 进入计划前强制刷新 |
| SDK版本、许可证和变更日志 | 14天 | 升级前强制刷新 |
| 隐私、数据地域和服务条款 | 90天，且每次生产评审前强制刷新 | 未刷新不得形成生产建议 |
| Praxis实现与测试状态 | 每次相关代码、依赖或计划变更后立即更新 | live state优先并修正矩阵 |

### 9.2 立即失效触发器

- Provider发布新 Endpoint、API版本、模型兼容表或弃用公告；
- Token/Coding Plan更换 Key、Base URL、支持工具、额度或使用政策；
- 云平台改变 Region、IAM、Project/Workspace或数据保留规则；
- SDK release、许可证、依赖或官方维护主体变化；
- 真实 API返回新的 400/401/402/403/404/409/410/429/5xx；
- 流事件、错误体、未知字段或模型别名与固定样本不一致；
- 创建计划、开始实现、合并、发布或生产启用前。

旧结论不得删除；改为 `invalidated` 或 `deprecated` 并记录替代路线。`stale`、`unverified`、`terms_blocked` 路线不得进入可调用注册表。

## 10. 实现前门槛

每条 Route进入实现前必须全部满足：

- [ ] Provider、Offering、Deployment、Protocol、Endpoint、Credential与 Model引用均已分离；
- [ ] 至少一份正式 API Reference；只有博客或营销页不得实施；
- [ ] 允许用途、客户端限制、账号共享、生产边界和余额回退已核验；
- [ ] SDK所有权、语言、版本、许可证和扩展字段能力已核验；
- [ ] 主协议、降级协议和不支持协议已确定；
- [ ] 工具、推理、结构化输出、续接、缓存、用量和错误方言已列出；
- [ ] 被忽略字段、未知字段和静默降级路径已列出并测试；
- [ ] 非流式、流式、工具参数分片、异常流、订阅到期和配额耗尽已有 fixture；
- [ ] Credential脱敏、Endpoint安全、Region、取消和重试边界已设计；
- [ ] 离线契约测试通过；
- [ ] 生产启用前完成明确授权的真实烟测；
- [ ] Matrix、design、plan、module、properties与 memory同步。

## 11. 官方来源索引

| 范围 | 官方来源 |
|---|---|
| OpenAI协议与 SDK | <https://developers.openai.com/api/docs/libraries>、<https://developers.openai.com/api/docs/guides/migrate-to-responses>、<https://github.com/openai/openai-go> |
| Anthropic直连、消费者订阅与云路线 | <https://platform.claude.com/docs/en/api/overview>、<https://platform.claude.com/docs/en/cli-sdks-libraries/sdks/go>、<https://platform.claude.com/docs/en/build-with-claude/claude-in-amazon-bedrock>、<https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai>、<https://support.anthropic.com/en/articles/9876003-i-subscribe-to-a-paid-claude-ai-plan-why-do-i-have-to-pay-separately-for-api-usage-on-console> |
| Gemini Developer API | <https://ai.google.dev/api>、<https://ai.google.dev/gemini-api/docs/libraries> |
| xAI | <https://docs.x.ai/developers/quickstart>、<https://github.com/xai-org/xai-sdk-python>、<https://github.com/xai-org/xai-proto>、<https://x.ai/news/grok-build-cli>、<https://docs.x.ai/grok/faq> |
| DeepSeek | <https://api-docs.deepseek.com/>、<https://api-docs.deepseek.com/guides/anthropic_api>、<https://api-docs.deepseek.com/quick_start/pricing> |
| Kimi开放平台与 Kimi Code | <https://platform.kimi.com/docs/api/overview>、<https://www.kimi.com/code/docs/en/> |
| MiniMax按量与 Token Plan | <https://platform.minimax.io/docs/api-reference/api-overview>、<https://platform.minimax.io/docs/token-plan/intro>、<https://platform.minimax.io/docs/token-plan/quickstart> |
| Qwen/百炼与阿里订阅 | <https://help.aliyun.com/en/model-studio/qwen-api-via-openai-responses>、<https://www.alibabacloud.com/help/en/model-studio/install-sdk/>、<https://help.aliyun.com/en/model-studio/coding-plan>、<https://help.aliyun.com/en/model-studio/more-tools>、<https://help.aliyun.com/en/model-studio/savings-plan-and-resource-package> |
| Meta Llama API | <https://github.com/meta-llama/llama-api-python/blob/main/api.md>、<https://github.com/meta-llama/llama-api-python>、<https://github.com/meta-llama/llama-api-typescript>、<https://ai.meta.com/blog/llamacon-llama-news/> |
| Z.AI按量与 GLM Coding Plan | <https://docs.z.ai/api-reference/llm/chat-completion>、<https://docs.z.ai/devpack/overview>、<https://docs.z.ai/devpack/usage-policy>、<https://docs.z.ai/devpack/faq> |
| MiMo按量与 Token Plan | <https://mimo.mi.com/docs/en-US/quick-start/first-api-call>、<https://mimo.mi.com/docs/en-US/tokenplan/integration/tools-overview>、<https://mimo.mi.com/docs/en-US/tokenplan/Token%20Plan/quick-access>、<https://mimo.mi.com/docs/en-US/tokenplan/Token%20Plan/subscription> |
| AWS Bedrock | <https://docs.aws.amazon.com/bedrock/latest/userguide/endpoints.html>、<https://docs.aws.amazon.com/bedrock/latest/userguide/apis.html>、<https://docs.aws.amazon.com/bedrock/latest/userguide/bedrock-mantle.html>、<https://docs.aws.amazon.com/bedrock/latest/userguide/inference-messages-api.html>、<https://docs.aws.amazon.com/bedrock/latest/userguide/batch-inference.html>、<https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/go_bedrock-runtime_code_examples.html> |
| Vertex AI Gemini/Claude | <https://docs.cloud.google.com/vertex-ai/generative-ai/docs/start/quickstart>、<https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/claude/use-claude>、<https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/claude/batch> |
| Azure OpenAI / AI Foundry | <https://learn.microsoft.com/en-us/azure/foundry/openai/api-version-lifecycle>、<https://learn.microsoft.com/en-us/azure/developer/ai/get-started-azure-openai-starter-kit>、<https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/models-sold-directly-by-azure#gpt-oss> |

## 12. 仍需专项核验

- Azure OpenAI v1的逐 Deployment Responses/Chat兼容表、Preview/GA和 Entra刷新行为；Azure AI Foundry其他模型仍需逐模型来源卡；
- Meta Llama API的公开服务范围、SLA、实际 serving backend和生产条款；
- Kimi Code、MiniMax Token Plan、MiMo Token Plan的完整字段、流事件、并发与生产使用限制；
- GLM Coding Plan是否未来正式允许 Praxis或通用 SDK接入；当前答案是否；
- Bedrock Mantle与 Runtime逐模型 API兼容、结构化输出、缓存、状态和 Guardrail差异；
- Vertex Claude的 Anthropic SDK语言覆盖、Preview/GA状态、Region与 Provisioned Throughput差异；
- Groq、Cerebras、Together、Fireworks、OpenRouter、Cloudflare、NVIDIA和 Hugging Face的官方路线卡；
- 所有路线的模型 ID、价格、配额和真实能力必须在实现前刷新，并在明确授权的烟测中确认。
