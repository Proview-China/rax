# 外围能力并集与本地上游实施计划 v1

## 1. 状态与产物

- 状态：陈旧计划（已完成）
- 日期：2026-07-14
- 设计：[外围能力并集与本地上游 v1](../../design/model-invoker/peripheral-union-and-local-upstream-v1.md)

完成后将产出：独立外围 Operation/Resource/Realtime 公共契约、官方与自建上游 operation catalog、OpenAI-family 通用执行器、本地 OpenAI-compatible/Ollama/llama.cpp 适配、离线黑白盒与低预算真实特殊能力验证。

## 2. 清单

### A. 官方事实与冻结设计

- [x] 盘点现有 LLM core 与能力名占位；
- [x] 研究 OpenAI、Anthropic、Gemini、xAI 官方外围能力；
- [x] 研究 Kimi、MiniMax、Z.AI、Qwen、MiMo、DeepSeek 官方能力；
- [x] 研究 Ollama、llama.cpp 与任意 OpenAI-compatible 自建端点；
- [x] 固定四种生命周期和 v1 类型边界；

### B. 公共内核

- [x] 实现 Operation 类型、校验、能力、Registry、Invoker、Composite、结果与流；
- [x] 实现 Artifact、Job、ResourceRef 和统一状态；
- [x] 实现 Resource、Job 与 Realtime 接口；
- [x] 实现脱敏、错误、重试所有权和审计边界；

### C. 上游执行器

- [x] OpenAI-family JSON/multipart/binary/SSE/NDJSON operation transport；
- [x] OpenAI Platform operation specs；
- [x] Anthropic Files/Batch/Token Count specs及Files beta header；
- [x] Gemini Media/Embedding/Files/Batch specs及可信resumable upload；
- [x] xAI Media/Files/Collections/Batch/Voice specs，Management与Inference凭据分面；
- [x] Kimi Files/Batch specs；
- [x] MiniMax Media/Speech/Music/Files首批官方spec；
- [x] Z.AI Image/Video/ASR首批官方spec；
- [x] Qwen Media/Embedding/Rerank及OpenAI-compatible Batch specs；
- [x] MiMo ASR/TTS specs；

### D. 本地与企业自建

- [x] `local-openai-compatible` 文本与外围 Route；
- [x] Ollama OpenAI-compatible文本Route及native Embedding/实验Image operation；
- [x] llama.cpp OpenAI-compatible文本Route及native Embedding/Rerank/Token Count operation；
- [x] endpoint/auth/model/capability allowlist 和离线探测快照；本机无运行中服务，真实本地推理标记`not_run`；

### E. 测试与收口

- [x] 单元、表驱动、负向、fuzz；
- [x] `httptest` JSON/multipart/binary/SSE/NDJSON/WebSocket/resumable upload黑盒；
- [x] OpenAI-compatible/Ollama/llama.cpp三种身份的假服务系统集成；
- [x] 运行统一离线门禁；
- [x] 使用用户临时中转 Key 只测试未覆盖的低成本Files List能力；四路均未证明透传外围API；
- [x] 复用 Codex 订阅仅沿用已完成的Codex官方Harness证据，不把订阅外推为Platform API；
- [x] 同步 module README、design/plan 索引和 memory；
- [x] 最终 Review 与未验证矩阵。

## 3. 真实测试预算规则

1. 密钥只从进程环境或一次性 stdin 注入，不写仓库、日志、测试快照或 shell history 资产；
2. 已通过的文本与 Tool Call 不重测；
3. 优先 Files list/get、Batch list/get、模型错误边界等零生成或低成本请求；
4. 媒体生成只有端点和模型被中转明确支持时才发最小请求；
5. 每次真实请求记录 Provider、Operation、HTTP、request ID、usage/计费可见性和结果，不记录密钥；
6. 累计上限沿用用户给出的 20 美元，无法从响应确认金额时只报告请求和 usage，不虚构美元数。
