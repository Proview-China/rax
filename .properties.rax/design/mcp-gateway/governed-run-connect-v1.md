# Run/Session 级受治理 MCP Connect V1

## 1. 冻结结论

- 生命周期：每个 Praxis `Run + Harness Session + MCP Server`建立独立连接；不同 Run 不复用连接或认证状态。
- 连接恢复：断连、Server 重启、Session Resume 失败或重新 Connect 都创建更高 `ConnectionEpoch`；旧 Epoch 的迟到消息只进入历史 Observation。
- Runtime 矩阵：`OperationScopeKind=run`、`EffectKind=praxis.mcp/connect`、`PolicyProfile=praxis.mcp/run-connection-v1`。
- Applicability：Run、Session 为 `required`；Turn、Action、Context 为 `forbidden`。Connect 不是 Model Tool Call，不得伪装成 `praxis.tool/execute` 或复用单 Action 五维矩阵。
- Evidence 时点：连接发生在 Run/Session 已存在之后，不触发 pre-run Evidence Delta；prepare 与 execute 各自使用独立 Enforcement、Evidence Candidate、Handoff 与 Consumption。
- Transport：V1 只组装官方 Go SDK 的 `CommandTransport`（stdio）和 `StreamableClientTransport`（Streamable HTTP）。Praxis 不重写 MCP wire、initialize、分页或 Session 协议。

## 2. Owner 与非 Owner

| 对象/行为 | Owner | 非 Owner约束 |
|---|---|---|
| Server Descriptor、Transport Config、Connect Intent、Connection/Epoch、Receipt、Inspect/CAS | Tool/MCP | 不写 Runtime Outcome、Review Verdict或Sandbox事实 |
| Operation Intent、Admission、Permit、Begin、Fence、Runtime Settlement | Runtime | 不解释MCP协议和Server能力 |
| Review Verdict | Review | 不Dispatch、不Connect |
| Sandbox/Network/Credential current与实际强制 | 对应Sandbox/治理Owner | Tool只读复读，不复制、不自签 |
| Run/Session current | Runtime/Harness公开Reader | Tool不得导入Harness私有Port或实现 |
| 物理Connect入口 | Sandbox内Tool MCP executor | 必须在同一入口fresh复读并立即调用官方SDK，不得验证后再转发到raw Transport |

## 3. Tool Owner对象

### 3.1 `MCPConnectionCoordinateV1`

不可变稳定坐标：

`TenantID + IdentityID/Epoch + PlanDigest + InstanceID/Epoch + RunID + SessionID/Revision/Digest + ServerRef + ConnectionEpoch`。

- `ConnectionEpoch`从同一稳定Scope的owner current单调增加；create仅epoch 1，successor必须携带完整expected-current并为`current+1`。
- Stable ID从上述完整坐标canonical派生；不得从Provider Session ID派生。
- 同一Run/Session可以连接多个Server；同一Server在不同Run之间必须使用不同坐标。

### 3.2 `MCPTransportConfigV1`

公共合同保持Transport one-of，禁止在Tool语义中暴露官方SDK对象：

```text
Ref{ID, Revision, Digest}
ServerRef
Kind = praxis.mcp.transport/stdio | praxis.mcp.transport/streamable-http
ProviderTransportBinding
ArtifactDigest
ConfigDigest
NetworkScopeDigest
SandboxRequirementDigest
StdioConfig? {Executable, Arguments[], WorkingDirectory, CredentialPlaceholders[]}
HTTPConfig?  {Endpoint, DisableStandaloneSSE}
CreatedUnixNano
```

约束：

- stdio不继承未声明环境；配置不保存Secret值，Credential只以Runtime公开Lease Ref在actual point物化；stdout只承载协议，stderr独立观察。
- HTTP Endpoint必须是`https`或明确测试用loopback `http`；禁用跨Origin redirect；认证头只由Credential materializer在actual point注入。
- 官方SDK `StreamableClientTransport.MaxRetries`固定为负值，禁止SDK在同一Epoch内静默重连；恢复必须走新Connect Operation和新Epoch。
- official SDK返回的`InitializeResult.ProtocolVersion`必须位于exact `MCPServerDescriptor.MinimumProtocol..MaximumProtocol`且不高于模块稳定上限。Driver记录的initialize bytes必须逐字等于同一Session `InitializeResult`的canonical JSON；任一漂移发生在物理Connect之后，因此Entry进入inspect-only Unknown并保留Session残留供未来受治理Close；不得封存可用Receipt、自动Close或重连。
- Config Repository为Tool Owner唯一history/current仓：rev1 create；successor `expected current + current+1` CAS；lost reply只exact Inspect；same ID换内容、旧revision回投和ABA均Conflict。

### 3.3 `MCPConnectIntentV1`

持久化发生在任何进程启动或网络访问之前。canonical至少绑定：

- 完整Connection Coordinate、Server Descriptor exact Ref/current、Transport Config exact Ref/current；
- Operation/OperationDigest、Effect ID/Revision/IntentDigest、Attempt；
- Sandbox Requirement、Network Scope、排序后的Credential Lease Refs；
- Provider/ProviderTransport bindings；
- Created、NotAfter、Revision、Digest。

Intent只表达领域命令，不授执行权。Provider回包不能创建或修改Intent。

## 4. Runtime additive Port Delta

现有`ControlledOperationProviderRequestV2`、Route V2和Physical Authorization V3硬绑定单Action矩阵，不可包装扩权。需要Runtime Owner additive公开面：

```go
const OperationScopeEvidenceMCPConnectEffectKindV1 = "praxis.mcp/connect"
const OperationScopeEvidenceMCPConnectPolicyProfileV1 = "praxis.mcp/run-connection-v1"

func OperationScopeEvidenceMCPConnectMatrixV1() OperationScopeEvidenceApplicabilityMatrixKeyV3

type ControlledMCPConnectPhysicalAuthorizationV1 struct {
    StableKey, UnifiedNotAfter
    Operation, OperationDigest, OperationScopeDigest
    EffectID, EffectRevision, IntentDigest, Attempt
    PrepareEnforcement, ExecuteEnforcement
    PrepareConsumption, ExecuteHandoff
    SandboxCurrent
    CredentialCurrents
    Provider, ProviderTransport
    DomainCommand OperationDomainCommandRefV1
}

type ControlledMCPConnectPhysicalAuthorizationPortV1 interface {
    AuthorizeControlledMCPConnectPhysicalV1(context.Context, RequestV1) (AuthorizationV1, error)
}
```

Runtime发行端必须S1/S2复读Run、Session、Operation governance、Review、Budget、Authority、Fence、Sandbox、Network、Credential、两阶段Enforcement/Evidence和Tool Domain Command Association。任何Unavailable/NotFound/漂移/过期/clock rollback均零Provider。

该Delta是additive；V2/V3 Action合同保持原样，不扩大其Effect或矩阵。

## 5. actual-point顺序

```text
Tool MCP Connect Intent/Reservation (persisted)
  -> Runtime Admission / Permit / Begin
  -> prepare Enforcement + prepare Evidence issue/current/handoff/consume
  -> Tool prepare: exact Server/Config/Run/Session/Sandbox/Credential read
  -> execute Enforcement + independent execute Evidence issue/current/handoff
  -> Runtime issues bounded physical authorization
  -> Tool actual-point fresh S2 read of same exact refs
  -> CAS physical admission (success means Provider may have been contacted)
  -> immediately construct official SDK transport and Client.Connect
  -> initialize result becomes MCP Protocol Receipt/Observation
  -> consume execute Evidence from that Observation; it is never forged as a pre-effect authorization
  -> Tool Owner Inspect/CAS MCPConnectionRef + Epoch
  -> Runtime Evidence/Observation/Settlement
```

### 5.1 Connection事实的Session消歧

live旧`MCPConnectionRef`只有`SessionID string`，无法同时无歧义表达Praxis
Run内Session与Provider返回的可选Session ID。新Connect路径采用additive
`MCPConnectionFactV2`：

- `Coordinate.Session ObjectRef`是Praxis Session唯一身份来源；
- `ProviderSessionID`只保存Receipt中观察到的远端标识，允许为空，不参与
  Connection稳定ID或Epoch；
- Fact canonical必须绑定exact Connect Intent、Transport Config、Server、Protocol
  Receipt、Provider/Transport Binding与Connection Coordinate；
- SDK initialize回包不能直接创建Fact。Tool Owner必须重新Inspect exact Receipt、
  original physical Entry及同一official Session，再CAS create-once Fact；
- 丢失Fact create回包只按Fact exact Ref Inspect，不重新Connect；同ID换Receipt、
  Provider Session或任一因果字段均Conflict；
- 旧`MCPConnectionRef`保持兼容，不由本路径包装或赋予V2语义。

### 5.2 Observation、Evidence与Settlement复用

- `MCPConnectProtocolReceiptV1`通过既有Runtime-neutral
  `OperationProviderReceiptReaderV1`投影；Runtime通用
  `OperationProviderReceiptObservationGatewayV1`负责正式Observation/Evidence摄取，
  Tool不得自签Runtime Observation；
- execute Evidence Consumption只能由上述formal Observation构造Candidate后，经既有
  `ConsumeOperationScopeEvidenceV3`与原execute Handoff闭合；
- Tool Owner在exact复读Connection Fact、Receipt、physical Entry、formal Observation、
  prepare/execute Consumption closure后，create-once
  `MCPConnectDomainResultFactV1`；Unknown/Unavailable时零DomainResult；
- Runtime Settlement V4复用公开DomainResult current reader与两项Evidence Binding，
  只引用typed Connect DomainResult；Runtime不选择Tool Outcome/Disposition；
- Tool Apply只能在fresh Runtime V4 Inspection/Association闭合后执行。Connection能力
  在Apply前不得发布给Discovery/Call。

- stdio的物理effect点是`CommandTransport.Connect`启动进程；HTTP的物理effect点是`Client.Connect`发送initialize。二者必须位于受控executor内部。
- admission CAS后崩溃或丢回包：只Inspect原StableKey/Attempt/进程或远端Session；不得新建Attempt或调用`Connect`。
- `Unavailable`、`Indeterminate`和超时不等于`NotFound`；只有权威NotFound且physical admission尚未提交才可继续原canonical command。

## 6. SDK/API/CLI边界

- SDK `Connect`只接收已Seal的Run/Session请求或Application公共DTO，不接受raw `exec.Cmd`、`http.Client`、官方SDK Transport或Credential值。
- CLI `praxis mcp connect`只是Application工作流入口，必须经过同一Runtime链；不提供绕过治理的`--raw-command`或任意Header注入。
- API返回Connection/Receipt/Unknown/Inspect结果；不返回内部官方SDK Session对象。
- owner-local fixture可以使用loopback HTTP与受控测试进程；不得宣称production Network/Credential backend或SLA。

## 7. 硬反例与验收

1. Run/Session任一缺失，或Turn/Action/Context被填充：零process/network。
2. 同Connection坐标跨Run复用、Epoch回退、同Epoch换Server/Config：Conflict，零Provider。
3. Server Descriptor、Transport Config、Provider/Transport Binding、Sandbox、Network、Credential任一漂移：零Provider。
4. prepare/execute复用Evidence Candidate/Handoff：Fail Closed；execute Consumption只能来自Connect后的Observation，不能作为Connect前置或被预填。
5. official HTTP transport自动重连未禁用：Conformance失败。
6. raw `exec.Cmd`、raw `http.Client`、raw Transport、Credential明文/任意Header注入：构造失败。
7. admission后lost reply、ctx取消或SDK error：原Attempt Unknown并Inspect-only，重复投递Provider调用仍为1。
8. 64同canonical并发：单admission/单Connect；64不同Run或Server可并行。
9. TTL在Reader、CAS或actual call-entry穿越、clock rollback：零Provider。
10. stdio stdout非JSON-RPC、超大消息、HTTP redirect/origin/session/protocol漂移：Observation隔离，不形成active Connection/Snapshot。

验证门：contract/unit、whitebox、blackbox、fault、conformance、targeted ordinary×100、race×20、full ordinary/race、vet、gofmt、import boundary与zero raw bypass。

## 8. 当前状态

本设计的owner-local实现与软件门已经闭合：Runtime MCP Connect矩阵/physical authorization、Tool Transport Config/Intent、official SDK stdio/Streamable HTTP executor、Protocol Receipt、正式Observation/Evidence、typed DomainResult、Runtime Settlement V4、Tool ApplySettlement与Connection Availability均已落盘。live Action V2/V3未被复用，lost reply保持Inspect-only。

这不等于production GO：production Network/Credential materializer、durable State Plane backend、Application/composition root、远端部署与SLA仍未落地；CLI/API只有exact只读Inspect，没有绕过治理的Connect写入口。
