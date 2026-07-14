# Runtime Plan入口

## 当前状态

- 当前版本：通过独立文件复审并正在分切面落地的Runtime V1 Plan；
- 状态：P0.1至P0.5与P0.6 Runtime公共治理入口已完成；当前继续Application/Harness组合闭环与最终解锁门禁，尚未解锁6+1并行实现；
- 旧版本：2026-07-14早期Plan草稿及两次文件复审修正前候选已由本候选完全替换，不再有效；
- 设计事实源：[Runtime设计入口](../../design/runtime/README.md)。

## 当前候选

- [Runtime V1执行治理核心计划](./runtime-v1.md)

## 边界

已完成切面采用Go module，落点为`ExecutionRuntime/runtime`。数据库、协议、进程拓扑、生产Sandbox、SLA以及相邻组件真实实现仍未决定，未经后续确认不得引入。
