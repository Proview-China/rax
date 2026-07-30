# Harness Exact Model Turn Adapter V2 两项 P1 返修

时间：2026-07-31 04:25（Asia/Shanghai）

## 事件

独立复审发现并返修两项 owner-local P1：

1. attempt create、Model attempt 与 outcome bind 的 Unknown 恢复不再直接使用
   无界 `context.WithoutCancel`；三条路径统一重新附加 Envelope、S1 或最终共同
   最短 TTL 的 exact deadline。
2. SQLite sidecar 不再只信任代码常量的 ledger digest；Open 时要求版本集合
   精确为 `{2}`，并验证完整 DDL、`table_xinfo`、`index_list/index_xinfo`、
   无额外 trigger 以及回滚正负约束 probe。

## 回归资产

- 三条恢复路径的 fake Inspect 会真实等待 `ctx.Done()`，同时确认 caller cancel
  被移除、exact TTL deadline 未被移除。
- reopen 反例覆盖无 PK、额外列、错误同名 index、弱化 CHECK 与额外 ledger
  version。
- 单连接 Open 回归证明物理验证不会因未关闭 PRAGMA rows 耗尽连接池。

## 验证事实

- focused ordinary `count=100`：通过，216.210s。
- focused race `count=20`：通过，175.803s。
- Harness 全量 ordinary：通过。
- Harness 全量 race：通过。
- Harness 全量 vet、gofmt 与新增文件 no-index diff-check：通过。

## 边界

本次只返修 Harness owner-local adapter 和 SQLite sidecar，不新增 production
Pair Reader、composition root、Provider/Tool/PendingAction/Turn 能力。
