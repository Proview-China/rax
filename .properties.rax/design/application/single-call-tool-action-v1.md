# Application G6A SingleCallToolActionPortV1设计

## 1. 状态与目的

状态：V1 Application owner-local协调实现与测试已完成；但缺少Identity V2、Assembler及system链，不能计为系统G6A或production GO。

兼容说明：V1保留现有字段、canonical与Tool局部payload equality，但缺少Model→SettledTurn→PendingAction Identity及系统Assembler证明，不能冒充完整G6A系统闭环。additive修正见[SingleCallToolAction V2 Identity](./single-call-tool-action-v2.md)；V2联合设计终审已`YES`并冻结，但尚未实现，系统G6A仍为`NO-GO`。

本Delta只解决N=1 G6A的一个问题：Application如何用自己的窄Port和中立DTO，把一个已经由Harness提交并经Owner-current投影确认的单Call PendingAction交给Tool Owner Adapter，并在回包不确定时只恢复原请求，最终取得settled ToolResult坐标、Runtime current V4 Inspection和经公开Inspect核对的Association。

G6A在这里硬停止。Context Refresh、Continuation、Turn推进、Capability启用、Checkpoint、`N>1`和通用Action编排都不属于本合同。

## 2. Owner与非Owner

| 对象/动作 | 唯一Owner | Application只做什么 | 禁止 |
|---|---|---|---|
| Workflow Contract/Step、G6A协调Attempt | Application | 冻结请求、write-ahead、恢复水位、结果坐标 | 生成Tool/Runtime/Harness/Context事实 |
| Session、Turn、PendingAction | Harness | 消费Harness模块内Adapter给出的中立坐标 | import Harness类型或直写Session |
| Model ToolCall Observation Projection | Model Invoker | 通过已终审YES的Model公共只读Projection Reader复读完整Projection Ref与正文 | 持有Model publish/write口、从其他载荷反推Projection或把Observation升级为Action |
| Observation Evidence Record | Evidence Owner | 引用Runtime公开Evidence Record | 把Evidence Record当作Model Projection正文或current证明 |
| Assembly Generation | Harness Assembly | 保存中立Generation坐标 | import Assembly实现或宣称Binding |
| Generation-Binding、Authority、Settlement V4 | Runtime | 只消费Runtime公开typed refs和只读Inspect | 持有`OperationSettlementFactPortV4`或调用Commit |
| ActionCandidate、Reservation、DomainResult、ApplySettlement、ToolResult | Tool Owner | 经`SingleCallToolActionPortV1`协调 | 创作Tool结果或解释Provider Receipt |
| ParentFrame metadata与CTX-D10 applicability source | Context Owner | 分别携带metadata坐标和distinct中立source四元组 | import Context类型、把metadata冒充source或创建新Frame |
| 具体实例wiring | 未来生产composition root | G6A仅用测试组合注入接口 | 宣称生产root已经存在 |

Application Coordinator不是Gateway、Fact Owner或Provider。Tool Adapter必须位于`tool-mcp/applicationadapter`，只依赖Application公开`contract/ports`与其本Owner公开合同。Harness Adapter必须位于`harness`，只负责Owner对象到中立DTO的精确投影。Application任何生产包不得import Harness、Tool或Context模块。

## 3. 版本与公共类型

公共合同版本固定为：

```text
praxis.application.single-call-tool-action/v1
```

这是additive V1，不修改现有Workflow V2、GovernedOperation V3、Runtime Evidence V3、Dispatch V4.0、Enforcement 4.1或Settlement V4摘要。

### 3.1 中立坐标

所有中立坐标都是Application自有nominal type；不得用一个通用`map[string]any`、opaque JSON或无类型字符串替代。每个坐标都有独立`Validate`，防止Run、Session、Frame、Action和Result互相type-pun。

| 类型 | 精确字段 | 说明 |
|---|---|---|
| `SingleCallWorkflowCoordinateV1` | Workflow Contract Version、Plan ID/Revision/Digest、Journal ID/Revision/Digest、Step ID/Kind、`StepDescriptorRefV2`、Workflow Attempt | 绑定exact Contract与Step；Step Kind必须是冻结的单Call governed step |
| `SingleCallRunCoordinateV1` | `core.AgentRunID`、Runtime Run Fact Revision/Digest | 只标识Run，不复制Runtime Run Fact |
| `SingleCallSessionCoordinateV1` | Session ID/Revision/Digest、phase=`waiting_action`、Projection CheckedAt/ExpiresAt | 只标识Harness current projection；观察租约不是领域TTL |
| `SingleCallSessionApplicabilitySourceCoordinateV1` | Kind、ID、Revision、Digest | Application中立nominal镜像；canonical domain固定为Session source，不是Runtime Applicability Fact Ref |
| `SingleCallTurnCoordinateV1` | Turn ID、Ordinal、Revision、Digest | 当前Turn，禁止携带NextTurn或推进标志 |
| `SingleCallTurnApplicabilitySourceCoordinateV1` | Kind、ID、Revision、Digest | Application中立nominal镜像；canonical domain固定为Turn source，不是Runtime Applicability Fact Ref |
| `SingleCallPendingActionCoordinateV1` | Action Ref、Request Digest、Capability、Payload Schema/Payload Digest、Source Candidate ID/Revision/Digest、Projection Digest | 不携带payload bytes；Tool Adapter必须从Owner Reader精确复读 |
| `SingleCallObservationCoordinateV1` | Projection Contract Version、Projection ID/Revision/Digest、Invocation ID/Digest、Observation Digest、Source ResponseID/SourceSequence、Call Count=`1`、独立`EvidenceRecordRefV2` | 一一映射Model公开Projection Ref但不import其类型；Model/Harness Owner Reader必须复读完整Projection，Evidence另由Runtime公开Ref定位 |
| `SingleCallAssemblyCoordinateV1` | Generation ID/Revision/Digest、`GenerationBindingAssociationRefV1`、Tool `ProviderBindingRefV2` | Generation是中立坐标；Binding与Association复用Runtime公共typed ref |
| `SingleCallParentFrameCoordinateV1` | Frame ID/Revision/Digest、Generation ID/Revision/Digest、ExpiresAt | 只绑定产生本次Tool Call的ParentFrame，不是下一Turn Frame |
| `SingleCallParentFrameApplicabilitySourceCoordinateV1` | Kind、ID、Revision、Digest | Application中立nominal镜像；逐字段映射Context Owner CTX-D10 source，canonical domain与ParentFrame metadata、Session/Turn source均不同 |
| `SingleCallToolResultCoordinateV1` | Result ID/Revision/Digest、Action Coordinate Digest、ApplySettlement ID/Revision/Digest、`OperationSettlementRefV4`、Result Schema/Payload Digest、FinalizedAt/ExpiresAt | 只投影settled ToolResult；不复制ToolResult struct或领域Outcome |

`SingleCallWorkflowCoordinateV1`、ExecutionScope digest、Run、Session及其Applicability Source、Turn及其Applicability Source、PendingAction、Observation、Assembly、Binding、Authority、ParentFrame metadata及其CTX-D10 Applicability Source全部进入Request canonical digest。字段不得默认填充；所有ID去除首尾空白后必须保持原值；revision必须非零；digest必须有效；所有时间使用Unix nanoseconds。

### 3.2 RequestV1

候选Go形状如下，字段名是冻结设计，不代表代码已实现：

```go
type SingleCallToolActionRequestV1 struct {
    ContractVersion     string
    ID                  string
    Revision            core.Revision // 必须为1
    Workflow            SingleCallWorkflowCoordinateV1
    ExecutionScope      core.ExecutionScope
    ExecutionScopeDigest core.Digest
    Run                 SingleCallRunCoordinateV1
    Session             SingleCallSessionCoordinateV1
    SessionApplicabilitySource SingleCallSessionApplicabilitySourceCoordinateV1
    Turn                SingleCallTurnCoordinateV1
    TurnApplicabilitySource SingleCallTurnApplicabilitySourceCoordinateV1
    PendingAction       SingleCallPendingActionCoordinateV1
    Observation         SingleCallObservationCoordinateV1
    Assembly            SingleCallAssemblyCoordinateV1
    Authority           runtimeports.AuthorityBindingRefV2
    ParentFrame         SingleCallParentFrameCoordinateV1
    ParentFrameApplicabilitySource SingleCallParentFrameApplicabilitySourceCoordinateV1
    CreatedUnixNano     int64
    ExpiresUnixNano     int64
    Digest              core.Digest
}
```

不变量：

1. `ExecutionScopeDigest`必须由完整`ExecutionScope`按Runtime公共算法重算；恢复后的Instance/SandboxLease/Authority epoch漂移使旧请求失效；
2. `Observation.CallCount`只能等于1；DTO没有Calls数组。Harness Adapter和Tool Adapter必须分别复读完整Observation Projection，`0`或`N>1`在任何Application Attempt、Tool Candidate、Reservation或Provider写入前Fail Closed；
3. PendingAction的Capability、Payload Schema/Digest和Source Candidate必须与Owner Reader精确结果一致；DTO不搬运payload bytes，不能用`OpaquePayloadV2`或JSON绕开Owner Reader；
4. `SessionApplicabilitySource`、`TurnApplicabilitySource`与`ParentFrameApplicabilitySource`必须分别使用冻结的Session/Turn/Context CTX-D10 Kind，且按三个不同canonical domain重算；三个nominal type不可赋值、互换、与`SingleCallParentFrameCoordinateV1`互换，或经通用Object union type-pun。Harness/Input Adapter逐字段映射并复读真实source current；
5. Request不包含、预载或接受Runtime `OperationScopeEvidenceApplicabilityFactRefV3`。后续Tool/Runtime router只能把每个exact source `{Kind,ID,Revision,Digest}`逐字段无损投影为公共ref，绝不重新分配ID、Revision或Digest；Context source公共ref必须由Context Owner Reader复读验证；
6. Assembly Generation、Generation-Binding Association、Tool Provider Binding、Authority和ParentFrame必须属于同一ExecutionScope/Run/Turn可适用集合；
7. `ExpiresUnixNano`不得晚于Session/Turn/ParentFrame CTX-D10 source current投影、Observation来源、Generation-Binding currentness、Authority currentness、ParentFrame metadata及Policy允许窗口的最短值；
8. `ID`由除`ID/Digest`外的canonical request subject确定性派生。相同subject不得换ID；同ID换任一内容必须Conflict；
9. canonical禁止map、浮点时间和不稳定集合；本合同无可选slice，未来增加集合必须排序、去重并在新版本定义nil/empty规则。

### 3.3 ResultV1

```go
type SingleCallToolActionResultV1 struct {
    ContractVersion             string
    ID                          string
    Revision                    core.Revision // 必须为1
    RequestID                   string
    RequestRevision             core.Revision
    RequestDigest               core.Digest
    ToolResult                  SingleCallToolResultCoordinateV1
    Inspection                  runtimeports.OperationInspectionSettlementRefV4
    Association                 runtimeports.OperationSettlementEvidenceAssociationRefV4
    AssociationCheckedUnixNano  int64
    ExpiresUnixNano             int64
    Digest                      core.Digest
}
```

Result只允许以上G6A闭包：

- `Inspection.Settlement`必须与`ToolResult.Settlement`exact相同；
- `Association`必须与`Inspection.Association`exact相同；
- Tool Adapter必须调用Runtime Application-facing公开`InspectOperationSettlementEvidenceAssociationV4`，返回Fact的Ref必须exact等于`Association`，且完整prepare/execute独立、同Attempt；
- `AssociationCheckedUnixNano`必须位于Inspection current窗口内，Result `ExpiresUnixNano <= Inspection.ExpiresUnixNano`；
- `ToolResult.ActionCoordinateDigest`必须由Request中的PendingAction、Observation、Run/Session/Turn canonical坐标重算；
- Result `ID`必须由Request ID/Digest、ToolResult、Inspection与Association的canonical subject确定性派生；同subject换ID无效，同ID换内容Conflict；
- Result不得含Context Frame、Continuation、NextTurn、Turn advancement、Capability activation、Provider Receipt或领域Fact正文。

### 3.4 Observation Ref一一映射

Application不import Model Invoker包，但`SingleCallObservationCoordinateV1`必须无损映射live `ToolCallCandidateObservationRefV1`：

| Application中立字段 | Model Owner公开Ref字段 | 校验 |
|---|---|---|
| `ProjectionID` | `Ref.ID` | 非空、exact |
| `ProjectionRevision` | `Ref.Revision` | 必须为1 |
| `ProjectionDigest` | `Ref.Digest` | Model Owner Reader重算后exact |
| `InvocationID` | `Ref.InvocationID` | 非空、exact |
| `InvocationDigest` | `Ref.InvocationDigest` | exact |
| `ObservationDigest` | `Ref.ObservationDigest` | exact等于Projection内Observation digest |
| `SourceResponseID` | `Ref.Source.ResponseID` | 保留空值或原值，不默认生成 |
| `SourceSequence` | `Ref.Source.SourceSequence` | 必须大于0、exact |
| `Evidence` | Runtime `EvidenceRecordRefV2` | 另一个独立exact字段，不替代Model Projection Ref |
| `CallCount` | `len(Projection.Observation.Calls)` | 必须等于1 |

Projection Contract Version也进入Application coordinate digest，用于选择正确Model Owner Reader；它不是Application对Model合同的重新声明。Harness Owner Adapter负责把Model公开Ref逐字段投影到Application中立坐标，并在`SingleCallToolActionInputCurrentReaderV1`的S1与S2中调用已由Model Owner实现且终审YES的公开只读Projection Reader。该Reader必须按完整Projection Ref逐字段读取完整Projection，重算Projection/Invocation/Observation digest并验证`len(Projection.Observation.Calls)==1`。Reader不可用、Ref任一字段漂移、Observation digest漂移或Calls不等于1，均在任何Tool command前Fail Closed。

当前live Model Invoker尚无该exact Reader；Application资产只冻结依赖边界，不命名其最终Model包接口，也不在Application内发明实现、缓存副本或兼容投影。Application与Coordinator不持有Model publish/write口，不得从PendingAction payload、Harness event JSON、compat tool calls或Runtime Evidence Record反推/重建Projection。

## 4. Port与只读依赖

```go
type SingleCallToolActionPortV1 interface {
    ExecuteSingleCallToolActionV1(context.Context, SingleCallToolActionRequestV1) (SingleCallToolActionResultV1, error)
    InspectSingleCallToolActionV1(context.Context, InspectSingleCallToolActionRequestV1) (SingleCallToolActionResultV1, error)
}

type InspectSingleCallToolActionRequestV1 struct {
    RequestID      string
    RequestDigest  core.Digest
    ScopeDigest    core.Digest
}

type SingleCallToolActionInputCurrentReaderV1 interface {
    InspectSingleCallToolActionInputCurrentV1(
        context.Context,
        SingleCallToolActionRequestV1,
    ) (SingleCallToolActionInputCurrentProjectionV1, error)
}

type SingleCallOperationSettlementCurrentReaderV1 interface {
    InspectCurrentOperationSettlementV4(
        context.Context,
        runtimeports.InspectCurrentOperationSettlementRequestV4,
    ) (runtimeports.OperationInspectionSettlementRefV4, error)
    InspectOperationSettlementEvidenceAssociationV4(
        context.Context,
        runtimeports.OperationSubjectV3,
        runtimeports.OperationSettlementEvidenceAssociationRefV4,
    ) (runtimeports.OperationSettlementEvidenceAssociationV4, error)
}
```

`Execute`是同canonical command的start-or-inspect入口，不是exactly-once transport、Permit或Enforcement。Tool Adapter必须先create/Inspect自己的request/coordination watermark；重复收到同Request ID/Revision/Digest/Scope时只Inspect或继续内部恢复，只有其内部权威水位证明Provider边界从未开始时才能继续。Provider调用后出现任何未知结果，只Inspect原attempt，不能因为Application重投command再次调用Provider。Tool Adapter内部仍须走Runtime Dispatch V4.0、Enforcement 4.1、Evidence V3两phase、Tool DomainResult、Runtime Settlement V4和Tool ApplySettlement。`Inspect`只读原Request ID，不接受替代ID或新payload。

`SingleCallToolActionInputCurrentProjectionV1`是Application sealed neutral projection，必须逐字段返回Request中的Session、`SingleCallSessionApplicabilitySourceCoordinateV1`、Turn、`SingleCallTurnApplicabilitySourceCoordinateV1`、PendingAction、Observation、Assembly Generation/Binding、Authority、ParentFrame metadata与`SingleCallParentFrameApplicabilitySourceCoordinateV1`，以及Request ID/Revision/Digest、Scope Digest、CheckedAt、ExpiresAt和Projection Digest。三个source coordinate各自使用不同静态类型和canonical domain；S1/S2若互换、与ParentFrame metadata或Runtime公共ref互换、Kind错误或任一ID/Revision/Digest漂移都必须拒绝。它不返回任何Owner struct或写接口。`SingleCallToolActionInputCurrentReaderV1`由Harness Owner Adapter实现，并通过注入的Owner-current Readers聚合只读投影：ParentFrame source必须委托Context Owner CTX-D10 Reader按四字段及完整scope逐字段复读，Observation必须委托已终审YES的Model公共只读Projection Reader按完整Projection Ref复读完整Projection并验证Calls恰为1；不得用PendingAction、event JSON或compat tool calls代替。Application在首次command前执行S1，在接受Result前执行S2；Reader不可用或S1/S2任一字段、digest、Calls漂移均Fail Closed，且在S1闭合前Tool command调用数必须为0。该投影不宣称跨Owner原子事务，其短观察租约取全部来源最短值，包括CTX-D10 projection expiry。

Applicability链保持单向且不扩权：

```text
Harness真实Session/Turn source coordinate + Context Owner CTX-D10 ParentFrame source coordinate
-> Harness/Input Adapter逐字段映射Application三个distinct neutral coordinates
-> Application Request + InputCurrentProjection S1/S2
-> Tool/Runtime router逐字段无损投影OperationScopeEvidenceApplicabilityFactRefV3
-> Harness-backed或Context-backed OperationScopeEvidenceApplicabilityCurrentReaderV3复读对应source current
```

公共Applicability ref只是一组定位坐标，不授Evidence资格、Permit或Enforcement。Application不预载该公共ref；Runtime现有FactPort没有Applicability Fact的Create/Inspect路径，本设计也不新增或暗示Runtime创建Applicability Fact。资格仍由Evidence Owner按既有Policy、Qualification和current Reader链判断。

Application也不持有、缓存或复述Tool Owner的Provider Boundary proof；该proof只由Runtime受控Provider seam通过Tool Owner公开current Reader复读，不进入Request、InputCurrentProjection或Result。

`SingleCallOperationSettlementCurrentReaderV1`是另一个独立最小只读接口，只暴露Runtime Gateway的current Inspection和public Association Inspect。它不得与Input Reader合并，也不得包含`SettleOperationV4`、`OperationSettlementFactPortV4`或任何Commit方法。Application不持有更宽的Runtime Governance写接口。

## 5. 最小Coordinator与恢复状态机

Application拥有`SingleCallToolActionCoordinationFactV1`，只记录协调水位：Request ID/Revision/Digest、Scope/Workflow/Step、Session/Turn applicability source坐标、状态、可选Result ref、时间。它不是Tool Action、Effect、Settlement或Outcome。

```text
零写前Validate + N==1 + InputCurrentReader S1
  -> create-once prepared(rev1)
  -> CAS dispatch_intent(rev2)     # 必须早于Port Execute
  -> InspectSingleCallToolActionV1(original canonical key)
       exact result -> S2 + Runtime closure -> completed
       authoritative NotFound + same InputCurrentReader current
               -> submit exact same canonical Execute command
       unavailable/indeterminate -> waiting_inspect, no command submit
  -> ExecuteSingleCallToolActionV1(same ID/revision/digest/scope only)
       success -> Runtime current Inspection + public Association re-read
               -> CAS completed(result exact ref)
       error/timeout/lost reply
               -> CAS waiting_inspect
               -> InspectSingleCallToolActionV1(original request ID/digest/scope)
                    exact result -> Runtime current/public Association re-read -> completed
                    authoritative NotFound + same current -> resubmit same canonical command
                    unavailable/indeterminate/drift -> stay waiting_inspect
```

恢复规则：

1. `dispatch_intent`后恢复必须先调用Tool Owner `Inspect`。只有同Owner权威返回NotFound，且`InputCurrentReader`再次证明全部输入仍exact current时，Application才可重投**同Request ID/Revision/Digest/Scope**的canonical command；永不换ID、内容或Scope；
2. create/CAS/Execute/Inspect回包丢失都先Inspect本Owner原Fact。Unavailable/Indeterminate或Inspect unavailable时保持`waiting_inspect`，不得重投；
3. Tool Adapter的`Execute`必须是start-or-inspect：先线性化/Inspect Tool自己的request watermark，再依据内部阶段恢复。crash-before-first-call允许同canonical command继续；lost-reply-after-provider只能Inspect原attempt，Provider调用次数不得因command重投增加；
4. 不承诺exactly-once transport。合同只承诺同canonical command幂等、Tool Owner阶段化恢复和Provider边界后不盲重派；
5. Tool Adapter返回Result不代表Application完成。Coordinator必须执行InputCurrentReader S2，并在完成CAS前复读Runtime current Inspection及完整Association；过期、NotFound、Reader unavailable或任一exact ref漂移保持`waiting_inspect`；
6. Application不解释Tool Outcome，不从Receipt/Observation构造Result，不补写Tool/Runtime事实；
7. G6A completed只表示三元闭包已核对，不调用Context/Harness，不注册Capability，不写Continuation，不推进Turn。

## 6. Import DAG与落点

```text
runtime/core + runtime/ports
          ^
          |
application/contract/single_call_tool_action_v1.go
          ^
          |
application/ports/single_call_tool_action_v1.go
          ^                         ^
          |                         |
application/single_call_tool_action_coordinator_v1.go
                                    |
tool-mcp/applicationadapter/single_call_tool_action_v1.go

harness/applicationadapter  -> application public contract/ports

future host production composition root
  -> constructs Tool/Harness adapters and injects interfaces
```

禁止边：

- `application/** -> harness/**`；
- `application/** -> tool-mcp/**`；
- `application/** -> context-engine/**`；
- Tool domain/kernel导入Application；只有`tool-mcp/applicationadapter`可以导入Application公开contract/ports；
- Harness Assembly实例化Tool/Context Adapter；Assembly只接收已注入接口。

当前live仓库没有生产composition root。G6A实现只冻结接口、中立DTO、Application协调Fact和测试组合注入边界；测试中可以手工组合Fake/Adapter。Context等Owner-local实现可使用本Owner fixture；G6A PASS后，G6B test-only跨模块fixture也可手工注入Application公共Port与各Owner Adapter，但不是production root，不启用Capability、不生产调用Continuation或推进Turn。生产root的具体包、进程与生命周期由宿主Owner在G6B完整验收后的production enablement前联合确定、实现、验收并完成真实接线Conformance，不反向阻塞G6A、Owner-local实现或G6B fixture。

## 7. 候选实现文件

联合评审YES后，Application Owner候选文件为：

```text
ExecutionRuntime/application/contract/single_call_tool_action_v1.go
ExecutionRuntime/application/ports/single_call_tool_action_v1.go
ExecutionRuntime/application/single_call_tool_action_coordinator_v1.go
ExecutionRuntime/application/fakes/single_call_tool_action_v1.go
ExecutionRuntime/application/conformance/single_call_tool_action_v1.go
ExecutionRuntime/application/tests/single_call_tool_action_contract_v1_test.go
ExecutionRuntime/application/tests/single_call_tool_action_coordinator_v1_test.go
ExecutionRuntime/application/tests/single_call_tool_action_conformance_v1_test.go
ExecutionRuntime/application/tests/single_call_tool_action_imports_v1_test.go
```

跨Owner候选文件由各Owner另行实现，不属于Application写入范围：

```text
ExecutionRuntime/tool-mcp/applicationadapter/single_call_tool_action_v1.go
ExecutionRuntime/harness/applicationadapter/single_call_tool_action_v1.go
```

## 8. G6A/G6B分段裁决

G6A技术门与本资产中央联合评审YES后，可以按现有总授权实现Application合同、Coordinator、Fake、Conformance和测试组合；输出硬停在settled ToolResult中立坐标、current V4 Inspection与public Association exact验证。

只有G6A全部unit/whitebox/blackbox/fault/conformance/race PASS后才进入G6B test-only跨模块fixture。Owner-local Context实现可独立使用本Owner fixture；G6B fixture只手工注入公共Port/Adapter并验证链，不启用Capability、不生产调用Continuation或推进Turn。G6B完整验收与真实production root接线Conformance同时PASS后，production Capability、Continuation和Turn推进才可GO。`N>1`、通用Refresh与Checkpoint继续冻结到G7。

## 9. 相关资产

- [Application设计入口](./README.md)
- [G6A测试矩阵](./single-call-tool-action-v1-test-matrix.md)
- [调用与依赖图](./single-call-tool-action-v1.drawio)
- [实施计划](../../plan/application/single-call-tool-action-v1.md)
- [既有P0计划](../../plan/application/p0-execution-coordinator.md)
- [Harness Action Gateway设计](../harness/assembly/action-gateway-v1.md)
- [Tool Port Delta](../tool-engine/port-delta.md)
