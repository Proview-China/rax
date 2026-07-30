# Agent Run Service 模块

状态：公共合同与 reference-executable 横向运行层已实现（2026-07-30）；production NO-GO

## 当前事实

- 代码：ExecutionRuntime/agent-run-service；
- 公共面：CrossLanguageWirePrimitivesV1、AgentRunServiceV1、StrictJSONDecoderV1；
- 能力：Negotiate、InspectAgentRun、InspectOriginal、WatchAgentRun、CancelAgentRun、StopAgentHost；
- 运行层：ServiceV1、窄 Owner Adapter、Memory/SQLite Command Journal、SQLite owner-event 投影流；
- 保证：exact command 幂等、跨 handle 线性化、lost reply Inspect、event resume/retention gap resync；
- 边界：横向 composition/transport，不是 Host/Runtime/Application/Harness Owner；
- production composition root：不存在，production NO-GO。

## 禁止误读

- command receipt 不是 Runtime outcome；Host Stop 不是 Run Cancel；
- Owner projection/event 不是第二事实账本；sequence gap 只能 RESYNC_REQUIRED；
- 无 AgentVM、PraxisCommands、Canvas、Sidebar、页面 VM、Console 命令或页面接线；
- 无 TypeScript DTO、codegen、TS backend、Build/Run/Create/Start 或 Builder/AgentPackage coordinates。
- 仍无 production Owner Adapter registry/root，不声明 Console 页面已接线。

## 当前验证门

- ordinary/race/vet；
- 2^53、uint64/int64 边界、TTL/clock regression、UTC RFC3339Nano；
- idempotency replay/conflict、lost reply Inspect、event identity splice、cursor/gap/resync；
- strict JSON 与 import boundary。
