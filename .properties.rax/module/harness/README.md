# Harness模块说明

## 1. 作用

Harness是Agent执行外壳的公共行为层。它消费上游已经编译完成的配置事实，组织当前Run内部的Context准备、Model Turn、Action/Input等待、取消、事件和完成Claim，并通过Runtime的`ExecutionPort`进入实例生命周期。

它不替代Runtime、Model Invoker、Context、Tool/MCP、Review、Memory或Sandbox。

## 2. 已落地组成

| 组成 | 作用 | 当前状态 |
|---|---|---|
| `contract` | 冻结Bootstrap、Manifest、Run/Event/Action/Claim、Governed Session/Candidate及Committed PendingAction source coordinate/binding | 已实现 |
| `ports` | Context、Model Turn、Event Candidate、Governed Fact与Committed PendingAction只读窄接口 | 已实现 |
| `kernel` | legacy交互循环、不执行Provider的Governed Candidate准备器及Committed PendingAction S1/S2 current reader | 已实现首切面 |
| `runtimeadapter` | 接入Runtime现有`ExecutionPort`；显式V2治理身份与关闭后恢复观察；将Harness source coordinate复读为Runtime公共Applicability current projection | 已实现首切面 |
| `applicationadapter` | Application V3 Model Turn Domain到Harness持久事实接线 | 已实现 |
| `applicationadapter/single_call_tool_action_assembler_v2` | 只读聚合Session V4、Current V3、DomainResult、Model Projection与Runtime Authority，组装sealed Application Request并实现InputCurrent Reader V2 | 最终独立代码审计YES(P0/P1/P2=0) |
| `applicationadapter/g6a_cross_module_v2_test` | 显式测试组合Harness Assembler/InputCurrent、Application Coordinator与Tool V2 Adapter，闭合到settled ToolResult及current V4 Inspection/Association并验证重放只执行一次 | test-only composition candidate；不含Tool生产Execution bridge、Context Refresh、Continuation或production root |
| `bridgecontract` | Application专用Reservation/Binding桥合同；不污染Harness核心Session语义 | 已实现 |
| `assemblycontract/compiler/adapter` | Controlled Operation Provider Route V2的Declaration、多声明merge、结构化Conflict、Conformance、required Manifest extension、全图no-bypass与exact Route Current Reader | 第八独立短审YES(P0/P1/P2=0) |
| `contract/ports/kernel/fakes` Owner-current V3/V4 | Identity/Fact、Binding V2、Governed Session/CAS V4、Committed PendingAction Current/Reader V3与10-role owner-current exact读取 | 最终独立代码审计YES(P0/P1/P2=0) |
| `assemblyadapter/model_predispatch_assembly_current_v1` | Harness-owned A2 verified Assembly historical/current Store、publisher与Runtime neutral Current Reader；通过Runtime canonical Gateway与Tool C2公开Reader复读Owner current | `owner-local implementation_software_test_yes`；不外推Tool P4、system或production root |
| `modelinvokeradapter` | Model公开`PreparedModelInvocationCommitGateV1`的Harness concrete Gate与同实例ACK create-once Repository；按Prepared+Current恢复、Tool公开Binding Ensure/Inspect、S1/S2及fresh clock封存Model ACK | `owner-local implementation_software_test_yes`；不代表Model actual-point全路径或production root已接线 |
| `assemblyadapter/*sqlite*` | 持久保存Route Declaration/Conformance/Current/Owner Artifact及M2/A2 immutable history/current；提供full-Ref CAS、ABA防护、S1/S2、restart、row/index digest与lost-reply Inspect | 单节点durable backend候选；不等于真实route wiring、Handoff current闭包或deployment attestation |
| `modelinvokeradapter/sqlite_prepared_model_invocation_ack_repository_v1` | 持久保存Model ACK create-once三索引并按Prepared+Current稳定键恢复 | 单节点durable backend候选；不替代actual-point全路径guard |
| `releasecandidate` | 发布Harness完整声明式Release/Readiness/Conformance与descriptor-only Factory | `reference_only`；production P0见[模块说明](./component-release-v1.md) |
| G6B公共接线依赖 | Application `ContextTurnRefreshPortV1`与Context Owner三段Refresh合同 | live Application候选当前建模Memory/Knowledge且要求至少一项非空，不兼容`Tool=1 / Memory=0 / Knowledge=0 / Continuity=0`；[Port Delta](../../design/harness/port-deltas/context-turn-refresh-g6b-v1.md)为`candidate-p0`，Harness未接入 |
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
20. `CommittedPendingActionReaderV1`只签发Harness拥有的Session/Turn source coordinate；两类coordinate分别使用独立nominal type、canonical内容与digest domain，不是`OperationScopeEvidenceApplicabilityFactRefV3`，也不授予Evidence资格。
21. `CommittedPendingActionApplicabilityBindingV1`是immutable、deep-cloned、canonical-sealed的Runtime Adapter构造配置，只封存稳定`CommittedPendingActionSubjectV1`与source coordinates；`CheckedAt/Checked/Expires`等观察时间不进入Binding identity。Adapter仅在构造期接受有限binding集合，以公共ref四字段exact索引；同键同binding幂等，同键换内容冲突，运行期没有注册、删除或替换入口。
22. Applicability current读取会按binding恢复稳定Subject，以第一次fresh clock生成本次Reader request；Reader返回自己的Checked/Expires后，Adapter再采第二次fresh clock验证`now>=checked && now<expires`、公共expiry不超过底层projection，同时重新执行S1/S2并核对Execution Scope与source coordinates。unknown、drift、clock rollback、TTL crossing、expiry或Reader不可用均Fail Closed。
23. Controlled Operation Provider Route V2只由Harness拥有Declaration/Conformance/Current Fact语义；Runtime只拥有六个中立Route公共类型。CurrentID确定性派生，Current Store使用单调CAS，Reader按exact Ref执行S1/current inputs/S2；ProviderTransport与actual Provider分别绑定，任何Slot、Factory、Dependency、Hook/Phase、V1 route或真实wiring alias旁路均Fail Closed。
24. Owner-current V3 Reader只接受Runtime窄`OperationSettlementCurrentReaderV3`与其余公开只读Reader；按Session S1→Candidate→DomainResult/Identity→Model Projection→Settlement→Association→Generation→Route→10-role Provider Binding canonical分组单读→Context→Session S2→fresh clock执行。Binding V2把Model Operation digest与Settlement attempt exact绑定，V4 Session在一次CAS写完整PendingAction+Binding；任一valid owner splice、clock rollback、TTL crossing、typed nil或deep alias均Fail Closed。
25. `SingleCallToolActionAssemblerV2`只持`SessionCurrentReaderV4`、`CommittedPendingActionReaderV3`、`SettledTurnDomainResultReaderV3`、Model公开exact Projection Reader与`AuthorityFactReaderV2`五个窄只读能力。Request与InputCurrent均执行完整S1/S2；S2 Current租约只能不变或收窄，所有proof在S2完成后的fresh `nowS2`封装，Model canonical arguments逐层deep-copy。该候选不包含Tool调用、Application Coordinator、P4/system fixture或production root。
26. M2 Assembly Current已按A2+B1+C2闭合Owner-local实现：A2保存并复读完整verified Compile/Manifest/Graph/Handoff/Conformance，B1只通过Runtime canonical GenerationBindingAssociation Gateway重建current，C2只通过Tool公开ToolSurface current Reader复读；caller echo不能替代Owner事实。in-memory与SQLite historical Store、stable current index、full Ref CAS、ABA防护、hard-cap Unknown恢复、publisher与Runtime neutral Current Reader均已落地。SQLite实现增加单事务S1/S2、返回/提交前fresh-clock TTL、row/index digest、restart与lost-reply Inspect。该结论不解锁独立Handoff current、Tool P4、system fixture或production root。
27. `PreparedModelInvocationSurfaceCommitGateV1` exact实现Model公开短方法`Commit + InspectExactAck`，只依赖Model公开Prepared/Current Readers、Runtime neutral Assembly Current Reader及Tool公开Surface/Binding Port。`Commit`第一项Owner调用固定为同实例ACK Repository按完整Prepared+Current恢复；authoritative absent后才执行Prepared/Assembly/Surface S1、Tool winner Inspect/Ensure、S2、fresh clock Seal与ACK Ensure。ACK Repository以同一锁域维护`by_ack_id/by_prepared_current/by_prepared_ref`，create-once、same Prepared换Current Conflict、lost reply exact Inspect、TTL/clock rollback及deep clone均Fail Closed。Tool P4-1 contract/repository独立全绿并冻结hash后，Gate定向ordinary/race与Harness full ordinary/race/vet复跑仍全部PASS；Model actual-point guard全路径、Tool P4消费与production composition仍需各Owner独立验收。
28. G6A test-only组合以公开合同串接真实Harness Assembler/InputCurrent、Application Coordinator与Tool V2 Adapter，输出止于settled ToolResult及current V4 Inspection/Association；binding/settlement fixture按exact request复读，重放只允许Inspect且Tool执行计数保持一次。最终Tool执行面仍为测试fixture，因此该证据不授system G6A、Context Refresh、Continuation或生产能力。

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

当前测试资产分为单元、包内白盒和外部黑盒三层；按fresh确定性口径`go test -count=1 -coverpkg=./... -coverprofile=<temp> ./...`并以`go tool cover -func=<temp>`读取total，实测跨包语句覆盖率74.8%。普通、定向100轮、Race 20轮、全量Race与Vet的本次最终结果以任务回传为准；覆盖合同漂移、证据过期、错误ActionRef、事件背压、取消/迟到结果、跨租户同ID、同Instance顺序Run、原子Reservation、自定义Provider Conformance、丢回包恢复、治理换链、Inspect provenance、Owner-current valid splice、10-role分组单读、V2/V3/V4反向占键、deep no-alias和Runtime Foundation兼容路径。

最终组合验收使用真实Runtime Binding/Plan Certification/Operation Governance/Run Claim/Settlement事实、真实Application Coordinator与真实Harness governed relay。正常链收敛到Runtime Termination Report与Cleanup closed；unknown链证明Provider Execute仅一次，连续Resume不重派，恢复只能通过独立Inspect。自定义namespaced组件通过Manifest、Capability、Domain Adapter和Port接入，不要求Runtime、Application或Harness增加组件kind分支。

## 5. 未实现

- 任何具体Codex、Claude、ACP或自定义生产Harness；
- 真实Model Invoker桥接、Tool/MCP执行和Review Engine；
- Application生产协调接线、Tool Consumer/P4、system fixture与生产composition root；Harness P3只读Assembler/InputCurrent Reader已通过最终独立代码审计，但只支持显式测试fixture手工注入，Harness Assembly不承担具体wiring；
- G6B Application公共三段Refresh合同仍需联合闭合：Harness只接受settled ToolResult + current V4 Inspection + full Association且Memory/Knowledge Reader调用数为0，不会为当前live候选伪造source Envelope或建立私有兼容Adapter；
- Controlled Operation Provider Route V2已完成Owner-local合同、编译器、内存Store、单节点SQLite Route/Owner Artifact durable backend与只读Adapter；仍没有真实双层route wiring、composition root或Capability启用；
- Session/Event、Assembly Publication、M2/A2 Current、Route与ACK已有单节点SQLite backend；仍没有跨进程Store、Scheduler、RPC、进程拓扑、deployment attestation或生产SLA；
- M2的独立Handoff current exact Reader/backend尚未闭合；现有Publication Store不能从`GenerationID/handoff`无损取得scope/InputDigest，禁止私建sidecar；
- Checkpoint/Restore、跨Run Session、生产Inspect Provider与Settlement Owner；
- 生产级Claim/Evidence、Run Settlement、可信Plan Assembler和Termination事实后端；当前公共协调合同及其Runtime Port实现使用确定性测试替身，不宣称生产能力。
- 生产宿主Execution Adapter到Data Plane Harness Provider、Runtime Gateway与持久State Plane后端。

设计事实源见[Harness设计入口](../../design/harness/README.md)，实现入口见[Go模块说明](../../../ExecutionRuntime/harness/README.md)。
