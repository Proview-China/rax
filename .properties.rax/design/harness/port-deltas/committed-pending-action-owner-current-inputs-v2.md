# Port Delta：Committed PendingAction Owner-current exact输入

状态：Port Delta既有Owner-current V3/V4部分、additive public `SessionCurrentReaderV4`及Harness P3 Assembler/InputCurrent Reader均完成实现并通过对应独立设计/代码终审，P3最终代码审计为`YES(P0/P1/P2=0)`。Tool Consumer/P4、system G6A/G6B/production root仍未完成并保持`NO-GO`。

本Delta不修改`PendingActionApplicationBindingV1`、`GovernedSessionV3`、`CommittedPendingActionSubjectV2/RequestV2/CurrentV2/ReaderV2`，不迁移既有Session，不创建Harness私有跨Owner Reader，不把Action Operation创建后的Intent/Permit/Review/dispatch Policy倒灌到pre-Action Session。

## 1. 用例、Owner与唯一缺口

新版Current Reader必须固定执行：Request V3 Validate → Session S1 V4 → Candidate → DomainResult/Identity → Model Projection → Runtime Settlement → Association → Generation → Route → role-tagged Provider Binding闭集 → Context → Session S2 V4 → fresh clock → min expiry → seal Current V3。

live Owner-current Readers基本存在；缺口是旧四字段Binding/Session/Subject没有携带调用Reader所需的exact refs，且Runtime V3 Settlement当前只以含`Settle`的Governance Port公开。仅凭digest、BindingSet ID、map扫描或sidecar不能恢复完整输入。

- Model Owner：只拥有Projection Observation及公开exact Reader；
- 绑定的Model-turn `SettlementOwner`：拥有Identity与SettledTurn DomainResult的语义及持久Fact；
- Harness Owner：拥有Identity/DomainResult公共类型、Run-local Session、PendingAction、Session CAS和Current投影；**不拥有Identity/DomainResult Fact语义或持久化**；
- Runtime Owner：拥有Operation Settlement、Generation/Binding/Route current和中立Applicability Port；
- Context Owner：通过Runtime中立Applicability Ref提供Frame current；Harness不复制Context类型；
- Application Owner：只在Action Operation创建后协调Authority/Policy/Review/Permit等后续S1/S2，本Delta不实现Application。

## 2. 完整版本闭包

### 2.1 exact常量

| 对象 / 常量 | ContractVersion | Canonical domain | Canonical subject |
|---|---|---|---|
| `CommittedPendingActionOwnerCurrentInputsV1` / `CommittedPendingActionOwnerCurrentInputsContractVersionV1` | `praxis.harness.committed-pending-action-owner-current-inputs/v1` | `praxis.harness.committed-pending-action-owner-current-inputs` | `CommittedPendingActionOwnerCurrentInputsV1` |
| `PendingActionApplicationBindingV2` / `PendingActionApplicationBindingContractVersionV2` | `praxis.harness.pending-action-application-binding/v2` | `praxis.harness.pending-action-application-binding` | `PendingActionApplicationBindingV2` |
| `GovernedSessionV4` / `GovernedContractVersionV4` | `praxis.harness.governed/v4` | `praxis.harness.governed-session-v4` | `GovernedSessionV4` |
| `SessionCASRequestV4` / `SessionCASContractVersionV4` | `praxis.harness.session-cas/v4` | `praxis.harness.session-cas-request-v4` | `SessionCASRequestV4` |
| `CommittedPendingActionCurrentV3` / `CommittedPendingActionCurrentContractVersionV3` | `praxis.harness.committed-pending-action-current/v3` | `praxis.harness.committed-pending-action-current` | `CommittedPendingActionCurrentV3` |

每个Digest均使用上表指定的domain/version/subject，覆盖对象全部字段并只把自身`Digest`置空；禁止复用其他对象的domain/subject、只比汇总digest或把一个对象Digest当另一对象canonical body。Current V3沿用Current家族domain但以V3 version/subject隔离；GovernedSession V4按裁决使用独立V4 domain。`CommittedPendingActionSubjectV3`、`CommittedPendingActionCurrentRequestV3`与`CommittedPendingActionReaderV3`不另带`ContractVersion/Digest`，不定义独立canonical domain；它们是Current V3合同内的closed nominal类型，必须按显式字段逐项Validate，不能以V2结构兼容或摘要替代。

### 2.2 exact字段

```text
CommittedPendingActionOwnerCurrentInputsV1 {
  ContractVersion               string                                                   `json:"contract_version"`
  ModelTurnOperation            runtimeports.OperationSubjectV3                          `json:"model_turn_operation"`
  GenerationBindingAssociation runtimeports.GenerationBindingAssociationRefV1           `json:"generation_binding_association"`
  RouteCurrent                  runtimeports.ControlledOperationProviderRouteCurrentRefV2 `json:"route_current"`
  RouteMatrix                   runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3 `json:"route_matrix"`
  ContextApplicability          runtimeports.OperationScopeEvidenceApplicabilityFactRefV3 `json:"context_applicability"`
  Digest                        core.Digest                                               `json:"digest"`
}

PendingActionApplicationBindingV2 {
  ContractVersion    string                                  `json:"contract_version"`
  Base               PendingActionApplicationBindingV1       `json:"base"`
  OwnerCurrentInputs CommittedPendingActionOwnerCurrentInputsV1 `json:"owner_current_inputs"`
  Digest             core.Digest                             `json:"digest"`
}
```

`BindingV2.Base`是V1四字段的唯一真值：`PendingAction/IdentityRef/DomainResultFactRef/ModelTurnSettlementRef`。不得在V2顶层再次复制四字段；V2 Validate先调用`Base.Validate()`，再核`OwnerCurrentInputs`与Base/Session exact关系。

`CommittedPendingActionOwnerCurrentInputsV1.Validate()`除调用每个公共Ref/Subject/Matrix的intrinsic Validate外，必须在任何Context Reader调用前要求`ContextApplicability.Kind == runtimeports.OperationScopeEvidenceContextParentKindV3`，即exact字符串`praxis.context/parent-frame-current-v1`。其他合法namespaced Kind，包括run/session/turn/action applicability或自定义Kind，一律Fail Closed；不得把Kind路由责任延迟给Context adapter。

```text
GovernedSessionV4 {
  ContractVersion        `json:"contract_version"`
  ID                     `json:"session_id"`
  Revision               `json:"revision"`
  Run                    `json:"run"`
  Endpoint               `json:"endpoint"`
  Phase                  `json:"phase"`
  Turn                   `json:"turn"`
  Candidate              `json:"candidate,omitempty"`
  DomainReservation      `json:"domain_reservation,omitempty"`
  Execution              `json:"execution,omitempty"`
  PendingAction          `json:"pending_action,omitempty"`
  ApplicationBinding *PendingActionApplicationBindingV2 `json:"application_binding,omitempty"`
  PendingInput           `json:"pending_input,omitempty"`
  UndispatchedSettlement `json:"undispatched_settlement,omitempty"`
  CompletionClaim        `json:"completion_claim,omitempty"`
  CreatedUnixNano        `json:"created_unix_nano"`
  UpdatedUnixNano        `json:"updated_unix_nano"`
  Digest                 `json:"digest"`
}
```

V4镜像V3全部公共JSON字段，但把原`application_binding: *PendingActionApplicationBindingV1`**替换**成唯一`*PendingActionApplicationBindingV2`；不存在第二个V1 Binding字段或sidecar。`waiting_action`必须有且仅有V2 Binding，其他phase必须nil。V4与V2/V3共享完整Run/Scope+SessionID冲突域，禁止并存、翻译、默认升级或双写。

```text
SessionCASRequestV4 {
  ContractVersion  string            `json:"contract_version"`
  Run              RunRef            `json:"run"`
  SessionID        string            `json:"session_id"`
  ExpectedRevision core.Revision     `json:"expected_revision"`
  ExpectedDigest   core.Digest       `json:"expected_digest"`
  Next             GovernedSessionV4 `json:"next"`
  Digest           core.Digest       `json:"digest"`
}

SessionCurrentReaderV4 {
  InspectSessionV4(context.Context, RunRef, string) (GovernedSessionV4, error)
}

SessionFactPortV4 {
  SessionCurrentReaderV4
  CreateSessionV4(context.Context, GovernedSessionV4) (GovernedSessionV4, error)
  CompareAndSwapSessionV4(context.Context, SessionCASRequestV4) (GovernedSessionV4, error)
}
```

`SessionCASRequestV4.Digest`使用上表V4 CAS domain/version/subject覆盖`ContractVersion/Run/SessionID/ExpectedRevision/ExpectedDigest/Next`完整字段并只把自身Digest置空。`SessionCurrentReaderV4`只公开既有exact Inspect方法；`SessionFactPortV4`兼容嵌入该Reader并保留既有Create/CAS，因此展开方法集不变、现有实现天然结构兼容。该加法不修改`GovernedSessionV4`、CAS对象、canonical/digest、Store、冲突域或方法语义，也不拆分V4 Session写Owner。V2/V3/V4 Store仍须在同一实例、同一线性点共享完整Run/Scope+SessionID key冲突域。

P3 `single_call_tool_action_assembler_v2`构造器的静态参数类型必须精确为Harness public `SessionCurrentReaderV4`，禁止接收`SessionFactPortV4`、Store/fake具体类型或私有同形接口。调用方可以注入一个同时实现FactPort的对象，但Assembler字段与方法集只能看见`InspectSessionV4`，无法调用Create/CAS。nil或typed-nil Reader必须在任何下游读取及Request Seal前返回`Unavailable/ComponentMissing`，不得panic。

- Create仅接受`revision=1 + phase=creating + ApplicationBinding=nil`；absent时create，same canonical幂等，同key任一字段不同`Conflict`。Create回包丢失只可用同Run/Scope+SessionID执行V4 exact Inspect，完整结果等于原输入才恢复。
- CAS要求ExpectedRevision/ExpectedDigest与current exact、`Next.Revision=ExpectedRevision+1`且Run/Scope/SessionID不变；V4继承V3全部既有合法transition与本地current→Next lineage规则。`waiting_settlement→waiting_action`必须一次写完整Binding V2，不能部分写Base或OwnerCurrentInputs。
- CAS正常返回或回包丢失后的Inspect均必须与expected `Next GovernedSessionV4`逐字段及canonical exact；只同revision/digest前缀、合法但不是该successor、或Binding Base/OwnerCurrentInputs任一漂移均不得恢复为成功。

```text
CommittedPendingActionSubjectV3 {
  Base               CommittedPendingActionSubjectV2   `json:"base"`
  ApplicationBinding PendingActionApplicationBindingV2 `json:"application_binding"`
}

CommittedPendingActionCurrentRequestV3 {
  Subject                   CommittedPendingActionSubjectV3 `json:"subject"`
  RequestedNotAfterUnixNano int64                           `json:"requested_not_after_unix_nano"`
}
```

`SubjectV3.Base`保持旧Subject V2的无`ContractVersion`语义；Subject V3逐字段验证Base与Binding V2中的PendingAction/Identity/DomainResult/Settlement完全相等，不以digest替代字段比较。

```text
CommittedPendingActionCurrentV3 {
  ContractVersion      `json:"contract_version"`
  Run                  `json:"run"`
  ExecutionScopeDigest `json:"execution_scope_digest"`
  SessionID            `json:"session_id"`
  SessionRevision      `json:"session_revision"`
  SessionDigest        `json:"session_digest"`
  Phase                `json:"phase"`
  Turn                 `json:"turn"`
  PendingAction        `json:"pending_action"`
  ApplicationBinding PendingActionApplicationBindingV2 `json:"application_binding"`
  SessionApplicability `json:"session_applicability"`
  TurnApplicability    `json:"turn_applicability"`
  CheckedUnixNano      `json:"checked_unix_nano"`
  ExpiresUnixNano      `json:"expires_unix_nano"`
  Digest               `json:"digest"`
}

CommittedPendingActionReaderV3 {
  InspectCommittedPendingActionCurrentV3(
    context.Context,
    CommittedPendingActionCurrentRequestV3,
  ) (CommittedPendingActionCurrentV3, error)
}

func (CommittedPendingActionCurrentRequestV3) Validate(now time.Time) error
func (CommittedPendingActionCurrentV3) Validate(now time.Time) error
func (CommittedPendingActionCurrentV3) ValidateAgainst(
  expected CommittedPendingActionCurrentRequestV3,
  now time.Time,
) error
```

Current V3相对Current V2只additive承载Binding V2语义，但使用独立V3类型/version/domain/subject。**Reader V2不得返回、嵌入、type-pun或默认转换含Binding V2的Current；V2 Current仍只接受Binding V1。**

三组方法职责精确为：

- `RequestV3.Validate(now)`：`now`必须fresh且非零；验证完整Subject V3、Base Subject V2、Binding V2及其exact关系。`RequestedNotAfterUnixNano < 0`为`InvalidArgument`，`==0`表示调用方不增加上界，`>0`只能缩短且`<=now`为`PreconditionFailed`；不得借正值延长natural expiry。
- `CurrentV3.Validate(now)`：验证自身version/canonical、完整Binding V2、Session/Turn applicability、`Checked < Expires`及`Checked <= now < Expires`；不读取外部Owner。
- `CurrentV3.ValidateAgainst(expected, now)`：先调用`expected.Validate(now)`和`CurrentV3.Validate(now)`，再逐字段exact比较完整Subject V3与Current/Binding V2；canonical digest只能辅助检测，不能替代Run/Scope、Session、Turn、PendingAction、Base四字段和OwnerCurrentInputs的closed逐字段比较。`requested>0`时还须`Expires<=requested`。

所有新增value object必须提供深拷贝与Seal边界：`CommittedPendingActionOwnerCurrentInputsV1.Clone/Seal`、`PendingActionApplicationBindingV2.Clone/Seal`、`GovernedSessionV4.Clone/Seal`、`SessionCASRequestV4.Clone/Seal`、`CommittedPendingActionSubjectV3.Clone`、`CommittedPendingActionCurrentRequestV3.Clone`、`CommittedPendingActionCurrentV3.Clone/Seal`。Seal必须先deep clone全部指针/slice/ExecutionScope/SandboxLease/Settlement Evidence/Delegation/DomainResultSchema/PendingAction payload，再重算本对象Digest并Validate；调用方后改输入或返回值不得污染Store/Reader。Reader V3的任一注入依赖即使接口值非nil但底层为typed nil，也必须返回`Unavailable/ComponentMissing`而不是panic。

## 3. exact输入、产生时点与公开Reader

| 输入 | 产生时点 / Owner | CAS前存在 | 既有字段能否无损取得 | Reader / TTL |
|---|---|---:|---|---|
| Candidate full Ref/Fact | Harness Candidate创建 | 是 | `BindingV2.Base.PendingAction.SourceCandidate` | `CandidateFactPortV2.InspectCandidateV2`；`ExpiresUnixNano` |
| DomainResult/Identity | SettlementOwner提交 | 是 | `BindingV2.Base.DomainResultFactRef/IdentityRef` | `SettledTurnDomainResultReaderV3`；Identity `NotAfterUnixNano` |
| Model Projection | Model Owner完成 | 是 | 复读DomainResult/Identity后取得full Ref | Model根包public exact Reader；无TTL |
| Model-turn Operation Subject | Runtime Operation创建 | 是 | **旧Binding不可派生**；由`OwnerInputs.ModelTurnOperation`携完整值 | 用于Runtime Settlement Inspect与Context scope exact；无独立TTL |
| Runtime Settlement | Runtime Owner提交 | 是 | Base已有Settlement Ref和EffectID，但OperationDigest不可逆 | 新增Runtime窄只读Reader，历史Settlement无TTL |
| Generation-Binding Association Ref | Runtime Assembly/Activation | 是 | **旧Session不可派生**；OwnerInputs携full Ref | Gateway按ID读，返回Fact含fresh `ExpiresUnixNano` |
| Generation | Association Candidate | 是 | Association exact Fact的`Candidate.Generation.Generation`取得`GenerationArtifactRefV1` | `GenerationCurrentReaderV1(GenerationArtifactRefV1)`；返回完整Projection，`ExpiresUnixNano` |
| Route Ref + Matrix | Harness Assembly/Runtime route current | 是 | **旧Session不可派生**；OwnerInputs分别携完整Ref和Key | `ControlledOperationProviderRouteCurrentReaderV2(ref,matrix)`；`ExpiresUnixNano` |
| Context applicability | Context source经Runtime中立投影 | 是 | **旧Session不可派生**；OwnerInputs只携Runtime中立Ref | `OperationScopeEvidenceApplicabilityCurrentReaderV3`；`ExpiresUnixNano` |
| Authority/Policy/Review/Permit | Action Operation创建后 | 否 | 不适用，禁止倒灌 | Application V2后续编排；不属于Current V3 natural expiry |

## 4. 公共Port Delta：Runtime Settlement窄只读能力

Runtime当前只公开：

```go
type OperationSettlementGovernancePortV3 interface {
    SettleOperationEffectV3(...)
    InspectOperationSettlementV3(context.Context, OperationSubjectV3, core.EffectIntentID) (OperationSettlementRefV3, error)
}
```

Harness Current Reader不应获得`Settle`能力。需要Runtime Owner additive公开：

```go
type OperationSettlementCurrentReaderV3 interface {
    InspectOperationSettlementV3(
        context.Context,
        OperationSubjectV3,
        core.EffectIntentID,
    ) (OperationSettlementRefV3, error)
}

type OperationSettlementGovernancePortV3 interface {
    OperationSettlementCurrentReaderV3
    SettleOperationEffectV3(...)
}
```

这是Runtime public ports的能力收窄加法：方法集、Operation Settlement对象与canonical/digest均不变；既有Governance实现因已有同签名Inspect而天然结构兼容。Harness Current Reader构造器参数必须精确为Runtime公开的`runtimeports.OperationSettlementCurrentReaderV3`，不得接受`OperationSettlementGovernancePortV3`，也不得在Harness `contract/ports/kernel/adapter`自建同形跨Owner接口。Application后续因需要提交Settlement可持有Runtime公开Governance Port，但不得把该宽能力传入Harness Current Reader。Inspect返回后必须`Validate()`并与Binding V2 Base中的完整Settlement Ref逐字段exact；Unavailable/Unknown不得调用Settle或换ID重建。本轮只形成Runtime additive Port需求，不修改Runtime/Harness Go。

### 4.1 Harness Session V4窄只读能力候选

```go
type SessionCurrentReaderV4 interface {
    InspectSessionV4(context.Context, contract.RunRef, string) (contract.GovernedSessionV4, error)
}

type SessionFactPortV4 interface {
    SessionCurrentReaderV4
    CreateSessionV4(context.Context, contract.GovernedSessionV4) (contract.GovernedSessionV4, error)
    CompareAndSwapSessionV4(context.Context, contract.SessionCASRequestV4) (contract.GovernedSessionV4, error)
}
```

- Reader实现只需实现`InspectSessionV4`，不需要Create/CAS；现有FactPort实现因展开方法集未变而天然兼容；
- P3 Assembler只能import并接收Harness public `SessionCurrentReaderV4`，不得定义私有跨Owner接口，也不得通过类型断言、反射或保存宽接口恢复写能力；
- Reader只返回既有`GovernedSessionV4` exact clone，不创建Fact、Authority或current摘要；
- 该additive Port已获独立设计短审YES并完成Harness最小实现；Application V2 public contract/ports完整落盘并compile前，不实现P3 Go。

## 5. 固定S1/S2顺序与逐项不变量

1. fresh `nowS1`；`RequestV3.Validate(nowS1)`验证完整Subject V3、RequestedNotAfter规则和Binding V2；
2. Session S1读取完整`GovernedSessionV4`，核Run/Scope/Session/Revision/Digest/phase/turn/PendingAction/Binding V2 exact；P3 Assembler只通过`SessionCurrentReaderV4`执行该Inspect，不接收`SessionFactPortV4`；
3. 复读Candidate；Candidate full Ref、Run/Scope/Session/Turn与PendingAction/Identity SourceKey exact；
4. 复读DomainResult/Identity；Fact Owner是SettlementOwner，Harness只验证类型、Ref、四字段body及Session关系；
5. 以full Ref复读Model Projection，要求`Calls==1`并与Identity canonical arguments逐字段exact；
6. 以`OwnerInputs.ModelTurnOperation + Base.ModelTurnSettlementRef.Attempt.EffectID`调用Runtime `OperationSettlementCurrentReaderV3`，返回Ref与Base exact；
7. Association Gateway以`OwnerInputs.GenerationBindingAssociation.ID`读取，内部复读Generation/完整BindingSet/Activation；调用方仍须`Fact.Validate()`、state=`active`、`nowS1 < ExpiresUnixNano`、`Fact.RefV1()==expected`后才能使用Candidate；
8. 额外调用`GenerationCurrentReaderV1(Candidate.Generation.Generation)`只作exact current交叉；入参必须是嵌套`GenerationArtifactRefV1`，不得把`GenerationCurrentProjectionV1`整体传入或自造Ref。返回Projection再与完整`Candidate.Generation`逐字段、digest及current exact，并取真实expiry。Association Gateway已内部复读Activation，因此不再额外调用`GenerationActivationCurrentReaderV1`；Association expiry已经是Generation/BindingSet/Activation的最小值；
9. `RouteMatrix.Validate()`且逐字段exact等于live `OperationScopeEvidenceActionMatrixV3()`：`run / praxis.tool/execute / praxis.tool/single-call-action-v1`；以`(RouteCurrent, RouteMatrix)`调用Route Reader，并验证返回Projection的Ref、matrix digest、完整Route/BindingSet字段；
10. Association `Candidate.Binding`是单个BindingSet current projection，不是Provider成员集合。调用`ValidateCurrent(expected set ID/revision, nowS1)`，并与Route Projection的`BindingSetID/Revision/Digest/SemanticDigest/CurrentnessDigest`逐字段exact；其expiry由Association Fact已纳入，但仍不得漏验；
11. 构造role-tagged Provider Binding闭集，来源**仅**：
    - `session_endpoint`：Session S1 `Endpoint.Binding`；
    - `candidate_provider`：Candidate `Provider`；
    - `identity_settlement_owner`：Identity `SettlementOwner`；
    - Route Projection七角色：`tool_adapter/gateway/provider_transport/prepared_reader/boundary_reader/provider_inspect/provider`。
12. 每个role的`ProviderBindingRefV2`必须`Validate()`且`BindingSetID/Revision`等于Association Candidate.Binding exact set；按完整Ref canonical排序，以完整Ref exact重复时合并为一个read并保留排序后的role集合。禁止扫描、按BindingSet猜成员、加入额外binding或依赖map顺序；每个唯一Ref调用`ProviderBindingCurrentnessPortV2`，要求Projection `Ref` exact、active/fresh，并把`Projection.BindingSetDigest`与`Association Candidate.Binding.BindingSetDigest`、`Projection.BindingSetSemanticDigest`与`Association Candidate.Binding.BindingSetSemanticDigest`分别exact交叉；BindingSet ID/revision仍从Projection.Ref逐字段校验。全部通过后才取expiry；
13. 调用Context Reader前再次要求`BindingV2.OwnerCurrentInputs.ContextApplicability.Kind == runtimeports.OperationScopeEvidenceContextParentKindV3`；wrong-kind必须在零Reader调用处Fail Closed。Reader返回后调用`projection.Validate(BindingV2.OwnerCurrentInputs.ContextApplicability, BindingV2.OwnerCurrentInputs.ModelTurnOperation.ExecutionScopeDigest, nowS1)`。ModelTurnOperation必须`Kind=run`，其`RunID/ExecutionScope/ExecutionScopeDigest`与Session Run/Scope逐字段exact；
14. Context Owner adapter还必须复读并验证完整Run/Session/Turn/Frame sealed subject；只看Runtime中立Ref、`Current=true`或Reader自报scope不合格；
15. Session S2再次读取完整V4，逐字段比较S1/S2完整Session与Binding V2，digest不能替代字段比较；
16. fresh `nowS2`且`nowS2>=nowS1`；
17. `Expires=min(nowS1+30s, Candidate.Expires, Identity.NotAfter, Association.Expires, Generation.Expires, 每个role Provider Binding Projection.Expires, Route.Expires, Context.Expires, caller requested bound)`；任一TTL crossing/Unavailable/漂移Fail Closed；
18. Model Projection和历史Runtime Settlement没有TTL，禁止伪造并加入min；seal Current V3并分别执行intrinsic Validate与ValidateAgainst Request V3。

## 6. Effect、Recovery与硬反例

全部调用只读，零Candidate/Evidence/Settlement/Session写。Unavailable/Unknown只能重新执行完整S1→all Readers→S2；不得缓存、跳读、换ref、调用Settlement Settle或创建sidecar。

1. OwnerInputs/Binding V2任一ContractVersion、字段或Digest缺失/漂移；
2. V2 Binding顶层复制Base四字段形成双真值，或Session V4同时保存V1/V2 Binding；
3. Reader V2返回含Binding V2的Current V2，或V3/V4对象默认转换成旧版本；
4. RouteMatrix缺字段、只携MatrixDigest、与Action Matrix常量不等，或调用Reader时用常量替换Binding值；
5. Route Reader返回另一Ref/MatrixDigest/Projection/BindingSet字段；
6. ModelTurnOperation不是run，或RunID/ExecutionScopeDigest/完整Scope与Session不同；
7. ContextApplicability使用run/session/turn/action或任意其他合法namespaced Kind，或Ref正确但scope不同，或Context adapter未复读Run/Session/Turn/Frame sealed subject；wrong-kind必须零Context Reader调用；
8. role闭集漏掉Endpoint/Candidate/Identity/Route任一角色，加入无公开来源成员，按BindingSet扫描，或未保留duplicate ref的全部role；
9. 任一role binding不属于Association exact BindingSet ID/revision，Provider current Projection虽Ref相同但BindingSetDigest/SemanticDigest被splice，或BindingSet五字段在Association与Route间漂移；
10. Association只按ID成功便使用，未Fact.Validate/Ref exact/active/fresh，向Generation Reader误传整个`Candidate.Generation` projection，或返回Generation与完整Candidate.Generation字段/digest/current漂移；
11. 重复复读Activation并使用错误run OperationSubject，或静默把Activation TTL从Association min中删除；
12. Harness构造器接收Runtime Governance Port而获得Settle能力、以宽接口变量/adapter绕过窄能力、import非Runtime公开Port，或自建私有Settlement Reader；
13. Runtime Settlement返回同ID但revision/digest/Attempt/Observation/DomainResult不同；
14. 把Identity/DomainResult持久Fact Owner写成Harness，而非绑定的SettlementOwner；
15. 把Model Projection/历史Settlement伪造TTL，或漏掉Candidate/Identity/Association/Generation/任一role Binding/Route/Context expiry；
16. S1后任一Owner漂移但Session不变，Session S2漂移但Owner结果相同，`nowS2<nowS1`，或caller bound延长natural expiry；
17. V3/V4 Session占用同一Run/Scope+SessionID key，CAS V4复用V3 domain/subject，或Create接受非rev1/creating/非nil Binding；
18. V4 CAS回包丢失后Inspect到合法但非expected successor，或完整Binding Base/OwnerCurrentInputs任一字段漂移仍当幂等成功；
19. Request V3把`RequestedNotAfter==0`当过期、接受负数/`<=now`正数、用正数延长natural expiry，或ValidateAgainst只比digest不比完整Subject/Binding；
20. 新对象Clone/Seal保留PendingAction payload、ExecutionScope/SandboxLease、Settlement Evidence/Delegation/DomainResultSchema等别名，或Reader依赖typed nil导致panic；
21. 用短名alias、sidecar、mutable registry、可逆ID、裸string pair或全局扫描补齐`CommittedPendingActionOwnerCurrentInputsV1`。
22. P3构造器接收`SessionFactPortV4`、Store/fake具体类型或私有同形Reader，因而可见Create/CAS；
23. 只实现`InspectSessionV4`的合法Reader因缺Create/CAS无法通过compile/conformance，或现有FactPort因接口嵌入被误判不兼容；
24. `SessionCurrentReaderV4`为nil/typed nil仍进入Session/Fact/Model/Runtime读取或Request Seal，或Assembler通过类型断言/反射恢复写方法。

任一反例均Fail Closed，零Current V3、零Application Request、零Tool写。

## 7. 兼容、迁移与验收影响

- V1 Binding、Session V3、Subject/Request/Current/Reader V2保持历史只读兼容，不迁移、不翻译；
- `SessionFactPortV4`兼容嵌入候选`SessionCurrentReaderV4`，展开方法集不变；FactPort与V2/V3 Store仍在同一实例共享Session冲突域，V4 Create/CAS/lost-reply均使用exact V4对象，只有新CAS一次写完整Binding V2后，Reader V3才可读取；
- Current V3是新公共合同，不扩展Reader V2返回类型；调用方必须显式选择V3；
- Runtime仅新增窄只读`OperationSettlementCurrentReaderV3`类型，Governance Port兼容嵌入后方法集不变，既有实现天然兼容；Harness构造器只接受窄Reader，Application后续可持Governance Port；本轮不修改Runtime；
- Authority/Policy/Review/Permit仍在Application V2后续S1/S2，不进入Binding V2或Current V3 TTL；
- test fixture只能显式注入公共Reader子集；当前无production composition root，不注册/激活Capability；
- 既有Owner-current Delta、对应V3/V4实现、`SessionCurrentReaderV4`及Harness P3 Assembler/InputCurrent Reader均已实现并通过独立终审；P3 typed-nil、nil receiver/context、S1/S2租约收窄、RequestedNotAfter三态与deep-copy反例均已通过。该YES不构成Tool Consumer/P4、system G6A/G6B或production root通过。
