# Sandbox Snapshot purge第五候选摘要环与Subject分层返修

时间：2026-07-17 00:11:46 +0800
状态：`review_pending / implementation-NO-GO / 第五候选独立审计待审`

## 审计输入

- 第四候选第一独立审：owner-local `P0=1 / P1=1`。
- 第三审指定的Purge/cleanup Evidence DTO、Settlement全shape与source→neutral单向映射保持闭合。
- external P0保持`3`：Retention/Legal Hold、Runtime Snapshot purge sibling、Management最终DTO。

## 本轮纠偏

1. `SnapshotArtifactPurgeRequestV1`删除`DeletionAttemptRefV2`，并禁止Deletion Reservation反向引用
   Request/Attempt。canonical创建链固定为DeletionReservation→PurgeRequest→DeletionAttempt；只有
   Attempt单向绑定`PurgeRequestDigest`。Request/Attempt lost reply分别Inspect原identity，禁止回填或
   重封Request。
2. Subject拆为两层：`SnapshotArtifactSubjectIdentityV2`只表达stable identity，不含Revision、TTL、
   RequestedNotAfter或current语义；`SnapshotArtifactSubjectRefV2`独立表达versioned exact/current ref，
   携Revision、Schema、StableSubjectDigest、SubjectDigest与Expires。
3. Runtime neutral BindingSet只接收versioned exact/current SubjectRef并将其Expires纳入四源最短TTL；
   stable identity不进入TTL闭集，Adapter不得为其推导revision/TTL或伪造成neutral current ref。

## 当前结论

- Sandbox owner-local候选自查：`P0/P1/P2=0/0/0`，等待第五候选独立审计。
- 继续`implementation-NO-GO`；未写Go、未修改Runtime/Harness/Retention/Management、未stage/commit。
