# Operation Review Authorization V5 实施计划

## 1. 目标

以 [`operation-review-authorization-v5`](../../design/runtime/operation-review-authorization-v5/README.md) 为唯一增量合同，为 Human multi-sign V2 quorum与独立PolicyNotRequired提供Runtime current authorization；不修改V4语义，不实现Permit/Begin/Provider。

## 2. 依赖DAG

```text
Review Human Multi-Sign V2 current Reader ----+
Review BypassDecision + Policy current -------+--> Runtime V5 ports/kernel/SQLite/conformance
Organization/Authority current ---------------+                 |
Binding/Scope/Evidence current ----------------+                 +--> agent-host root
Operation Effect/Governance/Fence V3/V4 -------------------------+
```

无SCC：Runtime不import Review；Review adapter只import runtime public ports；Application/Harness只消费Runtime public current；root最后注入。

## 3. 文件级落点

| 阶段 | 文件 | 产物 |
|---|---|---|
| P1 public contract | `ExecutionRuntime/runtime/ports/operation_review_authorization_v5.go` | neutral exact refs、quorum/not-required union、Validate/Seal/Digest、Fact/Reader/Governance Ports |
| P2 Owner kernel | `ExecutionRuntime/runtime/kernel/operation_review_authorization_v5.go` | S1/S2、fresh clock、create-once、current Inspect、terminal CAS、lost-reply recovery |
| P3 memory/fake | `ExecutionRuntime/runtime/fakes/operation_review_authorization_v5.go` | 仅test/reference，不宣称production |
| P4 production state | `ExecutionRuntime/runtime/storage/sqlite/operation_review_authorization_v5.go` | append-only history、current index、V4/V5 shared effect guard、restart/atomic CAS |
| P5 conformance | `ExecutionRuntime/runtime/conformance/operation_review_authorization_v5.go` | reusable ports/kernel/store suite |
| P6 Review adapters | `ExecutionRuntime/review/runtimeadapter/reader_v5.go`、`bypass_current_v1.go` | Human quorum/Bypass read-only neutral projection |
| P7 integration | `ExecutionRuntime/runtime/tests/**/operation_review_authorization_v5_test.go`、`ExecutionRuntime/review/tests/runtimeintegration/reader_v5_test.go` | exact cross-owner integration |
| P8 host | `ExecutionRuntime/agent-host/**` | V4/V5 explicit route和唯一production composition root |

## 4. 验收矩阵

| 类别 | 必须覆盖 |
|---|---|
| unit | canonical golden、union、quorum、not-required、TTL min、transition |
| whitebox | S1/S2顺序、fresh clock、exact reread、V4/V5 shared guard、zero staged leak |
| blackbox | accepted quorum可创建current V5；PolicyNotRequired无Verdict；所有漂移Fail Closed |
| fault | create/inspect/CAS lost reply、ctx cancel/deadline、Unavailable/Indeterminate、restart |
| conformance | FactPort、Gateway、Reader、SQLite history/current、deep clone、closed errors |
| concurrency | 同ID same canonical幂等；changed content Conflict；同Effect V4/V5 64并发仅一个current |
| integration | Review quorum/Bypass + Runtime Effect/Governance/Fence + SQLite + host injection |
| system | Gate等待/恢复、Permit前current重验、actual-point二次门禁、无Provider调用越界 |

重复门：targeted `-count=100`、targeted `-race -count=20`、module full ordinary/race/vet、gofmt、diff/import扫描。

## 5. 完成判据

- public shapes与design逐字段一致，V4 hash/行为不变；
- quorum不被Runtime重新聚合，not-required无accepted Verdict/Reviewer Attestation；
- SQLite restart后historical exact可读、current index与shared effect guard一致；
- unknown只Inspect，clock rollback/TTL crossing zero write；
- `agent-host`以真实production readers注入，无fake、无第二root；
- Review/Application/Harness/Runtime module/memory同步真实支持与unsupported；
- 独立复审P0/P1/P2=0后才宣称production GO。
