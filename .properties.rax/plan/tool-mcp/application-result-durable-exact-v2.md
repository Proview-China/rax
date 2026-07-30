# Application Result Durable Exact V2 计划

状态：Application durable exact 子切片完成；完整 B1 BLOCKED。

## 已完成

- [x] Application result exact record 与 original-ref read-only Port。
- [x] 现有 `ApplicationResultStoreV2` record 复用该 exact record。
- [x] SQLite 实现真实 Adapter close create-once seam。
- [x] request key 与 original result ref 共用同一 canonical row。
- [x] 64 并发同 ref byte-identical、SQLite restart、splice/异 ref、lost create reply recovery。
- [x] exact read 期间 Tool execute 增量为零。

## 未授权且未实现

- [ ] ToolResult/DomainResult/Apply durable exact closure。
- [ ] `SettledToolResultProjectionV1` producer/store/current reader。
- [ ] public settled Context handoff 与 production composition。
- [ ] Context ContentRef、token、sensitivity、recipe、cache、TurnContinuation。

只有 Tool Owner 冻结真实 V2 projection producer与同 Owner durable commit/read seam后，才能开始完整 B1。
