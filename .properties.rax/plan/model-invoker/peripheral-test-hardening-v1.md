# 外围并集大批量测试与集成加固计划 v1

## 状态

- 状态：陈旧计划（已完成）
- 日期：2026-07-14
- 对象：`ExecutionRuntime/model-invoker/{operation,resource,job,realtime,provider/localcompat}`

## 目标

通过大批量白盒单元、表驱动负向、HTTP/WebSocket黑盒、跨生命周期集成、Race、Shuffle和Fuzz，证明外围并集的路由、模型、认证、状态、流终态和错误边界稳定，并以测试暴露的问题反向修正实现。

## 完成清单

- [x] 建立外围切片统一覆盖基线：62.9%；
- [x] 覆盖File/Store/Video/Batch全部Client方法和错误分支；
- [x] 覆盖Composite分派、重叠、身份错配和能力漂移；
- [x] 覆盖全部官方/本地Catalog构造与闭合验证；
- [x] 覆盖Artifact、Request、NativeHTTP Config/Spec负向矩阵；
- [x] 覆盖Gemini resumable upload配置、请求、HTTP状态、解码和限额；
- [x] 覆盖OpenAI-compatible/Ollama/llama.cpp三产品×Chat/Responses双协议Invoke；
- [x] 覆盖Chat/Responses SSE流与能力/模型/身份错配；
- [x] 覆盖Realtime配置帧模型、query模型、文本、二进制、CloseWrite与握手错误；
- [x] 新增本地配置与Realtime配置凭据安全Fuzz；
- [x] 新增HTTP Operation→Resource→Job→Local LLM→Realtime跨域离线集成；
- [x] 运行全仓普通、shuffle、race、integration-tag和统一离线门禁；
- [x] 最终外围切片统一语句覆盖率达到85.2%。

## 测试发现并修复

1. `resource.Client.Content/Search`及`job.Client.List/Results/Cancel/Delete`的nil receiver会panic；现统一返回未初始化错误；
2. NDJSON和binary HTTP流在正常EOF时没有统一`StreamCompleted`；现正常EOF恰好产生一次完成事件，错误EOF不伪造完成。

## 非目标

- 不执行新的付费API或套餐调用；
- 不以覆盖率替代真实Provider账号、区域、模型和额度验证；
- 不修改本轮之外的Harness/Runtime用户改动。
