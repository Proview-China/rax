# 2026-07-31 Workspace Read Admission Attempt Binding V2

- 用户授权Sandbox Owner实现additive V2，V1 wire/digest保持不变。
- Sandbox新增full Runtime Attempt到Admission与original WorkspaceRead Attempt的
  exact历史binding；公开面只暴露只读Reader。
- writer改由Sandbox `internal/owner/workspaceread` nominal capability保护，
  只有kernel完成S1 current closure后可构造；Seal helper不赋权。
- SQLite v18在一个事务中create-once V1 Reservation/origin/current/binding和
  V2 binding，完整持久化Delegation等Runtime Attempt exact轴。
- Reader严格复读V2、V1 binding、Command、Reservation和origin；V2存在后的
  引用缺失按Conflict处理，origin/Reservation stable digest分别与各自body重算。
- public executor黑盒覆盖64并发单winner、physical actual=1、restart/8 handles、
  lost reply exact Inspect、splice、no-alias和读取前后全表hash不变。
- v18 schema严格验证namespace、ledger、PK autoindex、index list/xinfo及字面
  `BINARY`；ledger存在或partial namespace时绝不silent repair。
- PR #76返修增加引号/注释感知的`sqlite_master.sql`词法语义闭包，覆盖table、
  三个显式index及PK autoindex；extra CHECK/FK/trigger和非索引列COLLATE均拒绝。
- 过期V2历史binding仍可full-exact读取，但不会恢复current/execute资格，也不会
  产生recovery evidence或第二次physical read。
- 实测通过：定向ordinary×20/race×5、`go test ./...`、
  `go test -race ./...`、`go vet ./...`、`go mod verify`。
- 当前为`implemented-candidate / owner-local / PR #76 / 已提交未合并`；
  Tool full composition、Runtime/Application接线和production readiness继续NO-GO。
