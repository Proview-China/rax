## Memory&Knowledge Documentary

### 综述

- 这一部分负责两个彼此相关但不能混为一谈的能力:长期Memory和前置Knowledge.
- Memory来自Agent,用户或者组织在工作中的经历,偏好,判断,纠正,结果和经验;Knowledge来自有来源,有版本,可验证,可撤回的外部或内部知识材料.
- 一次Run的Transcript不是Memory,模型生成的一段总结也不是Knowledge.它们都只能先成为Candidate,经过来源,权限,质量,冲突和写入治理以后,才可能进入正式存储.
- Memory绑定Identity,Lineage,用户,团队,业务域或者组织Scope,不会因为某个AgentInstance被销毁而丢失;Knowledge绑定Source,Package,Snapshot和Authority,不会因为某个Sandbox被释放而改变.
- Context Engine负责决定当前模型应该看见哪些Memory和Knowledge;Memory&Knowledge Engine负责检索,版本,来源,写入,更正,遗忘和权限,不能为了召回命中就把全部内容直接注入模型.
- Continuity保存经历和因果历史,为Memory Consolidation提供材料;但是Timeline Event不会自动晋升为Memory,Memory也不能替代完整历史.
- 我仍然认为Skill Index,Vector和Graph三种方式都应该保留.它们不是三种互相排斥的Memory产品,而是三类索引和访问视图:Skill Index擅长流程和精确经验,Vector擅长语义候选,Graph擅长关系和多跳约束.
- Naive RAG只做一次向量Top-K通常不够可靠.更好的方式是先理解查询意图,组合Lexical,Index,Vector和Graph召回,再使用权限过滤,来源验证,重排和按需展开获得可引用结果.
- Praxis需要提供的是一个可替换Backend,可组合策略,可治理Scope和可评测质量的框架,而不是强迫每个开发者使用某一种向量库或者图库.

### 关于Memory和Knowledge的边界

- Memory回答的是:这个Identity或者Scope过去经历过什么,形成了哪些偏好,纠正,工作方式和可复用经验.
- Knowledge回答的是:在某个版本和来源范围内,有哪些可以被查询,引用,验证和撤回的前置认知材料.
- Memory可以记录“这个用户偏好中文短回答”“上次这种部署失败是因为权限链”“这个Agent已经学会某个工作流程”;Knowledge可以记录产品文档,法律条文,代码结构,组织制度,数据字典和经过来源治理的事实.
- Memory可以带有置信度和主观性,但是必须说明是谁的Memory;Knowledge可以存在冲突,但是必须保留来源,版本,有效期和Authority,不能把冲突来源强行合成一个无来源真相.
- Asset Manager拥有原始文件,文档,音视频和其他Artifact字节;Knowledge保存Source Ref,解析结果,索引和Snapshot,不应该复制并取代正式资产所有权.
- Skill可以是一种可复用工作资产,也可以成为Skill Index中的入口;但是Skill文件不天然等于Memory Record,更不等于经过验证的Knowledge.
- 语义层可以把多个Knowledge Snapshot,Memory View和组织Projection组合成业务域认知面;Memory&Knowledge模块提供受治理材料,不接管整个前置语义层的组织和投影职责.

### 关于Scope,继承和视图

- Memory至少需要区分Run Working Memory,Identity Private Memory,Agent Lineage Memory,User Memory,Team Memory,Domain Memory和Organization Shared Memory.
- Working Memory通常属于当前Context Generation和Continuity,可以随着Run结束压缩或者清理;长期Memory需要经过独立Commit并位于可销毁Sandbox之外.
- Knowledge至少需要区分Personal Source,Project Source,Team Source,Domain Source,Organization Source和External Source,每一种都拥有独立Authority和可见范围.
- 继承不是把上层全部内容复制给下层.更合理的是形成Memory View和Knowledge View,根据Identity,Authority,Scope,Policy和任务目的解析出当前可查询范围.
- 子Agent和Auto Reviewer不能自动继承父Agent全部Memory.它们只能获得当前Profile和任务显式允许的View,并且需要防止私人Memory通过共享Context泄漏.
- Domain之间默认隔离.法务,财务,开发和销售可以共享经过批准的公共知识,但是不能因为处于同一个Agent Cluster就互相读取全部Memory和敏感Knowledge.
- 每一次Query和Commit都要绑定Tenant,Identity,Authority,Scope,Policy Revision和Purpose.权限不仅决定能否读取,也决定是否可以得到全文,摘要,引用或者仅存在性信息.

### 关于核心对象

- `MemoryCandidate`是从Event,用户纠正,Agent总结,Review或者外部输入中提出的待写入内容,带有来源,Scope,敏感级别,置信度和候选用途.
- `MemoryRecord`是经过治理后提交的正式Memory,至少包含MemoryID,Revision,Kind,Scope,Subject,ContentRef,SourceRef,Confidence,Policy,CreatedAt,ExpiresAt和Digest.
- `MemoryRef`是供Context,Continuity和其他组件使用的稳定引用;它不能携带超出当前权限的原始内容.
- `KnowledgeSource`描述原始来源,Owner,Authority,License,Version,AcquiredAt,Validity和Asset Ref.
- `KnowledgePackage`描述一组可独立版本,验证,索引和撤回的知识材料.
- `KnowledgeSnapshot`冻结某个Source集合和索引版本,让Agent查询具有可复现的知识范围.
- `KnowledgeRecord`或者`KnowledgeClaim`保留内容,来源,支持Evidence,冲突,可信状态,有效期和撤回关系,不能只保存一个无来源Embedding.
- `RetrievalCandidate`是检索阶段的中间结果;`RetrievalResult`是在权限,来源,重排和预算处理以后可以交给Context Engine的候选集合.它们都不自动进入Context Frame.
- `MemoryView`和`KnowledgeView`描述当前Agent可以查询的Scope,Snapshot,过滤条件,脱敏规则和预算.

### 关于Skill Index Based Memory

- Skill Index适合保存流程,习惯,判断,勘误,工具使用方法,失败经验,风格和其他可以被精确说明的内容.
- 它更像一份分层目录和说明书:索引项保持短小,稳定并容易搜索;详细内容按需展开到独立Memory Record,Skill或者Artifact.
- Index至少需要Title,Description,Keywords,Scope,UseWhen,DoNotUseWhen,SourceRef,Revision,Digest和DetailRef,不能只靠文件名和自由文本猜测用途.
- Agent可以在工作结束或者Review以后提出Index Update Candidate,但是不应该每个Turn无条件改写Skill Base.高频自动写入容易产生重复,污染和错误固化.
- Consolidator可以比较当前Index与新经验的Delta,生成新增,更新,合并或者删除建议,再根据Policy进入Auto Commit,Review或者Human Gate.
- Skill Index对精准流程和已知关键词很高效,但是不擅长发现未知同义表达和复杂关系,所以它需要与Vector和Graph互补.

### 关于Vector Based Memory和Knowledge

- Vector Index适合在开发者不知道精确关键词时寻找语义相近的候选范围,也适合处理大量扁平文本,段落,事件摘要和知识片段.
- Vector召回结果只是Candidate,不能因为相似度高就进入Context或者成为事实.相似不等于相关,相关也不等于正确,更不等于当前Identity有权访问.
- 每个Embedding必须绑定原始Record ID,Revision,Chunk范围,Embedding Model,Dimension,Index Version和Content Digest.原文更新以后旧Embedding必须失效或者进入历史.
- Chunk不能破坏来源和语义边界.代码,表格,法律条文,会话经验和普通文档需要不同的切分策略,不能使用同一个固定长度规则.
- 查询需要结合Lexical Match,Metadata Filter,Scope Filter,Recency,Source Authority,Diversity和Reranker,不能只依赖Top-K Cosine Similarity.
- Vector Index可以替换,重建和迁移.正式Memory和Knowledge记录不能以某一家向量库作为唯一权威副本.

### 关于Graph Based Memory和Knowledge

- Graph适合保存Entity,Concept,Artifact,Identity,Event,Claim,Source以及它们之间的关系,尤其适合多跳查询,依赖,冲突,时间和来源追踪.
- Graph Node和Edge也必须有Owner,Scope,Revision,Source,Confidence,Valid Time和Transaction Time,不能把模型推断的关系无痕写成永久事实.
- 小型个人Agent不一定需要完整图库.它可以只使用Skill Index和轻量Vector;大型Agent Cluster,业务知识域和组织语义层更适合配置共享Graph Snapshot.
- Graph更新先形成Node/Edge Candidate,经过去重,实体对齐,冲突检查和写入策略以后再Commit.删除来源时,由该来源支持的关系需要重新计算可信状态.
- Graph不是为了替代文档全文.它负责关系和导航,详细Evidence仍然通过Knowledge Source,Asset或者Record Ref按需读取.
- Memory Graph和Knowledge Graph可以共享查询协议,但是必须保留不同Owner和写入规则.个人经验关系不能静默升级为组织知识关系.

### 关于Hybrid Retrieval

- 一个可靠Query应该先解析Intent,Scope,时间,实体,需要的Evidence类型和Context预算,再决定使用Index,Lexical,Vector,Graph或者它们的组合.
- 已知流程优先查Skill Index和精确关键词;未知同义表达使用Vector寻找候选;关系和多跳约束使用Graph;需要原文证明时回到Source和Artifact.
- 多路召回以后先做权限和Scope过滤,再做去重,来源聚合,冲突展开,重排和预算截断.没有权限的候选不应该先取回全文以后再删除.
- Retrieval Result需要说明为什么命中,使用了哪些Index,对应哪个Record Revision,来源是什么,可信状态如何,是否存在冲突以及建议全文还是引用.
- Context Engine根据当前Context Recipe,Frame预算和模型任务决定最终注入.Recall成功只表示找到了候选,不表示模型已经看到.
- 查询结果和模型实际使用效果需要反馈到Evaluation中,但是不能直接通过点击率或者模型引用次数修改正式事实权重.

### 关于Candidate,Commit和生命周期

- Agent,人类,Continuity Consolidator,Tool和外部同步都只能先产生Memory Candidate或者Knowledge Candidate.
- Candidate Admission检查Schema,Source,Scope,Sensitivity,Duplicate,Poisoning Risk,Policy和Target Revision,然后决定Reject,Merge,Review或者进入受治理Commit.
- 正式Commit需要唯一Owner,Revision,CAS和Receipt.模型说“请记住”可以发起Candidate,但不能跳过组织和隐私策略直接写入所有Scope.
- 更新不应该覆盖历史.更正使用Supersede或者Correction Link;冲突保留多个版本和来源;过期改变Current View;删除使用Tombstone和后续Physical Purge.
- Memory需要支持Decay,Expiry,Pin,Archive,Merge和Forget;Knowledge需要支持Source Refresh,Snapshot,Conflict,Withdraw,Reindex和Deprecate.
- 正式Record变化以后,所有受影响的Index,View和Context Cache都需要基于Revision和Digest失效,不能继续返回陈旧内容.
- 写入失败或者回包未知时只能Inspect原Commit Attempt,不能重复创建语义相同但身份不同的Record.

### 关于从Continuity形成经验

- Continuity提供完整经历,Tool Result,Review Finding,用户纠正,Outcome和Artifact关系,Memory Consolidator负责从中提出值得保存的Candidate.
- 不是所有成功都值得记忆,也不是所有失败都应该固化.候选需要说明未来在什么条件下有用,是否可以验证,是否已经存在以及是否会泄漏任务敏感信息.
- Result Bundle,高信号Review Finding,重复出现的失败模式,用户明确偏好和经过验证的工作流程更适合作为长期Memory材料.
- 未结算Effect,Outcome Unknown,临时猜测,原始Chain of Thought和未经授权的私人数据不应该自动进入长期Memory.
- Consolidation应该是可回放的独立任务,记录输入Timeline范围,使用的Policy,模型或规则版本,候选列表,最终Commit和被拒绝原因.

### 关于Knowledge同步和前置语义层

- Knowledge Source可以来自文件库,数据库,代码仓库,API,规则系统,人工维护资料和外部公开信息.每种来源都需要独立Connector和更新语义.
- 同步过程包括Acquire,Parse,Normalize,Validate,Index,Snapshot和Publish,不能在抓取完成以后直接替换当前Knowledge View.
- 新Snapshot发布以前需要比较Source增删,Schema变化,冲突,权限和索引完整性.旧Snapshot继续服务已经绑定它的Run,新Run按Plan选择新版本.
- 前置语义层可以选择多个Knowledge Snapshot,Graph View和Memory View形成Domain Projection.投影结果需要版本和Digest,并进入ResolvedAgentPlan.
- 正式Knowledge撤回以后,历史Run仍然可以通过Evidence看到当时使用的版本,但是新的Context不能继续把已撤回内容当作当前事实.

### 关于安全,隐私和Poisoning

- Memory和Knowledge都可能成为长期Prompt Injection载体.所有外部文本,Tool Result和模型总结按不可信内容处理,不能包含新的系统权限或者执行命令.
- 写入需要做Sensitivity,PII,Secret,License,Source Trust和Retention检查.秘密明文不应该进入Embedding,Index,日志或者共享Memory.
- Query需要Purpose和最小权限.同一个Record可以根据调用方返回全文,摘要,脱敏片段,引用或者只返回不可访问状态.
- 用户需要拥有Inspect,Correct,Forget,Export和Scope管理能力;组织需要拥有Retention,Legal Hold,Audit和Policy能力.
- Poisoning检测可以发现异常来源,重复注入,冲突激增和低可信批量写入,但是最终不能依赖单个LLM判断所有安全问题.
- Reviewer和管理Agent默认使用受限View,不能因为它们负责审核或者管理就读取所有私人Memory.

### 关于存储和性能

- 公共合同围绕Record,Snapshot,View,Ref和Index定义,不绑定具体数据库.默认实现可以组合关系库,KV,对象存储,全文索引,Vector Index和Graph Store.
- Record和Source是权威内容,Index是可重建Projection.任何Index损坏或者模型升级都应该能够从Record重新构建.
- 热门Index和小型Record可以缓存;大型Source和Artifact按需读取;查询结果使用Snapshot,Revision,Scope和Policy作为Cache Key组成部分.
- Embedding和Graph构建可以异步,但是尚未完成的Index必须显式报告Partial Coverage,不能假装查询已经覆盖全部Knowledge.
- 容量,延迟,召回质量,无结果率,冲突率,陈旧率,权限拒绝率,Context采用率和最终任务效果都需要进入可观测指标.

### 关于Memory&Knowledge SDK,CLI和API

- SDK需要提供Submit Candidate,Review Candidate,Commit,Correct,Forget,Register Source,Build Snapshot,Publish View,Query,Inspect Record,Inspect Source,Watch Changes和Reindex等能力.
- 核心类型至少包括MemoryCandidate,MemoryRecord,MemoryRef,MemoryView,KnowledgeSource,KnowledgePackage,KnowledgeSnapshot,KnowledgeRecord,KnowledgeView,RetrievalQuery,RetrievalCandidate,RetrievalResult和CommitReceipt.
- CLI可以提供`praxis memory search`,`praxis memory inspect`,`praxis memory forget`,`praxis knowledge source list`,`praxis knowledge snapshot build`,`praxis knowledge query`和`praxis index status`等入口.
- API需要支持混合检索,分页,Cursor,Snapshot绑定,权限过滤,引用展开,异步Index任务,CAS Revision,幂等写入和审计.
- 开发者可以实现新的Retriever,Indexer,Consolidator,Store和Policy Adapter,但是不能绕过Record Owner和Commit治理.

### 和其他组件的联动

- 和Context&Cache联动时,Memory&Knowledge返回带来源,版本,权限和预算信息的Candidate;Context决定选择,排序,展开和注入.
- 和Continuity联动时,Continuity提供经历和完整来源链,并记录Candidate,Commit,Correction,Forget和Snapshot发布Event;Memory&Knowledge拥有正式Record.
- 和Tool&MCP联动时,Tool可以查询View,获取Source或者提出Candidate,但是所有写入仍然经过统一Commit链.
- 和Review联动时,重要共享Memory,组织Knowledge,冲突合并和高风险自动写入可以进入Review;Review Verdict不能自己执行Commit.
- 和Sandbox联动时,Sandbox只能挂载获准Snapshot,View或者缓存;正式Memory和Knowledge位于可销毁Sandbox之外.
- 和Harness联动时,Harness通过Slot和Port调用Query与Commit流程,不拥有索引策略,Record和Snapshot.
- 和Runtime联动时,Runtime绑定Identity,Scope,MemoryView,KnowledgeSnapshot和Capability Grant;Memory&Knowledge不控制Instance和Run生命周期.

### 我们最后要落成的

- 一套明确分开Memory经历与Knowledge前置认知的领域模型,同时允许它们通过统一Ref,View和Retrieval协议协作.
- 一套Identity,Lineage,User,Team,Domain和Organization分层Scope与继承视图,避免复制和越权共享.
- 一套MemoryCandidate,MemoryRecord,KnowledgeSource,KnowledgePackage,KnowledgeSnapshot,KnowledgeRecord和CommitReceipt合同.
- 一套Skill Index,Lexical,Vector和Graph协同的Hybrid Retrieval,把语义搜索作为候选发现,把来源和Evidence作为最终依据.
- 一套Candidate,Admission,Review,Commit,Correction,Supersede,Expiry,Forget,Withdraw和Reindex生命周期.
- 一套从Continuity经历中可回放地产生经验Candidate的Consolidation流程,不把每个Turn无条件写成长期记忆.
- 一套Knowledge同步,Snapshot发布,Domain Projection和来源撤回机制,让前置语义层可以消费稳定版本.
- 一套权限,隐私,Secret,Poisoning,License,Retention和Legal Hold治理.
- 一套Backend无关的Record与Index架构,让Index可以替换和重建,并对Partial Coverage和陈旧状态保持诚实.
- 一套Memory&Knowledge SDK,CLI和API,让开发者可以自定义Store,Retriever,Indexer和Consolidator,又不破坏Owner和Commit边界.
- 最重要的是,我们最后产出的不是一个自动把所有对话写进向量库的Memory插件,也不是一个新的Wiki生成器,而是一套能够把经历和前置知识真正变成可治理,可引用,可继承,可纠正和可复用认知的基座.
