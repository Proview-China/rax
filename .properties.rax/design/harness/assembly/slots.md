# Harness类型化Slot与六组件接线

## 1. Slot合同

Slot是具有稳定类型、唯一Owner、生命周期、数量、并发和失败语义的接线位置。模块、Plugin和Port都不能自行创造未被SlotSpec声明的运行期入口。

每个SlotSpec必须冻结：

- SlotID、ContractVersion、LifecycleScope、Cardinality、Required；
- Slot语义Owner与OwnerCapability；
- 允许的`SlotContributionKind`：`owner|source|provider|reference`；
- 运行期`PhaseCapability`独立封闭为`observer|filter|gate|port`，只能通过`PhaseContributionV1`引用HookFace Catalog；不得混入Slot Contribution Kind；
- Input/Output Schema、EffectClass、ConcurrencyPolicy；
- FailurePolicy、DegradationPolicy、Dependencies与Digest；
- 是否允许动态启停，以及动态包络的Capability/Policy引用。

Slot Owner形成该Slot唯一当前结论；Contributor只提供材料或能力。一个Contribution不能借注册身份修改其他Slot或领域状态。

## 2. 设计冻结候选Slot表

本表是Harness Assembly唯一公共Catalog候选，不是各组件可以复制或追加的枚举。新增/变更公共Slot必须回到Harness接线联合评审并发布新Catalog版本；组件Contribution只引用稳定SlotID和Digest。

| Slot | 生命周期/数量 | 语义Owner | 允许贡献 | 接线结果与失败语义 |
|---|---|---|---|---|
| `kernel.loop` | Instance内、每Run 1 | Harness | owner、reference | 使用现有Governed Session推进；缺失直接拒绝装配 |
| `model.turn` | 每Generation 1 Active Binding | Model Invoker拥有Route语义；Harness拥有Run内协调 | provider、reference | 必须走governed Operation；未知只Inspect |
| `context.frame` | 每Run 1 Owner + N Source | Context&Cache | owner、source、reference | 交付冻结Frame；未治理披露/网络/Cache写拒绝 |
| `event.candidate` | 每Session 1 source journal | Harness拥有source order；Evidence/Continuity拥有各自正式账本 | source、reference | Append不确定先exact Inspect，不能自分配Runtime ledger sequence |
| `action.router` | 每Run 1 Owner | Tool&MCP | owner、provider、reference | 只形成/路由Action Candidate，未获Permit不得dispatch |
| `tool.provider.*` | 0..N | Tool&MCP | provider | namespaced能力，绑定Schema/Scope/Review/Budget/Settlement |
| `mcp.provider.*` | 0..N | Tool&MCP | provider | MCP生命周期和结果归Tool&MCP，远程调用属Effect |
| `review.gate` | 每策略域1 Owner + N Reviewer Provider | Review | owner、provider、reference | Reviewer只是领域角色；Gate/Filter/Observer能力由PhaseContribution表达。Harness只暂停/继续；Verdict currentness漂移Fail Closed |
| `sandbox.execution` | 每Instance 1 Active Binding | Sandbox | owner、provider、reference | 实际执行点二次验证；残留/cleanup由Sandbox Owner Inspect |
| `continuity.timeline` | 每Run 1 Owner + N Source | Continuity | owner、source、reference | 接收有序Candidate；Timeline/Checkpoint事实不归Harness |
| `memory.state` | 0..N | Memory | owner、source、provider、reference | Recall/Candidate/Commit使用领域Port；Harness不正式写Record |
| `knowledge.query` | 0..N | Knowledge | owner、source、provider、reference | Query/Projection返回稳定Ref，经Context选择后才入Frame |
| `asset.store` | 0..N | Asset | provider、reference | 大结果、Diff、截图、报告只传Ref；正式发布归Asset Owner |
| `identity.authority` | 每Run 1 Ref | Runtime/Authority | reference | 只读currentness引用，Plugin不得修改Epoch/Authority |
| `organization.policy` | 每Run 1 Ref | Organization | reference | 管理边界优先于模块/扩展策略 |
| `budget.policy` | 每Run 1 Ref | Budget Authority | reference | Token/费用/Turn/时间/并发预算；过期不得dispatch |
| `evidence.sink` | 每Session 1治理入口 | Runtime Evidence Owner | source、reference | PhaseReceipt/Event先是Candidate，按注册Policy进入Evidence |
| `runtime.gateway` | 每Instance 1 | Runtime | reference | 唯一Fence/Permit/Inspect/Settlement/Run入口 |
| `management.control` | 0..1 | Management/Application | reference | Gate/Observer能力另由PhaseContribution表达；只提出控制意图，不直写Harness或Runtime事实 |
| `domain.<namespace>.<name>` | 0..N | 对应namespaced领域Owner | owner、provider、source、reference | Filter/Observer/Gate/Port能力另由PhaseContribution表达；必须发布Versioned Port和Owner，不得新增Runtime/Harness kind switch |

治理引用Slot只携带已认证Ref/Port，不把Runtime、Authority、Organization或Budget权威搬进Harness。

## 3. Cardinality与冲突规则

- `1`：必须且只能有一个有效Owner/Binding；缺失、多选或证据过期拒绝；
- `0..1`：可不绑定；一旦绑定仍只能有一个Owner；
- `0..N`：Contribution按namespaced ID去重，重复ID同摘要幂等、换摘要冲突；
- `1 Owner + N Source/Reviewer`：Owner唯一，贡献集合规范化排序后进入Digest；
- `1 Active Binding`：Generation内固定；运行中切换产生新Generation，不能热替换；
- exact Capability决定隔离，不能仅凭ComponentID相同推断可复用。

Slot选择顺序由Resolved Plan的required/optional与Capability约束决定。Assembly Compiler只验证实际绑定，不做新的版本解析或偏好选择。

## 4. 六组件接入矩阵

| 组件 | 必须发布 | Harness只消费 | 禁止 |
|---|---|---|---|
| Context&Cache | Context Frame/Manifest/Compact/Cache Plan的Versioned Port与Manifest | frozen Frame Ref/Digest、当前性和Evidence | 使用Harness私有ContextPort直接联网/写Cache；Model前偷偷改Frame |
| Tool&MCP | Capability/Schema/Action Admission/Dispatch/Inspect/Settlement与MCP生命周期Port | Action Candidate结果、Tool Result Ref、Settlement | Provider Receipt直升Tool Result；Hook直接调用Tool |
| Review | Case/Candidate/Verdict/Satisfaction/Trace Port | exact Verdict/Policy/Authority/TTL Ref | Harness自判批准或把Attestation当Permit |
| Sandbox | Lease/Workspace/Enforcement/Inspect/Cleanup Port | Binding、Enforcement、Residual/Cleanup Observation | 仅靠Prompt或Context声明权限；Harness改Lease/Fence |
| Continuity | Timeline/Timestamp/Checkpoint/Fork/Restore Port | Event/Turn/Frame/Action/Claim Candidate与恢复Ref | Harness自建Timeline事实或宣称外部世界回滚 |
| Memory&Knowledge | Recall/Query/Candidate/Commit/Inspect及来源版本Port | 查询结果/候选Ref，经Context投影 | 模型输出直接Commit正式Memory/Knowledge |

各组件当前尚未联合冻结的内部对象不得写入Compiled Graph公共合同。Assembly只保存稳定PortSpec、SchemaRef、Capability和opaque/ref边界。

## 5. 动态能力

运行期动态Tool、Skill、MCP或Provider只能在sealed Generation已声明的能力包络内启停：

- 不改变Slot Owner、核心Phase顺序、Schema主版本、权限或EffectClass；
- 当前Capability/credential/evidence过期立即停止新dispatch；
- 启停形成Observation/Residual，并由对应Owner Inspect；
- 需要新外部Effect、权限扩大、核心Slot或HookFace变化时生成新Plan和新AssemblyGeneration。

## 6. 最小首切面

首个可实施切面只编译和检查以下五个必需Slot：

1. `kernel.loop`；
2. `model.turn`；
3. `context.frame`；
4. `event.candidate`；
5. `runtime.gateway`。

它只产出不可执行的sealed Assembly artifacts与诊断，不启用真实Provider。其余Slot合同同时冻结，但只有对应领域Port经联合评审后才进入可运行Graph。这避免拿未冻结的六组件内部类型提前组装。
