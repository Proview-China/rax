# Workspace Snapshot Artifact available与Host Local加密存储

时间：2026-07-18 10:42 CST

## 结论

用户确认`Workspace Snapshot + Host Local`后，Sandbox Owner完成首个capture owner-local切片：

- `SnapshotStorageArtifactRefV2`与`SnapshotArtifactFactV2`完整canonical/exact合同；
- `ReserveArtifact -> CommitArtifact`把aggregate从`reserved`推进为`available`；
- Owner在CAS前执行同一sealed current projection的S1/S2复读；
- Artifact Fact、Artifact Entry、Envelope rev2与CurrentIndex rev2在同一Store线性化边界提交；
- old Envelope/Entry保持历史可读，lost reply只Inspect原winner；
- Host Local Data Plane实现AES-256-GCM加密、content-addressed create-once单文件envelope、
  fsync与exact Inspect，公共Storage Ref不含path/key/credential。

Checkpoint Participant生产phase、Application Coordinator接线、Continuity Manifest消费、Restore、
Purge/Delete、remote backend与deployment SLA仍为NO-GO；`FeatureSnapshotArtifactOwner=false`保持不变。

## 验证证据

在`ExecutionRuntime/sandbox`实际通过：

```text
go test ./contract ./kernel ./tests -run 'SnapshotArtifact' -count=100
go test -race ./dataplaneadapter/hostlocal ./contract ./kernel ./tests -run '(SnapshotArtifact|StoreV2)' -count=20
go test ./...
go test -race ./...
go vet ./...
```

覆盖canonical digest/TTL篡改、raw locator缺失、S1/S2漂移零写、same-base不同Artifact
Conflict、64路单赢家、lost reply recovery、clone/no-alias、密文篡改、exact expiry与并发create-once。

Checkpoint capture复用live Runtime/Application exact ref；本切片未修改Runtime schema。
