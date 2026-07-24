# Agent Assembler V1

`agent-assembler` 是 Definition 与 Runtime/Harness 之间的确定性装配层。它只读取 sealed Definition、Resolution Facts 和 Component Release Catalog，输出：

- sealed `ResolvedAgentPlanV1`；
- Runtime `BindingPlanV2`；
- Harness `AssemblyInputV1`。

## 包入口

| 包 | 责任 |
|---|---|
| `contract` | Release、Catalog、Facts、Resolved Plan、current projection 公共合同与 canonical seal |
| `ports` | Assembler、exact readers、plan repository、Owner release publisher/reader 边界 |
| `resolver` | production release 选择、依赖/能力/版本/locality/credential/artifact 解析和 S1/S2 复读 |
| `mapper` | 生成 Runtime BindingPlan 和 Harness AssemblyInput |
| `repository` | create-once、exact inspect、revisioned current CAS 的 Memory 与单节点 SQLite WAL 实现 |
| `conformance` | Component Release 与三输出交叉绑定检查，不授予 activation/authority/dispatch |

## 最小调用

```go
service, err := resolver.New(factsReader, catalogReader, planRepository, clock)
result, err := service.Resolve(ctx, contract.ResolveRequestV1{
    Definition: definition,
    FactsRef:   factsRef,
    CatalogRef: catalogRef,
})
```

生产 Resolve 要求 Definition 的 6+1 七个核心 requirement 全部解析到 `production`、fully-controlled、residual-free 且有 certification/evidence 的 exact release。Assembler 不启动 Agent、不下载 artifact、不读取 secret、不调用 Provider，也不授予运行权限。

Production Component Release 还必须闭合以下 exact 集合：

- Manifest provided capability；
- provided `CapabilityDescriptorV1`；
- Module capability 与 manifest/artifact projection；
- `ModuleFactoryDescriptorV1.OutputCapability`；
- 由 CapabilityDescriptor owner 绑定的 `PortSpecV1`。

每个 capability 只能出现一次，不允许重复、alias 或集合拼接。远程组件也必须发布 host adapter factory，不能以 remote locality 绕过构造门。`CertificationRef.Digest` 等于 `ComponentReleaseCertificationDigestV1`，从而绑定 Release identity/revision、Manifest、Artifact、Contract、Conformance、构造闭包、证据和有效期；外层 `ReleaseDigest` 再封装 certification ref，避免自引用摘要。

同一个 Catalog snapshot 中，一个 `ReleaseID` 只能出现一个 current revision；历史 revision 可由 Owner repository exact inspect，但不能与 current revision 同时进入可选 Catalog。

## 验证

```bash
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

正向黑盒会把生成的 `AssemblyInputV1` 交给现有 Harness Compiler，验证 Generation、Manifest、Graph 和 Handoff 都可 sealed 产出。

`repository.SQLiteV1`持久化 immutable ResolvedPlan history 与 revisioned current
CAS/history，使用 schema digest、复合外键、row digest 和严格 JSON decode，拒绝
stale CAS 与历史 Plan ABA。它只提供单节点本机 crash durability，不提供 HA、
远程副本或 SLA。Resolution Facts 与 Component Release Catalog 仍只有公开 exact
Reader，没有 Owner Repository 写口；生产部署必须从各自外部 Owner 注入真实 Reader。
