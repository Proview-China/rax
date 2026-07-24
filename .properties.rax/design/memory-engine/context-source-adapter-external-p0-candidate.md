# P0-4 Memory Context Source Adapter候选

> 状态：**design_frozen / adapter_implemented / reference_integration_software_test_yes**。Memory Adapter直接封装live V2 Owner refs，不建立第二current；Application三阶段Port、Harness exact Turn映射、Context TransitionProof及Memory=1 reference fixture已闭合。production root与远程Retrieval仍NO-GO。

## 1. Owner与非Owner

| 对象/动作 | 唯一Owner | Consumer | 禁止 |
|---|---|---|---|
| Memory Attempt/CurrentState/Projection/Content、V2 Reader、StableClosure | Memory Owner | Memory Adapter只读复读 | Adapter/Application/Context不得改写为Memory事实 |
| Memory Adapter Binding与association envelope canonical | Memory Adapter Owner | Application只传opaque envelope；Context只在P0-3/P0-2闭合后消费 | 不得成为Memory current、Context Fact或Runtime事实 |
| Application协调attempt/prepare/advance/inspect | Application Owner（P0-3） | Context与两Owner Adapter | Memory不得命名或实现P0-3合同 |
| pending/proof/Frame/Generation/Apply/current | Context Owner | Harness只消费published exact Frame | Memory Adapter不得创建Context Fact |
| Binding/Generation currentness | Runtime Owner | Adapter只读Inspect | Adapter不得授予、续租、撤销或CAS Runtime Binding |

Adapter是无权威转换层：它只验证并封装未来`MemoryContextSourceCurrentReaderV2`返回的exact refs/digests，不保存Memory对象、不建立Store/current pointer、不排序、不补字段、不产生Candidate。

## 2. 唯一上游合同与当前门禁

唯一上游为同一Memory Owner合同族：

```text
MemoryContextSourceCurrentReaderV2
  InspectAttempt(context.Context, AttemptCoordinateV2)
  InspectForTurn(context.Context, CurrentRequestV2)
  ReadContentExact(context.Context, ExactContentRequestV2)
```

其七个V2 nominal、StableClosure和set digest以[Owner V2冻结合同](../../plan/memory-knowledge/context-source-reader-v2-frozen.md)为唯一设计真值。Adapter禁止复制`AttemptCoordinateV2`、`CurrentProjectionV2`、`ProjectionItemV2`或`ExactContentObservationV2`；live V1不能经Adapter补字段冒充V2。

live Application已发布三阶段Attempt/Prepare/Apply/Inspect nominal，Context已发布nonzero source request/response。Adapter方法直接接收Memory V2 owner nominals与外部Owner exact refs，不创建第二套neutral current DTO。

## 3. Canonical常量

| 项 | 值 |
|---|---|
| Adapter ContractVersion | `praxis.memory/context-source-adapter/v1` |
| Stable ObjectKind | `memory_context_source_adapter_stable_association` |
| Envelope ObjectKind | `memory_context_source_association_envelope` |
| Stable domain | `praxis.memory/context-source-adapter/stable-association` |
| Envelope domain | `praxis.memory/context-source-adapter/association-envelope` |
| Provider BindingSet ContractVersion | `praxis.memory/context-source-adapter/provider-binding-set/v1` |
| Provider BindingSet ObjectKind | `memory_context_source_provider_binding_set` |
| Owner Reader ContractVersion | `praxis.memory/context-source-current-reader/v2` |

两个digest均使用`contract.Digest(canonicalInput)`；`DigestDomain`是canonical input的第一个wire字段，因此domain由JSON正文直接进入digest，不再添加prefix或二次hash。sealed对象只增加输出digest，digest本身不进入对应canonical input。禁止`MustDigest`、JSON map、unknown/duplicate/trailing字段、`omitempty`。任何canonical失败只返回错误，不得panic。

### 3.1 三角色Provider Binding闭集

`ProviderBindingRefV2`本身没有role discriminator；Adapter必须增加以下固定role-tagged闭集，成员数量精确为3且顺序固定，不允许扫描BindingSet或接收第四个角色：

| 顺序 | Role tag | Expected Capability常量 | distinct裁决 |
|---:|---|---|---|
| 0 | `owner_adapter` | `praxis.memory/context-source-adapter` | 与另两项full Ref、ComponentID、Capability均pairwise distinct |
| 1 | `application_coordinator` | `praxis.application/coordinate-memory-context-source` | 与另两项full Ref、ComponentID、Capability均pairwise distinct；仍待Application Owner接受 |
| 2 | `context_adapter` | `praxis.context/consume-memory-context-source` | 与另两项full Ref、ComponentID、Capability均pairwise distinct；仍待Context Owner接受 |

role不可由slice位置、ComponentID前缀或Capability猜出；`Role`、`Ref.Capability`、`ExpectedCapability`必须逐项满足上表。任何role substitution、同Ref复用两角色、同ComponentID承担两角色、capability alias或成员重排均Fail Closed。

```go
type MemoryContextSourceBindingRoleV1 string

const (
    MemoryOwnerAdapterBindingRoleV1       MemoryContextSourceBindingRoleV1 = "owner_adapter"
    MemoryApplicationCoordinatorRoleV1   MemoryContextSourceBindingRoleV1 = "application_coordinator"
    MemoryContextAdapterBindingRoleV1     MemoryContextSourceBindingRoleV1 = "context_adapter"
    MemoryOwnerAdapterCapabilityV1        runtimeports.CapabilityNameV2 = "praxis.memory/context-source-adapter"
    MemoryCoordinatorCapabilityV1         runtimeports.CapabilityNameV2 = "praxis.application/coordinate-memory-context-source"
    MemoryContextAdapterCapabilityV1      runtimeports.CapabilityNameV2 = "praxis.context/consume-memory-context-source"
)

type MemoryContextSourceRoleBindingV1 struct {
    Role               MemoryContextSourceBindingRoleV1 `json:"role"`
    Ref                runtimeports.ProviderBindingRefV2 `json:"ref"`
    ExpectedCapability runtimeports.CapabilityNameV2     `json:"expected_capability"`
}

type MemoryContextSourceProviderBindingSetV1 struct {
    ContractVersion                 string                                               `json:"contract_version"`
    ObjectKind                      string                                               `json:"object_kind"`
    GenerationBindingAssociationRef runtimeports.GenerationBindingAssociationRefV1       `json:"generation_binding_association_ref"`
    BindingSetID                    string                                               `json:"binding_set_id"`
    BindingSetRevision              core.Revision                                        `json:"binding_set_revision"`
    BindingSetDigest                core.Digest                                          `json:"binding_set_digest"`
    BindingSetSemanticDigest        core.Digest                                          `json:"binding_set_semantic_digest"`
    Members                         []MemoryContextSourceRoleBindingV1                    `json:"members"`
}
```

## 4. Stable association body

`MemoryContextSourceAdapterStableAssociationV1`字段顺序冻结如下：

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `DigestDomain` | string | 固定Stable domain |
| 2 | `ContractVersion` | string | 固定Adapter版本 |
| 3 | `ObjectKind` | string | 固定Stable ObjectKind |
| 4 | `OwnerReaderContractVersion` | string | 必须为Memory V2，不接受V1 |
| 5 | `ProviderBindings` | `MemoryContextSourceProviderBindingSetV1` | 上节三角色闭集；精确绑定Generation association Candidate.Binding |
| 6 | `ApplicationAttemptRef` | P0-3 exact ref | 类型由Application Owner发布；本文不定义 |
| 7 | `SourceTurnRef` | `contract.Ref` | 只能来自P0-1具名Turn Owner Reader |
| 8 | `SourceTurnOrdinal` | uint32 | 满足Turn/Tool/ExpectedCurrent等式 |
| 9 | `Purpose` | string | 与Memory V2 request/projection exact一致 |
| 10 | `ScopeAuthorityBindingDigest` | digest | 由P0-3正式合同提供并复读，不由Memory补造 |
| 11 | `OwnerStableClosureDigest` | string | Memory V2 stable closure exact digest |

sealed wire对象为`MemoryContextSourceAdapterStableAssociationV1{Canonical, StableAssociationDigest}`；`StableAssociationDigest=contract.Digest(Canonical)`。

Stable body明确排除：AttemptInspectionRef、CurrentProjectionRef、ExactContentObservation refs、Binding current projection digests、CheckPhase、Owner/Adapter checked time、ExpiresAt、Envelope self digest、正文bytes、Context pending/proof/Frame refs。Memory V2 stable closure已绑定ordered items/content/source/evidence/projection/citation集合、预算和Owner语义，Adapter不得复制这些领域字段形成第二DTO。

## 5. Opaque fresh envelope

`MemoryContextSourceAssociationEnvelopeV1`字段顺序冻结如下：

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `DigestDomain` | string | 固定Envelope domain且进入digest |
| 2 | `ContractVersion` | string | 固定Adapter版本 |
| 3 | `ObjectKind` | string | 固定Envelope ObjectKind |
| 4 | `StableAssociation` | 上节sealed对象 | 重算`StableAssociationDigest` |
| 5 | `OwnerAdapterBindingCurrent` | role + Runtime full current projection | role固定`owner_adapter` |
| 6 | `CoordinatorBindingCurrent` | role + Runtime full current projection | role固定`application_coordinator` |
| 7 | `ContextAdapterBindingCurrent` | role + Runtime full current projection | role固定`context_adapter` |
| 8 | `OwnerAttemptInspectionRef` | Memory `contract.Ref` | fresh V2 Inspection exact ref |
| 9 | `OwnerCurrentProjectionRef` | Memory `contract.Ref` | fresh V2 Projection exact ref |
| 10 | `OwnerExactContentObservationRefs` | ordered `[]contract.Ref` | 按Owner projection rank；非空、拒绝重复 |
| 11 | `CheckPhase` | Memory `CheckPhaseV2` | 仅`s1`或`s2` |
| 12 | `AdapterCheckedUnixNano` | int64 | Adapter fresh clock，必须正值 |
| 13 | `ExpiresUnixNano` | int64 | 所有currentness上界最小值 |

sealed wire对象为`MemoryContextSourceAssociationEnvelopeV1{Canonical, Digest}`；`Digest=contract.Digest(Canonical)`。

Envelope只是一份短期Association Observation：没有`Current`字段、Revision/CAS/current pointer、Candidate、Frame或body。Application可以校验并持久关联其digest，但不得解析Memory领域内容；Context必须通过Adapter/Owner Reader在S2复读，不能仅凭Envelope授予current。

正文bytes不进入Envelope，也不允许Application缓存。未来Context materialization调用必须沿同一Adapter调用栈使用Memory V2 bounded `ReadContentExact`，body只在调用期存在，并由Observation Ref/ContentRef/ObservedDigest绑定。

## 6. Validate与Binding

每次S1/S2都必须：

1. Validate三项role/member/Capability闭集、每个`ProviderBindingRefV2`、Generation association、Application Attempt exact ref、SourceTurn exact ref与所有Memory refs；
2. 通过`GenerationBindingAssociationCurrentReaderV1`读取exact association Fact，要求Ref exact、Fact/Candidate canonical有效、State=`active`且fresh；
3. 将ProviderBindings的`BindingSetID/Revision/Digest/SemanticDigest`逐字段等同于`association.Candidate.Binding`同名字段，并要求三项Ref的BindingSet ID/revision全部相同；
4. 通过Runtime `ProviderBindingCurrentnessPortV2`分别fresh Inspect三个role Ref；每个Projection必须`ValidateCurrent(expectedRef, now)`、State=`active`，且其`BindingSetDigest`、`BindingSetSemanticDigest`逐字段等于`association.Candidate.Binding`；
5. 要求association Candidate.Generation与本次P0-3 generation exact一致；跨generation沿用旧association、把一个role的Projection/Ref替入另一role、或Projection虽Ref exact但两个set digest被splice，全部Fail Closed；
6. 调用Memory V2 `InspectAttempt`、`InspectForTurn`与逐项bounded `ReadContentExact`；
7. exact比较Owner、Identity/Session/Turn、Purpose/Scope/Authority、StableClosure、Projection与Content Observation链；
8. 再次复读三个Binding、Generation association和Adapter fresh clock；
9. 计算TTL最小值后才seal Envelope。

Adapter生产包未来只能依赖Memory公开合同、Runtime `core/ports`和正式P0-3 public contract；禁止依赖Runtime kernel/foundation/fakes、Harness private ports/internal、Context kernel/store或其他组件实现包。

Capability/Binding必须单一选择Memory V2 Adapter。V1/V2双Binding、同一generation注册两个Memory current source、只验Manifest不验BindingSet revision/digest均Fail Closed。

### 6.1 Canonical input Go schema与golden

以下是Adapter自身唯一canonical input；字段声明顺序、Go类型、JSON tag和wire name均冻结。它们不复制Memory Owner Projection/Item；Runtime current projection直接嵌套公开类型。

```go
type MemoryContextSourceAdapterStableCanonicalInputV1 struct {
    DigestDomain                string                                  `json:"digest_domain"`
    ContractVersion            string                                  `json:"contract_version"`
    ObjectKind                 string                                  `json:"object_kind"`
    OwnerReaderContractVersion string                                  `json:"owner_reader_contract_version"`
    ProviderBindings           MemoryContextSourceProviderBindingSetV1 `json:"provider_bindings"`
    ApplicationAttemptRef      contract.Ref                            `json:"application_attempt_ref"`
    SourceTurnRef              contract.Ref                            `json:"source_turn_ref"`
    SourceTurnOrdinal          uint32                                  `json:"source_turn_ordinal"`
    Purpose                    string                                  `json:"purpose"`
    ScopeAuthorityBindingDigest core.Digest                            `json:"scope_authority_binding_digest"`
    OwnerStableClosureDigest   string                                  `json:"owner_stable_closure_digest"`
}

type MemoryContextSourceAdapterStableAssociationV1 struct {
    Canonical                 MemoryContextSourceAdapterStableCanonicalInputV1 `json:"canonical"`
    StableAssociationDigest  string                                           `json:"stable_association_digest"`
}

type MemoryContextSourceRoleBindingCurrentV1 struct {
    Role       MemoryContextSourceBindingRoleV1              `json:"role"`
    Projection runtimeports.ProviderBindingCurrentProjectionV2 `json:"projection"`
}

type MemoryContextSourceAssociationEnvelopeCanonicalInputV1 struct {
    DigestDomain                         string                                         `json:"digest_domain"`
    ContractVersion                     string                                         `json:"contract_version"`
    ObjectKind                          string                                         `json:"object_kind"`
    StableAssociation                   MemoryContextSourceAdapterStableAssociationV1   `json:"stable_association"`
    OwnerAdapterBindingCurrent          MemoryContextSourceRoleBindingCurrentV1         `json:"owner_adapter_binding_current"`
    CoordinatorBindingCurrent           MemoryContextSourceRoleBindingCurrentV1         `json:"coordinator_binding_current"`
    ContextAdapterBindingCurrent        MemoryContextSourceRoleBindingCurrentV1         `json:"context_adapter_binding_current"`
    OwnerAttemptInspectionRef           contract.Ref                                   `json:"owner_attempt_inspection_ref"`
    OwnerCurrentProjectionRef           contract.Ref                                   `json:"owner_current_projection_ref"`
    OwnerExactContentObservationRefs    []contract.Ref                                 `json:"owner_exact_content_observation_refs"`
    CheckPhase                          CheckPhaseV2                                    `json:"check_phase"`
    AdapterCheckedUnixNano              int64                                           `json:"adapter_checked_unix_nano"`
    ExpiresUnixNano                     int64                                           `json:"expires_unix_nano"`
}

type MemoryContextSourceAssociationEnvelopeV1 struct {
    Canonical MemoryContextSourceAssociationEnvelopeCanonicalInputV1 `json:"canonical"`
    Digest    string                                                   `json:"digest"`
}
```

Memory stable golden的literal canonical JSON为（单行、无空白变化）：

```json
{"digest_domain":"praxis.memory/context-source-adapter/stable-association","contract_version":"praxis.memory/context-source-adapter/v1","object_kind":"memory_context_source_adapter_stable_association","owner_reader_contract_version":"praxis.memory/context-source-current-reader/v2","provider_bindings":{"contract_version":"praxis.memory/context-source-adapter/provider-binding-set/v1","object_kind":"memory_context_source_provider_binding_set","generation_binding_association_ref":{"id":"generation-binding-association-1","revision":1,"digest":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"},"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"binding_set_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","binding_set_semantic_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","members":[{"role":"owner_adapter","ref":{"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"component_id":"praxis.memory/context-source-adapter","manifest_digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444","artifact_digest":"sha256:7777777777777777777777777777777777777777777777777777777777777777","capability":"praxis.memory/context-source-adapter"},"expected_capability":"praxis.memory/context-source-adapter"},{"role":"application_coordinator","ref":{"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"component_id":"praxis.application/memory-context-source-coordinator","manifest_digest":"sha256:5555555555555555555555555555555555555555555555555555555555555555","artifact_digest":"sha256:8888888888888888888888888888888888888888888888888888888888888888","capability":"praxis.application/coordinate-memory-context-source"},"expected_capability":"praxis.application/coordinate-memory-context-source"},{"role":"context_adapter","ref":{"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"component_id":"praxis.context/memory-context-source-consumer","manifest_digest":"sha256:6666666666666666666666666666666666666666666666666666666666666666","artifact_digest":"sha256:9999999999999999999999999999999999999999999999999999999999999999","capability":"praxis.context/consume-memory-context-source"},"expected_capability":"praxis.context/consume-memory-context-source"}]},"application_attempt_ref":{"id":"application-attempt-1","revision":1,"digest":"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},"source_turn_ref":{"id":"turn-4","revision":1,"digest":"sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},"source_turn_ordinal":4,"purpose":"assist","scope_authority_binding_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","owner_stable_closure_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}
```

`contract.Digest(stableCanonical)`固定为`sha256:1ff0c32305cecab39e1cd36611a5351c2757dc711044e5b5626319c6d10930ea`。

Memory envelope golden使用上述sealed stable对象和三份live `ProviderBindingCurrentProjectionV2`，其关键literal canonical JSON（完整wire字段、按Go声明顺序）固定为：

```json
{"digest_domain":"praxis.memory/context-source-adapter/association-envelope","contract_version":"praxis.memory/context-source-adapter/v1","object_kind":"memory_context_source_association_envelope","stable_association":{"canonical":{"digest_domain":"praxis.memory/context-source-adapter/stable-association","contract_version":"praxis.memory/context-source-adapter/v1","object_kind":"memory_context_source_adapter_stable_association","owner_reader_contract_version":"praxis.memory/context-source-current-reader/v2","provider_bindings":{"contract_version":"praxis.memory/context-source-adapter/provider-binding-set/v1","object_kind":"memory_context_source_provider_binding_set","generation_binding_association_ref":{"id":"generation-binding-association-1","revision":1,"digest":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"},"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"binding_set_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","binding_set_semantic_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","members":[{"role":"owner_adapter","ref":{"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"component_id":"praxis.memory/context-source-adapter","manifest_digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444","artifact_digest":"sha256:7777777777777777777777777777777777777777777777777777777777777777","capability":"praxis.memory/context-source-adapter"},"expected_capability":"praxis.memory/context-source-adapter"},{"role":"application_coordinator","ref":{"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"component_id":"praxis.application/memory-context-source-coordinator","manifest_digest":"sha256:5555555555555555555555555555555555555555555555555555555555555555","artifact_digest":"sha256:8888888888888888888888888888888888888888888888888888888888888888","capability":"praxis.application/coordinate-memory-context-source"},"expected_capability":"praxis.application/coordinate-memory-context-source"},{"role":"context_adapter","ref":{"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"component_id":"praxis.context/memory-context-source-consumer","manifest_digest":"sha256:6666666666666666666666666666666666666666666666666666666666666666","artifact_digest":"sha256:9999999999999999999999999999999999999999999999999999999999999999","capability":"praxis.context/consume-memory-context-source"},"expected_capability":"praxis.context/consume-memory-context-source"}]},"application_attempt_ref":{"id":"application-attempt-1","revision":1,"digest":"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},"source_turn_ref":{"id":"turn-4","revision":1,"digest":"sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},"source_turn_ordinal":4,"purpose":"assist","scope_authority_binding_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","owner_stable_closure_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"},"stable_association_digest":"sha256:1ff0c32305cecab39e1cd36611a5351c2757dc711044e5b5626319c6d10930ea"},"owner_adapter_binding_current":{"role":"owner_adapter","projection":{"contract_version":"2.0.0","ref":{"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"component_id":"praxis.memory/context-source-adapter","manifest_digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444","artifact_digest":"sha256:7777777777777777777777777777777777777777777777777777777777777777","capability":"praxis.memory/context-source-adapter"},"state":"active","binding_set_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","binding_set_semantic_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","binding_id":"binding-memory-owner-adapter","binding_revision":3,"grant_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","projection_digest":"sha256:bf950f5f4cf7616a9f87a1493296ad703b65989b872fc7f891370186ab70c767","issued_unix_nano":1000000000,"expires_unix_nano":2000000000}},"coordinator_binding_current":{"role":"application_coordinator","projection":{"contract_version":"2.0.0","ref":{"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"component_id":"praxis.application/memory-context-source-coordinator","manifest_digest":"sha256:5555555555555555555555555555555555555555555555555555555555555555","artifact_digest":"sha256:8888888888888888888888888888888888888888888888888888888888888888","capability":"praxis.application/coordinate-memory-context-source"},"state":"active","binding_set_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","binding_set_semantic_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","binding_id":"binding-memory-application-coordinator","binding_revision":3,"grant_digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","projection_digest":"sha256:41fb541682f5709eb096356346967c3cc18191390b260fced0607d2914d8018d","issued_unix_nano":1000000000,"expires_unix_nano":2000000000}},"context_adapter_binding_current":{"role":"context_adapter","projection":{"contract_version":"2.0.0","ref":{"binding_set_id":"binding-set-g6b-7","binding_set_revision":7,"component_id":"praxis.context/memory-context-source-consumer","manifest_digest":"sha256:6666666666666666666666666666666666666666666666666666666666666666","artifact_digest":"sha256:9999999999999999999999999999999999999999999999999999999999999999","capability":"praxis.context/consume-memory-context-source"},"state":"active","binding_set_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","binding_set_semantic_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","binding_id":"binding-memory-context-adapter","binding_revision":3,"grant_digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","projection_digest":"sha256:41a2bd31335a93ae5633adc2ebe8a8cda4fc5cec930775eae564cd6614b23b8b","issued_unix_nano":1000000000,"expires_unix_nano":2000000000}},"owner_attempt_inspection_ref":{"id":"memory-inspection-1","revision":1,"digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},"owner_current_projection_ref":{"id":"memory-projection-1","revision":1,"digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},"owner_exact_content_observation_refs":[{"id":"memory-content-observation-1","revision":1,"digest":"sha256:9999999999999999999999999999999999999999999999999999999999999999"}],"check_phase":"s1","adapter_checked_unix_nano":1100000000,"expires_unix_nano":1900000000}
```

`contract.Digest(envelopeCanonical)`固定为`sha256:4fc77c0219c2ec9bd14fa6c8361232f5d443b87d944bbf4cf25d15dd675d409d`。

golden测试必须逐字段tamper：stable的domain/version/kind/Owner版本、ProviderBindings每个set字段、每个member的role/ref六字段/ExpectedCapability/顺序/数量、ApplicationAttempt、SourceTurn、ordinal、Purpose、ScopeAuthority和Owner closure；envelope的domain/version/kind/sealed stable、三个role/current projection的每个公开字段、Owner Inspection/Projection/Observation refs及顺序、phase、checked、expiry。任一修改必须重算出不同digest；违反角色、capability、Runtime projection self digest、association Candidate.Binding或TTL规则时还必须Validate失败。

## 7. S1/S2与lost reply

```text
P0-3 Application Attempt + P0-1 Turn exact
 -> Binding S1
 -> Memory V2 InspectAttempt/InspectForTurn/ReadContentExact(S1)
 -> Binding S1 reread
 -> Memory S1 opaque envelope
 -> Context pending outputs + P0-2 proof（外部Owner）
 -> Binding S2
 -> Memory V2 InspectAttempt/InspectForTurn/ReadContentExact(S2)
 -> Binding S2 reread
 -> Memory S2 opaque envelope
 -> Context atomic Apply/Generation CAS（外部Owner）
```

S2必须携带S1 `StableAssociationDigest`作为期望值。S1/S2的Inspection/Projection/Observation refs、Binding current digests、checked/expiry和Envelope digest可以不同；StableAssociationDigest、SourceTurn、Purpose/Scope/Authority、Memory StableClosure和canonical ordered exact集合必须相同。任一stable漂移关闭整个Context refresh，不在原attempt补选低位项。

Adapter纯只读、零Provider/网络/Resolver。回包丢失不创建Memory Attempt、不Retrieve、不写第二Store；Application若已持久关联Envelope则Inspect原P0-3 attempt，若尚未关联只能用原Memory Attempt fresh复读并重新生成Envelope，仍须stable digest相同。

## 8. TTL/currentness

`now >= ExpiresUnixNano`即过期。Envelope TTL取以下所有非零上界最小值：P0-3 request NotAfter；P0-1 Session/Turn；Memory Attempt/Projection/items/StatePlaneBinding/Content Observation；三个Runtime Binding projections；Generation association；Identity/Authority/Policy；S2时另含Context pending/proof/ExpectedCurrent。

Adapter在进入调用、Owner读后和返回前使用fresh clock；clock rollback、锁/Get等待跨TTL、Binding在Owner读期间revoked/expired/drift、Generation切换均返回零Envelope/零body。caller时间和Envelope expiry都只能缩短，不能延长Owner或Runtime TTL。

## 9. Closed errors

| 条件 | Memory closed error |
|---|---|
| malformed、unknown/type/version、空/重复Observation refs、canonical失败 | `ErrInvalidArgument` |
| exact ref、Binding、Generation、StableClosure、Projection/Content链漂移 | `ErrEvidenceConflict`或`ErrRevisionConflict` |
| TTL到期、clock rollback、Owner/Binding不再current | `ErrNotCurrent` |
| Scope/Authority/Identity/Session/Turn/Purpose不一致 | `ErrScopeDenied` |
| Sensitivity越界 | `ErrSensitivityDenied` |
| exact content已evicted | `ErrNotFound` |
| Owner或Binding inspection覆盖不完整 | `ErrInspectionIncomplete` |
| 调用V1、远程路径，或production root未装配获批ports却直接启用 | `ErrUnsupported` |

失败返回零Envelope、nil body、零状态变化；不得自由文本reason、Partial success、自动远程fallback或把`ErrNotFound`当空成功。

## 10. NO-GO反例

1. Adapter复制Memory Projection/Items形成第二current DTO；
2. Envelope带`Current=true`、current pointer或CAS；
3. Application/Context根据Envelope直接写Memory事实；
4. Adapter用V1补Identity/Session/Turn或stable closure字段；
5. fresh Inspection/Projection/TTL进入StableAssociationDigest；
6. S2 fresh ref不同即拒绝，而不比较stable digest；
7. S2 stable closure漂移仍沿用S1 Envelope；
8. 只验Adapter Binding，不验Coordinator/Context/Generation Binding；
9. Binding读后漂移但仍返回Envelope；
10. Adapter缓存或返回未由exact Content Observation绑定的body；
11. evicted后启动远程Retrieval；
12. P0-3未发布就私建Application DTO/Port；
13. Context cardinality仍Memory=0时root先启用来源1；
14. Harness直接调用Memory Adapter或消费Envelope。

## 11. 无SCC DAG与门禁

```text
Memory V2 Go/public Reader ----+
P0-1 Turn exact Reader --------+--> Memory Adapter review/implementation
Runtime Binding readers -------+               |
P0-3 Application Port ---------+               v
P0-2 Context proof ------------+--> Context nonzero Memory source contract
                                               |
                                               v
                                      production root/G6B
                                               |
                                               v
                                      Context exact Frame -> Harness
```

Memory V2、Harness exact mapping、Application三阶段、Context proof与nonzero reference source均已YES，Adapter实现关闭原P0-4的Memory侧reference切片；production root与远程路径不因此启用。
