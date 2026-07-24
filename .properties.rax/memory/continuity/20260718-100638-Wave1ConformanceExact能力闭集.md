# Wave1 Conformance exact能力闭集

时间：2026-07-18 10:06:38 CST

## 事件

Continuity Wave1 Conformance从“只检查若干禁用能力”收紧为supported/unsupported两个exact闭集：未知、缺失、重复声明全部Fail Closed。已存在的只读Runtime/Application Adapter不再被误报为整体unsupported，禁用项改为明确的`production-runtime-root`与`production-application-root`。

当前唯一接受的等级仍是`restricted_controlled + reference_only + ProductionSLA=false`。`fully_controlled`、`contained_observe_only`或`rejected`不能冒充当前Wave1；这不提升生产能力。

## 实现与反例

- `ExecutionRuntime/continuity/conformance/manifest.go`：能力闭集与exact集合校验。
- `ExecutionRuntime/continuity/tests/conformance/conformance_test.go`：未知/缺失/重复supported与unsupported、四级漂移、Adapter/production root真值。
- `ExecutionRuntime/continuity/releasecandidate`继续消费该Conformance并固定`reference_only assembly_candidate`。

## 验证证据

```text
go test ./conformance ./tests/conformance ./releasecandidate -count=100
PASS

go test -race ./conformance ./tests/conformance ./releasecandidate -count=20
PASS

go test ./...
PASS

go test -race ./...
PASS

go vet ./...
PASS
```

## 边界

- 本次没有新增Runtime/Application/Harness公共Port或Capability。
- ComponentRelease仍不提供production root、Participant capture、Restore Execute、remote Provider、cleanup proof或SLA。
- Provider调用保持为零；未知/丢回包仍只Inspect原exact release identity。
