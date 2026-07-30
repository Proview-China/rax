# Harness Exact Model Turn Adapter V2 Post-Bind Fresh Clock 返修

时间：2026-07-31 04:45（Asia/Shanghai）

## 事件

独立终审发现 outcome Bind 只在写入前采样 `finalNow`。本次在 Bind 正常返回和
Indeterminate 后 exact Inspect 恢复的统一出口增加 post-bind fresh trusted
clock：

- 必须不早于 `finalNow`；
- 必须严格早于共同最短 TTL；
- 必须用 fresh 时间重新验证 Model Outcome currentness。

## 反例

- Bind 已落盘后 clock rollback：返回 `ReasonClockRegression`，不返回
  `outcome_bound`。
- Bind 已落盘且丢回复，经 exact Inspect 恢复后 `now == common expiry`：返回
  `ReasonBindingExpired`，不返回 `outcome_bound`。

两种情况下 durable Harness fact 保持 `outcome_bound`，后续只能按精确坐标
Inspect；本返修不伪造回滚或覆盖 durable winner。

## 验证事实

- focused ordinary `count=100`：通过，287.629s。
- focused race `count=20`：通过，212.474s。
- Harness 全量 ordinary：通过。
- Harness 全量 race：通过。
- Harness 全量 vet、gofmt、新增文件 no-index diff-check 与 import 边界：通过。

## 边界

未修改三条 bounded recovery、SQLite 物理 schema 门禁、Provider/Tool/
PendingAction/Turn 边界，也未 rebase、commit 或 push。
