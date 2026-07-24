# Controlled Operation Provider V2合同

## 1. 版本与调用资格

- SemVer：`2.0.0`；canonical domain：`praxis.runtime.controlled-operation-provider/v2`；
- Application Coordinator唯一入口仍是`SingleCallToolActionPortV1`；
- 只有经Harness canonical Declaration、post-binding Conformance及Runtime RouteCurrent Projection共同认证的Tool Application Adapter可持`ControlledOperationProviderPortV2`；
- Application、Tool Adapter、raw Provider均不得持Entry FactPort；raw Provider只能由kernel内部Runner持有；
- V1与V2 Assembly route互斥，V1不改、不升级、不共享Entry index。

## 2. Harness Route nominal refs与Runtime current投影

Harness Assembly Owner拥有唯一canonical `ControlledOperationProviderRouteDeclarationV2`事实、post-binding `ControlledOperationProviderRouteConformanceV2`事实及Route Current发布语义。为避免Go import cycle，跨Owner中立的DeclarationRef、ConformanceRef、CurrentRef、Projection、Reader与Matrix Go类型由`github.com/Proview-China/rax/ExecutionRuntime/runtime/ports`唯一拥有；Harness Assembly导入这些类型并实现Reader。type Owner不等于fact semantic Owner：Runtime不得定义Harness Declaration/Conformance事实struct、Validate、canonical或digest，Harness不得重定义/alias中立nominal struct。

Runtime已落地的additive合同：

```go
type ControlledOperationProviderRouteDeclarationRefV2 struct {
	RouteID              string        `json:"route_id"`
	Revision             core.Revision `json:"revision"`
	PublisherComponentID string        `json:"publisher_component_id"`
	DeclarationDigest    core.Digest   `json:"declaration_digest"`
}

type ControlledOperationProviderRouteConformanceRefV2 struct {
	ConformanceID     string                                          `json:"conformance_id"`
	Revision          core.Revision                                   `json:"revision"`
	DeclarationRef    ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceDigest core.Digest                                     `json:"conformance_digest"`
}

type ControlledOperationProviderRouteCurrentRefV2 struct {
	CurrentID      string                                            `json:"current_id"`
	Revision       core.Revision                                     `json:"revision"`
	DeclarationRef ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceRef ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
	MatrixDigest   core.Digest                                       `json:"matrix_digest"`
	Watermark      core.Digest                                       `json:"watermark"`
	Digest         core.Digest                                       `json:"digest"`
}

type ControlledOperationProviderRouteCurrentProjectionV2 struct {
	ContractVersion             string                                            `json:"contract_version"`
	Ref                         ControlledOperationProviderRouteCurrentRefV2     `json:"ref"`
	DeclarationRef              ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceRef              ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
	Generation                  GenerationArtifactRefV1                           `json:"generation"`
	HandoffID                   string                                            `json:"handoff_id"`
	HandoffRevision             core.Revision                                     `json:"handoff_revision"`
	HandoffDigest               core.Digest                                       `json:"handoff_digest"`
	BindingSetID                string                                            `json:"binding_set_id"`
	BindingSetRevision          core.Revision                                     `json:"binding_set_revision"`
	BindingSetDigest            core.Digest                                       `json:"binding_set_digest"`
	BindingSetSemanticDigest    core.Digest                                       `json:"binding_set_semantic_digest"`
	BindingSetCurrentnessDigest core.Digest                                       `json:"binding_set_currentness_digest"`
	ActiveRouteID               string                                            `json:"active_route_id"`
	ActiveRouteRevision         core.Revision                                     `json:"active_route_revision"`
	ActiveRouteDigest           core.Digest                                       `json:"active_route_digest"`
	ToolAdapterBinding          ProviderBindingRefV2                              `json:"tool_adapter_binding"`
	GatewayBinding              ProviderBindingRefV2                              `json:"gateway_binding"`
	ProviderTransportBinding    ProviderBindingRefV2                              `json:"provider_transport_binding"`
	PreparedReaderBinding       ProviderBindingRefV2                              `json:"prepared_reader_binding"`
	BoundaryReaderBinding       ProviderBindingRefV2                              `json:"boundary_reader_binding"`
	ProviderInspectBinding      ProviderBindingRefV2                              `json:"provider_inspect_binding"`
	ProviderBinding             ProviderBindingRefV2                              `json:"provider_binding"`
	CheckedUnixNano             int64                                             `json:"checked_unix_nano"`
	ExpiresUnixNano             int64                                             `json:"expires_unix_nano"`
	ProjectionDigest            core.Digest                                       `json:"projection_digest"`
}
```

真实Go接口固定为：

```go
type ControlledOperationProviderRouteCurrentReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteV2(
		context.Context,
		ControlledOperationProviderRouteCurrentRefV2,
		OperationScopeEvidenceApplicabilityMatrixKeyV3,
	) (ControlledOperationProviderRouteCurrentProjectionV2, error)
}
```

Runtime `ports`唯一拥有五个V2新增跨边界中立类型：DeclarationRef、ConformanceRef、CurrentRef、CurrentProjection、CurrentReader，并复用既有`OperationScopeEvidenceApplicabilityMatrixKeyV3`。Harness Assembly Adapter只实现接口，不重定义/alias任一类型。

DeclarationRef/ConformanceRef的`ContractVersion`与`ObjectKind`由所属canonical domain/type discriminator固定，不进入struct，也不允许自由`map`或optional metadata。二者`Validate`要求全部string/digest非空、Revision非零和嵌套DeclarationRef合法；同RouteID或ConformanceID下任一其他字段漂移必须Conflict。Declaration Fact与Conformance Fact分别产出对应Ref；`ConformanceID = Derive(RouteID, GenerationRef, BindingSetID)`，Conformance Revision由Harness Fact Owner单调CAS。Generation/Handoff全量只在Conformance Fact与CurrentProjection中出现，不重复进入ConformanceRef。

`ContractVersion`必须精确等于新V2常量`ControlledOperationProviderRouteCurrentContractVersionV2 = "2.0.0"`。Current与Projection的canonical domain/ObjectKind由类型固定，全部字段按上述顺序编码，禁止unknown field、自由map和nil/empty互换。Current Validate要求所有ID、Revision、Digest及嵌套Ref有效。Projection Validate要求：Ref/Declaration/Conformance逐字段闭合；`Generation.Validate()`；Handoff、BindingSet、ActiveRoute的ID非空、Revision非零、Digest有效；七个`ProviderBindingRefV2.Validate()`全部通过且各字段角色的Capability/Owner匹配；`CheckedUnixNano > 0 && CheckedUnixNano < ExpiresUnixNano`，`CheckedUnixNano >= ExpiresUnixNano`必须拒绝；ProjectionDigest清空自身后重算一致。

Handoff与ActiveRoute明确使用flattened exact ID/revision/digest，不新增未定义Ref类型。Generation复用现有`GenerationArtifactRefV1`；七个Binding复用现有`ProviderBindingRefV2`。`PreparedReaderBinding/BoundaryReaderBinding/ProviderInspectBinding`的字段角色由各自closed expected Capability约束，`ProviderBindingRefV2`足以表达Component/Manifest/Artifact/Capability/BindingSet exact身份，不得把Provider/Transport或三类Reader互换。若未来需要超出这些字段的Reader语义，必须另行additive Review，不能type-pun。

`ControlledOperationProviderRouteCurrentRefV2.Digest`是该Ref清空`Digest`后的canonical摘要，覆盖`CurrentID/Revision/DeclarationRef/ConformanceRef/MatrixDigest/Watermark`。Projection使用独立`ProjectionDigest`，其canonical覆盖完整Projection但清空自身`ProjectionDigest`；Ref Digest只作为Projection的一个叶子值，禁止Ref反向包含ProjectionDigest，因而不形成循环摘要。

expected Matrix必须exact为`run + praxis.tool/execute + praxis.tool/single-call-action-v1`。Projection Validate必须逐字段证明：

- `Projection.Ref`等于Reader key的exact CurrentRef；
- `Ref.DeclarationRef == Projection.DeclarationRef`；
- `Ref.ConformanceRef == Projection.ConformanceRef`；
- `Ref.MatrixDigest`等于expected closed Matrix的canonical digest，并与Declaration/Conformance route matrix一致；
- `Ref.Watermark`等于由current Conformance、BindingSet currentness与ActiveRoute current事实确定的组合水位；
- Ref Digest与ProjectionDigest分别重算一致。

Projection还必须关联Generation/Handoff、BindingSet、active-route scan及七个公开Binding。Gatewayfresh读取Projection后分别复读Generation/Handoff、active-route scan和七个Binding current，不能只信Declaration、Conformance或Projection内嵌值。`ControlledOperationProviderRunnerV2`是kernel内部实现细节，不得出现在Declaration、Conformance、Projection或任何public ref。

Reader严格以exact Ref为key：同`CurrentID`但Revision、DeclarationRef、ConformanceRef、MatrixDigest、Watermark或Digest任一不同必须Conflict；只有`CurrentID`从未存在才返回NotFound。

### 2.1 Route Current发布与线性化

- Harness Assembly Route Current Store/Adapter是Fact Owner；
- `CurrentID = Derive(RouteID, MatrixDigest)`，caller不能自由提供另一个ID；
- Revision由该Owner单调CAS；同revision换内容、revision回退/跳过或旧Ref写入Conflict；
- Watermark是Generation、Handoff、BindingSet、ActiveRoute、七个Binding与WiringInventory的canonical current digest；
- post-binding Conformance成功后才可seal并publish exact Projection/Ref；
- 重复同内容publish幂等，lost reply只用detached exact Inspect恢复；changed-content与并发CAS只一胜；
- Tool Adapter与Runtime Gateway只使用composition注入的exact CurrentRef，不提供list/latest/free discovery；
- revoke、expire、stale current或unavailable均Fail Closed；CurrentRef不授Binding，不改变既有`AssemblyInputV1`字段/digest。

### 2.2 closed error语义

接口返回现有`core.DomainError`，实现只用`core.NewError/HasCategory/HasReason`：

| 语义 | core类别 | 精确边界 |
|---|---|---|
| Invalid | `core.ErrorInvalidArgument` | malformed Ref、非法expected Matrix或结构非法请求 |
| NotFound | `core.ErrorNotFound` | `CurrentID`从未存在；不得表示same-ID drift |
| Conflict | `core.ErrorConflict` | same-ID revision/digest/ref/watermark漂移、type-pun或current mismatch |
| Expired | `core.ErrorPreconditionFailed` | TTL crossing、expired或stale binding；复用`ReasonBindingExpired/ReasonBindingDrift/ReasonClockRegression`等既有reason |
| Unavailable | `core.ErrorUnavailable` | Reader/Fact Owner暂不可用且无权威结果 |
| Unknown | `core.ErrorIndeterminate` | current/outcome无法确定；不得降级为NotFound |

不得新增`ErrExpired`、`ErrUnknown`或平行错误枚举。

七个Binding语义冻结：

1. `ToolAdapterBinding`：持Governance Port的Tool Application Adapter；
2. `GatewayBinding`：Runtime Governance Gateway；
3. `ProviderTransportBinding`：Runner实际调用的受控transport adapter；
4. `PreparedReaderBinding`：Prepared current Reader；
5. `BoundaryReaderBinding`：Tool Boundary current Reader；
6. `ProviderInspectBinding`：unknown恢复的只读Inspect adapter；
7. `ProviderBinding`：Prepared/Request中的actual data-plane Provider identity与Capability Owner。

Harness Declaration必须分别声明`ProviderTransport`与closed role `provider`的nominal Provider endpoint/candidate。`ProviderTransportBinding`描述Tool侧受控transport adapter与Runner调用通道；`ProviderBinding`描述该transport背后的actual Provider权威主体、identity与Capability。post-binding Conformance分别解析二者；它们必须分别current并与Prepared/Request exact，不能因为Component ID相同而合并。两者仅公开typed binding/ref，不公开raw句柄；no-bypass全图必须同时扫描Transport与actual Provider的alias及真实注入。Provider role可公开声明，但kernel内部Runner不可进入Declaration或Projection。

Declaration作为required extension进入`ComponentManifestV2.Extensions`，Declaration digest经Manifest digest间接进入现有`AssemblyInputV1`摘要路径。Runtime不新增`AssemblyInputV1`字段、不改变digest算法，也不使用`RouteBindings[]ObjectRef`或自由ObjectRef替代typed refs。

## 3. Request与stable identity

```text
ControlledOperationPreparedSemanticSnapshotV2 {
  PreparedRef
  Delegation
  PersistedEnforcement     PersistedOperationEnforcementRefV3
  OperationDigest
  EffectID / IntentRevision / IntentDigest
  AttemptID / PermitID / PermitRevision / PermitDigest
  ProviderBinding
  PayloadSchema / PayloadDigest / PayloadRevision
  SemanticDigest
}

ControlledOperationProviderRequestV2 {
  ContractVersion
  RouteDeclarationRef
  RouteConformanceRef
  RouteCurrentRef
  ToolAdapterBinding
  Operation                OperationSubjectV3
  OperationDigest
  OperationScopeDigest
  EffectID / EffectRevision / EffectKind / IntentDigest
  Attempt                  OperationDispatchAttemptRefV3
  ProviderBinding
  Prepared                 PreparedProviderAttemptRefV2
  PreparedSemantics        ControlledOperationPreparedSemanticSnapshotV2
  ExecuteEnforcement       OperationDispatchEnforcementPhaseRefV4
  ExecuteEvidenceHandoff   OperationScopeEvidenceProviderHandoffRefV3
  Boundary                 OperationProviderBoundaryRefV1
  EvidencePolicy           OperationScopeEvidencePolicyRefV3
  ApplicabilityPolicy      OperationScopeEvidenceApplicabilityPolicyRefV3
  CallerDeadlineUnixNano
  RequestDigest
}
```

Request不含自由Entry ID，也不携fresh Checked projection。Runtime派生：

```text
EntryStableKeyV2 = canonical(
  OperationDigest,
  EffectID,
  stable OperationDispatchAttemptRefV3 coordinates,
  Prepared.ID,
  Prepared.Digest
)
EntryID = Derive(EntryStableKeyV2)
```

Boundary revision、current policy/binding revision、Checked time、expiry与Unified NotAfter不进入Entry ID；它们进入Entry content。相同stable key换这些水位必须Conflict或走同Entry current validation，不能换ID重执行。

## 4. Prepared current Projection

```text
ControlledOperationPreparedCurrentProjectionV2 {
  ContractVersion
  Delegation               ExecutionDelegationRefV2
  Prepared                 PreparedProviderAttemptRefV2
  PersistedEnforcement     PersistedOperationEnforcementRefV3
  Operation / OperationDigest
  EffectID / IntentRevision / IntentDigest
  Attempt                  OperationDispatchAttemptRefV3
  ProviderBinding
  PayloadSchema / PayloadDigest / PayloadRevision
  CheckedUnixNano
  ExpiresUnixNano
  ProjectionDigest
}

ControlledOperationPreparedCurrentReaderV2 {
  InspectCurrentControlledOperationPreparedV2(ctx, exact Prepared ref)
    -> exact current projection
}
```

Reader Adapter必须复用`PreparedExecutionGovernanceResultV2`的完整语义，不能丢`PersistedOperationEnforcementRefV3`。Gateway只把Projection的稳定字段与Request snapshot exact比较；`CheckedUnixNano`来自fresh Reader，不与Request逐字比较，不参与Entry ID。Projection完整值进入Entry Fact以供审计。

## 5. Policy与Handoff闭包

Gateway分别读取：

- `OperationScopeEvidencePolicyFactV3` current；
- `OperationScopeEvidenceApplicabilityPolicyFactV3` current。

Profile只从Applicability Policy读取。两Fact的Ref/revision/digest/state/TTL分别进入Entry。Gateway从execute Handoff读取exact Qualification，再读取Qualification的full Scope，要求：

- Handoff、Qualification、Scope、Operation、Attempt、phase全exact；
- Scope绑定同Evidence Policy与Applicability Policy；
- `EffectKind=praxis.tool/execute`；
- `PolicyProfile=praxis.tool/single-call-action-v1`；
- `ProviderBinding.Capability == EffectKind`；
- Runtime Effect/Intent current kind、Provider、revision、digest exact。

## 6. Entry Owner与opaque claim

```text
ControlledOperationProviderEntryFactV2 {
  ContractVersion
  EntryID / Revision / Digest
  StableKeyDigest
  State                    entered | unknown | observed | rejected_no_effect
  Request                  exact immutable stable request
  UnifiedNotAfterUnixNano
  FreshEffectCurrent
  FreshRouteCurrent
  FreshToolBindingCurrent
  FreshProviderBindingCurrent
  FreshPreparedProjection
  FreshEvidencePolicy
  FreshApplicabilityPolicy
  FreshBoundary
  FreshExecuteEnforcement
  FreshExecuteHandoff / Qualification / Scope
  AdmissionReceiptRef      optional
  ObservationRef           optional ProviderAttemptObservationRefV2
  EnteredUnixNano / UpdatedUnixNano
}

CreateEnteredResultV2 {        // kernel/control internal only
  Fact
  Disposition              created | existing
  opaqueClaim              // only created; not serializable/copyable/persistable
}
```

FactPort必须在同一锁内按derived Entry ID和stable key create-once。只有`created`拥有opaque claim；`existing`永远没有。claim不进入Fact、digest、Result、日志或sidecar。

## 7. internal Authorization与Runner

```text
ControlledOperationProviderRunnerV2 {   // kernel internal
  RunControlledOperationProviderV2(ctx, internal Authorization)
    -> ControlledProviderRawResultV2
}
```

Authorization由Gateway在同一call stack用`created + opaqueClaim + fresh Entry Fact`构造，包含Operation、EffectKind、ProviderBinding、Prepared、Attempt、Boundary、execute refs、两份Policy、Unified NotAfter与stable idempotency key。它不是公共port结果，也不可被Tool/Application持久化或重放。

raw Provider只能由Runner持有。Runner不得持Evidence Consumption、DomainResult、Settlement或Continuation写Port。raw result只能是admission receipt/unknown，不能直接成为Observation。

## 8. Provider不可逆admission

Runner把Authorization传到raw Provider方法；Provider在方法内以同stable key原子执行：

```text
fresh clock < UnifiedNotAfter
+ create-once irreversible execute admission
```

结果：

- 可证明未admit：返回可验证`rejected_no_effect` receipt；
- 已admit或可能admit：返回admitted receipt或unknown；
- 无法证明任何状态：unknown。

Runtime不承诺适配器方法零进入。跨Owner撤销仍可能发生在最后current read之后；V2只把读到的exact水位、Entry CAS和Provider admission统一审计化。

## 9. Inspect与Result

```text
ControlledProviderInspectPortV2 {       // read-only, injected into Gateway
  InspectOriginalControlledProviderAttemptV2(ctx, prepared, attempt)
    -> exact ProviderAttemptObservationV2 | NotFound | Unknown
}

ControlledOperationProviderResultV2 {
  ContractVersion
  EntryRef
  Status                   entered | unknown | observed | rejected_no_effect
  Error                    none | inspection_required |
                           provider_outcome_unknown |
                           inspection_unavailable
  Prepared                 exact original ref
  Attempt                  exact original ref
  AdmissionReceiptRef      optional
  ObservationRef           optional exact ProviderAttemptObservationRefV2
  InspectedUnixNano
  ResultDigest
}
```

Validate/canonical规则：

- `entered`：无Observation，`error=inspection_required`；
- `unknown`：无伪Observation，error为封闭unknown类；
- `observed`：必须有exact Observation且`error=none`；
- `rejected_no_effect`：必须有可验证AdmissionReceiptRef、无Observation、`error=none`，且同stable key永远不可重新execute；
- Result不携Authorization、opaque claim、Outcome、DomainResult、Evidence Consumption或Settlement；
- Inspect坐标只能从Entry Fact读取原Prepared/Attempt，公共caller不能替换；
- raw provider return不能直接填Observation。

## 10. Governance Port算法

```text
ControlledOperationProviderPortV2 {
  EnterControlledOperationProviderV2(ctx, request) -> Result
  InspectControlledOperationProviderV2(ctx, operation, derived entry key) -> Result
}
```

1. 验证Tool Adapter caller identity、Harness DeclarationRef、ConformanceRef、RouteCurrentRef与Request stable relations；
2. 派生Entry ID并Inspect：existing只Inspect，换stable content Conflict；
3. NotFound时fresh读取Effect/Intent、RouteCurrent Projection、Generation/Handoff、BindingSet、active-route scan、七个Binding、Prepared、两Policy、Enforcement、Handoff/Qualification/Scope、Boundary；active-route scan必须证明V2唯一active且V1未激活；
4. fresh clock不回拨；Gateway计算Unified NotAfter为Intent、两Policy、RouteCurrent、七个Binding、Prepared、Boundary、Enforcement、Handoff/Qualification与caller deadline的最小值；若未来另有Operation Policy，只能作为exact current门禁，未冻结TTL Owner前不得加入NotAfter；
5. Entry Owner create：
   - `existing`：返回Inspect结果，零Runner；
   - `created + opaqueClaim`：同call stack构造Authorization并调用Runner一次；
   - lost create reply：detached Inspect只恢复Entry，绝不恢复claim，零Runner；
6. raw reply不是Observation；可验证receipt证明未admit时CAS `rejected_no_effect`，不能证明则CAS unknown；Gateway按原Entry调用只读Inspect，证明exact Observation才CAS observed；
7. CAS lost reply只Inspect，绝不再次调用Runner。

## 11. 兼容与Assembly

- V1保持fixture-only；V1 route与V2 route互斥由Harness active-route scan提供current proof，Runtime每次Enter fresh复读；
- V1与V2各守自身既有身份空间，Assembly route互斥且不建立跨版本共同索引；
- V1 Boundary/成功/Observation不得伪装V2 Route、claim或Authorization；
- Harness canonical Declaration/Conformance事实和Route Current发布Owner保持唯一；Runtime `ports`只新增/拥有中立DeclarationRef、ConformanceRef、RouteCurrent Ref/Projection/Reader。Declaration digest通过Manifest digest间接受现有`AssemblyInputV1`摘要约束，Runtime不改字段或算法；
- 当前不提供production composition root。

## 12. 已验证实现边界

- Route中立公共面严格冻结为五个V2新增类型加既有MatrixKeyV3；AST/反射测试冻结type集合、唯一Reader签名、Projection全字段与JSON tag；
- Prepared链使用legacy V3 Permit水位，execute Attempt/Boundary/Enforcement使用V4 Permit Fact水位，二者各自exact且不得互换；
- Create、lost-create与CAS lost-reply恢复只比较immutable Entry identity/request；允许合法后继revision被Inspect，但不会恢复opaque claim或重入Runner；
- Entry Fact闭合Route七Binding的角色、BindingSet digest/semantic digest以及entered时刻的Checked/Issued/Expires/NotAfter历史真实性；恶意重Seal、role swap、future-issued或expired-at-entry均拒绝；
- 当前仅有reference store、fake transport与public Conformance，无production root/backend/SLA或物理exactly-once声明。
