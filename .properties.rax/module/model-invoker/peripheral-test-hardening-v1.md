# 外围并集大批量测试与集成加固 v1

## 结果

外围切片`operation/resource/job/realtime/provider/localcompat`使用统一`-coverpkg`口径，语句覆盖率从`62.9%`提升到`85.2%`。本轮新增18个测试/Fuzz入口，完整全仓普通、shuffle、race和integration-tag测试均通过。

## 白盒与负向矩阵

- 所有Resource与Job公共方法、正确Operation映射、输入map防御性复制、nil receiver与错误Kind；
- Invoker身份补全、超时/取消/typed/opaque错误、nil stream与上下文Stream包装；
- Composite排序、能力聚合、Invoke/Stream分派、重叠所有者、错误身份、未知Kind与能力漂移；
- 14组官方/本地Catalog逐一构造成闭合Provider，检查重复Operation和不可调用Spec泄漏；
- Artifact、Request、NativeHTTP Config/Spec、Gemini upload、Realtime Config与localcompat Config大批量负向用例；
- Gemini upload握手/完成HTTP错误分类、可信URL、响应限额和资源解码。

## 黑盒与集成矩阵

- HTTP：JSON、multipart、binary、SSE、NDJSON、重定向、状态码、响应限额、模型和认证；
- 本地：OpenAI-compatible、Ollama、llama.cpp分别执行Chat和Responses，共六个Invoke组合；Chat与Responses流各自真实运行SSE状态机；
- Realtime：真实WebSocket握手、首帧模型绑定、query模型绑定、文本/二进制双向帧、CloseWrite和关闭幂等；
- 跨域集成：同一测试串联Operation、Resource Client、Batch Job NDJSON、Ollama兼容文本和Realtime WebSocket。

## Fuzz

- 原有Operation请求与NativeHTTP Spec安全Fuzz继续保留；
- 新增Realtime配置凭据安全Fuzz，验收运行约50,137次；
- 新增本地兼容配置凭据安全Fuzz，验收运行约18,517次。

## 修正

1. 所有Resource/Job专用公共方法均对未初始化Client fail closed，不再panic；
2. HTTP SSE `[DONE]`、NDJSON EOF和binary EOF均保证唯一正常完成事件；完成后重复`Next`稳定返回false。

## 验证命令

```text
go vet ./...
go test -count=1 ./...
go test -count=1 -shuffle=on ./...
go test -race -count=1 ./...
go test -tags=integration -count=1 ./tests/integration
go test -covermode=atomic -coverpkg=./operation/...,./resource/...,./job/...,./realtime/...,./provider/localcompat/... ./tests/operation ./tests/realtime ./tests/localcompat
./scripts/verify-offline.sh
```
