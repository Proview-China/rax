# Praxis 总体架构索引

## 1. 状态

- 当前阶段：总体架构设计继续推进；Runtime设计资产通过独立文件复审，公共合同与组件中立最小基座已完成；
- 当前重点：`harness`公共骨架已经接入`runtime`，下一步按用户选择逐个接入真实组件；
- 当前授权：Runtime与Harness公共合同、Kernel、Adapter、fake Port、测试和说明资产已实现；不允许生产后端、具体Harness、真实外部集成或相邻组件内部实现；
- 最近更新：2026-07-14。

本目录描述 Praxis 的项目级总体架构。它只负责把设计域、模块关系和推进顺序组织成索引，不替代各模块自己的 `design/<模块名>/` 设计事实源。

## 2. 已确认的总体原则

1. Praxis 不修改模型权重、不提取隐藏推理、不静默篡改模型输入输出；系统干预的是执行过程，不是 AI 的内部认知。
2. `AgentIdentity` 持久存在，`AgentInstance` 可以替换，`AgentRun` 是更短暂的一次执行。
3. Runtime 拥有实例生命周期、挂载、状态协调、事件和控制命令，不拥有 Context、Memory、Tool、Review 等引擎的内部语义。
4. V1 中一个 `AgentInstance` 独占一个 `SandboxLease`；身份、记忆和正式资产必须位于可销毁沙箱之外。
5. 所有外部干预必须通过可认证、可授权、可追溯的控制协议进入 Runtime，不允许后门式修改。
6. Harness具有所有路线必须满足的共用行为合同，具体实现可由Praxis、官方Harness或第三方Adapter提供。
7. 组件通过Capability、Provider和命名空间扩展接入，不能把官方Sandbox或单机实现写死进Runtime。
8. REPL、API与SDK共享同一Application Command API；Context Engine内置Cache Manager并与Model Profile和Invoker协作。

## 3. 总体分层

```text
定义与装配面
  Agent Definition -> Profile System -> Agent Assembler
                         |
                         v
                 Resolved Agent Plan
                         |
                         v
能力与执行依赖面 -------------------------------+
  Harness / Model Invoker / Context / Tool / MCP |
  Memory / Knowledge / Asset                     |
                                                  v
执行面                                      Praxis Runtime
  Runtime Kernel <-> Runtime Control Plane <-> Sandbox
                                                  ^
                                                  |
治理面 -------------------------------------------+
  Organization / Review / Management
```

## 4. 设计域

| 设计域 | 说明 | 入口 |
|---|---|---|
| 定义与装配 | 声明 Agent、解析 Profile、形成不可变装配计划 | [assembly](./assembly/README.md) |
| 能力依赖 | Context、Tool、MCP、Memory、Knowledge和正式资产 | [capabilities](./capabilities/README.md) |
| 核心执行 | Runtime实例生命周期、调度协调与沙箱租约 | [execution](./execution/README.md) |
| 治理控制 | 组织职权、审核判断、管理意图和外部控制 | [governance](./governance/README.md) |

## 5. 模块状态

| 模块 | 当前状态 | 说明 |
|---|---|---|
| `model-invoker` | 已有离线实现候选 | Runtime未来挂载的模型执行依赖之一，不是Runtime本身 |
| `runtime` | 公共合同与最小可运行基座已实现 | Activation容灾、监督Policy、Timeline/Checkpoint/Restore、装配门禁、Foundation闭环和版本化Port已落地；生产门禁继续关闭 |
| `harness` | 公共合同与最小可运行骨架已实现 | 不可变Bootstrap、单Run交互循环、Action Gateway和Runtime ExecutionPort接线已落地；具体生产Harness继续关闭 |
| 其他设计域 | 设计入口草案 | 仅固定初步职责和边界，等待逐模块共同设计 |

Runtime 的当前设计事实源见 [runtime设计入口](../../design/runtime/README.md)。

## 6. 当前推进顺序

1. Runtime组件中立公共合同、Kernel、线性化事实和最小Foundation闭环已经完成；
2. 保留现有`model-invoker`，未来只通过版本化Port和获批Adapter接入；
3. 具体Harness、Context Engine（含Cache）、工具链和审批链按用户后续指导依次设计与实现；
4. 每个组件先完成所有权、Port、Effect、Evidence和失败合同，再进入内部实现；
5. 未确认的数据库、Transport、进程拓扑、真实Sandbox和外部集成保持禁用。
