# Model Pre-Dispatch Assembly Current V1 Port Delta

状态：**七个public declarations、两个窄Reader、Validate/canonical/digest与public Conformance已实现；双独立代码审计YES（P0/P1/P2=0/0/0），全门PASS**。

## 1. 已实现公共面

`ExecutionRuntime/runtime/ports`已实现且冻结的public declarations为七项（五个DTO、两个Reader）：

- `RegistrySnapshotRefV1`；
- `RegistrySnapshotExactReaderV1`；
- `ModelPreDispatchAssemblyExactRefV1`；
- `ModelPreDispatchAssemblyBindingSetRefV1`；
- `ModelPreDispatchAssemblyCurrentRefV1`；
- `ModelPreDispatchAssemblyCurrentProjectionV1`；
- `ModelPreDispatchAssemblyCurrentReaderV1`。

精确字段见[contracts](./contracts.md)。Current Ref本身完整携带ToolSurface、Profile、Registry、Semantic、Currentness、Checked/Expires和ProjectionDigest，不能仅靠外层Projection补齐。Runtime ports只拥有Go nominal形状、Validate/canonical约束和Reader interface；Harness Assembly Owner拥有事实发布、revision/current index、Reader实现和Assembly语义。

`RegistrySnapshotExactReaderV1`由`RegistrySnapshotRefV1.Owner`指向的Registry Authority Owner实现；Runtime ports只拥有接口形状。它按完整Ref读取、验证historical immutable record与Owner current pointer、返回deep clone，不提供Registry Store或写能力。

## 2. 精确Reader签名

```go
type RegistrySnapshotExactReaderV1 interface {
    InspectExactRegistrySnapshotV1(
        context.Context,
        RegistrySnapshotRefV1,
    ) (RegistrySnapshotRefV1, error)
}

type ModelPreDispatchAssemblyCurrentReaderV1 interface {
    InspectCurrentModelPreDispatchAssemblyV1(
        context.Context,
        ModelPreDispatchAssemblyCurrentRefV1,
    ) (ModelPreDispatchAssemblyCurrentProjectionV1, error)
}
```

Reader keyed by exact Ref，不提供latest、list、discovery、mutation、publish或CAS。消费者必须通过composition获得已选定的exact Ref；不存在自由发现路径。

## 3. Consumer约束

- Harness publisher直接产出Runtime public Ref/Projection，不定义Harness alias；
- Model只无损携带完整Projection或exact Ref，不生成新ID/digest；
- Tool直接嵌入Runtime public Projection，不定义Tool echo、alias、generic ObjectRef或map；
- Runtime Gateway未来只做exact current复读，不持有Surface/Prepared事实，不把本Projection当Provider gate。

## 4. 兼容与迁移

本Delta是additive实现，不改`GenerationArtifactRefV1`、AssemblyInput V1字段或digest。三方旧Registry局部命名必须在各Owner后续适配时迁移到唯一`RegistrySnapshotRefV1`，而不是形成alias；Runtime实现未修改Harness资产或Go。

## 5. 实施阻塞

Runtime neutral DTO、Registry exact Reader、Assembly Current Reader、Validate/canonical/digest、public-only Conformance和Runtime import-boundary测试已经落地。后续仍需Registry Authority真实Reader、Harness发布/CAS/current Reader、Model无损carrier、Tool无echo适配分别获得各Owner授权；当前无production composition root/backend/durability/SLA。
