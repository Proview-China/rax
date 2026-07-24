# Memory Engine 可实施设计 v1

- [终局框架设计 V1](./framework-completion-v1.md)

## 1. 状态与依据

- 设计状态：Memory backend-neutral framework、V1/V2 Current Reader、Memory Adapter、Application三阶段协调及Context非零reference fixture已实现并完成软件测试；Owner-local与cross-owner reference integration均为**P0=0/P1=0/P2=0**。真实远程Gateway、生产Backend及production G6B/root仍NO-GO。
- 最高业务输入：`tmp.document/Memory&Knowledge.md`。
- 共同治理输入：仓库`AGENTS.md`、`.properties.rax/MAIN.md`、6+1组件治理合同及Runtime/Harness live公共合同。
- 实现位置：`ExecutionRuntime/memory-knowledge/**`；本设计继续作为实现与验收基线。
- 语言：默认Go。v1不规划Rust；只有后续基准证明混合召回、图遍历或批量向量运算成为可隔离热点，才单独评审Rust边界。

Memory与Knowledge共用引用、视图和检索协议，但拥有不同事实、状态机、Admission与Commit Owner。共用协议不构成共用事实库，也不允许Memory经验静默晋升为Knowledge事实。

## 2. 目标与冻结边界

Memory回答“某个Identity或Scope过去经历过什么，并形成了哪些偏好、纠正、工作方式和可复用经验”。v1闭合：

1. 来源可追溯的`MemoryCandidate`；
2. Schema、Scope、敏感度、重复、Poisoning、Policy和目标版本Admission；
3. Review、受治理正式Commit、Receipt、Inspect、CAS与Settlement；
4. `MemoryRecord`、`MemoryRef`、`MemoryView`与版本化检索；
5. Correction、Supersede、Merge、Expiry、Archive、Forget、Tombstone和Purge申请；
6. Skill Index、Lexical、Vector和Graph作为可重建Projection；
7. 向Context Engine返回有来源、有版本、有权限水位的候选，而非直接注入模型。

v1明确排除：

- Run Working Memory。它由Context Generation与Continuity管理；
- Transcript自动落长期Memory；
- 原始Chain of Thought、未结算Effect、Outcome Unknown猜测和未经授权私人数据自动晋升；
- 生产数据库、向量库、图库、RPC、进程拓扑、SLA和默认自动衰减参数；
- Memory修改Task、Goal、Runtime Run或ExecutionOutcome；
- 用Provider命中、模型总结、点击率或引用次数直接改写正式Memory。

## 3. Owner与非Owner

| 语义 | 唯一Owner | Memory Engine只做什么 | Memory Engine禁止做什么 |
|---|---|---|---|
| Memory Component Manifest/Run Requirement | Memory Owner | 发布namespaced能力、版本与依赖声明 | 定义公共Binding、Slot/Phase合并或Run Plan |
| Candidate/Admission/Record/Correction/Forget | Memory Owner | Validate、Inspect、CAS、生成Receipt和领域Settlement | 接受模型输出为永久事实 |
| Runtime Scope/Run/Fence/Operation治理 | Runtime | 消费精确Ref、Permit、Fence和Settlement合同 | 修改Run/Instance/Outcome |
| 跨域编排 | Application Coordinator | 接收调用并返回领域结果 | 由Memory直接串改其他组件事实 |
| Review Verdict | Review Owner | 消费精确Candidate/Intent Verdict | 自审后自己Dispatch，或把Attestation当Verdict |
| Evidence Ledger | Evidence Owner | 提供Owner Fact Ref与来源事件 | 把Ledger记录本身当Memory事实 |
| Context Frame与最终注入 | Context Owner | 返回`RetrievalResult`候选 | 决定最终Prompt顺序、预算或Channel |
| Timeline/Checkpoint/Fork | Continuity Owner | 输出Record/Snapshot/View Ref | 把Timeline Event自动晋升Memory |
| 原始Artifact字节 | Asset Owner | 保存Asset/Content Ref | 复制并取代正式Artifact所有权 |
| Sandbox/Tool/Harness | 各领域Owner | 接收其受治理Candidate/Observation | 导入其实现包或接管其状态机 |

Memory domain/kernel未来只依赖自身合同。任何Runtime Adapter只能依赖`runtime/core`与`runtime/ports`；不得依赖Runtime foundation/fakes/kernel或Harness kernel/fakes/internal。

## 4. 放置与信任边界

```text
Sandbox/Data Plane
  模型输出、用户纠正、Tool结果、Consolidator输出
              |
              v  只形成Observation/Candidate
Application Coordinator + Host Governance Gateway
              |
              v
Sandbox外Memory Controller（Admission/Owner/Commit/Inspect/CAS）
              |
              v
State Plane（Candidate、Record、Commit Attempt、Projection、View）
              |
              +--> Context Engine只读消费RetrievalResult
              `--> Remote Store/Indexer（可选Effect；回包只作Receipt）
```

正式Memory和权威Record元数据必须位于Sandbox外。Sandbox本地副本、向量索引、Graph和Cache均不是唯一权威副本。远程检索、外发、写入、删除和保留变化必须作为受治理Operation Effect。

## 5. Scope与View

长期Memory Scope固定支持：

| Scope Kind | 用途 | 默认共享 |
|---|---|---|
| `identity_private` | 某Identity私有偏好与经验 | 禁止 |
| `lineage` | Agent Lineage可复用经验 | 仅显式View |
| `user` | 用户授权的跨Agent偏好 | 仅当前Purpose允许 |
| `team` | 团队流程与共同经验 | 需团队Authority |
| `domain` | 业务域经验 | 域间默认隔离 |
| `organization` | 组织批准共享经验 | 需组织Policy/Review |

`MemoryView`不是内容副本，而是不可变查询能力描述，至少绑定：Tenant、Principal/Identity、Authority Ref/Epoch、Policy Ref/Revision、Purpose、允许Scope集合、Subject过滤、Sensitivity上限、可返回形态（全文/摘要/引用/存在性）、Snapshot/Record水位、Projection集合、预算、TTL与Digest。

子Agent、Reviewer和管理Agent不继承父Agent全部Memory；它们必须获得单独View。每次Query与Commit都绑定当前Purpose和Action Scope，旧Authority Epoch只能用于历史Evidence，不能继续读取或写入。

## 6. 领域架构

### 6.1 权威层与Projection层

- `MemoryRecord`和其版本链是权威事实；
- `SkillIndexEntry`、Lexical Posting、Embedding、Vector分片和Graph Node/Edge是可重建Projection；
- Projection必须绑定Record ID/Revision、Content Digest、Builder/Model/Schema/Index Version、Coverage、State和Projection Digest；
- Projection损坏、过期或模型升级不得修改Record，只能标记`stale`并重建；
- `partial`必须向查询结果显式传播，不得伪装全覆盖。

### 6.2 Candidate到正式Record

```text
proposed
  -> admission_checking
  -> rejected | merged | review_pending | commit_ready
review_pending
  -> rejected | expired | commit_ready
commit_ready
  -> commit_dispatching
  -> commit_unknown | committed | rejected
commit_unknown
  -> Inspect原Attempt
  -> committed | confirmed_not_applied | reconciliation_required
```

Admission只决定候选能否继续，不产生正式Memory。`merged`表示候选并入另一候选或目标修订建议，不代表Record已变更。

Runtime live P0.1-P0.6、Operation V3与Application Coordinator已经闭合，本组件直接复用，不重造治理链。所有外部动作严格保持唯一顺序：

```text
领域Intent/Reservation
 -> Operation Admission
 -> Permit
 -> Begin
 -> Delegation/Prepare
 -> Enforcement
 -> Execute或Inspect原Attempt
 -> Observation
 -> Memory Owner Inspect形成DomainResultFact
 -> Evidence Owner追加绑定该DomainResultFact的Evidence
 -> Runtime Operation Settlement Owner提交Operation Settlement
 -> Memory ApplySettlement CAS形成领域settled/result_ready投影
```

领域细化如下：

1. Candidate持久化并计算Canonical Digest；Memory Owner独立Inspect来源、Scope、Policy、目标Revision与Admission事实；
2. 需要Review时，Review Verdict精确绑定Candidate Digest、Candidate Revision、Effect Payload Digest、Scope、Authority、Policy和TTL；`operation_not_required`也必须有正式事实；
3. Application Coordinator提交namespaced Operation Intent与Reservation；写入类使用`formal_commit`语义，携带稳定幂等键、Conflict Domain和Effect/Settlement/Cleanup Owner；
4. Runtime完成Admission、Permit与Begin；Delegation/Prepare后，宿主Gateway在dispatch前执行Enforcement；
5. Memory实际Commit点再次独立读取当前Identity、Binding、Authority、Review、Budget、Policy、Scope与Fence并验证Permit；
6. State Plane以`ExpectedRecordRevision`或`expect_absent`执行一次CAS；远程Provider/Store回包只形成Receipt/Observation；
7. Memory Owner按原Attempt Inspect实际Record与Commit Journal，形成精确Owner Fact；Evidence Owner只引用该Fact，不能把Provider回包升级为权威事实；
8. Runtime Settlement Owner基于精确Observation/Inspect引用提交Operation Settlement；Memory只ApplySettlement形成领域settled/result_ready投影。Coordinator关联全部Settlement，但Memory不得选择或修改Runtime Outcome。

任何门禁不可读、摘要漂移、TTL过期、Scope缩小、Review撤销、Budget失效、Fence变化或CAS冲突都Fail Closed。

## 7. UnknownOutcome与恢复

- Commit前失败：可判定`confirmed_not_applied`，不得生成Record；
- 实际Commit点Prepare之后超时、断链或回包丢失：进入`commit_unknown`；
- `commit_unknown`只能按原`CommitAttemptID + IntentID/Revision + IdempotencyKey`调用`InspectCommit`；
- 禁止创建新Candidate ID或新Record ID来“重试同一语义”；
- Inspect确认已应用后，复用原Record/Revision并结算；确认未应用后，是否再次派发必须由新治理Attempt和当前Policy决定；
- Inspect仍不完整时保持`reconciliation_required`并占用稳定冲突域；
- Remote Residual、Cleanup与业务Commit结果分别报告，不能用“Record存在”推导远程索引或敏感副本已清理。

### 7.1 Pre-run Evidence声明

本设计**条件性触发pre-run Evidence**：无活跃Run的管理员纠错、Forget/Purge、组织级Scope变更可使用admin/custom Operation Subject。时点固定为：

```text
Memory Owner CAS形成Owner Fact
 -> Evidence Owner追加绑定OperationSubject的authoritative Evidence
 -> Runtime Operation Settlement Owner提交Operation Settlement
 -> Memory ApplySettlement CAS
```

不得伪造Run。当前Run内Evidence路径继续使用既有合同；只对上述无Run管理Operation提出`OperationScope-aware Evidence` Delta，因此它不是Runtime总阻塞。但该结论不覆盖远程Retrieval：Tool G6A与Checkpoint V5均不适用，Retrieval仍等待专用加法版本。

### 7.2 Effect、Conflict Domain、OperationScope、RunStart/RunSettlement Requirement

| Effect kind | 动作 | 关键恢复 |
|---|---|---|
| `praxis.memory/record-commit` | 正式Record创建、纠错、Merge、Supersede | CAS；Begin后只Inspect原Attempt |
| `praxis.memory/forget` | Tombstone并切断新View可见性 | Owner Fact与物理Purge分开结算 |
| `praxis.memory/purge` | 删除远程副本、索引或正文 | 每个Backend独立Inspect并保留Residual |
| `praxis.memory/remote-query` | 向远程检索服务披露Query/标识 | 记录披露、费用与Partial Coverage |
| `praxis.memory/remote-index-build` | 远程Embedding/Graph/Indexer | 失败只降级Projection，不改Record |
| `praxis.memory/reindex-publish` | 切换Projection/View水位 | Expected Revision CAS并可回读旧水位 |
| `praxis.memory/view-publish` | 发布受治理View | Authority/Policy TTL与Digest精确绑定 |

本地纯读、Candidate持久化、Admission以及基于权威Record的本地Projection计算不创建Runtime Effect，但仍受Authority、Scope与资源策略约束。

Runtime Conflict Domain使用tenant-stable namespaced基类：`praxis.memory/record`、`view`、`projection`、`purge`。具体Record/View的排他性由Target Digest、稳定Idempotency与Memory Store CAS共同实现，不能包含会随Run/Instance变化的值。

Memory分别声明：`RunStartRequirement`要求精确`MemoryView + Record/Snapshot Watermark + ProjectionCoverage + Authority/Policy TTL + ContextReference能力`；远程查询只产生精确`OperationScope`及Effect结算；Run内正式写入时才贡献唯一Owner的`praxis.memory/domain-commits` RunSettlementRequirement参与者；无Run管理动作使用admin/custom OperationScope，不伪造RunStartRequirement或RunSettlementRequirement。Memory不创建Run Plan、不认证Binding、不选择Runtime Outcome。

## 8. 纠错、遗忘与冲突

### 8.1 Record生命周期

```text
active -> superseded | expired | archived | tombstoned
expired -> active（只允许新Revision和显式Policy）| archived | tombstoned
archived -> active（新Revision）| tombstoned
tombstoned -> purge_pending -> purged
```

- 更新不覆盖历史；Correction创建新Revision并保存`corrects/supersedes`关系；
- 主观Memory冲突可以并存，必须保留Subject、Owner、来源和冲突组；
- Merge产生新Record或新Revision，不能无痕删除来源记录；
- Expiry改变Current View，不销毁历史Evidence；
- Forget首先创建受治理Tombstone，使新View不可见；Physical Purge是独立Effect；
- Legal Hold或审计Policy可以阻止物理清除，但必须明确返回`purge_blocked`，不能声称已删除；
- Purge后仍保留不含敏感正文的最小审计凭证，具体法务字段由管理线Policy决定。

### 8.2 Feedback

检索采用率、用户纠正、任务结果和Reviewer反馈只形成`RetrievalFeedbackCandidate`。它绑定Query、Result、Record Revision、Context Frame和Outcome Evidence；只能触发评测、重排策略候选或Correction Candidate，不能直接修改Record、Projection权重或正式事实。

## 9. 检索、引用与Context边界

```text
RetrievalQuery
 -> Resolve MemoryView
 -> 在取正文前做Purpose/Scope/Policy/Sensitivity过滤
 -> Intent Router选择Skill Index/Lexical/Vector/Graph
 -> 多路候选、去重、冲突分组、来源验证
 -> Rerank与预算截断
 -> Retrieval Observation Ref + Result Ref
 -> Memory Owner Inspect current Commit chain/content/TTL
 -> Application Coordinator交给Context Owner
 -> Context物化Candidate并Admission/排序/冻结Frame
```

`RetrievalResult`必须说明命中原因、使用的Projection、Record Revision、来源Evidence、冲突、可信/主观状态、Coverage和建议展开形态。它只产生可Inspect的Observation/Result精确引用，不直接成为Context Candidate或Frame。Recall成功不表示模型已经看到；Context Engine拥有最终选择、排序、展开、压缩、Channel和Token预算。

Memory只能向Harness间接提供已经由Context/Coordinator物化的Snapshot。Harness私有`ContextPort`不得直接访问Memory Store、发起远程检索或写反馈。

### 9.1 Memory Owner独立Inspect到Context物化桥

Memory与Knowledge保持两个独立Owner、Port、Contribution和预算域。Memory侧面向Context的纯只读能力命名为`MemoryContextSourceCurrentReaderV1`：

完整字段、canonical、S1/S2、Unknown与Checkpoint分离合同见：[Memory Current Reader设计Delta](./context-source-port-v1.md)。Memory Reader、Memory Adapter、Application三阶段Port与Context Owner-local Refresh/Apply/Inspect均已YES；reference fixture的Memory来源数为1。远程Gateway与production G6B/root仍NO-GO。

```text
InspectAttempt(localOwnerAttemptRef) -> LocalAttemptInspectionV1
InspectForTurn(request) -> MemoryContributionCurrentProjectionV1
ReadContentExact(localOwnerContentRef) -> LocalExactContentObservationV1
```

`InspectForTurn`请求至少绑定：Contract Version、Execution Scope Digest、Run/Turn、原Retrieval Attempt、Observation/Result exact refs、Expected Query/View/Watermark、Authority/Policy/Purpose、独立MaxItems/Bytes/Tokens与单项上限、Token Estimator Ref、Inspect时间、Frame Deadline和Idempotency Key。

返回的`MemoryContributionObservationV1`至少包含：Contribution exact ref、固定Owner=`memory`、Run/Turn/Scope、Retrieval Observation/Result、Query/View/Watermark、Coverage/NextCursor/Result/Evidence Digest、Observed/Expires、Residual，以及稳定有序的Items。每个Item绑定Rank/Score、Record/Content exact refs、DomainResult Ref、`DomainResultAssociation`、SettlementApplication Ref、Source/Evidence/Projection/Citation、Token Estimate/Estimator Digest和Record TTL；整体再计算Association Set、Citation Set与Contribution canonical digest。

Memory Owner必须从当前State Plane反向Inspect Record对应的Commit链，重算DomainResult canonical digest并验证Association的ID/revision/digest精确绑定、Owner、Subject Ref、CASAfter和SettlementApplication；随后验证Record仍是当前active revision、View/Watermark/Authority/Policy/Purpose/Scope/Sensitivity仍有效。Context Owner不得绕过Port读取Memory Store来替Memory判断currentness。

Frame Assembler通过Current Reader复读Owner current事实，只读取Memory Owner State Plane已经本地可exact读取的正文并重算Length/MediaType/Digest。S1后Context只能形成不可见pending DomainResult；S2 fresh current/TTL通过后，Context Owner才在单个本地原子边界执行ApplySettlement与Generation current CAS，S2失败不得发布current。远程Query或正文读取不属于该Reader，必须由Application经Runtime治理的Retrieval Domain Gateway携带Operation/Attempt/Permit、prepare+execute Enforcement执行；远程lost reply只调用Gateway的`InspectOriginalAttempt`。

当前G6A closed matrix仅覆盖Tool，Checkpoint V5也不能用于Retrieval。Retrieval-specific additive Applicability/Evidence/Settlement版本由相应Owner联合冻结前，Memory远程Gateway全部`unsupported`且Provider调用数为0；本地Current Reader不受影响。

### 9.2 排序、预算与Fail Closed

- Owner内稳定顺序固定为`Score降序 -> Record ID升序 -> Revision降序 -> Digest升序`；Score不代表事实置信度；
- Memory只在自己的`MaxItems/MaxBytes/MaxTokens/PerItem`预算内按exact bytes和固定Estimator贪心选择，并记录所有跳过原因；Context Recipe/区域/总预算仍是最终Admission权威；
- Memory与Knowledge的Score不比较、不归一化，预算不互借；相同Content Digest不跨Owner自动去重；
- 为避免Context现有候选稳定排序破坏检索rank，最小桥每个Owner产生一个canonical Contribution Bundle，Bundle内保留有序Items和全部Citation；Owner间位置只由公共Context Recipe决定；
- 已选高排名项在freeze前发生currentness、TTL、内容或Association漂移时，当前Frame attempt失败，不得在同attempt静默以低排名项补位；
- `now == ExpiresUnixNano`视为过期。压缩summary不是Memory事实；exact anchor未保留，或保留后Owner current事实已经漂移，都必须rematerialize并Fail Closed；
- Memory必需Contribution不可验证时Frame失败；可选Contribution只能按已冻结Recipe产生显式Residual，不能声称模型已经看到。

## 10. SDK、CLI和API边界

- SDK公开：SubmitCandidate、InspectCandidate、RequestAdmission、RequestCommit、InspectCommit、Correct、Forget、Query、InspectRecord、WatchChanges、Reindex；写操作只提交Intent，不直接触碰Fact Store。
- CLI建议：`praxis memory search|inspect|correct|forget|index status`；CLI必须调用相同Application/治理入口。
- API必须支持Cursor、Snapshot/View绑定、Purpose、最小权限、Citation展开、异步Projection任务、CAS Revision、Idempotency、审计与显式Partial Coverage。
- Store/Retriever/Indexer/Consolidator/Policy Adapter可以替换，但不得绕过Memory Owner、Review、Runtime Operation治理或Evidence。
- 原始正文、大型Source和敏感内容使用Ref；公共Envelope只携带有界元数据和Digest。

## 11. 公共/私有Port、Slot/Phase贡献与依赖

### 11.1 Port边界

| 面 | 公共 | 私有/禁止外借 |
|---|---|---|
| Memory领域 | 版本化Candidate、Admission、Query/View、Commit Intent、Inspect、CAS Result、Settlement、Run Requirement | Store事务、Projection Builder、Reranker、Cache实现 |
| Runtime/Application | Operation V3、公开Runtime Port与Application Coordinator接线 | Runtime foundation/fakes/kernel内部实现 |
| Harness | 仅消费Assembler接好且Context Owner已物化的公共对象 | Harness私有`ContextPort`、`ModelTurnPort`、`EventCandidatePort` |
| Model Invoker | 仅通过`RouteID + routegateway/公开execution union`间接消费最终Context | model-invoker/internal、厂商SDK、Raw/Native事件 |

Tool Call只可能成为Action Candidate；model stream/completed/cache usage/Provider状态都只是Observation，不得成为Memory事实、Review Verdict、Timeline或Run终态。

### 11.2 Slot/Phase贡献声明

本组件不定义公共Slot、Hook或Phase枚举，只引用Harness接线定义的namespaced、版本化对象：

- `memory.state`：贡献0..N个只读Memory View/Watermark需求与绑定；
- `context.frame`：仅作为Source贡献版本化Context Candidate，不拥有Frame；
- `context.sources.collect`：`Port`贡献，输入精确View/Query，输出有界Retrieval Result；
- `context.frame.validate`：`Filter`贡献，只验证Memory引用的Scope、Digest、Coverage和可物化性；
- `context.frame.frozen`：`Observer`贡献，只记录已冻结Frame所引用的Memory Ref；
- `cleanup.before`：`Port`贡献，返回Memory参与者的Inspection/Cleanup状态；
- `cleanup.after`、`residual.detected`：`Observer`贡献，只报告Receipt/Residual，不能写Runtime或其他领域Fact。

Phase Contribution能力限定为Observer/Filter/Gate/Port；Memory不提供可任意改Context、联网或写Fact的通用Hook。ContextReference对某Route不能安全物化时，必需内容Fail Closed，可选内容返回Residual/Partial，禁止声称模型已看到。

### 11.3 依赖DAG与装配阻塞

```text
Runtime Operation V3 + Application Coordinator
       + Review/Evidence公共合同
       + Context候选/Reference能力
       + Continuity/Asset精确Ref
                    |
                    v
       Memory Domain Ports/Owner Controller
                    |
                    v
        Memory View/Retrieval Contribution
                    |
                    v
    Agent Assembler -> CompiledGraph -> Harness
```

Agent Assembler最终输出、Assembly SDK/CompiledGraph、Slot/Phase合并规则、Binding V2映射、Checkpoint/Action Gateway和per-turn refresh接线仍是公共装配依赖，由Harness/Assembler方向统一闭合；Memory只声明贡献与需求，不私建替代对象。

## 12. 治理与冲突检查矩阵

| 动作 | Review/Authority/Fence/Budget | Unknown | Settlement/Cleanup/Residual |
|---|---|---|---|
| 本地Query | View绑定的Authority/Policy/Scope；无外部Effect | 不适用；Partial独立表达 | 查询Receipt；无领域Commit |
| 远程Query | 精确披露Payload、Review/Permit和执行点二次Enforcement | Begin后Inspect原远程Attempt | Effect Settlement；超时/覆盖不足为Residual |
| Record Commit/Correction | Candidate、Admission、Review、Expected Revision、双Fence | Inspect原Attempt与Commit Journal | `domain-commits` Settlement；CAS冲突不重试语义 |
| Forget/Purge | Requester Authority、Retention/Legal Hold、每Backend Scope/Budget | 每Backend原Attempt独立Inspect | Tombstone/Purge分开结算，残留副本显式Residual |
| Reindex/View Publish | Snapshot/Projection Digest、Expected Revision、当前Policy | Inspect发布水位 | 旧Projection可保留；Cleanup不改变Record事实 |

冲突检查：Memory不得与Knowledge共用事实Owner；不得把Memory反馈静默晋升Knowledge；不得把Projection当Record；不得让Harness私有Port成为公共Port；不得把Provider Receipt当Fact；不得通过组件Settlement修改Runtime Outcome；不得在ContextReference能力未知时默认可物化。

## 13. Runtime/Harness/Application映射

| 层 | 输入 | 输出 | Memory边界 |
|---|---|---|---|
| Application Coordinator | 领域Intent、Candidate/Query请求 | Operation关联、Inspection/Settlement请求 | 跨域编排，不直写Memory Fact |
| Runtime | Operation Subject、Requirement、Effect/Settlement声明 | Admission、Permit、Begin、Fence与Operation状态 | 线性化治理，不拥有Memory语义 |
| Memory Owner | Candidate/Attempt/当前治理事实 | Admission、Owner Fact、Inspect/CAS、Settlement | 唯一领域事实Owner |
| Context/Assembler | Retrieval Result与SlotContribution | Context Frame/CompiledGraph绑定 | 决定注入；Memory只供候选 |
| Harness | 已装配、已物化Snapshot | Run内有序Observation/Completion Claim | 不查询Memory Store，不提交Memory Fact |

## 14. 性能与实现语言

v1计划全部使用Go：合同校验、状态机、Owner Controller、检索编排、Projection管理与Adapter均不构成已证明的Rust热点。性能通过批量读取、流式Cursor、并发受限的多路召回、内容寻址、结构共享和异步Projection构建获得。

只有满足以下全部条件才提出Rust Delta：真实基准显示单一算法热点消耗显著CPU且Go优化后仍不达已批准目标；输入输出可以用版本化有界Schema隔离；FFI或独立进程失败可被视为Retriever/Indexer失败而不损坏Record事实；Go仍保留Owner、Admission、CAS、Effect和Recovery。当前无此结论。

## 15. 兼容与迁移

- 合同使用SemVer、Schema Ref、Canonical Digest和严格解码；未知必填字段拒绝；扩展走显式Opaque Extension；
- Record ID稳定，Revision严格递增；Projection版本不等于Record版本；
- 后端迁移使用Snapshot + 增量双读校验 + Digest对账 + 切换水位，不双写两个权威Owner；
- Vector模型升级创建新Projection Version，可并存验证后切View；
- Scope、Authority、Sensitivity或Commit语义变化属于不兼容迁移，必须新合同版本或显式迁移任务；
- legacy Runtime `StatePort/MemoryPort`只视为窄Observation骨架，不承载本设计的正式Commit。

## 16. 设计产物与评审门槛

- 详细对象字段与状态机：[Memory合同v1](./contract-v1.md)
- Context owner-current只读Port：[MemoryContextSourceCurrentReaderV1设计Delta](./context-source-port-v1.md)
- Delta 10/11 Owner V2冻结与External接线候选：[Memory Context Refresh Delta 10/11](./context-refresh-neutral-delta10-11-v1.md)
- Effectful Retrieval公共缺口：[Memory Retrieval Domain Gateway Delta](./retrieval-domain-gateway-v1.md)
- 图形设计：[Memory架构图](./architecture.drawio)
- 共同实施计划：`../../plan/memory-knowledge/`

进入Memory-to-Context source Adapter、Application public三阶段Port、production root或远程Retrieval Gateway实现前，仍须完成对应联合评审；该门禁不回退已完成的Memory Reader与Context Owner-local Refresh链。

## 17. Assembler Component Release

[Component Release V1](component-release-v1.md)将Memory/Knowledge公开为两个独立Owner Capability。当前reference与owner-local Store最多形成非生产候选；durable stores、Credential、生产current、Purge/Cleanup、deployment attestation和独立Certification闭合前，production保持NO-GO。
