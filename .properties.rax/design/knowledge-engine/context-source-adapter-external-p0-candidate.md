# P0-4 Knowledge Context Source Adapter候选

> 状态：**design_frozen / adapter_implemented / reference_integration_software_test_yes**。Knowledge Adapter直接封装live V2 Owner refs，不建立第二current；Application三阶段Port、Harness exact Turn映射、Context TransitionProof/`knowledge_reference`及Knowledge=1 reference fixture已闭合。production root与远程Retrieval/Resolver仍NO-GO。

## 1. Owner与非Owner

| 对象/动作 | 唯一Owner | Consumer | 禁止 |
|---|---|---|---|
| Knowledge Attempt/Record/Package/Snapshot/Pointer/Source/Projection/Citation/License/Conflict/Content、V2 Reader、StableClosure | Knowledge Owner | Knowledge Adapter只读复读 | Adapter/Application/Context不得改写为Knowledge事实 |
| Knowledge Adapter Binding与association envelope canonical | Knowledge Adapter Owner | Application只传opaque envelope；Context只在外部门闭合后消费 | 不得成为Knowledge current、Context Fact或Runtime事实 |
| Application协调attempt/prepare/advance/inspect | Application Owner（P0-3） | Context与两Owner Adapter | Knowledge不得命名或实现P0-3合同 |
| `knowledge_reference` Binding/Candidate、pending/proof/Frame/Generation/Apply/current | Context Owner（P0-5/P0-2） | Harness只消费published exact Frame | Knowledge Adapter不得创建Context Fact或选择Fragment kind |
| Binding/Generation currentness | Runtime Owner | Adapter只读Inspect | Adapter不得授予、续租、撤销或CAS Runtime Binding |

Adapter只验证并封装未来`KnowledgeContextSourceCurrentReaderV2`的exact refs/digests。它不保存Knowledge对象、不建立Store/Pointer/current、不补Citation/License/Conflict、不改变rank或预算，也不把检索命中升级为事实。

## 2. 唯一上游合同与当前门禁

唯一上游为同一Knowledge Owner合同族：

```text
KnowledgeContextSourceCurrentReaderV2
  InspectAttempt(context.Context, AttemptCoordinateV2)
  InspectForTurn(context.Context, CurrentRequestV2)
  ReadContentExact(context.Context, ExactContentRequestV2)
```

其七个V2 nominal、StableClosure和Knowledge八个set digest以[Owner V2冻结合同](../../plan/memory-knowledge/context-source-reader-v2-frozen.md)为唯一设计真值。Adapter不得复制`CurrentProjectionV2`、`ProjectionItemV2`或Knowledge source chain形成neutral第二current；P0-5 Context Binding只能引用Owner exact refs。

P0-3 Application Attempt、P0-5 Context `knowledge_reference` request/binding仍无正式public nominal。因此本资产不冻结跨Owner方法名或外部入参struct，只冻结Adapter输出及Validate语义。未来方法必须直接使用Knowledge V2 owner nominals和对应Owner发布的exact refs。

## 3. Canonical常量

| 项 | 值 |
|---|---|
| Adapter ContractVersion | `praxis.knowledge/context-source-adapter/v1` |
| Stable ObjectKind | `knowledge_context_source_adapter_stable_association` |
| Envelope ObjectKind | `knowledge_context_source_association_envelope` |
| Stable domain | `praxis.knowledge/context-source-adapter/stable-association` |
| Envelope domain | `praxis.knowledge/context-source-adapter/association-envelope` |
| Provider BindingSet ContractVersion | `praxis.knowledge/context-source-adapter/provider-binding-set/v1` |
| Provider BindingSet ObjectKind | `knowledge_context_source_provider_binding_set` |
| Owner Reader ContractVersion | `praxis.knowledge/context-source-current-reader/v2` |

两个digest统一使用`contract.Digest(canonicalInput)`；`DigestDomain`是canonical input第一个wire字段，domain由JSON正文直接进入digest，不添加prefix或二次hash。sealed对象只增加输出digest，digest本身不进入对应canonical input。禁止`MustDigest`、JSON map、unknown/duplicate/trailing字段和`omitempty`；untrusted输入失败不得panic。

### 3.1 三角色Provider Binding闭集

`ProviderBindingRefV2`本身不带role；Adapter必须增加固定role-tagged闭集，成员数精确为3且顺序固定：

| 顺序 | Role tag | Expected Capability常量 | distinct裁决 |
|---:|---|---|---|
| 0 | `owner_adapter` | `praxis.knowledge/context-source-adapter` | 与另两项full Ref、ComponentID、Capability均pairwise distinct |
| 1 | `application_coordinator` | `praxis.application/coordinate-knowledge-context-source` | 与另两项full Ref、ComponentID、Capability均pairwise distinct；仍待Application Owner接受 |
| 2 | `context_adapter` | `praxis.context/consume-knowledge-context-source` | 与另两项full Ref、ComponentID、Capability均pairwise distinct；仍待Context Owner接受 |

禁止由slice位置、ComponentID前缀或Capability猜role；`Role`、`Ref.Capability`、`ExpectedCapability`必须逐项满足上表。role substitution、同Ref/ComponentID承担两角色、capability alias、成员重排或第四成员全部Fail Closed。

```go
type KnowledgeContextSourceBindingRoleV1 string

const (
    KnowledgeOwnerAdapterBindingRoleV1       KnowledgeContextSourceBindingRoleV1 = "owner_adapter"
    KnowledgeApplicationCoordinatorRoleV1   KnowledgeContextSourceBindingRoleV1 = "application_coordinator"
    KnowledgeContextAdapterBindingRoleV1     KnowledgeContextSourceBindingRoleV1 = "context_adapter"
    KnowledgeOwnerAdapterCapabilityV1        runtimeports.CapabilityNameV2 = "praxis.knowledge/context-source-adapter"
    KnowledgeCoordinatorCapabilityV1         runtimeports.CapabilityNameV2 = "praxis.application/coordinate-knowledge-context-source"
    KnowledgeContextAdapterCapabilityV1      runtimeports.CapabilityNameV2 = "praxis.context/consume-knowledge-context-source"
)

type KnowledgeContextSourceRoleBindingV1 struct {
    Role               KnowledgeContextSourceBindingRoleV1 `json:"role"`
    Ref                runtimeports.ProviderBindingRefV2    `json:"ref"`
    ExpectedCapability runtimeports.CapabilityNameV2        `json:"expected_capability"`
}

type KnowledgeContextSourceProviderBindingSetV1 struct {
    ContractVersion                 string                                               `json:"contract_version"`
    ObjectKind                      string                                               `json:"object_kind"`
    GenerationBindingAssociationRef runtimeports.GenerationBindingAssociationRefV1       `json:"generation_binding_association_ref"`
    BindingSetID                    string                                               `json:"binding_set_id"`
    BindingSetRevision              core.Revision                                        `json:"binding_set_revision"`
    BindingSetDigest                core.Digest                                          `json:"binding_set_digest"`
    BindingSetSemanticDigest        core.Digest                                          `json:"binding_set_semantic_digest"`
    Members                         []KnowledgeContextSourceRoleBindingV1                 `json:"members"`
}
```

## 4. Stable association body

`KnowledgeContextSourceAdapterStableAssociationV1`字段顺序冻结如下：

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `DigestDomain` | string | 固定Stable domain |
| 2 | `ContractVersion` | string | 固定Adapter版本 |
| 3 | `ObjectKind` | string | 固定Stable ObjectKind |
| 4 | `OwnerReaderContractVersion` | string | 必须为Knowledge V2，不接受V1 |
| 5 | `ProviderBindings` | `KnowledgeContextSourceProviderBindingSetV1` | 上节三角色闭集；精确绑定Generation association Candidate.Binding |
| 6 | `ApplicationAttemptRef` | P0-3 exact ref | 类型由Application Owner发布；本文不定义 |
| 7 | `SourceTurnRef` | `contract.Ref` | 只能来自P0-1具名Turn Owner Reader |
| 8 | `SourceTurnOrdinal` | uint32 | 满足Turn/Tool/ExpectedCurrent等式 |
| 9 | `Purpose` | string | 与Knowledge V2 request/projection exact一致 |
| 10 | `ScopeAuthorityBindingDigest` | digest | 由P0-3正式合同提供并复读，不由Knowledge补造 |
| 11 | `OwnerStableClosureDigest` | string | Knowledge V2 stable closure exact digest |

sealed wire对象为`KnowledgeContextSourceAdapterStableAssociationV1{Canonical, StableAssociationDigest}`；`StableAssociationDigest=contract.Digest(Canonical)`。

Knowledge V2 StableClosure已经绑定Record/Package/Snapshot/Pointer、Source/Evidence/Projection、Citation/License/Conflict、Content、Coverage/NextCursor/Result/Evidence digest、预算和ordered items。因此Adapter只携`OwnerStableClosureDigest`，不得复制这些领域字段形成第二DTO。

Stable body排除AttemptInspectionRef、CurrentProjectionRef、ExactContentObservation refs、Binding current digests、CheckPhase、所有checked/expiry、Envelope self digest、正文bytes、Context `knowledge_reference`/pending/proof/Frame refs。

## 5. Opaque fresh envelope

`KnowledgeContextSourceAssociationEnvelopeV1`字段顺序冻结如下：

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `DigestDomain` | string | 固定Envelope domain且进入digest |
| 2 | `ContractVersion` | string | 固定Adapter版本 |
| 3 | `ObjectKind` | string | 固定Envelope ObjectKind |
| 4 | `StableAssociation` | 上节sealed对象 | 重算`StableAssociationDigest` |
| 5 | `OwnerAdapterBindingCurrent` | role + Runtime full current projection | role固定`owner_adapter` |
| 6 | `CoordinatorBindingCurrent` | role + Runtime full current projection | role固定`application_coordinator` |
| 7 | `ContextAdapterBindingCurrent` | role + Runtime full current projection | role固定`context_adapter` |
| 8 | `OwnerAttemptInspectionRef` | Knowledge `contract.Ref` | fresh V2 Inspection exact ref |
| 9 | `OwnerCurrentProjectionRef` | Knowledge `contract.Ref` | fresh V2 Projection exact ref |
| 10 | `OwnerExactContentObservationRefs` | ordered `[]contract.Ref` | 按Owner projection rank；非空、拒绝重复 |
| 11 | `CheckPhase` | Knowledge `CheckPhaseV2` | 仅`s1`或`s2` |
| 12 | `AdapterCheckedUnixNano` | int64 | Adapter fresh clock，必须正值 |
| 13 | `ExpiresUnixNano` | int64 | 所有currentness上界最小值 |

sealed wire对象为`KnowledgeContextSourceAssociationEnvelopeV1{Canonical, Digest}`；`Digest=contract.Digest(Canonical)`。

Envelope没有`Current`、Revision/CAS/Pointer、Candidate、Fragment kind、Frame或body。Application只能把它作为opaque association传递；Context不能只靠Envelope生成`knowledge_reference`，必须通过P0-5 Binding和S2 exact复读。

正文bytes不进入Envelope或Application缓存。未来Context materialization必须沿同一Adapter调用栈使用Knowledge V2 bounded `ReadContentExact`；License、ContentRef、ObservedDigest及Knowledge source chain由Owner Observation/Projection exact refs证明。

## 6. Validate与Binding

每次S1/S2都必须：

1. Validate三项role/member/Capability闭集、每个`ProviderBindingRefV2`、Generation association、P0-3 Application Attempt、SourceTurn及所有Knowledge refs；
2. 通过`GenerationBindingAssociationCurrentReaderV1`读取exact association Fact，要求Ref exact、Fact/Candidate canonical有效、State=`active`且fresh；
3. 将ProviderBindings的`BindingSetID/Revision/Digest/SemanticDigest`逐字段等同于`association.Candidate.Binding`同名字段，并要求三项Ref的BindingSet ID/revision全部相同；
4. fresh Inspect三个role Ref；每个`ProviderBindingCurrentProjectionV2`必须`ValidateCurrent(expectedRef, now)`、State=`active`，其`BindingSetDigest`、`BindingSetSemanticDigest`逐字段等于`association.Candidate.Binding`；
5. association Candidate.Generation必须与本次P0-3 generation exact一致；跨generation复用、role substitution、或Projection虽Ref exact但set digest splice全部Fail Closed；
6. 调用Knowledge V2 `InspectAttempt`、`InspectForTurn`和逐项bounded `ReadContentExact`；
7. exact比较Owner、Identity/Session/Turn、Purpose/Scope/Authority、AllowedLicenses/Sensitivity、StableClosure、Pointer与Projection/Content Observation链；
8. 再次复读三个Binding、Generation association和Adapter fresh clock；
9. 计算TTL最小值后才seal Envelope。

未来Adapter生产包只能依赖Knowledge公开合同、Runtime `core/ports`和正式P0-3 public contract；禁止依赖Runtime kernel/foundation/fakes、Harness private/internal、Context kernel/store或其他组件实现包。

Capability/Binding必须单一选择Knowledge V2 Adapter。V1/V2双Binding、第二Pointer/current、同generation多个Knowledge current source或只验Manifest不验BindingSet均Fail Closed。

## 7. S1/S2与lost reply

```text
P0-3 Application Attempt + P0-1 Turn exact
 -> Binding S1
 -> Knowledge V2 InspectAttempt/InspectForTurn/ReadContentExact(S1)
 -> Binding S1 reread
 -> Knowledge S1 opaque envelope
 -> Context P0-5 binding + pending outputs + P0-2 proof（外部Owner）
 -> Binding S2
 -> Knowledge V2 InspectAttempt/InspectForTurn/ReadContentExact(S2)
 -> Binding S2 reread
 -> Knowledge S2 opaque envelope
 -> Context atomic Apply/Generation CAS（外部Owner）
```

S2必须携带S1 `StableAssociationDigest`。S1/S2 fresh Inspection/Projection/Observation refs、Binding digests、checked/expiry和Envelope digest可以不同；StableAssociationDigest、SourceTurn、Purpose/Scope/Authority、Knowledge StableClosure和ordered exact集合必须相同。Pointer、License、Conflict、Citation、NextCursor或Content任何stable漂移都关闭整个refresh。

Adapter纯只读、零Provider/网络/Resolver。lost reply不新建Knowledge Attempt、不Retrieve、不远程读取；已关联Envelope时Inspect原P0-3 attempt，未关联时只能基于原Knowledge Attempt fresh复读，并要求stable digest相同。

## 8. TTL/currentness

`now >= ExpiresUnixNano`即过期。Envelope TTL取以下非零上界最小值：P0-3 NotAfter；P0-1 Session/Turn；Knowledge Attempt/Projection/items/StatePlaneBinding/Content Observation；Record/Package/Snapshot/Pointer/Source/Citation/License/Conflict；三个Runtime Binding projections；Generation association；Identity/Authority/Policy；S2时另含P0-5 Binding、Context pending/proof/ExpectedCurrent。

Adapter调用前、Owner读后、返回前均取fresh clock；clock rollback、锁/Get跨TTL、Binding漂移、Pointer切换、License缩小、withdrawn/evicted/poisoned均返回零Envelope/零body。caller时间和Adapter cap不能延长Owner TTL。

## 9. Closed errors

| 条件 | Knowledge closed error |
|---|---|
| malformed、unknown/type/version、空/重复Observation refs、canonical失败 | `ErrInvalidArgument` |
| exact ref、Binding、Generation、StableClosure、Pointer/Projection/Content链漂移 | `ErrEvidenceConflict`或`ErrRevisionConflict` |
| TTL到期、clock rollback、Owner/Binding不再current | `ErrNotCurrent` |
| Scope/Authority/Identity/Session/Turn/Purpose不一致 | `ErrScopeDenied` |
| Sensitivity或License越界 | `ErrSensitivityDenied`或`ErrCandidateRejected` |
| exact content已evicted | `ErrNotFound` |
| Owner或Binding inspection覆盖不完整 | `ErrInspectionIncomplete` |
| 调用V1、远程路径，或production root未装配获批ports却直接启用 | `ErrUnsupported` |

失败返回零Envelope、nil body、零状态变化；不得Partial Coverage、成功空Envelope、自由文本reason或远程fallback。

## 10. NO-GO反例

1. Adapter复制Knowledge Projection/Items/source chain形成第二current；
2. Envelope带`Current=true`、Pointer/current CAS或Fragment kind；
3. Knowledge Adapter创建`knowledge_reference` Candidate/Fact；
4. Adapter用V1或Context缓存补Session/Turn/License；
5. fresh Inspection/Projection/TTL进入StableAssociationDigest；
6. S2 fresh ref变化被当stable漂移，或stable漂移却被忽略；
7. Pointer/License/Conflict/Citation变化仍沿用S1 Envelope；
8. 只验Knowledge Adapter Binding，不验Coordinator/Context/Generation Binding；
9. Binding读后漂移仍返回Envelope；
10. Application缓存正文、Citation text或License内容；
11. evicted/withdrawn后自动启动Resolver/remote Retrieval；
12. P0-3未发布就私建Application Port；
13. Context未接受`knowledge_reference`却换成Memory/Artifact kind；
14. Context cardinality仍Knowledge=0时root先启用来源1；
15. Harness直接调用Knowledge Adapter或消费Envelope。

## 11. 无SCC DAG与门禁

```text
Knowledge V2 Go/public Reader --+
P0-1 Turn exact Reader ---------+--> Knowledge Adapter review/implementation
Runtime Binding readers --------+               |
P0-3 Application Port ----------+               v
P0-5 knowledge_reference -------+--> Context nonzero Knowledge source contract
P0-2 Context proof -------------+               |
                                                v
                                       production root/G6B
                                                |
                                                v
                                       Context exact Frame -> Harness
```

Knowledge V2、Harness exact mapping、Application三阶段、Context proof/`knowledge_reference`与nonzero reference source均已YES，Adapter实现关闭原P0-4的Knowledge侧reference切片；production root与远程路径不因此启用。
