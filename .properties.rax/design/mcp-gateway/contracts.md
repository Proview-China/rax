# MCP Gateway对象与生命周期合同

## 1. MCP核心对象

| 对象 | 必需字段 | 不变量 |
|---|---|---|
| `MCPServerDescriptor` | ServerID、Revision、Source、SupportedProtocolRange、TransportCapabilities、AuthRequirement、TrustClass、NetworkScope、Limits、Artifact/Config Digest、Owner、Conformance、Residual | 不存Credential值；Source/Transport不进入Tool语义 |
| `MCPConnectionRef` | ConnectionID、Epoch、ServerRef、Tenant/Identity/Plan/Instance/可选Run、CredentialRef、NetworkPolicyDigest、NegotiatedVersion、SessionRef、Created/Expires | Epoch单调；Provider Session ID不是Praxis身份 |
| `MCPCapabilityObservation` | ConnectionRef、ProtocolVersion、Tools/Resources/Prompts、ServerCapabilities、SourceSequence、PayloadDigest、ObservedAt | 外部Observation，不能直接进入Surface |
| `CapabilitySnapshotV2` | SnapshotID、Revision、ServerRef、ConnectionEpoch、Validated Tools/Resources/Prompts、SourceDigest、ValidationDigest、Conformance、Residual、Expires | 兼容历史；同Revision内容不可变，不含完整Material provenance |
| `CapabilitySnapshotV3` | V2业务字段、每Page Command/Receipt/Apply/MaterialSet、每Tool/Resource/Prompt exact Material Ref | provenance进入canonical；V2不得包装成V3；漂移生成新Revision |
| `MCPCallEnvelope` | Action/Attempt、ConnectionRef、Method、RequestID、ParamsDigest、CapabilityRef、TaskPreference、Limits | 必须绑定Runtime Prepared Attempt；不携带明文Secret |
| `MCPProtocolReceipt` | CallRef、RequestID、ProviderOperation/TaskRef、Response/Error/Progress摘要、Source/Epoch/Sequence、EvidenceRef、ObservedAt | 只作Observation；重复ID/响应冲突 |
| `MCPResidual` | Server/Connection/Attempt、Class、Scope、Inspectable、CleanupOwner、Evidence、ObservedAt | 关闭连接不自动清零 |

## 2. Effect、Conflict Domain、OperationScope、RunStart/RunSettlement Requirement与pre-run Evidence

- MCP全部外部动作及其治理Effect kind以`../tool-engine/contracts.md`第3节为唯一清单；Gateway不得用协议Method绕过`praxis.mcp/*` Effect分类。
- Registry、连接与远端资源写入的Conflict Domain分别绑定`tenant+server`、`tenant+server+identity`和`tenant+server+remote-resource/account`；Descriptor模板编译Domain，模型与Server不能自由填写。
- Run内`call/read-resource/get-prompt/cancel/inspect/cleanup`使用run类型OperationScope并绑定Run、Plan、Instance、Identity、Action、Attempt与当前Fence；Run前`connect/discover/refresh/register/revoke`只能使用显式`OperationScopeAdminV3`，不能伪造RunStartRequirement或RunSettlementRequirement。MCP连接能力若是启动前置条件则单独发布RunStartRequirement；阻塞收口/termination report时才发布namespaced RunSettlementRequirement参与者。
- 本地静态Registry/Schema验证不要求pre-run Evidence。真实Run前connect/discover/refresh/register/revoke一旦触达外部系统并产生Operation Observation/Settlement，必须使用统一OperationScope-aware Evidence V3或真实管理Run；否则Capability保持unsupported且不得触达Server。
- Begin后丢回包只Inspect原Attempt；远端Inspect本身是具有关联Relation、独立Admission/Permit/Begin/Enforcement/Settlement的Operation Effect。

## 3. Server Registry状态机

```text
submitted -> validated -> registered -> active -> deprecated -> revoked
                      \-> rejected
```

- `validated`只证明Descriptor结构和供应链检查；
- `registered`不代表允许联网；
- `active`表示可供Plan选择，不代表任一Run已授权；
- `revoked`阻止新连接/绑定并触发活跃连接Reconcile；历史Descriptor保留。

## 4. Connection状态机

```text
registered
  -> resolving
  -> connecting
  -> initializing
  -> discovering
  -> bound
  -> degraded
  -> draining
  -> closed
```

失败分支：

- 任何接触Provider前失败：`connect_failed`，可由新的受治理Connect Attempt重试；
- Begin后回包丢失：`outcome_unknown`，只Inspect原Attempt；
- Initialize版本不兼容：`rejected_incompatible`，不得继续发现；
- Discovery超限/非法：`quarantined_observation`，不生成active Snapshot；
- Session过期/重启：旧Connection进入draining/closed，新连接使用更高Epoch。

## 5. Capability Snapshot状态机

```text
observed -> validated -> admitted -> active -> superseded|revoked|expired
            \-> rejected
```

- Server list_changed只触发新Observation；
- Schema、名称、数量、大小、注解与版本检查完成后才可validated；
- Tool Engine显式`MCPToolMappingManifestV1`与组织Policy完成后才可admitted；名称、Schema或annotations不能自动Admission；
- active Snapshot被Surface引用后不可原位修改；
- Run内收到新Snapshot时，当前Surface保持不变或进入显式Degradation/Reconcile。

## 6. Call、Task、Progress与取消

- 普通`tools/call`使用JSON-RPC request ID；取消用`notifications/cancelled`；
- 2025-11-25 Task增强仅在协商Capability后使用`tasks/get/result/cancel/list`；Task取消不得使用普通cancel notification；
- Progress Token只关联过程事件，不能证明工作继续、完成或成功；
- Streamable HTTP/SSE断开不等于取消；
- Task/Provider Operation Ref只作为远端定位符；
- 普通Call没有可查询语义时，Unknown保持Unknown并声明Residual；
- Task status/结果仍是Provider Observation，必须进入Receipt/Inspect/Settlement链。

### Progress与Logging Observation V1

官方SDK的`notifications/progress`与`notifications/message`只进入Tool/MCP Owner的有界
process Observation journal。canonical对象精确绑定Connection、Connection Epoch、当前
Capability Snapshot、单调Source Sequence、correlation digest、payload digest与Observed时间；
Progress只保存有限的progress/total，Logging只保存闭集level与有界logger。原始不可信数据只
参与不超过MCP message上限的canonical digest，不作为系统指令或权威事实持久化。

process Observation不是ToolResult、Runtime Evidence、Timeline Fact、Review Verdict或执行权。
wrong Session、过期Connection、clock rollback、非标量Progress Token、NaN/Inf、非法level、
超限payload与typed-nil全部Fail Closed。当前official Go SDK v1.6.1没有可直接复用的Task
public nominal；Task仍保持未实现，不能以私有字符串状态冒充标准支持。

有界读取复用同一Tool/MCP Owner Journal和
[`MCPProcessObservationReadPortV1`](process-observation-read-v1.md)：Request固定exact
Connection Object Ref+Epoch、Snapshot、exclusive after sequence与`1..256` limit；返回严格
source-ordered Observation、next/upper bound/has-more和完整PageDigest。空页、NotFound或
`has_more=false`不证明Provider未执行或Action完成；V1不提供`--follow`、Webhook、SSE、Task
Watch或production retention。

### MCP Server Descriptor Repository V1

Tool/MCP Owner使用唯一Repository保存immutable `MCPServerDescriptor` history与current索引。
revision 1只能create；successor必须携带current full Ref并满足`revision=current+1`的CAS；命中
历史版本时只有该版本仍为current才可幂等返回，current已经前进后重投旧revision必须Conflict，
不得回退或ABA。SDK Register只封装该Repository，不Connect、不Initialize、不Discover，也不
选择stdio/HTTP、Credential、进程或production backend。

### 6.1 `tools/call`的Single Call Adapter边界

- 单个`MCPCallEnvelope`只对应一个已Settlement `PendingActionV2`、一个ActionCandidate、一个Reservation和一个Runtime Attempt；输入为集合、批次、custom effect或`N > 1`时Fail Closed且不得向Server发送任何消息；
- Envelope必须携带并校验原ToolCall Observation ref/digest、PendingAction Ref/RequestDigest、Action/Reservation、Surface/Capability/Tool/Input Schema、Payload/Params、SourceCandidate、Attempt、Connection Epoch和Request ID的exact refs/digests；只匹配JSON-RPC ID或Action Ref不足以执行；
- 本路径固定`OperationScopeKind=run`、`EffectKind=praxis.tool/execute`、`PolicyProfile=praxis.tool/single-call-action-v1`，Run/Session/Turn/Action/Context全部required；
- Prepare必须等待V4.1 prepare Enforcement与Evidence V3 prepare Issue/current/handoff；Execute必须exact绑定Prepared Attempt/prepare receipt，再等待V4.1 execute Enforcement与独立Evidence execute Issue/current/handoff。对应handoff前Transport写次数必须为0；
- `OperationScopeEvidenceCandidateV3`只携带Qualification与MCP Observation内容，`ConsumeOperationScopeEvidenceRequestV3`独立携带Handoff；prepare/execute不得复用Qualification、Handoff、EventID、SourceSequence或ConsumptionID；
- 丢失Call回包、SSE断流、Transport超时或Task状态不明一律保持原Attempt Unknown，只允许本地Inspect原Attempt；远端Task Inspect另起关联原Effect的受治理Operation；
- MCP Result/Error/Task状态只是Observation。Tool Owner复读Candidate/Reservation current并独立Inspect后CAS `ToolDomainResultFactV2`；Runtime Settlement V4只绑定typed DomainResult与prepare/execute两项Evidence Binding并投影settled。G6A先取得current `OperationInspectionSettlementRefV4`并通过Application-facing只读Association Inspect验证两phase，再`ApplySettlementV4`，权威输出只到settled `ToolResultV2 + Inspection`并硬停；不调用Context/Harness、不启用能力。G6B才把该结果作为`ContextTurnRefreshPortV1`输入；Tool/MCP不得构造Harness Continuation；
- legacy `ActionPort`/`ToolPort`/`MCPPort`、`GovernedExecutionProviderV2`、Provider `EvidenceRecordRefV2`和Settlement V3都不得包装为上述公共合同。

## 7. Transport Adapter合同

Transport Adapter只负责字节/连接语义，不决定Capability、Review或Result：

- `stdio`：UTF-8、一行一个JSON-RPC消息、stdout只允许协议消息、stderr独立采集；
- Streamable HTTP：POST/GET/SSE、Origin校验、Accept/Content-Type、MCP-Protocol-Version、Session ID、Last-Event-ID与安全重连；
- 自定义Transport：必须声明独立Extension、消息边界、身份、恢复和安全语义；
- Adapter返回`not_sent / sent_response_received / sent_outcome_unknown`交付确定性；不得把socket错误统一映射成not_sent；
- Credential Materializer只接收短期Lease/Ref并在实际请求点注入，禁止反向写入Descriptor/日志。

## 8. 标准MCP与Praxis映射

| MCP对象 | Praxis对象 | 映射限制 |
|---|---|---|
| Tool | Capability Observation -> ToolDescriptor mapping | 注解不可信；Effect/Risk由Praxis Owner决定 |
| Resource | Context Source Candidate | read属于Effect；不能当Tool或系统指令 |
| Prompt | Prompt Asset Candidate | 必须经过Context/Review策略；不能授予权限 |
| tools/call result | MCPProtocolReceipt | 不直接成为ToolResult |
| Task | ProviderOperationRef/过程Observation | 不成为Runtime Run或Harness Session |
| list_changed | 新Discovery Observation | 不直接修改当前Surface |
| Server instructions | 不可信外部内容 | 不进入System Policy/Authority |

三类`MCP*ObservationV2`只保存摘要；完整Tool/Resource/Prompt标准对象由
[MCP Discovery Material V1](mcp-tool-discovery-material-v1.md)以exact Page Command、Connection
和typed canonical JSON保存。逐字段digest闭合后仍只是Observation；没有版本化Mapping
Manifest时不得提交Capability/Tool Registry事实，Resource/Prompt也不得直接成为Context事实或
系统指令。

## 9. MCP Server Facade合同

Inbound Server Facade依赖宿主提供的已认证Session Binding和Tool Surface Ref：

1. Initialize只协商协议与Capabilities；
2. tools/list从精确Surface读取；
3. tools/call创建Action Candidate并等待统一治理；
4. 普通/Task等待状态通过标准消息表达，但内部Attempt/Settlement身份不泄漏为Authority；
5. disconnect只结束Transport，不修改Action事实；
6. 多Tenant与多Identity必须物理或逻辑隔离Session；
7. 未绑定Authority/Scope/Surface的Session最多允许协议级健康操作，不允许列出或调用真实能力。

## 10. 兼容策略

- 稳定基线：2025-11-25；初始化按双方支持版本协商；
- 2025-06-18的official public type与Praxis strict codec降级Conformance已通过；official SDK
  v1.6.1未公开旧版本Client option，故真实public Session降级与支持期限仍由管理线冻结；
- draft/2026-07-28不进入发布承诺，测试只能标记experimental；
- 标准字段保真，不使用Praxis字段替换或重定义标准字段；
- Praxis扩展仅放在显式协商的namespace/meta/sidecar中；
- 升级前对Schema、Task、Transport、Auth、取消、Result Content和Capability进行Diff与Conformance；
- 不兼容Peer Fail Closed并记录Residual，不默默降级成未经治理调用。
