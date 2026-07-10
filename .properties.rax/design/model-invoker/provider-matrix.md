# Provider 与 SDK 调查矩阵

## 1. 调查说明

- 调查日期：2026-07-10
- 调查对象：Praxis 第一批模型 Provider
- 证据范围：厂商官方文档、官方 SDK 仓库和本机 Go Module 实时查询
- 注意：API、模型、SDK 和兼容边界变化频繁，进入 plan 和实现前必须重新核对。

## 2. Provider 矩阵

| Provider | 主要模型 | 官方 API 类型 | 官方 Go SDK | Praxis 初步接入方式 |
|---|---|---|---|---|
| OpenAI | GPT | Responses、Chat Completions、Realtime | 有 | `openai-go/v3` |
| Anthropic | Claude | Messages、Message Batches、Token Count | 有 | `anthropic-sdk-go` |
| Google | Gemini | Interactions、GenerateContent、StreamGenerateContent、Live、Batch | 有 | `go-genai`；Interactions 必要时补 REST |
| xAI | Grok | Responses、Chat Completions、原生 gRPC | 未发现 | REST 使用 `openai-go`；gRPC 后续独立适配 |
| DeepSeek | DeepSeek | OpenAI Chat、Anthropic Messages、FIM Completion | 未发现 | `openai-go`、`anthropic-sdk-go` 与扩展字段 |
| Moonshot | Kimi | OpenAI Chat、Anthropic Messages、Files、Batch | 未发现核心 Go SDK | 兼容 SDK 与必要的原生 HTTP |
| MiniMax | MiniMax | Anthropic Messages、OpenAI Chat、原生多媒体 API | 未发现 | Anthropic Go SDK 优先；OpenAI Go SDK补充 |
| Alibaba Cloud | Qwen | Responses、Chat Completions、DashScope、Batch | 未发现 | OpenAI Go SDK 与 DashScope 原生 HTTP |
| Meta | Llama | Llama API、OpenAI 兼容 Chat | 未发现 | OpenAI Go SDK；实际托管 Provider 单独建档 |
| Z.AI | GLM | OpenAI Chat、Anthropic Messages、原生多媒体 API | 未发现 | OpenAI/Anthropic Go SDK 与扩展字段 |
| Xiaomi | MiMo | Responses、Chat Completions、Anthropic Messages、ASR/TTS | 未发现 | OpenAI/Anthropic Go SDK 与原生媒体接口 |

## 3. 已核验的 Go 环境

| 项目 | 现场结果 |
|---|---|
| 本机 Go | `go1.25.6 linux/amd64` |
| OpenAI Go SDK | `github.com/openai/openai-go/v3@v3.41.1` |
| Anthropic Go SDK | `github.com/anthropics/anthropic-sdk-go@v1.56.0` |
| Google Gen AI Go SDK | `google.golang.org/genai@v1.63.0` |

本机 Go 版本满足三套官方 SDK 的当前要求。

## 4. 关键兼容风险

### 4.1 Responses API 不是一个固定能力集合

- OpenAI 是原生实现；
- xAI、Qwen、MiMo 提供兼容实现；
- 各家对后台执行、服务端状态、托管工具和参数的支持不同；
- 未在官方文档中确认 DeepSeek、Kimi、MiniMax、Meta 和 GLM 提供通用 Responses 端点。

因此必须按 Provider 维护 Responses 能力表，不能只维护一个 Base URL。

### 4.2 Anthropic Messages 兼容存在子集差异

- DeepSeek、Kimi、MiniMax、GLM 和 MiMo 提供或声明 Anthropic 兼容路径；
- 兼容实现可能忽略部分参数；
- thinking block、签名、工具内容块、图片和文档支持范围不同；
- 多轮工具调用可能要求完整回传厂商响应块。

### 4.3 Gemini 需要独立协议族

Gemini 的 `Content`、`Part`、`GenerateContent`、`Interactions` 和 Live 事件不能完整等价为 OpenAI Chat。Praxis 必须保留 Gemini 原生 Adapter，再映射到统一语义。

### 4.4 Meta Llama 必须区分模型与托管方

Llama 是模型家族；Meta Llama API、第三方云平台和本地推理服务是不同 Provider。相同模型名称不代表认证、限流、工具或响应结构相同。

## 5. SDK 使用判断

| 场景 | 判断 |
|---|---|
| 官方 Go SDK覆盖完整 | 直接使用并封装 |
| 官方 Go SDK缺失，官方 TS SDK更完整 | 使用隔离 TS Sidecar |
| 厂商正式支持 OpenAI/Anthropic 兼容 | 复用对应官方 Go SDK，但维护能力方言 |
| 兼容 SDK无法表达扩展字段 | Provider 内增加原生 HTTP/SSE/WebSocket |
| 仅有社区 SDK | 默认不用，先审计再决定 |

## 6. 官方资料入口

- OpenAI Go SDK：<https://github.com/openai/openai-go>
- Anthropic Go SDK：<https://platform.claude.com/docs/en/cli-sdks-libraries/sdks/go>
- Gemini SDK：<https://ai.google.dev/gemini-api/docs/libraries>
- Gemini API：<https://ai.google.dev/api>
- xAI API：<https://docs.x.ai/developers/rest-api-reference/inference/chat>
- DeepSeek API：<https://api-docs.deepseek.com/>
- Kimi API：<https://platform.kimi.com/docs/guide/start-using-kimi-api>
- MiniMax API：<https://platform.minimax.io/docs/api-reference/api-overview>
- Qwen Responses：<https://help.aliyun.com/en/model-studio/qwen-api-via-openai-responses>
- Meta Llama API 说明：<https://ai.meta.com/blog/llamacon-llama-news/>
- GLM Chat API：<https://docs.z.ai/api-reference/llm/chat-completion>
- MiMo Responses：<https://mimo.mi.com/docs/en-US/api/chat/responses>

## 7. 下一次调查要求

进入 plan 前，需要为每个 Provider 单独补齐：

- 全部请求和响应字段；
- 流式事件顺序；
- 支持模型与地域；
- 认证和请求头；
- 工具、多模态、缓存与状态能力；
- 错误码、限流和重试建议；
- SDK 版本、许可证和变更日志；
- 官方文档未说明或互相冲突的部分；
- 真实 API 烟雾测试结果。
