# Context Engine公共装配与组件映射

## 1. 公共/私有Port边界

| 面 | 性质 | Context使用方式 |
|---|---|---|
| Application-owned `ContextTurnRefreshPortV1` | N=1三段公共协调合同 | Context Owner实现Adapter；test fixture可手工注入做隔离验证；真实接线仅production composition root；Application只编排 |
| Application-owned `ContextOwnerSourceReaderV1` | Memory/Knowledge Owner current与exact正文中立读缝隙 | Context只消费Owner V2 Adapter投影；不吞并Memory/Knowledge事实 |
| `ContextParentFrameCurrentReaderV1` | Context-owned只读公共Port | exact Source/Frame/Manifest/Generation请求；S1/InspectFrame/S2；无写口 |
| Runtime Binding/Operation/Review/Evidence/Settlement | 已有公共治理合同 | 只消费公共types与Gateway，不写Runtime Owner事实 |
| Runtime `OperationScopeEvidenceApplicabilityCurrentReaderV3` | 已有公共只读合同 | `context-engine/runtimeadapter`仅实现Context Kind；不创建Evidence或Applicability Fact |
| Application Operation Domain Router/State Port | 公共编排面 | 通过Context Adapter保留Reservation并吸收Settlement |
| Harness `ports.ContextPort` | Harness私有 | 本组件不得实现成公共跨模块依赖；由Harness/集成Adapter消费已物化Frame |
| Harness `ModelTurnPort` / `EventCandidatePort` | Harness私有 | Context不得导入或调用 |
| Model Invoker RouteID/routegateway/公开execution union | Model Invoker公共执行面 | 通过集成Adapter使用；禁止internal、厂商SDK、Raw/Native事件 |

## 2. Slot/Phase贡献声明

Harness live Assembly Contract/Compiler已经发布版本化Slot/Phase Catalog、CompiledGraph、Assembly Generation/Handoff及Runtime Generation-Binding Association接线。本组件不发明枚举、Hookface或替代Graph，只引用以下live对象：

- Slot：`context.frame`（Run lifecycle，one owner/many sources）、`model.turn`、`action.router`、`memory.state`、`continuity.timeline`；
- Phase：`turn.continuation.evaluate`、`context.sources.collect`、`context.frame.validate`、`context.frame.frozen`、`model.request.prepare`、`model.dispatch.before`；
- Binding：sealed Assembly Generation、Generation-Binding Association、Binding/Activation currentness。

| 贡献角色 | 语义需求 | 禁止能力 |
|---|---|---|
| Observer | 读取已治理Source/Outcome引用并产生Observation Candidate | 不写其他Owner事实、不联网 |
| Filter | 对ContextCandidate执行Recipe/Authority/Freshness/Budget Admission | 不改变Source事实、不执行Effect |
| Gate | 在Model Turn前要求exact Frame frozen及Expected/Actual策略满足 | 不直接Dispatch模型或修改Run |
| Port | 通过版本化Context Domain Port Prepare/Inspect/Materialize | 不成为任意Hook、不绕过Application/Runtime治理 |

正式实现只能引用Harness接线最终发布的namespaced、版本化公共Slot/Phase对象；没有该对象时实现阶段Fail Closed。

## 3. 依赖DAG

```text
Harness Assembly Generation / CompiledGraph（live）
  -> Runtime Binding V2 / Run Settlement Plan
  -> Application SingleCallParentFrameCoordinateV1 (neutral exact coordinate)
  -> Runtime router losslessly projects Kind/ID/Revision/Digest
  -> Context runtimeadapter Context kind
  -> Context Owner ResolveExactSourceBinding
  -> Frame/Manifest/Generation exact FactRef + scope S1 / InspectFrame / S2
  -> Runtime public applicability current projection
  -> G6A settles ToolResultV2 + current V4 Inspection + verified Association
  -> Application-owned SettledActionContextSourceCurrentReaderV1
  -> Application calls ContextTurnRefreshPortV1.Refresh
  -> [test-only] fixture manually injects public Ports
     OR [production] composition root injects Context Owner Adapter
  -> Context G6B S1 / deterministic Reservation / Admission / frozen Manifest+Frame
  -> pending Context DomainResult (no current pointer)
  -> Application calls ContextTurnRefreshPortV1.Apply
  -> Context S2 fresh owner-current reread
  -> atomic Context ApplySettlement + expected Generation current CAS
  -> applied_current G6B acceptance candidate
  -> G6B acceptance / capability enablement gate
  -> Application may call Harness PrepareContinuationCandidateV2 with exact FrameRef+Digest
  -> Model Invoker RouteID + public execution union
  -> Actual Injection / Cache Usage / Model Observation
  -> Context Owner Inspect/DomainResultFact CAS
  -> Runtime Operation Settlement
  -> Context ApplySettlement
  -> Runtime Run Settlement participant closure
  -> Continuity Frame/Outcome events
```

依赖方向禁止反转：Context不导入Harness/Application/Model Invoker实现包，其他组件不写Context Fact。

## 4. Runtime/Harness/Application映射

| Context动作 | Runtime | Application | Harness |
|---|---|---|---|
| Recipe/Frame准备 | 提供Binding/Scope currentness；不编译 | 创建/恢复namespaced Step与Context Attempt | 不参与 |
| 远程Resolve/Cache（非G6B首切面） | Operation Effect V3 Admission/Permit/Begin/actual-point Enforcement/Inspect；仅使用Runtime Owner明确支持的外部Operation settlement | 推进统一Effect状态机 | 不得作为隐式网络后门 |
| Frame交付 | 不持有正文 | 关联exact FrameRef/Digest | 私有Adapter物化Snapshot；Candidate绑定ref/digest |
| N=1 refresh | `CTX-D09-R1`已落地：零Runtime Settlement/Port变更 | 已发布三段Port/DTO及Owner Source Reader并完成B-cross fixture；只编排Prepare→Apply→Inspect，不改领域事实 | 不import Context实现；production启用前不调用Continuation |
| G6A ParentFrame current | 公共router只投影source四元组并验证公共projection；不创建Context Fact | `SingleCallParentFrameCoordinateV1`/Input S1/S2只携带中立exact坐标并依赖Reader | 不import Context实现、不持有Reader写口 |
| Actual Injection | 只保存治理/证据关联 | 关联Observation/Settlement | Harness/Invoker产生Observation，不判定Context事实 |
| Run关闭 | Runtime Settlement读取Participant | 请求Context Owner Inspect | Harness terminal Event仍只是Claim |

## 5. G6A/G6B分段门禁

1. G6A已证明：settled `ToolResultV2`、current V4 Inspection、verified Association和CTX-D10 ParentFrame current Reader可提供exact-current输入；G6B不重造这些事实；
2. CTX-D09公共合同：Application已发布三段`ContextTurnRefreshPortV1`及`ContextOwnerSourceReaderV1`；Context Adapter与B-cross fixture已完成，首切面Source基数固定`Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`；
3. Memory/Knowledge各自唯一public V2 Reader经其Owner Adapter实现Application中立Port；Context不定义平行nominal/DTO、不依赖Owner concrete Store/internal。`memory_recall`与`knowledge_reference`仅作为DynamicTail受限材料，S1/S2稳定关联和exact content observation失败即Fail Closed；
4. Turn mapping固定为具名Session/Turn Owner Reader返回Source Turn exact ref，其`Ordinal=T`必须exact等于settled Tool/ExpectedCurrent `uint32` Turn；`TargetTurn=T+1`只由Context `childExecution`生成并归属child Frame/Generation。唯一transition proof由Context Owner在pending seal后产生；Application只编排、不mint。Memory/Knowledge只返回T/current projection，不得自行`+1`、补造Session/Turn或产生proof；
5. `CTX-D09-R1`已落地，A层Context独占实现及本组件B层fixture已完成并通过二轮独立复审；二者不等待production root；
6. 依赖方向固定为Context Owner Adapter只依赖Application公共`contract/ports`；Application与Harness不import Context实现。B-cross fixture已手工注入公共Ports，且不得冒充production root；
7. G6B能力启用门禁：G6B验收、Generation current/CAS、Owner-current Readers、权威State Plane、Route exact materialization与Fail Closed/Residual策略全部通过；
8. G6B启用前不得调用Harness Continuation、不得推进Turn；
9. N>1、通用Refresh与Checkpoint接线继续冻结，不得借G6B扩展。

Application公共DTO/Port、Context Adapter及Memory/Knowledge B-cross fixture已完成；C层production Capability与Application/Harness真实装配仍只能由production composition root在G6B验收后启用。

**A Owner-local：已完成/二审YES。** kernel/store已由本组件fixture验证。**B-local：已完成/二审YES。** 只注入Owner-local中立接口。**B-cross：已完成。** 手工注入Application与Memory/Knowledge公共Ports并通过ordinary100/race20。**C production：NO-GO。** G6B production依赖通过后才由composition root启用。

当前truth：`CTX-D09-R1` A/B-local、Application公共Port Adapter及Memory/Knowledge B-cross fixture已完成；C层仍等待production composition root、Capability、Harness exact Continuation与Turn推进验收。

## 6. 冲突检查

- 同一Slot/Phase的多个Gate必须由CompiledGraph定义合并语义；缺规则即Plan拒绝；
- Filter不得重排强制Instruction；排序只由Recipe/Frame Compiler；
- 同Frame Attempt只能有一个Manifest/Frame CAS胜者；
- 同Run/Turn只能绑定一个Current Frame，替换必须新Generation/Revision事实；
- Source Turn Ref不来自具名Owner Reader、Ordinal不exact等于settled Tool/ExpectedCurrent T、Target不是Context childExecution的T+1、proof Owner不是Context，或Run/Session/proof不exact一致时Fail Closed；拒绝跨Turn replay、Application/Memory/Knowledge mint proof、各自`+1`和补造Session/Turn；
- 顺序不是`pending seal → Context proof → S2 → atomic Apply/CAS → publish`，stable closure含phase/time/self-digest，或fresh observation缺phase/Checked/Expires/current projection digests时Fail Closed；
- Harness Candidate的ContextRef/Digest必须与Application绑定的Frame一致；
- Model Route、Model Profile、Harness Capability或Tool Schema漂移会使Frame/CachePlan失效；
- ContextReference不可物化的Route必须Fail Closed或声明Residual，不能静默丢弃；
- Cache reuse scope与Run审计Scope冲突时取更窄隔离，不以命中率覆盖Authority。
- N=1首切面出现Tool 0/2、Memory/Knowledge/Continuity任一非0、全链Digest漂移、StablePrefix/SemiStable exact ContentRef变化、PrefixDigest/cache key drift或Frame TTL长于任一输入，均必须拒绝。
- S2 currentness变化后发布current pointer、ApplySettlement与Generation CAS非原子可见、扩展V4/复用Tool settlement、Unknown/cancel/deadline被降格为确定性错误或lost reply创建新ID，均不得产生`applied_current`或Harness调用。
- ParentFrame source/public ref Kind不匹配、四元组type-pun、同FrameID换Revision/Digest、同ID跨Tenant/ExecutionScope歧义、Frame/Manifest/Generation/current pointer/scope/run/session/turn漂移、取回Frame计算的scope digest与请求/公共projection不一致、TTL crossing或Kind router缺失，必须在Evidence Issue、Tool watermark和Provider之前Conflict/Fail Closed。

## 7. Residual

允许Residual必须在CompiledGraph/Binding/Frame Manifest三处一致，包含Owner、影响、Mitigation与Conformance降级。官方Harness opaque prompt、隐藏compaction、不可观察cache、无法full replace的instruction属于典型Residual；强制Authority/Secret/Workspace/Tool Surface漂移不允许Residual。

Continuity只在Context ApplySettlement之后，以Evidence Ledger sequence投影Frame/Generation exact Owner refs；Context不得创建Timeline事实、分配序列或直接绑定SQLite/RocksDB。RocksDB接口存在不代表生产Backend、持久性或SLA。

live Continuity的`TimelineOwnerFactRefV1`和Checkpoint V2 exact Context refs已经覆盖投影/恢复所需的引用形状；CTX-D06不再要求新增第二套Context事件或结构共享DTO。当前唯一缺口是Continuity typed Owner-current Reader/Router仍位于`continuity/runtimeadapter`而非公共`continuity/ports`。在Continuity Owner发布该公共Port或唯一无损facade前，Context不得导入其runtimeadapter、复制nominal或注册production Timeline投影；公共合同冻结后，Context只提供回读自身exact Frame/Generation/Outcome的只读Adapter，事件顺序、Evidence复读、publish、storage与lost-reply恢复继续由Continuity拥有。
