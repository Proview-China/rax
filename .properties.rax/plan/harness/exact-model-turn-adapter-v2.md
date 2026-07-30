# Harness Exact Model Turn Adapter V2 落地计划

状态：**owner-local 代码候选的 post-bind fresh-clock P1 已返修并通过机械门；等待独立返修复审。**

设计入口：
[Harness Exact Model Turn Adapter V2](../../design/harness/exact-model-turn-adapter-v2/README.md)。

## 范围

- [x] 新增独立 Envelope、DispatchRef 与两阶段 Dispatch Fact。
- [x] 新增 Harness additive Port，不修改旧 `ModelTurnPort`。
- [x] 新增单节点 durable SQLite sidecar。
- [x] 用 Model 公共四源 Pair Reader 做 S1/S2。
- [x] 接入 `GovernedModelTurnPortV3` 的 owner-local start-or-inspect。
- [x] 覆盖 TTL、时钟回退、typed-nil、漂移、重启、丢回包和 64 并发。
- [x] Unknown 恢复统一使用 `WithDeadline(WithoutCancel(ctx), exactTTL)`。
- [x] outcome Bind 正常/恢复返回前统一重取 fresh clock 并复核共同 TTL 与
  Outcome currentness。
- [x] SQLite ledger 与物理 DDL/列/index/约束形成 reopen fail-closed 门禁。
- [x] 区分 fresh 与已应用 schema：已应用 V2 禁止 Open 时静默 repair，
  partial no-ledger 必须 fail closed。
- [ ] 独立代码终审。
- [ ] Context/Tool Owner 的生产 Pair Reader composition adapter。
- [ ] production composition root。

## 明确不做

- 不调用 Provider。
- 不执行 Tool。
- 不创建 PendingAction。
- 不推进 Turn 或 Continuation。
- 不修改旧 Harness Loop、ModelTurnPort。
- 不写 Context、Tool、Model 或 Runtime 权威事实。
- 不声明 HA、生产后端或 SLA。

## 验收命令

```bash
cd ExecutionRuntime/harness
go test -count=100 ./tests/modelinvokeradapter
go test -race -count=20 ./tests/modelinvokeradapter
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

还必须执行 `gofmt -l`、`git diff --check` 与新增代码 import 扫描。

## 解锁条件

owner-local 完成条件是上述门禁通过并获得独立终审 YES。

production GO 还需要：

1. Context Owner 与 Tool Owner 的真实事实库适配为 Model 公共 Pair Reader；
2. composition root 只注入这些公共 Reader 与 Model V3 Port；
3. cross-owner 黑盒测试证明没有 fixture、自签 current 或 Provider 旁路。
