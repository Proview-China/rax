# Harness SessionCurrentReaderV4最小Port实施

时间：2026-07-16 18:21（Asia/Shanghai）

## 事件

`SessionCurrentReaderV4` additive设计短审获`YES(P0/P1/P2=0)`后，Harness完成最小public Port实现：Reader只暴露`InspectSessionV4`，`SessionFactPortV4`兼容嵌入并保留原Create/CAS方法。

## 实施边界

- 未修改`GovernedSessionV4`、`SessionCASRequestV4`、canonical/digest、Store、CAS或fake语义；
- only-Inspect实现满足Reader；现有`GovernedStoreV2`继续满足FactPort及Reader；
- 窄Reader方法集不含Create/CAS，未来P3构造器只能接收该public能力；
- P3 typed-nil的`Unavailable/ComponentMissing`运行时分类留待Assembler构造器实现；
- Application V2 public types完整落盘并compile前，P3仍不实施。

## 验证状态

首次Harness包级验证被共享工作树中在途Application V2 contract/ports编译错误阻断，未跨Owner修复。Application Owner恢复compile后已复跑并通过：

- `go test ./ports ./tests/conformance ./tests/contract`：PASS；
- `go test -race ./ports ./tests/conformance ./tests/contract`：PASS；
- `go vet ./ports ./tests/conformance ./tests/contract`：PASS；
- gofmt与`git diff --check`：PASS；
- Port与import boundary单文件ordinary/race/vet：PASS。

## 关联资产

- [Owner-current Port Delta](../../design/harness/port-deltas/committed-pending-action-owner-current-inputs-v2.md)
- [Identity冻结反例矩阵](../../design/harness/assembly/model-tool-call-pending-action-identity-v1-test-matrix.md)
- [Identity实施计划](../../plan/harness/model-tool-call-pending-action-identity-v1.md)
