# Application Agent Activation V2 设计候选

## 1. 状态与结论

- 状态：候选，待用户设计审核；未授权实现。
- 独立设计反审：`YES（P0/P1=0）`；该结论不替代用户审核。
- 上游：[Agent Host H4](../agent-host/h4-production-lifecycle-v2.md)。
- 结论：additive `AgentActivationCoordinationCurrentReaderV1` 是必要的 Application 因果链 Reader，但不足以实现真实激活；必须新增 additive V2，不修改 V1 JSON、canonical、digest 或 fake/reference 行为。

V1 step 只有 ActivationID、StartDigest、Step、AttemptID、PredecessorDigest 和泛型 Owner current。它不能构造真实 Runtime/Sandbox/Harness 调用，也不能在回包丢失后读取权威结果。

## 2. Owner 边界

- Application：拥有 activation step journal、调用顺序、因果链和恢复决策。
- Runtime：拥有 ActivationAttempt、IdentityLease、ActivationAttempt中的Budget binding与ActivationCommit；Budget decision和not-required Policy仍由Budget/Policy Owner拥有。
- Sandbox：拥有 Reservation、Lease、Activation、Placement/Enforcement current。
- Harness：拥有 Preflight/Open/Ready 的执行表面事实与 endpoint current。
- Host：只调用 `StartOrInspectAgentActivationV2`，不编排八个 Owner 步骤。

Application 不签发 Runtime/Sandbox/Harness Fact，不把 Observation 升格为 current，不持有 Owner 写 Store。

## 3. 公共因果 Reader

```text
AgentActivationCoordinationCurrentReaderV1
  InspectAgentActivationCoordinationV1(ctx, activationID)
    -> full append-only AgentActivationCoordinationFactV1
```

`AgentActivationCoordinationFactPortV1` 可兼容嵌入该 Reader。每个 adapter 通过 full Fact 验证 ActivationID、StartRequestDigest、固定 Step/AttemptID、PredecessorResultDigest、InvocationEvent 与 unknown 状态；这只证明 Application 因果链。

## 4. V2 公共信封

`AgentActivationStepRequestV2` 是稳定 envelope，至少包含：

- CoordinationRef：ActivationID、revision、digest、StartRequestDigest；
- InvocationEventRef：sequence、digest；
- 固定 Step、AttemptID、PredecessorResultRef；
- closed `StepInputsV2` tagged union；每个步骤只允许自身输入 role；
- RequestedNotAfter 与 RequestDigest。

Preflight输入是pre-allocation `ProposedActivationScopeV2`：绑定tenant/identity/lineage/proposed InstanceRef+epoch/authority、Definition/Plan/Assembly/Binding exact refs、RequirementDigest与ProbeBudget，明确禁止SandboxLeaseRef。Preflight、Snapshot、Identity/Budget和Allocate只能使用该proposed类型；Runtime `ActivationCommit`保持同一Instance exact，原子激活Identity/Instance binding并附加SandboxLease，形成唯一完整`core.ExecutionScope`。Activate、Open和Ready必须从exact Commit predecessor取得同一个CommittedScope，禁止重新携带或从proposed值推导Lease。

Operation/Intent/Fence按步骤required/forbidden：Preflight、Snapshot、Identity/Budget禁止Provider dispatch refs；Allocate只在Identity/Budget current后由Runtime Effect/Governance Owner签发；Activate只在Commit current后签发；Open只在SandboxActive current后签发。Application只保存和转交exact refs，不预签或生成Fence。

`AgentActivationStepResultV2` 使用 closed role union；每个 role 只允许该步骤的 typed proof，所有 proof 都有 exact Ref、Checked/Expires 和 result digest。

## 5. 八步合同

| 步骤 | 调用 Owner 与输入 | typed result | 必需 authoritative Reader |
|---|---|---|---|
| Preflight | Harness Execution，ActivationSubject/Requirement/ProbeBudget；携proposed InstanceRef+epoch，仅无SandboxLease | Preflight report current | Preflight historical/current exact Reader |
| Snapshot | Runtime Admission，Preflight exact proof | ActivationSnapshot中立exact ref/digest + ActivationAttempt current | ActivationAttempt exact current Reader |
| Identity + Budget | Runtime Control/Budget，Snapshot/Authority/Policy | IdentityLease current + Budget current 或 explicit not-required Policy | IdentityLease、Budget/Policy exact current Readers |
| Sandbox Allocate | Sandbox，Identity/Budget predecessor + Reservation Intent/Fence/Attempt | Reservation current + LeaseRef | stable Allocate StartOrInspect/InspectExact Gateway + Reservation/Lease current Readers |
| Activation Commit | Runtime，Attempt revision + Lease revision + Authority epoch | committed Attempt + IdentityLease + 唯一完整ExecutionScope | narrow Commit StartOrInspect/InspectExact Gateway + Attempt/Lease Readers |
| Sandbox Activate | Sandbox，exact CommittedScope + activate Intent/Fence | SandboxActivation current | stable Activate StartOrInspect/InspectExact Gateway + Activation/Lease Readers |
| Execution Open | Harness，exact CommittedScope/SandboxActive/Requirement/open Intent/Fence | endpoint + ExecutionOpen current | stable Open StartOrInspect/InspectExact Gateway + Endpoint current Reader |
| Ready Inspect | Runtime Activation predecessor + Sandbox + Harness | Sandbox lease/active current + ExecutionReady current | ActivationAttempt、Sandbox与Harness exact current Readers |

Environment/Execution 的通用 Observation 不能单独成为这些结果。Application 的 `Ready Inspect` 只证明本次 Activation 已提交、Sandbox active且Harness execution ready；Host随后才聚合Generation与全组件`SystemReady`并发布既有Host-owned `AgentExecutionAvailabilityV1`。Availability不得进入Activation结果或形成循环。

跨Owner中立ref/projection由Runtime public `ports`作为唯一代码type owner；Runtime、Sandbox、Harness adapters把各自事实映射为这些nominal refs。Application合同只携中立exact refs以及Commit返回的`core.ExecutionScope`和Harness endpoint neutral ref，不复制`runtime/admission`、Sandbox或Harness事实DTO。type owner不改变领域Fact semantic owner。

## 6. 恢复时序

```text
append Application step intent
  -> CAS invocation_recorded
  -> only normal exact CAS reply owns Start right
  -> Owner StartOrInspect
  -> success: fresh Inspect exact current
  -> append typed result
  -> next step

lost/unknown
  -> append unknown event
  -> only Inspect same Attempt/Intent
  -> exact result visible: append recovered result
  -> unavailable/not found/indeterminate: remain inspect-only
```

Application step intent与Runtime EffectIntent/dispatch intent是两层不同write-ahead事实，禁止互相替代。CAS lost reply、Conflict、Unavailable、Indeterminate或Inspect恢复到`invocation_recorded`都不获得Start权，永久Inspect-only。

`ActivationCommit` 是不可跨越的线性化边界：Commit 前 Sandbox 只能 reserved_quarantined；Commit 后才能产生 activate Intent，SandboxActive后才能产生open Intent。Restore 或新 Instance 必须使用新 epoch/ActivationID，不能复用旧 effect identity。

## 7. 最小 Owner Port Delta 候选

Runtime需要中立nominal refs/projections与窄接口：

- ActivationAttemptCurrentReaderV2；
- IdentityLeaseCurrentReaderV2；
- BudgetBindingCurrentReaderV2；Budget decision与explicit not-required Policy继续由其领域Owner Reader提供；
- ActivationCommitStartOrInspectGatewayV2。

Sandbox需要：AllocateStartOrInspectGatewayV2、ActivateStartOrInspectGatewayV2、ReservationAttemptCurrentReaderV2、LeaseCurrentReaderV2、ActivationCurrentReaderV2。Harness需要：PreflightStartOrInspectGatewayV2、OpenStartOrInspectGatewayV2、PreflightCurrentReaderV2、ExecutionOpenCurrentReaderV2、ExecutionReadyCurrentReaderV2。stable Gateway必须能在尚无Lease或Endpoint时按Attempt/Intent exact Inspect，不能要求未知结果作为Inspect key。

这些只是待对应 Owner 审核的 Port Delta，不能在 Application 包复制 DTO 或私建 Reader。

## 8. 兼容与反例

- V1 保留给 owner-local/reference fixtures；生产 Host 只接受 V2 exact result。
- 新增Application-owned `AgentActivationVersionClaim`，stable key=`ActivationID`，绑定claimed ContractVersion、StartRequestDigest、CoordinationID、initial Fact digest与Created；Claim与初始Coordination Fact由同一Store/Owner在一个原子线性点create-once。V1/V2双向竞争、same canonical lost reply、wrong version/content、64并发均必须证明只产生一个版本；禁止先Claim后Fact两次写。
- 缺任一 Reader、Intent、Fence、typed proof 或 current window，零下一步调用。
- Preflight Observation 冒充 current、Budget Observation 冒充 decision、Environment Inspect 冒充 active 均拒绝。
- Allocate/Open 回包丢失后换 AttemptID 或盲重派，拒绝。
- Commit 使用 stale Attempt revision、Lease revision 或 Authority epoch，拒绝。
- Open endpoint 未绑定 stable attempt/current，Ready 零写。
- 三方 Ready 任一漂移、过期、时钟回退，Activation result 零写。
- 64 个 Coordinator 共享 Fact Store，每个 exact step 只允许一个正常CAS Start权。
- Application adapter 导入 Runtime/Sandbox/Harness 实现包或写 Store，import/conformance 拒绝。

测试编号至少覆盖：AAV2-01 proposed/committed Scope type-pun；AAV2-02 pre-Commit Activate/Open；AAV2-03 unknown Allocate无Lease；AAV2-04 unknown Open无Endpoint；AAV2-05 Availability循环；AAV2-06/07 V1/V2双向Claim；AAV2-08 Observation冒充Fact；AAV2-09 S1/S2 TTL crossing；AAV2-10 clock rollback；AAV2-11 invocation CAS lost reply；AAV2-12 64独立Coordinator。

## 9. 完成门

八步 typed Owner contract、durable Application journal、所有窄 Readers、lost-reply/restart/64并发和production Host Service V3生命周期接线全部通过前，Agent Activation production 仍为 NO-GO。HostV2只保留reference conformance，不作为production完成证据。
