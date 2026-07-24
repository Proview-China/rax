# Runtime Shared Engine P4 Component Release收口

时间：2026-07-18 01:47 CST

## Live裁决

- Runtime已有Binding、Command、Admission、Activation、Run、Effect、Evidence、Settlement与Checkpoint的广泛公共Fact/Gateway；
- SQLite真实持久化Binding Facts/Sets、Review Binding、EvidenceSubject、Review Evidence及Review Governance current子集；
- Command/Desired State/Outbox、Identity/Activation、Run/Effect/Settlement、Checkpoint等完整生产事实仍依赖foundation、内存fake、reference Owner或Conformance candidate；
- 当前无生产Scheduler/Supervision、Activation/Run/Cleanup root、可执行Factory、deployment attestation或production composition root；
- 因此Release固定`reference_only`，partial SQLite不提升为完整production State Plane。

## 产物

- `ExecutionRuntime/runtime/ports/component_identity_v1.go`：冻结`RuntimeSharedEngineComponentIDV1=components/runtime`；
- `ExecutionRuntime/runtime/tests/ports/component_identity_v1_test.go`：外部导入与alias canonical漂移反例；
- `ExecutionRuntime/runtime/releasecandidate/release.go`及`release_test.go`；
- `design/runtime/component-release-v1.md`；
- `plan/runtime/component-release-v1.md`；
- `module/runtime/component-release-v1.md`及入口索引。

Application后续只需导入`runtime/ports.RuntimeSharedEngineComponentIDV1`，无需复制字符串或依赖`runtime/releasecandidate`；Runtime没有反向导入Application。

Release闭合Manifest、Module、Capability、Port、三Owner、Artifact、Certification、Evidence、TTL与descriptor-only Factory。Ensure丢回包后只按exact Release Ref Inspect一次，不重试mutation。

## Production P0

1. Command/Desired State/Outbox、Identity/Activation、Run/Effect/Settlement、Evidence与Checkpoint完整durable facts/current；
2. Scheduler/Supervision与持续Reconcile；
3. Activation/Run Gateway及Cleanup/Reconciliation真实接线；
4. Checkpoint/Restore生产Provider；
5. 可执行Factory、deployment attestation与production composition root。

## 验证

- `go test ./releasecandidate ./tests/ports -count=100`：PASS；
- `go test -race ./releasecandidate ./tests/ports -count=20`：PASS；
- `go test ./...`：PASS；
- `go test -race ./...`：PASS；
- `go vet ./...`：PASS；
- releasecandidate import boundary：PASS，无Application/Host/6+1实现或Assembler repository/resolver依赖；
- gofmt、diff-check、trailing-whitespace：PASS。

本次未stage、未commit。
