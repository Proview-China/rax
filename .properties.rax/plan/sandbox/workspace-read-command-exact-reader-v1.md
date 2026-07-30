# Workspace Read Command Exact Reader V1 Plan

## 施工清单

1. 在 Sandbox ports 发布独立 `WorkspaceReadCommandExactReaderV1`；
2. SQLite V17 增加owner-local full canonical body seal表，并以同一事务
   create-once写入Command与seal；
3. 迁移不回填legacy body；无proof行与同Fact Create retry均Fail Closed；
4. Exact Reader JOIN完整coordinate与row seal，只校验shape，不校验current TTL；
5. `table_xinfo`门禁严格验证seal表物理shape；
6. Physical Executor只读转发该Port，便于composition注入窄接口；
7. 增加过期后历史读取、重启、64并发、typed nil/context、Unavailable、
   Ref/body/digest/Created/Updated canonical splice、legacy migration、事务
   rollback与零写/零Provider证明；
8. 运行 focused ordinary×100、race×20、Sandbox full ordinary/race、
   `go vet`、import boundary 与 `git diff --check`。

## 产物与非目标

产物是 Sandbox-owned immutable Command exact Reader。它允许消费者在 terminal
Attempt 恢复时验证 `SourceToolCommand`、payload、workspace/range 与 Runtime
坐标。

本切片不修改 Tool adapter，不新增 Tool Result、Runtime Settlement、Provider
调用、Host composition 或 Console 合同；也不把 historical Command 解释为
当前执行资格。
