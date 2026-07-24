# Harness Assembly结构化Port Delta

本文件只登记联合Owner需要冻结的公共合同差额，不授权修改Runtime、Application、Agent Assembler、Model Invoker或六组件代码，也不允许Harness私建兼容接口。

## HA-A01：Agent Assembler最终输出到AssemblyInput

- 用例：把最终Resolved Agent Plan、Harness Bootstrap、Expected Injection Manifest和组件贡献交给pre-binding Assembly Compiler；运行期Actual Injection不属于该输入；
- 语义Owner：Agent Assembler拥有Resolved输出，Harness Assembly拥有输入消费合同；
- 输入：ResolvedAgentPlan/HarnessBootstrapPlan Ref+Digest、Required/Optional Capability、Route/Context/Tool计划、Expected Manifest；
- 输出：版本化`AssemblyInputV1`；Compiler产物经独立`AssemblyHandoffV1`交给Runtime形成Binding，随后只读执行`AssemblyBindingConformanceV1`；
- 不变量：Assembler决定“应该是什么”，Compiler验证pre-binding贡献可否形成sealed Graph；本次Generation对应的BindingSet只能在handoff后形成，Actual Injection只能在Model Turn后形成；缺项/漂移Fail Closed；
- Effect/Recovery：纯数据交接，无pre-run Effect/Evidence；版本不兼容返回Diagnostic；
- 反例：Harness根据缺失字段重新选Route或通过私有extension补造计划；
- 兼容影响：旧Bootstrap只能经显式迁移器升级；不得静默默认；
- 状态：已冻结为独立`AssemblyInputV1`与后续`AssemblyHandoffV1`/`AssemblyBindingConformanceV1`，允许P1/P2/P3a实现；

## HA-H01：Assembly SDK、CompiledGraph与Slot/Phase合并

- 用例：六组件共享同一公共Catalog和Contribution合同，编译出可复现Graph；
- 语义Owner：Harness Assembly；六组件Owner只拥有各自Contribution/Port/Run Requirement；
- 输入：版本化SlotSpec、HookFaceSpec、Module/Slot/Phase Contribution、Dependency、PortSpec；
- 输出：AssemblyManifest、CompiledHarnessGraph、Diagnostic、Conformance/Residual Report；
- 不变量：只有Observer/Filter/Gate/Port；DAG与排序确定；组件不能自建枚举；私有Harness Port不外露；
- Effect/Recovery：纯编译，无Effect；冲突拒绝并可用同输入重算；
- 反例：组件注册任意字符串Hook并联网改Context，或按注册先后决定顺序；
- 兼容影响：新增可选Slot/Phase走minor；权限、Schema或主语义变化走major和新Generation；
- 状态：公共Catalog与Contribution分类已冻结，允许P1/P2/P3a实现；运行期P3b与六Owner Adapter仍冻结。

## HA-R01：Assembly Generation到Runtime Binding V2映射

- 用例：Runtime Activation能验证某Endpoint/Instance绑定的是确切sealed Generation/Manifest/Graph；
- 语义Owner：Runtime Binding；Harness仅产出handoff candidate；
- 输入：Generation/Manifest/Graph Digest、BindingSetRef、ComponentManifestRefs、Conformance/Residual摘要；
- 输出：Runtime拥有的Binding/Activation关联Fact，或经批准的`GovernanceExtensionV2`映射；
- 不变量：Harness不能写Binding Fact；Activation和每次高风险dispatch仍重验currentness；
- Effect/Recovery：Binding/Activation沿Runtime既有治理；漂移生成新Generation/Binding，不原地改Graph；
- 反例：把`RuntimeProviderBindingV1`候选当正式Binding或Activation资格；
- 兼容影响：需要Runtime Owner确定first-class字段还是已登记Extension；旧Binding未声明映射时不得推断；
- 状态：已由Runtime Generation–Binding Association公共合同与Harness `assemblyadapter` exact映射/只读Conformance闭合，不再是Action Gateway阻塞；后续不得重造Runtime Binding Fact语义。

## HA-M01：Model Invoker公开Route桥

- 用例：`model.turn` Slot通过RouteID和公开execution union发起governed Model Operation；
- 语义Owner：Model Invoker拥有Route解析、Provider调用、流事件归一化与本地Invocation结果投影；Runtime Settlement Owner拥有Operation Settlement；Harness只拥有Run内协调；
- 输入：RouteID、public request union、Context payload/ref、Operation/Fence/Scope/Budget refs；
- 输出：public Observation/Result union、ToolCall/FunctionCall Candidate Observation、Provider Inspect引用与Runtime Operation Settlement引用；Harness据此创建PendingAction，Tool Engine再从精确PendingAction创建领域ActionCandidate，Model Invoker不直接创建二者；
- 不变量：不依赖internal/vendor SDK/Raw/Native；Tool Call不是结果；Provider状态不是Runtime终态；
- Effect/Recovery：Operation V3；Begin后丢包只Inspect原attempt；
- 反例：Harness解析厂商流事件并据`completed`直接完成Run；
- 兼容影响：ContextReference未获Route支持时required Fail Closed、optional Residual；
- 状态：公共桥可设计，具体Adapter在Model Invoker公开合同联合冻结后实施。

## HA-M02：Model PreDispatch Surface Commit Gate V1

- 用例：把tool-bearing Model调用拆为纯本地Preparation与ACK后actual-point，在两者之间把Invocation、Tool Surface及Assembly single composite current create-once绑定；
- Owner：Model拥有Preparation、ACK public nominal/canonical与actual-point调用；Harness/host拥有唯一Commit Gate、Model ACK create-once Repository与接线次序；Tool拥有Surface Current及InvocationSurfaceBinding Repository；
- 输入：Model公开完整Prepared/Current refs与Readers；Harness A2完整Verified Assembly OwnerCurrent Reader；Runtime窄`GenerationBindingAssociationCurrentReaderV1` canonical Gateway；Harness Handoff current Reader；Tool public完整`ToolSurfaceManifestCurrentReaderV1`；Runtime Registry Exact Reader；最终Runtime-neutral `ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1`；
- 输出：Model-neutral exact ACK；Tool Owner用自身Clock生成Created/NotAfter并Seal Tool Fact/Ack，Harness ValidateAgainst Tool winner后用Owner clock Seal并Ensure Model ACK；
- 不变量：Phase A外部调用0；Owner读取固定`A2→B1→Handoff→C2→Registry`并完整S1/S2；Tool Surface expected只等于Model ActualToolSurfaceDigest；ActualProviderInjectionDigest独立保存；最终仍只有一个Runtime-neutral Assembly composite；Binding/Ack不授Provider进入权；
- Effect/Recovery：Tool仓与Model ACK仓不宣称跨仓原子；Commit第一项Owner调用先`InspectByPreparedCurrent`。命中后必须复读同一Prepared Current并取fresh Owner clock验证stored ACK+Current freshness，成功才返回clone；该路径零Tool/Ensure/重Seal但不得零clock，过期PreconditionFailed。只有同实例`by_prepared_current + by_prepared_ref`证明authoritative never-created，才恢复/Ensure Tool winner，S2后取fresh clock Seal/Ensure Model ACK；Unavailable/Indeterminate只Inspect原key，不盲写；每个Provider attempt/Open/Stream/continuation前重新Inspect exact Ack与全部current；
- 反例：把Context ExpectedInjection与Model provider actual强制相等、按Fact查latest Current、Harness提交sealed Tool Fact、ACK前Resolve/Capabilities/Preflight、一次ACK永久放行、continuation改变actual digest；
- 兼容：不改ModelTurnPort/Loop V1字段与摘要，不改Application P2；需Model两阶段重构和所有actual-point guard，旧Generation不得推断支持；
- Model Delta：Harness只无损复用Model公开完整Fact/Ref/Reader nominal，不定义HistoricalProjection、Plan/Tools/Route/Profile/Registry/Current镜像；完整Registry exact Ref必须含Owner/ContractVersion/ID/Revision/Digest，完整Current Ref必须含ContractVersion/Prepared/Checked/Expires/NotAfter；Historical NotAfter是资格绝对上界，Current/Tool窗口不得超过；
- Gate/ACK方法集：Harness只exact实现Model短名`Commit + InspectExactAck`；同一Harness-owned ACK Repository实例提供Owner-local `EnsureAck + InspectExactAck + internal InspectByPreparedCurrent`，原子维护`by_ack_id/by_prepared_current/by_prepared_ref`，stable key为完整Prepared+Current且一个Prepared epoch只能有一个Current/ACK；
- Tool方法集：Tool只注入公开Writer/Reader并调用`EnsureToolSurfaceInvocationBindingV1`；Exact Reader以Model ACK公开neutral SurfaceBindingRef单参读取，Harness仅无损映射Owner/Contract/ID/Revision/Digest，不定义Harness CommitRequest/Commit Writer、不按Kind猜源；
- A2 Owner artifact：Harness唯一Store以immutable `(ID,revision)` history、stable current index与full-Ref CAS完整保存`CompileResultV1 + AssemblyBindingConformanceV1`；Reader用unexported marker package-sealed。stable ID/CompileDigest/ProjectionDigest分别使用具名canonical input、独立固定domain/version/discriminator与literal golden；HandoffRef exact为`GenerationRef.ID+"/handoff"`/同revision/`Handoff.Digest`。Generation/Manifest/Graph分别重算`GenerationDigestV1/ManifestDigestV1/GraphDigestV1`并执行字段硬门，Handoff调用live `Validate()`，Conformance调用`Validate(now)`；逐字段闭合Diagnostics/Residuals与Manifest Plan Profile/ToolSurface，所有nested pointer/slice/bytes输入、保存和返回均deep clone；
- B1 Runtime current：静态能力只暴露`GenerationBindingAssociationCurrentReaderV1`，但M2集成、验收、production与canonical fixture必须注入Runtime concrete `GenerationBindingAssociationGatewayV1`；注入资格只由`AssemblyBindingConformanceV1.Association` exact Ref、Conformance分离的BindingSet ID/revision/digest/semantic/currentness/projection字段、sealed `ProviderBindingCandidateV1`→Runtime BindingSet目标member full Ref映射、完整assembly lineage与Runtime Gateway package/type identity共同证明；association path Conformance Binding/Capability/Schema必须为空。禁止GovernancePort和新增BindingSet Reader。Runtime public conformance只验Fact shape且`ProductionClaimEligible=false`，仅限接口单测；static/cache/self-built/wrapper即使通过conformance也不得进入M2。concrete Gateway每次Inspect复读Generation/Activation并从完整Binding facts重建BindingSet current；成员Grant内容/TTL漂移、换代或撤销均在Gateway内Fail Closed；
- C2 Tool current：只注入Tool public `ToolSurfaceManifestCurrentReaderV1`并调用`InspectExactToolSurfaceManifestCurrentV1`；Tool唯一`ToolSurfaceManifestCurrentRepositoryV1`以`ToolSurfaceManifestCurrentEnsureRequestV1{ContractVersion,Manifest,ExpectedCurrent full Ref}`保存完整Manifest并拥有Ensure，Harness不获得Writer/Repo。rev1要求ExpectedCurrent严格零值，successor只允许current full Ref+1 CAS，current推进后重投旧revision必须Conflict/PreconditionFailed；lookup使用A2 Manifest Plan ToolSurface exact ID/Revision/Digest构造`ToolSurfaceManifestCurrentRefV1`并补public contract version；调用方不预知ProjectionDigest，返回后才独立验证完整`ToolSurfaceManifestCurrentProjectionV1`的Manifest/Owner/Profile/entries/expected injection/Checked/Expires及ProjectionDigest；
- Runtime ports additive Port Delta：新增唯一neutral `ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1`及field-specific Handoff/BindingSet/Manifest/Conformance/ToolSurface refs，并新增统一`RegistrySnapshotRefV1{Owner core.OwnerRef,ContractVersion,ID,Revision,Digest}`；禁止Harness私有Registry Ref与旧Model名。type owner=Runtime ports，semantic/publisher/revision/current index/Reader implementation Owner=Harness。完整Generation复用`GenerationArtifactRefV1`；Current Ref与Projection都完整携Generation/Handoff/BindingSet/Manifest/Conformance/Watermark/ToolSurface/Profile/Registry/Semantic/Currentness/Checked/Expires/ProjectionDigest并逐字段exact；计算时排除各自own digest，最终Ref Digest/Ref ProjectionDigest/Projection Digest相等，无Kind；
- Composite映射：live DAG下Harness Gate需依赖Tool公开Port，Tool反向import Harness会成SCC；Harness Reader返回Runtime public neutral type，Tool Binding直接嵌入同一Projection，禁止Tool echo/alias/Kind或字段转换；Runtime只拥有type，不拥有Assembly语义；
- TTL与fixture：`Checked=max(A,B.Binding.Issued,Handoff,C)`，`Expires=min(A,B.Fact,Handoff,C)`；Registry无公开TTL时不伪造。真实fixture必须分别使用Harness A2 concrete Store、由Association Ref + Conformance分离BindingSet字段 + ProviderCandidate→Runtime member exact mapping证明的Runtime concrete Gateway、Harness Handoff Store、Tool唯一Surface Repo、Runtime Registry Exact Reader和Harness composite Store；禁止一个fake扮演多个Owner。conformance只在独立Reader接口单测运行，不进入fixture资格判定；
- Tool边界：NotAfter只取显式Owner Fact/current与caller deadline的min，拒绝额外匿名上界；
- Context边界：Context ExpectedInjection保留独立Injection Conformance，不是本Bridge Reader依赖，也不进入Tool Binding等式；
- Memory/Knowledge Delta10/11评审：第二套DTO已撤但G6B仍P0；Context是TransitionProof唯一Owner，Application只协调；SourceTurn full exact ref来自Session/Turn Owner Reader，Harness不接SourceTurn/Proof，只消费Context final TargetTurn current Frame；G6B前Sources=0且Reader calls=0；
- 状态：`A2+B1+C2` M2、Runtime neutral Current Reader、Harness concrete Model Gate与同实例ACK Repository均已完成Owner-local实现/测试并通过对应独立代码审计。剩余Port/接线缺口是Model actual-point全路径guard inventory、Harness Assembly专用required Capability/Conformance及Tool V2 Consumer；system G6A与production root继续`NO-GO`。详见[完整裁决](../port-deltas/model-predispatch-assembly-owner-current-v1.md)、[完整设计](./model-predispatch-surface-commit-gate-v1.md)与[测试矩阵](./model-predispatch-surface-commit-gate-v1-test-matrix.md)。

## HA-D01：六组件Contribution与OperationScope / RunStart / RunSettlement Requirement发布

- 用例：六组件向已冻结Slot/Phase声明能力、Port、依赖和收口要求；
- 语义Owner：各领域Owner拥有Port/Fact/Inspect/CAS/Settlement，Harness拥有Contribution envelope与Catalog；
- 输入：ComponentManifestV2Ref、Module/Slot/Phase Contribution、PortSpec/Binding、OperationScope、RunStartRequirement、RunSettlementRequirement/Participant refs；
- 输出：编译后的Contribution binding和Residual；
- 不变量：组件不导入彼此实现、不复制共享类型、不新增公共枚举、不把Receipt升级为Fact；
- Effect/Recovery：声明本身无Effect；运行期Effect走领域Operation和Owner Inspect；
- 反例：Tool组件直接实现Harness私有ModelTurnPort，或Sandbox Receipt被Harness写成Cleaned Fact；
- 兼容影响：六组件可独立增发namespaced Contribution，但公共Catalog变化由Harness版本治理；
- 状态：需六Owner分别冻结最小贡献及三类治理声明，禁止用泛化Run Requirement互相代替。

## HA-X01：Action Gateway、Checkpoint与per-turn refresh接线

### HA-X01A：单Call Action Gateway

- 用例：将Model Invoker已完成且`Calls`恰好为1的Tool Call Observation，经Runtime Evidence和绑定Settlement Owner升级为Harness PendingAction，再通过Tool领域治理、V4 Enforcement/Settlement形成settled Action输出，经Context Refresh Settlement/Generation CAS/S2取得new Frame后完成Harness Continuation；
- 语义Owner：Model Invoker拥有Observation；Runtime拥有Evidence、V4治理、Operation Settlement；Model-turn Settlement Owner拥有`SettledTurnResultV2`语义；Harness拥有Session/PendingAction/Continuation CAS；Tool&MCP拥有ActionCandidate、Reservation、DomainResultFact、Inspect/CAS/ApplySettlement；Application Owner只拥有窄协调Port与中立DTO，Application Coordinator只协调；
- 现有公开输入输出：Model侧`ToolCallCandidateObservationV1`；Run Evidence V2；Model-turn `OperationSettlementRefV3`与Harness `ApplySettledTurnV2`；Runtime Dispatch V4/Enforcement 4.1/Evidence V3；Tool侧`ActionCandidate`、Reservation、DomainResult；Runtime `OperationSettlementRefV4`、`OperationSettlementEvidenceAssociationRefV4`、`OperationSettlementTerminalGuardRefV4`、`OperationSettlementTerminalProjectionRefV4`与current `OperationInspectionSettlementRefV4`；完整prepare/execute通过`InspectOperationSettlementEvidenceAssociationV4`读取；
- 封闭矩阵：`OperationScopeKind=run`、`EffectKind=praxis.tool/execute`、`PolicyProfile=praxis.tool/single-call-action-v1`，Run/Session/Turn/Action/Context全部required；其他组合Fail Closed；
- Model只读依赖：live `ToolCallCandidateObservationRefV1`、`ToolCallCandidateObservationProjectionV1`、公共Projection exact Reader与原子`Ensure`已经终审YES。Harness Adapter形成Application Request/复读Input前必须通过该Reader按完整Ref复读Projection并验证Ref/lineage/digest及`Calls==1`；Reader unavailable、Ref变化或Calls不等于1时零Application dispatch。不得从PendingAction payload、event JSON或compat tool calls反推Projection；本设计不定义Model写口、Repository或`Ensure`实现；
- Harness新增公共面：只读`CommittedPendingActionReaderV1`返回完整PendingAction、Session canonical digest、最长30秒projection，以及Harness-owned distinct Session/Turn source coordinates；两者不直接返回公共applicability ref、不授Evidence资格。新增Harness-owned `CommittedPendingActionApplicabilityBindingV1`作为immutable、deep-cloned、subject-scoped Adapter配置，只canonical绑定稳定`CommittedPendingActionSubjectV1`、预期coordinates与digest，明确排除CheckedAt/Checked/Expires。Harness `runtimeadapter`lookup后以fresh CheckedAt反查Reader，返回后再采时验证无回拨、未跨TTL且公共expiry不超过底层projection。Application-facing `SettledActionContinuationPortV1`与additive `ActionContinuationBindingV1`只绑定exact refs，不原地扩大`ContinuationRefV2`；
- Runtime接线（无新FactPort Delta）：G6A Action applicability router只把Harness source coordinate的`Kind/ID/Revision/Digest`逐字段无损投影为公共`OperationScopeEvidenceApplicabilityFactRefV3`，不创作新ID/Digest、不声称新Runtime Fact。Evidence Gateway复用现有`OperationScopeEvidenceApplicabilityCurrentReaderV3`校验current后Issue；live `OperationScopeEvidenceFactPortV3`没有Applicability Fact Create/Inspect；
- 跨Owner依赖：Application Owner发布G6A/G6B窄Port与中立DTO；Application Request只携带Harness source-coordinate的distinct中立镜像，不预塞公共applicability refs或Binding；后者仅在后续Tool/Runtime路由内由exact coordinates投影。Binding不是Application DTO、Fact、Authority或Evidence，不进入Runtime FactPort。Tool Adapter在`tool-mcp`、Context Adapter在`context-engine`、Continuation Adapter在`harness`内实现这些Application公开Port。各Owner Adapter允许依赖Application公开contract/ports，但不得依赖Application coordinator/kernel/实现；Application不得import Harness/Tool/Context。Context Owner-local实现可用本Owner fixture隔离；G6A PASS后，G6B test-only跨模块fixture可手工提供Binding并注入公共Port/Adapter，但不是production root、不启用Capability、不生产调用Continuation或推进Turn。当前不宣称production composition root存在；宿主Owner在G6B生产启用前另行设计、实现、验收真实root并负责生产Binding构造。只有G6B完整验收与真实root接线Conformance同时PASS后，production Capability、Continuation和Turn推进才可GO。Harness Assembly只接收、校验和组装已注入接口，不承担具体wiring、不import Tool/Context实现、不复制类型、不创建跨域Composition模块；
- Evidence闭包裁决：复用current `OperationInspectionSettlementRefV4`内四类typed exact refs及DomainResult/Owner/Effect revision/TTL，并public Inspect完整Association prepare/execute；不接受裸pair、字符串ref或Harness私封Evidence闭包；
- 不变量：Calls exactly 1；Observation/Receipt不可直接升级PendingAction或ActionResult；Tool Candidate只能来自已提交PendingAction；Application不能创作Owner Fact；Tool DomainResult→Runtime Settlement V4→Tool Apply分层；settled ToolResult必须再经`Context Refresh → pending DomainResult → S2 → Context Owner local atomic ApplySettlement/Generation CAS`，new Frame才可Continuation；Context本地迁移零Runtime Settlement；P3b不得参与；
- Effect/Recovery：Intent/Reservation→Admission→Permit→Begin→Delegation/Prepare→Enforcement prepare→Evidence consume→Enforcement execute→Evidence consume→Tool DomainResult→Settlement V4→Tool Apply→Context Refresh→pending Context DomainResult→S2→Context Owner local atomic ApplySettlement/Generation CAS→Continuation；任一lost reply先Inspect exact source/attempt/revision，UnknownOutcome只Inspect原attempt；
- 反例：取`Calls[0]`；未通过Model exact Reader即形成Application Request；从PendingAction payload/event JSON/compat calls反推Projection；Model Reader unavailable/Ref漂移/Calls不等于1仍dispatch；Provider Receipt直建Result；Action维度引用PendingAction；五维缺一；两个Coordinator同PendingAction换ID/内容；Evidence append后换sequence；Begin后换attempt；两phase交换/复用；current V4 Inspection、完整Association、ToolResult漂移；Tool Apply后用旧/未settled Frame直接Continuation；Context Frame/Generation或Continuation摘要漂移；Harness Reader直返公共applicability ref；router改写source四字段；unknown ref/Binding缺失/Binding digest漂移/Reader drift仍Issue；同键换Subject或coordinate；Binding包含观察时间、重放构造期CheckedAt、clock rollback/TTL crossing仍current；可逆ID解码；运行期注册或全局mutable registry；Binding进入Application DTO/Runtime FactPort；Session/Turn coordinate type-pun；Application Request预塞公共ref；Application import任一Owner；Owner Adapter依赖Application coordinator/kernel/实现；Harness Assembly import Tool/Context实现或承担具体wiring；
- 兼容影响：纯文本Run不绑定该能力；单Call能力只随新Catalog/Generation显式启用；旧Graph不得推断支持；`N>1`、P3b与per-turn refresh需要新版本；live ToolResult若仍绑定V3，必须由Tool Owner做additive升级，Harness不翻译；
- 状态：PendingAction Reader、Runtime/Tool/Model前置、Route、Identity/Owner-current及Harness P3 Assembler/InputCurrent Reader均为`YES`；Tool Consumer/P4与system fixture未实现，G6A/G6B/production root继续`NO-GO`。未来G6A输出仍止于settled ToolResult + current V4 Inspection + public Association Inspect。详见[完整设计](./action-gateway-v1.md)与[测试矩阵](./action-gateway-v1-test-matrix.md)。

### HA-X01B：N>1、Checkpoint与per-turn refresh

- 用例：后续多Call聚合、Continuity Barrier/Restore和每Turn Context current refresh；
- 语义Owner：Tool&MCP、Continuity、Context&Cache分别拥有领域语义；Runtime/Application拥有治理与跨域编排；Harness只拥有等待点；
- 输入输出：仅允许未来联合冻结的Versioned Domain Port、Operation/Settlement refs和Harness Candidate；
- 不变量：`N>1`只保留完整Observation/Evidence并NO-GO，禁止部分执行；Checkpoint不拥有外部世界；refresh不得绕过Disclosure/Route支持；
- Effect/Recovery：按各领域Effect/ConflictDomain治理；Unknown只Inspect原attempt；
- 反例：`action.batch.completed`被当成已支持多Call；Harness在Hook里直调Tool；把本地快照称权威Checkpoint；每Turn隐式联网更新Context；
- 兼容影响：分别作为后续可选Capability加入；未冻结前不进入可运行Graph；
- 状态：继续延期，需独立多Owner设计。

## HA-X02：Controlled Operation Provider强类型Route V2

- 用例：证明G6A actual Tool Provider只有`Application Tool Port -> Tool Adapter -> Runtime V2 Governance Gateway -> exact Tool/Provider Transport`一条可装配路线；Runtime kernel-internal Runner不进入Harness公共合同或fixture；
- Owner：Harness Assembly拥有Declaration/Conformance事实canonical/digest及Route Current Store/Adapter发布语义；Runtime ports只拥有六个跨边界中立类型。`type Owner != fact semantic Owner`，两侧禁止反向import或复制类型；
- 合同：additive `ControlledOperationProviderRouteDeclarationV2`、required registered `GovernanceExtensionV2`、`ControlledOperationProviderRouteConformanceV2`及Runtime-facing Current Projection/Reader。Declaration独立声明ProviderTransport与actual Provider；Compile Result从真实Manifest/Module/PortSpec/Candidate形成含PortSpecDigest/ConflictDomain的两项post identity，post-binding分别形成ProviderTransportBinding与ProviderBinding，七Binding TTL闭包不变；no-bypass同时扫描两层alias/真实注入；
- 公共Owner/签名：`ExecutionRuntime/runtime/ports`唯一拥有DeclarationRef、ConformanceRef、CurrentRef、CurrentProjection、CurrentReader及现有MatrixKeyV3六个类型。Ref Revision统一为`core.Revision`、digest统一为`core.Digest`并带冻结snake_case JSON tags；Projection复用`GenerationArtifactRefV1`与七个`ProviderBindingRefV2`，Handoff/ActiveRoute使用flattened ID/revision/digest。Harness facts只产出runtimeports refs，不重定义/alias；真实Reader签名为`InspectCurrentControlledOperationProviderRouteV2(context.Context, ControlledOperationProviderRouteCurrentRefV2, OperationScopeEvidenceApplicabilityMatrixKeyV3) (ControlledOperationProviderRouteCurrentProjectionV2, error)`；
- 发布：ConformanceID按`Derive(RouteID, GenerationArtifactRefV1, BindingSetID)`确定，CurrentID按`Derive(RouteID, MatrixDigest)`确定；各Fact Owner单调CAS Revision。Conformance InputsReader与OwnerSource均package sealed；OwnerSource只给stable exact refs，Compile/ActiveRoute/Wiring各由独立Reader读取。Current Watermark直接绑定ConformanceRef、Generation/Handoff/BindingSet/ActiveRoute与七Binding，WiringInventory由Conformance Fact绑定并经ConformanceRef间接进入Watermark；Conformance成功后才sealed publish post-binding Current Ref/Projection。Tool Adapter与Runtime Gateway只接收composition注入的exact CurrentRef，无List/latest/discovery；
- Reader role可表达性：live `ProviderBindingRefV2`只在field-specific nominal位置、七项互不复用的namespaced Capability及Harness Conformance的Role+PortSpec映射同时exact时可用；任一映射不唯一即零publish。若Runtime Review认为该证明不足，必须新版本公共Delta，不得type-pun或静默修改既有类型/digest；
- 错误：只复用`core.ErrorInvalidArgument/ErrorNotFound/ErrorConflict/ErrorPreconditionFailed/ErrorUnavailable/ErrorIndeterminate`，same-ID drift=`ErrorConflict`、absent ID=`ErrorNotFound`，禁止新增`ErrExpired/ErrUnknown`；
- 兼容：不修改`AssemblyInputV1`或通用`RouteBindings`字段/digest，不创建万能Hook；
- 门禁：Runtime V2、Tool Adapter与本Route实现/Conformance未完成时，G6A cross-module fixture NO-GO；production root继续等待G6B完整验收；
- Conflict：closed code/phase强制provenance；prebinding alias/V1仅携AssemblyInputDigest，postbinding active-route携AssemblyInput/Graph/Wiring三项。Alias Conflict携closed exact AliasSurface；Legacy Fact只允许active/inactive/revoked，绑定Record-derived exact RouteBindingRef、sealed wiring与Checked/Expires；Compile只经Harness Owner-current Reader执行fresh-clock S1/S2；
- 反例：lost Conformance/Current publish、重复publish、CAS竞争、旧Ref、lost Entry create reply、两处TTL crossing、unknown、64并发、外包OwnerSource绕回、PortSpecDigest/ConflictDomain/Binding漂移、Conflict provenance/type-pun、AliasSurface漂移、Port kind生产路径、expired proof、不相关RouteBindingRef、同identity另一个active V1、S1/S2 current漂移、raw/alias Transport/Provider bypass及V1/V2 simultaneous-active结构化Conflict，详见[Route V2设计](./controlled-operation-provider-route-v2.md)和[测试矩阵](./controlled-operation-provider-route-v2-test-matrix.md)。

## 不提出的Delta

- Runtime P0.1-P0.6、Operation V3、Application Coordinator已经闭合，不重复提出；
- Assembly V1不生成权威pre-run Evidence，因此不提出pre-run Evidence Delta；
- Runtime cleanup exact Inspect已经有live接线，不重复提出旧Cleanup Port Delta。
