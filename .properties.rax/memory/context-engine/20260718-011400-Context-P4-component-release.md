# Context+Cache P4 Component Release/readiness

时间：2026-07-18 01:14 +08:00

## live裁决

- Offline SDK、Frame/Manifest/Generation exact current Reader、N=1 owner-local Refresh与Application公共Adapter已完成软件验证。
- refresh/cache/ref/outcome/release/prompt stores仍为进程内reference；Memory/Knowledge B-cross为test-only fixture。
- 真实durable State Plane/cache、Provider Cache current、Harness per-turn Injection/Continuation、Turn推进、cleanup与production deployment root不存在。

因此本轮只发布`reference_only ComponentReleaseV1`；没有production promotion API，强改production由Assembler conformance/residual校验失败关闭。

## 实现

- 新增`ExecutionRuntime/context-engine/releasecandidate`：完整Manifest/Module/Capability/Port/Factory descriptor、effect/settlement/cleanup owners、artifact、candidate certification、evidence、TTL、owner-local conformance与readiness proof boundary。
- publisher只依赖Agent Assembler公共publisher/reader ports；Ensure回包indeterminate时只按同一exact ref Inspect，不重派mutation。
- release包不导入Host、Assembler repository/resolver、Harness实现、Application或Memory/Knowledge实现。
- production P0：durable state/cache、Source/Provider current、per-turn injection/refresh/continuation、cleanup conformance与deployment composition root。

## 验证

在`ExecutionRuntime/context-engine`实际通过：

```text
go test ./releasecandidate -count=100
go test -race ./releasecandidate -count=20
go list -deps ./releasecandidate | rg 'ExecutionRuntime/(agent-host|host|application|memory-knowledge|model-invoker)|agent-assembler/(repository|resolver)|harness/(kernel|internal|ports)'
go test ./...
go test -race ./...
go vet ./...
```

import命令无输出；release package未导入所列Owner或实现包。仓库范围`git diff --check`、新文件显式`--no-index --check`及trailing-whitespace扫描均通过。

覆盖：publisher lost reply exact recovery、64并发确定性、TTL crossing、clock regression、artifact/published/proof/conformance drift、typed nil、production fail-closed、公共类型复用与Factory-descriptor-only。

未运行真实Provider Cache、durable State Plane/cache、Harness per-turn Injection/Continuation、Turn推进、cleanup或deployment root测试，因为这些production能力不存在；不得记录为通过。
