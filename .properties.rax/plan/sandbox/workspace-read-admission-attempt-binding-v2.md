# Workspace Read Admission Attempt Binding V2 Plan

状态：`implemented-candidate / owner-local / PR #76 / 已提交未合并`。

## 文件落点

- `ExecutionRuntime/sandbox/ports/workspace_read_admission_attempt_binding_v2.go`
  - additive V2 object、canonical seal/validate/clone与public exact Reader；
- `ExecutionRuntime/sandbox/internal/owner/workspaceread/**`
  - S1后才能构造的owner-private capability及import-boundary门；
- `ExecutionRuntime/sandbox/kernel/workspace_read_v1.go`
  - actual physical path创建internal request；public V2 Inspect只读转发；
- `ExecutionRuntime/sandbox/storage/sqlite/{store.go,workspace_read_v1.go,workspace_read_admission_attempt_binding_v2.go}`
  - v18 strict schema、同事务create-once、full exact lookup与历史闭包；
- `ExecutionRuntime/sandbox/{ports,kernel,dataplaneadapter,storage/sqlite,internal/owner/workspaceread}/**/*test.go`
  - canonical、owner boundary、public executor、schema、并发和故障反例。

## 已执行验收

- public `WorkspaceReadPhysicalExecutorV1`经S1→internal capability→SQLite writer；
- 64并发同authorization只有一个V2 winner，physical actual计数为1；
- lost reply/restart/8 handles按full Runtime Attempt读取同一binding；
- Runtime Attempt的Delegation及全部exact轴splice拒绝；
- Authorization、Association、Command、Admission、original Attempt、
  Reservation与payload closure逐轴splice写入为零；
- ExpectedFileRef与nested refs clone/no-alias；
- V2/V1 canonical body、binding digest、denormalized exact columns、
  origin/Reservation stable digest协同篡改均Fail Closed；
- V2存在但origin、Reservation或V1 binding引用行缺失时返回Conflict；
- 64并发只读前后全部相关表count与canonical row hash不变；
- V1-only actual path拒绝，Reader零Provider、零写；
- v18 missing/extra/partial/NOCASE/lowercase-binary/index seq/key/aux/ledger
  及完整`sqlite_master.sql`词法语义反例全部Fail Closed；
- extra CHECK、extra FK、trigger与非索引列COLLATE漂移均返回Conflict且绝不repair；
- V2历史exact binding过期后仍可读取，但不恢复current或execute资格，
  physical read与recovery evidence增量均为零；
- full ordinary、full race、vet与module verify通过。

## 剩余边界

本计划只完成Sandbox owner-local历史恢复坐标。Tool consumer adapter、Runtime或
Application composition、production Provider/root、跨Owner发布资格均不属于
本切片，继续NO-GO。变更已发布至ready PR #76，等待独立返修复审，尚未合并。
