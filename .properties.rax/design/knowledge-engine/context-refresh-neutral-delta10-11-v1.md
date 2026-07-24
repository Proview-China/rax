# Knowledge Context Refresh Delta 10/11 Owner V2冻结与External接线候选

> 状态：**design_frozen / owner_local_and_reference_integration_software_test_yes**。live Knowledge V1/V2属于同一Reader合同族并共享唯一Knowledge Owner Store/Journal/current，不存在第二public facade、第二Pointer或第二current。Knowledge Adapter、Application三阶段协调、Context `knowledge_reference`/TransitionProof/原子Apply+Generation CAS与Knowledge=1的reference fixture已YES；production root与远程Retrieval/Resolver仍未启用。

## 1. Owner与非Owner

| 对象/动作 | Owner | Knowledge侧允许 | Knowledge侧禁止 |
|---|---|---|---|
| Retrieval Attempt、Observation/Result、Source/Package/Record/Snapshot/Pointer | Knowledge Owner | Inspect、currentness、Citation/License/Conflict exact refs | 把Provider命中或Trust标签当权威事实 |
| Owner-local正文 | Knowledge Owner | `ReadContentExact`并重算bytes、License与Source currentness | 读取Asset远程正文、Context缓存或Resolver结果 |
| Candidate/Fragment、pending Context DomainResult、Frame、Generation | Context Owner | 只提供既有Reader合同族的Owner exact refs和fragment语义候选 | 创建、Apply或发布任何Context Fact |
| per-turn顺序与attempt关联 | Application Owner | 接收协调，不解释Knowledge事实 | 写Knowledge/Context Fact或补造Citation/License |
| Run内Loop与模型输入 | Harness Owner | 只消费Context已冻结的exact Frame | 调用Knowledge Reader、Store、Resolver或Gateway |

Knowledge与Memory保持独立Owner、合同、端口实例、预算和SourceSet Digest。Context缓存、Citation文本、Compaction Summary和Frame都不是Knowledge真值。

## 2. 固定链与可见性

```text
Application只协调pre-frame transition request（SourceOrdinal=T、ExpectedTargetOrdinal=T+1）
 -> KnowledgeContextSourceCurrentReaderV1.InspectAttempt
 -> S1 InspectForTurn(Current=true)
 -> S1 ReadContentExact(local Knowledge State Plane only)
 -> Context Owner按Knowledge fragment kind候选materialize/admit/compile
 -> pending Context DomainResult + candidate Manifest/Frame/Generation
    （全部不可见、非current）
 -> Context Owner seal final TransitionProof（绑定new Frame/Generation exact refs）
 -> S2以相同stable ClosureDigest与exact集合重新InspectForTurn
 -> S2重新ReadContentExact并复算bytes/Source/License/TTL/currentness
 -> Context Owner单个本地原子边界：Context ApplySettlement + expected Generation current CAS
 -> Context Owner Inspect原Refresh Attempt
 -> exact Frame/Manifest/Generation/ExpectedInjectionManifest refs
 -> Harness只消费exact Frame
```

Knowledge不创建Context Candidate/Fragment Fact、pending DomainResult、ApplySettlement、Generation、Frame、ExpectedInjectionManifest或Continuation。

## 3. 复用现有public Reader合同族

唯一Owner public面保持`praxis.knowledge/context-source-current-reader/v1`合同族及其`AttemptCoordinate/AttemptInspection/CurrentRequest/CurrentProjection/ProjectionItem/ExactContentRequest/ExactContentObservation`。冻结合同采用同族`v2`演进，不创建`praxis.knowledge/context-source-neutral/v1`、Adapter DTO、Asset wrapper或Context侧副本；V1不得由Adapter补造V2字段。

### 3.1 `AttemptCoordinateV2`与`AttemptInspectionV2`

| 字段 | Owner V2冻结要求 |
|---|---|
| `ContractVersion` / `ObjectKind` / `Owner` | 同Reader合同族v2；Inspection继续使用Knowledge discriminator与Owner |
| `TenantID` / `IdentityRef{ID,Revision,Digest}+Epoch` / `ExecutionScopeDigest` | 逐项exact；不得只信caller聚合摘要 |
| `RunID` / `SessionRef{ID,Revision,Digest}` | Session exact coordinate只能来自具名Session Owner Reader；Knowledge只验证/回显，不得补造 |
| `SourceTurnOrdinal uint32` | `SourceTurnOrdinal == Tool.Execution.Turn == ExpectedCurrent.Turn`；这是live uint32可证明的唯一直接等式 |
| `SourceTurnRef contract.Ref` / `SourceTurnOrdinal uint32` | 非零来源时required；Harness committed PendingAction current及public Adapter必须证明`SourceTurnRef/Ordinal == Tool.Execution.Turn == ExpectedCurrent.Turn`；不得在Ref上伪造Ordinal或从uint32构造ID/revision/digest |
| `AttemptRef` / `ObservationRef` / `ResultRef` | 必须来自同一Knowledge Owner settled chain |
| `RequestDigest` / `IdempotencyKey` | 同身份换Query/View/Scope语义产生Conflict |
| `CheckedUpperBound` / `NotAfter` | caller只给因果上界，不能授current |
| Inspection echo | Inspection回传Owner原始Identity/Run/Session/SourceTurnOrdinal/SourceTurnRef、Attempt/Observation/Result、Status与TTL；不得补字段 |

当前V1只有RunID/TurnID，没有Identity exact ref/epoch、Session coordinate或Turn revision/digest。因此V2与正确Owner currentness接线闭合前，非零Knowledge来源继续NO-GO。Knowledge只绑定并防replay，不拥有Session/Turn current事实。

### 3.2 `CurrentRequestV2`与`CurrentProjectionV2`

| 字段组 | 必需字段 |
|---|---|
| Identity | ContractVersion、ObjectKind=`knowledge_contribution_current_projection`、Owner、Projection exact ref |
| Coordinate | 原AttemptCoordinate全部字段、Run/Session/SourceTurnOrdinal及具名Owner提供的SourceTurnRef、ExecutionScopeDigest；不含TargetTurn或Context transition proof |
| Retrieval | Observation/Result/Query/View/Published Snapshot/Pointer exact refs、Coverage、NextCursor、ResultDigest、EvidenceDigest；声明/canonical顺序固定为Coverage→NextCursor→ResultDigest→EvidenceDigest→Items |
| Governance | Authority/Policy exact refs、Purpose、Scopes、AllowedLicenses、SensitivityMax |
| Settlement | DomainResult exact ref、DomainResultAssociation exact digest、Knowledge SettlementApplication exact ref |
| Locality | CurrentState ref、StatePlaneBinding ref、AccessKind=`owner_state_plane_local_exact` |
| Phase | `CheckPhase=s1|s2`；只进入fresh Projection/Observation digest，不进入stable ClosureDigest |
| Budget | MaxItems/MaxBytes/MaxTokens/PerItemMaxBytes、Estimator exact ref/digest |
| Stable closure | ordered Items与所有exact source/content/association/current-state refs、Citation/Source/License/Conflict Set Digests、预算/Estimator语义；排除`AttemptInspectionRef`、CheckPhase、OwnerCheckedAt、ExpiresAt与Projection self ref |
| Fresh projection | Projection Ref/Digest、`AttemptInspectionRef`、CheckPhase、OwnerCheckedAt、ExpiresAt、stable ClosureDigest、`Current`；重新Inspect时S1/S2可产生不同fresh Projection Ref/Digest |

Knowledge八个set digest的domain/version/ObjectKind、canonical body、stable item投影和Ref/Content/String正规化算法已在[V2冻结合同](../../plan/memory-knowledge/context-source-reader-v2-frozen.md#44-set-digest精确算法)逐项冻结；不得只验非空或让item `ExpiresAt/Digest`进入`OrderedItemSetDigest`。

`Current=true`要求Published Snapshot/Pointer、Package/Record/Source/Projection/License/Conflict均由Knowledge Owner fresh复读；失败经现有typed result/AttemptStatus/`errors.Is`返回，不增加ClosedReason DTO。Projection不得包含组织Trust结论、Runtime Outcome、Review Verdict、Provider Receipt或Context Frame。

### 3.3 `ProjectionItemV2`与`ExactContentObservationV2`

| 字段组 | 必需字段 |
|---|---|
| Order | `Ordinal`由`Score desc -> Record ID asc -> Revision desc -> Digest asc`派生；caller Rank不可信 |
| Knowledge facts | Record、Package、Snapshot、Content、Source、Evidence、Projection exact refs |
| Citation/conflict | CitationDigest、License、TrustState、ConflictGroup；均是Knowledge Owner current事实/标记 |
| Settlement | DomainResult、Association canonical digest、SettlementApplication exact refs |
| Materialization | 现有ExactContentObservation ref、Observed Length/Media/Digest/License、StatePlaneBinding ref；fresh Observation Digest包含CheckPhase、ObservedAt、ExpiresAt、stable ClosureDigest与exact内容/License字段 |
| Budget | exact bytes、TokenEstimate、Estimator exact ref/digest；不借Memory预算 |
| Lifetime | Record/Source/Projection/License/Content/Owner current最小ExpiresAt |

duplicate semantic Record key、重复Ordinal/Source、map last-wins、无Citation来源或同ContentRef换bytes/License必须拒绝。

### 3.4 逐字段V1→V2映射

| live V1对象/字段 | V2演进 | 兼容与current要求 |
|---|---|---|
| `AttemptCoordinate.ContractVersion` | `...current-reader/v2` | V1/V2 strict decode互拒，不隐式升级 |
| `TenantID` | 原样保留 | 同一Knowledge Owner store分区 |
| 无Identity字段 | 新增Identity ID/Revision/Digest/Epoch | required；Adapter不得从ExecutionScope猜测 |
| `ExecutionScopeDigest` / `RunID` | 原样保留 | 逐项exact |
| `TurnID` | 保留；新增`SourceTurnOrdinal uint32`，SourceTurn exact ref仅接收具名Turn Owner Reader输出 | `SourceTurnOrdinal == Tool.Execution.Turn == ExpectedCurrent.Turn`；有exact ref时`TurnID == SourceTurnRef.ID`且ordinal一致；Knowledge/Application不得补ID/revision/digest |
| 无Session字段 | 新增retrieval-bound Session ID/Revision/Digest | 只接收具名Session Owner Reader输出并验证/回显；缺失即Fail Closed，Knowledge与Application均不得补造 |
| Attempt/Observation/Result refs、RequestDigest、IdempotencyKey | 原样保留 | 同ID换内容为Conflict |
| `AttemptInspection`现有字段 | 增加Identity/Session/Turn exact echo | 不新增Inspection wrapper |
| `CurrentRequest.ExpectedS1ClosureDigest` | 保留并新增`CheckPhase=s1|s2` | S1必须空；S2精确等于S1 stable ClosureDigest；phase不进入ClosureDigest |
| Snapshot/Pointer/Authority/Policy/Purpose/Scopes/AllowedLicenses/Sensitivity | 原样保留 | stable语义进入Closure；phase/time/expiry只进fresh Projection/Observation digest；Authority Epoch为V2 required |
| 无显式预算 | 新增MaxItems/MaxBytes/MaxTokens/PerItemMaxBytes、Estimator Digest | 全部正值且有Owner cap；不借Memory预算 |
| `CurrentProjection`现有refs/current/Closure/TTL | 原样保留并新增Identity/Session/SourceTurn/Phase、Source/Citation/License/Conflict Set Digest | S1/S2 fresh Projection refs可不同；stable ClosureDigest与exact ordered集合必须相同 |
| `ProjectionItem.DomainResultRef/ApplicationRef` | 保留并新增Association canonical digest、TokenEstimate/EstimatorDigest | Context不得补造Association/Citation/License |
| `ExactContentRequest`现有Projection/Run/Turn/Rank/时间 | 新增`context.Context`、Identity/Session/Turn exact、CheckPhase、`MaxBodyBytes` | `MaxBodyBytes>0`；cancel返回零body |
| `ExactContentObservation`现有binding/closure/content/license/observed/TTL | 新增Identity/Session/Turn exact与CheckPhase echo | `ObservedLength == len(body) <= MaxBodyBytes`，License与S2一致 |

### 3.5 Source/action turn与Context独占TransitionProof候选

待Context/Application/Tool Execution联合冻结的精确语义为：

```text
Source/ActionTurn = T
SourceTurnOrdinal = Tool.Execution.Turn = ExpectedCurrent.Turn
SourceTurnRef only from named Turn Owner Reader
legacy TurnID = SourceTurnRef.ID (when exact ref exists)

Target/ContextTurn = T+1
  belongs to Context childExecution + new Frame + new Generation
  and is bound only by the Context-owned final TransitionProof
```

Knowledge V2只验证/回显retrieval-bound `SourceTurnOrdinal`与Harness committed PendingAction current经Application无损传递的Session/Turn exact ref；不得从live uint32、legacy TurnID、Run、Context缓存或Application字段补造坐标。它不得携带或计算TargetTurn、执行`Ordinal+1`、拥有或seal任何TransitionProof。

唯一Owner是Context，Application只协调两个不同阶段的Context对象：

1. pre-frame transition request只携带SourceTurnOrdinal=T、ExpectedTargetOrdinal=T+1、ExecutionScope/Run、具名Owner给出的Session/Turn证据、ExpectedCurrent与Refresh Attempt；它不可能含尚未形成的Frame/Generation refs，也不是proof；
2. Context先seal pending DomainResult、Manifest、Frame、Generation等不可见输出，再seal final TransitionProof；final proof StableBody无损绑定pre-frame request、Source/Target ordinal、Context childExecution、pending DomainResult及new Frame/Generation exact refs与stable SourceSet digest；phase/time/expiry只进入fresh proof Projection，不进入StableBody；
3. 随后才执行Owner S2，最后由Context Owner atomic ApplySettlement+Generation CAS并publish。proof存在不代表current，Application不得seal、改写或把它当发布证明。

V2七struct逐字段Go type/JSON tag/字段顺序及StableClosure精确常量/canonical body见：[Context Source Current Reader V2冻结合同](../../plan/memory-knowledge/context-source-reader-v2-frozen.md)。Owner-local P1已经关闭；其他Owner设计项保持Review YES且不重开。本轮不代表Go已实现或跨Owner合同已YES。

## 4. Knowledge fragment kind候选

Context Owner live `FragmentKind`目前没有Knowledge专属值。本模块仅提交以下**加法设计候选**，不宣称该枚举或其canonical已经由Context Owner发布：

| 项 | 设计候选 |
|---|---|
| Requested kind | `knowledge_reference` |
| 语义 | 有Source/Package/Snapshot/Pointer、Citation、License、Conflict与exact content锚点的Knowledge Owner current材料 |
| 禁止替代 | 不得冒充`memory_recall`、`artifact_reference`、`instruction`或`state_observation` |
| Owner | Fragment Fact仍由Context Owner创建；Knowledge只提供现有Reader的Projection/Item/Content Observation exact refs |
| Trust | Knowledge不得请求`authoritative_instruction`；Context按Policy选择`observation`或`restricted_material` |
| Materialization | Context按Route能力与Recipe选择inline/reference；Knowledge不决定Region、Role、Position、Required或Token预算 |
| Fail Closed | kind未发布、Recipe无规则或Route不能exact物化时返回`unsupported_fragment_kind`/Residual，不降级换kind |

不新增`KnowledgeFragmentSourceRefV1`。Context Owner必须演进自己的Candidate/Fact exact source binding，直接seal Knowledge CurrentProjection Ref、Item Rank、ExactContentObservation Ref及Record/Package/Snapshot/Pointer/Source/Projection、Citation、License、Conflict、Content和TTL。live `ContextCandidate.SourceRef+SourceRevision`缺Digest，尚不足以完成该绑定，因此Knowledge来源继续为0。

## 5. 公开Reader面与canonical

公开只读面保持三个方法，不增加`Retrieve`、`ResolveRemote`、`Apply`、`Freeze`或`Continue`：

```text
V1 live: InspectAttempt(AttemptCoordinate) -> AttemptInspection
         InspectForTurn(CurrentRequest) -> CurrentProjection
         ReadContentExact(ExactContentRequest) -> ExactContentObservation + local body

V2冻结：InspectAttempt(context.Context, AttemptCoordinateV2) -> AttemptInspectionV2
         InspectForTurn(context.Context, CurrentRequestV2) -> CurrentProjectionV2
         ReadContentExact(context.Context, ExactContentRequestV2) -> ExactContentObservationV2 + bounded local body
```

`context.Context`只承载cancel/deadline，不承载Identity、Authority、Session、Turn、License或Trust事实。调用前/锁等待/Get期间取消或deadline crossing返回兼容context错误、零result/零body且不改变Owner state；不得转成NotFound、Unknown成功或新Attempt。

Canonical规则：

1. seal`domain + contract version + object kind + owner + canonical payload`，Digest字段置空后重算；
2. exact refs逐项比较ID、Revision、Digest；Package/Snapshot/Pointer/Source不能被普通Ref同形替换；
3. Source/Evidence/Projection refs按`ID -> Revision -> Digest`排序、逐项Validate并拒绝重复；Items按领域语义排序；
4. stable ClosureDigest包含Identity/Run/Session/SourceTurn、ExecutionScope、Association、locality、Citation/License/Conflict集合、Coverage、预算及ordered exact集合；明确排除`AttemptInspectionRef`、CheckPhase、OwnerCheckedAt、ExpiresAt和Projection self ref；
5. fresh Projection/Observation digest分别包含其ContractVersion/ObjectKind、phase、fresh time/expiry、stable ClosureDigest、fresh self/exact refs及Observation内容；S1/S2 fresh refs允许不同；
6. 未知必填字段、重复键、尾随文档、跨版本/type-pun和canonical失败均Fail Closed，untrusted输入不得panic。

## 6. TTL、S1/S2与currentness

- `InspectAttempt`、`InspectForTurn`先进入Knowledge Owner一致性锁域再读取fresh owner clock；等待锁跨TTL时拒绝。
- caller `CheckedUpperBound`不能批准current；Owner clock rollback拒绝。
- `ReadContentExact`在同一RLock内执行S1 fresh clock/current/binding → Get → S2 fresh clock/current/binding/closure；Get阻塞跨TTL、Pointer/Source/License/binding漂移或bytes变化均返回零body。
- V2先验证`ContentRef.Length <= MaxBodyBytes`，底层local reader必须接收同一`context.Context`与硬上限；超限、无法响应cancel或返回超长body均Fail Closed，不截断后伪装exact。
- `now >= ExpiresAt`即过期。Context候选Frame/Generation TTL取请求、Knowledge projection/items/content/Source/License、Recipe/Authority/Parent Generation及其他Owner source的最小值。
- S2允许产生新的Projection/Observation exact refs与fresh digest，但必须复用S1 Source coordinate、Attempt/Observation/Result语义、Published Snapshot/Pointer stable语义、stable ClosureDigest及完全相同的ordered exact Item集合；Withdraw、License缩小、Conflict/Citation或任一stable字段变化都使整个Refresh Attempt失败。
- S2成功不创建Context事实；只有Context Owner原子Apply+Generation CAS成功后exact Frame才可current。

## 7. Closed errors

不新增ClosedReason DTO，继续使用现有`AttemptStatus`和Go `errors.Is`闭集：`persisted_and_settled | persisted_unsettled | confirmed_not_persisted`；`ErrInvalidArgument`；`ErrEvidenceConflict/ErrRevisionConflict`；`ErrNotCurrent`；`ErrScopeDenied`；`ErrSensitivityDenied`；`ErrInspectionIncomplete`；`ErrNotFound`（仅ReadContentExact中的evicted）；`ErrContextUnmaterialized`；`ErrUnsupported`。License专用sentinel若经Review需要，只能在原`memory-knowledge/contract` error set加法。

错误优先级固定为contract/type → coordinate/exact ref → authority/scope/license/sensitivity → currentness/withdraw/poison/TTL → local content → fragment/capability unsupported。失败返回零result/零body，不得解析错误字符串、返回自由文本reason、成功空列表或Partial Coverage。

## 8. Delta 10/11边界与反例

- Delta 10：Context Owner-local Refresh/Apply/Inspect、单一Owner backend/lock、原子Apply+Generation CAS及其fixture已YES；`knowledge_reference`仍只是kind/exact-binding候选且Knowledge来源=0。
- Delta 11：尚缺Application public三阶段Port（pre-frame request、apply/advance、inspect）及Knowledge Adapter/nonzero source接线。Application只协调；Harness只消费Context Owner current exact Frame。
- Frame refresh lost reply只由Context Owner Inspect原Refresh Attempt；Knowledge Reader不能证明Frame已发布。
- 远程Query/Resolver/正文读取仍走独立Gateway且unsupported；不得藏进Reader、materialize或Harness。

NO-GO反例：

1. Knowledge把Provider命中直接包装成`knowledge_reference` Fragment Fact；
2. Context尚未增加kind时改用`memory_recall`或`artifact_reference`；
3. Application补造Citation、License或Session后写Context Fact；
4. Harness直连Knowledge Reader/Resolver或消费Citation bundle；
5. S1后发布Frame，再用S2复读Pointer/License；
6. S2 Knowledge失败但沿用旧Knowledge、新Memory拼Frame；
7. Apply成功、Generation CAS失败仍暴露candidate Frame；
8. Source withdrawn、License缩小或poisoned后继续使用旧Projection；
9. 本地evicted后读取Asset或远程Resolver；
10. production root在kind、Session/Turn映射或公共Port未装配时把Knowledge来源从0改为1。
11. Knowledge Reader携带或计算TargetTurn，或对SourceTurn执行`Ordinal+1`；
12. SourceTurn与settled Tool `Execution.Turn`或`ExpectedCurrent.Turn`任一不exact；
13. Application仅加一Ordinal并自造Target ID/Revision/Digest，或transition proof未seal childExecution/new Frame/new Generation；
14. Memory SourceTurn=T与Knowledge SourceTurn=T+1被拼入同一Context refresh。
15. 从`Tool.Execution.Turn uint32`生成SourceTurn ID/revision/digest，或legacy TurnID缺具名Turn Owner证据仍宣称exact；
16. Application seal final TransitionProof，或pre-frame request提前包含尚未seal的Frame/Generation refs；
17. ClosureDigest包含CheckPhase/OwnerCheckedAt/ExpiresAt/Projection self ref，导致S1/S2稳定集合相同却closure不同；
18. 强制S1/S2 Projection/Observation exact ref相同，或fresh ref不同便错误拒绝，而未比较stable ClosureDigest与ordered exact集合。
19. 重新Inspect仅改变Inspection的OwnerCheckedAt/ExpiresAt与exact ref，stable集合未变，却因把`AttemptInspectionRef`纳入closure而改变StableClosureDigest，或要求S2 request携带S1 Inspection ref。

## 9. 联合Owner问题

| 等级 | 问题 | 关闭Owner/标准 |
|---|---|---|
| P0-1 | live已有Harness `CommittedPendingActionReaderV3`与canonical Session/Turn applicability coordinate，Application已有中立Session/Turn nominal；剩余无损映射/传递并入P0-3 | 不新增Turn Store/Reader；任何Owner不得从uint32/legacy字符串补造；Knowledge来源仍为0 |
| P0-2 | Memory/Knowledge侧exact payload候选已资产化；Context Owner尚未接受/发布nominal、canonical、state/TTL或Reader | Context发布前不得把候选当proof；Application只协调 |
| P0-3 | Application public三阶段Port未发布 | Application只协调prepare/apply/inspect，不拥有proof或Context Fact |
| P0-4 | Knowledge Adapter、nonzero cardinality、G6B/root接线未实现 | 未YES前Knowledge Reader调用数和来源数均为0 |
| P0-5 | `knowledge_reference` exact binding候选已资产化；Context Owner尚未接受/发布 | 未YES前Knowledge注入Fail Closed，禁止换kind |
| Owner-local | 七个Knowledge V2 struct逐字段schema与StableClosure精确常量/body已冻结 | P0=0/P1=0/P2=0；本文未写Go |

Owner-local与cross-owner reference integration均P0=0/P1=0/P2=0。stable/fresh、AssociationDigest、ctx-aware bounded cancel与diagnostics已通过，不重开；production root与远程Gateway保持NO-GO。

P0-5 exact payload与live Context实现依据见[`knowledge_reference`联合设计](./knowledge-reference-external-p0-candidate.md)。

## 10. 迁移与唯一Owner/current

1. V2复用Knowledge Owner同一Journal、Attempt Store、CurrentState/Snapshot/Pointer Store、StatePlaneBinding和content store；不得复制第二仓、第二Pointer或第二current。
2. `V1 implemented`与`V2 design_frozen`不是双实现真值。V2获实现授权后先仅做shadow conformance，不能参与Context current或非零来源。
3. 切换必须由单一Capability/Binding generation选择V2；同一Owner/Tenant/ExecutionScope/Run/Session/Turn只能有一个Reader contract version获准返回可消费的current projection。
4. 切换后V1只允许历史exact Inspect/迁移验证，不得经Facade补字段后继续作为G6B current source。
5. V1记录若缺Identity/Session/Turn原始证据，不可无损升级；保持历史或重新取得新V2 Attempt，不得从Pointer/Context猜测。
6. 回滚必须经新Capability generation和当前Fence重新选择唯一版本，禁止V1/V2双current。
