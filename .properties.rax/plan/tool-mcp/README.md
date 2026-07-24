# Tool/MCP实施计划入口

## 状态

- 计划状态：Tool G6A V2 Owner-local隔离实现第三轮独立审计最终YES（P0/P1/P2=0）；仅覆盖Tool隔离实现与测试，不代表系统G6A或production GO。Identity/Assembler、production root/backend与G6B仍为NO-GO。
- Application G6A V2 Binding Current状态：Tool P4-0 `ToolSurfaceManifestCurrent`、P4-1 `SurfaceInvocationBinding`与P4-2 InputContract/CandidateV3/BindingV2 Owner-local实现均为`implementation_software_test_yes`。Ref/Projection、create-once、stable issuance、lost reply、deep-copy与并发门已通过；Harness M2只接Reader且EnsureCalls=0。跨Owner actual-point四读、完整P4/system/production仍NO-GO。
- Single Call Action Gateway状态：Tool `runtimeadapter.ControlledProviderV2`已直接接入Runtime public V2 Port，Owner flow已闭合Boundary→Gateway→独立Observation Inspect→DomainResult→Settlement V4→Apply/ToolResult。V1持续Fail Closed；Context Refresh、Application/Harness总装与生产能力启用仍属于G6B。
- 官方MCP Go SDK Discovery/notification、Call第一切片与Connect状态：`implementation_software_test_yes`。Discovery已按每页独立`praxis.mcp/discover`闭合Runtime Page gate、official SDK单页调用、Receipt→正式Observation/Evidence→typed DomainResult→Runtime Settlement V4→Tool ApplySettlement→Capability Snapshot，并提供exact-current SDK/API/CLI只读入口。受治理Call已穿过official SDK真实子进程stdio与loopback Streamable HTTP Server fixture；另以真实Session证明admission后context取消只形成Unknown、同key零重派。Connect链同样已闭合到Connection Availability。受治理Cancel写口、Application多页/list_changed调度、`tools/call`领域结果宿主总装与production能力仍未启用。
- MCP Discovery Material与Snapshot V3状态：`implementation_software_test_yes`。Tools/Resources/
  Prompts page完整canonical JSON已与Page Command/Connection/typed Observation摘要闭合并原子保存；
  [Snapshot V3](../../design/mcp-gateway/mcp-discovery-snapshot-v3.md)把Page/Receipt/Apply/Material Set
  及逐项Material exact Ref纳入canonical，SDK/API/CLI `snapshot-v3`只读已闭合。三类保持语义
  隔离；Resource/Prompt Context消费与production持久仓仍待后续。
- MCP Tool显式映射状态：`implementation_software_test_yes`。用户确认的
  [Mapping Manifest V1](../../design/tool-engine/mcp-tool-mapping-manifest-v1.md)已闭合Snapshot/Material
  S1-S2、Capability/Tool exact及Capability/Tool/Mapping三Record单RegistryRevision Admission；
  generic MCP Tool transition拒绝。V1只到admitted，不自动active/enable或触达Provider。
- official SDK Result Safety状态：owner-local实现已完成，合法Content/structured object、双重输出上限、有界canonicalization、Unknown inspect-only与CLI raw输出抑制通过targeted ordinary×100、race×20及5秒Fuzz。完整Artifact Store/背压、Context消费与production backend仍NO-GO。
- official SDK Connect协议范围门已补齐：协商版本必须位于exact Server Descriptor范围且不高于稳定上限；越界发生在Provider后时只形成Unknown/Residual并保留Session供未来受治理Close，同canonical重投不再次Connect。V1正式链继续锁`2025-11-25`；`2025-06-18` official public type/strict-codec降级Conformance已通过，但SDK无public旧版本Client option，未宣称真实Session降级。
- Tool Package离线Verify/强Admission为`implementation_software_test_yes`：
  Runtime public `PD-TM-PKG-01` Artifact/Trust/Policy Document neutral nominal与Readers已落盘；
  Tool复用OCI Artifact、官方Sigstore Go Bundle与in-toto Statement，强Admission在同一CAS复读
  Verification、Package current、Trust Policy与Artifact exact。SDK/API/CLI exact入口及官方离线
  key-bundle Conformance已完成；targeted ordinary×100、race×20、模块full ordinary/race/vet及
  Runtime Supply Chain ports门均通过。Fetch/Install/Enable、production Artifact/Trust backend、
  在线Trust freshness与root继续NO-GO。
- official SDK Progress/Logging Observation V1及有界读取为`implementation_software_test_yes`：只记录bounded digest与有限标量，exact绑定Connection/Snapshot/epoch/source-sequence；公共Reader、唯一Journal、Go SDK/API/CLI `mcp process`有界pull-page已闭合。空页不证明no-effect，streaming follow/Webhook/Task Watch与production backend仍未实现。
- SDK/API首批及受治理Action V2状态：`implementation_software_test_yes`。Go SDK提交Registry `submitted`事实、按exact Snapshot Resolve/Inspect/Compile，并只对已Seal Application V2 Request提供公共start-or-inspect Execute/Inspect；transport-neutral Catalog已增加Capability/Tool/Package/Tool Alias closed typed exact Inspect，只读API另覆盖Connect Intent/Receipt/Connection/DomainResult/Apply/Availability。SDK不组装PendingAction、不直调Provider；Cancel、ToolResult/Task streaming Watch、Connect写入口、Webhook和production Transport仍未实现，process Observation有界pull除外。
- Model Tool组装状态：`implementation_software_test_yes`。只复用Model Invoker公开neutral `Tool`与既有多厂商adapter；Tool Owner已闭合Schema/Description exact Material Repository、current Surface复读、64并发、TTL/clock/digest/typed-nil反例，并新增portable名称/strict Schema交集门，不新增厂商DTO或业务Tool。[PD-TM-05 Model Route Tool Compatibility联合候选](../../design/tool-engine/model-route-tool-compatibility-v1.md)已形成，但仍为`joint_candidate_no_go`：Model historical/current/Prepared Association及actual-point exact Readers尚未联合冻结，production注入与G6B仍未启用。
- Tool Alias V1状态：`implementation_software_test_yes`。Alias只在装配期按Owner+namespaced name
  解析active exact Tool；唯一Registry维护immutable history/current与current+1 full-Ref CAS，
  SDK在exact Snapshot S1/S2内返回Alias+Tool闭包。Package Alias、runtime latest与production
  Assembler/Reconcile仍NO-GO。
- Application-facing V2 start-or-inspect返修状态：Tool Owner内部定向ordinary×100（511.883s）、race×20（1186.629s）及模块full ordinary/race/vet均PASS；第二次独立代码复审仍为`NO(P0=1/P1=1/P2=0)`。P0是Application跨TTL历史恢复公共Delta，恢复必须是`waiting_inspect -> Tool exact Inspect -> Runtime historical V4 closure -> historical validation -> completion CAS`，禁止重读Binding/Input current和重Execute。Tool fixture-only构造器在production root前仍需装配门；当前不得宣称系统闭环完成。
- CLI首批状态：`implementation_software_test_yes`。可嵌入Runner实现`tool list|inspect`（含
  `tool-alias` list与alias exact Inspect）、`mcp status|inspect|availability|snapshot|process`只读命令；
  `process`仅按exact流坐标有界拉取，NotFound不等于未执行。`call/connect/discover/install`等
  写命令确定性拒绝且零输出。根CLI与production装配仍未实现。
- MCP Status只读状态：`implementation_software_test_yes`。SDK/CLI按exact Connection Ref双读Lifecycle事实；不触发Connect/Discover，不把stored状态当执行资格。
- MCP Server Descriptor Register/Inspect状态：`implementation_software_test_yes`。Tool唯一内存Repository完成revision 1 create、successor expected-current CAS、history/current exact读、lost-reply Inspect、deep clone、64并发、ABA/回退拒绝；它不创建Connection或外部Transport。
- Tool Owner V2字段已冻结：三种合法Outcome/Disposition组合；Reservation只绑定中立`ApplicationAttemptRefV1`；DomainResult使用Runtime公开Observation、两phase Enforcement/Consumption；历史Fact的Owner current短租约最大30秒且不作为SLA。
- Tool boundary只读边界：内部SourceRef无损映射Runtime `OperationProviderBoundaryRefV1{ID,Revision,Digest}`；Tool实现`InspectCurrentOperationProviderBoundaryV1(ctx, exactRef)`，Projection只证明CAS/current。
- Application `SingleCallToolActionPortV1.Execute`在Tool侧冻结为same-canonical-command start-or-inspect：Model exact Reader门通过后，Adapter才create/Inspect Tool Owner Watermark；Provider边界前只在current exact+权威NotFound后继续，Provider边界后只Inspect原Attempt/Observation，不承诺transport exactly-once。
- Model Projection消费门：Tool已接入Model公开exact Reader；只有完整`ToolCallCandidateObservationProjectionV1`的Ref全字段、Observation digest与`Calls == 1`通过后才写Watermark，失败时零Tool写/零Gateway，Tool无Model publish/write口。
- N=1 payload关联：`CanonicalArgumentsDigest == PendingAction.PayloadDigest == Candidate.Payload.ContentDigest`已冻结并实现；不等时零Watermark/Candidate/Gateway。V1不支持隐式schema transformation，未来转换需另立typed Fact/Port Delta。
- 最高业务输入：`tmp.document/Tool&MCP.md`。
- 设计输入：
  - [Tool Engine设计](../../design/tool-engine/README.md)
  - [Tool合同与状态机](../../design/tool-engine/contracts.md)
  - [公共接线与贡献](../../design/tool-engine/integration.md)
  - [Port Delta](../../design/tool-engine/port-delta.md)
  - [Tool验收](../../design/tool-engine/acceptance.md)
  - [Tool Alias装配期解析 V1](../../design/tool-engine/tool-alias-v1.md)
  - [MCP Gateway设计](../../design/mcp-gateway/README.md)
  - [MCP合同与生命周期](../../design/mcp-gateway/contracts.md)
  - [MCP验收](../../design/mcp-gateway/acceptance.md)
- 详细计划：[tool-mcp-v1.md](tool-mcp-v1.md)。

## 预期产物

G6A公共技术门已联合YES并完成Tool侧隔离实现；G6B仍按其启用门推进。本计划将产出：

1. 独立Go module `ExecutionRuntime/tool-mcp`；
2. Tool Capability/Descriptor/Surface/Package与Action/Receipt/Result合同；
3. MCP Server/Connection/Snapshot、stdio/Streamable HTTP标准适配；
   连接生命周期按[Run/Session级受治理MCP Connect V1](../../design/mcp-gateway/governed-run-connect-v1.md)实现：每个Run/Session/Server独立Connection与新Epoch，不跨Run静默复用。
4. Tool侧SingleCall Adapter、narrow Action current reader及Runtime V2受控Provider Adapter；隔离链已接入Runtime V4.1/V2 public Gateway与Settlement V4；
5. Registry、Surface Compiler、Action领域状态机、MCP生命周期与过程观察；
6. Go SDK、transport-neutral API和模块内CLI命令层；
7. 单元、白盒、黑盒、故障注入、Conformance、Race、Vet、集成和系统测试；
8. 实现完成后的module/memory说明资产。

## 分阶段Gate

### G6A：Tool Adapter隔离实现/测试Gate

- [x] Tool/MCP design与plan的live状态通过Tool G6A V2 Owner-local第三轮独立审计最终复核（P0/P1/P2=0）；
- [x] PD-TM-01 N=1 Action Gateway的Owner与公共合同冻结；
- [x] Application发布`SingleCallToolActionPortV1`及窄Request/Result DTO；Tool Adapter只依赖Application公共`contract/ports`，Application/Harness不import Tool实现；
- [x] `Execute`映射为Tool Owner `SingleCallToolActionCoordinationWatermarkV1`支撑的start-or-inspect；相同Request ID/Revision/Digest/Scope/canonical command只Inspect或继续，Unavailable/Indeterminate不等于NotFound；
- [x] Model Owner发布Projection exact只读Reader；Tool只消费Reader，不从Application DTO/PendingAction/event JSON/compat calls反推事实；Reader失败或`Calls != 1`零Watermark；
- [x] settled PendingAction projection、PendingActionDigest、owner-current、payload/capability/schema/source exact binding通过联合评审并实现；
- [x] Outcome/Disposition三种合法组合、Unknown不Apply通过联合评审并实现；
- [x] Reservation/ApplicationAttempt与DomainResult/RuntimeAttempt分层及因果证明通过联合评审并实现；
- [x] `ProviderAttemptObservationRefV2`、两phase Enforcement/Consumption强类型输入与DomainResult 30秒current lease通过联合评审并实现；
- [x] Evidence V3矩阵加入`run + praxis.tool/execute + praxis.tool/single-call-action-v1`，Run/Session/Turn/Action/Context全required，Runtime调用narrow readers并执行S1/S2；
- [x] Runtime公开V4.1 prepare/execute Enforcement到受控Provider seam的exact关联，且两phase各自独立Evidence Qualification/Handoff/Consumption；
- [x] 既有旧Flow的Tool Watermark boundary CAS可绑定execute Enforcement/Handoff；该live旧Watermark不含BindingV2 Ref，不能作为PD-TM-04 P4恢复根；
- [x] Runtime Owner发布并实现`ControlledOperationProviderPortV2/RequestV2`；physical executor在自身入口fresh-clock原子验证exact Provider/Prepared current/Enforcement/Handoff/Boundary/UnifiedNotAfter后直接执行；V1不得包装升权；
- [x] Runtime Settlement V4 Submission exact绑定typed Tool DomainResult与prepare/execute两项Evidence Binding，current Inspection及`InspectOperationSettlementEvidenceAssociationV4`返回可验证的Settlement/Association/Guard/Projection/DomainResult closure；Runtime不携带Disposition；
- [x] G6A测试断言Context/Harness调用、Provider能力启用、Continuation与Turn推进均为零；只允许内存/本地测试Provider；
- [x] 中央联合评审及Runtime V2第三轮独立审计对公共技术门给出YES；沿用当前正式开发授权，无新增用户授权Gate。

### Application G6A V2 Binding Current候选Gate

- [x] Application V2设计已冻结`SingleCallToolActionRequestV2`、`SingleCallToolActionInputCurrentProjectionV2`与`SingleCallToolActionInputCurrentReaderV2.InspectSingleCallToolActionInputCurrentV2`；
- [x] `PD-TM-04`第二短审返修：requested bound纳入Issuance Subject、Inspect-before-create/原子单赢家、issuance/exact双读法；
- [x] Provider改由Runtime Route current唯一解析，并闭合Action Matrix、Generation/BindingSet水位、Route expiry及SettlementOwner非执行语义；
- [x] 历史CandidateV2 Resolver合同已审；第七候选P4已由ActionCandidateV3 builder替代，CandidateV3无独立current Reader/Store；
- [x] 第三短审历史返修的Projection/Issuance/TTL硬门继续继承；第七候选ClosureV2改绑Tool-local PendingAction Ref与完整S1来源，不再伪造PendingActionCurrent；
- [x] 第四短审Owner时序继续继承：Route/Provider先闭合，ActionCandidateV3 ExpectedOwner只从Route Provider派生；不存在单独的PendingAction owner-current nominal；
- [x] 第五短审时间门继续继承：外部owner currents可fresh复读，InputContract/BindingV2 immutable时间不刷新，BindingV2 lease取全部S1/S2真实上界最小值；
- [x] 已提交Runtime additive `GenerationBindingAssociationCurrentReaderV1`设计Delta；Tool只接窄Reader，不获取Associate写口；
- [x] Application V2公共合同已落盘且基础compile PASS；
- [x] Application V2 Owner-local P2第四独立代码审计YES；该YES不关闭Tool PD-TM-04第七设计门；
- [x] Runtime Owner已落盘`GenerationBindingAssociationCurrentReaderV1`并通过独立代码短审YES（P0/P1/P2=0）及ordinary100/race20/full ordinary/race/vet；
- [x] 历史：Application/Tool/Runtime/Model完成`PD-TM-04`第六次独立设计短审；live字段复核已重新打开第七设计门，该历史YES不得作为继续写Go依据；
- [x] 第七候选第一轮独立只读审计记录为NO（设计P0=5/P1=2/P2=0；Go施工骨架P0=5/P1=2/P2=0），PendingAction rev1与ActiveRoute归属两项已接受；
- [x] 第二审前可实现性自检已把Surface/Capability/Tool三个source的独立时间窗、Projection最小TTL、S1 ExpectedOwner入参与InputSchemaCurrent跨Provider隔离写入候选；该自检不是YES；
- [ ] 用户确认第七设计修正候选：PendingAction Tool-local Revision=1、Route-only ActiveRoute proof、Tool Input Contract Current additive合同；
- [x] 历史记录：第七候选第二独立短审最终NO（P0=5/P1=2/P2=0），其后各轮返修结论由当前三Owner门状态覆盖；
- [x] 第二审返修补齐Registry Object Kind映射与三类digest语义、contract层中立坐标、BindingV2 distinct Resolve/IssuanceLookup/InspectExact及逐类型validators；该项只表示资产已写入，不是终审YES；
- [x] 用户已接受Surface Invocation三Owner方向；Tool Owner已冻结Invocation/Binding坐标、唯一Repository、Writer/Reader/Ack、canonical/TTL/S1与专项测试候选；Assembly不定义Tool neutral coordinate，只等待Runtime ports唯一nominal；
- [x] Runtime ports唯一Assembly Current/RegistrySnapshot public Go nominal与Reader已落盘，Tool/Harness直接共享且禁止alias/type-pun/echo；
- [x] Tool P4-0/P4-1/P4-2 Owner-local合同、Repository/Reader/Store与隔离测试闭合；该勾选不包含Harness M2 wiring或actual-point四读；
- [x] Surface Repository Current、Capability/Tool Registry Object Current、historical Model、CandidateV3、BindingV2唯一恢复根与immutable TTL已按Owner-local合同实现并通过software test；
- [x] existing skeleton P0-a/b/c/d、nominal request、nil-context、typed-nil、TTL、lost-reply、deep-copy与并发反例已落为测试并通过；
- [ ] Harness M2 wiring与attempt/Open/Stream/continuation actual-point四读闭合；未通过前system/production能力保持关闭。

### Component Release/readiness

- [x] [Component Release V1](component-release-v1.md) owner-local候选实现：Assembler公共Release、两Capability/Port/Factory、三档支持模式、exact readiness与Catalog恢复；
- [ ] durable State Plane、Credential、production transport/current、MCP cleanup、deployment attestation和独立Certification闭合；未完成前production NO-GO。
- [ ] [MCP Cancel/Drain/Close V1候选](../../design/mcp-gateway/governed-cancel-drain-close-v1.md)完成用户及Runtime/Application联合终审；当前普通Cancel/Close缺独立Runtime矩阵与actual-point Port，Drain仅为Tool本地候选，Task仍因official SDK public nominal缺失而NO-GO。
- [ ] Runtime修复Discovery Page公开矩阵未进入通用Matrix Validate/Policy subject注册闭集的既有P0，并联合终审[Lifecycle Port Delta](../../design/mcp-gateway/controlled-mcp-lifecycle-port-delta-v1.md)；未完成前Cancel/Close/Drain写入口保持关闭。

### G6B：总装、Context Refresh与能力启用Gate

- [ ] 系统G6A验收PASS；Tool Owner-local第三审YES不等于本项或production GO；
- [ ] 宿主Owner实现未来production composition root并注入Tool；live当前不存在，G6A仅由test fixture手工注入；
- [ ] Harness Assembly Adapter把settled PendingAction投影成Application DTO，Harness不import Tool实现；
- [ ] `ContextTurnRefreshPortV1`完成Tool settled输入→pending Context DomainResult→S2→Context Owner本地原子ApplySettlement+Generation current CAS→new exact Frame；Context链不创建Runtime Settlement；
- [ ] Harness发布公开Continuation验证/CAS入口，Application仅在Context S2后携带new Frame调用；Tool Port无`BuildContinuation*`；
- [ ] PD-TM-02 Tool Surface Association的Plan/Binding/Harness/Context落点冻结；
- [ ] Agent Assembler、Assembly SDK/CompiledGraph、Slot/Phase合并和Binding V2映射冻结；
- [ ] Tool Engine/MCP Gateway/Host Relay/Data Provider/Domain Adapter的精确Manifest V2角色冻结；
- [ ] 管理线确认命名空间、Package信任根、首批生产Transport/Backend范围；
- [ ] G6B联合验收与能力启用授权。

Tool领域事实、Application Adapter、Runtime V2 Tool Adapter及Owner flow已按当前授权隔离实现/测试；V1路径固定unsupported且Provider=0。G6B PASS前，production root/backend、Provider生产能力启用、Continuation与Turn推进保持NO-GO。任何阶段都不得私建兼容接口。
