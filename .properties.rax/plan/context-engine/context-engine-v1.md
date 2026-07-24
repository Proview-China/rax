# Context Engineering与Cache v1实施计划

状态：规划内无需production composition root的Owner-local/B-cross/reference-only切片均已完成软件验证，包括`CTX-D09-R1`、`CTX-D10`、Offline/Engineering SDK、Compaction/Generation、Outcome、Recipe/PromptAsset pre-release、Prompt Provenance、durable Reviewer Context、Restore Context materialization及Component Release候选。production Recipe/Prompt发布仍等待CTX-D07；C层production composition、Capability、Harness Continuation与Turn推进未启用。

## 1. 范围

`CTX-D09`三段Refresh/Apply/Inspect、Owner-local settled-action current projection和本组件fixture已经实现并通过第二轮独立复审；`CTX-D10` Reader/Adapter、Application公共Port Adapter及Memory/Knowledge G6B B-cross fixture也已完成。这些完成项不授予production root、Capability、Harness Continuation或Turn推进资格。

V1范围：Context Source/Candidate、Recipe、Admission、Manifest、Frame、Generation/Compaction、Artifact Anchor/Delta、Stable Prefix、Provider-neutral CachePlan/Cache Fact、Expected/Actual Injection、Outcome/Evaluation、Go SDK/API/CLI及治理Adapter。

不做：生产DB/RPC/拓扑/SLA、通用Tokenizer选择、厂商SDK直连、Rust、Runtime/Harness/Model Invoker内部修改、任意Hookface、pre-run Runtime Evidence。

## 2. 实施前硬依赖

G6B隔离实现开门条件仅有：

1. settled `ToolResultV2` exact ref；
2. 对应current V4 Inspection exact ref；
3. verified Association exact ref；
4. Application发布三段`ContextTurnRefreshPortV1`与只读`SettledActionContextSourceCurrentReaderV1`公共合同后，Context Owner Adapter才可进入跨模块编译/fixture；当前本组件中立合同与fixture不得冒充该公共Port；
5. `CTX-D09-R1`已冻结：本地迁移无需且不得创建/消费Runtime settlement；Context Owner原子ApplySettlement+expected Generation current CAS。

Runtime P0已解除：A/B-local、Application公共Port Adapter与Memory/Knowledge B-cross fixture已完成，且不构成production root。C层仍要求Route exact materialization、Production State Plane/root及系统验收全部通过。

### 2.2 Root门三层

- A Owner-local：已完成并通过第二轮独立复审；只在Context独占目录实现kernel/store/adapter，无需production root；
- B 本组件test-only fixture：已完成并通过第二轮独立复审；只验证Owner-local合同与backend，不冒充Application公共Port或跨模块集成；
- G6B test-only cross-module fixture：已通过Application公共Port/DTO手工注入Memory/Knowledge V2 Owner Readers并完成验证；它不是production root，且未注册Capability、调用Harness或推进Turn；
- C production：只有production composition root完成真实Application/Context/Harness装配且G6B验收及生产依赖通过后，才可启用capability、调用Harness Continuation和推进Turn。

### 2.1 CTX-D10 G6A ParentFrame只读前置

1. 采用可反查方案A：Source Coordinate `ID=FrameID`、`Revision=Frame Revision`，Digest seal exact Frame/Manifest/Generation/ExecutionScope/Run/Session/Turn/Parent binding；ID只作首个查询key，不假设跨Tenant/ExecutionScope全局唯一；
2. 新增Context Owner只读metadata ports：`ResolveExactSourceBinding`、`FrameByExactRef`、`ManifestByExactRef`、`GenerationByExactRef`、`InspectCurrentGenerationPointer`；所有对象按完整`FactRef{ID,Revision,Digest}`与预期scope exact读取，live ReferenceStore只存Content bytes，不承担metadata/current；
3. Reader执行S1 metadata/ReferenceStore复读、完整`InspectFrame`、TTL最小值、S2同集合复读；
4. `runtimeadapter`只依赖Runtime公共`core/ports`与Context Reader ports，按Context Kind实现既有Applicability current接口；构造时禁止注入Frame/Manifest/Generation对象快照；
5. Application `SingleCallParentFrameCoordinateV1`/Input只携带中立坐标；Runtime router只无损投影四元公共ref；公共ref不授Evidence资格；
6. Reader/Kind router不可用或任一漂移时，零Evidence Issue、零Tool watermark、零Provider。

CTX-D10的线程安全metadata Fake与test fixture已完成隔离验证，但不是production backend/root/SLA。当前production composition root仍不足以启用G6B或调用Harness continuation。

Harness live Assembly Generation/CompiledGraph、`context.frame` Slot、既定Context/Turn phases、Generation-Binding Association及Candidate `ContextRef+Digest`承载不再列为缺失能力。Checkpoint不是首个per-turn闭环前提，但Refresh不得私建Checkpoint替代品。

`CTX-D09-R1`本组件A层、B层fixture、Application公共Port Adapter与Memory/Knowledge B-cross均已完成并通过验证。禁止私建Runtime settlement接口、扩V4、引入additive Runtime settlement或复用Tool settlement；C层仍只可由production composition root启用。

## 3. 预期代码文件级落点

```text
ExecutionRuntime/context-engine/
  go.mod
  README.md
  contract/
    version.go
    recipe.go
    source.go
    admission.go
    manifest.go
    frame.go
    parent_frame_current.go
    generation.go
    artifact.go
    cache.go
    injection.go
    outcome.go
    release.go
  ports/
    refresh.go
    source.go
    facts.go
    frame.go
    metadata_current.go
    parent_frame_current.go
    artifact.go
    cache.go
    evaluation.go
  kernel/
    refresh.go
    parent_frame_current.go
    compiler.go
    admission.go
    ordering.go
    budget.go
    materializer.go
    compaction.go
    cache_planner.go
    injection_compare.go
  applicationadapter/
    domain.go
  runtimeadapter/
    manifest.go
    settlement.go
    parent_frame_applicability_v3.go
  sdk/
    recipe.go
    preview.go
    inspector.go
  cmd/context/
    main.go
  api/
    service.go
  internal/testkit/
    metadata_store_v1.go
  tests/
    contract/
    blackbox/
    failure/
    conformance/
    integration/
    system/
```

Harness/Model Invoker/Continuity/Artifact侧Adapter必须落在各自Owner或统一集成目录，不写入本模块，不由本计划擅自确定。

A层live落点为`contract/refresh.go`、`ports/refresh.go`、`kernel/refresh.go`、`refreshstore/memory.go`、`internal/testfixture/refresh_v1.go`及对应测试。Store已支持pending DomainResult不可见与最终ApplySettlement+Generation current CAS原子提交；Apply DTO无Runtime settlement字段。`applicationadapter/context_turn_refresh_v1.go`只实现Application公共Port，不是production composition Adapter/root。

Application公共合同已由其Owner发布在`ExecutionRuntime/application/contract/context_turn_refresh_v1.go`与`ExecutionRuntime/application/ports/context_turn_refresh_v1.go`，包含三段Port/DTO与中立Owner Source Reader；这些文件不属于Context独占范围，本组件没有复制定义。B-cross fixture已手工注入公共Port；C层仍须由production composition root注入Context Adapter。

`CTX-D10`live落点为`contract/parent_frame_current.go`、`ports/metadata_current.go`、`ports/parent_frame_current.go`、`kernel/parent_frame_current.go`、`runtimeadapter/parent_frame_applicability_v3.go`、`internal/testkit/metadata_store_v1.go`及对应contract/blackbox/failure/conformance测试；本阶段只复用，不修改其合同或扩大范围。

## 4. 阶段计划

### P0：联合合同冻结

- 审核全部Port Delta、Slot/Phase贡献和Dependency DAG；
- 记录Runtime Owner已冻结`CTX-D09-R1`为零Runtime Settlement；不新增Runtime schema/Port；
- 分配Schema namespace、SemVer、Capability、Effect kind和Run Requirement；
- 确认Context Reference不可物化Route的Fail Closed/Residual政策；
- 记录G6A三项输入与`CTX-D09-R1`本组件A/B完成；Application公共Port/DTO仍为跨模块Delta，不触发C层能力启用。

### P1：合同与纯验证

- 实现canonical schema、strict decode、Digest、clone与limit；
- 实现Recipe/Source/Frame/Cache/Anchor/Outcome状态验证；
- 导入边界测试禁止跨组件实现依赖；
- 不接网络、不写生产事实。

### P1.5：CTX-D10 ParentFrame Current Reader（已实现并隔离验证）

- 实现nominal Source Coordinate seal/validate，固定`ID=FrameID`首个查询key，但不假设全局唯一；
- 实现只读exact source binding/metadata ports与线程安全test Fake；
- 实现S1→ResolveExactSourceBinding→Frame/Manifest/Generation exact FactRef+scope/current pointer/ReferenceStore→完整InspectFrame→TTL→S2；
- 实现Context Kind runtimeadapter到现有Runtime公共Applicability projection；Adapter只转交公共四元组，不保存metadata快照/binding map；
- 禁止Evidence Issue、Tool watermark、Provider、DomainResult、Settlement、Apply、Generation CAS、新Frame、Continuation；
- 本阶段已完成；其current projection只作为G6A已证明输入，不替代CTX-D09联合Review或G6B启用门禁。

### P2：Domain Fact与状态机

- 实现线程安全Fact Port合同与CAS转换验证；
- Frame Attempt、Recipe Release、Cache Entry、Anchor、Generation状态机；
- Owner-local `ContextRecipeComparisonV1`纯结构diff：exact Base/Candidate Recipe Ref、全字段覆盖、规则顺序变化、规范change digest；不产生质量/兼容/发布结论；
- Context Outcome/Evaluation/Feedback Candidate不可变事实与Put-once/Inspect；量化观测不升级Task成功、Cache hit、Review或Runtime Outcome；
- Recipe pre-release不可变版本与`draft→validated→evaluated→review_pending|rejected` lifecycle-head CAS；published/rollback/revoke等待CTX-D07，不私建治理替代；
- PromptAsset按用户裁决采用内嵌规范化`instruction/example/policy`片段规格与exact ContentRef；不可变Put/Inspect、distinct `PromptAssetRefV1`、pre-release lifecycle-head CAS及确定性Candidate projection已经完成并通过分层验证。Prompt不决定Frame Region，production发布仍等待CTX-D07；
- lost reply exact Inspect、TTL、Authority/Scope drift与并发冲突。

PromptAsset实际文件为：`contract/prompt.go`、`contract/prompt_release.go`、`ports/prompt.go`、`kernel/prompt.go`、`promptstore/memory.go`、`internal/testkit/prompt.go`及contract/kernel/blackbox/failure/conformance tests。已执行验收覆盖ContentDigest、Role→Kind/Trust映射、RenderCompatibility exact membership、Authority/TTL、片段/evidence规范集合、same-ID换包、no-alias、cancel、Unknown/Unavailable、lost reply、非法状态、distinct PromptAsset ref、64并发单一successor/确定性投影与production动作unsupported。

### P3：Context Compiler

- Source currentness/Admission、语义去重、稳定排序；
- Required/Optional降级、预算与Manifest；
- Frame冻结、内容寻址、Delta/Rebase、Artifact Anchor；
- 本地Compaction与Generation。

Compaction/Generation Owner-local小闭环已完成：`contract/compaction*.go`、`ports/compaction.go`、`kernel/compaction*.go`及`refreshstore.Memory`冻结exact Source Frame Range、Summary、Retained Anchor/Open Effect/Outstanding Work集合、expected current、确定性候选Generation、S2、原子Generation current CAS与Inspect-only恢复。该线不依赖Application/Harness/Model Invoker/Continuity实现，也不产生Runtime Settlement。

### P4：Cache与Injection

- Stable/Semi-stable/Dynamic布局与CachePlan；
- 精确Partition、TTL、invalidation、经济性与usage解释；
- Expected/Actual Injection比较；
- Provider Cache能力未联合接入前只运行离线planner，不宣称真实hit。

### P5：Application/Runtime治理Adapter

- Context Domain Reservation/Attempt接入Application namespaced Step；
- 只有未来真实远程/披露/Provider写入才复用Operation Effect V3完整Admission/Permit/Begin/actual-point Enforcement/Inspect顺序，不重造Gateway；CTX-D09首切面不得创建该Effect链；
- 实现Context pending DomainResult、S2复读及原子ApplySettlement+Generation current CAS；Apply DTO和状态机不含任何Runtime settlement/ref；
- Begin后Unknown只Inspect原attempt；
- 不写Runtime Outcome/Binding/Policy/Trust。

### P6：Context Owner-local Offline SDK V1（独立软件验收已YES）

- 第七资产审计已确认中央零字节澄清后的冻结合同`P0/P1/P2=0`；中央随后单独授权Owner-local Go实现，当前`sdk`与唯一context-aware staged kernel候选已落地并完成owner自验；
- V1已实现`ValidateRecipeV1 / CompareRecipesV1 / CompileFrameV1 / PreviewFrameV1 / InspectFrameExactV1 / InspectCachePlanV1`；Compare是additive结构diff，Cache Inspect只核验调用者给定的provider-neutral Plan/Profile、TTL/currentness与离线经济性；两者均不包含质量评测/replay/refresh、Provider调用、Cache写或hit声明；其后的API/CLI只由独立`ContextOfflineIngressV1`包装这六个operation；
- live落点：`sdk/offline.go`、`codec.go`、`request_codec.go`、`bundle.go`、`operations.go`、`errors.go`、`workspace.go`及对应tests；
- 既有四入口保持第六审冻结合同；Compare新增exact Request/Response、两个非null Recipe、`MaxRecipes=2`、48/48 MiB、Request/Result/Comparison digest等式与零optional字段；Cache Inspect新增非null exact Plan/Profile、48/48 MiB、checked time、Plan/Partition/Key/Profile/Inspection digest等式。Operation闭集当前为六值；
- SDK已由Context Owner在`kernel/compiler_staged_v1.go`、`kernel/frame_staged_v1.go`和`kernel/reference_store_context_v1.go`抽取唯一context-aware staged Compile/Inspect/store helper；合同为`ContextAwareReferenceStoreV1`、`CompileStagedV1(ctx, store, CompileRequest, CompileWorkLimitsV1)`、`InspectFrameStagedV1(ctx, store, manifest, frame, InspectWorkLimitsV1)`；旧`Compile/InspectFrame`只作共享算法wrapper，SDK没有复制sort/admission/budget/render/Inspect算法；
- SDK仅依赖Context `contract/kernel`与Go标准库；Compile使用调用内context-aware ephemeral workspace并返回`Authoritative=false` bundle，不暴露`ReferenceStore`写口；
- Offline SDK的跨Owner source计数固定`MemorySources=0 / KnowledgeSources=0 / ContinuitySources=0`，Memory/Knowledge public contextsource Reader调用数为0；未单独确认的`knowledge_reference`按`unsupported`返回零Response；
- live `MemoryContextSourceCurrentReaderV1`、`KnowledgeContextSourceCurrentReaderV1`及Owner DTO是未来唯一上游来源；新语义只能由对应Owner additive V2或唯一无损facade承载，Context不建第二套nominal/DTO、不依赖Owner concrete Store/internal；
- Validate只做结构与canonical Digest；Preview只返回Admission/区域/token/exact refs/digests；Inspect只证明bundle内部exact，不声明Owner currentness；
- codec单元：六个Decode/Encode按operation/方向执行Validate 48/48 MiB、Compare 48/48 MiB、Compile 48/144 MiB、Preview 144/48 MiB、Inspect Frame 144/48 MiB、Inspect Cache 48/48 MiB hard cap，并拒绝unknown/trailing/递归duplicate key；独立base64 primitive golden覆盖0/1/2、48 KiB-1/exact/+1、96 KiB双chunk，零字节只允许`[]↔[]` primitive，不构造ContentRef/ContentItem；保留padding/URL/raw/no-padding/whitespace/short non-final/redundant反例；
- limits/deep-copy单元：input raw 24 MiB/1024 items、Compile-derived generated<=52 MiB/output<=76 MiB，68 MiB/4 items与100 MiB/1028 items只是independent global guards；operation wire为Validate 48/48、Compare 48/48、Compile 48/144、Preview 144/48、Inspect Frame 144/48、Inspect Cache 48/48 MiB；non-content wire 4 MiB、tokens/diagnostics各边界与+1、算术溢出、全量pointer/slice/map/bytes clone表和Response no-alias；
- ContentItem/missing分支：每个Offline ContentItem Ref必须通过live Validate且`Length>0`，bytes非空并exact；零长度Ref/空bytes构造=`invalid_argument`+零Bundle。Effective-required合法Ref missing只在SDK边界映射`not_found`；optional missing复用live `content_unavailable` Residual。Required空内容没有合法非零Ref时Fail Closed，不能借空item绕过；
- cancel/deadline：strict token/chunk、candidate/content循环、sort/admission/budget/render/Inspect/store clone、seal/deep-copy/return前全部有检查点；单次不可中断工作不超过64 KiB wire/render chunk、48 KiB raw chunk或一次512-candidate live stable sort；不声明sort comparison公式界，改以对抗fixture实测comparisons/耗时和同Go版本保守防回归阈值；取消返回零Response；
- workspace/cancel：实现internal `Begin/Seal/Export/Abort/Destroy`；workspace构造后、Begin前先`defer Destroy()`且`Destroy(new)`合法，Begin成功后再`defer Abort()`；Begin失败、取消和成功路径均最终destroyed，部分Put不可达；成功后只Export深拷贝sealed snapshot；禁止`goroutine+select`外层假取消；
- renderer compatibility：streaming Stable/SemiStable/Dynamic/Rendered与旧`renderRegions`在golden fixture上逐字节、Ref、Digest相同；旧Compile/InspectFrame wrapper不承诺cancel；
- 并发：small fixture执64并发/race20；max-size fixture只执1/2/4/8并发并记录内存/耗时，不声称production SLA；
- 单元：完整DTO、typed error闭集、零值/边界、canonical digest；白盒：ephemeral workspace不逃逸、零Owner Store/CAS/Provider调用；黑盒：六入口、零正文Preview及Cache Inspect不含hit；故障：missing/drift/expiry/cancel/deadline；Conformance：零Capability/Effect/Settlement；并发：乱序/64并发确定性；
- 实现门：既有四入口独立软件验收已YES；additive Compare与Cache Inspect入口均已通过本轮target100、race20、full ordinary/race及vet。Cache Inspect还覆盖Plan/Profile expiry、exact ref/digest/Partition/Key漂移、递归duplicate key、null Plan、cancel、response tamper、usage不等于hit与64并发确定性；typed零分配成功路径预检、pre-materialization wire token bounds、context-aware canonical、conditional/null presence、request-specific limits与既有max-size 1/2/4/8实证均保持；
- `ContextOfflineIngressV1`已完成公开request seal/encode、六typed/JSON API和stdin/stdout CLI；Compare实现位于`sdk/compare.go`，Cache Inspect位于`sdk/cache_inspect.go`，其余live文件为`sdk/request_codec.go`、`offlineapi/service.go`、`cmd/context/main.go`及对应tests。它不提供Store、listener、Capability、Provider访问或写命令；publish/rollback及production API仍是未来治理Delta。

### P6.1：Context Engineering SDK V1（Owner-local SDK已完成，API/CLI本轮实施）

- 保留`ContextOfflineSDKV1`六operation闭集，新增独立typed `ContextEngineeringSDKV1`；首切面不新增transport server或production root；
- Prompt入口为`validate_prompt_asset / preview_prompt_candidates`，复用distinct PromptAsset ref与kernel唯一Candidate projector，不写Prompt Store/lifecycle；
- Evaluation入口为`prepare_context_evaluation / admit_context_evaluation / build_feedback_candidate`；Prepare核验1..64个完整Outcome exact对象，Evaluator只产Observation，Admit S2后由Context生成Evaluation Fact；
- 可插拔`ContextEvaluatorV1`允许本地规则/回放/人工Adapter；远程或model Judge必须另走受治理Effect，本阶段不实现；
- hard limits：Prompt fragments 64、Outcomes 64、nested refs 32768、Evidence 512、Diagnostics 1024、canonical 32 MiB、wire 48 MiB；preflight必须在clone/canonical/Evaluator前；
- DTO/错误/TTL/Unknown/依赖DAG以design `engineering-sdk.md`为准；任何错误零Response，所有返回deep-copy/no-alias；
- 候选文件：`contract/evaluator.go`、`ports/evaluator.go`、`kernel/evaluator.go`、`sdk/engineering*.go`、`internal/testkit/engineering.go`及分层tests；
- 已按contract→kernel→SDK/codec→API/CLI→tests顺序实现并完成target100、race20、full ordinary/race/vet；`engineeringapi/service.go`与既有`cmd/context`五个stdin/stdout命令只复用SDK/strict codec，不新增server/listener/Store/Capability/Provider/发布/root。该Owner-local软件切面已完成；远程Evaluator和production root仍需独立治理。
- Prompt Provenance切面已按用户确认的“官方Coding Agent明文优先”策略完成：Codex/Gemini/Kimi/Grok明文可进入离线候选；Claude SDK只保留preset引用，DeepSeek/MiniMax只保留模型template/profile证据，OpenCode仅作B级对照。`contract/prompt_provenance.go`、`kernel/prompt_provenance.go`、`internal/testkit/prompt_provenance.go`及unit/blackbox/fault/conformance tests已实现exact repo/commit/path/license bytes/range、transform连续性、stable/semi/dynamic closure、32MiB preflight、chunked cancel与deep-copy/no-alias。它不联网、不自动裁决license、不取得Authority/published资格；Model Invoker exact Profile ref/current reader与T3Code宿主Adapter仍是production Port Delta。

### P7：联合接线与验收

- CompiledGraph/Binding/Run Requirement映射；
- Harness私有Adapter消费exact Frame；
- Model Invoker RouteID/公开execution union与Actual Injection；
- Continuity live `TimelineOwnerFactRefV1`/Checkpoint V2 exact refs可复用，不新增Context事件DTO；等待Continuity Owner把typed Owner-current Reader/Router从`continuity/runtimeadapter`提升为公共Port或唯一无损facade后，Context才在本组件实现只读exact-current Adapter。该Adapter只回读Frame/Generation/Outcome，不写Event/Evidence/Timeline/RocksDB；
- Artifact Resolver；
- 本组件CTX-D09隔离矩阵及Application+Memory/Knowledge G6B B-cross矩阵均已执行；它们不注册Capability、不调用Harness、不推进Turn，也不构成production能力授权。

### P8：N=1 Context G6B（A/B-cross已完成，C启用NO-GO）

- 输入边界只接受G6A已验证的`settled ToolResultV2 + current V4 Inspection + verified Association`；Source基数固定`Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`；
- Memory/Knowledge经各Owner唯一public V2 Reader及Application `ContextOwnerSourceReaderV1`做S1/S2 exact current与内容读取；`memory_recall`/`knowledge_reference`只进入DynamicTail，正文每Owner聚合上限64KiB；
- Application三段`ContextTurnRefreshPortV1`与Owner Source Reader已发布；Context Owner Adapter只依赖Application公共`contract/ports`，Application与Harness不import Context实现；
- Application只编排；Context不写Tool、Runtime、Harness或Continuity事实，不构造Continuation；
- Source Turn exact ref仅来自具名Session/Turn Owner Reader，其`Ordinal=T`必须exact等于settled Tool/ExpectedCurrent `uint32` Turn；Target T+1只由Context childExecution生成。Transition proof唯一Owner是Context，Application只编排不mint，Memory/Knowledge不`+1`、不补造Session/Turn、不产生proof；
- 顺序固定为`Refresh/S1/Reservation/Admission → pending Frame/Generation seal → Context TransitionProof → Apply/S2 → atomic ApplySettlement+expected Generation current CAS → publish`；stable closure排除phase/time/self-digest，fresh observation包含phase/Checked/Expires/current projection digests；`Inspect`严格只读恢复原Attempt/proof/result；
- RefreshAttempt/Frame/Generation ID由完整canonical request、G6A链、scope/run/session、Source Turn exact ref/Ordinal T、Context childExecution Target T+1、ParentFrame/Generation、Recipe、stable source/cache identity、projection、expiry与idempotency先行派生，不包含尚未产生的proof；TransitionProof随后pending exact refs、Attempt及stable/fresh closure派生。相同输入同ID/Digest，同ID换内容Conflict；
- S1/S2复读Session/Turn/PendingAction、ToolResult全链、Assembly Generation/Binding/Activation、Authority、ParentFrame/Manifest/Generation、Source/Artifact/Cache currentness；
- Create/DomainResult/Apply/CAS lost reply只Inspect原Attempt/Frame/Generation；Unknown/cancel/deadline写前零状态、写后`waiting_inspect`；
- 父Frame不可变；StablePrefix与SemiStable逐项复用exact `ContentRef{Ref,Digest,Length}`，仅DynamicTail加入本次有界ToolResult Fragment；PrefixDigest seal完整StablePrefix，stable cache identity使用StableSourceSetDigest；
- Receipt/Observation/raw大输出不得进入Frame；大文件与旧Tool输出只允许bounded summary或exact Artifact ref/version/digest/range；Delta链有界，压缩后仅RetainedAnchorSet有效；
- Frame/Generation/Cache TTL取请求NotAfter与全部Owner-current来源、Binding/Authority、Artifact/Provider currentness严格最短值；`checked >= expires`拒绝；计算provider-neutral Cache economics与ExpectedInjectionManifest；
- 本首切面无远程Source/Provider Effect；禁止扩V4或复用Tool settlement。S2失败最多保留pending/diagnostic fact且current pointer不可见；只有原子ApplySettlement+Generation current CAS成功才形成验收候选；
- Provider Observation、Harness Actual Manifest与Context Conformance保持三Owner分层；
- Continuity只在事实落定后投影Owner refs；Context不写Timeline/RocksDB；
- `CTX-D09-R1`已按零Runtime Settlement落地，A/B-local与Memory/Knowledge B-cross fixture已完成；这些不等同于Production State Plane/root认证；
- C层production composition root接线、capability、Harness Continuation与Turn推进必须等待G6B验收、Generation current/CAS、Owner-current Readers、Production State Plane与Route exact materialization通过；
- N>1 Tool action、通用Refresh、Checkpoint及Continuity来源继续冻结；Memory/Knowledge仍只允许当前各一项projection闭集。

## 5. Effect与Conflict Domain实施清单

实现必须登记并测试：

- `context/remote-source-resolve` → `context/remote-source`；
- `context/provider-cache-query|create|write|refresh|invalidate|delete` → `context/provider-cache`；
- `context/remote-evaluation` → `context/remote-evaluation`；
- `context/recipe-release|rollback|revoke` → `context/recipe-release`。

全部使用tenant-stable conflict scope，精确资源放idempotency/payload；本地validate/compare/compile/preview/frame-inspect/cache-inspect无Effect。

## 6. RunStartRequirement与RunSettlementRequirement实施清单

- `RunStartRequirement`：Binding V2要求精确Context Manifest/Artifact/Contract及`frame-prepare/frame-inspect/frame-materialize`；
- `RunSettlementRequirement context/frame-closure`：completion phase，Context Owner authoritative/attestation evidence；
- `RunSettlementRequirement context/effect-cleanup`：termination_report phase；无Effect时由精确policy标记operation_not_required；
- ContextReference materialization capability不满足时Fail Closed或按已批准Plan Residual降级；
- V1不产生pre-run Runtime Evidence。

## 7. 技术选择

Go 1.25为默认；只依赖标准库与批准的Runtime公共合同。无Rust计划。未来热点必须重新提交带profile、基准、Go边界、FFI/进程隔离和失败语义的设计。

## 8. 回退与迁移

- 未绑定新Context capability时不影响legacy静态Prepare；
- 影子Frame优先，不直接切换生产Route；
- Current Recipe回退通过新Binding Fact，不改历史；
- Unknown Effect先Inspect/Settlement，停止使用组件不等于Cleanup；
- 迁移阶段见design/migration.md。

## 9. 完成条件

- G6A三项输入技术PASS；Application公共Port/DTO合同须在跨模块G6B开始前由其Owner发布并通过联合评审；
- design/plan通过G6B范围评审；
- Context独占A/B-local与Memory/Knowledge B-cross fixture及其测试矩阵已通过；
- 没有越界修改或Fake生产能力声明。

CTX-D09本组件A/B完成条件已经满足：已按R1完成Context独占实现、fixture隔离反例、count100、race20、full ordinary/race/vet。G6B跨模块与C层能力启用完成条件另计；启用前不得调用Harness或推进Turn。

`CTX-D10`、`CTX-D09-R1` A/B-local、Application公共Port Adapter与Memory/Knowledge B-cross fixture均已完成并验证；C层production composition root、Capability、Harness Continuation与Turn推进继续NO-GO，直至生产依赖和系统验收通过。
