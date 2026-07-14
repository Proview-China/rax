# Runtime原始图与中文执行说明

## 1. 历史Grounding

[Agent核心结构图](../../model-invoker/grounding/agent-core-overview.png)只作为早期组成视图，不是Runtime最终合同，也不表示Runtime属于Model Invoker。

## 2. 当前图组

| 图 | 表达内容 | 原始文件 | 渲染图 |
|---|---|---|---|
| 系统上下文 | 使用入口、装配、Runtime、Sandbox、能力与治理 | [drawio](./runtime-system-context.drawio) | [png](./runtime-system-context.png) |
| 对象与状态 | Identity、Lineage、Instance、Run、所有权与三维状态 | [drawio](./runtime-identity-lifecycle.drawio) | [png](./runtime-identity-lifecycle.png) |
| Fence与控制 | Revocation、Conflict Domain、ReplacementPermit、命令线性化与CAP | [drawio](./runtime-fence-control.drawio) | [png](./runtime-fence-control.png) |
| Admission与Saga | Static Admission、Preflight、ActivationSnapshot、Binding Saga与Compensation | [drawio](./runtime-admission-saga.drawio) | [png](./runtime-admission-saga.png) |
| Effect与证据 | 统一Effect、Budget、Receipt、UnknownOutcome、事实层级与Write-ahead | [drawio](./runtime-effect-evidence.drawio) | [png](./runtime-effect-evidence.png) |
| 连续性与远程状态 | Checkpoint、Cache Partition、RemoteContinuation、Artifact/Memory Commit | [drawio](./runtime-continuity-remote.drawio) | [png](./runtime-continuity-remote.png) |

图组把十二个主题合理合并为六张高信息密度图。每张图的对象顺序、箭头、基数和条件都必须与文字合同一致；不能用“图只是简化”掩盖冲突箭头。文字合同提供完整字段，图提供不可矛盾的主关系。

## 3. 中文执行解读

### 对象与状态

Lineage和Instance先以proposed身份创建，Snapshot不引用真实Lease/Reservation；Identity Lease、适用Budget Reservation与Sandbox Lease依次reserved，ActivationCommit后才active。每个Lineage有1..N历史Instance，每个Instance各自拥有0..1活跃Run、自己的SandboxLease和一个包含Lifecycle、Execution Certainty、Cleanup的正交状态向量；三维互不作为对方来源。Fenced不等于进程死亡，Terminal不等于完全清理。

### Fence与控制

命令先在线性化事实源排序，安全命令支配推进命令。最终效果边界校验Fence；离线撤销按显式Policy上限生效。ReplacementPermit按冲突Effect Domain阻止新旧实例并发写。

### Admission与Saga

Static Admission无外部Effect；随后先创建proposed Lineage/Instance，再以该稳定身份执行有界Preflight。无循环Activation继续按Snapshot、reserved Identity Lease、适用Budget Reservation、reserved_quarantined Sandbox Lease、ActivationCommit和active Sandbox Lease推进。Snapshot只记录Budget Policy/requested cap，不虚构真实Reservation。跨系统绑定是可恢复Saga；只有失败/取消且显式策略允许时才发起独立受权Compensation Effect，Unknown Effect阻止Ready。

### Effect与证据

模型Context外发、费用、资源、Hosted Tool、Provider Cache和正式提交都属于Effect。派发前持久化Intent并Reserve Budget，之后取得Receipt或进入UnknownOutcome。Provenance、Observation和Attestation互不自动升级；Authoritative Fact只来自唯一领域所有者或合同允许的独立Inspect。

### 连续性与远程状态

Checkpoint用统一Effect Barrier/Watermark形成一致集合；Cache按Tenant/Authority/Context/Route/Harness/Tool分区。Sandbox释放不结束Provider Session/Batch/Cache；Review Rejected终止提交支路，只有Accepted候选可在当前Fence/Authority下创建正式Commit Intent并取得Receipt。
