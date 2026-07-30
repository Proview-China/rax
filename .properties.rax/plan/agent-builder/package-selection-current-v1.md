# AgentPackage Selection Current V1 落地计划

## Owner 与允许目录

Owner：`ExecutionRuntime/agent-builder`。

允许代码目录：

- `contract/verified_package_closure_v1.go`
- `contract/package_selection_current_v1.go`
- `ports/package_selection_v1.go`
- `loader/loader.go`
- `selection/service_v1.go`
- `repository/selection_sqlite_v1.go`
- `tests/blackbox/package_selection_current_v1_test.go`

禁止修改 `agent-host`、Runtime、StartV3、Factory 与 Console。

## 完成清单

- [x] Loader 发布 `VerifiedAgentPackageClosureV1` 窄 Reader；
- [x] closure 复算 Package/Publication/artifact/Input/compiler/frozen exact 链；
- [x] 冻结 Selection Ref、Current 与三条 Owner ports；
- [x] Service 内部派生 PublicationRef/ClosureDigest；
- [x] SQLite WAL/FULL append-only history + current CAS；
- [x] zero expected create、revision+1 advance；
- [x] same expected/same next 幂等，different next Conflict；
- [x] Unknown 只 exact Inspect；
- [x] Repository-owned clock 拒绝过期 current；
- [x] 真实 Package SQLite + Harness Publication 黑盒；
- [x] splice/missing 零写、64 并发、lost reply、restart、expiry、schema 黑盒；
- [x] ordinary/race/vet/diff 最终验证；

## 产出

产出是 Agent Builder Owner-local 的 nominal Package Selection current 与历史记录，不是 Host 可启动或 production 可用证明。
