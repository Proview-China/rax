# Memory + Knowledge 结构化 Port Delta

> Current truth：**Owner-local与cross-owner reference integration均P0=0/P1=0/P2=0**。V2 schema、Harness exact Turn映射、Application三阶段、Context TransitionProof/`knowledge_reference`、双Owner Adapter及非零fixture已实现；下方P0-1至P0-5保留为已解决Delta记录。production root、真实远程Gateway与生产Backend继续NO-GO。

本文件只提出缺口，不在组件内创建兼容接口。Runtime live P0.1-P0.6、Operation V3与Application Coordinator视为已闭合；下列Delta仅覆盖Memory/Knowledge领域语义或精确跨域接线。

## Delta 1：Memory Domain Port v1

- 用例：提交/查询Memory Candidate，执行Admission，正式Commit/Correction/Forget，Inspect原Attempt并返回CAS/Settlement。
- 语义Owner：Memory Owner；Application只编排，Runtime只治理Operation。
- 输入输出：输入版本化Candidate/View/Query/Commit Intent/Expected Revision/治理Refs；输出Admission Fact、Retrieval Result、Owner Fact、Receipt、Inspection、Settlement、Residual。
- 不变量：Candidate不等于Fact；正式Record只由Owner CAS；历史Revision不覆盖；Provider回包只作Observation；Context只收候选。
- Effect/Recovery：正式写入、Indexer、Forget/Purge走相应已冻结Operation合同；远程Query另受Delta 9A约束，在retrieval-specific三版本冻结前unsupported且Provider=0。Begin后只Inspect原Attempt。
- 反例：扩展现有窄`runtime.MemoryPort`并让模型输出直接`Put`长期记忆。
- 兼容影响：新增版本化组件公共Port；现有Runtime窄Port不删除，只明确降级为Observation骨架。

## Delta 2：Knowledge Domain Port v1

- 用例：Source注册/Acquire、Package/Record Candidate、Admission、Snapshot Publish/Withdraw、Query/Citation、Inspect/CAS/Settlement。
- 语义Owner：Knowledge Owner；Asset仍拥有原始字节。
- 输入输出：输入Source/Asset Ref、Candidate、View/Snapshot Query、Expected Revision、治理Refs；输出Source/Package/Record/Snapshot Fact、Citation/Coverage、Inspection、Settlement、Residual。
- 不变量：Source/Record/Snapshot分别版本化；Snapshot发布不可变；Projection不是Fact；冲突Claim保留来源；Provider命中不升级事实。
- Effect/Recovery：Acquire/Index、Commit/Publish/Withdraw/Purge走相应已冻结Operation合同；远程Query/Resolver另受Delta 9A约束，在retrieval-specific三版本冻结前unsupported且Provider/Resolver=0。Begin后Inspect原Attempt。
- 反例：把现有窄`runtime.KnowledgePort.Query`的Provider命中当正式Knowledge或Current Snapshot。
- 兼容影响：新增版本化组件公共Port；不改变现有Query Observation合同。

## Delta 3：OperationScope-aware Evidence

- 用例：没有活跃Run时，记录Memory管理员Correction/Forget/Purge和Knowledge Source注册/撤回/Snapshot发布的权威Owner Fact。
- 语义Owner：Evidence Owner定义Ledger/Projection；Runtime Owner定义Operation Subject/currentness；Memory/Knowledge只提供Owner Fact Ref。
- 输入输出：输入OperationSubject kind/ID/revision/current projection、Tenant/Identity/Lineage、Owner Fact ID/revision/digest/payload、source epoch/sequence、authority/policy；输出append-only Evidence Ref及currentness结果。
- 不变量：时点为Owner CAS形成DomainResultFact后、Runtime Operation Settlement前；不伪造Run；authoritative evidence必须引用精确Owner Fact；OperationScope currentness不能退化。
- Effect/Recovery：Evidence追加失败不得回滚已CAS领域Fact；Settlement携带Evidence Residual，并按原Operation恢复追加。
- 反例：创建假Run以通过Evidence v2 active Run校验，或直接把Provider Receipt写成authoritative evidence。
- 兼容影响：对Evidence/Runtime是加法版本；Run Evidence v2保持不变。只阻塞无Run管理Operation，不阻塞Run内路径。

## Delta 4：Operation Review Verdict公共合同

- 用例：admin/custom Operation在无Run时对Memory Forget/Purge、Knowledge Publish/Withdraw、远程披露等高风险Effect取得可验证Verdict。
- 语义Owner：Review Owner；Runtime校验绑定/currentness；组件只消费。
- 输入输出：输入Operation Subject、Intent/Effect Payload Digest、Authority/Policy/Scope/Budget、Reviewer/TTL；输出Case/Verdict ID、revision、decision、exact bindings、revocation/currentness。
- 不变量：Attestation不是Verdict；Verdict精确绑定Intent revision与Payload；失效/撤销Fail Closed；组件不可自审。
- Effect/Recovery：Review本身不证明Effect已执行；Begin后仍按原Attempt Inspect。
- 反例：复用强制RunID的Review v2对象并填充伪Run，或用`operation_not_required`字符串绕过Review Owner。
- 兼容影响：加法Operation-scope版本；Run Review v2不放宽。

## Delta 5：Memory/Knowledge Domain Adapter与Operation映射

- 用例：Application Coordinator将namespaced Operation V3的execute/inspect/settlement请求映射到Memory/Knowledge Owner，而不识别组件内部实现。
- 语义Owner：Application Owner拥有跨域映射；Runtime Owner拥有Operation协议；Memory/Knowledge拥有领域Manifest、Fact、Inspect/CAS/Settlement。
- 输入输出：输入Domain/Step kind、Manifest/Capability、Operation/Attempt全部Refs、Payload Ref/Digest；输出有界Observation、Owner Inspection、Settlement/Residual。
- 不变量：顺序不可跳过；实际执行点二次Enforcement；Adapter不写Kernel/Runtime Outcome；runtimeadapter只依赖runtime/core与runtime/ports。
- Effect/Recovery：Execute/Inspect均受Operation治理；Inspect原Attempt，不能递归无限创建Inspect Operation。
- 反例：在Application里解析Memory内部状态并直接写Fact，或在组件内重建私有Permit/Fence“兼容层”。
- 兼容影响：新增namespaced Domain Adapter binding；不修改Operation V3语义。

## Delta 6：Retrieval/Citation/Context Candidate与Reference能力

- 用例：Memory/Knowledge把有版本、来源、Citation、Coverage和预算的候选交给Context，按Route能力安全物化。
- 语义Owner：Memory/Knowledge拥有Retrieval Result；Context Owner拥有Frame/Admission/物化；Model Invoker只消费最终Context。
- 输入输出：输入View/Snapshot/Query/Purpose/Authority/Policy/Route capability；输出Context Candidate、Citation、Coverage、Representation/Reference、Residual。
- 不变量：Result不是Frame；取正文前过滤；ContextReference不支持时必需内容Fail Closed、可选内容Partial；不得声称模型已看到未物化引用。
- Effect/Recovery：远程Query/Resolver是待支持Effect；当前只返回unsupported且Provider/Resolver=0。未来仅在Delta 9A三版本冻结后，Unknown才按原Gateway Attempt Inspect；本地Partial不是Unknown。
- 反例：Harness私有ContextPort直连Knowledge Store，或Model Route读取厂商Raw事件来展开引用。
- 兼容影响：新增Context公共版本化对象与Route能力声明；不公开Harness私有Port。

## Delta 7：RunStart与RunSettlement Requirement分型

- 用例：Assembler/Coordinator分别认证Run启动所需Memory View、Knowledge Snapshot、Coverage和ContextReference能力，并只在Run内正式写入时等待对应Owner的领域Settlement。
- 语义Owner：Memory/Knowledge发布Requirement declaration；Assembler合并；Runtime拥有Run Requirement/Settlement Plan与Outcome。
- 输入输出：输入Manifest、View/Snapshot/Watermark/Coverage/TTL/Capability与参与者；输出版本化RunStartRequirement，以及分别namespaced为`praxis.memory/domain-commits`、`praxis.knowledge/domain-commits`的RunSettlementRequirement contribution和Participant Inspection。
- 不变量：RunStartRequirement、OperationScope、RunSettlementRequirement不混用；组件不创建Run Plan、不认证Binding、不选择Outcome；本地只读Query不虚构domain commit；远程Retrieval当前unsupported，未来只在专用Applicability/Evidence/Settlement版本冻结后按其合同单独结算。
- Effect/Recovery：Run内Begin后的领域Attempt必须settled或显式indeterminate/reconciliation；Cleanup/Residual独立。
- 反例：Memory检测到Record存在后直接把Run标记成功。
- 兼容影响：对Assembler/Runtime Requirement是加法贡献；需要统一合并规则。

## Delta 8：Assembler Slot/Phase贡献映射

- 用例：把`memory.state`、`knowledge.query`和Context Source贡献合入CompiledGraph，并在指定Phase调用受限Port。
- 语义Owner：Harness/Assembler Owner定义公共Slot/Phase/merge/Binding；Memory/Knowledge只声明贡献。
- 输入输出：输入版本化`SlotContribution/PortSpec/PortBinding/PhaseContribution/DependencySpec`；输出CompiledGraph中的确定性绑定、冲突/残缺诊断。
- 不变量：只用既有namespaced对象；Phase kind仅Observer/Filter/Gate/Port；无任意Context修改/联网/写Fact通用Hook；合并确定且可解释。
- Effect/Recovery：Phase Port不得触发当前unsupported的远程Retrieval；未来若专用版本获批，也只能经Application/Retrieval Gateway。Harness异常只产Observation/Claim，不产领域Fact。
- 反例：组件自行声明`MemoryBeforeTurnHook`枚举并在Harness内直连网络。
- 兼容影响：依赖公共Agent Assembler输出、Assembly SDK/CompiledGraph、Slot/Phase合并、Binding V2、Checkpoint/Action Gateway和per-turn refresh统一合同；本组件不提供替代实现。

## Context Refresh四组Delta（Delta 9与公共reference接线已完成；远程Gateway仍NO-GO）

以下Delta补充且收窄Delta 6/8：Owner Current Reader只读本地State Plane；effectful Retrieval/远程正文读取进入独立Gateway；Memory/Knowledge Owner分别Inspect；Context Owner才可物化和冻结；Application协调；Harness只消费Frame。在对应Owner联合接受和用户再次授权前，不得实现私有替代接口。

## Delta 9：两个独立Owner Current Reader

Owner专属字段合同已分别实现：[MemoryContextSourceCurrentReaderV1](../../design/memory-engine/context-source-port-v1.md)与[KnowledgeContextSourceCurrentReaderV1](../../design/knowledge-engine/context-source-port-v1.md)，并通过第三次独立复审YES。两者仍是独立Owner与独立合同，任何一方的状态都不能替另一方授权或形成统一Owner。

- 用例：对已经由Owner持久化并settled的Retrieval Observation/Result，在Frame freeze前证明Record、DomainResultAssociation、SettlementApplication、本地exact content和TTL仍为current。
- 语义Owner：Memory Owner与Knowledge Owner分别拥有自己的Port、Observation、状态、排序和预算；不得合并为一个`MemoryKnowledgeOwner`。Context只调用，不解释Owner Store。
- 输入：Contract Version、Execution Scope Digest、Run/Turn、原Attempt、Observation/Result exact refs、Expected Query/View/Watermark；Knowledge另含Published Snapshot；Authority/Policy/Purpose、Owner独立Items/Bytes/Tokens/PerItem预算、Estimator、Inspect时间、Frame Deadline和Idempotency。
- 输出：固定Owner的Contribution exact ref、Observation/Result、Query/View/Watermark/Snapshot、Coverage/NextCursor/Result/Evidence Digest、Observed/Expires、Residual和有序Items；Item含Rank/Score、Record/Content、DomainResult/Association/SettlementApplication、Source/Evidence/Projection/Citation，Knowledge另含Package/Snapshot/License/Trust/Conflict。
- 不变量：Owner重算DomainResult canonical digest并精确验证Association ID/revision/digest、Owner、Subject Ref、CASAfter与SettlementApplication；复读current Record/View/Watermark，Knowledge再复读Published Snapshot pointer、Package/Source/Projection/License；`now >= Expires`拒绝。
- 排序/预算：Owner内`Score desc -> Record ID asc -> Revision desc -> Digest asc`；跨Owner不比较Score、不借预算、不自动去重。exact bytes和固定Estimator决定token；换序、Citation/Coverage/NextCursor变化必须改变canonical digest。
- Effect/Recovery：Reader只有本地`InspectAttempt/InspectForTurn/ReadContentExact`，零Provider/网络/远程Resolver。出现`remote_required`只返回Residual，不得自行执行Effect；远程lost reply不由Reader恢复。
- 反例：Reader包含`Retrieve`；本地正文evicted后自动联网；用本地`InspectAttempt`推断Provider执行成功；未完成Evidence/Settlement/Apply仍返回current；同Observation ref换Result、错Association、Watermark/Pointer/License/TTL漂移。
- 兼容影响：给两个领域分别增加版本化Port；不修改Runtime窄Port，不公开Store，不允许一个Owner导入另一个Owner实现。
- G6B门禁：两个Owner-local Reader、双Adapter、Application三阶段Port与Context非零reference fixture已YES（Memory=1、Knowledge=1）；production root仍未装配。远程Gateway仍NO-GO；Checkpoint/Restore另行接口。

## Delta 9A：两个effectful Retrieval Domain Gateway

Owner专属Delta：[Memory Retrieval Domain Gateway](../../design/memory-engine/retrieval-domain-gateway-v1.md)与[Knowledge Retrieval Domain Gateway](../../design/knowledge-engine/retrieval-domain-gateway-v1.md)。它们不是Context Source Reader，也不因Reader Review YES自动获批。

- 用例：远程Query、Resolver或正文读取；持久化Provider Observation及Owner Retrieval Observation/Result exact refs。
- 语义Owner：Application编排；Runtime拥有Operation/Attempt/Permit/Begin/Fence/Settlement；Memory/Knowledge分别拥有领域Inspect、DomainResultFact与ApplySettlement；Provider只拥有Observation。
- 输入：Operation Subject/ID/revision/digest、Intent/Reservation、Admission/Review、Permit、Begin、Delegation/Prepare、prepare+execute Enforcement、原Attempt、Idempotency/Conflict Domain、Provider/Remote Target、Payload digest，以及Authority/Policy/Scope/Purpose/Sensitivity/Budget；Knowledge另含License、Source/Snapshot/Pointer。
- 输出：Dispatch Receipt、Provider Observation、Owner Retrieval Observation/Result refs、DomainResultAssociation、Evidence、Runtime Settlement ref、ApplySettlement projection与Residual。Receipt不证明成功。
- 不变量：顺序固定为`Intent/Reservation -> Admission -> Permit -> Begin -> Delegation/Prepare -> 双Enforcement -> Execute/Inspect -> Observation -> DomainResult -> Evidence -> Runtime Settlement -> ApplySettlement`；错Operation/Attempt/Permit/DomainResult/Settlement拒绝。
- Effect/Recovery：Begin后lost reply只`Gateway.InspectOriginalAttempt`，携带原Operation/Attempt/Permit/Prepare/Enforcement/Provider坐标；不得用Reader Inspect、盲重派或换Provider。远程Inspection是Effect时走非递归Inspection Operation。
- 支持门禁：当前G6A closed matrix仅覆盖Tool，Checkpoint V5不得复用。必须由Application/Runtime/Evidence/领域Owner联合冻结retrieval-specific additive Applicability、Evidence、Settlement版本，并闭合Provider Observation、Owner DomainResult、Runtime ref-only Settlement与Domain Apply；任一未YES时Gateway稳定`unsupported`且Provider/Resolver调用数为0。
- 反例：签名只有Query/Scope没有治理坐标；套用Tool G6A或Checkpoint V5；只获Evidence版本却执行Provider探测；Provider success直入Frame；远程正文直接流进Context；Evidence失败仍宣称closed；同DomainResult ID换revision/digest后Apply成功。
- 兼容影响：加法Delta，不修改Runtime/Asset现有Port，不私建兼容接口，不预选后端、RPC、进程拓扑或SLA。
- G6B门禁：reference与未装配的production root中两个远程Gateway调用数均为0。

## Delta 10：Context Owner-local Refresh source扩展

- 本模块Owner输入：[Memory Owner V2冻结与接线合同](../../design/memory-engine/context-refresh-neutral-delta10-11-v1.md)与[Knowledge Owner V2冻结/fragment kind合同](../../design/knowledge-engine/context-refresh-neutral-delta10-11-v1.md)。V2 Reader字段仍属于两个Owner各自合同；Turn/Context/Application/Adapter/fragment由对应Owner public nominal实现，不是第二套Owner DTO。
- 用例：在每个Model turn前分别读取两个Owner Contribution，复读current DomainResult/Association/exact content/TTL，物化Context content-addressed refs，并按Recipe Admission、Compile、Inspect和freeze exact Frame。
- 语义Owner：Context Owner拥有live owner-local Refresh/Apply/Inspect、Candidate、Manifest、Frame、Generation、Recipe、TransitionProof与最终预算；Memory/Knowledge只提供Owner current Observation，不拥有Context对象。本Delta不伪造新的Context public Port名称。
- 输入：ContractVersion/ObjectKind、ExecutionScopeDigest、RunID、Harness committed PendingAction Session/Turn exact证据、`SourceTurnRef contract.Ref`、`SourceTurnOrdinal uint32`、Context pre-frame transition request、Recipe/Parent/ExpectedCurrent、Refresh Attempt/Idempotency/CheckedUpperBound/NotAfter；要求`SourceTurnRef/Ordinal == Tool.Execution.Turn == ExpectedCurrent.Turn`。Memory/Knowledge只验证/回显Owner原始Source字段，不得携带TargetTurn或补造Session/Turn exact ref。
- 输出：S1后先seal不可见pending Context DomainResult/Manifest/Frame/Generation，再由Context seal final TransitionProof；S2和单个本地原子提交成功后才publish current exact Frame/Manifest/Generation refs、SourceSet Digests、Residual、Checked/Expires及canonical Digest。
- 不变量：Context Owner-local Refresh/Apply/Inspect、单一backend/lock、atomic Apply+Generation CAS与fixture已YES。扩展链必须是Owner S1→pending DomainResult/Manifest/Frame/Generation seal→Context Owner final TransitionProof seal→Owner S2→atomic Apply/CAS→publish。Application只协调；proof不授current。
- 排序/预算：每Owner一个canonical Contribution Bundle保存内部rank；Owner间位置由公共Recipe确定。Memory现有`memory_recall`只能由Context Owner创建Candidate；Knowledge请求加法kind=`knowledge_reference`，未由Context Owner发布时返回`unsupported_fragment_kind`，不得降级冒充Memory/Artifact/Instruction。不得编码rank到ID绕过Context排序。
- TTL/currentness：stable ClosureDigest排除`AttemptInspectionRef`、CheckPhase、OwnerCheckedAt、ExpiresAt与Projection self ref；fresh Projection保留Inspection ref并与Observation一起包含phase/time/expiry/stable closure。重新Inspect可改变fresh exact refs，但stable ClosureDigest、Source coordinate和ordered exact Item集合必须相同；S2不要求携带S1 Inspection ref，Frame/Generation TTL取所有fresh上界最小值。
- Set digest：Memory六个、Knowledge八个set digest必须按[V2冻结合同](./context-source-reader-v2-frozen.md#44-set-digest精确算法)逐项重算；OrderedItemSet只摘要stable item body，Ref/Content/String集合采用冻结正规化，空集合编码`[]`。字符串非空不构成验证。
- Closed errors：沿用两个Owner现有`AttemptStatus`与Go `errors.Is`闭集；仅允许在原`memory-knowledge/contract` error set版本化加法，不创建`ClosedReason` DTO。错误时零body，禁止成功空列表掩盖失败。
- Effect/Recovery：远程解析不得藏进Context source扩展；Refresh/Apply回包丢失只调用live Context Owner-local Inspect原Attempt，禁止以同一身份重新Compile不同内容。
- 反例：从live uint32或legacy TurnID生成exact ref；任一Owner补Session；Application seal proof；pre-frame request含未形成的Frame/Generation refs；proof在pending outputs前seal；Closure混入phase/time/self ref；S1/S2 fresh ref不同即拒绝却不比stable集合；Memory T与Knowledge T+1混帧；S2失败仍publish；Knowledge kind缺失时换成`memory_recall`。
- 兼容影响：需要Context Candidate/Fragment能绑定Owner Contribution exact ref、Association/Citation set digest与Expires；Knowledge fragment kind由Context Owner冻结，Memory/Knowledge不得私建。

## Delta 11：Application public三阶段协调与Harness exact Frame消费

- 用例：在Model dispatch前，Application只在Gateway已获YES时触发受治理Retrieval，再调用两个Current Reader与Context Frame Assemble；Harness只接收可精确物化的Frame引用。
- 语义Owner：Application拥有调用编排和attempt关联；Harness拥有Run内Loop但不拥有检索、currentness、Frame或领域事实；Route Gateway拥有精确Reference物化能力声明。
- 输入输出：Application public三阶段Port只协调：①pre-frame transition request；②Context prepared/final proof后的apply/advance；③lost-reply inspect原attempt。Context是final proof唯一Owner；Target=T+1、childExecution与new Frame/Generation refs只由Context seal。Harness只接收Context Owner发布的current exact Frame，不接Owner projection/body。
- 不变量：不使用Harness私有`ContextPort/ModelTurnPort/EventCandidatePort`作为公共Port；不新增通用Hook或组件私有Phase；Route不能精确物化时必需内容Fail Closed，可选内容按Recipe记录Residual。
- Effect/Recovery：Gateway lost reply只`InspectOriginalAttempt`；Reader只Inspect本地Owner Journal；Context Refresh lost reply只Inspect原Frame Attempt。三类Attempt不得互相代替。Application不得重建Owner/Context Fact；Harness terminal Event、Completion Claim或Provider状态不能证明Frame已物化。
- 反例：Harness在turn内直连Memory/Knowledge；Application把DTO写成Context Fact；Owner Reader携带TargetTurn或补Session；Application只做`T.Ordinal+1`便自造Target exact ref；transition proof缺Source/Target/childExecution/Frame/Generation任一exact字段；用字符串拼接revision；Route失败时回退未验证Payload；Harness消费candidate/pending Frame；Frame回包丢失后新建Attempt。
- 兼容影响：需要公共per-turn executor/handler binding与结构化Frame ref；只引用Harness/Assembler既有namespaced对象，不由本组件修改Harness或Application。

## Delta 10/11联合Owner关闭清单

| 等级 | 已闭合reference能力 | production门禁 |
|---|---|---|
| P0-1 | Harness `CommittedPendingActionCurrentV3`→Application exact Session/Turn无损映射 | 禁止第二Turn Store或从ordinal补ref |
| P0-2 | Context create-once TransitionRequest/Proof、InspectExact/history及Apply exact binding | proof不授current；production root仍需装配 |
| P0-3 | Application Prepare/Apply/Inspect三阶段Port与lost-reply original-attempt恢复 | Application不创建Owner Fact |
| P0-4 | Memory/Knowledge V2 Adapter与Memory=1/Knowledge=1 reference fixture | reference fixture不代表production root |
| P0-5 | Context `knowledge_reference`与exact Knowledge source chain | 远程Resolver/Provider仍为0 |
| Owner-local | 两Owner各七个V2 struct、StableClosure与bounded reader | P0=0/P1=0/P2=0 |

Owner-local及cross-owner reference integration均P0=0/P1=0/P2=0；stable/fresh、AssociationDigest、ctx-aware bounded cancel与diagnostics保持YES。远程Gateway、production root和生产Backend仍**NO-GO**。

## 已解决External reference Delta（P0-1至P0-5）

以下五项由对应Owner联合发布并实现；本节保留用例和不可违反的输入输出约束，不改变各对象Owner。

### P0-1：具名Turn exact Ref/Reader与Session证据传递边界

- 用例：把settled Tool action turn `T`无损关联到两个Owner retrieval的SourceTurn，并为Context child turn `T+1`提供可验证的父坐标。
- live签名：Harness公开`CommittedPendingActionReaderV3.InspectCommittedPendingActionCurrentV3(context.Context, CommittedPendingActionCurrentRequestV3) -> CommittedPendingActionCurrentV3`；返回Run/ExecutionScope、Session ID/revision/digest、Turn ordinal、canonical `SessionApplicability`/`TurnApplicability`、Checked/Expires与Digest。Application已有`SingleCallSession/TurnCoordinateV1`及对应Applicability Source nominal；不需要新建Turn Reader。
- 语义Owner：Harness拥有current与exact applicability coordinates；Harness-owned Application Adapter负责无损投影到Application neutral nominal，Application只协调。Memory/Knowledge只消费投影后的exact refs，不导入Harness实现或补全坐标。
- 输入/输出要求：输入execution scope、Run exact ref、Session ID、SourceTurn ordinal与purpose；输出Session exact ref、Turn exact ref、ordinal、Owner evidence exact ref、checked/expires和canonical digest。
- 不变量：`TurnOwnerFact(SourceTurnRef).Ordinal == SourceTurnOrdinal == Tool.Execution.Turn == ExpectedCurrent.Turn`且legacy `TurnID == SourceTurnRef.ID`；Ref ID/revision/digest与ordinal只能由具名Turn Owner Reader共同证明。
- Effect/Recovery：纯只读；missing/stale/expired/Owner不明均Fail Closed。回包丢失只复读同一V3 current请求，不创建Turn事实。
- 反例/兼容：把`uint32`哈希成Turn digest、由Memory/Knowledge/Context补Session、Application重算而非Adapter无损映射、另建Turn Store/Reader。只在Application Refresh合同中加法携带既有nominal。

### P0-2：Context-owned TransitionProof

冻结依据见：[P0-2 Context TransitionProof设计](../../design/memory-engine/context-transition-proof-external-p0-candidate.md)；live public nominal与Store由Context Owner发布。

- 用例：证明Source/action turn `T`到Context child turn `T+1`的单一转换，并阻止pending Frame提前可见。
- 语义Owner：Context唯一拥有proof nominal、canonical、state/currentness和TTL；Application只协调。
- 输入/输出要求：pre-frame transition request只引用Source ordinal/exact evidence、ExpectedTargetOrdinal、ExpectedCurrent、parent/current与refresh attempt；Context final proof无损绑定该request、Context childExecution、pending DomainResult、new Manifest/Frame/Generation exact refs与stable SourceSet digest。Memory/Knowledge候选已经给出Ref/StableBody/fresh Projection/Reader及domain/version/ObjectKind/字段顺序；仍待Context Owner接受后才可成为public nominal/state。
- 状态/顺序要求：`Owner S1 -> pending Context DomainResult/Manifest/Frame/Generation seal（不可见） -> Context final proof seal -> Owner S2 -> Context atomic local ApplySettlement+Generation current CAS -> publish`。proof不得授current，S2或CAS失败不得publish。
- TTL/currentness：proof上界不得超过pre-frame request、Source owner evidence、pending outputs、parent/current和Recipe的最小TTL；S2必须fresh复读stable closure与exact集合。
- Effect/Recovery：本地Context Owner现有Refresh/Apply/Inspect、single backend/lock、atomic CAS与fixture已YES；新增proof回包丢失只Inspect原Context attempt。
- 反例/兼容：Application seal proof、pre-frame request含未来Frame ref、proof缺childExecution或Generation、用Checkpoint V5代替。该Delta是Context加法合同，不能修改Memory/Knowledge V1。

### P0-3：Application三阶段namespaced Port

- 用例：以一个跨域attempt协调pre-frame prepare、Context apply/advance与lost-reply inspect，而不接管任何Owner事实。
- 语义Owner：Application拥有调用编排和attempt关联；Context拥有proof与Frame，Memory/Knowledge各自拥有current Inspection。
- 输入/输出要求：三个namespaced、版本化阶段必须传递exact execution/request/attempt/owner result/proof/pending/apply refs、deadline/cancellation和Residual；实际Port名、nominal与canonical由Application Owner发布。
- 不变量：prepare不创建Context Fact；advance不重算Owner结果；inspect只能检查原attempt；Harness只消费Context已发布且current的exact Frame。
- Effect/Recovery：本地Reader调用无Provider；若未来包含远程Retrieval，仍须经过独立Gateway治理，不能藏入该Port。lost reply不得新建attempt。
- 反例/兼容：用Harness私有ContextPort、三阶段合成通用Hook、Application seal proof或创建Memory/Knowledge Fact。加法发布，不由本模块私建facade。

### P0-4：两Owner Adapter、非零来源与production root

Memory/Knowledge侧Adapter候选已经分别资产化：

- [Memory Context Source Adapter候选](../../design/memory-engine/context-source-adapter-external-p0-candidate.md)
- [Knowledge Context Source Adapter候选](../../design/knowledge-engine/context-source-adapter-external-p0-candidate.md)

两者均已实现stable association与opaque fresh envelope并用于nonzero reference fixture；这不代表production root已关闭。

- 用例：通过公共类型把Memory/Knowledge V2 current结果分别送入Context，并在G6B/root启用可证明的非零来源。
- 语义Owner：各Owner Adapter只做public contract映射；Application协调；Context选择Recipe/预算并创建Context Fact；Harness不调用Adapter。
- 输入/输出要求：Capability/Binding generation必须唯一选择V1或V2；Adapter只封装原Owner V2 exact refs、StableClosureDigest、fresh Projection/Observation refs、三个Binding current digests、Generation association、phase/TTL和closed error，不复制Owner DTO或导入Owner实现包。
- 不变量：Memory与Knowledge Adapter、Store、current、预算和排序保持隔离；来源cardinality在所有前置P0/P1 Review YES前固定为0。
- Effect/Recovery：零网络、零Provider、零Resolver；Adapter失败只返回typed Residual/closed error，不回退远程路径。
- 反例/兼容：第二Store/第二current、V1/V2双真值、root先开非零来源、Adapter改写rank/digest。只允许加法Binding迁移与shadow conformance。

### P0-5：`knowledge_reference`与exact Knowledge source chain

冻结依据见：[P0-5 knowledge_reference设计](../../design/knowledge-engine/knowledge-reference-external-p0-candidate.md)；live FragmentKind与binding由Context Owner发布。

- 用例：Context以独立Knowledge fragment kind引用Owner-current材料，同时保留可回读Citation/License/Conflict来源链。
- 语义Owner：Context拥有FragmentKind/Candidate/Frame；Knowledge拥有Projection、Record、Package、Snapshot、Pointer、Source、Citation/License/Conflict与current判断。
- 输入/输出要求：Context Candidate必须exact绑定Knowledge CurrentProjection ref、Item rank、ExactContentObservation ref、Record/Package/Snapshot/Pointer/Source/Projection refs、Citation/License/Conflict/Content digests和TTL。
- live闭合：Context已接受`knowledge_reference`并以exact source binding表达完整Knowledge source chain；Owner Adapter仍只传refs与digest，不传正文真值。
- 不变量：Context不得把Knowledge命中升级为事实；Knowledge不得创建Context Fact或选择Context预算/Region/Required；production sources未装配前保持0。
- Effect/Recovery：仅Owner-local exact content；evicted/stale/withdrawn/poisoned/license drift均Fail Closed，不启动Resolver。
- 反例/兼容：降级冒充`memory_recall`/Artifact/Instruction、只带Record ID、在diagnostics泄正文或citation text。fragment kind为Context加法版本，不修改Memory kind。

## Owner-local轴与External轴分离

Owner-local冻结合同见[Context Source Current Reader V2冻结合同](./context-source-reader-v2-frozen.md)：七个V2 struct与StableClosure精确schema已经实现并关闭Owner P1；不得复用包含`ExpiresAt`的V1 ClosureDigest helper冒充stable V2 closure。
