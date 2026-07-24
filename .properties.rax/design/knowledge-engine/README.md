# Knowledge Engine 可实施设计 v1

- [终局框架设计 V1](./framework-completion-v1.md)

## 1. 状态与依据

- 设计状态：Knowledge backend-neutral framework、V1/V2 Current Reader、Knowledge Adapter、Application三阶段协调、Context `knowledge_reference`及非零reference fixture已实现并完成软件测试；Owner-local与cross-owner reference integration均为**P0=0/P1=0/P2=0**。真实远程Gateway、生产Backend及production G6B/root仍NO-GO。
- 最高业务输入：`tmp.document/Memory&Knowledge.md`。
- 实现位置：`ExecutionRuntime/memory-knowledge/**`；本设计继续作为实现与验收基线。
- 默认语言：Go。v1不规划Rust或特定数据库、向量库、图库、RPC、进程拓扑与SLA。

Knowledge负责“在某个来源、版本、Authority和有效期范围内，有哪些可以查询、引用、验证和撤回的前置认知材料”。它作为反馈前置Knowledge Gateway，输出受治理材料与检索候选；前置语义层仍由Context/Assembler组合Knowledge Snapshot、Memory View和组织Projection。它与Memory共用Ref/View/Retrieval外形，但拥有独立Source、Package、Snapshot、Record、Conflict、Withdraw与Feedback事实。

## 2. 目标与冻结边界

v1闭合：

1. `KnowledgeSource`注册、Acquire/Parse/Normalize/Validate和来源治理；
2. `KnowledgePackage`、`KnowledgeRecord/Claim`、冲突与来源关系；
3. Skill Index、Lexical、Vector和Graph Projection；
4. `KnowledgeSnapshot`构建、校验、发布、撤回和可复现查询；
5. `KnowledgeView`和Domain Projection所需的稳定贡献；
6. Hybrid Retrieval、Citation、Partial Coverage、分页和预算；
7. Candidate、Admission、Review、Operation Effect、Commit/Publish、Receipt、Inspect、CAS与Settlement；
8. Source Refresh、Correction、Conflict、Withdraw、Deprecate、Reindex和反馈候选。

v1明确排除：

- 把模型总结、网页抓取成功、Provider命中或Embedding当成正式Knowledge；
- 复制或取代Asset Manager的原始文件所有权；
- 把多个冲突来源强行合成无来源“真相”；
- 由Knowledge决定Context Frame、Prompt顺序、Token预算或Runtime Outcome；
- 强制所有部署使用同一Vector/Graph Backend；
- 对生产召回率、时延、容量、可用性或外部来源新鲜度作无证据承诺。

## 3. Owner与非Owner

| 语义 | 唯一Owner | Knowledge Engine拥有 | 禁止 |
|---|---|---|---|
| Knowledge Component Manifest/Run Requirement | Knowledge Owner | 发布namespaced能力、版本与依赖声明 | 定义公共Binding、Slot/Phase合并或Run Plan |
| Source/Package/Record/Snapshot/View | Knowledge Owner | 版本、来源、冲突、发布、撤回、Inspect/CAS/Settlement | 接管Asset字节或组织Authority |
| 原始Artifact | Asset Owner | 保存Asset Ref、范围和Digest | 复制成第二正式资产库 |
| Authority/License/Retention Policy | 对应Policy Owner | 消费精确事实并Fail Closed | 自行扩大许可、可见范围或保留期 |
| Review | Review Owner | 提交Candidate并消费Verdict | 把Reviewer Attestation当正式Verdict |
| Operation治理 | Runtime/Application | 提供领域Intent、Reservation、Run Requirement和Settlement | 重造Runtime Permit/Fence/Binding/Policy/Trust |
| Evidence | Evidence Owner | 提供Source/Owner Fact Ref | 用Provider回包伪造authoritative fact |
| Context注入 | Context Owner | 返回Retrieval候选和Citation | 直接把召回结果灌入Harness |
| Continuity | Continuity Owner | 输出Snapshot/Publish/Withdraw Event Ref | 重写Timeline或历史Run |

Knowledge domain/kernel未来只依赖自身合同。不得导入其他组件实现包、Runtime内部实现或Harness私有Port。

## 4. 放置与总体流程

```text
Source Connector / Asset Ref / API / Database / Repository
          |
          v
Acquire -> Parse -> Normalize -> Validate -> Candidate
          |                                  |
          |                                  v
          |                         Admission/Conflict/Review
          |                                  |
          v                                  v
   Source/Package Facts            Governed Commit/Publish
          |                                  |
          +--------> Record + Projection <---+
                              |
                              v
                     Snapshot -> Published View
                              |
                              v
              Hybrid Retrieval -> Citation -> Context Candidate
```

Controller与正式State位于Sandbox外State Plane。Connector、Indexer和Retriever可以本地或远程；任何网络、披露、费用、远程持久状态或Provider保留都必须经过Runtime Operation V3治理。Provider回包只作Observation/Receipt，Knowledge Owner独立Inspect并CAS。

## 5. Source、Package、Record与Snapshot

### 5.1 Source

`KnowledgeSource`至少绑定Source ID/Revision、Owner、Authority、License、Asset/Connector Ref、Source Version、AcquiredAt、Valid Time、Content Digest、Sensitivity、Retention、Current State与Provenance。

Source正文不由Knowledge复制成为正式资产；解析结果可以存储，但必须保留Asset Ref、范围和Content Digest。Source更新创建新Revision，不原地覆盖。

### 5.2 Package和Record

- `KnowledgePackage`是一组可独立验证、索引、撤回和发布的材料；
- `KnowledgeRecord/Claim`保存内容Ref、来源、支持Evidence、可信状态、有效期、Conflict Group与Withdraw关系；
- 单个Embedding、Graph Edge或Reranker Score不是Record；
- 多来源冲突必须保留多个Claim和来源；Current View按Policy呈现“冲突”，而不是静默选真。

### 5.3 Snapshot

Snapshot冻结Source/Package/Record集合、各自Revision/Digest、Projection版本、Coverage、Policy、Authority水位和Manifest Digest。

```text
building -> partial | ready | failed
partial -> building | ready | deprecated
ready -> published | deprecated
published -> deprecated | withdrawn
deprecated -> withdrawn
```

旧Snapshot继续服务已精确绑定它的历史Run；新Run不得把已Withdraw Source当作当前事实。紧急撤回可使新查询Fail Closed，同时历史Evidence仍能说明当时使用的版本。

## 6. Candidate、Admission与正式发布

Candidate来源包括Connector、人工维护、Tool、Continuity、Memory经验提议和模型建议。所有来源只能先产生Candidate。

Admission检查：Schema、Source/License、Authority、Scope、Sensitivity、Duplicate、实体对齐、Conflict、Poisoning、Policy、Target Package/Record/Snapshot Revision和Projection兼容性。

Decision闭集：`rejected`、`merged`、`conflict_pending`、`review_required`、`commit_ready`。Admission不是正式知识结论。

### 6.1 外部动作顺序

正式Record Commit、Snapshot Publish、Withdraw、远程Acquire、远程Query与远程Reindex严格复用Runtime live链：

```text
领域Intent/Reservation
 -> Operation Admission
 -> Permit
 -> Begin
 -> Delegation/Prepare
 -> Enforcement
 -> Provider/Resolver Execute或Inspect原Attempt
 -> Observation
 -> Knowledge Owner Inspect形成DomainResultFact
 -> Evidence Owner追加绑定该DomainResultFact的Evidence
 -> Runtime Operation Settlement Owner提交Operation Settlement
 -> Knowledge ApplySettlement CAS形成领域settled/result_ready投影
```

组件不重造Runtime Gateway。Begin之后任何丢回包只能Inspect原Attempt；不得新建Attempt盲目重派。Provider执行结果先是Observation；Knowledge Owner独立Inspect后，在实际CAS点完成第二次门禁并形成Owner Fact。该执行点必须重读当前Identity、Binding、Authority、Review、Budget、Policy、Credential、Scope和Fence。

### 6.2 Pre-run Evidence

本设计**触发条件性pre-run Evidence**：Source注册/撤回、Snapshot Publish、组织Knowledge纠错等管理型Operation可以在没有活跃Run时发生。时点固定为：

```text
Provider Observation
 -> Knowledge Owner Inspect并CAS形成Owner Fact
 -> DomainResultFact
 -> 追加绑定OperationSubject与该DomainResultFact的authoritative Evidence
 -> Runtime Operation Settlement
 -> Knowledge ApplySettlement CAS
```

不得合成假Run。当前Evidence v2的current projection校验要求活跃Run，因此需要Runtime Owner评审`OperationScope-aware Evidence` Delta；该管理型Delta不是Runtime总阻塞，但也不授权Run内或Run外远程Retrieval。Tool G6A与Checkpoint V5均不适用，Retrieval仍等待专用加法版本。

## 7. Effect清单与Conflict Domain

| Effect kind | 触发动作 | 默认关系 |
|---|---|---|
| `praxis.knowledge/source-acquire` | 网络、数据库、仓库或API读取 | data disclosure/cost/remote residual按Policy |
| `praxis.knowledge/source-register` | 正式Source Fact提交 | formal commit |
| `praxis.knowledge/record-commit` | Knowledge Record/Claim提交或纠错 | formal commit |
| `praxis.knowledge/snapshot-publish` | 发布新Snapshot/View水位 | formal commit |
| `praxis.knowledge/source-withdraw` | 撤回Source及Current View | formal commit + cleanup |
| `praxis.knowledge/remote-query` | 向远程检索服务披露Query/标识 | disclosure/cost |
| `praxis.knowledge/remote-index-build` | 远程Embedding/Graph/Indexer | hosted execution/disclosure/cost |
| `praxis.knowledge/reindex-publish` | 切换Projection版本 | formal commit |
| `praxis.knowledge/purge` | 物理删除副本/索引 | external mutation + cleanup |

本地纯读、Candidate持久化、Admission、基于权威Record的本地Projection计算不创建Runtime Effect，但仍须授权、审计和有界资源策略。

Runtime Conflict Domain使用tenant-stable namespaced基类：`praxis.knowledge/source`、`package`、`snapshot`、`record`、`projection`。具体Source/Record/Snapshot的排他性由Payload中的Target Digest、稳定Idempotency与Knowledge Store CAS共同实现；不得把Runtime Conflict Domain缩窄到易随Run/Instance变化的Scope。

## 8. Projection与Hybrid Retrieval

### 8.1 Projection

- Skill Index：精确流程、关键词、UseWhen/DoNotUseWhen和Detail Ref；
- Lexical：全文、字段、代码符号和精确短语；
- Vector：语义候选发现；每个Embedding绑定Record/Revision/Chunk/Model/Dimension/Index Version/Content Digest；
- Graph：Entity、Concept、Artifact、Claim、Source与关系；Node/Edge保存Owner、Scope、Revision、Source、Confidence、Valid/Transaction Time。

Projection可替换、重建和迁移；Record与Source是权威内容。Graph不取代全文，Vector不取代Citation。

### 8.2 Query

```text
解析Intent/Scope/时间/实体/Evidence类型/预算
 -> Resolve KnowledgeView与Snapshot
 -> 在取正文前做Authority/Scope/License/Sensitivity过滤
 -> 路由Skill/Lexical/Vector/Graph
 -> 去重、来源聚合、冲突展开、当前性验证
 -> Rerank和预算截断
 -> RetrievalResult/Citation/Coverage
```

远程Query遵守完整Operation Effect链。本地Query也必须绑定Purpose、View、Snapshot、Policy Revision和Authority。Partial Coverage、Projection stale、Source withdrawn和ContextReference未能物化都必须显式返回Residual或Fail Closed。

Model Invoker Route只通过`RouteID + routegateway/公开execution union`消费最终Context；Knowledge不得依赖model-invoker/internal、厂商SDK或Raw/Native事件。Model stream/completed/cache usage/Provider状态都只是Observation，不能成为Knowledge事实。

## 9. Context、Projection与引用边界

Knowledge返回的`RetrievalResult`包含精确Snapshot/View水位、Record/Claim Revision、Source/Asset范围、Citation、Conflict、Coverage、Representation与Evidence。它不自动成为Context Frame。

Context Engine负责Domain Projection组合、Context Admission、排序、展开、压缩、Channel和预算。由于ContextReference物化尚非所有Route闭合：

- 能安全物化且Digest一致时返回有界内容；
- Route不支持受治理Reference Resolver时，必需内容Fail Closed；
- 可选内容可以显式Residual/Partial，且不能声称模型已看见；
- Harness私有ContextPort只消费已物化Snapshot，不直接查询Knowledge或发起远程Effect。

### 9.1 Knowledge Owner独立Inspect到Context物化桥

Knowledge不得与Memory合并Owner、Port、Contribution或预算域。Knowledge侧面向Context的纯只读能力命名为`KnowledgeContextSourceCurrentReaderV1`：

完整字段、canonical、S1/S2、Unknown与Checkpoint分离合同见：[Knowledge Current Reader设计Delta](./context-source-port-v1.md)。Knowledge Reader/Adapter、Application三阶段Port与Context `knowledge_reference`/Refresh均已YES；reference fixture的Knowledge来源数为1。远程Gateway与production G6B/root仍NO-GO。

```text
InspectAttempt(localOwnerAttemptRef) -> LocalAttemptInspectionV1
InspectForTurn(request) -> KnowledgeContributionCurrentProjectionV1
ReadContentExact(localOwnerContentRef) -> LocalExactContentObservationV1
```

Current Reader只接受已由Knowledge Owner持久化且完成DomainResult/Runtime Settlement/ApplySettlement闭表的Observation/Result exact refs，不发起Retrieval，也不返回Candidate或Frame。`InspectForTurn`请求绑定Contract Version、Execution Scope Digest、Run/Turn、原Retrieval Attempt、Observation/Result、Expected Query/View/Watermark/Published Snapshot、Authority/Policy/Purpose、独立MaxItems/Bytes/Tokens与单项上限、Token Estimator、Inspect时间、Frame Deadline和Idempotency。

返回绑定固定Owner=`knowledge`、Contribution/Observation/Result、Query/View/Watermark/Published Snapshot、Coverage/NextCursor/Result/Evidence Digest、Observed/Expires/Residual及有序Items。每个Item必须包含Rank/Score、Record/Content、DomainResult/Association/SettlementApplication、Package/Snapshot、Source/Evidence/Projection/Citation、License/Trust/Conflict、Token Estimate和全部TTL。

Knowledge Owner必须复读当前Published Snapshot pointer，并依次验证Snapshot、Package、Record、Source、Projection、License、Authority、Policy、Scope、Sensitivity和TTL；同时重算DomainResult canonical digest，验证Association精确绑定相同ID/revision/digest、Owner、Subject Ref、CASAfter及SettlementApplication。Context只能通过Knowledge Port请求该判断，不能直接读取Knowledge Store或把Provider命中升级为事实。

Frame Assembler只读取Knowledge Owner State Plane本地可exact证明的正文，重算Length/MediaType/Digest后投影到Context Reference Store。S1只允许Context形成不可见pending DomainResult；S2 fresh current/TTL/License通过后，Context Owner才在单个本地原子边界执行ApplySettlement与Generation current CAS，S2失败不得发布current。远程Query、Resolver或正文读取移出Current Reader，必须经Application与Runtime治理的Retrieval Domain Gateway携带Operation/Attempt/Permit、prepare+execute Enforcement执行；lost reply只调用Gateway的`InspectOriginalAttempt`。

当前G6A closed matrix仅覆盖Tool，Checkpoint V5不得套用给Retrieval。Retrieval-specific additive Applicability/Evidence/Settlement版本由相应Owner联合冻结前，Knowledge远程Gateway全部`unsupported`且Provider/Resolver调用数为0；本地Current Reader不受影响。

### 9.2 排序、预算、压缩与Knowledge类型门禁

- Owner内固定`Score降序 -> Record ID升序 -> Revision降序 -> Digest升序`；预算按exact bytes与固定Estimator计算；
- Memory/Knowledge不跨Owner比较Score、借用预算或自动去重；Context Recipe是Owner间位置、区域和总预算的唯一Owner；
- 每个Owner只形成一个canonical Contribution Bundle以保持内部rank和Citation；已选项在freeze前漂移时整个Frame attempt失败，不在原attempt补位；
- `now >= Expires`、Published Snapshot pointer变化、Source withdraw、Package/Projection stale、License/Policy缩小、内容或Association摘要不一致均Fail Closed；
- 压缩summary不成为Knowledge事实；缺少Retained exact anchor必须rematerialize，anchor仍在但Owner current事实漂移仍拒绝复用；
- 本模块侧仅提出加法候选`knowledge_reference`；live Context合同尚未接受该fragment kind，现有Candidate也没有绑定完整Knowledge exact source chain。在Context Owner发布名义类型、canonical与验收合同前，Knowledge Context注入为NO-GO，不得冒充`memory_recall`、Artifact或Instruction。

## 10. 纠错、冲突、撤回与反馈

- Correction创建新Record/Claim Revision及`corrects/supersedes`关系；
- Conflict保留各来源和Claim，不覆盖；来源撤回后重算受其支持的可信状态；
- Withdraw使新View不再返回当前内容；历史Run保留当时Snapshot/Citation Evidence；
- Deprecate表示仍可历史读取但不再为新Plan默认选择；
- Purge是独立Effect，Legal Hold可阻止物理删除并返回Residual；
- `RetrievalFeedbackCandidate`绑定Query、Result、Citation、Context Frame、Outcome和Evidence，只进入Evaluation或新Candidate，不直接改Record、权重、可信状态或Snapshot。

## 11. RunStart、OperationScope与RunSettlement Requirement

Knowledge向Assembler声明而不自行认证：

1. `RunStartRequirement`：精确`KnowledgeView + Snapshot + PackageSetDigest + ProjectionCoverage + Authority/Policy TTL + ContextReference能力`；
2. Query-only Run：不产生RunSettlementRequirement；远程Query只进入精确OperationScope及Runtime Effect结算；
3. Run内开始正式Knowledge写入时：贡献唯一Owner的`praxis.knowledge/domain-commits` RunSettlementRequirement参与者，要求所有Begin后的Attempt在Run收口前已Settlement或按Runtime Plan显式进入indeterminate/reconciliation；
4. 无Run管理Operation：使用admin/custom OperationScope，不伪造RunStart或RunSettlement Requirement。

Knowledge只发布分型Requirement declaration和Participant Inspection；Trusted Assembler按ID/Owner/Subject/Phase认证，不创建Run Plan、不认证Binding、不选择Runtime Outcome。

## 12. SDK、CLI、API与扩展

- SDK：RegisterSource、Acquire、BuildPackage、SubmitCandidate、RequestAdmission、RequestCommit、BuildSnapshot、PublishView、Query、InspectSource/Record/Snapshot、Withdraw、WatchChanges、Reindex；
- CLI：`praxis knowledge source list|inspect|withdraw`、`snapshot build|publish`、`knowledge query`、`index status|rebuild`；
- API：混合检索、分页/Cursor、Snapshot绑定、权限过滤、Citation展开、异步Index任务、CAS Revision、幂等、审计和Partial Coverage；
- Connector/Retriever/Indexer/Store/Policy Adapter可替换，但不能绕过Knowledge Owner或Runtime治理；
- SDK/CLI/API都是Application客户端，不得直连Fact Store或Runtime raw Fact Port。

## 13. 公共/私有Port、Slot/Phase贡献与依赖

### 13.1 Port边界

| 面 | 公共 | 私有/禁止外借 |
|---|---|---|
| Knowledge领域 | Source/Package/Snapshot/View、Candidate/Admission、Query/Citation、Commit Intent、Inspect/CAS Result、Settlement、Run Requirement | Connector事务、Parser、Projection Builder、Reranker、Cache实现 |
| Runtime/Application | Operation V3、公开Runtime Port与Application Coordinator接线 | Runtime foundation/fakes/kernel内部实现 |
| Harness | 仅消费Assembler接好且Context Owner已物化的公共对象 | Harness私有`ContextPort`、`ModelTurnPort`、`EventCandidatePort` |
| Model Invoker | 仅通过`RouteID + routegateway/公开execution union`间接消费最终Context | model-invoker/internal、厂商SDK、Raw/Native事件 |

Tool Call只可能成为Action Candidate；model stream/completed/cache usage/Provider状态都只是Observation，不能成为Knowledge Fact、Review Verdict、Cache命中、Timeline或Run终态。

### 13.2 Slot/Phase贡献声明

本组件不定义公共Slot、Hook或Phase枚举，只引用Harness接线定义的namespaced、版本化对象：

- `knowledge.query`：贡献0..N个Knowledge View/Snapshot/Query需求与绑定；
- `context.frame`：仅作为Source贡献有引用与Coverage的Context Candidate，不拥有Frame；
- `context.sources.collect`：`Port`贡献，输入精确View/Snapshot/Query，输出有界Retrieval Result；
- `context.frame.validate`：`Filter`贡献，只验证Citation、License、Scope、Digest、Coverage与可物化性；
- `context.frame.frozen`：`Observer`贡献，只记录冻结Frame引用的Knowledge Ref；
- `cleanup.before`：`Port`贡献，返回Knowledge参与者的Inspection/Cleanup状态；
- `cleanup.after`、`residual.detected`：`Observer`贡献，只报告Receipt/Residual。

Phase Contribution能力限定为Observer/Filter/Gate/Port；Knowledge不提供能任意改Context、联网或写Fact的通用Hook。

### 13.3 依赖DAG与装配阻塞

```text
Asset/Continuity精确Ref
       + Runtime Operation V3/Application Coordinator
       + Review/Evidence公共合同
       + Context候选/Reference能力
                    |
                    v
       Knowledge Domain Ports/Owner Controller
                    |
                    v
      Snapshot/View/Retrieval Contribution
                    |
                    v
    Agent Assembler -> CompiledGraph -> Harness
```

Agent Assembler最终输出、Assembly SDK/CompiledGraph、Slot/Phase合并规则、Binding V2映射、Checkpoint/Action Gateway和per-turn refresh接线仍是公共装配依赖，由Harness/Assembler方向统一闭合；Knowledge只声明贡献与需求，不私建替代对象。

## 14. 治理、恢复与冲突检查矩阵

| 动作 | Review/Authority/Fence/Budget | Unknown | Settlement/Cleanup/Residual |
|---|---|---|---|
| 本地Query | View/Snapshot绑定的Authority、License、Scope；无外部Effect | 不适用；Partial独立表达 | 查询Receipt；无领域Commit |
| 远程Acquire/Query | 精确披露Payload、Review/Permit和执行点二次Enforcement | Begin后Inspect原Provider Attempt | Effect Settlement；连接器/覆盖Residual显式保留 |
| Record Commit/Correction | Candidate、Admission、Review、Expected Revision、双Fence | Inspect原Attempt与Owner Journal | `domain-commits` Settlement；冲突不盲重派 |
| Snapshot/Projection Publish | Manifest/Projection Digest、Expected Revision、当前Policy | Inspect当前发布水位 | 旧版本可历史读取；Cleanup不改Record事实 |
| Withdraw/Purge | Authority、License/Retention/Legal Hold、每Backend Scope/Budget | 每Backend原Attempt独立Inspect | Current View、物理删除、残留副本分开结算 |

冲突检查：Knowledge不得接管Asset字节；不得将Memory经验静默晋升为Knowledge；不得把Projection/Provider命中当Record；不得让Harness私有Port成为公共Port；不得让组件Settlement修改Runtime Outcome；不得在ContextReference能力未知时默认可物化；不得无来源消解冲突Claim。

## 15. Runtime/Harness/Application映射

| 层 | 输入 | 输出 | Knowledge边界 |
|---|---|---|---|
| Application Coordinator | 领域Intent、Source/Query请求 | Operation关联、Inspection/Settlement请求 | 跨域编排，不直写Knowledge Fact |
| Runtime | Operation Subject、Requirement、Effect/Settlement声明 | Admission、Permit、Begin、Fence与Operation状态 | 线性化治理，不拥有Knowledge语义 |
| Knowledge Owner | Candidate/Attempt/当前治理事实 | Admission、Owner Fact、Inspect/CAS、Settlement | 唯一领域事实Owner |
| Context/Assembler | Retrieval Result、Citation与SlotContribution | Context Frame/CompiledGraph绑定 | 决定注入；Knowledge只供候选 |
| Harness | 已装配、已物化Snapshot | Run内有序Observation/Completion Claim | 不查询Knowledge Store，不提交Knowledge Fact |

## 16. 兼容、迁移与性能

- Source/Record/Snapshot/View分别版本化；Snapshot发布不可变；
- 后端迁移通过Snapshot导出、增量对账、双读校验和水位切换，不形成两个权威Owner；
- Parser、Chunker、Embedding、Graph Schema或Reranker变化创建新Projection Version；
- 旧Snapshot绑定旧Projection时继续可复现，除非Authority或撤回Policy要求Fail Closed；
- v1全部使用Go。只有真实基准证明计算热点且可以保持Go Owner/CAS/Effect边界时才提出Rust；Rust失败只能导致Projection partial/failed，不能损坏Source/Record/Snapshot事实。

## 17. 设计入口

- 详细对象字段与不变量：[Knowledge合同v1](./contract-v1.md)
- Context owner-current只读Port：[KnowledgeContextSourceCurrentReaderV1设计Delta](./context-source-port-v1.md)
- Delta 10/11现有Reader V2与fragment kind候选：[Knowledge Context Refresh Delta 10/11](./context-refresh-neutral-delta10-11-v1.md)
- Effectful Retrieval公共缺口：[Knowledge Retrieval Domain Gateway Delta](./retrieval-domain-gateway-v1.md)
- 图形设计：[Knowledge架构图](./architecture.drawio)
- 共同实施计划：`../../plan/memory-knowledge/`

进入Knowledge-to-Context source Adapter、Application public三阶段Port、production root或远程Retrieval Gateway实现前，仍须通过对应联合评审；该门禁不回退已完成的Knowledge Reader与Context Owner-local Refresh链。

## 18. Assembler Component Release

Knowledge与Memory共用组件发布边界，但各自拥有独立Capability/Port/Factory，详见[Memory/Knowledge Component Release V1](../memory-engine/component-release-v1.md)。reference fixture、内存Store和owner-local Reader测试不能成为production证明。
