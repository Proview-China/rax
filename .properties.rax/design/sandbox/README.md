# Sandbox 设计骨架

## 设计状态

- 当前阶段：设计草案。
- 当前授权：仅允许共同讨论和完善设计，不存在实现授权。
- 本文件不是冻结合同，也不代表已经创建 Sandbox 实现模块。
- V1已确认基线：一个AgentInstance独占一个SandboxLease，禁止活跃实例间共享。

## 作用

Sandbox 为 AgentInstance 提供可租用、可验证、可回收的隔离执行环境。
它负责把资源、文件、网络、进程、秘密引用和执行边界落实为现实约束。
Sandbox 是 Runtime 之下的承载底座，不是 Agent 内部能力，也不是 Harness 本身。

## 后端中立原则

公共合同不得写死 Docker、Podman、进程、虚拟机、MicroVM 或远程 Worker。
调用方声明所需隔离效果，不直接选择某个后端命令或实现细节。

## 核心输入

- `SandboxRequest`：Instance 身份、隔离需求、资源预算和生命周期意图。
- `SandboxRequirement`：文件、网络、进程、设备、系统调用和持久化要求。
- `AuthorityGrantSetRef`：Organization Engine 编译出的有效职权引用。
- `ArtifactMountRef[]`：只读或可写资产、工作区和产物挂载要求。
- `SecretRef[]`：由秘密管理面解析的引用，不携带明文秘密。

## 核心输出

- `SandboxLease`：租约身份、所有者、期限、能力和状态。
- `SandboxEndpoint`：供 Runtime 启动或连接 Harness 的受控端点。
- `IsolationAttestation`：实际隔离后端、策略摘要和验证证据。
- `SandboxEvent[]`：分配、启动、异常、回收和清理事件。
- `ReleaseReport`：终止、清理、残留和不可确认状态报告。

## 本模块拥有

- SandboxLease 的分配、续期、撤销、回收和终态报告。
- 隔离后端适配、能力协商和实际约束落实。
- 租约内资源上限、文件边界、网络边界和进程边界。

## 本模块不拥有

- 不拥有 Agent 业务逻辑、Prompt、Context 或模型调用语义。
- 不拥有 Runtime 状态机、调度目标或 Harness 执行循环。
- 不解析明文 Credential，不自行扩大挂载、网络或设备权限。
- 不把容器启动成功等同于 AgentInstance 已经 Ready。

## 与 Runtime 的关系

Runtime根据ResolvedAgentPlan向Sandbox请求租约，并验证返回能力是否满足要求。
Runtime 只有在租约有效且隔离证明通过后，才可绑定并启动 Harness。
Runtime 验证 Management 的 ControlIntent 后，调用 Sandbox 续期、冻结或释放能力。
租约失效、后端崩溃或隔离不确定时，Runtime 必须进入失败或协调流程。

## V1独占租约基线

- 映射关系为 `1 AgentInstance -> 1 active exclusive SandboxLease`。
- 一个Instance内多个AgentRun如何使用同一租约，仍需生命周期设计决定。

## 依赖

- Runtime 的 AgentInstance、状态机、租约和控制结果合同。
- Organization Engine 的 AuthorityGrantSet。
- Artifact、Secret、网络策略、观察和 Evidence 基础设施。

## 待共同决定

- 租约与AgentInstance、AgentSession、AgentRun的精确生命周期关系。
- 工作区持久化、快照、恢复和销毁的默认策略。
- 网络默认拒绝范围、出站代理和动态授权方式。
- `indeterminate` 副作用和清理不完全时的处置方式。

## 进入 Plan 阶段的门槛

- 冻结 SandboxRequest、SandboxLease、Attestation 和 ReleaseReport 的 v1 候选合同。
- 确认 v1 独占租约、租约期限、续期、撤销和失联语义。
- 冻结文件、网络、进程、资源和秘密引用的最小隔离要求。
- 获得用户对范围、产物和实施顺序的明确审核与授权。
