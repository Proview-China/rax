# Context Engine对象与版本合同

状态：`CTX-D09-R1` A/B-local、Application公共Port Adapter与Memory/Knowledge B-cross fixture、`ContextOfflineSDKV1`、`ContextOfflineIngressV1`及`ContextEngineeringSDKV1`独立软件验收均已YES。production C层未启用。

## 1. 通用Envelope

所有权威事实至少包含：`ContractVersion`、`SchemaRef`、`ID`、`Revision`、`FactDigest`、`PayloadDigest`、`OwnerBinding`、`ExecutionScope`、`ScopeDigest`、适用`RunID/Turn/AttemptID`、`AuthorityBinding`、`Created/Updated/ExpiresUnixNano`。集合必须规范排序；Digest使用Runtime canonical JSON；未知字段严格拒绝。

内容正文与事实Envelope分离。正文通过有界inline或内容寻址Ref提供；Credential、Secret和内部不可见规则不得进入Frame正文或Trace。

## 2. Recipe与Prompt资产

### `ContextRecipeFact`

| 字段组 | 必需内容 |
|---|---|
| 身份 | RecipeID、SemanticVersion、Revision、RecipeDigest、SchemaVersion |
| 归属 | Tenant、Owner Binding、Authority Binding、Publication Scope |
| 规则 | Source Selectors、Admission Policy Ref、Ordering、Budget、Degradation、Compaction、Render、Cache Policy Ref |
| 适配 | Model Profile Range、Harness Capability Range、Expected Injection Policy |
| 状态 | draft/evaluated/review_pending/published/superseded/revoked、ParentVersion、TTL |
| 证据 | EvaluationSetDigest、Review/Verdict Ref、Publish Effect/Settlement Ref |

发布后的版本不可修改；新改动产生新SemanticVersion与新Fact。Rollback不是改写旧版本，而是发布一个指向历史版本的新Current Binding Fact。

### `PromptAssetFact`

用户已裁决V1采用“内嵌规范化片段规格 + exact `ContentRef`”，不新增第二层`PromptFragment Fact`，也不把Prompt降格为普通无版本Source。

```go
type PromptFragmentRoleV1 string
const (
    PromptFragmentInstructionV1 PromptFragmentRoleV1 = "instruction"
    PromptFragmentExampleV1     PromptFragmentRoleV1 = "example"
    PromptFragmentPolicyV1      PromptFragmentRoleV1 = "policy"
)

type PromptFragmentSpecV1 struct {
    ID              string               `json:"fragment_id"`
    Role            PromptFragmentRoleV1 `json:"role"`
    Content         ContentRef           `json:"content"`
    Required        bool                 `json:"required"`
    TokenEstimate   uint64               `json:"token_estimate"`
    EstimatorDigest Digest               `json:"estimator_digest"`
    CacheStability  uint8                `json:"cache_stability"`
    Evidence        EvidenceRef          `json:"evidence"`
}

type PromptAssetV1 struct {
    ContractVersion    string                 `json:"contract_version"`
    ID                 string                 `json:"prompt_id"`
    SemanticVersion    string                 `json:"semantic_version"`
    Revision           uint64                 `json:"revision"`
    Owner              OwnerRef               `json:"owner"`
    AuthorityDigest    Digest                 `json:"authority_digest"`
    Sensitivity        Sensitivity            `json:"sensitivity"`
    Fragments          []PromptFragmentSpecV1 `json:"fragments"`
    ContentDigest      Digest                 `json:"content_digest"`
    RenderCompatibility []FactRef             `json:"render_compatibility"`
    Evidence           []EvidenceRef          `json:"evidence"`
    CreatedUnixNano    int64                  `json:"created_unix_nano"`
    ExpiresUnixNano    int64                  `json:"expires_unix_nano"`
}

type PromptAssetRefV1 struct {
    ID       string `json:"id"`
    Revision uint64 `json:"revision"`
    Digest   Digest `json:"digest"`
}
```

Role是Context语义，不是Provider `system/developer/user`字符串：`instruction`确定性映射为`FragmentInstruction + TrustAuthoritativeInstruction`；`example`映射为`FragmentConversation + TrustRestrictedMaterial`；`policy`映射为`FragmentPolicySnapshot + TrustRestrictedMaterial`。片段按`fragment_id`严格排序且唯一，1..64项；每个ContentRef必须Length>0且exact。`ContentDigest`规范seal完整片段规格，RenderCompatibility为1..64个规范排序且唯一的外部Profile/Renderer exact ref；每个片段Evidence必须包含在资产Evidence闭包中。

`PromptAssetRefV1`是Context-owned nominal exact ref，不能用普通`FactRef`或Recipe ref直接代入；其Digest seal完整PromptAsset，不只seal ID或ContentDigest。

PromptAsset不决定Frame Region、最终Role、顺序或Cache placement。`BuildPromptCandidatesV1`只在Context Owner Store exact Inspect资产后，把片段确定性投影为`ContextCandidate`；请求必须绑定distinct AssetRef、Execution（含Authority）、一个被资产声明的RenderCompatibilityRef、Created/NotAfter和RequestDigest。Candidate ID/Idempotency由完整冻结输入确定性派生，Sensitivity/Content/Evidence/TokenEstimate/CacheStability无损继承，Expires取资产与请求上界最小值。最终区域仍由Recipe Rule决定。

PromptAsset拥有distinct pre-release lifecycle：`draft → validated → evaluated → review_pending`，任一阶段可`rejected`。Lifecycle head只在Context Owner Store内expected-CAS；lost reply只Inspect exact head。`publish/rollback/revoke`继续由CTX-D07阻塞并显式unsupported，不能复用Recipe lifecycle ref、模型自评、普通FactRef或Run内Settlement。

### `ContextRecipeComparisonV1`

Owner-local纯结构报告绑定Base/Candidate两个exact Recipe Ref、规范排序的字段变化、Checked/Expires和自摘要。V1覆盖Recipe ID、SemanticVersion、Revision、Owner、Rule集合与顺序、Budget、RenderVersion和lifetime；变化只分`added|removed|modified|reordered`，值只记录before/after field digest，不复制内容。报告不判断质量、兼容性或发布资格，不写Recipe lifecycle/current，也不能替代Evaluation/Review。

## 3. Source Candidate与Admission

### `ContextCandidate`

| 字段组 | 必需内容 |
|---|---|
| Identity | CandidateID、Revision、Digest、Kind、SchemaRef、IdempotencyKey |
| Source | Source Owner Binding、SourceRef、SourceRevision、ContentDigest、EvidenceRef |
| Runtime | ExecutionScope、RunID、Turn、AttemptID、Authority、Currentness/Expires |
| Semantics | Sensitivity、TrustClass、Freshness、TokenEstimate+EstimatorProfile、CacheStability |
| Materialization | inline/reference、Range/Query、ResolverCapability、Required/Optional |

Source Owner只能声明候选内容和证据；不能设置最终Role、排序、Cache placement或Admission结果。

### `ContextAdmissionDecisionFact`

每个Candidate形成`admitted | excluded | rejected | residual`之一，并绑定Recipe、Frame Attempt、Candidate Digest、Policy Digest、Scope/Authority Currentness、Reason Code、预算贡献和Evidence。Required Candidate只有`admitted`或整个Frame Attempt失败；Optional才允许`excluded/residual`。

### `ContextParentFrameApplicabilitySourceCoordinateV1`

这是Context Owner的distinct nominal source coordinate，不是Runtime `OperationScopeEvidenceApplicabilityFactRefV3`、普通`FactRef`或Application DTO的类型别名。

| 字段 | 冻结要求 |
|---|---|
| `Kind` | 固定namespaced kind `praxis.context/parent-frame-current-v1`；其他Kind拒绝 |
| `ID` | 方案A固定为exact FrameID，作为Context Owner metadata index的首个可查key；不得使用不可逆hash ID，也不得假设FrameID跨Tenant/ExecutionScope全局唯一 |
| `Revision` | exact等于Frame Revision；V1 live Frame通常为1，但不得硬编码绕过Owner读取 |
| `Digest` | Context canonical digest，seal下述完整subject |

sealed subject必须包含：exact `FrameRef`、exact `ManifestRef`、`ContextGenerationRef{ID,Revision,Digest}+Ordinal`、`ExecutionScopeDigest`、Run ID、neutral Session ID/Revision/Digest、Turn ordinal、ParentFrame/Generation binding digest、RecipeRef和AuthorityDigest。改变FrameID时改变Coordinate ID/Digest；改变其他任一字段至少改变Digest。禁止只哈希Frame ID，或把Application `SingleCallParentFrameCoordinateV1`、Runtime公共ref、普通FactRef或通用ObjectRef直接强转为该类型。

### `ContextParentFrameCurrentReaderV1`

公开只读请求`ContextParentFrameCurrentRequestV1`至少绑定：ContractVersion、exact SourceCoordinate、FrameRef、ManifestRef、GenerationRef+Ordinal、ExecutionScopeDigest、Run ID、neutral Session exact coordinate、Turn、ParentFrame/Generation binding digest、`CheckedUnixNano`、请求`NotAfterUnixNano`和RequestDigest。

返回Context-owned `ContextParentFrameCurrentProjectionV1`：ContractVersion、exact SourceCoordinate、FrameRef、ManifestRef、GenerationRef+Ordinal、ExecutionScopeDigest、`Current=true`、CheckedUnixNano、ExpiresUnixNano和ProjectionDigest。Projection不包含Evidence资格、Permit、Tool watermark、Provider handoff或任何写能力。

Reader算法冻结为：

1. S1先以完整Source Coordinate四元组调用Context Owner `ResolveExactSourceBinding`；Owner metadata index返回sealed subject中的exact Frame/Manifest/Generation refs、ordinal、scope/run/session/turn及Parent binding。不得由Adapter从Coordinate Digest猜测Frame Digest；
2. 以请求和binding中的完整`Frame FactRef{ID,Revision,Digest}`加预期`ExecutionScopeDigest`调用`FrameByExactRef`，再以完整Manifest/Generation FactRef调用对应exact reader，并读取Generation current pointer、Recipe/Authority可读current投影及ReferenceStore；
3. 从取回Frame计算实际ExecutionScopeDigest并与请求、sealed binding和最终公共projection逐项核对；重算Frame/Manifest/Generation/ExecutionScope/Run/Session/Turn/Parent binding并完整执行`kernel.InspectFrame`，包括所有Stable/Semi-stable/Dynamic/Rendered ContentRef；
4. 观察租约`ExpiresUnixNano = min(request.NotAfter, checked+30s, Frame真实Expires, Manifest真实Expires, Generation current上界, Recipe current上界, Authority current上界, 其他实际可读current上界)`；V1 30秒是最大cap而非SLA，不能延长任何领域TTL；
5. 返回前S2重新执行同一Owner-current复读和完整`InspectFrame`；任一ref/digest/current pointer/ExecutionScope/Run/Session/Turn漂移或TTL crossing都Fail Closed；
6. Generation/Recipe/Authority current上界不可读时不得伪造Expiry；当前live `ContextGeneration`没有Expiry/current pointer，因此实现前必须提供Context-owned只读current来源。

最小只读metadata ports冻结为：

- `ContextParentFrameSourceBindingReaderV1.ResolveExactSourceBinding(exact SourceCoordinate) -> sealed Frame/Manifest/Generation/scope/run/session/turn/parent binding`；
- `ContextFrameMetadataReaderV1.FrameByExactRef(exact FrameRef, expected ExecutionScopeDigest) -> ContextFrame`；
- `ContextManifestMetadataReaderV1.ManifestByExactRef(exact ManifestRef, expected ExecutionScopeDigest) -> ContextManifest`；
- `ContextGenerationMetadataReaderV1.GenerationByExactRef(exact GenerationRef, expected ExecutionScopeDigest) -> ContextGeneration`；
- `ContextGenerationCurrentPointerReaderV1.InspectCurrentGenerationPointer(ExecutionScopeDigest, RunID, neutral Session coordinate, Turn) -> exact GenerationRef+Ordinal+ParentBindingDigest+Expires`。

这些Port只读且由Context Owner实现。Source binding index必须以完整四元Coordinate和Owner scope分区定位；同FrameID多Tenant/多scope、同ID换Revision/Digest或返回多项均为Conflict，不能任选一项。live `ReferenceStore`只存Content bytes，不能替代metadata ports；runtimeadapter构造参数不得直接塞入Frame/Manifest/Generation对象、sealed binding map或不可变快照冒充current。未来线程安全Fake只用于测试，必须在每次调用时从Owner-style store复读，并支持NotFound、Unavailable、revision/digest drift、跨scope歧义与并发current pointer切换。

Reader unavailable或任一检查不确定时返回Unavailable/Stale/Conflict，不返回`Current=true`投影。S1/S2均为只读：不产生ContextTurnRefresh、DomainResult、Runtime Settlement、Context ApplySettlement、Generation CAS、新Frame或Continuation。

### `ContextTurnRefreshPortV1`

这是Application Owner已发布的窄公共Port/DTO，不是Harness私有`ports.ContextPort`。`context-engine/applicationadapter`只依赖Application公共`contract/ports`并实现三段映射；Application、Harness不反向import Context实现。B-cross test-only fixture已手工注入Memory/Knowledge Owner V2 Reader并完成验证；production composition root仍未接线。首切面严格N=1 settled Tool action：`Tool=1`，`Memory=0..1`，`Knowledge=0..1`，`Continuity=0`。

```go
type ContextTurnRefreshPortV1 interface {
    PrepareContextTurnRefreshV1(context.Context, application.ContextTurnRefreshPrepareRequestV1) (application.ContextTurnRefreshPreparedV1, error)
    ApplyContextTurnRefreshV1(context.Context, application.ContextTurnRefreshApplyRequestV1) (application.ContextTurnRefreshResultV1, error)
    InspectContextTurnRefreshV1(context.Context, application.ContextTurnRefreshInspectRequestV1) (application.ContextTurnRefreshResultV1, error)
}
```

三段语义不可合并：`Refresh`核验settled Tool exact chain，执行S1、确定性Reservation/冻结并产生`pending` Context DomainResult；候选Frame/Generation此时不得成为current。`Apply`先执行S2 fresh owner-current复读，只有S2全部通过后才在一个原子可见性边界内同时提交Context ApplySettlement与expected Generation current CAS；`Inspect`严格只读，只按原RefreshAttempt exact ref恢复Unknown/lost reply，不创建新Attempt、Frame、Generation或Settlement。

| 字段组 | `ContextTurnRefreshRequestV1`必需输入 |
|---|---|
| Identity | ContractVersion、RefreshAttemptID、FrameID、NextGenerationID、IdempotencyKey、CheckedUnixNano、NotAfterUnixNano、RequestDigest；三个ID均由冻结输入确定性派生 |
| Execution | ExecutionScopeDigest、RunID、Session exact ref、ExpectedSessionRevision/Digest、具名Session/Turn Owner Reader的SourceTurn exact ref、`SourceTurn.Ordinal=T`、Context `childExecution.TargetTurn=T+1`、AuthorityDigest；Refresh request不携带TransitionProof |
| Assembly | HarnessGeneration exact ref、GenerationBindingAssociation exact ref、Binding/Activation current digest与Expires |
| Context | Recipe exact ref、CTX-D10 ParentFrame current projection、ParentFrame/Manifest/Generation exact refs、expected current Generation pointer revision/digest、NextGenerationOrdinal |
| G6A Input | 恰好一个settled `ToolResultV2` exact ref、对应current V4 Inspection exact ref、verified Association exact ref；三者必须绑定同一Execution/Action/Attempt/Result/currentness |
| Source cardinality | `Tool=1`、`Memory=0..1`、`Knowledge=0..1`、`Continuity=0`；非此基数在读取或写入前拒绝 |
| Memory/Knowledge source | 只消费对应Owner唯一public V2 Reader，经Application中立Reader/DTO映射；Context禁止平行nominal/DTO或Owner concrete Store/internal依赖 |
| `knowledge_reference` | 已冻结为Knowledge Owner exact projection的DynamicTail受限材料；不携Authority、不替代Knowledge current事实 |
| Source material | Application-owned current projection给出的有界摘要，或exact Artifact `{Owner,Version,Digest,Range}`；Receipt、Observation、原始或无界Tool输出禁止进入 |
| Prefix/Cache | 父Frame完整Stable/SemiStable `ContentRef{Ref,Digest,Length}`、StableSourceSetDigest、Recipe/Render/Model/Harness/ToolSchema/Authority/Isolation/ProviderProfile/KeyVersion及请求TTL上界 |

`ContextTurnRefreshPreparedV1`只返回RefreshAttemptRef、AdmissionDecisionRefs、ManifestRef、候选FrameRef、候选NextContextGenerationRef、Context-owned TransitionProof exact ref/digest、CachePlan/EconomicDecision、ExpectedInjectionManifestRef、ResidualRefs、pending Context DomainResultFactRef、Checked/Expires/Digest和`pending_domain_result`状态；不返回current Generation或Continuation。`ApplyContextTurnRefreshRequestV1`只携带原Attempt、pending Context DomainResult、TransitionProof exact ref/digest、expected Generation current pointer revision/digest和检查时间上界，不含任何Runtime settlement/ref字段。它禁止携带或复用Runtime `OperationSettlementV4`、任何additive Runtime settlement、上游Tool settlement或其别名。`ContextTurnRefreshResultV1`只有在S2通过且Context ApplySettlement+expected Generation current CAS原子成功后才可为`applied_current`。

### `SettledActionContextSourceCurrentReaderV1`

这是Application Owner后续实现的只读公共Port；Context Adapter只消费其projection，不实现或吞并Tool、Memory、Knowledge、Continuity事实。

```go
type SettledActionContextSourceCurrentReaderV1 interface {
    InspectSettledActionContextSourceCurrentV1(context.Context, SettledActionContextSourceRequestV1) (SettledActionContextSourceCurrentV1, error)
}
```

请求绑定G6A settled ToolResultV2、current V4 Inspection、verified Association、Execution/Run/Session/Turn/Action/Attempt、Checked/NotAfter与Digest。返回只允许中立exact ToolResult/DomainResult/Apply/V4 Inspection/Association refs、一个有界Source body或exact Artifact `{Owner,Version,Digest,Range}`、Checked/Expires/ProjectionDigest。它没有Execute、Settlement、CAS或Continuation能力；不得返回Receipt、Observation、Provider状态、raw/unbounded output，也不得把普通Application DTO冒充任一Owner current fact。Unknown、cancel、deadline保持indeterminate并返回零stale projection。

Turn是双坐标，不是一个可由上游Owner隐式增量的整数：`SourceTurn.Ordinal=T`必须exact等于settled Tool Execution与ExpectedCurrent的`uint32` Turn，完整Source Turn Ref只能从具名Session/Turn Owner Current Reader的current projection获得；`TargetTurn=T+1`只能由Context `childExecution`生成，用于候选child Frame/Generation归属。唯一`ContextTurnTransitionProofV1`的Owner是Context，Application只编排并传递exact refs，不mint、不改写proof。Memory/Knowledge Reader只回传其Owner的Source Turn T/current projection，不得自行`+1`、补造Session/Turn或产生proof。

`ContextTurnTransitionProofV1.StableClosureDigest` exact seal `ContractVersion + Owner(Context) + ExecutionScopeDigest + RunID + Session exact ref/revision/digest + SourceTurn exact ref/Ordinal + TargetTurn + childExecution digest + settled Tool chain + ParentFrame/Generation + pending Frame/Generation + RefreshAttempt`，明确排除phase、Checked/Expires和proof自身digest。`FreshObservationDigest` seal `StableClosureDigest + phase + Checked/Expires + Session/Turn/Tool/Parent/Generation S1 current projection digests`；proof自身digest再以置空的self-digest封口完整对象。产生/可见顺序唯一为`pending Frame/Generation seal → Context proof → S2 fresh reread → atomic ApplySettlement+expected Generation current CAS → publish`；proof产生不代表Frame/Generation current。S2必须同时复验stable closure不fresh observation的currentness，任一漂移时proof可留diagnostic，current pointer不可见。

调用顺序冻结为：`核验settled Tool exact chain → Application调用Refresh → Context S1/Reservation/Freeze/pending Frame+Generation+DomainResult seal → Context mint TransitionProof → Application调用Apply → Context S2 fresh owner-current复读 → 原子Context ApplySettlement + expected Generation current CAS → publish/G6B acceptance candidate`。本地迁移不创建、请求或消费任何Runtime settlement；S2失败不得发布current Generation，最多保留pending/proof/diagnostic fact。仅C层能力启用后才可由Application调用Harness continuation；Context不构造Continuation。

父Frame不可变。新Frame的StablePrefix和SemiStable必须逐项复用父Frame exact `ContentRef{Ref,Digest,Length}`，仅DynamicTail追加本次N=1 settled Tool结果。`PrefixDigest` seal完整StablePrefix ContentRefs；稳定cache identity使用`StableSourceSetDigest`，不得使用随DynamicTail变化的完整Manifest SourceSetDigest。Cache key至少seal ReuseScope、Isolation、Authority、Sensitivity、StableSourceSetDigest、Recipe、Render、Model、Harness、ToolSchema、StablePrefix、ProviderProfile与KeyVersion。Frame/Generation/Cache Expires严格取请求上界和全部Owner-current输入TTL的最小值，边界`checked >= expires`拒绝；不得伪造、延长TTL或宣称SLA。

RefreshAttempt/Frame/Generation ID由同一canonical request确定性派生，seal G6A exact链、scope/run/session、Source Turn exact ref/Ordinal T、Context childExecution Target Turn T+1、父Frame/Generation、recipe、stable source set/cache identity、source projection、expiry与idempotency；它们不seal尚未产生的TransitionProof，避免循环。Proof反向seal这些pending exact refs和Attempt。相同输入同ID/Digest可重放；同ID换内容Conflict。任一S1/S2、write、CAS或回包Unknown只Inspect原ID；cancel/deadline必须保真，写前零状态，写后进入`waiting_inspect`，不得分配新ID、重跑Tool或推进Turn。

Root门分三层：A）Context Owner-local kernel/store已完成；B-local）本组件test-only fixture已完成并通过二轮独立复审；B-cross）Application公共Port、Context Adapter及Memory/Knowledge test-only跨模块fixture已完成；C）production capability、真实跨模块composition、Harness Continuation与Turn推进仍NO-GO，必须等待production composition root及其系统验收。

## 4. Manifest与Frame

### `ContextManifestFact`

- ManifestID、Revision、Digest、FrameAttemptID；
- 精确Recipe/Model Profile/Harness Capability/Render版本；
- admitted/excluded Candidate及原因；
- Fragment Ref、Source版本、物化模式、Role/Position、Stable/Semi-stable/Dynamic区段；
- token estimate、预算、裁剪/降级、Reference Resolution；
- Prefix Breakpoints、Cache Eligibility与Expected Injection Manifest Ref；
- Scope/Run/Turn/Generation/ParentFrame、Trigger Event、Evidence与TTL。

Manifest冻结后不可增删Fragment。任何新增内容进入下一Frame Generation/Turn。

### `ContextFrameFact`

- FrameID、Revision=1、FrameDigest、ManifestRef+Digest；
- ParentFrameID、GenerationID/Ordinal、DeltaBase/DeltaRoot；
- StablePrefixRef+Digest、SemiStableRef+Digest、DynamicTailRef+Digest；
- RenderedPayload Ref+Digest、RenderCodecVersion；
- Scope/Run/Turn/Model Profile/Harness Capability；
- CachePlanRef+Digest、ExpectedInjectionManifestRef+Digest；
- SourceSetDigest、ArtifactAnchorSetDigest、Evidence、Created/Expires。

Frame ID exact-idempotent：同ID同Digest可重放；同ID不同Digest产生冲突。Frame正文不允许可变指针或延迟无治理网络取回。

`ContextFrameInspectPortV1`只接受exact FrameRef、ManifestRef、ExecutionBinding与current time，重算Manifest/SourceSet及stable/semi/dynamic/rendered ContentRefs后返回冻结引用；它不得联网、写Cache或解析可变指针。

### `ContextOutcomeFact`

绑定FrameRef、Model Attempt/Observation/Settlement、ActualInjectionManifest、Tool/Action/用户纠正Evidence、质量评估、cache read/write/dynamic token、费用、延迟、压缩损失和Evaluation Policy。它是Context优化证据，不代表Task/Goal/Artifact成功。

#### Owner-local exact Outcome/Evaluation/Feedback V1

- `ContextOutcomeMetricsV1`只记录可量化观测：input/output、cache-eligible prefix、cache read/write、dynamic token、retry、latency、cost与compaction loss。Provider usage仍是Observation；`cache_read_tokens>0`不自动产生Cache hit Fact。
- `ContextOutcomeFactV1` exact绑定Execution、Frame/Manifest/Recipe/Generation、Model Attempt/Response Observation refs、可选Model Settlement与ActualInjection Manifest ref、规范排序且唯一的Tool/Action refs、用户纠正Evidence、任务Evidence及Evaluation Policy。它没有`TaskSucceeded`、Runtime Outcome、Trust、Review或Authority写字段。
- `ContextEvaluationFactV1`绑定一个非空Outcome exact ref集合、Baseline/Candidate Recipe、Evaluation Policy、质量/经济/风险的百万分比整数、`better|worse|inconclusive`结论与Evidence。它只表达该Policy下的Context优化评测，不替代任务或领域Owner结论。
- `ContextFeedbackCandidateFactV1`绑定Base Recipe、Outcome集合、Evaluation、候选Change Digest、风险百万分比、Evidence和`candidate|evaluated|declined|review_pending`状态。Candidate不会修改Recipe current，也不能自动进入published。
- 三类事实均为Context Owner不可变rev1、exact-idempotent：同ID同Digest可重放，同ID换内容Conflict；Store只提供Put-once/Inspect，不联网、不发布、不创建Runtime Settlement或Continuity Event。

## 5. Artifact Anchor与Delta

### `ArtifactAnchorFact`

字段包括AnchorID/Revision/Digest、Artifact Owner Binding、ArtifactRef、ObservedVersion/Digest、Range/Symbol/Query、MaterializationMode、Frame/Generation、Source Evidence、Currentness、ExpiresAt。

### `ArtifactDeltaFact`

绑定BaseAnchor、TargetArtifactVersion/Digest、DiffAlgorithm/Version、Range、DeltaDigest、TargetAnchor、Evidence与RebaseCost。Delta必须能证明基线和目标；无法证明时拒绝增量并请求重新物化。

## 6. Generation与Compaction

### `ContextGenerationFact`

包含GenerationID、Revision、Digest、Ordinal、ParentGeneration exact ref、RootFrame、CompactionSummaryRef、RetainedAnchorSet、RecentTailRef、OpenEffects、OutstandingWork、Created/Expires和Evidence。Context Owner另维护只保存exact GenerationRef的current pointer，并用ExpectedCurrentGenerationRevision+Digest CAS；Continuity Event Ref只能是事后投影引用，不参与current裁决。

压缩后只有`RetainedAnchorSet`中ID/Revision/Digest完全一致且仍current的Anchor可继续参与diff。未列入、换包、过期或Authority漂移的旧Anchor必须重新物化。

### `CompactionSummaryFact`

包含SourceFrameRange、Algorithm/Model/Profile版本、SourceDigest、SummaryDigest、保留/丢弃分类、token前后、质量评估、UncompressibleRefs、Evidence。Summary只能作为新Fragment，不能成为被摘要事实的Owner Fact。

#### Owner-local首切面 exact 合同（`ContextCompactionV1`）

- `ContextCompactionSourceRangeV1`固定携带`FirstFrameRef`、`LastFrameRef`和`FrameCount`；首尾必须是exact `FactRef`，不得只按Frame ID描述范围。
- `ContextCompactionSummaryV1`固定携带`SourceGenerationRef`、上述SourceRange、`AlgorithmID/AlgorithmVersion`、可选exact `ModelProfileRef`、`SourceDigest`、`Summary ContentRef`、`RetainedAnchorRefs`、可选`RecentTail`、`OpenEffectRefs`、`OutstandingWorkRefs`、`UncompressibleRefs`、token前后、可选`QualityEvaluationRef`、`Evidence`及Created/Expires。所有Ref集合必须按`ID/Revision/Digest`规范排序、唯一且有界；`TokensAfter < TokensBefore`，Summary不升级任何来源事实。
- `ContextCompactionPlanV1`绑定exact expected Generation current pointer、SummaryRef、目标Generation ID、目标RootFrameRef、Checked/Expires和自摘要。Plan只准备候选Generation，不修改current pointer。
- 纯Prepare输出的`ContextGeneration`必须以expected current Generation为Parent，Ordinal严格`+1`，Summary exact绑定，RetainedAnchor/OpenEffect集合逐项复用Summary；同输入同Digest，expected current、Summary或RootFrame任一漂移Fail Closed。
- Owner-local Apply已实现S2 current复读与expected Generation current CAS；候选Manifest/Frame/Generation/current pointer只在同一Store锁域原子可见。回包Unknown/lost reply只Inspect原Compaction Attempt。该闭环不调用Runtime Settlement，不写Continuity，不创建外部Effect，也不宣称production State Plane。

## 7. Cache对象

### `ProviderCacheProfileRef`

由Model Profile/Provider Adapter Owner提供：Provider/Deployment/Model/Protocol、Capability/Manifest Digest、request control、key ownership、TTL control、state relation、breakpoint、cache read/write usage可观测性、价格/计量事实引用、ExpiresAt。

### `CachePartition`

| 维度 | 说明 |
|---|---|
| Audit Scope | 完整Tenant/Identity+Epoch/Authority/Lineage/Instance/Run/Effect上下文 |
| Reuse Scope | tenant/identity/lineage/instance/run之一及IsolationDigest；默认最窄 |
| Data | Sensitivity、SourceSetDigest、AuthorityEquivalencePolicyDigest |
| Semantic | Recipe/Prompt/Render/Model/Harness/ToolSchema/StablePrefix Digest |
| Provider | ProviderCacheProfile、Deployment、CacheKeyVersion、Retention/TTL |

跨Run/Instance复用必须由显式Reuse Scope、Authority等价策略、Sensitivity和Provider隔离共同允许；事实审计字段不得因复用而丢失。

### `CachePlanFact`

包含Partition、Stable Prefix范围、breakpoints、cache key材料、read/write/refresh策略、TTL、失效条件、预期reuse概率、read/write/keepalive成本、最小经济收益、Capability Currentness和Plan Digest。Plan不等于已创建Cache。

Cache key材料必须完整覆盖Partition全部语义维度；TTL为Recipe/Frame/Source/Artifact/Provider Profile/Assembly Binding与Activation/Cache policy的最短有效期。经济性判定使用先精确乘除、最终`uint64`饱和且加法不回绕的算术，只有`expected read savings > write + keepalive`才建议创建；建议不等于Effect或Entry。

### `CacheEntryFact`

包含EntryID/Revision/Digest、PartitionDigest、PrefixDigest、ProviderOperationRef、State、Created/LastInspected/Expires、InvalidationGeneration、Receipt/Observation/Evidence、Owner Inspect与Settlement Ref。`hit`必须由Owner当前Inspect/CAS产生；Provider usage仍是Observation。

## 8. Injection对象

### `ExpectedInjectionManifestFact`

绑定Frame、强制/可选区域、Role/Instruction/Tool/Workspace/Secret政策、允许Residual、Harness Capability与Digest。

该对象只属于Context Frame/field Injection Conformance链，不是Tool Surface Gate输入。Tool Surface的expected digest由Tool Owner的`ToolSurfaceManifest.ExpectedInjectionDigest`独立拥有；两者名称相近但语义、Owner、Digest subject和消费点不同，禁止互换、type-pun或建立隐式等式。

### `ProviderActualInjectionObservation`

由Model Invoker提供Route/Provider请求可见区域、追加/替换/opaque字段、native session、hidden compaction/retry可观测性、Context/Tool digests、Route/Attempt/source sequence与Evidence；只作Observation。

### `ActualInjectionManifestFact`

由Harness Model Turn Adapter聚合同一Run/Turn/Frame/Route/Attempt的Provider Observation与Harness实际发送顺序，绑定全部source sequence、不可观察区域和Residual。Harness只拥有该聚合事实，不判断是否符合Expected。

Observation集合非空；每个`ActualInjectionObservationRef`至少含ID、Revision、Digest、Execution、Route、Attempt、Frame、source sequence与`complete|partial|unavailable` fidelity，并确定性排序、ID/sequence唯一。

### `InjectionConformanceFact`

Context Owner Inspect Expected/ActualInjectionManifest后CAS，状态为`matched | allowed_residual | rejected | unknown`。强制字段漂移直接`rejected`；不可观察内容为`unknown`，不得推断matched。

Inspect必须接收对应Provider Observation或已独立验证的类型化refs，核验Execution/Route/Attempt/Frame/sequence/Revision/Digest；Expected过期、Observation缺失或fidelity不完整绝不能`matched`。

Context Conformance结论不授予Tool调用资格，也不能替代Tool Surface current Reader/Gate；Tool Surface Gate同样不得反向修改Context Expected/Actual或Conformance事实。

## 9. Prompt反馈与发布

### `ContextFeedbackCandidateFact`

绑定Base Recipe/Prompt精确版本、Frame/Outcome集合、用户纠正/任务证据、候选变更Digest、Evaluation Policy、风险、TTL。反馈不会自动更新生产资产。

### `ContextReleaseFact`

发布时绑定Candidate、Evaluation、Review（含明确not-required）、Authority、Operation Effect/Permit/Settlement、Expected Base Revision和新Recipe/Prompt版本。CAS冲突必须重评，禁止覆盖最新版本。

#### Owner-local pre-release lifecycle V1

- Context Owner先实现不可变Recipe版本注册与`draft → validated → evaluated → review_pending`，任一阶段可进入`rejected`；Lifecycle Fact绑定exact RecipeRef、前一LifecycleRef、Validation Report、Evaluation、Feedback Candidate、Review Case和Evidence，按状态严格presence。
- Lifecycle head使用expected exact FactRef CAS；同一Recipe同一head只允许一个后继，64并发单赢家。Fact自身均rev1且不可修改；SemanticVersion变化必须注册新Recipe Fact。
- V1 owner-local pre-release合同刻意不包含`published/superseded/revoked`写入口，也没有production Recipe current binding。Run外Review/Operation适用性仍由`CTX-D07`裁决；在获批公共合同与Reader前，publish/rollback/revoke必须返回`unsupported`，不得用普通FactRef、Run内V4或上游Settlement代替。

## 10. Context Owner-local Offline SDK V1

`ContextOfflineSDKV1`提供`ValidateRecipeV1 / CompareRecipesV1 / CompileFrameV1 / PreviewFrameV1 / InspectFrameExactV1 / InspectCachePlanV1`。详细请求/响应见[SDK、CLI与API边界](./sdk-api.md)。六个入口共享以下不变量：

- 输入输出只复用Context-owned V1合同；typed请求的`ContractVersion`、`Operation`、`RequestDigest`必须exact。只有显式`Decode*RequestV1` codec承诺递归strict JSON，typed Go入口不声称检测duplicate key；
- Compile只在单次调用内使用ephemeral content-addressed workspace，输出明确`Authoritative=false`的Manifest/Frame/content bundle；不写Owner Store、Domain Fact、Generation current或Runtime/Application/Harness状态；
- Preview只返回规范排序的Admission、区域、token、exact refs和digest诊断，V1不返回原始正文；
- Compare只返回两个exact Recipe refs、规范排序的field-path change与before/after digest；不产生质量、兼容、Review、发布或回滚结论；
- Compare的Checked必须同时位于两个Recipe有效期内，报告Expires不得超过任一Recipe expiry；Changes present、唯一排序且最多80项，完全相同返回`[]`；
- Inspect只证明Manifest/Frame/bundle内部exact且可复现，不证明任何Owner currentness，不替代CTX-D10、Evidence、Injection Conformance或Route materialization；
- Cache Inspect只接受调用者已经构造的provider-neutral `CachePlan + ProviderCacheProfile + CheckedUnixNano`，核验Plan/Profile exact digest、Partition/Key、最短有效期与离线经济性；`Current=true`只表示该离线闭包在checked time有效，不是Cache Entry current、Provider状态或cache hit，也不产生Plan、访问事实、写入或Effect；
- limits、完整DTO、typed error闭集、deep-copy/no-alias及cancel阶段检查点以[第六审SDK设计](./sdk-api.md)为准；任何error除Validate的确定性`Valid=false`诊断外都返回零Response；
- workspace必须在构造后、`Begin`前注册无ctx的`Destroy`，且`Destroy(new)`合法；Begin失败、取消和成功路径均以destroyed收口。Optional missing复用live `content_unavailable` Residual；required missing只在SDK边界映射`not_found`，共享helper/wrapper不得改写Owner语义；
- Offline ContentItem/Bundle只接受live `ContentRef.Validate`通过且`Length>0`的exact Ref及非空bytes，不修改全局ContentRef、不建立第二nominal。零字节只属于独立base64 primitive `encode/decode []↔[]` golden，不得构造ContentItem；required空内容没有合法非零Ref时Fail Closed；
- 同canonical请求与bundle的输出逐字节确定；并发调用不共享可变状态。任何SDK返回都不授予Capability、Review、Permit、Settlement、Cache hit、Run Requirement或Turn推进资格。
- Offline SDK的`MemorySources/KnowledgeSources/ContinuitySources`均为0，不调用Memory/Knowledge public contextsource Reader。未来跨Owner Source只能经对应Owner的唯一public Reader（必要时由Owner additive V2或唯一无损facade）提供，Context不建第二套nominal/DTO；`knowledge_reference`仍未获Context Owner单独候选确认。

既有四入口的第七资产短审与后续独立软件验收均已YES；additive `CompareRecipesV1`仅复用已实现的Owner-local确定性结构比较内核；additive `InspectCachePlanV1`仅复用现有Cache合同的currentness/digest/economics纯函数。二者均单独执行SDK/API/CLI/strict codec/cancel/并发验证。该结论只覆盖Owner-local Offline SDK，不构成production启用声明；不得借SDK增加质量评测、replay/refresh、Provider Cache操作或跨Owner Adapter。

## 11. Context Engineering SDK V1

用户已确认Prompt开发入口与可插拔Evaluator/Policy联合设计；Owner-local Go首切面及分层测试已完成。exact DTO、limits、Observation→Fact Admission、S1/S2与Unknown边界见[Context Engineering SDK V1](./engineering-sdk.md)。Evaluator输出只为Observation；Context Owner复核exact输入后才形成Evaluation/Feedback Fact。既有Offline SDK六operation不变，production发布/远程Judge继续NO-GO。模型专属预埋提示词只把官方开源coding agent作为有版本、有license、有变换摘要的上游候选，不把上游文本直接升级为Authority或published资产。

`ContextOfflineIngressV1`是SDK之后的独立只读Delta：公开request Seal/Encode、`offlineapi.ContextOfflineAPIV1`六typed方法/JSON dispatch及六条CLI命令。它不拥有Fact/Store/current pointer，不创建Effect/Settlement，不注册Capability，也不构成production API server/root。
