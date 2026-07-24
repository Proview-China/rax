# 官方MCP Go SDK组装与Discovery V2

## 1. 状态

- 业务依据：`tmp.document/Tool&MCP.md`要求优先兼容现有MCP生态，而不是重新实现一套私有协议栈；
- live上游：`github.com/modelcontextprotocol/go-sdk v1.6.1`，Go 1.25，当前公开稳定协议为`2025-11-25`；
- V1正式链只接受Server Descriptor明确包含且不高于`2025-11-25`的协商版本；`2025-06-18`仍是后续独立降级Conformance，不由日期格式或SDK回包自动放宽；
- 实施状态：官方SDK Session之上的Tools/Resources/Prompts分页发现与Praxis typed Snapshot V2已完成Owner-local实现和软件测试；不创建production root、Credential、进程或网络配置；
- `tools/call` owner-local actual-point第一切片已实现：Runtime public V3授权、Tool
  canonical command/current Reader、official SDK initialized Session绑定、create-once
  admission与真实in-memory `CallTool`均通过软件门。正式Evidence/Provider Observation已
  闭合；`tools/call` DomainResult/Settlement宿主总装与production root仍未闭合。
- Connect owner-local链已实现：独立Run/Session矩阵、official SDK stdio/Streamable HTTP
  actual-point、Receipt、正式Observation/Evidence、typed DomainResult、Runtime Settlement V4、
  Tool ApplySettlement与Connection Availability均通过软件门；production Credential/Network
  backend与composition root仍未闭合。

## 2. 组装原则

1. 官方SDK拥有MCP wire、initialize、notification、pagination transport语义；Praxis不复制官方Client/Server/Transport实现；
2. Praxis拥有Server/Connection/Capability Snapshot、治理Effect、Receipt/Inspect/CAS/Settlement语义；官方SDK对象只是Provider Observation；
3. Tool domain contract不导入官方SDK nominal；SDK依赖只出现在`mcp` adapter包；
4. Connect V1现已使用official SDK stdio/Streamable HTTP受控入口；旧的注入式initialized Session仅保留owner-local测试，不是production构造器；
5. Discovery必须消费已Apply的Connection Availability；每一个Tools/Resources/Prompts分页请求仍是独立Effect，按[受治理Discovery Page V1](governed-discovery-page-v1.md)执行。Runtime专属Page gate与Tool owner-local分页闭环已实现；旧注入式Discovery不得包装或升级为真实能力；
6. 所有分页设置page/item上限、cursor环检测、nil item检查和canonical digest；外部说明、annotation、meta与schema均是不可信数据；
7. Snapshot V2把Tool、Resource、Prompt保持三种nominal，不折叠成统一Tool；stable排序后Seal，Provider顺序不进入语义；
8. list_changed只触发新的Discovery请求，不原位修改已Seal Snapshot或当前Tool Surface。

live owner-local实现新增`MCPListChangedObservationV1`与
`OfficialSDKListChangedBridgeV1`：Bridge只安装官方SDK
Tools/Resources/Prompts三个notification handler，按exact Connection/Session/旧Snapshot
记录Tool-owned Observation；同namespace同旧Snapshot的重复/并发通知合并为一个pending
Fact。只有新Snapshot已经由受治理Discovery产出后，journal才接受successor acknowledgement并
释放pending索引。Handler不调用Discovery、不改Snapshot、不接触网络。

## 3. Owner-local对象

`MCPCapabilitySnapshotV2`新增：

- Server、Connection、ConnectionEpoch、ProtocolVersion；
- ServerInfo、ServerCapabilities、Instructions的canonical digest；
- `MCPToolObservationV2[]`：Name、Title、Description、Input/Output Schema、Annotations、Meta摘要；
- `MCPResourceObservationV2[]`：URI、Name、Title、MIME、Size、Description、Annotations、Meta摘要；
- `MCPPromptObservationV2[]`：Name、Title、Description、Arguments、Meta摘要；
- SourceDigest、ValidationDigest、Conformance、Residual、Created/Expires与Snapshot canonical Digest。

V1合同保持不变；V2是additive successor，不允许把Resources/Prompts塞入V1 Tools或修改V1 canonical。

## 4. 调用与失败语义

```text
settled Connection Availability
  -> one governed Page Effect per tools/list, resources/list or prompts/list request
  -> each Page Receipt -> formal Observation/Evidence -> DomainResult -> Settlement -> Apply
  -> Application advances only from an applied next-cursor fact
  -> Tool aggregates applied pages, normalizes and canonical-sorts
  -> Seal MCPCapabilitySnapshotV2
```

- nil/typed-nil Session、nil/canceled context、过期Connection、协议漂移：零SDK读取；
- Unavailable、timeout、decode或分页异常：不产生Snapshot；
- cursor重复、超过页数/对象数、duplicate Tool name/Resource URI/Prompt name：Fail Closed；
- 同输入同内容产生同一canonical Snapshot；same ID/revision换内容Conflict；
- Snapshot Repository保留immutable history与单一current：revision 1 create，successor必须携
  full expected-current Ref且只允许`current+1` CAS；lost reply重复winner幂等，rev2成为current后
  重投rev1或ABA不得回退current；
- SDK返回的Tool annotation、Server instructions或Meta不得形成Authority、Review、Effect Class或系统指令。
- notification携带wrong Session、Connection过期、clock rollback、pending期间换Snapshot时
  Fail Closed；同旧Snapshot的64并发通知只产生一个Observation。该Observation不是Runtime
  Intent/Permit，也不授权Connect或Discovery。

## 5. 真实Call公共接线

Runtime public V3与Tool侧exact桥已经按[受治理MCP tools/call actual-point Port Delta](controlled-mcp-call-port-delta-v1.md)落盘：

1. Runtime public `ControlledOperationPhysicalExecutionAuthorizationV3`无损携带StableKey、统一NotAfter、Transport/Provider、Operation/Scope、Prepared/Attempt、execute Enforcement/Handoff、Boundary、Association与DomainCommand；
2. Tool exact `MCPExecutionCommandCurrentReaderV1`复读BindingV2/ActionCandidateV3/canonical payload与MCP Connection/Snapshot Tool；
3. `OfficialSDKPhysicalExecutorV1`在实际入口fresh复验授权、Association、Command与initialized Session后直接调用官方SDK `CallTool`；
4. create-once admission成功即可能已调用；lost reply/SDK error只Inspect原StableKey Entry，不重发Request ID；
5. 回包先形成`MCPProtocolReceiptV1`；Tool exact Reader与Runtime-owned协调Port复用既有
   Evidence/Observation Gateways形成正式`ProviderAttemptObservationRefV2`，Tool不得自行构造Evidence Ref；
6. Runtime V3 authorization issuer会在签发前复读完整V2 current closure与create-once
   Prepared-domain-command Association，本身不调用Provider。

该owner-local软件闭环不是production GO：受治理stdio/HTTP Connect虽已在测试装配中闭合，
但production Credential、Network policy materializer、持久Session/State Plane、Evidence
Source production provisioning与root尚未闭合。

## 6. 验收

- 官方SDK in-memory真实Client/Server完成initialize并发现Tools/Resources/Prompts；
- 分页、cursor环、duplicate、超限、错误Capability、协议/Session/TTL漂移反例；
- Connect协商版本超出exact Server Descriptor范围，或driver记录的initialize bytes与同一Session canonical InitializeResult漂移时，不形成Receipt/Connection；Entry保持Unknown、Session作为Residual保存且重投零Provider。Connect Operation不自行调用Session Close；清理由独立受治理Close完成；
- 64并发Discovery无数据竞争，所有Snapshot canonical一致；
- Tool contract不import官方SDK，adapter不导出raw Transport/Client Connect；
- ordinary×100、race×20、full ordinary/race、vet与import boundary通过。

当前实测已通过Discovery全部门，以及受Runtime V3授权的official SDK in-memory真实
`tools/call`、64同key单effect、lost reply inspect-only、binding/TTL/clock/typed-nil
反例；official SDK真实`tools/list_changed`通知、pending coalescing/successor ack、64并发、
targeted ordinary×100、race×20、Tool full ordinary/race与vet也已通过。该结果只表示
owner-local software test YES。正式Connect Observation/Settlement已经在Connect专属链闭合；
Discovery Page owner-local链已闭合到`Page Receipt -> formal Observation/Evidence -> typed DomainResult -> Runtime Settlement V4 -> Tool ApplySettlement -> Capability Snapshot`，并提供exact-current SDK/API/CLI `mcp snapshot`只读入口。Application多页编排、list_changed调度、`tools/call`领域结果宿主总装及production能力仍NO-GO。
