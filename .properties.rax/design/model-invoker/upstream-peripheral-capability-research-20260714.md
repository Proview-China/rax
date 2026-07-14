# 上游外围能力官方事实研究（2026-07-14）

## 1. 研究目的与边界

本研究只回答“每个上游官方当前提供什么可调用能力、采用什么生命周期和协议”，为 Praxis 的上游并集调用面提供事实依据。

本轮不设计或实现：密钥安全存储、账号登录器、组织管理、费用管理、微调、评测、模型部署和统一集成测试器。订阅 Harness 仍只服务其官方允许的模型调用面；不能把 Codex/Claude Code/Kimi Code 等订阅认证外推到媒体或平台资源 API。

能力来源必须显式区分：

- `provider_native`：供应商官方原生 API；
- `protocol_compatible`：供应商明确声明的 OpenAI/Anthropic 兼容面；
- `cloud_native`：Azure、Vertex、Bedrock 等云平台原生面；
- `local_native`：Ollama、llama.cpp 等本地服务器原生 API；
- `harness_hosted`：官方 CLI/SDK/App Server 中的模型或工具面；
- `unavailable`：官方当前没有该能力；
- `unverified`：没有足够官方证据，不能升级为可调用。

## 2. 统一生命周期分类

外围能力不能继续塞进现有单次 LLM `Request`。官方 API 实际分为四类：

| 生命周期 | 典型能力 | 统一所需对象 |
|---|---|---|
| 同步/流式推理 | Embedding、Rerank、ASR、TTS、图像生成、Moderation | `OperationRequest/OperationResult/Artifact` |
| 异步作业 | 视频、长语音、Batch、部分图像 | `JobRef/JobStatus/JobResult/Cancel` |
| 持久资源 | Files、Uploads、Vector Store、Collection、Cache | `ResourceRef/ResourcePage/Content` |
| 双向会话 | OpenAI Realtime、Gemini Live、xAI Voice、MiniMax WebSocket TTS | `Session/ClientEvent/ServerEvent/Close` |

这四类只统一意图、状态和结果；请求体内仍允许受控的 `provider_options`，不能假装所有供应商参数等价。

## 3. 官方能力矩阵

### 3.1 国际核心上游

| 上游 | 同步/流式推理 | 异步作业 | 持久资源 | 双向会话 | 关键边界 |
|---|---|---|---|---|---|
| OpenAI Platform | Embeddings、Image generate/edit/variation、ASR/translation、TTS、Moderation | Videos create/edit/extend/remix、Batch | Files、Uploads、Vector Stores/Search、视频内容与角色资源 | Realtime voice、transcription、translation；WebSocket/WebRTC/SIP | ChatGPT/Codex 订阅不等于 Platform API 额度 |
| Anthropic API | Messages 内图像/PDF/文件、Token Count | Message Batches | Files API；代码执行/Skills 生成文件可下载 | 未确认独立实时语音 API | Anthropic 不提供自有 Embedding；官方文档推荐第三方 Voyage，必须建独立 Provider 身份 |
| Gemini API | 多模态 Embedding、图像、TTS、GenerateContent/Interactions 多模态 | Veo/Omni 视频、Batch GenerateContent/Embedding | Files、File Search Store/Documents、显式 Cache（协议相关） | Live API：音频/视频/文本输入和原生音频输出 | `generateContent`、Interactions、Live 和长任务不是一个协议 |
| xAI API | Image、Video、文件随消息、Collection Search | Image/Video/Chat Batch | Files、Public URL、Collections | Voice Agent WebSocket | Collection 管理使用 Management API Key，推理搜索使用普通 API Key；本轮只做调用契约，不做管理认证器 |

### 3.2 中国上游

| 上游 | 已确认官方外围能力 | 当前边界 |
|---|---|---|
| Kimi Platform | Files upload/list/get/delete/content；文件目的含 extract/image/video/batch；Batch create/get/list/cancel；Chat 内多模态理解 | 未确认官方图像/视频/语音“生成”产品 API；不能从开源模型推导平台能力 |
| MiniMax Platform | 图像文生图/图生图；视频文生/图生/首尾帧/主体参考/Agent；同步与异步 TTS；WebSocket TTS；音乐；声音复刻/设计/管理；Files | 能力协议差异大，必须走 MiniMax 原生 operation 描述，不能只走 Chat 兼容面 |
| Z.AI | 图像同步/异步、视频异步、ASR、Chat 多模态输入、OCR/Layout、Web Reader、Context Cache | 文件管理仍有 beta/权限边界；未确认 TTS、Embedding、Batch 就标为不可用或未验证 |
| Qwen/Model Studio | 文本/多模态 Embedding、Rerank、图像、视频、ASR、TTS/声音复刻与设计、实时语音、音乐、文件问答、Batch | DashScope 原生、OpenAI 兼容和 Realtime 协议必须分 Route；地域和模型支持不能合并 |
| Xiaomi MiMo | Chat 内图像/音频/视频理解；ASR；TTS、声音复刻、声音设计；流式音频 | 官方 FAQ 明确当前不支持本地文件上传；这些能力复用 Chat Completions 外形但字段是 MiMo 方言 |
| DeepSeek | Chat Completions、Anthropic 兼容、FIM、JSON/Tool 等文本面 | 官方公开 API 未提供可确认的媒体生成、Files、Embedding、Batch；外围矩阵为 `unavailable`，不是待模拟 |

### 3.3 本地与企业自建

| 上游 | 已确认能力 | 适配策略 |
|---|---|---|
| Ollama native | `/api/chat`、`/api/generate`、`/api/embed`、模型管理；工具、结构化输出、视觉、思考、流式 | 独立 `ollama_native` Route，保留 keep-alive/options 等本地方言 |
| Ollama OpenAI compatibility | Chat Completions、Responses（有状态能力受限）、Embeddings、实验性 Images、Models | 独立 `ollama_openai_compat`，不能标成 OpenAI 官方等价 |
| llama.cpp native | completion、embedding/embeddings、rerank、FIM、tokenize、health/props/slots/metrics；实验性多模态 | 独立 `llamacpp_native`，能力由启动参数和加载模型动态探测 |
| llama.cpp OpenAI compatibility | Chat Completions、Completions、Responses 映射、Embeddings、Models | 独立 `llamacpp_openai_compat`；工具依赖 chat template/`--jinja`，Rerank/Embedding 依赖启动模式 |
| 任意企业 OpenAI-compatible | 只能按实际声明支持的 endpoint/model/capability 使用 | 必须显式 Base URL、路径、模型白名单、认证方式和 capability allowlist；默认不自动探测公网、不冒充 OpenAI |

## 4. 不能抹平的差异

1. “图像生成”可能是同步 base64、短期 URL、Responses 工具输出或异步 Job。
2. “视频生成”几乎总是异步，但 Job 状态名、轮询、下载与 URL 有效期不同。
3. “Files”可能是临时输入、永久资源、Batch JSONL、模型生成物或 Collection 文档，生命周期不能合并。
4. “Realtime”不是普通 SSE；它需要连接级状态、客户端与服务端事件、背压、半关闭和断线语义。
5. OpenAI-compatible 只说明部分协议外形相似，不证明 Responses state、工具、usage、Files、Batch 或媒体端点存在。
6. 本地服务器的能力由二进制版本、启动参数、模型和模板共同决定，Profile 必须基于探测结果而不是品牌名。

## 5. 官方一手来源

- OpenAI：[Image generation](https://developers.openai.com/api/docs/guides/image-generation)、[Video generation](https://developers.openai.com/api/docs/guides/video-generation)、[Text to speech](https://developers.openai.com/api/docs/guides/text-to-speech)、[Speech to text](https://developers.openai.com/api/docs/guides/speech-to-text)、[Realtime](https://developers.openai.com/api/docs/guides/realtime)、[Embeddings](https://developers.openai.com/api/docs/guides/embeddings)、[Batch](https://developers.openai.com/api/docs/guides/batch)、[File Search](https://developers.openai.com/api/docs/guides/tools-file-search)。
- Anthropic：[Files API](https://platform.claude.com/docs/en/build-with-claude/files)、[Message Batches](https://platform.claude.com/docs/en/api/messages/batches)、[Token counting](https://platform.claude.com/docs/en/build-with-claude/token-counting)、[Embeddings boundary](https://platform.claude.com/docs/en/build-with-claude/embeddings)。
- Google：[Image generation](https://ai.google.dev/gemini-api/docs/image-generation)、[Video generation](https://ai.google.dev/gemini-api/docs/video)、[TTS](https://ai.google.dev/gemini-api/docs/speech-generation)、[Live API](https://ai.google.dev/gemini-api/docs/live-api/get-started-sdk)、[Embeddings](https://ai.google.dev/gemini-api/docs/embeddings)、[Batch](https://ai.google.dev/gemini-api/docs/batch-api)、[File Search](https://ai.google.dev/gemini-api/docs/file-search)。
- xAI：[Batch](https://docs.x.ai/developers/advanced-api-usage/batch-api)、[Files](https://docs.x.ai/developers/files)、[Collections](https://docs.x.ai/developers/files/collections)、[Voice Agent](https://docs.x.ai/developers/models/voice-agent-api)。
- Kimi：[API overview](https://platform.kimi.ai/docs/api/overview)、[Files upload](https://platform.kimi.ai/docs/api/files-upload)、[Batch create](https://platform.kimi.ai/docs/api/batch-create)。
- MiniMax：[API overview](https://platform.minimaxi.com/docs/api-reference/api-overview)、[官方文档索引](https://platform.minimaxi.com/docs/llms.txt)。
- Z.AI：[Chat](https://docs.z.ai/api-reference/llm/chat-completion)、[ASR](https://docs.z.ai/api-reference/audio/audio-transcriptions)、[Video](https://docs.z.ai/api-reference/video/generate-video)、[官方文档索引](https://docs.z.ai/llms.txt)。
- Qwen/Model Studio：[模型能力](https://help.aliyun.com/en/model-studio/models)、[Embedding](https://help.aliyun.com/zh/model-studio/embedding)、[Batch](https://help.aliyun.com/zh/model-studio/batch-inference)、[TTS](https://help.aliyun.com/zh/model-studio/speech-synthesis-api-reference/)、[Video](https://help.aliyun.com/zh/model-studio/use-video-generation)。
- MiMo：[Models](https://mimo.mi.com/docs/en-US/quick-start/model)、[Chat API](https://mimo.mi.com/docs/api/chat/openai-api)、[ASR](https://mimo.mi.com/docs/en-US/usage-guide/Speech-Recognition)、[TTS](https://mimo.mi.com/docs/usage-guide/speech-synthesis-v2.5)、[API integration boundary](https://mimo.mi.com/docs/en-US/quick-start/faq/api-integration)。
- Local：[Ollama OpenAI compatibility](https://docs.ollama.com/api/openai-compatibility)、[Ollama Embed](https://docs.ollama.com/api/embed)、[llama.cpp server](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md)。

## 6. 研究结论

首版外围并集不应试图把供应商参数全部翻译成一个巨型结构。正确边界是：

1. 固定 operation 意图、生命周期、资源引用、作业状态、产物和错误；
2. 每个 Provider 用独立方言把统一意图编译为官方请求；
3. 对尚未拥有稳定跨厂商字段的能力，允许受控 `provider_options`，并把未消费字段视为错误；
4. 保留原始响应和 Provider 元数据，但认证头永不进入审计载荷；
5. 本地/中转 Route 与官方直连身份严格分开；
6. 订阅 Harness 只能使用其官方客户端允许的能力，不能成为 Platform API 通用凭据。
