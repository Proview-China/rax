# Harness P4 Component Release收口

时间：2026-07-18 01:32 CST

## 现场裁决

- Assembly current、Controlled Operation Provider Route current及Model PreDispatch CommitGate均有owner-local精确实现与恢复测试；
- Assembly/Route Store仍有内存或测试候选，Model Gate尚未由Model Owner证明所有actual-point强制调用；
- G6A组合仍是test-only，Tool Consumer、Application生产协调、Context Refresh/Continuation、可执行Factory、持久后端和production composition root未闭合；
- 因此Harness P4 Release固定`reference_only`，不得以Route candidate、owner-local current或fixture升级production。

## 本次产物

- `ExecutionRuntime/harness/releasecandidate/release.go`；
- `ExecutionRuntime/harness/releasecandidate/release_test.go`；
- `ExecutionRuntime/harness/go.mod`增加Agent Assembler/Definition本地公共合同依赖；
- `design/harness/component-release-v1.md`；
- `plan/harness/component-release-v1.md`；
- `module/harness/component-release-v1.md`及对应入口索引。

Release闭合Manifest、Module、Capability、Port、三Owner、Artifact、Certification、Evidence、TTL、九项Plan Artifact和descriptor-only Factory。Publisher在Ensure indeterminate后只按exact Release Ref Inspect一次，不重试mutation；发布后重验返回Release、TTL和时钟。

## Production P0

1. 持久Session/Event、Assembly current与Route current Store；
2. production Route双层wiring current及全路径no-bypass；
3. Model actual-point guard/inventory/receipt；
4. Tool Consumer、Application production current与Context Refresh/Continuation；
5. 可执行Factory、Cleanup Conformance、deployment attestation；
6. production composition root。

## 验证

在`ExecutionRuntime/harness`执行：

- `go test ./releasecandidate -count=100`：PASS；
- `go test -race ./releasecandidate -count=20`：PASS；
- `go test ./...`：PASS；
- `go test -race ./...`：PASS；
- `go vet ./...`：PASS；
- releasecandidate import boundary扫描：PASS，无Host/Application/Model/Tool/Context实现或Assembler repository/resolver依赖；
- gofmt、diff-check、trailing-whitespace：PASS。

本次未stage、未commit。
