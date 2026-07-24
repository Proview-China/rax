# RestoreAttempt/Eligibility最小Runtime Delta

时间：2026-07-18 12:00（Asia/Shanghai）

## 事件

用户授权完成阻塞Continuity后续Restore的最小Runtime Delta。live实现新增：

- Runtime公开`CheckpointRestoreOperationScopeV2`、`RestoreAttemptFactV2`、`RestoreEligibilityFactV2`、`RestoreGovernancePortV2`及Plan/prerequisite current Reader；
- Runtime create-once原子提交RestoreAttempt与fresh Instance/high Epoch/new Lease/Fence Reservation；Continuity Plan中的Identity始终只是proposal；
- Eligibility在Action Admission前Issue/Bind，TTL上限5分钟，只绑定Review target/requirement/policy basis和Authority/Scope/Budget/Binding/Context requirement exact refs，不包含accepted Verdict、Review Authorization或Permit；
- Attempt create与Eligibility bind均执行S1/S2；Eligibility current Inspect复读exact Plan、reserved Attempt history与prerequisite current，并在返回前复读Attempt/Eligibility current；
- changed content、Instance/Lease重复、stale CAS、ABA、跨Tenant/Scope splice均Fail Closed；Unknown/lost reply只Inspect原Attempt/Eligibility identity；
- Continuity新增`runtimeadapter.RestorePlanCurrentReaderV2`，只读exact submitted Plan、immutable ManifestSeal与Runtime Consistency。Owner sealed `Updated/Expires/Digest`保持稳定，fresh now只Validate；Adapter不创建Runtime Fact。

## 实际验证

- Runtime：`go test ./kernel -run 'RestoreGovernanceV2' -count=100` PASS；
- Runtime：`go test -race ./kernel -run 'RestoreGovernanceV2' -count=20` PASS；
- Runtime Delta四包ordinary/race/vet PASS；
- Runtime：`go test ./...`、`go test -race ./...`、`go vet ./...` PASS；
- Continuity Adapter：`go test ./runtimeadapter -run 'RestorePlanCurrentReaderV2' -count=100` PASS；
- Continuity Adapter：`go test -race ./runtimeadapter -run 'RestorePlanCurrentReaderV2' -count=20` PASS；
- Continuity：`go test ./...`、`go test -race ./...`、`go vet ./...` PASS。

## 当前边界

最小链停在Eligibility。Application Restore Intent到专用Action Admission、独立Review Authorization、Permit/Fence current重验、Begin、Sandbox Workspace Stage、Restore Evidence/Settlement applicability、Context Refresh、Activate以及production Provider/root仍未实现。Provider调用保持0；不宣称外部世界回滚；Partial Checkpoint仍只诊断；legacy Restore接口不得扩权。
