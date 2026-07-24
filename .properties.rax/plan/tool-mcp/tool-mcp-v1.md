# Tool + MCP v1详细实施计划

状态：既有Tool G6A V2 Owner-local隔离实现第三轮独立审计YES保持；Runtime ports唯一Assembly Current/RegistrySnapshot public Go nominal与Reader已落盘。Tool P4-0 C2、P4-1 SurfaceInvocationBinding与P4-2 InputContract/CandidateV3/BindingV2 Owner-local切片均为`implementation_software_test_yes`。Harness M2、system/production仍NO-GO。

## 1. 范围

### 1.1 包含

- Tool与MCP统一Semantic Capability；
- Tool Registry、Capability Discovery、Tool Surface、Package治理；
- MCP Registry、Client、Server Facade、Connection/Session、Snapshot与标准兼容；
- Action Candidate、领域Reservation、Receipt、Inspect、Result、CAS投影；
- Application-owned `SingleCallToolActionPortV1`：Tool内`SingleCallToolActionAdapterV1`实现settled PendingAction projection exact binding、owner-current、两阶段Evidence V3、typed DomainResult/Settlement V4/Apply；G6A只返回settled ToolResult/current Inspection，不构造Continuation；
- 复用Runtime Operation V3、Dispatch V4与Application Coordinator的完整外部Effect链；
- 宿主Relay与实际Runner双重门禁；
- 过程Observer、SDK、CLI命令层与transport-neutral API；
- 完整自动化测试与真实MCP/Tool系统验收。

### 1.2 不包含

- 修改Runtime、Application、Harness、Model Invoker、Context、Review、Sandbox或其他组件；
- 修改Go Workspace、根CLI、CI、全局索引、根配置或生产部署；
- 私建Intent/Permit/Review/Binding/Evidence/Settlement兼容接口；
- 生产数据库、RPC、消息队列、Credential Broker、Sandbox Provider或SLA选型；
- 厂商SDK、`model-invoker/internal`、Raw/Native Provider事件依赖；
- 把Fake、fixture或本地演示声明为生产Backend；
- Rust/FFI。当前无基准证据支持引入。
- `N > 1` Action Gateway、批量拆分、聚合执行或部分成功语义；这些保持NO-GO，需另立设计与联合评审。

## 2. 冻结执行顺序

所有外部动作必须保持：

```text
Model ToolCall Observation (exactly one)
-> Harness模型Operation已Settlement并CAS PendingAction/waiting_action
-> Harness Assembly Adapter -> Application-owned SingleCallToolActionRequestV1
-> Application SingleCall cardinality gate (N == 1) via SingleCallToolActionPortV1
-> Tool applicationadapter调用Model Owner公开exact Reader
-> 复读完整ToolCallCandidateObservationProjectionV1；验证Ref全字段/ObservationDigest/Calls == 1
-> Tool Owner create/Inspect same-canonical-command Coordination Watermark
-> PendingActionDigest + payload/capability/schema/source exact admission
-> Tool ActionCandidate CAS -> owner-current Reservation CAS
-> Runtime Intent -> Admission -> Permit -> Begin
-> Runtime Enforcement 4.1 prepare
-> Evidence V3 prepare Issue -> InspectCurrent -> Handoff
-> controlled Provider Prepare
-> Evidence V3 prepare Candidate -> Consume(独立Handoff)
-> Runtime Enforcement 4.1 execute(exact prepare receipt/prepared attempt)
-> Evidence V3 execute Issue -> InspectCurrent -> Handoff
-> Tool绑定Runtime Attempt后进入Provider前Gate
-> Runtime V2 physical executor在自身入口fresh-clock原子验证exact Provider/Prepared current/Enforcement/Handoff/Boundary/UnifiedNotAfter
-> Runtime V2直接执行物理effect；或Inspect原Attempt
-> Evidence V3 execute Candidate -> Consume(独立Handoff)
-> Tool Owner独立Inspect/current reread -> typed DomainResult Fact CAS
-> Runtime Operation Settlement V4（只绑定typed DomainResult并投影settled）
-> InspectCurrentOperationSettlementV4（Association/Guard/Projection closure）
-> Application-facing readonly InspectOperationSettlementEvidenceAssociationV4
-> Tool ApplySettlementV4 CAS -> settled ToolResultV2 + Runtime current closure
== G6A hard stop: Context/Harness/capability enable/Continuation/Turn均为零 ==
== G6B在G6A验收后消费 ==
-> Application调用ContextTurnRefreshPortV1
-> pending Context DomainResultFact -> S2
-> Context Owner local atomic ApplySettlement + Generation current CAS
-> new exact FrameRef/Digest
-> Application调用Harness公开Continuation Port
-> Harness Owner验证并CAS下一轮
```

Begin后丢回包禁止新建执行Attempt；只允许Inspect原Attempt。远端Inspect自身是关联原Effect的独立Operation。

Application重复投递不表示重新执行。Tool Adapter每次先按DTO携带的Model Projection exact Ref调用Model已公开只读Reader；完整Projection的Ref全字段、Observation digest与`Calls == 1`通过后，才按Application Request/Scope/CanonicalCommandDigest create/Inspect Owner Watermark。Reader unavailable/indeterminate、漂移或基数错误时零Tool写/零Gateway/Provider。Tool不得从DTO、PendingAction payload、event JSON或compat tool calls反推Model事实，也不持有Model publish/write Port。

其中`ActionCandidate`与Reservation只属于Tool领域事实，不是Permit或执行授权。本路径固定`OperationScopeKind=run`、`EffectKind=praxis.tool/execute`、`PolicyProfile=praxis.tool/single-call-action-v1`，Run/Session/Turn/Action/Context全部required。Evidence Candidate只携带单一Qualification，Consume独立携带Handoff；prepare/execute不得复用资格、Handoff或phase。Tool没有BuildContinuation操作。Application Port、Evidence矩阵/五维Readers、V4.1 seam、Settlement V4 current closure与只读Association Inspect是G6A实现门；Context Refresh settled Frame链、Harness入口与总装只是G6B启用门，不是G6A写码前提。

## 3. 依赖与导入规则

| 依赖 | 用途 | 导入限制 |
|---|---|---|
| `ExecutionRuntime/runtime/core` | Digest、ExecutionScope、Fence、Typed Error | 仅公共包 |
| `ExecutionRuntime/runtime/ports` | live Operation/Dispatch/Enforcement/Evidence/Settlement、boundary current Reader，以及已落盘的唯一`ModelPreDispatchAssemblyCurrent*V1/RegistrySnapshotRefV1` public Go | 仅公共包；Assembly current语义/发布Owner是Harness，Runtime ports仅中立nominal；Tool不私建echo/fallback |
| `ExecutionRuntime/application/contract`、`application/ports` | Application-owned Single Call窄Port/DTO及Domain Reservation/State Adapter | 仅`applicationadapter`和对应测试导入；Tool domain/kernel不得导入；不得依赖Application实现 |
| `ExecutionRuntime/harness/contract` | PendingAction/Continuation公共值 | Tool不导入；由Harness Assembly Adapter投影成Application DTO |
| Model Invoker公开Projection exact Reader | 复读完整`ToolCallCandidateObservationProjectionV1`；公共Reader与producer-side atomic Ensure已终审YES | 仅`applicationadapter`消费Reader；不调用Ensure、不导入实现/internal、不持有publish/write Port；Harness Identity/Assembler/wiring/root仍未闭合 |
| Context/Review/Sandbox/Continuity | 版本化Ref/Port依赖 | 不导入实现包 |

生产包禁止导入Runtime kernel/foundation/fakes、Harness kernel/fakes/internal/private ports、Application实现、Context实现和其他组件实现包。Application/Harness不import Tool实现。live当前无production composition root；G6A测试由test fixture手工注入，未来production root由宿主Owner在G6B前闭合。

依赖DAG与Slot/Phase贡献以`design/tool-engine/integration.md`为准；组件不定义公共Slot/Hook/Phase枚举。

### 3.1 Application V2 Binding Current编译门

`PD-TM-04`只规划Tool Owner的Binding Current、immutable lease、Tool Input Contract Current与CandidateV3 build closure。Application V2公共合同与Runtime Association Reader均已通过各自Owner审计；PendingAction Tool-local rev1、ActiveRoute Route-only proof及Schema/LimitPolicy current已进入第七候选。Surface Invocation桥的Tool Owner合同已按第四审返修；Assembly current不import Harness、不私建Tool echo，等待Runtime ports唯一`ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1` Delta。Tool不得引用、包装或type-pun V1类型，亦不得用latest/name/caller自报选择Surface。

实施门中Runtime唯一Assembly Current、Tool P4-0 C2、P4-1 SurfaceInvocationBinding与P4-2 Tool Owner实现均已满足。P4-2已经原子持久Tool Owner `BindingCurrentProjectionV2` durable root并早于任何Watermark；BindingV2 Ref是唯一恢复根，CandidateV3/InputContract/ClosureV2全部进入BindingV2 canonical。InputContract issuance按用户确认仅由稳定Application Request/PendingAction/Scope/Provider/Surface/Capability/Tool/Schema/requested坐标派生；Owner/Entry/Artifact/current closure只进入immutable Projection。P5仅派生handoff proof；三Owneractual-point四读与Harness M2仍属于后续跨Owner门。

existing skeleton P0-a/b/c/d、不同nominal request、nil-context前置拒绝、typed-nil、requested/TTL/clock、lost-reply/create-once、deep-copy与64并发单赢家已进入P4-2测试并通过；该证据只闭合Owner-local slice，不能替代Harness M2或system验收。

P4-2文件级实施合同已经落盘：

1. `tool_input_contract_current_v1.go`候选：Binding/Issuance Subject、三种nominal Reader、Projection Validate/ValidateCurrent/ValidateAgainst、15秒独立TTL cap；
2. `tool_input_contract_store_v1.go`候选：mandatory CreateOnce/InspectByIssuanceID/InspectExact，lost reply只走独立issuance lookup request确定性重建issuance；
3. `contract/surface_manifest_current_v1.go`与`surface/manifest_current_repository_v1.go`：公共Reader只含`InspectExact`，唯一Repository嵌Reader再加`EnsureExact`；Ref无损复用Manifest/Plan ID/Revision/Digest，ProjectionDigest独立；Capability/Tool Registry Object Reader仍单独落点；
4. `tool_input_contract_current_v1.go`：Surface Ref+ordinal+完整Entry、唯一InputSchema Kind、immutable exact Projection与Surface真实expiry；
5. `action_v3.go`/`candidate_builder_v3.go`：Model historical Source Ref、ActionCandidateV3，InputContract Ref进入Candidate canonical；CandidateV3无独立current Reader/Store；
6. `contract/binding_current_v2.go`只放Ref/中立Subject/Issuance；`applicationadapter/binding_current_v2.go`放distinct Resolve/IssuanceLookup/InspectExact request、Closure/S2Snapshot/Projection/Reader，Reader内部完成S1+S2并持久完整root；`binding_lease_store_v2.go`只存Projection；
7. P5另立派生handoff版本，只引用P4 BindingV2 root，不推迟任何P4关联，不冒充live旧Watermark已有该字段。
8. `surface_invocation_binding_v1.go`候选：Tool Invocation/Binding Ref/Fact/Ack，单Repository内两个唯一索引，窄Writer/Reader，same canonical幂等、same Invocation漂移Conflict、lost reply Inspect恢复；Prepared直接复用Model public nominal，Assembly直接嵌入Runtime ports唯一nominal。
9. C2 `ToolSurfaceManifestCurrent`与SurfaceInvocationBinding Go均已落盘并通过独立软件验收：Harness M2只import Tool contract、构造器只接Reader，所有读取路径EnsureCalls=0；Harness Model Gate未来可调用独立SurfaceInvocationBinding Writer，不得与C2 Repository混用。actual-point attempt/Open/Stream/continuation四读未复审通过前，system/production hard-block且Provider生产能力为0。

候选文件精确为`contract/{surface_manifest_current_v1.go,surface_invocation_binding_v1.go,registry_object_current_v1.go,input_contract_current_v1.go,action_v3.go,binding_current_v2.go}`、`surface/manifest_current_repository_v1.go`与`applicationadapter/{surface_invocation_binding_v1.go,registry_object_current_v1.go,tool_input_contract_current_v1.go,tool_input_contract_store_v1.go,model_source_historical_v1.go,candidate_builder_v3.go,binding_current_v2.go,binding_lease_store_v2.go}`。Harness只能import `contract`；appadapter必须引用`toolcontract` nominal，禁止复制；BindingV2必须在完整S1+S2及共同min之后、任何Watermark前原子create-once。

## 4. 规划文件树

以下路径在对应G6A/G6B技术门通过后，按当前正式开发授权和阶段边界创建：

```text
ExecutionRuntime/tool-mcp/
  go.mod
  doc.go
  README.md
  contract/
    version.go
    capability.go
    descriptor.go
    surface.go
    package.go
    action.go
    action_v2.go
    owner_current_v1.go
    outcome_v2.go
    receipt.go
    result.go
    domain_result_v2.go
    settlement_v4_closure.go
    provider_boundary_v1.go
    mcp.go
    event.go
    errors.go
    clone.go
  ports/
    registry.go
    surface.go
    action.go
    mcp.go
    provider.go
    process.go
    storage.go
  registry/
    controller.go
    validate.go
    aliases.go
    compatibility.go
    revocation.go
  surface/
    compiler.go
    dialect.go
    diff.go
    admission.go
  action/
    controller.go
    controller_v2.go
    reservation.go
    reservation_v2.go
    state.go
    inspect.go
    settlement.go
    settlement_v2.go
    current_reader.go
    domain_result_current_v1.go
  actiongateway/
    contract.go
    adapter.go
    validate.go
    inspect.go
    g6a_result.go
    evidence_v3.go
    inspection_v4.go
    settlement_v4.go
  mcp/
    protocol.go
    lifecycle.go
    client.go
    server.go
    discovery.go
    snapshot.go
    tasks.go
    cancellation.go
    limits.go
    transport/
      stdio.go
      streamable_http.go
  runner/
    runner.go
    gates.go
    concurrency.go
    cancellation.go
    output.go
  applicationadapter/
    adapter.go
    reservation.go
    domain_state.go
    coordination_watermark_v1.go
    single_call_start_or_inspect.go
    binding_current_v1.go
    binding_current_v1_test.go
    binding_lease_store_v1.go
    binding_lease_store_v1_test.go
    candidate_current_resolver_v1.go
    candidate_current_resolver_v1_test.go
    tool_input_contract_current_v1.go
    tool_input_contract_current_v1_test.go
    tool_input_contract_store_v1.go
  runtimeadapter/
    action_current_projection.go
    single_call.go
    provider_boundary_current_v1.go
    provider_boundary_current_v1_test.go
    evidence_phase.go
    settlement_v4.go
    inspect.go
  process/
    events.go
    projection.go
    watch.go
  sdk/
    client.go
    registry.go
    surface.go
    action.go
    mcp.go
    package.go
  api/
    service.go
    requests.go
    responses.go
  cli/
    commands.go
    render.go
  internal/testkit/
    facts.go
    stores.go
    provider.go
    mcpserver.go
    pd_tm_04.go
  tests/
    blackbox/
      pd_tm_04_test.go
    fault/
      pd_tm_04_test.go
    conformance/
      provider_boundary_current_v1_test.go
      pd_tm_04_test.go
    integration/
    system/
```

若联合评审调整公共Port，先更新design/plan再改变文件树。

公共候选文件`ExecutionRuntime/runtime/ports/operation_provider_boundary_current_v1.go`由Runtime Owner评审/发布，不属于Tool写入范围；Tool不得先在本模块私建同名兼容合同。

## 5. 阶段计划

### 阶段0：分阶段合同冻结

产物：只更新Tool/MCP design/plan，直到公共依赖冻结。

任务：

- [x] 审核PD-TM-01 PendingAction Action Bridge；
- [ ] 审核PD-TM-02 Tool Surface Association；
- [ ] 冻结Agent Assembler/CompiledGraph输入输出与Slot/Phase合并；
- [ ] 冻结Binding V2中Host Relay、Data Provider、Domain Adapter、Tool Engine、MCP Gateway的Manifest/Capability映射；
- [ ] 冻结namespaced StepKind/Capability/Effect kind；
- [ ] 确认ContextReference不闭合Route的Fail Closed/Residual策略；
- [ ] 确认Checkpoint与per-turn refresh接线仅由公共Owner提供；
- [x] 冻结Evidence V3 Action矩阵、Fact-kind reader router、S1/S2 current reread与Observation lease边界；
- [x] 冻结V4.1 prepare/execute Enforcement到受控Provider seam的exact公共关联；
- [x] 冻结Settlement V4的typed DomainResult与两项Evidence Binding字段，以及`OperationInspectionSettlementRefV4`的Association/Guard/Projection exact closure；Runtime不含Disposition；
- [x] 冻结Application-owned `SingleCallToolActionPortV1`及窄Request/Result DTO；Tool Adapter只依赖公共`contract/ports`，Application/Harness不import Tool实现；
- [x] 冻结`Execute` same-canonical-command start-or-inspect与Tool Owner `SingleCallToolActionCoordinationWatermarkV1`：稳定键、CanonicalCommandDigest覆盖面、单调阶段、NotFound/current恢复和Provider边界后inspect-only；
- [x] 对齐Tool-owned `ToolProviderBoundarySourceRefV1`到Runtime `OperationProviderBoundaryRefV1{ID,Revision,Digest}`无损映射、精确Reader方法与Projection字段；
- [x] Model Owner冻结并发布`ToolCallCandidateObservationProjectionV1`公开exact只读Reader；Tool消费门验证Ref全字段、Observation digest、`Calls == 1`，失败时零Watermark；
- [x] 冻结G6A只读Association Inspect验收：prepare/execute exact、独立且同Attempt；输出止于settled ToolResult/current Inspection；
- [x] 冻结Tool自有Outcome/Disposition枚举及三种合法组合；Unknown/indeterminate禁止Apply/Result；
- [x] 冻结Reservation只绑定`ApplicationAttemptRefV1 + Intent/Subject/Session`，DomainResult才绑定`OperationDispatchAttemptRefV3`并证明因果来源；
- [x] 冻结DomainResult强类型输入：`ProviderAttemptObservationRefV2`、prepare/execute各一个Enforcement Phase Ref与Evidence Consumption Ref；
- [x] 冻结current窗口：Candidate取全部来源expiry最小值；DomainResult历史truth的新签lease最大30秒，30秒非SLA；
- [x] 中央联合评审对G6A技术门给出YES；沿用当前正式开发授权，无新增用户授权Gate。
- [ ] G6B另行冻结ContextTurnRefreshPortV1输入、Context settled Frame/S2输出、Harness Assembly Adapter、Harness公开Continuation验证/CAS入口与Application恢复编排；

验收：无私有替代合同；G6A公共Owner签字即可进入隔离实现/测试；G6B未闭合不阻塞G6A，但持续阻止Provider生产能力启用、Continuation与Turn推进。

### 阶段1：Go module与纯合同

文件：`go.mod`、`contract/**`及`action/registry/surface/mcp/applicationadapter/runtimeadapter`内已实现的package-local narrow interfaces；当前无独立`process/watch`公共Port目录。

任务：

- [x] 固定已实现对象的ContractVersion和namespaced枚举；
- [x] 实现Capability/Tool/Surface/Package/MCP/Action/Receipt/Result/Event对象；
- [x] 实现严格Validate、Canonical Digest、Store/Compiler边界深拷贝隔离及大小/数量/排序上限；
- [x] 实现Effect kind、Conflict Domain、Run/Session坐标与Action/Attempt因果Relation校验；
- [x] 定义Registry/Surface/Action/MCP公共领域API与narrow current reader；
- [x] Process公共`MCPProcessObservationReadPortV1`与有界pull-page已实现；真正streaming
  Watch/Webhook/Task Watch留在阶段7后续且保持NO-GO；
- [x] 将Store/Clock/Transport/Executor保持为私有装配Port；
- [x] import-boundary测试禁止内部实现依赖。

验收：合同单元/Fuzz通过；等Revision换内容、Owner漂移、跨Tenant复用、非规范排序全部拒绝。

### 阶段2：Registry、Package与Surface

文件：`registry/**`、`surface/**`。

任务：

- [x] Descriptor/Package在submit前严格Validate，并实现submitted→admitted→active→deprecated/revoked create-once/CAS状态机；`validated`是写前门而非持久状态；
- [x] SDK在单一exact Registry Snapshot内解析active Package、每个exact Tool及其exact Capability；拒绝inactive、Snapshot漂移、依赖漂移和Package低报Tool Effect，不执行Fetch/Enable；
- [x] 按[Tool Alias装配期解析 V1](../../design/tool-engine/tool-alias-v1.md)实现唯一Registry
  history/current、current+1 CAS、SDK exact Snapshot Resolve与Surface冻结反例；Run禁止Alias Reader；
- [ ] Package fetch、在线Trust freshness/撤回与production enable外部动作；owner-local离线
  Sigstore/in-toto/OCI verify已实现，但不宣称网络fetch、production trust/backend或enable；
- [x] 保存official MCP Tools/Resources/Prompts page完整canonical JSON为typed
  `MCP*DiscoveryMaterialV1`，逐项exact绑定Page Command、Connection与对应Snapshot Observation；
  与Page Receipt/Observation原子提交，Go SDK/API/CLI exact Reader双读deep-copy读取，lost reply
  只Inspect；Resource/Prompt不被提升为Tool、Context事实或系统指令。
- [x] 增加三类`MCPDiscoveryPage*MaterialSetV1`：按exact Page Receipt返回排序唯一的typed
  Observation+Material Ref集合，绑定Command/Connection/ResponsePageDigest；SDK/API/CLI只读，
  不暴露内部Physical Entry或latest/name/URI弱查询。
- [x] 按用户裁决实现[MCPCapabilitySnapshotV3来源闭包](../../design/mcp-gateway/mcp-discovery-snapshot-v3.md)：Page/Receipt/Apply/typed Material Set及每个Tool/Resource/Prompt Material exact Ref进入V3 canonical；V2历史不重写；Repository current+1 CAS、SDK/API/CLI exact双读、lost reply/deep-copy/64并发已通过；
- [x] 按用户裁决实现版本化[MCP Tool Mapping Manifest V1](../../design/tool-engine/mcp-tool-mapping-manifest-v1.md)：Snapshot/Material S1-S2、Capability/Tool exact、三Record单RegistryRevision Admission及generic MCP transition拒绝已通过；只到admitted，不自动active/enable；
- [x] 用户确认[Tool Package Offline Verification与Admission V1](../../design/tool-engine/package-offline-verification-v1.md)业务语义：复用OCI content-addressed Artifact、Sigstore Bundle与in-toto Statement，且Verification、Package current、Trust Policy、Artifact exact必须同一CAS Admission；
- [x] Runtime public ports已落盘`PD-TM-PKG-01` neutral Artifact/Trust/Policy Document nominal与Readers；Tool直接导入公共类型且不复制共享nominal；
- [x] 用户确认`PD-TM-PKG-02` verification-aware strong Admission；不硬编码15秒Package current cap，expiry取所有真实Owner窗口与caller deadline共同min；
- [x] 已在`contract/package_verification_v1.go`、`packageverify/**`和`sdk/package_verify_v1.go`实现owner-local离线Verify、immutable Observation/Fact/current与exact Inspect；Fetch/Install仍单独NO-GO；
- [x] 已实现Package强Admission安全写口：同一CAS复读Package Registry current、Verification Fact/current、Trust Policy/Policy Document与Artifact exact；generic package admitted/active生产路径Fail Closed，旧assembly测试使用test-only投影fixture；
- [x] 已完成官方Sigstore Go + in-toto离线key-bundle正向Conformance、tamper/TTL/clock/
  lost-reply/64并发反例，以及SDK/API/CLI exact入口；Package targeted ordinary×100、
  race×20、模块full ordinary/race/vet与Runtime Supply Chain ports门均已通过；
- [ ] Surface Compiler消费公共CompiledGraph分配的Slot/Phase Ref，不创建本地枚举；
- [x] visible/allowed/pre-approved集合分离；
- [x] 稳定排序、Dialect输入与Expected Injection Digest实现；
- [x] Tool Definition Material exact Repository、neutral `modelinvoker.Tool`组装及portable
  function-tool V1实现；名称限制为OpenAI/Anthropic/Gemini live adapter交集，strict JSON
  Schema缺required、开放additionalProperties或厂商专用keyword均Fail Closed；
- [x] 已形成[PD-TM-05 Model Route Tool Compatibility V1联合候选](../../design/tool-engine/model-route-tool-compatibility-v1.md)：Prepared-scoped historical Fact、current Projection、create-once Association、Route/adapter/protocol/model、ToolControl与逐Tool typed decision已写清；候选状态为`joint_candidate_no_go`，不等于Model合同冻结；
- [ ] Model/Runtime/Harness联合冻结并实现PD-TM-05 public nominal、Route/Catalog exact-current Evidence与所有Provider Open/Stream/continuation actual-point Gate；在此之前portable compile只属owner-local表达证明，不宣称某条production Route可调用；
- [ ] Actual Injection Association与Surface Diff依赖PD-TM-02/Assembler，当前未实现；
- [x] Owner-local Surface Compiler已证明Registry Snapshot digest漂移必然生成新Surface ID/Digest，原exact Snapshot可重建原Surface且旧对象不被原地修改；
- [ ] 漂移后的Application/Assembler Reconcile与当前Run不变性仍依赖跨Owner接线，Tool不自行修改Run；
- [x] Registry revoked终态与确定性Snapshot历史读取实现；
- [ ] 撤回阻止新装配并产生Reconcile需求依赖Assembler/Application接线，当前未实现。

验收：确定性编译、Profile/Schema/Effect冲突、并发CAS、撤回与历史读取测试通过。

### 阶段3：MCP标准兼容与生命周期

文件：`mcp/**`。

任务：

- [x] 实现2025-11-25严格JSON-RPC codec、Initialize与Capability/Protocol Negotiation；
- [x] 使用官方SDK实现Tools/Resources/Prompts分页发现、规范排序与typed `MCPCapabilitySnapshotV2`；
- [x] 按[受治理Discovery Page V1](../../design/mcp-gateway/governed-discovery-page-v1.md)把每个分页请求接入独立`praxis.mcp/discover` Runtime Effect；Runtime public Page gate、Tool actual-point单页调用、Receipt→Observation/Evidence→DomainResult→Settlement V4→Apply及Snapshot聚合已通过owner-local软件门；Application多页编排与production root仍未启用；
- [x] official SDK Tools/Resources/Prompts `list_changed`通知消费、Tool-owned pending
  Observation journal、同旧Snapshot合并与successor ack实现；
- [ ] list_changed Observation到受治理新Discovery Operation的Application调度尚未实现；
- [ ] 普通取消与Task增强语义；official SDK v1.6.1当前无可复用Task public nominal，禁止
  私造字符串Task状态；
- [x] test-only official SDK普通取消Conformance：真实in-memory Session在Call admission后
  context取消，Tool Entry保持Unknown，同key重投只Inspect且Provider调用恒为1；该证据不开放
  Cancel API，也不替代Runtime Cancel矩阵/actual-point Port；
- [ ] 按[Cancel/Drain/Close V1候选](../../design/mcp-gateway/governed-cancel-drain-close-v1.md)
  联合冻结：普通Cancel使用Action五维独立Effect，Drain为Tool Owner本地准入CAS，Close使用
  Run/Session独立Effect；Runtime/Application P0未归零前所有写入口保持NO-GO；
- [ ] Runtime先补齐Discovery Page公开矩阵的通用Validate/Policy subject注册缺口，再按
  [Lifecycle Port Delta](../../design/mcp-gateway/controlled-mcp-lifecycle-port-delta-v1.md)
  冻结Cancel/Close独立Route、Authorization与physical Port；Tool不得私建兼容注册表；
- [x] official SDK Progress/Logging Handler、bounded process Observation journal、exact
  Session/Connection/Snapshot/epoch/sequence/correlation/payload digest与Inspect已实现；
  process Observation不升级为ToolResult/Evidence/Timeline/Review/Authority；
- [x] stdio与Streamable HTTP受控Adapter；只在Tool executor actual-point构造official SDK Transport，未提供raw Transport/production Credential或Network backend；
- [x] 按[Run/Session级受治理MCP Connect V1](../../design/mcp-gateway/governed-run-connect-v1.md)实现`praxis.mcp/connect`独立矩阵、Tool Transport Config/Connect Intent、Runtime additive physical authorization与official SDK actual-point；未复用单Action V2/V3或跨Run静默复用Connection；
- [x] Server Descriptor、Connection Epoch、Session隔离和Capability Snapshot生命周期/CAS/Inspect实现；
- [x] `MCPCapabilitySnapshotV2` Repository维护immutable history与单一current；revision 1 create、successor full expected-current/current+1 CAS、lost-reply winner重读、历史回退/ABA拒绝与64并发单winner通过；
- [x] Tool-owned MCP Server Descriptor唯一Repository与Go SDK Register/Inspect实现：revision 1
  create、successor full expected-current CAS、history/current exact读、lost reply与ABA/回退拒绝；
  Register不Connect、不启动进程或网络；
- [x] 严格Schema深度/节点、对象数量/大小/名称及JSON-RPC结果形状检查；
- [x] 组装官方`github.com/modelcontextprotocol/go-sdk v1.6.1`，实现注入式initialized Session、Tools/Resources/Prompts分页Discovery与`MCPCapabilitySnapshotV2`；Tool contract不导入SDK nominal；
- [x] 分页上限、cursor环、nil/duplicate对象、canonical摘要、TTL/clock与64并发反例；
- [x] 受Runtime public V3 actual-point授权的official SDK `tools/call`、Tool
  `MCPExecutionCommand`/Protocol Receipt、create-once admission与inspect-only恢复已实现；
- [x] 同一受治理Call链已穿过official SDK真实子进程stdio与loopback Streamable HTTP
  Server fixture：仍由Runtime V3 actual-point授权、exact Command/Session current复读和
  create-once admission控制，回包只形成immutable Protocol Receipt；不构成production backend；
- [x] Runtime Prepared-domain-command Association create-once Gateway、V3 authorization issuer及
  正式Receipt→Evidence→Provider Observation协调已实现并通过owner-local软件门；
- [x] Tool `MCPProviderObservationReaderV1`按exact Attempt只读关联唯一Command、正式Observation
  与immutable Receipt，并输出既有Owner inspection投影；它不调用Provider或创建DomainResult；
- [x] Connect专属Receipt→正式Observation/Evidence→`MCPConnectDomainResultFactV1`→Runtime
  Settlement V4→`MCPConnectApplySettlementFactV1`→Connection Availability闭环与Inspect-only恢复；
- [x] official SDK远端回包owner-local安全门：合法Call Result形状、Tool ResultLimit/Receipt
  双重上限、有界canonicalization、Unknown inspect-only与CLI raw输出抑制已实现；
- [ ] 完整Result Artifact/背压、宿主/Application对既有DomainResult/Settlement Owner flow的
  总装与production Transport尚未实现；
- [ ] MCP Plus放入独立experimental extension adapter，不进入标准声明；
- [x] 可选2025-06-18降级Conformance：official Go SDK public协议类型的initialize、
  tools/list、tools/call已通过Praxis严格JSON-RPC codec往返，并以反例证明不放宽正式
  `2025-11-25`链；SDK v1.6.1没有public旧版本Client option，故未宣称真实Session降级；
- [x] Connect actual point拒绝协商版本超出exact Server Descriptor范围及initialize response/Session漂移：Provider后转inspect-only Unknown、保留Session Residual、同canonical重投零Provider且不自行Close；V1正式链仍锁2025-11-25；
- [ ] Inbound MCP Server Facade仅在管理线确认后启用。

验收：官方规范Conformance、真实stdio/HTTP Server、恶意Server、断线/迟到/Task/取消测试通过。

阶段3的Discovery和owner-local physical `tools/call`不等待production root。live实现直接消费Runtime public V3与Tool `MCPExecutionCommandCurrentReaderV1`，不得从Runtime kernel私有Request复制类型，也不得把官方SDK Session暴露为绕过治理的开发者入口。

联合Port Delta已写入`design/mcp-gateway/controlled-mcp-call-port-delta-v1.md`：Runtime
Prepared-domain-command Association create-once Port、public physical authorization/execution V3、
Tool official SDK physical executor及Runtime-owned Evidence source+Observation协调owner-local门已通过。
正式Observation及其到Tool Owner inspection投影的只读join均已形成；Application/宿主对既有
DomainResult/Settlement Owner flow的总装、production Source provisioning与production能力仍保持关闭。

### 阶段4A：Tool Owner领域事实切片

文件：`contract/action_v2*.go`、`action/*_v2.go`及模块内测试。只依赖Tool自身合同与Runtime公开强类型值；不依赖Application、Harness、Context实现或Runtime Action矩阵，不新增Provider seam。

任务：

- [x] 实现`ActionCandidateV2`/`ActionReservationFactV2` create-once/CAS/Inspect；同ID同内容幂等，同ID换内容冲突；
- [x] `ActionCandidateV2.PendingActionDigest`精确等于调用方提供的已提交PendingAction `RequestDigest`坐标；第七候选把Revision限定为Tool-local wrapper revision `1`，跨Owner不证明Revision，也不挪用其他Revision；Payload、Capability、Schema、SourceCandidate仍需各自权威current闭合；
- [x] 定义`OwnerCurrentRefV1{Kind,ID,Revision,Digest,Owner,CheckedUnixNano,ExpiresUnixNano}`；Candidate固定携带PendingAction、Surface、Capability、Tool、InputSchema、SourceCandidate六项，不接受自由map/无类型数组；
- [x] Candidate current projection expiry取requested expiry与六项`OwnerCurrentRefV1.ExpiresUnixNano`最小值；缺失、过期、S1/S2漂移零写；
- [x] Reservation只绑定中立`ApplicationAttemptRefV1{ID,Revision,Digest}`、IntentDigest、DomainSubjectDigest和Session；不得包含或回填Runtime Dispatch Attempt；
- [x] Reserve校验Reservation expiry不超过Candidate最小current上界，全部拒绝路径零写；
- [x] Provider Observation仅接受`ProviderAttemptObservationRefV2`；prepare/execute各接受一个`OperationDispatchEnforcementPhaseRefV4`和一个`OperationScopeEvidenceConsumptionRefV3`；不新增字符串ReceiptRef；
- [x] 定义`RuntimeAttemptCausalityV1{ReservationRef,ApplicationAttemptRef,Operation,OperationDigest,Attempt,EffectID,EffectRevision,IntentDigest,Digest}`；新增typed `ToolDomainResultFactV2`并用该对象证明Runtime Attempt源自同Reservation/ApplicationAttempt；
- [x] DomainResult Fact作为历史truth永久可Inspect；`ToolDomainResultCurrentProjectionV1`精确绑定Fact、Causality digest、Observation、两phase Enforcement/Consumption、Owner与窗口，TTL必须`>0 && <=30s`；历史Reservation当前过期不否定late truth；
- [x] 新增`ToolOutcomeV2{succeeded,failed}`、`ToolDispositionV2{confirmed_applied,confirmed_not_applied}`与合法组合校验；Unknown/indeterminate不属于枚举且不能Apply；
- [x] 新增typed `ToolApplySettlementFactV2`、`ToolResultV2`；要求fresh Runtime V4 Settlement/Association/Guard/Projection/DomainResult refs及`OperationInspectionSettlementRefV4`，所有ref包含ID/Revision/Digest；
- [x] ToolResult关闭DomainResult、Settlement、Association、Guard、Projection、Apply并复制合法Outcome/Disposition；
- [x] 实现Tool narrow Action/DomainResult current reader；Candidate/Reservation lease只覆盖一次handoff，DomainResult 30秒上限不是生产SLA；
- [x] 明确拒绝`N > 1`、数组输入和成员聚合；不实现Batch Gateway；
- [x] 仅接受Runtime Settlement V4 exact typed refs后执行ApplySettlement CAS；ToolResult关闭DomainResult、Settlement、Association、Guard、Projection与Apply；
- [x] CAS/创建丢回包精确Inspect。

验收：unit/whitebox/blackbox/fault/conformance、ordinary100、race20与full ordinary/race/vet通过；三种合法Outcome组合通过，非法组合/Unknown零写；TTL边界、late truth、换Attempt/换Receipt/换Consumption、lost reply与32+并发单赢家全部通过。

### 阶段4B：Application公共Port Adapter

文件：`applicationadapter/**`。前置：阶段4A验收PASS且Application公开合同已联合YES。

任务：

- [x] 不以`OperationDomainStatePortV3`包装Settlement V4/Evidence V3；G6A只走Application公开V1 Port与Runtime V2/V4强类型合同；
- [x] `Execute`第一步调用Model公开exact Reader；成功后才按Tenant/Application Request ID/Revision/Scope稳定键create/Inspect Watermark，并绑定Projection/Request/CanonicalCommand digest；Application DTO只携带Projection Ref；
- [x] Reader unavailable/indeterminate、Ref任一字段或Observation digest漂移、`Calls != 1`时零Watermark/Candidate/Reservation/Provider；
- [x] 禁止从PendingAction payload、event JSON或compat tool calls反推/复制Model事实；Adapter不依赖Model实现且不持有publish/write Port；
- [x] 相同Request ID/Revision/Digest/Scope/canonical command重复投递只Inspect或从最后已提交阶段继续；换Digest/Scope/command冲突且零Provider；
- [x] crash-before-first-provider-call仅在Watermark current exact且下一阶段Owner事实权威NotFound后继续；Unavailable/Indeterminate/超时不得当NotFound；
- [x] Watermark进入Provider边界后，重投/lost reply/重启只Inspect原Runtime Attempt/Provider Observation，不再次调用Provider；不声明transport exactly-once；
- [x] CAS `provider_boundary_crossed`前复读并单调绑定同Attempt current execute `OperationDispatchEnforcementPhaseRefV4`与`OperationScopeEvidenceProviderHandoffRefV3`；Handoff Fact必须绑定同一execute phase；
- [x] V1到`runtime_attempt_bound`即停止；V2链在boundary CAS后只通过Runtime public V2 Gateway Enter一次，丢回包/Unknown按派生Entry key与原Attempt Inspect，Consumption仅在响应并经Tool独立Inspect后进入DomainResult；
- [x] boundary CAS后形成`ToolProviderBoundarySourceRefV1`；同Watermark ID/Revision换Digest冲突，SourceRef不授Authority；
- [x] N=1 `ApplySettlementV4`实现Application-owned版本化Port，并消费Application Owner发布的V4 exact association；
- [x] 所有方法绑定同一StepKind、ApplicationAttempt、Runtime Attempt、Intent、Provider、Authority、Session和Candidate exact refs；
- [x] domain状态仅prepared/observed/unknown/settled单调投影，不复制Runtime Permit状态；
- [x] Adapter只依赖Application公共`contract/ports`；G6A由test fixture手工注入，不宣称production root存在。

验收：Application Adapter Conformance、import-boundary、start-or-inspect、lost reply Inspect与换链拒绝通过；同canonical command 32+并发单Watermark/单阶段CAS，Provider边界后重投Provider调用计数不增加；阶段4B不反向改变阶段4A领域Fact。

### 阶段5：Runtime V4.1公共关联与受控Provider接线

文件：`runtimeadapter/**`；`runner/**`只在Runtime/Application发布受控Provider关联且另获真实Provider授权后实施。

任务：

- [x] 只消费Runtime Owner发布的V4.1 prepare/execute Enforcement到受控Provider seam的exact关联；
- [x] Tool `runtimeadapter`实现Runtime冻结的`InspectCurrentOperationProviderBoundaryV1(ctx, exactRef)`并按exact Ref复读Watermark；Runtime不import Tool；
- [x] Projection字段只含`ContractVersion、Ref、Operation、OperationDigest、OperationScopeDigest、Attempt、ExecuteEnforcement、ExecuteEvidenceHandoff、Stage、CheckedUnixNano、ExpiresUnixNano、Digest`；不得添加Request/SourceKind/Owner/Current；
- [x] Runtime seam自行与已知Operation/Attempt/execute refs交叉校验；NotFound/unknown/unavailable/drift/expired、sameID换digest、crossAttempt、其他三字段Ref type-pun全部零Provider；
- [x] 明确拒绝用`GovernedExecutionPortV2`/`GovernedExecutionProviderV2`、Evidence V2或V3 Settlement包装扩权；
- [x] Prepare对应V4.1 Enforcement与Evidence prepare handoff完成后才允许受控Provider Prepare；
- [x] Execute exact绑定prepare receipt/prepared attempt，并在V4.1 execute Enforcement与独立Evidence execute handoff完成后才允许Provider Execute；
- [x] Runtime已发布并实现V2公共关联；Tool `ControlledProviderV2`只依赖public `core/ports`，验证Route current/七Binding/Prepared/Boundary/Evidence/Applicability/Enforcement/Handoff后进入Gateway；V1仍返回unsupported且Provider调用数为0；
- [ ] Tool本地/远程/MCP Provider Adapter统一交付确定性；
- [ ] `InspectPrepared`/`InspectLocalAttempt`保证纯本地State Plane读取；
- [ ] Remote Inspect通过独立关联Operation；
- [ ] 并发、Timeout、Cancel、Backpressure、Artifact化和Output Limits；
- [ ] Credential只在实际点按Lease/Ref物化并及时清除；
- [ ] Cleanup/Residual不确定时保持冲突域占用。

验收：两phase双重门禁、零越权Provider调用、每phase最多一次、Unknown只Inspect、legacy包装反例与race通过。

### 阶段6A：Single Call Tool Action Adapter隔离收口

文件：`applicationadapter/single_call.go`、`actiongateway/**`及对应模块内测试。实施顺序固定为：阶段4A领域Fact/current reader完成 → Model Projection exact Reader消费门 → 阶段4B Application窄Port Adapter/Watermark → Runtime V2 actual-point公共合同/实现联合YES → Evidence V3/Enforcement/Provider/Prepared/NotAfter闭合 → Settlement V4 current Inspection与只读Association Inspect → Tool Apply。Context/Harness不是本阶段写码前置。

当前补充真值：Tool Owner内部V2持久claim/inspect-only/因果校验返修已通过ordinary×100、race×20及模块full门，但第二次独立代码复审仍为`NO(P0=1/P1=1/P2=0)`。剩余P0不在Tool Owner写面：Application必须提供历史Settlement closure只读恢复入口，使过期后的`waiting_inspect`只读闭合原attempt并完成CAS，不得重新进入current Binding/Input或Execute。在该Application additive Delta完成并联合复审前，本阶段的跨Owner/系统验收保持NO-GO；下列既有owner-local勾选不等于关闭该P0。

任务：

- [x] Tool内`SingleCallToolActionAdapterV1`实现Application-owned `SingleCallToolActionPortV1`；仅`applicationadapter`依赖Application公共`contract/ports`，Tool domain/kernel不依赖Application；
- [x] `Execute`先通过Model Projection exact Reader门，再走Watermark start-or-inspect；相同canonical command只Inspect/继续，Provider边界后只Inspect原Attempt/Observation；
- [x] Reader结果与PendingAction只做exact关联，不互相替代或反推；Watermark不授执行权且不改变四项领域合同；
- [x] V1 exact payload关联固定为Model唯一Call `CanonicalArgumentsDigest == PendingAction.PayloadDigest == Candidate.Payload.ContentDigest`；拼接攻击在Watermark前冲突且零Gateway，未来Transformation另立公共Fact/Port Delta；
- [x] Request DTO只承载Model Projection exact Ref、已settled模型Operation、Run/Session/Turn、PendingAction projection与`RequestDigest`、payload/capability/schema/source exact refs；不得承载可替代Model事实的Projection内容或导入Harness类型；
- [x] Admission校验基数精确为1及全部exact binding后生成Candidate；Reservation独立owner-current CAS，二者均不授执行权；
- [x] Action profile固定run/tool execute/single-call，五维required；各Owner narrow reader完成S1/S2；
- [x] prepare/execute分别完成V4.1 Enforcement和独立Evidence V3 Issue/current/handoff/candidate/consume；Candidate不含Handoff，不复用Qualification/Handoff/EventID/SourceSequence/ConsumptionID；
- [x] Runtime Owner冻结并实现`ControlledOperationProviderPortV2/RequestV2`：exact Provider Binding、Prepared Attempt/current proof、execute Enforcement/Handoff、Boundary与统一NotAfter；physical executor在自身入口fresh-clock原子ValidateCurrent后直接执行；
- [x] Runtime additive实现`OperationProviderReceiptRefV1/ProjectionV1/ReaderV1`与`OperationProviderReceiptObservationPortV1`；Tool runtimeadapter exact读取`MCPProtocolReceiptV1`，Runtime以每Prepared Attempt专属Evidence Source、固定sequence=1串联Append→Provider Observation，lost reply只Inspect同source key；
- [x] MCP正式Observation按exact Runtime Attempt回读唯一Command，并以
  `Observation.ProviderOperationRef`精确关联immutable Receipt；换Attempt/Prepared/Receipt/payload
  一律Conflict，ToolError只形成既有Owner inspection投影而不升级DomainResult；
- [x] Tool不以多一次Clock、锁或V1 Wrapper伪装actual-point闭合；actual-point admission由Runtime V2 Gateway/Runner拥有，V1在`runtime_attempt_bound`之后持续unsupported且Provider调用为零；
- [x] 仅使用内存/本地测试Provider和test transport；输入固定为`ProviderAttemptObservationRefV2`、prepare/execute各一项Enforcement Phase Ref与Consumption Ref，Tool Owner复读因果链后CAS typed `ToolDomainResultFactV2`；
- [x] Runtime Settlement V4精确绑定同一Attempt、typed DomainResult与两项Evidence Binding；先读current Inspection，再调用Application-facing只读`InspectOperationSettlementEvidenceAssociationV4`证明prepare/execute exact、独立且同Attempt；
- [x] Tool Owner只对三种合法Outcome/Disposition组合执行`ApplySettlementV4`；Unknown/indeterminate或`succeeded+confirmed_not_applied`零写；Result DTO严格只有settled `ToolResultV2 + OperationInspectionSettlementRefV4`；
- [x] G6A输出后硬停止：Context/Harness调用、Provider能力注册/启用、Continuation构造和Turn推进全部为零；
- [x] Prepare/Execute/DomainResult/Settlement/CAS所有lost reply按原ID/Revision/Digest Inspect；Unknown禁止重派；
- [x] Application/Harness import `tool-mcp`、Tool import Harness/Context/Application实现、或Tool承担总装均由import-boundary测试拒绝；
- [x] `N > 1`、数组、batch、custom effect Fail Closed，零状态变化、零Provider调用。

验收：G6A unit/whitebox/blackbox/fault/conformance/race/vet通过；只读Association不能闭合任一phase时零Apply/Result；所有Context/Harness/activation/Continuation/Turn计数为零。Tool始终没有`BuildContinuation*`。

### 阶段6B：宿主总装、Context Refresh与Harness推进

前置：G6A验收PASS；PD-TM-02及公共Assembler/Slot/Phase合同合入。本阶段由Application、Harness、Context及未来宿主production composition root拥有；live当前无该root，Tool只提供已验收Port实现和G6A Result。

任务：

- [ ] Harness Assembly Adapter把已settled `PendingActionV2`精确投影成Application-owned Request DTO；Harness不import Tool实现；
- [ ] 宿主Owner实现未来production composition root，构造`SingleCallToolActionAdapterV1`并注入Application-owned Port；Application绝不import `tool-mcp`；
- [ ] Tool Surface Association从Plan/Binding传到Harness/Context；
- [ ] Application把G6A Result交给`ContextTurnRefreshPortV1`；Tool不调用Context；
- [ ] Context Owner完成pending DomainResult→S2→本地原子ApplySettlement+Generation current CAS并返回new exact FrameRef/Digest；Context链不创建Runtime Settlement；
- [ ] Application仅在上述完成后调用Harness公开Port；Settlement字段取Tool Inspection.Settlement，Evidence字段取Tool Inspection.Association，Candidate Context绑定new Frame；
- [ ] Harness Owner CAS推进下一Candidate；Tool无Harness操作；
- [ ] ContextReference无法物化时Fail Closed或记录Plan允许Residual；
- [ ] Checkpoint/per-turn refresh只接公共Port，不在Tool/MCP自建Phase；
- [ ] G6B PASS与显式启用授权前，Provider生产能力、Continuation与Turn推进保持关闭。

验收：waiting_action完整闭环、错误PendingRef/Surface/Settlement/Evidence拒绝、无反向实现依赖；无new exact Frame时Continuation零写。G6B失败不撤销G6A验收，但禁止能力启用。

### 阶段7：SDK、CLI、API与过程暴露

文件：`process/**`、`sdk/**`、`api/**`、`cli/**`。

任务：

- [x] Go SDK首批覆盖Capability/Tool/Package提交到`submitted`、exact Inspect、同一Registry Snapshot内的Capability/Tool/Package完整assembly Resolve与Surface Compile；Package Resolve闭合active Package→Tool→Capability并拒绝低报Effect；SDK无Admission/Provider后门；
- [x] Go SDK受治理Action V2 Execute/Inspect只接受已Seal Application Request，复用公共
  start-or-inspect Port；fresh S1/S2、64同canonical单effect、Unknown零重派及TTL/clock/result
  drift反例通过，不组装PendingAction或暴露Provider；
- [x] Go SDK Package Verify/current/exact Inspect/强Admission，transport-neutral API exact双读与CLI `package verify --request-json`已实现；
- [ ] Go SDK后续Cancel、ToolResult/Task streaming Watch、Package Fetch与受治理MCP Connect写入口；
  process Observation有界pull已实现，Connect exact只读Inspect已由transport-neutral API/CLI闭合；
- [x] transport-neutral Catalog API完成exact Registry Snapshot、typed cursor、稳定分页、filter与S1/S2漂移拒绝；另完成Capability/Tool/Package/Tool Alias的exact
  `kind+ID+revision+digest` closed typed union Inspect、ProjectionDigest、deep-copy及64并发门；
- [ ] transport-neutral API后续CAS命令、幂等Action、流式状态和Webhook语义；
- [x] transport-neutral MCP Read API复用Owner exact Reader，支持Server Descriptor current/exact
  与bounded process Observation exact Inspect/page，并新增Connect Intent/Receipt/Connection/DomainResult/
  Apply/Availability exact Inspect；不复制nominal或选择HTTP/gRPC；
- [x] transport-neutral Package Verification Read API完成Observation/Fact/Current exact双读、
  S1/S2漂移拒绝、typed-nil与current closure校验；不选择HTTP/gRPC或暴露材料bytes；
- [x] CLI首批可嵌入Runner实现`tool list|inspect`，只消费SDK/Catalog，跨页漂移时零输出且不创建根二进制；
- [x] SDK/CLI `mcp status` exact只读入口、S1/S2漂移与并发反例；
- [x] Discovery Snapshot exact-current Go SDK/API与CLI `mcp snapshot --id --revision --digest`只读入口；S1/S2、typed-nil、TTL/clock、digest漂移、deep-copy与64并发反例通过，未暴露raw Session或Discovery写后门；
- [x] immutable `MCPExecutionCommandFactV1`与`MCPProtocolReceiptV1` exact Inspect已通过
  transport-neutral API、Go SDK及CLI `mcp inspect --kind call-command|call-receipt`暴露；
  S1/S2漂移、typed-nil、nil/canceled context、deep-copy、wrong digest、64并发及Conformance
  通过。CLI只输出Ref、digest、长度、状态与时间，不输出Params Inline或Canonical Response；
  Receipt NotFound不证明Provider未执行，也不允许重派；
- [x] Go SDK/API/CLI `mcp process`有界pull-page已实现：exact Connection/Epoch/Snapshot、
  source sequence、limit与PageDigest闭合，不启动后台follow或输出raw notification data；
- [ ] CLI后续`call/cancel/watch --follow/mcp discover/package register|revoke|fetch|install|enable`只在对应治理Port闭合后实现；`package verify`已实现为sealed exact request入口；
- [x] 当前已实现的CLI/API只调用统一Application或Tool/MCP领域Port，不直连Provider/Transport；
- [ ] `--dry-run`输出Candidate而不执行；
- [x] 普通过程Observer保持Connection/Snapshot/epoch/sequence/correlation/payload digest；Action
  权威Evidence只引用Runtime Evidence V3 Consumption，不把过程Observation包装成Evidence；
- [x] process Observation只保存有界摘要/digest与有限标量，不保存raw notification data；
- [x] 通用Hookface拒绝Conformance：全Tool/MCP production Go源禁止定义generic
  Hook/HookFace/Slot/Phase或BeforeTurn/AfterTurn/MutateContext/WriteFact/OpenNetwork接口，
  并禁止导入Harness private/internal/ports、Context实现、Model internal及Runtime
  kernel/fakes/internal；`release`可消费Harness公开`assemblycontract`，组件贡献只走已冻结
  namespaced/versioned公共对象。

验收：SDK/CLI/API无直连Runner/Provider后门；权限不足、Review缺失和Context残缺全部Fail Closed。

### 阶段8：完整测试、真实兼容与性能基线

任务：

- [x] Owner-local单元、白盒、黑盒、故障注入、Conformance、Race、Vet与既有Fuzz目标全量；
  跨Owner/system/production验证由下列独立项持续NO-GO；
- [ ] Runtime/Application/Harness/Context/Review/Sandbox联合集成；
- [x] official SDK真实子进程stdio MCP与loopback Streamable HTTP MCP fixture完成受治理
  `tools/call`验证；
- [ ] 真实本地Tool与2025-11-25 Task fixture；
- [ ] ContextReference未闭合Route反例；
- [x] Owner-local official SDK Call/Connect/Discovery已覆盖Provider无远端Inspect时的Unknown/Residual：只Inspect原physical Entry/Receipt journal，same canonical重投Provider调用总数保持1；
- [x] Owner-local Package revoke已证明旧pre-revocation Snapshot与新revoked Snapshot都不能Assembly，revoked终态不能重活；
- [x] Owner-local MCP Connection Epoch连续性、跨Run隔离、Snapshot expire/revoke/supersede、lost reply exact Inspect与64并发单CAS赢家已有白盒反例；
- [x] Owner-local Registry Snapshot漂移只生成新Surface ID/Digest，原Surface不被原地改写；
- [ ] Surface Reconcile调度与当前Run不变性仍需Application/Assembler跨Owner集成；
- [x] MCP JSON-RPC Decoder、Tool Schema、`tools/call` canonical arguments与official SDK
  Discovery pagination Fuzz目标落盘并各运行5秒；
- [x] official SDK Result Content有界canonicalization与MCP lifecycle状态机Fuzz已落盘并各运行5秒；Lifecycle本次126,210次执行、81个有效语料；
- [ ] SSE Event、Artifact Store与真实背压Fuzz仍等Process/Artifact公共合同；
- [x] 首批Go性能基线已采集：单Tool Surface Compile约30.5-30.9µs/232 allocs，Registry
  exact Tool Resolve约151-153ns/1 alloc，MCP JSON-RPC Decode约10.5-10.8µs/169 allocs
  （AMD 7840HS、Go 1.25.6、`-benchmem -count=3`）；
- [ ] 后续补批量Action、process Event吞吐、Discovery大Snapshot与Artifact输出基线；
- [ ] 只有实际Profile显示计算热点且优化Go后仍不达联合目标，才另起Rust设计；本计划不包含Rust。

验收：见第7节测试矩阵；任何未跑命令不得宣称通过。

### 阶段9：说明、迁移与收口

前置：实现和测试完成。

任务：

- [x] 生成并持续同步`ExecutionRuntime/tool-mcp/README.md`；
- [x] 持续更新`.properties.rax/module/tool-mcp/**`与`.properties.rax/memory/tool-mcp/**`；
- [x] 已记录MCP 2025-11-25、official Go SDK v1.6.1、Conformance、Residual、production Backend缺口和每批真实验证结果；
- [x] 提供legacy Action/Tool/MCP Port迁移矩阵、Surface/Package版本升级与非破坏性回退策略；无自动包装升权或活跃Run原地换面；
- [ ] 向单一集成任务提交全局索引增量，不直接修改全局文件。

本阶段的owner-local说明、迁移和验证证据持续同步；全局索引仍交由单一集成任务。

## 6. 文件级验收映射

| 文件组 | 主要验收 |
|---|---|
| `contract/*` | Validate/Digest/Clone/Fuzz/兼容/上限 |
| `registry/*` | CAS、Alias、撤回、历史可验证性 |
| `surface/*` | 稳定排序、集合分离、Dialect不扩权、Diff/Reconcile |
| `action/*` | Candidate最小expiry、Reservation/ApplicationAttempt、typed Observation/两phase refs、DomainResult历史truth与30秒current lease、Outcome/Disposition、Apply/ToolResult V4 closure；不包含Provider或Batch Gateway |
| `actiongateway/*` | G6A：Application DTO、SingleCall基数、五维applicability、两phase Evidence V3、V4.1 Enforcement、Provider零越权、Settlement V4、只读Association闭包、settled ToolResult/current Inspection；不含Context/Harness/Continuation builder |
| `mcp/*` | 2025-11-25协议、stdio/HTTP、Tasks、恶意Server |
| `applicationadapter/*` | 消费Model公开exact Reader后实现Tool Owner Watermark/start-or-inspect与Application-owned公共Port；禁止Model publish/write、Application/Model实现依赖及DTO反推事实 |
| `runtimeadapter/*` | 实现Runtime-neutral boundary current Reader、Action projection、V4.1两phaseref、Settlement Inspect；不得包装legacy |
| `runner/*` | 仅在公共受控Provider关联和真实Provider授权后：双重门禁、并发、Cancel、Backpressure、Secret边界 |
| `process/*` | source sequence、Evidence冲突、脱敏、Watch |
| `sdk/api/cli/*` | 无后门、严格请求、dry-run、分页/CAS/流式 |

## 7. 测试矩阵

| 测试层 | 范围 | 必须证明 |
|---|---|---|
| 单元 | 每个对象、校验器、摘要、Outcome组合、TTL边界、错误分类 | 无效输入在任何后端调用前拒绝；30s通过、30s+1ns拒绝 |
| 白盒 | Registry/Surface/Action/MCP/Runner内部状态 | create-once/CAS单调；late truth保留；Reservation无Runtime Attempt |
| Adapter白盒 | Watermark稳定键、CanonicalCommandDigest、阶段CAS、NotFound/current与Provider边界 | 同command重复只Inspect/继续；换Digest/Scope/command冲突；Unavailable/Indeterminate零继续；边界后零重复Provider |
| Provider boundary白盒 | 同Attempt execute Enforcement/Handoff current/exact、boundary CAS、响应后Consumption | 换Handoff/phase/Attempt/过期均零CAS/Provider；CAS成功后崩溃不重派；Consumption不预填 |
| Boundary Reader单元/Conformance | Tool SourceRef→Runtime exact Ref无损映射、方法签名、Projection字段/expiry/digest | Runtime不import Tool；无额外Request/字段；其他三字段Ref type-pun及不确定读取零Provider |
| Model Reader消费门 | Projection Ref全字段、Observation digest、Calls基数、Reader错误 | unavailable/indeterminate/drift/Calls=0或2时零Watermark/Candidate/Reservation/Provider；无publish/write依赖 |
| PD-TM-04候选 | PendingAction Tool-local Revision、Route-only ActiveRoute proof、InputContract/BindingV2 immutable lease、Surface Invocation因果桥 | 旧26项26/26与schema 8P0/3P1/1P2共12/12均有唯一测试ID；新增跨OwnerSurface桥P0单列且未闭合，未终审前不得运行Gateway |
| SurfaceInvocationBinding V1 | Model public Historical/Current原样引用、Runtime ports唯一Assembly composite、Tool-owned Ensure/internal Commit、唯一Repository双索引、六项TTL min、lost reply、Invocation epoch与race | 执行`SIB-V1-UNIT/IDEMP/CONFLICT/FAULT/READ/RACE/CONF/BOUNDARY/IMPORT`全矩阵；禁止Harness import/Tool echo/Kind转换；terminal Projection不能伪证pre-dispatch injection，Ack不授Provider执行权 |
| 黑盒 | 仅公开SDK/API/Port | 业务闭环且不暴露内部实现 |
| 故障注入 | G6A Tool链与G6B Context/Harness链分层 | Watermark/Candidate/Reservation/DomainResult/Apply lost reply只Inspect；因果ref漂移不签lease/不Apply；Provider边界后重投调用数不增加；G6A跨域调用为零 |
| Conformance | Runtime Binding/Operation、Application Domain、MCP标准、组件等级 | 声明外能力不暴露，Residual准确 |
| Race | G6A Candidate/Reservation/DomainResult/Boundary Reader/Apply；G6B Context/Harness | 32+同内容幂等；Watermark supersede与Reader竞争只返回exact current或Fail Closed；G6A零跨域调用 |
| Vet/Fuzz | 全模块、JSON/MCP/SSE/Schema/状态机 | 无panic、宽松解析、资源失控 |
| 集成 | Runtime+Application+Harness+Tool/MCP+Review+Sandbox+Context | Owner与水位精确，Action回注不绕过治理 |
| 系统 | 真实Tool/MCP Server/网络/Sandbox | 真实Effect、Unknown、Cleanup和Residual可审计 |
| 性能 | Surface编译、Discovery、Action并发、结果背压、Event吞吐 | 先形成Go基线和可复现实验，不预设SLA |

计划命令：

```bash
cd ExecutionRuntime/tool-mcp
go test -count=1 -shuffle=on ./...
go test -count=20 ./...
go test -count=100 ./action/... ./actiongateway/... ./mcp/...
go test -count=100 ./runtimeadapter/...
go test -count=20 -race ./action/... ./actiongateway/... ./mcp/...
go test -count=20 -race ./runtimeadapter/...
go test -count=1 -race -shuffle=on ./...
go vet ./...
go test -run '^TestConformance' ./...
```

系统与真实Provider测试使用显式build tag/环境Gate，禁止在普通单元测试隐式联网、产生费用或修改外部状态。

## 8. 故障注入清单

1. Reservation写成功回包丢失；
2. Admission成功回包丢失；
3. Permit Issue/Begin成功回包丢失；
4. V4.1 prepare Enforcement、Evidence prepare Issue/current/Handoff、Provider Prepare、Candidate/Consume各点丢回包；
5. V4.1 execute Enforcement、Evidence execute Issue/current/Handoff、Provider Execute、Candidate/Consume各点丢回包；
6. Evidence Handoff丢回包只复用同HandoffID+Qualification，Consume丢回包只复用同ConsumptionID+Handoff+Candidate；
7. Provider Receipt/Observation写入不确定与Tool Owner Inspect/规范化崩溃；
8. typed DomainResult CAS成功回包丢失；
9. Runtime Settlement V4/CAS成功回包丢失；
10. ApplySettlement/ToolResult CAS成功回包丢失；
11. Context Refresh/pending DomainResult/S2/local atomic ApplySettlement+Generation current CAS各点丢回包；
12. Harness Continuation CAS成功回包丢失；
13. Cancel与原Effect竞争；
14. MCP Session重启、Epoch漂移和迟到响应；
15. list_changed与当前Run Surface竞争；
16. Package撤回与Plan装配竞争；
17. ContextReference无法物化；
18. Cleanup Unknown与冲突域复用；
19. PendingActionDigest缺失/错误、同Ref换Payload/Capability/Schema/SourceCandidate；
20. Candidate/Reservation/current authorization/Evidence Qualification冒充Runtime V4.1 Enforcement；
21. Candidate内嵌Handoff，prepare/execute复用Qualification/Handoff/phase或Consume换Candidate；
22. Runtime持久化对应phase Enforcement或Evidence handoff前Provider被调用；
23. Provider Observation跳过Tool Owner Inspect/typed DomainResult或Runtime Settlement V4直接形成ToolResult；
24. Settlement V4缺typed DomainResult/任一Evidence Binding，Runtime携带Disposition，Inspection closure不exact/过期，或ToolResult缺V4 closure；
25. G6A不执行Application-facing只读Association Inspect、Association缺prepare/execute任一phase/Attempt不一致，或G6A调用Context/Harness、启用能力、构造Continuation、推进Turn；
26. Application或Harness import `tool-mcp`实现，Tool import Harness/Context/Application实现，或Tool承担composition root总装；
27. G6B PASS前启用Provider生产能力、写Continuation或推进Turn；Application跳过Context Refresh，Context未settled/S2漂移/无new Frame仍调用Harness；
28. Execute lost reply/Unknown后重新调用Provider或创建新Attempt；
29. legacy Action/Tool/MCP Port、GovernedExecutionProviderV2、Evidence V2、Settlement V3包装扩权；
30. `N > 1`、batch、custom effect拆分、选首项、部分Reservation或部分Provider调用。
31. `succeeded+confirmed_not_applied`或Unknown/indeterminate进入Apply/ToolResult；
32. Reservation包含/回填Runtime Attempt，或DomainResult Runtime Attempt不能证明源自同Reservation/ApplicationAttempt；
33. 字符串Receipt/Opaque JSON替代`ProviderAttemptObservationRefV2`、两phase Enforcement或两Consumption；
34. Candidate忽略PendingAction/来源expiry，Reservation超过最小上界；
35. DomainResult current TTL为零/超过30秒、因果复读漂移仍签发，或仅因历史Reservation过期拒绝truthful late result；
36. 把30秒V1合同上限宣称为生产SLA。
37. `Execute`跳过Watermark，或同Request ID/Revision换Digest/Scope/CanonicalCommandDigest仍继续。
38. crash-before-first-provider-call在Watermark非current、下一阶段非权威NotFound，或读取Unavailable/Indeterminate/超时时继续。
39. Provider边界后Application重投、Adapter lost reply或重启导致新Runtime Attempt/重复Provider调用。
40. 宣称transport exactly-once，或用该声明替代canonical command create-once/CAS和Unknown inspect-only。
41. 未调用Model公开exact Reader，或从Application DTO、PendingAction payload、event JSON、compat tool calls反推/复制Model事实。
42. Reader unavailable/indeterminate、Projection Ref字段/Observation digest漂移、`Calls != 1`仍写Watermark/Candidate/Reservation。
43. Tool持有Model publish/write Port、依赖Model实现或私建compat Reader。
44. boundary缺execute Enforcement/Handoff，二者换摘要/跨Attempt/非current，或Handoff Fact未绑定同一execute phase仍CAS/调用Provider。
45. boundary CAS前预填prepare/execute Consumption，或CAS成功后崩溃再次Execute。
46. Runtime seam跳过boundary Reader，或Reader NotFound/unknown/unavailable/indeterminate/drift/expired仍调用Provider。
47. Boundary Ref同ID/Revision换Digest、crossAttempt、其他三字段Ref type-pun、Projection换字段/摘要仍通过。
48. 把Boundary Projection当Authority/Fence/Permit，或用它替代execute Enforcement/Handoff。
49. 把合法Model call与不同PendingAction payload拼接，或以未定义schema transformation放宽`CanonicalArgumentsDigest == PayloadDigest`仍创建Watermark/Candidate/Runtime Entry。
50. Application G6A V2路径引用`SingleCallToolActionRequestV1`/`SingleCallToolActionInputCurrentProjectionV1`，或通过alias/wrapper/JSON重编码/type-pun冒充V2。
51. V2 Application Input Current Reader未落盘、typed-nil、Unavailable或Indeterminate时仍创建Binding Projection、Watermark、Candidate或调用Gateway。
52. 只比较三个Projection摘要，未读取完整Application V2 Input、Model Projection和Tool Registry/Surface current对象即解析Action/Capability/Tool/Provider。
53. Binding S1成功后S2漂移/过期/clock rollback仍返回current，或Provider Boundary前未按同一exact Ref再次Inspect。
54. Binding Projection被当作Authority、Review Verdict、Permit或Fence，或用它替代Runtime V2 actual-point门禁。
55. Binding lease重复Resolve/Inspect重算Checked/Expires，或same ID changed body不Conflict。
56. Candidate Payload不是inline-only/非deep-copy，或canonical bytes未闭合Model→Application Identity→PendingSubject→Candidate链。
57. Tool获得含Associate的Generation Governance Port，或Association/Generation/Provider current任一exact/active/fresh/TTL回扣缺失。
58. RequestedExpires未进入issuance identity/recovery request，Resolve未先Inspect，或并发loser拿fresh clock body和winner比较。
59. 从Identity SettlementOwner而非Runtime Route current解析Provider，Route Matrix/Generation/BindingSet水位/expiry漂移仍继续。
60. CandidateV3 builder在S2按name/latest重新Resolve来源，而非按S1保存的Surface/Registry/InputContract exact refs复读并重算同一canonical。
61. 未按fresh nowS1→Request/Subject→Application→Model→Association→Generation→Route→Provider→Tool Candidate及同refs S2顺序执行。
62. Projection顶层Subject/requested与Issuance Subject重复字段漂移，或顶层0掩盖issuance正数TTL，仍被Seal/Validate/Store/Inspect接受。
63. CandidateV3/ClosureV2缺Tool-local PendingAction Ref，或ID/RequestDigest与Application Input真实坐标不exact；Revision不为1，或伪造不存在的PendingAction Owner/current nominal。
64. ClosureDigest domain/version/discriminator/body排除规则或nil规范漂移，S2仍通过。
65. CandidateV3 builder在Route/Provider/SurfaceInvocationBinding/InputContract闭合前Seal，或S2未复读同一exact source refs即重算Candidate。
66. CandidateV3 build closure缺完整Route/ProviderCurrent/SurfaceInvocationBinding/InputContract，ExpectedOwner未按Route Provider的settlement role/component/manifest构造。
67. Tool要求Application/Harness提供不存在的PendingAction Owner/current nominal，或从Identity SettlementOwner猜ExpectedOwner而非Route Provider派生。
68. S2合法刷新Checked/Expiry却被要求与S1时间exact；或Checked回退/未用fresh now ValidateCurrent仍继续。
69. S2短窗已过仍create，S2长窗延长Candidate/lease，或最终lease遗漏任一S1/S2真实上界。
70. 从Session/Identity/SourceCandidate Revision填充PendingAction Revision，或声称Application/Harness证明Tool-local Revision=1。
71. 从Association/Generation读取不存在的ActiveRoute字段，或用fixture摘要冒充Route current自身canonical/exact证明。
72. 缺`ToolInputContractCurrentProjectionV1`仍由Candidate自报LimitPolicy/InputSchema current并继续；S2/Boundary重新Resolve而非Inspect exact Ref。
73. 同issuance并发loser因fresh Checked/Expires/Ref Digest不同而Conflict，而不是返回winner持久lease。
74. Resolve、Inspect-by-issuance、Inspect-exact使用type alias/JSON type-pun；nil context进入WithoutCancel/Reader/Store；typed-nil构造后panic或触达下游。
75. SurfaceInvocationBinding同Invocation换Prepared/Surface/Expected或ActualInjection/Profile/RegistrySnapshot/composite/内嵌Generation/Handoff/BindingSet/RequestedNotAfter仍幂等或生成第二ID。
76. Manifest ExpectedInjectionDigest未用public canonical重算或不等Model ActualToolSurfaceDigest；Provider Actual被错误强等；Prepared Historical/Current漂移，或使用Harness import/Tool Assembly echo/Kind转换/type-pun仍返回Ack。
77. Writer与Reader注入不同Repository/第二仓，private Committer落Application adapter，或by-ID与by-Invocation索引撕裂后仍返回Fact。
78. public Writer允许Harness提交完整Fact/Created/NotAfter/Digest；或Ensure create已提交但回包丢失后再次create/选择Surface，而非按原Invocation Inspect winner。
79. 缺RequestedNotAfter、过期、TTL crossing、clock rollback，或NotAfter超过Historical NotAfter、Prepared Current/Surface/composite/caller任一上界仍Ensure/Ack/current。
80. Unavailable/Indeterminate/timeout被转换为NotFound后create，或same ID换Digest未Conflict。
81. terminal Model Projection被当作pre-dispatch injection证明，缺SurfaceInvocationBinding仍进入InputContract/CandidateV3/BindingV2。
82. 任一provider attempt/Stream/Open/continuation边界未重新Inspect exact Prepared Current、Binding/Ack、Surface Current与Assembly composite，或仅凭Ack/历史Prepared Fact/裸Registry digest跨TTL或查latest调用Provider。
83. same canonical 64并发产生多Fact/Ack；同Invocation不同canonical没有单赢家Conflict；不同Invocation被全局锁串行化。
84. Tool contract导入Model/Harness私有类型，或用Go alias/JSON/反射临时兼容未冻结public nominal。
85. 同Invocation内RequestTools/Plan/Route/Profile/任一Actual digest变化后覆盖/续租/复用旧Binding，而非要求新Invocation epoch。
86. Prepared Historical/Current、Assembly或Binding仍引用旧Model Registry nominal，或用alias/type-pun/JSON重编码冒充`runtimeports.RegistrySnapshotRefV1`仍Ensure/Ack。
87. `runtimeports.RegistrySnapshotRefV1` Owner/ContractVersion/ID/Revision/Digest任一漂移，或Prepared/Assembly/Binding不是同一nominal整体exact仍进入actual-point。
88. C2 Reader method-set暴露Ensure、Harness M2构造器接Repository/concrete store，或任一M2读取路径EnsureCalls不为0。
89. `ToolSurfaceManifestCurrentRefV1`按Owner/prefix派生第二ID、猜ProjectionDigest，或不等于Plan ToolSurface ID/Revision/ManifestDigest仍Inspect成功。
90. Manifest digest正确但ProjectionDigest错误仍返回，或ProjectionDigest正确但Manifest/Ref digest错误仍按latest/另一个对象读取。
91. same Manifest.ID跨Owner产生第二lineage而非Conflict；64同key并发产生多winner。
92. nil/canceled ctx、typed-nil Reader/Store、TTL crossing、deep-clone tamper任一触达写或导致panic。
93. C2 history命中时未验证winner仍为current就返回；rev2已current后重投rev1导致回退，或successor未携ExpectedCurrent full Ref/current+1 exact CAS。
94. C2 same ID/revision/digest ABA、CAS间current漂移仍提交successor；标准context取消cause被改写为不存在的runtime/core领域类别。

## 9. 兼容、迁移与回退

| 旧面/变更 | 目标面 | 迁移规则 | 回退/失败语义 |
|---|---|---|---|
| legacy `ActionPort/ToolPort/MCPPort` | Application `SingleCallToolActionPortV2`、Runtime V2 Gateway、MCP Connect/Discovery/Call独立受治理Port | 旧Port只保留`contained_observe_only`或历史Inspect；不转译Permit/Evidence/Settlement | 新Port不可用时Fail Closed，不回落到旧Execute |
| `GovernedExecutionProviderV2`/Evidence V2/Settlement V3 | `ControlledOperationProviderPortV2`、Evidence V3、Settlement V4 | 只允许重新产生完整V3/V4 exact事实；禁止wrapper/type-pun/自由bundle | 保留旧事实历史读；不补造新Owner事实 |
| Registry Snapshot digest变化 | 新`ToolSurfaceManifest` lineage | 重新Resolve exact Package/Tool/Capability并Compile；Surface ID必须变化 | 旧Surface不原地改写；只停止新Binding，活跃Run继续持有原exact Surface |
| 同Surface lineage的租约/元数据successor | `ToolSurfaceManifestCurrent` revision+1 | 必须full `ExpectedCurrent` CAS且新revision=current+1 | 不回退current index；使用前一内容需以更高revision新建successor |
| Package Manifest/Artifact内容变更 | 新Package revision/digest及新Verification Fact | 不兼容Schema/Effect/Scope变更提升major并重新Review/Binding；Verification不回写Manifest V1 | 停止新装配，以前一exact Artifact和更高Registry revision新建候选；revoked不可原地复活 |
| MCP协议/Capability Snapshot改变 | 新Connection Epoch或Snapshot revision | 只使用显式协商且已通过Conformance的版本；Capability漂移产生successor | 未通过版本拒绝/Residual；不修改旧Session/Snapshot历史 |

通用顺序固定为：先停止新Binding/Admission -> 对未知Effect只Inspect ->
Fence/Drain已绑定资源 -> 生成新Descriptor/Package/Surface/Binding -> 只对新Run启用。
Receipt/Observation/DomainResult/Settlement与旧Manifest全部保留，不删除历史、不用破坏性数据回滚掩盖外部Effect。

## 10. 风险

| 风险 | 控制 |
|---|---|
| Application Single Call Port/DTO或Model Reader发生回归/字段漂移 | Tool exact Reader与import-boundary Fail Closed；不得私建Port，Application/Harness不得import Tool实现 |
| Evidence V3 Action矩阵/Reader回归或漂移 | 五维required、Fact-kind router、S1/S2任一不闭合时V2 Gateway零admission |
| Runtime actual-point Provider V2回归或Route current/七Binding闭包漂移 | Tool V2 Adapter逐字段Fail Closed并保持V1 unsupported；same-ID drift、alias、TTL crossing、clock rollback均零Gateway/零新admission |
| Boundary current Reader或public V2 Route发生回归/字段漂移 | Tool V2 Adapter Fail Closed；Runtime不得import Tool，Tool不得私建兼容Port |
| Settlement V4 current closure/Association Inspect回归或漂移 | 阻止Apply/Result；不得包装V3或新增Evidence Bundle |
| Outcome/Disposition被误当Runtime字段 | Tool自有枚举与三种合法组合；Runtime不选择，Unknown不Apply |
| Reservation提前绑定Runtime Attempt | 使用中立ApplicationAttempt exact坐标；Runtime Attempt只在DomainResult首次绑定并证明因果来源 |
| 弱Receipt或Opaque替代typed refs | 只接受Runtime公开Observation、两phase Enforcement与两Consumption强类型ref |
| current lease被当SLA或late truth被过期窗口抹除 | DomainResult Fact与current projection分离；Owner复读签发最大30秒租约，历史truth保留 |
| Context Refresh new exact Frame链未闭合 | 不阻止G6A；阻止G6B、能力启用、Continuation与Turn推进；pending DomainResult→S2→Context Owner本地原子ApplySettlement+Generation current CAS未闭合时Harness零写；Context链无Runtime Settlement |
| Surface/Slot/Phase未统一 | PD-TM-02和CompiledGraph为硬Gate；不自建枚举 |
| MCP标准演进 | 稳定版本冻结+协商+Conformance；draft不承诺 |
| Provider不可Inspect | non-retryable+Unknown/Residual+降低Conformance |
| 模型方言诱导扩权 | Dialect Diff强制Effect/Scope/Review不弱化 |
| 大Schema/输出/事件风暴 | 硬上限、分页、Artifact、背压和断路 |
| Secret泄漏 | Ref/Lease、实际点物化、日志/Context/Descriptor禁入 |
| 假Backend被误用 | internal/testkit隔离、Manifest/Conformance拒绝生产等级 |
| 过早引入Rust | 仅在可复现Go基准和联合目标证明后另立设计 |

## 11. 管理线仍需裁决

1. 全局Capability、StepKind和Effect kind命名空间；
2. PD-TM-01、PD-TM-02的公共Owner和文件落点；
3. Agent Assembler/CompiledGraph与Slot/Phase合并规则；
4. Package签名信任根、透明日志、市场Owner和漏洞撤回流程；
5. v1是否启用Inbound MCP Server与2025-06-18兼容窗口；
6. 首个生产State Plane、Credential Broker、Sandbox、API Transport和SLA；
7. MCP Plus独立评审时间与extension namespace。
8. G6A public contracts与Tool隔离接线已经闭合；后续只裁决G6B的Identity/Assembler、`ContextTurnRefreshPortV1` settled Frame链、Harness Assembly Adapter、公开入口与production启用顺序。
9. Runtime public V2 Port与`OperationProviderBoundaryCurrentReaderV1`已落盘，Tool V2 Adapter conformance已通过；Tool G6A V2 Owner-local第三轮独立审计最终YES（P0/P1/P2=0）。
10. C2 ToolSurfaceManifestCurrent Reader/Repository、SurfaceInvocationBinding及P4-2 InputContract/CandidateV3/BindingV2均已实现并通过Owner-local software test；Harness M2保持只读Reader、EnsureCalls=0。所有actual-point四读、Harness M2、system/production仍NO-GO。

## 12. 完成条件

- design、plan、PD-TM-01/02与Boundary Reader Delta联合评审通过；
- Tool G6A V2 Owner-local完成条件：Application Port/DTO、Model Reader、Boundary current Reader、Evidence V3/readers、V4.1/V2 Gateway关联、Settlement V4 current closure/只读Association Inspect全部闭合；Tool Adapter隔离测试已通过且权威输出止于settled ToolResult/current Inspection，第三轮独立审计最终YES（P0/P1/P2=0）；该结论不等于系统G6A或production GO；
- G6B完成条件：未来宿主production composition root注入、Harness Assembly Adapter、Context Refresh→pending DomainResult→S2→Context Owner本地原子ApplySettlement+Generation current CAS→new exact Frame、Application编排与Harness continuation全部闭合；Context链不创建Runtime Settlement。live当前无该root，在此之前Provider生产能力、Continuation与Turn推进保持关闭；
- 所有规划文件、合同、状态机和适配器实现；
- 全部测试矩阵有真实命令与结果证据；
- 至少一个真实Tool、一个stdio MCP、一个Streamable HTTP MCP完成系统验证；
- Unknown、Cancel、Cleanup、Residual和撤回路径真实可复查；
- module/memory与索引增量交付；
- 未宣称未验证的生产Backend、兼容性或SLA。
