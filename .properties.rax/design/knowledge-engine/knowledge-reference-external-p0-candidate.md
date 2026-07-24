# P0-5 `knowledge_reference` 联合审查候选

> 状态：**design_frozen / context_owner_implemented / reference_integration_software_test_yes**。Context Owner已发布`knowledge_reference` FragmentKind与exact source binding，Knowledge V2 Reader/Adapter经Application三阶段进入非零reference fixture。真实production root、远程Retrieval/Resolver或Provider路径仍NO-GO。

## 1. Owner与边界

| 对象 | 唯一Owner | Consumer/协调者 | 禁止 |
|---|---|---|---|
| Record/Package/Snapshot/Pointer/Source/Projection/Citation/License/Conflict、StableClosure、exact content | Knowledge Owner | Application无损传递Owner refs；Context做S1/S2复读 | Context缓存或Candidate不得成为Knowledge真值 |
| `knowledge_reference` Binding、Candidate、Fragment Fact、Frame/Generation | Context Owner | Harness只消费published exact Frame | Knowledge/Memory/Application不得创建Context Fact |
| 三阶段跨Owner编排 | Application | Context与Knowledge public Port | Application不得补造Owner字段、proof、Citation/License |
| Provider/远程Retrieval | 当前unsupported | 无 | 不得藏入Current Reader或正文读取 |

检索命中、Projection和exact body都只是Observation/Result refs；不是Context Frame或正式Knowledge新事实。Context必须复读Knowledge Owner current projection与exact content后，才能seal自己的reference binding。

## 2. Live复用与禁止第二DTO

- Knowledge live V1/V2 Reader属于同一Owner合同族并已完成软件验收；`knowledge_reference`仍只是Context Owner未接受的外部候选，不改变Owner current真值。
- P0-5直接引用Knowledge Owner同一V2合同族的`CurrentProjectionV2`、`ProjectionItemV2`、`ExactContentObservationV2` exact refs/digest；不复制其完整DTO、不建立neutral第二仓/第二current。
- `SourceTurnRef`只来自Harness committed PendingAction current经public Adapter的无损映射；`SourceTurnOrdinal`必须满足`Tool.Execution.Turn == ExpectedCurrent.Turn`。
- Context live `FragmentKind`尚无`knowledge_reference`，live `ContextCandidate.SourceRef+SourceRevision`也缺Digest和完整Knowledge source chain；因此当前必须`ErrUnsupported`，不得换成`memory_recall`/`artifact_reference`。

## 3. Canonical envelope与常量

| 项 | 值 |
|---|---|
| ContractVersion | `praxis.context/knowledge-reference/v1` |
| Binding Ref ObjectKind | `context_knowledge_reference_binding_ref` |
| StableBody ObjectKind | `context_knowledge_reference_binding_stable_body` |
| Projection ObjectKind | `context_knowledge_reference_binding_current_projection` |
| Candidate ObjectKind | `context_knowledge_reference_candidate` |
| FragmentKind candidate | `knowledge_reference` |
| Binding ID domain | `praxis.context.knowledge-reference-binding-id` |
| Stable domain | `praxis.context.knowledge-reference-binding-stable` |
| Projection domain | `praxis.context.knowledge-reference-binding-current` |
| Association domain | `praxis.context.knowledge-reference-association` |

Digest算法：

```text
sha256(CanonicalJSON({
  "domain": <constant>,
  "contract_version": "praxis.context/knowledge-reference/v1",
  "object_kind": <constant>,
  "body": <按字段声明顺序、Digest字段置空后的对象>
}))
```

Canonical JSON禁止unknown/duplicate/trailing字段和`omitempty`；exact ref逐字段校验ID/revision/digest；列表按Knowledge Owner已冻结语义顺序，不能由Context按map last-wins或caller Rank重排。untrusted输入失败只返回错误，禁止panic。

## 4. Binding Ref与StableBody

### 4.1 `ContextKnowledgeReferenceBindingRefV1`

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `ObjectKind` | string | Binding Ref ObjectKind |
| 2 | `ID` | string | Binding ID domain派生 |
| 3 | `Revision` | uint64 | v1固定1 |
| 4 | `Digest` | Digest | 等于StableBody canonical digest |

`ID="ctx-knowledge-reference-binding:v1:" + hex(sha256(CanonicalJSON({execution,source_turn_ref,knowledge_owner,knowledge_stable_closure_digest,item_rank,content_ref})))`。同ID不同revision/digest返回Conflict。

### 4.2 `ContextKnowledgeReferenceBindingStableBodyV1`

字段声明顺序即canonical顺序：

| 顺序 | 字段 | wire类型 | 来源/约束 |
|---:|---|---|---|
| 1 | `ContractVersion` | string | 固定合同版本 |
| 2 | `ObjectKind` | string | StableBody ObjectKind |
| 3 | `Execution` | Context ExecutionBinding | Run/Scope/Authority exact |
| 4 | `SourceTurnRef` | P0-1 `TurnExactRefV1` | 具名Turn Owner exact ref |
| 5 | `SourceTurnOrdinal` | uint32 | 与Tool/ExpectedCurrent exact一致 |
| 6 | `KnowledgeOwner` | Context OwnerRef | 指向Knowledge组件及binding digest |
| 7 | `KnowledgeStableClosureDigest` | Digest | Knowledge V2 stable closure；排除fresh字段 |
| 8 | `Coverage` | Knowledge public Coverage wire value | 与Owner Projection一致 |
| 9 | `NextCursor` | string | 与Owner Projection一致；变化必须改变stable digest |
| 10 | `ResultDigest` | Digest | Owner persisted Result exact digest |
| 11 | `EvidenceDigest` | Digest | Owner persisted Evidence exact digest |
| 12 | `ItemRank` | int | Knowledge semantic order中的exact rank |
| 13 | `RecordRef` | Knowledge exact Ref | 与ProjectionItem一致 |
| 14 | `PackageRef` | Knowledge exact Ref | 与published chain一致 |
| 15 | `SnapshotRef` | Knowledge exact Ref | 与published chain一致 |
| 16 | `PointerRef` | Knowledge exact Ref | Owner current Pointer exact ref |
| 17 | `SourceRefs` | ordered Knowledge exact Refs | ID/revision/digest逐项Validate、拒绝重复 |
| 18 | `EvidenceRefs` | ordered Knowledge exact Refs | 同上 |
| 19 | `ProjectionRefs` | ordered Knowledge exact Refs | 同上 |
| 20 | `CitationDigest` | Digest | Owner已冻结Citation集合digest |
| 21 | `LicenseDigest` | Digest | Owner已冻结License集合digest |
| 22 | `ConflictDigest` | Digest | Owner已冻结Conflict集合digest |
| 23 | `ContentRef` | Knowledge ContentRef | owner-local exact正文锚点 |

StableBody明确排除：Knowledge fresh CurrentProjection Ref、ExactContentObservation Ref、AssociationDigest、CheckPhase、OwnerCheckedAt/ObservedAt、ExpiresAt、Context Candidate/Frame/Generation、Binding self ref/digest与正文bytes。fresh时间或TTL改变不得改变StableBody；NextCursor、Result/Evidence digest、Citation/License/Conflict或任何exact source chain变化必须改变StableBody。

## 5. Fresh Projection、Association与Reader

### 5.1 `ContextKnowledgeReferenceBindingCurrentProjectionV1`

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `ContractVersion` | string | 固定合同版本 |
| 2 | `ObjectKind` | string | Projection ObjectKind |
| 3 | `BindingRef` | Binding Ref | exact |
| 4 | `StableBody` | StableBody | 重算等于BindingRef.Digest |
| 5 | `KnowledgeCurrentProjectionRef` | Knowledge V2 exact Ref | S1或S2 fresh ref |
| 6 | `ExactContentObservationRef` | Knowledge V2 exact Ref | local-only、bounded body observation |
| 7 | `AssociationDigest` | Digest | 见5.2 |
| 8 | `CheckPhase` | string | `s1`或`s2` |
| 9 | `Current` | bool | 仅true可交给Context pending/apply |
| 10 | `OwnerCheckedUnixNano` | int64 | Context Owner fresh check time |
| 11 | `ExpiresUnixNano` | int64 | 所有来源TTL最小值 |
| 12 | `Digest` | Digest | Projection domain重算 |

### 5.2 Association digest

Association canonical body字段顺序固定为：

1. `BindingRef`
2. `KnowledgeCurrentProjectionRef`
3. `ExactContentObservationRef`
4. `SourceTurnRef`
5. `SourceTurnOrdinal`
6. `Purpose`
7. `ScopeAuthorityBindingDigest`
8. `CheckPhase`

使用Association domain与同一contract version封装。Purpose/Scope/Authority来自Knowledge Owner V2 request/projection回显，Context/Application不得补值。S1/S2 Association digest可以不同；StableBody与Knowledge StableClosure、exact ordered source chain必须相同。

### 5.3 Reader候选

```text
InspectKnowledgeReferenceBindingExactV1(
  context.Context,
  ContextKnowledgeReferenceBindingRefV1,
) -> ContextKnowledgeReferenceBindingStableBodyV1

InspectKnowledgeReferenceBindingCurrentV1(
  context.Context,
  ContextKnowledgeReferenceBindingCurrentRequestV1,
) -> ContextKnowledgeReferenceBindingCurrentProjectionV1
```

CurrentRequest字段顺序：`ContractVersion string`、`ObjectKind string=context_knowledge_reference_binding_current_request`、`BindingRef`、`KnowledgeCurrentProjectionRef`、`SourceTurnRef`、`SourceTurnOrdinal uint32`、`Purpose string`、`ScopeAuthorityBindingDigest Digest`、`CheckPhase string`、`RequestedNotAfterUnixNano int64`、`Digest Digest`。

这些是Context-owned sealed binding/projection，不替代Knowledge Owner `InspectAttempt/InspectForTurn/ReadContentExact`。Context只有在Application提供真实Knowledge Owner S1/S2结果后才能seal；production root未装配时生产调用仍为0。

## 6. `knowledge_reference` Candidate候选

Context Owner未来若接受kind，`ContextKnowledgeReferenceCandidateV1`最小加法字段为：

| 字段 | 约束 |
|---|---|
| `ContractVersion` / `ObjectKind` | 固定本合同值 |
| `Kind` | 固定`knowledge_reference`，禁止降级换kind |
| `Owner` | Context Owner；不是Knowledge Owner |
| `Execution` | 与Binding StableBody exact一致 |
| `SourceBindingRef` | `ContextKnowledgeReferenceBindingRefV1`，替代不足的`SourceRef+SourceRevision` |
| `Content` | 与StableBody.ContentRef及S1 exact content observation一致 |
| `Trust` | 只允许`observation`或Policy允许的`restricted_material`，禁止`authoritative_instruction` |
| `Sensitivity` | 不低于Knowledge Owner值，不能降级 |
| `Mode` | 固定`reference`；inline须由Context另行materialize并重新审查 |
| `TokenEstimate` / `EstimatorDigest` | 来自Context Recipe/Estimator，受总预算约束 |
| `CreatedUnixNano` / `ExpiresUnixNano` | fresh字段，不进入Binding StableBody |
| `Digest` | Candidate自身fresh canonical digest |

Candidate仍只是Context pending输入；只有P0-2 proof、S2和Context atomic Apply+Generation CAS成功后，引用才可进入published exact Frame。Knowledge不得返回这个Candidate或Fragment Fact。

## 7. S1/S2与TTL

```text
Knowledge Owner S1 InspectForTurn + ReadContentExact
 -> Application无损传递Owner refs/body上限
 -> Context exact复读并sealBinding StableBody + S1 fresh Projection
 -> Context seal pending DomainResult/Manifest/Frame/Generation
 -> Context seal P0-2 final TransitionProof
 -> Knowledge Owner S2 InspectForTurn + ReadContentExact
 -> Context复读Binding、S2 Projection、StableClosure与exact source chain
 -> Context atomic ApplySettlement + Generation current CAS
 -> publish Frame
```

`now >= ExpiresUnixNano`即过期。Context Binding Projection TTL取以下非零上界最小值：P0-1 Session/Turn、Knowledge Attempt/CurrentProjection、Record/Package/Snapshot/Pointer、Source/Evidence/Projection/Citation/License/Conflict、ExactContentObservation/Content、Context Recipe/Authority/parent/current/pending、Refresh NotAfter。Context cap只能缩短，不能延长Owner TTL。

S1/S2必须由各Owner fresh clock和同一Owner锁域裁决；Context检查只能再次收紧。Get期间跨TTL、Pointer/License/Conflict/Content漂移、evicted或poisoned均返回零body并使整个refresh Fail Closed。

## 8. Closed errors

| 条件 | Context映射 |
|---|---|
| malformed、unknown、version/type mismatch、非法kind/rank | `ErrInvalid` |
| exact ref、StableClosure、source chain、content、association漂移/重复 | `ErrConflict` |
| TTL到期、clock rollback、锁/Get跨TTL | `ErrExpired` |
| Identity/Session/Turn/Scope/Authority/Purpose/License/Sensitivity不符 | `ErrUnauthorized` |
| exact binding/content已evicted | `ErrNotFound` |
| Knowledge/Context Owner reader不可用 | `ErrUnavailable` |
| lost reply或inspection覆盖不完整 | `ErrUnknown` / `ErrInspectOnly` |
| kind/Reader/Application Port/adapter/root未发布，或远程路径 | `ErrUnsupported` |
| body/预算超过硬上限 | `ErrLimitExceeded` |

失败返回零Projection、零Candidate、零body、零Context current变化。远程缺正文不得自动Resolve；当前Provider调用数固定0。

## 9. 必须拒绝的反例

1. Knowledge或Application创建`knowledge_reference` Context Fact；
2. Context未接受kind时改用`memory_recall`/`artifact_reference`；
3. 只用`SourceRef+SourceRevision`，不绑定Digest与完整Knowledge chain；
4. Context缓存命中被当成Knowledge current/正式事实；
5. Provider命中或Retrieval Result直接成为Frame；
6. S1后发布Frame，再做S2；
7. S2 Pointer/License/Conflict/Content漂移仍沿用S1 binding；
8. fresh projection/time/TTL变化被写入StableBody；
9. NextCursor、Result/Evidence或Citation/License/Conflict变化却stable digest不变；
10. caller Rank重排、duplicate semantic Record last-wins；
11. stale/withdrawn/evicted/poisoned内容自动远程Resolve或返回旧body；
12. lost reply新建Knowledge Attempt或推断Apply成功；
13. Cross-Run/Session/Turn replay，或从ordinal补造Turn ref；
14. Application补造Purpose/Scope/Authority/License/Citation；
15. Harness直读Knowledge Reader/body或消费Binding Projection；
16. production root绕过Application三阶段与Context S2就把Knowledge来源或Reader调用数改为非0。

## 10. 无SCC DAG与联合裁决门

```text
P0-1 Turn exact Ref/Reader --------+
                                    |
Knowledge Owner V2 public Reader ---+--> P0-5 Context kind/binding/reader
                                    |               |
P0-2 Context TransitionProof -------+               v
                                             P0-3 Application 3-stage Port
                                                        |
                                                        v
                                             P0-4 adapters/nonzero/root
                                                        |
                                                        v
                                               Context exact Frame -> Harness
```

Context Owner已接受kind、Binding canonical与Reader链；P0-3只协调而不拥有事实，Knowledge Adapter只投影Owner refs。reference integration已关闭原P0-5；production root与远程路径保持独立NO-GO。
