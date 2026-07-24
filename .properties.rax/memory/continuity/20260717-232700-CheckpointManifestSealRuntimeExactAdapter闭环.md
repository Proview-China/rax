# Checkpoint Manifest Seal Runtime Exact Adapter闭环

时间：2026-07-17 23:27 CST

## 结论

用户授权的最小Runtime Delta与Continuity只读Adapter已闭合。该切片只把Continuity immutable revision-1 Manifest Seal按完整exact coordinate投影给Runtime Checkpoint Consistency Gateway；不捕获Snapshot、不调用Participant/Provider、不创建Runtime Consistency，也不实现Restore。

## 已实现

- Runtime `CheckpointManifestSealContractVersionV2`升为`2.1.0`，新增完整Continuity OwnerBinding与外部exact ref DTO；raw Owner digest保留用于exact lookup。
- `NormalizeCheckpointExternalSHA256DigestV2`冻结external SHA-256到Runtime `core.Digest`的唯一canonical转换；大写、非SHA-256及非法长度Fail Closed。
- `InspectCheckpointManifestSealRequestV2`绑定expected Runtime Participant Set digest与sorted current Participant closures。
- `DeriveCheckpointParticipantClosureExactRefV2`以完整Provider OwnerBinding、Tenant、Scope、closure ID/revision/digest生成唯一结构化映射；无字符串拼接identity。
- Continuity Manifest/Seal冻结`RuntimeClosureRef`、`RuntimeParticipantSetDigest`、`ContextClosureDigest`与`ArtifactClosureDigest`。Context算法只覆盖exact Generation+normalized Frame refs；Artifact算法只覆盖normalized Memory/Knowledge/Snapshot/Coverage refs。
- `runtimeadapter.CheckpointManifestSealReaderV2`只调用Continuity exact Seal Reader，逐项校验Seal/Manifest、Attempt、Barrier、EffectCut、frozen set、Runtime Participant Set及每个Participant closure映射。
- Runtime Gateway先读current Participant Set/closures再读Seal，并在Consistency CAS前重复S2；任一字段漂移Conflict。

## 反例与恢复

- 完整Owner任一字段、Tenant/Scope、Manifest digest、Participant Set、Participant ID/Owner/digest、Context/Artifact closure任一漂移均拒绝。
- Seal exact Inspect lost reply保持indeterminate；恢复只重复同一exact request，不换Seal ID或Participant坐标。
- typed-nil Reader在backend读取前返回unavailable。
- 历史Seal和Manifest不可覆盖；Adapter无Create/CAS/Capture/Restore/Execute/Provider方法。

## 验证证据

- Continuity targeted ordinary `-count=100`：PASS。
- Runtime targeted ordinary `-count=100`：PASS。
- Continuity targeted `-race -count=20`：PASS。
- Runtime targeted `-race -count=20`：PASS。
- Continuity `go test ./...`、`go test -race ./...`、`go vet ./...`：PASS。
- Runtime `go test ./...`、`go test -race ./...`、`go vet ./...`：PASS。

覆盖包括happy、canonical normalization、delimiter-safe/full Owner drift、跨Tenant/Scope splice、S1/S2 Seal drift、Participant mapping drift、typed-nil、lost Inspect reply、immutable history及NO-GO reflection。

## 当前门禁

仍未解锁：Harness/Application/Participant端到端Checkpoint装配、Checkpoint capture、真实Participant执行、Provider、production root/CLI、Restore Attempt/Eligibility/Stage/Activate，以及任何“外部世界回滚”声明。Partial仍只作诊断。

## 入口

- [Continuity设计入口](../../design/continuity/README.md)
- [公共Port Delta](../../design/continuity/port-delta.md)
- [实施计划](../../plan/continuity/README.md)
- [模块入口](../../module/continuity/README.md)

