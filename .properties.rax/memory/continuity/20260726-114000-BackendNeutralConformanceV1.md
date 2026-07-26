# Backend-neutral Conformance V1

## 粗粒度进展

Continuity既有plan中的`conformance/backend.go`已落地为公开`RunBackendSuiteV1`。替代Backend开发者可以提供隔离namespace与现有：

- `ports.MetadataStore`
- `ports.ContentStore`
- `ports.RetentionStore`

suite会真实写入隔离测试对象，并验证11项既有不变量：

1. Content missing/Put/Get；
2. Content input/output clone；
3. same Chunk ref换bytes拒绝；
4. Manifest stage/Inspect clone；
5. same Object ID换Manifest拒绝、wrong digest commit拒绝及visible；
6. Journal create/Inspect；
7. same Journal ID完整identity/content drift拒绝；
8. Journal CAS与stale拒绝；
9. Journal 32路CAS精确单赢家；
10. Retention create/Inspect与changed create拒绝；
11. Retention CAS与stale拒绝。

公开报告固定`ReferenceOnly=true`、`ProductionEligible=false`，检查集合为exact闭集，clone不暴露alias。Wave1 Manifest新增`continuity/backend-conformance-suite-v1-reference`，但不因此提升整体Conformance等级。

## 修复的owner-local漂移

memory reference Backend过去对同Journal ID的create replay只比较`ObjectID + ManifestDigest`，换`ObjectDigest`也会静默成功。现已收紧为完整`WriteJournal`精确比较：完整相同才幂等，任一identity/state/revision/time/residual漂移均`revision_conflict`。SQLite原有完整body digest行为不变。

## 已覆盖装配

- memory reference Backend：Metadata+Content+Retention；
- SQLite schema v9：Metadata+Retention，搭配reference Content；
- RocksDB build tag：Content，搭配reference Metadata+Retention；
- Go Example提供最小可调用示例。

## 硬边界

suite需要调用方提供fresh隔离namespace并会写测试对象。它不检查或认证真实Participant、remote Provider、跨存储部署拓扑、KMS/Secret、backup/restore、remote durability、root、SLA或production readiness；Fake/reference green不能成为production证明。

## 实际验证

- memory、SQLite+reference content与RocksDB content定向suite：`count=100` PASS；
- 同一矩阵：`race count=20` PASS；
- `go test -count=1 ./...`：PASS；
- `go test -race -count=1 ./...`：PASS；
- `go vet ./...`：PASS；
- `go test -count=1 -tags continuity_rocksdb ./...`：PASS；
- `go test -race -count=1 -tags continuity_rocksdb ./...`：PASS；
- `go vet -tags continuity_rocksdb ./...`：PASS；
- `ExampleRunBackendSuiteV1`真实执行输出：`true true false 11`，依次表示无错误、reference-only、非production eligible、11项检查。
