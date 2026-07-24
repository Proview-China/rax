# Sandbox Snapshot purge V5 sibling候选提交独立审计

- 时间：2026-07-16 23:10 CST
- 状态：`review_pending / implementation-NO-GO`
- 范围：仅Sandbox自有design/plan/memory资产；未修改Runtime、Management、Continuity、Harness、
  Application或Go代码，未stage/commit。

## live核对

Runtime live `OperationSettlementCurrentReaderV5`只暴露
`InspectCheckpointPhaseSettlementCurrentV5`。它的Submission/CommitBundle/current Inspection全部绑定
CheckpointAttempt/Participant Phase，public测试锁定“一方法Reader + Gateway-backed marker”，raw
Fact Port与普通Reader不能满足production wiring。因此Snapshot purge不能扩义或复用该合同，只能
等待Runtime additive sibling。

## 本轮候选真值

1. delete request只形成Sandbox删除候选/Reservation，不等于deleted、Dispatch或Provider执行权；
2. `praxis.sandbox/snapshot-artifact-purge`是唯一物理删除Effect；
   `praxis.sandbox/snapshot-artifact-purge-cleanup`是独立Effect，不能复用purge的Operation/Attempt/
   Permit/Enforcement/Evidence/Settlement，也不能以cleanup成功推进deleted；
3. deleted只允许经Provider Observation→Evidence consumed→Sandbox DomainResult CAS→Runtime
   Snapshot purge sibling Settlement current→Sandbox Apply CAS后发布terminal Tombstone/CurrentIndex；
4. Retention/Legal Hold候选冻结Subject/Coverage/Index current/Carry/NoActive current exact DTO与
   Index current、NoActive current/historical、Carry exact四项Reader；caller RequestedNotAfter不进入
   外部Owner Projection/canonical，S1→S2按generation/sequence词典序，跨代必须连续carry；
5. Runtime候选新增`OperationSnapshotPurgeSettlementCurrentReaderV5`与Gateway-backed provider
   marker，保持现有Checkpoint V5 Reader一方法不变。Settlement Ref opaque，不含Disposition/Outcome；
6. Reader error闭表为既有Runtime category/reason组合。所有错误返回零Inspection且零Settle/Commit/
   Provider/Apply；未列backend错误必须在Runtime Gateway归一为
   `internal/execution_inspection_invalid`，不得透传secret或Provider payload；
7. Issue/Begin、Enforcement、Provider、Evidence、DomainResult、Settlement、Apply任一步lost reply只
   Inspect原Operation/Effect/Attempt或expected successor；NotFound/进程死亡/宿主漂移不恢复重派权。

## 资产落点

- `design/sandbox/snapshot-artifact-owner-v2.md`：新增第10节字段级合同、第11节Review账本；
- `design/sandbox/port-delta.md`：补Runtime additive sibling、closed errors与Effect拆分；
- `design/sandbox/README.md`、`acceptance.md`、`workspace-checkpoint.md`：同步状态和验收真值；
- `plan/sandbox/snapshot-artifact-owner-v2.md`：新增SA-P0R、外部审计顺序与测试矩阵；
- `plan/sandbox/README.md`、`test-matrix.md`：同步`review_pending`状态与反例。

## 审计账本

- Sandbox owner-local：`P0/P1/P2=0/0/0`；
- external P0=`3`：Retention/Legal Hold公共Index/Carry/Reader、Runtime Snapshot purge
  Evidence/Settlement/Reader/Gateway、Management CurrentIndex/Tombstone最终DTO与持久化/续期；
- 结论：候选可提交独立审计，但整体不可实现。Runtime/Retention/Management未全部Review YES并
  落地公共合同前，SnapshotArtifactOwnerV2、purge/cleanup handler、Provider、production
  backend/root与Go实现继续NO-GO，Provider调用为0。
