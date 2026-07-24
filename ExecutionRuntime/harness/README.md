# Praxis Harness公共基座

本模块实现组件中立的Harness公共合同与最小交互循环。它把已经编译完成的Profile/Runtime Policy/Harness Stack摘要作为不可变`BootstrapPlan`接入，并通过Runtime现有`ExecutionPort`参与Activation、Open、Ready、Control和Close闭环。

## 当前包含

- `contract`：Manifest、BootstrapPlan、Run State、Event、Action、Completion Claim，以及不反向依赖Application的Governed V2 Endpoint/Session/Candidate/Continuation；
- `bridgecontract`：承载Application与Harness之间的Model Turn Reservation/Binding不可变关联，并以类型别名/窄接口复用Application Review waiting的Phase/Target/Current对象；不复制Application DTO或进入Harness核心Session语义；
- `ports`：Context、单次Model Turn、Event Candidate及Governed Session/Candidate Fact Port；
- `kernel`：legacy最小交互循环，以及不执行Provider的Governed Candidate持久准备器；
- `runtimeadapter`：`ports.ExecutionPort`适配器，以及必须显式注入治理身份的`DescriberV2`；
- `applicationadapter`：把Application V3的单一namespaced Model Turn Step接入Harness持久Session/Candidate/Binding事实；
- `assemblycontract/compiler/adapter`：Controlled Operation Provider Route V2的Harness事实、required Manifest extension、全图no-bypass、fixture-only CAS Store与exact Runtime current Reader；
- `storage/sqlite`：同一WAL/FK数据库内的Governed Session V4 history/current与Event Candidate exact source journal；提供有界TTL的durable Session/Event proof current，但不发布整体ProductionReadiness；
- `assemblyadapter/*sqlite*`：单节点持久Route Declaration/Conformance/Current/Owner Artifact与M2/A2 Assembly current；提供immutable history、full-Ref CAS、ABA防护、S1/S2、restart、row/index digest和lost-reply Inspect；
- `modelinvokeradapter/sqlite_prepared_model_invocation_ack_repository_v1`：单节点持久Model ACK create-once三索引与Prepared+Current稳定键恢复；
- `releasecandidate`：向Agent Assembler发布完整但固定`reference_only`的Harness Component Release、Readiness、Conformance及descriptor-only Factory；
- `fakes`：确定性Context、Model和Event测试替身；
- `tests`：合同、Kernel和Runtime Foundation完整闭环测试。

## 所有权边界

- Runtime拥有实例生命周期、权威Run Record和Execution Outcome；
- Harness只拥有当前Run内部的Interaction Loop与Session状态；
- Harness的完成、取消和失败是Claim/Observation，不直接成为Runtime权威事实；
- Tool/MCP首切面只产生`action_requested`，等待Review/Gateway返回结果，不直接执行；
- 每次Model Turn dispatch必须带已持久化`EffectIntent`和当前`ExecutionFence`；
- Runtime Binding V2不从legacy Manifest猜测Locality、Owner或Residual；未显式提供V2 Manifest的适配器只能走受限兼容路径，已提供V2身份的宿主Adapter会拒绝legacy Open/Control/Close。Application V3 governed Domain桥是独立受治理路径，不把legacy调用升级为生产dispatch；
- Endpoint、Harness Session与Event按完整Execution Scope和Run分区；不同租户可复用全部本地ID，同一Instance的顺序Run不会复用Evidence source sequence；Runtime stable session、Harness session和Provider native session始终是三个独立身份；
- Event append的任何错误（包括原生timeout/cancel）都先按精确source key Inspect；模型调用或调用后Evidence持久化不确定时进入`waiting_reconciliation`，禁止盲目重派；
- 已关闭Endpoint只允许`cleanup`恢复观察，不能继续Ready/State/Event或Control；该观察仍不等于Runtime权威Cleanup事实；
- Harness私有ContextPort只接收已经物化的Context Snapshot，不得成为未记账网络、披露或Cache写入后门；
- 已提供Checkpoint Gate/Guard及Harness Participant公开Adapter，并以两个Participant完成Checkpoint-first跨Owner reference纵切；EffectCut绑定前驱Attempt revision，Gate绑定当前紧邻revision，终态以更高Attempt revision释放，重放只返回可证明的单调successor。当前仍不承诺生产Participant phase bridge、真实Snapshot Provider、跨Run/跨Instance Session恢复或Restore。
- Governed V2 Candidate只是一份不可变待治理输入；同一自定义Component可提供多个能力，但Harness和Model exact Capability不得复用；Session/Candidate的Create/CAS回包丢失只按exact Fact Inspect；
- Application适配器只接受配置时冻结的一个合法namespaced StepKind和Domain Adapter Binding，不硬编码6+1 kind；Delegation endpoint/session/host relay/provider、Application attempt、Runtime attempt、Candidate与scope必须完全一致。未来自定义组件可用相同公共Port实例化自己的Adapter，但不能借用另一个已绑定入口；
- Model Turn首次越过Runtime前，Harness同一Fact Owner必须原子提交`waiting_model_dispatch→model_dispatch_reserved` Session CAS与create-once Reservation Fact；同一Session+Candidate只有一个Attempt可获胜，回包丢失只按Attempt Inspect。Reservation绑定Candidate/Intent/Adapter及最短expiry，首次创建校验current；已提交的过期Fact仍可供历史恢复和收口，但不能重新授予dispatch；
- `ModelTurnOperationBindingFactV3`持久保存Application attempt到Run/Session/Candidate、完整Delegation和Runtime水位的关联；每个状态都从内部事实重新计算Application Basis，外部提供的摘要不能掩盖Observation、Authorization、Settlement或DomainResult换包；
- pre-prepared unknown只能从精确`model_dispatch_reserved`落到显式failed terminal；post-prepared unknown必须先进入`reconciling`，且Runtime Settlement携带独立Inspect Effect+Settlement provenance；两类路径都禁止盲目重派；
- terminal Event可由Application `RunCoordinatorV3`按精确source coordinates摄取为Runtime Claim Evidence Association；该Association仍不选择Outcome，Runtime Settlement Owner独立复读Execution/Effect/Participant后提交终态，unknown cleanup继续显式Reconcile；
- Controlled Operation Provider Route V2只拥有Assembly Declaration/Conformance/Current事实，不持有raw Provider句柄；CurrentID、Watermark和七Binding闭包复用Runtime公共合同，ProviderTransport与actual Provider保持两层独立Binding，V1/V2或任一图/wiring旁路均Fail Closed；
- Completion Review Gate V2只覆盖已登记的`subagent.completion.validate`与`run.completion.validate`。它在Application协调前后对同一Phase/Target current做S1/S2、fresh-clock与最短TTL校验，完整保留allow/deny/ask/defer；Harness不重投未知mutation，不写Completion Claim、Runtime Outcome、Verdict或Authorization，也不提供production root；

## 验证

在本目录执行：

```bash
go test -count=1 -shuffle=on ./...
go test -count=20 ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
go test -count=1 \
  -coverpkg=./contract,./ports,./kernel,./runtimeadapter,./applicationadapter,./fakes \
  -coverprofile=/tmp/praxis-harness.cover ./...
```

测试分为公共合同/Port单元测试、Kernel与Adapter白盒测试，以及Runtime Foundation与真实Application Coordinator跨模块黑盒测试。Completion Review Gate V2另有target100、race20、两completion phase、四态决策、S1/S2漂移、TTL crossing、clock rollback、unknown单调用、claim-lost-reply Inspect-only及64并发Conformance覆盖。测试总数与覆盖率必须从当前live代码重新生成，不在共享并行开发期间固化旧计数。

最终三模块组合测试使用真实Runtime Binding/Plan Certification/Operation Governance/Run Claim/Settlement事实、真实Application Coordinator和真实Harness governed relay。正常链一直收敛到Termination Report与Cleanup closed；unknown链固定Execute一次，连续Resume只Inspect且不重派。新增namespaced组件只需提供Manifest、Capability、Domain Adapter与Port实现，无需修改Runtime/Application/Harness的组件kind分支。

当前已有单节点SQLite Session/Event、Assembly Publication、M2/A2 Current、Route/Owner Artifact与ACK durable backend；仍没有M2独立Handoff current backend、真实Harness Provider、真实模型、真实Tool/MCP、生产Scheduler、跨进程RPC后端或可信Plan Assembler。这些backend不产生Runtime Outcome、整体production认证、deployment attestation、root或SLA；`releasecandidate`继续固定`reference_only`。Checkpoint-first reference纵切使用公开Runtime/Application/Continuity/Sandbox Port和测试fixture，证明合同组合与故障恢复，不是生产Snapshot capture或Provider认证。公共基座解锁6+1开发不等于生产部署完成。
