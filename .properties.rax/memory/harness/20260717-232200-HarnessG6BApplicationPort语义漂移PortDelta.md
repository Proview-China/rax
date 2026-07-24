# Harness G6B Application Port语义漂移Port Delta

## 事件

2026-07-17，Harness共用接线按live Application公共候选复核G6B `ContextTurnRefreshPortV1`。Application `contract/ports`当前可以编译，但其DTO与验证逻辑服务Memory/Knowledge source collection，和已冻结N=1 settled Tool action Refresh不是同一语义。

## 当前真值

- 冻结首切面：`Tool=1 / Memory=0 / Knowledge=0 / Continuity=0`；输入必须是settled `ToolResultV2`、current V4 Inspection及经公开Reader验证的完整Association；
- live候选：`ContextTurnRefreshPrepareRequestV1/ApplyRequestV1`要求Memory或Knowledge至少一项非空，缺Tool/V4/Association exact链，首方法名为`PrepareContextTurnRefreshV1`；
- `go test ./contract ./ports`在Application模块PASS只证明当前候选内部可编译，不构成跨Owner语义Conformance；
- Harness已新增[正式Port Delta](../../design/harness/port-deltas/context-turn-refresh-g6b-v1.md)，并同步Action Gateway设计、测试矩阵和计划；当前状态`candidate-p0`；
- Harness没有修改Application、Context、Runtime、Tool或Model代码，没有创建私有兼容DTO/Reader，也没有把G6B、Capability、Continuation或Turn推进标为完成。

## 验证

- `cd ExecutionRuntime/application && go test ./contract ./ports`：PASS；该结果只作为live候选编译证据；
- `cd ExecutionRuntime/harness && go test ./...`：PASS；其中`assemblyadapter` 11.246s；
- `cd ExecutionRuntime/harness && go test -race ./...`：PASS；其中`assemblyadapter` 48.858s；
- `cd ExecutionRuntime/harness && go vet ./...`：PASS；
- Harness本轮只修改自身design/plan/module/memory资产，没有修改Go或测试。

## 后续门

Application与Context Owner须先冻结唯一三段G6B公共Port，完成Tool=1正例、Memory/Knowledge零Reader、exact input漂移、S1/S2、lost-reply、TTL和import DAG Conformance。其后Harness才实现Continuation Adapter与G6B test-only跨模块fixture；production root仍需宿主Owner另行验收。
