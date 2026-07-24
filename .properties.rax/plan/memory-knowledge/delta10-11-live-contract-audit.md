# Delta 10/11 live contract差异审计

> 状态：**historical_live_audit / superseded_by_reference_implementation**。本文保留实现前差异；Memory/Knowledge V1/V2仍属于同一Reader合同族并复用同一Owner Store/Journal/current。cross-owner reference链现已实现；production root与远程Retrieval仍未启用。

## 1. live基线

本次逐项复读：

- `ExecutionRuntime/memory-knowledge/memory/contextsource/current_reader.go`；
- `ExecutionRuntime/memory-knowledge/knowledge/contextsource/current_reader.go`；
- `ExecutionRuntime/memory-knowledge/contract/{common,errors,settlement}.go`；
- `ExecutionRuntime/context-engine/contract/{source,frame,refresh}.go`；
- live Harness `CommittedPendingActionReaderV3.InspectCommittedPendingActionCurrentV3`已提供canonical Session/Turn applicability coordinate、Turn ordinal与fresh TTL；Application已有中立Session/Turn exact nominal。仍缺的是Harness-owned Adapter到未来Application Refresh合同的无损映射与S1/S2传递，不需要第二Turn Store/Reader；
- `ExecutionRuntime/harness/README.md`与Harness私有`ports.ContextPort/ModelTurnPort/EventCandidatePort`边界；
- Application当前N=1 G6A合同；Context已有Owner-local Refresh/Apply/Inspect，但Application尚无Memory/Knowledge可用的public三阶段Port且该Port尚未命名。

首个live Context Refresh cardinality仍固定`Tool=1, Memory=0, Knowledge=0, Continuity=0`。Context `ContextCandidate.SourceRef`只有ID字符串与Revision，没有Source Digest；`FragmentKind`有`memory_recall`但没有Knowledge kind。Application尚未发布Memory/Knowledge可用的Context Refresh公共Port，Harness只允许消费已物化Context输入。

## 2. 差异矩阵

| 面 | live事实 | Delta 10/11所需 | 裁决 | 等级 |
|---|---|---|---|---|
| Reader接口 | 两Owner均已有`InspectAttempt/InspectForTurn/ReadContentExact` | 不增加Retrieve/Apply/Freeze方法 | 原接口族升版，方法集合不变 | closed |
| Contract family | Memory/Knowledge各自`...context-source-current-reader/v1` | 跨Owner接线仍由各Owner公开类型承载 | V2七struct逐字段schema已冻结；不新建`context-source-neutral`族 | closed |
| Identity | Memory仅`IdentityID`，无exact ref/epoch；Knowledge无Identity字段 | exact Identity ID/Revision/Digest/Epoch并入Request/Projection/Observation canonical | V1 Adapter不得补造；V2 required | P0-1坐标合同 |
| Session | 两Reader均无Session coordinate；Harness V4只有Session exact与`Turn uint32` | Session ID/Revision/Digest只能来自具名Session Owner Reader | Memory/Knowledge/Application只验证/回显，不得补造 | P0-1 |
| Turn | Reader只有`TurnID string`；Context/Tool live是`Turn uint32`；无具名Turn exact Reader | `SourceTurnOrdinal == Tool.Execution.Turn == ExpectedCurrent.Turn`；SourceTurn exact ref只能来自具名Turn Owner Reader；有ref时legacy `TurnID == SourceTurnRef.ID` | 禁止从uint32/legacy字符串生成ID/revision/digest；Reader禁止Target/+1/proof | P0-1 |
| Attempt current | `InspectAttempt`在Owner锁域fresh clock，区分ID不存在与ref漂移 | 保留；Inspection回传完整Identity/Session/Turn | 升版字段进入canonical | External P0-1随坐标 |
| Owner current | `InspectForTurn`复读settled Attempt与Owner current state，输出ClosureDigest/TTL | stable Closure与fresh Projection/Observation digest分层 | 精确常量/body已冻结 | closed |
| Local content | sealed `LocalContentReader`、Owner StatePlaneBinding、同RLock内fresh S1→Get→fresh S2 | Context只能消费bounded body并materialize；Application/Harness不得接触body | 原`ReadContentExact`复用，不新增Content DTO或远程fallback | closed |
| Association | Store内部验证DomainResultAssociation；ProjectionItem只公开DomainResultRef/ApplicationRef | Context source binding必须证明同一DomainResult association | AssociationDigest设计已通过；V2字段已逐项列出 | closed |
| Budget | live Reader request/projection无MaxItems/Bytes/Tokens或Estimator | Context Candidate要求TokenEstimate/EstimatorDigest；Owner bundle必须有界 | V2 request/item/projection预算字段已逐项冻结 | closed |
| Closed errors | 现有sentinel errors加AttemptStatus；多种current失败折叠为`ErrNotCurrent`/`ErrEvidenceConflict` | 消费方必须稳定Fail Closed且不能靠字符串解析 | 第八审已通过；不建ClosedReason DTO | closed |
| Memory kind | Context live有`memory_recall` | Memory Reader不得创建Context Candidate | Context Owner可在Admission后选择该kind；Owner输出不携带kind | closed |
| Knowledge kind | Context live无Knowledge kind | 需要独立、可引用、有Citation/License边界的kind | 候选`knowledge_reference`；未接受时unsupported，禁止换kind | External P0-5 |
| Context source binding | Candidate SourceRef缺Digest，无法exact绑定Owner Projection+Item+Content Observation | Context Fact必须seal Owner exact source chain | 由Context Owner加法演进其Candidate/Fact；Memory/Knowledge不创建Source wrapper DTO | External P0-5 |
| Context Owner-local | live已有`RefreshContextTurnV1/ApplyContextTurnRefreshV1/InspectContextTurnRefreshV1`、单一Owner backend/lock、atomic Apply+Generation CAS及并发/Unknown/故障fixture | 复用，不重造 | 当前YES；未来source扩展不得削弱原子性 | closed |
| Transition | live尚无Memory/Knowledge所需pre-frame request/final proof分层 | Context独占final proof；pending outputs seal→proof seal→S2→atomic Apply/CAS→publish | Application只协调，不拥有或seal proof | External P0-2 |
| Application | 当前未发布Memory/Knowledge Refresh三阶段公共Port | pre-frame request、apply/advance、inspect三阶段只传播exact refs | 不解释closed error、不补字段、不写Fact | External P0-3 |
| Harness | 私有ContextPort不是公共6+1 Port | 只消费current exact Frame | 不接Reader、Projection、body或pending Frame | closed |
| Remote | Retrieval Gateway仍unsupported | 本地已settled refs不依赖远程Gateway | local-only Reader独立；Provider/Resolver=0 | closed |

## 3. 唯一演进路径

```text
MemoryContextSourceCurrentReaderV1 / KnowledgeContextSourceCurrentReaderV1
  └─ 同一合同族冻结V2：相同三方法、相同Owner package与语义对象
       + exact Identity ref/epoch
       + named Session Owner Reader exact Session ID/revision/digest
       + SourceTurnOrdinal uint32 and named Turn Owner Reader exact ref
       + CheckPhase(s1|s2)与ExpectedS1ClosureDigest规则
       + AssociationDigest、Source/Citation set digest
       + MaxItems/Bytes/Tokens/Estimator
       + context.Context cancellation/deadline与MaxBodyBytes hard cap
       + 可errors.Is的闭集加法
```

禁止路径：

- 新建`praxis.memory/context-source-neutral/v1`或`praxis.knowledge/context-source-neutral/v1`；
- 由Application/Context复制一套Owner DTO；
- 用Runtime通用Ref、Application DTO或Context FactRef强转Owner对象；
- 用V1 Adapter补造Identity epoch、Session或Turn revision/digest；
- 让Knowledge fragment source wrapper绕过Context Candidate exact source binding缺口。

## 4. Source/Target Turn与proof所有权候选

1. 唯一直接等式是`TurnOwnerFact(SourceTurnRef).Ordinal == SourceTurnOrdinal == settled Tool Execution.Turn == ExpectedCurrent.Turn`，ordinal类型为live `uint32`。V2 `SourceTurnRef`的Go类型是`contract.Ref`；不能在Ref上加私有Ordinal字段，也不能据此凭空生成ID/revision/digest。
2. SourceTurn ID/revision/digest只能由具名Turn Owner Reader提供；Session exact coordinate只能由具名Session Owner Reader提供。Memory/Knowledge只验证/回显；有SourceTurnRef时legacy `TurnID == SourceTurnRef.ID`且ordinal一致。
3. 两Owner request/projection/content observation不得携带、计算或验证TargetTurn，不得执行`+1`，不得拥有或构造TransitionProof。
4. Context是TransitionProof唯一Owner。Application先协调pre-frame transition request，只含SourceOrdinal=T与ExpectedTargetOrdinal=T+1等已有字段；Context pending DomainResult/Manifest/Frame/Generation seal后，Context再seal final proof并绑定childExecution及new Frame/Generation exact refs。
5. 唯一顺序是`pending outputs seal -> Context final proof seal -> Owner S2 -> Context atomic ApplySettlement+Generation CAS -> publish`。proof存在不等于current；Application不得seal或改写proof。
6. Context逐项组合两个Owner SourceTurn与final proof；任一Owner ordinal不等于T、具名Owner exact ref缺失/漂移、proof与childExecution/Frame/Generation不一致均Fail Closed。
7. live V1只有TurnID且无revision/digest/ordinal、Session；缺原始证据的记录不可无损升级，只能历史Inspect或取得新V2 Attempt。
8. Harness只消费Context current exact Frame；Harness Event、Continuation Claim或Frame消费不能证明Owner SourceTurn或Target transition current。

## 5. S1/S2与local-only内容冻结设计

| 阶段 | Reader调用 | 不变量 |
|---|---|---|
| S1 Attempt | `InspectAttempt` | `persisted_and_settled`；完整坐标一致 |
| S1 Current | `InspectForTurn(CheckPhase=s1, ExpectedS1ClosureDigest=empty)` | fresh Owner clock；形成stable ClosureDigest与S1 fresh Projection，不发布Context current |
| S1 Content | `ReadContentExact(CheckPhase=s1)` | 同RLock内部Get前后双fresh检查；只返回Owner-local bounded body |
| Context pending/proof | Context Owner materialize/admit/compile后seal final proof | pending outputs与proof均不可见、非current；proof绑定new Frame/Generation refs |
| S2 Current | `InspectForTurn(CheckPhase=s2, ExpectedS1ClosureDigest=S1)` | fresh Projection/Observation refs可不同；stable ClosureDigest与ordered exact集合必须相同 |
| S2 Content | `ReadContentExact(CheckPhase=s2)` | 再读bytes并复算binding/content/TTL；失败零body |
| Context atomic | Context local ApplySettlement + expected Generation CAS | 单可见性边界；失败只Inspect原Refresh Attempt |
| Consume | Harness接收exact Frame | 不接Owner Projection或body |

Reader内部`ReadContentExact`的Get前后S1/S2与Context外层S1/S2是两层不同检查，二者都必须保留，不能互相替代。

stable `ClosureDigest`不得包含`AttemptInspectionRef`、`CheckPhase`、`OwnerCheckedAt`、`ExpiresAt`或Projection self ref；fresh Projection必须保留Inspection ref，Projection/Observation digest必须包含各自phase、fresh time/expiry、stable ClosureDigest和fresh exact refs。重新Inspect导致Inspection/Projection ref变化时，只要S1/S2 stable集合相同就不能仅因fresh ref不同拒绝；S2 request不携S1 Inspection ref。若fresh ref相同但stable集合漂移，也必须拒绝。

## 6. Closed error冻结设计

不新增`ClosedReason` DTO。公共语义沿用每个方法的typed result、AttemptStatus和Go `errors.Is`：

| 公共分类 | live/演进载体 | 语义 |
|---|---|---|
| `persisted_and_settled` / `persisted_unsettled` / `confirmed_not_persisted` | `AttemptStatus` | 仅InspectAttempt成功返回 |
| invalid contract | `ErrInvalidArgument` | version/kind/必填字段/strict validation |
| exact/coordinate conflict | `ErrEvidenceConflict`或`ErrRevisionConflict` | exact ref、canonical、Identity/Session/Turn不一致 |
| not current/expired/withdrawn/poisoned | `ErrNotCurrent` | 不返回stale projection/body |
| scope/license/sensitivity denied | `ErrScopeDenied`、`ErrSensitivityDenied`；License专用sentinel为原error set加法候选 | 不泄露来源或正文 |
| inspection incomplete | `ErrInspectionIncomplete` | 不降级为NotFound或空命中 |
| evicted | `ErrNotFound`（仅ReadContentExact方法域） | 不联网、不读Context缓存 |
| remote required/unmaterialized | `ErrContextUnmaterialized` | Reader零Effect |
| capability/kind unsupported | `ErrUnsupported` | 首个G6B来源和调用数仍为0 |

错误时result为零值、body为空；不得解析错误字符串、返回成功空列表、Partial Coverage或自由文本reason。若需要更细粒度License/Withdraw语义，只能在现有`memory-knowledge/contract` error set做版本化加法并保持`errors.Is`，不能包装成第二套跨Owner DTO。

## 7. `knowledge_reference`边界

- `knowledge_reference`只是提交给Context Owner的`FragmentKind`加法候选，不是Knowledge Reader字段或Knowledge Fact。
- Knowledge Reader输出仍是现有CurrentProjection、ProjectionItem和ExactContentObservation的升版；不新增`KnowledgeFragmentSourceRefV1`。
- Context必须在自己的Candidate/Manifest/Fragment中exact seal：Knowledge Projection Ref、Item Rank、ExactContentObservation Ref、Record/Package/Snapshot/Pointer/Source/Projection refs、CitationDigest、License、Conflict、ContentRef和TTL。
- live `ContextCandidate.SourceRef + SourceRevision`缺少Digest，也不能表达上述完整source chain，因此在Context Owner加法合同YES前Knowledge来源保持0。
- 禁止降级到`memory_recall`、`artifact_reference`、`instruction`或`state_observation`；Route/Recipe不支持时返回unsupported/Residual。
- Knowledge不选择Context Region、Position、Required、TrustClass或Token预算；Context也不能把Knowledge Trust标签升级为`authoritative_instruction`。

## 8. P0/P1/P2

### P0（5）

1. SourceTurnOrdinal等式、Harness V3 exact Session/Turn证据经Harness-owned Adapter到Application Refresh合同的无损映射与S1/S2传递；任何Owner不得补造，也不得新建第二Turn Reader。
2. Context-owned pre-frame transition request/final TransitionProof及固定时序。
3. Application public三阶段Refresh Port；Application只协调。
4. Memory/Knowledge adapters、nonzero source cardinality、G6B/production root接线。
5. Context Owner接受`knowledge_reference`及Knowledge Candidate exact Owner source binding；此前Knowledge=0。

### Owner-local轴

- P0=0。
- P1=0：两Owner各七个V2 public struct的逐字段Go type/JSON tag/字段顺序，以及StableClosureDigest精确domain/version/ObjectKind/canonical body已经冻结；live V1 ClosureDigest含`ExpiresAt`且不可复用。
- P2=0。

stable/fresh、Owner evidence/no-migration、DomainResultAssociation digest、ctx-aware bounded cancel与diagnostics均已通过，不再列为缺口。本轮未写Go，不把设计Review状态冒充实现状态。

当前裁决：**Owner-local设计合同已冻结且P0/P1/P2均为0；External P0=5未关闭前不写跨Owner Go、不启用非零Memory/Knowledge来源。**
