# Identity、Lineage、Instance、Run与结果对象

## 1. 唯一所有者表

| 对象 | 唯一语义所有者 | 生命周期与不变量 |
|---|---|---|
| `AgentDefinition` | Agent Definition模块 | 长期版本化，不包含运行对象 |
| `ResolvedAgentPlan` | Agent Assembler | 不可变、可摘要、秘密只存引用 |
| `AgentIdentity` | 每个Identity绑定的唯一Organization/Authority事实所有者 | 职责主体持久，不因实例替换变化 |
| `IdentityExecutionLease` | 每个部署绑定的唯一线性化Lease事实所有者 | V1保证一个Identity只有一个reserved或active Lineage占位；仅active授予一般执行权 |
| `InstanceLineage` | Runtime Control Plane | 先proposed后active；固定绑定一个Plan Digest，记录同一计划下的替换链 |
| `AgentInstance` | Runtime | 先proposed预留ID/epoch，ActivationCommit时绑定Lease并进入provisioning；ID永不复用 |
| `SandboxLease` | Sandbox Provider | V1独占具体Instance，不跨Instance复用 |
| `AgentRunRecord` | Runtime | V1每Instance最多一个活跃Run，记录执行生命周期 |
| `SessionState` | Harness | V1只限当前Run；Runtime只持有不透明SessionRef |
| `InteractionLoop` | Harness | 驱动模型轮次与工具结果回注，不决定Runtime终态 |
| `EffectIntent` | 每个Effect绑定解析出的唯一Intent事实所有者 | 记录规范意图、revision与dispatch资格 |
| `EffectSettlement` | 每个Effect目标绑定的唯一结算事实所有者 | 记录Receipt、Inspect与UnknownOutcome；可与Intent所有者不同但不得多主 |
| `ReviewVerdict` | Review责任域中选定的唯一Verdict所有者 | 决定候选的审核结论，不执行提交 |
| `ArtifactOrMemoryCommit` | 每个Candidate绑定的一个Asset或Memory提交所有者 | 决定并记录正式Commit状态，不改写Review结论 |
| `TaskOutcome` | 每个Task绑定的唯一任务/Management所有者 | 不由Runtime执行状态推导 |
| `GoalOutcome` | 每个Goal绑定的唯一Goal所有者或用户 | 不由Runtime执行状态推导 |

“共同拥有”不是合法所有权。多个组件只能贡献Observation、Attestation或候选结果，权威状态由表中所有者决定。

## 2. 核心对象可判定合同矩阵

| 对象 | 唯一语义所有者 | 输入 | 输出 | 不变量 | 失败、迟到与漂移处理 |
|---|---|---|---|---|---|
| `IdentityExecutionLease` | 绑定的线性化Lease事实所有者 | Identity、proposed Lineage、ActivationAttempt、Authority epoch、期限 | reserved/active/renew/revoke结果与identity epoch | V1同一Identity最多一个reserved或active持有者；reserved只允许激活与清理 | 分区或CAS冲突则推进fail closed；旧续租结果只作迟到证据 |
| `InstanceLineage` | Runtime Control Plane | Identity、ResolvedPlanDigest、supersedes引用 | proposed/active/abandoned Lineage记录 | 一个Lineage只绑定一个Plan Digest；proposed不具有执行权 | Plan/Profile/Harness/Route强制语义漂移必须新建Lineage；失败proposed标记abandoned |
| `AgentInstance` | Runtime Kernel | proposed Lineage、Plan Digest、预留ID/epoch；ActivationCommit时附加SandboxLeaseRef | proposed/aborted/provisioning及三维状态、ExecutionOutcome | ID不复用；proposed无Lease要求且无执行权；active路径必须已绑定Lease | 迟到Observation不改变新Instance；Commit前失败标记aborted；强制事实漂移进入Fenced/Indeterminate |
| `AgentRunRecord` | Runtime | Instance、Run命令、Run输入摘要 | Run状态、Harness声明和Execution关联 | V1每Instance最多一个活跃Run | 并发请求返回`run_conflict`；迟到终态只关联原Run |
| `SessionState` | Harness | Run内交互与不透明SessionRef | Run内Session快照/Observation | V1不跨Run或Instance承诺恢复 | Harness丢失则Run不确定；不得据Session完成推导Task成功 |
| 三维生命周期状态 | Runtime Kernel | 权威命令、Inspect、Effect/Evidence事实 | `LifecyclePhase`、`ExecutionCertainty`、`CleanupStatus` | 三维独立，不能用单一终态遮蔽Unknown | 证据冲突进入Degraded/Indeterminate/Fenced；迟到结果按epoch隔离 |
| `ExecutionFence` | 每个部署绑定的唯一Fence/Authority epoch事实所有者 | Identity/Instance/Lease/Authority epoch、Capability、effect_intent_id/revision、Payload Digest | 当前Fence事实与allow/deny依据 | 最终效果发生前重验；事件generation不替代Fence | 旧epoch拒绝；离线生效受RevocationPolicy上限约束 |
| `RevocationPolicy` | 绑定的Security/Authority Policy所有者 | risk class、验证模式、撤销延迟、时钟边界 | 可执行的在线/离线约束 | 参数必须显式配置；未配置禁用离线Effect | 策略过期/漂移Fence对应能力，不猜默认数值 |
| `ConflictEffectDomain` | 对应资源/领域事实所有者 | 资源、目标、写命名空间、预算账户 | 冲突域标识与占用事实 | 同域旧Fence未失效时新实例不得取得能力 | 占用未知则Quarantine；迟到效果继续占域直到结算 |
| `ReplacementPermit` | Runtime Control Plane依据各权威边界签发 | Fence、网络、Secret、Remote、Budget、Commit隔离证明 | 分域允许/拒绝替换 | 不是仅靠Runtime状态记录签发 | 任一冲突域隔离未知即阻断该域；证明漂移立即撤销Permit |
| `HarnessConformance` | Runtime Admission依据独立证据判定 | Manifest、能力、Effect可控性、Inspect证据 | fully/restricted/observe-only/rejected | 不可Fence的持久Effect不得描述为受治理Agent | 自报能力过期或漂移则降级/Fence/拒绝；旧认证不沿用 |
| `BindingSaga` | Runtime Kernel协调；步骤事实归对应Provider | Binding DAG、Step Intent、Authority/Fence | 每步状态、Residual和Ready资格 | Required全部独立验证才可Ready | 迟到Receipt回写原步骤；unknown/compensation failed保留残留并Fence |
| `CompensationIntent` | 原Effect领域授权者；Runtime只协调 | 原Intent/Receipt、精确补偿摘要、新Authority/Fence/Budget | Receipt或compensation unknown | 补偿是新的外部Effect，不是免费回滚 | 未授权禁止；迟到结果关联原补偿；目标漂移时重审或人工介入 |
| `ActivationSnapshot` | Runtime Admission协调、各领域提供live fact | Preflight证据、Authority、Entitlement、Route、Trust、Budget Policy/requested cap、proposed身份与Sandbox需求等 | 绑定proposed Instance ID/epoch和Requirement Digest的不可变快照；不含真实Lease或Reservation ID | 首个Activation Effect前重验全部强制事实 | TTL过期或Digest漂移则Activation失败；运行中按Policy Fence/Degrade/Rebuild |
| `EffectIntent` | 本次绑定解析出的一个Intent事实所有者 | 规范Payload摘要、目标、Authority、Fence、Budget、幂等等级 | 可派发Intent revision | 高风险Effect必须write-ahead；同一Intent身份稳定 | 未持久化不派发；迟到Receipt只结算原Intent；参数漂移使审批失效 |
| `EffectReceipt/UnknownOutcome` | 本次目标绑定解析出的一个结算事实所有者 | Intent、Provider operation/Inspect证据 | applied/not-applied/unknown及Usage | 每个Intent只有一个权威Settlement归并点；超时不等于失败 | 迟到Receipt可从Unknown转为确定；冲突证据进入Reconcile，不盲重试 |
| `BudgetAuthorityPort` | Plan绑定的预算事实所有者 | Budget policy、Reservation、Usage、Intent | Reserve/Commit/Release/Reconcile事实 | Hard Budget依赖原子Reservation；Runtime不猜价格 | Unknown保留最坏占额；迟报Usage对账；价格/额度漂移阻断新计费Effect |
| `RuntimeLedgerRecord` | 指定scope的权威Ledger | 已认证SourceEvent、Intent/Outbox、因果引用 | scope sequence、摘要和Projection引用 | 全序只在声明scope；Source签名不等于事实真实 | 重复去重、缺口显式化、旧epoch只作证据；Projection故障不丢权威Intent |
| `CheckpointManifest` | Runtime协调；各组件拥有自己的贡献状态 | Barrier、各贡献摘要、Plan/Profile/Authority/Fence兼容证据 | complete/partial Manifest与Restore资格 | Required贡献必须处于同一一致切面 | partial只供诊断；恢复漂移则拒绝/收紧；半恢复不得Ready |
| `CachePartition` | Context Engine Cache Manager | tenant、principal/authority epoch、分类、Context/Route/Model/Harness/Tool摘要 | 分区Key、CachePlan、命中证据 | 不跨租户、权限、Harness或工具可见面隐式复用 | 命中时重鉴权；任一摘要漂移失效；Provider隔离不明则敏感数据禁用缓存 |
| `ExtensionManifest` | Extension Registry/Trust责任域 | Publisher、Artifact Digest、SBOM、Schema、Capability、资源上限 | verified/revoked/expired及绑定证据 | 签名不等于发布者获信；Observer无权威append权 | Trust撤销/TTL过期Fence；版本不变但Digest漂移拒绝旧Plan |
| `RemoteContinuation` | Provider操作事实源；Runtime协调 | EffectIntent、operation ID、Fence、Retention、Budget | active/cancelled/completed/orphaned/unknown等状态 | Sandbox释放不推导远程结束 | 迟到完成结算原Intent；不可取消则明确残留并占用冲突域 |
| `ReviewVerdict` | 本次Review绑定的唯一Verdict所有者 | Candidate Digest、Policy、Reviewer Authority | accepted/rejected/conditions | 只给审核结论，不执行Commit | 过期Verdict或Candidate漂移必须重审；迟到Verdict不覆盖新revision |
| `Artifact/Memory Commit` | Candidate明确选择的一个Asset或Memory提交所有者 | Candidate Digest、Review引用、Authority、Fence、幂等键 | committed/rejected/commit unknown | Harness只产候选；Review不直接Commit；Runtime只协调 | 旧epoch迟到Commit拒绝或重新采纳；回包丢失先查原Intent |

这张表是跨文件索引；字段细节和反例分别以Safety、Admission、Effects、Evidence、Continuity、Extensions合同为准。任何对象若无法给出表中六列，不得进入实现Schema。

Fence可以有多个强制执行点（Sandbox、网络、Invoker、Tool/MCP、Cache、Commit Gateway），但它们只是读取/验证同一权威Fence事实并产出Enforcement Receipt，不共同拥有Fence epoch。执行点无法取得权威事实且Policy不允许离线时必须fail closed；执行点冲突进入EvidenceConflict。

## 3. 无循环创建与激活状态

```text
Lineage:       proposed -> active | abandoned
Instance:      proposed -> provisioning -> ... -> terminal
                       \-> aborted（仅ActivationCommit前）
IdentityLease: requested -> reserved -> active -> revoked | expired | released
SandboxLease:  requested -> reserved_quarantined -> active -> releasing
                                             \-> released | failed | indeterminate
```

创建依赖方向固定为：Plan/Identity → proposed Lineage → proposed Instance → Bounded Preflight → ActivationSnapshot → reserved Identity Lease → Budget Reservation（或not_required）→ reserved_quarantined SandboxLease → ActivationCommit → active SandboxLease。Snapshot只绑定proposed身份、Sandbox Requirement Digest与Budget requested cap，不绑定真实Lease/Reservation ID；Instance在proposed阶段也不要求SandboxLease输入。ActivationCommit是首次把实际Reservation/LeaseRef写入Instance并让Identity Lease变为active的线性化点。

## 4. Lineage与Plan

```text
AgentIdentity
  └── InstanceLineage(plan_digest=A)
      ├── Instance I1, epoch=1
      ├── Instance I2, epoch=2
      └── Instance I3, epoch=3
```

同一Plan下的崩溃、节点替换或Lease重建产生同Lineage的新Instance和更高epoch。以下变化创建新Resolved Plan与新Lineage：

- Agent Definition版本；
- 任一最终Profile摘要；
- Harness ID、版本或二进制摘要；
- Model、Route、Provider、Offering或Execution Surface；
- 强制Sandbox能力、Context语义或工具可见面；
- Authority范围扩大。

Authority缩小或撤销会Fence当前实例；同一SecretRef背后的短期凭据轮换属于live fact，不改变Plan语义。新Lineage可以记录`supersedes_lineage_id`，但不能复用旧Lineage掩盖升级。

## 5. Identity级唯一执行权

V1一个`AgentIdentity`最多有一个`reserved`或`active`的`IdentityExecutionLease`：

```text
identity_id
lineage_id
activation_attempt_id
state: reserved | active
identity_epoch
authority_epoch
expires_at
```

多个历史Lineage可以存在。reserved Lease只允许同一ActivationAttempt的Sandbox申请、最终重验和清理；只有active Lease指向的Lineage可以获得一般Effect授权、Memory/Artifact正式写权和Run Budget Reservation。无法访问唯一线性化Lease事实源时不能新建、激活或续租。

## 6. Run与Session

- V1每个Instance最多一个活跃Run；新Run遇到活跃Run返回`run_conflict`，不隐式排队；
- Run Record由Runtime拥有，并通过线性化`RunFactPort`持久化；进程内Registry不能作为24×7事实源；
- Run的创建、停止与终态使用revision CAS；写入成功但回包丢失时必须先Inspect，不能盲目创建第二个Run或覆盖终态；
- 相同终态结算可以按已持久事实幂等返回，不同终态Claim发生冲突时第一份线性化结果保持不变；
- Harness Completion Event必须先验证Opaque Payload摘要、Instance epoch和source sequence，再幂等写入Evidence并关联不可变`RunCompletionClaim`；同一source sequence换内容产生`evidence_conflict`；
- Claim摄取不改变Run状态和`ExecutionOutcome`。只有完成独立Execution Inspect、必需Effect结算与Fence核对后，Runtime才能CAS提交终态；
- Harness拥有Interaction Loop和Run内Session State；
- V1不承诺跨Run或跨Instance Session恢复；
- Harness的completed/end只形成`HarnessCompletionClaim`；
- Runtime根据执行、Fence、Effect和清理状态形成`ExecutionOutcome`；
- 必需Effect存在UnknownOutcome时，Runtime只能报告相应`ExecutionOutcome`与关联的Effect不确定性；业务/Task所有者据此独立判断TaskOutcome。

## 7. 结果维度

| 维度 | 候选值 | 所有者 |
|---|---|---|
| `ExecutionOutcome` | completed、cancelled、failed、lost、indeterminate、needs_reconciliation | Runtime |
| `EffectSettlement` | settled、partial、unknown、compensated、reconciliation_required | 每个Effect绑定的唯一Settlement所有者 |
| `ReviewVerdict` | pending、accepted、rejected、conditions | Review Verdict所有者 |
| `ArtifactPublicationStatus` | candidate、commit_dispatched、committed、commit_unknown | 选定的Asset或Memory Commit所有者 |
| `TaskOutcome` | achieved、not_achieved、blocked、unknown | 每个Task绑定的唯一Task/Management所有者 |
| `GoalOutcome` | accepted、not_met、abandoned、unknown | 每个Goal绑定的唯一Goal所有者或用户 |

这些结果只能关联，不能折叠为一个`Succeeded`。

## 8. 事件关联键

相关记录至少携带：

- `agent_identity_id`与`identity_epoch`；
- `instance_lineage_id`与`resolved_plan_digest`；
- `instance_id`与`instance_epoch`；
- `sandbox_lease_id`与`lease_epoch`；
- `run_id`、`task_id`、`session_ref`（适用时）；
- `effect_intent_id`或`remote_continuation_id`（适用时）；
- `event_id`、ledger scope/sequence、source identity/sequence、causation ID；
- actor、authority epoch、capability digest和Fence。

## 9. 最低反例

- `OBJ-01`：Harness摘要变化但试图在同Lineage重建，必须拒绝并要求新Plan；
- `OBJ-02`：两个Control Plane同时激活同Identity，只能一个取得IdentityExecutionLease；
- `OBJ-03`：旧epoch的Pause作用于新Instance，返回`stale_instance_epoch`；
- `OBJ-04`：Harness报告完成但必需Effect未知，Execution只能是`indeterminate/needs_reconciliation`；
- `OBJ-05`：Artifact被Review拒绝时，Runtime不得把Task标为成功。
- `OBJ-07`：proposed Instance被要求先提供SandboxLease ID时必须拒绝循环Schema；
- `OBJ-08`：reserved Identity Lease尝试提交业务Effect时必须拒绝，只允许激活/清理范围。
- `FENCE-04`：两个执行点持有不同Fence epoch时，旧epoch执行点必须拒绝并上报冲突，不能各自声明权威；
