# Sandbox Snapshot purge第三候选neutral源字段纠偏

时间：2026-07-16 23:48:06 +0800
状态：`review_pending / implementation-NO-GO / Harness facts双审待审`

## 最新裁决

上一条memory记录的opaque neutral方向已被最新管理裁决覆盖；旧memory不回改。本条记录当前真值：

1. `SnapshotArtifactPurgeRequestV1`、`SnapshotArtifactSubjectRefV2`、
   `SnapshotArtifactAggregateRefV2`、`SnapshotArtifactDeletionAttemptRefV2`均由Sandbox控制域独立
   `SnapshotArtifactOwnerV2`真实seal Runtime neutral DTO需要的
   `Owner/Kind/ID/Revision/Digest/TenantID/Schema/ExpiresUnixNano`。
2. cleanup的Request/Subject/Aggregate/Attempt同样提供完整真实字段。Runtime-owned
   `OperationDomainFactExactRefV1`只接收逐字段复制；Adapter不得推导、默认填充、重封或延长TTL。
3. Purge Evidence继续保持完整ContractVersion/TypeURL/DigestDomain/TTL、Handoff exact-current最短
   TTL、全部Port Request/Result，以及Record+Chain+Cursor+Consumption+Qualification-consumed同Evidence
   Owner原子CAS。
4. cleanup ProviderHandoff、NeutralCleanupBindingSet、两阶段EvidenceBindingSet、Settlement
   CommitBundle/Inspection及其组成对象保持完整shape/canonical/TTL，不留占位。

## 审查状态

- Sandbox owner-local候选：`P0/P1/P2=0/0/0`。
- external P0=`3`不变：Retention/Legal Hold、Runtime Snapshot purge sibling、Management最终DTO。
- 未写Go、未修改Runtime/Harness/Retention/Management、未stage/commit；提交Runtime/Harness facts双审。
