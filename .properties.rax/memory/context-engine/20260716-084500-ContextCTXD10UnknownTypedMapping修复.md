# Context CTX-D10 Unknown Typed Mapping修复

时间：2026-07-16 08:45（Asia/Shanghai）

## 事件

独立代码Review发现CTX-D10 Runtime Adapter把`ErrUnknown`错误降级为`ErrorUnavailable`，且默认分支会把取消与超时降级为`InvalidArgument`；metadata测试Fake也未优先保真返回`ctx.Err()`。本次仅在Context独占模块及对应module/memory资产内完成P1修复，没有修改Runtime、Application、Harness、Tool或Model Invoker。

## 修复内容

- `ErrUnknown`、`context.Canceled`和`context.DeadlineExceeded`统一映射为Runtime `ErrorIndeterminate`；
- 仅真实`ErrUnavailable`映射为`ErrorUnavailable`，Conflict、Stale/Expired与NotFound继续按冻结合同分类；
- metadata Fake、ParentFrame Owner Reader和Runtime Adapter在输入验证、依赖读取及投影返回边界优先检查并原样传播`ctx.Err()`；
- 新增cancel、deadline与Owner Unknown的故障/Conformance反例，断言零current projection；预取消或超时请求还断言Owner Reader零调用。

## 实际验证

- `go test -count=100 ./internal/testkit ./kernel ./tests/failure ./tests/conformance -run 'Canceled|Deadline|Unknown|ParentFrame|MetadataStoreV1'`：PASS；
- `go test -race -count=20 ./internal/testkit ./kernel ./tests/failure ./tests/conformance`：PASS；
- `go test -count=1 ./...`：PASS；
- `go test -race -count=1 ./...`：PASS；
- `go vet ./...`：PASS；
- 对本次Go文件执行`gofmt -w`：完成。

## 保留边界

修复不创建Applicability Fact或Evidence，不写Tool watermark、Provider、DomainResult、Settlement、Apply、Generation CAS、新Frame或Continuation；不进入G6B。测试Fake仍不是production backend、State Plane root、持久化承诺或SLA。
