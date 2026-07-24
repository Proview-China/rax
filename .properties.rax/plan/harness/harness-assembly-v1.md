# Harness Assembly公用接线V1实施计划

状态：P1/P2/P3a、Generation-Binding、PendingAction Reader、Route、G6A Identity/Owner-current及Harness P3 Assembler/InputCurrent Reader均有live实现资产并通过对应独立审计。H-ID-P0/P1/P2/P4已完成；Tool Consumer/P4、system G6A/G6B与production root仍为`NO-GO`。

## 1. 目标与边界

把已解析、已版本固定的Agent计划和六组件贡献编译为确定性的`AssemblyManifestV1`与`CompiledHarnessGraphV1`，再由Runtime Binding/Activation决定是否可运行。

- 实现范围仅规划`ExecutionRuntime/harness/**`；本次文档修订不修改代码，但不形成新的实现授权门；
- 不重写现有Harness Loop、Session、Runtime Adapter或Runtime Operation V3/Application Coordinator；
- Harness私有`ContextPort`、`ModelTurnPort`、`EventCandidatePort`不升级为6+1公共Port；
- 六组件只发布Contribution、领域Port与Run Requirement，不实现公共Slot/Hook/Phase枚举；
- 不修改`model-invoker/**`，Model桥仅依赖RouteID和公开`routegateway`/`execution`/`union`；
- Assembly V1不产生权威pre-run Evidence，不触发该Runtime Delta；
- Go为默认实现语言，不规划Rust、生产DB/RPC/拓扑/SLA。

## 2. 实施前硬门

以下Port Delta未冻结前不得开始对应代码：

| 门 | 依赖Delta | 解锁范围 |
|---|---|---|
| G1 | HA-A01 Agent Assembler最终输出 | `AssemblyInputV1`与上游Adapter |
| G2 | HA-H01 SDK/Graph/Slot/Phase合并 | contract、compiler、SDK首切面 |
| G3 | HA-R01 Generation–Binding Association映射（已闭合基线） | Runtime handoff Adapter与Activation Conformance |
| G4 | HA-M01 public Route桥 | Model Turn Adapter与真实Route Conformance |
| G5 | HA-D01 六组件贡献/Run Requirement | 对应领域Slot接入 |
| G6A | PendingAction Reader、Runtime/Tool/Model前置、HA-X02 Route、Harness Owner-current V3/V4与P3 Assembler/InputCurrent Reader均已YES；Tool Consumer/P4与system fixture未完成 | 当前system NO-GO；Harness Owner-local输出已闭合，仍禁止启用能力/Continuation/Turn推进 |
| G6B | Context Owner-local实现可先使用本Owner fixture；**test-only跨模块fixture前置为G6A验收PASS**，并要求CTX-D09、Settlement/Apply、Generation CAS、S2、new Frame Reader、Owner Adapter及[G6B公共Port Delta](../../design/harness/port-deltas/context-turn-refresh-g6b-v1.md)联合闭合 | 手工注入公共Port验证N=1 Context Refresh + Continuation Adapter合同；当前Application live候选语义漂移为P0；不是production root，不启用能力 |
| G7 | HA-X01B N>1/Checkpoint/通用per-turn refresh | 多Call、Checkpoint与后继refresh闭环 |

Runtime P0.1-P0.6、Operation V3与Application Coordinator已闭合，不列为总阻塞。任一门未过时，不得通过Harness私有兼容对象绕开。

## 2.1 治理执行摘要

- pre-run Evidence：V1 compile/inspect/explain/diff/conformance不产生权威Evidence，不提Runtime Delta；
- 全部Effect类别：Assembly纯计算为`none`；运行Graph只绑定领域Owner声明的Model Turn、Action/Tool/MCP、Context remote/cache write、Sandbox lifecycle/execute、Review remote、Memory/Knowledge/Asset commit、Timeline/Checkpoint/Restore、Cleanup/remote Inspect等namespaced EffectKind，不自定字符串常量；
- ConflictDomain：由领域Port/Resolved Plan绑定Tenant/Organization、Owner与稳定Resource/Operation Scope；缺失即拒绝；
- Requirement分类：effectful Required Slot必须声明精确OperationScope及Inspect/DomainResultFact/Runtime Operation Settlement/ApplySettlement合同；只有启动前置条件才引用RunStartRequirement，只有阻塞CompleteRun或termination report才引用Owner的RunSettlementRequirement/Participant；Assembly不创建领域声明；
- Unknown：Permit前失败可新尝试；Begin后只Inspect/Reconcile原Operation/Attempt，禁止盲重放；
- 顺序：领域Intent/Reservation→Admission→Permit→Begin→Delegation/Prepare→Enforcement→Execute/Inspect→Observation/Evidence→DomainResultFact→Runtime Operation Settlement→Domain ApplySettlement；Review以当前Fact绑定Admission/Permit；
- 双重门禁：所有高风险Effect在宿主Gateway与实际执行点重验Fence、Authority、Review、Budget和Scope。

## 3. 阶段与文件级落点

### P0：联合合同冻结

产物：本design/plan评审结论和五类Delta Owner裁决，不写实现。

验收：Owner、对象版本、公共/私有Port、DAG、Effect/Review/Fence/Unknown、Settlement/Cleanup/Residual、Runtime/Harness/Application映射和冲突规则均有签字结论。

### P1：公共对象与确定性核心

计划文件：

```text
ExecutionRuntime/harness/assemblycontract/version.go
ExecutionRuntime/harness/assemblycontract/envelope.go
ExecutionRuntime/harness/assemblycontract/module.go
ExecutionRuntime/harness/assemblycontract/slot.go
ExecutionRuntime/harness/assemblycontract/port.go
ExecutionRuntime/harness/assemblycontract/hookface.go
ExecutionRuntime/harness/assemblycontract/manifest.go
ExecutionRuntime/harness/assemblycontract/diagnostic.go
ExecutionRuntime/harness/assemblycontract/validate.go
ExecutionRuntime/harness/assemblycontract/digest.go
```

工作：只实现已冻结公共对象、Schema/Owner/authority-level校验、规范化Digest、错误分类和版本兼容；禁止依赖Harness kernel/internal/fakes。

验收：同语义输入换序Digest相同；未知Catalog ID、组件自造Phase、私有Port暴露、Effect/Run Requirement缺字段全部拒绝。

### P2：Assembly Compiler纯计算首切面

计划文件：

```text
ExecutionRuntime/harness/assemblycompiler/compiler.go
ExecutionRuntime/harness/assemblycompiler/normalize.go
ExecutionRuntime/harness/assemblycompiler/dependency.go
ExecutionRuntime/harness/assemblycompiler/resolve_slots.go
ExecutionRuntime/harness/assemblycompiler/resolve_phases.go
ExecutionRuntime/harness/assemblycompiler/conflicts.go
ExecutionRuntime/harness/assemblycompiler/conformance.go
ExecutionRuntime/harness/assemblycompiler/residual.go
ExecutionRuntime/harness/assemblycompiler/graph.go
```

工作：编译五个首切面Slot：`kernel.loop`、`model.turn`、`context.frame`、`event.candidate`、`runtime.gateway`；产出sealed但不可执行的Graph、Diagnostic和Report，不实例化Provider。

验收：DAG环、cardinality、write-set、Gate优先级、Schema/Owner/Digest/currentness冲突Fail Closed；Compile无网络、凭据、Provider或pre-run Evidence Effect。

### P3a：只读Assembly SDK

计划文件：

```text
ExecutionRuntime/harness/assemblysdk/builder.go
ExecutionRuntime/harness/assemblysdk/compile.go
ExecutionRuntime/harness/assemblysdk/inspect.go
ExecutionRuntime/harness/assemblysdk/explain.go
ExecutionRuntime/harness/assemblysdk/diff.go
```

工作：实现Builder、Compile、Inspect、Explain、Diff、Conformance只读SDK；CLI/TypeScript暂不做第二套语义，也不实例化或执行Hook/Port。

验收：SDK纯计算、确定性、无网络/Provider/Permit/Effect；未知Catalog、非法DAG/write-set/cardinality全部Fail Closed。

### P3b：HookFace真实运行面

状态：暂缓，等待运行期Hook、Application/Runtime接线与领域Port联合冻结。

计划文件：

```text
ExecutionRuntime/harness/hookface/runtime.go
ExecutionRuntime/harness/hookface/observer.go
ExecutionRuntime/harness/hookface/filter.go
ExecutionRuntime/harness/hookface/gate.go
ExecutionRuntime/harness/hookface/port.go
ExecutionRuntime/harness/hookface/receipt.go
```

工作：实现Observer/Filter/Gate/Port权限上限、确定性执行、有界Observer和结构化Receipt。

验收：Observer不能阻断，Filter不能联网/扩大Scope/写Owner Fact，Gate不能发Permit，Port不能绕过治理；panic/timeout有结构化结论。

### P4：Runtime Binding/Activation handoff

状态：既有Generation–Binding Association Adapter与只读Conformance基线已闭合；本次不重开、不修改代码。

前提：G3通过。

计划文件：

```text
ExecutionRuntime/harness/assemblyadapter/runtime_binding.go
ExecutionRuntime/harness/assemblyadapter/currentness.go
ExecutionRuntime/harness/tests/assembly/runtime_binding_test.go
```

工作：将sealed Generation/Manifest/Graph摘要映射到Runtime Owner批准的Binding V2形式；复用现有`runtimeadapter.Adapter`和Runtime公共Ports，不修改Runtime。

验收：handoff candidate不能冒充Binding；Activation与dispatch重验currentness；旧/错Scope/Fence/Binding/Graph全部拒绝。

### P5：Model Invoker public Route桥

前提：G4通过；不得修改`model-invoker/**`。

计划文件由联合评审决定是否归入`assemblyadapter`或现有Harness拥有的Adapter目录，禁止预先固定跨Owner包边界。最小实现只引用Model Invoker公开`routegateway`/`execution`/`union`和RouteID。

工作：把public Observation映射为Harness Model Observation及ToolCall/FunctionCall Candidate Observation；Harness随后创建PendingAction，Tool Engine再创建领域ActionCandidate；ContextReference按Route能力Fail Closed或Residual。

验收：无internal/vendor/Raw/Native依赖；stream/completed/cache usage/provider状态不能形成Tool Result、Review Verdict、Timeline或Run终态；Begin后丢包只Inspect原attempt。

### P6：六组件Contribution接入

前提：每个组件分别通过G5；可逐组件小步合入。

计划文件：

```text
ExecutionRuntime/harness/assemblycontract/catalog_v1.go
ExecutionRuntime/harness/assemblycompiler/run_requirements.go
ExecutionRuntime/harness/tests/assembly/contributions/<domain>_test.go
```

工作：只登记Harness已冻结的namespaced Slot/Phase和组件Contribution/Port/Run Requirement映射，不导入组件实现包。各组件代码由其Owner维护。

验收：每个effectful Slot具备EffectKind、ConflictDomain、Review/Authority/Scope/Budget、Inspect、Settlement和Residual；Provider Receipt保持Observation。

### P7：Action Gateway、Checkpoint与per-turn refresh

前提：PendingAction Reader、Runtime/Tool/Model前置、Route、Harness Owner-current V3/V4与P3 Assembler/InputCurrent Reader均为`YES`；Tool Consumer/P4与system fixture仍未完成。Harness P3输出止于sealed Request/InputCurrent proof；完整G6A未来仍止于settled ToolResult + current V4 Inspection + public Association Inspect。system G6A验收PASS后才允许创建G6B test-only跨模块fixture。G6B完整验收与真实root接线Conformance同时PASS前不得启用Capability、生产调用Continuation或推进Turn。N>1、Checkpoint及N=1 settled-action以外的通用refresh仍等待G7。

#### P7a：单Call Action Gateway

状态：Harness source-coordinate/current PendingAction Reader、Runtime/Tool/Model前置、Identity、Route、[Owner-current exact输入Delta](../../design/harness/port-deltas/committed-pending-action-owner-current-inputs-v2.md)对应V3/V4实现及Harness P3 Assembler/InputCurrent Reader均通过对应独立审计；H-ID-P0/P1/P2/P4已完成，Tool Consumer/P4、G6B与production enablement继续NO-GO。字段合同见[单Call Action Gateway V1](../../design/harness/assembly/action-gateway-v1.md)，自动化门见[测试矩阵](../../design/harness/assembly/action-gateway-v1-test-matrix.md)。

G6A真实Provider接线另受[HA-X02强类型Route](../../design/harness/assembly/controlled-operation-provider-route-v2.md)约束：不得用`RouteBindings []ObjectRefV1`、Slot或Hook直连raw/alias ProviderTransport或actual Provider；Runtime kernel-internal Runner不进入Harness公共对象、role enum或fixture注入面。Runtime V2与Tool前置已`YES`，HA-X02第八独立短审亦为`YES`；但cross-module fixture仍等待system G6A其余Owner链闭合。Runtime ports唯一拥有DeclarationRef/ConformanceRef/CurrentRef/Projection/Reader/MatrixKeyV3六个中立类型；Harness拥有Declaration/Conformance/Route Current事实语义、CAS与发布，只实现真实Go Reader接口，不复制公共类型。ProviderTransportBinding与actual ProviderBinding必须分别current并由Request/Prepared exact比较。

##### P7a.0 前置门

G6A实现前只需满足：

1. Runtime Settlement V4 public实现通过中央ordinary/race/vet/conformance；
2. Runtime登记唯一矩阵`OperationScopeKind=run`、`EffectKind=praxis.tool/execute`、`PolicyProfile=praxis.tool/single-call-action-v1`，Run/Session/Turn/Action/Context全部required；
3. Model Owner已终审YES的公共只读Projection exact Reader可按完整`ToolCallCandidateObservationRefV1`复读`ToolCallCandidateObservationProjectionV1`；Harness Adapter在形成Application Request前验证完整Ref、lineage/digest及`Calls==1`，Reader unavailable、Ref变化或Calls不等于1时零Application dispatch；
4. Run、Tool Action、Context Frame的Owner-current Readers，以及Harness Session/Turn source-coordinate Reader通过联合Conformance；Runtime G6A applicability router只把coordinate的`Kind/ID/Revision/Digest`无损nominal projection为公共ref。Harness `runtimeadapter`构造时接收immutable、deep-cloned、只seal稳定Subject的`CommittedPendingActionApplicabilityBindingV1`；收到公共ref后fresh生成Request CheckedAt、复读Harness Reader，再以第二次fresh clock验证`now>=checked && now<expires`、公共expiry不超过底层projection；
5. Tool Owner提供additive V4 ToolResult/ApplySettlement current合同；不得把live V3 ToolResult私下翻译成V4；
6. Application Owner的G6A窄协调Port与中立DTO冻结；Application Request只承载Harness Session/Turn source-coordinate distinct中立镜像，不预塞公共applicability refs，不import或复制Harness/Tool/Context事实类型；
7. Tool Owner在`tool-mcp`内实现Application公开Port的Adapter边界；Harness Assembly的已注入接口校验边界与G6A显式test composition/fixture手工注入边界冻结。当前不宣称production composition root存在；Harness Assembly不import Tool实现、不承担具体wiring。

G6B分为Owner-local、test-only跨模块fixture与production enablement三层。Owner-local实现可使用本Owner fixture，不要求production root；G6A验收PASS后才允许创建G6B test-only跨模块fixture。该fixture手工注入公共Port/Adapter，不是production root，不启用Capability、不生产调用Continuation或推进Turn。

G6B test-only跨模块fixture前必须满足：

1. G6A隔离单元、白盒、黑盒、故障、Conformance与race验收PASS；
2. Context `ContextTurnRefreshPortV1`、pending DomainResult、S2、本地原子ApplySettlement/Generation current CAS与new Frame current Reader联合YES；`CTX-D09-R1`本地迁移零Runtime Settlement；
3. Application Owner的G6B Context Refresh/Continuation窄Port与中立DTO冻结；
4. Context Owner在`context-engine`内、Continuation Owner在`harness`内实现Application公开Port的Adapter冻结；test-only composition手工注入这些公共Port/Adapter，Harness Assembly注入校验联合YES。

production enablement门：G6B完整验收后，宿主Owner还必须另行设计、实现、验收production composition root并通过真实接线Conformance；两者同时PASS前，不注册可运行Capability、不把Slot设为active、不生产调用Continuation、不推进Turn。G6B fixture可验证Continuation Adapter合同/CAS，但不能触碰生产Session。`N>1`、P3b与Checkpoint继续NO-GO；除N=1 settled-action Refresh外的通用per-turn refresh继续冻结。

##### P7a.1 固定调用序列

Additive Identity、Harness Owner-current V3/V4及P3 Assembler/InputCurrent Reader已获最终独立审计`YES(P0/P1/P2=0)`；固定链在Model Projection之后、Runtime Model-turn Settlement之前由绑定Settlement Owner创建`ModelToolCallPendingActionIdentityV1`，并在Harness Session CAS中与PendingAction一起应用。Tool Consumer/P4与system fixture仍未实现；详见[Identity V1计划](./model-tool-call-pending-action-identity-v1.md)。Owner-local通过不单独构成system G6A PASS。

```text
ToolCallCandidateObservationV1(N=1)
-> Run Evidence V2 append / exact Inspect
-> Model-turn Settlement Owner SettledTurnResultV2(action_required)
-> Runtime Operation Settlement V3
-> Harness PendingAction CAS(waiting_settlement -> waiting_action)
-> Tool ActionCandidate + Reservation
-> Admission/Review/V4 Permit/Begin
-> Delegation/Prepare/Enforcement4.1 + Evidence V3 prepare consume
-> Execute-or-Inspect/Enforcement4.1 + Evidence V3 execute consume
-> Tool DomainResultFact
-> Runtime Operation Settlement V4(ref only)
-> Tool ApplySettlement -> ToolResult
-> Runtime OperationInspectionSettlementRefV4 current typed refs
-> public Inspect full Association prepare/execute
-> Action Gateway settled output: ToolResult + current V4 Inspection
-> ContextTurnRefreshPortV1 S1/reserve/collect/admit/freeze
-> pending Context DomainResult
-> S2 -> Context Owner local atomic ApplySettlement + expected Generation CAS
-> settled new Frame exact ref/digest
-> Harness Continuation CAS(waiting_action -> waiting_model_dispatch, turn+1)
```

Provider Receipt/Observation绝不能成为PendingAction或ActionResult。Action settled-output只包含settled ToolResult、current `OperationInspectionSettlementRefV4`及完整Association校验；它必须作为`ContextTurnRefreshPortV1`输入。只有pending Context DomainResult、S2与Context Owner本地原子ApplySettlement/Generation CAS完成后，new Frame exact ref/digest才可进入Continuation；本地迁移零Runtime Settlement。不传裸pair、字符串Evidence ref或Harness私封闭包。

##### P7a.2 分阶段实施顺序

| 阶段 | Owner | 工作 | 完成门 |
|---|---|---|---|
| AG-P0A | 联合 | 冻结G6A版本、closed matrix、Runtime nominal router + Harness applicability current Reader + sealed Binding、ToolResult V4、Application中立Port/DTO、Owner Adapter、test fixture手工注入与Harness Assembly校验边界；明确当前无production root | 仅G6A前置门YES |
| AG-P0M | Model Owner + Harness | 消费已终审YES的Model公共只读Projection exact Reader/atomic Ensure；Harness Adapter按完整Ref复读并验证`Calls==1`后才形成Application Request | Reader unavailable/changed ref/Calls不等于1时零Application dispatch；Harness不新增Model写口、Repository或Ensure实现 |
| AG-P1 | Harness | 只读committed PendingAction projection、S1/S2、完整Session digest、最长30秒TTL、distinct Session/Turn source coordinates；sealed stable-subject Binding与immutable exact map；`runtimeadapter`每次fresh生成观察Request并二次验时 | Reader不直返公共ref；延迟调用不因构造时间stale；回拨/TTL crossing Fail Closed；unit/whitebox/conformance/race |
| AG-P2 | Harness Adapter + Application + Model Settlement Owner | Model exact Reader复读→Observation→Evidence V2→SettledTurnResult→V3 Settlement→PendingAction编排 | 不从PendingAction/event/compat calls反推Projection；N=1正例、N>1 NO-GO、lost reply |
| AG-P3 | Tool + Runtime | Runtime router从Harness coordinates无损投影公共applicability refs；Evidence Gateway经Harness Owner-current Reader校验后Issue；PendingAction→Action/Reservation→V4/4.1→Evidence V3 prepare+execute→DomainResult | exact scope/attempt/fence/authority/review/budget；缺Reader/Binding、unknown ref或source漂移拒绝 |
| AG-P4 | Runtime + Tool Owner Adapter + Application | V4 current Inspection + public Association Inspect→Tool ApplySettlement/ToolResult | 四类typed refs exact，prepare/execute完整，V3/V4终态互斥；G6A输出后停止，不Continuation/Turn推进 |
| AG-P4A | 联合 | G6A隔离验收 | PASS后才创建G6B test-only跨模块fixture；不阻塞Owner-local实现 |
| AG-P5 | Context Owner Adapter + Application | 按[Context P8](../context-engine/context-engine-v1.md#p8n1-context-g6bab实现goc启用no-go)执行settled ToolResult/V4 Inspection→Refresh S1→Frame freeze→pending Context DomainResult→S2→Context Owner本地原子Apply/Generation CAS | 只交付已settled new Frame exact ref/digest；本地迁移零Runtime Settlement；完成G6B Context切片 |
| AG-P6 | Harness Continuation Adapter + Application | settled new Frame→ActionContinuationBinding→Candidate create→Session CAS | lost reply/并发/漂移全部Fail Closed |
| AG-P7 | 联合 | 黑盒、故障、race、vet、集成、系统回归 | 测试矩阵全部实际通过 |

##### P7a.3 文件级Owner与独占候选路径

以下冻结实现落点与Owner边界；中央联合YES及对应技术门PASS后，各Owner可按现有总授权直接修改自己的目录，无需再次用户授权：

```text
ExecutionRuntime/model-invoker/**（仅跨Owner依赖，不是Harness实施落点）
  # Model Invoker Owner：公共只读Projection exact Reader与atomic Ensure已终审YES
  # Harness只消费Reader，不设计或实现Model写口、Store、Repository/Ensure，不创建PendingAction

ExecutionRuntime/runtime/{ports,control,kernel,tests}/**
  # Runtime Owner：Action矩阵、applicability nominal router、V4/4.1/Evidence V3/Settlement V4
  # router只做Kind/ID/Revision/Digest exact投影；不新增Applicability FactPort方法或Runtime Fact

ExecutionRuntime/application/contract/action_gateway_v1.go
ExecutionRuntime/application/ports/action_gateway_v1.go
  # Application Owner：仅中立DTO与窄协调Port；只可依赖自身合同及runtime/core、runtime/ports
  # 禁止import ExecutionRuntime/harness、tool-mcp、context-engine

ExecutionRuntime/tool-mcp/applicationadapter/action_gateway_v1.go
ExecutionRuntime/tool-mcp/tests/**
  # Tool Owner：在本模块用公开Tool合同实现Application公开Port；不得依赖Application coordinator/kernel/实现

ExecutionRuntime/context-engine/applicationadapter/action_gateway_v1.go
ExecutionRuntime/context-engine/tests/**
  # Context Owner：在本模块用公开Context合同实现Application公开Port；不得依赖Application coordinator/kernel/实现

ExecutionRuntime/harness/contract/action_gateway_v1.go
ExecutionRuntime/harness/ports/action_gateway_v1.go
ExecutionRuntime/harness/kernel/action_gateway_v1.go
ExecutionRuntime/harness/contract/pending_action_reader_v1.go
ExecutionRuntime/harness/contract/pending_action_applicability_binding_v1.go
ExecutionRuntime/harness/ports/pending_action_reader_v1.go
ExecutionRuntime/harness/kernel/pending_action_reader_v1.go
ExecutionRuntime/harness/runtimeadapter/operation_scope_evidence_applicability_current_v3.go
ExecutionRuntime/harness/applicationadapter/action_gateway_input_v1.go
ExecutionRuntime/harness/tests/kernel/pending_action_reader_v1_test.go
ExecutionRuntime/harness/tests/runtimeadapter/operation_scope_evidence_applicability_current_v3_test.go
ExecutionRuntime/harness/tests/runtimeadapter/pending_action_applicability_binding_v1_test.go
ExecutionRuntime/harness/tests/applicationadapter/action_gateway_input_v1_test.go
ExecutionRuntime/harness/applicationadapter/action_gateway_continuation_v1.go
ExecutionRuntime/harness/assemblyadapter/action_gateway_injection_v1.go
ExecutionRuntime/harness/tests/actiongateway/**
ExecutionRuntime/harness/tests/assemblyintegration/action_gateway_dependency_test.go
  # Harness Owner：Continuation Adapter、PendingAction/Continuation CAS；Assembly只校验/组装已注入的Application Port接口
  # Harness input Adapter只依赖Model Owner已终审YES的公共只读Reader合同，不依赖model-invoker/internal或实现

Owner-local isolated implementation
  # 各Owner使用本Owner fixture；不要求production root

G6A/G6B显式test composition/fixture（test-only，不是production root）
  # G6B仅在G6A PASS后手工构造Tool/Context/Harness各Owner Adapter并注入Application与Harness Assembly
  # 不启用Capability，不生产调用Continuation或推进Turn

production composition root（当前不存在，不预选路径）
  # G6B完整验收后、production enablement前由宿主Owner另行设计、实现、验收并通过接线Conformance；不新增Harness模块
```

依赖方向固定为：`Application coordinator -> Application-owned ports/neutral DTO <- 各Owner模块内Adapter <- 各Owner公开contract/ports`。Owner-local实现可用本Owner fixture；G6A/G6B test-only fixture手工构造并注入具体实例，其中G6B只在G6A PASS后创建。当前不宣称production composition root存在；production root不反向阻塞Owner-local实现或G6B fixture，只在G6B完整验收后的production enablement前由宿主Owner另行设计、实现、验收并通过接线Conformance。Harness Assembly只接收、校验和组装已注入的Application Port接口。Application任何包不得import Harness/Tool/Context；Tool、Context、Harness各Owner Adapter**允许依赖Application公开contract/ports**以实现窄接口，但不得依赖Application coordinator/kernel/实现，也不得形成包循环。Harness Assembly不得import Tool/Context实现、不得承担跨域composition root或具体wiring、不得复制具体Owner类型。

##### P7a.4 Harness合同落点

- `CommittedPendingActionReaderV1`：输入Run/Scope/Session/revision/turn/PendingAction ref+digest，输出Harness-owned distinct `CommittedPendingActionSessionApplicabilityCoordinateV1`/`CommittedPendingActionTurnApplicabilityCoordinateV1`、完整PendingAction、canonical Session digest与最长30秒TTL；S1/S2任一漂移拒绝。coordinates是真实可复读的source coordinates，Reader不直接返回公共applicability ref、不授Evidence资格；
- `CommittedPendingActionApplicabilityBindingV1`：Harness-owned Adapter配置，canonical seal不含观察时间的`CommittedPendingActionSubjectV1`、预期Session/Turn coordinates与digest；CheckedAt/Checked/Expires明确不进入identity；immutable、deep-cloned、subject-scoped，不是Fact/Authority/Evidence/Application DTO；
- Harness `runtimeadapter`：构造器接收有限Binding集合，零写建立exact四字段只读map；同键同binding幂等，同键换Subject/coordinate/digest Conflict。公共current Inspect时exact lookup Subject，以第一次fresh clock生成完整Request，复读底层Reader，再以第二次fresh clock验证`now>=checked && now<expires`和公共expiry不超过底层projection；不新增Runtime FactPort、运行期写口或全局mutable registry；
- `SettledActionContinuationPortV1`：输入exact PendingAction、Tool Action/ToolResult current refs、Runtime current `OperationInspectionSettlementRefV4`、Context Owner已Apply且S2通过的new Frame/NextGeneration exact refs与next Candidate；Harness Adapter通过注入的Runtime只读public Inspect能力读取完整Association，不信任调用方副本；
- `ActionContinuationBindingV1`：additive typed binding，不原地扩展`ContinuationRefV2`；不复制Evidence、Permit、Enforcement或Owner Fact；
- CAS顺序：create immutable Candidate → Session CAS `waiting_action -> waiting_model_dispatch`、`turn+1`；lost reply只Inspect原Candidate/Session。

Application中立DTO仅承载协调坐标：Contract/Step ID、ExecutionScope digest、Run/Session/Turn、经Model公共只读exact Reader验证的完整Observation Ref中立镜像、Harness Session/Turn distinct source-coordinate中立镜像，以及Settlement V4 typed refs。Harness Adapter必须先按完整Model Ref复读Projection并验证`Calls==1`，Application Request才可形成；Request不承载未经Reader复读的Projection副本，也不预塞公共applicability refs或`CommittedPendingActionApplicabilityBindingV1`。公共refs只在后续Tool/Runtime路由内由exact source coordinates逐字段投影；Binding只由G6A test fixture手工注入Adapter，未来production root负责生产构造。Harness/Tool/Context具体Fact由各自模块内Adapter Inspect并映射为中立坐标。Application不得持有具体领域struct、把coordinate镜像type-pun为公共ref，或通过opaque JSON绕过类型边界。Harness Assembly只校验注入接口的版本、能力、owner/slot绑定和sealed摘要，不调用或import Tool/Context实现。

##### P7a.5 验收与反例

验收：Calls exactly 1；Observation不能直接升级PendingAction；Tool Candidate只能来自已提交PendingAction；五维required且current；V4 prepare/execute属于同Scope/Attempt；Tool DomainResult→Runtime Settlement V4→Tool Apply分层；settled ToolResult/V4 Inspection进入Context Refresh；pending Context DomainResult→S2→Context Owner本地原子Apply/Generation CAS后才产生Continuation可用new Frame，Runtime Settlement调用为0；所有lost reply只Inspect原对象；并发同ID换内容Fail Closed。

硬反例：`Calls>1`取首项；Model Reader unavailable/changed ref/Calls不等于1仍形成Application Request或dispatch；从PendingAction payload/event JSON/compat calls反推Projection；Receipt直建Result；Action维度指向PendingAction；五维缺一；Evidence phase缺失/交换/复用；Harness Reader直返公共applicability ref；router重封或改写source四字段；unknown ref/Binding缺失、同键换Subject/coordinate、Binding digest/Reader drift仍Issue；Binding digest包含CheckedAt/Checked/Expires、Adapter重放构造期时间、Reader后不二次采时、clock rollback或TTL crossing仍返回current；可逆ID解码；运行期注册或全局mutable registry；Binding进入Application DTO/Runtime FactPort；Session/Turn coordinate互换或coordinate/public ref type-pun；Application Request预塞公共ref；V3 Tool Settlement冒充V4；裸Evidence pair；Tool Unknown把Session改为model `reconciling`；Tool Apply后绕过Context Refresh使用旧/未settled Frame；Context Apply/Generation CAS或S2缺失仍Continuation；CAS丢包后换Candidate ID；Application import任一Owner；Owner Adapter依赖Application coordinator/kernel/实现；Harness Assembly import Tool/Context实现或自行充当composition root。

#### P7b：N>1、Checkpoint与通用per-turn refresh

状态：继续冻结。`N>1`只保存完整Observation/Evidence并NO-GO，不取首项、不拆分、不经`action.batch.completed`隐式执行。P7a只纳入Context Owner已冻结的N=1 settled-action Refresh；批量、非Tool触发或通用refresh与Checkpoint继续等待G7。

验收：ActionRequired停在`waiting_action`直到领域DomainResultFact、Runtime Operation Settlement与领域ApplySettlement闭环引用回注；Checkpoint贡献写State Plane且不宣称外部世界回滚；每Turn refresh遵守Context披露Effect和Route能力；Unknown只Inspect原attempt。

### P8：迁移、性能与发布门

工作：旧Route Engine草案迁移为Assembly Adapter/Compiler语义；提供显式旧Manifest诊断/升级工具；补齐benchmark、文档、module/memory（不属于G6A，按对应阶段与Owner边界执行）。

验收：无静默默认、无旧legacy Intent/Fence生产路径；Fake仅测试；所有自动化门通过后仍需真实Backend/账号另行认证。

## 4. 测试矩阵

| 层级 | 计划位置 | 核心用例 | 阶段门 |
|---|---|---|---|
| 单元 | `assemblycontract/*_test.go`、`assemblycompiler/*_test.go`、`hookface/*_test.go` | 版本、Digest、DAG、cardinality、排序、write-set、Gate合并、Residual、错误分类 | P1-P3 |
| 白盒 | 同包测试 + `tests/assembly/whitebox` | Generation状态、CAS/currentness、Run Requirement覆盖、Handler panic/timeout、有界Observer | P2-P3 |
| 黑盒 | `ExecutionRuntime/harness/tests/assembly`外部测试包 | 只经Assembly SDK compile/inspect/explain/diff；五Slot正反例 | P2-P3 |
| 故障注入 | `tests/assembly/faults` | Begin后lost reply、stale fence、expired review/budget、duplicate/conflict、Port断连、partial cleanup | P4-P7 |
| Conformance | `tests/assembly/conformance` | Runtime Binding/Operation/ExecutionPort、Model public union、六领域Contribution Schema | P4-P7 |
| Race | 全部新增包与现有Harness | 并行Contribution、Observer、Graph只读、Session CAS、sealed Binding map并发只读与无运行期写 | 每阶段 |
| Vet | 全部新增Go包 | `go vet`无新增问题 | 每阶段 |
| 集成 | `tests/assembly/integration` | Assembler→Assembly→Binding；Model/Context/Action/Checkpoint分段 | P4-P7 |
| 系统 | 现有黑盒Harness/Runtime系统套件增量 | 纯文本Run、Action Run、Unknown恢复、取消/Cleanup、全Settlement CompleteRun | P7-P8 |
| Action Gateway G6A | 各Owner独占测试；详见[专项矩阵](../../design/harness/assembly/action-gateway-v1-test-matrix.md) | N=1、sealed Binding canonical/deep clone、exact map duplicate/conflict/unknown/drift、并发只读、五维current、prepare+execute Evidence V3、settled ToolResult + current V4 Inspection + public Association Inspect；确认无Context/Continuation/Turn副作用 | G6A自身门通过后 |
| Action Gateway G6B | Owner-local测试 + G6A PASS后的test-only跨模块fixture | ToolResult→Context Refresh/Settlement/Apply/S2→new Frame→Continuation Adapter合同、lost-reply/并发/漂移、N>1 NO-GO、手工注入/import DAG | fixture不是production root；生产启用需G6B完整验收+真实root接线Conformance |
| Benchmark | `assemblycompiler/*_test.go`、`hookface/*_test.go` | 大Module/DAG编译、lookup、dispatch、Observer背压、alloc/op | P2-P8 |

每阶段最少运行：

```text
go test -count=1 -shuffle=on ./...
go test -count=20 ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
git diff --check -- ExecutionRuntime/harness .properties.rax/design/harness .properties.rax/plan/harness
```

联合回归按Owner批准的Runtime、Application、Model Invoker和六组件范围执行。没有实际运行就不得声称通过。

## 5. 依赖与迁移

依赖DAG：Agent Assembler final output → Assembly contract/compiler → Runtime Binding V2 → Runtime Activation → Harness Session → Application settlement aggregation → Runtime CompleteRun；Model Invoker与六组件仅通过公开Port作为侧向依赖。

迁移规则：

- 旧`route-engine`设计/计划标记为被本计划取代，不删除历史；
- 旧Resolved/Bootstrap对象必须显式升级并保留source digest；
- sealed Generation不可原地升级，任何主版本/Owner/Slot/Phase/Graph变化产生新Generation；
- legacy Harness private Port只保留内部Adapter用途，不包装成六组件公共合同；
- 已有Runtime cleanup exact Inspect继续复用，不保留旧Port Delta为开放阻塞。

## 6. 联合管理线裁决

1. HA-A01采用独立`AssemblyInputV1` envelope，不塞入Runtime Governance Extension；
2. HA-R01首版以Assembly Host required `ComponentManifestV2`和已登记required `GovernanceExtensionV2`承载Generation/Manifest/Graph/Catalog/Slot-Phase摘要；Runtime BindingSet仍是权威Fact；
3. Actual Injection采用三层Owner：Model Invoker Route级Observation、Harness Model Turn Adapter聚合Manifest、Context Owner Conformance Fact；
4. V1 Plugin只允许静态注册、受信in-process Go `ModuleFactory`；remote/WASM/Go plugin均延期；
5. TypeScript glue不进入P1-P3，待Go Schema/Golden/Conformance稳定后只做映射与展示；
6. HA-X01/HA-X02冻结顺序为强类型Provider Route → G6A Action Gateway → G6B per-turn Context refresh → G7 Checkpoint；
7. P1、P2、P3a只读SDK可进入首个实现波次；P3b真实Hook运行面继续等待公共接线冻结。
8. PendingAction Reader、Runtime/Tool/Model前置、Identity、Route、Harness Owner-current及P3 Assembler/InputCurrent Reader最终代码审计均为`YES`。Tool Consumer/P4与system fixture未实现；因此system G6A仍`NO-GO`。G6A验收PASS后才创建G6B test-only跨模块fixture；G6B完整验收与真实root接线Conformance同时PASS前禁止能力启用、production Continuation与Turn推进。`N>1`与P3b继续冻结。
9. G6B三层门冻结：Owner-local fixture隔离实现 → G6A PASS后G6B test-only跨模块fixture → G6B完整验收与真实production root接线Conformance后production enablement；Checkpoint只在后续G7解冻。

## 7. 完成定义与当前裁决

设计/计划完成：既有资产经过联合评审并处理HA-A01/HA-H01/HA-R01/HA-M01/HA-D01/HA-X01结论；HA-X02已通过Harness Route V2第八独立短审。该结论只解除Identity Owner-local实现门，不启用fixture。

实现完成：P1-P8全部通过对应门、自动化和Owner Conformance；Runtime CompleteRun仍以真实RunSettlementRequirement Participant Fact为准。

当前结论：**既有P1/P2/P3a与Generation-Binding不重开；PendingAction Reader、Runtime/Tool/Model前置、Identity、Route、Harness Owner-current及P3 Assembler/InputCurrent Reader最终代码审计均为YES。Tool Consumer/P4与system fixture未实现，因此system G6A/G6B/production root继续NO-GO。G6A PASS后才创建G6B test-only跨模块fixture；G6B完整验收与真实root接线Conformance同时PASS前，能力启用、production Continuation与Turn推进保持NO-GO。P3b、N>1、Checkpoint与通用per-turn refresh继续冻结到G7。**
