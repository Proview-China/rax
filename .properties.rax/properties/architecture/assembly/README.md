# 定义与装配设计域

## 1. 目标

把一组可独立演进的 Agent 配件，确定性地解析为 Runtime 可以接纳的不可变 `ResolvedAgentPlan`。装配过程发生在实例创建之前，不在 Runtime 运行期间临时猜测依赖。

## 2. 当前模块

| 模块 | 职责 | 不得承担 |
|---|---|---|
| [`agent-definition`](../../../design/agent-definition/README.md) | 声明 Agent 需要什么 | 解析真实凭据、启动进程 |
| [`profile-system`](../../../design/profile-system/README.md) | 解析可组合 Profile 和覆盖关系 | 绕过 Provider、组织或权限合同 |
| [`agent-assembler`](../../../design/agent-assembler/README.md) | 解析版本、能力和依赖，生成装配计划 | 创建沙箱、运行 Harness |
| [`harness`](../../../design/harness/README.md) | 提供可启动的 Agent 执行外壳合同 | 取代 Runtime 或其他引擎 |

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

## 4. 设计边界

- 装配失败必须发生在Sandbox分配和任何外部副作用之前；
- 同一输入和同一事实版本必须产生相同装配结果；
- Runtime只消费已解析计划，不替Agent补齐缺失设计；
- 计划中的权限与Provider合同不能被后续Profile覆盖。

## 5. 待共同决定

- Definition和Profile的版本、继承与迁移规则；
- 装配结果是单文件Manifest、对象图还是两者同时提供；
- 可选组件缺失时允许降级还是必须拒绝；
- 装配计划签名、摘要和供应链证据方式。

