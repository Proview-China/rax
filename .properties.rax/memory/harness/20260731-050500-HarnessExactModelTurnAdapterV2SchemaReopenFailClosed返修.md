# Harness Exact Model Turn Adapter V2 Schema Reopen Fail Closed 返修

时间：2026-07-31 05:05（Asia/Shanghai）

## 事件

最终独立敌手审计复现：数据库已经存在正确 V2 ledger，但 exact index 被删除
后，旧 Open 顺序会先执行 `CREATE ... IF NOT EXISTS`，把损坏对象静默补回，再
通过物理验证。

本次把 Open 收紧为三态：

1. ledger、fact table、exact index 全部不存在，才允许作为 fresh 数据库创建；
2. 三个对象全部存在时，不执行 migration，直接验证既有 ledger、DDL、列、
   index、trigger 与约束；
3. 任意 partial 状态直接 fail closed。

## 反例

- 正确 V2 ledger + 缺失 exact index：reopen Conflict；
- 正确 V2 ledger + 缺失 fact table：reopen Conflict；
- fact table/index 存在但 ledger 缺失：reopen Conflict；
- fresh 空数据库仍可创建并通过单连接物理验证。

## 边界

本返修只处理 Harness owner-local SQLite sidecar 的 schema 初始化顺序，不改变
Model、Context、Tool、Provider、PendingAction 或 Turn 语义；production
composition 继续 NO-GO。
