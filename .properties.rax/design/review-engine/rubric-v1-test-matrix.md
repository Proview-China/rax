# Review Rubric V1 测试矩阵

| ID | 层级 | 场景 | Oracle |
|---|---|---|---|
| RUB-01 | unit | 七个 kind baseline seal/validate | 全部 exact digest 成功且 criteria/rules 非空 |
| RUB-02 | unit | unknown kind / universal prompt 名 | InvalidArgument，零事实 |
| RUB-03 | unit | criteria/rules 空或未排序/重复 | InvalidArgument/Conflict |
| RUB-04 | unit | Rule 引用不存在 criterion | Conflict |
| RUB-05 | unit | write/execute capability | InvalidArgument/Conflict |
| RUB-06 | unit | termination 0/越界 | InvalidArgument |
| RUB-07 | unit | partial Finding schema / Accept 作为 failure | Fail Closed |
| RUB-08 | store | create + same canonical replay | revision1一次写；replay同 digest |
| RUB-09 | store | same ID/revision 换 payload | Conflict，旧 exact 可读 |
| RUB-10 | store | supersede revision+1 | current前进；旧 history exact 可读 |
| RUB-11 | store | rollback/gap/stale expected | Conflict，history/current/highest 零变化 |
| RUB-12 | store | revoke next revision | terminal current；ValidateCurrent失败；历史可读 |
| RUB-13 | store | revoked 后 revive | Conflict |
| RUB-14 | store | pure TTL crossing | current read失败，不新增 revision |
| RUB-15 | store | deep clone | 修改返回 slice 不影响 stored fact |
| RUB-16 | store | corrupt highest/current/history | Snapshot seal/restore Fail Closed |
| RUB-17 | concurrency | 64 different CAS 同 expected | 恰好一个 revision winner |
| RUB-18 | SQLite | 64 different CAS + generation CAS | 恰好一个 winner，无 partial snapshot |
| RUB-19 | SQLite | close/reopen/integrity | current/history/digest保持 |
| RUB-20 | fault | Publish reply lost | mutation一次；detached exact Inspect恢复 |
| RUB-21 | fault | Revoke reply lost | mutation一次；detached exact Inspect恢复 |
| RUB-22 | admission | Rubric current 不存在 | Case/Target/Request/Trace零写 |
| RUB-23 | admission | S1/S2之间 supersede | Conflict，全部 admission facts 零写 |
| RUB-24 | admission | Store actual point前 revoke/drift | compound mutation零写 |
| RUB-25 | admission | Rubric为唯一最短TTL | Request超过 Rubric expiry 时零写 |
| RUB-26 | admission | clock rollback | ClockRegression，零写 |
| RUB-27 | error | cancelled ctx | 保留 closed typed error，不退化 NotFound |
| RUB-28 | import | production包依赖扫描 | 只依赖 Review公开包 + Runtime public core，无其他Owner实现包 |

Reusable suite：`conformance.CheckRubricStoreV1` 同时运行 memory 与 SQLite；targeted 门为 `go test -count=100 -run 'Rubric' ./...`、`go test -race -count=20 -run 'Rubric' ./...`，另跑 full ordinary/race/vet、gofmt 与 import/diff 检查。

