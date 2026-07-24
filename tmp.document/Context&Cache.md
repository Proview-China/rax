## Context&Cache Documentary

### 综述

- 这一部分本质上是整个agent真正进入LLM之前的最后一层,也是所有Memory,Knowledge,Tool,MCP,Sandbox,Continuity,Skill,Index和人类输入最终汇合的位置.
- 一个agent真正运行起来以后,无论前面做了多少复杂的设计,最后仍然要把当前应该让模型知道的内容整理出来,形成一次模型可以消费的输入,然后等待模型回复,执行对应行为,再把回复和行为结果重新放回下一轮上下文.
- 所以Context并不是简单的conversation history,也不是把几个prompt字符串拼起来.它本质上是一套将当前世界投影给模型的编译层.
- 我们需要允许开发者自由插拔Context来源,定义固定的预埋提示词组,再按照不同的profile,model family,工具策略,权限范围,任务状态和当前信息,动态地插入真正有用的内容.
- 但是我们正在做的是一个框架,所以只做运行时的Context Assembler是不够的.我们还需要提供一整套Context Engineering能力,让开发者可以创建,预览,测试,评估和优化自己的上下文工程.
- 这套工程的目标有三个:第一是让模型看到正确的内容;第二是让Context尽可能短,避免无意义的信息污染;第三是在前两项成立的情况下,尽可能提高Prefix稳定性和Prompt Cache的经济性.
- 这里的优先级必须明确:Context正确性高于缓存命中率,缓存命中率高于单纯追求更长的Prefix.一个稳定但是无用的Prefix,仍然会消耗token,注意力和缓存成本.
- 我们原来的Praxis已经做过比较复杂的PromptPack,Context Compact和CachePlan.这一次不应该继续堆叠复杂对象,而应该先建立稳定,清晰,可追踪,可扩展的Context层.

### 关于Context Engineering

- Context Engineering是这一部分必须正式提供给开发者的工程面,而不是Praxis内部偷偷完成的一段组装逻辑.
- 开发者真正需要定义的内容,通常并不是某一家Provider的messages字段,而是Agent应该持有什么固定认知,如何获得动态信息,哪些内容需要保留,哪些内容需要按需展开,以及这些内容应该按照什么顺序进入模型.
- 所以开发者应该可以定义ContextFragment,ContextRecipe,选择函数,预算策略,压缩策略,引用解析方式和不同Model Profile下的转译方式.
- ContextFragment是最小的上下文材料.它可以来自预埋提示词,Profile,人类输入,文件,命令,Tool,MCP,Memory,Knowledge,Continuity,Sandbox,Skill,Index或者任何其他经过治理的Context Source.
- ContextRecipe是开发者定义的组装规则.它描述什么内容应该进入,什么时候进入,放在什么位置,使用全文,摘要还是引用,最多占据多少token,以及在内容缺失或者预算不足的时候如何降级.
- 开发者还需要能够在真正调用模型之前预览最终结果,看到每一个Fragment来自哪里,为什么被选择,占用了多少token,进入了哪个缓存区段,又有哪些候选内容因为权限,时效,相关性或者预算被排除.
- Context Engineering还需要提供真实反馈.一次Prompt是否有用,不能只看文字是否漂亮,还要看模型回复,Tool Call,执行结果,用户纠正,任务成功率,重试次数,缓存读写,token成本,延迟和压缩损失.
- 所以这部分最终要形成一条完整的开发链路:创建ContextRecipe,编译和预览ContextFrame,使用真实或者回放任务进行评测,比较不同版本的质量和经济性,然后发布,回滚或者继续优化.
- 我们可以参考Codex,Claude Code,Gemini CLI和OpenCode的实现,但是不能直接照搬.它们更多是把Context策略写死在产品内部,而Praxis需要把这些能力抽象出来,成为开发者可以使用和二次开发的框架.

### 关于Context的来源

- ContextFrame可以表达任何真正需要让模型看到的内容.
- 它可以放预埋提示词,Profile,项目规则,人类说的话,人类输入产生的后果,对话历史,文件引用,文件全文,文件片段,命令,命令结果,Tool和MCP的定义,Tool Call,Tool Result,Memory召回,Knowledge投影,Continuity状态,Sandbox观察结果,Skill,Index以及压缩摘要.
- 但是“什么都可以放”不代表把所有内容无类型地塞进一个文本数组.如果没有类型,来源,版本和权限边界,后面就无法正确处理顺序,缓存,压缩,审计和恢复.
- 所以ContextFragment至少需要区分Instruction,Conversation,ArtifactInline,ArtifactReference,ToolDeclaration,ToolCall,ToolResult,StateObservation,MemoryRecall,PolicySnapshot和CompactionSummary等不同语义.
- ArtifactInline代表内容已经真正进入模型输入;ArtifactReference只代表模型知道存在这个资产,需要通过对应Resolver或者Tool继续读取.一个裸文件路径并不会自动让模型知道文件内容.
- 所有来源先产生Context Candidate,然后再由Context层判断它是否能够进入当前Frame.候选内容很多,不代表都应该被注入.
- 这个判断至少要考虑当前任务相关性,来源权威,内容新鲜度,权限范围,引用版本,重复程度,token成本,缓存价值和模型是否已经在当前Context Generation中看过它.
- 权限和治理结果也可以成为模型可见的Context,但是应该放入安全的能力说明和决策快照,而不是把credential,secret或者内部不可见规则直接交给模型.
- Context层负责决定什么应该被模型看到,但是它不应该取代真正的权限执行.模型看见“禁止外发”并不等于系统已经阻止外发,最后的强制门禁仍然属于Tool,Sandbox和Review等执行边界.

### 关于Context Source Port和Admission

- 每个Context Source应该通过Versioned Port提交`ContextCandidate`,而不是直接向messages数组追加字符串.
- Candidate至少携带CandidateID,Kind,Owner,Scope,SourceRef,Revision,Digest,Authority,Sensitivity,Freshness,Token Estimate,Cache Stability,ExpiresAt和EvidenceRef.
- Context Engine先验证Scope,权限,来源和当前性,再根据Recipe,任务相关性,重复程度,预算和模型能力形成`ContextAdmissionDecision`.被排除的Candidate也需要记录原因,方便开发者调试和评测.
- Source只能描述自己提供的内容和Evidence,不能决定最终放置顺序,Role,缓存区段和是否全文展开.这些语义属于Context Recipe和Frame Compiler.
- Tool Result,Memory Recall,Knowledge Retrieval,Sandbox Observation和Continuity Summary全部属于不可信或者受限材料.它们可以被引用,但不能携带新的系统权限,覆盖开发者指令或者改变Review Policy.
- Required Source不可用,版本漂移或者权限无法验证时默认拒绝生成Frame;Optional Source可以按照Recipe显式降级,但是Frame Manifest必须记录缺失和Residual.
- Context Prepare,Reference Resolve和Frame Freeze都需要幂等键,Revision和Digest.回包未知时先Inspect原Frame Attempt,不能重新生成一个内容可能不同却复用同一FrameID的对象.
- Context Port返回的是不可变Frame Ref和Manifest,不是对内部存储的可变指针.Harness消费Frame以后,任何新增内容都进入下一次Frame Generation.

### 关于ContextManifest和ContextFrame

- 我认为这里需要把ContextRecipe,ContextManifest,ContextFrame和ContextOutcome四个概念分开.
- ContextRecipe是开发者定义的长期规则;ContextManifest是这一次组装真正解析出来的计划;ContextFrame是Manifest完成渲染以后形成的不可变冻结点;ContextOutcome则记录这个Frame被模型消费以后产生的回复,行为和实际效果.
- Manifest需要记录这一轮选择了哪些Fragment,每个Fragment来自哪里,对应什么版本和引用,为什么进入或者被排除,使用全文还是引用,占据多少token,处于什么缓存区域,以及最终按照什么顺序渲染.
- ContextFrame则是当前一次模型调用的精确状态.它应该直接送进Agent本轮的模型调用,模型根据这个冻结点进行回复,而不是在发送过程中继续偷偷改变内容.
- 每一个ContextFrame都应该拥有稳定的FrameID,ParentFrameID和Context Generation,并绑定Recipe版本,Manifest摘要,模型Profile,来源版本集合,渲染版本,缓存计划,token预算和触发它的Event.
- Frame必须可以被精确检查和回放.当模型行为异常时,我们不能只知道“大概给过这些内容”,而应该能够知道当时模型究竟看到了什么,顺序是什么,哪些内容被压缩了,哪些引用还没有展开,又是哪一个变化破坏了缓存.
- 但是完整留痕不代表每一轮都复制一遍几万token的Prefix,文件和Tool Result.我们应该使用内容寻址,Fragment引用和结构共享,让多个Frame共同引用同一份稳定资产,只记录本轮新增,替换和失效的Context Delta.
- 这样一个Frame可以由Stable Prefix引用,Project Context引用,Session Summary引用,当前工作集引用,本轮Dynamic Tail和Render Manifest共同组成.它在语义上是完整的,在存储上却不需要重复复制全部内容.
- ContextFrame是一个冻结点,不是新的事实权威.文件,记忆,权限和外部状态的权威仍然属于各自组件;Frame只是记录在这一时刻,这些事实以什么版本和什么形式被投影给了模型.

### 关于Context的顺序和Prompt Cache

- Prompt Cache的本质是Prefix复用.所以Context层必须把稳定内容放在前面,把变化频繁的内容放在后面,并且保证相同语义在不同Turn中拥有稳定的顺序和稳定的渲染结果.
- 比较稳定的Prefix通常包括Praxis内核提示词,Agent Profile,Model Profile,长期项目规则,基础工具定义和稳定的Context协议.
- 半稳定区域可以放项目状态,Session Summary,当前工作集索引,活跃Memory和已经确定的Continuity状态.
- Dynamic Tail则放最近对话,当前文件片段,实时Tool Result,临时观察值和用户本轮输入.
- 工具定义,Skill说明和其他集合型内容应该使用稳定排序.不能因为一个新的MCP Tool插入到中间,就让前面已经稳定的大段Prefix全部失效.
- 但是Prefix不是越长越好.一个内容只有在有用,稳定并且很可能被复用的时候,才值得长期放在Prefix中.无关内容即使被缓存,仍然会产生cached token成本和注意力污染.
- 所以我们真正追求的是:Prefix尽可能稳定并且有用,ContextFrame整体尽可能短.
- 我们希望达到90%以上的缓存使用率,但是这个指标应该以cache-eligible prefix为分母,而不是拿全部输入token计算.用户本轮输入,实时工具结果和其他天然动态内容不应该被算成缓存失败.
- Context层需要记录cacheable prefix token,cache read token,cache write token,dynamic token,cache miss原因,Prefix发生变化的位置,不同Fragment的贡献和最终费用.没有这些反馈,开发者就无法真正优化Context Engineering.
- 不同Provider的缓存机制,TTL,价格,缓存断点和cache key策略都不一样,并且会持续变化.所以Praxis应该提供ProviderCacheProfile,由Model Profile和Provider Adapter负责转译,不能把某一家厂商的cache_control直接写进Context核心语义.
- 对于5分钟,30分钟,一小时或者更长时间的缓存,我们可以根据真实复用概率设计预热和续期策略,但是不应该无条件每隔几分钟制造一次模型请求.只有预计节省的读取成本高于写入,保活和调用成本时,主动刷新才有经济意义.
- Cache优化最终仍然要服从Context正确性.如果权限,文件版本,项目规则或者当前任务已经变化,就应该主动产生新的Frame和新的Prefix版本,而不是为了命中缓存继续使用陈旧内容.

### 关于不同Harness中的实际Context控制

- Direct API Route通常允许Praxis完整控制Instructions,Messages,Tool Definitions和History;Codex,Claude Code以及其他官方Harness则可能拥有自己的系统提示,内置工具,Session历史,Compaction,Skill,Hook和启动上下文.
- 所以ContextFrame是Praxis希望交付的语义输入,Actual Injection Manifest则记录Provider或者官方Harness最终实际接收和额外注入了什么.两者不能因为调用成功就假定相同.
- 每个Harness Capability Profile需要声明哪些区域可以full replace,哪些只能append,哪些由官方Harness拥有,哪些完全不可见,以及是否能够控制Tool Registry,Memory,Skill,MCP,Hook和Compaction.
- Preflight必须比较Expected与Actual Manifest.强制指令,权限,Secret,Workspace,Tool Surface和Context Source出现未允许漂移时直接拒绝;允许的官方Residual进入Conformance和Evidence.
- Provider原生Session可以帮助复用历史和缓存,但是Praxis仍然需要自己的Context Generation,FrameID和Run Session Ref.原生Session ID只能作为Observation和Resume Hint.
- Provider自己发生Compaction或者隐藏重试时,如果无法获得完整机制事件,应该把event fidelity标记为partial或者unavailable,不能补造一个看似精确的Context历史.
- Cache命中也需要区分Praxis规划的Prefix和Provider实际报告的Cache Read/Write.无法观测的厂商缓存只能标记unknown,不能为了达成90%指标推算成命中.
- 这套Expected/Actual机制让同一Context Recipe可以同时服务Direct API和官方Harness,又不会把不同产品的隐藏行为伪装成统一事实.

### 关于文件,Artifact和锚点

- 文件是Coding Agent最容易浪费Context的地方,所以每一次文件读取,创建,写入和修改都应该产生可追踪的Artifact Anchor.
- 比如Agent第一次创建了一个一千行的A.go,并且完整内容已经出现在当前Context中,那么下一轮并没有必要为了证明文件存在,再次把一千行全部读入.
- 但是我们也不能单纯依靠模型“记得自己写过”.系统需要知道Agent看过或者写入的是哪个文件版本,看过完整文件还是某个区间,这个版本后来有没有被其他进程修改,以及压缩以后原始内容是否仍然处于当前Context Generation.
- 所以锚点的意义不是告诉模型“文件肯定没变”,而是证明某一个Agent在某一个Frame中,已经获得了某一版本,某一范围的文件内容.
- 当文件版本和读取范围都没有变化时,重复读取可以只返回file unchanged和对应Anchor引用;当文件发生变化时,优先提供从已知版本到当前版本的diff;只有锚点失效,内容冲突或者任务需要更大范围理解时,才重新读取完整内容.
- 小文件可以直接全文读取.大文件应该先使用Index,符号,结构,精准搜索和范围读取来确定工作区间,然后只注入相关片段.修改以后继续使用diff,搜索结果和新的Anchor维护认知.
- 但是diff链不能无限增长.当变化累积过多,跨越Context Generation,或者继续回放diff已经比重新物化文件更贵时,系统应该生成新的基准版本和新的Artifact Anchor.
- Tool的写入参数,实际文件结果和写入后的版本必须对应起来.一次Tool Call声称写入成功,并不自动证明磁盘上的最终内容与参数完全一致;必要时仍然需要通过receipt,摘要或者定向读取完成确认.
- 这套机制不仅适用于代码文件,也适用于文档,命令结果,网页快照,数据库查询结果和其他大型Artifact.

### 关于Context Compact和Context Generation

- Context不可能无限增长,所以压缩是Context层的必需能力,但是压缩不能只是把旧对话随便总结成一段文字.
- 一次可靠的压缩至少要保留当前目标,用户纠正,有效要求,关键决策,已验证事实,文件和Artifact Anchor,Tool状态,未完成工作,阻塞项,外部Effect和对应来源引用.
- 压缩以后应该形成新的Context Generation.旧Frame和旧历史仍然由Continuity保留,新的Generation使用CompactionSummary,保留的最近Tail和仍然有效的Artifact Reference继续运行.
- CompactionSummary本身也是一种ContextFragment,它应该有来源范围,生成方式,版本,摘要前后的token变化和质量评估,不能无痕覆盖原始历史.
- 对于不能安全压缩的内容,比如精确代码,结构化数据,权限凭证和不可逆Effect结果,应该保留引用或者重新物化,而不是让LLM总结后把摘要当成新的事实权威.
- 压缩也会破坏Prompt Cache,所以我们需要记录一次cache miss是来自TTL过期,Prefix变化,Tool定义变化,模型切换还是Context Generation更新.只有知道真实原因,后面才有可能优化.
- Context压缩的最终目标不是简单减少token,而是在更短的Context中保留继续完成任务所必需的因果链和工作状态.

### 和Continuity的联动

- ContextFrame和ContextOutcome都必须进入Continuity的timeline.每一次模型调用之前应该产生ContextFramePrepared,调用完成以后应该产生ModelResponseObserved和ContextOutcomeRecorded,压缩时则产生ContextGenerationCompacted.
- Context层负责Frame的语义,组装,渲染和缓存计划;Continuity负责这些Frame在Session,Turn,Run,Checkpoint,Fork和Rewind中的顺序,关系,持久化和恢复凭证.
- 按照Continuity目前的设计,SQLite可以负责Event原数据,关系和查询索引,RocksDB负责高吞吐的Frame Manifest,Context Delta,Fragment,大型Tool Result和可复用状态流.
- 但是Context Engine不应该把自己的公共合同直接绑定到SQLite或者RocksDB.它应该输出稳定的Frame,Fragment和Event合同,由Continuity的默认实现决定如何存储和消费.
- 通过内容寻址和结构共享,Continuity可以保存完整的Context历史而不必反复复制大型资产;通过FrameID,ParentFrameID和Context Generation,又可以还原任意一次模型调用真正看到的内容.
- Rewind也不能只恢复一段聊天记录.它需要从一个一致的Checkpoint重新生成新的AgentInstance和新的Context Generation,并明确继承哪些Frame,Artifact,文件改动和不可逆外部Effect.

### 和其他组件的联动

- 和Memory&Knowledge联动时,Memory和Knowledge负责生产有版本,有权限,有来源的候选内容;Context负责根据当前任务选择,排序,压缩和注入,不能因为召回命中就把全部内容直接塞给模型.
- 和Tool&MCP联动时,工具定义,Skill说明,Tool Call和Tool Result都可以成为ContextFragment;Tool&MCP负责能力合同和真实调用,Context负责模型可见形式,稳定排序,按需展开和token治理.
- 和Sandbox联动时,Sandbox负责产生文件,进程,网络,workspace diff和执行残留等真实观察;Context负责把当前需要的观察结果投影给模型,但是不能用一段Context说明代替执行门禁.
- 和Review联动时,Context可以向模型展示安全的Review要求和Verdict摘要,但是Verdict的权威,版本绑定和强制执行仍然属于Review和Effect Gateway.
- 和Runtime联动时,Runtime在每个Turn和Tool Loop边界请求新的ContextFrame,管理AgentInstance和运行状态;Context Engine负责Prepare和Inspect,并把冻结后的Frame交给Model Invoker.
- 和Model Profile联动时,同一个ContextRecipe可以被编译成不同Model Family适合的顺序,工具表达和缓存计划,但是不能因此改变上层Context语义和来源事实.

### 我们最后要落成的

- 一套面向开发者的Context Engineering SDK和工程流程,允许创建,组合,预览,评测,优化,发布和回滚ContextRecipe.
- 一套稳定的Context Source和ContextFragment合同,能够接入Prompt,Profile,Human,File,Tool,MCP,Memory,Knowledge,Continuity,Sandbox,Skill和Index等来源.
- 一套ContextCandidate,Context Source Port和Context Admission机制,让来源只贡献受治理材料,不能直接改写最终模型输入.
- 一套ContextRecipe到ContextManifest再到ContextFrame的编译流程,保证每一次模型调用都拥有不可变,可检查,可回放的上下文冻结点.
- 一套Context Admission,Ordering,Budget和Reference Resolver机制,只让真正有用,有权限,有版本依据的内容进入模型.
- 一套Provider无关的Prompt Cache规划层,配合Model Profile处理稳定Prefix,Dynamic Tail,cache key,breakpoint,TTL和经济性反馈.
- 一套Artifact Anchor和Context Delta机制,让大型文件和工具结果能够通过版本,范围,diff和按需读取被高效使用,避免反复灌入完整内容.
- 一套可靠的Context Compact和Context Generation机制,能够在压缩以后保留任务因果链,有效引用,工作状态和恢复凭证.
- 一套Context Outcome和评测反馈机制,把模型质量,Tool行为,执行结果,用户纠正,缓存,token,费用和延迟重新反馈给开发者.
- 一套与Continuity打通的Frame/Event持久化和结构共享机制,让完整上下文历史可追踪,可查询,可复用,可Fork,可Rewind,又不会因为重复存储拖垮系统.
- 一套Expected/Actual Injection Manifest和Harness Capability Profile机制,诚实表达Direct API与Codex,Claude Code等官方Harness的上下文控制差异.
- 最重要的是,我们最后产出的不是一个更复杂的PromptPack,而是一套真正能够控制LLM入口,服务不同Agent,不同Model Family和不同业务域的Context Engineering基座.
