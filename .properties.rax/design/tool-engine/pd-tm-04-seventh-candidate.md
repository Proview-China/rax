# PD-TM-04第七设计修正候选

状态：**Tool P4-0 C2、P4-1 SurfaceInvocationBinding与P4-2 InputContract/CandidateV3/BindingV2 Owner-local切片均为`implementation_software_test_yes`。** P4-2 targeted ordinary×100、race×20、full ordinary/race与vet已通过；该结论不包含修改Application、Harness、Runtime、Model、Context，也不代表Harness M2、system或production GO。

## 1. 已接受且不得回退的边界

### 1.1 PendingAction Revision

live Application与Harness没有PendingAction Revision。`PendingActionExactRefV2.Revision=1`只表示Tool-local wrapper revision；跨Owner只证明`PendingAction ID + RequestDigest`。禁止挪用Session、Identity、Model SourceCandidate或Runtime Revision。若未来要求跨Owner Revision，必须由Application/Harness另发additive合同。

### 1.2 Active Route

`ActiveRouteID/Revision/Digest`只由完整`ControlledOperationProviderRouteCurrentProjectionV2`及其exact Reader证明。Association/Generation只交叉其真实拥有的Generation和BindingSet五字段；不得补造Active Route第二来源。

## 2. P0-1：Surface来源、Registry对象来源与Route ExpectedOwner分离

live Registry只保存Capability/Tool/Package Record，没有Surface Record、ResolveSurface或Surface Store。候选必须使用两种不同nominal：

```go
const ToolRegistryObjectCurrentContractVersionV1 = "praxis.tool.registry-object-current/v1"
const MaxToolRegistryObjectCurrentTTLV1 = 15 * time.Second
const ToolRegistryCapabilityCurrentKindV1 runtimeports.NamespacedNameV2 = "praxis.tool/registry-capability-current"
const ToolRegistryDescriptorCurrentKindV1 runtimeports.NamespacedNameV2 = "praxis.tool/registry-descriptor-current"

type ToolRegistryObjectCurrentRefV1 struct {
    Kind     runtimeports.NamespacedNameV2 `json:"kind"`
    ID       string                        `json:"id"`
    Revision core.Revision                 `json:"revision"` // RegistryRevision
    Digest   core.Digest                   `json:"digest"`   // source record digest
}

type ToolRegistryRecordSourceV1 struct {
    Kind             string        `json:"kind"`
    ID               string        `json:"id"`
    ObjectRevision   core.Revision `json:"object_revision"`
    ObjectDigest     core.Digest   `json:"object_digest"`
    State            string        `json:"state"`
    RegistryRevision core.Revision `json:"registry_revision"`
    UpdatedUnixNano  int64         `json:"updated_unix_nano"`
    Digest           core.Digest   `json:"digest"`
}

type ToolRegistryObjectCurrentProjectionV1 struct {
    ContractVersion  string                               `json:"contract_version"`
    Ref              ToolRegistryObjectCurrentRefV1       `json:"ref"`
    Source           ToolRegistryRecordSourceV1           `json:"source"`
    Object           toolcontract.ObjectRef               `json:"object"`
    RegistryOwner    core.OwnerRef                        `json:"registry_owner"`
    CheckedUnixNano  int64                                `json:"checked_unix_nano"`
    ExpiresUnixNano  int64                                `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                          `json:"projection_digest"`
}

type ToolRegistryObjectCurrentReaderV1 interface {
    ResolveExactToolCapabilityCurrentV1(context.Context, toolcontract.ObjectRef) (toolcontract.CapabilityDescriptor, ToolRegistryObjectCurrentProjectionV1, error)
    InspectExactToolCapabilityCurrentV1(context.Context, toolcontract.ObjectRef, ToolRegistryObjectCurrentRefV1) (toolcontract.CapabilityDescriptor, ToolRegistryObjectCurrentProjectionV1, error)
    ResolveExactToolDescriptorCurrentV1(context.Context, toolcontract.ObjectRef) (toolcontract.ToolDescriptor, ToolRegistryObjectCurrentProjectionV1, error)
    InspectExactToolDescriptorCurrentV1(context.Context, toolcontract.ObjectRef, ToolRegistryObjectCurrentRefV1) (toolcontract.ToolDescriptor, ToolRegistryObjectCurrentProjectionV1, error)
}
```

Capability/Tool路径：adapter按exact object读取`registry.Record + 完整Descriptor`。live裸`Record.Kind=capability`只能映射`ToolRegistryCapabilityCurrentKindV1`，live裸`Record.Kind=tool`只能映射`ToolRegistryDescriptorCurrentKindV1`；禁止把裸`capability/tool`直接塞入`NamespacedNameV2`。Record必须active，Kind/ID/ObjectRevision/ObjectDigest逐字段回扣descriptor；Owner只从descriptor读取。`ToolRegistryRecordSourceV1` canonical固定domain=`praxis.tool`、version=`ToolRegistryObjectCurrentContractVersionV1`、discriminator=`ToolRegistryRecordSourceV1`，body覆盖全部source字段并排除自身Digest。Ref ID derive `{mapped Kind,Object,RegistryOwner}`，Revision=`RegistryRevision`，Digest=active source fact digest。

Registry Object Projection canonical固定domain=`praxis.tool`、version=`ToolRegistryObjectCurrentContractVersionV1`、discriminator=`ToolRegistryObjectCurrentProjectionV1`；body覆盖`ContractVersion,Ref,Source,Object,RegistryOwner,CheckedUnixNano,ExpiresUnixNano`并只排除`ProjectionDigest`。因此`Ref.Digest`始终是active source fact digest，`ProjectionDigest`是fresh Projection digest，二者不得相等断言或互换。TTL不超过独立Registry cap；Capability与Tool各自Owner允许不同。

Surface路径的Repository/current/canonical已由中央M2裁决单独冻结为[ToolSurfaceManifestCurrent V1](tool-surface-manifest-current-v1.md)：compiler产出完整Manifest后，Tool Owner唯一Repository执行`EnsureExactToolSurfaceManifestCurrentV1`，同一concrete实例同时维护history/current并向Harness只暴露`ToolSurfaceManifestCurrentReaderV1.InspectExactToolSurfaceManifestCurrentV1`。Projection闭集为完整Manifest、Owner、Checked/Expires与独立ProjectionDigest；Ref的ID/Revision/Digest必须无损等于Manifest ID/Revision/Digest，使Harness能用既有Plan坐标exact读取，禁止latest或猜ProjectionDigest。Ensure仅允许rev1从NotFound create；successor必须current+1并携ExpectedCurrent full Ref做CAS，history hit也必须current==winner才幂等返回。Harness M2只import Tool contract、构造器只接Reader，所有读取路径Ensure调用数必须为0。

Capability/Tool typed Reader必须在一次调用中返回完整Descriptor与对应fresh Projection。S1使用Resolve exact ObjectRef；S2/Boundary使用`ObjectRef + Expected Current Ref` InspectExact。每次都先`Descriptor.Validate()`，再逐字段验证Record Kind/ID/ObjectRevision/ObjectDigest/active、descriptor Owner、Capability/InputSchema/ArtifactDigest/EffectKinds等完整内容及Current Ref。禁止另调弱`Resolve(name/latest)`拼装Descriptor。

Route `ExpectedOwner`仍只从Route Provider派生，独立于Surface Owner、Capability Owner与Tool Owner。两类current都只读且不授执行权。

### 2.1 Surface Invocation三Owner桥

用户已接受三Owner方向。BindingRef、Ack canonical、唯一Repository、Tool-owned Ensure/Commit、单参InspectExact、TTL、Invocation epoch和测试合同以[Tool SurfaceInvocationBinding V1](surface-invocation-binding-v1.md)为唯一细化来源；本文件不保留Model Prepared影子DTO、Harness import、Tool Assembly echo或Kind翻译。Assembly直接嵌入Runtime ports已落盘的唯一public Go Ref/Projection/RegistrySnapshot Ref与Reader，不存在Tool fallback。

Tool侧返修结论：Prepared Historical Fact完整保存RequestTools/Plan/Route/Profile、Capability Ref、完整`runtimeports.RegistrySnapshotRefV1`、两个Actual digest与Historical NotAfter；Current只保存Current Ref/Historical Ref/Checked/Expires/ProjectionDigest并回扣Historical。Harness只交sealed composite current watermark，Tool不import Harness、不建Assembly echo、不转换Kind；Tool/Harness直接共享Runtime ports唯一Assembly Ref/Projection/RegistrySnapshot Ref。Tool Manifest expected digest用public canonical重算并只与Model ActualToolSurfaceDigest强等，Model ActualProviderInjectionDigest独立保存；Context Frame/field注入链不进入本Binding。唯一Repository在一个线性化事务维护两个唯一索引，public Writer只收exact EnsureRequest，Tool Owner clock经internal private Commit create-once；NotAfter只取Historical NotAfter、Prepared/Surface/composite current、必需RequestedNotAfter与caller deadline共同min，不冻结默认秒数或额外cap。同canonical幂等；同Invocation内RequestTools/Plan/Route/Profile、两个Actual digest或任一canonical坐标变化均Conflict且必须新Invocation epoch；lost reply只按Invocation Inspect恢复。

Prepared Historical/Current、Assembly与Binding中的Registry exact Ref统一为live `runtimeports.RegistrySnapshotRefV1`，禁止旧Model Registry nominal、alias/type-pun。Harness的Model Gate可调用SurfaceInvocationBinding Writer；但Harness M2对ToolSurfaceManifestCurrent只注入窄Reader且Ensure调用数必须为0。Application不改。Binding/Ack不授Provider执行权，每个attempt/Open/Stream/continuation actual-point必须复读Model Prepared Current、Tool Binding/ToolAck、Tool Surface Manifest Current、Harness Assembly composite并重新ValidateCurrent。P4-2 Owner-local Go已经闭合；Harness wiring、actual-point四读、system与production继续hard-block。

## 3. P0-2：Tool Input Contract Current

### 3.1 类型

```go
const ToolInputContractCurrentContractVersionV1 = "praxis.tool.input-contract-current/v1"
const ToolInputSchemaCurrentKindV1 = "praxis.tool/input-schema-current"
const MaxToolInputContractCurrentTTLV1 = 15 * time.Second // InputContract-only upper bound; not Surface Binding TTL or SLA

type ToolInputContractCurrentRefV1 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"` // fixed 1
    Digest   core.Digest   `json:"digest"`
}

type ToolInputSchemaCurrentRefV1 struct {
    Kind                 runtimeports.NamespacedNameV2 `json:"kind"`
    ID                   string                        `json:"id"`
    Revision             core.Revision                 `json:"revision"`
    Digest               core.Digest                   `json:"digest"`
    InputSchema          runtimeports.SchemaRefV2      `json:"input_schema"`
    Authority            ToolRegistryObjectCurrentRefV1 `json:"authority"`
    RegistryOwner        core.OwnerRef                 `json:"registry_owner"`
    CheckedUnixNano      int64                         `json:"checked_unix_nano"`
    ExpiresUnixNano      int64                         `json:"expires_unix_nano"`
}

type ToolInputContractBindingSubjectV1 struct {
    ApplicationRequestID       string                        `json:"application_request_id"`
    ApplicationRequestRevision core.Revision                 `json:"application_request_revision"`
    ApplicationRequestDigest   core.Digest                   `json:"application_request_digest"`
    PendingAction              toolcontract.PendingActionExactRefV2 `json:"pending_action"`
    OperationScopeDigest       core.Digest                   `json:"operation_scope_digest"`
    ProviderBinding     runtimeports.ProviderBindingRefV2 `json:"provider_binding"`
    ExpectedOwner       runtimeports.EffectOwnerRefV2     `json:"expected_owner"`
    SurfaceOwner            core.OwnerRef                 `json:"surface_owner"`
    CapabilityRegistryOwner core.OwnerRef                 `json:"capability_registry_owner"`
    ToolRegistryOwner       core.OwnerRef                 `json:"tool_registry_owner"`
    Surface             toolcontract.ObjectRef            `json:"surface"`
    SurfaceEntryOrdinal uint32                            `json:"surface_entry_ordinal"`
    SurfaceEntry        toolcontract.ToolSurfaceEntry     `json:"surface_entry"`
    Capability          toolcontract.ObjectRef            `json:"capability"`
    Tool                toolcontract.ObjectRef            `json:"tool"`
    ToolArtifactDigest  core.Digest                       `json:"tool_artifact_digest"`
    InputSchema         runtimeports.SchemaRefV2          `json:"input_schema"`
    LimitPolicy         runtimeports.OpaqueLimitPolicyRefV2 `json:"limit_policy"`
    Digest              core.Digest                       `json:"digest"`
}

type ToolInputContractLookupSubjectV1 struct {
    ApplicationRequestID       string                              `json:"application_request_id"`
    ApplicationRequestRevision core.Revision                       `json:"application_request_revision"`
    ApplicationRequestDigest   core.Digest                         `json:"application_request_digest"`
    PendingAction              toolcontract.PendingActionExactRefV2 `json:"pending_action"`
    OperationScopeDigest       core.Digest                         `json:"operation_scope_digest"`
    ProviderBinding            runtimeports.ProviderBindingRefV2   `json:"provider_binding"`
    ExpectedOwner              runtimeports.EffectOwnerRefV2       `json:"expected_owner"`
    Surface                    toolcontract.ObjectRef               `json:"surface"`
    CallName                   string                               `json:"call_name"`
    Capability                 toolcontract.ObjectRef               `json:"capability"`
    Tool                       toolcontract.ObjectRef               `json:"tool"`
    InputSchema                runtimeports.SchemaRefV2             `json:"input_schema"`
    Digest                     core.Digest                          `json:"digest"`
}

type ToolInputContractIssuanceSubjectV1 struct {
    ContractVersion          string                           `json:"contract_version"`
    LookupSubject            ToolInputContractLookupSubjectV1 `json:"lookup_subject"`
    RequestedExpiresUnixNano int64                            `json:"requested_expires_unix_nano"`
    Digest                   core.Digest                      `json:"digest"`
}

type ToolInputContractResolveRequestV1 struct {
    ApplicationRequestID       string                        `json:"application_request_id"`
    ApplicationRequestRevision core.Revision                 `json:"application_request_revision"`
    ApplicationRequestDigest   core.Digest                   `json:"application_request_digest"`
    PendingAction              toolcontract.PendingActionExactRefV2 `json:"pending_action"`
    OperationScopeDigest       core.Digest                   `json:"operation_scope_digest"`
    ProviderBinding          runtimeports.ProviderBindingRefV2 `json:"provider_binding"`
    ExpectedOwner            runtimeports.EffectOwnerRefV2     `json:"expected_owner"`
    Surface                  toolcontract.ObjectRef            `json:"surface"`
    CallName                 string                            `json:"call_name"`
    Capability               toolcontract.ObjectRef            `json:"capability"`
    Tool                     toolcontract.ObjectRef            `json:"tool"`
    InputSchema              runtimeports.SchemaRefV2          `json:"input_schema"`
    RequestedExpiresUnixNano int64                             `json:"requested_expires_unix_nano"`
}

type ToolInputContractInspectByIssuanceRequestV1 struct {
    ResolveRequest ToolInputContractResolveRequestV1 `json:"resolve_request"`
}

type ToolInputContractInspectExactRequestV1 struct {
    ResolveRequest ToolInputContractResolveRequestV1 `json:"resolve_request"`
    Expected       ToolInputContractCurrentRefV1      `json:"expected"`
}

type ToolInputContractCurrentProjectionV1 struct {
    ContractVersion          string                            `json:"contract_version"`
    Ref                      ToolInputContractCurrentRefV1     `json:"ref"`
    IssuanceSubject          ToolInputContractIssuanceSubjectV1 `json:"issuance_subject"`
    BindingSubject           ToolInputContractBindingSubjectV1 `json:"binding_subject"`
    SurfaceCurrent           ToolSurfaceManifestCurrentProjectionV1 `json:"surface_current"`
    CapabilityCurrent        ToolRegistryObjectCurrentProjectionV1 `json:"capability_current"`
    ToolCurrent              ToolRegistryObjectCurrentProjectionV1 `json:"tool_current"`
    InputSchemaCurrent       ToolInputSchemaCurrentRefV1        `json:"input_schema_current"`
    RequestedExpiresUnixNano int64                              `json:"requested_expires_unix_nano"`
    CheckedUnixNano          int64                              `json:"checked_unix_nano"`
    ExpiresUnixNano          int64                              `json:"expires_unix_nano"`
    ProjectionDigest         core.Digest                        `json:"projection_digest"`
}

type ToolInputContractCurrentReaderV1 interface {
    ResolveToolInputContractCurrentV1(context.Context, ToolInputContractResolveRequestV1) (ToolInputContractCurrentProjectionV1, error)
    InspectToolInputContractCurrentByIssuanceV1(context.Context, ToolInputContractInspectByIssuanceRequestV1) (ToolInputContractCurrentProjectionV1, error)
    InspectExactToolInputContractCurrentV1(context.Context, ToolInputContractInspectExactRequestV1) (ToolInputContractCurrentProjectionV1, error)
}

type ToolInputContractLeaseStoreV1 interface {
    CreateToolInputContractCurrentOnceV1(context.Context, ToolInputContractCurrentProjectionV1) (ToolInputContractCurrentProjectionV1, error)
    InspectToolInputContractCurrentByIssuanceIDV1(context.Context, string) (ToolInputContractCurrentProjectionV1, error)
    InspectExactToolInputContractCurrentV1(context.Context, ToolInputContractCurrentRefV1) (ToolInputContractCurrentProjectionV1, error)
}
```

本节全部nominal（Ref、LookupSubject、BindingSubject、Issuance、Projection与中立Resolve/Inspect request）落在`tool-mcp/contract/input_contract_current_v1.go`，只依赖Tool contract与Runtime public core/ports。`LookupSubject`只覆盖调用方在首次Owner读取前即可稳定重建的Application Request、PendingAction、Scope、Provider、Surface、CallName、Capability、Tool、Schema与requested坐标；它及requested共同派生issuance ID。Registry Owner、完整Surface Entry、Tool Artifact、LimitPolicy及各current closure不得进入issuance ID，只进入首次单赢家持久化的immutable `BindingSubject/Projection`。因此lost reply无需重跑fresh Owner读取即可Inspect同一ID，并发loser也不会因各自Checked/Expires不同制造Conflict。`applicationadapter`可以携带Application V2 DTO，但必须先调用Application公共验证器，再逐字段转换并exact对照本节中立request；禁止把Application协调器DTO下沉contract层。

Reader仍是distinct `Resolve/InspectByIssuance/InspectExact`，内部Store仍是mandatory `CreateOnce/InspectByIssuanceID/InspectExact`。两种Inspect都携原始Resolve Request；InspectExact另携Expected Ref。Resolve必须先InspectByIssuance，只有权威NotFound才create；lost create reply只InspectByIssuance。

六个方法的职责与错误语义固定：

- `Resolve...`先`request.Validate(freshNow)`并派生Subject/Issuance/ID，再调用Store `Inspect...ByIssuanceID`；只有Store对该ID返回权威NotFound才读取SurfaceInvocationBinding、Surface/Registry exact sources并Seal/Create。Unavailable、Indeterminate、timeout、decode error一律原样Fail Closed；
- `Inspect...ByIssuance`必须从完整中立Resolve request确定性重建同一Issuance ID后只读Store；不得create、续租或重读latest/name来源；lost create reply只走本方法；
- `InspectExact...`先验证Expected Ref，再以Expected Ref exact读Store，随后把持久Projection与原Resolve request、Expected Ref逐字段对照；不得重签Checked/Expires；
- Store `Create...Once`只接受已Seal且`Validate/ValidateAgainst`通过的immutable Projection。同issuance单赢家；loser验证稳定Subject/Issuance后返回winner，不比较loser fresh Checked/Expires或临时source projection；same ID稳定业务字段不同为Conflict；
- Store `Inspect...ByIssuanceID`只接受canonical stable ID并返回deep-copy winner；authoritative absent与Unavailable/Indeterminate必须不同错误分类；Store `InspectExact...`要求ID/Revision/Digest全部exact，same ID换digest为Conflict；
- Reader/Store constructor均拒绝nil/typed-nil；所有方法在任何Clock/Reader/Store前拒绝nil context；所有入参、持久值与返回值deep-copy。Invalid shape/canonical=`InvalidArgument`，exact drift/corruption=`Conflict`，过期/clock rollback=`PreconditionFailed`，依赖不可读=`Unavailable`，未知提交=`Indeterminate`；任何错误零Watermark/BindingV2/Gateway/Provider。

### 3.2 Surface Entry与LimitPolicy

live没有独立`ToolSurfaceEntry` canonical合同。候选不再声称复用不存在的Entry digest，也不新增第二个Entry truth：Binding Subject保存`Surface exact Ref + Entry.Order ordinal + 完整Entry deep-copy`。Reader必须从同一Surface Manifest按ordinal读取，逐字段等于保存的Entry，并验证`Entry.Order==ordinal`、CallName唯一命中、visible/allowed、EffectKinds包含`praxis.tool/execute`、Capability/Tool/InputSchema exact。

LimitPolicy固定为Tool内建`praxis.tool/input-payload-v1`。其Digest canonical固定domain=`praxis.tool`、version=`ToolInputContractCurrentContractVersionV1`、discriminator=`ToolInputLimitPolicySubjectV1`，body为`{Surface,SurfaceEntryOrdinal,SurfaceEntry,Capability,Tool,InputSchema,MaxInlineBytes=runtimeports.MaxOpaqueInlineBytes,InlineRequired=true,RefForbidden=true}`。V1不接受可配置政策。

### 3.3 Current与immutable lease

- `ToolInputSchemaCurrentKindV1`唯一值是`praxis.tool/input-schema-current`；全文禁止`praxis.tool/input-schema`作为该nominal Kind；
- Surface Current的Manifest ID/Revision/Digest/Owner分别exact等于Binding Subject Surface/SurfaceOwner；Capability/Tool Registry Object Current的Object与各自exact Ref逐字段相等，RegistryOwner分别exact等于`CapabilityRegistryOwner/ToolRegistryOwner`及各自Descriptor Owner。三Owner允许不同，禁止归一；Surface不含RegistryRevision；
- `SchemaRefV2`本身没有Owner；InputSchema Current的Authority只能是exact Tool Registry Object Current Ref，RegistryOwner必须exact等于ToolDescriptor.Owner。稳定ID canonical固定domain=`praxis.tool`、version=`ToolInputContractCurrentContractVersionV1`、discriminator=`ToolInputSchemaCurrentIdentityV1`、body=`{InputSchema,Authority}`；ID=`input-schema-current-v1-`加64位hex、无colon且不超过96字符；Revision=1，Digest=`InputSchema.ContentDigest`；不得任取Surface/Capability Owner充当Schema authority；
- Projection Checked取S1 Surface Repository与Registry读取完成后的fresh clock；Expires=`min(Surface Manifest真实expiry, Capability Registry Object Current expiry, Tool Registry Object Current expiry, Checked+15s, requested>0)`，必须严格晚于Checked；InputSchema Current使用同一Checked/Expires；
- Input Contract Projection是create-once immutable lease。`InspectExact`必须返回同一Ref、同一canonical projection、同一Checked/Expires；S2不得刷新、缩短、延长或重签该Projection；
- Surface在S2/Boundary只能按Surface exact Ref从Repository Inspect；Capability/Tool Registry Object Currents可以返回fresh Projection，但必须仍绑定同一Record/Object/各自Owner且active。fresh时间只用于校验与收紧deadline，不得改写已持久Input Contract或Candidate canonical；
- Boundary使用fresh now验证immutable Input Contract、一个Surface Current和两个Registry Object Currents。任一过期、Owner/type漂移、clock rollback都Fail Closed。

InputContract Binding Subject canonical必须覆盖Application Request ID/Revision/Digest、Tool-local PendingAction ID/Revision=1/RequestDigest与OperationScopeDigest，再覆盖Provider/Surface/Capability/Tool/Schema/Policy来源。不得加入fresh clock epoch。由此每个action拥有独立issuance；新action自然得到新ID，旧过期record不删除、不覆盖。

Issuance digest只派生稳定ID；InputContract是immutable lease，故computed Projection digest、`Ref.Digest`与`ProjectionDigest`三者exact相等。Projection canonical domain=`praxis.tool`、version=本合同version、discriminator=`ToolInputContractCurrentProjectionV1`，body排除`Ref.Digest/ProjectionDigest`。requested `<0` Invalid；generic `0`仅表示无caller附加上界，但ID仍由动作坐标隔离；PD-TM-04实际P4必须从Application Request/Input真实expiry传入`>0` requested并只能缩短。lease过期后Inspect返回PreconditionFailed，同一动作不得create新epoch或续命。

## 4. P0-3：Model historical SourceCandidate与ActionCandidateV3

### 4.1 immutable historical SourceCandidate

```go
type ModelSourceCandidateHistoricalRefV1 struct {
    ProjectionRef            modelinvoker.ToolCallCandidateObservationRefV1 `json:"projection_ref"`
    CallOrdinal              uint32                                          `json:"call_ordinal"` // fixed 0 for N=1
    CallID                   string                                          `json:"call_id"`
    CallName                 string                                          `json:"call_name"`
    CanonicalArgumentsDigest core.Digest                                     `json:"canonical_arguments_digest"`
    Digest                   core.Digest                                     `json:"digest"`
}
```

该Ref canonical固定domain=`praxis.tool`、version=`praxis.tool.model-source-candidate-historical/v1`、discriminator=`ModelSourceCandidateHistoricalRefV1`，body排除自身Digest。它必须由现有Model公开exact Reader返回的完整`ToolCallCandidateObservationProjectionV1`形成并逐字段验证`Ref`、`Calls==1`、Ordinal=0、CallID/Name/canonical arguments bytes+digest；完整Model Projection deep-copy进入BindingV2 closure。

该SourceCandidate是immutable historical lineage，不含Checked/Expires、Current Kind或EffectOwner，不参与TTL；Tool不为该historical lineage私建Model Port，也不把Model Observation升级成Tool事实或执行权。

### 4.2 additive ActionCandidateV3

`ActionCandidateV2`保持冻结兼容语义，但不能用于本P4。候选新增：

```go
const ActionContractVersionV3 = "praxis.tool-mcp.action/v3"

type ActionCandidateV3 struct {
    ContractVersion          string                            `json:"contract_version"`
    ID                       string                            `json:"id"`
    Revision                 core.Revision                     `json:"revision"` // fixed 1
    Digest                   core.Digest                       `json:"digest"`
    TenantID                 core.TenantID                     `json:"tenant_id"`
    RunID                    string                            `json:"run_id"`
    SessionID                string                            `json:"session_id"`
    TurnID                   string                            `json:"turn_id"`
    PendingAction            toolcontract.PendingActionExactRefV2 `json:"pending_action"`
    SourceCandidate          ModelSourceCandidateHistoricalRefV1 `json:"source_candidate"`
    Surface                  toolcontract.ObjectRef            `json:"surface"`
    Capability               toolcontract.ObjectRef            `json:"capability"`
    Tool                     toolcontract.ObjectRef            `json:"tool"`
    InputSchema              runtimeports.SchemaRefV2          `json:"input_schema"`
    Payload                  runtimeports.OpaquePayloadV2      `json:"payload"`
    PayloadRevision          core.Revision                     `json:"payload_revision"` // fixed 1
    LimitPolicy              runtimeports.OpaqueLimitPolicyRefV2 `json:"limit_policy"`
    InputContractCurrentRef  ToolInputContractCurrentRefV1     `json:"input_contract_current_ref"`
    SurfaceCurrent           ToolSurfaceManifestCurrentRefV1    `json:"surface_current"`
    CapabilityCurrent        ToolRegistryObjectCurrentRefV1    `json:"capability_current"`
    ToolCurrent              ToolRegistryObjectCurrentRefV1    `json:"tool_current"`
    InputSchemaCurrent       ToolInputSchemaCurrentRefV1       `json:"input_schema_current"`
    OperationScopeDigest     core.Digest                       `json:"operation_scope_digest"`
    EffectKind               runtimeports.EffectKindV2         `json:"effect_kind"`
    ExpectedOwner            runtimeports.EffectOwnerRefV2     `json:"expected_owner"`
    ConflictDomain           string                            `json:"conflict_domain"`
    IdempotencyKey           string                            `json:"idempotency_key"`
    CreatedUnixNano          int64                             `json:"created_unix_nano"`
    RequestedExpiresUnixNano int64                             `json:"requested_expires_unix_nano"`
}
```

CandidateV3 canonical固定domain=`praxis.tool-mcp.action`、version=`ActionContractVersionV3`、discriminator=`ActionCandidateV3`，body排除Digest。canonical必须覆盖Tool-local PendingAction Ref（Revision=1）、Model historical SourceCandidate Ref、Surface/Capability/Tool/InputSchema exact refs、Surface Current Ref、Capability/Tool Registry Object Current Refs、InputSchema Current、exact InputContract Ref、Payload、LimitPolicy、Scope/Effect/ExpectedOwner及时间字段。

CandidateV3 ID固定为`tool-action-candidate-v3-`加identity canonical digest去前缀后的64位hex；identity canonical固定domain=`praxis.tool-mcp.action`、version=`ActionContractVersionV3`、discriminator=`ActionCandidateV3Identity`、body=`{TenantID,RunID,SessionID,TurnID,PendingAction,SourceCandidate,InputContractCurrentRef}`。ID无colon且不超过96字符，Revision=1，PayloadRevision=1。CandidateV3只是BindingV2内的immutable candidate事实，不是独立current/root，也不得写入旧`action.StoreV2`。

硬门：Payload Inline非nil、Ref为空、Schema/ContentDigest/LimitPolicy逐字段等于Input Contract；顶层`LimitPolicy == Payload.LimitPolicy`；canonical bytes逐字等于Model historical Call和Application/PendingAction payload；Candidate的Surface Current及Capability/Tool Registry Object Currents与InputContract完整Projection中的Refs exact；InputSchemaCurrent exact；ExpectedOwner只等于Route Provider派生值。InputContract Ref必须进入Candidate digest，不能只放closure旁路证明。

## 5. P0-4：BindingCurrentProjectionV2与P4恢复根

冻结V1 canonical不修改、不包装升权；P4只使用additive V2：

```go
const SingleCallToolActionBindingCurrentContractVersionV2 = "praxis.tool.single-call-action-binding-current/v2"
const MaxSingleCallToolActionBindingCurrentTTLV2 = 15 * time.Second // independent cap

type SingleCallToolActionBindingCurrentRefV2 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"` // fixed 1
    Digest   core.Digest   `json:"digest"`
}

type SingleCallToolActionBindingSubjectV2 struct {
    ContractVersion            string                        `json:"contract_version"`
    ApplicationRequestID       string                        `json:"application_request_id"`
    ApplicationRequestRevision core.Revision                 `json:"application_request_revision"`
    ApplicationRequestDigest   core.Digest                   `json:"application_request_digest"`
    PendingAction              toolcontract.PendingActionExactRefV2 `json:"pending_action"`
    TenantID                   core.TenantID                 `json:"tenant_id"`
    RunID                      string                        `json:"run_id"`
    SessionID                  string                        `json:"session_id"`
    TurnID                     string                        `json:"turn_id"`
    ActionCoordinateDigest     core.Digest                   `json:"action_coordinate_digest"`
    ExecutionScope             core.ExecutionScope           `json:"execution_scope"`
    ExecutionScopeDigest       core.Digest                   `json:"execution_scope_digest"`
    SourceSubjectDigest        core.Digest                   `json:"source_subject_digest"`
    EffectKind                 runtimeports.EffectKindV2     `json:"effect_kind"` // exact runtimeports.OperationScopeEvidenceActionEffectKindV3()
    PolicyProfile              runtimeports.NamespacedNameV2 `json:"policy_profile"` // exact runtimeports.OperationScopeEvidenceActionPolicyProfileV3() == praxis.tool/single-call-action-v1
    CandidateContractVersion   string                        `json:"candidate_contract_version"` // fixed V3
    InputContractVersion       string                        `json:"input_contract_version"` // fixed V1
    Digest                     core.Digest                   `json:"digest"`
}

type SingleCallToolActionBindingIssuanceSubjectV2 struct {
    ContractVersion          string                               `json:"contract_version"`
    BindingSubject           SingleCallToolActionBindingSubjectV2 `json:"binding_subject"`
    RequestedExpiresUnixNano int64                                `json:"requested_expires_unix_nano"`
    Digest                   core.Digest                          `json:"digest"`
}

type SingleCallToolActionBindingResolveRequestV2 struct {
    ApplicationRequest       applicationcontract.SingleCallToolActionRequestV2 `json:"application_request"`
    SourceSubject            applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
    RequestedExpiresUnixNano int64                               `json:"requested_expires_unix_nano"`
}

type SingleCallToolActionBindingIssuanceLookupRequestV2 struct {
    ApplicationRequest       applicationcontract.SingleCallToolActionRequestV2 `json:"application_request"`
    SourceSubject            applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
    RequestedExpiresUnixNano int64                               `json:"requested_expires_unix_nano"`
}

type SingleCallToolActionBindingInspectExactRequestV2 struct {
    ApplicationRequest       applicationcontract.SingleCallToolActionRequestV2 `json:"application_request"`
    SourceSubject            applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
    RequestedExpiresUnixNano int64                               `json:"requested_expires_unix_nano"`
    Expected                 toolcontract.SingleCallToolActionBindingCurrentRefV2 `json:"expected"`
}

type SingleCallToolActionCandidateClosureV2 struct {
    ApplicationInput  applicationcontract.SingleCallToolActionInputCurrentProjectionV2 `json:"application_input"`
    ModelProjection   modelinvoker.ToolCallCandidateObservationProjectionV1             `json:"model_projection"`
    SurfaceInvocationBinding toolcontract.ToolSurfaceInvocationBindingV1                 `json:"surface_invocation_binding"`
    Association       runtimeports.GenerationBindingAssociationFactV1                   `json:"association"`
    Generation        runtimeports.GenerationCurrentProjectionV1                         `json:"generation"`
    Route             runtimeports.ControlledOperationProviderRouteCurrentProjectionV2   `json:"route"`
    ProviderCurrent   runtimeports.ProviderBindingCurrentProjectionV2                    `json:"provider_current"`
    SurfaceCurrent    toolcontract.ToolSurfaceManifestCurrentProjectionV1                `json:"surface_current"`
    CapabilityCurrent toolcontract.ToolRegistryObjectCurrentProjectionV1                 `json:"capability_current"`
    ToolCurrent       toolcontract.ToolRegistryObjectCurrentProjectionV1                 `json:"tool_current"`
    InputContract     toolcontract.ToolInputContractCurrentProjectionV1                  `json:"input_contract"`
    Candidate         toolcontract.ActionCandidateV3                                     `json:"candidate"`
    ClosureDigest     core.Digest                                                         `json:"closure_digest"`
}

type SingleCallToolActionBindingS2SnapshotV2 struct {
    ApplicationInput  applicationcontract.SingleCallToolActionInputCurrentProjectionV2 `json:"application_input"`
    SurfaceInvocationBinding toolcontract.ToolSurfaceInvocationBindingV1                `json:"surface_invocation_binding"`
    Association       runtimeports.GenerationBindingAssociationFactV1                   `json:"association"`
    Generation        runtimeports.GenerationCurrentProjectionV1                        `json:"generation"`
    Route             runtimeports.ControlledOperationProviderRouteCurrentProjectionV2  `json:"route"`
    ProviderCurrent   runtimeports.ProviderBindingCurrentProjectionV2                   `json:"provider_current"`
    SurfaceCurrent    toolcontract.ToolSurfaceManifestCurrentProjectionV1               `json:"surface_current"`
    CapabilityCurrent toolcontract.ToolRegistryObjectCurrentProjectionV1                `json:"capability_current"`
    ToolCurrent       toolcontract.ToolRegistryObjectCurrentProjectionV1                `json:"tool_current"`
    CheckedUnixNano   int64                                                              `json:"checked_unix_nano"`
    ExpiresUnixNano   int64                                                              `json:"expires_unix_nano"`
    Digest            core.Digest                                                        `json:"digest"`
}

type SingleCallToolActionBindingCurrentProjectionV2 struct {
    ContractVersion          string                                      `json:"contract_version"`
    Ref                      toolcontract.SingleCallToolActionBindingCurrentRefV2 `json:"ref"`
    IssuanceSubject          toolcontract.SingleCallToolActionBindingIssuanceSubjectV2 `json:"issuance_subject"`
    CandidateRef             toolcontract.ObjectRef                      `json:"candidate_ref"`
    InputContractCurrentRef  toolcontract.ToolInputContractCurrentRefV1  `json:"input_contract_current_ref"`
    CandidateClosure         SingleCallToolActionCandidateClosureV2     `json:"candidate_closure"`
    S2Snapshot              SingleCallToolActionBindingS2SnapshotV2     `json:"s2_snapshot"`
    RequestedExpiresUnixNano int64                                       `json:"requested_expires_unix_nano"`
    CheckedUnixNano          int64                                       `json:"checked_unix_nano"`
    ExpiresUnixNano          int64                                       `json:"expires_unix_nano"`
    ProjectionDigest         core.Digest                                 `json:"projection_digest"`
}

type SingleCallToolActionBindingCurrentReaderV2 interface {
    ResolveSingleCallToolActionBindingCurrentV2(context.Context, SingleCallToolActionBindingResolveRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
    InspectSingleCallToolActionBindingCurrentByIssuanceV2(context.Context, SingleCallToolActionBindingIssuanceLookupRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
    InspectExactSingleCallToolActionBindingCurrentV2(context.Context, SingleCallToolActionBindingInspectExactRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
}

type SingleCallToolActionBindingLeaseStoreV2 interface {
    CreateSingleCallToolActionBindingCurrentOnceV2(context.Context, SingleCallToolActionBindingCurrentProjectionV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
    InspectSingleCallToolActionBindingCurrentByIssuanceIDV2(context.Context, string) (SingleCallToolActionBindingCurrentProjectionV2, error)
    InspectExactSingleCallToolActionBindingCurrentV2(context.Context, toolcontract.SingleCallToolActionBindingCurrentRefV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
}
```

包边界固定：`SingleCallToolActionBindingCurrentRefV2/SubjectV2/IssuanceSubjectV2`是Tool contract层中立nominal，不导入Application；其中Subject只保存上列稳定primitives与Runtime public值。携Application DTO的`SingleCallToolActionBindingResolveRequestV2`、`SingleCallToolActionBindingIssuanceLookupRequestV2`、`SingleCallToolActionBindingInspectExactRequestV2`、Closure/S2 Snapshot/Projection Reader与Store实现位于`tool-mcp/applicationadapter`，允许依赖Application V2 public contract/ports并负责把DTO逐字段转换到中立Subject。Tool contract不得反向依赖Application、Harness或Application实现。

三个public request必须是distinct concrete nominal，禁止type alias或相互嵌套。Resolve、Lookup、InspectExact都只携`ApplicationRequest + SourceSubject + requested`稳定上游坐标；InspectExact再加Expected BindingV2 Ref。Caller不得传Candidate、InputContract、Closure或S2 truth；这些对象只能由Reader内部private build snapshot经权威S1/S2 Readers构造。Lookup必须能在不重跑S1/S2、不Resolve Candidate的情况下直接Seal中立Subject/Issuance并重建issuance ID。

V2 Subject canonical固定domain=`praxis.tool`、version=`SingleCallToolActionBindingCurrentContractVersionV2`、discriminator=`SingleCallToolActionBindingSubjectV2`，只覆盖预S1稳定中立因果坐标：Application Request ID/Revision/Digest、PendingAction Ref、Tenant/Run/Session/Turn、ActionCoordinateDigest、完整ExecutionScope+digest、SourceSubjectDigest及固定Effect/Profile/CandidateV3/InputContractV1版本坐标；排除自身Digest。adapter必须从已验证Application V2 DTO逐字段转换，不能把DTO本身写入Subject。CandidateRef、InputContract Ref、CandidateClosureDigest、任何Created/Checked/Expires和S2 Projection都不得进入Subject或ID派生。Issuance使用同domain/version、discriminator=`SingleCallToolActionBindingIssuanceSubjectV2`，只覆盖完整稳定Subject+requested并排除自身Digest。稳定ID=`single-call-tool-binding-v2-`加Issuance digest去前缀后的64位hex，无colon且不超过96字符，Ref Revision固定1；不得沿用V1 Subject/Issuance/ID前缀。

V2有distinct Resolve、InspectByIssuance、InspectExact request及mandatory create-once Store三口。Projection canonical固定domain=`praxis.tool`、version=`SingleCallToolActionBindingCurrentContractVersionV2`、discriminator=`SingleCallToolActionBindingCurrentProjectionV2`；body覆盖V2 Subject/Issuance、CandidateV3、InputContract Ref与完整Projection、S1 CandidateClosure、S2 Snapshot、Checked/Expires，排除Ref.Digest与ProjectionDigest。BindingV2也是immutable lease，故computed Projection digest、`Ref.Digest`与`ProjectionDigest`三者exact相等。CandidateClosure只证明S1构造且无独立Store；S2 Snapshot单独保存全部S2公共current与其真实上界，禁止混成一个Closure。

Association字段必须使用live `runtimeports.GenerationBindingAssociationFactV1`。读取后依次要求`Validate()==nil`、ID/Revision/Digest与Application exact Association Ref逐字段相等、State=`GenerationBindingAssociationActiveV1`、fresh now满足`UpdatedUnixNano <= now < ExpiresUnixNano`，并把Candidate内Generation/BindingSet五字段与Generation/Route逐字段交叉；Reader返回Fact，不得发明Current Projection包装。

BindingV2 `ExpiresUnixNano`必须为共同真实上界最小值：Application Request/Input、Association、Generation、Route、Provider、InputContract immutable expiry、Surface Repository返回Manifest真实expiry、Capability/Tool Registry Object Current、requested正数上界及`Checked+BindingV2 cap`。Surface Current Expires必须等于Manifest Expires；InputContract exact projection的Checked/Expires只能复制进入closure并参与min，不能在BindingV2签发或S2时刷新。

P4恢复规则：

1. BindingV2 durable root由Tool Owner拥有；对外唯一DTO是`SingleCallToolActionBindingCurrentRefV2`，权威Projection只存在BindingV2 Store；
2. 原子持久点固定在S1/InputContract、完整S2、CandidateV3、CandidateClosureV2与S2 Snapshot全部Seal成功之后、任何Watermark create/CAS之前。Store一次create-once提交完整BindingV2 Projection；未提交时Watermark/Reservation/Gateway均为零；
3. Resolve第一步把稳定上游坐标转换为Lookup并按V2 issuance ID Inspect；权威NotFound才在Reader内部构造S1/S2并CreateOnce；create回包丢失或进程重启第一步只按`SingleCallToolActionBindingIssuanceLookupRequestV2` InspectByIssuance，禁止先重跑S1/S2；
4. Create返回后，`BindingCurrentRefV2`是P4唯一恢复根；后续只允许`InspectExactV2(expected BindingV2 Ref)`；
5. InspectExact必须从BindingV2 Store返回同一immutable Projection，并一次拿到CandidateV3、InputContract Ref+完整Projection及完整closure；不依赖未定义CandidateClosureStore；
6. CandidateRef与CandidateV3 ID/Revision/Digest exact，InputContract Ref与closure内Projection Ref exact，ClosureDigest与ProjectionDigest均重算；
7. P5三Ref只能由BindingV2 Projection派生为handoff proof，引用`BindingV2 Ref + CandidateRef + InputContractRef`；它不是第二Truth Owner，不能替代BindingV2 Store，也不能把P4关键关联推迟到P5；
8. live旧Watermark不含BindingV2 Ref，不能冒充P4 durable root。未来P5 Watermark若携新版handoff proof，只是消费P4已经闭合的BindingV2 root。

同issuance并发只允许Store原子单赢家。loser不得拿自身fresh S1/S2 Candidate/InputContract/Closure与winner Projection比较并制造Conflict；它必须返回winner immutable Projection，先验证稳定Subject/Issuance exact，再以当前fresh readers验证winner。只有Application Request、Scope、SourceSubject、固定Effect/Profile/版本或requested等稳定业务输入改变时才形成不同issuance或Conflict。该规则保证lost reply可仅凭Lookup的稳定字段重建ID；`ValidateAgainst`不得要求caller重建fresh Candidate/InputContract/Closure。

### 5.1 Validators与各层调用顺序

正文冻结以下签名；实现不得以一个泛型`Validate(any)`替代：

```go
func (r ToolRegistryObjectCurrentRefV1) Validate() error
func (p ToolRegistryObjectCurrentProjectionV1) Validate() error
func (p ToolRegistryObjectCurrentProjectionV1) ValidateCurrent(now time.Time) error
func (p ToolRegistryObjectCurrentProjectionV1) ValidateAgainst(object toolcontract.ObjectRef, expected ToolRegistryObjectCurrentRefV1, now time.Time) error

func (r ToolSurfaceManifestCurrentRefV1) Validate() error
func (p ToolSurfaceManifestCurrentProjectionV1) Validate() error
func (p ToolSurfaceManifestCurrentProjectionV1) ValidateCurrent(expectedManifestRef ToolSurfaceManifestCurrentRefV1, now time.Time) error

func (r ToolInputContractCurrentRefV1) Validate() error
func (s ToolInputContractBindingSubjectV1) Validate() error
func (s ToolInputContractIssuanceSubjectV1) Validate() error
func (r ToolInputContractResolveRequestV1) Validate(now time.Time) error
func (p ToolInputContractCurrentProjectionV1) Validate() error
func (p ToolInputContractCurrentProjectionV1) ValidateCurrent(now time.Time) error
func (p ToolInputContractCurrentProjectionV1) ValidateAgainst(request ToolInputContractResolveRequestV1, now time.Time) error
func SealToolInputContractCurrentV1(ToolInputContractCurrentProjectionV1) (ToolInputContractCurrentProjectionV1, error)

func (c ActionCandidateV3) Validate() error
func (c ActionCandidateV3) ComputeDigest() (core.Digest, error)
func SealActionCandidateV3(ActionCandidateV3) (ActionCandidateV3, error)

func (r SingleCallToolActionBindingCurrentRefV2) Validate() error
func (s SingleCallToolActionBindingSubjectV2) Validate() error
func (s SingleCallToolActionBindingIssuanceSubjectV2) Validate() error
func (r SingleCallToolActionBindingResolveRequestV2) Validate(now time.Time) error
func (r SingleCallToolActionBindingIssuanceLookupRequestV2) Validate(now time.Time) error
func (r SingleCallToolActionBindingInspectExactRequestV2) Validate(now time.Time) error
func (c SingleCallToolActionCandidateClosureV2) Validate() error
func (s SingleCallToolActionBindingS2SnapshotV2) Validate() error
func (p SingleCallToolActionBindingCurrentProjectionV2) Validate() error
func (p SingleCallToolActionBindingCurrentProjectionV2) ValidateCurrent(now time.Time) error
func (p SingleCallToolActionBindingCurrentProjectionV2) ValidateAgainst(request SingleCallToolActionBindingResolveRequestV2, now time.Time) error
func SealSingleCallToolActionBindingCurrentV2(SingleCallToolActionBindingCurrentProjectionV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
```

职责与顺序固定：

1. `Validate`只做intrinsic shape、required/nil规范、nominal Kind、重复字段exact和canonical digest重算；不读取外部状态；
2. `ValidateCurrent`先调用`Validate`，再检查非零now、Checked/Expires、active及clock rollback；Surface还要求Manifest exact且`Expires==Manifest.Expires`；
3. `ValidateAgainst`先调用`ValidateCurrent`，再绑定request/expected Ref、Application/PendingAction/Scope、Provider/Owner、Surface/Descriptor完整内容和所有重复字段；BindingV2只从Resolve/Lookup/InspectExact的稳定坐标Seal中立Subject，不接受caller Candidate/InputContract/Closure/S2；
4. Seal顺序为输入DTO intrinsic Validate → canonical body/ID派生 → Ref/Projection digest → Projection Validate；CandidateV3还必须检查ID、Revision=1、PayloadRevision=1、Payload.LimitPolicy及InputContract Ref；
5. Store Create顺序为Projection Validate → ValidateAgainst(request,freshNow) → issuance ID/Ref重算 → 原子create-once → winner返回前再次Projection Validate；
6. InspectByIssuance顺序为LookupRequest Validate → 仅用稳定坐标Seal Subject/Issuance并重算ID → exact Store read → Projection Validate → stable Subject/Issuance重复字段exact → fresh验证winner；禁止重跑S1/S2后才能Inspect；
7. InspectExact顺序为InspectExactRequest Validate → Expected Ref Validate → exact Store read → Projection Validate → stored Ref与Expected逐字段exact → immutable Projection重复字段exact → 稳定request坐标ValidateAgainst → fresh验证winner；InputContract/BindingV2不得重签时间，也不要求caller重建fresh Candidate；
8. Surface EnsureExact先Manifest Validate/Seal、Repository create-once、winner exact验证；lost reply只Inspect exact Manifest Ref。Registry Object Reader每次先完整Descriptor Validate，再Record/source/Projection ValidateCurrent/ValidateAgainst；
9. 同issuance loser只要求winner稳定Subject/Issuance exact；不得比较loser临时S1/S2与winner Projection。随后以fresh readers验证winner current，失败返回PreconditionFailed而不是重建。

逐类型职责固定如下，任何重复字段都必须在所列层显式比较，不能只依赖外层digest：

| 类型/方法 | intrinsic与canonical | current | request/exact与重复字段硬门 |
|---|---|---|---|
| Registry Object Ref `Validate` | Kind只能是两个显式常量；ID、RegistryRevision、active source fact Digest有效 | 无 | Kind必须与live source `capability/tool`映射一致 |
| Registry Object Projection `Validate` | ContractVersion、Ref、Source canonical、Projection canonical全部重算 | 无 | `Ref.Revision==Source.RegistryRevision`、`Ref.Digest==Source.Digest`、Ref ID由mapped Kind+Object+RegistryOwner派生 |
| Registry Object `ValidateCurrent/ValidateAgainst` | 先Projection Validate | fresh now满足Checked/Expires和独立cap | expected Ref逐字段exact；Object ID/Revision/Digest与source及完整Descriptor exact；Descriptor Owner==RegistryOwner |
| Surface Ref/Projection | Manifest先Validate；Projection canonical重算 | fresh now且`Expires==Manifest.Expires` | expected Manifest Ref、Projection Ref与Manifest ID/Revision/Digest逐字段exact；Owner==Manifest.Owner |
| InputContract Ref/Lookup/Subject/Issuance | 各自Validate required/nil；Lookup、Subject、Issuance、Projection canonical逐层重算 | 无 | Lookup稳定请求坐标与Resolve exact；Projection中的Owner/Entry/Artifact/current closure由Tool Owner读取并封存；顶层requested==Issuance requested，Projection的stable字段必须回扣Lookup |
| InputContract Projection `ValidateCurrent/ValidateAgainst` | 先Projection Validate，且computed digest==Ref.Digest==ProjectionDigest | immutable Checked/Expires、独立cap、requested正数上界 | Resolve中立字段与Subject exact；一个Surface Current+两个Registry Object Currents+InputSchema Authority完整交叉；Inspect不得刷新时间 |
| CandidateV3 `Validate/ComputeDigest/Seal` | stable ID、Revision=1、PayloadRevision=1、canonical body/digest | Candidate本身不是current/root | PendingAction rev1、historical Model、InputContract Ref、四项source/schema ref、Payload/LimitPolicy/ExpectedOwner逐字段exact；Seal前必须已有权威closure |
| BindingV2 Ref/Subject/Issuance | 中立Subject/Issuance canonical与stable ID重算 | 无 | App Request/PendingAction/Tenant/Run/Session/Turn/Scope/SourceSubjectDigest及requested重复字段exact；不得含Application DTO或fresh facts |
| ClosureV2/S2SnapshotV2 `Validate` | 分别重算canonical digest并校验nil/deep-copy规范 | 各公共projection按其Owner验证 | S1 Candidate/InputContract/Source refs exact；S2只复读同refs，Association/Generation/Route/Provider/Surface/Registry对象逐字段exact |
| BindingV2 Projection `ValidateCurrent/ValidateAgainst` | 先Projection Validate，且computed digest==Ref.Digest==ProjectionDigest | immutable Checked/Expires、独立cap、完整S1/S2真实min | Resolve/Lookup/InspectExact稳定坐标Seal出的Subject/Issuance exact；CandidateRef、CandidateV3、InputContract Ref/Projection、Closure、S2Snapshot全部交叉；caller不得自报truth |

## 6. S1、S2、Boundary与Clock

1. Resolve先用稳定坐标执行BindingV2 Lookup；仅权威NotFound后进入S1：fresh clock → Application Input → Model exact Projection → 按Invocation coordinate Inspect exact SurfaceInvocationBinding → Association → Generation → Route → Provider Current → 由Binding指定的Surface Current + Capability/Tool Registry Object Currents → Input Contract Resolve/CreateOnce → CandidateV3 provisional Seal；
2. S2在BindingV2创建前完成：按S1保存的exact refs复读Application Input、SurfaceInvocationBinding、Association Fact、Generation、Route、Provider、Surface Current及Capability/Tool Registry Object Currents，InputContract InspectExact必须返回同一immutable Projection，再由内部builder重算CandidateV3；
3. 计算共同最小上界，必须包含S1和S2 Application Input、Association、Generation、Route、Provider、Surface Current、Capability/Tool Registry Object Currents、InputContract、Surface Manifest、requested与InputContract既有TTL上界；fresh nowS2/final now均须在窗内；
4. 只有完整S1+S2与共同min成功后，才Seal最终CandidateV3/CandidateClosureV2/S2Snapshot并原子CreateOnce immutable BindingV2；BindingV2 Checked/Expires自此绝不变化；
5. Boundary再以BindingV2 Ref InspectExact，并复读当次外部currents；effective deadline取BindingV2持久expiry和Boundary fresh上界/caller deadline最小值，不能绕过BindingV2已持久的S2最早上界；
6. zero clock、rollback、TTL crossing、NotFound/Unavailable/Indeterminate混淆、same-ID换Digest均Fail Closed；所有输入与返回deep-copy；constructor拒绝nil/typed-nil，nil context在任何Reader/Store/WithoutCancel前拒绝。

## 7. 反例与正例矩阵

### 7.1 原26项唯一编号映射

| 编号 | 硬门 |
|---|---|
| 1 | Application/Model exact Reader失败或漂移时零Tool写、零Effect |
| 2 | P4唯一合法组合是`ActionCandidateV3 + BindingCurrentProjectionV2`；非法组合包括CandidateV2+BindingV2、CandidateV3+BindingV1、任一V1 wrapper/alias/JSON type-pun |
| 3 | PendingAction跨Owner只证明ID+RequestDigest；Tool-local Revision必须为1 |
| 4 | Model Calls必须精确为1，Ordinal=0且CallID/Name/bytes/digest exact |
| 5 | Model bytes→Application V2 Input/PendingAction实际Payload字段→CandidateV3 Payload链必须闭合；禁止读取或伪造不存在的`PendingActionExactRefV2.PayloadDigest` |
| 6 | Surface/Capability/Tool/InputSchema/SourceCandidate任一exact字段漂移拒绝 |
| 7 | ActiveRoute只由Route current证明，不从Association/Generation补造 |
| 8 | Payload nil/ref/oversize/Length/Schema/LimitPolicy/ContentDigest错误拒绝 |
| 9 | Association必须按exact ID/ref读取并active/fresh |
| 10 | Generation/BindingSet五字段/Provider Current必须exact/current |
| 11 | requested负数拒绝、0不加上界、正数只缩短 |
| 12 | nowS1/nowS2/final clock必须fresh且不回拨 |
| 13 | S2必须Inspect S1保存的各Owner exact refs，并由内部builder重算同一CandidateV3 canonical；禁止按name/latest重新Resolve来源，也不存在Candidate current Reader |
| 14 | issuance必须含requested；并发CreateOnce单赢家且loser返回winner |
| 15 | Boundary只能按exact恢复根读取并压最早deadline，不授Authority |
| 16 | typed-nil、nil context、Unavailable/Indeterminate不得转NotFound |
| 17 | Harness私有类型、event JSON、compat tool calls不得进入Tool事实链 |
| 18 | Ref/Issuance/Projection重复字段漂移或正数requested被绕过拒绝 |
| 19 | V3 successor replacement：不沿用V2 `PendingActionCurrent`字段；CandidateV3 canonical直接绑定Tool-local PendingAction Ref(rev1)，BindingV2 closure另绑定Application current。理由是Harness/Application不提供EffectOwner nominal，继续复用会type-pun |
| 20 | canonical domain/version/discriminator/body或nil规范漂移拒绝 |
| 21 | S1/S2顺序错误、Route/Provider前提前Resolve Tool对象拒绝 |
| 22 | Route Provider/ExpectedOwner/Artifact绑定任一漂移拒绝 |
| 23 | 不得从Harness/Identity SettlementOwner猜Route ExpectedOwner |
| 24 | 合法source current刷新不得因时间不等而误判，但不可改immutable lease |
| 25 | S2/Boundary新窗口穿越拒绝，变晚不得延长持久上界 |
| 26 | BindingV2 CreateOnce必须晚于完整S1+S2；持久Expires包含S2 Application Input及所有S2 owner-current上界，不允许post-create旁路deadline |

上述`PD-TM04-N01`至`PD-TM04-N26`是唯一稳定测试编号；未来测试名、Conformance case与审计报告必须引用该前缀，不得另建一套“原26项”编号：

| 唯一编号 | 对应本表 | 验证阶段 |
|---|---:|---|
| `PD-TM04-N01`—`PD-TM04-N08` | 1—8 | Reader/Model/Payload白盒与黑盒 |
| `PD-TM04-N09`—`PD-TM04-N17` | 9—17 | Runtime current/issuance/Boundary故障注入 |
| `PD-TM04-N18`—`PD-TM04-N26` | 18—26 | canonical/Owner/Clock/恢复/Conformance/race |

### 7.2 跨Owner P0新增

| P0 | 正例 | 反例与期望 |
|---|---|---|
| Source/Route Owner | Surface Owner、Capability/Tool Registry Owner与Route ExpectedOwner各自exact，仍可形成CandidateV3 | 把Surface冒充Registry Record、强迫Owners相等或EffectOwner/core.OwnerRef type-pun：Conflict |
| historical Model | exact Model Projection形成historical Ref，无TTL仍可进入Candidate canonical | 伪造SourceCandidateCurrent/Owner/expiry或换Projection/Call：Conflict，零CandidateV3 |
| CandidateV3 | PendingAction rev1、一个Surface Current、两个Registry Object Currents、InputContract Ref、Payload/Policy/Schema全部进入digest | 只改InputContract Ref、任一current、historical Source或Payload任一字段：digest冲突，零BindingV2 |
| BindingV2 recovery | create回包丢失按issuance找到winner；随后单一BindingV2 Ref exact取回完整closure | 依赖CandidateClosureStore、仅持三Ref或用P5 proof恢复：Unavailable/Conflict，零后续写 |
| Entry trace | Surface Ref+ordinal+完整Entry逐字段回扣Manifest | 声称独立Entry canonical、换ordinal/Entry或Surface：Conflict |
| Current nominal | InputSchema Kind唯一、四项Object/Schema exact关系成立 | Kind冲突、跨Owner/type-pun、Object ID/Revision/Digest漂移：Conflict |
| immutable Inspect | S2 InspectExact返回InputContract与BindingV2原Checked/Expires | S2刷新/缩短/延长任一immutable projection：Conflict |
| Surface expiry | Surface Repository exact返回Manifest且使用其真实expiry | 把Surface冒充Registry current、忽略Manifest expiry或给Capability/Tool伪造descriptor expiry：PreconditionFailed |
| Surface Invocation P0-1 | Model Prepared Historical保存完整业务字段/两Snapshot refs/Historical NotAfter；Current只保存Ref关联与窗口 | 用retention替代current、Current虚构Historical字段、Current/Binding超过Historical NotAfter：零Ack/零Provider |
| Surface Invocation P0-2 | Harness sealed composite封存ToolSurface/Generation/Handoff/BindingSet及共同min；Tool只存exact composite | Tool自签三个current窗口、混入Context Frame/field注入链、换composite/ref/semantic/currentness：Conflict/PreconditionFailed，零Provider |
| Surface Invocation P0-3 | Assembly直接使用Runtime ports唯一Ref/Projection/RegistrySnapshot Ref，无Harness import、Tool echo或Kind转换；Tool Manifest expected只与Model ActualToolSurface强等 | 任一私有Assembly DTO/echo/type-pun、Provider Actual与Tool expected强等、Surface RegistrySnapshot不回扣Manifest：Conflict |
| Surface Invocation P0-4 | public Writer只收EnsureRequest；Tool Owner clock构造private Commit、Seal并create-once | Harness提交Fact/Created/NotAfter/Digest，或重投比较fresh clock：InvalidArgument/Conflict，零写 |
| Surface Invocation P1-1 | 每Invocation单Binding；同Invocation RequestTools/Plan/Route/Profile/两个Actual digest不变；每个provider边界重读current | 同Invocation变更未新建epoch，或仅凭Ack执行/跨TTL执行：Conformance失败，Provider能力不得启用 |

### 7.3 schema专审8P0/3P1/1P2逐项表

| 稳定编号 | 级别 | 可验证合同 | 期望 |
|---|---|---|---|
| `PD-TM04-S-P0-01` | P0 | issuance/canonical闭包：InputContract按Application/PendingAction/Scope隔离，BindingV2按预S1稳定坐标隔离；ID/Ref/Projection digest职责分离 | 共享动作issuance、时间epoch续命或canonical漂移时Conflict |
| `PD-TM04-S-P0-02` | P0 | create-once/lost-reply：Inspect-before-create、权威NotFound、单赢家、原Resolve request恢复 | Unavailable/Indeterminate后create或lost reply重建事实时失败 |
| `PD-TM04-S-P0-03` | P0 | Route Provider/ExpectedOwner：唯一取Route Provider，逐字段绑定Component/Manifest/Artifact/Capability；与Surface Owner及Capability/Tool Registry Owner分离 | 从Identity、任一source Owner或摘要猜Provider时零CandidateV3/BindingV2 |
| `PD-TM04-S-P0-04` | P0 | Surface完整来源：Surface exact Ref+entry ordinal+完整Entry回扣同一Manifest | 换Surface/ordinal/Entry或声称独立Entry truth时Conflict |
| `PD-TM04-S-P0-05` | P0 | LimitPolicy：只由固定typed subject和完整Surface来源派生 | 外部自报/可配置弱来源零InputContract |
| `PD-TM04-S-P0-06` | P0 | 四current nominal/exact：Surface Repository Current、Capability/Tool Registry Object Current、InputSchema Tool authority各自exact | Surface伪Registry、统一Owner、Kind/type-pun/object漂移拒绝 |
| `PD-TM04-S-P0-07` | P0 | exact authority/read seam：只用typed exact Registry/Application/Model/Runtime Reader，不拿写口 | latest/name/Candidate自报/实现包或Governance写口调用数为零 |
| `PD-TM04-S-P0-08` | P0 | S1/S2/Boundary持久闭包：完整S1+S2后原子持久BindingV2 root，P5仅派生handoff | post-create旁路deadline、依赖P5/三Ref恢复P4时零后续写 |
| `PD-TM04-S-P1-01` | P1 | validators与重复字段：Validate/ValidateCurrent/ValidateAgainst及Seal/Store/双Inspect硬门 | 任一层漏检由Conformance反例命中 |
| `PD-TM04-S-P1-02` | P1 | independent TTL/真实min：独立15秒cap、Surface Manifest真实expiry及全部S1/S2上界 | 伪造descriptor TTL、漏最小上界或刷新immutable时间时PreconditionFailed |
| `PD-TM04-S-P1-03` | P1 | distinct request nominal + nil/copy/error分类 | alias、typed-nil、nil context、浅拷贝或Indeterminate转NotFound均拒绝且零下游调用 |
| `PD-TM04-S-P2-01` | P2 | digest措辞与状态真值分三类：InputContract/BindingV2 immutable lease才满足`Ref.Digest==ProjectionDigest`；Registry Object Ref.Digest是active source fact digest且ProjectionDigest独立；Surface Ref.Digest是Manifest digest且ProjectionDigest独立；P4-0/P4-1/P4-2 Owner-local Go已通过software test | 任意跨类泛称/互换digest，或把Owner-local实现写成Harness M2/system/production YES即stale失败 |

## 8. 文件落点、包、Owner与门禁

- additive文件落点候选必须明确且不复用旧V1文件：

| 候选文件 | 包 | 责任与允许依赖 |
|---|---|---|
| `ExecutionRuntime/tool-mcp/contract/registry_object_current_v1.go` | `contract` | Capability/Tool Registry Record source/current nominal；只依赖runtime public core/ports |
| `ExecutionRuntime/tool-mcp/contract/surface_manifest_current_v1.go` | `contract` | Surface Manifest exact Ref/current nominal与只读Reader；Ref复用Manifest exact ID/Revision/Digest |
| `ExecutionRuntime/tool-mcp/surface/manifest_current_repository_v1.go` | `surface` | 唯一concrete Repository，嵌Reader并新增Ensure；Harness不import此包 |
| `ExecutionRuntime/tool-mcp/contract/surface_invocation_binding_v1.go` | `contract` | Invocation coordinate、SurfaceInvocationBinding Ref/Subject/Fact nominal；Prepared/Registry直接依赖Model public contract nominal，不复制第二套类型，不导入Application/Harness/Model实现 |
| `ExecutionRuntime/tool-mcp/contract/input_contract_current_v1.go` | `contract` | InputContract Ref/Subject/Projection nominal；供ActionCandidateV3同包引用，避免contract→applicationadapter循环 |
| `ExecutionRuntime/tool-mcp/contract/action_v3.go` | `contract` | historical Source Ref、ActionCandidateV3、Seal/Validate；只依赖Model public projection types与runtime public core/ports，不依赖Model实现 |
| `ExecutionRuntime/tool-mcp/contract/binding_current_v2.go` | `contract` | BindingV2 Ref、中立Subject、Issuance nominal；只保存稳定primitives/runtime public值，不导入Application |
| `ExecutionRuntime/tool-mcp/applicationadapter/tool_input_contract_current_v1.go` | `applicationadapter` | InputContract Reader实现与Registry read seam；依赖Tool contract/registry public read、Runtime public ports |
| `ExecutionRuntime/tool-mcp/applicationadapter/registry_object_current_v1.go` | `applicationadapter` | exact读取registry.Record+Capability/Tool Descriptor并签fresh Projection |
| `ExecutionRuntime/tool-mcp/applicationadapter/surface_manifest_repository_v1.go` | `applicationadapter` | compiler产物EnsureExact/InspectExact、create-once与lost reply恢复 |
| `ExecutionRuntime/tool-mcp/internal/owner/surfacebinding/repository_v1.go` | Tool internal Owner | 唯一Repository、private CommitRequest/Committer、Owner clock、Seal/create-once/Inspect；不得落入Application adapter语义层 |
| `ExecutionRuntime/tool-mcp/applicationadapter/surface_invocation_binding_adapter_v1.go` | `applicationadapter` | 仅把最终Model/Harness public nominal逐字段映射为Tool EnsureRequest并消费Writer/Reader；不构造Fact/Commit |
| `ExecutionRuntime/tool-mcp/applicationadapter/tool_input_contract_store_v1.go` | `applicationadapter` | immutable create-once/issuance/exact Store；Tool内部依赖 |
| `ExecutionRuntime/tool-mcp/applicationadapter/model_source_historical_v1.go` | `applicationadapter` | 消费Model最终公开的terminal exact Reader并形成historical Ref；Tool不私建、不实现Model Port |
| `ExecutionRuntime/tool-mcp/applicationadapter/candidate_builder_v3.go` | `applicationadapter` | Reader内部依据S1/S2 exact closure构造并Seal CandidateV3；无独立current Reader/Store |
| `ExecutionRuntime/tool-mcp/applicationadapter/binding_current_v2.go` | `applicationadapter` | distinct Resolve/IssuanceLookup/InspectExact request、CandidateClosureV2、S2SnapshotV2、Projection与Reader；引用`toolcontract` Ref/Subject/Issuance |
| `ExecutionRuntime/tool-mcp/applicationadapter/binding_lease_store_v2.go` | `applicationadapter` | BindingV2唯一create-once/issuance/exact Store；不另建CandidateClosureStore |

伪代码包解析规则：`ToolRegistryObjectCurrent*V1`、`ToolSurfaceManifestCurrent*V1`、`ToolSurfaceInvocationBinding*V1`、`ToolInputContractCurrent*V1`、`ModelSourceCandidateHistoricalRefV1`、`ActionCandidateV3`及BindingV2 `Ref/Subject/Issuance`位于`contract`包；在该包内部互引不写`toolcontract.`，在`applicationadapter`中必须写`toolcontract.`。BindingV2 `Resolve/IssuanceLookup/InspectExact request`、`CandidateClosureV2/S2SnapshotV2/Projection/Reader/Store`位于`applicationadapter`同包，互引不加前缀。禁止appadapter复制第二套BindingV2 Ref/Subject/Issuance nominal。Application/Model/Runtime类型只从各自公开包导入；不得为消除编译错误复制类型。

- 旧`candidate_current_resolver_v1.go`中的`SourceCandidateCurrent`与EffectOwner/TTL语义在P4退场；兼容V1文件不删、不改canonical，但P4不得调用；
- 只依赖Application V2公共contract/ports、Model最终公开的terminal exact Reader与Prepared Invocation public contract/只读Port、Runtime公共Association/Generation/Route/Provider readers和Tool Registry read seam；Surface Binding Prepared/Registry字段直接引用Model public nominal，不私建第二套类型，不依赖Model实现/internal；
- 禁止Tool导入Application coordinator/kernel/fakes、Harness实现/private类型、Runtime kernel/fakes/testsupport、Model internal；禁止获取Runtime Governance写口；
- `ActionCandidateV2`与`BindingCurrentProjectionV1`保持原canonical，仅作为兼容历史；P4必须使用V3+BindingV2，不能包装V1升权；
- Provider/ExpectedOwner仍由Route Owner链证明；Surface Current与Registry Object Current都不授Review、Authority、Fence、Permit、Runtime admission或Provider调用权；
- P4-2 Tool Owner代码与隔离测试已经完成并通过software test；跨Owner写入仍按阶段单独授权。system/production root/backend、Provider生产调用和Capability启用继续NO-GO。

## 9. 联合冻结与独立终审

三Owner Surface桥方向已经用户接受；Tool Owner字段、canonical、Repository、Reader/Writer/Ack、TTL与测试合同已按第一短审返修。仍待联合冻结/复核：

1. `ToolSurfaceManifestCurrent*V1`与`ToolRegistryObjectCurrent*V1`分离、各自Owner与Route ExpectedOwner分离，以及Surface Repository/Manifest真实expiry；
2. Surface Ref+entry ordinal+完整Entry作为唯一来源追溯，不新增独立Entry truth；
3. historical Model SourceCandidate、ActionCandidateV3 canonical及InputContract Ref入digest；
4. BindingCurrentProjectionV2作为P4唯一恢复根，P5仅派生handoff proof；
5. InputSchema Current唯一Kind与canonical identity、InputContract/BindingV2 immutable InspectExact；
6. 15秒InputContract cap仍是候选合同上限，不是生产SLA；
7. Model public Prepared historical Fact Ref、Current Projection/Reader与PreDispatchGate Go nominal可由Tool contract直接引用；Runtime ports唯一`ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1 + RegistrySnapshotRefV1` Delta落盘并独立审计YES，Tool/Harness直接共享，无映射层；
8. Harness/宿主sealed composite current Projection，以及所有provider attempt/Stream/Open/continuation在调用前统一Gate、重新InspectCurrent并取得Tool Ack，且Harness不创建Tool Fact。

本候选的P4-2 Tool Owner合同、Reader/Store与隔离测试已经落盘并通过software test。该结果不得描述为系统冻结、Harness M2或production YES；跨Owner门仍在相应阶段逐项复核。
