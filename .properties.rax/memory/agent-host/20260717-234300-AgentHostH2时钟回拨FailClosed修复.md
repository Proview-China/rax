# Agent Host H2 时钟回拨 Fail Closed 修复

## 事件

独立复核发现 Host lifecycle 曾在时钟不前进时以 `current+1` 合成 Journal 时间。该行为已删除：`advanceV1` 现在要求 Host clock 严格大于持久 `UpdatedUnixNano`；rollback 与 equal tick 均返回 `precondition_failed/clock_regression`，且 Journal 保持原 revision/phase。

Binding Host-owned 窄 Port 同步明确 H3 约束：真实 `BindAssemblyV1` 必须是 canonical start-or-inspect；lost/unknown reply 只能 Inspect 原 Runtime Binding identity，不得创建第二 Binding。

## 验证

已新增 rollback/equal tick 反例，并重跑 targeted、full ordinary、full race 与 vet。
