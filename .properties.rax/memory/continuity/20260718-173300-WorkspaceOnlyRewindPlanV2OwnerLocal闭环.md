# Workspace-only RewindPlan V2 owner-local闭环

- 时间：2026-07-18 17:33 CST
- 范围：`ExecutionRuntime/continuity/**`与Continuity独占资产

## 已实现

- `RewindPlanFactV2`只保存Checkpoint Consistency/Manifest Seal、Sandbox source Workspace View、expected revision/file scope、ordered keep/drop/planned ChangeSet、Dependency Inspection、Review Requirement、irreversible Effect与Residual exact refs。
- Plan使用stable tenant Workspace Conflict Domain；caller不能携文件payload、accepted Verdict、Permit/Fence、Provider、Outcome或可信current。
- 状态为`draft -> workspace_inspected -> dependencies_inspected -> admitted -> submitted`，另有`rejected|expired`；Residual非空不能admitted/submitted。
- create-once、history/current、CAS revision+1、TTL边界、same-ID/content conflict、lost durable reply exact Inspect、no-ABA、64路并发单赢家、跨Tenant同ID隔离、deep clone与canonical selection digest已闭合。
- 内存reference repository、SQLite additive schema v9 repository、只读SDK和Wave1 exact capability声明已接入。

## Owner边界

- Continuity只拥有Plan Fact与Inspect/CAS；不创建Sandbox Workspace View/ChangeSet，不执行文件，不写Runtime Operation/Settlement或Review Authorization。
- 实际Rewind必须由Sandbox Owner形成新的ChangeSet，并复用既有`praxis.sandbox/workspace-commit`治理链。
- live Sandbox当前只有capture现有diff与commit既有ChangeSet，没有keep/drop→new ChangeSet的公共Owner Port；该跨Owner切面继续NO-GO。
- Tool、邮件、交易、网络请求、remote blob等不在首版Rewind执行范围，不宣称外部世界回滚。

## 当前验证

已通过contract/domain/memory/SQLite/SDK/fault/conformance普通测试及Rewind定向`count=100`。race20、full race/vet与资产机械门将在本轮最终收口统一执行。
