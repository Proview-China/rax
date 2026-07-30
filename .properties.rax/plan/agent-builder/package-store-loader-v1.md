# AgentPackage Repository + Exact Loader V1 计划

## 状态

实现、验证与独立 Draft PR #29 已完成；目标分支 `agent/agentpackage-store-loader`，基线 `origin/main@13f63712`。当前等待独立审计，不合并。

## 实施项

- [x] 冻结 create-once、exact read、lost-reply 与 Loader closure 边界；
- [x] 新增窄 Repository/Reader ports 与 immutable verified closure；
- [x] 实现 SQLite WAL AgentPackage store；
- [x] 实现原 ref 限定的 lost-reply recovery；
- [x] 实现 Generation/Manifest/Graph/Handoff exact Loader；
- [x] 覆盖 restart、concurrent create、splice、body/lock drift 与缺 artifact 黑盒；
- [x] ordinary、race、vet、diff-check；
- [x] 独立 Draft PR #29 已创建；保持 Draft，等待独立审计后再决定 merge。

## Stop Gate

Host、Runtime、Factory activation、Console、TS DTO、跨 Owner artifact repository、production/SystemReady 均留给后续独立合同。
