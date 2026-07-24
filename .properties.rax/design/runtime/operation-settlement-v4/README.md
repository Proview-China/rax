# Runtime Operation Settlement V4设计

## 1. 状态与目标

- 合同族：`praxis.runtime.operation-settlement/v4`；
- 合同版本：`4.0.0`；
- 当前阶段：Runtime实现、Owner自测、中央独立复验与最终Review均已完成，最终裁决为YES；
- 前置事实：[OperationScope Evidence V3](../operation-scope-evidence-v3/README.md)首切面已通过最终联合审核；
- 目标：把Evidence Owner已经原子消费的pre-run Operation Evidence，按phase精确关联到Runtime权威Operation Settlement，同时保持V3 Settlement、Evidence V3、Dispatch V4.0与Enforcement 4.1既有摘要不变。

本设计只新增Settlement V4公共合同、Owner边界、恢复与验收语义；不选择数据库、RPC、队列、进程拓扑、生产后端或SLA，也不因本切面完成而自动授权Action Gateway、Provider或组件实现。

## 2. 不可协商边界

1. Settlement V4是additive `4.0.0`合同。不得修改或静默扩大既有V3 Settlement结构、canonical、digest或终态语义。
2. `[]EvidenceRecordRefV2`不能伪装OperationScope Evidence V3。它缺少Qualification、Consumption、Handoff、Attempt、4.1 phase和完整OperationScope关系，无法证明phase级资格被当前消费。
3. prepare与execute必须各有且仅有一份`OperationSettlementEvidenceBindingV4`；二者不可交换、复用、遗漏或额外添加。
4. late/observation、`consumed_observation`及任何非`consumed_current`资格一律不具Settlement资格。
5. DomainResult必须绑定exact authoritative Fact ref，并由按kind路由的current Reader复读；`SchemaRef + Digest`、Provider Receipt、Observation或组件自报不能替代领域Fact。
6. Evidence Owner先原子Consume；Runtime Effect/Settlement Owner随后在自身事务内原子写Settlement V4、Evidence Association、共享terminal guard和V4 terminal projection。两个Owner之间不宣称原子事务。
7. V3 CAS与V4 Commit复用同一Effect Owner锁与共享terminal guard。既有V3 settled占用guard；V4不得伪装、回填或旁挂成V3 settled。
8. Settlement只确认历史真实结果。历史Permit、Policy或Qualification在Provider调用及Evidence消费后到期，不阻止truthful settlement；但所有历史ref、digest、phase、attempt和关联必须exact。

## 3. Owner与职责

| 对象/动作 | 唯一Owner | 允许调用方 | 明确禁止 |
|---|---|---|---|
| Qualification、Handoff、Candidate、Record、Consumption及其原子关联 | Evidence Owner | Runtime/Application通过Evidence V3公共Port | Settlement Owner重建、补写或伪造Evidence消费 |
| DomainResult Fact及current判定 | 对应EffectKind绑定的领域Owner | Settlement Gateway通过可信kind-routed Reader | Runtime从Receipt/Observation推导领域终态 |
| Effect不可变owner、Settlement V4、Association、terminal guard、V4 terminal projection | Runtime Effect/Settlement Owner | Runtime Settlement Governance Gateway | Application、Harness、组件直接写终态 |
| V3 Settlement | 既有Runtime Effect/Settlement Owner | 既有V3 Port | V4改写V3 Fact/digest；V3/V4双终态 |
| 跨Owner恢复编排 | Application Coordinator或Runtime Reconciler | 只调用公共Inspect/Commit Port | 跨Owner事务宣称、盲重派Provider、以超时推导结果 |

## 4. 新增公共对象

V4只新增独立类型，不原地扩大旧类型：

- `OperationSettlementEvidenceBindingV4`：一个phase的完整Evidence V3关系；
- `OperationSettlementSubmissionV4`：exact Effect、Operation、DomainResult和prepare/execute绑定的提交候选；
- `OperationSettlementRefV4`：Settlement V4权威事实引用；
- `OperationInspectionSettlementRefV4`：Gateway current复读快照引用；
- `OperationSettlementFactPortV4`：同一Effect Owner内的原子Commit与历史/current Inspect；
- `OperationSettlementGovernancePortV4`：唯一受治理提交入口；
- `OperationSettlementDomainResultFactRefV4`：领域权威结果的exact typed ref；
- `OperationSettlementDomainResultCurrentReaderV4`：按EffectKind/DomainResultKind可信路由的只读current Reader；
- `OperationSettlementEvidenceAssociationV4`：Settlement与两份Evidence消费的create-once关联；
- `OperationSettlementTerminalProjectionV4`：V4独立终态投影；
- `OperationSettlementTerminalGuardV4`：同一Effect在V3/V4间只允许一个终态提交的Owner内部线性化guard。

字段、canonical与Port细则见[公共合同与原子边界](./contracts.md)，兼容性见[Port Delta](./port-delta.md)。

## 5. 每个phase必须精确绑定

每份`OperationSettlementEvidenceBindingV4`必须同时包含并交叉验证：

1. phase=`prepare | execute`；
2. exact `OperationScopeEvidenceConsumptionFactV3/RefV3`；
3. Consumption引用的issued Qualification历史ref；
4. Evidence Owner当前返回的final `consumed_current` Qualification fact/ref；
5. exact `OperationScopeEvidenceRecordRefV3`与Candidate canonical digest；
6. exact `OperationScopeEvidenceProviderHandoffRefV3`；
7. exact Provider Attempt ref；
8. exact Enforcement 4.1 phase ref；
9. normalized canonical full `OperationScopeEvidenceScopeV3` digest；
10. Source/Event identity与Consumption Association必须和Record、Candidate、Qualification一致。

issued Qualification是Handoff/Consumption绑定的历史资格；final Qualification是Consume完成后的current终态。两者不是可互换副本，Gateway必须同时复读并验证同一资格的合法revision演进。

## 6. 权威流程

```text
Evidence Owner
  Issue -> InspectCurrent -> Handoff -> Provider -> Consume(atomic)
  -> consumed_current Qualification + Record + Consumption + Association

Runtime Settlement Gateway
  -> Inspect Effect revision/immutable owner/shared terminal guard
  -> Inspect exact prepare + execute Evidence bindings
  -> Inspect exact DomainResult authoritative current Fact
  -> rebuild canonical Submission V4
  -> Runtime Effect/Settlement Owner atomic Commit
       Settlement V4
       + Evidence Association V4
       + shared terminal guard
       + V4 terminal projection
```

Provider不由Settlement Gateway调用。Evidence不推进DomainResult或Settlement；DomainResult也不回写Evidence。

## 7. current与historical复读

Commit前必须current复读：

- Effect identity、revision与immutable Settlement Owner；
- shared terminal guard尚未被V3或V4占用；
- exact DomainResult Fact仍是对应领域Owner的authoritative current结果；
- Evidence Consumption、final Qualification、Record、Handoff、Attempt、4.1 phase及Association逐字段一致；
- prepare/execute集合canonical唯一、完整且属于同一Operation、Effect revision和Attempt链；
- full OperationScope normalized canonical digest一致。

Permit、Policy、Admission、Authorization和Qualification的历史TTL不作为truthful settlement的重新授权门。Gateway只验证它们在历史执行链中的exact引用和不可变关系；不得因为现在过期而否认已经发生的真实结果，也不得因为历史存在就允许新的dispatch。

## 8. 原子性与恢复

- Evidence Owner的Consume事务先完成；Runtime Owner不跨库回滚它。
- Runtime Owner的一次Commit必须原子产生Settlement V4、Association、guard和terminal projection；任何半写都属于Owner合同破坏并fail closed。
- Evidence已消费但Settlement未写是合法恢复窗口。Reconciler使用原Qualification、Consumption、Record、Handoff、Attempt、phase与DomainResult refs重新Inspect并提交，不重新调用Provider或重新Consume。
- Commit回包丢失后只按Settlement ID/Effect identity Inspect；同ID同canonical内容幂等返回原事实，同ID换内容稳定Conflict。
- V3与V4并发只有一个能占用shared terminal guard；失败方Inspect胜者，不得创建sidecar或第二终态。

## 9. 兼容性

- V3 Settlement公共类型、Port和digest保持字节级不变；
- Evidence V3、Dispatch V4.0、Enforcement 4.1公共类型和digest保持不变；
- V3路径继续通过原Port工作，但同一Effect Owner实现必须在V3 terminal CAS中复用shared terminal guard；
- 已经V3 settled的Effect视为guard已占用，无需修改旧Fact或补写V4 sidecar；
- V4 terminal projection是独立权威投影，legacy V3 reader不自动升级或声称V4结果；
- 首批只接收Evidence V3已冻结的三元矩阵：`OperationScopeKind=activation_attempt`、`PolicyProfile=praxis.runtime/activation-evidence`、`EffectKind in {praxis.sandbox/allocate, praxis.sandbox/activate, praxis.sandbox/open}`；其他scope、profile或kind全部fail closed。

## 10. NO-GO

- 用V2 Evidence Record数组、schema+digest或opaque ref伪装V3消费链；
- 缺少prepare或execute、phase交换、同一Consumption复用两次或多余phase；
- late/observation进入Settlement；
- Reader不可用、返回NotFound、漂移或type-pun时仍写Fact；
- V3 settled后追加V4 sidecar，或V4完成后把V3 Effect state标成settled；
- Application/组件持有裸Store接口绕过Governance Gateway；
- 把Evidence消费、Provider Receipt、DomainResult和Runtime Settlement宣称为跨Owner原子；
- 因历史Permit/Policy过期而重派Provider，或把settlement当新执行资格。

## 11. 设计资产

- [公共合同、Owner与原子边界](./contracts.md)
- [Additive Port Delta](./port-delta.md)
- [黑白盒、故障与并发测试矩阵](./test-matrix.md)
- [原始流程图](./operation-settlement-v4.drawio)
- [实施计划](../../../plan/runtime/operation-settlement-v4.md)

## 12. 完成状态与验证证据

最终实现严格保持V3 Settlement、Evidence V3、Dispatch V4.0与Enforcement 4.1公共结构和摘要不变，并闭合以下Owner语义：

- V3 terminal CAS与V4 Commit在同一Runtime Effect Owner锁内竞争`(TenantID, EffectID)` shared terminal guard；
- V3-first与V4-first对称互斥，即使Settlement ID、OperationDigest不同也不能形成第二终态或sidecar；
- 相同Effect ID在不同Tenant分区中独立，不发生跨租户互锁；
- Settlement、Association、Guard与Projection四对象按Settlement ID形成历史闭包；历史Guard Inspect不借用可变化的current-by-Effect索引；
- Commit采用copy-on-write staged publish，阶段1—5故障全部保持四对象全无，成功时全有；
- lost reply只Inspect历史/current exact闭包，同canonical内容幂等，换内容Conflict，Provider调用数保持零。

Owner自测与中央独立复验均通过full ordinary、full shuffle、full race、`go vet`、`gofmt -l`与`git diff --check`。中央高重复复验：`count=100` PASS（127.334s），`race count=20` PASS（238.537s）。独立Review最终裁决为YES。

这些结果只证明公共合同、reference Owner、并发线性化、故障恢复与Conformance语义；不声明生产数据库持久性、进程崩溃耐久性、availability或SLA。
