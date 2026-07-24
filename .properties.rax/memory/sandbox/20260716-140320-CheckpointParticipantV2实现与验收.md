# Sandbox Checkpoint Participant V2实现与验收

- 时间：2026-07-16 14:03 CST
- 状态：Checkpoint-first Sandbox Owner切片已实现并通过本地全门验证。
- 范围：仅`ExecutionRuntime/sandbox/**`与Sandbox独占plan/module/memory资产；未修改
  Runtime、Continuity、Harness、Application、其他组件、根配置、Go workspace或CI。

## 已实现

- `contract/checkpoint.go`：phase Reservation、PreviousPhase exact closure、phase Fact、
  pre-admission/pre-prepare/pre-execute current请求/投影、最短TTL与Conformance合同；
- `ports/checkpoint.go`：Sandbox Owner `CheckpointPhaseStore`、逐Owner current source/
  Reader与纯本地Conformance边界；
- `kernel/checkpoint.go`：Reservation create-once、Fact create-once/CAS、lost-reply exact
  Inspect恢复、current独立复读、digest/revision/tenant/phase/TTL漂移fail closed；
- `internal/testkit/checkpoint.go`：线程安全内存Store、lost-reply注入、current fixture与
  local Conformance Fake；
- 状态机：`prepared -> commit XOR abort`；`failed -> incomplete`、
  `not_applied -> confirmed_not_applied`且均无后继；`unknown`只能Inspect/
  Reconcile并收敛为`indeterminate`；旧revision不能ABA复活。

## Owner与NO-GO

- Runtime仍唯一拥有CheckpointAttempt、Barrier、Effect Cut、Checkpoint资格、
  consistent/restore eligibility、Lease/Fence/Instance与Runtime Settlement；
- Continuity仍只拥有Manifest/RestorePlan Fact；Sandbox没有ManifestSeal写权；
- Sandbox本轮只拥有Participant Reservation/current projection/phase Fact；
- `SnapshotArtifactOwnerV2`、Compatibility、Runtime/Continuity Adapter、公共Assembly、
  Provider Prepare/Execute、Restore、production Backend/root均未实现；
- 生产代码没有Provider Port或调用路径，缺任一治理门时Provider调用固定为0；testkit
  仅为Fake，不是生产State Plane或Conformance证明。

## 实际验证

在`/home/proview/Desktop/property/praxis/ExecutionRuntime/sandbox`执行并通过：

```text
go test -count=100 -shuffle=on ./contract ./kernel ./tests -run 'Checkpoint'
go test -count=20 -race -shuffle=on ./contract ./kernel ./tests -run 'Checkpoint'
go test -count=1 -shuffle=on ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
```

定向反例覆盖TTL/digest篡改与边界、typed-nil、缺gate、current drift/expiry、64路并发
commit/abort、Reservation/Fact/CAS lost reply、no-ABA、历史过期终态可Inspect但不可新用、
本地Conformance ProviderCalls=0与production proof拒绝。最终gofmt、tracked/new-file
diff-check、trailing-whitespace、禁止跨Owner import与`go list`依赖扫描均已PASS。
