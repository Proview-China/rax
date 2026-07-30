# Agent Run Service 模块

状态：首个公共合同实施中（2026-07-30）

## 当前事实

- 代码：ExecutionRuntime/agent-run-service；
- 公共面：CrossLanguageWirePrimitivesV1、AgentRunServiceV1、StrictJSONDecoderV1；
- 能力：Negotiate、InspectAgentRun、InspectOriginal、WatchAgentRun、CancelAgentRun、StopAgentHost；
- 边界：横向 composition/transport skeleton，不是 Host/Runtime/Application/Harness Owner；
- production composition root：不存在，production NO-GO。

## 禁止误读

- command receipt 不是 Runtime outcome；Host Stop 不是 Run Cancel；
- Owner projection/event 不是第二事实账本；sequence gap 只能 RESYNC_REQUIRED；
- 无 AgentVM、PraxisCommands、Canvas、Sidebar、页面 VM、Console 命令或页面接线；
- 无 TypeScript DTO、codegen、TS backend、Build/Run/Create/Start 或 Builder/AgentPackage coordinates。

## 当前验证门

- ordinary/race/vet；
- 2^53、uint64/int64 边界、TTL/clock regression、UTC RFC3339Nano；
- idempotency replay/conflict、lost reply Inspect、event identity splice、cursor/gap/resync；
- strict JSON 与 import boundary。
