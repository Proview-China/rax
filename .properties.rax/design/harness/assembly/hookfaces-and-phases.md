# Harness HookFace、Phase与执行规则

## 1. HookFace定义

`HookFace`是Phase Point上严格类型化、权限有上限的扩展表面，不是万能回调。每个Contribution只能属于一个闭合类型：

| 类型 | 可以做 | 绝对禁止 |
|---|---|---|
| `Observer` | 观察、指标、审计、异步通知，产生Observation/Receipt | 阻断、改写当前对象、发起未治理Effect |
| `Filter` | 同步校验、规范化、缩减或在声明write-set内有限改写Candidate | 外部副作用、扩大Scope/权限、修改Owner事实 |
| `Gate` | 形成`allow|deny|ask|defer`决定 | 自己Dispatch、Commit、签发Runtime Permit |
| `Port` | 调用对应领域Owner的Versioned Port | 绕过Admission/Review/Governance或吞并领域语义 |

HookFaceSpec必须冻结AuthorityCeiling、MutationMask、EffectClass、同步/异步、超时、失败、并发和Receipt策略。Contribution无法通过自报提高上限。

## 2. 设计冻结候选Phase表

本表是Harness Assembly公共Phase/HookFace Catalog候选。六组件只能提交`PhaseContributionV1`引用已有Phase/HookFace ID与Digest；未知ID或自行扩展的字符串Phase一律拒绝。

| 阶段 | Phase Point | HookFace类型 | Owner/目的 |
|---|---|---|---|
| 装配 | `assembly.graph.compile.before/after` | Filter / Observer | Harness Assembly；检查输入、记录结果 |
| 装配 | `assembly.binding.validate` | Filter | Harness Assembly；Slot/Port/版本/摘要 |
| 预检 | `assembly.preflight.before/after` | Filter / Observer | Expected/Actual和currentness |
| Endpoint | `endpoint.open.before/after` | Gate / Observer | Runtime认证后打开Endpoint |
| Session | `session.start.before/after` | Filter / Observer | Harness；建立Run-local Session |
| Run | `run.start.before/after` | Gate / Observer | Runtime/Harness边界；记录启动Claim |
| 输入 | `input.accept.before/after` | Filter / Observer | 校验、规范化、记录输入 |
| Context | `context.sources.collect` | Port | Context Owner收集Candidate |
| Context | `context.frame.validate` | Filter | 权限、版本、预算、顺序、Cache Plan |
| Context | `context.frame.frozen` | Observer | 记录不可变Frame/Manifest摘要 |
| Model | `model.request.prepare` | Filter | Provider安全转译前检查 |
| Model | `model.dispatch.before` | Gate | 当前Fence/Review/Budget/Route资格 |
| Model | `model.response.observed` | Observer | Model响应观察，不是Settlement |
| Model | `model.output.validate` | Filter | 结构化输出/协议完整性 |
| Action | `action.candidate.created` | Observer | 记录Action Candidate |
| Action | `action.admission` | Filter | Tool/MCP Schema/Capability/Scope/Policy |
| Action | `action.review` | Gate | Review Owner current Verdict |
| Action | `action.dispatch` | Port | Governed Tool/MCP/Domain执行 |
| Action | `action.result.normalize` | Filter | 脱敏、Schema和结果规范化 |
| Action | `action.result.observed` | Observer | Receipt/Evidence观察 |
| Batch | `action.batch.completed` | Filter / Observer | 汇总并发Action并决定续行材料 |
| Turn | `turn.continuation.evaluate` | Filter | 继续、等输入、等Action或结束 |
| Turn | `turn.completed` | Observer | Turn状态摘要 |
| 压缩 | `context.compact.before/after` | Gate / Observer | Context Generation变化 |
| Checkpoint | `checkpoint.create.before/after` | Gate / Observer | Continuity一致性Barrier |
| 暂停 | `run.pause.before/after` | Gate / Observer | Runtime+Continuity控制，当前待Port Delta |
| 恢复 | `run.resume.before/after` | Gate / Observer | 复读旧事实和当前世界 |
| 多Agent | `agent.spawn.before/after` | Gate / Observer | 新Run/子Agent治理 |
| 多Agent | `agent.handoff.before/after` | Gate / Observer | 所有权转交或Agent-as-Tool |
| 多Agent | `subagent.completion.validate` | Gate | 父Run消费前检查 |
| 取消 | `run.cancel.before/after` | Gate / Observer | 下发取消并进入Effect reconciliation |
| 完成 | `run.completion.validate` | Gate | Pending/Unknown/Residual/Grounding检查 |
| 终态 | `run.terminal.observed` | Observer | 产生Harness Completion Claim |
| Session | `session.end.before/after` | Filter / Observer | 收口Run-local状态和服务引用 |
| Endpoint | `endpoint.close.before/after` | Gate / Observer | Fence新控制，保留Cleanup Inspect |
| Cleanup | `cleanup.before/after` | Port / Observer | 调用领域Owner清理并记录Observation |
| 残留 | `residual.detected` | Observer | 报告未清理资源和Unknown Effect |

表中Phase存在不等于当前live代码已实现。Checkpoint、Pause/Resume、真实Action Gateway、多Agent和完整Cleanup只有依赖Port冻结后才能进入实现计划对应阶段。

Action相关Phase只是Catalog中的类型化接线点，不是单Call Action Gateway实现。4.1前禁止用Observer/Filter/Gate/Port组合绕过`Observation→Runtime Evidence→Settlement Owner→Runtime Settlement→PendingAction CAS`，也禁止用`action.batch.completed`处理`N>1` Tool Calls；P3b万能Hook继续冻结。

per-turn Context refresh当前不得借用通用Hook隐式实现。它必须由HA-X01冻结其在`context.sources.collect`、`context.frame.validate`与`context.frame.frozen`之间的Owner、披露Effect、Route支持、Residual和Settlement语义后，才能加入Catalog后继版本。

## 3. 确定性排序

Compiler先验证依赖DAG，再生成稳定顺序。Authority层级固定为：

```text
Runtime Invariant
> Organization Managed Policy
> Resolved Plan Required Gate
> Slot Owner Filter/Gate
> Module Filter/Gate
> Extension Filter/Gate
> Observer
```

同层排序键固定为：依赖拓扑序、显式有界Priority、ModuleID、ContributionID。输入集合先规范化；禁止依赖注册先后、map迭代、进程地址或当前时间。

Gate合并：`deny > defer > ask > allow`。低权限Handler不能覆盖高权限deny。

Filter合并：

- 按稳定顺序执行；
- 每个Filter声明write-set和输入/输出Digest；
- write-set相交时必须存在已注册的组合规则，否则编译失败；
- Filter不得改Identity、Authority、Lease、Fence、SecretRef、Capability Grant、Owner和Runtime Fact引用。

Observer可并行，但必须有并发、队列、时间和内存上限。异步Observer不能在主流程继续后撤销当前动作。

## 4. PhaseDecision与Receipt

`PhaseDecisionV1`字段：Phase/HookFace/Handler Ref、Decision、ReasonCode、InputDigest、OutputDigest、MutationDigest、RequiredAction/Question Ref、Policy/Authority Ref、Expiry。

`PhaseReceiptV1`字段：ContractVersion、ReceiptID、Revision、Generation/Run/Turn/Step/Action Ref、Phase、Handler、Owner、SourceEpoch/Sequence、Input/Decision/Output Digest、开始/结束时间、耗时、超时、错误类别/原因、执行顺序、EvidenceRefs、ResidualRef。

Receipt先是Harness Candidate/Observation：

- 不分配Runtime Evidence Ledger sequence；
- 不形成ExecutionOutcome、ReviewVerdict或领域Commit；
- 同source coordinate同内容幂等，换内容冲突；
- 进入Evidence V2前必须有受治理Source Registration、Policy和Schema mapping。

## 5. 失败与恢复

- 必需Gate/Filter/Port超时或失败：Fail Closed，Session进入等待、失败或reconciling，取决于是否可能已产生Effect；
- 非关键Observer失败：允许降级，但必须产生错误Observation和Residual；
- Filter失败且未产生外部Effect：Rejected/Failed，可在新Step重试；
- Port回包未知：不得执行后续依赖Phase；进入reconciling并由Owner exact Inspect；
- Gate `ask`：保存问题/Review Case Ref，进入受控等待；
- Gate `defer`：保存缺失Fact/expiry条件，不忙等；
- Completion/Stop Gate要求修复时必须绑定最大轮次、时间、预算和Human Escalation；
- Plugin崩溃不能破坏Kernel锁或Session CAS；进程内panic必须隔离成结构化失败，远程断连按Effect边界判断Unknown。

## 6. 热路径性能

- Phase运行只访问预编译数组、索引和类型化函数/Port表；
- 不在Turn内解析Manifest、求解DAG、正则发现Plugin或反射扫描；
- Observer并发与Action并发分离，不能阻塞Session revision提交；
- 所有预算是显式Policy输入，不在代码中写生产SLA；
- 首先使用Go benchmark与profile证明热点。当前不规划Rust；未来只有明确CPU/内存热点、Go优化后仍不达经用户确认的基准时，才提出独立Rust Delta。
