# REV-D11 Binding Authoritative Current Port Delta V1（Review 第四候选）

## 1. 状态与双轴真值

- 候选状态：**FOURTH CANDIDATE / AWAITING TWO INDEPENDENT REVIEWS**。第三候选严格反方审计轴为 **NO，P0/P1/P2=2/3/1**；本轮只返修这些问题，不把未复审的第四候选写成YES。它不是Runtime Owner已冻结合同，也不授权Go实现。
- Review owner-local aggregate：**YES，P0/P1/P2=0/0/0**。Review 已有 exact Assignment/Target 构造路径、S1/S2 聚合算法、最短 TTL 与 closed recovery 设计。
- production external：**NO-GO，P0=6**。Binding、Evidence、Policy、Authority、Scope 各 1 项，加 production composition/root 1 项。本候选只细化其中 Binding P0=1，不关闭它，也不改变另外 5 项。
- 写入边界：本候选只存在于 Review 自有资产；不得修改 `ExecutionRuntime/runtime/**`，不得由 Review 私建 Go 类型、Adapter、Backend、fixture 或 root。
- Review消费面只读且无Effect；Runtime Binding Owner另持本候选的projection publisher，Review与production composition不得获得该写口。publisher只写Binding Owner自己的projection history/current index/highestRevision/publish receipt，不产生Grant、Evidence、Authorization、Permit、Begin、Verdict、Dispatch或Commit，也不触发pre-run Evidence。

## 2. 唯一 Owner、consumer 与最小公共面

| 对象/动作 | 唯一 Owner | Review/consumer 权限 | 禁止事项 |
|---|---|---|---|
| BindingSet、BindingFact、Grant、renew/revoke/current closure | Runtime Control Plane / Binding Fact Owner | 无写权；只消费 Owner-sealed current projection | Review 导入 `runtime/control`/`runtime/fakes`、复制 Raw Fact |
| subject-current index、projection history、highestRevision、publish receipt | Runtime Binding Owner | Review只按exact Source+Subject resolve/Inspect；Owner publisher首建/CAS续版 | Review获得publisher；by-name/latest/list/filter；caller自报Current/Next |
| host consumer association/current index | Runtime Binding Owner；host composition只绑定exact association ref | Review不能自报/发现association；Binding Reader每次复读 | 用caller consumer代替host association；把association当Authority |
| consumer Binding/Capability current | Runtime Binding Owner，复用live `ProviderBindingCurrentnessPortV2`语义 | Binding Reader按association.Consumer在S1/S2 exact复读 | 只验association不验consumer Grant/current/TTL |
| Assignment/Target exact Subject | Review Verdict Owner | 从自身 stored exact Facts 构造后传给 Binding Reader | Binding Owner 反向导入 Review Store；只做 shape 校验 |
| production 注入 | production composition Owner | 只注入下文 Reader interface | 注入 Binding 写口、Fake、全局 mutable registry |

唯一consumer是经production composition以sealed association ref绑定的Review Verdict Owner。consumer身份不作为caller字段；Reader构造时只接受一个exact association ref及其Owner Reader，运行期不得注册/替换。每次Resolve/InspectCurrent必须复读association及其Consumer Binding/Capability current。返回证明只是least-authority currentness，不能授执行或Evidence资格。

## 3. live 可复用与未闭合点

可复用：

1. `runtime/ports.ReviewComponentBindingRefV2` 六字段 exact Source；
2. `runtime/ports.CapabilityGrantV2` 的完整 capability/evidence/observed/expires 结构；
3. `runtime/control.BindingSetFactV2`、`BindingFactV2`、现有 Validate、Set/semantic/grant-set digest 算法与 Owner Store 实现模式；
4. Review `ReviewerAssignmentV1`、`TargetSnapshotV1` 与 `StoreV1.InspectAssignmentExactV1/InspectTargetExactV1`，用于 Review Owner 构造 exact Subject。

未闭合：

1. `BindingFactPortV2` 只有分离的按 ID Inspect，不能证明同一 snapshot；
2. live Provider current projection nominal 错误，不能 type-pun 为 Review Binding；
3. live 没有Review-binding nominal ProjectionRef/Projection/Reader/Publisher、append-only history、current full-Ref index、highestRevision、publish receipt或底层closure reread；
4. live没有Review-binding host consumer association nominal Ref/Projection/Reader；`ProviderBindingCurrentnessPortV2`只可复用consumer Binding/Capability current语义，不能替代association；
5. 当前完整Store是conformance fake，不是production Backend/root。

### 3.1 live 输入指纹

以下SHA-256是本候选的stale边界；任一文件变化后，Runtime Owner与两名独立审计者必须重新逐字段复读，不能沿用本候选授予准入：

| live 文件 | SHA-256 |
|---|---|
| `ExecutionRuntime/runtime/ports/review_v2.go` | `69b8719eed7dc519db23c7decbcefb9f53e02c614931259d5b2913f158e8b4d2` |
| `ExecutionRuntime/runtime/ports/binding_v2_types.go` | `436f06d7f6808cbcb4f8b1d7acc69859bc6e81fe39e31d58dbf5fdd2cebbf328` |
| `ExecutionRuntime/runtime/ports/binding_currentness_v2.go` | `43c960df736a01abdacafc1cf7587b870e43261efa5908e91b42f323080d9268` |
| `ExecutionRuntime/runtime/control/binding_fact_v2.go` | `623bfc546e682d9e729467c6c6f9d4752e67d4bbac7916b3a3cf383dbb5c4b7d` |
| `ExecutionRuntime/runtime/control/provider_binding_currentness_v2.go` | `2a446ee035c05385938e9bfab8cc896a06075f8491f2f51d9144734db0563cc1` |
| `ExecutionRuntime/runtime/control/run_settlement_fact_v2.go` | `22a9d303eca745a8839592a059e30cd439fb949e97fdedc25989e6b164e61a83` |
| `ExecutionRuntime/review/contract/reviewer.go` | `1074c9acd4098f10fc4cb3f85ac0423db83ee197312455396a2c51b4b629e872` |
| `ExecutionRuntime/review/ports/store.go` | `cc0115a32ccce31eafcc28b4434fab9aa7c8f8343e3e68dfe1db2572284f8be7` |
| `ExecutionRuntime/review/contract/current.go` | `04a564980317051979d4e6ffbfdc1286e62073ea20b4b25fab26dccc8b230634` |
| `ExecutionRuntime/review/ports/current.go` | `e9b1623f8bd9b888eb0eee582465b4a268a8f640a1b340bba727275c1723f6b1` |

## 4. 唯一公共窄 Port Delta：exact Go shape

以下 shape 是提交 Runtime Binding Owner 的候选；真实 Go 文件、package 与命名必须由 Runtime Owner 串行冻结。候选公共类型只能依赖 `context`、`time`、`runtime/core` 与同 package `runtime/ports` 类型，不能依赖 `runtime/control` 或 `review/*`。

```go
const ReviewBindingAuthoritativeCurrentContractV1 =
    "praxis.runtime.review-binding-authoritative-current/v1"

type ReviewBindingSubjectV1 struct {
    TenantID           core.TenantID  `json:"tenant_id"`
    AssignmentID       string         `json:"assignment_id"`
    AssignmentRevision core.Revision  `json:"assignment_revision"`
    AssignmentDigest   core.Digest    `json:"assignment_digest"`
    ReviewerID         string         `json:"reviewer_id"`
    TargetID           string         `json:"target_id"`
    TargetRevision     core.Revision  `json:"target_revision"`
    TargetDigest       core.Digest    `json:"target_digest"`
}

type ReviewBindingProjectionRefV1 struct {
    ID       string         `json:"id"`
    Revision core.Revision  `json:"revision"`
    Digest   core.Digest    `json:"digest"`
}

// This is a narrow public ref, not an alias of runtime/control.BindingSetFactV2.
type ReviewBindingSetExactRefV1 struct {
    ID              string        `json:"id"`
    Revision        core.Revision `json:"revision"`
    Digest          core.Digest   `json:"digest"`
    SemanticDigest  core.Digest   `json:"semantic_digest"`
    ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

// Canonical body for stable ProjectionID. Field order and JSON tags are frozen.
type ReviewBindingProjectionIdentityInputV1 struct {
    Source  ReviewComponentBindingRefV2 `json:"source"`
    Subject ReviewBindingSubjectV1      `json:"subject"`
}

type ReviewBindingConsumerAssociationIdentityInputV1 struct {
    ConsumerComponentID ComponentIDV2    `json:"consumer_component_id"`
    ConsumerCapability  CapabilityNameV2 `json:"consumer_capability"`
    SourceComponentID   ComponentIDV2    `json:"source_component_id"`
    SourceCapability    CapabilityNameV2 `json:"source_capability"`
}

type ReviewBindingConsumerAssociationRefV1 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"`
    Digest   core.Digest   `json:"digest"`
}

type ReviewBindingConsumerAssociationCurrentProjectionV1 struct {
    ContractVersion string                                        `json:"contract_version"`
    Ref             ReviewBindingConsumerAssociationRefV1        `json:"ref"`
    Consumer        ProviderBindingRefV2                          `json:"consumer"`
    Source          ReviewComponentBindingRefV2                   `json:"source"`
    Current         bool                                          `json:"current"`
    CheckedUnixNano int64                                         `json:"checked_unix_nano"`
    ExpiresUnixNano int64                                         `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                                  `json:"projection_digest"`
}

type ResolveReviewBindingCurrentRequestV1 struct {
    Source  ReviewComponentBindingRefV2 `json:"source"`
    Subject ReviewBindingSubjectV1      `json:"subject"`
}

type InspectReviewBindingProjectionRequestV1 struct {
    Ref             ReviewBindingProjectionRefV1 `json:"ref"`
    ExpectedSource  ReviewComponentBindingRefV2  `json:"expected_source"`
    ExpectedSubject ReviewBindingSubjectV1       `json:"expected_subject"`
}

type InspectCurrentReviewBindingRequestV1 struct {
    ExpectedRef     ReviewBindingProjectionRefV1 `json:"expected_ref"`
    ExpectedSource  ReviewComponentBindingRefV2  `json:"expected_source"`
    ExpectedSubject ReviewBindingSubjectV1       `json:"expected_subject"`
}

type ReviewBindingProjectionPublishRefV1 struct {
    ID     string      `json:"id"`
    Digest core.Digest `json:"digest"`
}

type CreateReviewBindingProjectionCommandInputV1 struct {
    Source      ReviewComponentBindingRefV2            `json:"source"`
    Subject     ReviewBindingSubjectV1                 `json:"subject"`
    Association ReviewBindingConsumerAssociationRefV1 `json:"association"`
}

type CompareAndSwapReviewBindingProjectionCommandInputV1 struct {
    ExpectedCurrent ReviewBindingProjectionRefV1            `json:"expected_current"`
    Source          ReviewComponentBindingRefV2              `json:"source"`
    Subject         ReviewBindingSubjectV1                   `json:"subject"`
    Association     ReviewBindingConsumerAssociationRefV1    `json:"association"`
}

type CreateReviewBindingProjectionRequestV1 struct {
    PublishRef ReviewBindingProjectionPublishRefV1          `json:"publish_ref"`
    Input      CreateReviewBindingProjectionCommandInputV1  `json:"input"`
}

type CompareAndSwapReviewBindingProjectionRequestV1 struct {
    PublishRef ReviewBindingProjectionPublishRefV1                  `json:"publish_ref"`
    Input      CompareAndSwapReviewBindingProjectionCommandInputV1  `json:"input"`
}

type ReviewBindingProjectionPublishReceiptV1 struct {
    ContractVersion string                                  `json:"contract_version"`
    PublishRef      ReviewBindingProjectionPublishRefV1     `json:"publish_ref"`
    Projection      ReviewBindingProjectionRefV1            `json:"projection"`
    CurrentIndex    ReviewBindingProjectionRefV1            `json:"current_index"`
    HighestRevision core.Revision                           `json:"highest_revision"`
    Digest          core.Digest                             `json:"digest"`
}

type ReviewBindingMemberCurrentRefV1 struct {
    ComponentID        ComponentIDV2 `json:"component_id"`
    BindingID          string        `json:"binding_id"`
    BindingRevision    core.Revision `json:"binding_revision"`
    BindingFactDigest  core.Digest   `json:"binding_fact_digest"`
    ManifestDigest     core.Digest   `json:"manifest_digest"`
    ArtifactDigest     core.Digest   `json:"artifact_digest"`
    SetGrantSetDigest  core.Digest   `json:"set_grant_set_digest"`
    FactGrantSetDigest core.Digest   `json:"fact_grant_set_digest"`
    BindingFactExpiresUnixNano int64 `json:"binding_fact_expires_unix_nano"`
    SetGrantMinExpiresUnixNano int64 `json:"set_grant_min_expires_unix_nano"`
    FactGrantMinExpiresUnixNano int64 `json:"fact_grant_min_expires_unix_nano"`
}

type ReviewBindingSelectedGrantRefV1 struct {
    ComponentID     ComponentIDV2    `json:"component_id"`
    BindingID       string           `json:"binding_id"`
    BindingRevision core.Revision    `json:"binding_revision"`
    Capability      CapabilityNameV2 `json:"capability"`
    SetGrantDigest  core.Digest      `json:"set_grant_digest"`
    FactGrantDigest core.Digest      `json:"fact_grant_digest"`
    ExpiresUnixNano int64            `json:"expires_unix_nano"`
}

// Canonical body for ClosureDigest. Field order and JSON tags are frozen.
type ReviewBindingAuthoritativeClosureInputV1 struct {
    Source              ReviewComponentBindingRefV2                          `json:"source"`
    BindingSet          ReviewBindingSetExactRefV1                           `json:"binding_set"`
    Members             []ReviewBindingMemberCurrentRefV1                    `json:"members"`
    SelectedGrant       ReviewBindingSelectedGrantRefV1                      `json:"selected_grant"`
    ConsumerAssociation ReviewBindingConsumerAssociationCurrentProjectionV1 `json:"consumer_association"`
    ConsumerBinding     ProviderBindingCurrentProjectionV2                   `json:"consumer_binding"`
    ExpiresUnixNano     int64                                                 `json:"expires_unix_nano"`
}

type ReviewBindingCurrentStateV1 string

const (
    ReviewBindingCurrentActiveV1     ReviewBindingCurrentStateV1 = "active"
    ReviewBindingCurrentRevokedV1    ReviewBindingCurrentStateV1 = "revoked"
    ReviewBindingCurrentExpiredV1    ReviewBindingCurrentStateV1 = "expired"
    ReviewBindingCurrentSupersededV1 ReviewBindingCurrentStateV1 = "superseded"
)

type ReviewBindingCurrentProjectionV1 struct {
    ContractVersion string                       `json:"contract_version"`
    Ref             ReviewBindingProjectionRefV1 `json:"ref"`
    Source          ReviewComponentBindingRefV2  `json:"source"`
    Subject         ReviewBindingSubjectV1       `json:"subject"`
    State           ReviewBindingCurrentStateV1  `json:"state"`
    Current         bool                         `json:"current"`

    BindingSetID              string        `json:"binding_set_id"`
    BindingSetRevision        core.Revision `json:"binding_set_revision"`
    BindingSetDigest          core.Digest   `json:"binding_set_digest"`
    BindingSetSemanticDigest  core.Digest   `json:"binding_set_semantic_digest"`
    BindingSetExpiresUnixNano int64         `json:"binding_set_expires_unix_nano"`

    Members       []ReviewBindingMemberCurrentRefV1 `json:"members"`
    SelectedGrant ReviewBindingSelectedGrantRefV1   `json:"selected_grant"`
    ConsumerAssociation ReviewBindingConsumerAssociationCurrentProjectionV1 `json:"consumer_association"`
    ConsumerBinding     ProviderBindingCurrentProjectionV2                   `json:"consumer_binding"`
    ClosureDigest core.Digest                       `json:"closure_digest"`

    CheckedUnixNano  int64       `json:"checked_unix_nano"`
    ExpiresUnixNano  int64       `json:"expires_unix_nano"`
    ProjectionDigest core.Digest `json:"projection_digest"`
}

type ReviewBindingAuthoritativeCurrentReaderV1 interface {
    ResolveCurrentReviewBindingV1(
        context.Context,
        ResolveReviewBindingCurrentRequestV1,
    ) (ReviewBindingProjectionRefV1, error)

    InspectReviewBindingProjectionV1(
        context.Context,
        InspectReviewBindingProjectionRequestV1,
    ) (ReviewBindingCurrentProjectionV1, error)

    InspectCurrentReviewBindingV1(
        context.Context,
        InspectCurrentReviewBindingRequestV1,
    ) (ReviewBindingCurrentProjectionV1, error)
}

type ReviewBindingConsumerAssociationCurrentReaderV1 interface {
    InspectCurrentReviewBindingConsumerAssociationV1(
        context.Context,
        ReviewBindingConsumerAssociationRefV1,
    ) (ReviewBindingConsumerAssociationCurrentProjectionV1, error)
}

// Only the Runtime Binding Owner control path receives this interface.
// Review and production composition receive Reader only.
type ReviewBindingProjectionPublisherV1 interface {
    CreateReviewBindingProjectionV1(
        context.Context,
        CreateReviewBindingProjectionRequestV1,
    ) (ReviewBindingProjectionPublishReceiptV1, error)
    CompareAndSwapReviewBindingProjectionV1(
        context.Context,
        CompareAndSwapReviewBindingProjectionRequestV1,
    ) (ReviewBindingProjectionPublishReceiptV1, error)
    InspectReviewBindingProjectionPublishV1(
        context.Context,
        ReviewBindingProjectionPublishRefV1,
    ) (ReviewBindingProjectionPublishReceiptV1, error)
}
```

候选 Validate surface：

```go
func (s ReviewBindingSubjectV1) Validate() error
func (r ReviewBindingProjectionRefV1) Validate() error
func (r ReviewBindingSetExactRefV1) Validate() error
func (i ReviewBindingProjectionIdentityInputV1) Validate() error
func (i ReviewBindingConsumerAssociationIdentityInputV1) Validate() error
func (r ReviewBindingConsumerAssociationRefV1) Validate() error
func (p ReviewBindingConsumerAssociationCurrentProjectionV1) Validate() error
func (p ReviewBindingConsumerAssociationCurrentProjectionV1) ValidateCurrent(
    expected ReviewBindingConsumerAssociationRefV1,
    expectedConsumer ProviderBindingRefV2,
    expectedSource ReviewComponentBindingRefV2,
    now time.Time,
) error
func (i ReviewBindingAuthoritativeClosureInputV1) Validate() error
func (r ReviewBindingProjectionPublishRefV1) Validate() error
func (i CreateReviewBindingProjectionCommandInputV1) Validate() error
func (i CompareAndSwapReviewBindingProjectionCommandInputV1) Validate() error
func (r CreateReviewBindingProjectionRequestV1) Validate() error
func (r CompareAndSwapReviewBindingProjectionRequestV1) Validate() error
func (r ReviewBindingProjectionPublishReceiptV1) Validate() error
func (r ResolveReviewBindingCurrentRequestV1) Validate() error
func (r InspectReviewBindingProjectionRequestV1) Validate() error
func (r InspectCurrentReviewBindingRequestV1) Validate() error
func (p ReviewBindingCurrentProjectionV1) Validate() error
func (p ReviewBindingCurrentProjectionV1) ValidateCurrent(
    expectedRef ReviewBindingProjectionRefV1,
    expectedSource ReviewComponentBindingRefV2,
    expectedSubject ReviewBindingSubjectV1,
    now time.Time,
) error
```

## 5. Canonical ID、digest、history 与 index

| 对象 | domain / version / discriminator | canonical body |
|---|---|---|
| BindingSet digest | 复用 `praxis.runtime.binding` / `BindingContractVersionV2` / `BindingSetFactV2` | 完整 Set；nil Members/Order/Residuals 规范为空集合 |
| BindingFact digest | `praxis.runtime.binding` / `BindingContractVersionV2` / `BindingFactV2` | 完整 Fact；nil Grants/RenewalEvidence 规范为空集合 |
| GrantSet digest | 复用 `praxis.runtime.binding` / `BindingContractVersionV2` / `BindingGrantSetV2` | 排序、唯一的完整 Grant 集合 |
| single Grant digest | `praxis.runtime.binding` / `BindingContractVersionV2` / `CapabilityGrantV2` | 完整 `CapabilityGrantV2` 四字段 |
| Consumer Association ID | `praxis.runtime.review-binding-current` / `ReviewBindingAuthoritativeCurrentContractV1` / `ReviewBindingConsumerAssociationIdentityInputV1` | consumer Component/Capability + source Component/Capability；不含mutable revision/digest/TTL |
| Consumer Association projection digest | 同domain/version / `ReviewBindingConsumerAssociationCurrentProjectionV1` | 完整projection，计算前清`Ref.Digest`与`ProjectionDigest`；两者seal为同一digest |
| Projection ID | `praxis.runtime.review-binding-current` / `ReviewBindingAuthoritativeCurrentContractV1` / `ReviewBindingProjectionIdentityInputV1` | 具名`ReviewBindingProjectionIdentityInputV1{Source,Subject}`；canonical digest 文本即稳定 ID |
| Closure digest | 同 domain/version / `ReviewBindingAuthoritativeClosureInputV1` | 具名closure含Source、Set exact ref、全部Member、SelectedGrant、Consumer Association current、Consumer Binding current与true min Expires |
| Projection digest | 同 domain/version / `ReviewBindingCurrentProjectionV1` | 完整 projection，但计算前同时清空 `Ref.Digest` 与 `ProjectionDigest`；计算后两字段必须相等 |
| Create PublishRef | 同domain/version / `CreateReviewBindingProjectionCommandInputV1` | 完整具名Input；digest文本同时作为PublishRef.ID与Digest |
| CAS PublishRef | 同domain/version / `CompareAndSwapReviewBindingProjectionCommandInputV1` | ExpectedCurrent+Source+Subject+Association；digest文本同时作为PublishRef.ID与Digest |
| Publish Receipt digest | 同domain/version / `ReviewBindingProjectionPublishReceiptV1` | 完整receipt，计算前清`Digest` |

同canonical Source+Subject永远使用同一ProjectionID。首建只能Revision=1；续版严格`current+1`。`publish receipt + history write + highestRevision advance + current full-Ref CAS`必须同一Binding Owner原子事务全有全无；stage failure零写。同ID/revision换digest、revision rollback/gap、旧Ref重新发布、same PublishRef换Input均Conflict。current index保存full Ref，`InspectCurrent`验证index与highestRevision以检测ABA。

### 5.1 两个具名 canonical input 的唯一算法

```go
identity := ReviewBindingProjectionIdentityInputV1{
    Source: p.Source,
    Subject: p.Subject,
}
projectionID, err := core.CanonicalJSONDigest(
    "praxis.runtime.review-binding-current",
    ReviewBindingAuthoritativeCurrentContractV1,
    "ReviewBindingProjectionIdentityInputV1",
    identity,
)

associationIdentity := ReviewBindingConsumerAssociationIdentityInputV1{
    ConsumerComponentID: consumer.Ref.ComponentID,
    ConsumerCapability: consumer.Ref.Capability,
    SourceComponentID: p.Source.ComponentID,
    SourceCapability: p.Source.Capability,
}
associationID, err := core.CanonicalJSONDigest(
    "praxis.runtime.review-binding-current",
    ReviewBindingAuthoritativeCurrentContractV1,
    "ReviewBindingConsumerAssociationIdentityInputV1",
    associationIdentity,
)

closure := ReviewBindingAuthoritativeClosureInputV1{
    Source: p.Source,
    BindingSet: ReviewBindingSetExactRefV1{
        ID: p.BindingSetID,
        Revision: p.BindingSetRevision,
        Digest: p.BindingSetDigest,
        SemanticDigest: p.BindingSetSemanticDigest,
        ExpiresUnixNano: p.BindingSetExpiresUnixNano,
    },
    Members: normalizedMembers,
    SelectedGrant: p.SelectedGrant,
    ConsumerAssociation: p.ConsumerAssociation,
    ConsumerBinding: p.ConsumerBinding,
    ExpiresUnixNano: p.ExpiresUnixNano,
}
closureDigest, err := core.CanonicalJSONDigest(
    "praxis.runtime.review-binding-current",
    ReviewBindingAuthoritativeCurrentContractV1,
    "ReviewBindingAuthoritativeClosureInputV1",
    closure,
)
```

`Projection.Validate()`与`Projection.ValidateCurrent()`都必须重算ProjectionID、Association ID/ProjectionDigest、ClosureDigest及Consumer `ProviderBindingCurrentProjectionV2` self digest。它们要求exact association Source等于p.Source、association Consumer等于ConsumerBinding.Ref、association ID命中stable identity、`string(projectionID)==p.Ref.ID`且`closureDigest==p.ClosureDigest`。两者还逐字段证明Set/Source/member/selected关系、Members完整排序唯一及true min TTL。`ValidateCurrent()`只增加expected三元组、active/current与fresh now；Owner index/highestRevision、association index及实时closure只能由Owner Readers证明。

所有字段与上方JSON tags均为V1 exact wire shape，禁止`omitempty`、匿名struct、map、别名字段或不同拼写。wire输入必须先经`core.DecodeStrictJSON`拒绝unknown/duplicate/trailing，再执行`Validate()`；缺字段即产生不可接受的零值并Fail Closed。nil `Members`规范为非nil空数组后再hash，但active projection要求至少一个完整member，故active输入中nil/empty均非法。PublishRef由具名CommandInput预先确定；publisher不得接受caller提交的Next Projection、Current布尔或TTL。

### 5.2 literal golden

Identity canonical JSON（单行、无额外空白）：

```json
{"source":{"binding_set_id":"binding-set-001","binding_set_revision":7,"component_id":"review/auto-worker","manifest_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","artifact_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","capability":"review/attest"},"subject":{"tenant_id":"tenant-a","assignment_id":"assignment-001","assignment_revision":3,"assignment_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","reviewer_id":"reviewer-001","target_id":"target-001","target_revision":5,"target_digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444"}}
```

Expected ProjectionID：`sha256:a7f1fa4cc093ca2dfb2e0e1aaf1660376d5c883e54d25429346e2614203581cc`。

Closure canonical JSON（单行、无额外空白）：

```json
{"source":{"binding_set_id":"binding-set-001","binding_set_revision":7,"component_id":"review/auto-worker","manifest_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","artifact_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","capability":"review/attest"},"binding_set":{"id":"binding-set-001","revision":7,"digest":"sha256:5555555555555555555555555555555555555555555555555555555555555555","semantic_digest":"sha256:6666666666666666666666666666666666666666666666666666666666666666","expires_unix_nano":2000000000},"members":[{"component_id":"review/auto-worker","binding_id":"binding-001","binding_revision":11,"binding_fact_digest":"sha256:7777777777777777777777777777777777777777777777777777777777777777","manifest_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","artifact_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","set_grant_set_digest":"sha256:8888888888888888888888888888888888888888888888888888888888888888","fact_grant_set_digest":"sha256:8888888888888888888888888888888888888888888888888888888888888888","binding_fact_expires_unix_nano":1900000000,"set_grant_min_expires_unix_nano":1800000000,"fact_grant_min_expires_unix_nano":1800000000}],"selected_grant":{"component_id":"review/auto-worker","binding_id":"binding-001","binding_revision":11,"capability":"review/attest","set_grant_digest":"sha256:9999999999999999999999999999999999999999999999999999999999999999","fact_grant_digest":"sha256:9999999999999999999999999999999999999999999999999999999999999999","expires_unix_nano":1800000000},"consumer_association":{"contract_version":"praxis.runtime.review-binding-authoritative-current/v1","ref":{"id":"sha256:c30d58cff3e5e3d477ba99dc71da047e08688f78f76d5a9bc2579cbf752bea11","revision":2,"digest":"sha256:9fa5ae3337dc08b68f25b4b3f39ef4b76d33ad885b78adf24142cb19c2fb39ff"},"consumer":{"binding_set_id":"host-binding-set-001","binding_set_revision":9,"component_id":"review/verdict-owner","manifest_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","artifact_digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","capability":"runtime/read-review-binding-current"},"source":{"binding_set_id":"binding-set-001","binding_set_revision":7,"component_id":"review/auto-worker","manifest_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","artifact_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","capability":"review/attest"},"current":true,"checked_unix_nano":1500000000,"expires_unix_nano":1750000000,"projection_digest":"sha256:9fa5ae3337dc08b68f25b4b3f39ef4b76d33ad885b78adf24142cb19c2fb39ff"},"consumer_binding":{"contract_version":"2.0.0","ref":{"binding_set_id":"host-binding-set-001","binding_set_revision":9,"component_id":"review/verdict-owner","manifest_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","artifact_digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","capability":"runtime/read-review-binding-current"},"state":"active","binding_set_digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","binding_set_semantic_digest":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","binding_id":"consumer-binding-001","binding_revision":4,"grant_digest":"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","projection_digest":"sha256:64727f41813a2e85d4cc068de0603eea9f55c650ddd4ce600731ddd4de867724","issued_unix_nano":1400000000,"expires_unix_nano":1700000000},"expires_unix_nano":1700000000}
```

Expected AssociationID：`sha256:c30d58cff3e5e3d477ba99dc71da047e08688f78f76d5a9bc2579cbf752bea11`；Association ProjectionDigest：`sha256:9fa5ae3337dc08b68f25b4b3f39ef4b76d33ad885b78adf24142cb19c2fb39ff`；Consumer Binding ProjectionDigest：`sha256:64727f41813a2e85d4cc068de0603eea9f55c650ddd4ce600731ddd4de867724`。

Expected ClosureDigest：`sha256:e56799384b6aea7fa8ef906c537292d1aa38324c85e631226b49b7214314abc7`。

## 5A. 首建/续版 publisher 与原子事务

`ReviewBindingProjectionPublisherV1`只由Runtime Binding Owner control path持有；Review与composition只拿Reader。Create/CAS请求不携Next Projection、Current、Checked、Expires或digest结果，Owner必须在fresh clock与同一authoritative snapshot内自行构造并seal。

1. **首建**：Create的具名Input派生stable ProjectionID与PublishRef；要求history/current/highest均不存在，构造Revision=1。`publish receipt + history(ID,1) + highestRevision=1 + current full Ref`同一事务全有全无。
2. **续版**：CAS要求current full Ref与highestRevision均exact等于ExpectedCurrent，且Owner本次authoritative Binding/association mutation确实改变sealed body；Next Revision严格`expected+1`。同一Owner事务同时提交底层Binding/Grant或association mutation、publish receipt、new history、highest与current CAS；禁止先改底层再独立publish。
3. **并发**：同ExpectedCurrent的64个不同command最多一个提交；loser为Conflict且零receipt/history/index/highest/底层mutation。same PublishRef+same Input仅能由historical receipt证明幂等结果；same Ref换Input/Digest为Conflict。
4. **staged failure**：任一步Validate、closure读取、consumer current读取、seal或store stage失败，全部对象零写；禁止删除回滚模拟原子性。
5. **无变化/纯时间**：sealed authoritative body未变时CAS为PreconditionFailed且不增Revision。时钟前进或`now >= Expires`绝不调用publisher、不创建`expired`projection；只让current Reader/ValidateCurrent失败。
6. **lost reply**：Create/CAS调用返回未知后永久失去mutation调用权，只以预先确定的exact PublishRef调用`InspectReviewBindingProjectionPublishV1`；NotFound/Unavailable/Indeterminate均不授权重调Create/CAS。Inspect成功必须复核receipt Input digest、Projection Ref与current/history闭包；changed command永远Conflict。

Publisher公共shape仅用于Runtime Owner实现与conformance；它不是Review public dependency。任何BindingSet/Fact/Grant或host association mutation若不能与受影响projection续版共享上述事务，production构造失败；Reader的closure复读只是Fail Closed第二防线，不替代publisher原子性。

## 6. `ReadAuthoritativeBindingClosure`：Owner 私有强制语义

本候选不公开第二套Raw BindingSet/Fact/Grant Reader。`ReadAuthoritativeBindingClosure`是Resolve与每次InspectCurrent内部必须执行的语义；它同时复读构造时bound的host association exact Ref以及association.Consumer对应的`ProviderBindingCurrentnessPortV2`。historical Inspect只按exact Ref+ExpectedSource+ExpectedSubject读取history并验projection digest，**不读取、不要求、也不修复current index/highest/closure/association**。

Runtime Binding Owner内部候选能力：

```go
// runtime/control内部；不得进入runtime/ports。
type bindingClosureSnapshotReaderV1 interface {
    inspectBindingClosureSnapshotV1(
        context.Context,
        string, // BindingSetID
    ) (BindingSetFactV2, []BindingFactV2, error)
}
```

强制约束：

1. 一次数据库 snapshot/事务或同一 Store 读锁读取当前 BindingSet 与其全部 member BindingFact；
2. 同一 snapshot 中读取每个 member 在 Set 侧与 Fact 侧的完整 Grant 集合，禁止只读 selected member；
3. `BindingFactPortV2`的分离 `InspectBindingSet/InspectBinding`可复用底层 concrete Store与校验算法，但该接口本身不证明同一 snapshot；禁止循环调用作为production fallback；
4. Set member与Fact必须逐字段核对BindingID/Revision、ComponentID、BindingSetID、Manifest/Artifact、Bound/Active state；两侧完整Grant集合必须canonical相等；
5. selected member/capability必须由Source六字段唯一确定；缺失、重复或type-pun均Fail Closed；
6. 同一次current调用复读bound Association Ref并验证其current index/ProjectionDigest/Source，再以Association.Consumer exact调用consumer Binding current Reader，验证active、capability exact与self digest；caller不得提交association或替换Consumer；
7. snapshot、projection current index/highestRevision、association current proof及consumer current proof必须形成同一次Owner read snapshot；无法提供则构造失败；
8. snapshot返回后对全部嵌套值/slice执行deep clone；任何错误零public result；
9. Raw facts不出`runtime/control`，public projection只暴露exact refs/digests/TTL水位。

即使projection index未推进，底层renew/revoke/expiry/grant drift也会被每次current读取重新发现。index只定位projection，不是current Authority。

## 7. S1/S2 与稳定 projection

### 7.1 S1

1. Review Owner exact Inspect stored Assignment和Target，证明Tenant、Assignment ID/Revision/Digest、ReviewerID、Target ID/Revision/Digest并构造Subject；Binding Owner不导入Review Store。
2. Reader构造时的bound association Ref不可来自request。调用Resolve；Owner取fresh baseline，复读Binding closure、association current、Association.Consumer Binding/Capability current，并核index/history/highest后返回full ProjectionRef。
3. 立即调用InspectCurrent；Owner再次独立复读上述三组current，原子校验index等于ExpectedRef且完整closure与sealed projection一致。
4. Review保存完整deep-cloned projection，不能只保存Ref、ClosureDigest或TTL。

### 7.2 S2

1. 不重新Resolve，不追随新Ref；只以S1保存的ExpectedRef+Source+Subject调用`InspectCurrentReviewBindingV1`。
2. Owner第三次读取全新Binding closure、同一bound association及同Consumer Binding/Capability，并复验index/highest/完整closure。
3. `InspectCurrentReviewBindingV1`先在Owner一致性域完成index/highestRevision/权威closure检查；Review再对返回值调用纯值`ValidateCurrent(expectedRef,expectedSource,expectedSubject,freshNow)`，并把S2与S1完整projection逐字段比较；固定Checked/Expires/Digest也必须相等。
4. 任一底层mutation、index变化、TTL crossing或clock rollback使本轮aggregate zero；上层只能从全新S1重启，不能拼接部分结果。

Resolve和每次InspectCurrent都必须重新复读Binding closure+association+consumer current；仅验证projection index或缓存consumer不合格。

## 8. Consumer、Grant、current、TTL 与状态真值

- Grant只证明Source指定ComponentID/Capability在该权威closure中当前存在；Grant/ref/projection均不授权Review执行Provider或写Runtime事实。
- Association只证明host把exact Review consumer绑定到exact Source；Consumer Binding current只证明该consumer仍持`runtime/read-review-binding-current` exact capability。两者都不授业务Authority。
- 对一次成功的`InspectCurrentReviewBindingV1`，`Current=true`必须由两层共同证明：Owner Reader在同一snapshot中证明subject-current index等于Ref、highestRevision等于Ref.Revision且权威closure与projection一致；纯值`ValidateCurrent`只证明sealed字段、expected三元组、active/current状态与fresh now早于完整TTL。任一层缺失均不是current证明。
- true min TTL无条件冻结为：

```text
min(
  BindingSet.Expires,
  every BindingFact.Expires,
  every Set-side Grant.Expires,
  every Fact-side Grant.Expires,
  ConsumerAssociation.Expires,
  ConsumerBinding.Expires
)
```

`Projection.ExpiresUnixNano`必须精确等于上式；Members必须完整覆盖Set全部member并按ComponentID排序唯一。Set/Fact两侧每个member的GrantSetDigest、最短Grant TTL及selected Grant digest必须可由Owner用完整Grant复算。

状态真值：

| State | Current | 合法条件 |
|---|---:|---|
| `active` | `true` | Set active、全部Fact bound、双侧full Grant相等、Source唯一匹配、TTL未到界 |
| `revoked` | `false` | 权威Set/必需Fact revoke后由Owner创建新terminal projection |
| `expired` | `false` | 仅权威Set/Fact发生显式governed expired状态mutation时创建terminal projection；纯时间到界不创建 |
| `superseded` | `false` | governed renewal/replacement使Source revision不再current并创建terminal projection |

没有`unknown/current=true`或`drifted`状态。底层/association/consumer不一致时Reader返回closed error，不合成事实。historical `active,true`保持immutable且不授current；即使current index损坏、缺失或指向坏Ref，historical exact Inspect仍只按请求exact history返回deep clone。纯时间到期后history内容/Revision不变。

## 9. Validate 与 deep-clone合同

`Validate()`必须验证完整shape、版本、排序/唯一、所有digest、Source/Subject、Set/member/selected、association/consumer exact关系、`0 < Checked < Expires`、Projection ID/revision/digest和true min TTL。纯值`ValidateCurrent`只增加expected Ref/Source/Subject、active/current与`Checked <= now < Expires`；它不得宣称验证projection/association index、highestRevision或实时closure。后者只能由current Readers完成。fresh now只验证，不重封也不发布expired revision。

historical/current Inspect与publisher receipt Inspect每次返回新的deep clone。修改Members、Association、ConsumerBinding、request/receipt任一嵌套值不得影响Store、history/index或其他goroutine。

## 10. ctx、clock 与 lost reply

每次Resolve/InspectCurrent：

```text
检查ctx
t0 = fresh Clock()，要求非零
读取同一Owner snapshot并deep clone
t1 = fresh Clock()，要求t1 >= t0
以t1验证全closure TTL和projection currentness
```

- projection的Checked/Expires/ProjectionDigest是状态变化时固定seal，不使用`t0/t1`刷新；
- ctx取消、Deadline或无法确认读取结果均为Indeterminate，零projection；
- lost reply只允许使用独立、有界、未继承原ctx取消状态的新context重试同一canonical request一次；边界由宿主批准，本候选不冻结超时/SLA；
- Resolve没有expected Ref：重试可能取得新current Ref，只能作为全新S1，不能声称恢复原结果；
- historical/current Inspect recovery必须保持同Ref+Source+Subject；historical不读current index；current仍复读association/consumer；不得重新Resolve、读latest/by-name或调用mutation；
- recovery前后都取fresh clock；TTL crossing、rollback、第二次Unknown或closure变化均Fail Closed；
- 只承诺Owner read-only canonical replay，不承诺transport exactly-once。

Create/CAS lost reply不适用上述只读retry：调用者只能用独立ctx exact Inspect预先sealed PublishRef；绝不重调mutation。Inspect Publish Receipt也使用fresh双时钟，Unknown/NotFound不恢复调用权。

## 11. 三方法 closed error矩阵

| 条件 | ResolveCurrent | InspectProjection | InspectCurrent |
|---|---|---|---|
| request/ref/source/subject非法 | `InvalidArgument/InvalidReference或InvalidDigest` | 同 | 同 |
| strict JSON unknown/改名/缺字段/非法shape | `InvalidArgument/InvalidCanonicalForm` | 同 | 同 |
| strict JSON任意层重复key | `Conflict/DuplicateCanonicalKey` | 同 | 同 |
| canonical current index不存在 | `NotFound/OwnerMissing` | — | `NotFound/OwnerMissing` |
| exact history不存在 | — | `NotFound/OwnerMissing` | `NotFound/OwnerMissing` |
| projection/closure digest损坏 | `Conflict/InvalidDigest` | `Conflict/InvalidDigest` | `Conflict/InvalidDigest` |
| history/index/highestRevision矛盾或ABA | `Conflict/RevisionConflict` | `Conflict/RevisionConflict` | `Conflict/RevisionConflict` |
| Tenant/Owner边界不成立 | `Forbidden/OwnerConflict` | `Forbidden/OwnerConflict` | `Forbidden/OwnerConflict` |
| Set/member/Grant/Source closure漂移或Source superseded | `Conflict/BindingDrift` | 不判断current | `Conflict/BindingDrift` |
| Set/Fact revoked、非bound或未认证 | `PreconditionFailed/BindingNotCertified` | 返回历史deep clone | `PreconditionFailed/BindingNotCertified` |
| TTL到界 | `PreconditionFailed/BindingExpired` | 返回历史deep clone | `PreconditionFailed/BindingExpired` |
| capability缺失/重复 | `PreconditionFailed/BindingNotCertified` | — | `PreconditionFailed/BindingNotCertified` |
| clock rollback | `PreconditionFailed/ClockRegression` | — | `PreconditionFailed/ClockRegression` |
| production snapshot capability未装配 | 构造期`CapabilityUnavailable/ComponentMissing` | 同 | 同 |
| backend transient unavailable | `Unavailable/EvidenceUnavailable` | 同 | 同 |
| ctx取消、Deadline、读取结果未知 | `Indeterminate/EffectUnknownOutcome` | 同 | 同 |
| Owner返回不可能shape/alias | `Internal/InvalidCanonicalForm` | 同 | 同 |

本表直接使用REV-D11聚合closed set已有组合，不设置第二层错误翻译或新增Reason。NotFound只表示请求中的权威key/exact history确实不存在；Unknown、Unavailable、漂移、过期、retention不允许降NotFound。

Publisher closed矩阵：首建已有current/history、CAS expected不等、same PublishRef换Input为`Conflict/RevisionConflict`；无sealed body变化为`PreconditionFailed/BindingDrift`；association或consumer失效为`PreconditionFailed/BindingNotCertified`；publish receipt exact不存在只返回`NotFound/OwnerMissing`且不授mutation retry；ctx/Deadline/commit结果未知为`Indeterminate/EffectUnknownOutcome`。所有失败均零部分写。

## 12. Import、兼容与迁移

```text
review/* -----------------> runtime/ports
runtime/control ---------> runtime/ports + runtime/core
runtime/ports -----------> runtime/core
```

1. `runtime/ports`不导入`runtime/control`或`review/*`；`review/*`不导入`runtime/control`、`runtime/fakes`；
2. 版本化加法，不修改`ReviewComponentBindingRefV2`、`ProviderBindingCurrentnessPortV2`、`BindingFactPortV2`或既有consumer；consumer current复用Provider nominal只允许exact调用与内嵌原projection，禁止type-pun为Review Source；
3. Provider nominal projection只能作为实现模式参考，禁止alias/conversion/type-pun；
4. Runtime Owner可复用现有concrete Store、校验与digest算法，但必须新增/证明内部同一snapshot能力；现有fake只用于测试；
5. legacy weak Reader、Review `memory`/`internal/testkit`、Runtime ReaderV4均不能升级为production source；
6. production composition只拿Reader及sealed association ref，不拿Publisher/association discovery；回退只停用能力，不覆盖history、不恢复旧Ref或caller Current。

## 13. Hard negatives 与未来门禁

| ID | 反例 | 必须结果 |
|---|---|---|
| BIND-01 | Set+全部Facts renew，projection/index未推进 | Resolve与InspectCurrent复读closure后zero、BindingDrift |
| BIND-02 | selected Fact revoke，index未变 | zero current |
| BIND-03 | 非selected member Fact revoke/expire | all-member closure失败 |
| BIND-04 | Set侧Grant更新、Fact侧旧值 | full Grant mismatch，zero |
| BIND-05 | Fact侧Grant更新、Set侧旧值 | full Grant mismatch，zero |
| BIND-06 | selected capability缺失或重复 | PreconditionFailed/BindingNotCertified |
| BIND-07 | 非selected Fact为唯一最短TTL | projection expiry精确取该值 |
| BIND-08 | Set侧非selected Grant为唯一最短TTL | 精确取该值 |
| BIND-09 | Fact侧非selected Grant为唯一最短TTL | 精确取该值 |
| BIND-10 | current index回到旧Ref但highestRevision更高 | Conflict/RevisionConflict，检测ABA |
| BIND-11 | historical `active,true`被作为current | historical可读，current拒绝 |
| BIND-12 | Resolve lost reply后底层已renew | recovery结果只能开启新S1，不恢复旧Ref |
| BIND-13 | snapshot读取期间clock rollback | zero projection、ClockRegression |
| BIND-14 | Identity literal golden | exact JSON与ProjectionID逐字等于5.2冻结值 |
| BIND-15 | Closure literal golden | exact JSON与ClosureDigest逐字等于5.2冻结值 |
| BIND-16 | `binding_set`改名为`bindingSet`/`binding_set_ref` | `InvalidArgument/InvalidCanonicalForm`；zero |
| BIND-17 | Identity或Closure任一exact字段缺失/重复 | 缺字段为`InvalidArgument/InvalidCanonicalForm`或Validate失败；重复key为`Conflict/DuplicateCanonicalKey`；zero且不产生ID/digest |
| BIND-18 | 首建成功 | Revision=1；receipt/history/highest/current四对象同事务全有 |
| BIND-19 | 首建任一stage失败 | 四对象与底层mutation全无；不得删除回滚 |
| BIND-20 | 64并发同ExpectedCurrent续版 | 仅一CAS成功；loser零sidecar/底层mutation |
| BIND-21 | Create/CAS lost reply | 仅Inspect同PublishRef恢复；NotFound/Unknown不得重调mutation |
| BIND-22 | same PublishRef换Input/digest | Conflict；既有receipt/history不变 |
| BIND-23 | historical exact Ref有效但current index缺失/坏/指向他项 | historical按Ref+Source+Subject+digest仍可读；current失败 |
| BIND-24 | 纯时间跨Expires | ValidateCurrent失败；history/current Ref/Revision/publish count不变 |
| BIND-25 | bound association revoke/漂移 | S1/S2 zero；不接受caller替换association |
| BIND-26 | consumer Binding或capability revoke/换Ref | S1/S2复读失败；zero current |
| BIND-27 | association为唯一最短TTL | Projection.Expires精确取association TTL |
| BIND-28 | consumer Binding为唯一最短TTL | Projection.Expires精确取consumer current TTL |

额外结构oracle：ctx取消零部分结果；修改返回嵌套值不污染Store；Reader mutation call count恒为零；Review静态类型不能调用Publisher。

未来Runtime Owner conformance与Review消费验证必须把BIND-01..28全部映射到：

```text
go test -count=100 ./... -run 'ReviewBindingAuthoritativeCurrent'
go test -race -count=20 ./... -run 'ReviewBindingAuthoritativeCurrent'
go test ./...
go test -race ./...
go vet ./...
```

`vet`仅作静态补充，不替代28个oracle。当前没有Go实现，以上命令未运行且不得宣称通过。

## 14. 准入与审计结论

Review owner-local aggregate YES与Binding候选审计轴严格分离：第三候选审计结论固定为`NO 2/3/1`；第四候选现在仅`READY FOR TWO INDEPENDENT REVIEWS`，不得自标YES。

Binding P0关闭必须同时取得：

1. Runtime Binding Owner逐字段冻结Reader+Publisher+consumer association nominal Port；
2. production Store证明publisher原子事务、同一current snapshot与无分离Inspect fallback；
3. Runtime Owner conformance完成BIND-01..28、ordinary100/race20/full/race/vet；
4. 两次独立审计均为P0/P1/P2=0；
5. Review重新核对live hashes并取得用户Go实现授权。

在此之前固定三轴结论：**Review owner-local aggregate YES 0/0/0；第三候选审计NO 2/3/1、第四候选仅READY；Binding production P0=1未关闭，external P0=6/NO-GO。**
