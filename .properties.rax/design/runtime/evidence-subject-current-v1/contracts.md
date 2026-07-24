# Evidence Subject Current V1 精确合同

状态：**第七候选第一独立资产审计YES（P0/P1/P2=0/0/0）；asset candidate等待Continuity第二独立资产审计，implementation仍NO-GO，未授权Go实现**。

## 1. 版本与canonical域

- additive contract固定为`EvidenceSubjectCurrentContractVersionV1 = "1.0.0"`；
- canonical domain固定为`EvidenceSubjectCurrentCanonicalDomainV1 = "praxis.runtime.evidence-subject-current"`；
- object discriminator闭表固定为`EvidenceSubjectKeyV1 | EvidenceSubjectProjectionIDV1 | EvidenceSubjectCurrentIndexIDV1 | EvidenceSubjectCurrentProjectionV1 | EvidenceSubjectConsumerAssociationIDV1 | EvidenceSubjectConsumerAssociationRefV1 | EvidenceSubjectConsumerAssociationCurrentProjectionV1 | EvidenceTombstoneAbsenceRefV1 | EvidenceReadabilityPolicyRefV1 | EvidenceSubjectRecordRegistrationCurrentRequestV1 | EvidenceSubjectRecordRegistrationCurrentResultV1 | EvidenceSubjectPresenceReadabilityCurrentRequestV1 | EvidenceSubjectPresenceReadabilityCurrentResultV1 | EvidenceSubjectCurrentLookupRequestV1 | EvidenceSubjectCurrentValidationRequestV1 | EvidenceSubjectMutationRequestV1 | EvidenceSubjectMutationStableKeyV1 | EvidenceSubjectMutationCommitV1`；不接受caller字符串、alias或namespaced扩展；
- 不修改Evidence V2、OperationScope Evidence V3或Checkpoint Evidence V1的对象、Validate、canonical或digest；
- nil/empty必须canonical唯一，所有集合有界、有序、唯一。

## 2. 公共候选类型

下列字段名、类型与JSON tag为V1第七候选；第七次双独立资产审计前不得再用自由map、alias或语义相近字段替代：

### `EvidenceSubjectKeyV1`

```go
type EvidenceSubjectKeyV1 struct {
    Record EvidenceRecordRefV2 `json:"record"`
    Source EvidenceSourceKeyV2 `json:"source"`
}
```

两字段均须先执行live `Validate()`；它不接受Event ID、payload ref、nil或caller字符串作为替代identity。`SubjectKeyDigest`固定为：

```text
CanonicalJSONDigest(
  "praxis.runtime.evidence-subject-current",
  "1.0.0",
  "EvidenceSubjectKeyV1",
  EvidenceSubjectKeyV1{Record, Source},
)
```

Projection/Index/Mutation ID统一直接使用`core.CanonicalJSONDigest`返回的`sha256:<64hex>`字符串；domain/version固定，discriminator与exact typed input见Derive golden。不得加前缀、截断，也不得把watermark/revision/TTL加入identity输入。

### `EvidenceSubjectProjectionRefV1`

```go
type EvidenceSubjectProjectionRefV1 struct {
    ProjectionID     string        `json:"projection_id"`
    Revision         core.Revision `json:"revision"`
    SubjectKeyDigest core.Digest   `json:"subject_key_digest"`
    OwnerWatermark   core.Revision `json:"owner_watermark"`
    Digest           core.Digest   `json:"digest"`
}
```

`ProjectionID = CanonicalJSONDigest(domain, version, "EvidenceSubjectProjectionIDV1", EvidenceSubjectProjectionIDInputV1{SubjectKeyDigest: subjectKeyDigest})`，只由稳定subject派生；Owner watermark、revision、TTL、Registration、Binding和其他digest均不得进入ID。相同subject永远使用同一ID，首次revision为`1`，后续只允许严格`+1`；gap、rewind、same revision换body/digest与换ID均Conflict。

### `EvidenceSourceRegistrationRefV1`

```go
type EvidenceSourceRegistrationRefV1 struct {
    RegistrationID      string           `json:"registration_id"`
    Revision            core.Revision    `json:"revision"`
    FactDigest          core.Digest      `json:"fact_digest"`
    ConfigurationDigest core.Digest      `json:"configuration_digest"`
    SourceID            NamespacedNameV2 `json:"source_id"`
    SourceEpoch         core.Epoch       `json:"source_epoch"`
}
```

它必须由`EvidenceSourceRegistrationFactV2`当前事实的ID、revision、`DigestV2()`、`ConfigurationDigestV2()`、Source ID和Source Epoch逐字段派生，禁止只携ID或caller自己重算一个看似合法digest。

### Reader Binding与Capability live映射

```go
type EvidenceSubjectReaderCapabilityRefV1 struct {
    Name                           CapabilityNameV2 `json:"name"`
    BindingRevision                core.Revision    `json:"binding_revision"`
    GrantDigest                    core.Digest      `json:"grant_digest"`
    BindingCurrentProjectionDigest core.Digest      `json:"binding_current_projection_digest"`
    IssuedUnixNano                 int64            `json:"issued_unix_nano"`
    ExpiresUnixNano                int64            `json:"expires_unix_nano"`
}

type EvidenceSubjectReaderBindingRefV1 struct {
    Binding                  ProviderBindingRefV2                  `json:"binding"`
    BindingSetDigest         core.Digest                           `json:"binding_set_digest"`
    BindingSetSemanticDigest core.Digest                           `json:"binding_set_semantic_digest"`
    BindingID                string                                `json:"binding_id"`
    Capability               EvidenceSubjectReaderCapabilityRefV1 `json:"capability"`
}
```

九项唯一live映射为：`Binding <- Ref`、`BindingSetDigest`、`BindingSetSemanticDigest`、`BindingID`、`Capability.BindingRevision <- BindingRevision`、`Capability.GrantDigest <- GrantDigest`、`Capability.BindingCurrentProjectionDigest <- ProjectionDigest`、`Capability.IssuedUnixNano <- IssuedUnixNano`、`Capability.ExpiresUnixNano <- ExpiresUnixNano`；`Capability.Name <- Ref.Capability`是Binding自身的类型回扣，不另计一项。必须满足`Binding.Capability == Capability.Name == CapabilityNameV2("praxis.runtime/read-evidence-subject-current")`、live state active、九项坐标与projection self digest exact。因此Lookup/Validation的expected consumer是对bound association的期望，不是任意业务Provider binding或权威来源。删除且禁止出现无live来源的Capability自由`Revision/Digest/Authority`与Binding `BindingSetCurrentnessDigest/BindingSetProjectionDigest/CheckedUnixNano/重复ExpiresUnixNano`。Source Authority与Policy Authority由各自Owner current Reader复读，不伪装成reader capability字段。

### bound consumer association current proof

Request中的consumer只是expected坐标，不是权威来源。Gateway构造时必须由host composition注入一个已绑定的association ref及其窄current Reader；不提供自由discovery，request不能换association ID。Runtime ports候选shape为：

```go
type EvidenceSubjectConsumerAssociationIDInputV1 struct {
    Principal            core.OwnerRef       `json:"principal"`
    ConsumerComponentID  ComponentIDV2       `json:"consumer_component_id"`
    ConsumerCapability   CapabilityNameV2    `json:"consumer_capability"`
    ExecutionScopeDigest core.Digest         `json:"execution_scope_digest"`
}

type EvidenceSubjectConsumerAssociationRefV1 struct {
    AssociationID       string               `json:"association_id"`
    Revision            core.Revision        `json:"revision"`
    Principal           core.OwnerRef        `json:"principal"`
    Consumer            ProviderBindingRefV2 `json:"consumer"`
    ExecutionScopeDigest core.Digest          `json:"execution_scope_digest"`
    Digest              core.Digest          `json:"digest"`
}

type EvidenceSubjectConsumerAssociationCurrentProjectionV1 struct {
    ContractVersion      string                                         `json:"contract_version"`
    Ref                  EvidenceSubjectConsumerAssociationRefV1       `json:"ref"`
    Principal            core.OwnerRef                                  `json:"principal"`
    Consumer             ProviderBindingRefV2                           `json:"consumer"`
    ExecutionScopeDigest core.Digest                                    `json:"execution_scope_digest"`
    BindingCurrent       ProviderBindingCurrentProjectionV2             `json:"binding_current"`
    CheckedUnixNano      int64                                          `json:"checked_unix_nano"`
    ExpiresUnixNano      int64                                          `json:"expires_unix_nano"`
    ProjectionDigest     core.Digest                                    `json:"projection_digest"`
}

type EvidenceSubjectConsumerAssociationCurrentReaderV1 interface {
    InspectEvidenceSubjectConsumerAssociationCurrentV1(
        context.Context,
        EvidenceSubjectConsumerAssociationRefV1,
    ) (EvidenceSubjectConsumerAssociationCurrentProjectionV1, error)
}
```

类型Owner是Runtime ports；association的semantic/publisher/current Owner是host Assembly/Binding Owner，Runtime Evidence Owner只复读。Projection必须证明`Ref.Principal == Principal`、`Ref.Consumer == Consumer == BindingCurrent.Ref`、`Ref.ExecutionScopeDigest == ExecutionScopeDigest`、`BindingCurrent` active/self-valid，且`CheckedUnixNano > 0 && CheckedUnixNano < ExpiresUnixNano <= BindingCurrent.ExpiresUnixNano`。Ref digest清自身后覆盖其余全字段；Projection digest清自身后覆盖完整Projection。same ID换revision/body/digest为Conflict，revoked/expired/stale为PreconditionFailed，typed-nil/unavailable为Unavailable。

`AssociationID = CanonicalJSONDigest(domain, version, "EvidenceSubjectConsumerAssociationIDV1", EvidenceSubjectConsumerAssociationIDInputV1)`。ID只使用publisher已验证的Principal、Consumer Component/Capability与ExecutionScopeDigest；BindingSet revision、manifest/artifact digest、TTL、Checked和caller字符串不参与稳定ID。同ID revision只能+1，Consumer full binding变化必须由association Owner CAS推进，不得换ID或ABA。

S1/S2必须通过同一Reader复读该bound ref，要求两次full-equal，并且`request.ExpectedConsumer == association.Consumer`、`request.ExpectedExecutionScopeDigest == association.ExecutionScopeDigest`。caller只提供exact expectation用于拒绝路由串线，不能自授consumer或scope。

### `EvidenceTombstoneRefV1`

```go
type EvidenceTombstoneRefV1 struct {
    Record   EvidenceRecordRefV2 `json:"record"`
    Source   EvidenceSourceKeyV2 `json:"source"`
    Revision core.Revision       `json:"revision"`
    Digest   core.Digest         `json:"digest"`
}
```

它由existing `EvidenceTombstoneFactV2`逐字段派生，`Revision == Fact.Revision == 1`，`Digest == Fact.DigestV2()`；不创建第二Tombstone Fact。

### `EvidenceTombstoneAbsenceRefV1`

```go
type EvidenceTombstoneAbsenceRefV1 struct {
    SubjectKeyDigest core.Digest  `json:"subject_key_digest"`
    Revision        core.Revision `json:"revision"`
    OwnerWatermark  core.Revision `json:"owner_watermark"`
    Digest          core.Digest   `json:"digest"`
}
```

revision与OwnerWatermark单调且非零。它是Runtime Evidence Owner对“截至该水位没有该subject Tombstone”的sealed证明；NotFound、空指针或Store缺键不能替代它。

Absence digest使用固定discriminator `EvidenceTombstoneAbsenceRefV1`，清空自身`Digest`后覆盖其余全部字段；错误非零ID/digest不得被Seal覆盖。Clone为纯值拷贝。

### `EvidenceReadabilityPolicyRefV1`

```go
type EvidenceReadabilityPolicyStateV1 string

const (
    EvidenceReadabilityPolicyActiveV1  EvidenceReadabilityPolicyStateV1 = "active"
    EvidenceReadabilityPolicyRevokedV1 EvidenceReadabilityPolicyStateV1 = "revoked"
    EvidenceReadabilityPolicyExpiredV1 EvidenceReadabilityPolicyStateV1 = "expired"
)

type EvidenceReadabilityPolicyRefV1 struct {
    PolicyID        string                           `json:"policy_id"`
    Revision        core.Revision                    `json:"revision"`
    Digest          core.Digest                      `json:"digest"`
    Owner           EvidenceProducerBindingRefV2     `json:"owner"`
    SubjectKeyDigest core.Digest                      `json:"subject_key_digest"`
    ExecutionScopeDigest core.Digest                  `json:"execution_scope_digest"`
    Consumer        ProviderBindingRefV2             `json:"consumer"`
    AllowRead       bool                             `json:"allow_read"`
    State           EvidenceReadabilityPolicyStateV1 `json:"state"`
    ExpiresUnixNano int64                           `json:"expires_unix_nano"`
}
```

它只引用Runtime Evidence Owner已接纳的retention/readability policy current事实；不得由Continuity或caller自由构造，也不替代`EvidenceSourcePolicyBindingRefV2`。Policy必须exact绑定`SubjectKeyDigest`、`ExecutionScopeDigest`、`Consumer`与`AllowRead`；因live Binding projection本身不带tenant/scope，不得宣称仅凭Binding即可发现wrong tenant/scope。只有Policy subject/scope/consumer分别full-equal当前Subject、Execution Scope和bound association，且`active && AllowRead`才可形成readable current。consumer不同、subject/scope不同、`AllowRead=false`或从未授权均返回`Forbidden`；revoked/expired仅能保留在historical projection，current返回`PreconditionFailed`。

Readability Policy digest使用固定discriminator `EvidenceReadabilityPolicyRefV1`，清空自身`Digest`后覆盖PolicyID/Revision/Owner/SubjectKeyDigest/ExecutionScopeDigest/Consumer/AllowRead/State/Expires全部字段；Owner与Consumer均必须是exact live `ProviderBindingRefV2`坐标，不得使用Component ID、capability字符串或caller boolean代替。Clone为纯值拷贝。

### closed presence/readability Go shape

```go
type EvidenceTombstonePresenceV1 string

const (
    EvidenceTombstoneAbsentSealedV1 EvidenceTombstonePresenceV1 = "tombstone_absent_sealed"
    EvidenceTombstonePresentV1      EvidenceTombstonePresenceV1 = "tombstone_present"
)

type EvidenceSubjectReadabilityV1 string

const (
    EvidenceSubjectReadableV1         EvidenceSubjectReadabilityV1 = "readable"
    EvidenceSubjectTombstonedV1       EvidenceSubjectReadabilityV1 = "tombstoned"
    EvidenceSubjectPolicyDeniedV1     EvidenceSubjectReadabilityV1 = "policy_denied"
    EvidenceSubjectRetentionExpiredV1 EvidenceSubjectReadabilityV1 = "retention_expired"
    EvidenceSubjectSourceInactiveV1   EvidenceSubjectReadabilityV1 = "source_inactive"
)
```

未知值、alias、大小写变体和空值都是`InvalidArgument`；不允许上层自由namespaced扩展。

### `EvidenceSubjectCurrentIndexRefV1`

```go
type EvidenceSubjectCurrentIndexRefV1 struct {
    IndexID            string                          `json:"index_id"`
    Revision           core.Revision                   `json:"revision"`
    SubjectKeyDigest   core.Digest                     `json:"subject_key_digest"`
    PreviousProjection *EvidenceSubjectProjectionRefV1 `json:"previous_projection,omitempty"`
    CurrentProjection  EvidenceSubjectProjectionRefV1  `json:"current_projection"`
    OwnerWatermark     core.Revision                   `json:"owner_watermark"`
    Digest             core.Digest                     `json:"digest"`
}
```

`IndexID = CanonicalJSONDigest(domain, version, "EvidenceSubjectCurrentIndexIDV1", EvidenceSubjectCurrentIndexIDInputV1{SubjectKeyDigest: subjectKeyDigest})`，同subject永不换ID。Index revision与`CurrentProjection.Revision`exact相等并严格递增；`OwnerWatermark == CurrentProjection.OwnerWatermark`。首次revision为`1`且`PreviousProjection=nil`；后续revision只能为旧Index revision `+1`，`PreviousProjection`必须full-equal旧Index的`CurrentProjection`。Index digest覆盖除自身`Digest`外的所有字段。

Index Clone必须deep-copy optional `PreviousProjection`；Subject Key/Tombstone Ref/Registration Ref/Projection Ref均为值语义。Tombstone Ref不另造self digest：其`Digest`必须exact复用existing `EvidenceTombstoneFactV2.DigestV2()`；任何字段变化都必须重新由Owner Fact派生，caller不得Seal。

### `EvidenceSubjectCurrentProjectionV1`

完整Go shape冻结为：

```go
type EvidenceSubjectCurrentProjectionV1 struct {
    ContractVersion    string                          `json:"contract_version"`
    Ref                EvidenceSubjectProjectionRefV1 `json:"ref"`
    Subject            EvidenceSubjectKeyV1           `json:"subject"`
    SubjectKeyDigest   core.Digest                    `json:"subject_key_digest"`
    PreviousProjection *EvidenceSubjectProjectionRefV1 `json:"previous_projection,omitempty"`

    Record               EvidenceRecordRefV2 `json:"record"`
    Source               EvidenceSourceKeyV2 `json:"source"`
    CandidateDigest      core.Digest         `json:"candidate_digest"`
    PreviousRecordDigest core.Digest         `json:"previous_record_digest"`

    Registration                EvidenceSourceRegistrationRefV1 `json:"registration"`
    RegistrationState           EvidenceSourceStateV2            `json:"registration_state"`
    RegistrationExpiresUnixNano int64                            `json:"registration_expires_unix_nano"`

    SourcePolicy                EvidenceSourcePolicyBindingRefV2 `json:"source_policy"`
    SourcePolicyState           EvidenceSourcePolicyStateV2      `json:"source_policy_state"`
    SourcePolicyOwner           EvidenceProducerBindingRefV2     `json:"source_policy_owner"`
    SourcePolicyAuthority       AuthorityBindingRefV2            `json:"source_policy_authority"`
    SourcePolicyAuthorityCurrent DispatchAuthorityFactV2          `json:"source_policy_authority_current"`
    SourcePolicyExpiresUnixNano int64                            `json:"source_policy_expires_unix_nano"`

    LedgerScope           EvidenceLedgerScopeV2      `json:"ledger_scope"`
    LedgerScopeDigest     core.Digest                `json:"ledger_scope_digest"`
    ExecutionScope        core.ExecutionScope        `json:"execution_scope"`
    ExecutionScopeDigest  core.Digest                `json:"execution_scope_digest"`
    CurrentScope          ExecutionScopeBindingRefV2 `json:"current_scope"`
    CurrentScopeWatermark core.Revision              `json:"current_scope_watermark"`
    ExecutionScopeCurrent ExecutionScopeCurrentFactV2 `json:"execution_scope_current"`

    Producer               EvidenceProducerBindingRefV2         `json:"producer"`
    ProducerBindingCurrent ProviderBindingCurrentProjectionV2   `json:"producer_binding_current"`
    Authority              AuthorityBindingRefV2                 `json:"authority"`
    AuthorityCurrent       DispatchAuthorityFactV2               `json:"authority_current"`
    ActionScopeDigest      core.Digest                           `json:"action_scope_digest"`
    Consumer               ProviderBindingRefV2                  `json:"consumer"`
    ReaderBinding          EvidenceSubjectReaderBindingRefV1     `json:"reader_binding"`
    ReaderCapability       EvidenceSubjectReaderCapabilityRefV1  `json:"reader_capability"`

    TrustClass        EvidenceTrustClassV2          `json:"trust_class"`
    ClaimKind         core.RunCompletionClaimKind   `json:"claim_kind,omitempty"`
    EventKind         NamespacedNameV2              `json:"event_kind"`
    CustomClass       NamespacedNameV2              `json:"custom_class"`
    Payload           EvidencePayloadRefV2          `json:"payload"`
    Causation         []EvidenceCausationRefV2      `json:"causation"`
    CorrelationID     string                        `json:"correlation_id"`
    OwnerFact         *EvidenceOwnerFactRefV2       `json:"owner_fact,omitempty"`
    HistoricalSource  *EvidenceHistoricalSourceV2  `json:"historical_source,omitempty"`
    ObservedUnixNano  int64                         `json:"observed_unix_nano"`
    IngestedUnixNano  int64                         `json:"ingested_unix_nano"`

    Presence          EvidenceTombstonePresenceV1    `json:"presence"`
    Readability       EvidenceSubjectReadabilityV1   `json:"readability"`
    Tombstone         *EvidenceTombstoneRefV1        `json:"tombstone,omitempty"`
    TombstoneAbsence  *EvidenceTombstoneAbsenceRefV1 `json:"tombstone_absence,omitempty"`
    ReadabilityPolicy EvidenceReadabilityPolicyRefV1 `json:"readability_policy"`

    CheckedUnixNano  int64       `json:"checked_unix_nano"`
    ExpiresUnixNano  int64       `json:"expires_unix_nano"`
    ProjectionDigest core.Digest `json:"projection_digest"`
}
```

`Record == Subject.Record`、`Source == Subject.Source`；重复字段用于closed decoder与all-ref cross-check，不允许不一致。`ExecutionScopeCurrent`必须是`ExecutionScopeFactReaderV2.InspectCurrentExecutionScope(CurrentScope.Ref)`的live exact结果，并逐字段回扣ExecutionScope/CurrentScope/Producer/Authority源。`ProducerBindingCurrent`必须是将`Producer`值语义转成`ProviderBindingRefV2`后由`ProviderBindingCurrentnessPortV2`读得的exact projection。`AuthorityCurrent`与`SourcePolicyAuthorityCurrent`分别必须是`AuthorityFactReaderV2.InspectDispatchAuthority(Authority.Ref)`和`InspectDispatchAuthority(SourcePolicyAuthority.Ref)`的exact fact，并各自回扣revision/digest/epoch、ExecutionScope与ActionScopeDigest。`Consumer == ReadabilityPolicy.Consumer`，且`ReaderBinding/ReaderCapability`按consumer live current projection唯一映射。Causation nil canonical规范化为`[]`；OwnerFact/HistoricalSource/Tombstone/TombstoneAbsence/PreviousProjection各自按nil/null唯一化。所有ID/revision/digest/state/scope/payload/TTL均逐字段Validate；`CheckedUnixNano > 0 && CheckedUnixNano < ExpiresUnixNano`。

Readability closed set固定为`readable | tombstoned | policy_denied | retention_expired | source_inactive`。只有`readable`且Tombstone分支为sealed absence时才能表达payload当前可读；其他状态只允许历史metadata审计，不授payload读取资格。

上述集合不是建议而是V1冻结闭表：

```text
EvidenceSubjectReadabilityV1 = readable | tombstoned | policy_denied | retention_expired | source_inactive
EvidenceTombstonePresenceV1  = tombstone_absent_sealed | tombstone_present
```

`readable | policy_denied | retention_expired | source_inactive`只允许`tombstone_absent_sealed`；`tombstoned`只允许`tombstone_present`。Tombstone一旦存在，Readability必须归一为`tombstoned`，不得同时保留其他原因状态；未知值、alias、大小写变体或矛盾组合均InvalidArgument。Projection另须完整包含`EvidenceSourceRegistrationRefV1`及按live投影派生的Reader Binding/Capability refs。

### Projection canonical与self digest

完整Projection canonical覆盖上述每个字段；domain/version/discriminator固定为`praxis.runtime.evidence-subject-current / 1.0.0 / EvidenceSubjectCurrentProjectionV1`。**Projection body与canonical严禁包含Current Index Ref、IndexID、Index revision或Index digest**。计算时只清空`Projection.Ref.Digest`与`Projection.ProjectionDigest`，其余字段不得省略；得到同一digest后同时写入两个位置，且`Projection.Ref.Digest == Projection.ProjectionDigest`。Seal对错误非零派生字段必须Conflict，不得覆盖洗白。same Ref换body、digest回流、漏字段或nil/empty歧义全部拒绝。

`CloneEvidenceSubjectCurrentProjectionV1`必须返回deep clone：Causation slice及五个optional pointer各自分配新值；不得复用caller slice/pointer。其余公共shape也提供deep clone；strict JSON使用`core.DecodeStrictJSON`拒绝unknown/duplicate/trailing字段。

各冻结shape的canonical/clone规则汇总如下：

| Type | 固定discriminator/摘要 | Clone规则 |
|---|---|---|
| `EvidenceSubjectKeyV1` | `EvidenceSubjectKeyV1`，输出`SubjectKeyDigest` | 值拷贝；Record/Source逐字段不变 |
| `EvidenceTombstoneRefV1` | 不造第二摘要；`Digest`复用exact Tombstone Fact digest | 值拷贝 |
| `EvidenceTombstoneAbsenceRefV1` | `EvidenceTombstoneAbsenceRefV1`，清自身Digest | 值拷贝 |
| `EvidenceReadabilityPolicyRefV1` | `EvidenceReadabilityPolicyRefV1`，清自身Digest | 值拷贝 |
| `EvidenceSubjectCurrentProjectionV1` | `EvidenceSubjectCurrentProjectionV1`，只清Ref.Digest与ProjectionDigest并写回同digest | Causation与五个optional pointer deep-copy |
| `EvidenceSubjectCurrentLookupRequestV1` | `EvidenceSubjectCurrentLookupRequestV1`，完整五字段 | 值拷贝 |
| `EvidenceSubjectCurrentSnapshotV1` | 不另造摘要；逐字段验证Projection/Index exact闭合 | Projection deep clone、Index optional previous deep-copy |
| `EvidenceSubjectCurrentValidationRequestV1` | `EvidenceSubjectCurrentValidationRequestV1`，完整十字段 | deep-copy ExpectedCurrentIndex.PreviousProjection |
| `EvidenceSubjectConsumerAssociationRefV1` | 同名discriminator，清Digest | 值拷贝 |
| `EvidenceSubjectConsumerAssociationCurrentProjectionV1` | 同名discriminator，清ProjectionDigest | 值拷贝 |
| `EvidenceSubjectRecordRegistrationCurrentRequestV1` | 同名discriminator，覆盖全字段 | 值拷贝 |
| `EvidenceSubjectRecordRegistrationCurrentResultV1` | 同名discriminator，清ProjectionDigest | Record/Registration canonical clone |
| `EvidenceSubjectPresenceReadabilityCurrentRequestV1` | 同名discriminator，覆盖全字段 | 值拷贝 |
| `EvidenceSubjectPresenceReadabilityCurrentResultV1` | 同名discriminator，清ProjectionDigest | Tombstone/Absence optional refs deep-copy |
| `EvidenceSubjectMutationRequestV1` | `EvidenceSubjectMutationRequestV1`，只清RequestDigest，four payload exact one-of | expected refs与four optional payload deep-copy |
| `EvidenceSubjectMutationKeyV1` | `EvidenceSubjectMutationStableKeyV1`派生StableKey，`MutationID == string(StableKeyDigest)` | expected refs deep-copy |
| `EvidenceSubjectMutationCommitV1` | `EvidenceSubjectMutationCommitV1`，只清CommitDigest | Request、Key、expected optional refs deep-copy |

所有canonical均复用本合同固定domain/version；nil optional pointer与present zero-value不是同一语义，present zero-value必须Validate失败。Seal只能填零值派生字段；错误非零派生字段一律Conflict，不洗白。

### Derive golden

所有derive必须使用下列命名input struct及exact snake_case JSON tag，禁止anonymous struct、map、字段别名或隐式omitempty：

```go
type EvidenceSubjectProjectionIDInputV1 struct {
    SubjectKeyDigest core.Digest `json:"subject_key_digest"`
}

type EvidenceSubjectCurrentIndexIDInputV1 struct {
    SubjectKeyDigest core.Digest `json:"subject_key_digest"`
}

type EvidenceSubjectMutationStableKeyInputV1 struct {
    SubjectKeyDigest          core.Digest                       `json:"subject_key_digest"`
    Kind                      EvidenceSubjectMutationKindV1     `json:"kind"`
    ExpectedCurrentIndex      *EvidenceSubjectCurrentIndexRefV1 `json:"expected_current_index,omitempty"`
    ExpectedCurrentProjection *EvidenceSubjectProjectionRefV1   `json:"expected_current_projection,omitempty"`
    FirstCreateSentinel       string                            `json:"first_create_sentinel,omitempty"`
}
```

Projection/Index ID分别用同名discriminator与上述input canonical派生。Mutation stable key固定为`CanonicalJSONDigest(domain, version, "EvidenceSubjectMutationStableKeyV1", EvidenceSubjectMutationStableKeyInputV1)`；`StableKeyDigest`exact等于该输出，`MutationID = string(StableKeyDigest)`，两者不得分别派生或带前缀。`RequestDigest`不参与StableKey，因此same stable key下换immutable Request会定义为payload mismatch `Conflict`，而不是第二个mutation identity。

冻结测试向量：

```json
{"record":{"ledger_scope_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000002","sequence":7,"record_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000003"},"source":{"registration_id":"reg-a","source_epoch":4,"source_sequence":7}}
```

固定输出为：

```text
SubjectKeyDigest          = sha256:aee9a1e15b583f37788f293173261d4945e5ab0e3e0cce7980c59dca6de9c005
ProjectionID              = sha256:6fe2f3efb4a395cc58f904f74ce45ba36c9597f5ce16621d9d6af224e4e37fe3
IndexID                   = sha256:cddc693c7eda17f0380135add4dafd8b32078b8c51457654e9a4543ce2840b3a
FirstTombstoneMutationID  = sha256:da80bf64a3ed35c17e24dad24424ac6ca7a3f2c48c8214b212a22efcba871864
```

四个discriminator依次为`EvidenceSubjectKeyV1`、`EvidenceSubjectProjectionIDV1`、`EvidenceSubjectCurrentIndexIDV1`、`EvidenceSubjectMutationStableKeyV1`。首次Mutation canonical输入固定为`EvidenceSubjectMutationStableKeyInputV1{SubjectKeyDigest:<golden>,Kind:"tombstone_create",ExpectedCurrentIndex:nil,ExpectedCurrentProjection:nil,FirstCreateSentinel:"no-current-v1"}`，其`MutationID`literal与`StableKeyDigest`literal均为上述`FirstTombstoneMutationID`。ports golden test必须直接比较四个literal与`MutationID == string(StableKeyDigest)`；禁止用被测derive函数同时生成expected。

### 无环seal与历史/current发布顺序

摘要依赖严格单向：

```text
Owner-current S1/S2 closure
-> seal immutable Projection body
-> derive complete Projection Ref and shared body digest
-> build Current Index Ref containing the complete Projection Ref
-> atomically publish historical Projection + CAS Current Index
```

Projection digest不读取Index；Index digest只读取已经seal的Projection Ref，因此不存在`ProjectionDigest -> IndexDigest -> ProjectionDigest`回路。Historical store以`(ProjectionID, Revision)`为immutable key；Current Index只保存最新完整Ref并通过同Owner CAS推进，不能覆盖、删除或重建历史。

首次seal只有一个合法形状：Projection ID与Index ID均按稳定subject派生；Projection/Index revision均为`1`；Projection Previous与Index Previous均为nil；两个OwnerWatermark exact相等且非零；historical Projection与Current Index必须同一线性化点全有或全无。后续seal必须以完整旧Index Ref作expected，new Projection/Index revision均为旧revision`+1`，Previous均full-equal旧Current Projection。

## 3. 公共只读Port

Record+Registration与Presence+Readability不得通过被禁止的raw `EvidenceLedgerFactPortV2`注入。两组Owner必须以具名Request/Result和结构不兼容raw Fact Port的窄Reader提供：

```go
type EvidenceSubjectRecordRegistrationCurrentRequestV1 struct {
    ContractVersion string               `json:"contract_version"`
    Subject         EvidenceSubjectKeyV1 `json:"subject"`
}

type EvidenceSubjectRecordRegistrationCurrentResultV1 struct {
    ContractVersion string                           `json:"contract_version"`
    Subject         EvidenceSubjectKeyV1             `json:"subject"`
    Record          EvidenceLedgerRecordV2           `json:"record"`
    Registration    EvidenceSourceRegistrationFactV2 `json:"registration"`
    CheckedUnixNano int64                            `json:"checked_unix_nano"`
    ExpiresUnixNano int64                            `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                     `json:"projection_digest"`
}

type EvidenceSubjectRecordRegistrationCurrentReaderV1 interface {
    InspectEvidenceSubjectRecordRegistrationCurrentV1(
        context.Context,
        EvidenceSubjectRecordRegistrationCurrentRequestV1,
    ) (EvidenceSubjectRecordRegistrationCurrentResultV1, error)
}

type EvidenceSubjectPresenceReadabilityCurrentRequestV1 struct {
    ContractVersion              string                            `json:"contract_version"`
    Subject                      EvidenceSubjectKeyV1              `json:"subject"`
    ExpectedConsumer             ProviderBindingRefV2              `json:"expected_consumer"`
    ExpectedExecutionScopeDigest core.Digest                       `json:"expected_execution_scope_digest"`
    ExpectedOwnerWatermark       core.Revision                     `json:"expected_owner_watermark"`
}

type EvidenceSubjectPresenceReadabilityCurrentResultV1 struct {
    ContractVersion string                           `json:"contract_version"`
    Subject         EvidenceSubjectKeyV1             `json:"subject"`
    SubjectKeyDigest core.Digest                     `json:"subject_key_digest"`
    Presence        EvidenceTombstonePresenceV1      `json:"presence"`
    Readability     EvidenceSubjectReadabilityV1     `json:"readability"`
    Tombstone       *EvidenceTombstoneRefV1          `json:"tombstone,omitempty"`
    TombstoneAbsence *EvidenceTombstoneAbsenceRefV1  `json:"tombstone_absence,omitempty"`
    ReadabilityPolicy EvidenceReadabilityPolicyRefV1 `json:"readability_policy"`
    OwnerWatermark  core.Revision                    `json:"owner_watermark"`
    CheckedUnixNano int64                            `json:"checked_unix_nano"`
    ExpiresUnixNano int64                            `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                     `json:"projection_digest"`
}

type EvidenceSubjectPresenceReadabilityCurrentReaderV1 interface {
    InspectEvidenceSubjectPresenceReadabilityCurrentV1(
        context.Context,
        EvidenceSubjectPresenceReadabilityCurrentRequestV1,
    ) (EvidenceSubjectPresenceReadabilityCurrentResultV1, error)
}
```

两个Result使用同名fixed discriminator与本合同domain/version，清`ProjectionDigest`后覆盖其余全字段。Record Result必须证明`Record.RefV2()==Subject.Record`、Record.Source==Subject.Source及Registration ID/epoch/current state/TTL exact；Presence Result必须证明subject digest、expected consumer/scope/watermark与Readability Policy及Tombstone/Absence one-of exact。两者S1/S2分别full-equal；typed-nil、Unavailable、same ID换body或S1/S2漂移均零Continuity publish。

```go
type EvidenceSubjectCurrentReaderV1 interface {
    InspectEvidenceSubjectProjectionV1(
        context.Context,
        EvidenceSubjectProjectionRefV1,
    ) (EvidenceSubjectCurrentProjectionV1, error)

    InspectEvidenceSubjectCurrentV1(
        context.Context,
        EvidenceSubjectCurrentLookupRequestV1,
    ) (EvidenceSubjectCurrentSnapshotV1, error)

    ValidateEvidenceSubjectCurrentV1(
        context.Context,
        EvidenceSubjectCurrentValidationRequestV1,
    ) (EvidenceSubjectCurrentSnapshotV1, error)
}
```

```go
type EvidenceSubjectCurrentLookupRequestV1 struct {
    ContractVersion              string                           `json:"contract_version"`
    Subject                      EvidenceSubjectKeyV1             `json:"subject"`
    ExpectedConsumer             ProviderBindingRefV2             `json:"expected_consumer"`
    ExpectedExecutionScopeDigest core.Digest                      `json:"expected_execution_scope_digest"`
    ExpectedSourcePolicy         EvidenceSourcePolicyBindingRefV2 `json:"expected_source_policy"`
}

type EvidenceSubjectCurrentSnapshotV1 struct {
    ContractVersion string                              `json:"contract_version"`
    Projection      EvidenceSubjectCurrentProjectionV1 `json:"projection"`
    CurrentIndex    EvidenceSubjectCurrentIndexRefV1    `json:"current_index"`
}

type EvidenceSubjectCurrentValidationRequestV1 struct {
    ContractVersion              string                               `json:"contract_version"`
    Subject                      EvidenceSubjectKeyV1                 `json:"subject"`
    ExpectedProjection           EvidenceSubjectProjectionRefV1       `json:"expected_projection"`
    ExpectedCurrentIndex         EvidenceSubjectCurrentIndexRefV1     `json:"expected_current_index"`
    ExpectedRegistration         EvidenceSourceRegistrationRefV1      `json:"expected_registration"`
    ExpectedReaderBinding        EvidenceSubjectReaderBindingRefV1    `json:"expected_reader_binding"`
    ExpectedReaderCapability     EvidenceSubjectReaderCapabilityRefV1 `json:"expected_reader_capability"`
    ExpectedConsumer             ProviderBindingRefV2                  `json:"expected_consumer"`
    ExpectedExecutionScopeDigest core.Digest                          `json:"expected_execution_scope_digest"`
    ExpectedSourcePolicy         EvidenceSourcePolicyBindingRefV2     `json:"expected_source_policy"`
}
```

`InspectEvidenceSubjectCurrentV1`是只读bootstrap lookup：按稳定IndexID定位current，caller无需预知Projection/Index/Registration/Binding/Capability refs；不存在current只能返回权威NotFound，**不得create**。Lookup禁止携Reader Binding/Capability、Readability/Tombstone/absence、TTL/now/caller current或`RequestedNotAfter`。`ExpectedSourcePolicy`必须来自S1 immutable Record，不是caller自由选择；`ExpectedConsumer`只是对Gateway-bound association current proof的exact期望，不授权，caller不得携association ref或自报principal。

Validation request携S1 snapshot全部exact refs，包括按live projection唯一映射的Reader Binding/Capability以及`ExpectedConsumer`；expected Current Index必须是完整Ref，不能缩成ID/revision/digest之一。`ExpectedConsumer`必须full-equal Lookup.ExpectedConsumer、bound association.Consumer、Projection.Consumer和ReadabilityPolicy.Consumer。它禁止加入association ref/principal、Readability、Tombstone、TTL、now、RequestedNotAfter或caller current布尔值。

Lookup/Validation request各自使用同名fixed discriminator并覆盖全部字段；无自由map/extension。Snapshot不独立Seal：其`Projection.Ref`必须full-equal`CurrentIndex.CurrentProjection`，且两对象各自canonical/self digest已通过。

`InspectEvidenceSubjectProjectionV1`是historical exact读取，不宣称current。`InspectEvidenceSubjectCurrentV1`按稳定IndexID返回首个current Snapshot。`ValidateEvidenceSubjectCurrentV1`必须按稳定IndexID读取current index，要求full-equal全部expected refs并完成fresh Owner current复读；成功返回与historical/S1逐字段相同的sealed Snapshot，不重封。

### 七组Owner-current exact S1/S2

Gateway先以构造时注入的bound association current proof确定principal/consumer/scope，再通过七个独立、typed-nil可检测的窄current依赖形成Evidence闭包；association是授权前置而不是第八个Evidence Owner事实。即使多个依赖由同一Runtime Evidence Owner实现，也不得合并字段或信任Projection自述：

1. **Record + Registration Owner current**：只通过`EvidenceSubjectRecordRegistrationCurrentReaderV1.InspectEvidenceSubjectRecordRegistrationCurrentV1`读取具名Result；Owner内部执行`InspectBySource`与`InspectRecord` exact双读，返回Registration ID/revision/fact digest/configuration digest/source ID/source epoch/state/TTL全坐标；Gateway不接受`EvidenceLedgerFactPortV2`；
2. **Source Policy Owner current**：Source Policy ref/revision/digest/state/Owner/Authority/TTL；
3. **Execution Scope current**：通过live `ExecutionScopeFactReaderV2.InspectCurrentExecutionScope(CurrentScope.Ref)`读取`ExecutionScopeCurrentFactV2`，验证Ref/revision/digest/state、exact Scope、CapabilityGrantDigest、全部source refs、watermark与TTL；
4. **Producer Binding current**：通过live `ProviderBindingCurrentnessPortV2.InspectProviderBindingCurrentV2(ProviderBindingRefV2(Producer))`读取`ProviderBindingCurrentProjectionV2`，验证self digest、active state、BindingSet full digests、grant与TTL；
5. **Authority current**：通过live `AuthorityFactReaderV2`分别以`Authority.Ref`和`SourcePolicyAuthority.Ref`读取两个`DispatchAuthorityFactV2`，逐一验证revision/digest/epoch、exact Scope、ActionScopeDigest、active state与TTL；
6. **Reader Binding + Capability current**：以bound association已冻结Consumer为expected ref，同一typed current Reader返回live `ProviderBindingCurrentProjectionV2`；Gateway按冻结九项映射表唯一派生两个nominal refs，并验证projection self digest、active state、capability exact、TTL及S1/S2 full equality；
7. **Presence + Readability Owner current**：只通过`EvidenceSubjectPresenceReadabilityCurrentReaderV1.InspectEvidenceSubjectPresenceReadabilityCurrentV1`读取具名Result，其包含exact Tombstone或sealed Absence one-of、OwnerWatermark、Readability closed state、Readability Policy ref/state/Owner/SubjectKeyDigest/ExecutionScopeDigest/Consumer/AllowRead/TTL及合法presence/readability pair；Gateway不接受raw Tombstone/Fact Port。

七组返回闭包的最小exact字段冻结如下；这些是Runtime Gateway依赖，不是Continuity可写Fact Port：

| Owner current闭包 | exact请求坐标 | S1/S2必须full-equal的返回字段 |
|---|---|---|
| Record + Registration | `EvidenceSubjectRecordRegistrationCurrentRequestV1{Subject}` | 具名Result的Record Ref、Source Key、Candidate/Previous digest、Registration Ref、registration state、Checked/Expires、projection digest |
| Source Policy | expected `EvidenceSourcePolicyBindingRefV2` | Policy Ref、revision/digest/state、Owner、Authority、Checked/Expires、projection digest |
| Execution Scope | exact `CurrentScope.Ref` | live `ExecutionScopeCurrentFactV2`全字段；S1/S2 same Ref/revision/digest/Scope/source refs/watermark/state/TTL |
| Producer Binding | `ProviderBindingRefV2(Producer)` | live `ProviderBindingCurrentProjectionV2`全字段；S1/S2 same set digests/grant/state/TTL |
| Authority | exact source `Authority.Ref` + policy `SourcePolicyAuthority.Ref` | 两个live `DispatchAuthorityFactV2`全字段；S1/S2各自same revision/digest/Scope/ActionScope/state/TTL |
| Reader Binding + Capability | bound association.Consumer为expected；Validate携S1派生的expected Binding/Capability refs | `ProviderBindingCurrentProjectionV2`九项唯一映射形成两个nominal refs；S1/S2 same Reader、same ref/body/TTL |
| Presence + Readability | 具名Request携Subject Key + OwnerWatermark + association-derived expected Consumer/Scope | 具名Result的Presence enum、exact Tombstone或Absence Ref、Readability enum、Readability Policy Ref/state/Owner/Subject/Scope/Consumer/AllowRead、Checked/Expires、projection digest |

S1读取七组projection/fact并验证request与sealed Projection；完成其他必要current检查后，S2用fresh clock通过**相同typed Reader实例**重新读取同七组。S2必须逐字段full-equal S1，并再次回扣request consumer、Projection、Current Index和natural min-TTL；换Reader、换Binding、只比较digest或任何same-ID revision/body/TTL漂移均Conflict/Fail Closed，且不得调用Continuity publish。

## 4. Owner内部原子mutation边界

下列mutation必须作为同一Runtime Evidence Owner事务的一部分提交：

```text
source registration/renew/revoke/expire
or source-policy accepted binding advance
or tombstone create
or retention/readability accepted binding advance
  + subject-current projection create
  + subject-current index CAS
  + tombstone absence watermark advance or tombstone ref bind
  + immutable MutationCommit append
= one linearization point
```

该事务是Owner内部能力，不加入Continuity-facing Reader。现有V2 `CreateTombstone`/`CompareAndSwapSource`可保留兼容历史语义，但在R-CTY-06 production composition中，未经上述原子publish的旧写入不得生成或维持current readability projection。

同ID同immutable mutation内容幂等；同ID换内容Conflict。回包丢失只Inspect原mutation及exact index；不能重复Tombstone/renew或自行重封projection。

Owner mutation stable key固定由domain/version、SubjectKeyDigest、closed mutation kind及完整expected Current Index/Projection refs派生；首次create使用唯一固定`no-current-v1`哨兵，不接受caller自定义空值。watermark、new revision、TTL、Checked和随机caller ID不得参与。create/CAS成功回包丢失后只按stable key读取mutation历史、historical projection与current index：同immutable request且合法progressed successor幂等恢复，same key换body/非法跳转/ABA为Conflict。watermark变化必须推进同一ProjectionID的revision，禁止派生新ProjectionID逃避历史。

Owner内部冻结以下closed kind、immutable request与derived key；它们不是Continuity-facing公共写Port：

```go
type EvidenceSubjectMutationKindV1 string

const (
    EvidenceSubjectMutationSourceRegistrationAdvanceV1 EvidenceSubjectMutationKindV1 = "source_registration_advance"
    EvidenceSubjectMutationSourcePolicyAdvanceV1       EvidenceSubjectMutationKindV1 = "source_policy_advance"
    EvidenceSubjectMutationTombstoneCreateV1            EvidenceSubjectMutationKindV1 = "tombstone_create"
    EvidenceSubjectMutationReadabilityPolicyAdvanceV1   EvidenceSubjectMutationKindV1 = "readability_policy_advance"
)

type EvidenceSubjectMutationRequestV1 struct {
    ContractVersion            string                            `json:"contract_version"`
    Subject                    EvidenceSubjectKeyV1              `json:"subject"`
    Kind                       EvidenceSubjectMutationKindV1     `json:"kind"`
    ExpectedCurrentIndex       *EvidenceSubjectCurrentIndexRefV1 `json:"expected_current_index,omitempty"`
    ExpectedCurrentProjection  *EvidenceSubjectProjectionRefV1   `json:"expected_current_projection,omitempty"`
    Registration               *EvidenceSourceRegistrationRefV1  `json:"registration,omitempty"`
    SourcePolicy               *EvidenceSourcePolicyBindingRefV2 `json:"source_policy,omitempty"`
    Tombstone                  *EvidenceTombstoneRefV1            `json:"tombstone,omitempty"`
    ReadabilityPolicy          *EvidenceReadabilityPolicyRefV1   `json:"readability_policy,omitempty"`
    RequestDigest              core.Digest                       `json:"request_digest"`
}

type EvidenceSubjectMutationKeyV1 struct {
    ContractVersion           string                            `json:"contract_version"`
    MutationID                string                            `json:"mutation_id"`
    SubjectKeyDigest          core.Digest                       `json:"subject_key_digest"`
    Kind                      EvidenceSubjectMutationKindV1     `json:"kind"`
    ExpectedCurrentIndex      *EvidenceSubjectCurrentIndexRefV1 `json:"expected_current_index,omitempty"`
    ExpectedCurrentProjection *EvidenceSubjectProjectionRefV1  `json:"expected_current_projection,omitempty"`
    StableKeyDigest           core.Digest                       `json:"stable_key_digest"`
    RequestDigest             core.Digest                       `json:"request_digest"`
}
```

Request canonical固定为`praxis.runtime.evidence-subject-current / 1.0.0 / EvidenceSubjectMutationRequestV1`，清空`RequestDigest`后覆盖其余全字段。它不带new Projection/Index、watermark、Checked、Expires或caller MutationID；这些只能由Owner current闭包与原子publish派生。Kind与four optional payload必须exact one-of：

| Kind | 唯一非nil payload |
|---|---|
| `source_registration_advance` | `Registration` |
| `source_policy_advance` | `SourcePolicy` |
| `tombstone_create` | `Tombstone` |
| `readability_policy_advance` | `ReadabilityPolicy` |

其余payload必须nil；missing/duplicate/extra/type-pun均`InvalidArgument`。首次create要求两个Expected指针同时nil并使用固定`no-current-v1`哨兵参与派生；后续CAS要求两个Expected指针同时非nil、分别Validate，且`ExpectedCurrentIndex.CurrentProjection` full-equal `ExpectedCurrentProjection`。lost-reply恢复只有三种合法结论：

1. exact immutable request已提交，或Owner历史证明其已进入合法单调后继：只Inspect并幂等返回，不重复mutation；
2. same stable key已有有效事实但`RequestDigest`或immutable body不同：`Conflict`；
3. mutation历史、Projection或Index无法形成权威完整结论：返回`Unavailable`或`Indeterminate`，不得把NotFound猜成未提交并盲重写。

每次成功发布还必须生成immutable Commit：

```go
type EvidenceSubjectMutationCommitV1 struct {
    ContractVersion            string                            `json:"contract_version"`
    Key                        EvidenceSubjectMutationKeyV1      `json:"key"`
    Request                    EvidenceSubjectMutationRequestV1  `json:"request"`
    Subject                    EvidenceSubjectKeyV1              `json:"subject"`
    RequestDigest              core.Digest                       `json:"request_digest"`
    ExpectedPreviousIndex      *EvidenceSubjectCurrentIndexRefV1 `json:"expected_previous_index,omitempty"`
    ExpectedPreviousProjection *EvidenceSubjectProjectionRefV1  `json:"expected_previous_projection,omitempty"`
    NewProjection              EvidenceSubjectProjectionRefV1    `json:"new_projection"`
    NewIndex                   EvidenceSubjectCurrentIndexRefV1  `json:"new_index"`
    CommittedUnixNano          int64                             `json:"committed_unix_nano"`
    CommitDigest               core.Digest                       `json:"commit_digest"`
}
```

Commit、Projection、Index history与current pointer必须同事务。CommitDigest清自身后覆盖全部字段。Request→Key expected→Commit new refs必须同时证明三组exact equality：

1. `Request.RequestDigest == Key.RequestDigest == Commit.RequestDigest`；
2. `Request.Subject == Commit.Subject`、`DigestSubject(Request.Subject) == Key.SubjectKeyDigest`、`Request.Kind == Key.Kind`；
3. `Request.ExpectedCurrentIndex == Key.ExpectedCurrentIndex == Commit.ExpectedPreviousIndex`且`Request.ExpectedCurrentProjection == Key.ExpectedCurrentProjection == Commit.ExpectedPreviousProjection`；新端另要求`Commit.NewIndex.CurrentProjection` full-equal `Commit.NewProjection`。

任一个nil/present、revision、digest、kind或subject不等都是`Conflict`，Seal不得覆盖错误非零值。

lost reply成功恢复必须同时满足exact Commit存在、RequestDigest一致、Commit的新Projection/Index exact存在，以及current等于Commit.NewIndex或完整immutable history证明Commit.NewIndex是current祖先。Commit不存在时，即使发现更高revision也不得推断原mutation成功；返回Conflict或Indeterminate且零补写。

## 5. TTL/currentness

Projection自然`ExpiresUnixNano`取bound consumer association current、Record/Registration、source policy、live `ExecutionScopeCurrentFactV2.ExpiresUnixNano`、live producer `ProviderBindingCurrentProjectionV2.ExpiresUnixNano`、source Authority与policy Authority两个live `DispatchAuthorityFactV2.ExpiresUnixNano`、Consumer Reader Binding/Capability live projection expiry、Presence/Absence seal、readability policy与相关Owner grant可证明上限的最小值。不得使用Projection自嵌TTL、caller `RequestedNotAfter`或已删除的Capability自由Authority/Revision伪造上限。任一上限缺失、非正、已过期或reader不可判定均Fail Closed。`CheckedUnixNano > 0 && CheckedUnixNano < ExpiresUnixNano`，两者一经Seal不可因Inspect时间变化。

current验证与mutation恢复使用同一closed error set：

- 结构、JSON shape、enum/presence pair、稳定ID或首seal形状非法：`InvalidArgument`；
- historical Inspect的完整`(ProjectionID, Revision)`从未存在，或任何mutation之前按稳定IndexID确认尚无current：`NotFound`；mutation已发出且回包丢失后，NotFound不得被解释为“安全重试”；
- same stable ID/revision换body/digest、full expected Index/Projection不相等、revision gap/rewind、history断链、ABA或same mutation key换RequestDigest：`Conflict`；
- exact historical存在但已非current，或七组Owner current、presence/readability、TTL、Authority、Policy、Binding被撤销/过期：`PreconditionFailed`；
- request结构合法但reader capability/authority从未授权、capability name不匹配或不覆盖该consumer：`Forbidden`；曾有效但撤销/过期仍为`PreconditionFailed`；
- typed-nil、reader/store不可用或无法完成读取：`Unavailable`；
- lost-reply后只见Projection/Index其中一半、S1/S2各自有效但不一致、absence/current结论无法确定：`Indeterminate`。

## 6. 线性化与恢复

Continuity消费顺序固定为：

```text
S1 Inspect bound Consumer Association current
-> S1 InspectEvidenceSubjectRecordRegistrationCurrent
-> S1 InspectEvidenceSubjectCurrent(Subject+ExpectedConsumer+Scope+Record Policy)
-> S1 InspectCurrentExecutionScope
-> S1 Inspect producer Binding current
-> S1 Inspect Authority current
-> S1 InspectEvidenceSubjectPresenceReadabilityCurrent
-> authoritative_fact按需Owner S1
-> S2 Inspect same bound Consumer Association current
-> S2 InspectEvidenceSubjectRecordRegistrationCurrent
-> S2 ValidateEvidenceSubjectCurrent(S1全部exact refs+same Consumer)
-> S2 InspectCurrentExecutionScope
-> S2 Inspect producer Binding current
-> S2 Inspect Authority current
-> S2 InspectEvidenceSubjectPresenceReadabilityCurrent
-> authoritative_fact按需Owner S2
-> S1/S2完整snapshot exact比较
-> fresh ValidateEvidenceSubjectCurrent(same refs，只验now/current且不重封)
-> Continuity自己的原子publish
```

R-CTY-06不Append Record、不创建Tombstone、不推进Continuity Timeline、不调用Provider。旧Projection Ref可historical Inspect，但current index前进后必须拒绝；不存在“TTL尚未过期所以旧ref仍current”的窗口。lost-reply恢复必须读取mutation历史、historical Projection与current Index三者：全有且exact或完整history证明合法后继时幂等成功；same key换内容或非法history为Conflict；半写、不可读或无法判定为Indeterminate/Unavailable，均零重写。
