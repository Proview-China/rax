# Context Turn Refresh G6B V1 Port Delta

## 1. 状态与用例

- 状态：`candidate-p0`；等待Application、Context与Harness联合审定，当前不授权Harness接入live `ContextTurnRefreshPortV1`，也不授权跨Owner改代码；
- 用例：N=1 Action Gateway在G6A产出settled `ToolResultV2`、current `OperationInspectionSettlementRefV4`及经Runtime公开Inspect验证的完整Association后，由Application协调Context Owner完成S1、Refresh reserve、pending DomainResult、S2、本地原子ApplySettlement/Generation CAS，并只把settled target Frame交给Harness Continuation；
- 现状：live Application `ContextTurnRefreshPortV1`与`ContextTurnRefreshPrepareRequestV1/ApplyRequestV1`当前建模Memory/Knowledge source collection，并强制Memory或Knowledge至少一项非空；它没有承载上述Tool settled exact链，且首方法名为`PrepareContextTurnRefreshV1`；这与已冻结G6B首切面`Tool=1 / Memory=0 / Knowledge=0 / Continuity=0`和`RefreshContextTurnV1`不一致；
- 当前裁决：Application `contract/ports`单测PASS只能证明schema自洽，不能证明与G6B业务合同一致。该漂移闭合前，Harness G6B Adapter、跨模块fixture、Capability、Continuation、Turn推进均为`NO-GO`。

## 2. Owner与非Owner

| 对象/动作 | 语义Owner | 允许 | 禁止 |
|---|---|---|---|
| settled ToolResult | Tool Owner | 公开exact Reader返回current结果 | Receipt/Observation冒充ToolResult |
| V4 Inspection与Association | Runtime Owner | 公开Reader按Operation/Effect与typed ref Inspect | Application或Harness重封Evidence pair |
| RefreshAttempt、TransitionProof、Frame、Generation、Context DomainResult与本地Apply/CAS | Context Owner | 实现Application公开窄Port | Application/Harness mint proof或写Context事实 |
| 跨域顺序与重试 | Application Owner | 只持中立DTO/Port并编排 | import Owner实现、持有Owner Store或成为事实Owner |
| Continuation与Run内Session/Turn | Harness Owner | 只接收已settled target Frame并CAS | 读取SourceTurn内容、补造T+1或绕过Context settlement |
| Memory/Knowledge source | 各领域Owner | 后续additive切面复用唯一公开Reader | 首个G6B切面调用；Harness复制DTO或建立第二current |

## 3. 必须冻结的Application公共面

Application Owner须发布或修正为唯一、版本化的三段窄Port；若Memory/Knowledge source collection仍有独立用例，应改用独立nominal/version，不得复用G6B `ContextTurnRefreshPortV1`的名称和canonical domain：

```go
type ContextTurnRefreshPortV1 interface {
    RefreshContextTurnV1(context.Context, ContextTurnRefreshRequestV1) (ContextTurnRefreshPreparedV1, error)
    ApplyContextTurnRefreshV1(context.Context, ContextTurnRefreshApplyRequestV1) (ContextTurnRefreshResultV1, error)
    InspectContextTurnRefreshV1(context.Context, ContextTurnRefreshInspectRequestV1) (ContextTurnRefreshResultV1, error)
}

type SettledActionContextSourceCurrentReaderV1 interface {
    InspectSettledActionContextSourceCurrentV1(
        context.Context,
        SettledActionContextSourceCurrentRequestV1,
    ) (SettledActionContextSourceCurrentProjectionV1, error)
}
```

请求/投影必须无损携带并逐字段exact验证：

1. Contract version、ExecutionScope digest、Run、Session、Source Turn `T`、expected Target Turn `T+1`；Source Turn full exact ref必须来自Session/Turn Owner Reader；
2. Tool Owner current `ToolResultV2` exact ref及与Action、DomainResult、ApplySettlement的绑定；
3. Runtime current `OperationInspectionSettlementRefV4`；
4. `Inspection.Association`对应的完整`OperationSettlementEvidenceAssociationV4`公开Inspect结果及prepare/execute同Operation、Effect、Attempt、Scope证明；
5. checked/not-after、各Owner current expiry严格最小值及canonical digest；
6. Source cardinality固定`Tool=1 / Memory=0 / Knowledge=0 / Continuity=0`。本版本不得出现要求Memory/Knowledge非空的验证条件，也不得调用两类Reader；
7. 输出只允许中立exact refs、有界summary或exact Artifact `{Owner,Version,Digest,Range}`、Checked/Expires/ProjectionDigest；不得返回Receipt、Provider状态、raw/unbounded output。

Application neutral DTO只协调exact refs，不复制Tool/Context事实内容。Context Adapter允许依赖Application公开`contract/ports`，但不得依赖Application coordinator/kernel；Application和Harness不得import Context实现。

## 4. 固定调用顺序与S1/S2

```text
G6A settled ToolResult current Inspect
-> Runtime V4 Inspection current Inspect
-> full Association public Inspect
-> Application invokes RefreshContextTurnV1
-> Context S1 rereads Session/Turn/Tool/Runtime/Assembly/Authority/ParentFrame current
-> deterministic Attempt/Frame/Generation reserve
-> pending Context DomainResult
-> Context S2 rereads the same closed owner set
-> Context local atomic ApplySettlement + expected Generation CAS
-> Inspect original Attempt on lost reply/Unknown
-> applied_current target Frame exact ref/digest
-> Application invokes Harness Continuation narrow Port
-> Harness rechecks Action closure + target Frame and performs Session CAS
```

Context是TransitionProof唯一Owner；Application只协调。Harness只消费Target/ContextTurn=`T+1`的current Frame，不读取Memory/Knowledge SourceTurn、TransitionProof内容或Owner source，不自行补Turn。

## 5. Effect、Recovery与TTL

- 本Delta不新增Runtime Effect、Permit、Evidence或Settlement类型；Context本地pending DomainResult到Apply/Generation CAS保持Context Owner原子迁移，Runtime Settlement调用为0；
- Provider/Tool Unknown由原Owner Inspect原attempt；Context Refresh或Apply丢回包只`InspectContextTurnRefreshV1`原Attempt，禁止新ID、重跑Tool、重复Apply或盲CAS；
- Unavailable、Indeterminate、cancel、deadline、S1/S2漂移、clock rollback或TTL crossing均Fail Closed，并保持Harness Session在`waiting_action`；
- target Frame/Generation expiry取请求上界与全部可读Owner current窗口严格最小值，不得延长或把cap解释为SLA。

## 6. 硬反例

1. `Tool=0/2`，或首切面`Memory/Knowledge/Continuity`任一非0：零Context写、零Continuation；
2. live Prepare/Apply因Memory与Knowledge同时nil而拒绝合法G6B请求：判定Port语义不兼容，不得由Harness填充假Envelope；
3. Request缺ToolResult、V4 Inspection或完整Association任一exact ref：在Context Reader/Store前Fail Closed；
4. Provider Receipt、Tool Observation或Application payload直接成为ToolResult/ActionResult：拒绝；
5. Source Turn不是Session/Turn Owner exact Reader返回，或Application/Harness/Memory/Knowledge自行`+1`：拒绝；
6. Context未完成pending DomainResult、S2、本地Apply/Generation CAS却返回Frame或调用Continuation：拒绝；
7. Tool/Inspection/Association/Run/Session/Turn/Action/Attempt任一splice、expiry延长或S1/S2漂移：拒绝；
8. lost reply后创建第二Attempt/Frame/Generation、重跑Tool或重复CAS：拒绝；
9. Application import Context/Harness/Tool实现，Harness Assembly具体wiring或复制Owner DTO：import/conformance失败；
10. 把Memory/Knowledge source collector继续命名为G6B `ContextTurnRefreshPortV1`并以同version/domain发布：版本闭包冲突，拒绝兼容映射。

## 7. 兼容、迁移与验收

- 不修改Runtime、Tool、Context、Model或Harness Go；Application Owner负责选择“修正未发布候选V1”或“保留独立Memory/Knowledge nominal并additive发布G6B V1”，但两个语义不得共享同一type/version/domain；
- Context Owner已有A/B owner-local实现与fixture不回滚；迁移只增加Application公共中立面和Context Adapter；
- 当前live Application文件处于共享脏工作树候选状态，Harness不得按其字段先写私有Adapter；
- 联合验收至少包含：Application contract/ports compile；Tool=1/Memory=0/Knowledge=0正例；Memory/Knowledge非0零读反例；三个exact输入缺失/漂移反例；S1/S2、lost reply、TTL/clock、import DAG；G6A PASS后的test-only跨模块fixture；
- G6B fixture不是production root。G6B完整验收与production composition root真实接线Conformance均PASS前，Capability、production Continuation和Turn推进保持`NO-GO`。

