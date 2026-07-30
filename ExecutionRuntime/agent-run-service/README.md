# Agent Run Service

本模块是 Host、Runtime 与 Application Owner 之上的横向 composition/transport 合同层。当前只交付 CrossLanguageWirePrimitivesV1 与 AgentRunServiceV1 skeleton，不提供 production composition root。

## 已冻结

- canonical decimal string：uint64、signed int64 UnixNano 与独立 RFC3339Nano 编码；
- exact ref：kind、id、revision、digest；
- version/capability negotiation；
- command envelope/receipt：idempotency conflict、expected current、notAfter、lost reply Inspect；
- Agent Run Inspect、event cursor/afterSequence/gap => RESYNC_REQUIRED、Cancel 与 Host Stop 的 transport-neutral DTO/ports；
- UNKNOWN_OUTCOME / INDETERMINATE 与完整公共 fault taxonomy，不折叠为 generic 500。

## 不拥有/不支持

- 不拥有 Host lifecycle、Runtime Run/Outcome、Application command 或 Harness observation；
- 不提供 AgentVM、PraxisCommands、Canvas、Sidebar、页面 VM、Console 命令或页面接线；
- 不提供手写 TypeScript DTO、codegen 或 TS backend；
- 不提供 Build/Run/Create/Start、Builder/AgentPackage coordinates 或 production composition root；
- 不导入 Owner 数据库、internal、fakes 或实现包。

当前 production NO-GO。

## 验证

在本目录运行：

    go test ./...
    go test -race ./...
    go vet ./...
