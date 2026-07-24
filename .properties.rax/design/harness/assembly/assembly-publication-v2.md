# Harness Assembly Publication V2

## 1. 状态与范围

- 状态：H4 已获用户确认；Harness Owner P0 合同与 reference backend 已实现。
- 目标：把 `AssemblyGenerationV1`、`AssemblyManifestV1`、`CompiledHarnessGraphV1`、`AssemblyHandoffV1` 作为一个完整发布闭包原子可见。
- 保留 `Compiler.Compile` / `CompileHarnessV1` owner-local 语义；本模块不做 Runtime Binding、Activation、Agent 启动或 production composition root。
- `MemoryStoreV2` 只证明合同与并发语义，不是 durable production State Plane。

## 2. 公共合同

```go
CompileAndPublishAssemblyV2(ctx, CompileAndPublishAssemblyRequestV2) (CompileAndPublishAssemblyResultV2, error)
EnsureAssemblyPublicationV2(ctx, CompileAndPublishAssemblyRequestV2) (CompileAndPublishAssemblyResultV2, error)
InspectAssemblyPublicationHistoricalV2(ctx, AssemblyPublicationRefV2) (AssemblyPublicationBundleV2, error)
InspectAssemblyPublicationCurrentV2(ctx, scopeRef string) (AssemblyPublicationCurrentV2, error)
```

`PublicationID = Derive(InputDigest, GenerationID)`。current 以现有 `AssemblyInputV1.ScopeRef` 分区；调用方必须提供 expected current revision+digest，初始发布使用显式 absent expectation。

## 3. 单一可见屏障

```text
Compile immutable V1 artifact set
  -> stage Generation       (consumer不可读)
  -> stage Manifest         (consumer不可读)
  -> stage Graph            (consumer不可读)
  -> stage Handoff          (consumer不可读)
  -> expected current CAS
       atomic: historical commit marker + scope current
  -> Historical / Current Readers可读
```

- Historical Reader 与 Current Reader 是分离Port；Historical只接受完整create-once Publication Ref。
- staged row无论写入多少都不属于historical/current；commit前所有consumer读取均为NotFound。
- publication content create-once：同ID同staged内容幂等，同ID不同内容Conflict。
- current revision只单调增加；predecessor revision或digest不匹配始终Conflict，即使desired内容相同。
- successor input必须以`PreviousGenerationRef`精确绑定current Generation；阻断ABA。
- commit同时发布historical marker与current；backend必须使用单事务或等价原子commit marker。

## 4. 恢复与currentness

- 每个staged写回包未知，只Inspect同PublicationID的owner-private staged digest，不改ID、不换内容。
- commit回包未知，只Inspect该Publication原子保存的exact committed-current，并核对Publication、AttemptID及完整digest；不得再次CAS。即使Scope latest current随后推进，原commit仍可恢复。
- Historical对象永久不可变；Current Reader使用fresh clock验证`Checked <= now < Expires`，时钟回退和TTL跨越Fail Closed。
- 已过期current仍可作为exact predecessor发布新current，但不能由Current Reader对consumer返回为可用current。
- caller在进程崩溃后只能用exact Historical/Current Reader恢复；用旧expected重新调用publish必须Conflict。

## 5. 文件

```text
ExecutionRuntime/harness/
|-- assemblycontract/publication_v2.go
`-- assemblypublication/
    |-- ports.go
    |-- publisher.go
    |-- memory_store.go
    |-- clone.go
    `-- publisher_internal_test.go
```

验收矩阵见 [Assembly Publication V2测试矩阵](assembly-publication-v2-test-matrix.md)。
