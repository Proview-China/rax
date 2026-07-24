# Memory Context Refresh Delta 10/11 Owner V2冻结与External接线候选

> 状态：**design_frozen / owner_local_and_reference_integration_software_test_yes**。live Memory V1/V2属于同一Reader合同族并共享唯一Memory Owner Store/Journal/current，不存在第二public facade或第二current。Memory Adapter、Application三阶段协调、Context TransitionProof/原子Apply+Generation CAS与Memory=1的reference fixture已YES；production root与远程Retrieval仍未启用。

## 1. Owner与非Owner

| 对象/动作 | Owner | Memory侧允许 | Memory侧禁止 |
|---|---|---|---|
| Retrieval Attempt、Observation/Result、Record/View/Watermark | Memory Owner | Inspect、currentness、exact refs、closed reason | 让Context或Application解释Memory事实 |
| Owner-local正文 | Memory Owner | `ReadContentExact`并重算bytes | 读取Context缓存、Sandbox副本或远程Locator |
| pending Context DomainResult、Manifest、Frame、Generation | Context Owner | 只提供既有Reader合同族的Owner exact refs | 创建、Apply或发布任何Context Fact |
| per-turn顺序与attempt关联 | Application Owner | 接收协调，不改变Memory语义 | 写Memory/Context Fact或把DTO冒充Owner Fact |
| Run内Loop与模型输入 | Harness Owner | 只消费Context已冻结的exact Frame | 调用Memory Reader、Store或Retrieval Gateway |

Memory与Knowledge必须保持不同Owner、合同版本、端口实例、预算和SourceSet Digest；任何统一`MemoryKnowledgeContextSource`均不属于冻结Owner合同或External候选。

## 2. 固定链与可见性

```text
Application只协调pre-frame transition request（SourceOrdinal=T、ExpectedTargetOrdinal=T+1）
 -> MemoryContextSourceCurrentReaderV1.InspectAttempt
 -> S1 InspectForTurn(Current=true)
 -> S1 ReadContentExact(local Owner State Plane only)
 -> Context Owner materialize/admit/compile
 -> pending Context DomainResult + candidate Manifest/Frame/Generation
    （全部不可见、非current）
 -> Context Owner seal final TransitionProof（绑定new Frame/Generation exact refs）
 -> S2以相同stable ClosureDigest与exact集合重新InspectForTurn
 -> S2重新ReadContentExact并复算bytes/TTL/currentness
 -> Context Owner单个本地原子边界：Context ApplySettlement + expected Generation current CAS
 -> Context Owner Inspect原Refresh Attempt
 -> exact Frame/Manifest/Generation/ExpectedInjectionManifest refs
 -> Harness只消费exact Frame
```

Memory只形成Owner current projection和exact content observation。它不形成Context Candidate、Fragment、pending DomainResult、ApplySettlement、Generation、Frame、ExpectedInjectionManifest或Continuation。

## 3. 复用现有public Reader合同族

唯一Owner public面保持`praxis.memory/context-source-current-reader/v1`合同族及其`AttemptCoordinate/AttemptInspection/CurrentRequest/CurrentProjection/ProjectionItem/ExactContentRequest/ExactContentObservation`。冻结合同采用同族`v2`演进，不创建`praxis.memory/context-source-neutral/v1`、Adapter DTO或Context侧副本；V1不得通过Adapter补造V2字段。

### 3.1 `AttemptCoordinateV2`与`AttemptInspectionV2`

| 字段 | Owner V2冻结要求 |
|---|---|
| `ContractVersion` / `ObjectKind` / `Owner` | 同Reader合同族v2；Inspection继续使用Memory discriminator与Owner |
| `TenantID` / `IdentityRef{ID,Revision,Digest}+Epoch` / `ExecutionScopeDigest` | 逐项exact；不得只用聚合hash替代Tenant/Identity分区 |
| `RunID` / `SessionRef{ID,Revision,Digest}` | Session exact coordinate只能来自具名Session Owner Reader；Memory只验证/回显，不得补造 |
| `SourceTurnOrdinal uint32` | `SourceTurnOrdinal == Tool.Execution.Turn == ExpectedCurrent.Turn`；这是live uint32可证明的唯一直接等式 |
| `SourceTurnRef contract.Ref` / `SourceTurnOrdinal uint32` | 非零来源时required；Harness committed PendingAction current及public Adapter必须证明`SourceTurnRef/Ordinal == Tool.Execution.Turn == ExpectedCurrent.Turn`；不得在Ref上伪造Ordinal或从uint32构造ID/revision/digest |
| `AttemptRef` / `ObservationRef` / `ResultRef` | 必须来自同一Memory Owner settled chain |
| `RequestDigest` / `IdempotencyKey` | 同身份换请求或语义产生Conflict |
| `CheckedUpperBound` / `NotAfter` | caller只给因果上界，不能授current |
| Inspection echo | Inspection回传Owner原始Identity/Run/Session/SourceTurnOrdinal/SourceTurnRef、Attempt/Observation/Result、Status与TTL；不得补字段 |

当前V1只有IdentityID和RunID/TurnID，没有完整Identity exact ref/epoch、Session coordinate或Turn revision/digest。因此V2与正确Owner currentness接线闭合前，非零Memory来源继续NO-GO。Memory只绑定并防replay，不拥有Session/Turn current事实。

### 3.2 `CurrentRequestV2`与`CurrentProjectionV2`

| 字段组 | 必需字段 |
|---|---|
| Identity | ContractVersion、ObjectKind=`memory_contribution_current_projection`、Owner、Projection exact ref |
| Coordinate | 原AttemptCoordinate全部字段、Run/Session/SourceTurnOrdinal及具名Owner提供的SourceTurnRef、ExecutionScopeDigest；不含TargetTurn或Context transition proof |
| Retrieval | Observation/Result/Query/View/Watermark exact refs、Coverage、NextCursor、ResultDigest、EvidenceDigest；声明/canonical顺序固定为Coverage→NextCursor→ResultDigest→EvidenceDigest→Items |
| Governance | Authority exact ref+epoch、Policy ref、Purpose、Scopes、SensitivityMax |
| Settlement | DomainResult exact ref、DomainResultAssociation exact digest、Memory SettlementApplication exact ref |
| Locality | CurrentState ref、StatePlaneBinding ref、AccessKind=`owner_state_plane_local_exact` |
| Phase | `CheckPhase=s1|s2`；只进入fresh Projection/Observation digest，不进入stable ClosureDigest |
| Budget | MaxItems/MaxBytes/MaxTokens/PerItemMaxBytes、Estimator exact ref/digest |
| Stable closure | ordered Items与所有exact source/content/association/current-state refs、CitationSetDigest、SourceSetDigest、预算/Estimator语义；排除`AttemptInspectionRef`、CheckPhase、OwnerCheckedAt、ExpiresAt与Projection self ref |
| Fresh projection | Projection Ref/Digest、`AttemptInspectionRef`、CheckPhase、OwnerCheckedAt、ExpiresAt、stable ClosureDigest、`Current`；重新Inspect时S1/S2可产生不同fresh Projection Ref/Digest |

Memory六个set digest的domain/version/ObjectKind、canonical body、stable item投影和Ref/Content/String正规化算法已在[V2冻结合同](../../plan/memory-knowledge/context-source-reader-v2-frozen.md#44-set-digest精确算法)逐项冻结；不得只验非空或让item `ExpiresAt/Digest`进入`OrderedItemSetDigest`。

`Current=true`要求所有exact refs都能由Memory Owner current store复读；失败经现有typed result/AttemptStatus/`errors.Is`返回，不增加ClosedReason DTO。Projection不得包含Runtime Outcome、Review Verdict、Provider Receipt、Context Candidate或Frame。

### 3.3 `ProjectionItemV2`与`ExactContentObservationV2`

| 字段组 | 必需字段 |
|---|---|
| Order | `Ordinal`由`Score desc -> Record ID asc -> Revision desc -> Digest asc`派生；caller Rank不可信 |
| Memory facts | Record current-head、Content、Source、Evidence、Projection exact refs |
| Citation | CitationDigest；Citation不把Observation升级为Memory事实 |
| Settlement | DomainResult、Association canonical digest、SettlementApplication exact refs |
| Materialization | 现有ExactContentObservation ref、Observed Length/Media/Digest、StatePlaneBinding ref；fresh Observation Digest包含CheckPhase、ObservedAt、ExpiresAt、stable ClosureDigest与exact内容字段 |
| Budget | exact bytes、TokenEstimate、Estimator exact ref/digest；不借Knowledge预算 |
| Lifetime | Record/Projection/Content/Owner current最小ExpiresAt |

duplicate semantic Record key、重复Ordinal、map last-wins、caller自报Rank或同ContentRef换bytes都必须拒绝。

### 3.4 逐字段V1→V2映射

| live V1对象/字段 | V2演进 | 兼容与current要求 |
|---|---|---|
| `AttemptCoordinate.ContractVersion` | `...current-reader/v2` | V1/V2 strict decode互拒，不做隐式升级 |
| `TenantID` | 原样保留 | 同一Owner store分区，不复制数据 |
| `IdentityID` | 保留并新增Identity Revision/Digest/Epoch | `IdentityID == IdentityRef.ID`；Adapter不得补造 |
| `ExecutionScopeDigest` / `RunID` | 原样保留 | 逐项exact，不只比较聚合摘要 |
| `TurnID` | 保留；新增`SourceTurnOrdinal uint32`，SourceTurn exact ref仅接收具名Turn Owner Reader输出 | `SourceTurnOrdinal == Tool.Execution.Turn == ExpectedCurrent.Turn`；有exact ref时`TurnID == SourceTurnRef.ID`且ordinal一致；Memory/Application不得补ID/revision/digest |
| 无Session字段 | 新增retrieval-bound Session ID/Revision/Digest | 只接收具名Session Owner Reader输出并验证/回显；缺失即Fail Closed，Memory与Application均不得补造 |
| Attempt/Observation/Result refs、RequestDigest、IdempotencyKey | 原样保留 | 同ID换revision/digest/request为Conflict |
| `AttemptInspection`现有字段 | 增加Identity/Session/Turn exact echo | 不新增Inspection wrapper |
| `CurrentRequest.ExpectedS1ClosureDigest` | 保留并新增`CheckPhase=s1|s2` | S1必须空；S2精确等于S1 stable ClosureDigest；phase不进入ClosureDigest |
| CurrentState/Authority/Policy/Purpose/Scopes/Sensitivity | 原样保留 | stable语义进入Closure；phase/time/expiry只进fresh Projection/Observation digest |
| 无显式预算 | 新增MaxItems/MaxBytes/MaxTokens/PerItemMaxBytes、Estimator Digest | 全部正值且有Owner cap；不能借Knowledge预算 |
| `CurrentProjection`现有refs/current/Closure/TTL | 原样保留并新增Identity/Session/SourceTurn/Phase、SourceSet/CitationSet Digest | S1/S2 fresh Projection refs可不同；stable ClosureDigest与exact ordered集合必须相同 |
| `ProjectionItem.DomainResultRef/ApplicationRef` | 保留并新增Association canonical digest、TokenEstimate/EstimatorDigest | Context不得自行重建Association |
| `ExactContentRequest`现有Projection/Run/Turn/Rank/时间 | 新增`context.Context`、Identity/Session/Turn exact、CheckPhase、`MaxBodyBytes` | `MaxBodyBytes>0`且不超过Owner/Context预算；cancel返回零body |
| `ExactContentObservation`现有binding/closure/content/observed/TTL | 新增Identity/Session/Turn exact与CheckPhase echo | `ObservedLength == len(body) <= MaxBodyBytes` |

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

Memory V2只验证/回显retrieval-bound `SourceTurnOrdinal`与Harness committed PendingAction current经Application无损传递的Session/Turn exact ref；不得从live uint32、legacy TurnID、Run、Context缓存或Application字段补造坐标。它不得携带或计算TargetTurn、执行`Ordinal+1`、拥有或seal任何TransitionProof。

唯一Owner是Context，Application只协调两个不同阶段的Context对象：

1. pre-frame transition request只携带SourceTurnOrdinal=T、ExpectedTargetOrdinal=T+1、ExecutionScope/Run、具名Owner给出的Session/Turn证据、ExpectedCurrent与Refresh Attempt；它不可能含尚未形成的Frame/Generation refs，也不是proof；
2. Context先seal pending DomainResult、Manifest、Frame、Generation等不可见输出，再seal final TransitionProof；final proof StableBody无损绑定pre-frame request、Source/Target ordinal、Context childExecution、pending DomainResult及new Frame/Generation exact refs与stable SourceSet digest；phase/time/expiry只进入fresh proof Projection，不进入StableBody；
3. 随后才执行Owner S2，最后由Context Owner atomic ApplySettlement+Generation CAS并publish。proof存在不代表current，Application不得seal、改写或把它当发布证明。

V2七struct逐字段Go type/JSON tag/字段顺序及StableClosure精确常量/canonical body见：[Context Source Current Reader V2冻结合同](../../plan/memory-knowledge/context-source-reader-v2-frozen.md)。Owner-local P1已经关闭；其他Owner设计项保持Review YES且不重开。本轮不代表Go已实现或跨Owner合同已YES。

## 4. 公开Reader面与canonical

公开只读面保持三个方法，不增加`Retrieve`、`Resolve`、`Apply`、`Freeze`或`Continue`：

```text
V1 live: InspectAttempt(AttemptCoordinate) -> AttemptInspection
         InspectForTurn(CurrentRequest) -> CurrentProjection
         ReadContentExact(ExactContentRequest) -> ExactContentObservation + local body

V2冻结：InspectAttempt(context.Context, AttemptCoordinateV2) -> AttemptInspectionV2
         InspectForTurn(context.Context, CurrentRequestV2) -> CurrentProjectionV2
         ReadContentExact(context.Context, ExactContentRequestV2) -> ExactContentObservationV2 + bounded local body
```

`context.Context`只承载cancel/deadline，不承载Identity、Authority、Session、Turn或其他业务事实。调用前已取消、等待锁时取消、Get期间取消或deadline crossing都返回`context.Canceled`/`context.DeadlineExceeded`兼容错误、零result/零body且不改变Owner state；不得把cancel转成NotFound、Unknown成功或新Attempt。

Canonical规则：

1. 每个对象seal`domain + contract version + object kind + owner + canonical payload`，Digest字段置空后重算；
2. exact refs一律比较ID、Revision、Digest，不能只比ID；
3. Source/Evidence/Projection refs按`ID -> Revision -> Digest`排序、逐项Validate并拒绝重复；Items按领域语义排序，不能再按caller Rank重排；
4. stable ClosureDigest包含Identity/Run/Session/SourceTurn、ExecutionScope、Association、locality proof、Coverage、Citation、预算及ordered exact集合；明确排除`AttemptInspectionRef`、CheckPhase、OwnerCheckedAt、ExpiresAt和Projection self ref；
5. fresh Projection/Observation digest分别包含其ContractVersion/ObjectKind、phase、fresh time/expiry、stable ClosureDigest、fresh self/exact refs及Observation内容；S1/S2 fresh refs允许不同；
6. 未知必填字段、重复键、尾随文档、跨版本/type-pun和canonical重算失败均Fail Closed，untrusted输入路径不得panic。

## 5. TTL、S1/S2与currentness

- `InspectAttempt`、`InspectForTurn`必须先进入Memory Owner一致性锁域，再读取fresh owner clock；等待锁期间跨TTL时拒绝。
- caller `CheckedUpperBound`只限制请求因果与NotAfter，不能批准current；Owner clock rollback或`ownerNow < lastOwnerNow`拒绝。
- `ReadContentExact`在同一Owner RLock内执行S1 fresh clock/current/binding → Get → S2 fresh clock/current/binding/closure；Get阻塞跨TTL、binding revision/RemoteLocator漂移或bytes变化均返回零body。
- V2先验证`ContentRef.Length <= MaxBodyBytes`，底层local reader必须接收同一`context.Context`与硬上限；超过上限、无法响应cancel或返回超长body均Fail Closed，不截断后伪装exact。
- `now >= ExpiresAt`即过期。Context候选Frame/Generation TTL必须是请求上界、Memory projection/items/content、Context Recipe/Authority/Parent Generation及其他Owner source TTL的最小值，不能用cap延长任何来源。
- S2允许产生新的Projection/Observation exact refs与fresh digest，但必须复用S1 Source coordinate、Attempt/Observation/Result语义、stable ClosureDigest及完全相同的ordered exact Item集合；任一stable字段或集合变化使整个Refresh Attempt失败，不在原attempt补选低位项。
- S2成功只证明Memory source仍current；只有Context Owner原子Apply+Generation CAS成功后，exact Frame才可current。

## 6. Closed errors

不新增ClosedReason DTO，继续使用现有`AttemptStatus`和Go `errors.Is`闭集：`persisted_and_settled | persisted_unsettled | confirmed_not_persisted`；`ErrInvalidArgument`；`ErrEvidenceConflict/ErrRevisionConflict`；`ErrNotCurrent`；`ErrScopeDenied`；`ErrSensitivityDenied`；`ErrInspectionIncomplete`；`ErrNotFound`（仅ReadContentExact中的evicted）；`ErrContextUnmaterialized`；`ErrUnsupported`。

错误优先级固定为contract/type → coordinate/exact ref → authority/scope/sensitivity → currentness/poison/TTL → local content → unsupported。失败返回零result/零body，不得解析错误字符串、返回自由文本reason、成功空列表或Partial Coverage。若需更细错误，只能在现有`memory-knowledge/contract` error set版本化加法。

## 7. Delta 10/11边界与反例

- Delta 10：Context Owner-local Refresh/Apply/Inspect、单一Owner backend/lock、原子Apply+Generation CAS及其fixture已YES；Memory只提供未来同Reader合同族V2 source，不返回Context Fact或第二套Source DTO。
- Delta 11：尚缺Application public三阶段Port（pre-frame request、apply/advance、inspect）及Memory Adapter/nonzero source接线。Application只协调；Harness只接收Context Owner current exact Frame。
- Frame refresh lost reply由Context Owner `InspectContextTurnRefresh`检查原Attempt；Memory Reader只检查Memory Attempt，二者不能互代。
- Remote Retrieval仍走独立Gateway Delta且当前unsupported；不得把远程Query、Resolver或正文读取藏入S1/S2。

NO-GO反例：

1. Application把Memory projection复制成Context Fact，或Memory/Application补写Session；
2. Memory返回`memory_recall` Candidate并声称已注入；
3. Harness直接调用Memory Reader或消费Content body；
4. S1后发布candidate Frame，再以S2作事后校验；
5. S2 Memory失败但沿用旧Memory、新Knowledge拼Frame；
6. Context ApplySettlement成功、Generation CAS失败却暴露Frame；
7. lost reply分配新Refresh Attempt或重跑Owner Reader生成不同集合；
8. 本地evicted后自动调用远程Gateway；
9. caller旧时间、缓存Projection或Context Summary授予current；
10. production root在未装配public Port/Session映射时把Memory来源数从0改为1。
11. Memory Reader携带或计算TargetTurn，或对SourceTurn执行`Ordinal+1`；
12. SourceTurn与settled Tool `Execution.Turn`或`ExpectedCurrent.Turn`任一不exact；
13. Application仅加一Ordinal并自造Target ID/Revision/Digest，或transition proof未seal childExecution/new Frame/new Generation；
14. Memory SourceTurn=T与Knowledge SourceTurn=T+1被拼入同一Context refresh。
15. 从`Tool.Execution.Turn uint32`生成SourceTurn ID/revision/digest，或legacy TurnID缺具名Turn Owner证据仍宣称exact；
16. Application seal final TransitionProof，或pre-frame request提前包含尚未seal的Frame/Generation refs；
17. ClosureDigest包含CheckPhase/OwnerCheckedAt/ExpiresAt/Projection self ref，导致S1/S2稳定集合相同却closure不同；
18. 强制S1/S2 Projection/Observation exact ref相同，或fresh ref不同便错误拒绝，而未比较stable ClosureDigest与ordered exact集合。
19. 重新Inspect仅改变Inspection的OwnerCheckedAt/ExpiresAt与exact ref，stable集合未变，却因把`AttemptInspectionRef`纳入closure而改变StableClosureDigest，或要求S2 request携带S1 Inspection ref。

## 8. 联合Owner问题

| 等级 | 问题 | 关闭Owner/标准 |
|---|---|---|
| P0-1 | live已有Harness `CommittedPendingActionReaderV3`与canonical Session/Turn applicability coordinate，Application已有中立Session/Turn nominal；剩余无损映射/传递并入P0-3 | 不新增Turn Store/Reader；任何Owner不得从uint32/legacy字符串补造；Memory来源仍为0 |
| P0-2 | Memory/Knowledge侧exact payload候选已资产化；Context Owner尚未接受/发布nominal、canonical、state/TTL或Reader | Context发布前不得把候选当proof；Application只协调 |
| P0-3 | Application public三阶段Port未发布 | Application只协调prepare/apply/inspect，不拥有proof或Context Fact |
| P0-4 | Memory Adapter、nonzero cardinality、G6B/root接线未实现 | 未YES前Memory Reader调用数和来源数均为0 |
| P0-5 | `knowledge_reference` exact binding候选已资产化但未获Context Owner接受/发布 | 整个联合refresh仍保持Knowledge=0，不得以Memory kind替代 |
| Owner-local | 七个Memory V2 struct逐字段schema与StableClosure精确常量/body已冻结 | P0=0/P1=0/P2=0；本文未写Go |

Owner-local与cross-owner reference integration均P0=0/P1=0/P2=0。stable/fresh、AssociationDigest、ctx-aware bounded cancel与diagnostics已通过，不重开；production root与远程Gateway保持NO-GO。

P0-2 exact payload与live Context实现依据见[Context TransitionProof联合设计](./context-transition-proof-external-p0-candidate.md)。

## 9. 迁移与唯一Owner/current

1. V2复用Memory Owner同一Journal、Attempt Store、CurrentState Store、StatePlaneBinding和content store；不得复制第二仓或第二current pointer。
2. `V1 implemented`与`V2 design_frozen`不是双实现真值。V2获实现授权后先仅做shadow conformance，不能参与Context current或非零来源。
3. 切换必须由单一Capability/Binding generation选择V2；同一Owner/Tenant/ExecutionScope/Run/Session/Turn只能有一个Reader contract version获准返回可消费的current projection。
4. 切换后V1只允许历史exact Inspect/迁移验证，不得通过Facade补字段后继续作为G6B current source。
5. V1→V2迁移必须验证相同Attempt/Observation/Result/CurrentState在同一Owner store上的canonical对应；缺Identity/Session/Turn原始证据的V1记录不可无损升级，只能保持历史或重新取得新V2 Attempt。
6. 回滚不得恢复双current；必须通过新Capability generation和当前Fence重新选择唯一版本。
