# Tool Call候选观测与公共投影v1模块说明

## 1. 作用

本切片把Provider返回的一整个Tool Call终态批次，转换为稳定、可验证、可精确引用的Model Invoker Observation。上层无需依赖某家Provider的原始Tool Call方言，也不会把流式半成品误当成可执行动作。

实现入口位于：

- `ExecutionRuntime/model-invoker/tool_call_observation.go`；
- `ExecutionRuntime/model-invoker/tool_call_observation_projection.go`；
- `ExecutionRuntime/model-invoker/tool_call_observation_repository.go`；
- `ExecutionRuntime/model-invoker/execution/direct/`；
- `ExecutionRuntime/model-invoker/tests/toolcallobservation/`；
- `ExecutionRuntime/model-invoker/tests/executiondirect/`。

## 2. 输入与输出

输入是Model Invoker统一`Response`或按序到达的公共`StreamEvent`，以及调用方已经确认的Invocation digest。

成功输出分两层：

1. `ToolCallCandidateObservationV1`：完整描述终态中的全部Call；
2. `ToolCallCandidateObservationProjectionV1`：为公共Union补充Contract、Invocation、Observation和source exact ref。

公共Projection通过`ModelEvent.Kind = model_tool_call_candidate_observation`发出Union事件。该事件payload是唯一权威Projection形态；G6A不能依赖Application DTO携带payload，而必须通过公共Exact Reader复读。逐Call `model_tool_call`只是兼容投影，明确不是Gateway authority。

当前已实现create-once Publisher、按完整Ref读取的Exact Reader、单方法原子Ensure Repository和线程安全内存reference store。Store以Projection ID与`InvocationID + InvocationDigest + ResponseID + SourceSequence`双索引，在一个线性化点create或返回existing完整clone。Direct只注入原子Repository；Application、Harness与Tool仍只依赖Reader窄接口，不获得Publisher或Repository。

direct sync/stream现统一执行`seal -> Repository.Ensure -> exact compare -> authoritative union event -> compatibility-only calls`。Repository缺失或typed-nil时，带Tool的Direct Route在Provider调用前拒绝；屏障失败时零权威Observation、零兼容Call和零新pending投影。

恢复合同不承诺transport exactly-once：Ensure返回`Indeterminate`时，只允许以同一sealed Projection重试同一Ensure一次；成功后必须exact比较返回Projection。第二次仍失败或任何内容漂移时Fail Closed。Direct不再根据Reader的`AuthoritativeNotFound`决定跨方法重投。transport Ensure最多两次，reference store最终只保存一份canonical Projection。

当前仍没有生产持久化driver、Continuity Adapter、production composition root、Retention SLA或跨进程事件exactly-once保证。未来Adapter只能opaque保存Ref/source与完整wire正文，并在读取后交回Model strict codec重验。

## 3. 关键API

| API | 用途 |
|---|---|
| `FinalizeToolCallCandidateObservationV1` | 对sync终态整批规范化并封存Observation |
| `NewToolCallCandidateStreamFinalizerV1` | 缓冲stream provisional证据并等待完整终态 |
| `ToolCallCandidateStreamFinalizerV1.FinalizedResponseID` | 返回成功终态唯一封存的来源Response ID |
| `NewToolCallCandidateObservationProjectionV1` | 生成带exact source/ref的公共Projection |
| `DecodeToolCallCandidateObservationProjectionV1` | 使用严格公共Codec解码并验证Projection |
| `ToolCallCandidateObservationProjectionPublisherV1` | create-once发布sealed Projection的窄写口 |
| `ToolCallCandidateObservationProjectionReaderV1` | 按完整Ref exact Inspect完整Projection clone的窄读口 |
| `ToolCallCandidateObservationProjectionRepositoryV1` | Direct生产侧唯一注入的单方法原子Ensure能力 |
| `EnsureToolCallCandidateObservationProjectionV1` | Model-owned canonical producer；接收完整已sealed Projection，统一typed-nil检查、原子Ensure、同sealed最多一次恢复与exact compare；不负责seal |
| `IsToolCallCandidateObservationProjectionRepositoryUnavailableV1` | canonical producer的框架Repository nil/typed-nil依赖检查；不是Provider能力探测 |
| `NewInMemoryToolCallCandidateObservationProjectionStoreV1` | 构造线程安全reference store/test fixture；不是生产driver |
| `ToolCallCandidateObservationProjectionErrorKindOfV1` | 读取typed conflict、authoritative/unknown NotFound、Retention、Unavailable与Indeterminate分类 |

## 4. 行为保证

- sync和stream在整批finalize后恰好构造并发出一次完整Projection事件；
- Observation保持终态Output顺序，ordinal从0连续，Call ID唯一；
- 参数统一为严格canonical JSON Object；
- N>1保存为一个完整Observation，不允许部分升级；
- 逐Call兼容事件都绑定同一Observation ref，并标记`compatibility_only_not_gateway_authority`；
- 失败、取消、EOF、partial JSON、重复ID、终态冲突和lost terminal均零Projection；
- 终态Response ID必须匹配已绑定ID；空ID只能继承唯一绑定；无绑定时拒绝；
- 公共Codec拒绝顶层和任意嵌套重复JSON key；
- ID与source双索引create-once；相同Ref/canonical内容幂等，任一identity/content漂移Conflict；
- Direct配置只接受单方法原子Repository；split Store A/B Publisher+Reader wrapper不满足该接口；
- 调用者负责先seal完整Projection；Direct wrapper仅构造sealed Projection并委托Model-owned canonical producer，不保留第二套恢复/比较逻辑；
- Ensure返回完整Projection并通过本地exact compare后，才允许发权威事件；事件payload使用Ensure返回的clone；
- lost Ensure reply只以同一sealed Projection重试同一Ensure一次；
- Direct不读取Reader NotFound触发重投；Unavailable、Conflict、Invalid与第二次Indeterminate均Fail Closed；
- typed-nil Repository在New、Preflight、Open和producer helper于Backend触达前拒绝；zero-value Store仍安全；
- N>1整批作为一个Projection保存与读取；Publisher/Reader本身不创建PendingAction、ActionCandidate或执行授权。

## 5. 所有权边界

该Projection只证明“模型产生了一个完整Tool Call候选批次”。它不证明动作已获批准、已派发、已执行或已Settlement。

- Model Invoker：拥有Provider invocation、Observation和公共投影；
- Harness：未来可读取完整N=1 Observation并形成自己的PendingAction；
- Tool领域：未来可根据Harness结果形成ActionCandidate；
- Runtime：继续独占Operation Settlement和线性化终态。

任何读取方都不得把逐Call兼容事件直接升级成动作。

## 6. 验证状态

- targeted ordinary 100轮通过；
- targeted race 20轮通过；
- full ordinary、full race、vet和gofmt通过；
- Repository/Reader测试覆盖unit、whitebox构造反例、split A/B不满足原子接口、typed-nil、fault、conformance、64并发Ensure/clone/no-alias、transport调用数与Repository记录数；
- 实施前资产已获中央复核与独立Review `YES`；本轮实现未使用真实API或订阅。

设计依据见[设计合同](../../design/model-invoker/tool-call-candidate-observation-v1.md)，完成清单见[历史计划](../../plan/model-invoker/tool-call-candidate-observation-v1.md)。

Publisher/Reader实施边界见[设计](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1.md)与[实施计划](../../plan/model-invoker/tool-call-observation-projection-publish-reader-v1.md)。
