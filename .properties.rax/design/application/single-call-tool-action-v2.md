# Application G6A SingleCallToolAction V2 additive设计候选

## 1. 状态与范围

状态：**owner-local design终审 YES；Application owner-local P2代码第四独立终审 YES（P0/P1/P2=0）**。第三审CAS schema漂移及Base schema、Harness OwnerInputs exact版本、ContextParent kind缺口均已按冻结合同闭合并通过新一轮机械门。Harness P3已实现并独立审计YES；该YES不覆盖ToolResult Owner真实性、可信生产时钟、Tool P4、P5跨模块fixture、system G6A或production composition root，后述切面继续`BLOCKED/NO-GO`。

系统G6A、production composition root、Capability启用、Context Continuation、Turn推进、Checkpoint与`N>1`继续`BLOCKED`。Harness Controlled Operation Provider Route V2已通过第八轮独立审计`YES(P0/P1/P2=0)`；旧V1仅完成Application owner-local协调与测试，不能计作系统G6A。

本设计是additive V2，不修改V1字段、canonical domain、digest或运行语义。它只接受以下live Harness版本闭包：

```text
PendingActionApplicationBindingV2
-> GovernedSessionV4 / SessionCASRequestV4
-> CommittedPendingActionSubjectV3
-> CommittedPendingActionCurrentRequestV3
-> CommittedPendingActionCurrentV3
-> CommittedPendingActionReaderV3
```

`CommittedPendingActionReaderV2`不得承载BindingV2、SessionV4、Subject/CurrentV3，也不得通过alias或私有翻译伪装升级。

## 2. Owner与不可越权边界

| 对象 | 唯一Owner | Application允许做什么 |
|---|---|---|
| Model Projection | Model Invoker | 携带完整neutral坐标，要求公开Reader复读 |
| Identity、SettledTurn DomainResult | Model-turn Settlement Owner | 携带exact ref/coordinate，不创建Fact |
| BindingV2、SessionV4、CurrentV3 | Harness | Harness Adapter读取并映射为Application neutral DTO |
| Route Current与Generation/Binding association | Harness Assembly / Runtime public facts | 仅通过BindingV2 OwnerInputs及CurrentV3证明 |
| Context applicability | Context Owner / Runtime public ref | 仅通过BindingV2 OwnerInputs及CurrentV3证明 |
| Dispatch Authority | Runtime | ActionCoordinate形成后签发；Application只读复读 |
| Request/Result/Coordination V2 | Application | canonical、write-ahead、CAS、恢复、S1/S2编排 |
| Tool执行/Watermark/Result | Tool Owner | 通过Application ToolAction Port V2提供start-or-inspect |

Application不import Harness、Model、Tool、Context事实或实现包，不创建Owner Fact，不持有Provider、Tool Boundary proof、Runtime Fact commit口或production root。

## 3. 版本闭包与neutral Binding coordinate

Application不复制Harness struct。以下类型是Application-owned nominal coordinate，仅无损镜像Harness公开事实；字段全部使用`snake_case` JSON tag且进入所属对象digest。

### 3.1 Identity coordinate

```go
type SingleCallModelPendingActionIdentityCoordinateV2 struct {
    IdentityContractVersion     string                             `json:"identity_contract_version"`
    IdentityID                  string                             `json:"identity_id"`
    IdentityRevision            core.Revision                      `json:"identity_revision"`
    IdentityDigest              core.Digest                        `json:"identity_digest"`
    CreatedUnixNano             int64                              `json:"created_unix_nano"`
    ModelProjectionID           string                             `json:"model_projection_id"`
    ModelProjectionRevision     core.Revision                      `json:"model_projection_revision"`
    ModelProjectionDigest       core.Digest                        `json:"model_projection_digest"`
    ModelInvocationID           string                             `json:"model_invocation_id"`
    ModelInvocationDigest       core.Digest                        `json:"model_invocation_digest"`
    ModelObservationDigest      core.Digest                        `json:"model_observation_digest"`
    ModelSourceResponseID       string                             `json:"model_source_response_id,omitempty"`
    ModelSourceSequence         uint64                             `json:"model_source_sequence"`
    SourceKeyDigest             core.Digest                        `json:"source_key_digest"`
    SourceExecutionScopeDigest  core.Digest                        `json:"source_execution_scope_digest"`
    SourceRunID                 string                             `json:"source_run_id"`
    SourceSessionID             string                             `json:"source_session_id"`
    SourceTurn                  uint32                             `json:"source_turn"`
    CallOrdinalEncodingVersion  string                             `json:"call_ordinal_encoding_version"`
    CallOrdinalPresent          bool                               `json:"call_ordinal_present"`
    CallOrdinalValue            uint32                             `json:"call_ordinal_value"`
    SettlementOwner             runtimeports.ProviderBindingRefV2 `json:"settlement_owner"`
    CallID                      string                             `json:"call_id"`
    CallName                    string                             `json:"call_name"`
    CanonicalArgumentsDigest    core.Digest                        `json:"canonical_arguments_digest"`
    PendingActionRef            string                             `json:"pending_action_ref"`
    PendingActionRequestDigest  core.Digest                        `json:"pending_action_request_digest"`
    PayloadSchema               runtimeports.SchemaRefV2           `json:"payload_schema"`
    PayloadContentDigest        core.Digest                        `json:"payload_content_digest"`
    Capability                  runtimeports.CapabilityNameV2      `json:"capability"`
    SourceCandidateID           string                             `json:"source_candidate_id"`
    SourceCandidateRevision     core.Revision                      `json:"source_candidate_revision"`
    SourceCandidateDigest       core.Digest                        `json:"source_candidate_digest"`
    DomainResultDigest          core.Digest                        `json:"domain_result_digest"`
    NotAfterUnixNano            int64                              `json:"not_after_unix_nano"`
    Digest                      core.Digest                        `json:"digest"`
}
```

`IdentityContractVersion`固定`praxis.harness.model-tool-call-pending-action-identity/v1`；`CreatedUnixNano`必须非零并进入Identity canonical。ordinal固定presence/versioned decode；`CanonicalArgumentsDigest`只接受Model canonical argument bytes经live `core.DigestBytes`所得值。Identity SourceKey的scope/run/session/turn、完整Projection Ref（含Invocation/Observation/source sequence）、ordinal、Candidate、SettlementOwner必须逐字段映射Fact并与当前Subject exact，禁止splice。

BindingV2只公开IdentityRef，不能直接产生上述完整Identity。Application另定义exact neutral ref：

```go
type SingleCallModelPendingActionIdentityRefCoordinateV2 struct {
    ID                         string        `json:"id"`
    Revision                   core.Revision `json:"revision"`
    Digest                     core.Digest   `json:"digest"`
    ModelProjectionID          string        `json:"model_projection_id"`
    ModelProjectionRevision    core.Revision `json:"model_projection_revision"`
    ModelProjectionDigest      core.Digest   `json:"model_projection_digest"`
    PendingActionRef           string        `json:"pending_action_ref"`
    PendingActionRequestDigest core.Digest   `json:"pending_action_request_digest"`
    DomainResultDigest         core.Digest   `json:"domain_result_digest"`
    SourceKeyDigest            core.Digest   `json:"source_key_digest"`
}

type SingleCallSettledTurnDomainResultFactRefCoordinateV2 struct {
    FactID          string                                                `json:"fact_id"`
    Revision        core.Revision                                         `json:"revision"`
    FactDigest      core.Digest                                           `json:"fact_digest"`
    SourceKeyDigest core.Digest                                           `json:"source_key_digest"`
    Schema          runtimeports.SchemaRefV2                              `json:"schema"`
    ContentDigest   core.Digest                                           `json:"content_digest"`
    IdentityRef     SingleCallModelPendingActionIdentityRefCoordinateV2   `json:"identity_ref"`
}
```

这两个nominal coordinate逐字段镜像公开Harness refs，不增加Application自签事实；same ID任一字段漂移均Conflict。

### 3.2 Binding coordinate

```go
type SingleCallHarnessBaseBindingCoordinateV2 struct {
    PendingAction       SingleCallPendingActionCoordinateV1                    `json:"pending_action"`
    IdentityRef         SingleCallModelPendingActionIdentityRefCoordinateV2     `json:"identity_ref"`
    DomainResultFact    SingleCallSettledTurnDomainResultFactRefCoordinateV2   `json:"domain_result_fact"`
    ModelTurnSettlement runtimeports.OperationSettlementRefV3                  `json:"model_turn_settlement"`
    Digest              core.Digest                                            `json:"digest"`
}

type SingleCallHarnessOwnerCurrentInputsCoordinateV2 struct {
    HarnessContractVersion       string                                                      `json:"harness_contract_version"`
    ModelTurnOperation           runtimeports.OperationSubjectV3                             `json:"model_turn_operation"`
    GenerationBindingAssociation runtimeports.GenerationBindingAssociationRefV1              `json:"generation_binding_association"`
    RouteCurrent                 runtimeports.ControlledOperationProviderRouteCurrentRefV2   `json:"route_current"`
    RouteMatrix                  runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3 `json:"route_matrix"`
    ContextApplicability         runtimeports.OperationScopeEvidenceApplicabilityFactRefV3   `json:"context_applicability"`
    HarnessDigest                core.Digest                                                 `json:"harness_digest"`
    Digest                       core.Digest                                                 `json:"digest"`
}

type SingleCallHarnessApplicationBindingCoordinateV2 struct {
    BindingVersion       string                                                `json:"binding_version"`
    Base                 SingleCallHarnessBaseBindingCoordinateV2              `json:"base"`
    OwnerInputs          SingleCallHarnessOwnerCurrentInputsCoordinateV2       `json:"owner_inputs"`
    HarnessBindingDigest core.Digest                                           `json:"harness_binding_digest"`
    Digest               core.Digest                                           `json:"digest"`
}
```

Base精确对应Harness BindingV2的四个权威事实坐标：PendingAction、IdentityRef、SettledTurn DomainResult FactRef、Model-turn Runtime Settlement；OwnerInputs精确对应五项public输入：Model operation、GenerationBinding association、Route Current、完整Route Matrix、Context applicability。`HarnessContractVersion/HarnessDigest`逐字段保留live OwnerInputs版本与digest，末尾`Digest`才是Application nominal coordinate摘要。Application不接收Harness payload bytes，也不构造`PendingActionApplicationBindingV2`。

`PendingAction.PayloadSchema`描述工具调用输入，必须与完整Identity中的`PayloadSchema` exact；`DomainResultFact.Schema`描述结算后的领域结果，必须与`ModelTurnSettlement.DomainResultSchema` exact。两者属于不同语义域，默认可以且通常应当不同；Application不得建立`PendingAction.PayloadSchema == DomainResultFact.Schema`的跨域等式。输入链或结果链各自漂移都必须拒绝，但不能用输入schema替代结果schema，反之亦然。

Harness Adapter必须先exact读取`GovernedSessionV4`取得完整BindingV2，校验Session/Binding version和digest，再逐字段映射上述neutral坐标，最后调用`CommittedPendingActionReaderV3`。任何字段不能由V1/V3摘要推导或调用方补签。

`BindingVersion`固定为`praxis.harness.pending-action-application-binding/v2`，`HarnessBindingDigest`必须逐字等于live BindingV2 digest；OwnerInputs的ContractVersion与digest也必须由Harness Fact验证后再映射，Application不得重算出不同的“等价”版本。

### 3.3 PendingAction subject

```go
type SingleCallRunSubjectCoordinateV2 struct {
    ExecutionScope       core.ExecutionScope `json:"execution_scope"`
    RunID                core.AgentRunID     `json:"run_id"`
    ExecutionScopeDigest core.Digest         `json:"execution_scope_digest"`
    Digest               core.Digest         `json:"digest"`
}

type SingleCallPendingActionSubjectCoordinateV2 struct {
    Run                    SingleCallRunSubjectCoordinateV2                `json:"run"`
    SessionID              string                                          `json:"session_id"`
    SessionRevision        core.Revision                                   `json:"session_revision"`
    SessionDigest          core.Digest                                     `json:"session_digest"`
    Turn                   uint32                                          `json:"turn"`
    PendingActionRef       string                                          `json:"pending_action_ref"`
    PendingActionDigest    core.Digest                                     `json:"pending_action_digest"`
    Binding                SingleCallHarnessApplicationBindingCoordinateV2 `json:"binding"`
    Identity               SingleCallModelPendingActionIdentityCoordinateV2 `json:"identity"`
    Digest                 core.Digest                                     `json:"digest"`
}
```

Run携完整ExecutionScope+RunID；scope digest必须用`runtimeports.ExecutionScopeDigestV2`重算。Subject与Harness SubjectV3、SessionV4、BindingV2四事实和OwnerInputs必须exact。完整Identity只能来自§5的Fact Owner exact Reader，且必须与Binding中的IdentityRef逐字段交叉。Runtime `OperationSettlementRefV3`只证明DomainResult schema+digest；DomainResult FactRef只来自Harness BindingV2，必须经Repository Reader交叉验证，不能声称Settlement含FactRef。

## 4. ActionCoordinate、Authority与Request V2

### 4.1 pre-authority ActionCoordinate

Authority由Runtime在Action Operation形成后、Tool dispatch前签发。先冻结不含Authority、TTL、Request ID的坐标：

```go
type SingleCallActionCoordinateV2 struct {
    ContractVersion      string                                     `json:"contract_version"`
    ExecutionScope       core.ExecutionScope                         `json:"execution_scope"`
    ExecutionScopeDigest core.Digest                                 `json:"execution_scope_digest"`
    PendingSubject       SingleCallPendingActionSubjectCoordinateV2 `json:"pending_subject"`
    Digest               core.Digest                                `json:"digest"`
}
```

`ContractVersion=praxis.application.single-call-action-coordinate/v2`。Runtime `DispatchAuthorityFactV2.ActionScopeDigest`必须exact等于`ActionCoordinate.Digest`，Scope必须exact相等；Authority绝不写回Harness ApplicationBinding。

Action Validate必须执行交叉闭包：顶层ExecutionScope及digest与`PendingSubject.Run`、OwnerInputs `ModelTurnOperation` exact；PendingAction、Identity、DomainResult、Settlement和OwnerInputs全部由Subject/Binding exact承载。任一重复坐标不一致都按Conflict处理，不能选一份继续。

旧V1字段在Action V2中的裁决：

| V1坐标 | 可证明来源 | V2裁决 |
|---|---|---|
| Workflow | G6A Tool dispatch不需要；本切面没有独立Workflow current Reader/TTL | 删除 |
| Observation | Identity Fact内完整Model Projection，且由Fact Reader exact验证 | 删除重复字段；只使用Identity中的Projection坐标 |
| Assembly | OwnerInputs `GenerationBindingAssociation`及CurrentV3内部Owner复读 | 删除重复字段 |
| RouteCurrent | OwnerInputs `RouteCurrent + RouteMatrix`及CurrentV3 | 删除重复字段 |
| ParentFrame | G6A不消费Frame；CurrentV3只验证Context applicability | 删除 |
| ParentFrameApplicabilitySource | OwnerInputs只有Runtime Context applicability ref，无法无损证明旧V1 source coordinate | 删除 |

strict decode必须拒绝把这些V1字段塞入Action V2；不得保留两份坐标后任选其一。

### 4.2 Request与Assembler输入

```go
type SingleCallToolActionRequestV2 struct {
    ContractVersion string                              `json:"contract_version"`
    ID              string                              `json:"id"`
    Revision        core.Revision                       `json:"revision"`
    Action          SingleCallActionCoordinateV2        `json:"action"`
    Authority       runtimeports.AuthorityBindingRefV2 `json:"authority"`
    CreatedUnixNano int64                               `json:"created_unix_nano"`
    ExpiresUnixNano int64                               `json:"expires_unix_nano"`
    Digest          core.Digest                         `json:"digest"`
}

type AssembleSingleCallToolActionRequestV2 struct {
    Action                    SingleCallActionCoordinateV2        `json:"action"`
    Authority                 runtimeports.AuthorityBindingRefV2 `json:"authority"`
    RequestedNotAfterUnixNano int64                               `json:"requested_not_after_unix_nano"`
}

type SingleCallToolActionRequestSubjectV2 struct {
    ActionDigest core.Digest                        `json:"action_digest"`
    Authority    runtimeports.AuthorityBindingRefV2 `json:"authority"`
}
```

Request稳定ID只由上述Subject在domain `praxis.application.single-call-tool-action-request-id-v2`、discriminator `SingleCallToolActionRequestSubjectV2`下派生，prefix固定`single-call-request:v2:`；Revision固定1。Request完整Digest另覆盖ContractVersion、ID、Revision、Action、Authority、Created与Expires。同subject重封但时间/TTL或任一内容不同会得到same ID/different digest并Conflict，不能静默替换。

Assembler输入必须携`AuthorityBindingRefV2`；Harness Adapter使用Runtime `AuthorityFactReaderV2`在S1/S2按exact ref读取，调用`ValidateCurrent(ref, Action.ExecutionScope, Action.Digest, freshNow)`。`RequestedNotAfterUnixNano < 0`为Invalid，`=0`不附加调用方上界，`>0`只能缩短。S2全部复读完成后取fresh `nowS2`作为Request `CreatedUnixNano`，要求clock不回拨；`ExpiresUnixNano = min(CurrentV3.ExpiresUnixNano, Authority.ExpiresUnixNano, RequestedNotAfter>0)`且`Created < Expires`。Application只消费CurrentV3已聚合的Expires，不重算Candidate/Context/Route/Generation等内部TTL；**不包含Policy**。未来若Policy成为输入，必须另立versioned ref/proof/input Delta。

## 5. 最小Owner-current proof

旧六proof废止，不允许Application压缩或伪造各Owner摘要。V2只使用一个Harness聚合当前证明和一个Runtime Authority证明：

### 5.1 Identity Fact Owner exact Reader

Application发布neutral只读合同；Harness Application Adapter实现它，并额外注入公开`SettledTurnDomainResultReaderV3`。Adapter把neutral FactRef逐字段映射为Harness公开FactRef后调用`InspectExact`，不能使用Repository的`EnsureExact`写口。

```go
type SingleCallModelPendingActionIdentityCurrentRequestV2 struct {
    ContractVersion              string                                                `json:"contract_version"`
    Run                          SingleCallRunSubjectCoordinateV2                      `json:"run"`
    SessionID                    string                                                `json:"session_id"`
    Turn                         uint32                                                `json:"turn"`
    IdentityRef                  SingleCallModelPendingActionIdentityRefCoordinateV2   `json:"identity_ref"`
    DomainResultFact             SingleCallSettledTurnDomainResultFactRefCoordinateV2 `json:"domain_result_fact"`
    RequestedNotAfterUnixNano    int64                                                 `json:"requested_not_after_unix_nano"`
    Digest                       core.Digest                                           `json:"digest"`
}

type SingleCallModelToolCallProjectionProofV2 struct {
    ProjectionContractVersion string        `json:"projection_contract_version"`
    ProjectionID             string        `json:"projection_id"`
    ProjectionRevision       core.Revision `json:"projection_revision"`
    ProjectionDigest         core.Digest   `json:"projection_digest"`
    InvocationID             string        `json:"invocation_id"`
    InvocationDigest         core.Digest   `json:"invocation_digest"`
    ObservationDigest        core.Digest   `json:"observation_digest"`
    SourceResponseID          string        `json:"source_response_id,omitempty"`
    SourceSequence            uint64        `json:"source_sequence"`
    CallOrdinal               uint32        `json:"call_ordinal"`
    CallID                    string        `json:"call_id"`
    CallName                  string        `json:"call_name"`
    CanonicalArguments        []byte        `json:"canonical_arguments"`
    CanonicalArgumentsLength  uint64        `json:"canonical_arguments_length"`
    CanonicalArgumentsDigest  core.Digest   `json:"canonical_arguments_digest"`
    Digest                    core.Digest   `json:"digest"`
}

type SingleCallModelPendingActionIdentityCurrentV2 struct {
    ContractVersion string                                                `json:"contract_version"`
    RequestDigest   core.Digest                                           `json:"request_digest"`
    IdentityRef     SingleCallModelPendingActionIdentityRefCoordinateV2   `json:"identity_ref"`
    DomainResultFact SingleCallSettledTurnDomainResultFactRefCoordinateV2 `json:"domain_result_fact"`
    Identity        SingleCallModelPendingActionIdentityCoordinateV2      `json:"identity"`
    Projection      SingleCallModelToolCallProjectionProofV2              `json:"projection"`
    CheckedUnixNano int64                                                 `json:"checked_unix_nano"`
    ExpiresUnixNano int64                                                 `json:"expires_unix_nano"`
    Digest          core.Digest                                           `json:"digest"`
}

type SingleCallModelPendingActionIdentityCurrentReaderV2 interface {
    InspectSingleCallModelPendingActionIdentityCurrentV2(context.Context, SingleCallModelPendingActionIdentityCurrentRequestV2) (SingleCallModelPendingActionIdentityCurrentV2, error)
}
```

Harness Adapter还必须注入live公开Model窄口：

```go
type ToolCallCandidateObservationProjectionReaderV1 interface {
    InspectExactProjectionV1(context.Context, modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error)
}
```

这是`ExecutionRuntime/model-invoker`根包现有唯一公开窄Reader；Application contract不import Model类型，只有Harness Adapter依赖根包接口并映射neutral Projection Proof，禁止依赖Model internal/execution/store/write口。`ProjectionContractVersion`逐字保留live `praxis.model-invoker.tool-call-observation-projection/v1`。Projection是immutable historical，无TTL/current语义；Retention导致NotFound或Reader Unavailable时fail closed，不能伪造expiry。Reader必须先执行Projection Validate并证明返回Ref与请求full Ref exact，再验证`Calls==1`、ordinal固定0、Observation digest、id/name、PendingAction、Candidate、SettlementOwner、Schema/ContentDigest，以及`Fact.CreatedUnixNano == Identity.CreatedUnixNano < Identity.NotAfterUnixNano`。

`CanonicalArguments`是唯一允许进入neutral coordinate/proof的受限bytes，且只能来自Model exact Reader返回并已通过Projection Ref、唯一Call和digest校验的canonical arguments。最大长度唯一复用`runtimeports.MaxOpaqueInlineBytes`，合法长度为`1..runtimeports.MaxOpaqueInlineBytes`；空bytes即使携非空digest也Invalid，oversize同样zero Request。Reader Adapter、Seal、Validate/Clone及所有返回路径必须各自deep-copy该slice；调用方修改输入slice或接收方修改返回slice均不得污染已sealed proof、digest或Store内对象。

`CanonicalArgumentsLength == len(bytes)`，且`core.DigestBytes(originalReaderBytes) == CanonicalArgumentsDigest == Identity.CanonicalArgumentsDigest == PendingAction.PayloadDigest`；禁止重序列化、从Event JSON、PendingAction payload或compat tool call反推。除这一条经exact Reader验证并deep-copy的ProjectionProof canonical arguments bytes外，所有其他payload bytes都禁止进入Application neutral coordinate/proof。Run scope digest、RunID、SessionID、Turn必须与Identity SourceKey exact。`RequestedNotAfter < 0`为Invalid；`=0`表示不附加调用方上界；`>0`只能缩短。Identity Current的Checked使用fresh Fact read时刻，Expires为`min(Identity.NotAfter, requested>0)`；Projection Proof本身不提供TTL。Unavailable/NotFound/Conflict/过期均fail closed。

S1/S2必须各自重新调用该Reader；BindingV2/CurrentV3只有IdentityRef，任何代码不得从Ref猜出Created、Owner、Call、arguments或NotAfter。

### 5.2 两个Application proof

```go
type SingleCallHarnessOwnerCurrentProofV3 struct {
    Subject                       SingleCallPendingActionSubjectCoordinateV2         `json:"subject"`
    Binding                       SingleCallHarnessApplicationBindingCoordinateV2    `json:"binding"`
    HarnessCurrentContractVersion string                                             `json:"harness_current_contract_version"`
    HarnessCurrentDigest          core.Digest                                        `json:"harness_current_digest"`
    IdentityCurrent               SingleCallModelPendingActionIdentityCurrentV2      `json:"identity_current"`
    CheckedUnixNano               int64                                              `json:"checked_unix_nano"`
    ExpiresUnixNano               int64                                              `json:"expires_unix_nano"`
    Digest                        core.Digest                                        `json:"digest"`
}

type SingleCallAuthorityCurrentProofV2 struct {
    Ref                    runtimeports.AuthorityBindingRefV2 `json:"ref"`
    ExecutionScopeDigest   core.Digest                        `json:"execution_scope_digest"`
    ActionCoordinateDigest core.Digest                        `json:"action_coordinate_digest"`
    FactRevision           core.Revision                      `json:"fact_revision"`
    FactDigest             core.Digest                        `json:"fact_digest"`
    CheckedUnixNano        int64                              `json:"checked_unix_nano"`
    ExpiresUnixNano        int64                              `json:"expires_unix_nano"`
    Digest                 core.Digest                        `json:"digest"`
}

type SingleCallToolActionInputCurrentProjectionV2 struct {
    ContractVersion string                                 `json:"contract_version"`
    RequestID       string                                 `json:"request_id"`
    RequestRevision core.Revision                          `json:"request_revision"`
    RequestDigest   core.Digest                            `json:"request_digest"`
    ActionDigest    core.Digest                            `json:"action_digest"`
    HarnessCurrent  SingleCallHarnessOwnerCurrentProofV3   `json:"harness_current"`
    AuthorityCurrent SingleCallAuthorityCurrentProofV2     `json:"authority_current"`
    CheckedUnixNano int64                                  `json:"checked_unix_nano"`
    ExpiresUnixNano int64                                  `json:"expires_unix_nano"`
    Digest          core.Digest                            `json:"digest"`
}

type SingleCallToolActionInputCurrentReaderV2 interface {
    InspectSingleCallToolActionInputCurrentV2(context.Context, SingleCallToolActionRequestV2) (SingleCallToolActionInputCurrentProjectionV2, error)
}
```

V2 Proof不携带旧V1的Session/Turn Applicability source字段；它们属于V1兼容合同，不能被补入V2、从CurrentV3猜测或作为额外TTL来源。Binding Base中的PendingAction必须直接复用Application既有nominal `SingleCallPendingActionCoordinateV1`，禁止复制同形`V2`类型形成第二真值。

`HarnessOwnerCurrentProofV3`只能由Harness Adapter把live CurrentV3与Fact Owner Identity Current逐字段映射后seal；`HarnessCurrentContractVersion`固定为`praxis.harness.committed-pending-action-current/v3`。它证明CurrentV3已经按Owner-defined顺序复读Model、DomainResult、Settlement、Generation、Route、Provider bindings、Context及Session S2，并额外保存公开Fact Reader所得full Identity；它不把这些事实重新压成Application自签摘要。不存在Harness CurrentV4要求。

Assembler顺序固定：

```text
validate ActionCoordinate
-> fresh clock
-> exact SessionV4读取BindingV2并逐字段映射neutral Binding
-> SettledTurnDomainResultReaderV3 + Model Projection exact Reader / neutral Identity Current S1
-> CommittedPendingActionReaderV3 S1（完整SubjectV3 + RequestedNotAfter）
-> AuthorityFactReaderV2 S1（exact ref/scope/ActionCoordinateDigest）
-> cross-check Subject/Binding/Current/Authority
-> fresh clock且不得回拨
-> SessionV4 + Identity Fact + Model Projection + CurrentV3 + Authority S2 exact复读
-> compare S1/S2
-> fresh nowS2 + min TTL + Created/Expires
-> seal Request V2 exactly once
-> return sealed Request V2
```

S1与S2都必须执行上述Action交叉闭包；CurrentV3返回成功不能替代Authority复读，Authority成功也不能替代Harness CurrentV3或Context/Route/Generation等Owner current。

CurrentV3内部Model读取不向Application暴露Projection/arguments，因此不能替代上述额外exact Reader。Application不持有Model publish口。

## 6. Tool Owner Result与Application Result双身份

live Tool Owner已有sealed `tool-mcp/contract.ToolResultV2`，但没有独立public `ToolResultRefV2`类型。Application不得重派Tool Result ID或重算/替换其digest；Tool Adapter只在Owner `ToolResultV2.Validate()`成功后逐字段投影以下neutral exact ref，`OwnerContractVersion`必须为live `praxis.tool-mcp.result/v2`。若Tool Owner后续发布nominal public Ref，只能additive替换Adapter来源，不能改变本坐标语义。

```go
type SingleCallToolOwnerResultRefCoordinateV2 struct {
    OwnerContractVersion string                                           `json:"owner_contract_version"`
    ID                   string                                           `json:"id"`
    Revision             core.Revision                                    `json:"revision"`
    Digest               core.Digest                                      `json:"digest"`
    ActionID             string                                           `json:"action_id"`
    ActionRevision       core.Revision                                    `json:"action_revision"`
    ActionDigest         core.Digest                                      `json:"action_digest"`
    ApplyID              string                                           `json:"apply_id"`
    ApplyRevision        core.Revision                                    `json:"apply_revision"`
    ApplyDigest          core.Digest                                      `json:"apply_digest"`
    Inspection           runtimeports.OperationInspectionSettlementRefV4  `json:"inspection"`
    Schema               runtimeports.SchemaRefV2                         `json:"schema"`
    PayloadDigest        core.Digest                                      `json:"payload_digest"`
    PayloadRevision      core.Revision                                    `json:"payload_revision"`
    FinalizedUnixNano    int64                                            `json:"finalized_unix_nano"`
}

type SingleCallToolActionResultCoordinateV2 struct {
    ContractVersion            string                                                   `json:"contract_version"`
    ID                         string                                                   `json:"id"`
    Revision                   core.Revision                                            `json:"revision"`
    RequestID                  string                                                   `json:"request_id"`
    RequestRevision            core.Revision                                            `json:"request_revision"`
    RequestDigest              core.Digest                                              `json:"request_digest"`
    ActionCoordinateDigest     core.Digest                                              `json:"action_coordinate_digest"`
    ToolResult                 SingleCallToolOwnerResultRefCoordinateV2                 `json:"tool_result"`
    Inspection                 runtimeports.OperationInspectionSettlementRefV4          `json:"inspection"`
    Association                runtimeports.OperationSettlementEvidenceAssociationRefV4 `json:"association"`
    AssociationCheckedUnixNano int64                                                    `json:"association_checked_unix_nano"`
    ExpiresUnixNano            int64                                                    `json:"expires_unix_nano"`
    Digest                     core.Digest                                              `json:"digest"`
}

type SingleCallToolActionResultCoordinateSubjectV2 struct {
    RequestID              string                                                   `json:"request_id"`
    RequestRevision        core.Revision                                            `json:"request_revision"`
    RequestDigest          core.Digest                                              `json:"request_digest"`
    ActionCoordinateDigest core.Digest                                              `json:"action_coordinate_digest"`
    ToolResultID           string                                                   `json:"tool_result_id"`
    ToolResultRevision     core.Revision                                            `json:"tool_result_revision"`
    ToolResultDigest       core.Digest                                              `json:"tool_result_digest"`
    InspectionDigest       core.Digest                                              `json:"inspection_digest"`
    Association            runtimeports.OperationSettlementEvidenceAssociationRefV4 `json:"association"`
}

type SingleCallToolActionResultV2 struct {
    ContractVersion string                                 `json:"contract_version"`
    Coordinate      SingleCallToolActionResultCoordinateV2 `json:"coordinate"`
    Digest          core.Digest                            `json:"digest"`
}

type SingleCallToolActionResultRefV2 struct {
    ID                     string        `json:"id"`
    Revision               core.Revision `json:"revision"`
    Digest                 core.Digest   `json:"digest"`
    RequestID              string        `json:"request_id"`
    RequestRevision        core.Revision `json:"request_revision"`
    RequestDigest          core.Digest   `json:"request_digest"`
    ActionCoordinateDigest core.Digest   `json:"action_coordinate_digest"`
    ToolResultID           string        `json:"tool_result_id"`
    ToolResultRevision     core.Revision `json:"tool_result_revision"`
    ToolResultDigest       core.Digest   `json:"tool_result_digest"`
}

type SingleCallToolActionInspectKeyV2 struct {
    ContractVersion        string        `json:"contract_version"`
    RequestID              string        `json:"request_id"`
    RequestRevision        core.Revision `json:"request_revision"`
    RequestDigest          core.Digest   `json:"request_digest"`
    ActionCoordinateDigest core.Digest   `json:"action_coordinate_digest"`
    ScopeDigest            core.Digest   `json:"scope_digest"`
    Digest                 core.Digest   `json:"digest"`
}

type SingleCallToolActionPortV2 interface {
    ExecuteSingleCallToolActionV2(context.Context, SingleCallToolActionRequestV2) (SingleCallToolActionResultV2, error)
    InspectSingleCallToolActionV2(context.Context, SingleCallToolActionInspectKeyV2) (SingleCallToolActionResultV2, error)
}

type SingleCallOperationSettlementCurrentReaderV2 interface {
    InspectCurrentOperationSettlementV4(context.Context, runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error)
    InspectOperationSettlementEvidenceAssociationV4(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error)
}
```

Application ResultCoordinate ContractVersion固定`praxis.application.single-call-tool-action-result-coordinate/v2`；ID在domain `praxis.application.single-call-tool-action-result-coordinate-id-v2`、discriminator `SingleCallToolActionResultCoordinateSubjectV2`下派生，prefix固定`single-call-result-coordinate:v2:`；完整Coordinate digest使用`praxis.application.single-call-tool-action-result-coordinate-v2`。`SingleCallToolActionResultV2.ContractVersion`固定`praxis.application.single-call-tool-action-result/v2`，只封装exact Coordinate并使用独立`praxis.application.single-call-tool-action-result-v2` domain；V1/V2或Tool/Application两个Owner域不得type-pun。

ValidateCurrentFor逐字段证明：Request ID/revision/digest/Action exact；Tool Owner ref ID/revision/digest与Tool Inspect返回的sealed Result exact；Tool Result Inspection、Application Inspection及Association Settlement exact；OperationDigest/EffectID/DomainResult/Guard/Projection闭合；Inspection scope exact等于Request Action scope；Tool Owner Schema/PayloadDigest exact等于Inspection DomainResult；Association checked与所有expiry current。ResultRef逐字段来自已验证Coordinate并保留Tool Owner原始ID/revision/digest。completed CAS只接受该exact Ref；lost reply先Inspect Tool、Runtime Settlement/Association与Coordination Fact，再封装同一Owner ref，不能重派Tool或Application ID。

Result只返回settled ToolResult ref、current V4 Inspection和public Association；不含Context、Continuation、Turn推进或Capability。只读Settlement Reader不嵌Runtime commit口。

Tool Owner已确认live尚缺“Application/Identity/Projection稳定坐标→完整Tool binding exact current”的公开Port；现有binding/plan reader只是`tool-mcp/applicationadapter`私有缝隙。因此P4 Tool consumer与system fixture显式阻断，不能把私有Reader或fixture当公共能力。该Tool Owner Port Delta的最终字段/TTL由Tool Owner另行冻结；Application owner-local contract设计不据此声称P4可实现。

## 7. Coordination Fact/CAS V2

```go
type SingleCallToolActionCoordinationStateV2 string
const (
    SingleCallToolActionPreparedV2       = "prepared"
    SingleCallToolActionDispatchIntentV2 = "dispatch_intent"
    SingleCallToolActionWaitingInspectV2 = "waiting_inspect"
    SingleCallToolActionCompletedV2      = "completed"
)

type SingleCallToolActionCoordinationFactV2 struct {
    ContractVersion string                                  `json:"contract_version"`
    ID              string                                  `json:"id"`
    Revision        core.Revision                           `json:"revision"`
    State           SingleCallToolActionCoordinationStateV2 `json:"state"`
    StartClaimID    string                                  `json:"start_claim_id,omitempty"`
    Request         SingleCallToolActionRequestV2           `json:"request"`
    Result          *SingleCallToolActionResultRefV2        `json:"result,omitempty"`
    CreatedUnixNano int64                                   `json:"created_unix_nano"`
    UpdatedUnixNano int64                                   `json:"updated_unix_nano"`
    Digest          core.Digest                             `json:"digest"`
}

type SingleCallToolActionCrossVersionConflictKeyV1 struct {
    ContractVersion            string          `json:"contract_version"`
    ExecutionScopeDigest       core.Digest     `json:"execution_scope_digest"`
    RunID                      core.AgentRunID `json:"run_id"`
    SessionID                  string          `json:"session_id"`
    Turn                       uint32          `json:"turn"`
    PendingActionRef           string          `json:"pending_action_ref"`
    PendingActionRequestDigest core.Digest     `json:"pending_action_request_digest"`
    Digest                     core.Digest     `json:"digest"`
}

type SingleCallToolActionVersionClaimV1 struct {
    ContractVersion        string                                      `json:"contract_version"`
    ConflictKey            SingleCallToolActionCrossVersionConflictKeyV1 `json:"conflict_key"`
    ClaimedActionVersion   string                                      `json:"claimed_action_version"`
    CoordinationID         string                                      `json:"coordination_id"`
    CoordinationDigest     core.Digest                                 `json:"coordination_digest"`
    Revision               core.Revision                               `json:"revision"`
    CreatedUnixNano        int64                                       `json:"created_unix_nano"`
    Digest                 core.Digest                                 `json:"digest"`
}

type SingleCallToolActionCoordinationCASRequestV2 struct {
    ContractVersion  string                                      `json:"contract_version"`
    Scope            core.ExecutionScope                         `json:"scope"`
    ID               string                                      `json:"id"`
    ExpectedRevision core.Revision                               `json:"expected_revision"`
    ExpectedDigest   core.Digest                                 `json:"expected_digest"`
    Next             SingleCallToolActionCoordinationFactV2      `json:"next"`
    Digest           core.Digest                                 `json:"digest"`
}

type SingleCallToolActionCoordinationFactPortV2 interface {
    CreateSingleCallToolActionCoordinationV2(context.Context, SingleCallToolActionCoordinationFactV2) (SingleCallToolActionCoordinationFactV2, error)
    InspectSingleCallToolActionCoordinationV2(context.Context, core.ExecutionScope, string) (SingleCallToolActionCoordinationFactV2, error)
    CompareAndSwapSingleCallToolActionCoordinationV2(context.Context, SingleCallToolActionCoordinationCASRequestV2) (SingleCallToolActionCoordinationFactV2, error)
}
```

唯一状态机：`prepared → dispatch_intent → waiting_inspect → completed`。`waiting_inspect`必须携唯一且不可替换的StartClaim。

Fact硬不变量：`Fact.ID == Fact.Request.ID`；Store/Create/CAS/Inspect的Scope必须`SameExecutionScopeV2(Request.Action.ExecutionScope)`且scope digest exact；Fact.Created必须等于Request.Created；Request/ActionDigest/Created在所有revision中immutable；next revision严格`current+1`，Updated不回拨；completed必须携exact ResultRef且其Request/Action/ToolResult三重坐标匹配Fact。Create/CAS/Inspect key任一字段缺失、版本错误或type-pun均fail closed。

`SingleCallToolActionCoordinationCASRequestV2`只携Expected revision/digest与structural exact Next；它不携完整Application Result、不复读Tool Owner、不把`Next.UpdatedUnixNano`当current检查时钟，因此不能独立证明ToolResult真实性。Coordinator必须先用fresh clock完成Tool Result、Runtime Settlement与Association复验，再由`CompleteSingleCallToolActionCoordinationFactV2`从完整Result派生Ref并CAS exact Next。Tool Owner canonical result current与提交边界可信时钟仍是P4/system硬门，P2 fake/FactPort通过不得升级为该权威证明。

Create对V2自身仍以`Scope+ID` create-once：absent时写prepared；same canonical返回同一Fact；same ID任一内容/版本不同Conflict；Create回包丢失只按Scope+ID Inspect并逐字段exact compare。

由于V1/V2 Request ID使用不同canonical domain/prefix，`Scope+ID`不足以阻止双版本执行。Application Coordination Owner必须额外维护上述跨版本稳定键：每次都从Request的scope/run/session/turn/PendingAction ref+request digest稳定语义坐标重新计算`ConflictKey`，禁止caller传入或缓存旧键；domain固定`praxis.application.single-call-tool-action-cross-version-key-v1`。

`ClaimedActionVersion`是closed string，只允许并必须exact等于V1或V2 Request的固定ContractVersion常量；它必须与同一原子写入中的`Fact.Request.ContractVersion` exact。Claim还必须满足：`CoordinationID == Fact.ID == Request.ID`；`CoordinationDigest == initial prepared Fact.Digest`；`CreatedUnixNano == Fact.CreatedUnixNano == Request.CreatedUnixNano`。其中`CoordinationDigest`永久绑定revision=1、state=`prepared`的初始Fact摘要，不随Fact后续revision/current digest变化；恢复时用`Claim.CoordinationID + Claim.CoordinationDigest`核对历史初始prepared Fact，禁止拿current Fact digest比较。

Application Coordination Owner的同一Store、同一锁/事务和同一原子线性点必须create-once写入`VersionClaim + initial prepared Coordination Fact`，不能先claim后fact、先fact后claim或用sidecar补齐。Claim revision固定1且create-once；相同语义键只能由一个closed `claimed_action_version`占用。另一版本、另一CoordinationID或changed digest一律Conflict，不能通过删除/重建切换版本。VersionClaim是冲突门而非第二Coordination Fact，不构成业务双写。

Create、Inspect和每次CAS都必须从Fact.Request重算同一ConflictKey，并复读、复验同一VersionClaim；wrong/unknown version string、wrong key、CoordinationID/Fact.ID/Request.ID不等、CoordinationDigest与初始prepared Fact不等、Created三方不等，全部zero write、zero state transition、zero Execute。Claim缺失、Reader不可用或无法证明初始prepared摘要时fail closed；CAS不得改写或替换Claim。

V1现有owner-local实现未接该Claim，因此V1必须由Assembly/Conformance从系统G6A route彻底排除，V1 Tool Port/Coordinator调用计数为0；若未来允许V1进入任何共享系统路径，必须先additive接入同一Coordination Owner、同一原子VersionClaim+Fact线性点并独立验收。禁止隐式转换、默认迁移、sidecar补claim或V1/V2业务Fact双写。Inspect/CAS必须拒绝缺ContractVersion/discriminator或旧版本对象。

执行顺序固定且不可交换：

```text
prepared --CAS exact--> dispatch_intent
dispatch_intent --CAS exact(StartClaim)--> waiting_inspect
CAS返回exact Next成功
-> 才获得一次ExecuteSingleCallToolActionV2调用权
```

StartClaim由Request ID/digest与Action digest在V2专属domain确定性派生，创建后不可替换。**只有第二个CAS成功回包是Execute权**；CAS lost reply、Conflict、Unavailable、Indeterminate，或Inspect发现已是`waiting_inspect`，全部禁止Execute，只允许Tool Inspect。即使事实可能在首次Tool调用前进入waiting_inspect，也选择fail closed，不以NotFound推断“从未开始”。Tool回包丢失后同样永久Inspect-only。

CAS正常回包或lost reply恢复只接受逐字段exact Next；同ID/Revision但ExpectedDigest、Next或CAS digest漂移必须Conflict。immutable Request、Created、StartClaim不得改变；Revision严格+1、Updated不回拨。CAS ContractVersion固定`praxis.application.single-call-coordination/v2`，InspectKey ContractVersion固定`praxis.application.single-call-tool-action/v2`且RequestRevision固定1。

## 8. Canonical、domain与TTL

所有集合稳定排序去重，nil slice按字段规则归一，禁止map、未知required字段、opaque JSON和owner struct。除Model exact Reader返回、经Ref/Call/digest验证并在Reader Adapter、Seal、Clone/返回路径deep-copy的ProjectionProof canonical arguments bytes外，其他payload bytes一律禁止进入neutral coordinate/proof。对象固定domain/type discriminator：

| 对象 | canonical domain | discriminator |
|---|---|---|
| Identity | `praxis.application.single-call-identity-coordinate-v2` | `SingleCallModelPendingActionIdentityCoordinateV2` |
| Identity ref | `praxis.application.single-call-identity-ref-coordinate-v2` | `SingleCallModelPendingActionIdentityRefCoordinateV2` |
| DomainResult Fact ref | `praxis.application.single-call-domain-result-ref-coordinate-v2` | `SingleCallSettledTurnDomainResultFactRefCoordinateV2` |
| Binding Base | `praxis.application.single-call-harness-binding-coordinate-v2` | `SingleCallHarnessBaseBindingCoordinateV2` |
| Binding OwnerInputs | `praxis.application.single-call-harness-binding-coordinate-v2` | `SingleCallHarnessOwnerCurrentInputsCoordinateV2` |
| Binding | `praxis.application.single-call-harness-binding-coordinate-v2` | `SingleCallHarnessApplicationBindingCoordinateV2` |
| Run subject | `praxis.application.single-call-pending-subject-v2` | `SingleCallRunSubjectCoordinateV2` |
| PendingAction subject | `praxis.application.single-call-pending-subject-v2` | `SingleCallPendingActionSubjectCoordinateV2` |
| Action coordinate | `praxis.application.single-call-action-coordinate-v2` | `SingleCallActionCoordinateV2` |
| Identity current request | `praxis.application.single-call-identity-current-v2` | `SingleCallModelPendingActionIdentityCurrentRequestV2` |
| Model Projection proof | `praxis.application.single-call-model-projection-proof-v2` | `SingleCallModelToolCallProjectionProofV2` |
| Identity current | `praxis.application.single-call-identity-current-v2` | `SingleCallModelPendingActionIdentityCurrentV2` |
| Request ID subject | `praxis.application.single-call-tool-action-request-id-v2` | `SingleCallToolActionRequestSubjectV2` |
| Request | `praxis.application.single-call-tool-action-v2` | `SingleCallToolActionRequestV2` |
| Tool Owner result ref coordinate | `praxis.application.single-call-tool-owner-result-ref-v2` | `SingleCallToolOwnerResultRefCoordinateV2` |
| Result coordinate ID subject | `praxis.application.single-call-tool-action-result-coordinate-id-v2` | `SingleCallToolActionResultCoordinateSubjectV2` |
| Result coordinate | `praxis.application.single-call-tool-action-result-coordinate-v2` | `SingleCallToolActionResultCoordinateV2` |
| Result | `praxis.application.single-call-tool-action-result-v2` | `SingleCallToolActionResultV2` |
| Result ref | `praxis.application.single-call-tool-action-v2` | `SingleCallToolActionResultRefV2` |
| Inspect key | `praxis.application.single-call-tool-action-v2` | `SingleCallToolActionInspectKeyV2` |
| Harness current proof | `praxis.application.single-call-current-v2` | `SingleCallHarnessOwnerCurrentProofV3` |
| Authority proof | `praxis.application.single-call-current-v2` | `SingleCallAuthorityCurrentProofV2` |
| Input current | `praxis.application.single-call-current-v2` | `SingleCallToolActionInputCurrentProjectionV2` |
| Coordination Fact | `praxis.application.single-call-coordination-v2` | `SingleCallToolActionCoordinationFactV2` |
| Coordination CAS | `praxis.application.single-call-coordination-v2` | `SingleCallToolActionCoordinationCASRequestV2` |
| Cross-version key | `praxis.application.single-call-tool-action-cross-version-key-v1` | `SingleCallToolActionCrossVersionConflictKeyV1` |
| Version claim | `praxis.application.single-call-tool-action-version-claim-v1` | `SingleCallToolActionVersionClaimV1` |

合同版本分别固定为`praxis.application.single-call-action-coordinate/v2`、`praxis.application.single-call-tool-action/v2`、`praxis.application.single-call-tool-action-result/v2`、`praxis.application.single-call-current/v2`与`praxis.application.single-call-coordination/v2`。每个对象只排除自身Digest字段后摘要；Request与ResultCoordinate ID以专属ID domain与subject discriminator派生。strict decoder必须检查ContractVersion+nominal discriminator，V1/V2或Tool/Application两个Owner域不能互解或默认迁移。

每个current/commit/return边界使用fresh clock并拒绝回拨。Application不重算Harness内部TTL；Request expiry严格取`min(CurrentV3.Expires, Authority.Expires, RequestedNotAfter>0)`，其中CurrentV3已由Harness Owner纳入Identity/Context/Route/Generation等最小值。任一Reader/Tool/Settlement/Association调用途中跨TTL，结果必须拒绝。

## 9. 依赖DAG与实施硬停

```text
runtime/core + runtime/ports
           ↑
application/contract -> application/ports -> application/coordinator
           ↑                    ↑
Harness-owned adapter           Tool-owned adapter
  -> SessionV4                  -> Tool owner facts
  -> SettledTurnDomainResultReaderV3
  -> CurrentReaderV3            -> Runtime Governance
  -> AuthorityFactReaderV2
```

Owner Adapter只依赖Application public contract/ports与各自Owner公开读口。Application不得import Owner实现；Harness不import Tool；Tool不import Harness。test-only fixture可以手工注入公开Ports，但不得直接Seal Request绕过Assembler，也不得声称存在production root。

本design/plan已通过owner-local独立终审；Application owner-local P2代码也已通过第四独立终审（P0/P1/P2=0）。Harness P3已实现，并在live合同漂移与并发P1返修后独立复审YES（P0/P1/P2=0）。Tool P4仍等待Tool Binding及Tool Owner result current公开Port，可信生产提交时钟也未闭合；P5必须等待P4并通过跨模块fixture全门。系统G6A、production root、G6B Context Refresh与Continuation继续独立阻断。
