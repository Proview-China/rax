# Application G6A V2 Schema与并发竞态收口

时间：2026-07-16 22:23（Asia/Shanghai）

## 结论

Application owner-local V2与Harness P3 live接线重新达到`P0/P1/P2=0`。这次收口不解除Tool P4、跨模块P5、系统G6A或production root门禁。

## 修复

1. Harness P3按Application新合同把完整`identityRequest`传入`SealSingleCallModelPendingActionIdentityCurrentV2`，不再使用旧两参签名。
2. 删除Application错误的`PendingAction.PayloadSchema == DomainResultFact.Schema`跨域约束：工具输入schema只与Identity exact；领域结果schema只与Runtime Settlement exact。
3. `finalizeV2`在终态构造前无条件复读权威Coordination current，并在复读后重新取fresh clock、复验Result current，关闭跨Coordinator实例的stale `dispatch_intent`竞态。
4. fresh current仍为`dispatch_intent`时拒绝已有Tool result绕过StartClaim；只有`waiting_inspect`可完成，`completed`只接受exact ResultRef。

## 新增回归

- Application fixture默认使用不同的工具输入schema与SettledTurn结果schema；
- Fact/Settlement schema不一致、DomainResult FactRef splice继续fail closed；
- Identity Current Request的Run、SessionID、Turn逐字段漂移均拒绝；
- 确定性stale dispatch快照在权威current为waiting时可恢复完成；
- 权威current仍为dispatch时零terminal CAS并返回PreconditionFailed。

## 验证

- Schema/Application/Harness定向ordinary100与race20：PASS；
- 并发与完整SingleCall V2族ordinary100与race20：PASS；
- Application full ordinary、full race、vet、gofmt：PASS；
- Harness full ordinary、full race、vet、gofmt：PASS；
- 两条独立代码复审确认并发收口`P0/P1/P2=0`。

## 仍未解除

- Tool P4公开Binding/current消费；
- P5 test-only跨模块fixture；
- production backend/root、durability、SLA、Continuation与Turn推进。
