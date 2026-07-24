# Memory + Knowledge v1 可实施计划

> Current truth：**Owner-local与cross-owner reference integration均P0=0/P1=0/P2=0**。Owner设计合同、V2 Reader、Application三阶段、Context proof/`knowledge_reference`、双Adapter与非零fixture均已实现；production root、远程Effect及生产Backend仍NO-GO。

## 1. 目标与不变量

在`ExecutionRuntime/memory-knowledge/**`内形成两个独立领域Owner、一套共享引用/检索外形的最小完整闭环：Memory Candidate到正式Record，Knowledge Source/Candidate到正式Record/Snapshot，以及二者向Context提供可验证候选。

全阶段不变量：

1. 模型、Provider、Connector与Harness只能产生Observation/Candidate；正式事实由Memory或Knowledge Owner独立Inspect并CAS；
2. 外部动作顺序固定为`Intent/Reservation → Admission → Permit → Begin → Delegation/Prepare → Enforcement → Execute/Inspect → Observation → Owner DomainResultFact → Evidence → Runtime Operation Settlement → Owner ApplySettlement`；
3. Begin后丢回包只Inspect原Attempt；Unknown不得盲重派；
4. 组件不写Runtime Outcome/Binding/Policy/Trust、Review Verdict、Evidence Ledger内部事实或其他领域Owner事实；
5. Harness私有Port不是公共Port；组件不定义公共Slot/Phase枚举；
6. 远程检索、写入、删除、外发、Provider保留、远程构建索引都是Effect；
7. ContextReference不能安全物化时Fail Closed或返回Residual；
8. State Plane保存权威事实；Sandbox本地盘、Cache和Projection不能成为唯一副本。

## 2. 依赖与实施DAG

```text
P0 合同冻结
  ├─ Memory/Knowledge领域合同
  ├─ Port Delta Owner裁决
  └─ Assembler/CompiledGraph/Slot/Phase/Binding接线合同
            |
            v
P1 Go合同 + Canonical编码 + 状态机
            |
            v
P2 Memory/Knowledge Owner Controller + Inspect/CAS
            |
            +----> P3 Projection/Retrieval/Citation
            |                |
            |                v
            |       P3.1 Owner Inspect -> Context Refresh合同冻结（当前NO-GO）
            |                |
            +----------------+----> P4 Runtime Operation Adapter/Application映射
                           |
                           v
                  P5 Settlement/Cleanup/Residual
                           |
                           v
                  P6 Conformance/集成/系统验收
```

外部依赖只走版本化公共Port：Runtime Operation V3、Application Coordinator、Review、Evidence、Context、Continuity、Asset、Assembler/CompiledGraph与Harness接线。禁止依赖Runtime foundation/fakes/kernel内部、Harness kernel/fakes/internal、model-invoker/internal或其他组件实现包。

## 3. 阶段与文件级落点

以下路径均是**获批后的计划落点**，本轮不创建。

### P0：联合合同冻结

落点：当前design/plan资产，不写实现。

完成项：Owner/非Owner、对象字段、状态机、Effect kind、Conflict Domain、Run Requirement、pre-run Evidence、Slot/Phase贡献、Unknown与迁移。

退出标准：`port-delta.md`逐项得到Owner的accept/revise/reject结论；公共装配对象有稳定版本；管理线裁决Retention/Legal Hold与组织级Knowledge Authority最小语义。

### P1：领域合同与确定性基础

计划文件：

```text
ExecutionRuntime/memory-knowledge/
  go.mod
  README.md
  contract/
    envelope.go
    canonical.go
    scope.go
    memory.go
    knowledge.go
    retrieval.go
    effect.go
    settlement.go
    errors.go
  contracttest/
    canonical_test.go
    strictdecode_test.go
    compatibility_test.go
```

交付：版本化有界对象、Canonical Digest、严格解码、稳定错误、CAS/幂等键和合同兼容测试。不得导入Runtime/Harness实现包。

退出标准：同对象跨进程/顺序的Digest稳定；重复键、未知治理字段、尾随文档、越界集合Fail Closed；v1 golden fixtures可复现。

### P2：领域Owner与权威Fact状态机

计划文件：

```text
ports/
  memory.go
  knowledge.go
  factstore.go
  projection.go
  governance.go
  clock.go
memory/
  controller.go
  admission.go
  commit.go
  lifecycle.go
  inspect.go
knowledge/
  controller.go
  source.go
  admission.go
  commit.go
  snapshot.go
  inspect.go
```

交付：Memory Candidate/Admission/Record/Correction/Forget与Knowledge Source/Package/Candidate/Record/Snapshot/Withdraw状态机；Owner Inspect、Expected Revision CAS、Receipt、DomainResultFact和ApplySettlement投影。

退出标准：任何正式事实只能从精确Candidate+Admission+Review/Governance引用产生；并发提交只有一个CAS获胜；Provider Receipt不能触发事实升级；状态迁移非法时无写入。

### P3：Projection、检索与Context候选

计划文件：

```text
retrieval/
  query.go
  planner.go
  merge.go
  citation.go
  coverage.go
projection/
  manager.go
  skill.go
  lexical.go
  vector.go
  graph.go
```

交付：Memory/Knowledge View解析、Skill/Lexical/Vector/Graph可重建Projection、混合候选、冲突分组、Citation、Cursor、Coverage和可持久Inspect的Retrieval Observation/Result refs。Retrieval不得直接形成Context Candidate或Frame。

退出标准：取正文前完成Authority/Scope/License/Sensitivity过滤；Snapshot/View/Projection水位稳定；Partial/withdrawn/unsupported reference不能伪装完整命中；Projection损坏不改权威Fact。

v1只提供Go编排与接口，测试Backend位于testkit。不得将Fake宣称为生产Backend或SLA证明。

### P3.1：per-turn Context Refresh桥（reference接线已完成；production root与远程Gateway NO-GO）

四组Port Delta保持边界分离：Memory与Knowledge各自的V1/V2 Current Reader、Application三阶段公共Port、Memory/Knowledge adapters、Context TransitionProof/`knowledge_reference`、atomic Apply+Generation CAS与nonzero reference fixture均已YES；effectful Retrieval仍只存在于两个Gateway Delta且未实现。未闭合的是production G6B/root与真实远程路径。

reference integration来源基线为：

```text
MemorySources    = 1
KnowledgeSources = 1
```

该基线只存在于真实V2 Owner Reader组成的reference fixture；production接线仍不得在未装配同一批获批ports时启用来源，也不得调用两个Retrieval Gateway。Reader/Adapter YES不授权Gateway或生产Backend。

已完成的Owner-local Reader落点为：

```text
ExecutionRuntime/memory-knowledge/memory/contextsource/**
ExecutionRuntime/memory-knowledge/knowledge/contextsource/**
```

远程Retrieval Gateway尚未授权实现，不预建实现目录或私有兼容接口。

跨Owner落点已由各自Owner发布，最小调用序列：

```text
reference fixture：MemorySources=1且KnowledgeSources=1

Application协调pre-frame request；Harness Adapter无损传递committed PendingAction的Session/Turn exact证据
 -> 校验SourceTurnRef/ordinal == Tool.Execution.Turn == ExpectedCurrent.Turn
 -> Application只协调已经settled的本地Owner Observation/Result exact refs
 -> 若未来确需远程Retrieval，另行要求Gateway及retrieval-specific Applicability/Evidence/Settlement版本全部YES；当前仍unsupported且Provider/Resolver=0
 -> Context Assembler调用CurrentReader.InspectAttempt(persisted_and_settled)
 -> 两Owner Reader只验证/回显SourceTurnOrdinal与具名Owner原始exact证据，不补造
 -> S1 CurrentReader.InspectForTurn并只从本地Owner State Plane ReadContentExact
 -> 各Owner一个有序Contribution Bundle
 -> Context Admission/预算/Compile seal pending DomainResult/Manifest/Frame/Generation（不可见、非current）
 -> Context Owner seal final TransitionProof，绑定pre-frame request、Target=T+1、childExecution、新Frame/Generation exact refs
 -> S2 fresh复读两个Owner；fresh Projection/Observation refs可不同，stable ClosureDigest与ordered exact集合必须相同
 -> Context Owner单个本地原子边界ApplySettlement + Generation current CAS
 -> 仅原子成功后InspectFrame并发布exact Frame/Generation current ref
 -> Route精确物化
 -> Harness消费Frame
```

排序预算：Owner内`Score desc -> Record ID asc -> Revision desc -> Digest asc`；Memory/Knowledge预算不可互借，Score不可跨Owner比较；exact bytes和版本化Estimator决定token；Context Recipe拥有最终区域/总预算。S2失败、TTL不足或原子CAS冲突必须使当前Frame attempt失败，pending DomainResult不得发布current；不得原attempt低位补选或混合新旧Owner结果。

reference退出标准已经满足：Context Owner接受`knowledge_reference`；Application三阶段Port、两Owner adapters、nonzero cardinality、结构化Frame ref、lost-reply Inspect与exact Frame fixture闭合。production root与Route实际装配仍是独立系统门，不能由本reference fixture代替。

Checkpoint/Restore不属于P3.1或G6B V1。未来只在Runtime CheckpointAttempt/Participant V2、Continuity Manifest和Restore治理分别冻结后，另行规划`MemoryCheckpointReaderV1/MemoryRestoreCompatibilityReaderV1`与`KnowledgeCheckpointReaderV1/KnowledgeRestoreCompatibilityReaderV1`；不得把Current Reader或Retrieval Gateway扩权为Checkpoint Participant。

### P4：Runtime Operation与Application公共映射

前置：Port Delta 3/4/5及公共装配合同获批。

计划文件：

```text
runtimeadapter/
  manifest.go
  operation.go
  requirements.go
  settlement.go
  inspection.go
assembly/
  contributions.go
```

`runtimeadapter`只能依赖runtime/core与runtime/ports公开包。`assembly/contributions.go`只编码已由Harness/Assembler Owner定义的namespaced、版本化`SlotContribution/PortSpec/PortBinding/PhaseContribution/DependencySpec`，不得自建Slot/Phase枚举。

Application侧Adapter、Assembler实现和Harness装配由各自Owner在其范围落地；本组件只公开领域Port和贡献声明。

退出标准：动作顺序和每一步引用可审计；实际执行点二次Enforcement；Begin后只Inspect原Attempt；Settlement不写Runtime Outcome；Context/Harness只能消费已装配对象。

### P5：Cleanup、Residual、纠错/遗忘/撤回

计划文件：

```text
memory/forget.go
memory/purge.go
knowledge/withdraw.go
knowledge/purge.go
recovery/reconciler.go
recovery/cleanup.go
recovery/residual.go
```

交付：Memory Correction/Supersede/Tombstone/Purge；Knowledge Correction/Conflict/Withdraw/Deprecate/Purge；原Attempt恢复、非递归Inspect、Residual与Cleanup分离。

退出标准：Forget不等于Purge；Legal Hold拒绝不会声称已删除；撤回后新View不可见但历史Citation可解释；远程副本/Projection残留逐项报告。

### P6：SDK/CLI/API与验收

第一实现闭环只冻结Go Application Client接口；CLI/API服务壳不在本组件首批范围，避免单方决定认证、部署和进程拓扑。获管理线单独授权后，计划落点可以是：

```text
sdk/go/
  memory.go
  knowledge.go
  query.go
cmd/praxis-memory-knowledge/
  main.go
```

所有写命令只提交Intent，不直连Fact Store或Runtime raw Fact Port。CLI/API必须复用相同Application入口。

退出标准：完成`acceptance-test-matrix.md`全部适用项；普通、race与vet分别有真实命令记录；集成环境只能声明已验证能力，不能外推生产SLA。

## 4. RunStart、OperationScope、RunSettlement与公共装配

Memory贡献`memory.state`，Knowledge贡献`knowledge.query`；二者只作为`context.frame` Source。Phase只使用接线文档已有的`context.sources.collect` Port、`context.frame.validate` Filter、`context.frame.frozen` Observer、`cleanup.before` Port、`cleanup.after/residual.detected` Observer。

三类对象必须分型：

- Memory `RunStartRequirement`：View、Record/Snapshot Watermark、Projection Coverage、Authority/Policy TTL、ContextReference能力；
- Knowledge `RunStartRequirement`：View、Snapshot、PackageSet Digest、Projection Coverage、Authority/Policy TTL、ContextReference能力；
- 本地只读查询不增加RunSettlementRequirement；远程Retrieval当前unsupported，未来只有在retrieval-specific Applicability/Evidence/Settlement版本冻结后才按专用OperationScope与Settlement合同结算；
- Run内正式写入分别声明`praxis.memory/domain-commits`与`praxis.knowledge/domain-commits`参与者；无Run管理Operation使用admin/custom OperationScope。

公共装配硬依赖：Agent Assembler最终输出、Assembly SDK/CompiledGraph、Slot/Phase合并、Binding V2映射、Checkpoint/Action Gateway、per-turn refresh。Memory/Knowledge不私建替代品。

## 5. Pre-run Evidence与Effect总表

本计划触发条件性pre-run Evidence：无Run的Memory管理员Correction/Forget/Purge/Scope变更，以及Knowledge Source注册/撤回、Snapshot发布、组织纠错。统一顺序为`Observation → Owner DomainResultFact → Evidence → Runtime Operation Settlement → Owner ApplySettlement`；权威Evidence绑定Operation Subject与精确DomainResultFact，不得合成Run。

此处已批准或待审的管理型Evidence合同不自动覆盖远程Retrieval。当前G6A closed matrix只有Tool，Checkpoint V5也不得套用。Memory/Knowledge远程Query、Resolver与正文读取必须等待对应Owner联合冻结retrieval-specific additive Applicability/Evidence/Settlement版本；在此之前全部Gateway `unsupported`、Provider/Resolver调用数为0，本地Current Reader照常可评审。

全部Effect kind：

- Memory：`record-commit`、`forget`、`purge`、`remote-query`、`remote-index-build`、`reindex-publish`、`view-publish`；
- Knowledge：`source-acquire`、`source-register`、`record-commit`、`snapshot-publish`、`source-withdraw`、`remote-query`、`remote-index-build`、`reindex-publish`、`purge`。

完整输入、门禁、恢复和Settlement见两份设计中的矩阵。任何新增Effect kind都必须先修订设计并联合评审。

## 6. 兼容与迁移

1. 合同SemVer；未知治理字段Fail Closed；大内容用Ref；
2. Record ID稳定、Revision单调；Snapshot发布不可变；Projection版本独立；
3. Backend迁移采用Snapshot导出、增量对账、双读Digest校验和单水位切换，不双写两个权威Owner；
4. Parser/Chunker/Embedding/Graph/Reranker变化创建新Projection版本；
5. Runtime窄`MemoryPort/KnowledgePort/StatePort`保留兼容，只作为Observation骨架，不伪装完整Commit；
6. 旧Cursor绑定旧View/Snapshot/Projection水位，水位漂移后显式失效；
7. Rust仅在真实Go基准确认隔离热点后另开Delta；Go保留Owner/CAS/Effect/Recovery，FFI或子进程失败只能使Projection partial/failed。

## 7. 管理线需裁决

1. Memory组织/团队共享与主观冲突的默认Policy；
2. Forget后的最小审计字段、Legal Hold优先级与Purge完成定义；
3. Knowledge Source的Authority、License、组织级“权威”与紧急撤回策略；
4. 是否批准无Run管理Operation以及对应Operation Review和pre-run Evidence Delta；
5. CLI/API是否进入首批交付，及其认证/部署Owner；
6. 性能目标和生产Backend必须由后续容量/部署评审确定，v1不预设。
7. Context Owner接受加法`knowledge_reference`或给出等价精确kind；未裁决前Knowledge注入Fail Closed，禁止冒充`memory_recall`或Artifact。
8. exact Turn方向为`SourceTurnRef/Ordinal == Tool.Execution.Turn == ExpectedCurrent.Turn`；V2 `SourceTurnRef`是`contract.Ref`，只来自Harness committed PendingAction current并经Application Port无损传递。任何Owner不得从uint32/字符串补造。
9. Context是final TransitionProof唯一Owner；Application只协调pre-frame/apply/inspect。顺序固定为pending outputs seal→proof seal→S2→atomic Apply/CAS→publish。
10. Context Owner-local Refresh/Apply/Inspect及atomic backend已YES；仍需Application public三阶段Port、两Owner adapters/nonzero sources、`knowledge_reference`、G6B/root。
