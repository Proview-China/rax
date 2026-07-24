# Sandbox Workspace、Checkpoint、Snapshot 与 Restore

状态：`implemented / terminal retention-purge external`。

## 1. Workspace

```text
WorkspaceView
  -> Overlay
  -> S1/S2 tree digest
  -> WorkspaceChangeSet + content-addressed blobs
  -> Review/Auth/Permit
  -> prepare/execute Enforcement
  -> Provider commit
  -> independent host revision Inspect
  -> DomainResult/Settlement/Apply
```

View 必须绑定 Base Revision、只读/可写/隐藏范围、symlink/mount policy、Secret/Network/Process
Scope、Lease/Policy/Capability refs 与 TTL。路径只存在于 trusted local binding，不进入公共 DTO。
Base/host drift、symlink escape、hidden path、lost reply 均 fail closed。

## 2. Checkpoint Owner边界

- Runtime：CheckpointAttempt、Barrier、Effect Cut、consistent、restore eligibility；
- Continuity：Manifest、RestorePlan；
- Sandbox：Participant phase、Compatibility、Workspace/Snapshot/Residual facts；
- Provider：local attempt 与 Observation/Receipt。

每个 prepare/commit/abort 有独立 Operation/Effect/Attempt/Permit、prepare Enforcement、
execute Enforcement、Evidence、DomainResult、Settlement 与 Apply。

```text
prepare.apply_settled(prepared)
  -> commit XOR abort

prepare.apply_settled(failed)
  -> incomplete

prepare.apply_settled(not_applied)
  -> confirmed_not_applied

prepare.unknown
  -> Inspect/Reconcile original Attempt
  -> prepared | failed | not_applied | indeterminate
```

同一 prepared closure 只能选择一个后继。lost reply 按 closure key Inspect；不得重开
prepare、切分支、换 Attempt/Provider 或产生 ABA。

## 3. Snapshot capture

当前 live 内容链：

```text
Rust Provider checkpoint staging/current record
  -> Go host-local S1 artifact record
  -> S1 content manifest
  -> canonical WorkspaceSnapshotBundle
  -> S2 record/content reread
  -> AES-256-GCM create-once content store
  -> SnapshotArtifact Reservation
  -> Owner current S1/S2
  -> immutable Artifact Fact/Entry/Envelope
  -> CurrentIndex reserved -> available
```

manifest 绑定目录、文件内容摘要与 executable bit；symlink/special file、digest/length/TTL
漂移均拒绝。Storage Ref 不携 path/key/credential，且与 Artifact Fact Ref 使用不同 type/digest
domain。

Checkpoint 能力分级：

| capability | 恢复范围 |
|---|---|
| `metadata_only` | 诊断/重新装配 |
| `workspace_snapshot` | 文件工作集 |
| `environment_snapshot` | 仅由具体 Backend+artifact+Compatibility 证明 |
| `unsupported` | 明确拒绝 |

Backend 名称或“snapshot exists”不自动授予 environment restore。

## 4. Restore

```text
Runtime typed RestoreAttempt + eligibility current
  -> Snapshot Fact/current/content exact reread
  -> Compatibility + Authority/Review/Budget/Scope
  -> fresh Instance / higher epoch / new Lease / new Fence
  -> prepare Enforcement
  -> Provider Prepare
  -> execute Enforcement
  -> host-local stage canonical bundle
  -> Inspect/Evidence
  -> Restore Stage DomainResult
  -> Runtime Settlement
  -> Sandbox ApplySettlement
```

Restore 只接受新目标 binding。旧 Instance/Lease 是 source provenance，不是执行资格。外部邮件、
网络、交易、数据库写和已提交 Workspace Revision 不回滚；Residual/Effect watermark 必须携带到
新执行状态。

## 5. Cleanup

```text
cancel (optional, independent)
  -> close
  -> fence
  -> release
  -> cleanup
  -> seven-dimension Inspect
  -> Cleanup DomainResult/Settlement/Apply
```

七维为 process、mount、network、secret、background、remote continuation、provider retention。
Release/进程死亡/Provider NotFound 不证明 clean。Residual/indeterminate 持续占用 tenant-stable
conflict domain。

## 6. Snapshot terminal外部门

`reserved -> available`已实现；terminal retention/delete/purge 尚需：

1. Retention/Legal Hold Owner 的 exact Index/Carry/NoActive current proof；
2. Runtime-owned neutral purge/cleanup Operation、Evidence、Settlement sibling；
3. Management CurrentIndex/Tombstone terminal DTO 与 certification。

删除请求不等于 deleted；purge 是唯一物理删除 Effect，cleanup 是独立 Effect。lost reply 只
Inspect 原 Attempt。上述公共合同未落地前，Sandbox 不实现私有替代链，
`FeatureSnapshotArtifactCapture=true`、terminal `FeatureSnapshotArtifactOwner=false`。
