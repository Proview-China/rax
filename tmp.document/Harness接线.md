## Harness接线 Documentary

### 综述

- 这一部分是六加一中的那个一.前面的Continuity,Tool&MCP,Memory&Knowledge,Sandbox,Review和Context&Cache分别拥有自己的领域语义,而Harness接线负责把这些已经成立的组件组合成一个真正可以运行的Agent.
- Harness接线不是第七个业务组件,也不是一个更大的Runtime.它是Agent执行外壳中的公用部分,负责把已经解析好的Profile,Model Route,Context,Action,Review,Sandbox,Continuity和其他模块贡献接入同一条Run内交互循环.
- 我们正在构建的是一个框架,所以这里不能只做一套Praxis内部写死的接线.开发者应该可以声明自己的模块,Slot,Port和Phase贡献,再由Assembly SDK完成校验,编译,预览和交付.
- 但是可插拔不等于任意代码都可以进入循环.每一个接线位置都必须拥有明确的类型,Owner,生命周期,数量,并发和失败语义;每一个扩展也必须经过版本,能力,权限和兼容性检查.
- Harness负责当前Run内部的Session,Turn,Model和Action协调;Runtime负责Identity,Instance,Run Record,Lease,Fence,Operation Governance,Execution Outcome,真实失败和Cleanup权威事实.
- 所以Harness产生的Ready,Completed,Failed,Cleaned等内容都只能先成为Observation或者Claim,不能因为Harness自己这么说就直接升级成Runtime事实.
- 现有Agent Definition,Profile System和Agent Assembler已经形成了明确的上游装配链.这一次不能再制造一个与Agent Assembler竞争的第二装配器.
- Agent Assembler负责把Definition,Profile,版本,能力和依赖解析成不可变ResolvedAgentPlan;Harness Assembly Compiler只负责把这个已解析计划和实际模块贡献编译成可执行接线图;Runtime再负责认证,激活和运行.
- 我们可以学习Codex的Thread,Session,Task,Turn,Step和Event切分,学习Claude Code与Claude Agent SDK的流式Query,Permission和Hook能力,也可以学习OpenAI Agents SDK的Agent,Runner,Guardrail,Approval,Handoff和Lifecycle Hook.
- 但是我们不能照搬它们已经逐渐膨胀的中心循环,也不能把Hook,权限,审批,上下文注入和真实执行混成一个万能回调.我们需要的是一套高性能,可组合,可证明并且能够持续增加模块的Harness公共基座.

### 关于六加一中的一

- 六个头等组件分别回答自己的领域问题:Continuity回答如何留下历史和恢复,Tool&MCP回答如何声明与调用能力,Memory&Knowledge回答如何形成可复用认知,Sandbox回答在哪里以及在什么边界执行,Review回答什么可以继续,Context&Cache回答模型这一轮究竟应该看见什么.
- Harness接线回答的是另外一个问题:这些组件在一个Agent Run中按照什么顺序被调用,如何交换稳定对象,如何暂停和继续,如何把Model输出变成Action Candidate,又如何把执行结果送回下一轮Context.
- 所以Harness不复制六个组件内部的类型和算法.它只消费它们提供的Versioned Port,Manifest,Capability和受治理的结果对象.
- 一个组件可以拥有自己的后台服务,数据库,远程进程,Container,MicroVM或者WASM Runtime.Harness不要求所有组件都和自己处于同一进程,只要求绑定到当前Run的Port满足同一个合同.
- Harness也不等于某一家官方Agent产品.Codex,Claude Code,Claude Agent SDK,OpenAI Agents SDK或者以后其他Harness都只能成为具体Route,Adapter或者Provider,不能反过来定义Praxis全部Agent的公共语义.
- Model Invoker仍然拥有Model Family,Provider Route,调用协议,重试和厂商差异.Harness只通过ModelTurn Port调用已经选定的Route,不能把Model Invoker吞进自己的Kernel.
- Application,CLI,SDK,API,REPL和UI属于外部使用面.它们可以创建装配请求,观察运行和提交控制意图,但是不能直接越过Runtime去操作Harness内部状态.

### 关于三段装配链

- 这里需要把Definition Assembly,Harness Assembly和Runtime Activation严格分开.
- 第一段是Agent Assembler.它消费AgentDefinition,Effective Profile,组件注册表,版本约束,组织权限和目标Runtime能力,产出不可变ResolvedAgentPlan和HarnessBootstrapPlan.
- 第二段是Harness Assembly Compiler.它消费ResolvedAgentPlan,HarnessBootstrapPlan,实际Module Descriptor,Slot Contribution,Port Binding和RouteBinding,验证Expected与Actual是否一致,然后产出CompiledHarnessGraph,AssemblyManifest,RuntimeProviderBinding和编译报告.
- 第三段是Runtime.Runtime根据计划摘要,证据,TTL,Conformance,Lease,Fence和当前事实完成Preflight,Open,Start,Inspect,Control,Settlement和Close.
- Harness Assembly Compiler不能重新选择组件版本,不能重新合成Profile,不能解释用户意图,不能扩大Capability Grant,也不能为了让编译通过而偷偷降级安全要求.
- 同一组确定输入和同一组组件事实必须产生相同的Assembly Manifest和Graph Digest.如果实际Route,工具面,Context Plan,Hook,Skill,MCP,权限或者Sandbox发生漂移,应该拒绝或者生成新的Plan和Assembly Generation.
- Harness运行中不得热改核心接线图.动态Tool,Skill,MCP和其他能力可以在Plan已经声明的能力包络内启停,但是更换Slot Owner,增加新的外部Effect,改变权限或者改变核心Phase顺序都必须重新装配.
- 这条链的本质是:Agent Assembler决定应该使用什么,Harness Assembly Compiler证明现在真正接上了什么,Runtime决定这一套东西此刻能不能运行.

### 关于统一Envelope和对象引用

- 七个组件需要共享最小Envelope和错误模型,但是不能为了统一而把所有领域对象塞进一个万能Event或者Provider接口.
- 适用对象至少携带ContractVersion,ObjectID,Revision,Owner,Scope,CreatedAt,Digest和EvidenceRef;需要当前性控制的对象再携带Epoch,ExpiresAt,PolicyRef和CapabilityGrantDigest.
- 跨组件传递优先使用带Revision和Digest的稳定Ref,大型Payload进入Artifact或者领域存储,避免在Harness热路径和Continuity Event中反复复制.
- Candidate,Observation,Attestation,Decision,Permit,Settlement和Fact必须保留类型.统一Envelope只解决身份,版本,范围和追踪,不能把它们升级成同等权威.
- 所有可能重试的命令都需要Idempotency Key;所有当前状态更新都需要Expected Revision和CAS;所有进入外部执行面的行为都需要Intent Revision,Payload Digest,Attempt和Fence.
- Error至少区分Rejected,Failed,Indeterminate,Cancelled,Expired,Conflict,Unavailable和Residual.Rejected表示尚未进入执行;Indeterminate表示可能已经产生Effect,后续只能Inspect;Residual表示主流程结束以后现实世界仍有未收敛部分.
- 各领域可以拥有自己的详细错误和状态机,但是不能把Indeterminate压缩成普通Failed,也不能把Provider Success直接翻译成Runtime Outcome.
- Versioned Port描述跨组件稳定语义,Domain Adapter负责本地或远程实现差异,Manifest和Capability描述装配事实.组件之间不能依赖对方的数据库Schema或者内部对象指针.

### 关于Slot

- Slot不是模块,不是Plugin,也不是一个任意名称的map key.
- Slot是Harness中一个具有稳定类型,明确Owner,数量约束,生命周期和失败语义的接线位置.
- 一个模块可以向多个Slot提供Contribution;一个Slot也可以拥有一个Owner和多个Source,Provider,Filter或者Observer.
- Owner负责该Slot的核心语义和唯一当前结论;Contributor只能按照Slot合同贡献材料或者能力,不能因为被注册进来就获得整个Slot的控制权.
- 比如Context Engine是context.frame的Owner,Memory,Knowledge,Tool Result和Continuity都可以贡献Context Source;但是这些来源不能自己决定最终Frame顺序和Token预算.
- Tool&MCP可以拥有action.router并接入多个Tool Provider和MCP Provider;Review可以在action.review阶段形成Verdict;Sandbox可以在真实执行点实施权限;但是普通Hook不能取代这些Owner.
- 每一个SlotSpec至少需要包含SlotID,ContractVersion,LifecycleScope,Cardinality,Required,OwnerCapability,AllowedContributionKinds,InputSchema,OutputSchema,EffectClass,ConcurrencyPolicy,FailurePolicy,DegradationPolicy,Dependencies和Digest.

### Slot分类

| 分类 | Slot | 数量 | 作用 |
|---|---|---:|---|
| 核心执行 | `kernel.loop` | 1 | 当前Run内的状态机与交互循环 |
| 核心执行 | `model.turn` | 1个Active Binding | 调用已经选定的Model Invoker Route |
| 核心执行 | `context.frame` | 1 Owner + N Source | 收集,编译,冻结并交付Context Frame |
| 核心执行 | `event.candidate` | 1 | 向Runtime和Continuity提交有序Event Candidate |
| 动作边界 | `action.router` | 1 Owner | 协调Tool,MCP和其他Action Candidate |
| 动作边界 | `tool.provider.*` | 0..N | 提供类型化Tool能力 |
| 动作边界 | `mcp.provider.*` | 0..N | 提供MCP Server,Tool和Resource能力 |
| 动作边界 | `review.gate` | 1 Owner + N Reviewer | Human,Auto和Bypass审核路由 |
| 动作边界 | `sandbox.execution` | 1 Active Binding | 实施Workspace,进程,网络和外部执行边界 |
| 状态组件 | `continuity.timeline` | 1 Owner + N Source | Timeline,Checkpoint,恢复和历史引用 |
| 状态组件 | `memory.state` | 0..N | Memory Candidate,Recall和Commit Port |
| 状态组件 | `knowledge.query` | 0..N | Knowledge Query,Projection和Evidence Port |
| 状态组件 | `asset.store` | 0..N | Artifact,Diff,截图,录屏,报告和大型结果 |
| 治理引用 | `identity.authority` | 1 Ref | Agent Identity,Authority和Accountability引用 |
| 治理引用 | `organization.policy` | 1 Ref | 组织策略,业务域和管理边界 |
| 治理引用 | `budget.policy` | 1 Ref | Token,费用,Turn,时间和资源预算 |
| 治理引用 | `evidence.sink` | 1 | Evidence Candidate和可验证Receipt出口 |
| 治理引用 | `runtime.gateway` | 1 | Runtime Fence,Control,Inspect和Settlement入口 |
| 治理引用 | `management.control` | 0..1 | 暂停,继续,终止和外部管理意图 |
| 扩展组件 | `domain.<namespace>.<name>` | 0..N | 财务,调度,外部接口和未来领域模块 |

- 治理引用Slot并不把Runtime,Organization或者Management的权威搬进Harness.它只让Harness携带并使用已经认证的引用和Port.
- 新组件不能通过在Runtime,Harness或者Application中增加新的component kind分支完成接入.标准方式应该是Namespaced Manifest,Capability Descriptor,Domain Adapter,Versioned Port和Slot Contribution.
- Slot的核心拓扑在Assembly Generation内保持不变.任何运行期动态能力都必须受到原有Manifest,Capability Grant和Degradation Policy约束.

### 关于Hook Phase

- Praxis需要提供完整的生命周期扩展面,但是不应该把所有扩展都叫做拥有相同能力的Hook.
- Claude Code的Hook可以在很多位置注入Context,修改Tool Input,拒绝调用,请求用户批准或者阻止Agent停止.这种能力非常灵活,但是也容易让普通扩展获得过大的执行权.
- OpenAI Agents SDK更倾向于把Lifecycle Hook用于观察,把阻断,审批和执行塑形交给Guardrail,Approval和Filter.这种边界更清晰,但是如果扩展点过少,开发者又会被迫修改中心循环.
- 所以Praxis统一暴露Phase Point,但是每个Contribution必须明确属于Observer,Filter,Gate或者Port中的一种.
- Observer只能观察,审计,记录指标和发送异步通知;Filter可以校验,标准化,缩减或者有限改写当前候选对象;Gate可以形成allow,deny,ask或者defer;Port则调用真正的领域Owner完成操作.
- 一个普通Observer不能阻断行为,一个异步Handler不能改变已经继续执行的当前操作,一个Filter不能产生外部副作用,一个Gate也不能自己绕过Port去执行动作.

### 完整Phase表

| 阶段 | Phase Point | 类型 | 主要作用 |
|---|---|---|---|
| 装配 | `assembly.graph.compile.before/after` | Filter / Observer | 检查装配输入并记录编译结果 |
| 装配 | `assembly.binding.validate` | Filter | 校验Slot,Port,版本,能力和摘要 |
| 预检 | `assembly.preflight.before/after` | Filter / Observer | 在任何真实调用以前完成Expected/Actual检查 |
| Endpoint | `endpoint.open.before/after` | Gate / Observer | Runtime认证以后打开Harness Endpoint |
| Session | `session.start.before/after` | Filter / Observer | 建立Run内Session和初始状态 |
| Run | `run.start.before/after` | Gate / Observer | 验证启动条件并记录Run启动Claim |
| 输入 | `input.accept.before/after` | Filter / Observer | 校验,标准化并记录人类或系统输入 |
| Context | `context.sources.collect` | Port | 从各领域Owner收集Context Candidate |
| Context | `context.frame.validate` | Filter | 检查权限,版本,预算,顺序和缓存计划 |
| Context | `context.frame.frozen` | Observer | 记录不可变Context Frame和Manifest摘要 |
| Model | `model.request.prepare` | Filter | 做Provider安全转译和请求边界检查 |
| Model | `model.dispatch.before` | Gate | 校验Fence,预算,Route和执行资格 |
| Model | `model.response.observed` | Observer | 记录流式或最终Model Response观察 |
| Model | `model.output.validate` | Filter | 校验结构化输出和协议完整性 |
| Action | `action.candidate.created` | Observer | 记录Model产生的Action Candidate |
| Action | `action.admission` | Filter | 校验Schema,Capability,Scope和Policy |
| Action | `action.review` | Gate | 进入Human,Auto或者Bypass Review |
| Action | `action.dispatch` | Port | 通过受治理的Tool,MCP或Domain Port真实执行 |
| Action | `action.result.normalize` | Filter | 规范结果,脱敏并验证Tool Result |
| Action | `action.result.observed` | Observer | 记录结果,Evidence和执行Receipt |
| Batch | `action.batch.completed` | Filter / Observer | 汇总并发Action,决定是否进入下一轮 |
| Turn | `turn.continuation.evaluate` | Filter | 判断继续,等待输入,等待动作或者结束 |
| Turn | `turn.completed` | Observer | 记录Turn完成和当前状态摘要 |
| 压缩 | `context.compact.before/after` | Gate / Observer | 校验压缩资格并记录新的Context Generation |
| Checkpoint | `checkpoint.create.before/after` | Gate / Observer | 创建一致Checkpoint并记录凭证 |
| 暂停 | `run.pause.before/after` | Gate / Observer | 持久暂停并保留可恢复状态 |
| 恢复 | `run.resume.before/after` | Gate / Observer | 验证旧事实和当前世界以后继续运行 |
| 多Agent | `agent.spawn.before/after` | Gate / Observer | 创建受限子Agent并记录父子关系 |
| 多Agent | `agent.handoff.before/after` | Gate / Observer | 转交任务所有权或者把Agent作为Tool使用 |
| 多Agent | `subagent.completion.validate` | Gate | 验证子Agent结果是否可以被父Agent消费 |
| 取消 | `run.cancel.before/after` | Gate / Observer | 下发取消并进入Effect Reconciliation |
| 完成 | `run.completion.validate` | Gate | 校验核心语义,Review和Grounding是否满足 |
| 终态 | `run.terminal.observed` | Observer | 产生Harness Completion Claim |
| Session | `session.end.before/after` | Filter / Observer | 收口Session资源和状态 |
| Endpoint | `endpoint.close.before/after` | Gate / Observer | 关闭Endpoint并阻止新的控制请求 |
| Cleanup | `cleanup.before/after` | Port / Observer | 调用领域Owner清理并记录Cleanup Observation |
| 残留 | `residual.detected` | Observer | 报告未清理资源和未知Effect |

### Phase的顺序,合并和失败

- Phase的执行顺序必须在Assembly期间编译完成,不能在每一轮通过反射和动态查找重新排序.
- 总体优先级应该是Runtime Invariant高于Organization Managed Policy,高于Resolved Plan要求的Gate,高于Slot Owner Filter,高于Module Filter,高于Extension Filter,最后才是Observer.
- 同一个Gate出现多个决定时,按照deny高于defer,高于ask,高于allow进行合并.任何deny都不能被后面的低权限Handler覆盖.
- Observer可以并行运行;拥有数据改写能力的Filter必须按照依赖拓扑和稳定顺序执行.两个Filter同时改写同一个字段而无法证明兼容时,应该Fail Closed.
- 必需Gate,必需Filter和治理Port超时或者失败时默认失败关闭;非关键Observer失败可以降级,但是必须留下错误Observation和Residual.
- 异步Handler只能用于Observer.它可以发送日志,指标,Webhook或者通知,但是不能在Agent已经继续以后再声称撤销当前行为.
- 每一次Handler执行都应该产生PhaseReceipt,至少记录Phase,Handler,Input Digest,Decision,Output Digest,耗时,超时,错误,执行顺序和证据引用.
- PhaseReceipt和Hook输出先是Candidate或者Observation.它们不能自己分配Runtime Event Sequence,不能形成Execution Outcome,也不能修改Lease,Fence,Identity,Authority,SecretRef或者Capability Grant.
- Stop或者Completion Gate可以要求Agent继续修复问题,但是必须拥有最大轮次,预算,超时和Human Escalation,避免形成永远无法结束的Agent循环.

### 关于Assembly SDK

- Assembly SDK是这一部分正式向开发者开放的工程面.开发者不应该为了增加一个Context Source,Review Profile或者新的Domain Adapter就去修改Harness Kernel.
- SDK首先需要让开发者声明自己的Module Descriptor,Capability,Slot Contribution,Port,Phase和依赖,再通过Compiler得到完整的诊断,预览和不可变装配产物.
- Assembly SDK只负责声明,编译和检查.它不负责运行Agent,不直接调用模型,不执行Tool,不申请Sandbox,也不持有Runtime事实写权限.
- SDK应该同时提供严格类型化对象和可序列化Manifest.类型化对象适合Go,TypeScript和其他语言SDK使用;Manifest适合签名,分发,缓存,检查,跨进程传输和供应链验证.
- Assembly SDK还需要提供Inspect和Explain能力.开发者应该能够看到某个Slot为什么选择当前Owner,某个Contribution为什么被接受或者拒绝,某个Phase的精确顺序是什么,以及哪个摘要漂移导致装配失败.

### Assembly SDK需要保留的对象

| 对象面 | 对象 | 作用 |
|---|---|---|
| 模块声明 | `ModuleDescriptor` | 模块身份,版本,摘要,来源和兼容范围 |
| 模块声明 | `CapabilityDescriptor` | 模块提供和要求的能力 |
| 模块声明 | `SlotSpec` | 声明一个类型化Slot合同 |
| 模块声明 | `SlotContribution` | 声明模块向Slot提供的Owner,Source,Provider,Filter或Observer |
| 模块声明 | `PortSpec` | 声明Versioned Port的Schema和语义 |
| 模块声明 | `PortBinding` | 绑定实际Port实现和验证证据 |
| 模块声明 | `PhaseSpec` | 声明Phase类型,输入输出和失败语义 |
| 模块声明 | `PhaseContribution` | 注册Observer,Filter或者Gate Handler |
| 模块声明 | `DependencySpec` | 声明模块,Slot和Phase依赖关系 |
| 模块声明 | `ModuleFactory` | 在验证完成以后构造受限模块实例 |
| 编译输入 | `AssemblyInput` | 承载Resolved Plan,Bootstrap Plan和实际组件描述 |
| 编译过程 | `AssemblyBuilder` | 收集声明,不执行外部动作 |
| 编译过程 | `AssemblyCompiler` | 完成类型,能力,依赖,顺序和摘要校验 |
| 编译产物 | `AssemblyGeneration` | 标识一次不可变装配世代 |
| 编译产物 | `AssemblyManifest` | 保存全部Slot,Phase,Port和摘要 |
| 编译产物 | `CompiledHarnessGraph` | 供热路径直接使用的最终接线图 |
| 编译产物 | `AssemblyDiagnostic` | 表达错误,冲突,缺失,警告和拒绝原因 |
| 编译产物 | `ConformanceReport` | 比较Expected与Actual以及能力一致性 |
| 编译产物 | `ResidualReport` | 表达允许Residual和不可接受Residual |
| Runtime交接 | `RuntimeProviderBinding` | 交给Runtime的受治理Harness Provider入口 |
| Phase运行 | `PhaseContext` | 携带Scope,Run,Turn,Step,Action和摘要引用 |
| Phase运行 | `PhaseDecision` | 表达allow,deny,ask,defer或有限Filter结果 |
| Phase运行 | `PhaseReceipt` | 留下Handler运行,决定和证据记录 |

### 必须原样保留的上游和现有合同

- Assembly SDK不能重新解释ResolvedAgentPlan,HarnessBootstrapPlan,RouteBinding,ExpectedInjectionManifest,ActualInjectionManifest,CapabilityGrantDigest,RuntimePolicyDigest,ProfileDigest,HarnessStackDigest,ContextPlanDigest,ToolSurfaceDigest,Evidence Expiry和Allowed Residuals.
- 这些对象的来源,版本和摘要必须被原样带入Assembly Manifest和Runtime交接.任何不一致都应该显式失败,不能在SDK内部做宽松合并.
- 现有Harness Manifest,Harness Endpoint,Harness Run Session,Harness Event Candidate,Completion Claim和Cleanup Observation也应该继续作为稳定公共合同,而不是再制造一套名字相近的平行对象.
- Runtime打开Endpoint以后,外部SDK只通过Runtime允许的Start,Inspect,Provide Input,Provide Action Result,Cancel,Pause,Resume和Close等控制面工作,不能获得Harness内部Session指针.

### Assembly SDK不能暴露的内容

- 不能暴露Runtime Fact Store写权限,Event Sequence分配权,Lease或Fence修改权,Secret明文和未经治理的Provider Credential.
- 不能提供绕过Review,Tool Gateway,Sandbox或者Operation Governance的直接Dispatch入口.
- 不能把跨模块原始实现指针当成公共合同,也不能让一个Module直接修改另一个Module的内部状态.
- 不能允许激活后的CompiledHarnessGraph被原地改写.任何结构变化都必须产生新的Assembly Generation和新的摘要.
- 不能提供重新计算Profile,重新选择版本或者偷偷增加权限的入口.这些事情属于上游Assembler和Policy Owner.
- 不能把Provider原生Session ID当成Praxis Run Session权威身份.原生ID只能作为Observation和恢复引用存在.

### 关于Harness Kernel和性能

- Harness Kernel应该保持很小.它负责Run状态机,Turn推进,等待输入,等待Action,取消,终止Claim和调用编译好的Port,而不是直接导入六个组件的内部实现.
- 热路径应该使用已经编译好的数组,拓扑和类型化函数引用,避免每一轮进行Manifest解析,依赖求解,正则匹配和反射式Plugin查找.
- Session State和Session Services需要分开.前者保存当前Run真正需要的可恢复状态,后者保存经过装配的Port,Provider,Handler和外部服务引用.
- Turn和Step也需要分开.Turn描述一次模型交互周期;Step描述Context准备,Model调用,Action候选,Review,执行和结果回灌等更细的过程.
- Model流式输出,并行Tool Call,后台任务和异步Observer都可以并发,但是状态提交必须通过明确的顺序,Revision和CAS收口.
- Harness不能承诺Exactly Once.进入真实Model,Tool,MCP或者外部Effect以后,丢回包可能导致Outcome Unknown;此时只能Inspect和Reconcile,不能因为没收到回复就盲目重复派发.
- 每个Run需要明确的Turn,Event,Token,费用,时间,并发和Review预算.达到边界以后应该进入可解释的等待,失败,升级或者终止状态,不能无限循环.
- Assembly和Preflight可以做较重的检查;真正的Run Loop必须尽可能短,稳定,可预测,并且不让扩展系统拖垮全部Agent.

### 关于Codex,Claude Code和Agents SDK可以学习的内容

- Codex值得学习的是Thread,Submission,Session,Task,Turn,Step和Event的生命周期切分,以及Session State,Session Services,Tool Router,Hook Runtime,Approval和App Server的外部控制面.
- Codex不应该照搬的是逐渐变大的Submission Op分派和Run Turn中心函数.当Context,Plugin,Hook,Compaction,Tool,Provider和协作逻辑全部进入一个函数以后,后续模块会越来越难独立演进.
- Claude Code和Claude Agent SDK值得学习的是Async Message Stream,Session Resume,Permission Callback,PreToolUse/PostToolUse,Stop,Subagent,Compaction和结构化Result.
- Claude的Hook模型也提醒我们:Hook如果同时拥有注入,改写,审批,阻断和副作用能力,就必须在类型和优先级上严格治理,否则一个普通扩展会成为隐藏的执行Owner.
- OpenAI Agents SDK值得学习的是Agent和Runner分离,Agent as Tool与Handoff的所有权区别,RunHooks和AgentHooks,输入输出Tool Guardrail,Approval中断恢复,Session和Tracing.
- OpenAI的边界也值得直接吸收:Lifecycle Hook主要用于观察;真正会阻断或者塑造执行的内容应该进入Filter,Guardrail,Approval或者受治理Port.
- Codex SDK和Claude Agent SDK的Thread,Query,Run,Resume和Message Stream适合作为外层SDK体验,但是Praxis内部仍然要使用自己的Resolved Plan,Assembly Generation,Runtime Fact和Event合同.

### 七组件端到端交互链

- 一次典型Turn从Input开始.Context Engine从Memory,Knowledge,Continuity,Sandbox,Tool和人类输入收集Candidate,形成不可变Context Frame,Harness再通过ModelTurn Port交给已经选定的Model Route.
- Model返回文本,等待输入或者Tool Call.文本可以进入结果候选;Tool Call先成为Action Candidate,经过Tool&MCP Admission,Review Gate,Runtime Operation Governance和Sandbox实际执行点门禁以后才可能Dispatch.
- Tool或者MCP Provider返回Observation和Receipt,领域Owner通过Inspect形成Tool Result和Effect Settlement,Context Engine把安全结果编译进下一轮Frame,Harness继续循环.
- Continuity记录Frame,Turn,Action,Review,Attempt,Result和Claim之间的因果关系;Memory&Knowledge可以从完整经历提出Candidate,但是不会在热路径里无条件写入正式Record.
- 当模型声称完成时,Completion Gate检查核心任务,Result Bundle,Grounding,Pending Action,Unknown Effect和Residual.通过以后Harness产生Completion Claim,Runtime结合各领域事实形成最终Execution Outcome.
- 纯文本或者只读任务可以跳过Action和Sandbox执行,但是仍然需要Context,Model,Event和终态链;流程是按能力裁剪,不是强迫每个Run经过所有组件.

```text
Input
  -> Context Candidate -> Context Frame
  -> Model Turn
  -> Output | Action Candidate | Input Required
  -> Tool&MCP Admission -> Review Gate
  -> Runtime Permit -> Sandbox Enforcement
  -> Provider Observation -> Inspect -> Settlement
  -> Tool Result -> Next Context Frame
  -> Completion Gate -> Harness Claim
  -> Runtime Outcome + Continuity Timeline
  -> Memory/Knowledge Candidate
```

| 对象或动作 | 唯一语义Owner | Harness的作用 |
|---|---|---|
| Context Recipe,Manifest,Frame | Context&Cache | 请求并消费冻结Frame |
| Capability,Tool,MCP Connection,Tool Result | Tool&MCP | 形成Action Candidate并等待结果 |
| Review Case,Verdict,Authorization材料 | Review | 在Gate处暂停和继续 |
| SandboxLease,Workspace Change,Cleanup | Sandbox | 通过受治理Port请求执行 |
| Timeline,Checkpoint,Fork,Restore关系 | Continuity | 提交有序Candidate和恢复请求 |
| Memory Record,Knowledge Snapshot,Retrieval Result | Memory&Knowledge | 查询Candidate,不拥有正式Record |
| Instance,Run,Fence,Permit,Outcome | Runtime | 传播控制与Observation |

### 和六个组件的联动

- 和Context&Cache联动时,Context Engine拥有Context Recipe,Manifest,Frame,Compact和Cache Plan;Harness只在对应Phase调用Context Port并消费冻结Frame,不能在Model调用前继续偷偷改写.
- 和Tool&MCP联动时,Tool&MCP拥有Capability,Schema,Action Gateway,真实Tool Result和MCP生命周期;Harness负责把Model Tool Call转成Action Candidate,等待治理结果,再把结果送回下一轮.
- 和Review联动时,Review拥有Case,Reviewer,Attestation,Verdict和Trace;Harness在Gate处暂停和恢复,但是不能自己把Reviewer意见升级成Permit或者执行结果.
- 和Sandbox联动时,Sandbox拥有Workspace,Lease,文件,进程,网络,执行残留和实际隔离;Harness只通过已绑定的Execution Port工作,不能因为Context里写着禁止访问就认为权限已经执行.
- 和Continuity联动时,Harness提供Run,Turn,Step,Frame,Action和Claim等有序Candidate;Continuity负责Timeline,Timestamp,Checkpoint,Fork,Rewind,持久化和恢复关系.
- 和Memory&Knowledge联动时,Memory和Knowledge负责召回,候选,来源,版本和Commit;Harness不把它们直接塞进模型,而是让Context Engine按照当前Frame需求选择和投影.
- 六个组件的失败都必须保留自己的领域语义.Harness可以等待,降级,终止或者进入Reconciliation,但是不能把一个领域错误翻译成看似成功的通用Event.

### 和Runtime,Application和外部使用面的联动

- Runtime是Harness的宿主和治理权威.它验证Plan,绑定Provider,建立Instance和Run,持有Lease与Fence,接收Event Candidate,形成Outcome并完成Cleanup判断.
- Harness只拥有当前Run的交互循环和Session.它可以报告Completion Claim,但是Runtime必须结合真实Operation,Effect,Review,Sandbox和Cleanup事实形成最终Execution Outcome.
- Application负责把用户,工作流,外部系统和领域命令组织起来,并通过Runtime公共入口发起操作.它不能为了方便直接调用Harness Kernel.
- Assembly SDK服务模块开发者和平台装配者;Runtime SDK服务Agent运行和控制;各领域SDK继续服务Review,Context,Tool,MCP等自己的公共对象.这些SDK可以拥有一致的Envelope和错误模型,但是不能合并成一个万能接口.
- CLI,API,REPL和UI应该建立在这些SDK之上,支持Compile,Inspect,Explain,Diff,Preflight,Run,Watch,Pause,Resume,Cancel和Close,并把真实状态从Runtime和领域Owner读回来.

### 我们最后要落成的

- 一条稳定的Agent Definition,Profile System,Agent Assembler,Harness Assembly Compiler和Runtime Activation三段装配链,让应该使用什么,实际接上什么和此刻能否运行彼此分离.
- 一套共享ContractVersion,ObjectID,Revision,Owner,Scope,Digest,Evidence和幂等/CAS语义的最小Envelope,同时保留每个领域自己的对象和权威.
- 一套类型化Slot系统,覆盖Kernel,Model,Context,Event,Action,Tool,MCP,Review,Sandbox,Continuity,Memory,Knowledge,Asset,治理引用和未来Namespaced Domain组件.
- 一套Owner加Contributor模型,让每个领域拥有唯一语义,又允许其他模块通过Source,Provider,Filter和Observer安全贡献能力.
- 一张完整的Phase表,覆盖Assembly,Preflight,Endpoint,Session,Run,Input,Context,Model,Action,Review,Checkpoint,Pause,Resume,Subagent,Cancel,Completion和Cleanup.
- 一套Observer,Filter,Gate和Port严格分型的扩展模型,既能自由插拔,又不会让普通Hook获得隐藏的权限和执行权.
- 一套确定性的Phase排序,冲突合并,超时,失败关闭,异步观察和Phase Receipt机制,让扩展行为可以检查,回放和审计.
- 一套Assembly SDK,允许开发者声明Module,Capability,Slot,Port,Phase和依赖,并提供Compile,Inspect,Explain,Diff,Conformance和Residual报告.
- 一套不可变CompiledHarnessGraph和Assembly Generation机制,让装配阶段完成复杂校验,让Run热路径保持简单,快速和可预测.
- 一套与现有ResolvedAgentPlan,HarnessBootstrapPlan,RouteBinding,Harness Manifest,Runtime Binding和Event Candidate合同连续的对象体系,不重新制造平行事实.
- 一套面向Codex,Claude Code,OpenAI Agents SDK以及未来其他官方Harness的Adapter边界,学习它们的生命周期和SDK体验,但不把任何一家产品变成Praxis的全局定义.
- 一套和六个头等组件,Runtime,Application,CLI,SDK,API,REPL和UI真正接通的公共Harness基座.
- 一条从Context Frame,Model Turn,Action Candidate,Review,Permit,Sandbox Enforcement,Tool Result,Settlement到Completion Claim和Runtime Outcome的可检查端到端链.
- 最重要的是,我们最后产出的不是一个巨大而不可维护的agent loop,也不是一堆可以任意执行代码的hook.它应该是一套可以持续增加组件,保持高性能,保持权威边界,并且让开发者真正组装出不同Agent的Execution Harness.
