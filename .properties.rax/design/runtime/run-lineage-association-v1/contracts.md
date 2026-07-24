# Run Lineage Association V1 精确合同

状态：**已吸收首轮独立审计`NO（P0=1/P1=2/P2=0）`并完成最小资产返修；READY等待不同agent复审，不自标YES；implementation与production均NO-GO**。

## 1. 版本、canonical与边界

```go
const RunLineageAssociationContractVersionV1 = "1.0.0"
const RunLineageAssociationCanonicalDomainV1 = "praxis.runtime.run-lineage-association"
const MaxRunLineageCurrentProjectionTTLV1 = 15 * time.Second
const MaxRunLineageReadRecoveryTimeoutV1 = 2 * time.Second
```

canonical discriminator闭表：

```text
RunLifecycleRecordExactRefV1
RunLineageAssociationSubjectV1
RunLineageAssociationIDV1
RunLineageAssociationFactV1
RunLineageAssociationCurrentIndexIDV1
RunLineageAssociationCurrentIndexV1
RunLineageAssociationCurrentProjectionV1
RunLineageAssociationTerminalProjectionV1
RunLineageAssociationTerminalRequestV1
RunLineageCreateReceiptV1
```

不得使用map、`any`、caller自报digest、Review/Harness DTO或字符串拼接替代具名canonical输入。所有ID使用`core.CanonicalJSONDigest`完整`sha256:<64hex>`值，不截断、不加时间/TTL/revision。

## 2. Run exact ref

```go
type RunLifecycleRecordExactRefV1 struct {
    RunID                core.AgentRunID        `json:"run_id"`
    ExecutionScope       core.ExecutionScope    `json:"execution_scope"`
    RunIdentityDigest    core.Digest            `json:"run_identity_digest"`
    ExecutionScopeDigest core.Digest            `json:"execution_scope_digest"`
    SessionRef           string                 `json:"session_ref,omitempty"`
    RunRevision          core.Revision          `json:"run_revision"`
    Phase                RunLifecyclePhaseV3    `json:"phase"`
    RecordDigest         core.Digest            `json:"record_digest"`
}
```

派生函数固定为：

```go
func DeriveRunLifecycleRecordExactRefV1(
    envelope RunLifecycleEnvelopeV3,
) (RunLifecycleRecordExactRefV1, error)

func (r RunLifecycleRecordExactRefV1) ValidateAgainstV1(
    envelope RunLifecycleEnvelopeV3,
) error
```

`ExecutionScope`与`SessionRef`逐字段复制`envelope.Run`，使现有`InspectRunLifecycleV3(ctx, scope, runID)`可以直接使用exact ref而不依赖私有resolver。`RunIdentityDigest`复用live `RunIdentityDigestV2(envelope.Run)`；`ExecutionScopeDigest`复用live `ExecutionScopeDigestV2(envelope.Run.Scope)`；`RecordDigest`为：

```text
CanonicalJSONDigest(
  "praxis.runtime.run-lineage-association",
  "1.0.0",
  "RunLifecycleRecordExactRefV1",
  envelope.Run,
)
```

Ref不复制EffectIndex、Closure、Decision、Progress或Report为第二事实；public current projection会包含完整`RunLifecycleEnvelopeV3`并以projection digest覆盖这些sidecar。Ref只在Run record revision或V3 lifecycle phase变化时改变。

`ValidateAgainstV1`必须重算Scope/Run identity/Record digest并逐字段核对RunID、ExecutionScope、SessionRef、revision和phase；不能只比三个digest。对parent current：当`RunRevision == ParentAnchor.RunRevision`时，派生出的full exact Ref必须与ParentAnchor逐字段相等，same revision换RecordDigest、Phase、SessionRef或任一scope字段均为Conflict；只有更高revision才允许继续，并必须从新envelope重新派生RecordDigest/phase，验证identity/scope/session保持exact且phase可达不回退，禁止复制anchor digest或由caller填phase。对child current，无论revision是否变化，派生full Ref都必须逐字段等于同revision Association.ChildCurrent。Clone必须deep-copy `ExecutionScope.SandboxLease`。

## 3. 稳定subject与ID

```go
type RunLineageAssociationSubjectV1 struct {
    TenantID                  core.TenantID   `json:"tenant_id"`
    ParentRunID               core.AgentRunID `json:"parent_run_id"`
    ParentRunIdentityDigest   core.Digest     `json:"parent_run_identity_digest"`
    ParentExecutionScopeDigest core.Digest    `json:"parent_execution_scope_digest"`
    ChildRunID                core.AgentRunID `json:"child_run_id"`
    ChildRunIdentityDigest    core.Digest     `json:"child_run_identity_digest"`
    ChildExecutionScopeDigest core.Digest     `json:"child_execution_scope_digest"`
}
```

`TenantID`必须同时等于parent与child `ExecutionScope.Identity.TenantID`；parent/child Run ID必须不同。合同不强制相同Identity/Lineage/Instance，也不因此授予跨identity/scope执行资格；具体Review隔离/placement由Plan、Policy、Authority与宿主root另行验证。

```go
type RunLineageAssociationIDInputV1 struct {
    Subject RunLineageAssociationSubjectV1 `json:"subject"`
}
```

```text
SubjectDigest = CanonicalJSONDigest(domain, version,
  "RunLineageAssociationSubjectV1", Subject)

AssociationID = CanonicalJSONDigest(domain, version,
  "RunLineageAssociationIDV1",
  RunLineageAssociationIDInputV1{Subject})
```

TTL、phase、revision、Checked、caller purpose、Review Case/Target与Harness source都不得进入稳定ID。

## 4. Association history fact

```go
type RunLineageAssociationStateV1 string

const (
    RunLineageAssociationActiveV1          RunLineageAssociationStateV1 = "active"
    RunLineageAssociationTerminalCleanupV1 RunLineageAssociationStateV1 = "terminal_cleanup"
    RunLineageAssociationTerminalClosedV1  RunLineageAssociationStateV1 = "terminal_closed"
)

type RunLineageAssociationRefV1 struct {
    ID            string                      `json:"id"`
    Revision      core.Revision               `json:"revision"`
    SubjectDigest core.Digest                 `json:"subject_digest"`
    State         RunLineageAssociationStateV1 `json:"state"`
    Digest        core.Digest                 `json:"digest"`
}

type RunLineageAssociationFactV1 struct {
    ContractVersion string                       `json:"contract_version"`
    ID              string                       `json:"id"`
    Revision        core.Revision                `json:"revision"`
    Subject         RunLineageAssociationSubjectV1 `json:"subject"`
    SubjectDigest   core.Digest                  `json:"subject_digest"`
    ParentAnchor    RunLifecycleRecordExactRefV1 `json:"parent_anchor"`
    ChildCurrent    RunLifecycleRecordExactRefV1 `json:"child_current"`
    State           RunLineageAssociationStateV1 `json:"state"`
    CreatedUnixNano int64                        `json:"created_unix_nano"`
    UpdatedUnixNano int64                        `json:"updated_unix_nano"`
    NotAfterUnixNano int64                       `json:"not_after_unix_nano"`
    Digest          core.Digest                  `json:"digest"`
}
```

`Fact.Digest`清自身后覆盖其余全部字段。`Ref()`逐字段复制ID/Revision/SubjectDigest/State/Digest。rev1要求parent anchor为create事务内复读到的exact lifecycle，child为`pending_prepared`、Run revision 1；`Created==Updated`且`Created < NotAfter`。

后续revision必须：

- `ID/Subject/SubjectDigest/ParentAnchor/Created/NotAfter`完全不变；
- revision严格为旧revision `+1`；
- `Updated`不回退；
- `ChildCurrent`与同一事务后的child V3 lifecycle exact匹配；
- phase只能按V3 reachable顺序前进；
- state按child phase唯一映射：pending/running/stopping→active，terminal_cleanup→terminal_cleanup，termination_closed→terminal_closed；
- terminal_closed后不得再发布revision；
- 纯时间到期不发布revision，历史Fact仍可exact读取。

## 5. history、highest与current index

```go
type RunLineageAssociationCurrentIndexV1 struct {
    ContractVersion string                         `json:"contract_version"`
    IndexID         string                         `json:"index_id"`
    Revision        core.Revision                  `json:"revision"`
    SubjectDigest   core.Digest                    `json:"subject_digest"`
    HighestRevision core.Revision                  `json:"highest_revision"`
    Previous        *RunLineageAssociationRefV1    `json:"previous,omitempty"`
    Current         RunLineageAssociationRefV1     `json:"current"`
    Digest          core.Digest                    `json:"digest"`
}
```

`IndexID = CanonicalJSONDigest(domain, version, "RunLineageAssociationCurrentIndexIDV1", RunLineageAssociationIDInputV1{Subject})`。Index digest清自身后覆盖全字段；Clone必须deep-copy `Previous`。

任一合法Index都必须满足`Revision == HighestRevision == Current.Revision`、`SubjectDigest == Current.SubjectDigest`且`IndexID`由同一Subject重算一致。revision 1要求`Previous=nil`；revision N>1要求`Previous.Revision=N-1`并full-equal旧Index的`Current`。current CAS必须同时比较旧Index完整Ref与旧highest，不允许只比revision或ID。

首建事务必须同时提交：

```text
child pending Run bundle + empty EffectIndex
+ association history revision 1
+ highestRevision=1
+ current index(Current=full rev1 Ref, Previous=nil)
+ create receipt
```

续版事务必须先完整stage，再同一线性化点提交：

```text
child lifecycle mutation
+ association history revision N+1
+ highestRevision N->N+1
+ current index full-Ref CAS N->N+1
+ mutation receipt
```

任一validate、unique、CAS、storage或receipt stage失败时全部零写。禁止用删除回滚；history按`(ID, Revision, Digest)` exact存储，historical Inspect不读取current index。`highestRevision`与history/current不是最终一致异步更新。

## 6. child pending compound create

```go
type CreatePendingChildRunRequestV1 struct {
    ContractVersion  string                         `json:"contract_version"`
    Parent           RunLifecycleRecordExactRefV1  `json:"parent"`
    Child            CreatePendingRunRequestV3     `json:"child"`
    NotAfterUnixNano int64                          `json:"not_after_unix_nano"`
}

type RunLineageCreateReceiptV1 struct {
    ContractVersion string                         `json:"contract_version"`
    ID              string                         `json:"id"`
    Revision        core.Revision                  `json:"revision"`
    RequestDigest   core.Digest                    `json:"request_digest"`
    Child           RunLifecycleEnvelopeV3         `json:"child"`
    Association     RunLineageAssociationFactV1    `json:"association"`
    CurrentIndex    RunLineageAssociationCurrentIndexV1 `json:"current_index"`
    Digest          core.Digest                    `json:"digest"`
}
```

请求digest以具名`CreatePendingChildRunRequestV1`覆盖全部字段。Receipt ID另用具名输入冻结：

```go
type RunLineageCreateReceiptIDInputV1 struct {
    RequestDigest core.Digest `json:"request_digest"`
}
```

`Receipt.ID = CanonicalJSONDigest(domain, version, "RunLineageCreateReceiptV1", RunLineageCreateReceiptIDInputV1{RequestDigest})`；Receipt revision固定1，digest清自身后覆盖其余全部字段。Runtime Owner在同一事务内复读parent exact current、验证ordinary child V3 create、派生subject/ID/rev1 Fact，并完成全有全无写入。caller不能提交Association ID、revision、digest、state或current index。

public mutation能力仅给trusted assembler：

```go
type TrustedChildRunAssemblerPortV1 interface {
    CreatePendingChildRunV1(
        context.Context,
        CreatePendingChildRunRequestV1,
    ) (RunLineageCreateReceiptV1, error)
}
```

不公开独立`PublishAssociation`或`CompareAndSwapAssociation`给Application/Review/Harness。child V3生命周期mutation由Runtime Owner内部自动维护关联revision。

## 7. current与terminal projection

```go
type RunLineageAssociationCurrentRequestV1 struct {
    ContractVersion          string                         `json:"contract_version"`
    Subject                  RunLineageAssociationSubjectV1 `json:"subject"`
    RequestedNotAfterUnixNano int64                         `json:"requested_not_after_unix_nano"`
}

type RunLineageAssociationExactRequestV1 struct {
    ContractVersion          string                       `json:"contract_version"`
    Subject                  RunLineageAssociationSubjectV1 `json:"subject"`
    Association             RunLineageAssociationRefV1   `json:"association"`
    RequestedNotAfterUnixNano int64                       `json:"requested_not_after_unix_nano"`
}

type RunLineageAssociationTerminalRequestV1 struct {
    ContractVersion           string                          `json:"contract_version"`
    Subject                   RunLineageAssociationSubjectV1 `json:"subject"`
    RequestedNotAfterUnixNano int64                           `json:"requested_not_after_unix_nano"`
}

type RunLineageAssociationHistoricalRequestV1 struct {
    ContractVersion string                          `json:"contract_version"`
    Subject         RunLineageAssociationSubjectV1 `json:"subject"`
    Association     RunLineageAssociationRefV1     `json:"association"`
}

type RunLineageAssociationCurrentProjectionV1 struct {
    ContractVersion string                           `json:"contract_version"`
    Subject         RunLineageAssociationSubjectV1  `json:"subject"`
    Association     RunLineageAssociationFactV1     `json:"association"`
    CurrentIndex    RunLineageAssociationCurrentIndexV1 `json:"current_index"`
    ParentCurrent   RunLifecycleEnvelopeV3           `json:"parent_current"`
    ChildCurrent    RunLifecycleEnvelopeV3           `json:"child_current"`
    CheckedUnixNano int64                            `json:"checked_unix_nano"`
    ExpiresUnixNano int64                            `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                     `json:"projection_digest"`
}

type RunLineageAssociationTerminalProjectionV1 struct {
    ContractVersion string                            `json:"contract_version"`
    Subject         RunLineageAssociationSubjectV1    `json:"subject"`
    Association     RunLineageAssociationFactV1       `json:"association"`
    ParentCurrent   RunLifecycleEnvelopeV3             `json:"parent_current"`
    ChildTerminal   RunLifecycleEnvelopeV3             `json:"child_terminal"`
    ChildDecision   RunSettlementDecisionRefV3         `json:"child_decision"`
    ChildReport     RunTerminationReportRefV3           `json:"child_report"`
    CheckedUnixNano int64                              `json:"checked_unix_nano"`
    ExpiresUnixNano int64                              `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                       `json:"projection_digest"`
}
```

current projection digest清自身后覆盖完整Subject、Association、Index及parent/child lifecycle envelope、Checked/Expires。terminal projection digest清自身后覆盖完整Subject、exact historical Association、parent/child lifecycle envelope、Decision/Report与Checked/Expires；它不携带或派生第二个Run-current/terminal ProjectionRef，也不依赖current index才能Inspect exact history。terminal projection必须要求Association=`terminal_closed`、child lifecycle=`termination_closed`、Decision/Report与child envelope exact；它不嵌套current projection，避免active applicability TTL抹掉已经发生的Runtime终态。它仍不把Outcome升级为Review/Task/Goal结果。

TTL固定为：

```text
min(
  association.NotAfterUnixNano,
  request.RequestedNotAfterUnixNano（非零时）,
  baseline + 15s,
)
```

live Run V3 lifecycle没有独立current TTL字段，因此不得伪造parent/child Lease TTL或把历史Plan certification expiry错误当作truthful terminal Inspect禁令。Projection只是一段短期只读快照，不授Authority/Fence/Permit。`0 < Checked < Expires`；fresh `now`必须`now>=Checked && now<Expires`。

`ResolveTerminal`以Subject派生IndexID，要求current index指向`terminal_closed` exact Fact后返回truthful terminal projection；`InspectTerminal`只按caller给出的full exact terminal Association Ref读取immutable history，不读取或借用current index。两者都要求association与child已`termination_closed`，projection TTL固定为`min(request.RequestedNotAfter（非零）, baseline+15s)`，不再受已终止association的历史`NotAfter`阻断。该例外只允许truthful terminal read；不能恢复active current、签发Permit或改变任何历史。

## 8. Reader签名与S1/S2

```go
type RunLineageAssociationStoreReaderV1 interface {
    ResolveCurrentRunLineageAssociationIndexV1(
        context.Context,
        RunLineageAssociationSubjectV1,
    ) (RunLineageAssociationCurrentIndexV1, error)
    InspectRunLineageAssociationFactV1(
        context.Context,
        RunLineageAssociationHistoricalRequestV1,
    ) (RunLineageAssociationFactV1, error)
}

type RunLifecycleCurrentReaderV1 interface {
    InspectRunLifecycleV3(
        context.Context,
        core.ExecutionScope,
        core.AgentRunID,
    ) (RunLifecycleEnvelopeV3, error)
}

type RunLineageAssociationReadRecoveryPolicyV1 struct {
    ReadRecoveryTimeoutNanos int64 `json:"read_recovery_timeout_nanos"`
}

type RunLineageAssociationReaderDependenciesV1 struct {
    Store     RunLineageAssociationStoreReaderV1
    Lifecycle RunLifecycleCurrentReaderV1
    Clock     func() time.Time
}

type RunLineageAssociationCurrentReaderV1 interface {
    ResolveRunLineageAssociationCurrentV1(
        context.Context,
        RunLineageAssociationCurrentRequestV1,
    ) (RunLineageAssociationCurrentProjectionV1, error)

    InspectRunLineageAssociationCurrentV1(
        context.Context,
        RunLineageAssociationExactRequestV1,
    ) (RunLineageAssociationCurrentProjectionV1, error)

    ResolveRunLineageAssociationTerminalV1(
        context.Context,
        RunLineageAssociationTerminalRequestV1,
    ) (RunLineageAssociationTerminalProjectionV1, error)

    InspectRunLineageAssociationTerminalV1(
        context.Context,
        RunLineageAssociationExactRequestV1,
    ) (RunLineageAssociationTerminalProjectionV1, error)

    InspectRunLineageAssociationHistoricalV1(
        context.Context,
        RunLineageAssociationHistoricalRequestV1,
    ) (RunLineageAssociationFactV1, error)
}

func (p RunLineageAssociationReadRecoveryPolicyV1) Validate() error
func (d RunLineageAssociationReaderDependenciesV1) Validate() error
func NewRunLineageAssociationCurrentReaderV1(
    RunLineageAssociationReadRecoveryPolicyV1,
    RunLineageAssociationReaderDependenciesV1,
) (RunLineageAssociationCurrentReaderV1, error)
```

`ResolveCurrent/ResolveTerminal`唯一合法bootstrap是以完整Subject派生stable AssociationID/IndexID，调用Store线性化读取current full Ref；不提供by-name/latest/list-first。`InspectCurrent`必须由caller提供full exact Association Ref并证明它仍current；`InspectTerminal`也接收full exact terminal Association Ref，但只读取immutable history，不依赖current index。Store Reader与Lifecycle Reader都是Runtime Owner只读能力；constructor只接受上述具名policy/dependencies，拒绝nil/typed-nil Store、Lifecycle或nil Clock，禁止传入通用`any`、mutation Port、fake production fallback或全局registry。

`ResolveCurrent/InspectCurrent/ResolveTerminal`固定顺序：

```text
baseline=fresh clock（非零）
-> S1: current index + exact association history + parent lifecycle + child lifecycle
-> fresh clock，拒绝rollback/TTL crossing
-> S2: 同一index ID、同一exact association Ref、同一parent/child Run再读
-> 验证index仍full-equal，history full-equal，两个lifecycle envelope full-equal
-> child exact ref == Association.ChildCurrent
-> parent current identity/scope == ParentAnchor且revision/phase不回退
-> fresh clock，拒绝rollback/TTL crossing
-> seal短TTL projection并deep clone返回
```

`ResolveTerminal`还必须在S1和S2分别证明index current full Ref、highestRevision与exact terminal Fact一致；active或`terminal_cleanup`返回closed PreconditionFailed，不回退到current projection。S1/S2捕获EffectIndex、Closure、Decision、Progress、Report、Run Claim或phase任一读间变化；不能只比较Run revision。terminal必须在S2仍为termination_closed。

`InspectTerminal`固定顺序为`baseline -> S1 exact historical association + parent/child lifecycle -> fresh -> S2 same exact history + same parent/child -> fresh -> seal`；不读取current index，不因坏current/ABA拒绝truthful exact terminal history。S1/S2仍须full-equal association、parent/child envelope、Decision与Report，并按same-revision anchor/full-ref及higher-revision重新派生规则验证parent；lost reply只能重读同一Subject+Association full Ref。

## 9. lost reply、ctx与clock

- compound create一旦调用，任何Conflict/Unavailable/Indeterminate/cancel/deadline都只允许Inspect原child ID、rev1 association exact Ref与create receipt；缺任一对象即返回原Indeterminate，不重调create；
- child lifecycle mutation lost reply沿用V3 exact Inspect，并额外验证对应association exact history/current；不得重调Begin/Stop/Settle/Reconcile；
- read-only ResolveCurrent/ResolveTerminal丢回包可用同一canonical Subject做至多一次新S1；它返回新current snapshot，不声称恢复旧snapshot；
- exact InspectCurrent/InspectTerminal丢回包只可按同一Subject+full Association Ref至多一次恢复；InspectTerminal仍只读exact history，不切到current Resolve；
- `ReadRecoveryTimeoutNanos`是唯一恢复时限，constructor要求`0 < timeout <= 2s`。每次retry取`min(configured timeout, caller deadline remaining, request RequestedNotAfter remaining, current路径association NotAfter remaining)`；terminal truthful Inspect/Resolve不把历史association NotAfter纳入裁剪。结果`<=0`时不retry，返回原closed error；
- retry仅在policy明确启用时使用`context.WithoutCancel`，随后立即`context.WithTimeout(clipped)`；禁止裸detached context、goroutine或第三次调用；
- clock在baseline、S1后、S2后都重新读取；零值或回拨返回`ErrorIndeterminate/ReasonClockRegression`，zero projection；
- ctx cancel/deadline、backend unknown不得降级为NotFound。

## 10. closed errors

| 方法/场景 | Category | Reason | 约束 |
|---|---|---|---|
| 任一request/type/JSON shape非法 | `InvalidArgument` | `InvalidReference` | backend调用0 |
| ID/digest/canonical不匹配 | `Conflict` | `InvalidDigest` | zero projection/receipt |
| same ID换Subject/body | `Conflict` | `RunConflict` | 零写 |
| expected revision/current full Ref漂移 | `Conflict` | `RevisionConflict` | 零写/零泄露 |
| child/parent identity或scope漂移 | `Conflict` | `RunConflict` | zero projection |
| parent same revision full exact Ref不等于anchor；higher revision RecordDigest/phase重算失败 | `Conflict` | `RunConflict`或`InvalidDigest` | zero projection，不信caller phase/digest |
| phase回退、terminal方法遇非terminal | `PreconditionFailed` | `InvalidState` | 不推进状态 |
| current TTL crossing | `PreconditionFailed` | `CapabilityExpired` | 不创建expired revision |
| clock零值/回拨 | `Indeterminate` | `ClockRegression` | zero projection |
| exact ID或history从未存在 | `NotFound` | `InvalidReference` | 只表示线性化Fact Owner从未写入/已无exact历史 |
| recovery timeout `<=0`或`>2s` | `InvalidArgument` | `InvalidReference` | constructor失败，backend调用0 |
| Store/Lifecycle dependency nil/typed-nil或Clock nil | `InvalidArgument` | `ComponentMissing` | constructor失败，backend调用0 |
| `ResolveTerminal` current index指向active/cleanup | `PreconditionFailed` | `InvalidState` | zero terminal projection |
| `InspectTerminal` exact terminal history存在但current index缺失/损坏 | success | — | 不读取current index；truthful terminal closure仍S1/S2 |
| backend unavailable | `Unavailable` | 原Reason | 不转NotFound |
| ctx/unknown/lost reply无法证明 | `Indeterminate` | 原Reason | 只Inspect，不重mutation |

NotFound不得用于“current已变化”“active/cleanup尚非terminal”“retention未知”“eventual consistency”“backend unavailable”或“lost create reply没有完整closure”。historical/InspectTerminal只按exact Ref读取history，不依赖current index是否健康；ResolveTerminal只有Owner线性化证明Subject从未有association/index时才NotFound。所有read recovery失败保留原Category/Reason，不因第二次读取结果改写第一次unknown。

## 11. deep clone与并发

- Projection返回的`ExecutionScope`、CompletionClaim指针、Closure/Decision/Progress/Report指针及所有slice必须deep clone；
- 同Subject 64并发create只有一个canonical receipt，其余exact replay或Conflict，不允许第二child/association；
- 同expected index 64并发Run-record/phase publish只有一个N+1 winner，其余Inspect winner；
- 不同Tenant/Subject可并行，不使用全局mutable registry；
- runtime owner store锁/事务顺序固定为Run bundle→Run fact/sidecars→association history→highest→current index→receipt，避免SCC式跨锁回调。

## 12. 冻结的Validate/Clone/Digest方法面

```go
func (r RunLifecycleRecordExactRefV1) Clone() RunLifecycleRecordExactRefV1
func (r RunLifecycleRecordExactRefV1) Validate() error
func (s RunLineageAssociationSubjectV1) Validate() error
func (s RunLineageAssociationSubjectV1) DigestV1() (core.Digest, error)

func (r RunLineageAssociationRefV1) Validate() error
func (f RunLineageAssociationFactV1) Clone() RunLineageAssociationFactV1
func (f RunLineageAssociationFactV1) Validate() error
func (f RunLineageAssociationFactV1) DigestV1() (core.Digest, error)
func (f RunLineageAssociationFactV1) RefV1() (RunLineageAssociationRefV1, error)
func ValidateRunLineageAssociationTransitionV1(
    current, next RunLineageAssociationFactV1,
) error

func (i RunLineageAssociationCurrentIndexV1) Clone() RunLineageAssociationCurrentIndexV1
func (i RunLineageAssociationCurrentIndexV1) Validate() error
func (i RunLineageAssociationCurrentIndexV1) DigestV1() (core.Digest, error)

func (r CreatePendingChildRunRequestV1) Clone() CreatePendingChildRunRequestV1
func (r CreatePendingChildRunRequestV1) Validate(now time.Time) error
func (r CreatePendingChildRunRequestV1) DigestV1() (core.Digest, error)
func (r RunLineageCreateReceiptV1) Clone() RunLineageCreateReceiptV1
func (r RunLineageCreateReceiptV1) ValidateForV1(
    request CreatePendingChildRunRequestV1,
) error
func (r RunLineageCreateReceiptV1) DigestV1() (core.Digest, error)

func (r RunLineageAssociationCurrentRequestV1) Clone() RunLineageAssociationCurrentRequestV1
func (r RunLineageAssociationCurrentRequestV1) Validate(now time.Time) error
func (r RunLineageAssociationExactRequestV1) Clone() RunLineageAssociationExactRequestV1
func (r RunLineageAssociationExactRequestV1) Validate(now time.Time) error
func (r RunLineageAssociationTerminalRequestV1) Clone() RunLineageAssociationTerminalRequestV1
func (r RunLineageAssociationTerminalRequestV1) Validate(now time.Time) error
func (r RunLineageAssociationHistoricalRequestV1) Clone() RunLineageAssociationHistoricalRequestV1
func (r RunLineageAssociationHistoricalRequestV1) Validate() error

func (p RunLineageAssociationCurrentProjectionV1) Clone() RunLineageAssociationCurrentProjectionV1
func (p RunLineageAssociationCurrentProjectionV1) Validate() error
func (p RunLineageAssociationCurrentProjectionV1) ValidateCurrentForV1(
    request RunLineageAssociationCurrentRequestV1,
    now time.Time,
) error
func (p RunLineageAssociationCurrentProjectionV1) ValidateExactCurrentForV1(
    request RunLineageAssociationExactRequestV1,
    now time.Time,
) error
func (p RunLineageAssociationCurrentProjectionV1) DigestV1() (core.Digest, error)

func (p RunLineageAssociationTerminalProjectionV1) Clone() RunLineageAssociationTerminalProjectionV1
func (p RunLineageAssociationTerminalProjectionV1) Validate() error
func (p RunLineageAssociationTerminalProjectionV1) ValidateResolvedForV1(
    request RunLineageAssociationTerminalRequestV1,
    now time.Time,
) error
func (p RunLineageAssociationTerminalProjectionV1) ValidateExactForV1(
    request RunLineageAssociationExactRequestV1,
    now time.Time,
) error
func (p RunLineageAssociationTerminalProjectionV1) DigestV1() (core.Digest, error)

func (p RunLineageAssociationReadRecoveryPolicyV1) Validate() error
func (d RunLineageAssociationReaderDependenciesV1) Validate() error
```

所有`Validate`均不修正、排序、补默认或重封caller输入；所有`Clone`必须完成深拷贝。无self-digest的request/subject先完整Validate再Digest；带self-digest的Fact/Index/Receipt/Projection由`DigestV1`先执行不含self-digest比较的`validateBodyV1`，清且只清自身digest后覆盖完整body，`Validate`再比较recomputed digest。optional pointer/slice不得被忽略。Seal/derive函数只由Runtime Owner使用，consumer不能用Seal把坏输入变成合法事实。

## 13. Compatibility

- 不改`AgentRunRecord`、`CreatePendingRunRequestV3`、`RunLifecycleEnvelopeV3`、V3 digest或existing method set；
- legacy ordinary Run继续走现有V3，无association即无child语义；
- 已存在Run不得事后补造association或从SessionRef/Review Case猜parent；
- Review/Application/Harness只在未来适配器中消费runtime/ports中立类型，不导入Runtime kernel/control/fakes；
- 本合同不提供production backend、root、RPC、durability、retention或SLA。
