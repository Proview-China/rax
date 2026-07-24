# SDK Timeline Page与CLI读取闭环

时间：2026-07-18 09:58 CST

## 事件

Continuity owner-local完成下一最小切片：SDK不再信任Timeline Reader返回页，而是在返回调用方前复验Event canonical完整性、全部Query过滤条件、严格ledger sequence、PageLimit、Cursor query/currentness及exact page watermark；伪造输入Cursor在调用Reader前Fail Closed。

CLI自有映射补齐只读`timeline show`、`timeline watch`与`checkpoint inspect`。所有输入strict JSON decode，缺失Reader或未知字段均零输出失败；根CLI注册、endpoint、credential和production redaction policy仍不属于Continuity。

## 实现落点

- `ExecutionRuntime/continuity/contract/timeline.go`：公共`TimelineEventMatchesQuery`。
- `ExecutionRuntime/continuity/domain/timeline.go`：查询复用公共过滤谓词。
- `ExecutionRuntime/continuity/sdk/client.go`：输入Cursor前置校验与Reader Page exact复验。
- `ExecutionRuntime/continuity/sdk/timeline_page_validation_test.go`：Reader漂移、Cursor前置拒绝、clone/no-alias反例。
- `ExecutionRuntime/continuity/cli/runner_v1.go`：三类只读命令映射。
- `ExecutionRuntime/continuity/cli/runner_v1_test.go`：真实reference Reader、缺口Fail Closed和strict decode反例。

## 验证证据

```text
go test ./contract ./domain ./sdk ./cli -run '(TimelinePage|TimelineQuery|RunnerV1)' -count=100
PASS

go test -race ./contract ./domain ./sdk ./cli -run '(TimelinePage|TimelineQuery|RunnerV1)' -count=20
PASS

go test ./...
PASS

go test -race ./...
PASS

go vet ./...
PASS
```

## 边界

- 本切片不创建生产API/CLI root，不选择transport或credential。
- Continuity只验证请求过滤与Reader exact结果；授权脱敏策略由Policy/Application Owner提供。
- Checkpoint只读Inspect不授capture、Consistency、Restore资格或Provider能力。
- Restore Execute、remote purge/archive、真实Provider与production root继续NO-GO。
