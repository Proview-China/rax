# Tool/MCP结构化Port Delta

状态：既有G6A所需Runtime/Application/Model V1公共Delta已落盘，Tool G6A V2 Owner-local隔离闭环第三轮独立审计最终YES（P0/P1/P2=0）。Runtime ports唯一Assembly Current/RegistrySnapshot public Go nominal与Reader已落盘；Tool P4-0/P4-1/P4-2 Owner-local实现与software test已闭合。Harness M2、跨Owneractual-point四读、system/production仍NO-GO。

## PD-TM-01：N=1 Tool Action Gateway公共接线

状态：Tool侧合同与G6A公共门已冻结，Tool V2 Adapter/Owner flow隔离实现与测试已完成。G6B公共总装未验收前，production Provider能力、Context/Harness调用、Continuation与Turn推进仍为NO-GO。本Delta不授权Tool/MCP私建Runtime、Application或Harness兼容接口。

### 用例

Model Owner产生并持有`ToolCallCandidateObservationProjectionV1`。模型Operation已Settlement、Harness Owner已提交对应`PendingActionV2`后，Application DTO只传Model Projection exact Ref；Tool经Model公开Reader复读完整Projection并验证唯一Call，才可形成Watermark、Candidate与Reservation。G6A隔离链的Provider治理、typed DomainResult、Runtime Settlement V4及Tool Apply/Result已经实现；G6B Context/Harness链仍未启用，`N > 1`、batch与custom effect继续NO-GO。

补充消费门：上述Model事实不得由Application DTO或PendingAction代替。Application DTO只传Model Owner已公开的`ToolCallCandidateObservationProjectionV1` exact Ref；Tool `applicationadapter`必须先经Model公开只读Reader复读完整Projection，验证Ref全字段、Observation digest与`Calls == 1`，成功后才可创建Watermark/Candidate/Reservation。Reader unavailable/indeterminate、漂移或基数错误时零Tool写、零Gateway/Provider。

### 语义Owner

- Model Invoker：`ToolCallCandidateObservationProjectionV1`、publish/write与已公开exact Reader；不拥有PendingAction、Tool Action或Result；
- Harness：已Settlement模型轮次、PendingAction、Session与Continuation CAS；
- Tool/MCP：Action Candidate、Reservation、Provider Observation/Receipt的独立Inspect、typed DomainResult、ApplySettlement和ToolResult；
- Application：`SingleCallToolActionPortV1`及其窄Request/Result DTO、`N == 1`基数门、跨域编排、引用传递与恢复驱动；Application不导入Tool实现；
- Harness Assembly Adapter：把Harness公开对象投影成Application DTO；不导入Tool实现；
- 未来宿主production composition root：由宿主Owner在G6B前构造Tool实现并注入Application公开Port；live当前不存在，G6A仅用test fixture手工注入；
- Runtime：Intent/Admission/Permit/Begin、Enforcement 4.1、Evidence V3、Settlement V4与Fence线性化；
- Review：Verdict。

### 输入

- `SingleCallToolActionRequestV1`只携带Model Projection exact Ref；完整`ToolCallCandidateObservationProjectionV1`及唯一Call的Ordinal、CallID、Name、CanonicalArguments必须由Model公开Reader返回；
- 由Harness Assembly Adapter填充的已Settlement模型Operation、Run/Session/Turn、PendingAction exact projection及原始`RequestDigest`；Tool不导入Harness类型；
- PendingAction的Capability、Payload、SourceCandidate ID/Revision/Digest；
- Tool Surface、Capability、Tool、Input Schema与Provider Binding的exact ID/Revision/Digest；
- Tool Candidate/Reservation current ref/digest、中立`ApplicationAttemptRefV1{ID,Revision,Digest}`、IntentDigest、Session与Domain Subject；Reservation不含Runtime Dispatch Attempt；
- `OperationScopeKind=run`、`EffectKind=praxis.tool/execute`、`PolicyProfile=praxis.tool/single-call-action-v1`；
- Run、Session、Turn、Action、Context五个Applicability Fact的exact ID/Revision/Digest与currentness；
- Runtime V4.0 Intent/Admission/Permit/Begin、V4.1 prepare/execute Enforcement及同一Operation/Attempt/Fence；
- `ProviderAttemptObservationRefV2`，prepare/execute各自独立的`OperationDispatchEnforcementPhaseRefV4`与`OperationScopeEvidenceConsumptionRefV3`；不得以字符串Receipt或Opaque JSON替代；
- Runtime `OperationSettlementRefV4`及当前`OperationInspectionSettlementRefV4`，后者精确闭合Association/Guard/Projection/DomainResult。

### 输出

- create-once `ActionCandidateRef`与`ActionReservationRef`；
- prepare/execute两个不可互换的`OperationScopeEvidenceConsumptionRefV3`；
- typed authoritative `ToolDomainResultFactRefV2`；
- Runtime `OperationSettlementRefV4`与经公共Inspect得到的`OperationInspectionSettlementRefV4`；
- `ToolApplySettlementFactRefV2`与关闭`OperationSettlementDomainResultFactRefV4`、`OperationSettlementRefV4`、`OperationSettlementEvidenceAssociationRefV4`、`OperationSettlementTerminalGuardRefV4`、`OperationSettlementTerminalProjectionRefV4`及ApplySettlement的`ToolResultRefV2`；
- G6A结果DTO：仅含settled `ToolResultV2 + OperationInspectionSettlementRefV4`；Tool Port不返回Context、Harness、Capability activation、`ContinuationRefV2`或Turn字段；
- Context Owner对pending DomainResult完成S2，并在本地原子边界提交ApplySettlement+Generation current CAS后产出的new exact FrameRef/Digest；仅Application可在此后调用Harness公开Port，Continuation的Settlement字段取Tool Inspection.Settlement，Evidence字段取Tool Inspection.Association，并绑定新Frame；
- 仅由Harness Owner CAS后的下一Candidate/Session。

### 不变量

1. `PendingActionV2`只是Candidate来源，不是Intent或Verdict；
2. `ActionCandidate.PendingActionDigest`必须精确等于Harness `PendingActionV2.RequestDigest`，且Ref、Capability、Payload、SourceCandidate全部一致；
3. Application不能直接写Harness或Tool领域事实，也不得导入Tool实现；Tool只能依赖Application公开`contract/ports`，不得依赖Application实现；
4. Tool/MCP不能推进Harness Session；
5. Candidate与Reservation每次使用前必须按Owner current reader复读；Candidate current expiry取requested expiry与PendingAction/Surface/Capability/Tool/InputSchema/SourceCandidate六项`OwnerCurrentRefV1` expiry最小值，Reservation必须`<=`该上界；
6. 同一PendingAction revision只有一个获胜Reservation/Attempt；
7. Candidate/Reservation不授执行权；Runtime V4.1对应phase Enforcement及Evidence handoff前Provider对应phase调用数必须为零；
8. prepare与execute必须使用不同Qualification、Handoff、EventID、SourceSequence、ConsumptionID；不得复用资格、Handoff或phase；
9. `OperationScopeEvidenceCandidateV3`只携带Qualification与Observation内容；Handoff只出现在独立`ConsumeOperationScopeEvidenceRequestV3`，不得伪造进Candidate；
10. Provider事实只以`ProviderAttemptObservationRefV2`进入；Tool Owner必须复读Observation、prepare/execute两项Enforcement、两项Consumption及Action/Reservation/ApplicationAttempt因果链后CAS typed DomainResult；
11. Settlement V4 Submission必须exact引用同一DomainResult Fact与两项`OperationSettlementEvidenceBindingV4`；公共Inspect必须返回同一Settlement的Association/Guard/Projection/DomainResult closure。Runtime只投影settled，不选择领域Outcome/Disposition；V3 Settlement不得包装扩权；
12. ToolResult exact ref必须闭合DomainResult、Settlement/Association/Guard/Projection与ApplySettlement；G6A必须通过Runtime Application-facing只读Association Inspect确认prepare/execute完整关联，输出后硬停止，不得调用Context/Harness或形成Continuation；
13. Begin或任一Provider phase后任何不确定恢复只能Inspect原Attempt；Unknown/indeterminate不能Apply或形成ToolResult；
14. Model/Provider Observation不能直接构造ToolResult；Tool/Provider/Application都不能在Context Apply+Generation CAS+S2前构造Continuation；
15. 输入基数必须精确为1；`N > 1`、batch、custom effect拒绝且零状态变化。

### 冻结调用序列与原子边界

```text
Model ToolCall Observation (exactly one)
  -> Harness Owner settles model operation and CAS PendingAction/waiting_action
  -> Application N=1 gate
  -> Model exact Reader validates full Projection Ref/digest/Calls == 1
  -> Tool create/Inspect canonical-command Watermark
  -> Tool PutCandidate CAS -> Reserve CAS
  -> Runtime Intent -> Admission -> Permit -> Begin
  -> Runtime Enforcement 4.1 prepare
  -> Evidence V3 prepare Issue -> InspectCurrent -> Handoff
  -> controlled Provider Prepare
  -> Evidence V3 prepare Candidate -> Consume(独立Handoff)
  -> Runtime Enforcement 4.1 execute(exact prepare receipt/prepared attempt)
  -> Evidence V3 execute Issue -> InspectCurrent -> Handoff
  -> Tool CAS provider_boundary_crossed with same-Attempt execute Enforcement + Handoff refs
  -> controlled Provider Execute，或Inspect原Attempt
  -> Evidence V3 execute Candidate -> Consume(独立Handoff)
  -> Tool Owner Inspect current facts/receipt -> CAS ToolDomainResultFactV2
  -> Runtime Settlement V4（只绑定typed DomainResult并投影settled）
  -> Runtime InspectCurrentOperationSettlementV4
  -> Runtime Application-facing readonly InspectOperationSettlementEvidenceAssociationV4
  -> Tool ApplySettlementV4 CAS -> settled ToolResultV2 + Runtime current closure
  == G6A hard stop: zero Context/Harness/capability enable/Continuation/Turn progress ==
  == G6B consumes accepted G6A output ==
  -> Application调用ContextTurnRefreshPortV1
  -> pending Context DomainResult -> S2 current reread
  -> Context Owner local atomic ApplySettlement + Generation current CAS
  -> new exact FrameRef/Digest
  -> Application携带new exact FrameRef调用Harness公开Continuation Port
  -> Harness Owner复验并CAS下一轮
```

每个箭头只传不可变exact ref/digest。Harness、Tool、Runtime各自CAS是独立原子边界；Application不跨Owner事务写Fact。

### Application-owned `SingleCallToolActionPortV1`与Tool实现

Application发布`SingleCallToolActionPortV1`、`SingleCallToolActionRequestV1`和`SingleCallToolActionResultV1`。Tool Owner可在`tool-mcp`内提供实现，只依赖Application公共`contract/ports`。live当前无production composition root；G6A由test fixture手工注入，未来production root由宿主Owner在G6B前闭合。Application/Harness不反向import Tool实现，Tool不负责总装。

Application冻结的`Execute`语义在Tool侧映射为同一canonical command的`start-or-inspect`。Adapter在任何Tool写入前，先按DTO中的exact Ref调用Model已公开只读Reader；完整Projection的Ref全字段、Observation digest与`Calls == 1`验证通过后，才以`TenantID + ApplicationRequestID + ApplicationRequestRevision + OperationScopeDigest`稳定键create/Inspect Tool Owner Watermark。Watermark exact绑定Model Projection与canonical command；漂移即冲突。它不是Permit、Runtime Attempt或Provider Receipt。

| 操作 | 输入 | 输出/约束 |
|---|---|---|
| `Execute`（Tool实现入口） | Application Request ID/Revision/Digest、Tenant/Scope、Model Projection exact Ref、PendingAction projection及canonical command其余输入 | 先调用Model Reader并验证Ref全字段/Observation digest/`Calls == 1`；失败时零Watermark/Tool事实/Provider；通过后才start-or-inspect |
| `AdmitSettledPendingAction` | Reader返回的完整Model Projection、模型Settlement、Run/Session/Turn、完整PendingAction projection与Surface/Capability/Tool/Schema exact refs | 唯一Call与全部摘要重算一致后create-once Candidate；不得从Application DTO、PendingAction/event JSON/compat calls补造Model事实 |
| `ReserveCurrentCandidate` | Candidate expected revision/digest、`ApplicationAttemptRefV1`、IntentDigest、Session、Subject、`now`与TTL | Owner复读全部current上界；Reservation expiry晚于最小Candidate current expiry时零写；禁止Runtime Attempt字段 |
| `ProjectActionApplicability` | Candidate/Reservation exact refs | 只形成Action维度projection，不签发Evidence |
| `InspectOwnerCurrent` | Candidate/Reservation exact keys与来源current refs | Candidate projection expiry取全部上界最小值；支持S1/S2复读 |
| `CrossProviderBoundary` | 同一Runtime Attempt、current execute `OperationDispatchEnforcementPhaseRefV4`、current execute `OperationScopeEvidenceProviderHandoffRefV3` | 当前V1路径不得调用；未来仅由Runtime V2 actual-point链消费。已存在boundary视为可能已调用并只Inspect |
| `RecordTypedDomainResult` | Reservation/ApplicationAttempt、Runtime Operation/Attempt、`ProviderAttemptObservationRefV2`、prepare/execute Enforcement与Consumption、schema/payload/residual | Tool Owner独立Inspect后CAS历史权威`ToolDomainResultFactV2`；Runtime Attempt首次在此绑定且必须源自同Reservation链 |
| `InspectDomainResultCurrent` | DomainResult exact key、`now`、requested TTL | Tool Owner签发`>0 && <=30s`短租约前复读Observation、两phase Enforcement、两Consumption及因果链；30s非SLA |
| `ApplySettlementV4` | DomainResult exact ref、fresh current `OperationInspectionSettlementRefV4`及五项typed refs、Tool Outcome/Disposition、expected Tool revision | Association Inspect闭合后仅接受三种合法组合；Unknown/indeterminate、`succeeded+confirmed_not_applied`零写 |
| `InspectAttempt` | 原Operation/Attempt/phase/provider attempt exact refs | lost reply/Unknown只Inspect；不重新Prepare/Execute |

### Current Reader与S1/S2

- Model narrow reader：由Model Owner另行冻结公开exact只读合同；Tool只按完整Projection Ref读取并验证Ref全字段、Observation digest、`Calls == 1`，不持有publish/write口。Reader unavailable/indeterminate或漂移时不进入Tool Watermark。
- Harness narrow reader：Run、Session、Turn及已提交PendingAction；S1读取后，Tool Admission前S2复读同revision/digest/current window；Observation lease不跨S2授予执行权。
- Tool narrow reader：Action Candidate与Reservation；Candidate expiry取requested expiry与六项固定`OwnerCurrentRefV1`最小值，Runtime Issue/Enforce前S1读取，Provider handoff前S2复读；lease仅覆盖本次handoff。DomainResult Fact为历史truth，其current projection由Owner签发最长30秒短租约并重新验证完整因果链，不以Reservation当前过期作为否定依据。
- Context narrow reader：Context generation/frame、manifest、injection/conformance exact facts；Evidence Issue前S1、handoff前S2；无法物化时Fail Closed或产生Plan允许的Residual。
- Runtime applicability router按Fact kind路由上述Reader；Tool/MCP不实现Runtime kernel或跨Owner聚合Reader。

### Effect与Recovery

- 本Delta只允许`praxis.tool/execute`；Cancel、Remote Inspect、batch/custom effect是独立Operation/Delta，不继承本Permit；
- pre-provider拒绝若需形成终态，必须先由Tool Owner在独立Inspect后产生typed DomainResult/领域Outcome，再由Runtime Settlement V4绑定并投影settled；Runtime不表达confirmed-not-applied/failed；Tool Apply完成后仍必须走Context Refresh；
- pre-provider确定拒绝可形成`failed + confirmed_not_applied`；Provider错误但已执行可形成`failed + confirmed_applied`；成功只允许`succeeded + confirmed_applied`；
- Provider Prepare/Execute丢回包或Unknown只Inspect原attempt；需要远端Inspect时另起受治理Effect；
- 每次首次执行或pre-provider恢复都先通过同一Model exact Reader门；不得从Application中立DTO、PendingAction payload、event JSON或compat tool calls反推/复制Model事实；
- Adapter丢回包或Application重复投递先Inspect Tool Owner Watermark。Provider边界前，只有Watermark current exact且下一阶段Owner事实权威返回`NotFound`时，才可继续同canonical command；`Unavailable`、`Indeterminate`、超时或投影缺失都必须停止；
- 当前V1在`runtime_attempt_bound`停止且不CAS新boundary；已实现的Runtime V2隔离链中，boundary一旦CAS成功即视为可能已调用，回包丢失或之后崩溃一律Inspect原Attempt；
- prepare/execute Consumption只在Provider响应/Observation后进入DomainResult链，禁止在boundary预填；
- 不承诺消息/RPC/Provider transport exactly-once；只承诺canonical command create-once/CAS幂等和Provider未知时不盲重派；
- Evidence Issue/Handoff/Consume丢回包分别以同一Qualification、HandoffID、ConsumptionID及Candidate digest Inspect/幂等重放；
- Harness/Application/Tool/Runtime CAS丢回包均按精确ID+Revision/Digest Inspect。
- Context Refresh/pending DomainResult/S2/local atomic ApplySettlement+Generation current CAS任一点丢回包只Inspect原Context attempt/fact；无new exact Frame时Harness continuation零写。Context链不创建Runtime Settlement。

### 错误分类

| 条件 | 分类 | 稳定Reason | 状态变化 |
|---|---|---|---|
| 缺字段、非规范摘要、错误schema/payload | `invalid_argument` | `invalid_canonical_form` | 零写/零Provider |
| Model Reader unavailable/indeterminate、Projection Ref/Observation digest漂移或`Calls != 1` | `unavailable`、`indeterminate`、`conflict`或`invalid_argument` | `owner_inspection_unavailable`/`evidence_conflict`/`invalid_cardinality` | 零Watermark/零Candidate/Reservation/零Provider |
| Model `CanonicalArgumentsDigest`与PendingAction/Candidate `PayloadDigest`不相等 | `conflict` | `evidence_conflict` | V1零Watermark/零Candidate/零Gateway/零Provider；不推断Transformation |
| 合法Observation但`N != 1`、batch、custom effect | `forbidden` | `unknown_governance_category` | 零写/零Provider |
| 同ID/Ref换payload、capability、schema、source或Evidence Candidate | `conflict` | `idempotency_payload_mismatch`/`evidence_conflict` | 保留原事实 |
| Candidate/Reservation/Session/Context尚未生效、过期、撤回或S1/S2漂移 | `precondition_failed` | `effect_fence_stale`/`capability_expired`/`evidence_unavailable` | 零新写/零Provider；post-Tool Apply时零Continuation |
| Permit、V4.1 Enforcement、Handoff或current Authority/Scope/Budget缺失 | `forbidden` | `effect_authorization_missing`/`dispatch_permit_invalid` | 零Provider |
| execute Enforcement/Handoff过期、换摘要、跨Attempt/phase或Handoff Fact不绑定同一phase | `precondition_failed`或`conflict` | `effect_fence_stale`/`evidence_conflict` | 零boundary CAS/零Provider |
| Boundary Reader NotFound/Unavailable/Indeterminate、source/projection漂移或expired | `not_found`、`unavailable`、`indeterminate`、`conflict`或`precondition_failed` | `owner_inspection_unavailable`/`evidence_conflict`/`effect_fence_stale` | 零Provider；只Inspect同source/attempt |
| Owner CAS revision不符 | `conflict` | `revision_conflict` | 零覆盖 |
| Provider已接触但交付不确定 | `indeterminate` | `effect_unknown_outcome` | 原Attempt保持Unknown，只Inspect |
| Watermark/下一阶段事实读取`Unavailable`、`Indeterminate`或超时 | `unavailable`或`indeterminate` | `owner_inspection_unavailable`/`effect_unknown_outcome` | 不得当作NotFound；零新写/零Provider |
| 同Application Request ID/Revision换Digest、Scope或CanonicalCommandDigest | `conflict` | `idempotency_payload_mismatch` | 保留原Watermark；零Provider |
| Outcome/Disposition非法组合或Unknown尝试Apply | `precondition_failed` | `invalid_transition` | 零Apply/零ToolResult |
| DomainResult current TTL为零、超过30秒或因果复读漂移 | `precondition_failed` | `effect_fence_stale`/`evidence_conflict` | 不签发current projection；历史Fact保留 |
| Settlement V4 Owner/Attempt/DomainResult或Inspection Association/Guard/Projection不一致 | `precondition_failed`或`conflict` | `settlement_owner_mismatch`/`settlement_evidence_mismatch` | 零Apply/零Continuation |

错误必须携带原Operation/Action/Attempt/Fact key以便Inspect，但不得把Provider文本或legacy错误对象提升成权威分类。

### 反例

- 直接调用legacy `ActionPort.Execute`后把`ActionObservation`回注Harness；
- 只比较PendingActionRef，不校验RequestDigest或允许同Ref换Payload；
- 把Candidate/Reservation/current authorization或Evidence Qualification当作执行权；
- 把Handoff塞入`OperationScopeEvidenceCandidateV3`，或prepare/execute复用Qualification/Handoff/Consumption；
- Watermark boundary缺execute Enforcement/Handoff、二者换摘要/跨Attempt/非current，或Handoff Fact不绑定同一execute phase仍CAS/调用Provider；
- boundary CAS前预填prepare/execute Consumption，或CAS成功后崩溃仍再次Execute；
- Runtime未调用boundary current Reader，或Reader结果unknown/unavailable/drift/expired仍调用Provider；
- 同Boundary Ref ID/Revision换Digest、cross-Attempt、把其他三字段Ref type-pun为Boundary Ref，或把Projection当Authority/替代Enforcement/Handoff；
- Runtime持久化对应phase Enforcement或Evidence handoff前调用Provider，或Tool私建V4.1到旧执行seam的转换；
- 根据`ReviewRequired=false`跳过Verdict；
- Provider返回`success`后直接写DomainResult、Settlement、ToolResult或Continuation；
- Tool DomainResult没有exact绑定Reservation、Attempt、两次Consumption与schema/payload digest；
- 用V3 Settlement、`EvidenceRecordRefV2`或Provider Receipt包装出Settlement V4/Evidence V3；
- ToolResult只引用DomainResult或Settlement而未闭合ApplySettlement；
- `succeeded + confirmed_not_applied`，或把Unknown/indeterminate映射成Apply/ToolResult；
- Reservation携带或事后回填`OperationDispatchAttemptRefV3`；DomainResult不能证明Runtime Attempt源自同Reservation/ApplicationAttempt；
- 用字符串ReceiptRef、Opaque JSON或Provider文本替代`ProviderAttemptObservationRefV2`、两phase Enforcement与两Consumption；
- Candidate忽略PendingAction/来源current expiry，Reservation越过最小上界，或DomainResult current lease超过30秒；
- truthful late DomainResult仅因历史Reservation此刻过期被删除/拒绝，或未复读因果事实就签发current projection；
- Tool Port保留`BuildContinuation*`，Tool Apply后直接写Continuation，或Application跳过`ContextTurnRefreshPortV1`；
- G6A调用Context/Harness、注册或启用Provider能力、构造Continuation或推进Turn；
- G6A只看current Inspection而不调用Runtime Application-facing只读Association Inspect，或Association不能exact证明prepare/execute独立且属于同一Attempt；
- Application或Harness import `tool-mcp`实现，Tool import Harness/Context/Application实现，或Tool承担composition root总装；
- G6B未验收却启用Provider能力、写Continuation或推进Turn；
- pending Context DomainResult、S2、Context Owner本地原子ApplySettlement+Generation current CAS未完成，或没有new exact FrameRef时调用Harness continuation；
- 用新的Action Attempt重试Unknown；
- Application重复投递时跳过Watermark，或同Request ID/Revision换Digest/Scope/canonical command仍继续；
- Tool未调用Model exact Reader，或从Application DTO、PendingAction payload、event JSON、compat tool calls反推/复制Model Observation；
- Reader Ref任一字段/Observation digest漂移、Reader unavailable/indeterminate或`Calls != 1`仍写Watermark/Candidate/Reservation；
- Tool持有或调用Model Projection publish/write Port；
- 把`Unavailable`、`Indeterminate`、超时或读模型缺失当成`NotFound`并创建下一阶段事实；
- crash-before-first-provider-call未复读Watermark current exact/下一阶段权威NotFound就继续，或Provider边界后因重投再次调用Provider；
- 宣称消息、RPC或Provider transport exactly-once，而不是限定为canonical command幂等与未知不盲重派；
- 把`N > 1`拆分执行、选首项或形成部分Reservation/Provider调用。

### 兼容影响

Application Owner已发布窄Port/DTO，Tool侧已实现该Port。live当前无production composition root，G6A仅以test fixture手工注入；未来root由宿主Owner在G6B前闭合。Application与Harness不得反向依赖Tool实现。Runtime public V2 actual-point合同与Tool隔离接线已闭合；V1路径固定unsupported。production端到端另依赖Identity/Assembler、Context Refresh、Harness continuation与总装。legacy `ActionPort`/`ToolPort`/`MCPPort`、`GovernedExecutionProviderV2`与V3 Settlement继续可读，但不得包装成新链或宣称完整Governed语义。`N > 1`、batch、custom effect需要另立、另审公共语义。

### Runtime-neutral Provider Boundary Reader冻结Delta

用例：Runtime G6A受控Provider seam在实际执行点调用前，必须复读Tool Owner已CAS的`provider_boundary_crossed` current proof；Runtime不能import `tool-mcp`。

Owner与依赖：

- Tool Owner拥有Watermark与`ToolProviderBoundarySourceRefV1{WatermarkID,WatermarkRevision,WatermarkDigest}`；
- Runtime Owner已冻结公共`OperationProviderBoundaryCurrentReaderV1`、`OperationProviderBoundaryRefV1`与Projection；
- Tool `runtimeadapter`实现该Reader，只依赖Runtime公共合同；Runtime注入Reader接口，不依赖Tool类型或实现；
- G6A test fixture手工注入；live当前无production composition root，未来由宿主Owner在G6B前闭合。

输入只有exact `OperationProviderBoundaryRefV1{ID,Revision,Digest}`。方法签名精确为`InspectCurrentOperationProviderBoundaryV1(ctx, exactRef)`；Tool内部SourceRef逐字段无损映射到Runtime Ref，不新增Request DTO。

输出`OperationProviderBoundaryCurrentProjectionV1`字段精确为：`ContractVersion + Ref + Operation(OperationSubjectV3) + OperationDigest + OperationScopeDigest + Attempt(OperationDispatchAttemptRefV3) + ExecuteEnforcement + ExecuteEvidenceHandoff + Stage + CheckedUnixNano + ExpiresUnixNano + Digest`。没有SourceKind、Owner、Current字段。Expiry不得晚于Tool Owner current窗口、execute Enforcement与Handoff expiry最小值。

不变量：Projection只证明Tool boundary CAS已提交且current，不授Authority/Fence/Permit，不替代Enforcement/Handoff独立Validate。Runtime seam自行与已知Operation/Attempt/execute refs交叉校验。`NotFound`、Unknown、Unavailable、Indeterminate、expired、ref漂移、same ID/Revision换Digest、cross-Attempt、其他三字段Ref type-pun或Projection字段/摘要错误全部零Provider。

Effect/Recovery：Reader纯只读、无Effect；丢回包以同exactRef Inspect。任何不确定结果不得当成current，不得重派Provider。

兼容影响：这是已由Runtime Owner落盘的公共Port Delta，不得在Tool内私建Runtime兼容接口。Tool Adapter已在自身独占目录实现并通过隔离门禁；V1继续unsupported，不得作为V2降级路径。

### Live可复用类型与唯一新增面

| 类别 | 类型/合同 | 裁决 |
|---|---|---|
| Model Owner公开 | `ToolCallCandidateObservationProjectionV1` exact Ref与只读Reader | Tool已接入只读Reader；不复制Projection、不反推Model事实、不持有publish/write |
| 直接复用 | Harness `PendingActionV2`、`ContinuationRefV2`；Tool `ActionCandidate`、`ActionReservationFact` | 仅按各Owner公开Validate/Digest消费，不复制类型 |
| 直接复用 | Runtime `ProviderAttemptObservationRefV2`、`OperationDispatchAttemptRefV3`、prepare/execute `OperationDispatchEnforcementPhaseRefV4`、两项`OperationScopeEvidenceConsumptionRefV3`；Settlement V4的DomainResult、Settlement/Association/Guard/Projection与Inspection refs | 保持live字段与Owner；不新增字符串Receipt、不用Opaque JSON替代；Runtime不含Disposition |
| 只读历史 | live `DomainResultFact`/`ToolResult` v1、`OperationSettlementRefV3`、Provider `EvidenceRecordRefV2`、legacy Action/Tool/MCP/GovernedExecution V2 | 字段不足，不可用于N=1权威收口或包装升级 |
| Tool独占新增 | `ActionCandidateV2`、`ActionReservationFactV2`、`ApplicationAttemptRefV1`、Outcome/DomainResult/Apply/Result V2、`SingleCallToolActionCoordinationWatermarkV1`、`ToolProviderBoundarySourceRefV1` | Source Ref exact引用Watermark ID/Revision/Digest；Tool只写自身事实，不改变四项领域合同 |
| Runtime Owner已冻结公共合同 | `OperationProviderBoundaryRefV1`、`OperationProviderBoundaryCurrentReaderV1`、Projection V1 | 无Request DTO；Tool runtimeadapter无损映射/实现，Runtime不import Tool；本轮不修改Runtime |
| Application Owner新增 | `SingleCallToolActionPortV1`、`SingleCallToolActionRequestV1`、`SingleCallToolActionResultV1` | 窄公共Port/DTO；Tool只依赖其`contract/ports`；Application不import Tool实现 |
| G6A公共Owner已闭合 | Evidence V3 Action矩阵/router、V4.1/V2 controlled-provider association、Application N=1 Port/DTO、Settlement V4 current closure | Tool仅消费公开合同；Owner-local隔离闭环第三轮独立审计最终YES（P0/P1/P2=0），不代表系统G6A/production GO |
| G6B公共Owner待闭合 | Identity/Assembler接线、`ContextTurnRefreshPortV1` settled Frame链、Harness continuation public entry、Application跨域编排、Harness Assembly Adapter、production composition root | 不落在Tool/MCP资产外目录；未闭合不阻塞G6A隔离测试，但阻止production能力启用 |

### 分阶段门禁

#### G6A：Tool Adapter实现与离线测试Gate

以下六项已经按live公共合同闭合，并作为G6A隔离实现的持续回归Gate；任一未来漂移都必须Fail Closed。Context/Harness尚未完成不反向撤销G6A隔离实现：

1. Model公开`ToolCallCandidateObservationProjectionV1` exact只读Reader，Tool验证Ref全字段、Observation digest与`Calls == 1`；
2. Application公开`SingleCallToolActionPortV1`及窄Request/Result DTO，依赖方向为Tool实现→Application公共`contract/ports`；`Execute`在Model Reader门后进入same-canonical-command start-or-inspect；
3. Runtime public V2 actual-point合同与Gateway在physical executor入口校验current并直接执行；
4. Evidence V3矩阵固定run/tool/profile且五维均required，Runtime按required维度调用narrow current readers并完成S1/S2；
5. V4.1 prepare/execute Enforcement、Provider Binding、Prepared current proof、Boundary与统一NotAfter在Runtime V2中exact关联；
6. Settlement V4 exact引用typed Tool DomainResult与两项Evidence Binding，公共current Inspection/只读Association Inspect证明同一Settlement、同一Attempt的prepare/execute完整闭合。

G6A验收输出严格止于settled `ToolResultV2 + OperationInspectionSettlementRefV4`。G6A只能使用内存/本地测试Provider与test transport，不注册、不发布、不启用真实Provider能力；Context调用、Harness调用、Continuation构造与Turn推进计数必须为零。

#### G6B：宿主总装、Context Refresh与Harness推进Gate

G6A验收后才可进入G6B。以下任一未闭合，Provider能力启用、Continuation与Turn推进均保持NO-GO，但不反向阻塞G6A代码和离线测试：

1. Application未实现跨域编排，或未来宿主production composition root尚未把Tool实现注入Application公开Port；
2. Harness Assembly Adapter未能把settled PendingAction精确投影成Application DTO，或Harness/Application反向import Tool实现；
3. `ContextTurnRefreshPortV1`不能消费G6A Result，或pending Context DomainResult→S2→Context Owner本地原子ApplySettlement+Generation current CAS不能产出new exact Frame；
4. Harness没有公开Continuation验证/CAS入口，或Application不能在Context S2后携带new exact Frame调用；
5. Context exact-current不能满足；即使Plan允许Residual，也必须先形成已Apply、S2-current的新Frame，否则Continuation零写。

## PD-TM-03：Runtime actual-point Controlled Provider V2

状态：**Runtime public V2合同与Gateway已落盘，Tool V2 Owner-local隔离接线第三轮独立审计最终YES（P0/P1/P2=0）**。本节继续覆盖所有“V1 Boundary Reader后即可调用Provider”的错误表述；`OperationProviderBoundaryCurrentReaderV1`仍只是只读证明，不得包装或扩权成actual-point执行安全。该YES不代表系统G6A或production GO；Identity/Assembler、production Provider/Transport Backend、root与G6B仍为NO-GO。

### 已闭合Delta与Owner边界

`ControlledOperationProviderPortV1`的Request不携带actual Provider Binding、Prepared Attempt current proof及统一有效期。Tool即使在调用V1前再次读Clock或加锁，校验与物理副作用之间仍存在调度间隙；若Tool校验后再调用V1，也不构成actual-point闭合。该缺口属于Runtime公共Provider执行合同，Tool不得修改Runtime或私建V1兼容扩展。

Runtime Owner已发布对应public V2类型与Gateway；以下字段保持为Tool消费时的最小语义约束：

```text
ControlledOperationProviderPortV2.ExecuteControlledOperationProviderV2(
  ctx,
  request ControlledOperationProviderRequestV2
) -> ProviderAttemptObservationRefV2 | error
```

`ControlledOperationProviderRequestV2`最小强类型字段：

- `ContractVersion`；
- `Operation OperationSubjectV3`与`OperationDigest`；
- `OperationScopeDigest`；
- `Attempt OperationDispatchAttemptRefV3`；
- `Provider ProviderBindingRefV2`，且`Provider.Capability == EffectKind`；
- `PreparedAttempt PreparedProviderAttemptRefV2`；
- `PreparedAttemptCurrentProof`：Runtime Owner冻结的exact current projection/ref，必须绑定同一Operation/Attempt/Provider/Payload Schema/Payload Digest/Revision与Checked/Expires；
- `ExecuteEnforcement OperationDispatchEnforcementPhaseRefV4`；
- `ExecuteEvidenceHandoff OperationScopeEvidenceProviderHandoffRefV3`；
- `Boundary OperationProviderBoundaryRefV1`；
- `UnifiedNotAfterUnixNano`：不得晚于Provider Binding current、Prepared Attempt current、Boundary、execute Enforcement、execute Handoff及Runtime治理current窗口的最小上界。

### actual physical effect入口不变量

1. V2实现必须本身就是physical effect executor；它在自身入口采样fresh clock，原子执行`ValidateCurrent(request, now)`后直接产生副作用，中间不得再转调V1或另一个未受同一原子边界约束的Provider Port。
2. actual endpoint必须与`request.Provider`、`request.PreparedAttempt.Provider`及同一Attempt Delegation/Binding逐字段一致；Capability必须等于`praxis.tool/execute`与command/plan EffectKind。
3. `now`必须单调且严格早于`UnifiedNotAfterUnixNano`；任一proof unavailable、indeterminate、expired、drift、cross-attempt或same ID换digest均Fail Closed、零物理effect。
4. 返回值仅是`ProviderAttemptObservationRefV2` Observation；Tool Owner仍须独立Inspect并形成typed DomainResult。丢回包/Unknown只Inspect原Attempt，禁止新dispatch。
5. production-neutral V2 adapter seam与fake只可用于Conformance；fake不得宣称生产Backend或actual-point SLA。当前Runtime V2与Tool隔离接线通过门禁不等于production能力启用；production Provider/Transport Backend与root未闭合时仍保持关闭。

### 兼容与反例

- 不修改、不增加`ControlledOperationProviderPortV1`字段；V1继续只读兼容，不能通过Wrapper升级为V2。
- “Tool读取Boundary/Enforcement/Handoff -> fresh clock -> 调V1”仍是NO-GO，因为调度间隙可跨TTL或替换实际endpoint。
- Tool-owned controlled entry只有在它自己直接执行物理副作用时才可实现V2；验证后转调Runtime V1不是闭合。
- 换Provider Binding、Prepared Attempt/current proof、execute Enforcement/Handoff、Attempt、Operation/Scope或UnifiedNotAfter，均必须`boundary effect=0/provider=0/domain result=0/settlement=0`。

### 当前Tool落点

Tool V1 Owner Flow允许持久Candidate、Reservation及绑定Runtime Attempt；到`runtime_attempt_bound`后固定返回`unavailable/component_missing`，不CAS新的Provider Boundary、不调用Provider。V2 Owner Flow已经通过public Route/Gateway完成Boundary→Observation Inspect→DomainResult→Settlement V4→Apply/ToolResult隔离闭环，lost reply/Unknown只Inspect原Entry/Attempt。当前结论是“Tool G6A V2 Owner-local隔离实现第三轮独立审计最终YES（P0/P1/P2=0）”；该结论不代表系统G6A或production GO，Identity/Assembler、production root/backend与G6B仍为NO-GO。

## PD-TM-02：Tool Surface Manifest进入Plan、Binding与Harness Bootstrap

### 用例

Tool名称、顺序、Schema、Guidance和Dialect会影响模型行为与Prompt Cache。需要把Tool Engine编译的精确`ToolSurfaceManifest`冻结进Resolved Plan/Binding，并由Harness/Context验证Expected与Actual Injection，禁止Run中因Registry/MCP Snapshot漂移无声换面。

### 语义Owner

- Tool/MCP：Surface内容、排序、Capability映射与Revision；
- Profile/Assembly：Model Profile/Harness Capability Profile选择与Plan编译；
- Runtime Binding/Plan Certification：精确Ref/Digest关联与当前性；
- Harness：消费Expected Surface Association，不拥有Surface；
- Context：Actual Injection Manifest与物化Residual。

### 输入

- Tool Surface Ref、Revision、Digest、Expiry；
- Ordered Entries Digest、Dialect/Profile Digest；
- Capability Snapshot/Registry Snapshot refs；
- visible/allowed/pre-approved集合摘要；
- Expected Injection Digest与允许Residual。

### 输出

- Plan/Binding中的不可变Surface Association；
- Harness/Context可检查的Expected Surface Ref；
- Actual Injection Manifest关联或显式Residual；
- Snapshot漂移后的Reconcile/Degradation信号，不直接替换当前Run。

### 不变量

1. Plan内Alias必须已解析成精确Version+Digest；
2. visible、allowed、pre-approved不得合并；
3. Surface变化必产生新Revision/Digest；
4. Harness不能凭legacy `ToolSurfaceDigest`猜测完整对象；
5. ContextReference未物化时Fail Closed或记录Plan允许的Residual；
6. Model Profile适配不能扩大Effect权限。

### Effect与Recovery

- 纯本地编译不是Effect；外部Discovery/Refresh是Admin或Run Operation Effect；
- Surface/Binding写入不确定时按精确Ref/Digest Inspect；
- Snapshot过期或撤回触发Reconcile，不在Run中自动刷新。

### 反例

- 只保存一个摘要却无法证明条目顺序、Dialect或来源；
- MCP `tools/list_changed`到达后直接修改当前Model Context；
- 使用Provider自报Tool列表作为Binding权威事实；
- Context实际注入额外内置Tool但仍宣称Surface精确。

### 兼容影响

建议以可选的V3 Association增量扩展Plan/Binding/Harness governed bootstrap；旧Plan无Association时只能走显式Residual/受限Conformance，不从legacy摘要反推。具体落点由Profile/Assembly、Runtime与Harness Owner联合决定。

## PD-TM-04：Application V2 Single Call Tool Action Binding Current Reader

状态：**Runtime ports唯一Assembly Current/RegistrySnapshot public Go nominal与Reader已落盘；Tool P4-0/P4-1/P4-2 Owner-local实现与software test已闭合。** Harness M2、跨Owneractual-point四读、system/production均NO-GO。现行合同以[第七设计修正候选](pd-tm-04-seventh-candidate.md)、[SurfaceInvocationBinding V1](surface-invocation-binding-v1.md)与[ToolSurfaceManifestCurrent V1](tool-surface-manifest-current-v1.md)为准。

### 用例与唯一边界

Application G6A V2把稳定Application Request、完整Identity/ExecutionScope和Model Projection exact坐标交给Tool adapter。P4必须通过`ToolSurfaceManifestCurrentReaderV1`、Capability/Tool Registry Object Current及公共Readers形成`ActionCandidateV3 + ToolInputContractCurrent + CandidateClosureV2 + S2SnapshotV2`，完成全部S1/S2后、任何Watermark前原子持久BindingV2。BindingV2 Ref是唯一恢复根。

Prepared Historical/Current、Assembly与Binding的Registry exact Ref统一为live `runtimeports.RegistrySnapshotRefV1`，禁止旧Model Registry nominal、alias/type-pun。Tool BindingRef以Owner/ContractVersion/ID/Revision/Digest无损对应Model neutral SurfaceBindingRef；`InspectExact(ctx, BindingRef)`单参返回Binding+ToolAck。Harness只实现Model Gate并在内部调用Tool SurfaceInvocationBinding `Ensure`。Runtime ports live `ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1`由Tool/Harness直接共享。每个attempt/Open/Stream/continuation actual-point复读Prepared Current、Binding/ToolAck、Surface Current、Assembly composite。Application不改；Tool Owner-local实现已闭合，Harness接线与production启用继续hard-block。

### C2：ToolSurfaceManifestCurrent Reader/Repository

Tool public contract精确拆分：

```go
type ToolSurfaceManifestCurrentReaderV1 interface {
    InspectExactToolSurfaceManifestCurrentV1(context.Context, ToolSurfaceManifestCurrentRefV1) (ToolSurfaceManifestCurrentProjectionV1, error)
}

type ToolSurfaceManifestCurrentRepositoryV1 interface {
    ToolSurfaceManifestCurrentReaderV1
    EnsureExactToolSurfaceManifestCurrentV1(context.Context, ToolSurfaceManifestCurrentEnsureRequestV1) (ToolSurfaceManifestCurrentProjectionV1, error)
}
```

Projection闭集为`ContractVersion/Ref/完整Manifest/Owner/CheckedUnixNano/ExpiresUnixNano/ProjectionDigest`。current stable ID直接为Manifest.ID；Ref的ID/Revision/Digest逐字段等于Manifest/Plan.ToolSurface，不派生prefix或第二lineage。ProjectionDigest独立计算，禁止Manifest digest与Projection digest互猜。同Manifest ID跨Owner为Conflict。

唯一concrete Repository维护immutable history与current index；Ensure request携`ExpectedCurrent` full Ref：仅rev1可从current NotFound create，successor必须current+1并CAS exact expected。history hit只有current Ref仍等于winner Ref才可幂等返回，否则Conflict/Precondition且绝不回退current。Reader只做exact current读取，method set不含Ensure。Harness M2只import`tool-mcp/contract`且构造器只接Reader，正常/错误/恢复路径的Ensure调用数均必须为0；Harness Model Gate调用SurfaceInvocationBinding Writer与此C2 Ensure不是同一Port。

### Runtime ports additive Delta：ModelPreDispatchAssemblyCurrent V1

- 用例：解除Harness Gate必须import Tool Writer而Tool又需消费Harness Assembly current导致的package cycle；拒绝Tool/Harness各建影子DTO。
- 语义/发布Owner：Harness Owner。Runtime ports仅是三方共同依赖的中立nominal落点，Runtime不创建、不解释、不CAS Assembly事实。
- 输入：完整`runtimeports.ModelPreDispatchAssemblyCurrentRefV1`。
- 输出：完整`runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1`。
- Reader：`InspectCurrentModelPreDispatchAssemblyV1(ctx context.Context, ref runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error)`。
- 共享Registry nominal：`runtimeports.RegistrySnapshotRefV1{Owner, ContractVersion, ID, Revision, Digest}`；该Ref的Authority是Owner所指Registry Owner，Runtime/Harness/Model/Tool均不得以carrier身份改写。
- 字段专用nominal：`ModelPreDispatchAssemblyExactRefV1{ID,Revision,Digest}`只用于Handoff、Manifest、Conformance、ToolSurface的封闭字段位置；`ModelPreDispatchAssemblyBindingSetRefV1{ID,Revision,Digest,SemanticDigest,CurrentnessDigest,ProjectionDigest,ExpiresUnixNano}`完整保留BindingSet事实与current水位，禁止降格成三字段Ref。
- Ref exact shape：`ModelPreDispatchAssemblyCurrentRefV1{ContractVersion,ID,Revision,Digest,Generation,Handoff,BindingSet,Manifest,Conformance,WatermarkDigest,ToolSurface,ProfileDigest,RegistrySnapshot,SemanticDigest,CurrentnessDigest,CheckedUnixNano,ExpiresUnixNano,ProjectionDigest}`。
- Projection exact shape：`ModelPreDispatchAssemblyCurrentProjectionV1{ContractVersion,Ref,Generation,Handoff,BindingSet,Manifest,Conformance,ToolSurface,ProfileDigest,RegistrySnapshot,SemanticDigest,CurrentnessDigest,CheckedUnixNano,ExpiresUnixNano,ProjectionDigest}`。
- 不变量：Projection与Ref的Generation/Handoff/BindingSet/Manifest/Conformance/ToolSurface/RegistrySnapshot逐字段exact；Profile/Semantic/Currentness/Checked/Expires/ProjectionDigest全部exact；`CheckedUnixNano > 0 && CheckedUnixNano < ExpiresUnixNano`。Projection digest计算时同时清空`Projection.ProjectionDigest`、`Projection.Ref.Digest`和`Projection.Ref.ProjectionDigest`，最终三者exact等于同一完成摘要；Reader只接exact Ref，不接ID/latest。
- Effect/Recovery：纯只读Inspect，不授Authority/Review/Fence/Permit或Provider执行权。Unavailable/Indeterminate/非权威NotFound、drift、expired或clock rollback全部Fail Closed；不创建新Ref、不刷新TTL。
- 反例：Tool import Harness、Harness/Tool自建echo、Kind翻译、丢Manifest/Conformance/Watermark/完整Registry Ref、重算digest、裸digest查latest、same Ref换任一字段均零Ensure/零Provider。
- 兼容影响：additive Runtime ports合同已以public Go落盘；不扩权既有Runtime/Harness Port。Tool不定义fallback DTO、alias或echo；P4-0/P4-1/P4-2 Owner-local Go已闭合，Harness wiring未闭合前system/production保持hard-block。

本Delta包含并已在Tool Owner-local落盘的additive合同：Surface Manifest Repository、SurfaceInvocationBinding Repository、Capability/Tool Registry Object exact Reader、InputContract三方法Reader+三方法Store、BindingV2三方法Reader+三方法Store与CandidateV3 builder；它们不是“一个只读Port”。Application只获得窄Resolve/Inspect能力，不获得Tool Store写口。Runtime Association窄Reader已落盘；Harness M2与公共pre-invocation/actual-point Gate尚未闭合，因此完整P4/system/production继续hard-block。本文不修改其他Owner Go：

```go
const (
    SingleCallToolActionBindingCurrentContractVersionV1 = "praxis.tool.single-call-action-binding-current/v1"
    SingleCallToolActionBindingCurrentDomainV1          = "praxis.tool"
    SingleCallToolActionBindingCurrentKindV1            = "praxis.tool/single-call-action-binding-current"
    SingleCallToolActionBindingSubjectDiscriminatorV1   = "SingleCallToolActionBindingSubjectV1"
    SingleCallToolActionBindingIssuanceDiscriminatorV1  = "SingleCallToolActionBindingIssuanceSubjectV1"
    SingleCallToolActionBindingProjectionDiscriminatorV1 = "SingleCallToolActionBindingCurrentProjectionV1"
    SingleCallToolActionBindingCurrentIDPrefixV1        = "single-call-tool-binding:v1:"
)

type SingleCallToolActionBindingSubjectV1 struct {
    ContractVersion            string                                                     `json:"contract_version"`
    Domain                     string                                                     `json:"domain"`
    Kind                       runtimeports.NamespacedNameV2                              `json:"kind"`
    ApplicationRequestID       string                                                     `json:"application_request_id"`
    ApplicationRequestRevision core.Revision                                              `json:"application_request_revision"`
    ApplicationRequestDigest   core.Digest                                                `json:"application_request_digest"`
    ActionCoordinateDigest     core.Digest                                                `json:"action_coordinate_digest"`
    ExecutionScope             core.ExecutionScope                                        `json:"execution_scope"`
    ExecutionScopeDigest       core.Digest                                                `json:"execution_scope_digest"`
    SourceSubject              applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
    Digest                     core.Digest                                                `json:"digest"`
}

type SingleCallToolActionBindingCurrentRefV1 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"`
    Digest   core.Digest   `json:"digest"`
}

type SingleCallToolActionBindingIssuanceSubjectV1 struct {
    ContractVersion            string                               `json:"contract_version"`
    BindingSubject             SingleCallToolActionBindingSubjectV1 `json:"binding_subject"`
    RequestedExpiresUnixNano   int64                                `json:"requested_expires_unix_nano"`
    Digest                     core.Digest                          `json:"digest"`
}

type SingleCallToolActionBindingResolveRequestV1 struct {
    ApplicationRequest       applicationcontract.SingleCallToolActionRequestV2          `json:"application_request"`
    SourceSubject            applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
    RequestedExpiresUnixNano int64                                                       `json:"requested_expires_unix_nano"`
}

type SingleCallToolActionBindingInspectByIssuanceRequestV1 struct {
    ApplicationRequest       applicationcontract.SingleCallToolActionRequestV2             `json:"application_request"`
    SourceSubject            applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
    RequestedExpiresUnixNano int64                                                           `json:"requested_expires_unix_nano"`
}

type SingleCallToolActionBindingInspectExactRequestV1 struct {
    Issuance   SingleCallToolActionBindingInspectByIssuanceRequestV1 `json:"issuance"`
    Expected   SingleCallToolActionBindingCurrentRefV1               `json:"expected"`
}

type SingleCallToolActionBindingCurrentProjectionV1 struct {
    ContractVersion          string                                                     `json:"contract_version"`
    Domain                   string                                                     `json:"domain"`
    Kind                     runtimeports.NamespacedNameV2                              `json:"kind"`
    Ref                      SingleCallToolActionBindingCurrentRefV1                    `json:"ref"`
    Subject                  SingleCallToolActionBindingSubjectV1                       `json:"subject"`
    IssuanceSubject          SingleCallToolActionBindingIssuanceSubjectV1               `json:"issuance_subject"`
    RequestedExpiresUnixNano int64                                                      `json:"requested_expires_unix_nano"`
    ApplicationInput         applicationcontract.SingleCallToolActionInputCurrentProjectionV2 `json:"application_input"`
    ModelProjection          modelinvoker.ToolCallCandidateObservationProjectionV1      `json:"model_projection"`
    CallOrdinal              uint32                                                     `json:"call_ordinal"`
    CallID                   string                                                     `json:"call_id"`
    CallName                 string                                                     `json:"call_name"`
    CanonicalArgumentsDigest core.Digest                                                `json:"canonical_arguments_digest"`
    CandidateClosure         SingleCallToolActionCandidateCurrentClosureV1              `json:"candidate_closure"`
    Association              runtimeports.GenerationBindingAssociationFactV1            `json:"association"`
    Generation               runtimeports.GenerationCurrentProjectionV1                 `json:"generation"`
    Route                    runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 `json:"route"`
    Provider                 runtimeports.ProviderBindingRefV2                           `json:"provider"`
    ProviderCurrent          runtimeports.ProviderBindingCurrentProjectionV2             `json:"provider_current"`
    CheckedUnixNano          int64                                                       `json:"checked_unix_nano"`
    ExpiresUnixNano          int64                                                       `json:"expires_unix_nano"`
    ProjectionDigest         core.Digest                                                 `json:"projection_digest"`
}

// Tool Owner内部能力；不得注入Application/Harness，也不是跨Owner Port。
type SingleCallToolActionBindingLeaseStoreV1 interface {
    CreateSingleCallToolActionBindingLeaseOnceV1(
        context.Context,
        SingleCallToolActionBindingCurrentProjectionV1,
    ) (SingleCallToolActionBindingCurrentProjectionV1, error)
    InspectSingleCallToolActionBindingLeaseByIssuanceIDV1(
        context.Context,
        string,
    ) (SingleCallToolActionBindingCurrentProjectionV1, error)
    InspectExactSingleCallToolActionBindingLeaseV1(
        context.Context,
        SingleCallToolActionBindingCurrentRefV1,
    ) (SingleCallToolActionBindingCurrentProjectionV1, error)
}

type SingleCallToolActionBindingCurrentReaderV1 interface {
    ResolveCurrentSingleCallToolActionBindingV1(
        context.Context,
        SingleCallToolActionBindingResolveRequestV1,
    ) (SingleCallToolActionBindingCurrentProjectionV1, error)

    InspectSingleCallToolActionBindingByIssuanceV1(
        context.Context,
        SingleCallToolActionBindingInspectByIssuanceRequestV1,
    ) (SingleCallToolActionBindingCurrentProjectionV1, error)

    InspectExactSingleCallToolActionBindingCurrentV1(
        context.Context,
        SingleCallToolActionBindingInspectExactRequestV1,
    ) (SingleCallToolActionBindingCurrentProjectionV1, error)
}
```

### immutable lease、稳定ID与canonical digest

`SingleCallToolActionBindingSubjectV1.Digest`的canonical domain固定为`praxis.tool`、version固定为本合同版本、discriminator固定`SingleCallToolActionBindingSubjectV1`；subject body显式排除自身`Digest`。Subject必须与`ApplicationRequest.Action.PendingSubject`和独立传入的sealed `SourceSubject`完整相等，并绑定Request ID/Revision/Digest、Action digest及完整ExecutionScope/digest。

业务`BindingSubject`保持稳定，便于证明同一Application Request/SourceSubject；实际签发身份使用独立`IssuanceSubject`。`IssuanceSubject` canonical body覆盖完整`BindingSubject`与`RequestedExpiresUnixNano`并排除自身Digest；domain/version/discriminator固定为`praxis.tool`、本合同版本、`SingleCallToolActionBindingIssuanceSubjectV1`。稳定ID唯一为`single-call-tool-binding:v1:`加去掉`sha256:`前缀后的Issuance Subject digest，`Ref.Revision`固定1。因此requested bound是immutable issuance identity的一部分：同一业务Subject的不同requested bound得到不同issuance ID，不能碰撞或静默复用。

Projection canonical body覆盖上述全部字段，但**显式排除`Ref.Digest`与`ProjectionDigest`**；canonical domain/version/discriminator分别固定为`praxis.tool`、本合同版本和`SingleCallToolActionBindingCurrentProjectionV1`。Seal后必须满足：

```text
computed canonical digest == Ref.Digest == ProjectionDigest
Ref.ID == derive(IssuanceSubject)
Ref.Revision == 1
```

`ResolveCurrent...`先从完整Application Request、sealed SourceSubject和requested bound构造/校验Issuance Subject并派生ID，然后调用`Inspect...ByIssuanceID`。已存在时只验证并返回持久lease；只有Store给出该ID的**权威NotFound**才执行S1/S2并调用`Create...Once`。`Unavailable/Indeterminate/timeout`均不得create。并发create由Store原子单赢家：赢家一次写入`CheckedUnixNano/ExpiresUnixNano`；loser必须返回winner已持久lease，禁止拿各自fresh clock生成的candidate body与winner比较或制造Conflict。same issuance ID却持久非canonical/不同业务字段是Store corruption/Conflict。

BindingV2三种public request不可混用或互为alias：Resolve只携稳定Application Request+sealed SourceSubject+requested，由Reader内部权威构造S1/S2/Candidate/InputContract/Closure；`SingleCallToolActionBindingIssuanceLookupRequestV2`用相同稳定字段但为独立nominal，可不重跑S1/S2直接派生issuance ID并恢复lost reply；InspectExact只携Expected BindingV2 Ref+稳定request坐标，不要求caller重建fresh Candidate。两种Inspect都从Tool Owner BindingV2 Store返回首次持久化的同一Projection/Checked/Expires。live旧Watermark不保存该Ref，也不是恢复根。

以下重复字段不允许只靠canonical digest间接关联，必须在Projection Seal、Projection Validate、Store `Create...Once`写前/返回winner后、Store两种read返回前，以及外部两种Inspect返回前逐项硬门：

```text
Projection.Subject == Projection.IssuanceSubject.BindingSubject
Projection.RequestedExpiresUnixNano == Projection.IssuanceSubject.RequestedExpiresUnixNano
if Projection.IssuanceSubject.RequestedExpiresUnixNano > 0:
    Projection.ExpiresUnixNano <= Projection.IssuanceSubject.RequestedExpiresUnixNano
```

任一不等为Conflict/invalid canonical lease并零后续动作。顶层requested为0但Issuance Subject requested>0、或反向不一致，均不能借另一字段绕过TTL；Store读取到这种历史记录必须Fail Closed，不得修补、重封或续租。

### canonical bytes与payload闭包

`PendingActionExactRefV2`只有ID/Revision/RequestDigest，**没有PayloadDigest**，不得再写`PendingActionExactRefV2.PayloadDigest`链。其Revision在本轮候选中固定为Tool-local wrapper revision `1`；跨Owner只证明ID+RequestDigest，禁止挪用Session/Identity/SourceCandidate Revision并禁止宣称Application/Harness提供Revision。唯一合法bytes链为：

```text
Model exact Call canonical argument bytes
  == ApplicationInput.HarnessCurrent.IdentityCurrent.Projection.CanonicalArguments
core.DigestBytes(bytes)
  == Application IdentityCurrent.Projection.CanonicalArgumentsDigest
  == Application Identity.CanonicalArgumentsDigest
  == Application Identity.PayloadContentDigest
  == SourceSubject.Identity.CanonicalArgumentsDigest
  == SourceSubject.Identity.PayloadContentDigest
  == SourceSubject.Binding.Base.PendingAction.PayloadDigest
  == Candidate.Payload.ContentDigest
```

`Candidate.Payload`必须先满足`Candidate.Payload.Validate()==nil`，再逐字段证明`Inline != nil`且长度`1..runtimeports.MaxOpaqueInlineBytes`、`Ref == ""`、`Length == uint64(len(Inline))`、`Schema == Candidate.InputSchema == SourceSubject.Identity.PayloadSchema`、`ContentDigest == core.DigestBytes(Inline)`。live Surface/Schema没有authoritative LimitPolicy或完整InputSchema current来源，因此LimitPolicy与InputSchema current必须由第七候选的Tool Owner `ToolInputContractCurrentProjectionV1`证明；Candidate自报字段不能自证。`OpaqueLimitPolicyRefV2`没有独立校验方法，禁止虚构调用。`Inline`必须逐字等于上述Application Projection bytes和Model exact Call bytes，禁止重序列化或“语义等价”比较。Reader、resolver、Seal、Create、Inspect、Clone和所有返回路径均deep-copy所有byte slice；修改调用方输入或返回值不得污染持久lease、digest或后续Inspect。

### Tool Candidate current Resolver正式合同

```go
type SingleCallToolActionCandidateResolveRequestV1 struct {
    ApplicationRequest applicationcontract.SingleCallToolActionRequestV2          `json:"application_request"`
    ApplicationInput   applicationcontract.SingleCallToolActionInputCurrentProjectionV2 `json:"application_input"`
    ModelProjection    modelinvoker.ToolCallCandidateObservationProjectionV1      `json:"model_projection"`
    SourceSubject      applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
    Route              runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 `json:"route"`
    ProviderCurrent    runtimeports.ProviderBindingCurrentProjectionV2             `json:"provider_current"`
    InputContractCurrentRef ToolInputContractCurrentRefV1                           `json:"input_contract_current_ref"`
    InputContractCurrent ToolInputContractCurrentProjectionV1                       `json:"input_contract_current"`
}

// 以下SingleCallToolActionCandidateCurrentClosureV1与ResolverV1仅记录冻结兼容canonical。
// PD-TM-04 P4禁止调用；P4使用ActionCandidateV3 + ClosureV2 + BindingCurrentV2。
type SingleCallToolActionCandidateCurrentClosureV1 struct {
    Candidate              toolcontract.ActionCandidateV2 `json:"candidate"`
    PendingActionCurrent   toolcontract.OwnerCurrentRefV1  `json:"pending_action_current"`
    SurfaceCurrent         toolcontract.OwnerCurrentRefV1  `json:"surface_current"`
    CapabilityCurrent      toolcontract.OwnerCurrentRefV1  `json:"capability_current"`
    ToolCurrent            toolcontract.OwnerCurrentRefV1  `json:"tool_current"`
    InputSchemaCurrent     toolcontract.OwnerCurrentRefV1  `json:"input_schema_current"`
    SourceCandidateCurrent toolcontract.OwnerCurrentRefV1  `json:"source_candidate_current"`
    InputContractCurrentRef ToolInputContractCurrentRefV1   `json:"input_contract_current_ref"`
    InputContractCurrent    ToolInputContractCurrentProjectionV1 `json:"input_contract_current"`
    ClosureDigest          core.Digest                     `json:"closure_digest"`
}

type SingleCallToolActionCandidateCurrentResolverV1 interface {
    ResolveSingleCallToolActionCandidateCurrentV1(
        context.Context,
        SingleCallToolActionCandidateResolveRequestV1,
    ) (SingleCallToolActionCandidateCurrentClosureV1, error)
    InspectSingleCallToolActionCandidateCurrentV1(
        context.Context,
        toolcontract.ObjectRef,
    ) (SingleCallToolActionCandidateCurrentClosureV1, error)
}
```

Resolve只允许在Association、Generation、Route、Provider Current及Tool Input Contract Current已经exact/current闭合后调用。请求内`Route`、`ProviderCurrent`与完整`InputContractCurrentRef/Projection`必须是S1刚读取的值，不接受摘要、重建或CandidateSource自报；要求Provider/ExpectedOwner/Artifact、Surface完整Entry/Capability/Tool/InputSchema/LimitPolicy及四项current exact。唯一Model Call再闭合Identity内SourceCandidate。返回Candidate必须Seal/Validate通过，且六个current ref与Candidate内对应current字段逐字段相等；`ObjectRef{ID: Candidate.ID, Revision: Candidate.Revision, Digest: Candidate.Digest}`是唯一Inspect key。

P4不复用四项`OwnerCurrentRefV1`：Surface使用Tool Surface Repository Current；Capability/Tool使用Registry Record+Descriptor形成的Registry Object Current；Route ExpectedOwner保持独立EffectOwner。Input Contract Projection immutable；外部source fresh读取只能校验/收紧deadline，不能改写InputContract或CandidateV3。

Candidate `ExpectedOwner`唯一构造为：

```go
runtimeports.EffectOwnerRefV2{
    Role:           runtimeports.OwnerSettlement,
    ComponentID:    Route.ProviderBinding.ComponentID,
    ManifestDigest: Route.ProviderBinding.ManifestDigest,
}
```

它必须与`Projection.Provider`、`Route.ProviderBinding`及`ProviderCurrent.Ref`逐字段回扣；任何Component/Manifest/BindingSet漂移均在Seal Candidate前Fail Closed。Identity `SettlementOwner`仍只做来源闭包，不参与该构造。

V1 `PendingActionCurrent`只说明历史实现。P4由ActionCandidateV3 successor replacement直接canonical绑定Tool-local PendingAction Ref(rev1)，BindingV2 closure绑定Application current；不得继续给PendingAction伪造EffectOwner/current字段。

V1 Closure块仅保留兼容canonical。P4 additive CandidateClosureV2证明S1构造，S2SnapshotV2单独保存全部S2 source/public currents与上界；二者只随BindingV2 Projection存于唯一Store，不另建CandidateClosureStore。

### Application V2正式依赖与构造器窄口

Application V2设计已YES并正式冻结：

```go
type SingleCallToolActionInputCurrentReaderV2 interface {
    InspectSingleCallToolActionInputCurrentV2(
        context.Context,
        applicationcontract.SingleCallToolActionRequestV2,
    ) (applicationcontract.SingleCallToolActionInputCurrentProjectionV2, error)
}
```

Application V2 Go公共合同已落盘并基础compile PASS，但测试与独立代码审计尚未完成。Tool构造器只接受以下窄口，任一缺失或typed-nil均拒绝构造：

1. `applicationports.SingleCallToolActionInputCurrentReaderV2`；
2. `modelinvoker.ToolCallCandidateObservationProjectionReaderV1`；
3. 上述Tool Owner内部`SingleCallToolActionCandidateCurrentResolverV1`；
4. 本文Runtime additive候选`runtimeports.GenerationBindingAssociationCurrentReaderV1`；
5. 现有窄`runtimeports.GenerationCurrentReaderV1`；
6. 现有`runtimeports.ControlledOperationProviderRouteCurrentReaderV2`；
7. 现有只读`runtimeports.ProviderBindingCurrentnessPortV2`；
8. Tool Owner内部`SingleCallToolActionBindingLeaseStoreV1`；
9. injected monotonic Clock。

Tool不得拿`GenerationBindingAssociationGovernancePortV1`含Associate的写能力。`PD-TM-04` P4-2 Owner-local实现已通过software test，但不得回退V1、在Tool本地复制公共类型或把该结果扩写为Harness M2/system/production YES。

### Owner、exact与current

- Application Owner：V2 Request与Input Current Projection；Tool只消费公共`contract/ports`。
- Model Owner：`ToolCallCandidateObservationProjectionV1`及exact Reader；Tool不持有publish/write口。
- Tool Owner：Registry、Surface、Capability、Tool、Action Candidate与本Binding Current Projection。
- Runtime/Assembler Binding Owner：Generation Association、Generation Current与Provider Binding Current；Tool只通过窄Reader读取并交叉验证，不签发、续租、撤销或Associate。
- Runtime Owner：Intent/Attempt/Permit/Fence、Authority/Review/Budget/Scope治理与V2 actual-point Gateway。本Projection不授Authority、Review Verdict、Permit或Fence，也不替代Runtime Route/Prepared/Enforcement/Handoff/Boundary/Evidence。

V2 issuance ID只由预S1稳定Application Request/Scope/SourceSubject/固定版本策略+requested派生。权威NotFound后完成S1、InputContract、完整S2、CandidateV3、CandidateClosureV2、S2Snapshot与共同min，才在任何Watermark前原子持久BindingV2 root。Candidate/InputContract/Closure/时间不得进入ID派生。

### Runtime additive只读Port Delta：Generation Binding Association

用例：Tool只需按Association ID读取current Fact，不应获得`AssociateGenerationBindingV1`写能力。Runtime Owner候选增量：

```go
type GenerationBindingAssociationCurrentReaderV1 interface {
    InspectCurrentGenerationBindingAssociationV1(
        context.Context,
        string,
    ) (GenerationBindingAssociationFactV1, error)
}

type GenerationBindingAssociationGovernancePortV1 interface {
    GenerationBindingAssociationCurrentReaderV1
    AssociateGenerationBindingV1(
        context.Context,
        GenerationBindingAssociationCandidateV1,
    ) (GenerationBindingAssociationFactV1, error)
}
```

这是additive接口抽取：现有Governance实现保持兼容；Tool构造器只接收`GenerationBindingAssociationCurrentReaderV1`。本轮只提交Delta，不修改Runtime Go。

Association链固定为：

1. 以`ApplicationRequest.Action.PendingSubject.Binding.OwnerInputs.GenerationBindingAssociation.ID`调用Reader；返回Fact必须`Validate()==nil`、`RefV1()`与Request exact Ref逐字段相等、`State==active`且fresh now严格早于`ExpiresUnixNano`。
2. Generation Reader入参必须是`Association.Candidate.Generation.Generation`；返回`GenerationCurrentProjectionV1`必须与`Association.Candidate.Generation`完整exact，并对该Generation Ref执行`ValidateCurrent`。
3. Route Reader必须以`ApplicationRequest.Action.PendingSubject.Binding.OwnerInputs.RouteCurrent`与`RouteMatrix`调用`InspectCurrentControlledOperationProviderRouteV2`；Matrix必须`Validate()==nil`且exact等于`runtimeports.OperationScopeEvidenceActionMatrixV3()`，返回Projection必须`ValidateCurrent(RouteCurrent, RouteMatrix, freshNow)==nil`。
4. Route的`Generation`必须exact等于Association/Generation的Generation Ref；Route `BindingSetID/Revision/Digest/SemanticDigest/CurrentnessDigest`必须分别和Association/Generation实际拥有的公开坐标exact回扣。`ActiveRouteID/Revision/Digest`只由Route current自身canonical projection与exact Reader证明；Association/Generation没有这些字段，禁止伪造第二来源。
5. **执行Provider唯一取自`Route.ProviderBinding`**，不是Identity `SettlementOwner`。`SettlementOwner`只需作为Model/Harness来源闭包与SourceSubject/Identity exact，不参与Provider解析或替代Route。
6. Route Provider的BindingSet ID/Revision必须等于Route/Association BindingSet，Capability必须为`praxis.tool/execute`；Generation `ComponentManifests`必须存在唯一条目，使ComponentID、ManifestDigest、ArtifactDigest与该Provider完整相等。
7. `InspectProviderBindingCurrentV2(ctx, Route.ProviderBinding)`返回的`ProviderBindingCurrentProjectionV2.Ref`必须exact等于Route Provider、State active/fresh，且`BindingSetDigest`、`BindingSetSemanticDigest`分别回扣Route和Association同名字段。
8. Association、Generation、Route、Provider任一NotFound/Unavailable/Indeterminate/expired/revoked/drift均zero-effect；不得从Identity SettlementOwner、Route摘要或Application DTO反推Provider。

### TTL、S1/S2与Boundary再Inspect

BindingV2 expiry取S1/S2 Application/Runtime currents、immutable InputContract、Surface Repository Manifest真实expiry、Capability/Tool Registry Object Currents、requested和独立Binding cap最小值。InputContract/BindingV2 exact时间不可刷新；PD-TM-04 InputContract requested必须为正数并由动作current上界提供。

1. 先取fresh `nowS1`，要求非零且不早于任何已观察Checked；执行`ApplicationRequest.ValidateCurrent(nowS1)`，再验证SourceSubject canonical/Validate及与Request Action PendingSubject完整exact。失败时不得调用任何Owner Reader。
2. S1固定顺序：Application V2 Input → Model exact Projection → 以Projection Invocation坐标Inspect ToolSurfaceInvocationBinding → Association → Generation → Route → Provider Current → 以Binding指定Surface Ref读取Surface Current → Capability/Tool Registry Object Currents → Tool Input Contract Resolve → CandidateV3 builder。Application Input返回Request ID/Revision/Digest、ActionDigest必须exact，并只证明PendingAction ID/RequestDigest及真实current窗口；Tool-local PendingAction wrapper Revision固定1。Model Projection先`Validate()`并证明返回full Ref exact、`Calls==1`、唯一Call `Ordinal==0`，且Identity `CallOrdinalPresent==true`、encoding version合法、value==0；CallID/Name/canonical arguments bytes/digest逐字段exact。
3. Route/Provider闭合后，内部builder以完整Route Projection、ProviderCurrent、SurfaceInvocationBinding和InputContract closure按`CallName→Surface→Capability→Tool→Schema→historical SourceCandidate`形成唯一sealed CandidateV3；ExpectedOwner只从同一Route Provider派生。CandidateV3不是独立current/root，不存在单独的PendingAction owner-current nominal或Candidate current Reader。
4. BindingV2创建前完成S2：InputContract InspectExact返回原Checked/Expires；Surface按Repository exact Ref复读，Capability/Tool按Registry Record+Descriptor复读；所有S2上界进入S2Snapshot和BindingV2 Expires。
5. 每项S2 owner current都必须以该Reader返回后的fresh now执行`ValidateCurrent`并检查clock未回拨；InputContract immutable Projection必须exact不变。若S2任一真实expiry缩短且fresh now已到期，返回PreconditionFailed且零create；仍fresh但更早时只缩短BindingV2最终lease，变晚也不得越过任何S1上界。CandidateV3本身无current时间语义。
6. S2全部完成后取fresh `nowS2`；lease `CheckedUnixNano=nowS2`，按上述S1+S2全部上界计算一次immutable Expires。随后再取final fresh now，要求`final>=nowS2 && final<Expires`，否则零create。
7. 原子Create成功后重复Resolve/Inspect返回winner同一Checked/Expires，不重算lease；Inspect仍须按持久source refs复验current，但不得签发新时间窗。
8. 首次Create成功后才允许create/Inspect Watermark并形成Candidate/Reservation。
9. Provider Boundary前按`BindingCurrentRefV2` InspectExact durable root并复读外部currents；live旧Watermark不含该Ref。P5另立派生handoff版本消费BindingV2，不成为第二Truth Owner。

### typed-nil、错误与zero-effect

- constructor对上述九项窄依赖逐一执行nil/typed-nil检查；任一缺失返回`invalid_argument/component_missing`且不得返回可用实例；
- nil receiver或运行时依赖变为nil/typed-nil：`unavailable/component_missing`，零下游调用；
- nil `context.Context`：在`context.WithoutCancel`、任何Reader或Gateway之前返回`invalid_argument/invalid_reference`；
- `NotFound`只接受对应Owner的权威exact NotFound；`Unavailable`、`Indeterminate`、超时或解码失败不得转换为NotFound；
- Resolve/create lost reply只能以原Application V2 Request+sealed SourceSubject+requested bound派生issuance ID并调用`Inspect...ByIssuance`；不得再次create、重新Resolve或生成新lease；
- 任何失败均保持：Watermark=0、Candidate=0、Reservation=0、Boundary CAS=0、Gateway=0、Provider=0、DomainResult=0、Settlement=0。

### Import边界

- Tool InputContract Ref/Subject/Issuance/Projection、CandidateV3及BindingV2 Ref/中立Subject/Issuance nominal位于`tool-mcp/contract`，只依赖Tool contract与Runtime public core/ports，不得导入Application。携Application V2 DTO的Resolve/IssuanceLookup/InspectExact request、Closure/S2 Snapshot、Projection、Reader/Store实现位于`tool-mcp/applicationadapter`；该包允许依赖Application V2公共`contract/ports`、Model公开Reader、Runtime公开ports和Tool自身contract，并逐字段转换到中立Subject。appadapter引用前三项nominal必须使用`toolcontract`类型，禁止复制第二套。
- `tool-mcp/contract`、`action`、`registry`及其他domain/kernel包不得导入Application、Harness或Model实现；Application协调器类型不得下沉Tool domain。
- Application只依赖其自行冻结的G6A V2公共Action Port，并由未来宿主composition root注入Tool实现；Application不得import `tool-mcp`实现。本Delta不命名、不补造Application Action Port。
- Harness不得import Tool实现，也不得向Tool暴露私有`ContextPort/ModelTurnPort/EventCandidatePort`或私有PendingAction结构；Harness只经Application V2中立DTO参与未来总装。
- Tool禁止依赖Application coordinator/kernel/fakes、Model internal、Harness任何实现/私有类型、Runtime kernel/fakes。
- Runtime只发布并实现中立Association current Reader，不import Tool；Tool不得获得Runtime Association Governance写口。

### 硬反例

1. 三个Projection摘要正确，但完整Application V2 Input Current或Model Projection读取失败仍解析/执行；
2. 使用V1 Request/Input Projection别名、wrapper、JSON重编码或type-pun满足V2依赖；
3. Application Request、Identity/Scope、Action Coordinate、Observation、Generation/Association任一Revision/Digest漂移；PendingAction跨OwnerID/RequestDigest漂移，或Tool-local wrapper Revision不为1；
4. Model `Calls != 1`，或CallID/Name/canonical arguments与完整Projection不一致；
5. 读取不存在的`PendingActionExactRefV2.PayloadDigest`，或跳过Model bytes→Application Projection/Identity→PendingSubject Identity/PendingAction→Candidate Payload完整digest链；
6. CallName映射到另一Tool，或Surface/Capability/Tool/Schema/SourceCandidate任一换ID、Revision、Digest、Owner；
7. Route Ref/Matrix/Generation/BindingSet五水位/active route任一漂移，或从Identity SettlementOwner而非Route Provider解析执行Provider；
8. Candidate Payload使用Ref、nil/空/oversize Inline、错误Length/Schema/LimitPolicy/ContentDigest，或未deep-copy bytes；
9. Association按非ID弱查、Ref不exact、非active/expired；Generation不是以Association内Generation Ref读取或返回Projection漂移；
10. Route Provider跨BindingSet、ComponentManifest不匹配、Provider Current非active/fresh或BindingSet/SemanticDigest不回扣Route/Association；
11. RequestedExpires<0仍接受、=0被当已过期、>0延长来源或已过期仍create；
12. 未以fresh nowS1先校验Request/SourceSubject，Model full Ref/Validate/Calls==1/ordinal0及Identity ordinal presence/value、CallID/Name/arguments任一不exact仍继续；
13. Tool Resolve产生多义CallName映射，S2重新Resolve等价对象而非按exact Candidate ObjectRef Inspect，或六个current ref/closure漂移；
14. requested bound未进入Issuance Subject/恢复请求；Resolve未先Inspect，非权威NotFound仍create，或并发loser比较fresh-time body而不返回winner lease；
15. Boundary使用issuance弱读而非Expected Ref exact读、未再次验证Route/Provider current、未压CallerDeadline，或将Projection当Authority/Fence/Permit/Review；
16. typed-nil依赖、nil context、Unavailable/Indeterminate被当NotFound后继续；
17. Harness私有类型、event JSON、compat tool calls或摘要拼接攻击进入Tool解析链。
18. Projection顶层`Subject`与Issuance BindingSubject不同、顶层requested为0但Issuance requested>0、反向漂移，或Expires超过Issuance正数上界仍被Seal/Create/任一Inspect接受；
19. CandidateV3/Closure缺Tool-local PendingAction Ref，或ID/RequestDigest与Application Input真实坐标漂移；Tool-local Revision不为1，或ExpectedOwner不是由Route Provider派生；
20. ClosureDigest未使用冻结domain/version/discriminator、canonical body包含自身digest、nil与空值被归一为同一内容，或S2不重算exact digest。
21. S1在Route/Provider Current之前Resolve Candidate，或S2在exact复读Association/Generation/Route/Provider前Inspect Candidate；
22. Candidate Resolve request缺完整Route Projection/ProviderCurrent，或ExpectedOwner的Role/ComponentID/ManifestDigest未按Route Provider构造；
23. Tool要求Application/Harness提供不存在的PendingAction Owner/current nominal，或从Harness/Identity SettlementOwner猜ExpectedOwner而非Route Provider，仍Seal Candidate/Gateway。
24. S2返回相同稳定坐标与合法新Checked，却因Checked/Expiry不等于S1而错误Conflict；
25. S2 expiry缩短且fresh now已越过新窗口仍create，或S2 expiry变晚后把Candidate/lease延长到S1最早上界之后；
26. 最终BindingV2 lease只取S2上界而遗漏S1 Application Input/PendingAction真实窗口、SurfaceInvocationBinding所选Surface Manifest、InputContract、Association/Generation/Route/Provider或任一Registry Object Current较早expiry。
27. 一个Surface Current与两个Registry Object Currents时间不同却被强制改写为统一时间，或Projection未取三者最早expiry；必须保留各source exact current并取最小上界。
28. Surface被伪成Registry Record、Capability/Tool source Owner漂移，或InputSchema Authority不是exact Tool Registry Object Current Ref；必须零lease、零CandidateV3。

### 时间语义正向例

- 合法refresh：S1 PendingAction current为`Checked=100, Expires=200`，S2同稳定坐标返回`Checked=120, Expires=180`，fresh now=130；S2 ValidateCurrent成功，Candidate仍保存`100/200`，lease expiry取180及其他S1/S2上界的更小值。
- 合法但不得延长：S1为`100/180`，S2为`120/240`；Candidate仍为`100/180`，最终lease不得晚于180及其他较早上界。
- 必须拒绝：S1为`100/200`，S2为`120/125`，fresh now=130；新窗口已穿过验证点，返回PreconditionFailed，Store create/Watermark/Gateway均为零。

## PD-TM-05：Model Route Tool Compatibility只读投影候选

完整字段、historical/current/Prepared Association、S1/S2、Owner/import边界、迁移与反例见
[Model Route Tool Compatibility V1联合候选](model-route-tool-compatibility-v1.md)。本节只保留
跨Owner摘要；专门文件是PD-TM-05唯一详细候选，不代表Model Owner已经冻结或实现。

### 用例与Owner

Tool Owner已能从exact current Surface确定性编译Model Invoker公开neutral `Tool`，但不能
证明某条Route的provider dialect当前接受名称、Schema、strict与parallel组合。Model
Invoker Owner拥有Route、Protocol、Adapter dialect与Compatibility判定；Tool只提供exact
Surface及Compiled Tools坐标，Harness/Application只消费结果，不复制厂商规则。

### 候选公共输入

Model Owner候选发布Prepared-scoped historical Fact、current Projection、create-once Association
与exact Readers，至少绑定：

- exact `RouteID`与当前Route/Catalog Evidence坐标；
- exact Runtime neutral Assembly current与Prepared historical/current坐标；
- Model sealed request中的`RequestToolsDigest`及完整公开`[]modelinvoker.Tool`；
- `ToolChoice`、`ParallelToolCalls`与会改变provider映射的公开Request字段；
- Requested expiry/caller deadline只能缩短Owner current窗口。

Tool侧不冻结Model nominal的最终包名；联合冻结时应由Model Invoker public contract拥有，
禁止把provider/internal dialect、厂商SDK DTO或raw request下沉到Tool。

### 候选Projection与不变量

输出至少逐字段绑定Prepared、Assembly、RouteID、Route Evidence/AdapterID/Protocol/Model、
RequestToolsDigest、ToolControlDigest、每个Tool的`exact|transformed|degraded|rejected`判定、
完整Residual、Checked、Expires与ProjectionDigest。它只是Model表达兼容事实，不授
Authority/Review/Fence/Provider执行权。

- S1读取Compatibility；Model Prepared Fact封存其exact Ref；每次Provider actual-point前S2
  复读同Ref current，不能按RouteID重新选择“等价”结果；
- Tool Surface、Compiled Digest、Route Evidence、Protocol、Model或adapter任一漂移均Conflict；
- Unavailable/Unknown/expired/rejected、typed-nil、nil/canceled context、clock rollback均零Provider；
- provider adapter升级导致规则变化必须产生新Revision/Digest，不能原位改写历史Projection；
- `Request.Validate()`、粗粒度`CapabilityToolCalling`或Tool portable profile通过均不能伪造该事实。

### 兼容影响

这是additive Model Owner只读面；Tool不修改Model Provider接口，也不包装现有Route Gateway。
闭合前owner-local portable compile保持YES，production“Route可调用该Surface”保持NO-GO。
当前P0是Model public historical/current/Association nominal与Readers、Prepared对
ToolControl/Compatibility的绑定以及Route/Catalog exact-current Evidence尚未闭合；所有Provider
Open/Stream/continuation路径统一Gate是P1。

## PD-TM-PKG-01：Supply Chain Artifact与Trust中立只读Port

用户已确认Package首个完整边界为Verify与Admission同一CAS。Runtime public
`ports/supply_chain_artifact_trust_v1.go`已经落盘下列唯一neutral nominal；Tool直接导入这些
类型，没有复制同形类型，也没有以URL/tag/raw key/布尔trust私建兼容口。

| 项 | 正式Delta |
|---|---|
| 用例 | 对已进入State Plane的exact OCI Artifact、Sigstore Bundle、in-toto Statement与Trust Material做离线读取，并复读immutable current Trust Policy |
| 语义Owner | Artifact Owner拥有bytes；Trust/Policy Owner拥有Policy/Root；Runtime ports只拥有neutral nominal；Tool拥有Verification与Registry Admission语义 |
| 输入 | `SupplyChainArtifactContentRefV1`、`SupplyChainTrustMaterialRefV1`、`SupplyChainTrustPolicyDocumentRefV1`、`SupplyChainTrustPolicyCurrentRefV1` |
| 输出 | bounded exact content Reader、Trust Material/Policy Document exact Reader、全字段immutable Trust Policy Current Projection |
| 不变量 | sha256、media type、size、digest、historical Policy与current lease逐字段exact；URL/tag/latest/raw key禁止；Ref与Projection不得type-pun |
| Effect/Recovery | Reader不得联网；Fetch是独立受治理Effect；read/close/digest失败Fail Closed；lost reply只重读同exact Ref |
| 反例 | typed-nil/短读/多读/close error、same Ref换bytes、Policy刷新同Ref、clock rollback、Unavailable当NotFound |
| 兼容 | additive；不改legacy Tool/MCP/Action Port；public nominal已落盘，Tool owner-local Verify/强Admission已实现 |

唯一public类型与Reader形状已由Runtime public ports落盘，Tool设计镜像见
[Tool Package Offline Verification与Admission V1](package-offline-verification-v1.md#41-runtime-neutral-artifact与trust-port-delta)
；Runtime只拥有neutral nominal，Artifact/Trust Owner仍拥有repository与语义。

## PD-TM-PKG-02：Verification-aware Package Admission强写Port

Tool Registry Owner新增强写口：同一锁/事务复读exact Package current、Verification Fact/current、
historical+current Trust Policy、Artifact Binding及expected Registry Revision后，才允许
`submitted -> admitted`。输出successor Package Registry Record/current exact Ref；V1不自动active。

generic `Registry.Transition("package", admitted|active)`在production面必须Fail Closed或明确限
legacy test fixture，禁止先transition再异步附Verification。CAS回包丢失只Inspect exact Registry
revision；过期、换Policy/Artifact、旧Package current或依赖状态不足均零写。

## 不提出的Delta

- 不提出新的Runtime治理链；Operation V3/Application Coordinator已闭合并直接复用；
- 不提出Remote Inspect Delta；现有`OperationEffectRelationV3`、独立Inspect Effect和Inspection Settlement已覆盖；
- N=1 Action是active Run内Effect，不触发pre-run Evidence Delta；真实无Run外部connect/discover/package动作只有在Runtime Owner发布相应Admin applicability/profile或进入真实管理Run后才可执行，否则保持unsupported；
- 不提出Model Invoker内部实现或publish/write Delta；本资产只声明Tool消费边界。Model Owner另行冻结`ToolCallCandidateObservationProjectionV1`公开exact只读Reader，Tool不得私建替代Reader。
