# AgentPackage Repository + Exact Loader V1 计划

## 状态

Repository 基线与 Package→Harness Publication exact historical closure 已完成；分支 `agent/package-publication-loader`，Draft PR #34，基线 `origin/main@0b059422`。当前等待独立审查。

## 实施项

- [x] 冻结 create-once、exact read、lost-reply 与 Loader closure 边界；
- [x] 新增窄 Repository/Reader ports 与 immutable verified closure；
- [x] 实现 SQLite WAL AgentPackage store；
- [x] 实现原 ref 限定的 lost-reply recovery；
- [x] 实现 Generation/Manifest/Graph/Handoff exact Loader；
- [x] AgentPackage lock 增加 exact `AssemblyPublicationRefV2`；Compiler 使用真实 `NewAssemblyPublicationBundleV2`；
- [x] 将四个松散 artifact Reader 收敛为单个 Harness HistoricalReader-compatible port；
- [x] Loader 重读 exact Package + exact historical Publication Bundle 并验证完整闭包；
- [x] 覆盖真实 Harness SQLite Owner + Package SQLite Store restart、splice、missing/uncommitted 黑盒；
- [x] ordinary、race、vet、diff-check；
- [x] 独立 Draft PR #34。

## Stop Gate

Host、Runtime、Factory activation、Console、TS DTO、跨 Owner artifact repository、production/SystemReady 均留给后续独立合同。
