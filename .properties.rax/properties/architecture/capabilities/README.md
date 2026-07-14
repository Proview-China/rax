# 能力依赖设计域

## 1. 目标

为 Agent 提供模型执行之外的上下文、行动、记忆、知识和资产能力。每项能力独立设计，通过版本化挂载合同进入 Runtime，不直接取得实例生命周期所有权。

## 2. 当前模块

| 模块 | 核心职责 |
|---|---|
| [`context-engine`](../../../design/context-engine/README.md) | 编译用户定义提示、任务上下文和可解释上下文包 |
| [`tool-engine`](../../../design/tool-engine/README.md) | 管理工具定义、执行请求、权限门禁和结果合同 |
| [`mcp-gateway`](../../../design/mcp-gateway/README.md) | 管理MCP连接、能力发现、会话和外部Server边界 |
| [`memory-engine`](../../../design/memory-engine/README.md) | 保存、检索和演进Agent可持久记忆 |
| [`knowledge-engine`](../../../design/knowledge-engine/README.md) | 管理前置事实、来源、版本和可追溯知识投影 |
| [`asset-manager`](../../../design/asset-manager/README.md) | 接收、暂存、审核和发布Agent产物 |

模型调用由既有 `model-invoker` 负责；Harness执行外壳由 `harness` 设计域负责。

## 3. 共同挂载要求

- 每个模块暴露能力描述、版本、健康状态和失败分类；
- Runtime只保存稳定引用和实例绑定，不理解模块内部算法；
- 所有外部效果在执行前接受Authority和Policy门禁；
- Memory、Knowledge和正式Asset位于可销毁Sandbox之外；
- Context包、Tool结果和知识引用必须保留来源与摘要；
- 模块失效不得引发静默fallback或跨租户串线。

## 4. 待共同决定

- 六类能力的最小公共挂载协议；
- 同步调用、事件流和异步任务的统一边界；
- 能力健康变化如何触发Runtime暂停、降级或重建；
- 本地Sidecar、同Sandbox进程和远程服务的部署表达。

