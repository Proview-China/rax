# Model Context Authoritative Owner Binding V1 模块说明

本模块是Context authoritative Owner到Model中立Owner之间的只读合同边界。

它包含原始Owner DTO、Material exact lookup、带观察窗口的Request、完整
Material/Frame lineage Projection、canonical映射/摘要以及单一Reader Port。
`ContextOwnerIdentityDigest`保留完整`ComponentID + BindingDigest`，中立Owner固定
为`{Domain: praxis.context, ID: identity digest}`。

模块不读取或写入Context存储，不实现Reader，不调用Provider，不改变Invocation
Material wire/digest，也不提供Authorization、Permit或dispatch。Context Kind由调用方
在Context公共projection边界完成验证后逐字段传入。

验证入口：

```text
cd ExecutionRuntime/model-invoker
go test -count=100 ./tests/contextownerbindingv1
go test -race -count=20 ./tests/contextownerbindingv1
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

当前限制：Context adapter、消费者typed-nil防护和S1/S2总装尚未落地，因此
production为`NO-GO`。
