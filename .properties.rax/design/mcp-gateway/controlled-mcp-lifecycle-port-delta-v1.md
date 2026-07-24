# Controlled MCP Lifecycle Port Delta V1

## 1. 状态

- 状态：Tool/MCP Owner联合候选；尚未获得Runtime/Application独立终审，Go门关闭。
- 作用：把ordinary Call Cancel与Connection Close变成独立受治理Effect；Drain仍是Tool Owner
  本地CAS，不进入Runtime physical Port。
- 禁止：不得复用或包装Action Execute、Connect、Discovery Page或legacy MCP Port扩权。

## 2. Live公共合同缺口

Runtime当前已经发布：

- `OperationScopeEvidenceActionMatrixV3()`；
- `OperationScopeEvidenceMCPConnectMatrixV1()`；
- `OperationScopeEvidenceMCPDiscoveryPageMatrixV1()`；
- Connect/Discovery Page各自Route、Authorization Request/Authorization Port。

但是live `OperationScopeEvidenceApplicabilityMatrixKeyV3.Validate()`只登记Action与Connect，
`isRegisteredOperationScopeEvidencePolicySubjectV3`也只登记`praxis.tool/execute`与
`praxis.mcp/connect`；Discovery Page的`praxis.mcp/discover`尚未进入两个通用注册闭集。
因此production统一Evidence Policy/Gateway在新增Cancel/Close前必须先修正这一既有additive
registry gap，并增加“所有公开矩阵均通过通用Validate/Policy subject”的Conformance测试。
Tool不修改Runtime文件，也不私建兼容注册表。

## 3. OperationScope Evidence矩阵Delta

| 矩阵 | Operation | Effect | Profile | Run | Session | Turn | Action | Context |
|---|---|---|---|---|---|---|---|---|
| Ordinary Call Cancel V1 | `run` | `praxis.mcp/cancel` | `praxis.mcp/ordinary-call-cancel-v1` | required | required | required | required | required |
| Connection Close V1 | `run` | `praxis.mcp/close` | `praxis.mcp/run-connection-close-v1` | required | required | forbidden | forbidden | forbidden |

候选Runtime public文件：

- `runtime/ports/operation_scope_evidence_mcp_cancel_v1.go`；
- `runtime/ports/operation_scope_evidence_mcp_close_v1.go`；
- additive更新通用Matrix/Policy subject注册闭集，不改既有常量值和digest算法。

Cancel复用五维Owner source种类，但Action/Context必须仍是原`tools/call`所属Action与parent
Context exact current，不是Cancel Intent自报。Close复用Connect的Run/Session Owner source种类，
Turn/Action/Context必须显式forbidden。两者均需要独立`Is*MatrixKey`、routes、applicability
validator与current router nominal。

## 4. Ordinary Call Cancel public Port候选

### 4.1 Tool-neutral current source

Runtime只消费中立当前证明，不持有Go `context.CancelFunc`：

```text
MCPActiveCallCancellationRefV1
  {ID, Revision, Digest}

MCPActiveCallCancellationProjectionV1
  {ContractVersion, Ref, Command, OriginalAttempt, ConnectionAvailability,
   ConnectionEpoch, JSONRPCRequestID, State, CheckedUnixNano,
   ExpiresUnixNano, ProjectionDigest}

MCPActiveCallCancellationCurrentReaderV1
  InspectCurrentMCPActiveCallCancellationV1(ctx, exactRef) -> projection
```

`State`闭集为`active|terminal|lost`。只有`active`可用于Cancel authorization；Projection不携
取消函数、不授Authority。真实cancel handle只存在Tool physical executor内部的唯一active-call
repository，并按exact Ref读取。进程重启后handle缺失为`lost`，Cancel零发送；原Call仍保持其
Observation/Unknown语义。

### 4.2 Authorization Request

候选`ControlledMCPOrdinaryCallCancelPhysicalAuthorizationRequestV1`字段：

- Cancel专属Route current Ref；
- Cancel自身`ExecutePreparedRequestV2`、`OperationDispatchAttemptRefV3`；
- execute `OperationDispatchEnforcementPhaseRefV4`；
- prepare `OperationScopeEvidenceConsumptionRefV3`与execute
  `OperationScopeEvidenceProviderHandoffRefV3`；
- `PreparedDomainCommandAssociationRefV1`与Cancel Intent映射的
  `OperationDomainCommandRefV1`；
- exact原`MCPExecutionCommandRefV1`、原`OperationDispatchAttemptRefV3`；
- `MCPConnectionAvailabilityNeutralRefV1`、`MCPActiveCallCancellationRefV1`；
- 原JSON-RPC Request ID digest、Cancel reason digest、caller deadline。

Request验证必须证明Cancel Operation坐标彼此一致，并且原Command/Attempt/Connection/active
handle属于同一original call。Cancel reason只进入Cancel Intent/digest，不进入日志原文。

### 4.3 Authorization与physical entry

候选`ControlledMCPOrdinaryCallCancelPhysicalAuthorizationV1`除Request闭包外必须携：

- `StableKeyDigest, UnifiedNotAfterUnixNano`；
- Route解析的七Binding水位、ProviderTransport、Provider；
- Operation/Scope/Effect/Intent/Prepared/Attempt exact坐标；
- execute Enforcement、prepare Consumption、execute Handoff；
- Sandbox/Credential digests；
- 原Command/Attempt、Connection Availability、ActiveCancellation；
- `IssuedUnixNano, AuthorizationDigest`。

Provider capability必须exact为`praxis.mcp/cancel`；Transport capability沿用已冻结的受控MCP
transport capability，但Provider与Transport仍必须是不同binding。stable key至少绑定Cancel
Operation digest/Attempt/DomainCommand/Route、原Command/Attempt、Connection Availability和
ActiveCancellation Ref；fresh时间不进入stable key。

候选Ports：

```text
ControlledMCPOrdinaryCallCancelPhysicalAuthorizationPortV1
  AuthorizeControlledMCPOrdinaryCallCancelPhysicalV1(ctx, request) -> authorization

ControlledMCPOrdinaryCallCancelPhysicalPortV1
  ExecuteControlledMCPOrdinaryCallCancelPhysicalV1(ctx, authorization) -> admission receipt
```

Tool actual point必须在同一函数内fresh clock、复读Authorization/Intent/Command/Connection/
ActiveCancellation exact current、create-once admission后直接调用内部handle；不能验证后再转发
给raw Session/handle wrapper。admission后崩溃或回包丢失只Inspect同stable key，不重发取消。

## 5. Connection Close public Port候选

### 5.1 Tool-neutral输入

Close必须绑定：

- exact Connection Fact/Epoch与`MCPConnectionAvailabilityNeutralRefV1`；
- exact `MCPConnectionDrainRefV1`，默认必须处于`drained`；
- initialized official Session binding的Tool-neutral current Ref；
- Close Intent/DomainCommand、Server、ProviderTransport、Provider与caller deadline。

“紧急Close可跳过drained”不是布尔参数；若未来需要，必须由独立Policy profile与Review/Authority
事实表达。本V1只有drained路径。

### 5.2 Runtime candidates

候选文件`runtime/ports/controlled_mcp_close_v1.go`，形状与Connect相似但类型完全独立：

- `ControlledMCPConnectionCloseRouteCurrentRefV1/ProjectionV1/ReaderV1`；
- `ControlledMCPConnectionClosePhysicalAuthorizationRequestV1`；
- `ControlledMCPConnectionClosePhysicalAuthorizationV1`；
- `ControlledMCPConnectionClosePhysicalAuthorizationPortV1`；
- `ControlledMCPConnectionClosePhysicalPortV1`。

Authorization必须绑定Run/Session applicability、Route/BindingSet/Provider、execute Enforcement、
prepare Consumption、execute Handoff、Connection Availability、Drain、Session current与统一
NotAfter。Provider capability exact为`praxis.mcp/close`。Tool actual pointfresh复读后create-once
admission并直接调用同一official SDK Session `Close()`；不得把DELETE/进程kill/raw Transport作为
调用方可注入替代路径。

## 6. Drain Tool Owner合同候选

候选Tool文件：

- `contract/mcp_drain_v1.go`：Ref/Fact/Current Projection/Reader与closed state；
- `mcp/drain_repository_v1.go`：唯一history/current Store、create/CAS/Inspect；
- `runtimeadapter/mcp_drain_current_v1.go`：未来Close Runtime-neutral只读映射。

状态只允许`requested -> draining -> drained|residual`。创建Drain后，新的Connect reuse、Discovery
Page和Call actual point都必须复读Drain current；`draining|drained|residual`零Provider。`drained`
要求active-call index为空且每个已admit Attempt都有terminal或Unknown+Residual解释。Drain不调用
Provider、不创建Runtime Settlement、不删除历史。

## 7. Error、恢复和Settlement边界

| 场景 | 分类 | 结果 |
|---|---|---|
| exact source NotFound且尚未admit | `not_found` | 零physical effect；不代表原Call未执行 |
| Reader/Gateway unavailable | `unavailable` | 零新动作，不等于NotFound |
| ref/digest/epoch/attempt漂移 | `conflict` | 零physical effect |
| clock rollback/TTL crossing | `precondition_failed` | 零physical effect |
| admission后回复丢失 | `indeterminate` | 只Inspect原Entry，零redispatch |
| Cancel sent/Close called | Observation/Receipt | 不直接成为DomainResult或原Call结论 |

Cancel和Close各自仍需Tool typed DomainResult→Runtime Settlement V4→Tool Apply；Runtime只绑定
typed结果，不选择Tool Outcome/Disposition。Cancel成功永远不能把原Call写成
`confirmed_not_applied`。Close成功也不能证明远端Effect回滚。

## 8. 兼容影响与实施顺序

该Delta为additive；既有Action/Connect/Discovery digest和Port不变。实施顺序固定：

1. Runtime补齐Discovery通用矩阵注册缺口及Conformance；
2. Runtime冻结Cancel/Close matrices、Route与physical ports；
3. Tool实现active-call current与Drain合同/Store，但写入口保持disabled；
4. Tool实现Cancel/Close actual executor及Receipt/Inspect；
5. Runtime/Application闭合Evidence/Settlement调度；
6. SDK/API/CLI才开放写命令；
7. production root/backend、Credential、cleanup与系统测试完成后再讨论启用。

## 9. 硬反例

- Discovery public matrix不能通过通用Validate/Policy subject；
- Cancel复用`praxis.tool/execute`或原Call authorization；
- Close复用Connect authorization或Action五维矩阵；
- Cancel/Close缺任一required/forbidden applicability；
- 原Command/Attempt/Request ID/Epoch/handle任一漂移仍触达Provider；
- active handle已lost/terminal仍发送Cancel；
- Drain有active/Unknown未解释Attempt仍变drained；
- draining以后新Call/Discovery进入Provider；
- Close未绑定drained Fact或同一Session current；
- admission后lost reply再次调用Cancel/Close；
- typed-nil、nil/canceled ctx、clock rollback、TTL crossing触达Provider；
- 64同canonical产生多Intent/admission/physical动作；
- Runtime/Application/Harness import Tool实现，或Tool import其kernel/fakes/internal。

