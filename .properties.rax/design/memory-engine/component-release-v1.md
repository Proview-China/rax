# Memory/Knowledge Component Release V1

Memory与Knowledge通过同一Assembler Component Release发布，但保持两个独立Owner Capability、Port和Factory。发布面只描述构造合同，不拥有Host root、Runtime事实、Context Generation或生产认证。

| 模式 | exact证明 | 当前真值 |
|---|---|---|
| reference_only | 完整Manifest、Module、两个Capability/Port/Factory | 可发布 |
| standalone | Memory/Knowledge owner、Retrieval、两个Context Source、Purge Inspect六项owner-local current | 可发布候选 |
| production | durable Memory/Knowledge fact+content stores、Authority/Policy、Credential、Index/Context current、Settlement、Purge Effect、Cleanup、Deployment Attestation、独立Certification | NO-GO |

## Production closure V2候选

状态：`design_frozen_for_implementation`。该候选只拥有Memory/Knowledge的证明聚合与发布适配，不创建外部Owner事实，也不选择数据库、RPC、进程拓扑或SLA。

生产部署必须显式选择以下一种Profile：

| Profile | 必需证明 | 禁止宣称 |
|---|---|---|
| `non_ha` | 四个durable fact/content Resource Handle、single-writer Fence current、恢复证明、备份恢复证明 | 高可用、自动故障转移、仲裁写 |
| `ha` | `non_ha`全部证明，加replication current、quorum current、failover proof、monotonic-current proof；ReplicaCount >= 3且WriteQuorum > ReplicaCount/2 | 未经exact proof的副本数、RPO/RTO或SLA |

`ProductionProofBundleV2`是Memory/Knowledge Owner的关联封套，不是外部事实。它只保存：

1. Runtime-neutral `ResourceBindingSetRefV1`与四个role-tagged `ResourceHandleRefV1`；
2. Authority/Policy、Credential、Retrieval Index、Context Source、Settlement、Purge Effect、Cleanup、Deployment、Certification的opaque `OwnerCurrentRefV1`；
3. Profile-specific Fence/Recovery/Backup及HA证明Ref；
4. ReleaseID、Revision、ArtifactDigest、ManifestDigest、Checked/Expires与canonical Digest。

Reader固定为`InspectProductionProofBundleV2(ctx, releaseID, revision)`。Verifier在同一调用中执行S1读取、逐项Validate/current TTL、Resource BindingSet exact Inspect、四个Resource Handle exact Inspect、S2复读；S1/S2的stable digest、所有exact Ref与Profile必须相同。Reader丢回包或UnknownOutcome不得重新创建，只允许Inspect原坐标。任何cancel、typed-nil、过期、clock rollback、role alias、资源kind替换、Profile证明缺失、S1/S2漂移、Certification或Manifest不匹配均Fail Closed。

验证成功只产出`ProductionReadinessProjectionV1`并交给现有Publisher；Memory/Knowledge不创建Runtime Outcome、Context Fact、Authority/Policy/Credential事实或Certification。Application/Assembler只协调和消费exact结果；Harness不调用该Reader，只消费已经发布并Inspect为current的Frame与Release。

生产调用顺序：

```text
External Owners publish exact current/resources
  -> Memory/Knowledge ProductionProofBundle S1
  -> Resource BindingSet + four Handle exact Inspect
  -> ProductionProofBundle S2
  -> map existing ProductionReadinessProjectionV1
  -> ComponentRelease Publisher S1/S2 + Catalog Ensure/Inspect
```

Consumer约束：

| Consumer | 允许 | 禁止 |
|---|---|---|
| Application | 协调exact refs、选择已声明Profile、传播cancel | 补造Owner current、把non-HA升级成HA |
| Assembler/Host | 绑定Factory descriptor与实际Resource Handle | 把descriptor当已构造后端 |
| Context | 使用已current的Context Source exact ref | 把Retrieval Observation当Memory/Knowledge事实 |
| Harness | 消费exact Frame/Release | 直连Store、Reader或Provider |
| Runtime | 提供Resource/Settlement/Governance neutral refs | 解释Memory/Knowledge领域事实 |

`LocalReadinessProjectionV1`与`ProductionReadinessProjectionV1`都使用exact Ref、稳定摘要、共同TTL及S1/S2复读；Reader不可用、漂移、过期或clock rollback均Fail Closed，不得静默降级。Catalog `EnsureExact`丢回包只按原Ref `InspectExact`。

Factory仅为descriptor，不含构造函数、Store句柄或production root。`reference.Store`、Memory/Knowledge内存Store、reference fixture及owner-local Context Source测试均不能作为durable production证明。

验收覆盖三档模式、Certification exact、lost reply、64并发、TTL/clock、drift/alias、typed-nil、同ID换内容、Assembler Repository黑盒及完整ordinary/race/vet工程门。
