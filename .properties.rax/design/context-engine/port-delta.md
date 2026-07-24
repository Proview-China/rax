# Context Engine结构化Port Delta

状态：current-truth联合评审输入；Context只修改自身Adapter/资产，不直接修改Runtime、Harness、Application、Model Invoker或其他组件。

## Delta总览

业务终点逐项current truth见[Context&Cache覆盖矩阵](../../plan/context-engine/coverage-matrix.md)。Application Context Refresh公共Port及B-cross fixture已闭合；Harness exact new Frame continuation binding、Model Invoker exact ContextReference current projection和Continuity public typed Owner-current Reader仍未闭，下列剩余Delta不得由Context私有兼容接口替代。

| ID | Owner | 结论 |
|---|---|---|
| CTX-D01 | Context Engine | 必需：Context Domain/Frame Port v1 |
| CTX-D02 | Context Engine + Source Owner | 必需：Versioned Context Source Candidate Port |
| CTX-D03 | Harness接线/Agent Assembler | 硬阻塞：namespaced Slot/Phase贡献与CompiledGraph映射 |
| CTX-D04 | Model Invoker/Profile | 必需：ProviderCacheProfile与Frame/Actual Injection公共投影 |
| CTX-D05 | Artifact/Sandbox/Tool Owner | 必需：Artifact Snapshot/Delta Resolver |
| CTX-D06 | Continuity | P1：Frame/Outcome/Generation事件与结构共享入口 |
| CTX-D07 | Runtime Review Owner | 管理线裁决：非Run Recipe/Prompt release Review |
| CTX-D08 | Runtime Evidence Owner | 不提出：V1不产生pre-run权威Evidence |
| CTX-D09 | Application协调合同 + Context Owner | RESOLVED for A/B-cross：三段公共Port、Context Adapter与Memory/Knowledge Owner Source fixture已完成；production root/Capability/Harness Continuation仍NO-GO |
| CTX-D09-R1 | Runtime Owner | RESOLVED：Context本地状态迁移不创建/消费Runtime Settlement；Context Owner原子ApplySettlement+expected Generation current CAS |
| CTX-D10 | Context Engine | G6A只读前置：ParentFrame Applicability Current Reader V1；live实现与隔离验证已完成 |

## CTX-D01 Context Domain/Frame Port v1

- 用例：Application在已启动Run的每Turn边界reserve、prepare、inspect、materialize；Context Owner自行完成pending DomainResult、S2与最终原子ApplySettlement+Generation current CAS；
- 语义Owner：Context Engine；Application只编排；
- 输入：RefreshAttemptID/IdempotencyKey/current time、ExecutionScope/RunID/Turn/Authority、SessionRef/ExpectedSessionRevision、Harness Generation/Binding/Activation current refs、Recipe/ParentFrame/ContextGeneration refs、Trigger Settlement、exact Tool/Memory/Knowledge/Continuity/Candidate refs、Artifact Anchor/Current Observation/Delta refs、Model/Harness/ToolSchema/ProviderCacheProfile/Pricing refs；
- 输出：RefreshAttemptRef、AdmissionDecisionRefs、ManifestRef、候选Frame/Generation refs、CachePlan/EconomicDecision、ExpectedInjectionManifestRef、ResidualRefs、pending Context DomainResultFact及最终Context ApplySettlement/current Generation；不预设Runtime Settlement Ref；
- 不变量：reserve-once；exact-idempotent；Frame冻结；同ID换内容冲突；不写Runtime Outcome；
- Effect/Recovery：纯本地编译无Effect；未来真实远程/披露/Provider写入才进入Runtime Operation Effect V3的Admission/Permit/Begin/actual-point Enforcement/Inspect治理。本地Context状态迁移不得套用Settlement V4；S2后原子Apply/CAS丢回包只Inspect exact Attempt；
- 反例：直接返回messages数组、隐式网络、Frame创建前不reserve、Application自写Frame、Action/Memory raw内容无exact ref、过期Binding仍Refresh；
- 兼容影响：legacy Runtime ContextPort保持静态兼容，不升级为此Port；新能力用SemVer/SchemaRef和Application namespaced Step接入。

## CTX-D02 Versioned Context Source Candidate Port

- 用例：Memory/Knowledge/Tool/MCP/Sandbox/Continuity/Profile/Human等只提交Candidate+Evidence；
- Owner：Candidate正文/来源currentness归Source Owner；Admission归Context Engine；
- 输入：SourceRef/Revision/Digest、Scope/Authority、Sensitivity、Freshness、materialization能力、Evidence；
- 输出：immutable ContextCandidate或Inspect结果；
- 不变量：Source不能指定Role/顺序/Cache placement/最终Admission；不可信材料不能提升权限；
- Effect/Recovery：提交已存在Ref无Effect；远程Materialize必须Operation Effect；未知只Inspect；
- 反例：Source直接append message、Memory命中自动注入、Tool Result携带system权限；
- 兼容影响：各组件新增Adapter，不互相导入实现包；缺Adapter的Required Source使Frame失败，Optional留下Residual。

## CTX-D03 Slot/Phase与CompiledGraph装配

- 用例：把Context的Observer/Filter/Gate/Port贡献放进统一Agent Assembly与Harness接线；
- Owner：Harness Assembly；live Catalog/Compiler/Generation-Binding已闭合，Context只声明贡献；
- 输入：版本化Contribution对象、依赖DAG、冲突策略、Context能力Manifest、Run Requirement；
- 输出：CompiledGraph、Binding V2 requirements、Application Step DAG、Harness materialization binding；
- 不变量：只允许Observer/Filter/Gate/Port；不接受任意修改Context/联网/写Fact的Hookface；
- Effect/Recovery：装配是确定性编译，不得执行Effect；
- 反例：Context私建slot enum、通过字符串hook绕过Graph、未定义Gate合并语义；
- 兼容影响：必须引用live `context.frame` Slot及既定Context/Turn phases；公共Refresh和materialization仍不得用私有兼容接口代替。

## CTX-D04 ProviderCacheProfile与Frame/Actual Injection投影

- 用例：Context按精确RouteID规划Provider-neutral Cache；Model Invoker通过`routegateway`/公开execution union消费Frame并返回Actual Injection/cache usage Observation；
- Owner：Route/Provider能力、ProviderActualInjectionObservation与usage Observation归Model Invoker；ActualInjectionManifest归Harness Model Turn Adapter；CachePlan/Cache Entry/InjectionConformance领域事实归Context/Cache；
- 输入：RouteID、Model/Profile/Harness refs、FrameRef+Digest、Expected Manifest、ProviderCacheProfileRef；
- 输出：版本化ProviderCacheProfile、materialization capability、ProviderActualInjectionObservation、Harness聚合的ActualInjectionManifest、cache read/write usage Observation和fidelity；
- 不变量：禁止依赖model-invoker/internal、厂商SDK、Raw/Native事件；Model Tool Call只形成ToolCall/FunctionCall Candidate Observation，Harness随后创建PendingAction，Tool Engine再创建领域ActionCandidate；cache usage不是hit事实；
- 链分离：Context Expected/Actual Injection Conformance只核验Frame/field注入；Tool Surface Gate只消费Tool Owner `ToolSurfaceManifest.ExpectedInjectionDigest`。Context不为Surface Gate新增Expected Manifest Reader或digest等式；
- Effect/Recovery：Model调用按已有Operation链；Provider cache独立操作按登记Effect；Begin后未知只Inspect原attempt；
- 反例：从cached_tokens推断Cache Entry、所有Route都假定能物化ContextReference、直接塞provider cache_control进核心、把Context ExpectedInjectionManifest Digest当Tool Surface expected digest；
- 兼容影响：不能物化Reference的Route Fail Closed，或经Plan接受Residual并降级Conformance。

## CTX-D05 Artifact Snapshot/Delta Resolver

- 用例：按版本、范围、符号、查询和Anchor返回unchanged/diff/full；
- Owner：Artifact真实版本和读取Receipt归Artifact/Sandbox/Tool Owner；Anchor投影归Context；
- 输入：ArtifactRef、KnownVersion/Digest、Range/Query、AnchorRef、Scope/Authority、Resolver capability；
- 输出：Snapshot/Delta/Unchanged Observation、current version/digest、Evidence、Residual；
- 不变量：裸路径不是内容；diff必须绑定base/target；Tool声称写入成功不等于磁盘事实；
- Effect/Recovery：本地受权读取可非Effect；远程/披露/计费读取走Operation V3；未知只Inspect；
- 反例：模型“记得”即视为current、diff基线不匹配仍应用、Context直接读取不属于Scope的路径；
- 兼容影响：无Resolver时ArtifactReference保持未展开；Required Inline物化失败则Frame拒绝。

## CTX-D06 Continuity Frame/Outcome/Generation入口

- 用例：记录`ContextFramePrepared`、`ModelResponseObserved`、`ContextOutcomeRecorded`、`ContextGenerationCompacted`及结构共享refs；
- Owner：Frame/Outcome语义归Context；事件顺序/Session/Checkpoint/Fork/Rewind关联归Continuity；
- live已闭部分：Continuity `TimelineOwnerFactRefV1`已能携带Owner、Fact Kind/ID/Revision/Digest、Payload Schema/Digest/Revision与Scope Digest；Checkpoint V2已能携带exact Context Generation/Frame refs。引用形状与结构共享恢复坐标不再是本Delta阻塞。
- 唯一未闭Port Delta：Continuity当前要求的`TimelineTypedOwnerCurrentReaderV1`、`TimelineOwnerCurrentProjectionV1`与Router仍定义在`continuity/runtimeadapter`，不是可供Context实现的公共Port。由Continuity Owner将同一语义提升到`continuity/ports`，或发布唯一无损public facade；禁止Context复制第二套nominal、导入Continuity runtimeadapter或让composition root以未验证快照冒充Owner current。
- 候选公共Reader：`InspectTimelineOwnerCurrentV1(ctx, TimelineOwnerFactRefV1) -> {Fact, CheckedUnixNano, ExpiresUnixNano, Digest}`与`ValidateTimelineOwnerCurrentV1(ctx, projection) error`。Reader必须按Context exact ref回到同一Owner backend复读Frame/Generation/Outcome；S1/S2、TTL crossing、current pointer或scope漂移均Fail Closed。具体公共包名和版本由Continuity Owner裁决，Context只实现获批接口的Adapter。
- 输入：Frame/Outcome/Generation exact Owner refs、Parent、Run/Turn、causation/correlation与Runtime Evidence coordinates；输出由Continuity拥有的Event/Timeline ref与恢复凭证；
- 不变量：Timeline Observation不升级Context Fact；Context Reader只证明Context事实仍exact/current，不分配Timeline sequence、不写Event、不创建Evidence；Rewind产生新Instance/Generation；
- Effect/Recovery：Context侧Reader严格只读。Continuity publish/append回包未知只按原Attempt/exact source coordinates Inspect；远程后端治理与持久化归Continuity；
- 反例：公共Reader缺失时直接注册Context；把Context `FactRef` type-pun为`TimelineOwnerFactRefV1`；adapter缓存Frame/Outcome/Generation快照；S2后Owner current漂移仍发布Event；Context直接绑定SQLite/RocksDB；Rewind宣称撤销外部Effect；丢失事件后重排；
- 兼容影响：这是additive公共Reader/Router暴露，不改变现有Timeline/Checkpoint schema。公共Port冻结、Context Adapter conformance与production composition root验收前，只允许Context Owner exact refs被离线检查，不启用真实Timeline投影。

## CTX-D07 非Run Recipe/Prompt release Review

- 用例：SDK/CLI在Run外发布、回滚或撤销生产Recipe/Prompt Current Binding；
- Owner：Context拥有Release Fact；Review Owner拥有Verdict；Runtime拥有Operation治理；
- 输入：Admin/Custom OperationSubject、Release Candidate、Authority、Review Policy、Budget/Scope；
- 输出：Review Candidate/Verdict binding、Operation Settlement、Context Release CAS；
- 不变量：禁止合成假RunID；policy_not_required也必须显式；
- Effect/Recovery：`context/recipe-release`类Effect；Begin后未知只Inspect；CAS冲突重新评测；
- 反例：Review V2强塞synthetic Run、CLI直接改Current、模型自评自动发布；
- 当前Context Owner已完成不可变Recipe与pre-release lifecycle-head CAS；该head不是production current。`publish/rollback/revoke`代码明确unsupported，未消费普通Review FactRef、Run内V4或上游Settlement。CTX-D07仍是唯一发布阻塞。
- 兼容影响：当前Review V2 Candidate要求RunID，而Operation V3支持admin/custom subject；需Runtime/Review管理线裁决新版本或限制首期发布能力。

## CTX-D08 pre-run Evidence

- 时点声明：V1权威Frame Prepare只发生在Run running之后；
- 结论：不需要pre-run Authoritative Evidence，不提出OperationScope-aware Evidence Delta；
- 约束：离线validate/compare/compile/preview/frame-inspect/cache-inspect结果只作非权威开发诊断，不得作为Binding、Trust或Run Admission事实；Cache Inspect的`Current=true`与usage/economics输出均不是Cache Entry current或cache hit；
- 后续：若未来需要Run前Recipe认证，重新提交独立Delta，不在本设计暗建。

## CTX-D09 G6A exact输入与G6B隔离实现

- 已闭合同：`ContextTurnRefreshPortV1`固定为三段`PrepareContextTurnRefreshV1 / ApplyContextTurnRefreshV1 / InspectContextTurnRefreshV1`，并发布Application-owned只读`ContextOwnerSourceReaderV1`。Apply输入不含Runtime settlement/ref；不新增公共Slot/Hook/Phase或Gateway。
- Owner：Context实现Owner-local contract/kernel/store与Application Adapter并拥有RefreshAttempt、Admission、Manifest、Frame、Generation、pending DomainResult、ApplySettlement与expected-CAS；Application只编排，Memory/Knowledge只通过各自V2 Adapter返回Owner exact projection。Tool/V4 Inspection/Association各由原Owner持有；Runtime本地迁移零Settlement；Harness只拥有Continuation/Turn。
- 依赖方向：Context Adapter只依赖Application公共`contract/ports`；Application、Harness不import Context实现。B-cross fixture已手工注入Application及Memory/Knowledge公共Ports；真实跨模块接线仍只归production composition root。
- N=1输入：settled `ToolResultV2` + current V4 Inspection + verified Association，且`Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`。Tool exact refs绑定同一Execution/Action/Attempt/Result；Memory/Knowledge V2 projection绑定同一Run/Session/SourceTurn T、Owner closure、current pointer、stable association、exact content observation及TTL。
- Memory/Knowledge只消费对应Owner唯一public V2 Reader/DTO，经Application中立Port映射；Context不建第二套nominal/DTO。`memory_recall`/`knowledge_reference`均为DynamicTail受限材料，每Owner正文聚合上限64KiB，不取得Authority。
- Turn mapping：具名Session/Turn Owner Reader返回Source Turn exact ref，其`Ordinal=T`必须exact等于settled Tool/ExpectedCurrent `uint32` Turn；Target T+1只由Context childExecution生成。TransitionProof唯一Owner为Context，Application只编排不mint，Memory/Knowledge不`+1`、不补造Session/Turn、不产生proof。
- 三段输出：`Refresh`先核验settled Tool exact chain，经S1产生确定性Attempt、候选Frame/Generation、Admission/Manifest/ExpectedInjection/Residual及pending Context DomainResult；`Apply`先执行S2 fresh owner-current复读，再原子提交Context ApplySettlement+expected Generation current CAS，才返回`applied_current`；`Inspect`只读恢复原Attempt。S2失败最多保留pending/diagnostic fact，current pointer不可见。
- Frame/Cache：父Frame不可变；StablePrefix与SemiStable逐项复用exact `ContentRef{Ref,Digest,Length}`，仅DynamicTail追加本次结果。`PrefixDigest` seal完整StablePrefix，稳定cache identity使用`StableSourceSetDigest`，不得以完整Manifest SourceSetDigest替代；key seal隔离/Authority/Sensitivity/Recipe/Render/Model/Harness/ToolSchema/ProviderProfile/KeyVersion。
- TTL：Frame/Generation/Cache Expires取请求NotAfter及G6A链、Session/Turn、ParentFrame/Manifest/Generation、Recipe、Authority、Assembly Binding/Activation、Source/Artifact、Cache/Profile所有可读current上界的严格最小值；`checked >= expires`拒绝，不得延长或宣称SLA。
- 确定性与恢复：Attempt/Frame/Generation ID先seal完整canonical request和expiry，不包含尚未产生的proof；Context proof随后seal pending exact refs/Attempt/stable closure/fresh observation。相同输入同ID/Digest，换内容Conflict。Unknown、cancel、deadline保真；写前零状态，写后进入`waiting_inspect`，lost reply只Inspect原ID，禁止新ID、重跑Tool、重复CAS或推进Turn。
- Effect/Recovery：本首切面只做本地current读取、确定性编译和Context Owner Store/CAS，不产生远程Source、Provider Cache、披露或其他真实外部Effect，因此不创建Operation V3 Effect Attempt、Permit或Enforcement。固定顺序为`settled Tool exact chain → Refresh/pending DomainResult → S2 → atomic ApplySettlement+Generation current CAS`。现有Settlement V4闭表不支持Context本地DomainResult，禁止扩V4或复用上游Tool settlement。
- 硬反例：Tool 0/2、Memory/Knowledge任一>1或Continuity非0；Context平行Memory/Knowledge nominal、Owner concrete Store/internal依赖、正文超过64KiB；Source Turn不来自具名Owner Reader、Ordinal不等于Tool/ExpectedCurrent T、Target非Context childExecution T+1；Application/Memory/Knowledge mint proof或自行`+1`、补造Session/Turn；S1/S2 closure/association/content/TTL漂移；Unknown/Unavailable/cancel/deadline降格；其余proof、cache、CAS与lost-reply反例不变。
- Root门：`CTX-D09-R1` A/B-local、Application Adapter与B-cross fixture已完成；C层production composition、Capability、Harness Continuation/Turn仍NO-GO。

### `CTX-D09-R1` Runtime local-domain-transition Resolved Decision

- 裁决：Context本地`pending DomainResult → S2 → atomic ApplySettlement+Generation current CAS`无需Runtime settlement；
- Owner：Context Owner独立拥有并提交本地状态迁移；Application只调用公共Port；Runtime不创建、保存或返回该迁移的settlement ref；
- 不变量：Apply DTO无Runtime settlement字段；不得修改/扩展V4、引入任何additive Runtime settlement或复用Tool settlement；S2必须先于原子Apply/CAS；
- Recovery：Apply/CAS Unknown只Inspect原Context RefreshAttempt；未确认原子成功前current pointer不可见；
- 兼容影响：零Runtime schema/port变更；A/B-local与B-cross fixture均已完成，C层生产启用门不变。

## CTX-D10 Context ParentFrame Applicability Current Reader V1

- 用例：Runtime G6A Action矩阵把Context维度设为`required`时，证明产生本次Tool Call的ParentFrame、Manifest与Generation在检查时仍属于同一ExecutionScope/Run/Session/Turn，并向既有Runtime Applicability Reader合同返回短租约current投影；不证明下一Turn Frame。
- 语义Owner：Context Engine拥有`ContextParentFrameApplicabilitySourceCoordinateV1`、sealed subject、ParentFrame current projection及S1/S2 Inspect；Runtime拥有Evidence矩阵、公共ref/projection验证与Issue资格；Application只携带中立坐标。
- nominal Source Coordinate采用方案A：固定Context Kind，`ID=exact FrameID`，`Revision=Frame Revision`，Digest seal exact FrameRef、ManifestRef、GenerationRef+Ordinal、ExecutionScopeDigest、Run、neutral Session exact coordinate、Turn、ParentFrame/Generation binding、RecipeRef与AuthorityDigest；禁止只哈希Frame ID、不可逆hash ID或普通FactRef/Application DTO type-pun。
- Reader输入：exact SourceCoordinate/Frame/Manifest/Generation、ExecutionScopeDigest、Run/Session/Turn、Parent binding、checked time、request not-after；输出Context-owned exact current projection及Checked/Expires/ProjectionDigest。
- metadata反查：采用方案A，但`ID=FrameID`只提供首个可查key，不授予唯一性。Context Owner新增只读`ResolveExactSourceBinding`、`FrameByExactRef`、`ManifestByExactRef`、`GenerationByExactRef`、`InspectCurrentGenerationPointer` ports。S1/S2都以完整Source四元组从Owner metadata index解析sealed binding，再以完整`FactRef{ID,Revision,Digest}`和预期ExecutionScopeDigest复读Frame/Manifest/Generation/current pointer、Recipe/Authority上界与ReferenceStore，并完整执行`kernel.InspectFrame`；不得由Coordinate Digest猜测Frame Digest。
- TTL：`min(request上界, checked+30s cap, Frame真实Expires, Manifest真实Expires, Generation/Recipe/Authority及其他可读current上界)`；cap不是SLA，不得伪造、延长或用默认值替代领域TTL。current上界不可读即Unavailable。
- Runtime Adapter：`context-engine/runtimeadapter`实现既有`OperationScopeEvidenceApplicabilityCurrentReaderV3`的Context Kind路由。Runtime router仅把Source `Kind/ID/Revision/Digest`逐字段无损投影为公共`OperationScopeEvidenceApplicabilityFactRefV3`；Adapter将完整公共四元组交给Context Owner Source Binding/Current Reader，Owner exact读取后返回公共`Fact + ExecutionScopeDigest + Current + Expires + Digest`，不得重分配ID/Revision/Digest或延长Expiry。Adapter构造时只注入Reader接口、Kind和Clock，禁止注入Frame/Manifest/Generation对象、sealed binding map或快照冒充current。
- 不变量：Runtime不创建Context Applicability Fact；公共ref、Application DTO和普通FactRef本身都不授Evidence资格；只有矩阵required、Kind路由、Context Reader exact-current投影与Runtime公共验证共同成功后，Runtime才可继续其Owner流程。
- Run Requirement：CTX-D10不新增或改写RunStart/RunSettlement Requirement；它只在Runtime既有OperationScope Action矩阵已将Context标为`required`时提供该维度的Owner-current读取。activation中Context仍为forbidden的矩阵不注册此Kind。
- Effect/Recovery：Reader与Adapter严格只读，无Effect、Receipt、DomainResult、Settlement或CAS；Unavailable、丢读或并发漂移直接Fail Closed，调用方只能复读，不得创建替代坐标。
- 硬反例：Source binding或Frame/Manifest/Generation/current pointer NotFound/Unavailable；同FrameID换Revision或Digest；同FrameID跨Tenant/ExecutionScope返回歧义；取回Frame计算的scope digest与请求/公共projection不一致；ReferenceStore content changed；current pointer/Run/Session/Turn漂移；TTL crossing；source/public ref type-pun；Kind缺失或路由错误。全部Conflict或Fail Closed，并产生零Evidence Issue、零Tool watermark、零Provider调用。
- 兼容影响：不修改Runtime公共Port/schema；需要宿主composition root把Context Kind注册到公共Applicability router。Application `SingleCallParentFrameCoordinateV1`/Input S1/S2只携带中立exact坐标并依赖Reader；Application/Runtime/Harness不得import Context实现或获得写口。
- 当前状态：CTX-D10、CTX-D09 A/B-local、Application公共Port Adapter与Memory/Knowledge B-cross fixture均已实现并验证；Runtime Settlement调用为0。production root、Capability、Harness Continuation与Turn推进仍未实现/执行。
