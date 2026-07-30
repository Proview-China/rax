# AgentRunService 横向运行层 v1

状态：owner-local/reference-executable 已实现；production NO-GO

## 设计结论

ServiceV1 只做公共合同验证、能力协商、Owner Adapter 路由与不确定结果恢复。Host、Runtime、Application、Harness 及 6+1 继续拥有各自 Current；Service 不读 Owner 数据库，也不生成领域成功事实。

Command Journal 是横向命令的唯一线性点：

1. exact envelope 先以 idempotencyKey/commandId 预留；
2. 相同 key+payload 只读取原 receipt；不同 payload fail closed；
3. SQLite 使用 WAL、FULL synchronous、BEGIN IMMEDIATE，跨进程内 Service/多 handle 串行化预留；
4. pending reservation 只能返回 exact command UNKNOWN_OUTCOME/INSPECT，禁止第二次调用 Owner；
5. Owner error/panic/invalid receipt统一封成 durable INDETERMINATE receipt；receipt 写回丢失先 Inspect 原 reservation；
6. REJECTED 等 Owner typed fault 原样保留，不降成 error/500。

EventStream 只保存已经由 Owner seal 的投影事件，不成为第二领域账本。sequence 必须连续，afterSequence 可恢复；retention gap 返回 RESYNC_REQUIRED 和 Inspect 指令。

## 非目标

- Console 页面、Canvas、Sidebar、AgentVM、PraxisCommands；
- TS DTO、codegen、WebSocket/HTTP transport；
- production Owner Adapter 注册和 composition root；
- Build/Run/Create/Start 或新领域 Current。
