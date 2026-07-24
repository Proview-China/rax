# Review Result Bundle Current Grounding V2

## 1. 状态与业务目标

- 最高业务输入：`tmp.document/Review.md:141-151`。
- 当前状态：**Review owner-local Go与最终独立复审完成，P0/P1/P2=0；production external Owner adapters/certification/root仍NO-GO**。
- live `ReviewResultBundleV1` 已闭合 create-once、canonical、Target+Case+Bundle 原子存储与 exact historical Inspect；它只证明“某个 Bundle 被 Review Owner 持久化过”，不证明 Artifact、环境、验证范围和 Evidence 在决定实际点仍 current。
- V2 的目标是让结果 Review 能从 Claim 精确定位到 Artifact revision、typed Anchor、Evidence、Environment、Reviewer Context 与 Validation Scope，并在 Verdict CAS 前形成可重验的一致 current cut。
- Review 仍只判定，不 Dispatch、不 Commit、不创建 Artifact/Environment/Scope/Evidence current Fact，也不把截图、录屏或 Provider 回包升级为真值。

## 2. Owner 与非 Owner

| 对象/动作 | 唯一 Owner | Review 的权限 |
|---|---|---|
| `ReviewResultBundleV2`、Claim、Bundle immutable history | Review Verdict Owner | create-once/seal、exact historical Inspect、current聚合与Verdict输入；Bundle自身无current index |
| Artifact body/revision/current、Anchor locator语义 | 对应 Artifact Owner | Review只携带typed exact source ref与locator；Owner复读current并验证locator属于该revision |
| Sandbox Snapshot Artifact | Sandbox Artifact Owner | 只能作为`sandbox snapshot` kind的typed source adapter；不得冒充generic Artifact |
| Continuity `ArtifactRelationFactV1` | Continuity Owner | 只证明relation；不得冒充Artifact body/revision/current或创建generic Artifact current |
| Environment revision/current | 对应 Environment Owner | Review只消费exact current projection；Sandbox可实现`sandbox environment` source adapter |
| Reviewer Context envelope/source | Context Owner | 直接复用`ReviewerContextCurrentReaderV1`；Review不复制Frame/Context事实 |
| Validation Scope revision/current | Validation Scope Owner | 证明“验证了什么、以何方法、覆盖哪个exact Artifact/Anchor”；不得用Runtime execution scope或字符串digest代替 |
| Evidence record、subject、owner fact/current | Evidence Owner | 直接复用`ReviewEvidenceApplicabilityCurrentReaderV1`；Review只引用full `ReviewEvidenceRefV2` |
| host typed-owner router与production root | 宿主composition Owner | 注入只读Reader；closed kind/version，无fallback、无全局mutable registry |

Artifact locator的**语义 Owner就是该Artifact的Owner**。Review可以校验locator wrapper的canonical shape与digest，但不能解释“某行、某页、某DOM节点、某时间区间”是否真实存在于Artifact。

## 3. Review-owned V2 Bundle

以下是未来 Review Go 实现的唯一对象方向；外部 nominal 类型必须先由 Runtime public ports Owner 联合冻结，Review不得把本节复制成私有production兼容接口。

```go
type ReviewResultArtifactBindingV2 struct {
    Source  runtimeports.ReviewArtifactExactSourceRefV2 `json:"source"`
    Anchors []runtimeports.ReviewArtifactLocatorV2       `json:"anchors"`
}

type ReviewResultClaimV2 struct {
    ID        string                                      `json:"id"`
    Statement string                                      `json:"statement"`
    Artifact  runtimeports.ReviewArtifactExactSourceRefV2 `json:"artifact"`
    Anchor    runtimeports.ReviewArtifactLocatorV2        `json:"anchor"`
    Evidence  []runtimeports.ReviewEvidenceRefV2           `json:"evidence"`
}

type ReviewResultBundleV2 struct {
    FactIdentityV1
    Request                 ExactResourceRefV1                         `json:"request"`
    Target                  ExactResourceRefV1                         `json:"target"`
    OriginalIntent          contract.ReviewerContextSourceRefV1       `json:"original_intent"`
    AcceptanceCriteria      []contract.ReviewerContextSourceRefV1     `json:"acceptance_criteria"`
    Artifacts               []ReviewResultArtifactBindingV2           `json:"artifacts"`
    Claims                  []ReviewResultClaimV2                     `json:"claims"`
    Environment             runtimeports.ReviewEnvironmentExactRefV2  `json:"environment"`
    ReviewerContext         contract.ReviewerContextEnvelopeRefV1     `json:"reviewer_context"`
    ReviewerContextSources  []contract.ReviewerContextSourceRefV1     `json:"reviewer_context_sources"`
    ValidationScope         runtimeports.ReviewValidationScopeExactRefV2 `json:"validation_scope"`
    Limitations             []string                                  `json:"limitations"`
    Uncovered               []string                                  `json:"uncovered"`
    EvidenceSetDigest       core.Digest                               `json:"evidence_set_digest"`
    ExpiresUnixNano         int64                                     `json:"expires_unix_nano"`
}
```

### 3.1 canonical与cross-field不变量

1. Bundle revision仍为create-once `1`；新结果形成新Bundle ID，不覆盖历史。
2. Artifacts按`Source.Kind/Owner/ID/Revision/Digest`严格排序；同stable source同revision换digest为Conflict。
3. 每个Artifact的Anchors按`Kind/Schema.Key()/Digest`严格排序、无重复、数量有界；所有slice deep clone。
4. Claims按ID严格排序、无重复；每个Claim的Artifact full ref必须逐字段等于Artifacts中的exact source；Anchor必须逐字段等于该Artifact声明的一个Anchor。
5. 每个声明的Anchor至少被一个Claim引用，禁止无法到达的“装饰性定位”。
6. Claim Evidence使用完整`ReviewEvidenceRefV2`，按full ref排序、无重复；`EvidenceSetDigest`由所有Claim Evidence去重后用live canonical helper计算。
7. `ReviewerContextSources`必须逐字段等于Context Owner exact Inspect返回的Envelope Materials source set，不能由caller自行补齐或删减。
8. `OriginalIntent`必须逐字段等于Context Envelope中唯一`original_intent` material的Source；`AcceptanceCriteria`必须按Owner/ID/revision/digest严格排序、非空，逐字段等于Envelope内全部`acceptance_criterion` materials的Source集合。二者的内容digest由Context Owner material返回，Bundle不得自报`OriginalTaskDigest/AcceptanceDigest`，也不得从当前聊天、latest文件或UI文本重新推断。
9. `Limitations`和`Uncovered`允许为空但必须canonical sort；它们是披露，不会自动降低Rubric或Policy要求。
10. Bundle digest domain：`praxis.review.result-bundle/body/v2`；只清空Bundle自身Digest，完整包含所有typed refs、Anchor payload/digest、Evidence、TTL与披露字段。

## 4. 外部neutral nominal候选

### 4.1 共同Owner坐标

```go
type ReviewGroundingOwnerRefV2 struct {
    Binding        runtimeports.ReviewComponentBindingRefV2 `json:"binding"`
    SourceContract runtimeports.NamespacedNameV2              `json:"source_contract"`
}
func (r ReviewGroundingOwnerRefV2) Validate() error
```

该类型复用live `ReviewComponentBindingRefV2`而不复制Binding字段，只补public source contract名；它不授写权、Authority、Evidence资格或执行能力。各source类型必须是**nominal独立类型**，即便字段相似也禁止互转/type-pun。每个source projection都必须嵌入由live `ReviewBindingAuthoritativeCurrentReaderV1`对`Owner.Binding + stored Assignment/Target subject`返回的完整current projection，并纳入S1/S2与TTL。

### 4.2 Artifact exact ref与typed Anchor

```go
type ReviewArtifactExactSourceRefV2 struct {
    Kind        NamespacedNameV2          `json:"kind"`
    Owner       ReviewGroundingOwnerRefV2 `json:"owner"`
    TenantID    core.TenantID             `json:"tenant_id"`
    ID          string                    `json:"id"`
    Revision    core.Revision             `json:"revision"`
    Digest      core.Digest               `json:"digest"`
    ScopeDigest core.Digest               `json:"scope_digest"`
}

type ReviewArtifactLocatorV2 struct {
    Kind          NamespacedNameV2 `json:"kind"`
    Schema        SchemaRefV2      `json:"schema"`
    Payload       OpaquePayloadV2  `json:"payload"`
    LocatorDigest core.Digest      `json:"locator_digest"`
}
func (r ReviewArtifactExactSourceRefV2) Validate() error
func (v ReviewArtifactLocatorV2) Validate() error
func DigestReviewArtifactLocatorV2(ReviewArtifactLocatorV2) (core.Digest, error)
func SealReviewArtifactLocatorV2(ReviewArtifactLocatorV2) (ReviewArtifactLocatorV2, error)
```

- `LocatorDigest` domain：`praxis.review.artifact-locator/body/v2`，覆盖Kind、Schema与完整OpaquePayload；Review只验证wrapper与digest。
- Owner Adapter必须把所有locator与expected Artifact exact ref一起复读；success projection返回同一canonical locator set digest。
- locator Kind/version由production compiled root的closed declaration选择；unknown/undeclared kind Fail Closed。Review不提供“字符串路径”“latest page”“当前DOM”fallback。
- `OpaquePayloadV2.Ref`只是一段受Schema约束的locator payload引用，仍需Artifact Owner解析和验证；它不是Artifact body ref。

### 4.3 Environment与Validation Scope exact ref

```go
type ReviewEnvironmentExactRefV2 struct {
    Kind        NamespacedNameV2          `json:"kind"`
    Owner       ReviewGroundingOwnerRefV2 `json:"owner"`
    TenantID    core.TenantID             `json:"tenant_id"`
    ID          string                    `json:"id"`
    Revision    core.Revision             `json:"revision"`
    Digest      core.Digest               `json:"digest"`
    ScopeDigest core.Digest               `json:"scope_digest"`
}

type ReviewValidationScopeSourceIdentityV2 struct {
	Kind     NamespacedNameV2 `json:"kind"`
	TenantID core.TenantID    `json:"tenant_id"`
	ID       string           `json:"id"`
}
type ReviewValidationScopeExactRefV2 struct {
	Source      ReviewValidationScopeSourceIdentityV2 `json:"source"`
	Owner       ReviewGroundingOwnerRefV2              `json:"owner"`
	Revision    core.Revision             `json:"revision"`
	Digest      core.Digest               `json:"digest"`
	ScopeDigest core.Digest               `json:"scope_digest"`
}
func (r ReviewEnvironmentExactRefV2) Validate() error
func (i ReviewValidationScopeSourceIdentityV2) Validate() error
func (r ReviewValidationScopeExactRefV2) Validate() error
```

两者是不同nominal类型。Runtime `ReviewDecisionScopeCurrentReaderV1`继续证明“当前执行适用范围”，但不能替代“结果验证覆盖范围”；Sandbox `EnvironmentProjection`只能由Sandbox Adapter无损投影为`sandbox environment` kind，不能被Review直接import或包装成generic current。

`ReviewValidationScopeSourceIdentityV2`是Validation Scope实例唯一的Owner-neutral稳定键；`Owner`绝不参与该键的派生。相同`Source{Kind,TenantID,ID}`只能存在一个current Owner association。两个Owner即使分别构造了不同exact ref，也必须在association create/CAS或root构造时Conflict，不能因为Owner字段不同而被视为两个source。

## 5. REV-D13：三类Owner exact-current窄Port Delta

三类Port遵循同一个原子模型，但Ref、Subject、Projection、Reader和Publisher保持nominal独立，禁止`any`、弱字符串lookup或共享generic DTO。以下候选全部落在Runtime public ports；Review只能持Reader，Publisher只给对应source Fact Owner control path。

### 5.1 公共状态与三个nominal Ref

```go
type ReviewGroundingCurrentStateV2 string
const (
    ReviewGroundingCurrentActiveV2 ReviewGroundingCurrentStateV2 = "active"
    ReviewGroundingCurrentRevokedV2 ReviewGroundingCurrentStateV2 = "revoked"
    ReviewGroundingCurrentSupersededV2 ReviewGroundingCurrentStateV2 = "superseded"
)

type ReviewArtifactCurrentProjectionRefV2 struct { ID string `json:"id"`; Revision core.Revision `json:"revision"`; SubjectDigest core.Digest `json:"subject_digest"`; Digest core.Digest `json:"digest"` }
type ReviewEnvironmentCurrentProjectionRefV2 struct { ID string `json:"id"`; Revision core.Revision `json:"revision"`; SubjectDigest core.Digest `json:"subject_digest"`; Digest core.Digest `json:"digest"` }
type ReviewValidationScopeCurrentProjectionRefV2 struct { ID string `json:"id"`; Revision core.Revision `json:"revision"`; SubjectDigest core.Digest `json:"subject_digest"`; Digest core.Digest `json:"digest"` }

type ReviewArtifactCurrentProjectionIdentityInputV2 struct {
    Expected ReviewArtifactExactSourceRefV2 `json:"expected"`
}
type ReviewEnvironmentCurrentProjectionIdentityInputV2 struct {
    Expected ReviewEnvironmentExactRefV2 `json:"expected"`
}
type ReviewValidationScopeCurrentProjectionIdentityInputV2 struct {
    Source ReviewValidationScopeSourceIdentityV2 `json:"source"`
}
```

三个Ref和三个IdentityInput各自实现`Validate() error`；相同字段不表示可互转。Artifact/Environment的stable Projection ID只由完整exact source ref派生，Anchor、lease seal与current状态不进入identity；Validation Scope的stable Projection ID只由Owner-neutral `Source`派生，Owner、coverage/evidence digest与revision不进入identity。`SubjectDigest`仍覆盖完整Subject，`Digest`等于完整ProjectionDigest。三组IdentityInput的字段名、JSON tag和literal canonical JSON均是合同，禁止用anonymous struct、map或Subject整体替代。

### 5.2 Artifact Owner完整合同

```go
type ReviewArtifactCurrentSubjectV2 struct { Expected ReviewArtifactExactSourceRefV2 `json:"expected"`; Anchors []ReviewArtifactLocatorV2 `json:"anchors"` }
type ReviewArtifactCurrentProjectionV2 struct {
    ContractVersion string `json:"contract_version"`; Ref ReviewArtifactCurrentProjectionRefV2 `json:"ref"`; Subject ReviewArtifactCurrentSubjectV2 `json:"subject"`; Source ReviewArtifactExactSourceRefV2 `json:"source"`
    OwnerBinding runtimeports.ReviewBindingCurrentProjectionV1 `json:"owner_binding"`; LocatorSetDigest core.Digest `json:"locator_set_digest"`; State ReviewGroundingCurrentStateV2 `json:"state"`; Current bool `json:"current"`
    CheckedUnixNano int64 `json:"checked_unix_nano"`; ExpiresUnixNano int64 `json:"expires_unix_nano"`; ProjectionDigest core.Digest `json:"projection_digest"`
}
type ReviewArtifactCurrentReaderV2 interface {
    ResolveCurrentReviewArtifactV2(context.Context, ReviewArtifactCurrentSubjectV2) (ReviewArtifactCurrentProjectionRefV2, error)
    InspectCurrentReviewArtifactV2(context.Context, ReviewArtifactCurrentSubjectV2, ReviewArtifactCurrentProjectionRefV2) (ReviewArtifactCurrentProjectionV2, error)
    InspectHistoricalReviewArtifactV2(context.Context, ReviewArtifactCurrentProjectionRefV2) (ReviewArtifactCurrentProjectionV2, error)
}
func (s ReviewArtifactCurrentSubjectV2) Validate() error
func (p ReviewArtifactCurrentProjectionV2) Clone() ReviewArtifactCurrentProjectionV2
func (p ReviewArtifactCurrentProjectionV2) Validate() error
func (p ReviewArtifactCurrentProjectionV2) ValidateCurrent(ReviewArtifactCurrentProjectionRefV2, ReviewArtifactCurrentSubjectV2, time.Time) error
func DeriveReviewArtifactCurrentProjectionIDV2(ReviewArtifactCurrentProjectionIdentityInputV2) (string, error)
func DigestReviewArtifactCurrentProjectionV2(ReviewArtifactCurrentProjectionV2) (core.Digest, error)
func SealReviewArtifactCurrentProjectionV2(ReviewArtifactCurrentProjectionV2) (ReviewArtifactCurrentProjectionV2, error)
```

Owner必须复读body metadata/current index并验证expected revision/digest、tenant/scope、全部locator；`OwnerBinding.Source == Source.Owner.Binding`且Subject由stored Assignment/Target构造。Snapshot/Relation Adapter只能映射自己真实拥有的kind。

### 5.3 Environment Owner完整合同

```go
type ReviewEnvironmentCurrentSubjectV2 struct { Expected ReviewEnvironmentExactRefV2 `json:"expected"` }
type ReviewEnvironmentCurrentProjectionV2 struct {
    ContractVersion string `json:"contract_version"`; Ref ReviewEnvironmentCurrentProjectionRefV2 `json:"ref"`; Subject ReviewEnvironmentCurrentSubjectV2 `json:"subject"`; Source ReviewEnvironmentExactRefV2 `json:"source"`
    OwnerBinding runtimeports.ReviewBindingCurrentProjectionV1 `json:"owner_binding"`; OwnerLeaseDigest core.Digest `json:"owner_lease_digest"`; State ReviewGroundingCurrentStateV2 `json:"state"`; Current bool `json:"current"`
    CheckedUnixNano int64 `json:"checked_unix_nano"`; ExpiresUnixNano int64 `json:"expires_unix_nano"`; ProjectionDigest core.Digest `json:"projection_digest"`
}
type ReviewEnvironmentCurrentReaderV2 interface {
    ResolveCurrentReviewEnvironmentV2(context.Context, ReviewEnvironmentCurrentSubjectV2) (ReviewEnvironmentCurrentProjectionRefV2, error)
    InspectCurrentReviewEnvironmentV2(context.Context, ReviewEnvironmentCurrentSubjectV2, ReviewEnvironmentCurrentProjectionRefV2) (ReviewEnvironmentCurrentProjectionV2, error)
    InspectHistoricalReviewEnvironmentV2(context.Context, ReviewEnvironmentCurrentProjectionRefV2) (ReviewEnvironmentCurrentProjectionV2, error)
}
func (s ReviewEnvironmentCurrentSubjectV2) Validate() error
func (p ReviewEnvironmentCurrentProjectionV2) Clone() ReviewEnvironmentCurrentProjectionV2
func (p ReviewEnvironmentCurrentProjectionV2) Validate() error
func (p ReviewEnvironmentCurrentProjectionV2) ValidateCurrent(ReviewEnvironmentCurrentProjectionRefV2, ReviewEnvironmentCurrentSubjectV2, time.Time) error
func DeriveReviewEnvironmentCurrentProjectionIDV2(ReviewEnvironmentCurrentProjectionIdentityInputV2) (string, error)
func DigestReviewEnvironmentCurrentProjectionV2(ReviewEnvironmentCurrentProjectionV2) (core.Digest, error)
func SealReviewEnvironmentCurrentProjectionV2(ReviewEnvironmentCurrentProjectionV2) (ReviewEnvironmentCurrentProjectionV2, error)
```

Sandbox Adapter每次仍需复读真实`EnvironmentProjection.Meta/Lease` current，`OwnerLeaseDigest`由Sandbox Owner sealed source产生；不得缓存对象快照或由Review补签。

### 5.4 Validation Scope Owner完整合同

Validation Scope的唯一实例Owner由Owner-neutral `ReviewValidationScopeSourceIdentityV2`的权威current association确定。association的Fact Owner属于Validation Scope registry/composition control plane；Runtime public ports只拥有neutral合同，Review、Runtime execution-scope Owner、Evidence Owner和具体source Provider都不能写association。association指向声明`praxis.review/validation-scope-current-v2` capability的full `ReviewGroundingOwnerRefV2`。root对一个Source identity只允许一个current Owner binding。

```go
type ReviewValidationScopeOwnerAssociationRefV2 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"`
    Digest   core.Digest   `json:"digest"`
}
type ReviewValidationScopeOwnerAssociationSubjectV2 struct {
    Source ReviewValidationScopeSourceIdentityV2 `json:"source"`
}
type ReviewValidationScopeOwnerAssociationCurrentProjectionV2 struct {
    ContractVersion  string                                             `json:"contract_version"`
    Ref              ReviewValidationScopeOwnerAssociationRefV2        `json:"ref"`
    Subject          ReviewValidationScopeOwnerAssociationSubjectV2    `json:"subject"`
    Owner            ReviewGroundingOwnerRefV2                          `json:"owner"`
    Current          bool                                               `json:"current"`
    CheckedUnixNano  int64                                              `json:"checked_unix_nano"`
    ExpiresUnixNano  int64                                              `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                                        `json:"projection_digest"`
}
type ReviewValidationScopeOwnerAssociationCurrentReaderV2 interface {
    ResolveCurrentReviewValidationScopeOwnerAssociationV2(context.Context, ReviewValidationScopeOwnerAssociationSubjectV2) (ReviewValidationScopeOwnerAssociationRefV2, error)
    InspectCurrentReviewValidationScopeOwnerAssociationV2(context.Context, ReviewValidationScopeOwnerAssociationSubjectV2, ReviewValidationScopeOwnerAssociationRefV2) (ReviewValidationScopeOwnerAssociationCurrentProjectionV2, error)
    InspectHistoricalReviewValidationScopeOwnerAssociationV2(context.Context, ReviewValidationScopeOwnerAssociationRefV2) (ReviewValidationScopeOwnerAssociationCurrentProjectionV2, error)
}
type ReviewValidationScopeOwnerAssociationPublishRefV2 struct {
    ID     string      `json:"id"`
    Digest core.Digest `json:"digest"`
}
type CreateReviewValidationScopeOwnerAssociationRequestV2 struct {
    PublishRef ReviewValidationScopeOwnerAssociationPublishRefV2          `json:"publish_ref"`
    Value      ReviewValidationScopeOwnerAssociationCurrentProjectionV2   `json:"value"`
}
type CompareAndSwapReviewValidationScopeOwnerAssociationRequestV2 struct {
    PublishRef ReviewValidationScopeOwnerAssociationPublishRefV2          `json:"publish_ref"`
    Expected   ReviewValidationScopeOwnerAssociationRefV2                 `json:"expected"`
    Value      ReviewValidationScopeOwnerAssociationCurrentProjectionV2   `json:"value"`
}
type ReviewValidationScopeOwnerAssociationPublishReceiptV2 struct {
    ContractVersion string                                              `json:"contract_version"`
    PublishRef      ReviewValidationScopeOwnerAssociationPublishRefV2  `json:"publish_ref"`
    Projection      ReviewValidationScopeOwnerAssociationRefV2         `json:"projection"`
    CurrentIndex    ReviewValidationScopeOwnerAssociationRefV2         `json:"current_index"`
    HighestRevision core.Revision                                      `json:"highest_revision"`
    Digest          core.Digest                                        `json:"digest"`
}
type ReviewValidationScopeOwnerAssociationPublisherV2 interface {
    CreateReviewValidationScopeOwnerAssociationV2(context.Context, CreateReviewValidationScopeOwnerAssociationRequestV2) (ReviewValidationScopeOwnerAssociationPublishReceiptV2, error)
    CompareAndSwapReviewValidationScopeOwnerAssociationV2(context.Context, CompareAndSwapReviewValidationScopeOwnerAssociationRequestV2) (ReviewValidationScopeOwnerAssociationPublishReceiptV2, error)
    InspectReviewValidationScopeOwnerAssociationPublishV2(context.Context, ReviewValidationScopeOwnerAssociationPublishRefV2) (ReviewValidationScopeOwnerAssociationPublishReceiptV2, error)
}
func (r ReviewValidationScopeOwnerAssociationRefV2) Validate() error
func (s ReviewValidationScopeOwnerAssociationSubjectV2) Validate() error
func (p ReviewValidationScopeOwnerAssociationCurrentProjectionV2) Clone() ReviewValidationScopeOwnerAssociationCurrentProjectionV2
func (p ReviewValidationScopeOwnerAssociationCurrentProjectionV2) Validate() error
func (p ReviewValidationScopeOwnerAssociationCurrentProjectionV2) ValidateCurrent(ReviewValidationScopeOwnerAssociationRefV2, ReviewValidationScopeOwnerAssociationSubjectV2, ReviewGroundingOwnerRefV2, time.Time) error
func DeriveReviewValidationScopeOwnerAssociationIDV2(ReviewValidationScopeOwnerAssociationSubjectV2) (string, error)
func DigestReviewValidationScopeOwnerAssociationCurrentProjectionV2(ReviewValidationScopeOwnerAssociationCurrentProjectionV2) (core.Digest, error)
func SealReviewValidationScopeOwnerAssociationCurrentProjectionV2(ReviewValidationScopeOwnerAssociationCurrentProjectionV2) (ReviewValidationScopeOwnerAssociationCurrentProjectionV2, error)
func (r ReviewValidationScopeOwnerAssociationPublishRefV2) Validate() error
func (r CreateReviewValidationScopeOwnerAssociationRequestV2) Validate() error
func (r CompareAndSwapReviewValidationScopeOwnerAssociationRequestV2) Validate() error
func (r ReviewValidationScopeOwnerAssociationPublishReceiptV2) Validate() error
func (r ReviewValidationScopeOwnerAssociationPublishReceiptV2) DigestV2() (core.Digest, error)
func DeriveCreateReviewValidationScopeOwnerAssociationPublishRefV2(ReviewValidationScopeOwnerAssociationCurrentProjectionV2) (ReviewValidationScopeOwnerAssociationPublishRefV2, error)
func DeriveCompareAndSwapReviewValidationScopeOwnerAssociationPublishRefV2(ReviewValidationScopeOwnerAssociationRefV2, ReviewValidationScopeOwnerAssociationCurrentProjectionV2) (ReviewValidationScopeOwnerAssociationPublishRefV2, error)
func SealReviewValidationScopeOwnerAssociationPublishReceiptV2(ReviewValidationScopeOwnerAssociationPublishReceiptV2) (ReviewValidationScopeOwnerAssociationPublishReceiptV2, error)
```

association projection是状态变化时create-once/sealed的immutable projection，stable ID由完整`Subject.Source`派生；revision严格`+1`，history/highest/current full-ref/receipt同事务CAS。canonical domain=`praxis.runtime.review-validation-scope-owner-association-current`、contract=`praxis.runtime.review-validation-scope-owner-association-current/v2`、body为完整projection且只清空`Ref.Digest/ProjectionDigest`；PublishRef由完整具名Create或CAS input派生，Receipt digest只清空Receipt自身Digest。`ValidateCurrent`要求full Ref、Subject、Owner、`Current=true`、`0<Checked<Expires`和fresh now；Expires严格取association Fact/Owner Binding/consumer association/selected capability Grant最短TTL。纯时间到期只让current验证失败，不发布新revision。Owner换绑必须新revision并supersede旧current，旧exact history仍可读。lost publish reply只Inspect同canonical PublishRef；lost read遵循本文件统一S1/S2恢复规则。

```go
type ReviewValidationScopeCurrentSubjectV2 struct { Expected ReviewValidationScopeExactRefV2 `json:"expected"`; CoveredArtifactLocatorSetDigest core.Digest `json:"covered_artifact_locator_set_digest"`; EvidenceSetDigest core.Digest `json:"evidence_set_digest"` }
type ReviewValidationScopeCurrentProjectionV2 struct {
    ContractVersion string `json:"contract_version"`; Ref ReviewValidationScopeCurrentProjectionRefV2 `json:"ref"`; Subject ReviewValidationScopeCurrentSubjectV2 `json:"subject"`; Source ReviewValidationScopeExactRefV2 `json:"source"`
    OwnerBinding runtimeports.ReviewBindingCurrentProjectionV1 `json:"owner_binding"`; ValidationMethod runtimeports.SchemaRefV2 `json:"validation_method"`; CoveredArtifactLocatorSetDigest core.Digest `json:"covered_artifact_locator_set_digest"`; EvidenceSetDigest core.Digest `json:"evidence_set_digest"`
    State ReviewGroundingCurrentStateV2 `json:"state"`; Current bool `json:"current"`; CheckedUnixNano int64 `json:"checked_unix_nano"`; ExpiresUnixNano int64 `json:"expires_unix_nano"`; ProjectionDigest core.Digest `json:"projection_digest"`
}
type ReviewValidationScopeCurrentReaderV2 interface {
    ResolveCurrentReviewValidationScopeV2(context.Context, ReviewValidationScopeCurrentSubjectV2) (ReviewValidationScopeCurrentProjectionRefV2, error)
    InspectCurrentReviewValidationScopeV2(context.Context, ReviewValidationScopeCurrentSubjectV2, ReviewValidationScopeCurrentProjectionRefV2) (ReviewValidationScopeCurrentProjectionV2, error)
    InspectHistoricalReviewValidationScopeV2(context.Context, ReviewValidationScopeCurrentProjectionRefV2) (ReviewValidationScopeCurrentProjectionV2, error)
}
func (s ReviewValidationScopeCurrentSubjectV2) Validate() error
func (p ReviewValidationScopeCurrentProjectionV2) Clone() ReviewValidationScopeCurrentProjectionV2
func (p ReviewValidationScopeCurrentProjectionV2) Validate() error
func (p ReviewValidationScopeCurrentProjectionV2) ValidateCurrent(ReviewValidationScopeCurrentProjectionRefV2, ReviewValidationScopeCurrentSubjectV2, time.Time) error
func DeriveReviewValidationScopeCurrentProjectionIDV2(ReviewValidationScopeCurrentProjectionIdentityInputV2) (string, error)
func DigestReviewValidationScopeCurrentProjectionV2(ReviewValidationScopeCurrentProjectionV2) (core.Digest, error)
func SealReviewValidationScopeCurrentProjectionV2(ReviewValidationScopeCurrentProjectionV2) (ReviewValidationScopeCurrentProjectionV2, error)
```

`ReviewValidationScopeCurrentSubjectV2.Expected.Source`、Owner association的`Subject.Source`及projection identity input的`Source`必须逐字段相等；`Expected.Owner`必须逐字段等于S1/S2复读的association Owner和source projection `OwnerBinding.Source`。任一不等为`Conflict + BindingDrift`，整批零aggregate/Verdict。

三类Projection ID的literal golden输入固定如下，测试必须逐字比较canonical JSON与派生ID；删除字段、改JSON tag、使用错误nominal类型或把Subject整体传入均失败：

```json
{"expected":{"kind":"praxis.artifact/code","owner":{"binding":{"binding_set_id":"set-a","binding_set_revision":7,"component_id":"praxis.artifact/code-owner","manifest_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","artifact_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","capability":"praxis.artifact/current"},"source_contract":"praxis.artifact/current-v2"},"tenant_id":"tenant-a","id":"artifact-a","revision":9,"digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","scope_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"}}
expected ID: `sha256:01cf0b40f11c76489cdeb368ec8909efbf603ae333916b95a9a1ed56ac0ba9c3`

{"expected":{"kind":"praxis.environment/sandbox","owner":{"binding":{"binding_set_id":"set-a","binding_set_revision":7,"component_id":"praxis.sandbox/environment-owner","manifest_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","artifact_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","capability":"praxis.environment/current"},"source_contract":"praxis.environment/current-v2"},"tenant_id":"tenant-a","id":"environment-a","revision":3,"digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","scope_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"}}
expected ID: `sha256:66bffe251c5759f9a64bec3af11ba44fadab24aa7f9bc01e02d0f914c9cbd369`

{"source":{"kind":"praxis.validation/test-coverage","tenant_id":"tenant-a","id":"validation-scope-a"}}
expected ID: `sha256:f133bc4b9da38df6a69d194d4166ff0b43911ba630c4005dcdc421638fb303ca`
```

### 5.5 Owner-only Publisher与原子闭包

每种projection分别具备以下完整nominal闭包；`PublishRef={ID,Digest}`由完整command input canonical派生：

```go
type ReviewArtifactCurrentProjectionPublishRefV2 struct { ID string `json:"id"`; Digest core.Digest `json:"digest"` }
type CreateReviewArtifactCurrentProjectionRequestV2 struct { PublishRef ReviewArtifactCurrentProjectionPublishRefV2 `json:"publish_ref"`; Value ReviewArtifactCurrentProjectionV2 `json:"value"` }
type CompareAndSwapReviewArtifactCurrentProjectionRequestV2 struct { PublishRef ReviewArtifactCurrentProjectionPublishRefV2 `json:"publish_ref"`; Expected ReviewArtifactCurrentProjectionRefV2 `json:"expected"`; Value ReviewArtifactCurrentProjectionV2 `json:"value"` }
type ReviewArtifactCurrentProjectionPublishReceiptV2 struct { ContractVersion string `json:"contract_version"`; PublishRef ReviewArtifactCurrentProjectionPublishRefV2 `json:"publish_ref"`; Projection ReviewArtifactCurrentProjectionRefV2 `json:"projection"`; CurrentIndex ReviewArtifactCurrentProjectionRefV2 `json:"current_index"`; HighestRevision core.Revision `json:"highest_revision"`; Digest core.Digest `json:"digest"` }

type ReviewEnvironmentCurrentProjectionPublishRefV2 struct { ID string `json:"id"`; Digest core.Digest `json:"digest"` }
type CreateReviewEnvironmentCurrentProjectionRequestV2 struct { PublishRef ReviewEnvironmentCurrentProjectionPublishRefV2 `json:"publish_ref"`; Value ReviewEnvironmentCurrentProjectionV2 `json:"value"` }
type CompareAndSwapReviewEnvironmentCurrentProjectionRequestV2 struct { PublishRef ReviewEnvironmentCurrentProjectionPublishRefV2 `json:"publish_ref"`; Expected ReviewEnvironmentCurrentProjectionRefV2 `json:"expected"`; Value ReviewEnvironmentCurrentProjectionV2 `json:"value"` }
type ReviewEnvironmentCurrentProjectionPublishReceiptV2 struct { ContractVersion string `json:"contract_version"`; PublishRef ReviewEnvironmentCurrentProjectionPublishRefV2 `json:"publish_ref"`; Projection ReviewEnvironmentCurrentProjectionRefV2 `json:"projection"`; CurrentIndex ReviewEnvironmentCurrentProjectionRefV2 `json:"current_index"`; HighestRevision core.Revision `json:"highest_revision"`; Digest core.Digest `json:"digest"` }

type ReviewValidationScopeCurrentProjectionPublishRefV2 struct { ID string `json:"id"`; Digest core.Digest `json:"digest"` }
type CreateReviewValidationScopeCurrentProjectionRequestV2 struct { PublishRef ReviewValidationScopeCurrentProjectionPublishRefV2 `json:"publish_ref"`; Value ReviewValidationScopeCurrentProjectionV2 `json:"value"` }
type CompareAndSwapReviewValidationScopeCurrentProjectionRequestV2 struct { PublishRef ReviewValidationScopeCurrentProjectionPublishRefV2 `json:"publish_ref"`; Expected ReviewValidationScopeCurrentProjectionRefV2 `json:"expected"`; Value ReviewValidationScopeCurrentProjectionV2 `json:"value"` }
type ReviewValidationScopeCurrentProjectionPublishReceiptV2 struct { ContractVersion string `json:"contract_version"`; PublishRef ReviewValidationScopeCurrentProjectionPublishRefV2 `json:"publish_ref"`; Projection ReviewValidationScopeCurrentProjectionRefV2 `json:"projection"`; CurrentIndex ReviewValidationScopeCurrentProjectionRefV2 `json:"current_index"`; HighestRevision core.Revision `json:"highest_revision"`; Digest core.Digest `json:"digest"` }

type ReviewArtifactCurrentProjectionPublisherV2 interface { CreateReviewArtifactCurrentProjectionV2(context.Context, CreateReviewArtifactCurrentProjectionRequestV2) (ReviewArtifactCurrentProjectionPublishReceiptV2, error); CompareAndSwapReviewArtifactCurrentProjectionV2(context.Context, CompareAndSwapReviewArtifactCurrentProjectionRequestV2) (ReviewArtifactCurrentProjectionPublishReceiptV2, error); InspectReviewArtifactCurrentProjectionPublishV2(context.Context, ReviewArtifactCurrentProjectionPublishRefV2) (ReviewArtifactCurrentProjectionPublishReceiptV2, error) }
type ReviewEnvironmentCurrentProjectionPublisherV2 interface { CreateReviewEnvironmentCurrentProjectionV2(context.Context, CreateReviewEnvironmentCurrentProjectionRequestV2) (ReviewEnvironmentCurrentProjectionPublishReceiptV2, error); CompareAndSwapReviewEnvironmentCurrentProjectionV2(context.Context, CompareAndSwapReviewEnvironmentCurrentProjectionRequestV2) (ReviewEnvironmentCurrentProjectionPublishReceiptV2, error); InspectReviewEnvironmentCurrentProjectionPublishV2(context.Context, ReviewEnvironmentCurrentProjectionPublishRefV2) (ReviewEnvironmentCurrentProjectionPublishReceiptV2, error) }
type ReviewValidationScopeCurrentProjectionPublisherV2 interface { CreateReviewValidationScopeCurrentProjectionV2(context.Context, CreateReviewValidationScopeCurrentProjectionRequestV2) (ReviewValidationScopeCurrentProjectionPublishReceiptV2, error); CompareAndSwapReviewValidationScopeCurrentProjectionV2(context.Context, CompareAndSwapReviewValidationScopeCurrentProjectionRequestV2) (ReviewValidationScopeCurrentProjectionPublishReceiptV2, error); InspectReviewValidationScopeCurrentProjectionPublishV2(context.Context, ReviewValidationScopeCurrentProjectionPublishRefV2) (ReviewValidationScopeCurrentProjectionPublishReceiptV2, error) }

func (r ReviewArtifactCurrentProjectionPublishRefV2) Validate() error
func (r CreateReviewArtifactCurrentProjectionRequestV2) Validate() error
func (r CompareAndSwapReviewArtifactCurrentProjectionRequestV2) Validate() error
func (r ReviewArtifactCurrentProjectionPublishReceiptV2) Validate() error
func (r ReviewArtifactCurrentProjectionPublishReceiptV2) DigestV2() (core.Digest, error)
func DeriveCreateReviewArtifactCurrentProjectionPublishRefV2(ReviewArtifactCurrentProjectionV2) (ReviewArtifactCurrentProjectionPublishRefV2, error)
func DeriveCompareAndSwapReviewArtifactCurrentProjectionPublishRefV2(ReviewArtifactCurrentProjectionRefV2, ReviewArtifactCurrentProjectionV2) (ReviewArtifactCurrentProjectionPublishRefV2, error)

func (r ReviewEnvironmentCurrentProjectionPublishRefV2) Validate() error
func (r CreateReviewEnvironmentCurrentProjectionRequestV2) Validate() error
func (r CompareAndSwapReviewEnvironmentCurrentProjectionRequestV2) Validate() error
func (r ReviewEnvironmentCurrentProjectionPublishReceiptV2) Validate() error
func (r ReviewEnvironmentCurrentProjectionPublishReceiptV2) DigestV2() (core.Digest, error)
func DeriveCreateReviewEnvironmentCurrentProjectionPublishRefV2(ReviewEnvironmentCurrentProjectionV2) (ReviewEnvironmentCurrentProjectionPublishRefV2, error)
func DeriveCompareAndSwapReviewEnvironmentCurrentProjectionPublishRefV2(ReviewEnvironmentCurrentProjectionRefV2, ReviewEnvironmentCurrentProjectionV2) (ReviewEnvironmentCurrentProjectionPublishRefV2, error)

func (r ReviewValidationScopeCurrentProjectionPublishRefV2) Validate() error
func (r CreateReviewValidationScopeCurrentProjectionRequestV2) Validate() error
func (r CompareAndSwapReviewValidationScopeCurrentProjectionRequestV2) Validate() error
func (r ReviewValidationScopeCurrentProjectionPublishReceiptV2) Validate() error
func (r ReviewValidationScopeCurrentProjectionPublishReceiptV2) DigestV2() (core.Digest, error)
func DeriveCreateReviewValidationScopeCurrentProjectionPublishRefV2(ReviewValidationScopeCurrentProjectionV2) (ReviewValidationScopeCurrentProjectionPublishRefV2, error)
func DeriveCompareAndSwapReviewValidationScopeCurrentProjectionPublishRefV2(ReviewValidationScopeCurrentProjectionRefV2, ReviewValidationScopeCurrentProjectionV2) (ReviewValidationScopeCurrentProjectionPublishRefV2, error)
```

每个Receipt另有具名`SealReview{Artifact|Environment|ValidationScope}CurrentProjectionPublishReceiptV2`。domain沿用对应projection domain，body discriminator使用完整具名request input，禁止字段名漂移或弱idempotency字符串。

首建只允许revision=1；续版必须同stable ID且严格`expected+1`。Owner在同一事务先验证/封装source fact、OwnerBinding、projection与PublishReceipt，再原子写history/highestRevision/current full-ref/receipt；任一失败四者零写。publish reply丢失只`Inspect...Publish(PublishRef)`，exact receipt匹配即恢复，NotFound只有Owner线性化证明从未提交时才允许重投**同一canonical PublishRef**；unknown/unavailable不得重投。纯时间到期不发布新revision。

### 5.6 digest、TTL与closed errors

- 三个canonical domain分别为`praxis.runtime.review-artifact-current`、`praxis.runtime.review-environment-current`、`praxis.runtime.review-validation-scope-current`；contract分别为同名`/v2`；body为完整nominal projection，计算时只清空Ref.Digest/ProjectionDigest。
- `State=active`当且仅当`Current=true`；terminal历史Current字段保持sealed值，但`InspectCurrent/ValidateCurrent`必须拒绝非active或index不等于expected。`0 < Checked < Expires`；Expires必须等于`min(source fact, OwnerBinding及其consumer/grant closure, owner lease, locator/coverage/evidence source TTL)`中的适用项。
- success deep clone所有slice/pointer；same ID/revision不同digest Conflict；historical exact不借current index；旧history不可覆盖或复活。

以下矩阵对Artifact、Environment、Validation Scope projection及Validation Scope Owner association的同名nominal方法分别成立，不允许Owner/adapter自行换Reason：

| 方法 | 条件 | 唯一 Category + Reason |
|---|---|---|
| `Ref/Subject/IdentityInput/Projection/PublishRef/Request/Receipt.Validate` | 缺字段、零revision、坏namespace/shape | `InvalidArgument + InvalidReference` |
| 上述`Validate` | 输入未canonical、slice乱序/重复 | `InvalidArgument + InvalidCanonicalForm`；重复canonical key为`Conflict + DuplicateCanonicalKey` |
| 上述`Validate` | caller提供非法digest shape | `InvalidArgument + InvalidDigest` |
| `Projection/Receipt.Validate` | sealed Ref/Subject/Projection/Receipt digest不一致 | `Conflict + InvalidDigest` |
| 所有`Derive*ID/Derive*PublishRef/Derive*ReaderBindingRef` | 具名input的`Validate`失败 | 原样返回该input冻结的closed error；禁止改写为NotFound/Internal |
| 所有`Digest*`/`DigestV2` | value shape/canonical/digest前置校验失败 | 对应`InvalidArgument + InvalidReference/InvalidCanonicalForm/InvalidDigest`；sealed字段互相冲突为`Conflict + InvalidDigest` |
| 所有`Seal*Projection/Seal*Receipt/Seal*Catalog` | supplied ID/revision/digest与重算值不一致 | `Conflict + InvalidDigest`；非canonical shape仍为对应`InvalidArgument`closed reason |
| `ResolvedRouteProof.Validate` | Declaration/full Owner、Route、ReaderBinding.Route或adapter digest不一致 | `Conflict + BindingDrift` |
| `ResolvedRoute.Validate` | Proof有效但Reader为nil/typed-nil或nominal family不匹配 | `PreconditionFailed + OwnerMissing` |
| `ResolveCurrent...` | stable identity从未有history/current index | `NotFound + ReviewVerdictMissing` |
| `ResolveCurrent...` | history存在但无active current、terminal或TTL已失效 | `PreconditionFailed + ReviewVerdictStale` |
| `InspectCurrent...` | exact projection ID从未存在 | `NotFound + ReviewVerdictMissing` |
| `InspectCurrent...` | ID存在但expected revision不在history | `Conflict + RevisionConflict` |
| `InspectCurrent...` | revision存在但digest/SubjectDigest不同 | `Conflict + InvalidDigest` |
| `InspectCurrent...` | current index不等于expected、highestRevision回退或ABA | `Conflict + BindingDrift` |
| `InspectCurrent...` | source fact、locator/coverage、Owner association或Owner Binding current漂移 | `Conflict + BindingDrift` |
| `InspectCurrent.../ValidateCurrent` | terminal、revoked、superseded、TTL crossing | `PreconditionFailed + ReviewVerdictStale` |
| `InspectHistorical...` | exact projection ID从未存在 | `NotFound + ReviewVerdictMissing` |
| `InspectHistorical...` | ID存在但revision不存在 | `Conflict + RevisionConflict` |
| `InspectHistorical...` | revision存在但digest/SubjectDigest不同 | `Conflict + InvalidDigest` |
| `Create...` | stable ID已有history/current | `Conflict + AlreadyExists`；同PublishRef同canonical只返回原Receipt |
| `Create...` | Value revision不是1 | `Conflict + RevisionConflict` |
| `Create...` | PublishRef不由完整Value派生 | `Conflict + IdempotencyPayloadMismatch` |
| `CompareAndSwap...` | Expected不是current full Ref、next revision不是`expected+1`、highestRevision不单调 | `Conflict + RevisionConflict` |
| `CompareAndSwap...` | 同PublishRef换Expected/Value或same revision换digest | `Conflict + IdempotencyPayloadMismatch` |
| `Inspect...Publish` | PublishRef ID从未存在且Owner线性化证明未提交 | `NotFound + ReviewVerdictMissing` |
| `Inspect...Publish` | ID存在但PublishRef digest不同 | `Conflict + IdempotencyPayloadMismatch` |
| `NewReviewGroundingReaderResolverV2` | catalog缺required binding、多binding、错误family、nil/typed-nil Reader | `PreconditionFailed + OwnerMissing` |
| `NewReviewGroundingReaderResolverV2` | duplicate route/alias、full Owner或ReaderBinding冲突 | `Conflict + BindingDrift` |
| `ResolveReview*ReaderV2` | request full Owner/Family/Kind无exact declared route | `Forbidden + OwnerConflict` |
| `ResultBundleCurrentGroundingRequestV2.Validate` | exact refs/Run/Scope/Evidence不完整、乱序、重复或digest不匹配 | 对应`InvalidArgument + InvalidReference/InvalidCanonicalForm/InvalidDigest`，重复为`Conflict + DuplicateCanonicalKey` |
| `ResultBundleGroundingStoredFactsV2.ValidateAgainst` | 任一stored Ref、Tenant、Run/Scope、Evidence与request漂移 | `Conflict + BindingDrift` |
| `ResultBundleCurrentGroundingDependenciesV2.Validate`/`NewResultBundleCurrentGroundingReaderV2` | 任一必需Reader、Resolver、Clock为nil/typed-nil，或recovery policy非法 | `PreconditionFailed + OwnerMissing`；policy shape/timeout非法为`InvalidArgument + InvalidReference` |
| `InspectResultBundleCurrentGroundingV2` | stored exact Fact不存在 | `NotFound + ReviewVerdictMissing`；ordinary/eventual/retention不明不得声称authoritative NotFound |
| `InspectResultBundleCurrentGroundingV2` | stored/external S1/S2、route Proof、association或Binding闭包漂移 | `Conflict + BindingDrift`；TTL/terminal为`PreconditionFailed + ReviewVerdictStale` |
| `ResultBundleCurrentGroundingProjectionV2.Validate/ValidateCurrent` | route Proof/association/source projection缺失或digest/minTTL不一致 | `Conflict + BindingDrift/InvalidDigest`；zero/rollback clock为`PreconditionFailed + ClockRegression`；过期为`PreconditionFailed + ReviewVerdictStale` |
| 任一方法 | cross-tenant/scope、undeclared capability/SourceContract | `Forbidden + OwnerConflict` |
| 任一需要clock的方法 | zero/rollback clock | `PreconditionFailed + ClockRegression` |
| 任一Reader/Publisher | ctx取消、deadline、unknown backend/outcome | `Indeterminate + InspectCoverageIncomplete` |
| 任一Reader/Publisher | 已知Owner backend不可用 | `Unavailable + OwnerMissing` |

`NotFound`不能表示terminal、deny、retention不明、eventual read或unknown。

无`error`返回值的`Clone`不进入错误分类，但必须deep clone全部slice/pointer并保留sealed值；任何新增public error-returning方法在加入候选前必须先扩本矩阵与`RB-ERR-*` oracle，不能依赖实现自行选Category/Reason。

## 6. host typed-owner router

```go
type ReviewGroundingRouteFamilyV2 string
const ( ReviewGroundingArtifactRouteV2 ReviewGroundingRouteFamilyV2 = "artifact"; ReviewGroundingEnvironmentRouteV2 ReviewGroundingRouteFamilyV2 = "environment"; ReviewGroundingValidationScopeRouteV2 ReviewGroundingRouteFamilyV2 = "validation_scope" )
type ReviewGroundingRouteDeclarationV2 struct { Family ReviewGroundingRouteFamilyV2 `json:"family"`; Kind runtimeports.NamespacedNameV2 `json:"kind"`; Owner ReviewGroundingOwnerRefV2 `json:"owner"`; Required bool `json:"required"` }
type ReviewGroundingRouteRequestV2 struct { Family ReviewGroundingRouteFamilyV2 `json:"family"`; Kind runtimeports.NamespacedNameV2 `json:"kind"`; Owner ReviewGroundingOwnerRefV2 `json:"owner"` }
type ReviewGroundingRouteRefV2 struct { ID string `json:"id"`; Revision core.Revision `json:"revision"`; Digest core.Digest `json:"digest"` }
type ReviewGroundingReaderBindingRefV2 struct { ID string `json:"id"`; Revision core.Revision `json:"revision"`; Route ReviewGroundingRouteRefV2 `json:"route"`; AdapterArtifactDigest core.Digest `json:"adapter_artifact_digest"`; Digest core.Digest `json:"digest"` }
type ReviewArtifactResolvedRouteProofV2 struct { Declaration ReviewGroundingRouteDeclarationV2 `json:"declaration"`; Route ReviewGroundingRouteRefV2 `json:"route"`; ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"` }
type ReviewEnvironmentResolvedRouteProofV2 struct { Declaration ReviewGroundingRouteDeclarationV2 `json:"declaration"`; Route ReviewGroundingRouteRefV2 `json:"route"`; ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"` }
type ReviewValidationScopeResolvedRouteProofV2 struct { Declaration ReviewGroundingRouteDeclarationV2 `json:"declaration"`; Route ReviewGroundingRouteRefV2 `json:"route"`; ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"` }
type ReviewArtifactResolvedRouteV2 struct { Proof ReviewArtifactResolvedRouteProofV2 `json:"proof"`; Reader ReviewArtifactCurrentReaderV2 `json:"-"` }
type ReviewEnvironmentResolvedRouteV2 struct { Proof ReviewEnvironmentResolvedRouteProofV2 `json:"proof"`; Reader ReviewEnvironmentCurrentReaderV2 `json:"-"` }
type ReviewValidationScopeResolvedRouteV2 struct { Proof ReviewValidationScopeResolvedRouteProofV2 `json:"proof"`; Reader ReviewValidationScopeCurrentReaderV2 `json:"-"` }
type ReviewArtifactRouteBindingV2 struct { Declaration ReviewGroundingRouteDeclarationV2 `json:"declaration"`; ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"`; Reader ReviewArtifactCurrentReaderV2 `json:"-"` }
type ReviewEnvironmentRouteBindingV2 struct { Declaration ReviewGroundingRouteDeclarationV2 `json:"declaration"`; ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"`; Reader ReviewEnvironmentCurrentReaderV2 `json:"-"` }
type ReviewValidationScopeRouteBindingV2 struct { Declaration ReviewGroundingRouteDeclarationV2 `json:"declaration"`; ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"`; Reader ReviewValidationScopeCurrentReaderV2 `json:"-"` }
type ReviewGroundingRequiredRouteCatalogV2 struct { ContractVersion string `json:"contract_version"`; Artifact []ReviewGroundingRouteDeclarationV2 `json:"artifact"`; Environment []ReviewGroundingRouteDeclarationV2 `json:"environment"`; ValidationScope []ReviewGroundingRouteDeclarationV2 `json:"validation_scope"`; Digest core.Digest `json:"digest"` }
type ReviewGroundingReaderResolverV2 interface { ResolveReviewArtifactReaderV2(context.Context, ReviewGroundingRouteRequestV2) (ReviewArtifactResolvedRouteV2, error); ResolveReviewEnvironmentReaderV2(context.Context, ReviewGroundingRouteRequestV2) (ReviewEnvironmentResolvedRouteV2, error); ResolveReviewValidationScopeReaderV2(context.Context, ReviewGroundingRouteRequestV2) (ReviewValidationScopeResolvedRouteV2, error) }
func NewReviewGroundingReaderResolverV2(ReviewGroundingRequiredRouteCatalogV2, []ReviewArtifactRouteBindingV2, []ReviewEnvironmentRouteBindingV2, []ReviewValidationScopeRouteBindingV2) (ReviewGroundingReaderResolverV2, error)
func (d ReviewGroundingRouteDeclarationV2) Validate() error
func (r ReviewGroundingRouteRequestV2) Validate() error
func (r ReviewGroundingRouteRefV2) Validate() error
func (r ReviewGroundingReaderBindingRefV2) Validate() error
func (p ReviewArtifactResolvedRouteProofV2) Validate() error
func (p ReviewEnvironmentResolvedRouteProofV2) Validate() error
func (p ReviewValidationScopeResolvedRouteProofV2) Validate() error
func (r ReviewArtifactResolvedRouteV2) Validate() error
func (r ReviewEnvironmentResolvedRouteV2) Validate() error
func (r ReviewValidationScopeResolvedRouteV2) Validate() error
func (c ReviewGroundingRequiredRouteCatalogV2) Validate() error
func DeriveReviewGroundingRouteRefV2(ReviewGroundingRouteDeclarationV2) (ReviewGroundingRouteRefV2, error)
func DeriveReviewGroundingReaderBindingRefV2(ReviewGroundingRouteRefV2, core.Digest) (ReviewGroundingReaderBindingRefV2, error)
func SealReviewGroundingRequiredRouteCatalogV2(ReviewGroundingRequiredRouteCatalogV2) (ReviewGroundingRequiredRouteCatalogV2, error)
```

- `Declaration.Validate`要求Family/Kind/full `Owner.Binding`/SourceContract完整，`Required=true`；Route ID由完整Declaration canonical派生，revision=1，digest覆盖BindingSetID/revision、ComponentID、ManifestDigest、ArtifactDigest、Capability与SourceContract。构造时deep clone并seal，只读运行期不得注册、删除、替换。
- constructor只接受三个nominal route binding slice；禁止`any`、通用factory map或运行期interface猜型。每个binding的Declaration.Family必须与slice nominal种类一致。
- production root以`Family + Kind + full Owner` exact匹配immutable route table；不接受只匹配ComponentID/Capability的弱route。Resolve返回对应nominal sealed resolved-route，完整携带Declaration/full Owner、Route Ref、ReaderBindingRef（其中包含adapter artifact digest）与typed Reader；consumer必须保存`Proof`并纳入read cut/aggregate digest，不得只保留Reader或Route ID。Validation Scope还必须先复读Owner-neutral association并要求Request.Owner逐字段等于association Owner。
- `RequiredRouteCatalog`是独立sealed输入，三个nominal列表按Route Ref严格排序，声明集合与binding集合必须一一对应；因此整项binding被省略也会在constructor失败，而不是延迟到首个请求。
- Reader实例身份只由sealed `ReviewGroundingReaderBindingRefV2`判定；它绑定exact Route Ref与adapter artifact digest。禁止比较Go interface值、reflect pointer或进程地址。同key同Declaration+同ReaderBindingRef幂等；同key换Owner/contract/full Binding/ReaderBinding、重复alias、同ReaderBindingRef注入两个冲突Route均构造`Conflict + BindingDrift`。
- nil/typed-nil Reader、catalog缺required binding、binding多于catalog、错误family slice、坏ReaderBindingRef全部constructor Fail Closed。unknown family/kind/full Owner route在Resolve阶段`Forbidden + OwnerConflict`；root未构造为`PreconditionFailed + OwnerMissing`。不得使用不存在的`Unsupported` category。
- Router不import Sandbox/Continuity实现包；相应Owner Adapter实现public neutral Reader后由composition root注入。
- Router不缓存Owner current projection、不注册运行期mutable entry、不提供default provider。
- Sandbox Snapshot与Continuity ArtifactRelation分别保留自身nominal contract；Router只能调用被声明的typed adapter，不能进行结构相似转换。

## 7. Review聚合S1 -> exact Inspect -> S2

```go
type ResultBundleCurrentGroundingProjectionV2 struct {
    ContractVersion string `json:"contract_version"`; Bundle ExactResourceRefV1 `json:"bundle"`; Request ExactResourceRefV1 `json:"request"`; Target ExactResourceRefV1 `json:"target"`
    Context contract.ReviewerContextEnvelopeV1 `json:"context"`; OriginalIntent contract.ReviewerContextMaterialV1 `json:"original_intent"`; AcceptanceCriteria []contract.ReviewerContextMaterialV1 `json:"acceptance_criteria"`
    ArtifactRoutes []runtimeports.ReviewArtifactResolvedRouteProofV2 `json:"artifact_routes"`; Artifacts []runtimeports.ReviewArtifactCurrentProjectionV2 `json:"artifacts"`
    EnvironmentRoute runtimeports.ReviewEnvironmentResolvedRouteProofV2 `json:"environment_route"`; Environment runtimeports.ReviewEnvironmentCurrentProjectionV2 `json:"environment"`
    ValidationScopeOwnerAssociation runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2 `json:"validation_scope_owner_association"`; ValidationScopeRoute runtimeports.ReviewValidationScopeResolvedRouteProofV2 `json:"validation_scope_route"`; ValidationScope runtimeports.ReviewValidationScopeCurrentProjectionV2 `json:"validation_scope"`
    Evidence []runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1 `json:"evidence"`
    CheckedUnixNano int64 `json:"checked_unix_nano"`; ExpiresUnixNano int64 `json:"expires_unix_nano"`; ProjectionDigest core.Digest `json:"projection_digest"`
}
type ResultBundleCurrentGroundingRequestV2 struct {
    TenantID core.TenantID `json:"tenant_id"`; Bundle ExactResourceRefV1 `json:"bundle"`; Request ExactResourceRefV1 `json:"request"`; Target ExactResourceRefV1 `json:"target"`; Case ExactResourceRefV1 `json:"case"`; Round ExactResourceRefV1 `json:"round"`; Assignment ExactResourceRefV1 `json:"assignment"`
    RunID core.AgentRunID `json:"run_id"`; ExecutionScope core.ExecutionScope `json:"execution_scope"`; ActionScopeDigest core.Digest `json:"action_scope_digest"`; Evidence []runtimeports.ReviewEvidenceRefV2 `json:"evidence"`; EvidenceSetDigest core.Digest `json:"evidence_set_digest"`
}
type ResultBundleGroundingStoredFactsV2 struct {
    Request contract.ReviewRequestV1 `json:"request"`; Target contract.TargetSnapshotV1 `json:"target"`; Bundle contract.ReviewResultBundleV2 `json:"bundle"`; Case contract.ReviewCaseV1 `json:"case"`; Round contract.ReviewRoundV1 `json:"round"`; Assignment contract.ReviewerAssignmentV1 `json:"assignment"`
}
type ResultBundleGroundingStoredFactReaderV2 interface {
    InspectResultBundleGroundingStoredFactsV2(context.Context, ResultBundleCurrentGroundingRequestV2) (ResultBundleGroundingStoredFactsV2, error)
}
type ResultBundleCurrentGroundingDependenciesV2 struct {
    Stored ResultBundleGroundingStoredFactReaderV2
    Context ReviewerContextCurrentReaderV1
    Binding runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
    Evidence runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1
    ValidationScopeOwnerAssociation runtimeports.ReviewValidationScopeOwnerAssociationCurrentReaderV2
    Routes runtimeports.ReviewGroundingReaderResolverV2
    Clock func() time.Time
}
type ResultBundleCurrentGroundingReaderV2 interface {
    InspectResultBundleCurrentGroundingV2(context.Context, ResultBundleCurrentGroundingRequestV2) (ResultBundleCurrentGroundingProjectionV2, error)
}
type ResultBundleGroundingReadRecoveryPolicyV2 struct {
    ReadRecoveryTimeoutNanos int64 `json:"read_recovery_timeout_nanos"`
}
func (r ResultBundleCurrentGroundingRequestV2) Validate() error
func (v ResultBundleGroundingStoredFactsV2) Clone() ResultBundleGroundingStoredFactsV2
func (v ResultBundleGroundingStoredFactsV2) ValidateAgainst(ResultBundleCurrentGroundingRequestV2) error
func (d ResultBundleCurrentGroundingDependenciesV2) Validate() error
func (p ResultBundleCurrentGroundingProjectionV2) Clone() ResultBundleCurrentGroundingProjectionV2
func (p ResultBundleCurrentGroundingProjectionV2) Validate() error
func (p ResultBundleCurrentGroundingProjectionV2) ValidateCurrent(ExactResourceRefV1, time.Time) error
func DigestResultBundleCurrentGroundingProjectionV2(ResultBundleCurrentGroundingProjectionV2) (core.Digest, error)
func SealResultBundleCurrentGroundingProjectionV2(ResultBundleCurrentGroundingProjectionV2) (ResultBundleCurrentGroundingProjectionV2, error)
func (p ResultBundleGroundingReadRecoveryPolicyV2) Validate() error
func NewResultBundleCurrentGroundingReaderV2(ResultBundleGroundingReadRecoveryPolicyV2, ResultBundleCurrentGroundingDependenciesV2) (ResultBundleCurrentGroundingReaderV2, error)
```

aggregate domain=`praxis.review.result-bundle-current-grounding`、contract=`praxis.review/result-bundle-current-grounding-v2`、body=完整projection且只清空ProjectionDigest。`ArtifactRoutes`与Artifacts按同stable source一一对应且同序；Environment/Validation Scope各绑定唯一nominal route proof；Validation Scope Owner association完整projection必须封入aggregate。所有Proof的Declaration/full Owner、Route、ReaderBinding/adapter digest都参与digest，Reader capability本身不进入JSON或digest。`Clone`必须deep clone Route Proof、Context materials、Artifact/Binding members、Evidence及所有嵌套slice/pointer；`Validate/ValidateCurrent`必须从封入的association、route proof与Owner Binding closure重算cross-field关系、digest和min TTL。它不持久化current index；它是一次Decide调用内的sealed read cut，S1/S2 external projections本身提供可检测current index/ABA。

输入必须来自stored Review事实：exact Request、Target、Bundle、Case/Round/Assignment、Run/Scope/ActionScope，以及去重后的full Evidence。`StoredFactReader`只返回这些Review-owned事实的deep clone；`ValidateAgainst`逐字段证明request中的全部exact refs、Tenant、Run/Scope、Evidence set都与stored对象一致，不拥有任何external Owner写口。

1. 取非零`baseline`；exact Inspect stored Request、Target、Bundle，逐字段验证Bundle绑定和本域TTL。
2. Context S1：按stored Target/Case/Assignment派生`ReviewerContextSubjectV1`，调用live `ResolveCurrentReviewerContextV1`；返回Ref必须等于Bundle的`ReviewerContext`，再`InspectCurrent`并核对Sources。Envelope中唯一OriginalIntent和全部AcceptanceCriteria materials必须与Bundle source refs逐字段exact，aggregate保存完整materials/content digests。
3. 对每个Artifact，先用live Binding Reader按`Source.Owner.Binding + exact Assignment/Target subject` Resolve+Inspect；再按full expected source+anchor set调用typed Router S1 Resolve，验证并保存完整nominal resolved-route Proof后exact Inspect；projection Source/locator digest/OwnerBinding必须等于Bundle与刚取得的Binding current。
4. Environment先复读Owner Binding current，再S1 Resolve、保存Environment route Proof与exact Inspect。Validation Scope先按Owner-neutral Source identity Resolve+Inspect唯一Owner association，保存完整association projection并要求Bundle `Expected.Owner`等于association Owner；再复读该Owner Binding current，执行Scope S1 Resolve、保存Scope route Proof与exact Inspect。两者不得互换nominal ref。
5. 对每个去重Evidence构造live `ReviewEvidenceApplicabilitySubjectV1`，调用`Resolve...`与`InspectCurrent...`，完整验证Record/OwnerFact/trust/current。
6. S2只按S1保存的Context/Artifact/Environment/Scope/Evidence、Validation Scope Owner association及每个Owner Binding full ProjectionRef重新`InspectCurrent`；禁止重新Resolve或追随新ref。immutable router不再Resolve；S1保存的每个route Proof必须封入aggregate并与resolved Reader调用记录逐字段一致。
7. 取fresh `now`；零值或`now < baseline`为ClockRegression。逐个projection `ValidateCurrent(expected, now)`并逐字段比较S1/S2；任一index、source、locator、owner fact、TTL或digest漂移整批Fail Closed。
8. aggregate `ExpiresUnixNano = min(Bundle, Request, Target, Context envelope及全部Context source, all Artifact, Environment, Validation Scope Owner association, Validation Scope, all Evidence, every external Owner Binding closure)`；`now >= min`时零Verdict/CAS。
9. seal `ResultBundleCurrentGroundingProjectionV2`，返回deep clone。该projection只是Review决定输入，不是Evidence、Authority、Permit、Runtime Outcome或Artifact Commit。
10. Verdict Owner在实际CAS点再取fresh clock并验证aggregate current；跨读时clock rollback、TTL crossing或lost reply未恢复均零写。

## 8. lost reply、Unknown与恢复

- 整个流程只读；不得因为读失败重做Artifact/验证/Provider动作。
- `ResultBundleGroundingReadRecoveryPolicyV2.ReadRecoveryTimeoutNanos`是唯一恢复时限输入，必须满足`0 < timeout <= 2s`；constructor拒绝零值、负值或更大值。每次实际恢复还必须裁剪为`min(configured timeout, S1已知全部正TTL-now, caller/operation deadline remaining)`；裁剪结果`<=0`时不恢复，直接返回原closed error。
- S1 Resolve丢回包：废弃本轮全部已读projection；只允许一次新的完整S1。这是“新current cut”，不是恢复旧结果。若caller ctx已取消，只有宿主明确启用该policy时才可`context.WithoutCancel(ctx)`，并必须立即`context.WithTimeout(..., clippedTimeout)`；禁止裸`WithoutCancel`、后台goroutine或无限等待。
- S2 exact Inspect丢回包：至多一次使用同一subject+ProjectionRef的exact重读，不得Resolve新ref；同样使用裁剪后的bounded context。成功后仍需fresh clock、index unchanged、S1/S2逐字段相等与TTL验证；失败返回原`Indeterminate/Unavailable`，不得降为NotFound。
- caller context取消、DeadlineExceeded、未知错误、eventual/retention/ordinary NotFound均不能被解释为authoritative current absence。
- 只有Owner证明exact projection ID从未存在时才是NotFound；这也不授予Review创建Owner projection的权力。

## 9. legacy V1迁移

- `ReviewResultBundleV1`、string `Anchor`、`EnvironmentDigest`、`ValidationScopeDigest`继续historical exact Inspect和显示。
- legacy Bundle不得进入production result Verdict、Runtime Review current projection或Authorization；不得从digest、文件名、latest页面或Sandbox Snapshot反推V2 typed ref。
- 新production结果Review必须创建V2 Bundle；不能原地升级/覆盖V1。
- API/SDK可返回`grounding_version=v1_legacy|v2_exact`，但不得给V1标记`current=true`或`production_eligible=true`。

## 10. Effect、Conflict Domain与Run Requirement

| 项 | 裁决 |
|---|---|
| Effect kind | 本切片只有Review本域Bundle create-once atomic append与exact historical Inspect，无Bundle current index或replacement CAS；所有external current操作纯读。Artifact生成、验证执行、截图、录屏、网络与Provider动作不属于本Reader |
| Conflict Domain | Review：`(TenantID, BundleID)`；各Owner：自己的stable subject/current index；Router无写状态 |
| Run Requirement | result Review要求exact Request/Target/Bundle、Run/Scope/ActionScope、Context Envelope、所有typed external refs与Evidence；缺一Fail Closed |
| Settlement | Bundle create仍走Review领域Fact/Store；external current read无Runtime settlement；真实验证Effect必须先由其领域Owner完成Observation→Evidence→Settlement→Apply |
| Cleanup/Residual | 读失败不产生cleanup；Unknown、缺Owner、legacy、TTL crossing作为Residual/closed error留痕，不创建假current |
| pre-run Evidence | 不触发；这是run/result阶段current grounding。若未来搬到Run前，另提OperationScope-aware Evidence Delta |

## 11. 当前准入结论

- Review-owned V1 structure/store：YES。
- Review-owned V2 contract/store/aggregate/conformance：实现并通过最终独立复审。
- Context/Evidence Reader：live可复用，但仍需production root注入。
- Artifact/Environment/Validation Scope exact-current Readers与host typed router：OPEN Port Delta。
- production Result Grounding/Verdict/root：**NO-GO**。禁止用Fake、Continuity relation、Sandbox snapshot或digest-only V1消除该门禁。
