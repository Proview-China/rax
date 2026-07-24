# Content Integrity Audit V1 owner-local纵切

时间：2026-07-17 16:52 Asia/Shanghai
状态：`owner-local implemented / diagnostic-only / production cleanup NO-GO`

## 本次完成

- 新增`ContentIntegrityAuditFactV1`、exact Ref、closed Subject/Chunk/Finding/aggregate状态与canonical digest校验。
- 新增`ContentIntegrityAuditGovernancePortV1`与Controller：stable Audit/Idempotency坐标先Inspect收口lost reply；不存在时对bounded Object/Journal逐项执行Manifest/visibility、Journal、Chunk `Has+Get+length/digest` S1/S2。
- 新增内存reference repository、SQLite schema v6 durable repository、lost-reply fake及只读SDK Inspect。
- create-once Fact按`TenantID + ExecutionScopeDigest + AuditID`隔离；same-ID/idempotency exact replay，换请求/内容Conflict，历史不可变且Reader深拷贝。
- closed诊断为`healthy | write_incomplete | metadata_absent | journal_absent | dangling_reference | corrupt_content | indeterminate`；任一S1/S2漂移Fail Closed且零Fact。

## Owner边界

- `healthy`只覆盖请求显式列出的坐标与本次双轮读取，不表示全库无孤儿。
- 本切面不枚举Content keyspace，不形成Checkpoint/Fork/Review跨Owner引用闭包，不推进Journal/Retention/Tombstone，不回收、不删除、不调用remote Provider。
- physical purge、remote archive/blob、生产SLA、Application root、Checkpoint/Restore跨Owner链仍NO-GO。

## 主要反例

- caller Request没有healthy/classification/visibility/Journal state/Chunk state/Cleanup字段。
- key存在但bytes长度/digest错误形成`corrupt_content`；缺Chunk形成`dangling_reference`；backend不可用形成`indeterminate`。
- Manifest/Journal/Chunk S1/S2漂移不封Fact。
- commit回包丢失后只Inspect原Audit ID，不重扫、不换ID。
- 64路不同Finding并发只有一个create赢家，其余Conflict；跨Tenant同ID相互隔离；返回值无alias。
- SDK与Governance Port均无Purge/Delete/Retention/Provider方法。

## 实际验证

以下命令均在`ExecutionRuntime/continuity`执行并PASS：

```text
go test ./contract ./domain ./storage/memory ./storage/sqlite ./sdk ./tests/blackbox ./tests/conformance ./tests/fault -run 'ContentIntegrity' -count=100
go test -race ./contract ./domain ./storage/memory ./storage/sqlite ./sdk ./tests/blackbox ./tests/conformance ./tests/fault -run 'ContentIntegrity' -count=20
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
CGO_ENABLED=0 go test ./... -count=1
go test -tags continuity_rocksdb ./... -count=1
go test -race -tags continuity_rocksdb ./... -count=1
go vet -tags continuity_rocksdb ./...
```

`gofmt`首次在仓库根使用模块相对路径，因工作目录错误返回`stat ... no such file or directory`且未改文件；随后在`ExecutionRuntime/continuity`模块根对本切面文件重跑，exit 0。

## 尚未完成

- Content keyspace enumeration与全库orphan detection；
- 跨Owner引用闭包、Legal Hold/current Reader与受治理physical purge；
- remote Put/Delete/Archive Provider及Operation治理装配；
- Runtime Checkpoint Seal中立DTO与Continuity完整OwnerBinding/ScopeDigest、Participant closure的additive exact映射；
- 跨OwnerCheckpoint、Restore Execute、CLI/API/Application production root与系统测试。
