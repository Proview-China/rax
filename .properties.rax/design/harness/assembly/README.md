# Harness Assembly公用接线总设计

## 1. 状态与设计依据

- 状态：P1/P2/P3a与Generation-Binding接线已有live实现资产；PendingAction Reader、Runtime/Tool/Model前置、Harness Route V2、G6A Identity、Owner-current V3/V4及Harness P3 Assembler/InputCurrent Reader均通过对应独立审计。H-ID-P0/P1/P2/P4已完成；Tool Consumer/P4、system G6A/G6B/production root继续`NO-GO`；
- 最高业务输入：`tmp.document/Harness接线.md`；
- live基线：Runtime Binding/Operation Governance/Evidence/Review/Run Claim/Settlement V2/V3与Harness Governed V2/Application V3；
- Route第八返修已通过独立短审；该结论只解除Identity Owner-local Phase 1/2实现门，不等于system G6A或production root通过。
- G6A Identity additive Delta、Owner-current V3/V4与Harness P3 Assembler/InputCurrent Reader均为`YES(P0/P1/P2=0)`；N=1固定恒等映射与H-ID-P0/P1/P2/P4已闭合。Tool Consumer/P4与system fixture仍未验收，Owner-local通过不冒充系统闭环。详见[Identity V1](./model-tool-call-pending-action-identity-v1.md)与[冻结反例矩阵](./model-tool-call-pending-action-identity-v1-test-matrix.md)。

Harness Assembly是六加一中的公用接线面，不是第七个业务组件，也不是第二个Runtime或第二个Agent Assembler。它把已经解析、版本固定且获准参与装配的模块贡献编译为确定性的Run内接线图。

## 2. 三段装配链

```text
Agent Definition + Resolved Profile
  -> Agent Assembler
  -> ResolvedAgentPlan + HarnessBootstrapPlan

Resolved Plan + Bootstrap Plan
+ actual ComponentManifestV2 / RouteBinding / contributions
  -> Harness Assembly Compiler
  -> AssemblyManifest + CompiledHarnessGraph
+ RuntimeProviderBinding candidate + reports

sealed Assembly Generation + Runtime current facts
  -> Runtime Binding / Admission / Activation
  -> Endpoint / Run / governed Operations / Settlement
```

三段职责不得互换：

1. Agent Assembler决定“应该使用什么”：解析Definition、Profile、版本、能力、依赖和组织约束；
2. Harness Assembly Compiler证明“实际接上了什么”：验证Expected/Actual、Slot、HookFace、Port和确定性顺序；
3. Runtime决定“此刻能否运行”：验证Binding currentness、Lease、Fence、Authority、Review、Budget、Effect、Evidence和Settlement。

Compiler不得重新计算Profile、重新选择Model Route、扩大Capability、补造缺失组件或把Residual静默当成功。

## 3. Owner与非Owner

| 领域 | 唯一Owner | Harness Assembly职责 | 明确非Owner |
|---|---|---|---|
| Definition/Profile/版本解析 | Agent Definition、Profile System、Agent Assembler | 原样消费已解析引用和摘要 | 不重新合成或降级 |
| Assembly Manifest/Graph | Harness Assembly Compiler | 拥有装配规则、Slot/HookFace类型检查、确定性编译和说明报告 | 不启动Agent、不授予Runtime资格 |
| Run内Loop/Session/Event Candidate | Harness Kernel | 消费Compiled Graph推进现有Governed Session | 不拥有Runtime Run/Outcome |
| Model Route/Provider调用语义 | Model Invoker | 绑定一个已选定Model Turn Provider | 不复制Provider协议或重试算法 |
| Context Frame | Context&Cache | 调用冻结的Context Port并消费Frame | 不拥有Recipe、Cache或披露Effect |
| Tool/MCP/Action结果 | Tool&MCP | 形成Action Candidate、等待领域结果 | 不执行未经治理Action |
| Review Case/Verdict | Review | 在Gate暂停并消费当前Verdict引用 | 不决定Review、不发Permit |
| Sandbox执行与残留 | Sandbox | 使用已绑定执行面并传播观察 | 不拥有Lease、Workspace或Cleanup事实 |
| Timeline/Checkpoint/Restore | Continuity | 提交Run-local有序Candidate与恢复请求 | 不拥有Timeline或Checkpoint事实 |
| Memory/Knowledge/Asset | 对应领域Owner | 提交或查询Candidate/Ref | 不直接Commit正式Record |
| Operation治理/Run/Outcome | Runtime | 传播Runtime引用并消费Permit/Settlement | 不写Runtime Fact或分配Ledger sequence |
| 跨域编排 | Application Coordinator | 只调用Application Owner发布的窄Port与中立DTO | 不import Harness/Tool/Context，不让SDK或Plugin直调Harness Kernel |
| Owner Adapter | 各Owner模块 | Owner-local实现可用本Owner fixture隔离；G6B test-only跨模块fixture只手工注入公共Port | Harness Assembly不import Tool/Context实现、不承担具体wiring |
| production composition root | 宿主Owner（G6B生产启用残余） | 当前不存在；G6B完整验收与真实root接线后才可用于生产能力 | 不得以G6A/G6B fixture冒充生产root，不在Harness新建跨域Composition模块 |

## 4. 设计不变量

1. Slot是类型化接线位置，不是模块名、Plugin名或动态map key；
2. 每个语义Slot只有一个Owner，可有多个Source、Provider、Filter或Observer；
3. HookFace严格分为Observer、Filter、Gate、Port，权限不能相互升级；
4. Phase顺序在Assembly期间确定，Run热路径不做Manifest解析、依赖求解或反射式查找；
5. CompiledHarnessGraph激活后不可原地修改；结构变化产生新AssemblyGeneration和Digest；
6. Provider回包、PhaseReceipt和Harness终态均先是Observation/Candidate/Claim；
7. 真实Effect必须保持领域Intent/Reservation→Admission→Permit→Begin→Delegation/Prepare→Enforcement→Execute/Inspect→Observation/Evidence→DomainResultFact→Runtime Operation Settlement→Domain ApplySettlement；Review是Admission/Permit所绑定的当前治理事实，宿主Gateway和实际执行点均须重验；
8. UnknownOutcome只Inspect/Reconcile，禁止盲目重复Dispatch；
9. Harness私有`ContextPort`、`ModelTurnPort`、`EventCandidatePort`不是六组件公共Port；
10. 组件domain/kernel只依赖自身合同；跨域由Application Coordinator、版本化Port和稳定Ref关联。
11. Model Invoker的Tool Call只能形成完整Observation；只有经Runtime Evidence登记、绑定的Settlement Owner独立Inspect并提交`SettledTurnResultV2`、Runtime返回精确Operation Settlement后，Harness才能CAS产生`PendingAction`。

## 5. Run-local ownership

Assembly产物分离State与Services：

- `SessionState`：Harness拥有的Run-local可恢复事实，沿用`GovernedSessionV2`、Candidate、PendingAction/Input、Attempt Ref和CompletionClaim；
- `SessionServices`：由Compiled Graph提供的不可变Port、Provider、HookFace Handler和策略引用，不写入Session权威状态；
- Provider native session只作Observation/恢复引用，不能替代Runtime stable session或Harness session；
- Turn是一轮模型交互，Step是Context、Model、Action、Review、执行、结果回注等细粒度阶段；
- 并发执行可以发生在流、Action、后台任务和Observer，但状态提交必须经revision/CAS线性化。

现有Governed Session状态机保持事实源：

```text
creating
-> waiting_model_dispatch
-> model_dispatch_reserved
-> model_in_flight
-> waiting_settlement | reconciling
-> waiting_action | waiting_input | terminal
```

Assembly不得另建平行Session状态机。

## 6. 六组件接入边界

六组件通过自己的Versioned Port、`ComponentManifestV2`、Capability、稳定Ref和Slot/Phase Contribution接入。所有Contribution只能引用Harness Assembly Catalog中namespaced、versioned的`SlotSpecV1`、`HookFaceSpecV1`与Phase ID；组件不得自行发明公共枚举。Harness只绑定，不复制领域类型；当前通用Context/Action/State/Review/Timeline骨架不能替代各领域最终Commit合同。

运行时链路：

```text
Input -> Context Candidate -> frozen Context Frame
-> governed Model Turn
-> Output | ToolCall/FunctionCall Candidate Observation | Input Required
-> Runtime Evidence -> Model-turn Settlement Owner
-> SettledTurnResult(action_required) -> Runtime Operation Settlement
-> Harness PendingAction CAS -> Tool/MCP ActionCandidate
-> Tool/MCP Admission -> Review -> Runtime Permit
-> Begin -> Delegation/Prepare -> V4 Enforcement -> Execute/Inspect
-> Observation/Evidence -> Tool DomainResultFact
-> Runtime Operation Settlement -> Tool ApplySettlement -> Tool Result
-> current V4 Inspection + public Association Inspect
-> ContextTurnRefreshPortV1 -> pending Context DomainResult
-> S2 -> Context Owner local atomic ApplySettlement + Generation CAS
-> settled new Frame
-> Harness Continuation CAS -> Completion Gate -> Harness Claim
-> Runtime Claim Evidence -> Run Settlement -> Outcome
```

按能力裁剪，不要求纯文本Run经过无关Action/Sandbox步骤；跳过的能力必须在Manifest和Graph中明确为未绑定或不适用。

### 6.1 单Call Action Gateway冻结边界

首个最小切面只接受`ToolCallCandidateObservationV1.Calls`恰好为1。该Observation原子保存Model Invoker返回的全部Calls、顺序和参数摘要，不能直接升级为`PendingAction`或`ActionCandidate`。

```text
ToolCallCandidateObservationV1 (N=1)
-> Runtime EvidenceLedgerRecordV2
-> bound Model-turn Settlement Owner Inspect
-> SettledTurnResultV2(action_required)
-> Runtime OperationSettlementRefV3
-> Harness ApplySettledTurnV2 + Session CAS(waiting_settlement -> waiting_action)
-> committed PendingActionV2
-> Tool Owner ActionCandidate + Reservation
-> Runtime V4 Admission/Review/Permit/Begin
-> Delegation/Prepare/Enforcement + Evidence V3 prepare consume
-> Execute-or-Inspect/Enforcement + Evidence V3 execute consume
-> Tool DomainResultFact
-> Runtime OperationSettlementRefV4
-> Tool ApplySettlement -> ToolResult
-> Runtime OperationInspectionSettlementRefV4 current typed refs
-> public Inspect full Association prepare/execute
-> ContextTurnRefreshPortV1 S1/reserve/collect/admit/freeze
-> pending Context DomainResultFact
-> S2 -> Context Owner local atomic ApplySettlement + expected Generation CAS
-> settled new Frame exact ref/digest
-> Harness ActionContinuationBindingV1 + Session CAS(waiting_action -> waiting_model_dispatch)
```

Owner不随链路转移：Model Invoker只拥有Observation；Runtime拥有Evidence、V4治理和Operation Settlement；绑定的Settlement Owner拥有`SettledTurnResultV2`语义；Harness拥有Run-local Session、PendingAction和Continuation CAS；Tool Owner拥有ActionCandidate、领域Fact、Inspect/CAS/ApplySettlement；Application Owner只发布窄Port/中立DTO，Coordinator只协调，不创作任何Owner Fact。Tool、Context、Harness Adapter分别留在各自模块并允许依赖Application公开contract/ports。Context Owner-local实现可用本Owner fixture隔离；G6B test-only跨模块fixture可手工构造各Owner Adapter并注入公共Port，但它不是production root，不注册/激活Capability，也不生产调用Continuation或推进Turn。当前没有production composition root；只有G6B完整验收、宿主Owner完成真实root设计/实现/验收并通过接线Conformance后，production Capability、Continuation和Turn推进才可GO。Harness Assembly始终只校验/组装已注入接口，不负责具体wiring。

Harness `CommittedPendingActionReaderV1`只输出Harness-owned、真实可复读的Session/Turn distinct source coordinates：`CommittedPendingActionSessionApplicabilityCoordinateV1`与`CommittedPendingActionTurnApplicabilityCoordinateV1`。它们由完整Session/Turn/PendingAction exact内容seal。`OperationScopeEvidenceApplicabilityFactRefV3`只是对外部领域source的中立ref；live `OperationScopeEvidenceFactPortV3`没有Applicability Fact Create/Inspect。Runtime G6A router只把coordinate的`Kind/ID/Revision/Digest`无损投影为公共ref，不创作新ID/Digest或Runtime Fact。Harness `runtimeadapter`实现现有公共current Reader，收到公共ref后通过构造期sealed Binding exact恢复稳定Subject，以fresh clock生成本次Request；Reader返回自己的Checked/Expires后再采第二次fresh clock验证currentness。公共ref本身不授Evidence资格。

四字段公共ref无法反推出完整Reader请求，source ID也保持不可逆。Harness因此冻结`CommittedPendingActionApplicabilityBindingV1`：它只把不含观察时间的稳定`CommittedPendingActionSubjectV1`与预期Session/Turn source coordinates canonical seal为subject-scoped、deep-cloned immutable Adapter配置；`CheckedAt/Checked/Expires`明确不进入Binding digest。`runtimeadapter`构造器零写建立exact四字段只读map；同键同binding幂等，同键换Subject/coordinate/digest冲突。current读取必须fresh生成CheckedAt、重新执行完整S1/S2、二次采时验证`now>=checked && now<expires`及公共expiry不超过底层。Binding不是Fact/Authority/Evidence/Application DTO，不进入Runtime FactPort；G6A仅由test fixture手工注入，production root未来负责构造。

Model exact入口已经终审YES：live Model Invoker已提供`ToolCallCandidateObservationRefV1`、`ToolCallCandidateObservationProjectionV1`、公共exact Reader及原子`Ensure`。Harness Adapter形成Application Request前必须通过该公开只读Reader按完整Ref复读Projection并验证Ref/lineage/digest及`Calls==1`；Reader unavailable、Ref变化或Calls不等于1时零Application dispatch。PendingAction payload、event JSON和compat tool calls不得用于反推Projection；Harness不持有Model写口，也不重新实现Repository或`Ensure`。

Runtime `OperationInspectionSettlementRefV4`直接持有Settlement、Association、Guard、Projection四类typed exact refs以及DomainResult/Owner/Effect revision/current TTL；Application随后通过公开`InspectOperationSettlementEvidenceAssociationV4`读取完整prepare/execute。Harness不私封Evidence bundle，也不接受裸Evidence pair/string。

G6A的直接产物是settled ToolResult、current V4 Inspection和完整Association校验，不是下一Turn Frame；G6A在此停止并隔离验收。Context Owner-local实现可以独立完成；G6A PASS后，G6B test-only跨模块fixture才把上述输入经Context模块内Adapter送入`ContextTurnRefreshPortV1`。固定链为`Tool settled → Context Refresh → pending DomainResult → S2 → Context Owner local atomic ApplySettlement + Generation CAS → new exact Frame`；本地迁移不创建、请求或消费Runtime Settlement。只有本地原子提交完成且S2一致后，fixture才可验证new Frame exact ref/digest与Harness Continuation Adapter合同。G6B完整验收与真实root接线Conformance同时PASS前，production Capability、Continuation和Turn推进保持NO-GO。

首切面封闭矩阵固定为`OperationScopeKind=run`、`EffectKind=praxis.tool/execute`、`PolicyProfile=praxis.tool/single-call-action-v1`，Run/Session/Turn/Action/Context五维全部`required`。Harness Reader执行S1/S2、返回完整Session digest与最长30秒source projection；任一Owner Reader、Kind路由、nominal四字段映射或公共current Reader缺失/漂移即Fail Closed。当前由Harness Reader直接返回公共applicability ref的代码是未验收候选，须在本设计修正中央YES后再修改并重新验收。

`N>1`在4.1前保持完整Observation并Fail Closed：不得取首项、拆分、并发执行或创建任意`PendingAction`。多Call的顺序、部分失败、预算分摊、Settlement聚合和Continuation编码尚未冻结，因此明确NO-GO。

Provider Receipt/Observation绝不能成为PendingAction或ActionResult。Application只协调公开Port和exact refs，不拥有Observation、PendingAction、Tool Result、Evidence、Settlement或Context Frame事实。

完整字段、Reader、恢复与反例见[单Call Action Gateway V1](./action-gateway-v1.md)、[测试矩阵](./action-gateway-v1-test-matrix.md)、[治理与恢复](./governance-and-recovery.md#11-单call-action-gateway权威链)、[Port Delta](./port-deltas.md#ha-x01action-gatewaycheckpoint与per-turn-refresh接线)与[验收门](./acceptance.md#41-action-gateway分阶段实现门)。

## 7. 设计资产

- [Model PreDispatch Surface Commit Gate V1（M2与Harness concrete Gate Owner-local实现/测试YES；actual-point no-bypass待闭合）](./model-predispatch-surface-commit-gate-v1.md)
- [Model PreDispatch Assembly Owner-current A2+B1+C2裁决](../port-deltas/model-predispatch-assembly-owner-current-v1.md)
- [Model PreDispatch Surface Commit Gate V1流程图](./model-predispatch-surface-commit-gate-v1.drawio)
- [Model PreDispatch Surface Commit Gate V1测试矩阵](./model-predispatch-surface-commit-gate-v1-test-matrix.md)
- [Controlled Operation Provider强类型Route V2](./controlled-operation-provider-route-v2.md)
- [Controlled Operation Provider Route V2流程图](./controlled-operation-provider-route-v2.drawio)
- [Controlled Operation Provider Route V2测试矩阵](./controlled-operation-provider-route-v2-test-matrix.md)

Route V2跨Runtime/Harness边界只允许Runtime ports拥有六个中立类型：DeclarationRef、ConformanceRef、CurrentRef、CurrentProjection、CurrentReader及既有MatrixKeyV3；Harness Assembly仍是Declaration/Conformance/Route Current事实语义与发布Owner。`type Owner != fact semantic Owner`，Runtime不得定义Harness事实struct/canonical/digest，Harness不得复制或alias公共类型。
- [对象、版本与状态](./objects-and-state.md)
- [类型化Slot与六组件接线](./slots.md)
- [HookFace、Phase与执行规则](./hookfaces-and-phases.md)
- [治理Effect、恢复与Runtime交接](./governance-and-recovery.md)
- [Assembly SDK、插件与外部使用面](./sdk-and-plugins.md)
- [单Call Action Gateway V1](./action-gateway-v1.md)
- [单Call Action Gateway V1流程图](./action-gateway-v1.drawio)
- [单Call Action Gateway V1测试矩阵](./action-gateway-v1-test-matrix.md)
- [Model Tool Call → PendingAction Identity V1](./model-tool-call-pending-action-identity-v1.md)
- [Identity V1测试矩阵](./model-tool-call-pending-action-identity-v1-test-matrix.md)
- [Identity V1链路图](./model-tool-call-pending-action-identity-v1.drawio)
- [验收与测试设计](./acceptance.md)
- [结构化Port Delta](./port-deltas.md)

## 8. 本文排除范围

- 本次文档修订不实现Assembly包、SDK、CLI、Plugin loader或真实Provider；G6A后续实现按既有总授权和技术门执行；
- 不修改Runtime、Application、Model Invoker或六组件合同；
- 不选择数据库、RPC、进程拓扑、WASM Runtime、生产SLA或真实账号；
- 不把Fake/Conformance通过宣称为生产认证；
- 不规划Rust。当前设计没有已证明的计算稠密热点，Go是唯一默认实现语言。
