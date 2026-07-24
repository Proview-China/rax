# Tool/MCP Wave 1

本模块实现 Tool 与 MCP 的领域内核：版本化合同、稳定 ID/摘要、Descriptor/Package/Registry、确定性 Tool Surface、Action V1/V2/V3 领域状态机、G6A 单调用协调水位，以及 MCP 2025-11-25 的 JSON-RPC、初始化、生命周期、Session 与 Capability Snapshot。

## 当前边界

- 领域事实只依赖 `runtime/core`、`runtime/ports` 公开值合同；Model 消费门只依赖 `model-invoker` 公开 exact Projection Reader，不持有其 publish/write 能力。
- V2 保持 `ToolDomainResultFactV2 -> Runtime Settlement V4 -> ToolApplySettlementFactV2/ToolResultV2` 分层；Tool Owner flow 只向注入的 Runtime 公共 Governance Port 提交 typed DomainResult ref，并通过 current Inspection/Association Inspect 闭合，绝不生成或写入 Runtime Settlement、Association、Guard 或 Projection。
- `ActionCandidate.PendingActionDigest` 必须等于 Harness Owner 提供的 `PendingActionV2.RequestDigest`；Controller 对同一 PendingAction ref 的 payload、capability、source candidate 投影执行 create-once 绑定。
- `ActionCandidateV2`、中立 `ApplicationAttemptRefV1` Reservation、typed Provider Observation/Enforcement/Consumption、DomainResult 30 秒上限 current lease、Apply/Result 都采用 exact typed ref 与 create-once/CAS；Unknown/indeterminate 只能 Inspect。
- `SingleCallToolActionCoordinationWatermarkV1` 只在 Model exact Reader 验证 `Calls == 1` 后创建；Provider boundary CAS 前复读同 Attempt 的 execute Enforcement/Handoff，CAS 后只 Inspect 原 Attempt。
- `applicationadapter.SingleCallToolActionAdapterV1` 实现 Application 公共 N=1 Port：Model exact Reader 成功后才创建/Inspect Watermark；按 canonical key 协调并发，不同 key 不使用全局锁；长调用后的 current 判断使用 fresh clock。
- `ToolOwnerSingleCallFlowImplV1` 是 production-neutral 的真实 start-or-inspect 协调器，串联 Candidate→Reservation→Runtime Attempt→Boundary→Provider Observation→DomainResult→Runtime Settlement→Apply→ToolResult；边界 CAS 后 Unknown/lost reply 永远 Inspect 原 Attempt，不重新调用 Provider。
- `runtimeadapter.ProviderBoundaryCurrentAdapterV1` 只实现 Runtime-neutral `OperationProviderBoundaryCurrentReaderV1`，把 Tool Watermark ID/Revision/Digest 无损映射为 Runtime Ref；`DomainResultCurrentAdapterV1` 只映射 Tool authoritative fact/current lease 到 Runtime V4 current Projection；两者均不授予执行或 Settlement 权限。
- MCP Connection 注册和 Capability Snapshot 绑定都必须携带 current time，并拒绝尚未生效或已经过期的对象。
- MCP Snapshot 的 `expired/revoked/superseded` 只通过精确 revision+digest CAS形成；终态不原位替换Snapshot内容。
- `MCPCapabilitySnapshotV2` Repository保留immutable history与单一current；revision 1 create，
  successor只接受full expected-current且必须`current+1`，lost reply重读winner，旧revision与ABA
  不得回退current。
- 官方 `github.com/modelcontextprotocol/go-sdk v1.6.1` 仅在 `mcp` adapter层组装：调用方注入已初始化`ClientSession`后，Adapter分页发现Tools/Resources/Prompts，规范排序并Seal `MCPCapabilitySnapshotV2`；Tool领域合同不导入官方SDK nominal。
- 官方SDK旧注入式发现不创建网络或进程，不导出raw Connect/Transport构造器；owner-local
  `tools/call`只接受Runtime public V3 actual-point授权，并复读Tool canonical command、
  Prepared association与exact initialized Session。回包仅为`MCPProtocolReceiptV1`，
  Runtime V3授权签发器会复读完整V2治理闭包与Prepared-domain-command Association；
  exact Receipt已可经Runtime Evidence/Observation Gateway形成正式Provider Observation；
  `runtimeadapter.MCPProviderObservationReaderV1`再以exact Attempt只读关联原Command、正式
  Observation与immutable Receipt，输出既有Tool Owner inspection投影。它不创建DomainResult、
  Settlement或Provider调用。同一受治理Call已在测试装配中分别穿过official SDK真实子进程
  stdio与loopback Streamable HTTP Server；协议Transport证据不等于production Network/backend。
  宿主/Application对既有Owner flow的总装与production root尚未闭合。
- 官方SDK受治理Connect V1已组装stdio与Streamable HTTP：Tool先持久Connect Intent，Runtime
  使用独立`praxis.mcp/connect` Run/Session矩阵签发actual-point authorization，Tool executor
  在fresh current复读与create-once admission后直接调用official SDK；Receipt随后经正式
  Observation/Evidence、Tool authoritative DomainResult、Runtime Settlement V4和Tool
  ApplySettlement形成Connection Availability。lost reply只Inspect原Entry/Attempt，且没有
  production Credential、Network policy materializer、durable backend或composition root。
- Connect协商协议必须落在exact Server Descriptor范围并不高于稳定上限；Provider已连接后发现
  越界，或driver记录bytes与同一Session canonical InitializeResult漂移时，不封存Receipt/Connection；
  Entry保持inspect-only Unknown并保留Session残留供未来受治理Close，同canonical重投不再次
  Connect，Connect Operation自身也不自动Close。V1正式链当前只声明`2025-11-25`。
- `2025-06-18`另有official Go SDK public协议类型的initialize、tools/list、tools/call
  strict-codec降级Conformance；SDK v1.6.1未公开旧版本Client option，因此没有通过私有字段、
  反射或vendor internal伪造真实Session降级，正式链保持不变。
- 官方SDK受治理Discovery Page V1按每个Tools/Resources/Prompts page独立执行
  `praxis.mcp/discover`：Runtime专属Page authorization后，Tool actual-point复读Connection
  Availability与Session并只调用一次官方SDK；Receipt经正式Observation/Evidence、typed
  DomainResult、Runtime Settlement V4与Tool ApplySettlement后才可推进cursor。Tool只聚合全部
  applied terminal pages生成`MCPCapabilitySnapshotV2`，Unknown/lost reply只Inspect原page。
- Tools/Resources/Prompts page分别把official SDK返回的完整canonical对象保存为typed
  `MCP*DiscoveryMaterialV1`：exact绑定Page Command、Connection和对应Snapshot Observation摘要，
  并与Page Receipt/Observation在同一内存物理仓原子提交；exact Reader只deep-copy读取。
  Resource不变成Tool/Context事实，Prompt不变成系统指令；全部材料都不授Review、Authority、
  Registry Admission或执行权。
- 三类`MCPDiscoveryPage*MaterialSetV1`按exact Page Receipt返回排序唯一的typed
  `{Observation, Material Ref}`集合，并绑定Command/Connection/ResponsePageDigest；SDK/API/CLI
  `*-material[-set]`只做双读Inspect，不提供latest/name/URI查询或Snapshot索引。
- `MCPCapabilitySnapshotV3`不改写V2历史canonical，直接把每个Discovery Page的Command、
  Receipt、Apply、typed Material Set及逐项Tool/Resource/Prompt Material exact Ref纳入provenance；
  V3 Repository执行revision 1/current+1 full-Ref CAS，SDK/API/CLI `snapshot-v3`只做exact双读。
- `MCPToolMappingManifestV1`是MCP Tool进入Tool Registry的唯一显式Mapping：exact绑定Snapshot V3、
  Source Material、Capability、Tool与versioned policy；Mapping-aware Admission在一个Registry
  lock/CAS内以同一successor Revision推进Capability/Tool/Mapping三条Record。generic Transition
  对MCP Tool/Mapping fail closed，Admission不自动active/enable或授Provider执行权。
- official SDK `list_changed` Bridge只安装Tools/Resources/Prompts Handler并记录exact
  Session/Connection/旧Snapshot Observation；同旧Snapshot通知合并，successor Snapshot
  存在后才ack。它不自动调用Discovery或改写Snapshot。
- official SDK `progress`/`logging` Bridge只记录有界process Observation：保存exact
  Connection/Snapshot/epoch/source-sequence、correlation与payload digest及有限标量，不保存
  不受限原文；它不产生ToolResult、Evidence、Timeline、Review或执行授权。SDK v1.6.1没有
  可复用Task public nominal，因此Task未被本模块私造。
- process Observation另提供公共exact Reader与`1..256`有界pull-page：按Connection full
  digest/Epoch/Snapshot和exclusive source sequence读取，Go SDK/API/CLI `mcp process`只输出
  摘要/digest。空页不证明no-effect；没有后台follow、Webhook或production durable backend。
- official SDK `tools/call`回包使用有界canonicalization：只允许Call Result合法Content和
  object structured content，落盘字节上限取Tool Descriptor ResultLimit与Protocol Receipt
  上限最小值；无法完整持久化时Entry保持Unknown，重复请求只Inspect且不截断/伪造Artifact。
- SDK/CLI `mcp status`只按exact Connection Ref读取Lifecycle Manager的持久Record，
  做S1/S2一致性与Owner clock检查；它不触发Connect/Discover，也不把状态授予Provider。
- SDK `RegisterMCPServerV1`只写Tool-owned immutable Descriptor Repository：revision 1 create、
  successor full expected-current CAS、history/current exact Inspect、lost reply Inspect与ABA/回退
  拒绝均已闭合；登记Descriptor不创建Connection、进程、网络或执行权限。
- Model Tool内容物化与neutral组装为`implementation_software_test_yes`：Tool Owner
  用exact Tool/Schema/Description摘要维护create-once Material Repository，按current
  Surface复读并直接产出Model Invoker公开`modelinvoker.Tool`；OpenAI、Anthropic、
  Gemini、Qwen等协议表达继续由Model Invoker既有adapter负责，Tool不复制厂商DTO。
- Reserve或Snapshot生命周期回包不确定时，使用精确ID/digest的只读Inspect恢复，不创建新Attempt或新事实。
- 不实现 Application Coordinator/kernel/DTO、Harness Action Bridge、Context Refresh、Binding/Activation 或生产 composition root；Application Adapter 只依赖 Application 公共 contract/ports，并由测试 fixture 手工注入。
- 不提供生产网络、Credential、持久化 Backend、真实 pre-run connect/discover/package 外部动作。
- 不提供生产 Provider、Capability 启用、Backend 或 SLA；受控 Provider、Runtime Settlement gateway 与各 current Reader 都是注入 seam，G6A 仅由内存 Store 与测试 fixture 手工装配。
- `mcp.LocalTransport` 仅用于本模块测试；它不是生产 Transport 或 SLA 声明。
- 默认 Go 1.25；当前没有引入 Rust 的基准依据。

## 包

- `contract`：稳定合同、验证、Seal 与摘要。
- `registry`：并发安全的内存 Registry 与 CAS 状态转换。
- `registry`另维护Tool Alias immutable history/current：revision 1 create、successor expected-current
  full Ref/current+1 CAS，Target只允许active exact Tool；Alias只供装配期SDK解析。
- `surface`：确定性 Tool Surface 编译器、Tool Definition Material Repository与neutral Model Tool组装。
- `action`：Candidate、Reservation、DomainResult、Settlement 投影状态机。
- `applicationadapter`：Application N=1 公共 Port、按 key 协调、真实 Tool Owner start-or-inspect flow 与本地 Result reference store。
- `runtimeadapter`：Tool-owned Provider boundary/DomainResult source 到 Runtime-neutral current Projection 的只读适配器。
- `mcp`：严格 JSON-RPC codec、初始化协商、连接/Session/Snapshot状态机、本地测试Transport、受治理official SDK分页Discovery、受Runtime独立Connect门治理的stdio/Streamable HTTP executor、Connect/Page Receipt/DomainResult/Apply/Availability、Capability Snapshot聚合、list-changed与process Observation journal/Bridge，以及Runtime V3授权下的create-once physical `tools/call` executor。
- `packageverify`：Owner-local OCI/Sigstore/in-toto离线验签、immutable Observation/Fact/current、
  lost-reply Inspect与verification-aware Package强Admission；直接消费Runtime public Artifact/Trust/
  Policy Document exact Readers，不联网、不Fetch、不自动active/enable。
- `sdk`：Owner-local Go SDK；提交Registry `submitted`事实、按单一exact Snapshot解析active Package→Tool→Capability闭包并编译Surface，另提供Tool Alias装配期exact解析、Material Ensure/Inspect、current Surface→neutral Model Tool组装、MCP Server Descriptor Register/Inspect、MCP Lifecycle status、Capability Snapshot exact-current Inspect、immutable MCP Call Command/Receipt exact Inspect、Package exact Verify/current/强Admission，以及只接受已Seal Application V2 Request的受治理Action Execute/Inspect；不暴露generic Registry Transition、Fetch/Install/Enable、Connect写口、raw Provider handle或请求组装能力。Alias不支持runtime latest、Package Alias或Authority。
- `sdk`另提供Tool/Resource/Prompt Discovery Material及Page Material Set双读exact Inspect、Snapshot
  V3 exact-current Inspect与显式MCP Tool Mapping提交/Inspect/Admission；Material只返回deep-copy的
  有界不可信JSON，不自动映射Capability、不产生Context事实或Prompt authority。
- `api`：transport-neutral只读Catalog/MCP/Package Verification Read；提供exact Registry Snapshot、typed cursor、稳定分页，以及Capability/Tool/Package/Tool Alias/MCP Tool Mapping closed typed exact Inspect；另覆盖Package Verification Observation/Fact/current exact双读、MCP Server Descriptor、process Observation exact/page、Connect Intent/Receipt/Connection/DomainResult/Apply/Availability、Capability Snapshot V2/V3 exact-current、immutable MCP Call Command/Receipt与三类Discovery Material/Material Set exact Inspect；不选择HTTP/gRPC/Webhook或数据库。
- `cli`：可嵌入的CLI Runner V1，实现`tool list|inspect`、`package verify --request-json`、`mcp status|inspect|availability|snapshot|snapshot-v3|process`，其中Package Verify只接受sealed exact Request且不串联Admission/Fetch/Enable，`process`仅做有界pull-page，`tool inspect --kind=mcp-mapping`与`mcp inspect`只接受exact Ref；Call仍不输出Params Inline/Canonical Response，Discovery Material只输出其有界canonical Observation；不创建根二进制、不读进程环境，也不接受`call/connect/discover/install/enable`写命令。

模块未提供production Network/Credential materializer、持久外部状态Backend或composition root；Connect的真实stdio/loopback HTTP仅由受控测试装配证明，不构成生产部署承诺。Runtime V1缺少actual Provider Binding、Prepared Attempt current proof与统一NotAfter，因此V1 Owner Flow仍在`runtime_attempt_bound`后Fail Closed且不得包装升权。Runtime public V2 Gateway与Tool `ControlledProviderV2`隔离接线、Owner start-or-inspect闭环及第三轮独立审计已经完成；这不等于production端到端启用。Identity/Assembler接线、production composition root、真实Provider/Transport Backend与G6B尚未闭合，在这些门通过前生产能力持续NO-GO。
