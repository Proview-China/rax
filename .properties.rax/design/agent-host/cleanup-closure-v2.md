# Agent Host Cleanup Closure V2 设计候选

## 1. 状态与目标

- 状态：候选，待用户设计审核；未授权实现。
- 独立设计反审：`YES（P0/P1=0）`；该结论不替代用户审核。
- 目标：把 Start 实际选择、绑定且计划构造的完整对象集合，与唯一 `CleanupPlanV2` 在第一个Control construction前封存为Host-owned closure，防止Stop接受调用方裁剪或替换的清理计划。Closure不声称对象已经构造；实际构造进度仍由Host Journal和Owner facts证明。
- 上游设计：[H4 Production Lifecycle V2](h4-production-lifecycle-v2.md)；图形沿用 [H4 生命周期图](h4-production-lifecycle-v2.drawio)。

本 Delta 不修改 `HostJournalV2`、`SystemReadyV2`、`StartResultV2`、`CleanupPlanV2`、`CleanupAttemptV2` 或 Runtime Binding V1 的既有 canonical/digest。

## 2. 唯一 Owner 与原子边界

`HostCleanupClosureFactV2` 的唯一语义 Owner 是 Agent Host Cleanup Closure Fact Owner。它与 Host Journal 位于同一 State Plane、可由同一个 SQLite 实现提供，但使用独立表和独立 Port。

Closure Fact 内嵌唯一 `CleanupPlanV2`。同一 Store 同时实现 `CleanupPlanCurrentReaderV2`，避免出现第二个Plan仓。Embedded Plan+Closure是一行/一个事务；Journal intent、Ensure Closure、Journal result是三个独立线性点，只能靠exact Inspect恢复，绝不宣称跨Port原子。Production composition conformance必须证明Closure Port与Plan Reader指向同一Store/DB，拒绝两个SQLite文件拼接。

Harness、Runtime、Application、Control Adapter与组件只提供public exact refs。Host不根据这些refs猜Plan，而是消费已经验证的Cleanup Plan Template current。

## 3. 公共合同

### 3.1 Ref

```text
HostCleanupClosureRefV2
|-- closure_id = Derive(host_id, start_id)
|-- revision = 1
|-- host_id
|-- start_id
|-- plan_ref
|-- coverage_digest
`-- digest = Fact.ContentDigest
```

同一 `HostID + StartID` create-once。相同 canonical 内容幂等；相同 ID 任一字段漂移均为 Conflict。

`HostCleanupClosureFactV2`不内嵌Ref，避免摘要环：Fact以`ClosureID/Revision/.../ContentDigest`平铺保存；`ContentDigest`对除自身外的完整Fact body摘要；`RefV2()`从已Validate Fact派生上述Ref。不得同时再引入第二个FactDigest。

### 3.2 Assembly、Binding 与 Plan Template 坐标

`HostCleanupClosureAssemblyCoordinateV2` 完整绑定：

- ScopeRef、AssemblyInputRef、PublicationRef；
- GenerationRef、ManifestRef、GraphRef、HandoffRef；
- Harness Assembly OwnerCurrentRef。

这些字段必须逐项来自已经 Validate 的 `CompiledAssemblyArtifactsV2` 与 `AssemblyPublicationResultV2`，并交叉一致。

`HostCleanupClosureBindingCoordinateV2` 完整绑定：

- Binding AttemptID、RequestDigest；
- BindingSet exact ref 与全部 Binding refs；
- ResourceBindingSet exact ref；
- Checked/Expires 与 ResultDigest。

它只镜像 Runtime 已签发的 exact 结果，不复制或提升 Runtime Fact 语义。

`HostCleanupPlanTemplateCurrentV2`由Host cleanup orchestration compile Owner生成，输入是Harness Assembly Manifest中的Factory cleanup declarations、sealed Control Factory Registry cleanup routes、Runtime/Sandbox/Harness固定barrier adapters与ResourceBindingSet。每个route必须一一绑定exact Factory/Component/Artifact/Capability、CleanupContractRef、InspectPortBinding、request/result schema、resource class与依赖；未知、alias、缺失或多匹配均Fail Closed。Host只消费该exact template current，不从`ControlAdapterFactoryDescriptorV2`补猜缺失字段。

### 3.3 Control 与 Coverage

每个 `HostCleanupClosureControlCoverageV2` 至少包含：

- FactoryRef、ComponentID、ArtifactDigest、Capability；
- exact Binding、Generation、ResourceBindingSet；
- 全部 ResourceHandle refs；
- 该 Control Adapter 负责的 CleanupNodeIDs。

每个 `HostCleanupClosureCoverageEntryV2` 至少包含：

- SourceKind、SourceID、SourceRevision、SourceDigest；
- ComponentID、ResourceClass、CleanupNodeID。

Coverage 必须覆盖：每个 Binding Component、每个 planned Control Factory、每个 ResourceHandle，以及 `harness_close`、`sandbox_fence`、`sandbox_release`、`runtime_cleanup_aggregate` 四个固定 barrier。集合稳定排序、去重；`CoverageDigest` 对 canonical coverage 单独摘要。

### 3.4 Fact

```text
HostCleanupClosureFactV2
|-- contract_version
|-- closure_id
|-- revision
|-- start_claim_ref
|-- assembly
|-- binding
|-- cleanup_plan_template_current
|-- controls[]
|-- plan: CleanupPlanV2
|-- coverage[]
|-- created_unix_nano
`-- content_digest
```

`PlanRef`必须由内嵌Plan唯一派生。ContentDigest覆盖除自身外的全部字段；禁止自由map、隐式默认、重复集合、nil/empty双语义。

## 4. Port

```text
HostCleanupClosureFactPortV2
|-- EnsureHostCleanupClosureV2(ctx, exact FactV2) -> FactV2
|-- InspectHostCleanupClosureV2(ctx, exact RefV2) -> FactV2
`-- InspectHostCleanupClosureForStartV2(ctx, hostID, startID) -> FactV2
```

- `Ensure`：create-once；lost reply 后只 Inspect deterministic ClosureID。
- exact Inspect：same-ID revision/digest/Plan/Coverage 漂移为 Conflict。
- by-start Inspect：只负责定位，返回后仍须完整 Validate。

同一 Store 实现现有 `InspectCleanupPlanV2(ctx, PlanRef)`，返回内嵌 Plan 的严格深拷贝。

`HostV2StageInputsAssemblerV2` 仅新增纯函数式候选构造：

```text
BuildHostCleanupClosureCandidateV2(
  StartRequest,
  CompiledAssemblyArtifacts,
  AssemblyPublication,
  BindingResult,
  CleanupPlanTemplateCurrent,
  []ControlAdapterConstructRequestV2,
) -> HostCleanupClosureFactV2
```

它不写 Fact，不拥有 Plan，也不能补签 Owner current。

## 5. Start 与 Stop 时序

Start 插入点固定为：

```text
Binding exact settled
  -> Build/Validate complete Control Adapter requests
  -> Inspect exact Cleanup Plan Template current
  -> Build/Seal Closure candidate
  -> Journal cleanup-closure intent
  -> Ensure Closure
  -> lost reply: Inspect exact Closure
  -> Journal result_recorded(ClosureRef)
  -> first Control StartOrInspect
  -> Activation
```

Closure operation固定：`ContractKind=praxis.agent-host/cleanup-closure-v2`、`OwnerID=praxis.agent-host/cleanup-closure-owner`、`Current=false`、AttemptID由HostID+StartID+ClosureID确定性派生、RequestDigest绑定StartClaim/Assembly/Binding/Template/Control requests，Journal Inputs包含这些exact coordinates。ClosureRef只作为result coordinate记录。

Closure之前若已有Host-owned persistent resource Effect，则当前slice Fail Closed；该资源必须由独立Owner Effect/Cleanup链治理。后续Control/Activation的既有Owner request/digest不加字段；Host Journal对应operation Inputs必须包含同一Closure coordinate，且Host在每次调用前exact Inspect Closure。若未来要求Owner actual-point自行复读Closure，应新增版本化owner envelope，禁止修改既有DTO摘要。

Stop 请求中的 `CleanupPlanRef` 为兼容保留，但只作为 expected：

```text
Inspect Closure by HostID+StartID
  -> verify StartClaim and Journal result coordinate
  -> require request.PlanRef == Closure.PlanRef
  -> read embedded Plan from same Store
  -> verify coverage and currentness
  -> obtain fresh cleanup authorization/fence/current
  -> dispatch typed cleanup nodes with Closure targets
```

Legacy Start没有Closure时，Stop V2必须返回`cleanup_closure_missing`，不得回退到caller Plan、fixture或运行时重建。Closure result_recorded后，无论Host位于`constructing_control`、`activating`、`associating_generation`、`verifying`、`ready`或后续phase，Stop都必须允许进入draining/reconciling；不能只允许Ready之后清理partial Start。

现有`CleanupNodeRequestV2`不携Closure和完整target，只保留reference兼容。生产Stop使用additive `CleanupNodeDispatchEnvelopeV3`，至少绑定ClosureRef、PlanRef、NodeRef、closed `CleanupNodeTargetV3` tagged union、fresh cleanup authorization/fence current、predecessor barriers与TargetDigest。Owner通过public Closure/target Readers复读后才执行；Coverage本身不授dispatch。

`CleanupNodeTargetV3`按NodeKind和Host Journal实际phase只有三种互斥role：

- `constructed_target`：只携已经存在的exact target。`control_cleanup`要求ControlInstance+对应ResourceHandles；`harness_close`要求ExecutionOpen/Endpoint；`sandbox_fence|sandbox_release`要求Reservation/Lease，只有确已active时才允许ActivationRef；`runtime_cleanup_aggregate`只携已经存在的ActivationAttempt/Identity/Commit/Run refs。该NodeKind未列出的字段全部forbidden；
- `attempt_inspect`：只携该Owner已经write-ahead的stable Attempt/Intent/Operation exact refs，所有未知结果target字段forbidden。此role只允许Inspect，不允许cleanup dispatch；Inspect得到exact current后必须重新构造`constructed_target`，Unavailable/NotFound/Indeterminate保持unknown；
- `not_constructed`：只携Host Journal exact proof，证明对应Owner invocation从未进入`invocation_recorded`。所有Owner target/attempt字段forbidden，Host只记录operation_not_required，不调用Owner。

phase矩阵固定为：first Control前，Control/Harness/Sandbox/Runtime节点只能是`not_constructed`或已有ResourceHandle的`constructed_target`；Control已构造后仅对应Control节点可constructed；Allocate/Activate/Open各自在其invocation前not_constructed、invocation后结果未知时attempt_inspect、result_recorded后constructed。Activation/Sandbox/Execution refs从不作为Envelope通用必填值，也不得用空Ref、伪Ref或未来phase Ref占位。partial Start的Stop必须逐节点按该矩阵Seal并验证。

## 6. SystemReady 与兼容

当前 slice 不需要升级 `SystemReadyV2`。Closure 是 Start 安全侧车，且其 result 已由 Journal operation 记录。未来若独立消费者必须读取 cleanup posture，应新增 sidecar 或 `SystemReadyV3`，禁止给 V2 静默加字段。

## 7. 硬反例

1. 同 Host/Start 替换 caller PlanRef，zero dispatch。
2. 相同 ClosureID 替换 Assembly、Binding、Plan 或 Coverage，Conflict，原 Fact 不变。
3. Binding 任一 Component 没有 Coverage node，Ensure zero write。
4. Control Descriptor、ResourceHandle、Generation 或 Binding splice，Ensure zero write。
5. 四个固定 barrier 缺失、错 Owner、错 schema 或次序非法，Ensure zero write。
6. Ensure 提交后 lost reply，只 Inspect 原 Closure，禁止新 ID 或重建。
7. Journal intent 未确认或未 result_recorded，Control/Activation calls 为零。
8. Stop 的 ClosureRef、StartClaim、Journal result 坐标任一漂移，Conflict。
9. Cleanup registry 依据 Owner 字符串或错误 binding/schema type-pun，node calls 为零。
10. Closure后Binding/Control current过期或漂移，后续Start/正常Effect Fail Closed；historical Closure仍可用于Inspect/Settlement/Cleanup truth，但必须取得fresh cleanup authorization/fence，不能单独授dispatch。
11. pre-Activation partial Stop携空/伪Activation、Sandbox或Execution target，或把`attempt_inspect`当dispatch资格，zero Owner dispatch。
12. Journal已经invocation_recorded却声称`not_constructed`，或NodeKind与target role/字段不匹配，Conflict。

Residual、UnknownOutcome永不等于Closed，属于Stop本体的额外硬门。另测Closure已写但Journal result丢失的三写点崩溃、alternate Plan DB注入、historical TTL过期cleanup、partial Start各phase Stop、Coverage完整但target splice。

## 8. 完成条件

只有合同、SQLite Fact Store、Start write-ahead、Stop exact lookup、64 并发、lost-reply/restart、完整 coverage 和 typed cleanup system tests 全绿后，才可解除 StopV2 NO-GO。
