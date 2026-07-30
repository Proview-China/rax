# AgentRunService 横向运行层 v1 计划

状态：已完成（2026-07-30）；production wiring 待后续独立切片

## 已实施

1. ports/runtime_v1.go：CommandJournal、CommandOwner、Inspect、Attempt Inspect、EventStream 窄接口；
2. service/service_v1.go：六方法 reference Service、实际能力协商、typed result、unknown recovery；
3. storage/memory：reference journal；
4. storage/sqlite：durable command journal 与 owner-event stream；
5. tests：same-key replay/conflict、64并发、多 handle、restart、panic/error/invalid receipt、typed rejection、lost receipt reply、event resume/gap；
6. import boundary：SQLite adapter只能导入唯一 modernc driver，其余横向代码仍禁止 Owner implementation/storage/internal。

## 后续

- 注册真实 Application/Agent Host Owner Adapter；
- Wire IDL/codegen、TS Backend transport；
- auth/scope/session 与真实进程监督；
- production deployment/HA/retention 策略。

这些后续均不得生成 Console 页面合同，须等待前端冻结。
