# Review Detached Delivery V1

## 1. 裁决与边界

本设计落实 `tmp.document/Review.md` 的 Detached Review：审核可以在独立时间、进程、节点或外部系统中完成，但始终绑定原 Candidate Revision。它不是新的 Reviewer 类型，也不是 Review 私建的 Run/Thread 生命周期。

- Review Owner 只拥有 `DetachedReviewDeliveryBindingV1` 与 `DetachedReviewDeliveryClosureV1`；
- Runtime Owner 继续唯一拥有 Run、父子 lineage、Run current/terminal；
- Application Owner 继续唯一拥有 ReviewWaiting 与标准 `RunCoordinatorV3` 编排水位；
- Harness Owner 继续唯一拥有 Phase source/current；
- Human 平台回包只形成 Observation/Attestation，不能成为 Verdict；
- trusted Agent Host 是最终 composition root，Review 不建立第二个 root。

当前裁决：Review-owned 资产可评审；Runtime lineage/current、Application detached orchestration、Human Thread governed delivery/current、Host root 未闭合前，production Go 与 production root **NO-GO**。

## 2. 唯一合法流程

1. Review 原子 Admission：`ReviewRequest(detached) -> Target -> Case`；
2. Harness/Application 对父 Phase、Target、Scope 做 S1；
3. Application 持久写入既有 `ReviewWaitingV1`；
4. Runtime arm：Application 复用 `RunCoordinatorV3`，Runtime 原子创建 child pending Run 与父子 lineage；Thread arm：宿主通过正常治理链准备外部 delivery attempt；
5. Review 对 Request/Case/Target、父 Phase、ReviewWaiting、endpoint current 做 S1/S2，并 create-once `DeliveryBinding`；
6. child Run/外部 delivery 只产生 Observation、Evidence Candidate 或 Attestation；
7. Runtime child 必须完成既有 start、claim、stop、settlement、cleanup，最终 `termination_closed`；外部 Thread 必须完成 delivery/observation/settlement current；
8. Review Owner 独立复读 Target、Policy、Authority、Binding、Scope、Evidence 后 CAS Verdict；
9. Review 对 endpoint terminal、Application coordination、Case/Attestation/Verdict current 再做 S1/S2，create-once `DeliveryClosure`；
10. 父 ReviewWaiting 复读 current Verdict 与父 Phase；只有 current allow 才形成 Phase receipt。任何未知、漂移或过期继续等待、失败关闭或升级。

所有 mutation 丢回包只 exact Inspect 原 canonical ID；跨 Start/Provider 边界后永久 Inspect-only。每个 S1/S2 使用非零 baseline 与 fresh actual-point clock并拒绝 rollback；TTL 取全部 current 输入最短正值。

## 3. Review-owned 对象

### 3.1 `DetachedReviewDeliveryBindingV1`

Go 落地只能依赖 Review 自有合同与 Runtime public `core/ports`。Application/Harness/platform 实现包不得被 Review domain import；其 exact fact 通过下列**具名中立 source coordinate**逐字段映射，语义 Owner 仍由来源组件保持。Runtime-owned refs直接复用Runtime public Port；尚未存在的Runtime类型名是 Port Delta 依赖，不授权 Review 私建兼容实现。

```go
type DetachedReviewDeliveryBindingRefV1 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"`
    Digest   core.Digest   `json:"digest"`
}

type DetachedReviewParentWaitingCoordinateV1 struct {
    TenantID            core.TenantID   `json:"tenant_id"`
    ID                  string          `json:"id"`
    Revision            core.Revision   `json:"revision"`
    Digest              core.Digest     `json:"digest"`
    ParentRunID         core.AgentRunID `json:"parent_run_id"`
    ExecutionScopeDigest core.Digest    `json:"execution_scope_digest"`
    PhaseKind           runtimeports.NamespacedNameV2 `json:"phase_kind"`
    PhaseID             string          `json:"phase_id"`
    Target              ExactResourceRefV1 `json:"target"`
}

type DetachedReviewParentPhaseCoordinateV1 struct {
    Kind            runtimeports.NamespacedNameV2 `json:"kind"`
    ID              string                        `json:"id"`
    Revision        core.Revision                 `json:"revision"`
    Digest          core.Digest                   `json:"digest"`
    CheckedUnixNano int64                         `json:"checked_unix_nano"`
    ExpiresUnixNano int64                         `json:"expires_unix_nano"`
}

type DetachedReviewApplicationCoordinationCoordinateV1 struct {
    TenantID     core.TenantID   `json:"tenant_id"`
    ID           string          `json:"id"`
    Revision     core.Revision   `json:"revision"`
    Digest       core.Digest     `json:"digest"`
    ChildRunID   core.AgentRunID `json:"child_run_id"`
    ScopeDigest  core.Digest     `json:"scope_digest"`
    SubjectDigest core.Digest    `json:"subject_digest"`
}

type DetachedReviewRuntimeEndpointV1 struct {
    Lineage       runtimeports.RunLineageAssociationRefV1 `json:"lineage"`
    ChildRun      runtimeports.RunCurrentProjectionRefV1  `json:"child_run"`
    Coordination DetachedReviewApplicationCoordinationCoordinateV1 `json:"coordination"`
}

type DetachedReviewThreadEndpointV1 struct {
    AdapterKind     runtimeports.NamespacedNameV2 `json:"adapter_kind"`
    ExternalSubjectDigest core.Digest             `json:"external_subject_digest"`
    Envelope        ExactResourceRefV1                    `json:"envelope"`
    DeliveryIntent  ExactResourceRefV1                    `json:"delivery_intent"`
    EnvelopeBinding ExactResourceRefV1                    `json:"envelope_binding"`
    DeliveryAttempt runtimeports.GovernedExecutionAttemptRefsV2 `json:"delivery_attempt"`
}

type DetachedReviewDeliveryBindingV1 struct {
    ContractVersion string                              `json:"contract_version"`
    Ref             DetachedReviewDeliveryBindingRefV1 `json:"ref"`
    TenantID        core.TenantID                       `json:"tenant_id"`
    Request         ExactResourceRefV1                  `json:"request"`
    Case            ExactResourceRefV1                  `json:"case"`
    Target          ExactResourceRefV1                  `json:"target"`
    ParentWaiting   DetachedReviewParentWaitingCoordinateV1 `json:"parent_waiting"`
    ParentPhase     DetachedReviewParentPhaseCoordinateV1   `json:"parent_phase"`
    RuntimeEndpoint *DetachedReviewRuntimeEndpointV1    `json:"runtime_endpoint,omitempty"`
    ThreadEndpoint  *DetachedReviewThreadEndpointV1     `json:"thread_endpoint,omitempty"`
    CheckedUnixNano int64                               `json:"checked_unix_nano"`
    ExpiresUnixNano int64                               `json:"expires_unix_nano"`
    Digest          core.Digest                         `json:"digest"`
}
```

不变量：

- `RuntimeEndpoint` 与 `ThreadEndpoint` 必须 exactly-one；Inline 必须 zero；
- ID 由 `Tenant + Request + Case + Target + ParentWaiting + ParentPhase + endpoint kind/exact refs` canonical 派生，Revision 固定 1；
- 同 ID 同 body 幂等；同 ID 换任何 exact ref 为 Conflict；
- Binding 是 immutable create-once，不追随新 Target、Run、Thread 或 current index；
- `Expires` 精确等于 Request、Case、Target、父 Phase、ReviewWaiting、lineage、endpoint current 与其 Binding/Authority/Scope/Budget 的最短正 TTL；
- Binding 只证明“这个 Detached endpoint 被绑定到该 Review”，不授 Run start、Provider、Authority、Evidence 或 Verdict。

### 3.2 canonical identity、digest与方法

```go
type DetachedReviewEndpointKindV1 string

const (
    DetachedReviewRuntimeEndpointKindV1 DetachedReviewEndpointKindV1 = "runtime_review_run"
    DetachedReviewThreadEndpointKindV1  DetachedReviewEndpointKindV1 = "external_review_thread"
)

type DetachedReviewDeliveryBindingIdentityInputV1 struct {
    TenantID      core.TenantID                       `json:"tenant_id"`
    Request       ExactResourceRefV1                  `json:"request"`
    Case          ExactResourceRefV1                  `json:"case"`
    Target        ExactResourceRefV1                  `json:"target"`
    ParentWaiting DetachedReviewParentWaitingCoordinateV1 `json:"parent_waiting"`
    ParentPhase   DetachedReviewParentPhaseCoordinateV1   `json:"parent_phase"`
    EndpointKind  DetachedReviewEndpointKindV1        `json:"endpoint_kind"`
    RuntimeEndpoint *DetachedReviewRuntimeEndpointV1  `json:"runtime_endpoint,omitempty"`
    ThreadEndpoint  *DetachedReviewThreadEndpointV1   `json:"thread_endpoint,omitempty"`
}

func (i DetachedReviewDeliveryBindingIdentityInputV1) Validate() error
func DeriveDetachedReviewDeliveryBindingIDV1(i DetachedReviewDeliveryBindingIdentityInputV1) (string, error)
func (v DetachedReviewDeliveryBindingV1) Validate() error
func (v DetachedReviewDeliveryBindingV1) ValidateCurrent(expected DetachedReviewDeliveryBindingRefV1, now time.Time) error
func (v DetachedReviewDeliveryBindingV1) DigestV1() (core.Digest, error)
func SealDetachedReviewDeliveryBindingV1(v DetachedReviewDeliveryBindingV1) (DetachedReviewDeliveryBindingV1, error)
```

canonical domain固定为`praxis.review.detached-delivery`，contract固定为`praxis.review/detached-delivery-v1`，type names逐字使用上列名字。ID只由`IdentityInputV1`派生；`Checked/Expires`不进入ID但进入body digest。`DigestV1`只把`Ref.Digest`与顶层`Digest`置空，其余字段全量canonical；Seal要求两者空或与重算值相同，最后二者都等于重算digest。`Revision=1`、`0 < Checked < Expires`。时间前进只使`ValidateCurrent`失败，不创建新Binding或新revision。

### 3.3 `DetachedReviewDeliveryClosureV1`

```go
type DetachedReviewDeliveryClosureV1 struct {
    ContractVersion string                                  `json:"contract_version"`
    Ref             ExactResourceRefV1                      `json:"ref"`
    Binding         DetachedReviewDeliveryBindingRefV1      `json:"binding"`
    RuntimeTerminal *runtimeports.RunTerminalProjectionRefV1 `json:"runtime_terminal,omitempty"`
    Coordination    *DetachedReviewApplicationCoordinationCoordinateV1 `json:"coordination,omitempty"`
    ThreadSettlement *runtimeports.OperationSettlementRefV4 `json:"thread_settlement,omitempty"`
    DeliveryObservation *runtimeports.ProviderAttemptObservationRefV2 `json:"delivery_observation,omitempty"`
    Case            ExactResourceRefV1                      `json:"case"`
    Attestations    []ExactResourceRefV1                    `json:"attestations"`
    Verdict         *ExactResourceRefV1                     `json:"verdict,omitempty"`
    Residuals       []ExactResourceRefV1                    `json:"residuals"`
    CheckedUnixNano int64                                   `json:"checked_unix_nano"`
    ExpiresUnixNano int64                                   `json:"expires_unix_nano"`
    Digest          core.Digest                             `json:"digest"`
}
```

中立coordinate只携带来源Owner已sealed的exact字段，Review adapter必须逐字段映射并让来源Owner Reader复读；它们不允许Review重算、发布或伪造来源Fact。Runtime Port Delta 类型名在相应 Owner冻结后逐字采用。Closure create-once、append-only，绑定原 Binding。Runtime arm 必须同时证明 child `termination_closed` 与 Application coordination closed；Thread arm 必须证明 governed delivery settlement 与 exact Observation。两条 arm 都必须重新复读 Review current；Run Outcome、Completion Claim、平台评论或 delivery success 均不能推导 Verdict。

Closure ID由`Tenant + Binding full Ref + endpoint kind + terminal/settlement/observation exact refs + final Case/Attestation/Verdict exact refs + Residual set`具名`DetachedReviewDeliveryClosureIdentityInputV1`派生，Revision固定1。Attestations与Residuals按`ID,Revision,Digest`严格递增、无重复；`Checked/Expires`不进ID但进body digest。方法冻结为：

```go
func (i DetachedReviewDeliveryClosureIdentityInputV1) Validate() error
func DeriveDetachedReviewDeliveryClosureIDV1(i DetachedReviewDeliveryClosureIdentityInputV1) (string, error)
func (v DetachedReviewDeliveryClosureV1) Validate() error
func (v DetachedReviewDeliveryClosureV1) ValidateCurrent(expected ExactResourceRefV1, now time.Time) error
func (v DetachedReviewDeliveryClosureV1) DigestV1() (core.Digest, error)
func SealDetachedReviewDeliveryClosureV1(v DetachedReviewDeliveryClosureV1) (DetachedReviewDeliveryClosureV1, error)
```

### 3.4 Review Store窄Port

```go
type DetachedReviewDeliveryStoreV1 interface {
    CreateDetachedReviewDeliveryBindingV1(context.Context, DetachedReviewDeliveryBindingV1) (DetachedReviewDeliveryBindingV1, error)
    InspectDetachedReviewDeliveryBindingExactV1(context.Context, core.TenantID, DetachedReviewDeliveryBindingRefV1) (DetachedReviewDeliveryBindingV1, error)
    CreateDetachedReviewDeliveryClosureV1(context.Context, DetachedReviewDeliveryClosureV1) (DetachedReviewDeliveryClosureV1, error)
    InspectDetachedReviewDeliveryClosureExactV1(context.Context, core.TenantID, ExactResourceRefV1) (DetachedReviewDeliveryClosureV1, error)
}
```

Store只有create-once+historical exact Inspect，没有current index或CAS：Binding/Closure的身份包含所有exact输入，任何输入变化必须形成新的对象，不允许把旧ID更新成“当前”。同ID同body幂等，同ID换body为Conflict。memory/SQLite必须在同一Owner锁/事务先全量Validate、seal、clone、检查ID冲突，再一次提交；staged failure零对象泄漏，返回值deep clone。

## 4. S1/S2 与恢复

- S1 从 Review exact facts及各 Owner linearized current index取得完整 Ref；
- S2 只按 S1 exact Ref复读，并验证各 current index仍指向该 full Ref；不得 by-name/latest；
- S1 Resolve unknown 可开始一个新的完整 cut，但不得声称恢复原结果；
- exact Inspect unknown 最多一次使用 bounded `context.WithoutCancel` recovery重读同 Ref；
- create Binding/Closure unknown 只 Inspect原 canonical对象；NotFound不能授 mutation replay，除非 Owner repository线性化证明该 canonical从未写入且设计明确允许同 canonical create-once replay；
- clock rollback、TTL crossing、ABA、跨租户、endpoint arm不唯一、父子 lineage漂移、closure缺 terminal/current 全部Fail Closed。

Read recovery timeout是构造参数`0 < ReadRecoveryTimeout <= 2s`，且实际timeout必须裁剪到当前cut最短剩余TTL；`WithoutCancel`后立即`WithTimeout`。Mutation丢回包恢复同样受2s与对象TTL约束。

### 4.1 closed errors

| 方法 | 条件 | Category + Reason |
|---|---|---|
| `Validate/Seal/Derive*` | 缺字段、union错误、非canonical顺序 | `InvalidArgument + InvalidCanonicalForm` |
| `Validate/Seal/Derive*` | exact字段/digest/tenant/scope漂移 | `Conflict + ReviewCandidateConflict` |
| `ValidateCurrent` | now=0或now<Checked | `PreconditionFailed + ClockRegression` |
| `ValidateCurrent` | now>=Expires | `PreconditionFailed + ReviewVerdictStale` |
| `Create*` | 同ID同body已存在 | success，返回同deep clone |
| `Create*` | 同ID换body | `Conflict + IdempotencyPayloadMismatch` |
| `Create*` | repository staged failure | `Unavailable + ComponentMissing`，零写 |
| `Create*` | commit成功但reply未知 | `Indeterminate + EffectUnknownOutcome`；caller只Inspect原Ref |
| `Inspect*Exact` | exact ID从未存在 | `NotFound + InvalidReference` |
| `Inspect*Exact` | ID存在但revision/digest错 | `Conflict + RevisionConflict` |
| 任一Owner current read | current missing/expired/drift | 保留Owner closed error，不降级或改写 |
| 任一read | ctx canceled/deadline | `Indeterminate + EffectUnknownOutcome`，最多一次bounded exact recovery |

## 5. Port Delta

| Owner | 最小公共Delta | Review消费方式 | 当前门禁 |
|---|---|---|---|
| Runtime | `RunLineageAssociationRefV1`、child Run exact-current/terminal projection、Resolve/InspectHistorical/InspectCurrent；与child pending Run同事务创建 | 只读S1/S2，绝不写Run | OPEN |
| Application | `DetachedReviewCoordinationV1`：只关联 ReviewWaiting、RunCoordination、DeliveryBinding/Closure；exact history/current Reader | 只读coordination current | OPEN |
| Human delivery Owner | governed delivery attempt/current、Envelope/Intent/Binding exact refs、Observation/Settlement/cleanup/residual current | 只读；评论只作Observation | OPEN |
| Host/Assembler | reviewer-only Plan、Context、Binding、Authority、Budget、Sandbox/tool scope；唯一production root | constructor注入公开Readers | OPEN |
| Harness | Action/Run Phase已有Reader；完整Subagent detached若要求则补Owner Reader | 不新建waiting状态 | partial/NO-GO |

每个 Delta 都必须定义 Owner、exact request/result、canonical digest、publisher/current index/history、S1/S2、TTL、closed errors、lost reply、兼容与反例；Review不能以私有弱ref或fake关闭门禁。

## 6. Effect、Cleanup、Residual

- child Run start/model/tool/remote inspect全部走既有 Runtime governed operation；Review不Dispatch；
- Human notification/webhook/polling是external mutation/data disclosure或Observation，必须由宿主Gateway与执行点双重门禁；
- cleanup unresolved、迟到输出、外部thread残留必须进入Residual；成功Verdict不能掩盖；
- Closure只引用Cleanup/Residual事实，不创建或修改它们；
- 不承诺transport exactly-once、physical exactly-once、跨节点HA或SLA。

## 7. 兼容与迁移

- `ReviewRequestV1.Delivery=detached`和Application `ReviewWaitingDetachedV1`保留；它们目前只是意图/等待坐标，不自动升级为DeliveryBinding；
- 旧Case可historical Inspect，缺Binding/Closure的Detached记录不得进入production parent resume；
- `ReviewRunPhaseSourceRefV1`是父Run Harness source，不能type-pun为child Run；
- 不修改Runtime Run V3、Application RunCoordinatorV3或Harness私有Port；全部通过版本化加法Port关联。
