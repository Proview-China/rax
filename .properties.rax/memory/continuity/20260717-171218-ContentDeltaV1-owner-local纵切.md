# Content Delta V1 owner-local纵切

时间：2026-07-17 17:12 Asia/Shanghai
状态：`owner-local implemented / relation-only / Compaction and Purge NO-GO`

## 本次完成

- 新增`ContentObjectRefV1`，exact绑定Object/Manifest/content digest、schema、length、compression、encryption、classification、Owner、Scope与Retention Policy。
- 新增`ContentDeltaFactV1`、exact Ref、ordered target recipe与normalized reused/added/removed Chunk集合；reuse identity固定为`schema + digest + length`。
- 新增`ContentDeltaGovernancePortV1`与Controller：stable Delta/Idempotency坐标先Inspect收口lost reply；不存在时对Base/Target执行Manifest/visibility/全部Chunk bytes S1/S2并重算完整Object digest。
- 新增内存reference repository、SQLite schema v7 durable repository、lost-reply fake、只读SDK Inspect与Conformance capability。
- Fact固定revision 1、create-once、immutable，按`TenantID + ExecutionScopeDigest + DeltaID`隔离；same request exact replay，changed Conflict，Reader深拷贝。

## Owner与Effect边界

- caller只能提交Base/Target Object ID与expected Manifest digest，不能提交Chunk列表、reuse bool、bytes统计、payload或预制Fact。
- Delta只描述两个已存在、已可见Object的结构共享关系；不创建Target、不materialize patch、不执行Compactor/Indexer/Consolidator、不修改visibility/current/Retention、不删除base或Chunk。
- 相同Chunk关系不授予跨Policy读取、解密、Retention或Purge权。
- remote object Provider、physical purge、跨Ownerroot与生产SLA继续unsupported。

## 主要反例

- digest相同但schema/length不同不得reuse；
- Base/Target不可见、跨Scope、expected Manifest漂移、缺Chunk、坏Chunk或完整Object digest错误时零Fact；
- S1/S2漂移Fail Closed；
- same-ID/idempotency换内容Conflict，64路不同Fact只有一个赢家；
- commit回包丢失只Inspect原Delta ID，不重读Base/Target、不换ID；
- 内存/SQLite exact Reader无alias，SQLite schema 6→7 additive migration可重开。

## 实际验证

以下命令均在`ExecutionRuntime/continuity`执行并PASS：

```text
go test ./contract ./domain ./storage/memory ./storage/sqlite ./sdk ./fakes ./tests/blackbox ./tests/conformance ./tests/fault -run 'ContentDelta' -count=100
go test -race ./contract ./domain ./storage/memory ./storage/sqlite ./sdk ./fakes ./tests/blackbox ./tests/conformance ./tests/fault -run 'ContentDelta' -count=20
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
CGO_ENABLED=0 go test ./... -count=1
go test -tags continuity_rocksdb ./... -count=1
go test -race -tags continuity_rocksdb ./... -count=1
go vet -tags continuity_rocksdb ./...
```

## 尚未完成

- Compactor/Indexer/Consolidator派生任务、调度、Attempt与current Projection；
- Target Object materialization、online backup、remote archive；
- Content keyspace全库orphan检测、跨Owner引用闭包与受治理Purge；
- Runtime Checkpoint Seal/Participant additive exact DTO映射、跨OwnerCheckpoint与Restore Execute；
- CLI/API/Application production root与长Session/千Agent系统测试。
