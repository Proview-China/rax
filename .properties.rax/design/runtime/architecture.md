# Runtime 总体架构与责任边界

## 1. 系统位置

```text
REPL / Remote API / User SDK
              |
              v
Application Facade（跨领域门面，不属于Runtime内核）
              |
              v
Profile Compiler + Agent Assembler
              |
              v
ResolvedAgentPlan
              |
              v
Runtime Control Plane
  - IdentityExecutionLease / Command Log / Desired State
  - Static Admission / Activation协调 / Instance Registry
              |
              v
Runtime Kernel（每个具体AgentInstance）
  - 三维状态 / Reconcile / Binding Saga
  - Harness监督 / Effect协调 / 清理与证据关联
              |
              v
Versioned Ports + Capability/Fence/Evidence
  - Harness / Model Invoker / Context+Cache / Tool+MCP
  - Memory / Knowledge / Asset / Review / Management
  - Sandbox / Budget Authority / Evidence Store
```

Application Facade只统一资源引用、命令、查询、Operation和Watch语义；每个领域仍由自己的所有者处理。Runtime不是万能API层。

## 2. 核心数据流

```text
ResolvedAgentPlan
  -> Static Admission（纯本地、无外部Effect）
  -> 创建proposed Lineage与proposed Instance身份（纯本地，不获得执行权）
  -> Bounded Preflight（有界探测、显式Probe预算，关联proposed Instance）
  -> ActivationSnapshot冻结live facts与Sandbox需求（不引用尚不存在的Lease）
  -> 取得reserved IdentityExecutionLease（排他但只允许激活Effect）
  -> 取得适用的Budget Reservation（有限可执行cap；或明确not_required）
  -> 通过EffectIntent分配reserved + quarantined SandboxLease
  -> 最终重验并线性化ActivationCommit
     （Identity Lease active、Lineage active、Instance provisioning、记录SandboxLeaseRef）
  -> 激活SandboxLease并保持Fence约束
  -> 执行Binding Saga
  -> 独立Inspect后进入Ready
  -> 单一活跃Run
  -> EffectIntent / Receipt / UnknownOutcome
  -> Stop / Fence / RemoteContinuation协调
  -> 分维度报告执行、清理、远程状态与Effect结算
```

Static Admission和proposed身份创建无外部副作用；因此Instance从`preflighting`起已有稳定proposed ID/epoch。Preflight可能访问外部元数据，但不能留下未结算持久资源、发送用户Prompt或触发业务效果；Preflight未知或未清理时不得形成可激活Snapshot。`ActivationSnapshot`只冻结预算Policy与requested cap，不虚构尚未执行的Reservation。Identity Lease、Budget Reservation与Sandbox Lease依次进入可回收的reserved/quarantined状态，只有最终`ActivationCommit`才能授予一般执行权。任一步失败都按已创建资源逆序Fence/释放，未知结果保留Quarantine、占额与残留证据。

## 3. 五个事实面

| 事实面 | 内容 | 权威所有者 |
|---|---|---|
| 定义面 | Definition、Profile、Resolved Plan | Definition/Profile/Assembler |
| 控制面 | Command、Desired State、Identity Lease、Fence Epoch | 每个绑定解析到一个线性化Control/Authority事实所有者 |
| 执行面 | Harness循环、模型调用、工具、Sandbox进程 | 对应执行组件 |
| 效果意图面 | EffectIntent与dispatch revision | 每个Effect绑定的唯一Intent事实所有者 |
| 效果结算面 | Receipt、Provider Operation与Settlement | 该目标领域或Provider的唯一结算事实所有者 |
| 证据面 | 权威Intent、Source Observation、Attestation、Projection | 各领域事实源；Runtime关联 |

签名只提供Provenance。Runtime必须区分Observation、Attestation和Authoritative Fact，不得把Harness自报提升为真实世界事实。

## 4. 对象关系

```text
AgentIdentity
  1 -> 0..1 active IdentityExecutionLease（V1）
  1 -> N historical InstanceLineage

InstanceLineage
  1 -> 1 ResolvedAgentPlanDigest
  1 -> N AgentInstance（按epoch顺序替换）

AgentInstance
  1 -> 1 active SandboxLease（V1）
  1 -> 0..1 active AgentRun（V1）
  1 -> N EffectIntent / RemoteContinuation / EvidenceRecord
```

## 5. 失败真实性

Runtime不使用单一`Succeeded/Failed`覆盖所有结果。至少分别报告：

- `ExecutionOutcome`：Runtime拥有；
- `EffectSettlement`：每个Effect绑定的唯一结算事实所有者拥有；
- `RemoteContinuationStatus`：Provider Operation事实拥有；
- `ReviewVerdict`：Review所有者拥有；
- `ArtifactPublicationStatus`：选定的Asset或Memory提交所有者拥有；
- `TaskOutcome`和`GoalOutcome`：分别由每个Task/Goal绑定的唯一所有者拥有；
- `CleanupStatus`：各资源所有者贡献，Runtime聚合。

## 6. 未来集群边界

公共语义只使用`NodeRef`、`PlacementRef`、`ResourceClaim`、`SandboxLeaseRef`、Fence和Capability，不把本机PID、容器ID或宿主路径作为公共身份。网络分区时无法访问唯一线性化事实源的一侧只能提供带watermark的stale read或执行收紧权限的本地紧急动作。
