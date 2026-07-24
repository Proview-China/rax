# History Derivation Candidate V1 owner-local纵切

时间：2026-07-17 17:29 Asia/Shanghai
状态：`owner-local implemented / candidate-only / Compactor and Purge NO-GO`

## 本次完成

- 新增`HistoryDerivationCandidateFactV1`、closed `projection | summary | index` kind、ordered exact Timeline Event refs、exact output `ContentObjectRefV1`、source-set digest与candidate-only Authority。
- 新增`HistoryDerivationCandidateGovernancePortV1`与Controller：stable Candidate/Idempotency坐标先Inspect收口lost reply；不存在时逐Event与output Manifest/visibility/全部Chunk bytes执行S1/S2并重算完整Object digest。
- 新增内存reference repository、SQLite schema v8 durable repository、lost-reply fake、只读SDK Inspect与Conformance capability。
- Fact固定revision 1、create-once、immutable，按`TenantID + ExecutionScopeDigest + CandidateID`隔离；same request exact replay，changed Conflict，Reader深拷贝。
- 抽取`inspectContentObjectExactV1`供Content Delta与History Derivation共用，保持同一Manifest/Chunk/完整Object校验语义。

## Owner与Effect边界

- caller只提交ordered Evidence Record ref及expected Record/Projection digest、output Object ID及expected Manifest digest；不能提交Event payload、current、authority、summary correctness或删除计划。
- Candidate只描述一组immutable Event与既有visible Object之间的候选关系；不证明projection/summary/index语义正确，不成为Timeline current、领域Fact、Memory/Knowledge、Review Verdict或Run终态。
- 不创建或改写output Object，不修改source Event visibility/revision/bytes，不执行Compactor/Indexer/Consolidator，不触发Retention/Purge或Provider。
- production root、算法评估、预算/Review、remote Provider与SLA继续unsupported。

## 主要反例

- source重复、跨Scope、expected Record/Projection digest漂移时零Fact；source order进入canonical digest，换序形成不同内容并被create-once拒绝。
- output不可见、Manifest漂移、缺Chunk、坏Chunk或完整Object digest错误时零Fact。
- Event或output在S1/S2间漂移时`indeterminate`，零Fact。
- same-ID/idempotency换kind/source/order/output Conflict；内存/SQLite 64路不同内容只有一个赢家。
- commit回包丢失只Inspect原Candidate ID，不重读Event/Object、不换ID。
- SQLite v7→v8 additive migration可重开；exact Reader深拷贝，source historical Event保持不变。
- Governance Port和SDK均无publish/current/replace/mutate/compact/delete/purge/provider方法。
- canonical-tamper fuzz覆盖source Record/Projection digest、output digest、Authority、source-set digest与Fact digest；3秒实测69,234次执行，原Fact无alias。

## 实际验证

以下命令均在`ExecutionRuntime/continuity`执行并PASS：

```text
go test ./contract ./domain ./storage/memory ./storage/sqlite ./sdk ./fakes ./tests/blackbox ./tests/fault ./tests/conformance -run 'HistoryDerivation' -count=100
go test -race ./contract ./domain ./storage/memory ./storage/sqlite ./sdk ./fakes ./tests/blackbox ./tests/fault ./tests/conformance -run 'HistoryDerivation' -count=20
go test ./...
go test -race ./...
go vet ./...
CGO_ENABLED=0 go test ./...
go test -tags continuity_rocksdb ./...
go test -race -tags continuity_rocksdb ./...
go vet -tags continuity_rocksdb ./...
go test ./contract -run '^$' -fuzz '^FuzzHistoryDerivationCandidateRejectsCanonicalTamper$' -fuzztime=3s
go test ./... -coverprofile=/tmp/continuity-cover.out
```

覆盖率实测：全模块statement 59.5%；该数字是当前证据，不是已冻结验收目标。

## 尚未完成

- Compactor/Indexer/Consolidator受治理任务、算法证明、资源预算、Attempt/current Projection与真实执行；
- Target Object materialization、online backup、remote archive；
- Content keyspace全库orphan检测、跨Owner引用闭包与受治理Purge；
- Runtime Checkpoint Seal/Participant additive exact DTO映射、跨OwnerCheckpoint与Restore Execute；
- CLI/API/Application production root与长Session/千Agent系统测试。
