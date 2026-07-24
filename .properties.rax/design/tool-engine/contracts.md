# Tool Engine合同、状态机与调用链

## 1. 合同版本策略

Tool/MCP领域合同使用独立语义版本，不复用legacy Runtime `v1alpha1`对象：

| 合同 | 初始版本 | Owner |
|---|---|---|
| Capability | `praxis.tool-mcp.capability/v1` | Tool Engine |
| Tool Descriptor | `praxis.tool-mcp.tool/v1` | Tool Engine |
| Tool Surface | `praxis.tool-mcp.surface/v1` | Tool Engine |
| Action | Wave1历史`praxis.tool-mcp.action/v1`；G6A领域切片`praxis.tool-mcp.action/v2` | Tool Engine |
| Receipt/Result | Wave1历史`praxis.tool-mcp.result/v1`；G6A settled Result使用`praxis.tool-mcp.result/v2` | Tool Engine |
| Typed Domain Result | `praxis.tool-mcp.domain-result/v2` | Tool Engine |
| Settlement Apply/Tool Result | `praxis.tool-mcp.result/v2` | Tool Engine |
| Package | `praxis.tool-mcp.package/v1` | Tool Engine |
| MCP Server/Connection/Snapshot | `praxis.tool-mcp.mcp/v1` | MCP Gateway |
| Process Event | `praxis.tool-mcp.event/v1` | 对应领域Owner |

每个持久对象至少携带`ContractVersion`、稳定ID、Revision、Content Digest、Owner Binding、Created/Updated时间；外部来源对象还携带Source Identity/Epoch/Sequence。Semantic Version只描述对使用者的兼容版本，Revision描述同一事实的单调变化，两者不得互换。

## 2. 核心对象

| 对象 | 必需字段 | 关键约束 |
|---|---|---|
| `CapabilityDescriptor` | ID、Namespace、SemanticVersion、Owner、Input/Output Schema、EffectKinds、RiskClass、ActionScopeSchema、ReviewProfile、AuthorityRequirement、BudgetRequirement、SandboxRequirement、EvidenceRequirement、Compatibility | 只描述语义；不包含Transport、Credential值或模型方言 |
| `ToolDescriptor` | ToolID、CapabilityRef、ProviderBinding、ArtifactDigest、Mechanism、Schema、Timeout、Concurrency、Cancellation、Idempotency、ConflictDomainTemplate、ResultLimit、Conformance、Residual | 一个Tool实现一个或多个精确Capability版本；Effect要求不得弱于Capability |
| `ToolSurfaceManifest` | SurfaceID、Revision、ResolvedPlanDigest、ProfileDigest、CapabilityGrantDigest、OrderedEntries、Dialect、ExpectedInjectionDigest、ActualInjectionDigest、Residuals、Expires | 条目稳定排序；visible/allowed/pre-approved分别编码；Run内不可无声变化 |
| `ToolSurfaceEntry` | CapabilityRef、ToolRef、ModelName、InputSchemaDigest、DescriptionDigest、Order、Visibility、AdmissionClass、MechanismDigest | ModelName变化、Schema变化或Effect要求变化均产生新Surface Revision |
| `ToolPackageManifest` | PackageID、Version、Publisher、ArtifactDigest、Signatures、Dependencies、SupportedProfiles、Descriptors、RequestedCapabilities、EffectKinds、Sandbox/Review要求、Cleanup、Provenance | 安装、注册、装配、运行授权四阶段分离 |
| `ActionCandidateV2` | 既有字段与六项`OwnerCurrentRefV1` | 冻结兼容历史；不得包装为PD-TM-04 P4能力 |
| `ActionCandidateV3`（候选） | PendingAction rev1、Model historical Source Ref、Surface Repository Current、Capability/Tool Registry Object Current、InputSchema Current、InputContract Ref、Payload/LimitPolicy/Schema、Route ExpectedOwner | Surface不冒充Registry；Schema Authority绑定Tool Current；InputContract Ref进入canonical digest；P4只经BindingV2使用 |
| `BindingCurrentProjectionV2`（候选） | CandidateV3、InputContract Ref+immutable Projection、完整ClosureV2、Checked/Expires | Tool Owner在Watermark前原子create-once；BindingV2 Ref是P4唯一恢复根，P5只派生handoff proof |
| `SingleCallToolActionCoordinationWatermarkV1` | Watermark ID/Revision/Digest、Tenant/Application Request/Scope、Model Projection Ref/ObservationDigest、CanonicalCommandDigest、Stage、Runtime Attempt、execute Enforcement/Handoff及后续exact refs、Owner、Created/Updated | Model Reader通过后才create/CAS；当前V1到Runtime Attempt即停止。未来只有Runtime V2 actual-point executor接受完整current proof后才可推进boundary/物理effect；已存在boundary一律inspect-only |
| `ToolProviderBoundarySourceRefV1` | WatermarkID、WatermarkRevision、WatermarkDigest | Tool Owner exact source ref；只引用已CAS Watermark，不携带或授予Authority |
| `ActionReservationFactV2` | ReservationID/Revision=1/Digest、`core.TenantID`、Action exact ref、`ApplicationAttemptRefV1`、IntentDigest、SessionRef、DomainSubjectDigest、Reserved/Expires | 在Runtime Dispatch Attempt出现前create-once；不包含`OperationDispatchAttemptRefV3`。Reservation expiry不得晚于Candidate；后续DomainResult用`RuntimeAttemptCausalityV1`证明因果关联 |
| `ToolExecutionReceipt` | Action/Attempt/ProviderOperationRef、State、PayloadRef、Usage、Residual、Source/Epoch/Sequence、EvidenceRef、ObservedAt | Provider事实只作Observation；同序号换内容冲突 |
| `ToolDomainResultFactV2` | ResultFact ID/Revision/Digest、`core.TenantID`/ScopeDigest、Action/Reservation/`ApplicationAttemptRefV1`、`RuntimeAttemptCausalityV1`、`ProviderAttemptObservationRefV2`、prepare/execute各一个`OperationDispatchEnforcementPhaseRefV4`与`OperationScopeEvidenceConsumptionRefV3`、`SchemaRefV2`/PayloadDigest/Revision、`[]Residual`、`EffectOwnerRefV2`、CreatedAt | Tool Owner复读全部因果事实并独立Inspect后CAS。Fact是不可变历史权威事实，不因Reservation随后过期而失真 |
| `ToolApplySettlementFactV2` | Apply ID/Revision/Digest、Tenant/Scope、Action/Reservation/DomainResult exact refs、fresh `OperationInspectionSettlementRefV4`及其Settlement/Association/Guard/Projection/DomainResult typed refs、`ToolOutcomeV2`、`ToolDispositionV2`、`EffectOwnerRefV2`、AppliedAt | create-once；必须验证Runtime V4 public closure及current window。Unknown/indeterminate不得Apply；Outcome/Disposition只由Tool Owner表达 |
| `ToolResultV2` | Result ID/Revision/Digest、Action/Reservation/DomainResult/Apply refs、Runtime V4 DomainResult/Settlement/Association/Guard/Projection refs、`ToolOutcomeV2`、`ToolDispositionV2`、`SchemaRefV2`/PayloadDigest/Revision、`[]ObjectRef` Artifacts、`[]Residual`、FinalizedAt | 只能在ApplySettlement V4 CAS后形成；exact refs同时关闭Tool领域Fact与Runtime V4 public closure，任一摘要漂移均拒绝 |
| `BatchResultManifest` | BatchID、OrderedActionRefs、MemberStates、MemberResultRefs、AggregateDigest | 每个成员独立终态；aggregate不覆盖Unknown/Skipped/Synthetic |

### 2.1 Tool Outcome/Disposition V1冻结

这是Tool Owner自有合同，不复用Runtime V3 Disposition，Runtime V4也不参与选择：

| 类型 | 冻结值 |
|---|---|
| `ToolOutcomeV2` | `succeeded`、`failed` |
| `ToolDispositionV2` | `confirmed_applied`、`confirmed_not_applied` |

合法组合矩阵：

| Outcome | Disposition | 合法性 | 典型语义 |
|---|---|---|---|
| `succeeded` | `confirmed_applied` | YES | Tool语义成功且Effect已发生 |
| `failed` | `confirmed_applied` | YES | Provider返回错误或业务失败，但Effect已发生或部分发生，必须保留Residual |
| `failed` | `confirmed_not_applied` | YES | pre-provider确定拒绝，或独立Inspect确认Effect未发生 |
| `succeeded` | `confirmed_not_applied` | NO | 成功却确认未执行，语义自相矛盾 |

`unknown`、`indeterminate`不是上述枚举值，也不是第四种组合；它们是未收口水位，只能Inspect/Reconcile，禁止`ApplySettlementV4`和`ToolResultV2`。此表与既有Tool/MCP语义无冲突：它保持Provider回包仅为Observation、Unknown只Inspect，并避免把Runtime V3 Disposition移植到V4。

### 2.2 Exact坐标与current窗口V1

1. `ApplicationAttemptRefV1`是中立坐标，字段精确为`ID + Revision + Digest`；它不属于Application实现类型，也不等同Runtime Dispatch Attempt。
2. `ActionReservationFactV2`只绑定`ApplicationAttemptRefV1`、IntentDigest、DomainSubjectDigest和Session；不得提前或事后把`OperationDispatchAttemptRefV3`塞回Reservation。
3. `ToolDomainResultFactV2`首次绑定live Runtime `OperationDispatchAttemptRefV3`，并同时保留Reservation与`ApplicationAttemptRefV1`；其`RuntimeAttemptCausalityV1`必须证明Operation/Attempt/Effect来自同一Reservation链。
4. Provider Observation精确使用Runtime公开`ProviderAttemptObservationRefV2`；prepare/execute Receipt精确使用两项`OperationDispatchEnforcementPhaseRefV4`，并各自强制一个`OperationScopeEvidenceConsumptionRefV3`。不新增字符串ReceiptRef，也不得用`OpaquePayloadV2`代替这些引用。
5. `OwnerCurrentRefV1`与SourceCandidateCurrent只保留给ActionCandidateV2兼容路径。P4中Model SourceCandidate是historical exact Ref；Surface来自Tool Surface Repository，Capability/Tool来自Registry Record+Descriptor exact adapter；InputContract/BindingV2 immutable，Route ExpectedOwner独立。
6. `ToolDomainResultFactV2`是历史权威Fact。其current projection由Tool Owner在每次读取时重新签发，TTL必须`> 0 && <= 30s`；签发前复读Provider Observation、prepare/execute Enforcement、两项Consumption及Action/Reservation causal exact。30秒只是V1合同上限，不是生产SLA。
7. truthful late result不因历史Reservation在读取时已过期而被否定；但任何新的Provider handoff仍必须使用当时fresh Candidate/Reservation/Fence。Apply必须另外持有fresh Runtime V4 `OperationInspectionSettlementRefV4`并经Association Inspect闭合。
8. `RuntimeAttemptCausalityV1`字段精确为`ReservationRef + ApplicationAttemptRefV1 + OperationSubjectV3 + OperationDigest + OperationDispatchAttemptRefV3 + EffectID + EffectRevision + IntentDigest + Digest`。Tool Owner只在Action/Reservation/Tenant/Scope/Session、Attempt.OperationDigest/EffectID/IntentRevision/IntentDigest全部exact一致后签发；它证明关联，不签发Runtime Authority。
9. `ToolDomainResultCurrentProjectionV1`字段精确为`FactRef + RuntimeAttemptCausalityV1.Digest + ObservationRef + PrepareEnforcementRef + ExecuteEnforcementRef + PrepareConsumptionRef + ExecuteConsumptionRef + Owner + CheckedUnixNano + ExpiresUnixNano + Digest`；不得把原始对象序列化进Opaque JSON。

## 3. Effect kind、Conflict Domain与OperationScope / RunStart / RunSettlement Requirement

### 3.1 Effect kind全集

全部kind采用namespaced值并映射到Runtime `OperationEffectIntentV3.Kind`：

| Tool/MCP Effect kind | 典型动作 | 默认Requirement |
|---|---|---|
| `praxis.tool/execute` | 本地或远程Tool调用 | Run；完整Review/Authority/Budget/Scope/Sandbox/Fence |
| `praxis.tool/cancel` | 取消原Action | Run；关联原Effect；不能宣称原Effect未发生 |
| `praxis.tool/inspect` | 远程查询原Attempt | Run；独立Inspect Effect；Relation绑定原Intent revision |
| `praxis.tool/cleanup` | 释放Tool资源或补偿 | Run或Termination；独立Effect与Cleanup Owner |
| `praxis.tool/package-fetch` | 下载Package | Admin；供应链/网络/预算/凭据门禁 |
| `praxis.tool/package-register` | 注册验证后的Package | Admin；Formal Commit类Effect |
| `praxis.tool/package-revoke` | 撤回Package/Tool | Admin；阻止新绑定，保留历史 |
| `praxis.mcp/connect` | 建立远程或stdio连接 | Admin或Run；视Session绑定策略 |
| `praxis.mcp/discover` | tools/resources/prompts发现 | Admin或Run；网络/披露/大小边界 |
| `praxis.mcp/refresh` | 更新能力快照 | Admin或Run；不得无声替换Run Surface |
| `praxis.mcp/call` | MCP tools/call | Run；映射到`praxis.tool/execute`语义 |
| `praxis.mcp/read-resource` | resources/read | Run；Context Source，数据披露与Injection边界 |
| `praxis.mcp/get-prompt` | prompts/get | Run；外部提示资产，不是系统指令 |
| `praxis.mcp/cancel` | 普通请求或Task取消 | Run；关联原Effect |
| `praxis.mcp/inspect` | Task/Provider远端Inspect | Run；独立Effect，禁止盲重试 |
| `praxis.mcp/drain-close` | Drain/Close Session | Run、Termination或Admin；报告Residual |

这份全集是组件允许声明的上界；具体Descriptor只能声明实际实现且通过Conformance的子集。

### 3.2 Conflict Domain

Conflict Domain由Descriptor模板与Action Scope编译，不由模型自由填写：

- Workspace/File：`tenant/<t>/workspace/<w>/path/<normalized-path>`；
- Database：`tenant/<t>/db/<resource>/<record-or-range>`；
- Remote API：`tenant/<t>/provider/<provider>/<account>/<resource>`；
- Package/Registry：`tenant/<t>/tool-registry/<namespace>/<name>`；
- MCP Session：`tenant/<t>/mcp/<server>/<connection-epoch>`；
- 无法可靠收窄的外部写Effect使用Provider/Account级域，宁可降低并行度；
- Domain稳定范围至少为Tenant，不允许跨Tenant共享幂等或冲突键。

同域写Effect串行或使用Owner CAS；只读动作只有在Descriptor证明不发生费用、披露外溢或Provider continuation冲突时才可并行。

### 3.3 OperationScope、RunStartRequirement与RunSettlementRequirement

- Model产生的Action使用run类型`OperationScope`，必须绑定完整Run、Harness Session、PendingAction和Source Candidate；
- SDK/CLI/API交互式调用默认创建Application Workflow与run类型OperationScope，不能绕过Run；
- Package获取/注册/撤回和全局Registry维护使用`OperationScopeAdminV3`，不伪造RunStartRequirement或RunSettlementRequirement；
- 需要Sandbox、Run Credential或Run Budget的Tool把这些声明为精确`RunStartRequirement`；Instance创建前不得执行；
- Termination cleanup使用`OperationScopeTerminationV3`并关联原Effect/Residual；它只在Owner明确发布termination_report参与者时贡献`RunSettlementRequirement`；
- 阻塞CompleteRun的未决Action由Tool Owner发布namespaced `RunSettlementRequirement`与Participant Fact；普通OperationScope本身不自动阻塞Run收口；
- Registry读和本地静态Descriptor校验若不接触外部系统且不产生权威Evidence，可以作为纯读；一旦网络、进程、费用或正式提交即成为Operation Effect。

## 4. Pre-run Evidence裁决

本地Descriptor/Schema验证、纯Surface编译和不触达外部系统的协议单元测试不触发pre-run Evidence。真实`connect/discover/package-fetch/register/revoke`若在无Run状态接触外部系统并产生Operation Observation/Settlement，则必须复用统一`OperationScope-aware Evidence V3`，或运行在真实管理Run中；Evidence V3未合入且无管理Run时保持unsupported并保证零外部调用。

- Run前Package/Registry/Discovery通过`OperationScopeAdminV3`进入现有Operation治理；
- 其结果先形成Tool/MCP领域Fact与摘要，Binding/Plan只消费这些精确Ref/Digest；
- 无active Run的真实外部管理Operation若要形成权威结果，必须使用统一OperationScope-aware Evidence V3；缺失时保持unsupported且零外部调用；
- Run内Action过程才使用现有Run/Effect Evidence分区；
- 统一OperationScope-aware Evidence V3由Runtime/Evidence Owner发布，Tool/MCP只提交Candidate/Observation和DomainResultFact；组件不得私自实现或伪造Run。

### 4.1 N=1 Action的OperationScope Evidence V3矩阵

| OperationScopeKind | EffectKind | PolicyProfile | Run | Session | Turn | Action | Context |
|---|---|---|---|---|---|---|---|
| `run` | `praxis.tool/execute` | `praxis.tool/single-call-action-v1` | required | required | required | required | required |

五维required projection都必须携带Fact Kind、ID、Revision、Digest、Owner、current window；Action维度绑定Candidate与Reservation，Context维度绑定generation/frame、manifest、injection/conformance。Runtime按Fact Kind调用Harness、Tool、Context各自narrow current reader，Issue前S1读取、handoff前S2复读。Observation lease只覆盖对应phase的handoff，不跨phase、不授予Provider执行权。

该profile只适用于active Run内的N=1 Action，不产生pre-run Evidence。Run未建立、PendingAction未提交或任一required Fact不可读时Fail Closed，不得借Admin Scope或伪造Run进入执行。

最新Evidence V3映射固定为：

- `OperationScopeEvidenceCandidateV3`只携带`Qualification`、Source/Event/Trust/Payload/Causation/Correlation/ObservedAt；
- `ConsumeOperationScopeEvidenceRequestV3`独立携带`ConsumptionID + Handoff + Candidate`；
- `OperationScopeEvidenceConsumptionFactV3`原子绑定`Qualification + Handoff + CandidateDigest + Record`；
- prepare与execute分别使用独立Qualification、Handoff、EventID、SourceSequence、ConsumptionID；不得复用资格、Handoff、phase或把Handoff伪造进Candidate。

## 5. Action领域状态机

Tool/MCP不复制Application/Runtime状态机，只保存自己的领域事实和公共水位：

```text
candidate
  -> reserved
  -> prepare_enforced
  -> prepare_observed
  -> execute_enforced
  -> provider_observed
  -> domain_result_ready
  -> runtime_settled
  -> settlement_applied ------> result_ready
  -> outcome_unknown
       -> inspect_original_attempt
       -> provider_observed ----> domain_result_ready
```

规则：

1. `candidate -> reserved`由Action Domain Owner create-once；
2. 每次使用Candidate/Reservation前由Tool current reader复读Owner/revision/digest/Created/Expires及六项固定`OwnerCurrentRefV1`；Candidate current expiry取全部上界最小值，Reservation不得晚于Candidate；
3. Reservation只绑定中立`ApplicationAttemptRefV1`、Intent/Subject/Session，不绑定Runtime Attempt；`prepare_enforced`和`execute_enforced`才分别吸收对应`OperationDispatchEnforcementPhaseRefV4`，execute还必须绑定同一`OperationDispatchAttemptRefV3`；
4. `prepare_observed`与`provider_observed`各自只吸收独立`OperationScopeEvidenceConsumptionRefV3`；Provider结果使用`ProviderAttemptObservationRefV2`，仍只是Observation；
5. `outcome_unknown`绑定原Operation/Attempt/phase/provider attempt；任何恢复只Inspect原Attempt；
6. 远端Inspect创建独立Effect，`Relation`绑定原Intent ID/Revision；不得用新execute Attempt替代；
7. `domain_result_ready`只由Tool Owner独立Inspect、复读Observation、两phase Enforcement、两Consumption与Action/Reservation因果链后CAS typed DomainResult；该Fact为历史truth，晚到时不因Reservation此刻过期而被抹除；
8. `runtime_settled`只接受exact引用该DomainResult与prepare/execute两项`OperationSettlementEvidenceBindingV4`的Runtime Settlement V4；Runtime只投影settled，不判断领域Outcome/Disposition；
9. `settlement_applied -> result_ready`由Tool Owner create-once ApplySettlement V4，ToolResult关闭DomainResult/Settlement/Association/Guard/Projection/Apply exact refs；只接受三种合法Outcome/Disposition组合，Unknown/indeterminate不能进入；
10. pre-provider拒绝若需形成终态，必须由Tool Owner独立Inspect后形成typed DomainResult/领域Outcome，再由Runtime绑定并投影settled；Tool ApplySettlement后才可形成对应Result；
11. Cancel独立于原Action状态。原Action在Inspect/Settlement前仍可为Unknown。

### 5.1 Application-owned `SingleCallToolActionPortV1`

Application拥有并发布`SingleCallToolActionPortV1`、`SingleCallToolActionRequestV1`和`SingleCallToolActionResultV1`。Tool Owner可在`tool-mcp`内提供`SingleCallToolActionAdapterV1`实现，只依赖Application公共`contract/ports`，不得依赖Application实现。Application绝不反向import `tool-mcp`；Harness也不import Tool实现。live当前无production composition root；G6A仅由test fixture手工注入Adapter，未来production root由宿主Owner在G6B前闭合。该Port不是新的Runtime Gateway，也不拥有Harness或Runtime事实。每次调用的基数必须精确为`1`；请求出现数组、成员数不明或`N > 1`时，在创建Candidate/Reservation和调用Provider之前Fail Closed并保持零状态变化。

`SingleCallToolActionPortV1.Execute`在Tool侧固定为同一canonical command的`start-or-inspect`，而不是每次投递都启动新执行。Model Owner的公共只读exact Reader与producer-side atomic Ensure已经终审YES：Application中立DTO只可传递完整`ToolCallCandidateObservationProjectionV1` Ref坐标，Tool `applicationadapter`必须经Reader复读完整Projection，并逐字段验证Ref、Observation digest与`Calls == 1`。Reader成功前不得创建Tool Watermark、Candidate或Reservation。Tool不调用Ensure、不持有Model publish/write Port，也不得定义Reader实现。

Reader通过后，Tool Owner才可create/Inspect内部协调事实`SingleCallToolActionCoordinationWatermarkV1`，精确字段为：

- `WatermarkID + Revision + Digest + ContractVersion + core.TenantID`；
- `ApplicationRequestID + ApplicationRequestRevision + ApplicationRequestDigest + OperationScopeDigest + ToolCallCandidateObservationProjectionRef全字段 + ObservationDigest + CanonicalCommandDigest`；
- `Stage + Owner + CreatedUnixNano + UpdatedUnixNano`；
- 随阶段单调出现的`ActionCandidateRef + ActionReservationRef + OperationDispatchAttemptRefV3 + ExecuteEnforcementRef(OperationDispatchEnforcementPhaseRefV4) + ExecuteHandoffRef(OperationScopeEvidenceProviderHandoffRefV3) + ProviderAttemptObservationRefV2 + ToolDomainResultFactRefV2 + ToolApplySettlementFactRefV2 + ToolResultRefV2`。每个Ref只在其事实已权威存在后写入；`provider_boundary_crossed`强制Runtime Attempt与同Attempt execute Enforcement/Handoff，Observation尚未形成时保持空并按原Attempt Inspect。已写Ref必须为public exact typed ref，禁止弱字符串或Opaque JSON。

`WatermarkID`稳定键由`TenantID + ApplicationRequestID + ApplicationRequestRevision + OperationScopeDigest`导出。`CanonicalCommandDigest`必须覆盖Application Request ID/Revision/Digest、OperationScopeDigest、Reader返回的Projection Ref全字段与Observation digest、唯一Call的canonical内容、PendingAction ID/Revision/RequestDigest、Run/Session/Turn/Action坐标、Capability/Tool/Schema/SourceCandidate exact refs、canonical payload digest、`praxis.tool/execute`和`praxis.tool/single-call-action-v1`；current lease时间不进入canonical command。V1固定`CanonicalArgumentsDigest == PendingAction.PayloadDigest == Candidate.Payload.ContentDigest`：PendingAction payload就是Model唯一Call的canonical arguments，不存在隐式schema转换。任一digest不相等时必须在Watermark/Candidate前Fail Closed，零Gateway/Provider。未来若允许参数转换，必须另立由绑定Settlement Owner产出的typed Transformation/Association Fact与只读Port Delta，不得放宽V1。相同Request ID/Revision但RequestDigest、Scope、Model Projection或CanonicalCommandDigest任一变化均为冲突，零新写、零Provider调用。

Watermark阶段只允许单调转换：

```text
request_recorded
  -> candidate_recorded
  -> reservation_recorded
  -> runtime_attempt_bound
  -> provider_boundary_crossed
  -> provider_observed
  -> domain_result_recorded
  -> settlement_applied
  -> result_settled
```

规则：

1. `Execute`第一步必须调用Model已公开只读Reader，按调用方提供的exact Ref复读完整`ToolCallCandidateObservationProjectionV1`；Ref任一字段、Observation digest漂移，Reader unavailable/indeterminate，或`Calls != 1`时，Tool Watermark/Candidate/Reservation/Runtime Gateway/Provider调用全部为零。
2. Reader成功后才对Watermark稳定键执行Tool Owner create-once或Inspect；相同canonical command重复投递只返回已存在投影，或从最后一个已提交阶段继续，不能重建已存在事实。
3. 在`provider_boundary_crossed`前发生崩溃，恢复方必须重新通过同一Model exact Reader门、复读Watermark current exact，并对下一阶段Owner事实取得权威`NotFound`，才可用同一canonical command继续一次create-once/CAS；`Unavailable`、`Indeterminate`、超时或读模型缺失都不等于`NotFound`。
4. CAS进入`provider_boundary_crossed`前必须复读execute `OperationDispatchEnforcementPhaseRefV4`为current、`Phase=execute`且`AttemptID`与Watermark Runtime Attempt exact一致，并复读execute `OperationScopeEvidenceProviderHandoffRefV3`为current且其Fact绑定同一Enforcement Phase。Watermark单调绑定这两个public refs后才允许调用Provider。
5. 对既有或未来由Runtime V2 actual-point executor消费的boundary，CAS成功即视为Provider“可能已调用”；若CAS回包丢失或其后崩溃，恢复一律inspect-only。当前V1路径不得创建新的boundary来调用Provider。
6. prepare/execute `OperationScopeEvidenceConsumptionRefV3`只在Provider响应/Observation后进入DomainResult因果链，不得在boundary CAS预填、推测或伪造。
7. 该合同只承诺canonical command create-once/CAS幂等与Provider未知时不盲重派，不承诺消息、RPC或Provider transport exactly-once。
8. Watermark是Tool Owner协调事实，不改变Candidate/Reservation/DomainResult/Apply四项领域合同，不授予执行权，也不扩大`N == 1`。

| 操作 | 必需输入 | 输出与不变量 |
|---|---|---|
| `Execute`（Tool实现入口） | Application Request ID/Revision/Digest、Tenant/Scope、Model Projection exact Ref、PendingAction projection、canonical command其余输入 | 先经Model已公开Reader复读完整Projection并验证Ref全字段、Observation digest、`Calls == 1`；通过后才create/Inspect Watermark。Reader失败零Tool写/零Gateway/Provider；不是新增公共方法 |
| `AdmitSettledPendingAction` | Model Reader返回的完整Projection/唯一Call、已Settlement模型Operation、Run/Session/Turn、PendingAction projection与Tool exact refs | 重算Model/Pending/payload/capability/schema/source关联后create-once Candidate；Application DTO不能替代Model事实；输出不授执行权 |
| `ReserveCurrentCandidate` | Candidate expected revision/digest、中立`ApplicationAttemptRefV1`、IntentDigest、Session、Domain Subject、`now`与TTL | Owner复读六项固定`OwnerCurrentRefV1`；Reservation expiry晚于最小Candidate current expiry时零写；不接收Runtime Dispatch Attempt |
| `ProjectActionApplicability` | Candidate/Reservation exact refs及current window | 只形成Evidence V3 Action required projection，不签发Qualification/Handoff或执行权 |
| `PrepareProvider` | Candidate/Reservation、V4 Permit/Begin、V4.1 prepare Enforcement、prepare Qualification/Handoff、current Fence/Authority/Scope/Budget/Context | 仅经Runtime已公开V4.1/V2受控Provider seam调用Prepare；legacy V2 seam不能代替；缺任一exact ref零Gateway/Provider |
| `ExecuteAfterEnforcement` | 同一Operation/Attempt、Prepared及current proof、prepare Consumption、V4.1 execute Enforcement、独立execute Qualification/Handoff、current facts | Runtime public V2 physical executor在自身入口原子ValidateCurrent后直接执行；Tool isolated Owner flow已接入，V1路径unsupported。execute Consumption只在响应后进入DomainResult |
| `InspectPrepared` | 原Attempt、Delegation、Prepared精确引用 | 只读取原Prepare事实；不得重新Prepare |
| `InspectAttempt` | 原Attempt、Prepared、Provider local attempt精确引用 | 只读取原执行Attempt；不得创建新Attempt或重新Execute；远端Inspect另起受治理Effect |
| `RecordTypedDomainResult` | Candidate/Reservation/ApplicationAttempt、Runtime Operation/`OperationDispatchAttemptRefV3`、`ProviderAttemptObservationRefV2`、prepare/execute各一项`OperationDispatchEnforcementPhaseRefV4`和`OperationScopeEvidenceConsumptionRefV3`、Schema/PayloadDigest/Residual | Tool Owner独立Inspect后CAS `ToolDomainResultFactRefV2`；Operation/Attempt必须源自同Reservation链；不写Runtime Settlement |
| `InspectDomainResultCurrent` | DomainResult exact key、`now`与请求lease TTL | TTL必须`>0 && <=30s`；签发前复读Observation、两phase Enforcement、两Consumption及Action/Reservation因果链；历史Reservation过期不否定truthful Fact |
| `ApplySettlementV4` | DomainResult、fresh current `OperationInspectionSettlementRefV4`及其五项typed refs、Tool Outcome/Disposition、expected Tool revision | 通过Runtime Application-facing只读Association Inspect验证prepare/execute exact、独立且同Attempt后，检查合法组合并CAS Apply/Result；Unknown/indeterminate或非法组合零写 |

`PendingActionDigest`由Harness Owner产生并验证，Tool只能按公开算法重算投影并精确消费，不能重新定义摘要。`ToolResult`摘要不能替代Runtime Settlement、Association或ApplySettlement摘要；不得新增ActionEvidenceBundleRef。

### 5.2 Runtime-neutral Provider Boundary Current Reader V1冻结映射

Tool Owner以`ToolProviderBoundarySourceRefV1{WatermarkID, WatermarkRevision, WatermarkDigest}`暴露已CAS boundary的exact source。该Ref只在Watermark已达到精确`provider_boundary_crossed`且单调绑定同Attempt execute Enforcement/Handoff后形成；同ID/Revision换Digest冲突。

Runtime Owner已冻结公共`OperationProviderBoundaryCurrentReaderV1`；Tool `runtimeadapter`实现它。该Reader只证明Tool Owner boundary CAS current，不闭合actual physical effect入口。V1缺少exact Provider Binding、Prepared Attempt current proof与统一NotAfter，故不得在Reader后包装调用`ControlledOperationProviderPortV1`并宣称安全；actual-point只能走已经落盘的Runtime public V2 Route/Gateway。唯一方法签名：

```text
InspectCurrentOperationProviderBoundaryV1(
  ctx,
  exactRef OperationProviderBoundaryRefV1
) -> OperationProviderBoundaryCurrentProjectionV1
```

Runtime `OperationProviderBoundaryRefV1`精确字段只有：

- `ID + Revision + Digest`。

Tool `runtimeadapter`必须把内部`ToolProviderBoundarySourceRefV1{WatermarkID,WatermarkRevision,WatermarkDigest}`逐字段、无损映射为该Ref，不得增删字段或用其他Tool事实type-pun。

`OperationProviderBoundaryCurrentProjectionV1`精确字段：

- `ContractVersion`；
- `Ref OperationProviderBoundaryRefV1`；
- `Operation OperationSubjectV3 + OperationDigest + OperationScopeDigest`；
- `Attempt OperationDispatchAttemptRefV3`；
- `ExecuteEnforcement OperationDispatchEnforcementPhaseRefV4`；
- `ExecuteEvidenceHandoff OperationScopeEvidenceProviderHandoffRefV3`；
- `Stage + CheckedUnixNano + ExpiresUnixNano + Digest`。

Reader按`exactRef`复读Tool Owner Watermark current exact，验证Watermark ID/revision/digest与内部source逐字段一致，并从同一Watermark投影上述字段；Projection expiry必须大于Checked且不晚于Watermark Owner current窗口、execute Enforcement expiry、execute Handoff expiry三者最小值。Projection digest覆盖全部字段。Runtime seam自行把Projection与其已知Operation、OperationDigest/ScopeDigest、Attempt、execute Enforcement/Handoff逐字段交叉校验。

该Projection只证明“Tool boundary CAS已提交且当前仍精确”，不授予Authority、Permit、Fence或Provider执行权，也不替代execute Enforcement/Handoff。Runtime V2 physical executor必须在自身入口fresh-clock原子ValidateCurrent后直接执行；若验证后再转调V1，调度间隙仍未闭合。

`NotFound`、`Unavailable`、`Indeterminate`、ref/current漂移、expired、same ID/Revision换Digest、cross-Attempt、把其他三字段Ref强转为Boundary Ref的type-pun或Projection字段/摘要错误全部Fail Closed并保持零Provider调用；Unknown只Inspect原ref/attempt。V1路径固定unsupported且调用数为零；当前V2隔离路径必须完整通过public Route/Gateway exact校验，不能降级到V1。production Backend/root未闭合前不得启用真实能力。

### 5.3 Observation → PendingAction → Candidate精确映射

| 来源 | 目标 | 必须相等/可证明 |
|---|---|---|
| Model Reader返回的`ToolCallCandidateObservationProjectionV1` exact Ref/ObservationDigest | Tool消费门 | Ref全字段及Observation digest必须与请求坐标一致；Reader unavailable/indeterminate或漂移时不创建Watermark/Tool事实 |
| `ToolCallCandidateObservationProjectionV1.Calls` | Tool消费门 | 长度必须精确为1；空、多个或未知成员数不创建Watermark/Tool事实 |
| Projection唯一Call的CallID/Name/CanonicalArguments | 已提交`PendingActionV2` | V1要求`CanonicalArgumentsDigest == PendingAction.PayloadDigest`；Pending的Capability/Payload/SourceCandidate投影必须由Harness公开摘要与SourceCandidate exact ref证明；Application不得重写参数或隐式转换 |
| `PendingActionV2.RequestDigest` | `ActionCandidate.PendingActionDigest` | 逐字节相等，并用完整Pending投影重算；只比较Pending ref不够 |
| Pending Capability | Surface Entry/Capability/Tool | namespaced ID、Revision、Digest及Input Schema digest全部current且exact |
| Pending Payload | Candidate Payload | canonical bytes、PayloadRevision、PayloadDigest与Input Schema验证结果一致；`Candidate.Payload.ContentDigest`继续等于Model `CanonicalArgumentsDigest` |
| Pending SourceCandidate | Candidate SourceCandidate | ID、Revision、Digest一致，并能回指唯一Observation/已Settlement模型Operation |
| Candidate | Reservation | Action ID/Revision/Digest、Pending binding、`ApplicationAttemptRefV1`、Intent、Session、Domain Subject一致；Reservation expiry不得晚于Candidate current expiry |

Model Observation Projection和已提交PendingAction都是来源事实，不授予执行权。Tool不得从Application中立DTO、PendingAction payload、event JSON或compat tool calls反推、复制或补造Model事实；Application DTO只传Model exact Ref坐标。Model Owner拥有Projection publish/write、producer-side atomic Ensure与已终审YES的公共Reader合同，Tool只消费exact Reader。只有Harness Owner可提交PendingAction，只有Tool Owner可CAS Watermark/Candidate/Reservation；Harness Identity/Assembler/wiring/production root仍未闭合，Harness Assembly Adapter只投影Application DTO，Tool不负责总装。

## 6. Registry、Surface与Package状态机

### 6.1 Registry

```text
submitted -> validating -> admitted -> active -> deprecated -> revoked
                    \-> rejected
```

- `submitted`不进入Plan；
- `admitted`要求Schema、Artifact、Owner、Effect、Sandbox、Review、Compatibility和Conformance完整；
- `active`由组织Policy/Binding选择，不是Registry自授Authority；
- `revoked`阻止新装配并触发Reconcile标记，但历史事实不可删除。

### 6.2 Tool Surface

```text
surface_candidate -> compiled -> admitted -> bound -> superseded|revoked
```

- 编译输入：Resolved Plan、Model Profile、Harness Capability Profile、Registry Snapshot、Capability Grant；
- 编译输出稳定排序并计算Expected Injection Digest；
- Actual Injection Digest由Harness/Context实际物化后回填关联，不改变Expected；
- Run中Snapshot漂移只产生新Surface候选；当前Run维持原Surface或进入显式Degradation/Reconcile。

### 6.3 Package

```text
fetched -> verified -> registered -> enabled-in-plan -> bound-in-run
```

每一步单独记录。卸载只能停止新引用并执行Cleanup；被Timeline引用的Manifest与Digest仍需可验证。

## 7. 完整调用链

```text
Model ToolCall Observation (exactly one)
  -> Harness model Operation settled + PendingActionV2/waiting_action CAS
  -> Application N=1 gate
  -> Tool exact PendingAction/Surface/Capability/Schema admission
  -> Tool ActionCandidate CAS -> current Reservation CAS (no execution authority)
  -> Runtime Intent -> Admission -> Permit -> Begin
  -> Runtime Enforcement 4.1 prepare
  -> Evidence V3 prepare Issue -> current -> Handoff
  -> controlled Provider Prepare
  -> Evidence V3 prepare Candidate -> Consume(independent Handoff)
  -> Runtime Enforcement 4.1 execute(exact prepare receipt/prepared attempt)
  -> Evidence V3 execute Issue -> current -> Handoff
  -> controlled Provider Execute OR Inspect original Attempt
  -> Evidence V3 execute Candidate -> Consume(independent Handoff)
  -> Tool Owner Inspect/normalize/current reread
  -> CAS ToolDomainResultFactV2
  -> Runtime Operation Settlement V4（仅绑定typed DomainResult并投影settled）
  -> Runtime InspectCurrentOperationSettlementV4（Association/Guard/Projection closure）
  -> Runtime Application-facing readonly InspectOperationSettlementEvidenceAssociationV4
  -> Tool ApplySettlementV4 CAS -> settled ToolResultV2 + Runtime current closure
  == G6A hard stop: no Context/Harness/capability enable/Continuation/Turn progress ==
  == G6B consumes accepted G6A output ==
  -> Application calls ContextTurnRefreshPortV1
  -> pending Context DomainResultFact
  -> Context S2 owner-current reread
  -> Context Owner local atomic ApplySettlement + Generation current CAS
  -> new exact FrameRef/Digest
  -> Application calls public Harness continuation Port
  -> Harness Owner validates Tool Settlement/Association + new Frame and CASes next turn
```

Model stream/completed/cache usage或Provider状态都不能替代`PendingActionV2`、Tool Receipt、Review Verdict、Timeline事实或Run终态。Model路径只允许使用RouteID、Route Gateway及公开execution union，不依赖`model-invoker/internal`、厂商SDK或Raw/Native事件。

Tool Port没有`BuildContinuation*`操作。G6A输出严格止于settled ToolResult与current Inspection，且所有Context/Harness调用、能力启用、Continuation和Turn推进均为零。G6B按CTX-D09-R1执行：Application把Tool settled输出交给`ContextTurnRefreshPortV1`，Context Owner形成pending DomainResult并完成S2后，在本地原子边界提交ApplySettlement与Generation current CAS，产出new exact FrameRef/Digest；该Context链不创建Runtime Settlement。此后Application才把Tool `Inspection.Settlement.ID/Digest`、`Inspection.Association.ID/Digest`和new exact FrameRef/Digest交给Harness公开Port。没有new exact Frame时Continuation零写。

ContextReference若所选Route尚不能受治理地物化，Action Candidate必须Fail Closed；若Plan显式允许降级，只能产生Residual并禁止宣称完整Context支持。

## 8. 并发、流式、取消和恢复

- 并发：Action按ID独立，Conflict Domain控制写并发；Batch结果按固定成员顺序归档；
- 流式：Progress、Partial、Final、Terminal Error分离；Progress进入Evidence/Watch读模型，不推进领域终态；
- 超时：超时只触发取消请求和Inspect，不能推断未执行；
- 幂等：Descriptor声明`provider-key/queryable/non-retryable`并提供测试Evidence；无证据默认non-retryable；
- Evidence Issue/Handoff/Consume丢回包：分别用同一Qualification/HandoffID/ConsumptionID和Candidate digest Inspect或幂等重放；
- Prepare丢回包：只`InspectPrepared`原Attempt事实；
- Begin后Execute丢回包：Runtime标记Unknown，之后只`InspectLocalAttempt`或发起独立远端Inspect Effect；
- `ExecuteAfterEnforcement`超时、断连或回包丢失：保持原Attempt Unknown，只调用`InspectAttempt`，不得再次调用Provider；
- Observation写入丢回包：按source registration/epoch/sequence精确Inspect；
- Tool DomainResult/Runtime Settlement V4/Tool Apply/CAS丢回包：按各Owner的ID/Revision/Digest精确Inspect，不跨层猜测完成；Context Refresh另按CTX-D09-R1的pending DomainResult/S2/local atomic Apply+Generation CAS恢复；
- Provider重启/连接Epoch变化：迟到结果进入历史，不可污染当前Attempt；
- Cleanup无法确认：保留Residual和冲突域，不自动释放为可重用。

## 9. SDK、CLI与API边界

### 9.1 Go SDK

公开能力按已闭合切片逐步开放：当前Package专项SDK提供sealed exact Verify、
Observation/Fact/current Inspect与verification-aware强Admission；其余
Register/Inspect/Resolve、CompileSurface/DiffSurface及Action/MCP能力只在对应Port闭合后开放。
SDK只构造受验证请求并调用Application/领域Port，不暴露Runner handle、Permit签发或直接Provider调用。

### 9.2 CLI

命令全集候选：

- `praxis tool list|inspect|call|cancel|watch`
- `praxis mcp status|discover|refresh|inspect|drain`
- `praxis package verify|register|install|revoke`

可嵌入Runner现已实现`package verify --request-json=<sealed exact request>`，它只做离线Verify且
不串联Admission/Fetch/Enable；尚无production根CLI。`call/install/revoke/connect`等Effect命令
必须展示将创建的Operation/Effect，并支持`--dry-run`只输出Candidate；没有正式Verdict/Permit时
不得执行。CLI不接收明文Secret参数。

### 9.3 transport-neutral API

- Registry：分页、精确Version/Digest、CAS、Watch；当前只读Catalog已闭合exact
  `kind+ID+revision+digest` S1/S2 typed union Inspect，CAS/Watch仍须使用对应治理Port；
- Surface：Compile、Diff、Inspect、ResolveEntry；
- Action：CreateCandidate、Start、Resume、Cancel、Inspect、WatchResult；
- MCP：RegisterServer、Connect、Discover、Refresh、Call、Inspect、Drain、Close；
- Package：Fetch、Verify、Register、Enable、Revoke、InspectProvenance。

网络协议、Webhook实现、数据库和消息系统不在当前设计中选择。任何API调用都必须映射到相同Operation治理链。

## 10. 兼容与迁移

- Schema向后兼容只允许增加可选字段且不扩大Effect/Scope/Review/Sandbox要求；
- 删除字段、改变类型、收窄/扩大权限、改变副作用或默认Review均为不兼容变更；
- Tool Alias只在Plan编译时解析，迁移产出新Plan/Surface/Binding；
- V1 Alias使用[Tool Alias装配期解析合同](tool-alias-v1.md)：Owner+namespaced Alias派生稳定
  ID，revision 1 create、successor current+1 full-Ref CAS，Target必须是active exact Tool；SDK只在
  同一exact Registry Snapshot的S1/S2间返回Alias+Tool闭包。Run/Action/Provider禁止Alias Reader；
- legacy `ActionPort`/`ToolPort`/`MCPPort`只能作为`contained_observe_only`或明确受限迁移面；`GovernedExecutionProviderV2`、Evidence V2和Settlement V3不得包装成V4.1/Evidence V3/Settlement V4；
- 历史Package、Surface、Snapshot、Receipt和Result保持可读；撤回不删除历史；
- MCP标准版本升级必须重新跑协议Conformance与Capability Diff；扩展命名空间独立版本。
