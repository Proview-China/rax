# Runtime Checkpoint/Restore Governance V2实施计划

状态：**Checkpoint-first V2第三次独立代码终审YES（P0/P1/P2=0），第一波实施完成**。本Plan不接Provider、不建立production root；Restore shape保留但运行能力全部后置且unsupported。

设计入口：[README](../../design/runtime/checkpoint-restore-governance-v2/README.md)、[contracts](../../design/runtime/checkpoint-restore-governance-v2/contracts.md)、[port delta](../../design/runtime/checkpoint-restore-governance-v2/port-delta.md)、[test matrix](../../design/runtime/checkpoint-restore-governance-v2/test-matrix.md)。

## 1. 第一波预计产物

- additive Checkpoint Governance V2公共refs、DTO、Fact Owner、Gateway、reference store与Conformance；
- 原子Attempt+Barrier bundle、immutable EffectCut、原子Consistency+CloseBarrier、非成功Finalize+CloseBarrier；
- Participant ReservePhase中立Ref/current Reader与checkpoint prepare/commit/abort三phase治理顺序；
- Continuity immutable ManifestSeal中立Ref/Projection/Reader；
- Checkpoint专用Evidence V1与Operation Settlement V5 checkpoint rows；
- V3/V4/V5同一OperationEffectStore shared terminal guard；
- unit/whitebox/blackbox/fault/race/fuzz/conformance资产；
- 不包含Restore runtime path、真实Provider、生产backend、Scheduler、RPC、SLA或composition root。

## 2. P0联合合同冻结

- [x] 三方确认Attempt+Barrier只能由一个Owner原子create-once，并删除独立AcquireBarrier mutation；
- [x] 三方确认成功终结为Consistency+Attempt consistent+Barrier closed同事务；
- [x] 三方确认非成功终结只由Runtime派生incomplete/aborted/indeterminate并与CloseBarrier同事务；
- [x] 三方确认Finalize只读取Attempt冻结Policy；Diagnostics/Residuals Owners按同一Finalization Cut分别create-once immutable Seal，Runtime Closure只绑定两个typed Seal；caller仅可提交expected set exact compare；
- [x] Participant Owners确认ReservePhase先于Admission→Review/Auth→Permit→Begin→prepare/execute；
- [x] Participant Owners确认仅`prepared`可创建commit XOR abort并绑定完整`PreviousPhase`；failed/not_applied无后继，unknown只Inspect/Reconcile；
- [x] 三方确认Create冻结可终结unknown的Reconciliation deadline，拒绝会造成Barrier expiry死锁的Policy；
- [x] Continuity确认Runtime Consistency只绑定immutable ManifestSeal exact ref；
- [x] Continuity确认ManifestSeal Owner Port/Reader与lost-reply恢复合同是Runtime实现前置；
- [x] Sandbox确认Provider Observation→Evidence consumed_current→DomainResultFact→Runtime Settlement V5→ApplySettlement链及Phase Reservation current Reader；
- [x] Assembly确认required Participant set、phase、EffectKind与DomainResultKind的认证来源；
- [x] Evidence/Settlement Owners确认专用V1/V5及V3/V4/V5 shared terminal guard；
- [x] 所有Owner确认legacy restricted、Review YES和V2 public code/Conformance前Provider=0；
- [x] Restore所有运行入口保持unsupported并后置到Checkpoint最终YES之后。

联合Design Review已完成并获得Checkpoint第一波代码授权；下方清单按live实现真值勾选。未勾选项仍为uunsupported、外部Owner前置或后续实施项，不得因本轮返修升权。

## 3. Checkpoint第一波实施顺序

### P1：public合同与canonical

- [x] `ports/checkpoint_governance_v2.go`：Attempt/Barrier/Cut/Consistency/Finalize refs、requests、bundles与Governance Port；
- [x] `ports/checkpoint_governance_v2.go`：Barrier Policy current projection、Owner派生Barrier/Reconciliation TTL、Finalization Cut、两个Owner Seal与Runtime Closure；
- [x] `CheckpointAttemptTerminalCurrentProjectionV2`与唯一`InspectCheckpointAttemptTerminalCurrentV2`；旧Attempt/Finalization Inspect显式historical；
- [x] `ports/checkpoint_participant_reservation_v2.go`：Reservation ref/current projection/Reader与三phase enum；
- [x] `ports/checkpoint_manifest_seal_v2.go`：Continuity中立Seal ref/projection/Reader；
- [x] strict Validate、canonical digest、nil/empty、bounds、clone与type-pun反例；
- [x] public Conformance shape与import-boundary检查；
- [x] legacy类型无法构造V2 ref的编译/运行反例。

### P2：Runtime Checkpoint Fact Owner

- [x] 同一Owner store/lock原子Create Attempt+Barrier；
- [x] 唯一`InspectCheckpointAttemptV2`恢复完整bundle；
- [x] Freeze EffectCut与Attempt revision同事务；
- [x] Freeze Finalization Cut与Attempt→finalizing_inputs同事务；create Closure与Attempt history binding同事务；
- [x] Commit Consistency+CloseBarrier同事务；
- [x] Finalize Attempt+CloseBarrier同事务，caller不提交terminal state；
- [x] Prepare Finalization Inputs原子冻结Cut，调用两个Owner create-once Seal，复读Seal current后create-once Runtime Closure；
- [x] 每个Owner Seal精确绑定Owner identity、source epoch/sequence、ledger/root、complete-set与同一Cut；Seal后pre-cut publish必须Conflict；
- [x] Seal mutation分别归Diagnostics/Residuals语义Owner；Runtime只协调sealing/current-inspection dependencies，不持或代写Owner Store；
- [x] Finalize最终CAS前复读两个Seal current，并覆盖B2后/Finalize后晚到pre-cut unknown、Owner违规导致current terminal不可见、fake-safe-cancel；
- [x] terminal current Inspect复读Closure+两个Seal current，覆盖historical存在但root/sequence/set漂移、typed-nil/Unavailable/Indeterminate不可见；
- [x] staged failpoint证明Attempt+Barrier、Cut、Finalization Cut、Input Closure、Consistency与Finalize各原子bundle全有或全无；
- [x] lost reply用`context.WithoutCancel` exact Inspect；
- [x] lost reply只接受同immutable identity/history、revision单调、同分支的合法progressed successor；禁止ABA、revision rewind和兄弟分支切换；
- [x] same-ID same-content幂等、changed-content Conflict；
- [x] 64并发同内容/换内容与Commit/Finalize首胜。
- [x] nil/typed-nil Store/Reader/Gateway/Clock在backend前Fail Closed且零半写。

### P3：Barrier与EffectCut线性化

- [ ] Barrier current Reader接入Issue/Begin/actual execution point；
- [x] Barrier expiry与Reconciliation deadline由Attempt冻结Policy限幅并checked派生；非正TTL、overflow、expected mismatch与Policy deny写前拒绝；
- [x] Barrier仍current且`now < deadline`时unknown只Inspect/Reconcile；`now >= deadline`必须原子indeterminate+close Barrier；删除不可达的“Barrier过期但deadline未到”；
- [ ] Begin先胜进入Cut、Barrier先胜拒绝Begin的并发反例；
- [ ] Effect inventory分页/root/watermark逐项Freeze；
- [ ] 每个已Begin Effect必须有Settlement、confirmed-not-applied或unknown inspection；
- [ ] unknown/Residual/excluded_by_policy exact refs；
- [x] Consistency最终CAS前复读Effect inventory current，并与Cut root/watermark/count/entry set exact。
- [ ] 其他未接线Owner的Freeze/最终Commit current重读仍为后续前置；漂移时必须reconcile而非漏项。

### P4：Participant ReservePhase与Assembly

- [ ] Participant Governance Port先create-once Reservation；
- [x] Runtime只读取Participant current projection，不持Participant Fact Store；
- [ ] 固定Reserve→Admission→Review/Auth→Permit→Begin→双Enforcement→Provider顺序；
- [ ] prepare/commit/abort分别独立Reservation/Operation/Effect/Attempt/Permit；
- [x] 冻结prepared→commit XOR abort状态机与exact `PreviousPhase`；failed/not_applied不创建后继，unknown只Inspect/Reconcile并最终indeterminate；
- [ ] Participant declaration/set certification/compiled route；
- [ ] required set冻结、Binding drift、hot plug、unknown required反例；
- [ ] Sandbox/Context/Memory/Knowledge等Owner Adapter缺失时保持unsupported。

### P5：Evidence V1与Settlement V5 checkpoint rows

- [x] Evidence V1只注册checkpoint prepare/commit/abort；restore current rows保持unsupported；
- [x] Qualification/Handoff/Consume exact绑定Reservation/Attempt/Barrier/Cut/phase/4.1；Qualification TTL由Gateway current复读后派生；
- [x] Issue零cursor，只有consumed_current可进入V5；
- [x] Settlement V5绑定typed DomainResult、Participant Fact与Evidence closure；
- [x] V5扩展现有OperationEffectStore同一实例/同一锁；
- [x] V3/V4/V5共享`(TenantID, EffectID)` guard；
- [ ] V3/V4/V5 64并发、same tenant/different operation与cross-tenant反例；
- [x] V5四对象+Effect terminal staged atomic failure；
- [x] historical truthful settlement与late/observation拒绝。

### P6：Continuity ManifestSeal与Consistency闭环

- [x] Continuity ManifestSeal Owner Port/Adapter提供immutable Seal exact Reader；Runtime 2.1.0完整Owner/Scope lookup、Participant mapping、Context/Artifact digest与S1/S2已落地；
- [x] ManifestFact/SealFact Owner保持Continuity，Runtime无写口；
- [x] Manifest状态只使用`collecting|verified_candidate|partial|indeterminate|rejected`，拒绝旧`verified`歧义值；
- [x] Runtime仅保存ManifestSeal ref并复读Participant/Cut closure；
- [x] mutable Manifest、candidate、RestorePlan、错误Owner/Scope/closure/digest与S1/S2漂移反例；
- [x] all-required闭合后原子Consistency+CloseBarrier；
- [x] Runtime Closure只绑定两个Owner Seal；typed classifications派生incomplete/冻结策略允许的aborted/unknown deadline indeterminate，terminal-current再次派生并exact核对；
- [x] Commit/Finalize lost reply与并发首胜；
- [x] Checkpoint public-only端到端Conformance。

### P7：完整Checkpoint门禁与资产收口

- [x] targeted `go test -count=100`；
- [x] targeted `go test -race -count=20`；
- [x] full Runtime ordinary/shuffle/race/vet；
- [x] canonical/state/CAS fuzz；
- [x] gofmt、XML、relative links、diff-check；
- [ ] 独立代码Review YES；
- [x] module/memory仅在真实实现完成后同步。

## 4. Restore后置清单（本Plan不实施）

以下内容只保留shape与依赖，不进入P1-P7：

- RestorePlan、RestoreAttempt、new Instance/high Epoch/new Lease/Fence Reservation；
- short-TTL RestoreEligibility及Action Admission；
- Review/Authorization、Permit/Fence、Begin、Participant stage；
- restore Evidence current rows、Settlement V5 restore rows；
- Activate/Abort与Context target materialization。

在Checkpoint实现、Conformance和独立Review最终YES后，必须重新进行Restore联合Design Review并获得单独代码授权。此前任何Restore current/Issue/Admission/Begin/Handoff/Settlement/Activate/Provider调用都返回unsupported且零写、零Provider。

## 5. 实施依赖

1. [Continuity ManifestFact/ManifestSeal](../../design/continuity/port-delta.md)语义、exact Reader与lost-reply合同获Owner Review YES；
2. Continuity ManifestSeal Owner Port明确create-once Seal与historical/current exact Inspect；Runtime不复制该Owner；
3. SnapshotArtifact Owner及各Participant DomainResult/current Reader由对应Owner冻结；[Sandbox Checkpoint链](../../design/sandbox/workspace-checkpoint.md)保持Reservation→Provider Observation→Evidence consumed_current→DomainResultFact→Runtime Settlement V5→ApplySettlement；
4. Assembly提供required Participant Set Certification与phase route current Reader；
5. Review Authorization V4、Dispatch V4.0、Enforcement 4.1、Generation-Binding Association保持live合同不变；
6. OperationEffectStore允许以additive V5在同Owner锁内扩shared terminal guard；
7. Action/Assembly只发布Port与Conformance；production composition仍由宿主Owner后续授权。

## 6. 退出条件

第三次独立代码终审结论为YES（P0/P1/P2=0），Checkpoint第一波纵切完成。最终闭合EffectCut outer及V4 nested Attempt、Consistency inventory与Owner回值exact、Run current revision、create transition lineage、Evidence Permit/Policy/source/schema/payload/TTL与真实Ledger Record S1/S2 current闭包、Handoff/Consume历史/当前恢复、terminal-current typed-nil preflight；EffectCut V2对V5 terminal永久fail closed。Restore、production root/backend/Provider仍需单独联合Design Review与代码授权。
