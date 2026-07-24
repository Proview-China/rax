# Restore最小公共纵切与Enforcement恢复

时间：2026-07-18 16:57 CST

## 当前事实

- Continuity仍只拥有Restore Plan、Manifest/Seal与Timeline关系，不拥有Runtime Attempt/Eligibility/Activation、Review Authorization、Sandbox Stage、Context Generation/Frame或Provider事实。
- Restore最小公共参考纵切已闭合：Application immutable Intent → Runtime create-once Attempt/new Instance/high Epoch/new Lease/Fence Reservation → short-TTL Eligibility → Action Admission → Review/Authorization → Permit/Begin → Prepare/Execute双重Enforcement → Sandbox Workspace Stage/Inspect → Evidence → Runtime Settlement(ref only) → Sandbox ApplySettlement → Context新Generation/Frame → Runtime Activation。
- Sandbox Host-Local Stage可在隔离workspace root物化普通文件、目录与可执行位；特殊文件只形成Residual。它不宣称外部世界回滚，不覆盖邮件、交易、网络请求或远程数据库Effect。
- Context materialization对source/target Generation/Frame、requirements与Stage closure执行S1/S2、CAS和lost-reply Inspect；Residual非空阻断Activation。
- Runtime Activation只激活Reservation中的新Instance/更高Epoch/new Lease；旧Instance不会复活。

## 本次最小Runtime Delta

- `RestoreStageEnforcementGovernancePortV1`新增按原`EnforceRestoreStageDispatchRequestV1`精确Inspect的恢复方法。
- Runtime Gateway只读取已存在的Enforcement Journal，逐项核对Operation、Attempt、Eligibility、Identity、Snapshot、Sandbox projection、Permit、Review Authorization及prepare/execute closure；changed request返回Conflict，不创建新revision。
- Application在Prepare或Execute Enforcement未知/丢回包后只用原request Inspect，不重发Enforcement，更不会重复Sandbox Effect。

## 实际验证

- `go test ./...`：Runtime、Application、Sandbox、Context Engine、Continuity全部PASS。
- `go test -race ./...`：上述五模块全部PASS。
- `go vet ./...`：上述五模块全部PASS。
- `go test ./tests/fakes -run RestoreStageEnforcement -count=100`：Runtime PASS。
- `go test -race ./tests/fakes -run RestoreStageEnforcement -count=20`：Runtime PASS。
- `go test . -run 'RestoreProductionCompositionV1|RestoreStageActionGatewayV1' -count=100`：Application PASS。
- `go test -race . -run 'RestoreProductionCompositionV1|RestoreStageActionGatewayV1' -count=20`：Application PASS。

## 仍未解锁

- production trusted Assembler/current Reader与root credential/deployment attestation；
- 跨Owner全量Participant的all-or-nothing Stage集合；
- remote blob/archive/purge、KMS Provider、生产拓扑与SLA；
- 选择性Rewind真实ChangeSet Effect；
- 外部世界回滚（永久禁止宣称）；
- Continuity ComponentRelease仍为`reference_only`，production Capability保持unsupported。
