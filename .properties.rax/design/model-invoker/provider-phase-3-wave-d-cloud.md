# 第三阶段波次 D：云托管 Provider设计卡

## 1. 状态与边界

- 模块：`model-invoker`
- 波次：D
- 核验时间：2026-07-11 01:03 CST
- 授权：沿用已审核第三阶段总计划的离线实施授权
- 禁止：真实云账号认证、真实 Key/Token、付费调用、容量或生产结论
- 当前状态：离线实现与验收已完成；真实烟测继续逐路延期

## 2. 当前官方与 SDK证据

### AWS Bedrock

- AWS `bedrock-mantle`明确承载 Responses、Chat Completions与 Messages；`bedrock-runtime`承载 Converse、Invoke、Chat与 Messages，两者配额和 Endpoint分离。
- Mantle OpenAI路径只接受 Bedrock API Key或 AWS签名凭据，禁止误用 OpenAI Key。
- Responses默认 `store=true`，按 Project隔离并在源 Region保存30天；`store=false`时禁止 `previous_response_id`。
- 当前 `anthropic-sdk-go v1.56.0`本机源码存在 `bedrock.NewMantleClient`，只暴露 Mantle Messages，并支持 Bedrock bearer、显式 SigV4、Profile和默认凭据链。
- Runtime使用 `aws-sdk-go-v2/service/bedrockruntime v1.55.0`；Converse、ConverseStream、InvokeModel与流式 Invoke保持独立协议语义。

### Google Vertex AI

- Vertex Gemini继续使用 `google.golang.org/genai v1.63.0`的 `BackendVertexAI`，Project、Location与 ADC/API Key不能与 Gemini Developer API配置混用。
- 当前 `anthropic-sdk-go v1.56.0/vertex`提供 `WithGoogleAuth`与 `WithCredentials`，把 Messages重写为 `rawPredict/streamRawPredict`，版本为 `vertex-2023-10-16`。
- Vertex OpenAI兼容入口只按 Chat Completions建模，不推断 Responses。
- serverless、Provisioned Throughput和 self-deployed Model Garden是不同 Deployment。

### Azure OpenAI / Foundry

- Azure OpenAI v1使用 `/openai/v1/`，不再追加 dated `api-version`；Responses和 Chat按 Deployment能力分别验收。
- 请求 `model`是 Azure deployment name，不能假设等于模型 ID。
- OpenAI Go SDK可用自定义 Base URL；API Key使用 `api-key`，Entra ID使用 Azure Core bearer token policy并自动刷新。
- legacy dated API与 v1保持独立 Binding；其他 Foundry模型按具体模型、Region、Preview/GA和部署类型建卡，不继承 Azure OpenAI能力。

## 3. 实施切片

### D1 AWS

1. Catalog先固定 Mantle与 Runtime的 Provider/Offering/Deployment/Protocol/Endpoint/Credential差异。
2. Mantle Responses、Chat、Messages复用现有协议 driver，但拥有 Bedrock方言、Project状态约束和 Bedrock认证。
3. Runtime新增 `internal/protocol/bedrock`，实现 Converse与 Invoke的最小 Agent语义、流、错误和 Raw边界。
4. AWS SDK对象、签名凭据和请求不得进入公共 API或 unwrap链。

### D2 Vertex

1. Vertex Gemini复用 GenerateContent driver并注入 Vertex身份。
2. Vertex Claude复用 Messages driver与官方 Vertex middleware。
3. OpenAI兼容路线只复用 Chat driver。
4. Project/Location、Credential与 Deployment严格绑定。

### D3 Azure

1. v1分别复用 Responses和 Chat driver；API Key与 Entra ID产生不同 Credential Profile。
2. legacy只建立独立可测试 Binding，不向 v1追加 `api-version`。
3. Foundry其他模型保持逐模型 planned或 unverified，不能批量继承。

## 4. 统一验收

- 每个云 Provider至少一条 Route通过本机 HTTP/SDK fake黑白盒；
- 所有 Route测试 Provider/Protocol/Endpoint/Region/Project/Deployment name和 Credential错配；
- 云 SDK类型不出现在公共签名；错误与 Raw继续经过统一脱敏；
- integration只编译，真实云烟测保持逐路延期；
- Catalog、Schema、Matrix、module、properties与 memory同步 live state。

## 5. 落地结果

- Runtime Adapter：`aws-bedrock-mantle`、`aws-bedrock-runtime`、`google-vertex-ai`、`azure-openai`；
- 新协议：`bedrock_converse`、`bedrock_invoke_model`；
- Catalog：48条总记录，25条 callable、23条控制记录；所有云 callable仅为 `implemented_offline`；
- 离线证据：AWS SigV4/bearer、Mantle三协议与 Key刷新、Vertex Gemini/Claude middleware、Azure v1/legacy与 Entra刷新均通过本机 SDK HTTP fake；
- 延期项：真实云认证、付费调用、容量、具体模型/Region生产能力和第三方名单。
