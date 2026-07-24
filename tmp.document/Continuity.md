## Continuity Documentary

### 综述

- Continuity专门负责Agent运行历史的连续性,可恢复性和可追踪性.
- 它不是简单保存聊天记录,也不是给Session文件夹加一个数据库.一个真实Agent会不断经历Input,Context Frame,Model Turn,Action Candidate,Review,Effect,Tool Result,Checkpoint,Fork,Cancel和Cleanup,Continuity需要把这些离散对象组织成一条可以查询和恢复的因果链.
- Agent的行为更像一张带有分支的历史图,而不是一条永远向前追加的聊天数组.同一个历史节点可以继续运行,可以Fork出新的Reviewer或者子Agent,也可以从Checkpoint建立新的AgentInstance.
- 所以Continuity真正拥有的是Timeline,Event关系,Checkpoint Set,Fork/Rewind/Restore关系,Recovery Credential和历史资产引用;它不拥有其他组件内部的业务事实.
- Runtime仍然拥有Instance,Run,Lease,Fence,Execution Outcome和Cleanup权威状态;Review拥有Verdict;Sandbox拥有SandboxLease和Workspace Change;Context拥有Context Frame;Continuity只负责保存这些领域Owner已经形成的事实引用和有序Candidate.
- 一个Event被写入Timeline不代表它自动成为权威事实.Observation,Intent,Decision,Permit,Effect,Settlement和Claim必须保留自己的类型和Owner,不能因为落库以后就全部变成同一种Event.
- 我们需要同时服务个人设备上的单Agent和企业环境中的上千Agent,所以历史不能无限复制,也不能让高频日志拖垮磁盘.内容寻址,结构共享,分层存储,压缩,保留策略和后台整理都必须成为Continuity的一部分.
- 我仍然认为SQLite加RocksDB是一个很合适的默认实现方向:SQLite负责关系,索引,查询和小型权威元数据;RocksDB负责高吞吐状态流,Delta,Fragment和大型可复用内容.但是公共合同不能直接绑定这两个数据库,开发者应该可以替换后端.

### 关于Continuity拥有和不拥有的内容

- Continuity拥有Timeline,Event Record,Stream Cursor,Parent/Child Link,Checkpoint Manifest,Fork Point,Rewind Plan,Restore Plan,Recovery Credential,Retention Policy和历史查询合同.
- Continuity负责验证Source,Scope,Epoch,Sequence,Revision,Digest和时间关系,并保证同一Source Stream不发生无声覆盖和顺序倒退.
- Continuity可以保存其他组件的Snapshot Ref,Artifact Ref,Evidence Ref,Effect Watermark和Residual Ref,但是不能修改这些对象的领域语义.
- Continuity不决定Review是否通过,不决定Tool是否允许执行,不分配SandboxLease,不组装Context,不把Memory Candidate提升成正式Memory,也不形成Runtime Execution Outcome.
- Continuity不能声称Restore已经把外部世界回滚.邮件,交易,网络请求,远程数据库写入和其他不可逆Effect只能作为已发生或者Outcome Unknown的事实被继承到新分支.
- Continuity也不能把Provider时间戳直接当成全局顺序.每个Source使用自己的Epoch和SourceSequence,跨Source关系通过明确的Causation,Correlation,Parent和Barrier建立.

### 关于Timeline和Event模型

- Timeline是对一个Identity,Agent Lineage,Instance,Run或者Review Case历史的可查询视图;底层可以由多个独立Event Stream和对象引用组成.
- 每个Event Record至少需要EventID,EventKind,Owner,Scope,SourceComponent,SourceEpoch,SourceSequence,Revision,PayloadDigest,ParentRef,CausationRef,CorrelationRef,ObservedAt,RecordedAt和EvidenceRef.
- ObservedAt表示来源声称事情发生的时间;RecordedAt表示Continuity接受记录的时间.两者不能混用,也不能依靠墙上时钟决定因果顺序.
- SourceSequence只在SourceComponent+SourceEpoch内部单调.Source重启,迁移或者恢复以后必须使用新的Epoch,旧Epoch迟到事件只能进入历史,不能覆盖当前状态.
- Event Kind需要保留语义类别,至少区分Command,Intent,Candidate,Observation,Decision,Permit,Effect,Settlement,Claim,Control,Checkpoint和Cleanup.
- Event Candidate先经过Schema,Digest,Scope,Epoch,Sequence和Owner检查,再成为Timeline Event Record.Continuity接受记录只证明这条记录被可靠保存,不证明Payload中的业务结论真实成立.
- 所有查询都需要支持按Identity,Lineage,Instance,Run,Turn,Step,Action,Artifact,Effect和Time Range过滤,并能够从任意对象反查它的来源,父节点,后续结果和当前有效Revision.
- Timeline还需要提供稳定Cursor和增量订阅,让UI,CLI,Context Engine,Review和外部监控不必每次重读全部历史.

### 关于SQLite和RocksDB

- 默认实现中,SQLite适合保存Event元数据,对象关系,当前Revision,幂等键,Source Cursor,Checkpoint索引,Retention状态和面向查询的Projection.
- RocksDB适合保存Context Fragment,Frame Manifest,Tool Result,Workspace Delta,Snapshot Chunk,大型Evidence,压缩块和其他内容寻址Value.
- SQLite不能只作为RocksDB的消费层,RocksDB也不能成为所有事实的唯一来源.两者之间需要通过稳定Object Ref,Digest和Write Journal建立关系.
- 一次跨存储写入应该先形成Pending Record或者Outbox,再写内容块,最后通过CAS把引用提交为Current.进程在任意一步崩溃以后都必须能够Inspect并继续收敛,不能产生数据库之间的无声悬挂引用.
- 大型对象使用Content Addressing和Chunking.多个Frame,Checkpoint和Fork可以共享相同内容,只保存自己的Manifest和Delta,避免重复复制文件与长Context.
- 热数据,温数据和冷数据应该拥有不同Retention与Compaction策略.后台整理不能阻塞Agent热路径,也不能在没有引用证明的情况下删除仍被Checkpoint,Fork,Review或者审计使用的内容.
- SQLite WAL,Checkpoint频率,RocksDB MemTable,Block Cache,Compaction,Compression和Write Amplification都应该成为默认实现的可观测指标,而不是写死成单机内存和磁盘容量承诺.
- 企业部署可以替换为其他关系库,对象存储,流系统或者KV系统;只要满足同样的顺序,幂等,快照,内容寻址和恢复合同,上层不应该感知具体数据库.

### 关于Checkpoint和Snapshot

- Snapshot只是某个Participant在某个时刻提供的状态材料;Checkpoint是多个Participant在同一个Barrier和Effect Watermark下形成的一致恢复点.
- Checkpoint Set至少需要CheckpointID,Scope,BarrierID,Runtime State Ref,Harness Session Ref,Context Generation,Workspace Snapshot Ref,Memory/Knowledge Snapshot Ref,Pending Review Ref,Effect Watermark,Participant Report和Manifest Digest.
- 创建Checkpoint以前必须冻结或者阻止新的冲突Effect,等待正在进行的操作进入可说明状态,并让每个Participant返回Prepared,Unsupported,Partial或者Unknown.
- 只有全部Required Participant满足Policy并且Manifest通过验证,Checkpoint才能Commit.Partial和Unknown Report可以用于诊断,但是不能伪装成可自动Restore的完整Checkpoint.
- Snapshot内容可以位于Sandbox Provider,本地对象库或者远程存储;Continuity保存的是经过验证的Ref,Revision,Digest,Owner和可恢复范围,不能把一次Provider回包直接升级成Checkpoint事实.
- Checkpoint必须明确哪些内容没有被覆盖,例如外部网络状态,远端后台任务,Secret轮换,不可逆Effect和Provider残留.覆盖范围本身就是恢复凭证的一部分.

### 关于Fork,Rewind和Restore

- Fork表示从一个稳定历史节点建立新的Lineage或者Session分支,保留父节点引用,但从Fork以后使用新的身份范围,Epoch,Context Generation和Event Stream.
- Detached Review,Auto Reviewer,子Agent和用户从旧节点继续工作都可以使用Fork,但是它们的Authority,工具面,Context和Sandbox必须重新装配,不能自动继承父Agent全部权限.
- Rewind首先是一份计划,不是立即改变现实世界的命令.它需要声明目标Checkpoint,保留哪些Artifact Change,丢弃哪些Change,如何处理Context和Memory,以及如何继承不可逆Effect.
- 比如一个Turn修改了六个文件,用户只想保留其中两个,系统不能简单回到旧目录.它需要基于文件Revision和Workspace ChangeSet生成选择性Rewind Plan,验证依赖与冲突,再由Sandbox和Tool Gateway执行新的受治理Effect.
- Restore必须创建新的AgentInstance,新的Instance Epoch和新的SandboxLease.旧Instance进入历史或者Cleanup,不能让两个Instance同时声称拥有同一份活跃执行权.
- 新分支继承的是经过验证的状态引用和历史事实,不是复活旧进程.旧Provider Session可以作为恢复线索,但不能替代Praxis自己的Run Session和Runtime事实.
- Restore以后需要形成新的Context Generation,重新验证Profile,Tool Surface,MCP Connection,Review Policy,Authority,Budget和外部世界的新鲜度.任何漂移都必须显式进入Residual或者拒绝恢复.

### 关于文件,Artifact和外部Effect

- Continuity与Sandbox共同保存Workspace Change的历史关系.Sandbox拥有实际文件视图,Overlay,Diff和Commit状态;Continuity负责把每个ChangeSet绑定到Run,Turn,Action,Effect,Artifact Revision和Checkpoint.
- Artifact不能只保存一个路径.至少需要ArtifactID,Revision,Digest,Origin,Owner,Scope,StorageRef,ParentRevision和Evidence.
- Context中的Artifact Anchor也应该进入Timeline,这样以后可以知道Agent在某个Frame中看到的是哪个文件版本和范围,以及后续是否发生了外部修改.
- 文件变更可以通过新的Effect和ChangeSet进行选择性提交或者补偿,但是已经发送的邮件,交易和远程请求不能依靠Rewind消失.
- 对外部Effect,Continuity至少记录Intent,Permit,Attempt,Provider Observation,Settlement,Compensation和Residual之间的关系.Outcome Unknown时后续只能Inspect,不能把历史重放当成再次执行授权.

### 关于Session资产和历史整理

- Session结束以后不应该只留下一个Transcript.它可以产生Result Bundle,Context Frame链,Artifact,测试证据,Review Trace,失败经验,Memory Candidate和Knowledge Candidate.
- Continuity负责保存这些资产之间的来源关系,但是Memory Engine,Knowledge Engine和Asset Owner负责决定哪些内容正式晋升,合并,撤回或者删除.
- 历史整理可以使用后台Compactor,Indexer和Consolidator,但是它们只能生成新的Projection,Summary或者Candidate,不能无痕改写原始Event.
- 高频Debug日志可以按照Retention Policy降采样或者归档;与授权,Effect,Review,Checkpoint,恢复和最终结果相关的证据需要更长保留并支持Legal Hold.
- 删除也必须是显式状态.Tombstone,Privacy Erasure,Retention Expiry和Physical Purge需要区分,并保留允许范围内的审计证明.

### 关于故障,幂等和恢复

- Continuity的写入,查询,订阅,Checkpoint和Restore都需要Idempotency Key,Revision和CAS.网络重试不能产生两条互相冲突的当前事实.
- Source回包丢失时,调用方使用精确SourceEpoch+SourceSequence或者Object Ref Inspect,不能重新生成一个含义相同但身份不同的Event.
- 存储不可用时,新的外部Effect默认不应该继续,因为系统将失去Intent和结果之间的可靠连接.纯只读或者无副作用任务是否降级由Runtime Policy明确决定.
- Projection可以重建,原始Event和已提交对象不能依赖Projection作为唯一副本.索引损坏时应该从Event和Manifest重新生成.
- Cleanup未完成,Snapshot覆盖不确定或者外部Effect未结算时,Checkpoint和Restore必须显式携带Residual,并阻止冲突的新操作.

### 关于Continuity SDK,CLI和API

- Continuity需要向开发者提供Append Candidate,Inspect Event,Query Timeline,Watch Stream,Create Checkpoint,Inspect Checkpoint,Plan Fork,Plan Rewind,Restore,Attach Artifact和Resolve Retention等类型化能力.
- SDK至少需要EventCandidate,EventRecord,TimelineQuery,StreamCursor,CheckpointRequest,CheckpointManifest,ParticipantReport,ForkPlan,RewindPlan,RestorePlan,RecoveryCredential,ArtifactRef和ResidualRef.
- CLI可以提供`praxis timeline show`,`praxis timeline watch`,`praxis checkpoint create`,`praxis checkpoint inspect`,`praxis fork`,`praxis rewind plan`和`praxis restore`等入口.
- API需要支持分页,增量Cursor,事件流,幂等键,CAS Revision,权限过滤,脱敏,归档和长任务Inspect.
- 外部系统只能通过受权Projection查看Timeline.一个日志平台或者Webhook收到Event不代表它成为新的事实Owner.

### 和其他组件的联动

- 和Harness联动时,Harness产生Run,Turn,Step,Frame,Action和Completion Claim等有序Candidate;Continuity记录因果关系,但不把Claim升级成Runtime Outcome.
- 和Context&Cache联动时,Continuity保存ContextFrame,ContextDelta,ContextGeneration和Artifact Anchor的历史引用;Context Engine拥有Frame语义和渲染.
- 和Sandbox联动时,Sandbox提供Workspace ChangeSet,Lease,Snapshot Participant Report,Residual和Cleanup Observation;Continuity拥有它们在Checkpoint与恢复图中的关系.
- 和Tool&MCP联动时,Continuity记录Capability Snapshot,Action Candidate,Attempt,Tool Result,MCP Session变化和Effect Settlement,但不拥有工具业务语义.
- 和Review联动时,Review Case,Round,Attestation,Verdict,Finding和Escalation进入Timeline,Review Owner仍然形成唯一当前Verdict.
- 和Memory&Knowledge联动时,Continuity提供经历和来源材料,Memory与Knowledge产生Candidate并独立Commit;历史记录不能自动成为正式记忆或知识.
- 和Runtime联动时,Runtime提供Identity,Instance,Run,Epoch,Fence,Outcome和Cleanup事实;Continuity提供持久历史,Checkpoint和恢复凭证,不能反向控制Runtime状态机.

### 我们最后要落成的

- 一套Provider无关的Timeline和Event Record合同,能够保存多Source,多Epoch,多分支的Agent历史.
- 一套明确区分Observation,Intent,Decision,Permit,Effect,Settlement,Claim和权威事实的记录模型.
- 一套SQLite+RocksDB默认实现,用关系索引加高吞吐内容存储完成查询,结构共享,增量流和可靠恢复,同时允许替换Backend.
- 一套Content Addressing,Chunk,Manifest,Delta,Retention,Compaction和Projection重建机制,让大规模历史不会拖垮热路径.
- 一套多Participant Checkpoint,Barrier,Effect Watermark和Recovery Credential机制,让Checkpoint真正表示一致恢复范围.
- 一套Fork,Rewind Plan和Restore机制,支持选择性文件变化,新的Instance/Epoch/Lease以及不可逆Effect继承.
- 一套Artifact,Context Frame,Workspace Change,Review,Tool Result和Effect之间的完整因果关系.
- 一套Unknown Outcome只Inspect,幂等,CAS,Outbox和故障恢复纪律,避免重放历史造成重复Effect.
- 一套Continuity SDK,CLI和API,让开发者可以查询,订阅,检查,分支,规划回退和恢复Agent历史.
- 最重要的是,我们最后产出的不是一个无限增长的Session日志目录,而是一套能够让Agent历史可消费,可检查,可Fork,可Rewind,可Restore并且不篡改现实因果关系的连续性基座.
