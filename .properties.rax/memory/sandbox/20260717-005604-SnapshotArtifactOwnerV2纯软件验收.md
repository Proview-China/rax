# Sandbox SnapshotArtifactOwnerV2 owner-local纯软件验收

时间：2026-07-17 00:56:04 +0800

状态：`implementation_software_test_yes / owner-local Reserve-Inspect only`。

## 本次确认

Sandbox控制域独立`SnapshotArtifactOwnerV2`的owner-local Reserve/Inspect最小切片已经落地，
并通过独立纯软件验收。实现范围固定为：

- 无Revision/TTL的stable Subject identity与携Revision/TTL的versioned exact Subject Ref；
- create-once Reservation，以及原子创建Reservation Fact、Entry、Envelope rev1和reserved
  CurrentIndex；
- 公开`ReserveArtifact`、Reservation exact/stable-key Inspect、Aggregate/Entry历史Inspect与
  Aggregate current Inspect；
- append-only测试历史、current pointer复读、Owner clock与state-active TTL校验；
- exact replay、同稳定键内容漂移Conflict、lost reply只Inspect原winner、no-ABA与深拷贝隔离。

公开Port没有raw CAS/Apply、Artifact payload写入、Retention、Deletion、Evidence、Settlement、
Provider或Management方法。support matrix保持`FeatureSnapshotArtifactOwner=false`，避免把
owner-local软件切片冒充production或跨Owner能力。

## 验收证据

在`ExecutionRuntime/sandbox`实际通过：

- `go test -count=100 -shuffle=on ./contract ./kernel ./tests`：PASS；
- `go test -count=20 -race -shuffle=on ./contract ./kernel ./tests`：PASS；
- `go test -count=1 -shuffle=on ./...`：PASS；
- `go test -count=1 -race -shuffle=on ./...`：PASS；
- `go vet ./...`：PASS；
- 无写入gofmt、禁止import、公开写能力、尾随空白与Sandbox限定`git diff --check`：PASS。

覆盖反例包括create-once、64路不同内容单赢家、并发exact replay、lost reply、TTL进入canonical
digest、TTL篡改、`now == expires`、未来Owner clock、current revision漂移、revision回退、同revision
不同digest、typed-nil与历史过期后可Inspect但不可获得current资格。Provider调用为0。

## 继续NO-GO

以下external能力没有因本次软件验收获得授权或实现：

- Retention/Legal Hold Index、Carry、NoActive current proof及其Reader/Application；
- Runtime governed Snapshot purge/delete/cleanup sibling Evidence、Settlement、current Reader、
  Gateway与Sandbox Apply闭包；
- Management最终CurrentIndex/Tombstone DTO、terminal持久化/续期、production root；
- Artifact payload生产写入、production persistence/backend、Provider/Enforcer与SLA。

历史memory中的`implementation-NO-GO`记录保持为当时审计事实，不回改；本事件只把已授权并
已验收的owner-local Reserve/Inspect软件切片推进到`implementation_software_test_yes`。
