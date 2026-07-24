# Context Engineering、Context Frame与Cache治理设计

状态：规划内无需production composition root的切片已完成软件验证，包括`CTX-D09-R1` A/B-cross、`CTX-D10`、Offline/Engineering SDK与API/CLI、Compaction/Generation、Outcome、Recipe/PromptAsset pre-release、Prompt Provenance、durable Reviewer Context单节点参考能力、Restore Context materialization及`reference_only` Component Release候选。PromptAsset采用用户确认的内嵌规范化片段方案；production Recipe/Prompt发布仍等待CTX-D07，production State Plane/Cache、Capability、Harness Continuation与Turn推进未启用。

最高业务输入：[Context&Cache定稿文档](../../../tmp.document/Context&Cache.md)。本设计同时受根`AGENTS.md`、`.properties.rax/MAIN.md`、6+1组件治理合同及Runtime/Harness live contracts约束；业务语义以定稿文档为先，运行治理以live schema为准。

开发者工程面的[Context Engineering SDK V1](./engineering-sdk.md)已完成Prompt validate/preview与可插拔Evaluator Observation→Context Evaluation/Feedback Owner-local链；五入口transport-neutral API/CLI已通过target100/race20及full ordinary/race/vet。它不扩既有Offline SDK六operation闭集，也不实现远程Judge或production发布。模型专属Prompt的[官方上游审计](./prompt-upstream-audit.md)已覆盖Codex、Gemini、Kimi、Grok、Claude SDK、DeepSeek、MiniMax及T3Code消费边界；[Prompt Upstream Provenance V1](./prompt-provenance.md)的Owner-local exact合同、seal/verify核、fixture与分层反例已实现并通过相同门。具体预埋结构见[多模型 Prompt 候选架构](./prompt-family-candidates.md)；production模型适用性仍等待Model Invoker exact Profile ref/current reader。

## 1. 核心定位

Context Engine是模型调用前的最后一层“世界投影编译器”，不是conversation history容器，也不是字符串拼接器。它把Prompt/Profile/Human/Artifact/Tool/MCP/Memory/Knowledge/Continuity/Sandbox/Skill/Index等来源提交的受治理候选，按Recipe、Authority、Freshness、相关性、预算、Model Profile和Harness能力编译为一次模型调用可消费的不可变Context Frame。

优先级冻结为：

```text
Context正确性 > Cache经济性 > Prefix长度
```

稳定但无用的Prefix仍是污染；Cache命中不得保留已经失效的权限、来源或项目规则。

## 2. 四个核心事实

| 对象 | 含义 | Owner |
|---|---|---|
| `ContextRecipe` | 开发者长期定义的来源、选择、顺序、预算、降级、压缩与渲染规则 | Context Engine |
| `ContextManifest` | 某次编译真正选择/排除了什么、原因、版本、位置、预算与Cache区段 | Context Engine |
| `ContextFrame` | Manifest渲染后针对一次Model Turn的不可变冻结点 | Context Engine |
| `ContextOutcome` | Frame被消费后的模型、Tool、用户纠正、质量、成本与延迟关联结果 | Context Engine仅拥有评测关联；原始事实仍归各领域Owner |

四者不得合并成新的PromptPack大对象。Frame是投影，不是文件、Memory、Authority或外部状态的新权威。

## 3. Owner与非Owner

### 3.1 Context/Cache Owner

- Recipe、Fragment语义、Candidate Admission、Manifest、Frame、Generation、Compaction Summary；
- Context结构共享、Context Delta和Artifact Anchor投影语义；
- ParentFrame Applicability nominal source coordinate、exact-current Reader与Context-owned current projection；
- Stable Prefix划分、Provider-neutral CachePlan、Cache Partition与Cache Entry领域事实；
- Expected Injection Manifest、Expected/Actual差异判定和Context Conformance结果；
- Prompt/Recipe反馈候选、评测、版本发布与回滚领域状态；
- 自身Receipt、Inspect、CAS、Settlement与Cleanup。
- Context Owner-local离线SDK的Validate/Compile/Preview/Inspect诊断语义；SDK输出不是领域事实、current或执行资格。

### 3.2 明确非Owner

- Runtime拥有Identity/Lineage/Instance/Run/Fence epoch、Run Fact/ExecutionOutcome和全局线性化入口；
- Application Coordinator拥有跨域Workflow/Attempt编排，不拥有Context事实；
- Harness拥有Run内Session、Model Turn Candidate和source-ordered Event，不拥有Frame；
- Model Invoker拥有精确Route、Provider协议转译、ProviderCache能力事实、Actual Injection Observation与Provider Usage；
- Memory/Knowledge/Tool/MCP/Sandbox/Continuity/Review分别拥有其候选来源、执行、时序和Verdict事实；
- Artifact/File真实版本由产生或检查该资产的领域Owner提供，Context Anchor只证明某Frame消费过的版本与范围；
- Evidence Ledger拥有证据序列与信任分类，Context不得把Observation自升为Authoritative Fact。

## 4. 部署与依赖边界

| 区域 | 内容 |
|---|---|
| Sandbox外Controller | Recipe编译、Admission、Frame Freeze、Cache/Prompt Controller、Owner Inspect/CAS |
| Data Plane | 只消费已物化、已治理、精确Digest的Frame；最终注入由Harness/Invoker适配面完成 |
| State Plane | Recipe/Manifest/Frame/Delta/Anchor/Cache/Prompt版本事实；Sandbox本地盘不得是唯一副本 |
| Remote Provider | 可选远程Resolver或Cache；必须经过Operation Effect、Gateway、实际点二次校验与独立Settlement |

组件domain/kernel只依赖自身合同。Runtime Adapter只允许依赖`runtime/core`与`runtime/ports`；不得依赖Runtime foundation/fakes/kernel或Harness kernel/fakes/internal。Harness私有`ports.ContextPort`不是本组件公共Port。

## 5. Context编译流水线

```text
Source Candidate
  -> source currentness / scope / authority inspect
  -> Candidate Admission
  -> resolve inline/reference/anchor/diff
  -> semantic dedupe + explicit ordering
  -> budget + optional degradation
  -> Stable Prefix / Semi-stable / Dynamic Tail layout
  -> ContextManifest freeze
  -> provider-neutral render
  -> ContextFrame CAS freeze
  -> Expected Injection preflight
  -> Harness/Invoker consumes exact FrameRef+Digest
  -> Actual Injection / Model / Tool observations
  -> ContextOutcome inspect and evaluation
```

Required Source不可用、版本漂移、权限不可验证或强制注入差异时Fail Closed。Optional Source只能按Recipe中已声明的降级策略排除，并在Manifest留下Residual。

### 5.1 per-turn Context Refresh最小接线

Harness live `ModelTurnCandidateV2`与Assembly Slot/Phase只作为未来生产启用依赖；本Delta不新增Slot/Phase，也不调用Harness。Context已完成Owner-local三段Service、单一Owner backend/store、Application公共`ContextTurnRefreshPortV1` Adapter及本组件B-cross test-only fixture。Adapter只依赖Application公共`contract/ports`，Application与Harness均不import Context实现；fixture手工注入Memory/Knowledge Owner V2公共Reader，不是production composition root。

```text
Application neutral SingleCallParentFrameCoordinateV1
-> Runtime router losslessly projects Kind/ID/Revision/Digest
-> context-engine/runtimeadapter Context kind route
-> Context Owner resolves exact source binding
-> exact Frame/Manifest/Generation refs + scope S1 / InspectFrame / S2
-> Runtime public Applicability current projection
G6A: settled ToolResultV2 + current V4 Inspection + verified Association
-> Application public ContextTurnRefresh coordination + Owner source readers
-> Application calls ContextTurnRefreshPortV1.Prepare
-> B-cross test fixture manual injection (completed) OR production composition root (not wired)
G6B: S1 reread/reserve/collect/admit/freeze (Tool=1; Memory<=1; Knowledge<=1; Continuity=0)
-> context.sources.collect -> context.frame.validate -> context.frame.frozen
-> pending Context DomainResultFact (Frame/Generation not current)
-> Application calls ContextTurnRefreshPortV1.Apply
-> Context S2 fresh owner-current reread
-> atomic Context ApplySettlement + expected Generation current CAS
-> G6B acceptance boundary
-> [enablement only] Application calls Harness continuation with new exact FrameRef
-> model.request.prepare / governed dispatch
-> ProviderActualInjectionObservation -> Harness ActualInjectionManifest -> Context Conformance
```

上图前五步及其后的CTX-D09 Owner-local `Refresh → pending → S2 → atomic Apply/CAS → Inspect`、Application公共Port Adapter和Memory/Knowledge B-cross fixture均已实现并通过高重复/race验证；公共Applicability ref仍只是中立坐标，不是Evidence资格。Harness continuation与production composition/enablement仍未执行，不得由本地fixture冒充。

首切面仍固定一个settled Tool action：`Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`。Tool链只接受Owner已结算/Inspect的ToolResult/DomainResult/Apply/V4 Inspection/Association exact refs及有界ContentRef。Memory/Knowledge只经各Owner唯一public V2 Reader与Application-owned中立`ContextOwnerSourceReaderV1`做S1/S2 current复读、exact正文观察和稳定关联校验；每Owner物化正文聚合上限64KiB，进入DynamicTail的`memory_recall`/`knowledge_reference`均为受限Observation材料。raw字符串、Provider receipt、未经结算结果或第二套Owner nominal不得注入。Unknown/Unavailable/cancel/deadline保真，lost reply只Inspect原attempt。Context不构造Continuation，也不推进Turn。

G6B使用两个exact Turn坐标：`SourceTurn.Ordinal=T`必须exact等于settled Tool Execution/ExpectedCurrent的`uint32` Turn，其完整Turn Ref只能来自具名Session/Turn Owner Current Reader；`TargetTurn=T+1`只能由Context `childExecution`生成，用于child Frame/Generation。唯一transition proof的Owner是Context，Application只编排、不mint proof。proof exact绑定ExecutionScope、Run、Session、Source Turn Ref/T、Target T+1、settled Tool chain、Parent与RefreshAttempt；Memory/Knowledge不得各自`+1`、补造Session/Turn或产生proof。时序固定为`pending Frame/Generation seal → Context proof → S2 → atomic Apply/CAS → publish`。proof的stable closure排除phase/time/self-digest，fresh observation则包含phase、Checked/Expires与S1 current projection digests。

父Frame不可变。新Frame必须精确引用父Frame；StablePrefix和SemiStable逐项复用exact `ContentRef{Ref,Digest,Length}`，仅DynamicTail追加本次N=1结果。`PrefixDigest` seal完整StablePrefix；稳定cache identity使用`StableSourceSetDigest`，不得使用随DynamicTail变化的完整Manifest SourceSetDigest。任何稳定维度或TTL crossing均Fail Closed。

## 6. 来源与Admission

首期Fragment语义集合：`Instruction`、`Conversation`、`ArtifactInline`、`ArtifactReference`、`ToolDeclaration`、`ToolCall`、`ToolResult`、`StateObservation`、`MemoryRecall`、`PolicySnapshot`、`CompactionSummary`。允许扩展，但必须使用版本化namespaced kind与schema。

每个Source只提交`ContextCandidate`及Evidence，不能决定最终Role、顺序、Cache区段或全文展开。Candidate至少绑定：CandidateID、Contract/Schema Version、Kind、Owner Binding、完整Execution Scope、Run/Turn、SourceRef、Revision、Content Digest、Authority、Sensitivity、Freshness、Token Estimate Profile、Cache Stability、ExpiresAt、EvidenceRef和Idempotency Key。

Tool Result、Memory Recall、Knowledge Retrieval、Sandbox Observation和Continuity Summary默认是受限材料：可以成为模型数据，但不能携带新权限、覆盖developer/system instruction或改变Review Policy。

## 7. Context Frame、Generation与结构共享

Frame必须绑定`FrameID + Revision + Digest`、ParentFrame、Context Generation、Scope/Run/Turn、Recipe/Manifest、Model Profile、Harness Capability、Render版本、Source Set Digest、Stable Prefix、Semi-stable、Dynamic Tail、CachePlan、Expected Injection Manifest、触发Event、TTL和Evidence。N=1 Refresh的RefreshAttempt、Frame和Generation ID由冻结输入确定性派生；相同输入必须得到相同ID/Digest，不同内容复用ID必须冲突。

存储使用内容寻址与结构共享：稳定资产、Fragment和Prefix Blob只保存一次；Frame保存引用与本轮新增/替换/失效Delta。Frame语义上完整，但物理上不重复复制大Prefix。Delta链达到策略阈值、跨Generation或回放成本超过重新物化时必须Rebase成新基准。

Compaction产生新的Context Generation。它必须保留当前目标、用户纠正、有效要求、关键决定、已验证事实、Artifact Anchor、Tool状态、未完成工作、阻塞、外部Effect及来源引用。精确代码、结构化数据、Credential和不可逆Effect结果只能保留引用或重新物化，不能让摘要成为新事实权威。

新Generation的`RetainedAnchorSet`是压缩后Anchor继续有效的唯一证明。旧Anchor未被精确列入、Revision/Digest漂移或TTL失效时立即视为invalid并重新物化；摘要提到文件不等于保留Anchor。

## 8. Artifact、diff与Anchor

`ArtifactAnchor`记录：Artifact Owner、Artifact Ref、版本/Digest、已读范围/符号/查询、物化方式、Frame/Generation、Evidence、创建时间与有效性。它只证明“该Agent在该Frame获得过该版本的该范围”。

- 版本和范围未变：返回`unchanged + AnchorRef`；
- 版本变化且基线可用：优先返回`base→current diff`；
- Anchor失效、冲突或任务需要更大范围：重新物化；
- 小文件允许全文；大文件先Index/符号/搜索/范围读取；
- Tool写入参数、实际结果和写后版本必须通过Receipt、Digest或定向读取关联；
- diff链达到阈值时Rebase，禁止无限回放。

Context不直接读任意文件路径；Artifact Resolver是否本地读取或远程读取由对应Owner和Effect合同决定。大文件和旧Tool输出不得整段重灌，只能进入有界确定性摘要或`ArtifactRef + Version/Revision + Digest + Range`；Delta链必须有上限，压缩后未列入新Generation `RetainedAnchorSet`的Anchor立即失效。

## 9. Stable Prefix与Provider Cache

Frame逻辑布局固定为：

1. Stable Prefix：Praxis内核提示、Agent/Model Profile、长期项目规则、稳定工具定义、Context协议；
2. Semi-stable：项目状态、Session Summary、工作集索引、活跃Memory、Continuity状态；
3. Dynamic Tail：最近对话、当前片段、实时Tool Result、临时观察、本轮用户输入。

集合必须稳定排序；新Tool或Fragment不得无故插入并破坏此前Prefix。Cache指标以`cache-eligible prefix tokens`为分母，记录read/write/dynamic token、变化断点、miss原因、Fragment贡献、费用和延迟；不可观察的Provider Cache保持`unknown`。

`ProviderCacheProfile`由Model Profile/Provider Adapter拥有并版本化提供，Context只消费能力事实并生成`CachePlan`。Provider特有`cache_control`不进入Context核心合同。预热/续期只有在预期节省高于写入、保活和调用成本时才可成为Effect候选，绝不无条件制造请求。

Cache reuse partition与事实审计Scope分离：事实记录携带完整tenant/identity/authority/lineage/instance/run/effect上下文；复用范围由显式`ReuseScopeClass + IsolationDigest`决定。跨Run复用只在Recipe、Authority等价策略、Sensitivity和Provider能力均允许时成立，不能为了方便把Run字段从审计事实中删除。

Cache key必须覆盖`ReuseScope + Isolation + Authority + Sensitivity + StableSourceSetDigest + Recipe + Render + Model + Harness + ToolSchema + StablePrefix ContentRefs + ProviderProfile + KeyVersion`。TTL取请求NotAfter、Recipe、父Frame/Manifest/Generation、settled Tool source、Artifact currentness、Provider Profile、Assembly Generation/Binding/Activation currentness与Cache policy的最短值；命中时复验`Created <= now < Expires`、exact Partition/Key/Authority和InvalidationGeneration。只有预期读取收益严格大于write+keepalive成本时才形成远程Cache Effect候选；Plan、Receipt、usage/cached tokens都不是hit。

## 10. Expected与Actual Injection

`ContextFrame`描述Praxis希望交付的语义输入。实际注入分三层：Model Invoker拥有Route级`ProviderActualInjectionObservation`；Harness Model Turn Adapter拥有聚合Run/Turn/Frame/Route/Attempt/source sequence后的`ActualInjectionManifest`；Context Owner独立Inspect Expected/Actual并CAS `InjectionConformanceFact`。Harness Capability Profile必须声明full replace、append、provider-owned、opaque区域，以及Tool/Memory/Skill/MCP/Hook/Compaction控制能力。

两条链严格分离：Context `ExpectedInjectionManifest`只描述Frame/field注入预期，并与Provider Observation、Harness Actual Manifest形成Context Injection Conformance链；它不是Tool Surface expected digest，也不参与Tool Surface Gate。Tool Surface Gate只消费Tool Owner的`ToolSurfaceManifest.ExpectedInjectionDigest`及其独立current/assembly链，禁止把Context Expected Manifest、Context Frame digest或InjectionConformanceFact代入该等式。

Actual Manifest必须携带至少一个类型化Observation Ref，绑定Observation ID/Revision/Digest、Execution、Route、Attempt、Frame、source sequence与fidelity，并按source sequence规范排序且唯一。只有Expected处于`Created <= current < Expires`、全部Observation为`complete`且逐项exact一致时才可`matched`；缺失或不可观察为`unknown`，绑定漂移为`rejected`。

Preflight比较Expected/Actual：强制指令、权限、Secret、Workspace、Tool Surface或Context Source出现未允许漂移时拒绝；允许的官方Residual进入Evidence与Conformance。Provider原生Session仅是Observation/Resume Hint；隐藏Compaction或重试无法完整观测时，event fidelity标记`partial/unavailable`，不得补造历史。

## 11. Governed Effect、Receipt、Inspect、CAS、Settlement

本地纯编译和只读内容寻址Lookup在无网络、无费用、无写入时不是外部Effect，但仍须验证Current Scope/Authority并记录领域Access事实。以下动作必须走Runtime Operation Effect v3：远程Source Resolve、数据披露、Provider Cache创建/写/删/独立查询、预热/续期、生产Recipe/Prompt发布、可能计费的评测和任何外部持久写。

CTX-D09 G6B首切面不包含上述远程/Provider动作。`CTX-D09-R1`已裁决：本地迁移不创建、请求或消费任何Runtime settlement；Context Owner在S2后原子提交ApplySettlement+expected Generation current CAS。不得扩V4、引入additive Runtime settlement或复用Tool settlement。

统一链路严格复用已经闭合的Runtime Operation V3/Application链，不在组件内重造：

```text
Context领域Intent/Reservation
-> Runtime Admission（读取Authority/Review/Budget/Scope/Binding currentness）
-> Permit
-> Begin
-> Delegation / Provider Prepare
-> Enforcement持久化
-> Execute，或Begin后仅Inspect原attempt
-> Observation / Evidence
-> Context Owner Inspect / DomainResultFact CAS
-> Runtime Operation Settlement
-> Context ApplySettlement
```

Provider成功回包不是Cache hit、Frame、Prompt版本或ContextOutcome权威事实。任何Begin后回包未知都进入`unknown/reconciling`；只Inspect exact Attempt，禁止以新ID重派或复用同一FrameID生成不同内容。CTX-D09中Context先形成pending DomainResult，S2 fresh复读通过后才原子提交ApplySettlement+Generation current CAS；S2失败current pointer不可见。未来外部Effect仍消费其对应Runtime Operation Settlement，但不得借此推断Context本地迁移可使用V4。

### 11.1 pre-run Evidence裁决

V1的权威Context Frame Prepare只发生在Run已经进入running之后的每Turn边界，因此不产生pre-run Runtime Evidence，也不提出OperationScope-aware pre-run Evidence Delta。Recipe离线validate/compare/compile/preview与Frame inspect只生成非权威诊断报告；未来若要在Run前把Recipe认证结果写入Runtime Evidence，必须另开联合设计。

Runtime已冻结独立[OperationScope Evidence V3设计](../runtime/operation-scope-evidence-v3/README.md)的中立Applicability Reader合同。activation首批矩阵仍把Context设为forbidden；G6A Action矩阵则要求Context维度`required`，因此必须先闭合`CTX-D10`，不能把普通FactRef、Application DTO或公共ref本身当作current资格。Context不得直接Issue/Consume Evidence，也不得因此提前解冻per-turn Refresh。

### 11.1.1 G6A ParentFrame Applicability Current Reader V1

Context Owner发布distinct `ContextParentFrameApplicabilitySourceCoordinateV1`与只读`ContextParentFrameCurrentReaderV1`。方案A令Coordinate `ID=FrameID`，但ID只作Owner metadata index的首个查询key，不假设跨Tenant/ExecutionScope全局唯一。Source digest必须seal exact Frame/Manifest/Generation、ExecutionScope/Run/Session/Turn与Parent binding；Reader先按完整四元Coordinate解析sealed binding，再以完整`FactRef{ID,Revision,Digest}`和预期scope exact读取Frame/Manifest/Generation。取回Frame计算出的scope digest必须与请求及公共projection一致。

`context-engine/runtimeadapter`只实现既有Runtime `OperationScopeEvidenceApplicabilityCurrentReaderV3`的Context Kind路由：公共ref四元组无损映射到Context source coordinate，Adapter把完整四元组交给Owner Reader解析和复读，再生成Runtime公共current projection。Adapter不得持有Frame/Manifest/Generation、sealed binding map或快照冒充current。Runtime不创建Context Applicability Fact；Context不持有Evidence写口。V1观察租约最多30秒且取所有可读领域上界的最小值，不延长Frame、Manifest、Generation、Recipe或Authority TTL，不宣称SLA。同ID换Revision/Digest、跨Tenant/scope歧义或scope digest不一致均Conflict/Fail Closed。

### 11.2 Effect与Conflict Domain登记

| Effect kind | 触发点 | Conflict Domain |
|---|---|---|
| `context/remote-source-resolve` | 远程Context/Reference解析或数据披露 | `context/remote-source` |
| `context/provider-cache-query` | 独立于Model调用的远程Cache查询 | `context/provider-cache` |
| `context/provider-cache-create` / `write` / `refresh` | 建立、写入、预热或续期 | `context/provider-cache` |
| `context/provider-cache-invalidate` / `delete` | 远程失效或清理 | `context/provider-cache` |
| `context/remote-evaluation` | 计费或外发的Recipe/Frame评测、远程Compaction | `context/remote-evaluation` |
| `context/recipe-release` / `rollback` / `revoke` | 生产Recipe/Prompt Current Binding变化 | `context/recipe-release` |

Conflict Domain使用Runtime规定的tenant-stable scope；精确资源、Frame/Recipe版本和Attempt放在Idempotency Key/Payload中，不以缩窄到Run的方式规避冲突。普通本地Frame编译、只读内容寻址Lookup和离线preview不产生Effect。

### 11.3 RunStartRequirement与RunSettlementRequirement

使用Context Engine的Run必须通过`RunStartRequirement`在Binding/Plan中声明精确Context组件Manifest、Artifact Digest、合同版本和`frame-prepare/frame-inspect/frame-materialize`能力；ContextReference不能物化的Route必须Fail Closed，或在Run Plan中作为用户已接受的Residual并降低Conformance。

Run Settlement Plan包含两个Context Owner requirement：`context/frame-closure`（completion阶段，证明全部已引用Frame Attempt已有领域终态）和`context/effect-cleanup`（termination_report阶段，证明Context/Cache Effect已Settlement/Cleanup；确实无此操作时只能用精确Policy形成`operation_not_required`）。Context只提供Participant Fact，不选择Runtime Run Outcome。

## 12. SDK、CLI与API边界

- Go SDK：Schema builder、Recipe validate/compile/preview/diff/evaluate、Frame/Manifest inspector；不得暴露绕过Admission/CAS的Store句柄；
- CLI：当前已实现`recipe validate|compare|compile|preview`、`frame inspect`与`cache inspect`；Cache Inspect只核验已给定provider-neutral Plan/Profile，不生成Plan、不访问Provider、不声明hit；未来`evaluate|publish|rollback`、replay、cache/anchor写操作必须分别经过冻结合同与治理API，不由离线Ingress暗开；
- API：transport-neutral的Create/Inspect/CAS/Watch/Page合同；所有可变事实必须ExpectedRevision，回包丢失只Inspect；
- Owner-local Offline Ingress：公开六request Seal/Encode、六typed/JSON API及stdin/stdout CLI，严格复用Offline SDK唯一codec/canonical路径；不含listener、Store、Capability或写命令；
- Adapter：Runtime/Application/Harness/Model Invoker适配独立于domain/kernel；不得反向导入对方实现包；
- V1默认Go。当前没有可证明的计算稠密热点，不规划Rust、FFI或独立Rust进程。

## 13. 当前不做

- 不选择生产DB/RPC/进程拓扑、具体Tokenizer、Remote Provider或SLA；
- 不修改Runtime/Harness/Model Invoker，不私建兼容接口；
- 不把Cache hit、Provider receipt、模型总结或Context Frame升级为其他领域权威；
- 不宣称90%为已达成SLA；它只是按cache-eligible prefix定义的评测目标；
- `CTX-D09-R1`已冻结；A层Context Owner-local kernel/store与本组件B层test-only fixture已完成并通过二轮独立复审，均不依赖production root。
- Application公共Port、Context Adapter及Memory/Knowledge B-cross test-only fixture已完成；它不注册capability、不调用Harness、不推进Turn。C层production composition root与能力启用仍NO-GO。
- G6B验收、Generation current/CAS、Owner-current Readers、权威State Plane与Route exact materialization门禁全部通过前，production composition root接线、Refresh capability、Harness Continuation与Turn推进NO-GO。
- N>1 Tool action、通用Refresh、Checkpoint与Continuity来源继续冻结；Memory/Knowledge首个各一项Owner projection切面仅在B-cross fixture内完成。
- 内存ReferenceStore与test fixture只能证明合同，不得宣称production Backend/root/SLA。
- live代码已创建Context Owner-local Refresh、pending DomainResult、local ApplySettlement、Generation CAS、新Frame及Application公共Port Adapter；Runtime Settlement调用为0。未创建Harness Continuation、Turn推进或production root接线。

Continuity current truth：`TimelineOwnerFactRefV1`与Checkpoint V2已具备Context exact Owner refs，结构共享坐标不再缺失；但typed Owner-current Reader/Router仍是`continuity/runtimeadapter`私有装配接口。CTX-D06因此只保留“由Continuity Owner发布公共Reader或唯一无损facade”这一条Delta。Context不复制其nominal、不导入Continuity实现、不写Timeline/RocksDB；公共Port与production root未闭前，真实Timeline投影仍NO-GO。

## 14. 配套资产

- [对象与版本合同](./contracts.md)
- [状态机与恢复](./state-machines.md)
- [SDK、CLI、API边界](./sdk-api.md)
- [公共装配与组件映射](./integration.md)
- [结构化Port Delta](./port-delta.md)
- [兼容与迁移](./migration.md)
- [验收合同](./acceptance/README.md)
- [架构图](./architecture.drawio)
- [per-turn实施与NO-GO计划](../../plan/context-engine/context-engine-v1.md)
- [N=1 Refresh测试矩阵](../../plan/context-engine/test-matrix.md)
