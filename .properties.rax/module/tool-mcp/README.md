# Tool/MCP模块说明

Tool/MCP模块拥有Tool Descriptor/Capability/Registry、MCP生命周期、Tool Action领域事实与N=1 G6A start-or-inspect流程。它不拥有Runtime治理事实、Application工作流、Harness Continuation或Context Generation。

当前代码包含：

- `contract`：版本化合同、稳定ID、canonical digest、Tool/MCP Descriptor与Action V1/V2 typed facts；
- `registry`与`surface`：并发安全Registry、Package/Capability状态和确定性Tool Surface编译；
- Surface Registry Snapshot漂移不做原地修改：exact Snapshot重编译恢复同一Surface，新Snapshot digest产生新Surface ID/Digest；跨Owner Reconcile/Run不变性仍由Application/Assembler负责。
- `mcp`：JSON-RPC codec、initialize、Connection/Session/Snapshot生命周期与本地测试Transport；
- `mcp`官方SDK发现：直接组装`github.com/modelcontextprotocol/go-sdk v1.6.1`，只接收已初始化ClientSession，分页读取Tools/Resources/Prompts并形成typed `MCPCapabilitySnapshotV2/V3`；领域合同不复制SDK类型；
- `mcp` Discovery Material状态为`implementation_software_test_yes`：Tools/Resources/Prompts page
  完整canonical JSON与Page Command/Connection/typed Observation摘要exact绑定，并随
  Receipt/Observation在唯一owner-local物理仓原子保存；Go SDK、transport-neutral API与模块内
  CLI提供三类material/material-set双读exact deep-copy Inspect。Snapshot V3进一步把每个Page、
  Receipt、Apply、Material Set及逐项Material exact Ref纳入immutable provenance canonical，
  Repository current+1 CAS及SDK/API/CLI `snapshot-v3` exact Inspect已闭合。Resource不成为
  Tool/Context事实，Prompt不成为系统指令；production durable backend与Context消费仍未完成；
- `mcp`官方SDK调用：Runtime public V3授权下复读exact MCP command/Prepared association/Session，在actual point用fresh clock后直接执行official SDK `CallTool`；create-once admission与Protocol Receipt遵守lost-reply inspect-only；
- `mcp`官方SDK普通取消Conformance：真实Session在admission后取消Call context时Entry进入
  Unknown，同key重投只Inspect且Provider调用仍为1；它验证上游`notifications/cancelled`行为，
  不开放Cancel API、不证明原Effect未发生，也不替代Runtime Cancel治理Port；
- `mcp`官方SDK Connect协议与initialize exact门：协商版本必须属于exact Server Descriptor并不高于稳定上限，driver response必须逐字等于Session canonical InitializeResult；漂移时Entry转Unknown、保存Session Residual、重投不再Connect且不自行Close，当前正式链只声明`2025-11-25`；
- `mcp`的`2025-06-18`兼容证据只复用official Go SDK public协议类型，已覆盖
  initialize、tools/list、tools/call与不放宽stable链反例；SDK v1.6.1无public旧版本Client
  option，因此不冒充真实Session或production Transport降级证明；
- `mcp`回包安全：复用official SDK `CallToolResult` nominal，只允许Call Result合法Content与object structured content，并按Tool Descriptor ResultLimit和Receipt上限有界canonicalize；不可完整持久化时保持Unknown且二次调用只Inspect，不截断或伪造Artifact；
- Runtime V3授权签发：Runtime Owner create-once持久Prepared-domain-command Association，并在
  fresh S1/S2下复读V2 Route/七Binding/Effect/Prepared/Policy/Enforcement/Handoff/Evidence/
  Boundary完整闭包；签发器不接触Provider，Tool physical executor才是实际执行点；
- `runtimeadapter` MCP Receipt正式化：Tool exact Reader把`MCPProtocolReceiptV1`映射为Runtime-neutral historical Receipt Projection；Runtime coordinator用每Prepared Attempt专属Evidence Source、固定sequence=1复用既有Evidence/Observation Gateways，形成正式`ProviderAttemptObservationRefV2`，不重派Provider且不升级为DomainResult；
- `runtimeadapter` MCP正式Observation回读：按exact Runtime Attempt反查唯一Command，复读正式Observation与其`ProviderOperationRef`指向的immutable Receipt，输出既有Tool Owner inspection投影；ToolError只映射三态合同中的`applied`分支，不创建DomainResult/Settlement；
- `mcp`官方SDK通知：安装Tools/Resources/Prompts list-changed Handler，绑定exact initialized Session并写入owner-local pending journal；重复通知合并，successor Snapshot存在后才ack，不直接调Discovery或修改Snapshot；
- `mcp`官方SDK过程通知：安装Progress/Logging Handler，按exact initialized Session记录
  Connection/Snapshot/epoch/source-sequence及bounded canonical摘要；不保存不受限外部内容，
  不升级为ToolResult、Evidence、Timeline或执行权；同一Journal实现公共exact/page Reader，
  SDK/API/CLI只做`1..256`有界pull，不提供后台follow或production retention；
- `surface`/`sdk` Model Tool组装：exact Tool/Schema/Description坐标派生
  `ToolDefinitionMaterialRefV1`，唯一内存Repository create-once/deep-copy；SDK从exact
  Surface Current Ref开始复读，输出Model Invoker公开neutral `Tool`，不复制厂商DTO；
  `praxis.model/function-calling-v1`另以portable profile固定名称交集与strict JSON Schema
  keyword/required/additionalProperties闭集，防止neutral Validate通过后才在厂商映射失败；
- `sdk`：Go SDK V1提交Capability/Tool/Package到`submitted`、exact Inspect、按同一Registry Snapshot解析active Package→Tool→Capability完整闭包并编译Surface；Package专项SDK另提供sealed exact离线Verify、Observation/Fact/current Inspect与verification-aware强Admission，不暴露generic Registry Transition、Fetch、Provider或Runner；
- `registry`/`sdk` Tool Alias V1：同一Tool Registry维护Alias history/current与current+1 CAS；
  SDK只在exact Snapshot装配期解析Owner+Alias为exact Tool，Run不读取Alias。Package Alias、
  semver range和production Assembler/Reconcile未实现；
- `contract`/`registry`/`sdk` MCP Tool Mapping Manifest V1状态为
  `implementation_software_test_yes`：显式Mapping exact绑定Snapshot V3、Source Material、
  Capability与Tool；Mapping-aware Admission在同一Registry锁/CAS内以同一Revision推进三条Record，
  generic MCP Tool Transition不能绕过。它只到admitted，不自动active/enable或调用Provider；
- `packageverify`已直接复用Runtime public Artifact/Trust/Policy Document neutral nominal与Readers，
  用官方Sigstore Go + in-toto闭合OCI content-addressed Artifact离线Verify，维护immutable
  Observation/Fact/current，并在强Admission同一CAS复读Verification、Package current、Trust Policy
  与Artifact exact；generic Package admitted/active生产路径Fail Closed；
- Package SDK/API/CLI exact入口与官方离线key-bundle正向Conformance已实现；targeted
  ordinary×100、race×20、模块full ordinary/race/vet及Runtime Supply Chain ports门均通过。
  Fetch/Register/Install/Enable、production backend、在线Trust freshness/root继续NO-GO；
- `sdk`受治理Action V2：只接受Application Owner已经Seal的`SingleCallToolActionRequestV2`，
  转发公共start-or-inspect Port并用fresh S1/S2验证current Result；Unknown原样返回，SDK不重试
  Execute、不组装PendingAction、不调用Provider；
- `api`：transport-neutral Catalog/MCP Read V1，提供typed cursor、稳定分页/filter、空Registry与跨页Snapshot漂移拒绝；Capability/Tool/Package/Tool Alias/MCP Tool Mapping另按exact ref返回closed typed对象并执行S1/S2、Record绑定与ProjectionDigest校验；MCP Server Descriptor/process Observation、Snapshot V3及Discovery Material提供exact Inspect和有界pull-page。不预选网络协议或持久Backend；
- `cli`：可嵌入Runner V1提供`tool list|inspect`及MCP只读命令；`mcp snapshot-v3`与
  `tool inspect --kind=mcp-mapping`只接受exact Ref，`mcp process`按exact
  Connection/Epoch/Snapshot/source sequence输出有界摘要，不提供follow/Webhook或raw payload；
  只消费公共Reader，不导入Application/Harness/Model/Runtime kernel、网络或进程包；
- `sdk/cli` MCP Status：exact Connection Ref双读Lifecycle Record，输出stored状态；
  不判定执行资格、不触发Connect/Discover，旧Runner未注入Status Port时仍unsupported；
- `mcp`/`sdk` Server Descriptor Registry：唯一Tool-owned Repository维护immutable history/current，
  revision 1 create与successor expected-current CAS；SDK Register/Inspect不创建Connection、
  network/process Transport、Credential或Authority；
- `action.StoreV2`：ActionCandidate、ApplicationAttempt中立Reservation、Tool authoritative DomainResult、ApplySettlement与ToolResult的create-once/CAS/Inspect；DomainResult current lease V1上限30秒，该上限不是生产SLA；
- `SingleCallToolActionCoordinationWatermarkV1`：既有旧Flow单调水位；live版本不含PD-TM-04 BindingV2 Ref，不是P4 durable恢复根；
- `applicationadapter.SingleCallToolActionAdapterV1`：实现Application公共`SingleCallToolActionPortV1`。Model exact Projection Reader验证完整Ref、Observation digest及`Calls==1`后才创建/Inspect Watermark；按canonical key协调并发，不使用跨key全局锁；
- N=1 payload门：`SingleCallCanonicalCommandV1`强制Model `CanonicalArgumentsDigest`、PendingAction `PayloadDigest`和Candidate payload digest逐字节相等；不相等时在Watermark/Candidate/Gateway前冲突。V1不推断或承载参数Transformation；
- `applicationadapter.ToolOwnerSingleCallFlowImplV1`：production-neutral Owner状态机。Candidate/Reservation先持久，Runtime Attempt只做exact绑定；仅当V2计划与Tool V2 Adapter同时存在时CAS Provider Boundary并进入Runtime V2 Gateway，V1始终Fail Closed；
- `runtimeadapter.ControlledProviderV2`：直接消费Runtime public V2 types/ports，先验证Action Route current projection及七Binding闭包，再提交exact Request。它不持有raw Provider、ProviderTransport、Runtime kernel/fake或production root；真实actual-point current复读、统一NotAfter与原子admission属于Runtime Gateway/Runner；
- Unknown/lost reply：V2 Entry一旦创建或boundary已CAS即视为“可能已调用”，之后只按派生Entry key/原Attempt执行bounded exact Inspect，不重新Enter/dispatch。Runtime Settlement回包丢失同样只Inspect current V4 closure；Unavailable/Indeterminate不等于NotFound；
- 结果分层：typed `ToolDomainResultFactV2`→Runtime公开Settlement V4 Gateway/current Inspection/Association→`ToolApplySettlementFactV2`/settled `ToolResultV2`。Tool不写Runtime Fact，也不Build Harness Continuation；
- `runtimeadapter`：除V2 Gateway Adapter外，提供Provider Boundary和DomainResult current的Runtime-neutral只读适配器。exact映射不授Authority、Enforcement、Provider调用或Settlement权；
- 既有并发与恢复测试覆盖旧隔离Flow；不证明PD-TM-04 P4。新候选要求在Watermark前原子持久BindingV2，后续只以BindingV2 Ref恢复；P5仅派生handoff proof。

Tool G6A V2 Owner-local隔离实现第三轮独立审计最终YES（P0/P1/P2=0）；Runtime Controlled Operation Provider V2 public ports、Gateway、Tool Adapter/Owner flow及隔离测试门已闭合。该YES仅覆盖Tool隔离实现与测试，不代表系统G6A或production GO。Identity/Assembler、production composition root、生产Provider/ProviderTransport Backend、Credential、网络/RPC、持久Backend、Capability启用与G6B仍为NO-GO；Context Refresh、Harness Continuation、Turn推进和N大于1仍属于G6B/后续门禁。

迁移不采用包装升权：legacy Action/Tool/MCP Port、GovernedExecutionProvider V2、Evidence V2和Settlement V3只保留历史Inspect。Registry、Surface、Package或MCP协议变更通过新exact revision/digest/lineage及新Run切换；活跃Run不原地换面，revoked不复活，回退也不删除Receipt/Observation/DomainResult/Settlement历史。

Application-facing V2 start-or-inspect切片已完成Tool Owner内部返修并通过定向ordinary×100、race×20及Tool模块full ordinary/race/vet；持久claim、重启只Inspect、Result Store exact record、ToolResult→DomainResult→Runtime Settlement→ApplySettlement因果、deep clone、单调时钟与lost-reply权威错误均已闭合。第二次独立代码复审仍为`NO(P0=1/P1=1/P2=0)`：唯一P0属于Application跨TTL历史恢复，必须由Application新增历史Settlement closure只读能力并在`waiting_inspect`下跳过Binding/Input current、禁止Execute；Tool现有fixture-only便捷构造器在production composition前还需强类型装配门。该切片不得写成整体完成或production GO。

[SurfaceInvocationBinding V1](../../design/tool-engine/surface-invocation-binding-v1.md)已统一使用live Runtime public Assembly Current/RegistrySnapshot Go nominal：Prepared Historical/Current、Assembly与Binding中的Registry exact Ref全部为`runtimeports.RegistrySnapshotRefV1`，禁止旧Model nominal、alias/type-pun；Registry Owner仍是Authority，Model Historical仅无损carrier；BindingRef/Ack canonical/单参InspectExact保持。Assembly composite不import Harness、不建Tool echo。

C2 [ToolSurfaceManifestCurrent V1](../../design/tool-engine/tool-surface-manifest-current-v1.md)的P4-0状态为`implementation_software_test_yes`：public Ref的ID/Revision/Digest无损等于Manifest及Plan ToolSurface坐标，ProjectionDigest独立；唯一Tool-owned Repository原子维护history/current，支持revision 1 create、successor full Ref/current+1 CAS、lost-reply inspect、ABA/回退拒绝、deep clone和per-ID并发。53项关键矩阵及targeted ordinary×100、race×20、full ordinary/race、vet已通过。该YES只覆盖Tool Owner纯软件实现与测试，不代表Harness M2、完整P4或production GO。

P4-1 [SurfaceInvocationBinding V1](../../design/tool-engine/surface-invocation-binding-v1.md)状态为`implementation_software_test_yes`：public Writer/Reader/Repository、Tool Owner唯一create-once内存Repository、Binding/Ack canonical、Owner时钟共同TTL上界、双索引原子提交、lost-reply按Invocation Inspect、deep clone及64并发单winner已实现。独立软件验收确认targeted ordinary/race、full ordinary/race、vet、typed-nil与canonical冲突门全部通过。该YES只覆盖Tool Owner纯软件实现与测试，不授Provider执行权，也不代表完整四Owner接线或production GO。

Runtime ports `ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1 + RegistrySnapshotRefV1` public Go已落盘。P4-2 InputContract、CandidateV3与BindingV2 durable root Owner-local实现为`implementation_software_test_yes`：stable issuance不含fresh Owner truth，CandidateV3只接受N=1 exact Model/PendingAction payload，BindingV2在完整S1/S2后成为唯一durable恢复根；targeted ordinary×100、race×20、full ordinary/race与vet均通过。Harness M2 wiring及每个attempt/Open/Stream/continuation actual-point四读仍未闭合；system、production root/backend与能力启用继续NO-GO。Application不改，terminal Model Projection、latest/name或caller自报不能替代Binding。

官方MCP Go SDK Discovery V1、受治理Call第一切片与Connect状态为`implementation_software_test_yes`：Discovery每页通过Runtime专属`praxis.mcp/discover` Gate与official SDK actual-point，随后闭合Receipt→正式Observation/Evidence→typed DomainResult→Runtime Settlement V4→Tool ApplySettlement；只有全部applied terminal pages才可聚合`MCPCapabilitySnapshotV2`。Snapshot exact-current SDK/API及CLI `mcp snapshot`也已通过软件门。Call已在同一Runtime V3 actual-point链上穿过official SDK真实子进程stdio与loopback Streamable HTTP Server fixture，ordinary×100/race×20通过；Connect另使用独立Run/Session矩阵闭合到Connection Availability。该YES不包含Application多页/list_changed调度、`tools/call`领域结果宿主总装、production Evidence Source/Credential/Network/backend/root或SLA。

`MCPCapabilitySnapshotV2`唯一Repository维护immutable history与单一current：revision 1 create、successor full expected-current/current+1 CAS、same winner lost-reply幂等、旧revision回退/ABA拒绝及64并发单winner均通过。Capability漂移生成新revision，不原位改写历史Snapshot。

official SDK `list_changed` owner-local消费也为`implementation_software_test_yes`：真实
SDK通知、同旧Snapshot pending coalescing、successor ack、64并发、Session/TTL/clock/context
反例及Tool full ordinary/race/vet通过。它只记录Observation；通知到新Discovery Operation
的Application调度仍未实现。

official SDK Result Safety V1为`implementation_software_test_yes`：typed-nil、sampling-only
Content、非object structured content、循环/非有限JSON、超限、Provider后Unknown与零重派已通过
targeted ordinary×100、race×20和5秒Fuzz。公共Artifact Store/背压与production输出链未实现。

Go SDK/Catalog API首批及受治理Action V2状态为`implementation_software_test_yes`：SDK/API定向ordinary×100、race×20、full ordinary/race与vet通过；Action V2另完成64同canonical单effect、Unknown零重派、typed-nil、canceled context、clock rollback、TTL crossing及Result drift反例。Connect exact只读API已覆盖Intent/Receipt/Connection/DomainResult/Apply/Availability，process Observation有界pull也已闭合；该YES不包含Cancel、ToolResult/Task streaming Watch、Connect写入口、Webhook、Package供应链Verify或production服务。

模块级公共接线Conformance扫描全部production Go源：禁止generic Hookface/Slot/Phase及任意
Context mutation/network/write-fact通用Hook方法，并拒绝Harness private/internal/ports、Context
实现、Model internal和Runtime private实现包导入；`release`只允许消费Harness公开
`assemblycontract`，Tool/MCP通过公开versioned对象贡献能力。

MCP Call exact只读消费面为`implementation_software_test_yes`：transport-neutral API、Go SDK和
CLI `mcp inspect --kind call-command|call-receipt`只读取immutable Tool Owner Command/Protocol
Receipt；S1/S2 exact、typed-nil、nil/canceled context、wrong digest、deep-copy、64并发与
Conformance targeted ordinary×100/race×20通过。CLI投影删除Params Inline与Canonical Response，
只输出refs/digest/长度/状态/时间；Receipt NotFound不证明Provider未执行，不能触发重派。

首批Go性能基线（AMD 7840HS、Go 1.25.6、`-benchmem -count=3`）：MCP JSON-RPC Decode
约10.5-10.8µs/169 allocs，单Tool Surface Compile约30.5-30.9µs/232 allocs，Registry exact
Tool Resolve约151-153ns/1 alloc。当前只有基线、没有证明Go无法满足目标的Profile证据，因此
不引入Rust；后续优化优先针对JSON解码和Surface canonical分配。

Discovery Snapshot只读消费面为`implementation_software_test_yes`：Go SDK/API以exact Ref执行S1/S2 current复读，CLI提供`mcp snapshot --id --revision --digest`；typed-nil、nil/canceled context、TTL/clock、digest漂移、deep-copy和64并发通过。它不触发Discovery、不暴露raw Session或Provider。

CLI Runner V1首批状态为`implementation_software_test_yes`：`tool list|inspect`（含Tool Alias）与
`mcp status|inspect|availability|snapshot|process`均为只读入口；process只输出有界摘要，
`mcp inspect`支持Call Command/Receipt exact Ref。非法digest、typed-nil、cancel、跨页Snapshot
漂移和`tool call`/`mcp discover`等未授权写命令零输出。该YES不代表根CLI、执行命令或production装配完成。

Tool Alias V1为`implementation_software_test_yes`：stable ID绑定Owner+Alias，唯一Registry完成
revision 1 create、successor current+1 full-Ref CAS、history/current、revoke、lost-reply幂等与64
并发单revision；SDK exact Snapshot S1/S2解析active exact Tool，repoint后旧Snapshot拒绝且已编译
Surface不原地变化。targeted ordinary×100、race×20、full ordinary/race与vet通过。该YES不包含
Package Alias、runtime latest、production durable Registry或Assembler/Reconcile。

MCP Status只读切片状态为`implementation_software_test_yes`：SDK/CLI定向
ordinary×100、race×20、Tool full ordinary/race/vet及64并发读通过；S1/S2漂移、
wrong digest、typed-nil、canceled context与clock rollback拒绝。`mcp discover/connect/call`
仍未开放。

Model Tool内容物化/neutral组装状态为`implementation_software_test_yes`：Schema必须是
有界JSON object且bytes摘要exact，Description bytes摘要exact；portable V1名称只接受
OpenAI/Anthropic/Gemini live adapter交集，strict Schema缺required、开放
additionalProperties或出现厂商专用keyword均Fail Closed；64同Ref并发单winner，Surface
current/TTL、clock rollback、typed-nil、missing material、Dialect/visibility漂移也全部拒绝。
portable定向ordinary×100、race×20已通过；Route级兼容事实已有
[PD-TM-05联合候选](../../design/tool-engine/model-route-tool-compatibility-v1.md)，明确要求
Prepared historical/current/Association与actual-point exact复读，但尚未由Model Owner冻结或
实现，状态为`joint_candidate_no_go`。该YES不代表Model/Harness production注入、真实Provider
或G6B完成。

Component Release/readiness V1已形成owner-local代码候选：`release`包直接发布Assembler公共`ComponentReleaseV1`，将当前能力分为`reference_only`、`standalone`和`production`。当前G6A P4、Surface/Binding Current、Controlled Provider Adapter与official MCP SDK软件事实可支撑standalone；durable store、Credential、真实Provider transport/current、actual-point部署、完整MCP lifecycle cleanup、deployment attestation和独立Certification未闭合，因此production继续NO-GO。Factory仅为descriptor，不代表production root已注册。
