# Continuity ComponentReleaseV1 Delta

状态：**reference-only assembly candidate / production NO-GO**。

## 本轮闭合

- Continuity Owner 通过 Agent Assembler 公共 `ComponentReleaseV1` 与 publisher/reader ports 发布声明式候选；不复制公共类型。
- Manifest、Module、Capability、Port、Factory descriptor、effect/settlement/cleanup owners、artifact、candidate certification、evidence 与 TTL 精确闭合。
- `Ensure` 回包未知时不重派，只按候选 exact ref Inspect；内容漂移失败关闭。
- Factory 只有 descriptor；Continuity 不导入 Host、Assembler repository、Harness实现或其他 Owner 实现。

## production 门禁

SQLite WAL/FULL 元数据与可选 RocksDB Chunk 已形成真实 owner-local durable store，但它们不证明 production deployment，也不授予 Runtime Checkpoint consistency 或 Restore eligibility。

production release 必须由同一 current/certification cut 精确证明：durable checkpoint/timeline/artifact/history/restore stores、current indexes、真实 remote blob provider、Participant capture、Restore execute、cleanup/purge/archive治理、deployment/root attestation。当前 capture、Provider、Restore execute、cleanup/deployment root 均不存在，既有 `conformance.Wave1Manifest` 也强制 `reference_only=true`、`ProductionSLA=false`，所以没有 production promotion API。

唯一后续 Delta 是跨 Owner Checkpoint/Restore/Provider/Cleanup composition 与 deployment qualification；不得用 memory/fake、SQLite文件存在、RocksDB build tag或 owner-local测试自签 production。
