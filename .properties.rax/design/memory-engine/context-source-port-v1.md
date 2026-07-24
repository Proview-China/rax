# MemoryContextSourceCurrentReaderV1 设计Delta

状态：**completed；Owner-local Reader与cross-owner reference integration均为software-test YES（P0/P1/P2=0）**。Memory Adapter、Application三阶段Port、Context TransitionProof/atomic Refresh及非零reference fixture已实现；production G6B/root与远程Memory Retrieval Gateway仍NO-GO。

本合同只定义Memory Owner State Plane中的owner-current只读面。它不发起检索、不访问Provider/网络/远程Resolver，也不执行远程正文读取。effectful Retrieval与远程正文读取移至[Memory Retrieval Domain Gateway Delta](./retrieval-domain-gateway-v1.md)。reference integration使用一个Owner-local Memory source；production root尚未启用。

## 1. Owner与边界

- Memory Owner拥有Retrieval Journal、Observation/Result、Record/View/Watermark/Projection、DomainResultAssociation、SettlementApplication及本地Content currentness。
- Context Owner拥有Candidate、Admission、Recipe预算、Frame、Generation、materialize/freeze与S1/S2调用时点。
- Application只传播exact refs并协调调用；不解释Memory事实。
- Harness只消费已冻结的exact Frame；不得调用本Reader。
- Context缓存、Reference Store与Compaction Summary都不是Memory真值。

Reader输入中的Retrieval Observation/Result必须已经由Memory Owner持久化并完成对应领域ApplySettlement。命中、Score、Citation、Coverage和Result仍只是Observation，不是Memory Record或Context Frame。

## 2. 合同、canonical与本地性

合同名：`praxis.memory/context-source-current-reader/v1`。

所有持久对象使用`Ref{ID, Revision, Digest}`。Digest按`domain + contract version + discriminator + canonical payload`计算，计算时Digest字段置空；严格解码拒绝重复键、未知治理字段与尾随文档。

Canonical规则：

1. `nil`与空集合统一为空集合；无序Ref按`ID -> Revision -> Digest`排序并去重；
2. Items保持语义顺序：`Score降序 -> Record ID升序 -> Revision降序 -> Digest升序`；
3. Citation、Coverage、Dropped Reasons、Next Cursor、预算、locality proof和TTL全部入摘要；
4. 时间使用UTC UnixNano，`now >= ExpiresUnixNano`即过期；
5. 同一ID/revision换payload、digest、排序、Owner或locality proof均为冲突。

`LocalOwnerStatePlaneProofV1`必须绑定Owner、Store Domain、Content Ref、当前本地副本revision/digest、可读范围与Expires。Reader不得将Sandbox缓存、Context Reference Store、远程URL或Resolver句柄标记为本地Owner State Plane。

## 3. 纯只读Port签名

```text
MemoryContextSourceCurrentReaderV1
  InspectAttempt(MemoryLocalRetrievalAttemptCoordinateV1)
    -> MemoryLocalRetrievalAttemptInspectionV1

  InspectForTurn(MemoryContributionCurrentRequestV1)
    -> MemoryContributionCurrentProjectionV1

  ReadContentExact(MemoryLocalExactContentRequestV1)
    -> MemoryLocalExactContentObservationV1 + bounded local body
```

本Reader禁止出现`Retrieve`、`ResolveRemote`、`ReadRemoteContent`、dispatch、Provider client或网络参数。它不得返回Context Candidate/Frame/Generation、Runtime Outcome或Checkpoint eligibility。

## 4. InspectAttempt：只检查Owner本地Journal

`MemoryLocalRetrievalAttemptCoordinateV1`包含ContractVersion、Owner Retrieval Attempt exact ref、RequestDigest、IdempotencyKey、TenantID、ExecutionScopeDigest及Expected Observation/Result exact refs。

输出包含Inspection exact ref、原Attempt/Observation/Result refs、Journal revision、状态、Observed/Expires、Residual和canonical Digest。状态闭集：

- `persisted_and_settled`：Observation/Result canonical有效，且DomainResultAssociation、Runtime Settlement exact ref和ApplySettlement均精确闭合；
- `persisted_unsettled`：本地对象存在，但Evidence/Settlement/Apply任一未闭合，不可用于Context；
- `confirmed_not_persisted`：Owner Journal证明本地没有该结果；
- `inspection_incomplete`：本地Journal损坏、缺页或不可证明。

本方法只读Owner Journal，不运行Query、不联系Provider、不移动Watermark、不补选结果，也不承担远程Unknown恢复。远程执行或远程正文读取丢回包时，Application只能调用Retrieval Gateway的`InspectOriginalAttempt`；不得用本方法猜测Provider结果。

## 5. InspectForTurn：owner-current projection

请求至少包含ContractVersion、ContributionAttemptID、ExecutionScopeDigest、RunID/Turn、Observation/Result exact refs、Expected Query/View/Watermark refs、Authority/Policy/Purpose/Scopes/SensitivityMax、预算/Estimator refs、Checked/NotAfter、Required、IdempotencyKey，以及S2的`ExpectedS1ClosureDigest`。

输出包含Projection exact ref、Owner=`praxis.memory`、Current、Checked/Expires、Observation/Result/Query/View/CurrentWatermark refs、治理绑定、Coverage/预算/Residual、有序Items、`ClosureDigest`与`ProjectionDigest`。

每个Item绑定Rank/Score、Record current-head、Content、Source/Evidence/Projection/Citation、DomainResult、DomainResultAssociation、SettlementApplication、TTL和Token Estimate/Estimator Digest。

Memory Owner必须仅从本地Owner State Plane复读并验证：

1. Observation/Result canonical、顺序、Citation、Coverage与Next Cursor；
2. Retrieval DomainResult canonical、Association及Runtime Settlement/ApplySettlement闭表；
3. View、Watermark、Record current head与Projection currentness；
4. Tenant/Identity、Authority/Policy、Purpose、Scope、Sensitivity与预算；
5. Observation、Result、View、Record、Projection、Authority/Policy及Frame Deadline的最小TTL。

任一对象需要远程Resolver、Provider确认或未完成Evidence/Settlement闭表时，必须`Current=false`并返回Residual，不能从Reader发起Effect。

## 6. ReadContentExact：仅限本地Owner State Plane

`MemoryLocalExactContentRequestV1`绑定Contribution Projection、ClosureDigest、Item Rank、Record/Content exact refs、`LocalOwnerStatePlaneProofV1`、ExecutionScopeDigest、Checked/NotAfter及Idempotency。

输出绑定输入refs、实际Length/MediaType/Digest、Locality Proof、Observed/Expires和canonical Digest；正文仅通过受预算约束的本地body/stream返回。

Owner必须读取本地权威副本并重算bytes摘要。出现以下任一情况都返回明确的`remote_required`、`evicted`或`unavailable`，不得联网或降级：

- Content只有远程Locator/Resolver；
- 本地Owner副本已evicted；
- ContentRef与bytes/length/media/digest不一致；
- 只能从Context缓存、Summary、Embedding或Sandbox副本重建。

远程正文只能通过Retrieval Gateway的`ReadRemoteContentExact`治理执行，并在其DomainResult/Settlement/Apply闭合后形成新的Owner本地exact ref；本Reader随后才能读取。

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
  -> 用相同Observation/Result与ExpectedS1ClosureDigest再次InspectForTurn
  -> 再次本地ReadContentExact
  -> ClosureDigest一致且TTL覆盖原子提交及交付时点
  -> Context Owner单个本地原子边界执行ApplySettlement + Generation current CAS
  -> 仅原子成功后Frame/Generation可见且current
```

S2失败、超时、TTL相等即过期、摘要漂移或原子CAS冲突时，Context Owner不得发布current Frame/Generation；pending DomainResult只能保持不可见并按Context Owner合同标记失败/Residual，不能绕过S2重用。Memory Owner只提供fresh currentness，不执行Context ApplySettlement或Generation CAS。

reference fixture中`MemorySources=1`并调用本地Reader；Retrieval Gateway调用数仍必须为0。Reader/Adapter YES不等于远程Gateway或production root YES。

## 8. NO-GO与Unknown反例

- 把`Retrieve`或远程Resolver藏进Current Reader；
- Reader遇到`remote_required`后自行联网；
- Provider回包丢失后调用本地`InspectAttempt`并据此宣称Effect成功；
- Retrieval Observation/Result已持久化，但Evidence、Runtime Settlement或ApplySettlement未闭表仍返回Current；
- 旧Record仍active但current head已Correction/Tombstone；
- View Watermark、Projection、Authority、Scope、Purpose、Sensitivity或TTL漂移；
- ContentRef存在但本地bytes已evicted，改用Context缓存或Summary；
- 同ContentRef、Observation/Result或DomainResult ref换内容/排序/Association；
- 缺少正式Poisoning判断仍注入；
- lost reply后新建Attempt或同Attempt换请求，而非Gateway Inspect原Attempt。
- S1后立即发布Generation current，再把S2当事后检查；
- S2失败仍Apply pending Context DomainResult，或ApplySettlement成功但Generation current CAS失败后保留半发布状态；
- S2通过与Apply/CAS之间TTL到期，却仍发布current。

## 9. Checkpoint/Restore分离

本Reader不提供Checkpoint capture、Participant Prepare/Commit、Restore Stage或compatibility结论。未来另行冻结`MemoryCheckpointReaderV1`与`MemoryRestoreCompatibilityReaderV1`；不得以类型别名或Adapter扩权本Reader。

Checkpoint/Restore不混入当前G6B V1，也不随本Reader或Retrieval Gateway Review自动获得实现授权。

## 10. Delta 10/11 Owner V2冻结与External接线候选

V1/V2属于同一live Reader合同族并共享唯一current真值。V2要求`SourceTurnRef/Ordinal == Tool.Execution.Turn == ExpectedCurrent.Turn`。live Harness committed PendingAction current返回canonical Session/Turn applicability coordinate、ordinal与TTL，经Harness-owned Adapter无损投影并在Application三阶段合同中S1/S2复读，不建立第二Turn Store/Reader。Memory不得导入Harness实现，也不得从uint32或legacy TurnID补造。stable Closure与fresh Projection/Observation digest、ctx-aware bounded reader及Context独占TransitionProof边界见：[Memory Delta 10/11合同](./context-refresh-neutral-delta10-11-v1.md)。TargetTurn和proof不属于Memory DTO。

live Adapter与Application public三阶段Port已按本合同族实现；这不授权production root或远程Retrieval。reference fixture的Memory来源数与Reader调用数为1，生产调用保持0直到root装配。
