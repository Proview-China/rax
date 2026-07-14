# Binding Manifest V2 模块说明

Binding Manifest V2解决组件注册信息不确定、Capability自报越权、Manifest漂移以及用户自定义组件无法安全接入的问题。

主要入口：

- `ports.ComponentManifestV2`：确定化组件治理Envelope；
- `ports.ComponentRegistryV2`：Register与Probe使用不同Observation类型，均不产生Grant；
- `control.BindingFactPortV2`：权威Binding事实Port；
- `control.BuildBindingSetV2`：验证治理Catalog、Version Range、Grant和依赖DAG；
- `ports.SealBindingPlanV2` / `BindingPlanDigestV2`：从完整Requirement集合派生Plan身份，拒绝同摘要换Kind、Artifact、Contract或Capability；
- `control.ValidateBindingSetCurrentV2`：验证TTL、Fact revision及Manifest Probe漂移；
- `conformance.CheckBindingAdapterV2`：统一验证四档Conformance，但不授予Dispatch权限；
- `conformance.CheckAdapterRuntimeImportsV2`：构建/Conformance依赖门禁，只允许Adapter导入`runtime/core`与`runtime/ports`；
- `fakes.BindingStoreV2`：供合同、并发和恢复测试使用的内存Fact Owner。

兼容入口`ports.AdaptV1DescriptorToManifestV2`只会产生`restricted_controlled`、带Residual的Manifest，不会把旧Descriptor自动提升为V2认证或绑定。

验证命令位于`ExecutionRuntime/runtime`：

```text
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
```

Import扫描通过、Manifest注册或Conformance通过均不授予Binding、production或Dispatch资格。本模块不提供生产存储、远程协议或Dispatch Gateway。
