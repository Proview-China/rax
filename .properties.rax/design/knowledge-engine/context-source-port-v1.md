# KnowledgeContextSourceCurrentReaderV1 设计Delta

状态：**completed；Owner-local Reader与cross-owner reference integration均为software-test YES（P0/P1/P2=0）**。Knowledge Adapter、Application三阶段Port、Context `knowledge_reference`/TransitionProof/atomic Refresh及非零reference fixture已实现；production G6B/root与远程Knowledge Retrieval Gateway仍NO-GO。

本合同只定义Knowledge Owner State Plane中的owner-current只读面。它不发起检索、不访问Provider/网络/远程Resolver，也不执行远程正文读取。effectful Retrieval与远程正文读取移至[Knowledge Retrieval Domain Gateway Delta](./retrieval-domain-gateway-v1.md)。reference integration使用一个Owner-local Knowledge source；production root尚未启用。

## 1. Owner与边界

- Knowledge Owner拥有Retrieval Journal、Observation/Result、Source/Package/Record/Snapshot/Pointer/View/Projection、License/Trust/Conflict、DomainResultAssociation、SettlementApplication及本地Content currentness。
- Context Owner拥有Candidate、Admission、Recipe预算、Frame、Generation、materialize/freeze与S1/S2调用时点。
- Asset Owner仍拥有原始Artifact字节；Knowledge只读取已进入Knowledge Owner本地State Plane且可exact证明的正文副本。
- Application只传播exact refs并协调调用；Harness只消费S2-current exact Frame。
- Context缓存、Citation、Reference Store和Compaction Summary都不是Knowledge真值。

Reader输入中的Retrieval Observation/Result必须已经由Knowledge Owner持久化并完成对应领域ApplySettlement。Provider命中、Score、Citation、Coverage与Trust标签仍只是Observation。

## 2. 合同、canonical与本地性

合同名：`praxis.knowledge/context-source-current-reader/v1`。

持久对象使用`Ref{ID, Revision, Digest}`，正文使用`ContentRef{ID, Digest, Length, MediaType}`。Digest按domain/version/discriminator隔离，计算时Digest字段置空；严格解码拒绝重复键、未知治理字段与尾随文档。

Canonical规则：

1. `nil`与空集合统一；无序Ref按`ID -> Revision -> Digest`排序去重；
2. Items固定为`Score降序 -> Record ID升序 -> Revision降序 -> Digest升序`；
3. Citation、Coverage、Conflict、License、Trust、Dropped Reasons、Next Cursor、预算、locality proof与TTL全部入摘要；
4. `now >= ExpiresUnixNano`即过期；
5. 同一ID/revision换payload、Owner、排序、License/Trust/Conflict、locality proof或digest均冲突。

`KnowledgeLocalOwnerStatePlaneProofV1`绑定Knowledge Owner、Store Domain、Content/Asset exact ref、当前本地副本revision/digest、允许License/Scope与Expires。Asset locator、远程URL、Resolver句柄、Context缓存或Sandbox副本不构成本地Owner State Plane证明。

## 3. 纯只读Port签名

```text
KnowledgeContextSourceCurrentReaderV1
  InspectAttempt(KnowledgeLocalRetrievalAttemptCoordinateV1)
    -> KnowledgeLocalRetrievalAttemptInspectionV1

  InspectForTurn(KnowledgeContributionCurrentRequestV1)
    -> KnowledgeContributionCurrentProjectionV1

  ReadContentExact(KnowledgeLocalExactContentRequestV1)
    -> KnowledgeLocalExactContentObservationV1 + bounded local body
```

本Reader禁止出现`Retrieve`、`ResolveRemote`、`ReadRemoteContent`、dispatch、Provider client或网络参数。它不得返回Context Frame/Generation、组织Trust结论、Runtime Outcome或Checkpoint eligibility。

## 4. InspectAttempt：只检查Owner本地Journal

输入包含ContractVersion、Owner Retrieval Attempt exact ref、RequestDigest、IdempotencyKey、TenantID、ExecutionScopeDigest及Expected Observation/Result exact refs。

输出包含Inspection exact ref、原Attempt/Observation/Result refs、Journal revision、状态、Observed/Expires、Residual与canonical Digest。状态闭集：

- `persisted_and_settled`：Observation/Result canonical有效，DomainResultAssociation、Runtime Settlement exact ref及ApplySettlement精确闭合；
- `persisted_unsettled`：本地结果存在但Evidence/Settlement/Apply未闭合，不可注入；
- `confirmed_not_persisted`：Owner Journal证明没有本地结果；
- `inspection_incomplete`：本地Journal不可证明。

本方法不运行Query、不联系Provider/Resolver、不移动Snapshot pointer、不替换Source或补选结果。远程丢回包只能调用Retrieval Gateway的`InspectOriginalAttempt`。

## 5. InspectForTurn：owner-current projection

请求包含ContractVersion、ContributionAttemptID、ExecutionScopeDigest、RunID/Turn、Observation/Result exact refs、Expected Query/View/Published Snapshot/Pointer refs、Authority/Policy/Purpose/Scopes/AllowedLicenses/SensitivityMax、预算/Estimator、Checked/NotAfter、Required、Idempotency及S2的`ExpectedS1ClosureDigest`。

输出包含Projection exact ref、Owner=`praxis.knowledge`、Current、Checked/Expires、Observation/Result/Query/View/Published Snapshot/Current Pointer refs、治理绑定、Coverage/Trust/Conflict摘要、预算、Residual、有序Items、`ClosureDigest`与`ProjectionDigest`。

每个Item绑定Rank/Score、Record/Package/Snapshot/Source/Projection/Content、DomainResult/Association/SettlementApplication、Evidence/Citation、License/Trust/Conflict、TTL及Token Estimate/Estimator Digest。

Knowledge Owner必须仅从本地Owner State Plane复读：

1. Observation/Result canonical、Items顺序、Citation、Coverage、Conflict与Next Cursor；
2. Retrieval DomainResult canonical、Association及Runtime Settlement/ApplySettlement闭表；
3. View、Published Snapshot immutable manifest、current Pointer、Package/Record/Source/Projection；
4. License、Trust、Conflict、Poisoning/Admission、Authority/Policy、Purpose、Scope与Sensitivity；
5. Observation、Result、View、Snapshot policy、Record、Source、Projection、Authority/Policy和Frame Deadline的最小TTL。

任一对象需要远程Resolver/Provider确认、Source已Withdraw、Pointer漂移或未完成Evidence/Settlement闭表时，必须`Current=false`，Reader不得发起Effect。

## 6. ReadContentExact：仅限本地Owner State Plane

请求绑定Contribution Projection、ClosureDigest、Item Rank、Record/Source/Content exact refs、`KnowledgeLocalOwnerStatePlaneProofV1`、ExecutionScopeDigest、Checked/NotAfter与Idempotency。

输出绑定输入refs、实际Length/MediaType/Digest、Locality Proof、Observed/Expires与canonical Digest；正文只从受预算约束的本地body/stream返回。Owner必须重算bytes摘要，并复读Source、License、Authority、Scope及Sensitivity仍允许披露。

只有远程Asset/Resolver、正文已evicted、locality proof漂移或bytes摘要不一致时，返回`remote_required`、`evicted`或`unavailable`；不得联网，也不得使用Context缓存、Citation、Embedding或Summary代替。

远程正文只能通过Retrieval Gateway的`ReadRemoteContentExact`治理执行，并在DomainResult/Settlement/Apply闭合后形成新的Knowledge Owner本地exact ref；本Reader随后才能读取。

## 7. G6B S1/S2与原子发布

```text
前置：Application已经持有settled Retrieval Observation/Result exact refs

S1: Context reserve/collect前
  -> CurrentReader.InspectAttempt(persisted_and_settled)
  -> CurrentReader.InspectForTurn(Current=true)
  -> CurrentReader.ReadContentExact(local only)
  -> Context Owner materialize/admit并形成pending Context DomainResult
  -> pending DomainResult与Generation均不可见、不可current

S2: pending Context DomainResult形成后、任何current发布前
  -> 相同Observation/Result + ExpectedS1ClosureDigest再次InspectForTurn
  -> 再次本地ReadContentExact并复读License/current Pointer
  -> ClosureDigest一致且TTL覆盖原子提交及交付时点
  -> Context Owner单个本地原子边界执行ApplySettlement + Generation current CAS
  -> 仅原子成功后Frame/Generation可见且current
```

S2失败、Pointer/License/TTL漂移或原子CAS冲突时不得发布current Frame/Generation；pending Context DomainResult保持不可见并由Context Owner结算失败/Residual。Knowledge Owner不执行Context ApplySettlement或Generation CAS。

reference fixture中`KnowledgeSources=1`并调用本地Reader；Retrieval Gateway调用数仍必须为0。Reader/Adapter YES不等于远程Gateway或production root YES。

## 8. NO-GO与Unknown反例

- 把`Retrieve`、远程Asset读取或Resolver藏进Current Reader；
- Reader遇到`remote_required`后自行联网；
- Provider回包丢失后用本地`InspectAttempt`宣称远程Effect成功；
- Observation/Result已持久化但Evidence、Runtime Settlement或ApplySettlement未闭表仍返回Current；
- Published Snapshot存在但current Pointer已切换；
- Source已Withdraw、Package/Record已纠错或Projection stale仍复用旧Result；
- License/Policy/Scope/Sensitivity在S1/S2间缩小；
- Content只在远程或已evicted，却用Context缓存/Citation/Summary替代；
- 同ContentRef、Result/Citation/Coverage或DomainResultAssociation被篡改；
- 缺少正式Poisoning判断仍注入；
- lost reply后换Attempt、Provider或Source重新Query，而非Gateway Inspect原Attempt。
- S1后先发布Generation current，再执行S2；
- S2失败仍把pending Context DomainResult变为可见；
- ApplySettlement与Generation current CAS分成两个可观察提交，留下半发布状态；
- S2通过后、原子提交前License或TTL失效仍发布current。

## 9. Checkpoint/Restore分离

本Reader不提供Checkpoint capture、Participant Prepare/Commit、Restore Stage或compatibility结论。未来另行冻结`KnowledgeCheckpointReaderV1`与`KnowledgeRestoreCompatibilityReaderV1`；不得用Adapter扩权本Reader。

Checkpoint/Restore不混入当前G6B V1，也不随本Reader或Retrieval Gateway Review自动获得实现授权。

## 10. Delta 10/11 Owner V2冻结与External接线候选

V1/V2属于同一live Reader合同族并共享唯一current真值。V2要求`SourceTurnRef/Ordinal == Tool.Execution.Turn == ExpectedCurrent.Turn`。live Harness committed PendingAction current返回canonical Session/Turn applicability coordinate、ordinal与TTL，经Harness-owned Adapter无损投影并在Application三阶段合同中S1/S2复读，不建立第二Turn Store/Reader。Knowledge不得导入Harness实现，也不得从uint32或legacy TurnID补造。stable Closure与fresh Projection/Observation digest、ctx-aware bounded reader、Context独占TransitionProof及`knowledge_reference`边界见：[Knowledge Delta 10/11合同](./context-refresh-neutral-delta10-11-v1.md)。TargetTurn和proof不属于Knowledge DTO。

live Adapter、Application public三阶段Port与Context `knowledge_reference`已按本合同族实现；这不授权production root或远程Retrieval/Resolver。reference fixture的Knowledge来源数与Reader调用数为1，生产调用保持0直到root装配。
