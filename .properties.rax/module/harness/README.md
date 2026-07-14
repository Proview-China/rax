# Harness模块说明

## 1. 作用

Harness是Agent执行外壳的公共行为层。它消费上游已经编译完成的配置事实，组织当前Run内部的Context准备、Model Turn、Action/Input等待、取消、事件和完成Claim，并通过Runtime的`ExecutionPort`进入实例生命周期。

它不替代Runtime、Model Invoker、Context、Tool/MCP、Review、Memory或Sandbox。

## 2. 已落地组成

| 组成 | 作用 | 当前状态 |
|---|---|---|
| `contract` | 冻结Bootstrap、Manifest、Run/Event/Action/Claim及Governed Session/Candidate | 已实现 |
| `ports` | Context、Model Turn、Event Candidate与Governed Fact窄接口 | 已实现 |
| `kernel` | legacy交互循环及不执行Provider的Governed Candidate准备器 | 已实现首切面 |
| `runtimeadapter` | 接入Runtime现有`ExecutionPort`；显式V2治理身份与关闭后恢复观察 | 已实现首切面 |
| `applicationadapter` | Application V3 Model Turn Domain到Harness持久事实接线 | 已实现 |
| `bridgecontract` | Application专用Reservation/Binding桥合同；不污染Harness核心Session语义 | 已实现 |
| `fakes/tests` | 确定性替身与完整闭环验收 | 已实现 |

## 3. 关键合同

1. `EffectiveProfile`等配置由上游编译，Harness只消费不可变摘要，不在运行时重新合并Profile；
2. Runtime stable session、Harness稳定session与Provider native session永久分离；前两者分别由Runtime Endpoint+Run和完整Harness RunRef确定性派生，Provider native session只作为Observation；
3. `action_requested`必须停在外部Gateway边界；legacy路径仅校验ActionRef与Intent/Fence，不声称已完成Review V2结算；
4. Event Candidate成功保存后才允许下一步；Endpoint与source identity按完整Execution Scope+Run分区，任何append错误都先按精确source key Inspect；
5. Cancel单调收紧，进行中的Model调用被取消，晚到结果不能复活Run；
6. Completion Claim由Application `RunCoordinatorV3`按精确Candidate摄取为Runtime Evidence Association；它仍不直接收敛Run，Runtime Settlement Owner独立复读Execution、Effect和Participant后派生Outcome；
7. 首切面Checkpoint显式不支持，Descriptor不声明该能力。
8. Model调用或调用后Evidence持久化不确定时进入`waiting_reconciliation`，禁止盲重派；
9. 未显式注入Binding V2 Manifest的legacy Adapter不能获得V2 Binding资格；显式注入后旧Open/Control/Close也会拒绝。Application V3 governed Domain桥是独立路径，注册/Probe/Conformance和legacy调用都不自授生产或dispatch资格。
10. Governed Session/Candidate持久切面允许自定义同Component多Capability，但Harness与Model exact Capability必须分离；所有身份时间预分配，Create/CAS丢回包只exact Inspect。
11. Governed Session只有在持有Runtime公开的Admission→Permit→Delegation→Prepared→Enforcement引用后才能进入`model_in_flight`；Provider Observation只能推进到`waiting_settlement`。Action/Input/非取消terminal必须再持有同一attempt的Runtime Settlement，并由其schema+digest绑定的规范化DomainResult唯一派生；Harness仍只产生Claim，不产生Settlement或Outcome。
12. `GovernedRelayV2`只做宿主到Data Provider的翻译与回包恢复；Prepare/Execute未知结果各调用一次本地Inspect，Inspect不能证明时返回indeterminate，绝不二次调用Provider。
13. `CheckGovernedProviderV2`可直接复用于用户自定义Provider：并发Prepare必须content-idempotent且create-once，Prepared/Attempt Inspect必须exact；Conformance只形成认证候选，所有生产、Binding、Dispatch、Settlement和Completion资格固定为false。
14. Relay把Unavailable、Indeterminate、取消和超时都视为潜在丢回包，并拒绝Provider在Permit、payload、attempt、provider binding或delegation上的任何替换；TurnState重放同时绑定Settlement、DomainResult、派生sidecar/Claim和预分配时间。
15. `ModelTurnDomainAdapterV3`只服务配置时冻结的一个namespaced StepKind和一个Domain Adapter Binding；它严格解码Candidate Effect，并把Application attempt持久绑定到Run/Session/Candidate和完整Delegation route。用户新增组件必须拥有自己的Domain Adapter，不能借此入口越权。
16. `ModelTurnOperationBindingFactV3`不信调用方摘要：prepared/unknown/observed/settled分别从内部Runtime/Delegation/Authorization/Observation/Settlement/DomainResult重算Basis；Application、Runtime和Harness的Settlement必须完全一致。
17. Model Turn dispatch前，Harness Fact Owner原子提交Session `model_dispatch_reserved`与create-once Reservation索引；64个不同Attempt竞争同一Session+Candidate时仅一个获胜。首次创建校验current，历史过期Fact仍可Inspect用于恢复，但不恢复dispatch资格。
18. `harness/contract`不导入Application或`bridgecontract`；未来合法namespaced组件通过版本化Port和自己的exact Adapter绑定接入，不需要修改Runtime/Harness kind枚举。
19. pre-prepared unknown只允许failed terminal且没有Runtime/Delegation sidecar；post-prepared unknown先进入reconciling，并要求Runtime提供独立Inspect Effect+Settlement provenance。

## 4. 已验收闭环

```text
Runtime Registry
-> Activation / Preflight / Open / independent Ready
-> Runtime StartRun
-> ExecutionPort.Control(start_run)
-> Harness Context / Model Turn / Events
-> direct completion
   or action_requested -> external result -> completion
-> Runtime Stop / Harness Close / Sandbox Release
-> terminal cleanup
```

当前120个顶层Test/Fuzz入口（其中4个Fuzz入口）分为单元、包内白盒和外部黑盒三层。普通、Reservation与三模块集成100轮、Race 20轮、全量Race与Vet均通过；本轮生产包跨包语句覆盖率79.3%。覆盖合同漂移、证据过期、错误ActionRef、事件背压、取消/迟到结果、跨租户同ID、同Instance顺序Run、原子Reservation、自定义Provider Conformance、丢回包、治理换链、Inspect provenance、内部因果换包、真实Application Coordinator接线和Runtime Foundation兼容路径。

最终组合验收使用真实Runtime Binding/Plan Certification/Operation Governance/Run Claim/Settlement事实、真实Application Coordinator与真实Harness governed relay。正常链收敛到Runtime Termination Report与Cleanup closed；unknown链证明Provider Execute仅一次，连续Resume不重派，恢复只能通过独立Inspect。自定义namespaced组件通过Manifest、Capability、Domain Adapter和Port接入，不要求Runtime、Application或Harness增加组件kind分支。

## 5. 未实现

- 任何具体Codex、Claude、ACP或自定义生产Harness；
- 真实Model Invoker桥接、Tool/MCP执行和Review Engine；
- 生产Event Store、Session Store、Scheduler、RPC和进程拓扑；
- Checkpoint/Restore、跨Run Session、生产Inspect Provider与Settlement Owner；
- 生产级Claim/Evidence、Run Settlement、可信Plan Assembler和Termination事实后端；当前公共协调合同及其Runtime Port实现使用确定性测试替身，不宣称生产能力。
- 生产宿主Execution Adapter到Data Plane Harness Provider、Runtime Gateway与持久State Plane后端。

设计事实源见[Harness设计入口](../../design/harness/README.md)，实现入口见[Go模块说明](../../../ExecutionRuntime/harness/README.md)。
