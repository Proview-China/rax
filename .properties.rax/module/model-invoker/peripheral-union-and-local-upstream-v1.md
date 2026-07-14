# 外围能力并集与本地上游模块说明 v1

## 1. 作用

本切片把LLM核心之外的媒体、Embedding/Rerank、Moderation、Files/Stores、Batch/Video Job和Realtime能力纳入Praxis并集语义，同时支持任意显式配置的OpenAI-compatible企业端点、Ollama和llama.cpp。

统一的是Operation、Resource、Job、Session和可观测结果；HTTP路径、认证、模型位置、上传协议、事件帧与能力来源继续忠实保留上游差异。

## 2. 组成

| 部分 | 位置 | 职责 |
|---|---|---|
| Operation内核 | `ExecutionRuntime/model-invoker/operation/` | 类型、校验、能力、Registry、Invoker、Composite与统一结果 |
| 原生HTTP执行器 | `operation/nativehttp/` | JSON、multipart、binary、SSE、NDJSON、认证、限额、脱敏与生命周期 |
| 官方Spec | `operation/specs/` | 十家上游及Ollama、llama.cpp的method/path/model/capability事实 |
| 资源与作业 | `resource/`、`job/` | Files/Stores、Batch/Video生命周期和流式结果 |
| Gemini上传 | `operation/geminiupload/` | 同源且受限路径的两阶段resumable upload |
| Realtime | `realtime/`、`realtime/nativews/`、`realtime/specs/` | OpenAI Realtime、Gemini Live、xAI Voice和本地WebSocket |
| 本地兼容面 | `provider/localcompat/` | OpenAI-compatible、Ollama、llama.cpp与企业HTTPS自建身份 |

## 3. 已实现能力

- OpenAI：Embedding、Moderation、Images、Videos、Audio、Speech、Files、Uploads、Vector Stores、Batch和Realtime首批生命周期；
- Anthropic：Files、Message Batch、Token Count；
- Gemini：Images、Video、TTS、Embedding、Files、File Search、Batch、Live及可信resumable upload；
- xAI：Images、Video、Files、Collections、Batch和Voice，其中Management与Inference凭据分面；
- Kimi、MiniMax、Z.AI、Qwen、MiMo：按官方文档支持的Files、Batch、媒体、ASR/TTS、Embedding/Rerank等首批能力；
- 本地与企业：OpenAI-compatible文本面、Ollama原生Embedding/实验图像、llama.cpp原生Embedding/Rerank/Token Count。

每项能力显式标记`native`、`compatible`、`partial`或`unsupported`。模型型Operation必须由宿主提供精确模型白名单；不存在由Base URL推断出的隐式能力。

## 4. 安全与一致性

1. 官方、Relay、本地、企业四种信任策略分别限制scheme、host、base path和重定向；
2. 匿名本地调用在最终transport删除环境继承认证；企业调用只固定注入本Route的显式Bearer；
3. JSON和multipart中的`model`与公共请求二次绑定，阻止静默模型漂移；
4. Gemini上传URL只能来自可信握手响应，必须同源且位于限定路径；
5. xAI Collections管理面与推理搜索面不能共用凭据；
6. 同步调用拒绝NDJSON批结果，必须经`job.Client.StreamResults`消费；
7. 密钥、上传token、认证头、错误cause与超限响应不进入普通错误、快照或审计输出。
8. Realtime模型必须由一个位置独占绑定：OpenAI/xAI使用URL查询参数，Gemini使用首帧`setup.model=models/{model}`；请求与原生位置不一致时在拨号前拒绝。

## 5. 验证结果

- `./scripts/verify-offline.sh`：通过；包含module校验、gofmt、tidy漂移、diff、vet、普通、shuffle、race、integration-tag离线套件和Catalog资产门禁；
- Operation、Resource、Job、Gemini upload、Realtime和本地适配均有单元、负向、`httptest`/WebSocket黑盒测试；
- 两项3秒Fuzz通过；Operation校验约82k次、原生Spec/凭据安全约57k次；
- 集成测试默认不读取真实凭据，真实入口需要显式环境变量双开关。

## 6. 真实探针

使用用户临时Relay凭据且只经本模块`operation`调用器执行了零生成Files List探针：GPT、Grok分别返回503，Claude返回401，Gemini原生Files路径返回404。因此这些Relay只确认此前文本/Tool Call兼容，不得声明透传外围Files API。

本机未发现运行中的Ollama或llama.cpp，真实本地推理为`not_run`。Codex订阅只沿用已完成的官方Codex Harness证据，不外推为OpenAI Platform媒体、Files或Realtime授权。

## 7. 当前边界

- 已实现通用transport和首批官方Spec，但厂商媒体参数仍以受控原生Body为主，未全部形成强类型builder；
- 浏览器WebRTC、SIP、ephemeral token、MiniMax专用实时方言、声音复刻/设计/管理等长尾能力尚未形成专用门面；
- Hosted Tools、Prompt Cache创建、云异步推理和跨Provider资产迁移不在本切片；
- 离线通过不等于生产可用，具体账号、区域、模型与额度仍须逐Route真实验证。
