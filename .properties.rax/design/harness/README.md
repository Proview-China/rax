# Harness共用合同与Runtime交接

## 1. 状态

- 当前阶段：Harness公共合同与组件中立最小骨架已完成实现和验收；
- 当前接线：Application V3持久Domain Adapter与公共Run Claim→Settlement/Termination协调边界已闭合；Claim不升级为Outcome，生产Provider/Backend仍未选择；
- 实现授权：配置交接、Run内Kernel、窄依赖Port、Runtime ExecutionPort Adapter、fake与完整闭环测试；
- 明确不授权：具体官方/第三方Harness生产实现、真实模型调用、真实Tool/MCP、生产进程拓扑和持久后端；
- 核心原则：每条Harness Route必须满足或明确降级于同一外部合同，不要求内部复用同一代码。

## 2. 共用组成

每条Route提供或等价实现：Bootstrap、Run Controller、Interaction Loop、Run内Session、Model Turn、Tool协调、Context/CachePlan接入、Result/Artifact Candidate、Event、Control、Checkpoint（可选）、Error/Backpressure、Health Observation和Cleanup Observation。

Harness不拥有Identity、Runtime Run Record、Effect终态、正式Memory/Asset、Review决定、Budget事实或Sandbox Lease。

## 3. V1所有权

- Runtime拥有Run Record与Execution Outcome；
- Harness拥有Interaction Loop和当前Run内Session State；
- V1不承诺跨Run/跨Instance Session；
- Harness completed/end只形成Completion Claim；
- Harness Ready/Cleaned/EffectFailed都是Observation，需要独立验证；
- V1每Instance最多一个活跃Run。

## 4. Conformance

采用`fully_controlled`、`restricted_controlled`、`contained_observe_only`和`rejected`四级。受治理路线最低要求所有持久Effect受权且可Fence。不可拦截网络、长期明文Secret或Opaque Hosted Tool可能直接导致rejected。

Pause和Checkpoint是可选Capability；未支持时必须从API和Profile中明确移除，不得模拟成功。

## 5. Effect与Secret

模型Context外发、费用、Hosted Tool、Provider Session、Cache和工具活动都进入统一Effect合同。V1受治理Harness不得获得长期可重用明文Secret；优先使用SecretRef、Brokered Capability或受策略约束的短期Credential。

## 6. 进入Harness自身Plan的门槛

- Run内状态机、取消、错误和背压合同；
- 至少两种内部机制不同的Harness通过同一合同测试；
- Conformance、Ready独立验证和Effect/Fence边界；
- 崩溃、迟到、Remote Continuation和清理反例；
- 用户确认首个Harness实现范围。

以上门槛已经由用户确认进入“公共合同与最小骨架”切面；具体生产Harness仍需另行确认。

## 7. 当前设计资产

- [配置编译与Bootstrap交接](./configuration/README.md)
- [公共对象、Port与所有权](./contracts/README.md)
- [Run内Kernel与Interaction Loop](./kernel/README.md)
- [Runtime ExecutionPort接入](./runtime-integration/README.md)
- [首切面验收门禁](./acceptance/README.md)

实现与模块说明：

- [Harness Go模块](../../../ExecutionRuntime/harness/README.md)
- [Harness模块说明](../../module/harness/README.md)
