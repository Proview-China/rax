## Review Documentary

### 综述

- Review这一部分本质上不是一个简单的approval按钮,也不是让另外一个LLM随便看一眼结果.
- 它是agent从产生意图,形成动作候选,执行工作,产出Artifact,一直到结果交付之间的判断层.
- 如果一个agent的所有产出都完全不经过Review,它很难成为真正可靠的生产力工具;但是如果所有行为都必须等待人类逐个确认,它同样无法形成规模化生产力.
- 所以我们需要在Human Gate,Auto Review和Bypass之间建立一套稳定的路由,让不同风险,不同业务域,不同执行阶段和不同结果使用不同的审核方式.
- 这里还需要区分过程中的Review和结果的Review.前者关心一个动作是否可以继续,后者关心最后的产物是否真的满足原始意图,是否拥有足够的Grounding,以及人类能否据此作出可靠判断.
- 我们原来的Praxis已经存在bapr,yolo,permissive,standard和restricted五档Profile,也存在human approval和side-agent review之类的策略.这些经验可以继续使用,但是五档Profile不应该变成五套Review Engine.
- Profile应该负责把当前identity,authority,risk,scope,effect和业务策略映射到Human,Auto或者Bypass;Review Engine本身仍然使用统一的Request,Case,Reviewer,Attestation,Verdict和Trace合同.
- 我们还需要重点学习Codex的Review实现.它已经把代码Review,动作前的Auto Review,Inline和Detached Review,独立Reviewer身份,结构化输出和高信号Finding做出了比较清晰的拆分.
- 但是Praxis正在做的是一个框架,所以我们不能只做一个针对git diff的命令.我们需要把Review抽象成可以服务代码,文档,PPT,数据,法律意见,财务动作,外部Effect和任意Artifact的判断基座.

### 关于Review对象和两个阶段

- Review首先需要回答一个问题:我们到底在审核什么.
- 在动作发生以前,真正应该被审核的是Action Candidate,Effect Intent,Tool Call Candidate,Artifact Candidate或者其他尚未成为现实的候选对象.
- 一个已经发生的Event是事实记录,不能因为Review拒绝就假装它没有发生.所以我们平时说的event拦截,更准确地说是Runtime在Event Candidate或者Effect Intent进入真实执行以前触发Review Hook.
- 过程Review主要面对Intent和Action.它判断当前动作是否符合用户授权,业务策略,风险边界,权限范围和当前工作状态,以及是否需要人类介入.
- 结果Review主要面对Artifact和Outcome.它判断代码,文档,PPT,报告,网页,数据结果或者其他产物是否满足原始任务,核心语义和验收标准.
- 过程Review和结果Review可以共享基础设施,但是不应该共用一套含糊的Prompt.动作安全审核需要关注risk和authorization;代码审核需要关注具体缺陷和diff;结果审核则需要关注acceptance criteria,Grounding和coverage.
- 所以Review应该拥有不同的Review Profile或者Rubric,比如intent-safety,code-change,artifact-quality,outcome-acceptance,legal-compliance和finance-control.
- Review Target必须冻结到一个精确版本.它应该绑定CandidateID,Revision,Digest,Scope,Artifact版本,Context Frame和Evidence集合,不能只写成“审核一下当前状态”.
- 如果审核期间Candidate发生改变,旧Review只能保留为历史,不能继续授权新的Revision.

### 关于Review的三个Route和五档Profile

- 我认为Review真正的基础Route只有三个:Human Review,Auto Review和Bypass.
- Human Review代表当前Case必须交给拥有对应Authority的人类判断.它可以是每一关都停下来的Human Gate,也可以只在高风险,高价值,不可逆或者证据不足的时候触发.
- Auto Review代表Runtime创建一个专用Reviewer,让它在受限上下文和只读能力下完成审核,再把结构化结果交回Review Owner形成正式Verdict.
- Bypass代表当前Policy明确声明不需要阻塞式Review,也就是我们说的YOLO或者其他放行场景.
- 但是Bypass不是伪造一个accepted Verdict,更不是绕过Sandbox,Authority,Effect Gateway,审计和Evidence.它只代表当前Review操作被显式标记为not required,并且这个决定本身仍然需要留痕.
- 原来的五档Profile可以继续作为路由输入.不同Profile根据工具风险,数据范围,外部Effect,业务域和人类授权,决定什么时候Human,什么时候Auto,什么时候Bypass.
- 这意味着restricted可能对更多危险动作要求Human或Auto,standard采用风险路由,permissive减少阻塞,yolo显式跳过Review而保留其他治理,bapr则只能在更明确的开发或测试边界下使用.
- 但是准确的五档矩阵需要在后面的Policy设计和验收中冻结,不能只依靠Profile名字推断安全承诺.

### 关于Review Hook和Review Action

- Review Hook是Runtime或者Policy在关键边界自动触发的拦截点;Review Action则是Agent,人类,CLI,SDK或者外部系统主动发起的一次审核请求.
- 两者最后都应该生成相同的ReviewRequest和ReviewCase,这样系统不会因为入口不同而出现两套权威事实.
- 一个同步过程可以表示为:Candidate准备完成,ReviewRequested写入Continuity,Run进入waiting_review,Reviewer产生Attestation,Review Owner形成Verdict,Runtime复读当前事实以后继续,修订或者终止.
- 这里的同步不是让一个系统线程一直阻塞.它代表当前Run的状态机被持久化暂停,即使服务重启,任务也可以继续等待同一个ReviewCase.
- Verdict返回以后,Runtime仍然需要重新检查Candidate Revision,Digest,Identity,Authority,Scope,Policy,TTL和SandboxLease.审核通过不等于现实动作已经发生.
- Bypass也应该走同一个Hook,但是生成明确的operation_not_required或者BypassDecision记录,然后继续执行,不能静默跳过.

### 关于Review Gate和Harness Phase

- 在统一Harness模型中,Review Hook更准确的公共形态是Review Gate.它可以注册在`action.review`,`run.completion.validate`,`subagent.completion.validate`以及其他由Plan明确允许的Phase Point.
- Phase只负责把冻结Target,当前Scope,Policy和Evidence交给Review Port,并让Run进入可恢复等待;Review Engine负责Case,Reviewer,Attestation和Verdict;Runtime负责复读当前事实并决定是否签发后续Operation Permit.
- `PhaseDecision`是Harness在当前Phase看到的控制结果,`ReviewVerdict`是Review领域当前结论,`OperationReviewAuthorization`是Runtime对Case,Verdict,Reviewer Authority,Target和Policy当前性的受控投影.三者不能合并成一个布尔值.
- Review通过以后也不直接执行Tool,提交Artifact或者把Run标记为成功.它只满足后续治理条件之一,真实动作仍然通过Tool&MCP,Sandbox和Runtime Operation Governance.
- Review Gate必须支持allow,deny,ask和defer,但是这些词只描述当前流程控制.Accept,Request Changes,Escalate Human,Reject和Insufficient Evidence等领域Verdict仍然保留更完整语义.
- 多个Review要求同时命中时不能简单选择最宽松结果.硬性deny优先,需要Human的Authority不能被低权限Auto Reviewer覆盖,条件通过必须等所有Condition Satisfaction事实成立以后才能继续.
- Gate超时,Reviewer不可用,Case状态未知或者Continuity无法可靠保存等待状态时,有副作用的动作默认失败关闭;纯结果观察是否允许降级由明确Policy决定.

### 关于Verdict当前性和Authorization投影

- 正式Verdict至少绑定CaseID,Case Revision,TargetID,Target Revision,Target Digest,Policy Revision,Reviewer Identity,Reviewer Authority,Scope,Decision,Conditions,CreatedAt和ExpiresAt.
- Runtime使用Verdict以前需要重新读取当前Target,Authority,Policy,Capability Grant,Review Case和TTL.任一Revision,Digest,Epoch或者Scope发生漂移,旧Authorization立即失效.
- Reviewer原始输出先成为Attestation.无论来自人类,Auto Reviewer,CLI,Webhook还是外部Issue评论,都必须经过身份,Authority,Schema,Case和幂等校验以后才能形成唯一当前Verdict.
- 同一个Case只能通过CAS形成一个当前Revision.重复Webhook,两个Reviewer同时提交,服务重试和迟到响应不能产生两个互相矛盾的当前结论.
- Verdict撤销,过期,被新Revision替代或者Condition失效以后,旧Operation Permit不能重新复活.新的动作必须创建新的Candidate或者重新进入Review.
- Review历史可以长期保留,但是只有当前有效投影可以参与执行治理.人类曾经批准过某个相似动作,不等于未来所有相似动作都获得授权.

### 关于同步和异步Review

- Review的同步和异步应该被定义为Delivery方式,而不是新的Reviewer种类.
- Inline Review发生在当前Run里面.主Run暂停并等待当前Case得到解决,适合动作前门禁,高风险Tool Call,不可逆Effect和需要立即判断的结果.
- Detached Review会创建独立的ReviewRun或者ReviewThread.原工作状态被精确冻结并提供给Reviewer,但是审核可以在另外的时间,进程,节点或者外部系统中完成.
- Detached并不代表审核对象可以继续变化.它仍然绑定原来的Candidate Revision;新Revision必须创建新的Case或者显式替代旧Case.
- Human和Auto都可以同步或者异步.比如同步Auto Review可以判断一次shell动作,异步Auto Review可以审查一个大型PR;同步Human Gate可以拦截外发邮件,异步Human Review则可以交给法务或者财务在Linear,Jira和Slack中处理.
- 所以Review的表达至少需要四个正交维度:Review Target,Review Delivery,Reviewer Route和Review Profile.这比把所有组合硬编码成若干模式更加清晰.

### 关于Human Review

- Human Review不是简单地弹出一个approve和deny按钮.人类需要看到当前审核对象,原始意图,影响范围,关键Diff,风险,证据,限制和当前需要作出的精确决定.
- 人类可以Accept,Reject,Request Changes,给出Conditional Acceptance,或者声明当前证据不足.
- 人类的Review Authority也必须被验证.能够审核代码不等于能够批准生产发布;能够审核财务报告不等于能够批准真实交易.
- 每一个Human Verdict都应该绑定reviewer identity,authority,scope,revision,digest,timestamp和有效期,并留下判断说明.
- 对重要产物来说,Auto Reviewer应该能够主动Escalate Human.这不是失败,而是承认当前风险,主观性或者责任边界需要人类承担.
- Human Review本身也不能证明产物一定正确.我们要做的是把结果Grounding清楚,降低人类判断成本,并明确告诉人类哪些部分已经验证,哪些部分没有覆盖.

### 关于Auto Reviewer

- Auto Reviewer不是把工作Agent原样复制一份,然后让两个同样的Agent互相争论.
- 更准确的定义是:Auto Reviewer是从当前Work State派生出的,去工作身份化,只读,目标受限的一次性审核Agent.
- 我们要保留当前工作状态,但是删掉工作主体性.也就是说,Reviewer知道任务为什么开始,发生了什么,当前产出了什么,但是它不再拥有“继续完成任务”的工作目标.
- Reviewer应该获得原始人类意图,明确要求,核心验收标准,稳定项目规则,已经确认的关键决策,当前Candidate,Artifact或Diff,运行和测试证据,尚未解决的风险以及必要的Context引用.
- Reviewer不应该继承工作Agent的persona,自我叙述,执行计划,写入权限,Dispatch权限,提交权限,Spawn能力和其他与审核无关的工具.
- 它也不应该依赖隐藏Chain-of-Thought.可审核的依据应该来自人类消息,可见的Agent输出,Tool Call和Tool Result,Artifact,Diff,Event,Receipt和其他明确Evidence.
- 但是去掉工作身份不等于去掉项目治理.稳定的项目规则,用户要求,Review Rubric和Authority Policy仍然必须进入Reviewer Context.
- Reviewer可以拥有少量只读探索能力,用于定向读取代码,检查引用,运行安全的只读验证或者确认某个Finding.这些探索仍然需要受到Sandbox和Policy约束.
- Reviewer的工作是判断,不是接手任务.它不能一边审核,一边偷偷修改工作区来让自己的审核通过.

### 关于不同类型的Auto Reviewer

- Codex源码给出的一个重要经验是,代码Review和动作审批Review其实是两种不同的Reviewer.
- Artifact Reviewer更像Codex的`/review`:它冻结base branch,uncommitted changes,commit或者custom target,读取精确Diff,按照严格Rubric输出高信号Finding,然后结束.
- Action Reviewer更像Codex的auto-review guardian:它面对的是一个即将执行的精确动作,根据当前transcript,tool arguments,risk,authorization和policy作出allow或者deny判断.
- Praxis还需要Work-State Reviewer和Outcome Reviewer.前者检查当前工作链条是否仍然忠于原始意图;后者检查最终产物和Grounding是否足以交付.
- 这些Reviewer可以共享Reviewer Runtime,Context构建,模型调用,结构化输出,审计和生命周期,但是必须使用不同的Rubric,输出Schema,工具范围和终止条件.
- 我们不应该制造一个万能Review Prompt,因为一个同时审核安全,代码正确性,法律合规和最终用户体验的Reviewer,很容易在所有方面都只得到模糊判断.

### 关于Reviewer和工作Agent之间的交互

- 默认的Review应该是一次性过程:Reviewer检查冻结对象,输出结构化结论,然后退出.
- 如果Reviewer发现问题,它应该返回离散,可操作,有证据和精确位置的Finding,而不是和工作Agent开启无止境的自由对话.
- 工作Agent根据Finding产生新的Candidate Revision;如果Policy要求,新的Revision再进入下一轮Review.
- 因此,所谓Agent和Reviewer之间的交互,更适合被表达成Revision Round,而不是一条无限延长的聊天支线.
- 一个典型过程是Candidate R1经过Review后返回Request Changes,工作Agent形成Candidate R2,Reviewer再次审核R2,最后Accept或者Escalate Human.
- 每一轮都要完整留痕,但是旧Reviewer实例没有必要永久存活.它可以在输出完成以后被销毁,Review Trace和Finding则进入Continuity.
- 对大型异步Review,我们可以保留一个受控的Reviewer Trunk和Context Delta,减少重复注入完整历史;但是每次实际判断仍然需要绑定独立Case和精确Revision.

### 关于Auto Review的终止

- Reviewer什么时候结束,不能只靠模型自己觉得“差不多了”.Prompt中需要明确终止标准,Runtime也需要提供硬性边界.
- Reviewer可以输出Accept,Request Changes,Escalate Human,Reject或者Insufficient Evidence等审核结果.
- Accept代表核心验收标准已经满足,并且没有阻断交付或者执行的Finding;它不代表产物在所有方面都已经完美.
- Request Changes代表存在离散,可修复并且会影响正确性,安全性或者核心语义的问题.
- Escalate Human代表产物重要性,不可逆风险,主观判断,责任归属或者当前不确定性超过Auto Reviewer的授权.
- Reject代表当前候选违反硬性规则,核心语义无法成立,或者继续执行会产生不可接受的风险.
- Insufficient Evidence代表Reviewer无法根据现有Grounding可靠判断,需要补测试,补截图,补录屏,补日志,补数据或者补其他证明.
- Runtime还要限制最大Review Round,最大token和时间预算,重复Finding,重复拒绝,循环争议和超时.达到边界以后应该停止并升级给人类,不能让两个Agent永远困在Review循环中.
- 对动作前的Auto Review,模型超时,解析失败或者Reviewer不可用时应该默认失败关闭,并且把timeout和明确deny区分开来.
- Reviewer拒绝以后,工作Agent不能通过改写同一动作的表面形式绕过结论.它只能采用真正更安全的替代方案,修订Candidate,或者请求人类授权.

### 关于Codex Review可以学习的内容

- Codex代码Review会先冻结精确Target,可以选择base branch,未提交改动,具体commit或者自定义范围.这避免了“审核当前状态”在并发修改下产生漂移.
- Codex支持Inline和Detached两种Delivery.Detached Review从父任务历史派生独立Review Thread,不会要求主工作Agent永久承担Reviewer身份.
- Codex会给Reviewer使用独立Prompt和Rubric,可以配置不同的Review Model,并且收紧工具,协作和写入能力.
- 它强调Finding必须具体,可操作,由当前改动引入,能够证明受影响代码,并且值得作者真正修复.如果没有高信号Finding,宁可不输出噪声.
- 它还要求结构化输出overall correctness,confidence,priority,code location和Finding,让结果可以被UI,CLI和后续自动化稳定消费.
- Codex的动作Auto Review则使用紧凑的可见transcript和精确approval request,重点判断risk和user authorization,并把Tool参数和返回内容当作不可信Evidence而不是新的指令.
- 它允许少量只读检查,对超时和失败采用保守策略,对连续拒绝设置熔断,并明确告诉主Agent不能规避审核结论.
- 对连续审核,Codex还使用首次完整状态加后续Delta的方式复用Reviewer Context,并通过稳定的Reviewer会话改善Prompt Cache和成本.
- 这些做法很值得直接吸收:Target冻结,Reviewer去工作身份化,只读探索,专用Rubric,结构化输出,高信号Finding,失败关闭,反绕过,熔断和Context Delta.
- 但是Codex主要面向coding workflow.Praxis需要把这些能力扩展成Provider无关,Artifact无关,业务域可配置的Review Framework.

### 关于结果Review和Grounding

- 结果Review最重要的事情不是再让一个模型说“我觉得做得不错”,而是把最终结论和真实Evidence绑定起来.
- 一个Result Bundle至少需要包含原始任务,验收标准,最终Artifact及Revision,主要Claim,每个Claim对应的Evidence,运行环境,验证范围,限制和已知未覆盖部分.
- 对代码任务,Grounding可以是diff,测试结果,静态检查,构建日志,运行截图和行为录屏.
- 对网页和桌面任务,可以提供最终页面,关键交互截图,录屏,console和network结果,以及用户要求路径的实际操作证明.
- 对PPT和文档,可以提供最终文件,渲染后的页面,版式检查,内容覆盖和引用来源.
- 对财务,法务和数据任务,还需要数据版本,查询范围,计算方法,来源,签名,规则版本和不能由模型自行断言的责任边界.
- 截图和录屏很好,但是它们不是天然真相.所有Grounding都需要绑定Artifact Revision,环境,时间,操作者和覆盖范围,否则画面可能证明的是旧版本或者错误环境.
- 最终Review UI应该让人类从Claim直接定位到Evidence和Artifact位置,而不是要求人类重新翻阅整个Agent Timeline.
- Review通过只表示产物满足当前验收和权威边界,不表示外部Effect已经提交,也不表示未来状态不会发生变化.

### 关于异步Review的SDK,CLI和API

- Review必须向外部开放一个精确窗口.如果异步Review只能在Praxis内部UI中完成,它就无法进入企业真实工作流.
- 我们至少需要提供Submit Review,Get Review,List Pending Reviews,Watch Review,Resolve Review,Request Changes,Attach Evidence和Cancel Review等稳定能力.
- CLI可以围绕这些能力提供`praxis review list`,`praxis review show`,`praxis review approve`,`praxis review deny`,`praxis review request-changes`和`praxis review watch`等入口.
- SDK需要提供类型化的ReviewRequest,ReviewCase,ReviewEnvelope,ReviewAttestation,ReviewVerdict,ReviewFinding和ReviewEvent,让开发者可以创建自己的Reviewer和Review UI.
- API则需要支持同步请求,Detached任务,事件流,Webhook,幂等键,Reviewer领取和Lease,权限验证,超时,取消和结果Inspect.
- Linear,Jira,Slack以及后续其他平台都应该作为Review Adapter接入这套统一API,而不是分别成为新的Review事实权威.
- 外部Issue,Ticket或者Message只应该承载经过脱敏的Review Envelope,通知和Deep Link.一个评论写着“通过”并不自动成为正式Verdict.
- 外部响应先成为ReviewAttestation,经过身份,Authority,Case,Revision,Digest,TTL和幂等校验以后,再由Review Owner形成唯一Verdict.
- 异步通知应该按照at-least-once处理,Resolve则必须幂等并使用CAS.这样即使Webhook重复,网络断开或者两个Reviewer同时回答,也不会产生两个相反的当前结论.

### 关于Review Case,Verdict和留痕

- ReviewRequest只是一次审核请求;ReviewCase是被持久化,可分配,可等待,可过期和可恢复的审核实例.
- Human,Auto和Remote Reviewer的原始响应应该先成为Attestation或者Observation,而不是直接信任为系统Verdict.
- Review Owner负责验证Reviewer身份,Authority,Policy,Target和Evidence,再以CAS形成当前Case唯一的Verdict.
- 正式Verdict至少要绑定Case,Target Revision,Digest,Policy Revision,Reviewer Authority,Scope,Decision,Conditions,Reason,CreatedAt和ExpiresAt.
- Conditional不能只写一段自由文本然后授权执行.每一个Condition都需要能够被明确证明,并形成独立的Condition Satisfaction事实.
- Verdict一旦过期,撤销或者Target漂移,旧Permit不能重新复活.新的动作需要新的Candidate,Case,Verdict和Permit.
- Review只负责判断,不负责Dispatch,不负责Commit,也不负责把Run标记为成功.修复问题,执行动作和提交产物都要重新进入对应组件的正常治理路径.
- Continuity应该记录ReviewRequested,ReviewerAssigned,ReviewStarted,ReviewFindingObserved,ReviewAttestationRecorded,ReviewVerdictRecorded,ReviewExpired,ReviewEscalated,ReviewSuperseded和ReviewResolved等Event.
- 这些Event和Review Trace需要让我们以后能够回答:谁审核了什么版本,依据什么证据,使用什么Policy,为什么通过或者拒绝,后来是否过期,又是否真正执行和结算.

### 和Context&Cache的联动

- Review需要自己的Reviewer Context Frame,而不是把工作Agent的全部上下文原样复制进去.
- Context Engine负责从原始Intent,项目规则,Work State,Candidate,Artifact Anchor,Diff,Evidence和Review Profile中组装一个冻结的Reviewer Context Frame.
- Reviewer稳定的Identity,Protocol,Rubric和输出Schema可以成为可缓存Prefix;当前Candidate,Diff,实时Evidence和本轮问题则进入Dynamic Tail.
- 对连续Review,第一次可以提供完整工作状态,后续只提供从上一个已知Revision到当前Revision的Context Delta和必要的新证据.
- 但是Context优化不能牺牲Review正确性.如果Target,Policy,Authority或者Evidence已经变化,就必须形成新的Frame和新的缓存版本.
- Review结果可以作为安全的Verdict摘要进入工作Agent下一轮Context,但是正式权威仍然属于Review组件,不能因为模型在Context里看见“已通过”就自行获得执行权.

### 和其他组件的联动

- 和Runtime联动时,Runtime负责在Hook边界持久化暂停和恢复Run,复读当前Candidate,Identity,Authority,Scope,Policy,Fence和Verdict;Review负责唯一判断,不能替Runtime执行控制动作.
- 和Continuity联动时,Review的Case,Round,Attestation,Verdict,Finding,Escalation,过期和终止都进入Timeline,并且支持Session恢复,Fork,Inspect和审计.
- 和Tool&MCP联动时,每个工具和MCP能力需要声明风险,Effect类型,Review Profile和候选Scope,由Policy Router决定Human,Auto或者Bypass.
- 和Sandbox联动时,Reviewer默认使用只读Snapshot或者受限Workspace View.审核通过以后,真实执行点仍然需要Sandbox和Effect Gateway进行第二次门禁.
- 和Memory&Knowledge联动时,Review可以判断一个Memory或Knowledge Candidate是否值得正式Commit,但是它不能直接把候选写成权威知识.
- 和Artifact系统联动时,Review必须绑定Artifact Revision,Digest和Evidence;Artifact变更以后旧Verdict自动失效或者只能作为历史参考.
- 和Organization联动时,Reviewer Identity,Authority,Accountability和业务域Policy决定谁可以审核什么,谁需要为最终判断负责.
- 和Management联动时,Management可以根据Verdict提出暂停,修订,终止或者继续的Control Intent,但是不能把Review意见伪装成已经执行的Runtime事实.

### 我们最后要落成的

- 一套统一的Review Request,Case,Target,Profile,Attestation,Verdict,Finding,Condition和Trace合同,可以审核Intent,Action,Effect,Artifact和Outcome.
- 一套Review Hook和Review Action机制,支持Runtime自动拦截,Agent主动发起,人类发起和外部系统发起,并统一进入同一个Review事实模型.
- 一套与Harness Phase对齐的Review Gate,以及PhaseDecision,ReviewVerdict和OperationReviewAuthorization彼此分离的控制关系.
- 一套Verdict当前性,CAS,撤销,过期,Target漂移和Reviewer Authority校验,让旧批准不能重新复活或者授权新Revision.
- 一套Human,Auto和Bypass三Route的Policy Router,能够结合五档Profile,风险,Authority,业务域,Effect和证据状态完成路由.
- 一套Inline和Detached Delivery,让同步门禁可以持久等待,异步审核可以创建独立ReviewRun并在以后安全恢复.
- 一套去工作身份化的Auto Reviewer Runtime,保留Work State,移除工作主体性,使用专用Rubric,只读工具,结构化输出和明确终止协议.
- 一组不同领域的Reviewer Profile,至少覆盖action safety,code change,work state,artifact quality,outcome acceptance以及后续的法务和财务审核.
- 一套有轮次,有Revision,有预算,有熔断和有Human Escalation的Review Loop,避免Agent与Reviewer无限对话.
- 一套Result Bundle和Grounding协议,把最终Claim,Artifact Revision,截图,录屏,测试,日志,数据来源,覆盖范围和限制组织成人类可以快速判断的结果.
- 一套面向开发者和企业工作流的Review SDK,CLI和API,并允许Linear,Jira,Slack和其他系统作为Adapter安全接入.
- 一套与Continuity,Context,Runtime,Sandbox,Tool&MCP,Memory&Knowledge,Artifact和Organization打通的留痕和权威关系,确保Review不越权,不丢失,不漂移,可以恢复也可以审计.
- 最重要的是,我们最后产出的不是一个approve按钮,也不是一个永远和工作Agent争论的第二Agent,而是一套能够把人类责任,自动判断,现实执行和最终结果可靠连接起来的Review基座.
