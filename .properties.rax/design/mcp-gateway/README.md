# MCP Gateway 可实施设计

## 1. 状态与定位

- 设计域：`mcp-gateway`，与`tool-engine`共同组成`tool-mcp`组件线。
- 最高业务输入：`tmp.document/Tool&MCP.md`。
- 当前阶段：Tool G6A V2 Owner-local隔离实现第三轮独立审计最终YES（P0/P1/P2=0）；Application窄Port/DTO、Evidence V3 Action矩阵/五维Reader、V4.1/V2受控测试Provider seam、Settlement V4 current closure与只读Association Inspect均已用于隔离实现和测试。该YES不代表系统G6A或production GO；Identity/Assembler、production MCP Backend/root与G6B仍为NO-GO，G6B PASS前禁止MCP Provider生产能力启用、Continuation与Turn推进。
- 稳定兼容基线：官方MCP `2025-11-25`；`draft/2026-07-28`仅跟踪，不进入兼容承诺。
- 降级证据见[MCP 2025-06-18降级Conformance V1](mcp-2025-06-18-conformance-v1.md)：
  official Go SDK public协议类型与Praxis严格codec的initialize/list/call往返已通过；official
  SDK v1.6.1没有public旧版本Client option，因此未宣称真实public Session降级，正式治理链
  仍锁`2025-11-25`。
- 官方生态组装：新增[官方MCP Go SDK组装与Discovery V2](official-go-sdk-assembly-v1.md)，首批直接复用`github.com/modelcontextprotocol/go-sdk v1.6.1`的Session与协议实现，Praxis只负责typed Observation/Snapshot和治理边界，不重复实现Client/Server wire stack。
- 受治理分页发现：[Discovery Page V1](governed-discovery-page-v1.md)已完成owner-local软件闭环。每个SDK分页请求是独立`praxis.mcp/discover` Effect；Runtime专属Page gate、Tool Page Receipt/DomainResult/Apply与Capability Snapshot聚合均已落盘。旧注入式Discovery仍只作兼容测试，Application多页编排与production装配未启用。
- Discovery原始材料见[MCP Discovery Material V1](mcp-tool-discovery-material-v1.md)：official
  SDK返回的Tool/Resource/Prompt完整canonical JSON与各自Snapshot摘要逐字段闭合，并随Page
  Receipt在同一owner-local物理仓原子保存、exact Inspect；三类材料保持类型隔离，只是不可信
  Observation，不自动生成Capability、Context事实、Prompt authority或执行权限。
- [MCP Discovery Snapshot V3来源闭包](mcp-discovery-snapshot-v3.md)已按用户裁决实现并通过
  owner-local software test：V3 canonical直接绑定每个受治理Page、Receipt、Apply、typed
  Material Set及Tool/Resource/Prompt Material exact Ref；V2历史不重写、不包装。SDK/API/CLI
  `mcp snapshot-v3`只按exact Ref双读，production durable backend仍NO-GO。
- MCP Tool语义映射采用Tool Owner显式版本化
  [Mapping Manifest V1](../tool-engine/mcp-tool-mapping-manifest-v1.md)，已实现Snapshot/Material
  S1-S2与Capability/Tool/Mapping三Record单RegistryRevision Admission。它只到`admitted`，
  不自动active/enable，也不从annotations推断治理语义。
- Connect方向：[Run/Session级受治理MCP Connect V1](governed-run-connect-v1.md)已完成owner-local实现与软件门：每个Run/Session/Server独立Connection与新Epoch，`praxis.mcp/connect`使用Run/Session两维矩阵，不复用单Action五维门；production Network/Credential/backend/root仍NO-GO。
- 真实调用第一切片：[受治理MCP tools/call actual-point Port Delta](controlled-mcp-call-port-delta-v1.md)中的Runtime public V3、Tool command/Prepared关联、official SDK physical executor与create-once admission已完成owner-local软件测试；[Provider Receipt→Evidence→Observation协调](provider-receipt-observation-v1.md)也已完成owner-local软件门。Connect路径已经闭合typed DomainResult、Runtime Settlement V4、Tool ApplySettlement与Connection Availability；`tools/call`的宿主总装、Evidence Source production provisioning与production root仍NO-GO。
- 协议Transport证据：同一受治理`tools/call`已穿过official SDK真实子进程stdio与loopback Streamable HTTP Server fixture，并通过ordinary×100/race×20；该证据不包含production Credential、Network policy、durable backend或composition root。
- 回包安全见[Official SDK Result Safety V1](official-sdk-result-safety-v1.md)：只接受official Call Result合法内容形状，以Tool ResultLimit/Receipt上限有界canonicalize；无法完整持久化时保持Unknown并只Inspect，不截断或伪造Artifact。
- 体系图：[architecture.drawio](architecture.drawio)。协议对象、生命周期和兼容规则见[contracts.md](contracts.md)，验收见[acceptance.md](acceptance.md)；`tools/call`进入Tool侧单Call治理的字段与Owner边界见[Tool Engine合同](../tool-engine/contracts.md)和[公共接线](../tool-engine/integration.md)。
- Lifecycle只读状态入口见[status-inspect-v1.md](status-inspect-v1.md)；它只Inspect Tool Owner已持久Connection/Snapshot事实，不触发外部MCP动作。
- Cancel/Drain/Close拆分候选见[受治理生命周期Effect V1](governed-cancel-drain-close-v1.md)：本地Inspect已实现；普通Cancel与Close分别需要新的Runtime矩阵/actual-point Port，Drain只做Tool Owner本地准入CAS，Task保持NO-GO。该候选等待用户与Runtime/Application联合冻结，当前不解锁Go写入口。
- 生命周期公共Delta见[Controlled MCP Lifecycle Port Delta V1](controlled-mcp-lifecycle-port-delta-v1.md)：冻结Cancel五维/Close Run-Session矩阵候选、active-call current、Drain Store与physical authorization字段；同时记录live Discovery Page矩阵未进入Runtime通用注册闭集的既有P0。
- 过程暴露见[MCP Process Observation有界读取 V1](process-observation-read-v1.md)：复用official SDK progress/logging Handler与唯一Tool Journal，提供exact Inspect和Connection/Epoch/Snapshot/source-sequence有界pull-page，并经Go SDK/API/CLI暴露；它不是Evidence、Timeline、ToolResult、长连接Watch或production backend。

## 2. 作用

MCP Gateway拥有MCP Server的注册、解析、连接、初始化、发现、Session、能力快照、刷新、协议调用、取消、Inspect、Drain和Close。它把标准MCP的Tools、Resources与Prompts保真接入，但不会把协议可见性升级成Praxis执行授权。

`MCP Tool -> Praxis Capability`必须先经过Snapshot V3 exact provenance与Tool Engine显式Mapping
Manifest Admission；`MCP Resource`仍是Context Source；`MCP Prompt`仍是外部提示资产。三者不得在
内部Catalog里合并成同一种“工具”。

## 3. Owner与非Owner

### 3.1 MCP Gateway拥有

- `MCPServerDescriptor`、Server Registry、来源和协议兼容状态；
- `MCPConnectionRef`、Connection Epoch、Session绑定、传输与生命周期；
- Tools/Resources/Prompts的Provider Observation、Schema/数量/大小/名称校验和`CapabilitySnapshot`；
- MCP请求/响应、JSON-RPC错误、进度、取消、Task和协议Residual的规范化；
- 标准层与扩展层的Capability Negotiation、命名空间隔离和Conformance证据；
- MCP协议过程事件、健康/退化/Drain/Close事实。

### 3.2 MCP Gateway不拥有

- Tool Capability的业务语义、模型可见Surface、最终Tool Result或Action Settlement；
- Runtime Intent/Permit/Fence/Outcome，Review Verdict，Budget、Authority或Sandbox Policy；
- Server自报业务成功的真实性；Provider回包只能是Observation/Receipt；
- Context对Resource/Prompt的选择、顺序、展开与Prompt Cache；
- 生产Credential内容、网络出口、容器或进程调度。

## 4. 标准兼容边界

### 4.1 v1标准面

- JSON-RPC 2.0消息与严格请求/响应ID关联；
- `initialize -> initialize result -> notifications/initialized`生命周期和版本协商；
- `ping`、进度与日志按协商Capability处理；
- Tools：`tools/list`、`tools/call`、`notifications/tools/list_changed`；
- Resources：`resources/list`、`resources/read`及协商后的subscribe/list_changed；
- Prompts：`prompts/list`、`prompts/get`及协商后的list_changed；
- 普通请求取消使用`notifications/cancelled`；Task增强请求使用`tasks/get`、`tasks/result`、`tasks/cancel`和可选`tasks/list`，不得混用普通取消；
- 标准Transport Adapter首批规划`stdio`和Streamable HTTP，但核心合同不包含任一Transport字段假设；
- Streamable HTTP必须校验Origin、协议版本Header、Session ID和认证绑定，断连不能视为取消。

### 4.2 扩展面

- MCP Plus、Capability Card、工具折叠、按需展开、组合包和App Server元数据只进入`praxis.mcpplus/*`扩展命名空间；
- 标准消息字段不被重定义；未协商扩展的Peer只看到标准行为；
- 扩展失败不得破坏标准MCP Session；任何Surface变化仍生成新Snapshot/Surface Revision；
- 自定义Transport必须声明独立Conformance与安全属性，不得仅因使用JSON-RPC就宣称标准Transport兼容。

## 5. Connection与Session隔离

V1受治理Connect的每个连接绑定：Tenant、Identity+Epoch、Lineage/Plan Digest、Instance+Epoch、必需Run、Harness Session、Credential Ref、Network Policy、Server Descriptor、Connection Epoch与Capability Snapshot。

- 不同Tenant不得共享认证Session；
- Run/Session级V1禁止跨Run静默复用；同一Run/Session可以按Server建立多个独立连接；
- 断连、Server重启或Resume失败创建新Connection Epoch；
- 旧Epoch响应只进入历史Evidence；
- Provider Session ID只是远端标识，不是Praxis权威身份；
- Session本地缓存不是权威Snapshot或Receipt的唯一副本。

## 6. 调用与治理

1. `Connect/Initialize/Discover/Refresh/Call/Cancel/Remote Inspect/Close`只要接触外部进程或网络，均作为Operation Effect进入Runtime Operation V3 + Dispatch V4治理；
2. MCP Gateway的Runtime Provider Adapter在实际执行点再次验证Permit、Fence、Binding、Authority、Review、Budget、Scope、Credential和Lease；
3. `tools/call`只在已被Tool Engine映射并进入当前Tool Surface的Capability上执行；
4. `resources/read`和`prompts/get`不能伪装成无Effect调用，必须具备数据披露、大小和Prompt Injection边界；
5. 回包丢失进入Unknown；本地Inspect只读持久Session/Attempt事实，远端Task/Provider Inspect必须是关联原Effect的独立Operation；
6. 标准MCP没有提供可验证幂等或查询语义时，Descriptor必须声明Residual并降低Conformance，不能盲重试`tools/call`。
7. 当前Action Gateway只接收单个`tools/call`映射，必须绑定原Harness `PendingActionV2.RequestDigest`；`N > 1`、批量或聚合调用保持NO-GO；
8. `ActionCandidate`与Reservation只证明Tool领域当前性，不授予执行权；MCP Client Prepare/Execute各自只能在Runtime V4.1对应phase Enforcement和Evidence V3 current/handoff完成后接触Server；
9. prepare/execute分别使用独立Qualification、Handoff、EventID、SourceSequence、ConsumptionID；Evidence Candidate只携带单一Qualification，Consume独立携带Handoff，不得沿用旧的Candidate内嵌Handoff映射；
10. MCP回包先形成`MCPProtocolReceipt`/Provider Observation，再由Tool Owner独立Inspect并CAS typed `ToolDomainResultFactV2`；G6A链只收口到`Runtime Settlement V4 -> current/readonly Association Inspect -> Tool ApplySettlementV4 -> settled ToolResultV2 + current Inspection`并硬停，不调用Context/Harness、不启用能力；G6B才由Application调用`ContextTurnRefreshPortV1`并等待Context Apply/Generation CAS/S2/new Frame后再调用Harness；
11. legacy `ActionPort`/`ToolPort`/`MCPPort`、`GovernedExecutionProviderV2`、Evidence V2与Settlement V3不得包装扩权。

## 7. Inbound MCP Server

Praxis对外暴露MCP Server时，协议Server只是一层受控Facade：

- `tools/list`只返回已编译、已授权暴露且与Session绑定的Tool Surface；
- 每个`tools/call`生成Action Candidate，并经Application/Runtime/Review/Sandbox完整链；
- 客户端Session、组织Policy和Credential映射由宿主认证边界提供，MCP字段不能自授Praxis Authority；
- 长任务仅在双方协商Task Capability后映射；
- Server端回包只能引用已形成的Tool Result/Settlement，不能将Provider同步返回直接透传为权威成功；
- 关闭连接不取消可能已发生的外部Effect；必须保留Attempt、Receipt、Unknown与Residual。

## 8. 安全与资源上限

- 所有Server内容按不可信数据处理，不得成为系统指令、Policy、Grant或Secret；
- Descriptor必须给出消息、Schema、Tool数量、分页、并发、输出、Task和Session上限；
- 超限、非法JSON-RPC、重复ID、重复响应、Schema冲突、能力漂移、跨Epoch回包均Fail Closed并形成Evidence；
- stderr、HTTP状态、SSE断流和Provider错误分别分类，不能统一折叠成“Tool失败”；
- 二进制和大结果Artifact化，Event与Context只保存有界摘要/引用；
- 在公共Artifact Store/关联合同闭合前，超内联上限结果保持Unknown；digest不能冒充可恢复Artifact；
- Credential只由Broker以短期Ref/Lease注入Transport Adapter。

## 9. 管理线待裁决

1. v1是否同时交付Inbound MCP Server，还是先完成Outbound Client与Discovery；
2. 组织允许的Server来源、信任等级和默认网络Policy；
3. 生产认证机制、Credential Broker及Streamable HTTP部署入口；
4. MCP Plus的独立版本、扩展Namespace与Conformance门槛；
5. `2025-06-18`的wire/type降级Conformance已通过；真实public SDK Session降级与长期支持期限仍由上游能力和管理线冻结。
6. Application窄Port/DTO、Evidence V3 Action矩阵/五维readers、V4.1/V2受控测试Provider关联与live Settlement V4 current closure/只读Association Inspect已经闭合，Tool G6A V2 Owner-local隔离实现/测试第三轮独立审计最终YES（P0/P1/P2=0）。该YES不代表系统G6A或production GO；Identity/Assembler、Context Refresh new exact Frame链、Harness入口、production MCP Backend/root与G6B仍为NO-GO。
7. Runtime V3 actual-point authorization、Tool exact MCP command Reader、official SDK
   physical executor及专属source sequence=1的Receipt→Evidence→Provider Observation协调已完成
   owner-local闭环；Connect另已闭合正式DomainResult/Settlement/Apply/Availability。`tools/call`
   的领域结果总装、production Source provisioning与root前仍不启用能力。

## 10. Assembler发布面

MCP Gateway与Tool Engine共用一个组件发布边界，见[Tool/MCP Component Release V1](../tool-engine/component-release-v1.md)。official SDK Call、Discovery与Lifecycle只构成owner-local readiness；真实transport、Credential、durable store、cleanup和deployment attestation缺失时不得升级为production。
