# 定义与装配设计域

## 1. 目标

把一组可独立演进的 Agent 配件，确定性地解析为 Runtime 可以接纳的不可变 `ResolvedAgentPlan`。装配过程发生在实例创建之前，不在 Runtime 运行期间临时猜测依赖。

## 2. 当前模块

| 模块 | 职责 | 不得承担 |
|---|---|---|
| [`agent-definition`](../../../design/agent-definition/README.md) | 声明 Agent 需要什么 | 解析真实凭据、启动进程 |
| [`profile-system`](../../../design/profile-system/README.md) | 解析可组合 Profile 和覆盖关系 | 绕过 Provider、组织或权限合同 |
| [`agent-assembler`](../../../design/agent-assembler/README.md) | 解析版本、能力和依赖，生成装配计划 | 创建沙箱、运行 Harness |
| [`harness`](../../../design/harness/README.md) | 提供可启动的Agent执行外壳合同；公共最小骨架已实现 | 取代Runtime或其他引擎 |
| [`agent-builder`](../../../design/agent-builder/README.md) | 复用 Definition→ResolvedPlan→Harness Graph，封装不可变 AgentPackage/lock | 新增平行 Definition、Host/Runtime/Loop 或 Console DTO |

## 3. 核心产物

`ResolvedAgentPlan` 至少钉住：

- Agent Definition版本与摘要；
- Harness及模型执行Route/Profile；
- Context、Tool、MCP、Memory、Knowledge和Asset引用；
- AgentIdentity与组织职权引用；
- 审核、管理和外部控制策略；
- Sandbox需求、资源预算和网络边界；
- 所有组件版本、能力声明和兼容性结论；
- 只包含秘密引用，不包含秘密明文。

首个 `AgentPackageV1` 公共合同在其 lock 中继续钉住 Definition、Resolution Facts、Catalog、Component Releases、Binding/Assembly digest 与 Generation/Manifest/Graph/Handoff exact refs；它是编译 provenance，不是 activation 或 production 资格。

## 4. 设计边界

- 装配失败必须发生在Sandbox分配和任何外部副作用之前；
- 同一输入和同一事实版本必须产生相同装配结果；
- Runtime只消费已解析计划，不替Agent补齐缺失设计；
- 计划中的权限与Provider合同不能被后续Profile覆盖。

## 5. 待共同决定

- Definition和Profile的版本、继承与迁移规则；
- AgentPackage 的持久仓、加载协议与 Host handoff 由后续独立合同决定；
- 可选组件缺失时允许降级还是必须拒绝；
- 装配计划签名、摘要和供应链证据方式。
