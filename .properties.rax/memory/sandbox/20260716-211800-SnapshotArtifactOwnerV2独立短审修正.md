# SnapshotArtifactOwnerV2独立短审修正

- 时间：2026-07-16 21:18 CST
- 状态：`joint-review-candidate / implementation-NO-GO`；未写Go。
- 触发：独立短审发现Artifact/Retention digest循环、首切片伪执行TTL、Legal Hold负证明
  缺失、aggregate CAS与公共/内部Port边界未统一。

## 修正真值

1. `SnapshotArtifactFactV2`不再反向含`RetentionApplicationRef`。Owner先seal Fact，
   `SnapshotArtifactRetentionApplicationFactV2`再引用FactRef，后续aggregate revision关联二者；
   stable `SnapshotArtifactSubjectRefV2`不含Fact/Retention digest，创建顺序无环。
2. 首个Reserve/Inspect切片只返回`SnapshotArtifactAggregateCurrentProjectionV2`，TTL只取
   Store内Envelope/Reservation/已关联Entry，不声明Participant、Lease/Fence、Scope、
   Credential或Checkpoint/Restore执行资格。未来执行资格必须新增完整exact DTO/readers并
   在S1/S2复读。
3. `LegalHoldRef=nil`、空集合或caller布尔值不能证明无hold。Deletion confirmed必须绑定
   Retention/Legal Hold Owner分别在S1和CAS前S2 seal的fresh
   `NoActiveLegalHoldCurrentProjection`；Unknown/Unavailable/expired/drift全部零写。
4. Reservation、Artifact、Retention、Deletion统一进入append-only aggregate entry/envelope，
   CAS一律使用`ExpectedAggregateRef(ID/Revision/Digest)`，不能只给revision。
5. 外部`SnapshotArtifactOwnerPortV2`只Reserve/Inspect；Artifact/Retention/Deletion提交移到
   Owner-internal committer，调用方不能mint payload/Envelope。
6. Deletion `failed`只终结原Attempt，可重新经过fresh治理与fresh S1/S2创建新Attempt；
   `confirmed`与`indeterminate`是aggregate终态，禁止复活或新Attempt。

## 仍待联合Review

- Retention/Legal Hold Owner正式名称与negative current proof公共形状；
- 统一entry/envelope、ExpectedAggregateRef和内部committer精确DTO；
- Deletion Runtime Operation/Settlement与S1/S2 readers；
- 首切片只Reserve/Inspect还是连同已闭合Owner committer一起授权。

以上未全部YES前，Provider、Runtime/Continuity写、production Backend/root与Go实现均为0。

## P0/P1/P2收口

- P0：Sandbox候选内3项均已关闭——Fact/Retention无环；首切片删除伪execution TTL；
  Deletion强制S1/S2 sealed negative no-hold current proof。
- P1：Sandbox候选内3项均已关闭——统一Entry/Envelope与full ExpectedAggregateRef；公共面只
  Reserve/Inspect、提交仅Owner-internal；failed仅fresh治理Attempt重试，confirmed/indeterminate终态。
- P2：README、contracts、interfaces、state machine、workspace、port delta、acceptance、plan与
  test matrix已统一为上述候选真值；没有实现勾选。

可冻结性：Sandbox内部语义已达到联合Review输入质量，但仍是conditional NO。Retention/Legal Hold
Owner必须确认negative proof/Reader公共形状，Runtime必须确认Deletion Operation/Settlement与S1/S2
消费边界；两项均YES后才可改为approved。当前实现、Provider、Runtime/Continuity写均继续NO-GO。
