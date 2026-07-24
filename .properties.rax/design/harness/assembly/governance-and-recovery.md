# Harness Assembly治理、恢复与公共映射

## 1. 权威链与适用边界

Harness Assembly只编译接线声明，不重造Runtime Operation V3、Application Coordinator或领域状态机。任何外部动作必须保持下列顺序：

```text
领域Intent/Reservation
-> Admission
-> Permit
-> Begin
-> Delegation/Prepare
-> Enforcement
-> Execute/Inspect
-> Observation/Evidence
-> DomainResultFact
-> Runtime Operation Settlement
-> Domain ApplySettlement
```

硬规则：

1. `Permit`不等于已执行，Provider Receipt不等于领域Fact；
2. `Begin`后丢失回包，只能针对原Operation/Attempt执行exact Inspect，禁止换Key重放；
3. Harness只拥有Run内Loop、Session、source-ordered Event Candidate、PendingAction和CompletionClaim；
4. 各组件只拥有自己的Manifest、领域Fact、Inspect/CAS、ApplySettlement和Run Requirement声明；
5. Runtime拥有Operation、Fence、Binding、Policy/Trust、Run Fact和Outcome；Application负责跨域编排；
6. Harness Phase Receipt、Model stream/completed/cache usage/provider状态均是Observation；Model Tool Call只形成ToolCall/FunctionCall Candidate Observation。只有Runtime Evidence、绑定Settlement Owner的`SettledTurnResultV2`和精确Runtime Operation Settlement闭合后，Harness才可CAS产生PendingAction，Tool Owner再从已提交PendingAction创建ActionCandidate。

### 1.1 单Call Action Gateway权威链

只冻结`N=1`的最小垂直链。各Owner-local合同可独立实现/测试；真实Provider G6A cross-module fixture还必须通过Runtime V2、Tool Adapter、[HA-X02强类型Route](./controlled-operation-provider-route-v2.md)、Settlement V4、Action封闭矩阵、Owner-current Readers、ToolResult V4、Application窄Port/DTO及Owner Adapter全部门，输出止于settled ToolResult + current V4 Inspection + public Association Inspect；Context Owner-local实现可用本Owner fixture独立进行，G6A PASS后才创建G6B test-only跨模块fixture；G6B完整验收与真实root接线Conformance同时PASS前禁止能力启用、production Continuation与Turn推进：

```text
Model Invoker ToolCallCandidateObservationV1
-> Runtime EvidenceGovernancePortV2.AppendGoverned / exact Inspect
-> bound Model-turn Settlement Owner independent Inspect
-> SettledTurnResultV2(action_required)
-> Runtime OperationSettlementGovernancePortV3
-> Application invokes Harness ApplySettledTurnV2
-> Harness Session CAS to waiting_action + PendingActionV2
-> Tool Owner admits ActionCandidate from committed PendingAction
-> domain Intent/Reservation
-> Admission -> Review Authorization -> V4 Permit -> Begin
-> Delegation/Prepare -> Enforcement -> Evidence V3 prepare consume
-> Execute/Inspect -> Enforcement -> Evidence V3 execute consume
-> Tool Observation -> Tool DomainResultFact
-> Runtime Operation Settlement V4(ref only)
-> Tool ApplySettlement -> Tool Result
-> Application InspectCurrent V4 typed refs
-> public Inspect full Association prepare/execute
-> ContextTurnRefreshPortV1 S1/reserve/collect/admit/freeze
-> pending Context DomainResultFact
-> S2 -> Context Owner local atomic ApplySettlement + expected Generation CAS
-> settled new Frame exact ref/digest
-> Harness ActionContinuationBindingV1 + Continuation CAS
```

| 对象/动作 | 唯一Owner | Application可做 | Application禁止 |
|---|---|---|---|
| Tool Call Observation | Model Invoker | 传递完整sealed值并登记Evidence | 取首Call、改参数、生成PendingAction |
| Evidence record/sequence | Runtime Evidence | 调用治理Append/Inspect | 分配sequence、把Observation冒充Fact |
| `SettledTurnResultV2` | Intent绑定的Settlement Owner | 请求Owner Inspect并转交DomainResult | 代写action_required或选择Tool语义 |
| Session/PendingAction | Harness | 通过Application自有Port调用Harness模块内Adapter | 直接写Session或绕过ExpectedRevision |
| ActionCandidate/Tool Result | Tool Owner | 通过Application自有Port调用Tool模块内Adapter | import Tool、从Observation直建Candidate、决定Risk/Effect/Owner |
| V4治理、Evidence V3与Operation Settlement | Runtime | Issue/Inspect/Begin/Settle协调 | 写Permit/Enforcement/Evidence/Settlement Fact |
| Context Refresh/Frame/Generation | Context Owner | 通过Application自有Port调用Context模块内Adapter | import Context、重封Frame、跳过Context本地原子ApplySettlement/Generation CAS、忽略恢复后的Scope/epoch或Generation漂移 |
| Continuation | Harness CAS；Tool结果语义归Tool Owner；Frame语义归Context Owner | 通过Application自有Port调用Harness模块内Continuation Adapter | 仅凭Receipt、旧Frame、未settled Frame、裸Evidence pair或字符串ref继续下一Turn |

`N>1`只保存完整Observation和Evidence，Action Gateway停止：不得部分升级、不得隐式串行/并行、不得把`action.batch.completed`视为已支持。如何形成失败Settlement、预算分配、部分成功与Continuation需要后续独立设计裁决。

本链发生在Run内Model Turn之后，不产生pre-run权威Evidence，也不新增RunStartRequirement。首切面封闭矩阵固定为`OperationScopeKind=run`、`EffectKind=praxis.tool/execute`、`PolicyProfile=praxis.tool/single-call-action-v1`且Run/Session/Turn/Action/Context全部required；ConflictDomain仍来自Tool Owner封存的ActionCandidate/Intent。

Settlement V4的`OperationInspectionSettlementRefV4`直接持有Settlement、Association、Guard、Projection四类typed exact refs；完整prepare/execute通过公开`InspectOperationSettlementEvidenceAssociationV4`读取并与Inspection交叉校验。Harness不定义私有Evidence闭包对象。字段、Owner-current Reader、CAS和反例以[单Call Action Gateway V1](./action-gateway-v1.md)为准。

Session/Turn applicability分三层：Harness Reader seal真实可复读的distinct Session/Turn source coordinates并执行S1/S2，最长TTL为30秒；Runtime G6A router只把`Kind/ID/Revision/Digest`无损nominal projection为公共`OperationScopeEvidenceApplicabilityFactRefV3`；Harness `runtimeadapter`实现现有`OperationScopeEvidenceApplicabilityCurrentReaderV3`，通过构造期immutable Binding map恢复稳定Subject，每次Inspect用fresh clock生成Reader请求，再以第二次fresh clock验证`now>=checked && now<expires`、公共expiry不超过底层projection。live `OperationScopeEvidenceFactPortV3`没有Applicability Fact Create/Inspect，公共ref也不授Evidence资格。

`CommittedPendingActionApplicabilityBindingV1`把不含观察时间的`CommittedPendingActionSubjectV1`、预期Session coordinate、预期Turn coordinate及binding digest canonical seal为subject-scoped Adapter配置；`CheckedAt/Checked/Expires`不进入Binding identity。构造器deep-clone后，以两个coordinate各自的exact `Kind+ID+Revision+Digest`建立只读map；同键同binding幂等，同键换Subject/coordinate/digest冲突。运行期无注册、删除或替换入口，也不存在全局mutable registry。收到公共ref后exact lookup Binding，以fresh时间调用底层Reader重新执行S1/S2并在返回后再次采时验证；unknown ref、Binding缺失/漂移、Reader drift、clock rollback、TTL crossing、Scope/TTL漂移全部Fail Closed。Binding不是Fact/Authority/Evidence/Application DTO，不进入Runtime FactPort；G6A由test fixture手工注入，未来production root负责构造。

进入上述链之前还有Model exact入口门：Model Owner的公共Projection exact Reader与原子`Ensure`已经终审YES。Harness Adapter必须通过该Reader按完整`ToolCallCandidateObservationRefV1`复读`ToolCallCandidateObservationProjectionV1`，确认Ref/lineage/digest未漂移且`Calls==1`。Reader unavailable、Ref变化或Calls不等于1时零Application dispatch；PendingAction payload、event JSON与compat tool calls不能作为恢复来源。本设计不新增Model写口、Repository或`Ensure`实现。

G6A结果闭合后，G6B必须进入Context Owner已冻结的N=1 Refresh链：`Tool ApplySettlement -> ContextTurnRefreshPortV1 -> pending Context DomainResult -> S2 -> Context Owner local atomic ApplySettlement/Generation CAS -> Harness Continuation(new exact FrameRef)`。`CTX-D09-R1`明确本地迁移不创建、请求或消费Runtime Settlement。Application只调用自有窄Port；Tool、Context、Harness Adapter分别位于各Owner模块，允许依赖Application公开contract/ports但不得依赖Application coordinator/kernel/实现。Context Owner-local实现可用本Owner fixture隔离；G6A PASS后，G6B test-only跨模块fixture可手工注入公共Port/Adapter并验证完整链，但不是production root，不注册/激活Capability，也不生产调用Continuation或推进Turn。当前没有production composition root；宿主Owner只需在G6B生产启用前另行设计、实现、验收真实root。只有G6B完整验收与真实root接线Conformance同时PASS后，production Capability、Continuation和Turn推进才可GO。Harness Assembly只校验/组装已注入接口，不负责具体wiring、不import Tool/Context实现。

## 2. pre-run Evidence裁决

Assembly V1的`compile/validate/inspect/explain/diff/conformance`均为对既有签名引用、摘要和current facts的纯计算或只读检查，**不在Run前生成新的权威Evidence**。因此本设计不触发OperationScope-aware pre-run Evidence Delta。

- Conformance Report是Assembly产物，不是Runtime Evidence Fact；
- `EvidenceRefs`只引用上游已有Evidence，不重新签发；
- 远程探测、凭据验证、资源预热和真实Provider握手不属于V1编译；
- 将来若必须在Run前执行远程Probe，须另行提交Effect Kind、Operation Scope、Evidence Source Registration、Inspect/Settlement和兼容影响完整Delta，未获Runtime Owner确认不得加入。

## 3. Effect、Review、Fence与Unknown矩阵

`EffectKind`、`ConflictDomain`和Run Requirement由对应领域Owner的Versioned Port/Resolved Plan声明，Assembly只检查、绑定并固化引用，不能自行生成默认值。

| 动作类别 | Effect/Review | Fence、Authority、Scope、Budget | Unknown与结论Owner |
|---|---|---|---|
| Assembly compile/inspect/explain/diff | `none`；不要求Review | 校验输入Owner、Digest、Schema、Binding currentness；不分配Permit | 纯计算失败可用同输入重算；Assembly拥有Diagnostic/Generation |
| Model Turn | Model Invoker声明的namespaced Effect Kind；按Policy决定Review | RouteID + Runtime Operation V3；宿主Gateway和执行点双重校验 | Begin后只Inspect原attempt；Model Invoker归一化Invocation Observation/Result投影，Runtime Settlement Owner提交Operation Settlement，Harness只消费精确引用 |
| Action/Tool/MCP Dispatch | 首切面仅`praxis.tool/execute`；Action Candidate经Admission/Review | `run` scope与Run/Session/Turn/Action/Context五维current refs、Fence、Authority、Budget、ConflictDomain全部绑定Permit | Tool Owner Inspect/CAS/ApplySettlement；Runtime V4绑定prepare+execute Evidence；Harness在waiting_action等待Tool Result、current V4 Inspection与公开Inspect的完整Association |
| Context远程物化/Cache写 | Context&Cache声明读取披露或写入Effect；按数据策略Review | 数据Scope、Route支持、Disclosure、Budget、TTL | Owner Inspect；未闭合`ContextReference`必须Fail Closed或Residual |
| Sandbox allocate/open/execute/release | Sandbox声明资源Effect；高风险必须Review | Identity Lease、Fence epoch、Authority、Scope、Budget；执行点再验 | Sandbox Inspect/Cleanup/Settlement；本地盘不是唯一事实源 |
| Review远程调用 | Review声明Effect；Verdict本身不是Permit | Review Case、Policy、Authority、TTL/currentness | Review Owner Inspect/CAS；Harness仅Gate等待 |
| Memory/Knowledge/Asset commit | 各领域声明Commit Effect | Tenant/Agent/Run/Record Scope、Authority、Budget、ConflictDomain | 领域Owner Inspect/CAS/Settlement；模型输出仅Candidate |
| Timeline/Checkpoint/Restore | Continuity声明持久化/恢复Effect | Run/Branch/Checkpoint Scope、Fence、Authority、Budget | Continuity Inspect/CAS/Settlement；外部世界不宣称回滚 |
| Cleanup/remote inspect | 原领域Cleanup/Inspect Operation | 原资源、Attempt和Fence关系必须可追溯 | Owner形成Cleaned/Residual Fact；Harness只报Claim/Observation |

未在sealed Graph中绑定EffectKind、ConflictDomain、Owner、Inspect和Settlement的effectful Port不得实例化。ConflictDomain至少绑定租户/组织、领域Owner和稳定资源/Operation Scope；Assembly不得以SlotID或进程地址代替领域冲突键。

## 4. RunStartRequirement、OperationScope、RunSettlementRequirement、Cleanup与Residual

Assembly必须把三类对象分开编译，禁止仅因它们“与某个Run相关”就混为同一Requirement：

1. `RunStartRequirementV1`：启动前必须具备的View、Snapshot、Capability、Binding和current refs；属于Plan/Activation前置条件。
2. `OperationScopeV3`：某次Effect的run/admin/activation/termination/custom subject；不自动阻塞CompleteRun。
3. `RunSettlementRequirementV2`：仅含`run_completion`或`termination_report`阶段，必须绑定唯一Owner Participant Fact。

Trusted Assembler按Requirement ID、Owner、Subject Digest、Phase和Policy精确认证；组件只发布声明，不能自签Run Plan。

每个effectful Required Slot必须在编译期声明精确OperationScope；若其启动前置条件存在，则关联RunStartRequirement；只有确实阻塞CompleteRun或termination report时才关联领域Owner发布的RunSettlementRequirement/Participant Ref。Assembly只验证引用完整性和版本，不创建声明。最低覆盖：

- Model Turn的未决Operation、Model Invoker Invocation Observation/Result投影与Runtime Operation Settlement；
- Action/Tool/MCP的Pending Action、Inspect和Tool Result Settlement；
- Sandbox资源、Cleanup/Residual；
- 被启用的Context远程物化、Memory/Knowledge/Asset Commit；
- 被启用的Timeline/Checkpoint/Restore；
- Harness Completion Claim和Runtime Run Settlement的既有接入关系。

Run完成前，Application Coordinator聚合各领域`RunSettlementRequirement` Participant Fact；Runtime据此执行独立CompleteRun。它们与每次Effect的Runtime Operation Settlement是不同对象。Harness `terminal`、Endpoint `close`或`Cleaned` Claim都不能替代Runtime Outcome。

Residual必须包含`ResidualClass`、Owner、Scope、SourceAttempt、Inspect/Cleanup Contract、当前状态、允许继续的Policy Ref与下一Inspect条件。必需Slot、Fence、Authority、未决高风险Effect或不允许Residual存在时Fail Closed。

## 5. Runtime、Harness、Application映射

| 语义 | Runtime | Application Coordinator | Harness/Assembly |
|---|---|---|---|
| Definition解析与组件选择 | 消费绑定结果 | 协调上游请求 | Assembly只消费Resolved Plan |
| Binding/Activation/currentness | 唯一Owner | 发起/监督 | 产出handoff candidate，不写Binding Fact |
| Operation Governance V3/V4 | Admission、Review Authorization、Permit、Begin、Delegation、Fence与Settlement入口 | 组织跨域调用和Claim摄取；V4接线冻结前不得降级回V3冒充完成 | 绑定`runtime.gateway`，不复制治理链 |
| Run/Outcome/CompleteRun | 唯一Owner | 聚合Settlement并请求完成 | Run-local Loop与Completion Claim |
| 领域Fact与Settlement | 只保存/关联Runtime事实 | 请求领域Owner Inspect/Settlement | 仅消费稳定Ref |
| Slot/Phase公共装配 | 不拥有组件语义 | 不绕过Compiler直装Kernel | Assembly拥有公共对象、合并与CompiledGraph |
| 六组件贡献 | 不读组件内部类型 | 通过版本化Port关联 | 只接受namespaced/versioned Contribution |

公共/私有Port边界：

- Runtime公共入口：`runtime/ports.ExecutionPort`及经联合评审冻结的Binding/Operation/Application Ports；
- Harness私有装配缝隙：`harness/ports.ContextPort`、`ModelTurnPort`、`EventCandidatePort`，只能由Harness Adapter实现，不向六组件暴露；
- 六组件公共面：各自Versioned Domain Port、Manifest、Run Requirement、Inspect/CAS/Settlement；
- Model Invoker面：仅`RouteID`和公开`routegateway`/`execution`/`union`，禁止`internal`、厂商SDK、Raw/Native事件。

## 6. 依赖DAG与冲突检查

```text
Agent Assembler Final Output
  -> AssemblyInputV1
    -> SlotSpec/HookFaceSpec Catalog
    -> six-domain Contribution + PortSpec + Run Requirement
    -> Model Invoker RouteID/public execution union
    -> Runtime BindingSetV2/current facts
      -> Assembly Compiler
        -> AssemblyManifestV1
        -> CompiledHarnessGraphV1
        -> Conformance/Residual Reports
          -> Binding V2 mapping
            -> Runtime Activation
              -> Harness Run-local Session
                -> Application settlement aggregation
                  -> Runtime CompleteRun
```

Compiler必须拒绝：DAG环、缺失Owner、重复唯一Binding、Schema/Contract不兼容、摘要漂移、越权HookFace、Filter write-set无合并规则、Effect缺ConflictDomain/Inspect/Settlement、Run Requirement缺失、ContextReference必需但Route不支持、Binding/Evidence过期，以及组件自造Slot/Hook/Phase ID。

## 7. 当前公共装配硬阻塞

以下事项需要Harness接线设计统一，但在联合评审冻结前不得私建替代合同：

1. Agent Assembler最终输出到`AssemblyInputV1`的标准映射；
2. Assembly SDK、`AssemblyManifestV1`与`CompiledHarnessGraphV1`的公共合同；
3. Slot/Phase Contribution的合并、冲突和确定性排序规则；
4. Assembly Generation/Graph到Runtime Binding V2的正式映射；
5. 单Call Action Gateway的设计已冻结；实现仍依赖Settlement V4中央验证、Action封闭矩阵、五维Owner-current Readers、Tool Result V4 current合同与Application协调Port；
6. Checkpoint和per-turn Context refresh的端到端接线。

这些阻塞限制“开始实现”，不否定Runtime P0.1-P0.6、Operation V3和Application Coordinator已经闭合。
