# G6A Model Tool Call → PendingAction Identity V1

## 1. 状态、目的与范围

状态：Identity V1五资产、Owner-current [Port Delta](../port-deltas/committed-pending-action-owner-current-inputs-v2.md)与对应V3/V4实现、Harness P3 Assembler/InputCurrent Reader最终独立审计均为`YES(P0/P1/P2=0)`。Identity/Fact/Session、Binding/Current及P3只读Adapter已实现并验收；Tool Consumer/P4、system G6A/G6B/production root仍未完成并继续`NO-GO`。

### 0.1 实现前A–G精确冻结门

本节记录中央已接受的精确冻结值；独立设计短审前不得改写、弱化或以实现反推合同：

| 项 | 中央冻结值 | 实现硬门 |
|---|---|---|
| A | `GovernedSessionV3`完整镜像V2并additive增加`ApplicationBinding`与`Digest`；`SessionCASRequestV3`携完整expected与自身Digest；Create/CAS/V2共享冲突域见§3.4 | 一次CAS写完整Binding；V2不得翻译或冒充V3 |
| B | Content canonical body精确为Candidate、顶层完整Model Projection Ref、PendingAction、完整Identity；排除IdentityRef/ContentDigest/FactDigest，见§3.3 | IdentityRef只能在ContentDigest完成后派生，禁止摘要自引用 |
| C | 两个独立helper/domain/subject/prefix，见§3.1与§3.3 | 两个ID只共同绑定SourceKeyDigest，必须不同，禁止后缀推断 |
| D | `CommittedPendingActionSubjectV2`保持无`ContractVersion` | 不得自行增加、默认或从Current版本反推Subject版本 |
| E | 唯一Model import根包为`github.com/Proview-China/rax/ExecutionRuntime/model-invoker` | 仅公开full Ref/Projection/Reader；禁止internal/execution/direct/store/publisher/writer/event/provider类型 |
| F | 公共类型在Harness contract；唯一写口在Harness ports；exact SettlementOwner绑定provider adapter拥有语义与持久化，见§2/§3.3 | active binding只注入一个Repository capability；Application不得拥有Repository；fake仅测试 |
| G | `RequestedNotAfter==0`不增加上界；负数非法；正数只能缩短；三组Validate签名与时序见§3.5 | 固定fresh S1→Owner复读→S2→返回前fresh clock；30秒cap且不得延长natural expiry |

附加实现硬门：全链必须逐字段保留完整ExecutionScope/Projection/Candidate/Ordinal/SettlementOwner；Repository必须单线性提交FactID/IdentityID/SourceKey三键；Session V3必须原子携完整ApplicationBinding且V2不可冒充；Current V2必须固定S1→Owner复读→S2、30秒TTL cap；所有跨Port指针与slice必须deep clone。

本设计冻结N=1 G6A中缺失的强类型因果链：Model Invoker已经发布的一个完整Tool Call Projection，必须由绑定的Model-turn Settlement Owner以**恒等映射**写入同一份SettledTurn DomainResult；Runtime Settlement绑定该DomainResult后，Harness才可在一次Session CAS中提交`PendingAction`及其Identity。它不允许隐式参数转换，也不授权Provider调用。

首切面固定：

- `len(ToolCallCandidateObservationProjectionV1.Observation.Calls) == 1`；
- `CallOrdinal == 0`；
- `CanonicalArgumentsDigest == PendingAction.Payload.ContentDigest`；
- `PendingAction.SourceCandidate == SettledTurnResult.Candidate`；
- `PendingAction`与Identity必须处于同一canonical SettledTurn DomainResult；
- `N>1`、batch、schema/default transformation、Context Refresh、Continuation和Turn推进全部NO-GO。

现有V1 equality继续保留，但它只在Tool CanonicalCommand阶段拒绝摘要漂移，不能冒充Model→Settlement→Harness→Application的系统闭环。本Delta使用additive版本，不修改`SettledTurnResultV2`、`PendingActionV2`、`CommittedPendingActionCurrentV1`或`SingleCallToolActionRequestV1`既有摘要。

## 2. Owner与非Owner

| 对象/动作 | 唯一Owner | 允许 | 禁止 |
|---|---|---|---|
| Model Projection与canonical arguments | Model Invoker | 发布、按完整Ref exact Inspect | 创建PendingAction、Settlement或Tool Candidate |
| `ModelToolCallPendingActionIdentityV1`语义 | 绑定的Model-turn Settlement Owner | 从exact Projection与本次SettledTurn Candidate创建恒等Identity，并写进同一DomainResult | Harness、Application或Tool补造、转换或重封 |
| Model-turn Runtime Settlement | Runtime | 绑定exact DomainResult schema/digest和Operation事实 | 解释Tool Call语义、创建Identity |
| Session、PendingAction与应用CAS | Harness Session Owner | 只在exact Runtime Settlement后原子应用同一DomainResult中的PendingAction+Identity | 从Observation直接升级PendingAction、分两次提交因果关系 |
| `CommittedPendingActionCurrentV2` | Harness | exact读取Session、Identity、DomainResult/Settlement和短租约 | 生成新Identity、延长底层TTL |
| Application V2 DTO/Port/Coordinator | Application | 携带neutral exact Identity坐标并协调S1/S2 | import Harness/Model/Tool事实、成为Identity Owner |
| Application Assembler Adapter实现 | Harness Owner Adapter | 聚合Model/Harness/Context/Assembly/Runtime只读Readers，形成Application Request | 直接dispatch、写Tool/Runtime事实 |
| Tool V2 Consumer | Tool Owner Adapter | Watermark前通过Application公开neutral Reader复读Identity | import Harness实现、把V1 equality当系统闭环 |
| Route/Port注入与Conformance | Harness Assembly | 校验接口版本、Owner/Binding/Generation/Route current | 创作Identity、承担production composition root |

`type Owner != fact semantic Owner`：Model-turn Settlement Owner拥有Identity事实语义；Application只拥有给Coordinator和Tool消费的neutral coordinate/Reader接口；Harness Adapter实现这些接口，但不能改变Identity内容。

`SettledTurnDomainResultFactV3`、Identity、Ref及ID subject等公共值类型唯一落在`ExecutionRuntime/harness/contract`；唯一写能力`SettledTurnDomainResultRepositoryV3`落在`ExecutionRuntime/harness/ports`。Fact中exact `SettlementOwner`所绑定的provider adapter是Identity/DomainResult的语义与持久化Owner；每个active binding只能注入一个Repository capability，并由同一实例原子维护FactID、IdentityID、SourceKeyDigest三索引。`SettledTurnDomainResultReaderV3`只是该同实例的窄只读视图。Application只能定义neutral Reader/Assembler并协调调用，不得定义、持有或代理Repository；Harness fake只用于合同、并发和故障测试，不代表生产Backend、durability或SLA。

## 3. Additive对象

### 3.1 稳定source key、ordinal与Identity Ref

```go
type ModelToolCallOrdinalV1 struct {
    EncodingVersion string // 1.0.0
    Present         bool   // 必须true
    Value           uint32 // 首切面固定0
}

type ModelToolCallPendingActionIdentitySourceKeyV1 struct {
    ExecutionScopeDigest core.Digest
    RunID                string
    SessionID            string
    Turn                 uint32
    Candidate             CandidateRefV2
    ModelProjection       ToolCallCandidateObservationRefV1 // full ref
    CallOrdinal           ModelToolCallOrdinalV1
    SettlementOwner       runtimeports.ProviderBindingRefV2
}
```

`IdentitySourceKeyV1`是唯一稳定索引：`ExecutionScopeDigest + RunID + SessionID + Turn + CandidateRef + full ModelProjectionRef + CallOrdinal + bound SettlementOwnerRef`。`CallOrdinal`用presence+versioned decode区分“缺失”和合法0；`Present=false`、未知EncodingVersion或Value非0均在零写前拒绝。

Identity ID只允许通过下列helper派生：

```go
type IdentityIDSubjectV1 struct {
    SourceKeyDigest core.Digest
}

func DeriveModelToolCallPendingActionIdentityIDV1(sourceKeyDigest core.Digest) (string, error)
```

- canonical domain：`praxis.harness.model-tool-call-pending-action-identity-id`；
- canonical subject：`IdentityIDSubjectV1`；
- canonical version：`ModelToolCallPendingActionIdentityContractVersionV1`；
- 输出：`"mtpa-identity:v1:" + string(canonicalDigest)`；
- 输入只能是已经校验的完整`SourceKeyDigest`，不得包含PendingAction内容、TTL或完整Identity subject。

```go
type ModelToolCallPendingActionIdentityRefV1 struct {
    ID                         string
    Revision                   core.Revision // 固定1
    Digest                     core.Digest
    ModelProjectionID          string
    ModelProjectionRevision    core.Revision
    ModelProjectionDigest      core.Digest
    PendingActionRef           string
    PendingActionRequestDigest core.Digest
    DomainResultDigest         core.Digest
	SourceKeyDigest            core.Digest
}
```

Ref是exact历史定位，不是current证明、Evidence、Permit或Enforcement。Settlement Owner Repository以同一线性点维护`FactID`、`IdentityID`与`SourceKeyDigest`三个唯一索引：同source key同内容幂等，同source key换PendingAction/参数/TTL等内容Conflict；64个不同内容并发竞争同source key只有一个胜出，首记录不变。

### 3.2 `ModelToolCallPendingActionIdentityV1`

```go
type ModelToolCallPendingActionIdentityV1 struct {
    ContractVersion            string // praxis.harness.model-tool-call-pending-action-identity/v1
    ID                         string
    Revision                   core.Revision // 固定1
	SourceKey                  ModelToolCallPendingActionIdentitySourceKeyV1
    ModelProjection            ToolCallCandidateObservationRefV1
	CallOrdinal                ModelToolCallOrdinalV1
	SettlementOwner            runtimeports.ProviderBindingRefV2
    CallID                     string
    CallName                   string
    CanonicalArgumentsDigest   core.Digest
    PendingActionRef           string
    PendingActionRequestDigest core.Digest
    PayloadSchema              runtimeports.SchemaRefV2
    PayloadContentDigest       core.Digest
    Capability                 runtimeports.CapabilityNameV2
    SourceCandidate            CandidateRefV2
    CreatedUnixNano            int64
    NotAfterUnixNano           int64
    Digest                     core.Digest
}
```

不变量：

1. `ModelProjection`必须由Model Owner exact Reader复读完整Projection，Ref全字段、Observation digest与`Calls==1`均通过；
2. 唯一Call的ordinal/ID/name/canonical arguments digest与Identity逐字段相等；ordinal必须`Present=true, EncodingVersion=1.0.0, Value=0`；
3. `CanonicalArgumentsDigest`的唯一算法是对Model Owner公开Reader返回的canonical argument bytes调用live `core.DigestBytes(bytes)`；Settlement/Harness/Application/Tool不得接受Owner自签摘要，也不得重新canonicalize后换算法；结果必须等于`PayloadContentDigest`；
4. PendingAction的ref/request digest/schema/content digest/capability/source candidate与Identity完全相等；
5. Identity与PendingAction必须编码进同一`SettledTurnResultV3(action_required)` DomainResult；
6. Identity不持有payload bytes；canonical arguments和PendingAction payload仍由各自Owner读取；
7. `NotAfterUnixNano`是“允许创建新G6A Action”的上界，不删除历史事实、不延长Model Candidate、Session、Binding、Authority或Context current；
8. 模式固定为identity，不提供TransformationKind、自由extension或默认值注入。

`Identity.SourceKey.ModelProjection == Identity.ModelProjection`、`Identity.SourceKey.CallOrdinal == Identity.CallOrdinal`、`Identity.SourceKey.Candidate == Identity.SourceCandidate`、`Identity.SourceKey.SettlementOwner == Identity.SettlementOwner`必须逐字段exact成立；DomainResult顶层`ModelProjection/Candidate/SettlementOwner`又必须分别等于Identity这三个顶层字段。任何把A事实的SourceKey与B事实的Identity顶层字段拼接后重Seal的splice都必须在Repository零写前Conflict。

### 3.3 `SettledTurnDomainResultFactV3`

`SettledTurnResultV3`是additive schema，只扩展`action_required`分支；其权威持久对象不是裸body，而是Settlement Owner拥有的DomainResult Fact：

```go
type SettledTurnDomainResultFactRefV3 struct {
    FactID           string
    Revision         core.Revision
    FactDigest       core.Digest
    SourceKeyDigest  core.Digest
    Schema           runtimeports.SchemaRefV2
    ContentDigest    core.Digest
    IdentityRef      ModelToolCallPendingActionIdentityRefV1
}

type SettledTurnDomainResultFactV3 struct {
    ContractVersion  string
    FactID           string
    Revision         core.Revision // 固定1
    SourceKey        ModelToolCallPendingActionIdentitySourceKeyV1
    Candidate        CandidateRefV2
    ModelProjection  ToolCallCandidateObservationRefV1
    PendingAction    PendingActionV2
    Identity         ModelToolCallPendingActionIdentityV1
    SettlementOwner  runtimeports.ProviderBindingRefV2
    Schema           runtimeports.SchemaRefV2 // settled-turn-result@3.0.0
    ContentDigest    core.Digest
    CreatedUnixNano  int64
    FactDigest       core.Digest
}

type SettledTurnDomainResultRepositoryV3 interface {
    EnsureExact(context.Context, SettledTurnDomainResultFactV3) (SettledTurnDomainResultFactV3, error)
    InspectExact(context.Context, SettledTurnDomainResultFactRefV3) (SettledTurnDomainResultFactV3, error)
}

type SettledTurnDomainResultReaderV3 interface {
    InspectExact(context.Context, SettledTurnDomainResultFactRefV3) (SettledTurnDomainResultFactV3, error)
}
```

Content canonical body固定为：

```go
type SettledTurnDomainResultContentV3 struct {
    Candidate       CandidateRefV2
    ModelProjection ToolCallCandidateObservationRefV1 // 顶层完整public Ref
    PendingAction   PendingActionV2
    Identity        ModelToolCallPendingActionIdentityV1 // 包含Identity.Digest
}
```

`ContentDigest`精确覆盖上述四字段，明确排除`IdentityRef`、`ContentDigest`与`FactDigest`。`IdentityRef`只能在ContentDigest完成后由完整Identity与该ContentDigest派生并写入FactRef，禁止把IdentityRef放回Content body造成摘要自引用。`SettlementOwner`、`Schema`、`CreatedUnixNano`属于Fact envelope，只进入`FactDigest`；FactDigest还覆盖Fact header、SourceKey与ContentDigest。

Fact ID只允许通过独立helper派生：

```go
type FactIDSubjectV3 struct {
    SourceKeyDigest core.Digest
}

func DeriveSettledTurnDomainResultFactIDV3(sourceKeyDigest core.Digest) (string, error)
```

- canonical domain：`praxis.harness.settled-turn-domain-result-fact-id`；
- canonical subject：`FactIDSubjectV3`；
- canonical version：`SettledTurnDomainResultContractVersionV3`；
- 输出：`"settled-turn-fact:v3:" + string(canonicalDigest)`。

IdentityID与FactID仅共同绑定同一个SourceKeyDigest；因domain、subject与prefix均不同，两者必须不同，禁止截前缀、比较后缀或从一个ID推断另一个。同一个`SettledTurnDomainResultRepositoryV3` capability在同一线性化提交里写FactID、IdentityID与SourceKeyDigest三个唯一索引，并同时提供`EnsureExact/InspectExact`。同实例同canonical重放幂等，任一键换内容Conflict；`EnsureExact`丢回包后必须以同一canonical Fact重调同一实例的原子`EnsureExact`并逐字段比较返回值，不得换Repository或切Reader猜结果。Reader只能是该实例的窄只读视图，不能参与Create/恢复线性化。

live `runtimeports.OperationSettlementRefV3`只绑定`DomainResultSchema + DomainResultDigest`，没有也不得伪造`DomainResultFactRef`字段。Runtime Settlement必须逐字段等于本Fact的`Schema + ContentDigest`；exact `SettledTurnDomainResultFactRefV3`只由Harness `PendingActionApplicationBindingV1`持久保存，后续Reader同时复读Fact与Runtime Settlement并交叉验证schema/content digest。禁止先settle PendingAction、后以sidecar补Identity。Runtime Settlement与Harness Session CAS是两个Owner的顺序提交，不宣称跨Owner原子事务。

### 3.4 `PendingActionApplicationBindingV1`与Session V3原子提交

```go
type PendingActionApplicationBindingV1 struct {
    PendingAction          PendingActionV2
    IdentityRef            ModelToolCallPendingActionIdentityRefV1
    DomainResultFactRef    SettledTurnDomainResultFactRefV3
    ModelTurnSettlementRef runtimeports.OperationSettlementRefV3
}
```

Harness additive `GovernedSessionV3`继承`GovernedSessionV2`全部既有合法转换、字段规则与phase可达性；Create仍由`creating`开始，既有V2转换在V3中按原规则继续成立。本Delta唯一新增并解冻的转换语义是`waiting_settlement -> waiting_action`一次CAS原子写入完整`PendingActionApplicationBindingV1`。`PendingAction`、Identity Ref、DomainResult Fact Ref或ModelTurn Settlement Ref缺一/漂移均零CAS；不得先写PendingAction再补关联。未来若新增其他转换语义，必须新立版本；不得把“唯一新增转换”误读为“V3只允许这一条转换”。

`GovernedSessionV3`完整镜像V2字段，只additive增加ApplicationBinding与Digest；不允许删字段、改字段语义或把V2翻译成V3：

```go
type GovernedSessionV3 struct {
    ContractVersion        string // GovernedContractVersionV3
    ID                     string
    Revision               core.Revision
    Run                    RunRef
    Endpoint               EndpointRefV2
    Phase                  SessionPhaseV2
    Turn                   uint32
    Candidate              *CandidateRefV2
    DomainReservation      *ModelDispatchReservationRefV2
    Execution              *runtimeports.GovernedExecutionAttemptRefsV2
    PendingAction          *PendingActionV2
    ApplicationBinding     *PendingActionApplicationBindingV1
    PendingInput           *PendingInputV2
    UndispatchedSettlement *UndispatchedSettlementBindingV2
    CompletionClaim        CompletionClaim
    CreatedUnixNano        int64
    UpdatedUnixNano        int64
    Digest                 core.Digest
}

type SessionCASRequestV3 struct {
    ContractVersion  string // SessionCASContractVersionV3
    Run              RunRef
    SessionID        string
    ExpectedRevision core.Revision
    ExpectedDigest   core.Digest
    Next             GovernedSessionV3
    Digest           core.Digest
}
```

Session与CAS Request使用两套独立canonical常量，禁止复用domain、version或subject：

```text
GovernedSessionV3CanonicalDomain  = "praxis.harness.governed-session"
GovernedContractVersionV3         = "praxis.harness.governed/v3"
GovernedSessionV3CanonicalSubject = "GovernedSessionV3"

SessionCASRequestV3CanonicalDomain  = "praxis.harness.session-cas-request"
SessionCASContractVersionV3         = "praxis.harness.session-cas/v3"
SessionCASRequestV3CanonicalSubject = "SessionCASRequestV3"
```

`GovernedSessionV3.Digest`输入是完整V3对象并把自身`Digest`置空；覆盖`ContractVersion/ID/Revision/Run/Endpoint/Phase/Turn/Candidate/DomainReservation/Execution/PendingAction/ApplicationBinding/PendingInput/UndispatchedSettlement/CompletionClaim/CreatedUnixNano/UpdatedUnixNano`全部字段。`SessionCASRequestV3.Digest`输入精确覆盖`ContractVersion/Run/SessionID/ExpectedRevision/ExpectedDigest/Next`，计算时只把Request自身`Digest`置空；`Next.Digest`作为Next完整canonical内容的一部分保留。两者均通过各自domain、contract version与canonical subject调用同一live canonical JSON digest算法，不允许实现自行选择别名或复用另一对象的常量。

Session V3 invariant：

1. `Digest`严格使用上列对象专属domain/version/subject与字段集合；只排除正在计算对象的自身Digest；所有跨Port指针/slice必须deep clone；
2. 除`waiting_action`外`ApplicationBinding == nil`；`waiting_action`必须同时具有`PendingAction`与完整Binding，且`Binding.PendingAction`与Session PendingAction逐字段exact；
3. Binding的DomainResult schema/content必须与`Execution.Settlement.DomainResultSchema/DomainResultDigest`逐字段exact；FactRef只存Session Binding，不注入Runtime Settlement不存在的FactRef字段；
4. V3继承V2全部既有合法转换和字段规则；`waiting_settlement -> waiting_action`是本Delta唯一新增/解冻的原子Binding语义，一次CAS同时写PendingAction与完整Binding，禁止sidecar或第二次补写；
5. CAS Request的ContractVersion/Run/SessionID/ExpectedRevision/ExpectedDigest/Next/Digest全部必填；`Next.Revision == ExpectedRevision + 1`，Run与SessionID不变，ExpectedDigest必须exact命中当前Session V3；
6. Create只接受`Revision==1 && Phase==creating && ApplicationBinding==nil`。存储键为完整`Run{Scope,RunID}+SessionID`；absent时创建，同canonical重放幂等，同键换内容Conflict；lost reply只以完整键Inspect并比较exact canonical；
7. V2/V3共享同一完整Run/Scope+SessionID冲突域；任一版本已有同键事实时，另一版本Create必须Conflict。没有隐式迁移、默认升级、字段翻译或双写。

### 3.5 `CommittedPendingActionCurrentV2`

```go
type CommittedPendingActionCurrentV2 struct {
    ContractVersion         string
    Run                     RunRef
    ExecutionScopeDigest    core.Digest
    SessionID               string
    SessionRevision         core.Revision
    SessionDigest           core.Digest
    Phase                   SessionPhaseV2 // waiting_action
    Turn                    uint32
    PendingAction           PendingActionV2
	ApplicationBinding      PendingActionApplicationBindingV1
    SessionApplicability    CommittedPendingActionSessionApplicabilityCoordinateV1
    TurnApplicability       CommittedPendingActionTurnApplicabilityCoordinateV1
    CheckedUnixNano         int64
    ExpiresUnixNano         int64
    Digest                  core.Digest
}

type CommittedPendingActionSubjectV2 struct {
    ExecutionScopeDigest core.Digest
    Run                  RunRef
    SessionID            string
    SessionRevision      core.Revision
    SessionDigest        core.Digest
    Turn                 uint32
    PendingActionRef     string
    IdentityRef          ModelToolCallPendingActionIdentityRefV1
    DomainResultFactRef  SettledTurnDomainResultFactRefV3
    ModelTurnSettlement  runtimeports.OperationSettlementRefV3
}

type CommittedPendingActionCurrentRequestV2 struct {
    Subject               CommittedPendingActionSubjectV2
    RequestedNotAfterUnixNano int64
}

type CommittedPendingActionReaderV2 interface {
    InspectCommittedPendingActionCurrentV2(context.Context, CommittedPendingActionCurrentRequestV2) (CommittedPendingActionCurrentV2, error)
}

func (r CommittedPendingActionCurrentRequestV2) Validate(now time.Time) error
func (p CommittedPendingActionCurrentV2) Validate(now time.Time) error
func (p CommittedPendingActionCurrentV2) ValidateAgainst(expected CommittedPendingActionCurrentRequestV2, now time.Time) error
```

`CommittedPendingActionSubjectV2`保持上述精确字段集且**没有ContractVersion**；不得自行增加、从CurrentV2版本推断、使用零值默认或以另一DTO替代。

`CommittedPendingActionSubjectV2.Run`必须携完整Harness `RunRef{Scope,RunID}`；Validate必须调用`Run.Validate()`并用live `runtimeports.ExecutionScopeDigestV2(Run.Scope)`重算`ExecutionScopeDigest`，禁止调用方只给RunID或自签scope digest。`CommittedPendingActionReaderV2`按完整Subject exact读取，执行Session S1→字段校验→Session S2，并额外复读：

- exact Model Projection；
- exact `SettledTurnDomainResultFactV3`，重算Fact/schema/content digest；
- exact Runtime Model-turn Settlement；
- Identity与PendingAction完整相等关系。

Reader还必须把Identity稳定SourceKey与当前Session逐字段交叉验证：`SourceKey.ExecutionScopeDigest == Current.ExecutionScopeDigest`、`SourceKey.RunID == string(Current.Run.RunID)`、`SourceKey.SessionID == Current.SessionID`、`SourceKey.Turn == Current.Turn`。跨Run、跨Session或跨Turn splice即使Identity/DomainResult各自摘要合法，也必须在Application Request和Tool command前Conflict。

Current Request不允许调用方提供`CheckedAt/ExpiresAt`。`RequestedNotAfterUnixNano == 0`表示调用方不增加观察上界；小于0为`InvalidArgument`；大于0只能缩短自然expiry，若小于或等于本次fresh now则`PreconditionFailed`，晚于natural expiry不得延长。Reader自产`CheckedUnixNano/ExpiresUnixNano`。本段V2因缺少完整Owner-current exact refs，不能作为新Reader实现依据；不得向V1 Binding/V3 Session静默加字段。新实现必须使用[Owner-current Port Delta](../port-deltas/committed-pending-action-owner-current-inputs-v2.md)冻结的Binding V2/Session V4/Current V3与精确TTL闭集。Model Projection exact Reader和历史Runtime Settlement没有TTL，不得伪造TTL参与最小值；DomainResult/Identity历史事实的NotAfter只限制新dispatch资格，不改历史真实性。

三组Validate职责固定：

- `RequestV2.Validate(now)`：验证完整Subject、full Run/Scope重算、所有exact refs/digests、RequestedNotAfter的0/负数/正数规则；`now`必须fresh且非零；
- `CurrentV2.Validate(now)`：只验证投影自身intrinsic/canonical、完整ApplicationBinding、source coordinates、Digest、`Checked < Expires`、`Checked <= now < Expires`；回拨或过期Fail Closed；
- `CurrentV2.ValidateAgainst(expected, now)`：先分别调用上述Validate，再逐字段exact比较完整Subject与Current/ApplicationBinding；`requested > 0`时强制`Current.ExpiresUnixNano <= requested`，不得仅比较汇总摘要。

Reader调用顺序固定为：fresh clock → Session S1 → Model Projection/DomainResult Fact/Runtime Settlement及其余Owner-current Readers逐项复读 → Session S2 → 返回前第二次fresh clock → seal并执行`Validate`与`ValidateAgainst`。S1/S2任一revision/digest/phase/turn/PendingAction/Binding漂移、第二次时钟早于第一次、TTL crossing或来源过期全部Fail Closed。

## 4. Canonical、Digest与版本

- Identity合同版本固定`praxis.harness.model-tool-call-pending-action-identity/v1`；
- Current合同版本固定`praxis.harness.committed-pending-action-current/v2`；
- SettledTurn schema使用新的`praxis.harness/settled-turn-result@3.0.0`，不覆盖V2；
- canonical domain分别固定，不复用Model Projection、PendingAction、Application Request或Runtime Settlement domain；
- strict JSON，拒绝未知字段、重复键、尾随文档、非canonical JSON和空白漂移；
- 不使用map或自由extension；首切面没有slice，未来集合必须新版本并定义稳定排序、去重和nil/empty规则；
- `IdentitySourceKeyV1`全部字段进入SourceKey digest；Identity ID只通过独立Identity ID domain/subject/prefix helper由该digest派生，Identity其余完整内容另进入Identity digest；
- `SettledTurnDomainResultFactV3`的FactID只通过独立Fact ID domain/subject/prefix helper由同一SourceKeyDigest派生；Content与Fact envelope分层按§3.3固定；
- Current digest包含完整`PendingActionApplicationBindingV1`、Session/PendingAction/source coordinates及Checked/Expires；
- V1对象和摘要算法完全不变。

### 4.1 兼容与迁移影响

- 本Delta只新增Identity V1、DomainResult Fact V3、GovernedSession/SessionCAS V3与Current V2；既有`GovernedSessionV2`、`SessionCASV2`、`SettledTurnResultV2`、`PendingActionV2`及Current V1字段和摘要不变；
- V2/V3虽然共享完整Run/Scope+SessionID冲突域，但没有自动迁移或双写。已有V2同键事实会使V3 Create Conflict；迁移必须由未来单独版本、显式Owner流程和验收合同处理；
- DomainResult ContentDigest的四字段body与Fact envelope分层是新V3语义，不能用旧V2 body重封；IdentityRef后派生规则禁止循环摘要；
- IdentityID与FactID的新domain/subject/prefix均为nominal标识；调用方只能通过对应helper生成和exact比较，不得依赖字符串后缀兼容；
- Harness新增对Model Invoker公开根包的只读合同依赖，不允许传递到internal、execution/direct或写接口；
- Application DTO只携neutral坐标，不获得Repository或Session写能力；测试fake不构成生产迁移、持久化或SLA声明。

## 5. 状态、CAS与恢复

```text
Model Projection sealed
-> Repository EnsureExact: Identity + PendingAction in one SettledTurnDomainResultFactV3
-> Runtime Settlement committed
-> Harness SessionCASV3 atomically applies PendingActionApplicationBindingV1
-> waiting_action + PendingAction + Identity/DomainResult/Settlement current
-> Application Request V2 S1/S2
-> Tool V2 Identity reread
```

Identity本身是create-once immutable，不定义可逆状态机。可观察阶段为`sealed -> settled -> applied_current -> expired|revoked`；后两者表示不能创建新Action，不改变历史DomainResult/Settlement。

恢复规则：

1. DomainResult `EnsureExact`回包丢失，只用同一canonical Fact重调同一个Repository capability的原子`EnsureExact`并compare exact返回；Repository的FactID、IdentityID与SourceKeyDigest三个索引必须指向同一首记录，`InspectExact`只服务事实已恢复后的下游Reader；
2. Runtime Settlement回包丢失，只Inspect exact Operation/Effect/DomainResult；
3. Harness Session CAS回包丢失，只能按完整Run/Scope+SessionID Inspect到完整`GovernedSessionV3` successor，并逐字段exact匹配`Revision`、Session `Digest`及完整`ApplicationBinding`；Binding内`PendingAction`、`IdentityRef`、`DomainResultFactRef`、`ModelTurnSettlementRef`任一字段不同都不是幂等成功。只比revision、PendingAction或Identity不足以恢复；未看到exact successor时不得重封、换Next或推进；
4. 同内容重放幂等；同ID换Projection、Call、PendingAction、TTL或digest为Conflict；
5. 64并发同内容只能线性化一个Identity/DomainResult/Session successor；64个不同内容竞争同SourceKey只能一个胜出，其余Conflict，不能通过内容派生不同ID绕过；
6. Unknown/Unavailable/Indeterminate不得换ID、重封或创建Tool Candidate。

## 6. 依赖DAG与组装

```text
Model public Projection exact Reader
  -> bound Model-turn Settlement Owner
  -> Runtime Model-turn Settlement
  -> Harness Session Apply + CommittedPendingActionReaderV2
  -> Harness-owned Application Assembler Adapter
     -> Context CTX-D10 current Reader
     -> Assembly Route/Generation/Binding current Readers
     -> Authority current Reader
  -> Application Request/InputCurrent V2
  -> Tool V2 Consumer Identity reread
  -> Tool Watermark/Candidate/Runtime Gateway
```

Harness对Model Invoker的唯一允许import根包是`github.com/Proview-China/rax/ExecutionRuntime/model-invoker`，且只使用其公开完整`ToolCallCandidateObservationRefV1`、`ToolCallCandidateObservationProjectionV1`和只读Projection Reader。禁止import或暴露`model-invoker/internal`、`execution/direct`、store、publisher、writer、event、provider及厂商类型；禁止从PendingAction payload、Harness event、compat calls或Provider Receipt反推Projection。

Application只依赖自己的public contract/ports与Runtime `core/ports`。Harness Adapter允许依赖Application public contract/ports、Model public Reader、Context public Reader及Runtime public Readers；不得依赖Application coordinator或Tool实现。Tool Adapter只依赖Application public V2 contract/ports及其本Owner合同，不import Harness实现。

Harness Assembly只验证所注入Reader/Assembler的PortSpec、Owner、Manifest、Artifact、Capability、Generation、BindingSet与Route current；不得用Slot、Hook、Factory或测试fixture绕过Identity Reader。

## 7. Tool V2消费Delta

Tool V2 consumer必须在任何Watermark、ActionCandidate或Reservation写入前：

1. 复读Application Request V2携带的neutral Identity coordinate；
2. 通过Application public Identity Current Reader取得exact current projection；
3. 再次调用Model exact Reader并验证唯一Call；
4. 验证Identity、PendingAction、Candidate payload三者摘要相等；
5. 验证Identity未过期、未撤销，S1/S2期间未漂移；
6. 把Identity exact ref纳入Tool V2 canonical command/watermark digest。

现有V1 `CanonicalArgumentsDigest == PayloadDigest`继续保留，但V1没有Identity ref和系统Assembler证明，因此不得通过Conformance或fixture冒充G6A系统闭环。未来参数转换只能新立`ModelToolCallPendingActionTransformationFact`及新版本合同；当前Identity V1不预留隐式转换开关。

## 8. 测试、fixture与系统落点

Owner-local测试分别验证Model reader、SettledTurn seal、Session CAS/current Reader、Application assembler和Tool admission。test-only cross-module fixture必须调用真实公开Ports，禁止直接`SealSingleCallToolActionRequestV2`作为系统输入。

最小系统测试唯一候选落点：

```text
ExecutionRuntime/tool-mcp/tests/system/g6a_identity_v1_test.go
```

该测试只手工注入公共Reader/Port/Fake，证明G6A组合；不创建production root、不注册Capability、不调用Context Refresh/Continuation、不推进Turn。完整反例见[测试矩阵](./model-tool-call-pending-action-identity-v1-test-matrix.md)。

## 9. 明确非产物

- 不写Go、不实现Store/Backend/RPC/SLA；
- 不修改Runtime、Model、Tool或Context公共合同；
- 不启用Provider能力；
- 不进入G6B、Checkpoint或N>1；
- 不把Runtime Settlement与Harness Session CAS宣传为跨Owner原子事务；
- 不实现Transformation Fact。
