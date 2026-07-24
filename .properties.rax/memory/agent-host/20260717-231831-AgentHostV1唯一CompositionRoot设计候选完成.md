# Agent Host V1 唯一 Composition Root 设计候选完成

## 事件

用户确认由独立 `agent-host` 拥有唯一 Composition Root。设计候选冻结了 Host API/CLI、启动和反向停止时序、factory exact 注册、禁止依赖、Sandbox 硬前置，以及全部 6+1 production 后才能发布 `SystemReady` 的边界。

## 当前状态

- 新模块设计候选已落盘，等待用户确认。
- 仓库当前没有 production root，本事件不代表系统已可一键启动。
- 未进入 plan，未创建 `ExecutionRuntime/agent-host` 代码。

## 关联资产

- [设计入口](../../design/agent-host/README.md)
- [Composition Root](../../design/agent-host/composition-root-v1.md)
- [验收](../../design/agent-host/acceptance.md)
