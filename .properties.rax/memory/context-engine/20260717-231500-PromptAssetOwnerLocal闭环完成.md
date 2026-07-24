# PromptAsset Owner-local闭环完成

时间：2026-07-17 23:15（Asia/Shanghai）

## 当前真值

- 用户确认PromptAsset V1直接内嵌规范化`instruction/example/policy`片段规格与exact `ContentRef`；不新增PromptFragment Fact，也不把Prompt降格为普通Recipe Source。
- `ExecutionRuntime/context-engine`已完成distinct `PromptAssetRefV1`、immutable Put/exact Inspect、确定性Candidate projection、独立pre-release lifecycle-head expected-CAS与Inspect。
- Role映射固定为：instruction→Instruction/Authoritative，example→Conversation/Restricted，policy→PolicySnapshot/Restricted。Prompt不决定Provider message、Frame Region、最终顺序或cache placement。
- Store是线程安全进程内参考实现，不是production backend、State Plane root或SLA。
- CTX-D07未闭合前，Prompt `publish/rollback/revoke`保持`ErrUnsupported`；developer ingress、Application/Harness/Model Adapter、Capability与production root均未启用。

## 实际验证

- `go test -run Prompt -count=100 ./contract ./kernel ./tests/blackbox ./tests/failure ./tests/conformance`：PASS。
- `go test -race -run Prompt -count=20 ./contract ./kernel ./tests/blackbox ./tests/failure ./tests/conformance`：PASS。
- `go test -count=1 ./...`：PASS。
- `go test -race -count=1 ./...`：PASS。
- `go vet ./...`：PASS。

覆盖ContentDigest、规范集合、exact RenderCompatibility、Authority/TTL、same-ID换内容、no-alias、cancel、Unknown/Unavailable零产物、lost reply只Inspect、64并发单一lifecycle successor、64并发确定性Candidate以及production动作unsupported。
