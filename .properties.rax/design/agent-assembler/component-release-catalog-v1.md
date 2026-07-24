# Component Release Catalog V1

## 1. 必要性

Harness 已能消费 `ComponentManifestV2` 和装配贡献，但多数 6+1 组件尚未发布生产 Describer/Release。若由 Assembler 临时拼 Manifest，会形成第二事实 Owner，并让自定义组件无法安全接入。因此 Component Release 必须由组件 Owner 发布、Catalog 只索引、Assembler 只读。

## 2. ComponentReleaseV1

```text
ComponentReleaseV1
|-- contract_version / release_id / revision / digest
|-- support_mode
|-- component_manifest_v2
|-- module_descriptors[]
|-- capability_descriptors[]
|-- slot_specs[] / slot_contributions[]
|-- port_specs[]
|-- hookfaces[] / phase_contributions[]
|-- dependencies[]
|-- factory_descriptors[]
|-- provider_binding_candidates[]
|-- required_plan_artifacts[]
|-- source_ref / artifact_digest / evidence_refs[]
|-- created_unix_nano / expires_unix_nano
`-- release_digest
```

字段复用 Runtime Binding V2 与 Harness Assembly public contract；不复制类型，不携 factory 实例或 raw Provider handle。

## 3. 发布规则

- Component Owner 对 Release create-once；Catalog Owner 不改写内容。
- Catalog snapshot 固定所有 release exact refs、currentness、governance catalog digest 和 snapshot TTL。
- Artifact/Manifest/Contract/Capability/Schema/Owner/Locality/Residual/Extension 任一漂移产生新 Release revision。
- `production` 必须有独立 certification/conformance evidence；自报、fixture、registration observation 不能升级 support mode。
- unknown required capability/schema/extension fail closed。
- 自定义组件与内建组件走同一发布路径，无特殊信任旁路。

## 4. Owner additive Delta

每个 6+1 Owner 最少新增：

```go
type ComponentReleasePublisherV1 interface {
    EnsureExactComponentReleaseV1(context.Context, ComponentReleaseCandidateV1) (ComponentReleaseV1, error)
}

type ComponentReleaseReaderV1 interface {
    InspectExactComponentReleaseV1(context.Context, ComponentReleaseRefV1) (ComponentReleaseV1, error)
}
```

Publisher 属于组件 Owner 管理面，不注入 Assembler；Assembler 只拿 Catalog Reader。生产发布、签名、存储后端不在本设计中预选。

## 5. 首版 release 集合

至少包含：continuity、tool/mcp、memory/knowledge、sandbox、review、context/cache、harness，以及必要的 model-invoker、runtime/application adapter 发布项。分阶段实现允许，但最终 `SystemReady` 前全部必须 production。
