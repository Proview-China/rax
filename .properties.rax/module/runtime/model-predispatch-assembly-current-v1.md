# Runtime Model Pre-Dispatch Assembly Current V1模块说明

状态：**Surface neutral Go双独立代码审计YES（P0/P1/P2=0/0/0）；Runtime neutral ports完成**。

## 1. 作用

该纵切由Runtime ports提供唯一中立Go类型，供Harness发布Assembly current投影、Model无损携带Registry与Assembly坐标、Tool直接复用同一投影，避免Harness与Tool互相导入实现包或形成alias/echo。它不把Surface、Prepared、compile HookFace或Runtime Effect解释为Provider gate。

## 2. 组成

- `ports/model_predispatch_assembly_current_v1.go`：五个neutral DTO、`RegistrySnapshotExactReaderV1`、`ModelPreDispatchAssemblyCurrentReaderV1`、Validate/canonical/digest/JSON shape；
- `conformance/model_predispatch_assembly_current_v1.go`：只依赖public Reader的exact current、typed-nil与无写权报告；
- `tests/ports/model_predispatch_assembly_current_v1_test.go`：shape、method set、canonical、all-ref drift、TTL/currentness、closed error、typed-nil和import-boundary反例。

Runtime是Go type owner；Registry Authority Owner拥有Registry事实与current pointer，Harness Assembly Owner拥有Assembly publisher、revision CAS、current index和Reader实现。

## 3. 验证

双独立代码审计结论均为YES（P0/P1/P2=0/0/0）。验证覆盖target ordinary `count=100`、race `count=20`、Runtime full ordinary/race、`go vet`、gofmt、diff-check与import-boundary，结果全部PASS。

## 4. 限制

本纵切不实现Harness Store/publisher/CAS、Registry production repository、Model/Harness/Tool适配或system/production composition root，不选择backend、数据库、RPC、durability、availability或SLA。public Conformance的`ProductionClaimEligible=false`保持不变。

设计入口：[Model Pre-Dispatch Assembly Current V1](../../design/runtime/model-predispatch-assembly-current-v1/README.md)。
