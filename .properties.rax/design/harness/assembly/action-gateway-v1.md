# Harness Assembly N=1 Action Gateway V1

## 1. 裁决、范围与实现门

Identity前置：V1流程保留，但完整system G6A还必须满足additive [Model Tool Call → PendingAction Identity V1](./model-tool-call-pending-action-identity-v1.md)。Identity设计、Harness Owner-current V3/V4及P3 Assembler/InputCurrent Reader最终独立代码审计均已`YES(P0/P1/P2=0)`；Tool V2 Consumer/P4与system fixture尚未实现，Owner-local通过不能冒充系统闭环。

本设计冻结Harness侧首个单Call垂直链。它只覆盖一个已完成Model Turn恰好产生一个Tool Call的情况，不是P3b Hook运行面，也不提供多Call、批处理、Checkpoint、per-turn refresh或真实Provider Backend。

- 首切面：`ToolCallCandidateObservationV1.Calls`必须**恰好为1**；
- `N>1`：保存完整Observation与Run Evidence V2后Fail Closed，不取首项、不拆分、不创建`PendingAction`；
- P3b：继续NO-GO，禁止以通用Hook、Filter或Application私有对象绕过本链；
- Evidence V3：公开合同与首批activation矩阵已有live基线；本设计新增的Action矩阵必须由Runtime Owner显式登记；
- G6A：各Owner-local合同可在自身门通过后隔离实现/测试；真实Provider cross-module fixture还必须等待Runtime V2、Tool Adapter、[HA-X02强类型Route](./controlled-operation-provider-route-v2.md)实现/Conformance、source-coordinate、Model exact Reader、Settlement V4、Action矩阵、Runtime nominal router、ToolResult V4和Application窄Port边界全部PASS。输出严格止于settled ToolResult + current V4 Inspection + public Association Inspect，不启用能力、不调用Continuation、不推进Turn；
- G6B：Context Owner-local实现可用本Owner fixture独立进行；只有G6A隔离验收PASS后才可创建test-only跨模块fixture并验证Context Refresh与Harness Continuation Adapter合同。G6B完整验收与真实root接线Conformance同时PASS前，能力启用、production Continuation和Turn推进保持NO-GO；
- PendingAction Reader、Runtime/Tool/Model前置、Route、Identity、[Owner-current exact输入Delta](../port-deltas/committed-pending-action-owner-current-inputs-v2.md)对应V3/V4实现及Harness P3 Adapter最终独立代码审计均为`YES`。H-ID-P0/P1/P2/P4已完成；Tool Consumer/P4与system fixture未完成，system G6A保持`NO-GO`；不得把Owner-local通过替代系统实现验收。
- G6B公共合同当前有一项live P0：Application候选`ContextTurnRefreshPortV1`建模Memory/Knowledge source并要求至少一项非空，未承载settled ToolResult/V4 Inspection/full Association，与首切面`Tool=1 / Memory=0 / Knowledge=0 / Continuity=0`不兼容；见[G6B Context Turn Refresh Port Delta](../port-deltas/context-turn-refresh-g6b-v1.md)。闭合前Harness不得先写私有兼容Adapter。

## 2. 固定权威链

```text
ToolCallCandidateObservationV1 (Calls exactly 1)
-> Run Evidence V2 append / exact Inspect
-> Model-turn Settlement Owner independent Inspect
-> SettledTurnResultV2(action_required)
-> Runtime Operation Settlement V3
-> Harness ApplySettledTurnV2
-> Session CAS(waiting_settlement -> waiting_action) + PendingActionV2
-> Tool Owner ActionCandidate + ActionReservationFact
-> Runtime Dispatch V4 Admission / Review / Permit / Begin
-> Enforcement 4.1 prepare + Evidence V3 prepare consume
-> Enforcement 4.1 execute + Evidence V3 execute consume
-> Tool Owner DomainResultFact
-> Runtime Operation Settlement V4
-> Tool Owner ApplySettlement -> ToolResult
-> Action Gateway output: settled ToolResult + current V4 Inspection + full Association Inspect
-> ContextTurnRefreshPortV1 S1 / reserve / collect / admit / freeze
-> pending Context DomainResultFact
-> Context S2 owner-current reread
-> Context Owner local atomic ApplySettlement + expected Generation CAS
-> settled new Frame exact ref/digest
-> Harness action Continuation CAS
-> Session CAS(waiting_action -> waiting_model_dispatch, turn + 1)
```

两条Settlement不能混用：Model Turn到`PendingAction`继续使用现有Runtime Settlement V3；Tool Effect必须使用Settlement V4，因为只有V4精确绑定prepare与execute两份Evidence V3。`DomainResultFact -> Runtime Operation Settlement V4(ref only) -> Tool ApplySettlement`的Owner分层不可合并。

Provider Receipt、stream、completed、cache usage、Tool Call Observation、Evidence消费和Enforcement Receipt都只是Observation或治理事实，任何一个都不能直接成为`PendingAction`、`ActionResult`、`ToolResult`或Harness Continuation。

原始流程图见[Action Gateway V1流程图](./action-gateway-v1.drawio)，详细反例见[Action Gateway V1测试矩阵](./action-gateway-v1-test-matrix.md)。

## 3. Owner与非Owner

| 对象/动作 | 唯一Owner | Application允许 | 明确禁止 |
|---|---|---|---|
| `ToolCallCandidateObservationProjectionV1`及完整`ToolCallCandidateObservationRefV1` | Model Invoker | Harness Adapter仅通过已终审YES的Model公共exact Reader按完整Ref复读Projection并验证`Calls==1` | 从PendingAction payload、event JSON或compat tool calls反推Projection；设计Model写口、Repository或`Ensure`实现 |
| Run Evidence V2 | Runtime Evidence Owner | Append后按source exact Inspect | 自分配sequence、把Observation升级为领域Fact |
| `SettledTurnResultV2` | Model-turn绑定的Settlement Owner | 请求独立Inspect并转交exact结果 | 代写`action_required`、自行解释Tool语义 |
| Session、Turn、`PendingActionV2`、Continuation CAS | Harness | Harness模块内Adapter实现Application公开窄Port | Application直接写Store、绕过ExpectedRevision |
| `ActionCandidate`、Reservation、`DomainResultFact`、`ToolResult`、ApplySettlement | Tool Owner | Tool模块内Adapter实现Application公开窄Port | Application import Tool、从Observation直建Candidate、从Receipt直建Result |
| Operation、Permit、Begin、Enforcement、Evidence V3、Settlement V3/V4 | Runtime | 调用公开Governance/Inspect Port | 写Runtime Fact、伪造Permit/Evidence/Settlement |
| Context Refresh、Frame、Generation、ApplySettlement | Context Owner | Context模块内Adapter实现Application公开窄Port并调用`ContextTurnRefreshPortV1` | Application import Context；Harness/Application重封Frame、跳过Context本地原子ApplySettlement/Generation CAS或宣称current |
| 跨域顺序与恢复 | Application Coordinator | 只经Application自有窄Port和中立DTO组织同一canonical请求 | import Harness/Tool/Context、成为事实Owner、宣称跨Owner原子事务 |
| Owner-local隔离实现 | 各Owner fixture | Context等Owner可在自身模块内隔离实现/测试公共合同 | 要求production root才允许Owner-local实现 |
| G6B test-only跨模块组合 | 显式test composition/fixture | G6A PASS后手工构造各Owner Adapter并注入公共Port；可验证完整链但不启用生产能力 | 宣称fixture是production root，或注册Capability、生产调用Continuation/推进Turn |
| production composition root | 宿主Owner（G6B生产启用残余） | 当前不存在；G6B完整验收后完成真实接线与Conformance | 用G6A/G6B fixture冒充生产root，或把具体wiring下沉到Harness Assembly |

Harness只拥有Run-local状态。它不拥有Runtime Run/Outcome、Tool领域结果、Context Frame、Evidence、Permit、Enforcement或Settlement。

### 3.1 Model Projection exact入口门

Harness Adapter形成Application Request或复读Input前，必须调用已由Model Owner实现并终审YES的公共只读Projection exact Reader，以完整`ToolCallCandidateObservationRefV1`读取`ToolCallCandidateObservationProjectionV1`，校验返回Ref全字段未变、Projection lineage/digest有效且`len(Observation.Calls)==1`。Reader unavailable、返回Ref变化或`Calls!=1`时，Application Request不得形成，Application dispatch计数必须为零。

PendingAction payload、Harness event JSON与individual/compat tool-call事件都不是权威Projection Reader，禁止由它们反推或拼装Model Projection。本设计只消费Model公共只读Reader，不定义Model写口、Store、Repository或原子`Ensure`实现。

## 4. 封闭Action矩阵

唯一允许的首切面矩阵为：

| 字段 | 固定值 |
|---|---|
| `OperationScopeKind` | `run` |
| `EffectKind` | `praxis.tool/execute` |
| `PolicyProfile` | `praxis.tool/single-call-action-v1` |
| Run | `required` |
| Session | `required` |
| Turn | `required` |
| Action | `required` |
| Context | `required` |

Runtime Evidence V3当前live校验仍只接受activation首批矩阵；因此上述Action矩阵在Runtime公共Catalog、五维Owner-current Reader装配和Conformance通过前必须Fail Closed。不得用`custom`、旧activation profile、空Context或“可选Action”绕开。

## 5. Owner source coordinates、公共nominal refs与current Reader

`OperationScopeEvidenceApplicabilityFactRefV3`是对外部领域Fact/source的中立引用，不是Runtime另存的Applicability Fact。live `OperationScopeEvidenceFactPortV3`没有Applicability Fact的Create/Inspect；本设计禁止发明该能力。领域Owner Reader返回本Owner真实可复读的current事实或source coordinate；Runtime G6A Action applicability router只把exact `Kind/ID/Revision/Digest`无损投影为公共ref，不创作新ID/Digest，也不声称产生新的Runtime Fact。

| 维度/链上对象 | exact ref来源 | current Reader Owner | current条件 |
|---|---|---|---|
| Run | Runtime Run current Fact ref | Runtime | 同Tenant/Run/ExecutionScope，Run仍允许该Operation，Generation/Binding current |
| Session | Harness-owned `CommittedPendingActionSessionApplicabilityCoordinateV1` | Harness `runtimeadapter`经公共current Reader | 同Run、SessionID、revision、canonical session digest，phase=`waiting_action`；只是source coordinate |
| Turn | Harness-owned `CommittedPendingActionTurnApplicabilityCoordinateV1` | Harness `runtimeadapter`经公共current Reader | Turn号、PendingAction、Session revision与Session digest完全一致；只是source coordinate |
| PendingAction | `CommittedPendingActionCurrentV1` | Harness | Run/Session/Turn/phase/revision/full PendingAction exact且观察租约未过期 |
| Action | Tool `ActionCandidate` current fact ref | Tool Owner | Candidate仍current，且其PendingAction ref/digest、Run、Session、Scope、Effect、Owner完全一致 |
| Context | Tool Call来源的ParentFrame current fact ref | Context Owner | `Execution{ScopeDigest,RunID,Turn,AuthorityDigest}`、ParentFrame revision/digest与Generation current |

Action维度绑定Tool Owner的`ActionCandidate`，不是Model Invoker Observation或Harness `PendingAction`；`PendingAction`是产生Action Candidate之前的必要来源事实。Action Operation的Context维度绑定产生该Tool Call的ParentFrame；Harness下一Turn使用的new Frame必须由后续`ContextTurnRefreshPortV1`独立结算产生，二者不得混用。

所有current projection都必须包含`CheckedUnixNano`、`ExpiresUnixNano`和canonical digest。Harness Reader TTL上限固定为30秒，并取全部底层current上界的最短值；TTL只是“这次current读取可使用到何时”，不是延长底层Fact、Lease、Fence或Binding。

live `ContextFrame`没有独立`RestoreEpoch`字段，不能在Harness私造一个。恢复边界由Runtime新的ExecutionScope及其Instance/Authority/Lease epochs体现，并通过Context `Execution.ScopeDigest`、`AuthorityDigest`与新Generation绑定；任一变化都使旧Frame/current projection失效，必须产生新Operation，不能重封旧ref。

### 5.1 Harness窄Reader Delta

Harness后续只新增一个只读Port，不暴露`SessionFactPortV2`：

```text
CommittedPendingActionReaderV1
  InspectCommittedPendingActionCurrentV1(request)
    -> CommittedPendingActionCurrentV1
```

请求至少绑定：`ContractVersion`、`RunRef`、`ExecutionScopeDigest`、`SessionID`、`ExpectedSessionRevision`、`ExpectedTurn`、`ExpectedPendingActionRef`、`ExpectedPendingActionDigest`、本次调用fresh `CheckedAt`。前八项构成稳定Subject；`CheckedAt`只标记本次观察起点，不进入immutable Binding identity或digest。Reader以自己的fresh clock返回`CheckedUnixNano`与`ExpiresUnixNano`。返回还至少绑定Session canonical digest、phase=`waiting_action`、完整`PendingActionV2`、两个Harness-owned distinct source coordinates和projection digest。

```text
CommittedPendingActionSessionApplicabilityCoordinateV1
  {Kind, ID, Revision, Digest}
  Digest = Seal(full Run/ExecutionScope + SessionID/revision + canonical Session + waiting_action)

CommittedPendingActionTurnApplicabilityCoordinateV1
  {Kind, ID, Revision, Digest}
  Digest = Seal(Session coordinate + Turn + PendingAction ref/digest)
```

两者必须是不同静态类型和不同canonical domain；不能互换、强转或共用digest。它们是Runtime router建立公共nominal ref及Harness Owner复读事实时的source coordinates；公共ref只是同一`Kind/ID/Revision/Digest`的中立投影，不表示Evidence已登记、不授予Evidence资格，也不能绕过current Reader直接进入Permit/Enforcement。

实现必须执行S1 Inspect → 校验全部字段 → S2 Inspect；两次revision/digest/phase/turn/PendingAction任一变化即Fail Closed。该Reader不创建Candidate、不签发Evidence、不返回Store写入口。

### 5.2 nominal router与Harness Owner-current Reader边界

Runtime G6A router接收Session/Turn source coordinate后，只允许无损nominal projection：把coordinate的`Kind/ID/Revision/Digest`逐字段exact映射为公共`OperationScopeEvidenceApplicabilityFactRefV3`。不得重新seal、分配新ID/Revision/Digest、访问Harness Store或宣称创建Runtime Applicability Fact。

Harness Owner在`ExecutionRuntime/harness/runtimeadapter`实现现有`OperationScopeEvidenceApplicabilityCurrentReaderV3`。收到公共ref后，Adapter必须以四字段exact lookup构造期sealed Binding，恢复稳定Subject；以第一次fresh clock生成本次完整Request并调用Harness公开`CommittedPendingActionReaderV1`复读。Reader以自己的fresh clock封存Checked/Expires；Adapter读取结束后再采第二次fresh clock，要求`verified >= checked && verified < expires`，且公共projection expiry不得晚于底层projection。随后验证source coordinate、完整ExecutionScope、S1/S2、phase/turn/PendingAction与最长30秒短租约，再返回公共`OperationScopeEvidenceApplicabilityCurrentProjectionV3`。Evidence Gateway只有在现有current Reader校验通过后才可Issue；公共ref本身不授Evidence资格。缺Reader、unknown ref/Binding、时钟回拨、TTL crossing、source漂移或过期一律Fail Closed。

公共current Reader只收到`{Kind,ID,Revision,Digest}`，而底层Reader需要完整`InspectCommittedPendingActionCurrentRequestV1`；source ID是不可逆digest，禁止解码ID恢复请求。为此冻结一个Harness-owned、仅用于Adapter构造的配置对象：

```text
CommittedPendingActionApplicabilityBindingV1
  ContractVersion
  Subject: CommittedPendingActionSubjectV1
    # stable Run/Scope/Session/revision/turn/PendingAction fields only
  ExpectedSessionCoordinate: CommittedPendingActionSessionApplicabilityCoordinateV1
  ExpectedTurnCoordinate: CommittedPendingActionTurnApplicabilityCoordinateV1
  Digest
```

Binding必须immutable、deep-clone、canonical sealed且subject-scoped。`Digest`只覆盖稳定Subject与两个distinct coordinates，明确排除`CheckedAt`、`CheckedUnixNano`和`ExpiresUnixNano`等观察时间；同一Subject在不同观察时点必须得到同一Binding identity。它不是Fact、Authority、Evidence、Permit、Application DTO或Runtime FactPort对象。Binding只能来自同一稳定Subject的一次成功Harness Reader投影所返回的source coordinates；该预读不授current资格，公共current调用仍必须使用fresh时间重新执行底层Reader的完整S1/S2。

`runtimeadapter`构造器接收有限Binding集合，零写完成以下步骤后封存：验证每个Binding digest与Subject/coordinate；deep-clone；分别以Session和Turn coordinate的exact `Kind+ID+Revision+Digest`建立只读map。相同键且相同Binding digest重复输入幂等；相同键关联不同Subject、coordinate或Binding digest立即Conflict，构造失败。构造完成后不得注册、删除、替换或重封Binding，不提供运行期写口或全局mutable registry。

收到公共ref时，Adapter以四字段exact lookup Binding，clone出稳定Subject，以fresh clock生成本次`InspectCommittedPendingActionCurrentRequestV1.CheckedAt`并调用底层Reader；不得重放构造期时间。Reader返回自己的Checked/Expires后，Adapter以第二次fresh clock验证`now >= checked && now < expires`。返回值还必须满足S1/S2，并与Binding中的Subject、对应source coordinate、ExecutionScope及最长30秒TTL exact一致；随后才seal公共projection，且其`ExpiresUnixNano`不得晚于底层projection。unknown ref、Binding缺失、重复键冲突、Binding digest漂移、Reader drift、clock rollback、TTL crossing或过期全部Fail Closed。

Harness实现必须保持Reader只返回Owner source coordinates、Runtime Adapter才做公共nominal projection；本轮P1进一步要求Binding identity不封存观察时间。任何把`CheckedAt`写回Binding digest、重放构造期时间或省略第二次fresh clock验证的实现均不验收。

## 6. PendingAction、ToolResult、Settlement、Frame与Continuation绑定

### 6.1 PendingAction

`SettledTurnResultV2(action_required)`必须来自绑定Settlement Owner，并精确关联原Model Candidate、Run Evidence V2和Model-turn Runtime Settlement V3。只有`ApplySettledTurnV2`可将Session从`waiting_settlement` CAS到`waiting_action`并写入`PendingActionV2`。

Tool Candidate必须绑定完整`PendingActionV2`的ref/request digest、capability、payload、source candidate、RunID、SessionID和Action scope；不得仅绑定CallID或Arguments digest。

### 6.2 Settlement V4对两份Evidence的typed exact闭包

live Settlement V4已经提供完整公开类型：

- `OperationSettlementRefV4{ID,Revision,Digest,OperationDigest,EffectID,DomainResult}`：Settlement权威事实ref；
- `OperationSettlementEvidenceAssociationRefV4{ID,Revision,Digest,Settlement,OperationDigest,EffectID}`：typed exact Association ref；其公开Inspect返回`OperationSettlementEvidenceAssociationV4{ContractVersion,ID,Revision,Settlement,Prepare,Execute,Digest}`完整事实；
- `OperationSettlementTerminalGuardRefV4{ID,TenantID,EffectID,OperationDigest,Revision,Digest,Settlement}`：typed exact shared terminal guard ref；
- `OperationSettlementTerminalProjectionRefV4{ID,Revision,Digest,TenantID,OperationDigest,EffectID,Settlement,Association,Guard}`：typed exact terminal projection ref；
- `OperationInspectionSettlementRefV4{Settlement,Association,Guard,Projection,DomainResult,EffectFactRevision,Owner,CheckedUnixNano,ExpiresUnixNano,Digest}`：一次current Inspect的sealed引用，直接持有上述四类typed exact refs及currentness字段。

Continuation闭包统一为：

```text
InspectCurrentOperationSettlementV4(OperationSubjectV3, EffectID)
  -> current OperationInspectionSettlementRefV4
InspectOperationSettlementEvidenceAssociationV4(OperationSubjectV3, Inspection.Association)
  -> full OperationSettlementEvidenceAssociationV4{ID, Revision, Settlement, Prepare, Execute, Digest}
```

Tool模块内Adapter与Harness Continuation Adapter必须分别在其职责点验证完整Association的`RefV4()`与`Inspection.Association` exact一致，Association的Settlement与`Inspection.Settlement` exact一致，prepare/execute各一份且同Operation/Effect/Attempt/Scope；再交叉校验`Inspection.DomainResult`与Tool Owner的`DomainResultFact`、`ToolResult`和ApplySettlement一致。Application只接收中立校验结果，不import这些Owner类型。

`ActionContinuationBindingV1`只携带current `OperationInspectionSettlementRefV4`，不携带裸`prepareRef + executeRef`、两个字符串、任意Evidence数组或私有重封结构。完整Association通过Runtime公开Inspect读取，不复制进Harness事实。当前live公共合同已足够，不触发Runtime closure Delta。

### 6.3 ToolResult与Frame

Application通过自身注入的G6A窄Port请求settled Action输出；Tool模块内Adapter调用Tool Owner公开current Reader获得exact `ToolResult`投影，并验证Action ref、DomainResult ref、Settlement V4 ref、revision/digest与ApplySettlement终态完全一致。Provider Receipt或DomainResult payload本身不能代替ToolResult。

settled ToolResult、current `OperationInspectionSettlementRefV4`和经公开Inspect验证的完整Association共同构成Action Gateway对Context Refresh的exact输入；它们不是new Frame，也不能直接触发Harness Continuation。

G6A在上述输出处停止。Context Owner-local实现可独立验证自己的公共合同；G6A验收PASS后，G6B test-only跨模块fixture中的Application才通过自身注入的Context窄Port请求刷新，Context模块内Adapter调用Context Owner已冻结的版本化[`ContextTurnRefreshPortV1`](../../context-engine/contracts.md#contextturnrefreshportv1)。Owner顺序以[Context集成合同](../../context-engine/integration.md)为准：

```text
settled ToolResult + current V4 Inspection + full Association verification
-> S1 owner-current reread
-> deterministic RefreshAttempt / Frame / NextGeneration reserve
-> collect / admit / manifest / frame freeze
-> pending Context DomainResultFact
-> S2 owner-current reread
-> Context Owner local atomic ApplySettlement + expected Generation CAS
-> settled new Frame exact ref/digest
```

S1/S2必须复读Session/Turn/PendingAction、完整ToolResult链、Assembly Generation/Binding/Activation、Authority、ParentFrame/Generation及Source/Artifact/Cache currentness。新Frame必须绑定Run、NextTurn、ParentFrame、ToolResult、Execution Scope/Authority digest和NextGeneration；恢复后Scope或epoch变化必须使旧Attempt/Frame失效。

G6A与G6B必须分阶段接线：G6A只完成settled ToolResult/current V4 Inspection/public Association Inspect并隔离验收；Context Owner-local实现不受production root阻塞，G6A PASS后才允许创建G6B test-only跨模块fixture。pending Context DomainResult、S2或Context Owner本地原子ApplySettlement/Generation CAS任一未完成/Unknown/漂移时保持`waiting_action`并Fail Closed；该本地迁移不创建、请求或消费Runtime Settlement。ContextReference不能物化时按Context Owner合同Fail Closed或形成Residual，不能用旧Frame或空Context继续。G6B完整验收与真实root接线Conformance同时PASS前不得启用能力、生产调用Continuation或推进Turn。

### 6.4 Harness action continuation合同

`ContinuationRefV2`保留给既有路径，不原地扩字段。Action Gateway采用additive `ActionContinuationBindingV1`与窄Port：

```text
SettledActionContinuationPortV1
  CommitSettledActionContinuationV1(request)
    -> {Session, Candidate}
```

请求最少包含：

1. `RunRef`与`ExecutionScopeDigest`；
2. `SessionID`、`ExpectedSessionRevision`、`ExpectedTurn`、expected phase=`waiting_action`；
3. exact committed `PendingAction` ref/digest；
4. Tool Owner current `ActionCandidate`与`ToolResult` typed refs/digests；
5. Runtime current `OperationInspectionSettlementRefV4`；Harness Adapter必须使用其中`Association`调用公开`InspectOperationSettlementEvidenceAssociationV4`并验证完整prepare/execute；
6. Context Owner已经ApplySettlement并通过S2的new Frame exact ref、revision、digest及NextGeneration ref/digest；`ContextRef`与`ContextDigest`必须等于该Frame；
7. next Candidate ID、Input、Provider binding、created/expires；
8. `ActionContinuationBindingV1` canonical digest。

Port内部再次Inspect Harness Session与Candidate store：先create immutable next Candidate，同ID同内容幂等、同ID换内容Conflict；再CAS `waiting_action -> waiting_model_dispatch`、`turn+1`并清除PendingAction。CAS回包丢失后只Inspect exact Session/Candidate；不得创建第二Candidate或再次Apply Tool结果。

`ActionContinuationBindingV1`只封存跨Owner exact refs与摘要，不复制Tool/Context事实内容，不写Evidence/Permit/Enforcement，也不授予下一次Model dispatch。

## 7. Application协调序列与依赖反转

Application按以下顺序调用**Application Owner发布的窄Port**；每一步都先Inspect恢复同一canonical对象，再决定是否Create/Commit。具体Owner调用由各Owner模块内Adapter完成：

1. Inspect完整Model Tool Call Observation，确认Calls exactly 1；
2. Append/Inspect Run Evidence V2；
3. 请求Model-turn Settlement Owner独立Inspect并产生`SettledTurnResultV2`；
4. Settle/Inspect Runtime Settlement V3；
5. 调用Harness `ApplySettledTurnV2`并Inspect Session；
6. 调用Harness committed PendingAction Reader；
7. 请求Tool Owner create/Inspect Action Candidate与Reservation；
8. 调用Runtime V4 Admission/Review/Permit/Begin；
9. 分别完成prepare、execute Enforcement 4.1与Evidence V3 consume；
10. 请求Tool Owner Record/Inspect DomainResult；
11. Settle后调用`InspectCurrentOperationSettlementV4`取得current Inspection，再用`InspectOperationSettlementEvidenceAssociationV4`读取并验证完整prepare/execute；
12. 请求Tool Owner Apply/Inspect Settlement得到ToolResult；
13. G6A输出后停止；Context Owner-local实现可独立验证；G6A验收PASS后，G6B test-only跨模块fixture通过Application的Context窄Port把settled ToolResult、current V4 Inspection及完整Association校验结果交给Context Adapter，由其调用`ContextTurnRefreshPortV1`执行S1/reserve/collect/admit/freeze；
14. 请求Context Owner记录pending `Context DomainResultFact`；
15. Context Owner执行S2 fresh owner-current复读；
16. 请求Context Owner在同一本地原子边界提交ApplySettlement并执行expected Generation CAS，返回已settled new Frame exact ref/digest；
17. 通过Application的G6B Continuation窄Port调用Harness模块内Continuation Adapter；Adapter重新核对Action V4 current Inspection、完整Association和new Frame，再执行settled action Continuation CAS并Inspect结果。

Application不持有任何Owner Store，不重封Observation、DomainResult、Evidence或Settlement，不把跨Owner步骤宣称为一个事务。

### 7.1 Port、Adapter与wiring边界

依赖关系冻结为：

```text
Application coordinator
  -> Application-owned narrow Ports + neutral DTO
       <- Tool Adapter（tool-mcp模块）
       <- Context Adapter（context-engine模块）
       <- Continuation Adapter（harness模块）

Owner-local isolated implementation
  -> each Owner uses its own fixture; no production root required

G6B test-only cross-module fixture (after G6A PASS)
  -> manually construct Owner Adapters and inject public Ports
  -> verify the chain without Capability activation or production Turn progress
  -> not a production root

production composition root
  -> ABSENT in the current live repository
  -> required only for production enablement after full G6B acceptance
  -> Host Owner designs / implements / accepts real wiring separately

Harness Assembly
  -> validate version / capability / owner-slot binding / sealed digest
  -> assemble injected interfaces only
```

- Application Owner定义G6A/G6B窄协调Port与中立DTO；Application Request只携带Harness公开Session/Turn distinct source coordinates的中立镜像，不预塞公共applicability refs。公共refs只在后续Tool/Runtime路由中由exact coordinates无损投影；DTO不复制Harness、Tool或Context事实类型，也不得把coordinate镜像type-pun为公共ref；
- Tool、Context、Harness各Owner Adapter允许依赖Application公开`contract/ports`以实现窄接口，但不得依赖Application coordinator、kernel或实现包；
- Harness input Adapter只允许依赖Model Owner已终审YES的公共只读Projection Reader合同，不得依赖`model-invoker/internal`、具体执行实现、Repository写口或兼容事件解码路径；
- Owner-local实现可使用本Owner fixture；G6A/G6B test-only fixture手工提供sealed stable-subject applicability Bindings并构造immutable Harness current Reader，每次Inspect时间由Adapter fresh生成。Harness Assembly只接收已构造接口；production composition root未来负责生产构造，但当前不存在；
- Application任何包不得import Harness、Tool或Context；Harness Assembly不得import Tool/Context实现，也不成为跨域composition root；
- G6A与G6B test-only跨模块fixture都只允许手工注入公共Port/Adapter；G6B fixture必须在G6A PASS后创建，且不注册/激活Capability、不生产调用Continuation或推进Turn；当前不宣称production composition root存在；
- production composition root是G6B生产启用前由宿主Owner另行设计、实现、验收的残余，不反向阻塞Owner-local实现或G6B test-only fixture；本设计不创建新的Harness Composition模块；
- Harness Assembly只接收已注入接口，验证版本、能力、Owner/Slot绑定与sealed摘要后组装；它不得实例化Owner Adapter或借助opaque JSON绕过类型边界。

## 8. 状态、恢复与并发

| 位置 | 正常状态 | lost reply / Unknown恢复 |
|---|---|---|
| Model Observation / Run Evidence | 完整N=1 Observation已入Evidence V2 | 按source coordinate和record ref Inspect；不得换sequence |
| Model settlement / PendingAction | `waiting_settlement -> waiting_action` | Inspect原Settlement、Session revision和PendingAction；同ID换内容Conflict |
| Tool Candidate / Reservation | create-once current | 按Action/Reservation ID+digest Inspect；不得从Observation重建第二Action |
| Begin后Tool执行 | Harness保持`waiting_action` | 只Inspect原Operation/Effect/Attempt/phase；不得把Session改为model `reconciling` |
| Evidence V3已consume、V4未见 | 合法恢复窗口 | Inspect原prepare/execute、DomainResult、guard；重试同canonical V4，不再调用Provider |
| Settlement V4回包丢失 | typed terminal refs已写或未写 | `InspectCurrentOperationSettlementV4`取得四类exact refs，再public Inspect完整Association；同ID同内容幂等，同ID换内容Conflict |
| Tool ApplySettlement回包丢失 | ToolResult已写或未写 | Tool Owner Inspect exact Action/DomainResult/Settlement；Receipt不能补写Result |
| Context Refresh/Create/CAS回包丢失 | Attempt/Frame/Generation/Apply已写或未写 | 只Inspect原RefreshAttempt/Frame/Generation/Context Settlement；不得换ID、重跑Tool或推进Turn |
| Context S2漂移 | new Frame已冻结但不可消费 | 保持waiting_action；修复currentness后只恢复原Attempt，不把Frame交给Harness |
| Continuation CAS回包丢失 | 一个Candidate/Session CAS胜出 | Inspect exact Candidate与Session；漂移或第二胜者Fail Closed |

Model调用本身的Begin后Unknown可以使用现有Harness `reconciling`；Tool Action发生在`waiting_action`之后，Unknown不得挪用该状态。

## 9. 硬反例

- `Calls=0`或`Calls>1`仍产生PendingAction；
- 取`Calls[0]`、改写Arguments、把Provider Receipt当ActionResult；
- 未经Run Evidence V2、Model-turn Settlement Owner或Runtime Settlement V3直接CAS PendingAction；
- Action applicability ref指向PendingAction或Observation而不是Tool ActionCandidate；
- Run/Session/Turn/Action/Context任一缺失、错revision、错digest、过期，或恢复后的Execution Scope/epoch漂移仍Issue/Begin；
- prepare/execute缺一、交换、复用同Consumption、来自不同Attempt或用裸pair进入Continuation；
- Settlement V4存在但Association/Guard/Projection typed refs不exact，public Inspect返回的Association与ref不一致，或用V3 Tool settlement冒充闭合；
- Tool ApplySettlement之前创建Harness Continuation，或仅凭DomainResult/Receipt继续；
- Tool ApplySettlement后直接读旧Frame并Continuation，绕过`ContextTurnRefreshPortV1`、pending Context DomainResult、S2或Context Owner本地原子ApplySettlement/Generation CAS；
- Context返回未Apply、未S2或与NextGeneration不一致的Frame仍进入`ActionContinuationBindingV1`；
- 两个Coordinator并发生成不同Candidate/Continuation，或CAS丢包后换ID重试；
- Application、Harness Hook或Adapter写Runtime/Tool/Context Owner事实；
- Application import Harness/Tool/Context，或Owner Adapter依赖Application coordinator/kernel/实现；
- Harness Assembly import Tool/Context实现、实例化Owner Adapter或充当跨域composition root；
- Adapter复制Owner struct或把中立DTO退化为opaque JSON逃生口；
- Harness Reader直接返回`OperationScopeEvidenceApplicabilityFactRefV3`，router改写任一`Kind/ID/Revision/Digest`，或Evidence Gateway在没有`OperationScopeEvidenceApplicabilityCurrentReaderV3`时把公共ref冒充Evidence资格；
- Session coordinate与Turn coordinate互换、共用digest，Application Request预塞公共applicability ref，或把Harness coordinate镜像type-pun为公共ref；
- 通过可逆ID编码恢复Reader请求、运行期注册/替换Binding、使用全局mutable registry，或让Binding进入Application DTO/Runtime FactPort；
- unknown ref、Binding缺失、同键换Subject/coordinate、Binding digest漂移、构造期时间重放、clock rollback/TTL crossing或底层Reader drift仍返回current；
- 未经Model公共只读Reader按完整Ref复读Projection便形成Application Request，或从PendingAction payload、event JSON、compat tool calls反推Projection；
- P3b万能Hook直连Tool、网络或Fact Store。

## 10. 兼容、迁移与实现边界

- 纯文本Run、既有Model Turn和Assembly P1/P2/P3a不受影响；
- 现有`ContinuationRefV2`、Settlement V3、Evidence V3、Dispatch V4和Enforcement 4.1均不原地改写；
- Action能力只能随新Catalog/Generation显式启用；旧Graph不能推断支持；
- Tool live `ToolResult`若仍绑定Settlement V3，必须由Tool Owner先完成additive V4结果合同；Harness不得翻译或伪装；
- `N>1`需要新的多Call语义版本、预算/顺序/部分失败/聚合Settlement设计；
- G6A settled-output切片可在自身门通过后独立落地并隔离验收；Context Owner-local实现可用本Owner fixture独立进行，G6A PASS后才创建G6B test-only跨模块fixture；G6B完整验收与真实root接线Conformance同时PASS前不得启用能力、production Continuation或推进Turn；
- 未实现真实Backend、生产Enforcement、Action Gateway Adapter、Context Refresh、Checkpoint或SLA。

实施顺序与独占候选文件见[Harness Assembly V1实施计划](../../../plan/harness/harness-assembly-v1.md#p7a单call-action-gateway)，验收矩阵见[Action Gateway V1测试矩阵](./action-gateway-v1-test-matrix.md)。
