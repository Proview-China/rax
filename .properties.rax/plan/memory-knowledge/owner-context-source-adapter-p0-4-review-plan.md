# P0-4 Memory/Knowledge Owner Adapter联合审查计划

> 状态：**historical_review_plan / superseded_by_reference_implementation**。本文保留双Owner Adapter实现前审查门；两个Adapter与nonzero reference fixture现已YES。production G6B/root与远程Retrieval仍未授权。

## 1. 候选资产与Owner隔离

| Adapter | 资产 | 唯一Owner | 额外外部门 |
|---|---|---|---|
| Memory | [Memory Context Source Adapter候选](../../design/memory-engine/context-source-adapter-external-p0-candidate.md) | Memory Adapter Owner；只读Memory V2 | P0-1、P0-2、P0-3、Context nonzero/root |
| Knowledge | [Knowledge Context Source Adapter候选](../../design/knowledge-engine/context-source-adapter-external-p0-candidate.md) | Knowledge Adapter Owner；只读Knowledge V2 | P0-1、P0-2、P0-3、P0-5、Context nonzero/root |

两者不可合并为通用Owner current、不可用类型别名、不可共享Store/Pointer/预算/排序。共同之处只有Runtime Binding exact refs与“stable association + fresh envelope”结构原则。

## 2. 设计Review退出条件

独立审查必须确认：

1. Adapter只引用未来Owner V2 exact refs/digests，没有复制第二套Projection/Item/current；
2. StableAssociation字段、domain/version/ObjectKind/canonical顺序完整，明确排除fresh refs/times/expiry；
3. Envelope包含三个Binding current digests、Generation association、Owner fresh refs、phase/TTL和canonical digest，但没有`Current`/CAS/body；
4. S1/S2只允许fresh refs变化，stable digest和Owner exact集合必须相同；
5. TTL是所有Owner/Runtime/Context上界最小值，两侧fresh clock与Binding reread完整；
6. Memory与Knowledge错误闭集、License/Conflict差异和NO-GO反例完整；
7. P0-3 Application Attempt和Context source request仍是external imported nominal，未被本模块私造；
8. External P0仍为5，首个G6B Memory=0、Knowledge=0。

## 3. 无SCC实施顺序

```text
Owner V2 Go + P0-1 + Runtime Binding Readers
                     |
                     v
        two Owner Adapter implementations（可并行）
                     |
P0-2 + P0-3 + P0-5 --+
                     v
       Context additive source/cardinality
                     |
                     v
      Capability/Binding + production root
                     |
                     v
       G6B exact Frame -> Harness consume
```

Memory/Knowledge Adapter实现可并行；各自必须等待对应Owner V2 Go和P0-1。跨Owner fixture/root严格等待P0-2/P0-3，Knowledge另等待P0-5。Harness不新增直连Port。

## 4. 未来实现文件级落点（未授权）

| Owner | 候选落点 | 内容 |
|---|---|---|
| Memory | `ExecutionRuntime/memory-knowledge/memory/contextadapter/**` | Memory V2 Reader封装、stable/envelope canonical、Binding S1/S2、conformance |
| Knowledge | `ExecutionRuntime/memory-knowledge/knowledge/contextadapter/**` | Knowledge V2 Reader封装、License/Conflict/currentness、Binding S1/S2、conformance |

不得修改Owner domain/kernel/store来适配Application；Adapter层才可依赖Runtime公开`core/ports`。P0-3/Context/Assembly/root由对应Owner另行实现。

## 5. 实现后退出证据（本轮不运行）

每个Adapter独立要求：

- targeted ordinary `-count=100`；
- targeted race `-count=20`；
- 单元、白盒、黑盒、故障注入、Conformance；
- full `go test ./...`、`go test -race ./...`、`go vet ./...`；
- `gofmt -d`、`git diff --check`、import boundary、zero-network/Provider/Resolver扫描；
- Binding S1/S2漂移、TTL crossing、clock rollback、lost reply、copy isolation、64并发反例。

本轮只有Markdown/link/diff/hash轻门，不得宣称上述Go证据已执行。

## 6. 当前裁决

- Owner-local Reader实现：P0=0/P1=0/P2=0保持，live V1/V2属于同一Owner合同族并已完成软件验收；Adapter不得建立第二current或DTO真值。
- Adapter候选：`design_candidate / review_pending`，等待独立审查；不计为P0-4关闭。
- External：P0-1至P0-5仍全部open，**External P0=5**。
- P0-3 Application、Context nonzero source/cardinality、production root继续NO-GO。
