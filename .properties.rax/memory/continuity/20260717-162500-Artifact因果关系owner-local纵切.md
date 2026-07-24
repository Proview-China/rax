# Artifact因果关系owner-local纵切

时间：2026-07-17 16:25（Asia/Shanghai）

## 已完成

- 新增`ArtifactRefV1`与`ArtifactRelationSourceProjectionV1`。Artifact Fact、storage、parent revision和origin Evidence均由typed Artifact Owner Reader密封；Continuity不拥有Artifact正文、revision/current或storage语义。
- 新增coordinate-only `CreateArtifactRelationRequestV1`。caller只能请求Relation/Idempotency、Scope、Artifact/Related exact refs、closed relation kind、Evidence Record和可选expected source projection坐标，不能自报storage/parent/digest/current/Trust/Verdict/Outcome。
- 新增`ArtifactRelationControllerV1`：lost-reply先按稳定Relation ID Inspect；首次创建固定执行Timeline Event+typed Owner source projection S1、逐字段exact比较、S2完整复读、canonical一致、create immutable revision-1 Fact。
- 新增结构化Tenant/Scope/Relation ID隔离、same-ID/idempotency exact重放、changed Conflict、Artifact/Related exact索引、深拷贝和64路不同内容单赢家。
- 新增内存reference repository与SQLite schema v5持久化；SQLite重开后Relation、Timeline origin和两个索引可精确Inspect。
- 只读SDK新增Inspect/List Artifact Relations；不新增`AttachArtifact`或Fact Store写面。
- Conformance声明为`continuity/artifact-relation-governance-v1-reference`。真实typed Artifact Owner Router、Application/Assembler route和production root仍为NO-GO。

## 实际验证

- `go test ./contract ./domain ./storage/memory ./storage/sqlite ./sdk ./tests/blackbox ./tests/conformance ./tests/fault -run 'ArtifactRelation|ArtifactRef' -count=100`：PASS。
- `go test -race ./contract ./domain ./storage/memory ./storage/sqlite ./sdk ./tests/blackbox ./tests/conformance ./tests/fault -run 'ArtifactRelation|ArtifactRef' -count=20`：PASS。
- `go test ./... -count=1`：PASS。
- `go test -race ./... -count=1`：PASS。
- `go vet ./...`：PASS。
- `CGO_ENABLED=0 go test ./... -count=1`：PASS。
- `go test -tags continuity_rocksdb ./... -count=1`：PASS。
- `go test -race -tags continuity_rocksdb ./... -count=1`：PASS。
- `go vet -tags continuity_rocksdb ./...`：PASS。

覆盖的关键反例：caller可信字段注入、Artifact/Related/Evidence/Storage/Parent/source projection任一S1/S2漂移、Owner route错绑、Timeline缺少关系坐标、跨Tenant/Parent splice、same-ID换内容、commit回包丢失、typed-nil、未分类Reader错误、SQLite重开、深拷贝与并发单赢家。

## 未解锁

- 真实Artifact Owner Adapter、Application/Assembler route、production `AttachArtifact`。
- Artifact current、正文、Review Verdict、Tool Result、Effect Outcome或其他领域Owner事实写权。
- Checkpoint跨Owner接线、Restore Execute、remote Provider、CLI/API写面和生产SLA。
