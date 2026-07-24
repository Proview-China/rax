# Runtime Checkpoint/Restore Governance V2

状态：**Checkpoint-first V2第三次独立代码终审YES（P0/P1/P2=0），本纵切完成**。结论只覆盖Runtime Checkpoint第一波reference纵切；真实Checkpoint/Restore Provider路径、Restore运行能力、production backend/root与SLA仍为unsupported，Provider调用数必须为0。

## 1. 目标与非目标

本设计冻结Runtime对Checkpoint Attempt、Barrier、Effect Cut、历史一致性与非成功终结的唯一Owner合同，并提出Participant Reservation、专用Evidence V1、Operation Settlement V5及Continuity ManifestSeal中立引用。

非目标：不修改Operation V3、OperationScope Evidence V3、Operation Settlement V3/V4、Dispatch V4.0、Enforcement 4.1、Run Settlement或legacy Checkpoint类型；不选择数据库、RPC、Provider、production root或SLA；本轮不实现Restore运行能力。

## 2. 五个P0冻结结论

1. `CheckpointAttemptFactV2 + BarrierLeaseFactV2`由同一Runtime Checkpoint Fact Owner一次原子create-once提交，并通过唯一bundle Inspect恢复；不存在持久Attempt却没有Barrier，也不公开独立`AcquireBarrier` mutation。
2. 成功路径由同一Owner原子执行`CommitConsistencyAndCloseBarrier`：写immutable Consistency、终结Attempt并关闭Barrier全有或全无。非成功路径先冻结Runtime Finalization Cut，再由Diagnostics Owner和Residuals Owner分别create-once immutable Seal；Runtime只协调Owner Port，绝不代写Seal。`CheckpointFinalizationInputClosureRefV2`只绑定两个typed Seal，之后`FinalizeAttemptAndCloseBarrier`派生`incomplete | aborted | indeterminate`。
3. 每个Participant phase必须先由Participant Owner create-once `ReservePhase`；Runtime fresh Inspect其current projection后，才允许Admission→Review/Authorization→Permit→Begin→prepare/execute。只有`prepared`可由共享branch guard线性化commit或abort之一；`failed|not_applied`不创建后继并直接进入finalization input，`unknown`只Inspect/Reconcile并最终进入`indeterminate`。
4. Runtime `CheckpointConsistencyFactV2`对Continuity唯一绑定immutable `CheckpointManifestSealRefV2`；mutable ManifestFact、candidate或RestorePlan不得进入Consistency。
5. live `core.CheckpointSet`、`core.RestoreRequest`、`CheckpointParticipantPort`及Foundation路径仅为legacy/restricted test compatibility；V2 public code、Owner store、Gateway与Conformance全部落地并获Review YES前，不能获得生产资格或接触Provider。

## 3. Owner矩阵

| 对象 | 唯一Owner | 禁止 |
|---|---|---|
| Attempt+Barrier bundle、EffectCut、Finalization Cut/Input Closure、Consistency、Attempt finalization | Runtime Checkpoint Fact Owner | Continuity/Participant创建Barrier、认证跨Owner闭包、关闭Barrier或设置consistent |
| ManifestFact、immutable ManifestSeal、RestorePlan | Continuity Owner | 宣布Runtime consistent、创建Eligibility或调用Provider |
| Phase Reservation、Participant Fact、DomainResult、ApplySettlement | 各Participant Owner | 写Runtime Attempt/Barrier/Consistency/Outcome |
| Snapshot Artifact Fact/Retention | 注册Artifact Owner | Receipt或裸storage handle升权 |
| Evidence qualification/handoff/record/consumption/cursor | Evidence Owner | Runtime/Application分配sequence或第二Ledger |
| Settlement V5与V3/V4/V5 shared terminal guard | Runtime Effect/Settlement Owner | Participant选择Runtime Outcome或Checkpoint结论 |
| Participant set、phase/effect/domain-result route certification | Assembly/Binding Owner | Runtime从Go registry或自报Descriptor动态发现required Participant |
| RestoreAttempt、Eligibility、新Instance/Epoch/Lease/Fence、Activate/Abort | Runtime Restore/Instance Owner（后置） | 本轮Checkpoint实现提前创建或启用 |

## 4. Checkpoint第一波固定顺序

```text
Runtime atomic CreateAttemptAndBarrier
  -> Runtime从frozen Barrier Policy派生并限幅Barrier expiry + Reconciliation deadline
  -> Runtime Freeze immutable per-attempt EffectCut
  -> for each required Participant phase:
       Participant Owner ReservePhase create-once
       -> Runtime InspectCurrent reservation
       -> Admission
       -> Review / Authorization
       -> Permit
       -> Begin
       -> prepare Enforcement
       -> execute Enforcement
       -> Provider Observation
       -> Evidence V1 consumed_current
       -> Participant DomainResult Fact
       -> Settlement V5
       -> Participant ApplySettlement
  -> Continuity ManifestFact CAS
  -> Continuity immutable ManifestSeal create-once
  -> Runtime reread exact Cut, Participant closures and ManifestSeal
  -> atomic CommitConsistencyAndCloseBarrier
```

任一required phase、Evidence、DomainResult、Settlement、ApplySettlement或ManifestSeal缺失时不得consistent。Finalize前Runtime冻结Finalization Cut；两个Owner Seal均精确绑定同一Cut、Owner identity、source epoch/sequence、ledger/root与complete-set digest，Runtime Closure只引用这两个Seal。Seal之后任何pre-cut publish必须Conflict；最终CAS前Runtime再次Inspect两个Seal current。caller最多提交expected set做exact compare，不能提交实际集合、Policy、Trigger或terminal state。已知缺口/failed走`incomplete`；prepare为`not_applied`只以confirmed-not-applied参与；只有`prepared`后的显式abort闭包可形成`aborted`；任何unknown在deadline前只reconcile，到创建Attempt时冻结的Reconciliation deadline必须原子形成`indeterminate+closed Barrier`。若Owner错误接受Seal后的pre-cut unknown，current terminal projection必须Fail Closed不可见；Cut之后才发生的Observation只追加审计。

Barrier与冲突dispatch必须线性化：Begin先胜则Effect必须进入Cut，Barrier先胜则后续Begin fail closed。Barrier只阻止Policy声明冲突的新dispatch，不证明quiesced或consistent。

## 5. Restore后置边界

Restore shape继续保留：historical Consistency、Continuity RestorePlan、RestoreAttempt、新Instance/high Epoch/new Lease/Fence Reservation、short-TTL Eligibility、Participant stage与Activate/Abort。但本轮不实现Restore Governance、Evidence current matrix、Settlement运行路由、Action route或Provider Adapter。

在后续独立联合Review YES前：

- `RestoreEligibilityFactV2`、`CheckpointRestoreOperationScopeV2` restore branch和restore phase只允许shape验证/文档引用；
- `ValidateCurrent`、Issue、Admission、Permit、Begin、Handoff、Settlement和Provider调用均返回unsupported/fail closed；
- historical Consistency或ManifestSeal不得被解释为current Restore资格；
- legacy RestoreRequest不得构造V2 Attempt、Eligibility、新Instance或Lease/Fence。

## 6. 恢复与原子边界

- 所有mutation使用确定性ID/idempotency key并先Inspect；只在明确NotFound时写；
- Unavailable/Indeterminate后以`context.WithoutCancel`执行exact Inspect，不能盲重发；
- lost-reply只接受同immutable identity、revision单调增加且位于同一状态分支的合法progressed successor；same revision换digest、revision回退、commit/abort旁支切换、历史ref覆盖或terminal重写均为Owner合同破坏并Fail Closed，禁止ABA；
- Attempt+Barrier、Finalization Cut+Attempt revision、Input Closure+Attempt binding、Consistency+closed Barrier、非成功Attempt+closed Barrier分别是单Owner原子边界；跨Participant、Continuity、Evidence、Settlement不宣称分布式事务；
- 同ID同canonical幂等；同ID换Scope、Run、Barrier、Generation、Participant set、ManifestSeal或terminal content为Conflict；
- Barrier/Reservation/治理current TTL在fresh check时满足`now < expires`。因`deadline <= expires`，`now < deadline`时Barrier必然仍current且unknown只Inspect/Reconcile；`now >= deadline`时按exact Closure强制`indeterminate+close Barrier`。不存在“Barrier已过期但deadline未到”；historical Fact仍可Inspect但不重新授执行资格；
- Unknown只Inspect原Attempt/phase/provider coordinate，不换Attempt、Effect、Barrier、Lease或Checkpoint ID。
- `InspectCheckpointAttemptV2`与Finalization Fact Inspect只证明historical存在；唯一current入口`InspectCheckpointAttemptTerminalCurrentV2`必须复读Closure与两个Owner Seal current。任一root/sequence/set漂移、Owner violation、typed-nil或Indeterminate都使current terminal不可见，但不改写historical审计事实。

## 7. 联合Owner前置

Checkpoint第一波仍依赖两条尚需联合Review的Owner链，Runtime文档只引用、不得复制其Fact Owner：

- Continuity immutable ManifestSeal Owner Port与Runtime 2.1.0 exact Reader Adapter已落地（见[Continuity Port Delta](../../continuity/port-delta.md)）；Runtime只消费完整Owner/Scope exact `CheckpointManifestSealRefV2`并在CAS前S1/S2，不得写`CheckpointManifestFactV2`或Seal Fact；该reference链不等于Harness/Application/Participant production装配；
- Sandbox等Participant必须保持`ReservePhase → Provider Observation → Evidence consumed_current → Owner Inspect/CAS DomainResultFact → Runtime Settlement V5 → Participant ApplySettlement`（联合前置见[Sandbox Checkpoint链](../../sandbox/workspace-checkpoint.md)），Provider Observation/Receipt不能越级成为DomainResult或Consistency。

任一Owner Port尚未冻结、Reader为nil/typed-nil、current投影不可判定或TTL已越界时，Checkpoint真实Provider路径保持unsupported/Fail Closed且Provider调用数为0。

## 8. 文档组成

- [contracts.md](./contracts.md)：类型、状态机、canonical、CAS和TTL；
- [port-delta.md](./port-delta.md)：Checkpoint-first公共签名候选与Owner边界；
- [test-matrix.md](./test-matrix.md)：联合实现硬反例；
- [checkpoint-restore-governance-v2.drawio](./checkpoint-restore-governance-v2.drawio)：Owner、阶段与事务边界；
- [实施计划](../../../plan/runtime/checkpoint-restore-governance-v2.md)：仅计划Checkpoint第一波，Restore后置。

## 9. 2026-07-16独立审计返修真值

- EffectCut terminal改为封闭tagged union，版本/类型与Disposition一一对应；unknown、alias、混合sidecar在Owner写入前拒绝。
- Consistency最终CAS前再次读取Effect inventory current，并与冻结Cut的root、watermark、count及规范化entry set逐项exact比较。
- 非成功Finalize与terminal-current派生只使用Attempt创建时冻结的deadline mode与confirmed-not-applied模式；后续Policy expiry/revoke/drift不阻断deadline收口。
- Evidence Qualification stable ref直接绑定Barrier、EffectCut、Reservation与full Scope digest；full Scope覆盖Operation/Effect/Admission/Authorization/Permit、双Enforcement、Assembly/Generation、Lease/Fence、Evidence Policy、source epoch/sequence及schema/payload。Runtime Gateway对Attempt closure、Attempt inputs、Reservation、Permit/Enforcement、Policy与source执行S1/S2 current复读，并以全部真实上界、Policy maximum TTL和30秒安全cap的最小值派生TTL。caller expiry仅可作exact expectation。Handoff/Consumption均有self digest、historical exact Inspect与current reread；lost reply只Inspect。
- `ExpectedRunRevision`由`CheckpointRunCurrentReaderV2`复读真实running Run及stable identity/scope后消费，错误revision/digest零Attempt写入。
- Create回包丢失允许同immutable identity的合法progressed successor，经初始Attempt/Barrier historical exact与逐revision transition lineage证明恢复；缺revision、非法跳转、ABA、revision rewind和identity drift仍Conflict。
- EffectCut entry外层EffectID/Intent revision/digest必须等于其typed Operation Attempt；Consistency两次inventory复读必须精确绑定完整Attempt/Barrier refs。Fact Owner正常回值及lost-reply Inspect均须与Gateway expected bundle canonical exact。
- terminal-current在任何backend读取前preflight完整consistent/non-success分支依赖；任一nil/typed-nil零backend read。
- terminal-current复读两个typed Owner Seal后重新派生非成功终态，错误历史终态保持historical但current不可见。

第三次独立代码终审结论为YES（P0/P1/P2=0）。最终返修另闭合V4 terminal exact Dispatch Attempt、EffectCut V2对无法证明exact Attempt的V5 terminal永久fail closed，以及Consumption通过真实Evidence Ledger Record Owner Reader执行S1/S2 exact复读。该YES不构成生产持久化、Provider、Restore、SLA或production root声明。
