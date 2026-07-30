# AgentRunService 横向公共合同进入实施

- 时间：2026-07-30 18:50:22 +08:00
- 初始基线：origin/main@f6cf6805
- 当前基线：origin/main@f6014b93
- 分支：agent/host-agentrunservice
- 状态：owner-local 公共合同已实现；production NO-GO

## 决议

首个 AgentRunService PR 只冻结 CrossLanguageWirePrimitivesV1 与 AgentRunServiceV1 skeleton。覆盖 version/capability negotiation、wire-safe exact refs、command envelope/receipt、Inspect 原命令恢复、event sequence/cursor/afterSequence/gap=>resync，以及 UnknownOutcome/Indeterminate 的结构化结果。

所有 uint64、revision、epoch、sequence 使用 unsigned canonical decimal string；通用 int64/UnixNano 使用 signed canonical decimal string，time.Time 单独使用 UTC RFC3339Nano Z。Cancel 与 Stop 不可互换，receipt 不得改写 Runtime outcome。

该合同已从 Agent Host 内部撤出，位于独立 ExecutionRuntime/agent-run-service 横向模块；它不导入 Host/Runtime/Application 实现或 Owner 数据库。event identity 同时绑定 source owner/subject exact ref，EventRef.Digest 与 EventDigest 同源；gap 只能返回绑定 retention current 的 RESYNC_REQUIRED。

无页面 VM、无 Console 命令、无 TypeScript DTO。codegen 与 TS backend 留给后续独立 PR。Build/Run/Create/Start、Builder/AgentPackage coordinates 和 production composition root 不在本切片。

## 基线事实

旧基线的 golang.org/x/text lock 漂移已由独立 PR #25 修复。本分支 rebase 到 origin/main@f6014b93（含 AgentPackage PR #26/#29/#31、独立 model-invoker/CI 修复与 Tool-only Context PR #32）；本 PR 不修改任何既有 go.mod/go.sum，只新增无外部依赖的 agent-run-service go.mod。

## 当前边界

Console 集成尚未开始；本切片仅是 console-ready wire foundation。production NO-GO。

## 验证

go test ./...、go test -race ./...、go vet ./...、100 次 contract/jsonv1 重复测试均通过；go mod tidy -diff 为空，go list 未发现外部模块依赖。
