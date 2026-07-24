# Workspace Restore V1 实施计划

对应设计：[Workspace Restore V1](../../design/sandbox/workspace-restore-v1.md)。

## 阶段与文件落点

| 阶段 | 文件候选 | 完成条件 |
|---|---|---|
| S1 合同/Codec | `contract/workspace_snapshot_bundle_v1.go` | strict canonical JSON、目录/常规文件闭集、Residual、bounds与digest测试 |
| S2 Host Local capture/stage | `dataplaneadapter/hostlocal/workspace_restore_v1.go` | trusted roots、no-follow capture、temp+fsync+rename、exact marker Inspect、source零修改 |
| S3 Sandbox Owner | `ports/workspace_restore_v1.go`、`kernel/workspace_restore_v1.go`、`storage/sqlite/workspace_restore_v1.go` | coordinate-only Request、Attempt/history/current、S1/S2、lost-reply、DomainResult/Apply分层 |
| S4 Runtime additive | `runtime/ports|control|kernel/*restore_stage*` | Eligibility绑定Admission/Authorization/Permit/Begin、Restore Evidence/Settlement，Checkpoint V1/V5闭表不扩权 |
| S5 Application/Review/Context | Restore Coordinator与typed adapters | Application单一顺序、Review current exact binding、Context新Generation/Frame、Activate最后发生 |
| S6 Continuity | `runtimeadapter`与SDK/Inspect | 只关联Plan/Attempt/Eligibility/Stage/Context/Activation refs，不执行或写其他Owner事实 |
| S7 验证/资产 | 各Owner测试与module/memory | target100/race20、全量ordinary/race/vet、故障/Conformance与current truth全部通过 |

## 实施顺序

1. 先完成S1/S2 owner-local真实文件闭包，不接Runtime权限；
2. 再完成Sandbox Owner Fact与Provider Observation分离；
3. Runtime只新增Restore专用additive合同，不扩Checkpoint或Evidence V3/V4闭表；
4. Application在既有Action Gateway/Review/Enforcement之上编排，不创建第二治理链；
5. Context Refresh成功并形成exact新Generation/Frame后才调用Restore Activation；
6. 最后组合端到端happy/crash/lost-reply/stale/tamper并收口资产。

## 状态

- [x] 用户确认Workspace Snapshot、Host Local、Workspace+Context Restore。
- [x] 用户确认隔离新根；只恢复目录/常规文件与执行位；其他对象Residual。
- [x] 用户授权Application/Review/Runtime/Sandbox/Context/Continuity完整安全纵切。
- [ ] S1合同与Codec。
- [ ] S2 Host Local capture/stage。
- [ ] S3 Sandbox Owner治理。
- [ ] S4 Runtime additive合同。
- [ ] S5跨Owner协调与Activate。
- [ ] S6 Continuity关联。
- [ ] S7全门与资产收口。

