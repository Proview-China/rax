# Context Owner Binding Adapter V1 模块说明

## 作用

该模块把Context Owner权威的模型输入Material/Frame current lineage，转换为Model
Invoker可消费的owner-neutral只读projection。它解决的是Context原始Owner完整
provenance和Model neutral Owner之间的适配，不产生调用授权或dispatch资格。

## 组成

- `modelinvokeradapter.InvocationContextOwnerBindingAdapterV1`；
- 构造器接收Context `OwnerRef`、窄
  `ContextModelInputLineageCurrentReaderV1`与clock；
- 实现Model
  `InvocationContextOwnerBindingReaderV1`；
- blackbox/failure/concurrency/import-boundary测试。

## 输入与输出

输入为Model sealed `InvocationContextOwnerBindingRequestV1`。其中Material Kind必须
等于Context公开`praxis.context/model-input-material`，其
`ID/Revision/Digest`被逐字段转换为Context exact source。

输出为Model sealed `InvocationContextOwnerBindingProjectionV1`，包含：

- 完整Context Owner `{ComponentID, BindingDigest}`；
- Model公开规则派生的identity digest和neutral Owner；
- 无损转换的Material/Frame exact sources；
- 直接来自Context authoritative projection的lineage digest；
- fresh Checked与不延长任一上游窗口的Expires。

Owner、Material、Frame与Lineage的角色由字段位置、Kind和完整exact coordinate区分，
不要求跨domain digest值全不相等。特别是`Owner.BindingDigest`与Material digest相等
属于合法输入，适配器必须原样保留。

## 运行与验证

模块没有独立进程、配置或Store。由Context composition注入现有lineage窄Reader：

```text
cd ExecutionRuntime/context-engine
go test -count=100 ./modelinvokeradapter
go test -race -count=20 ./modelinvokeradapter
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go mod tidy -diff
go mod verify
```

测试覆盖ordinary、完整字段漂移、Material/Frame互换、S1/S2 drift、
history-only/current=false、TTL crossing、`now == expiry`、clock rollback、
typed nil/unavailable/cancel/Unknown、64并发稳定与flip窗口，以及production import
boundary。

当前验收已通过：

- focused ordinary `-count=100`；
- focused race `-count=20`；
- Context Engine full ordinary与full race；
- `go vet ./...`、`go mod tidy -diff`、`go mod verify`；
- gofmt、`git diff --check`与import boundary。

## 当前限制

本模块不写Context/Model事实，不调用Provider/Harness/Tool，不提供production
composition root。Context Frame durable current reader已由独立PR #57合并，并可通过
现有lineage窄Reader与本adapter共存；testfixture仍只用于owner-local/reference验证，
不得冒充production composition。RouteCall lowering与Harness production composition
尚未落地，因此production dispatch继续`NO-GO`。
