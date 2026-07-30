# AgentRunService 横向公共合同 v1 计划

状态：已完成（2026-07-30）；production wiring 未开始

## 目标

在独立 ExecutionRuntime/agent-run-service 模块交付 CrossLanguageWirePrimitivesV1 与 AgentRunServiceV1 skeleton，让后续 transport 可以在不丢失 uint64/int64/UnixNano 精度、不改写 Owner 语义的前提下接入 Inspect、Watch、Cancel、Stop。

## 实施切片

1. contract/wire_primitives_v1.go、wire_negotiation_v1.go、fault_v1.go
   - 规范十进制 uint64、signed int64/UnixNano、UTC RFC3339Nano 与 checked conversion；
   - ExactRefWireV1；
   - version/capability negotiation request/result；
   - 完整 public fault taxonomy、seal、digest 与 fail-closed validation。
2. contract/agent_run_command_v1.go、agent_run_inspect_v1.go、original_command_inspect_v1.go、agent_run_events_v1.go、agent_run_actions_v1.go
   - AgentRun exact target；
   - command envelope/receipt 与 idempotent replay validation；
   - Inspect、InspectOriginalCommand、Watch、Cancel、Stop request/result；
   - sourceOwner/subject event exact identity、sequence/cursor/afterSequence/gap=>RESYNC_REQUIRED；
   - UnknownOutcome/Indeterminate 的结构化 disposition/fault。
3. ports/agent_run_service_v1.go
   - transport-neutral AgentRunServiceV1 skeleton；
   - 只声明边界，不提供 production composition root。
4. transport/jsonv1/decoder.go 与 tests
   - size limit、duplicate key、unknown field、trailing document；
   - 2^53、uint64 max、int64 min/max、TTL/clock、UTC、enum、event splice golden。
5. tests
   - ordinary：wire round-trip、seal/validate、连续 event page、same-key same-payload replay；
   - race：same-key different-payload conflict、expected-current drift、sequence gap、cursor regression、expired notAfter；
   - vet：go test -race、go vet、JSON number 审计、import boundary；
   - diff-check：git diff --check 与范围/Owner 语义复核。

## 验收门

- revision/epoch/sequence/UnixNano JSON 字段均为字符串；
- version/capability negotiation 不静默降级 required capability；
- same key + same payload 可识别为原命令重放，different payload 必须 conflict；
- lost reply 只能 Inspect 原 command；
- sequence/cursor/afterSequence 不连续时只返回 resync_required；
- UnknownOutcome/Indeterminate 是结构化结果，不折叠成 500；
- receipt 不声称 Runtime outcome，Stop 不冒充 Cancel；
- 无页面 VM、Console 命令、TS DTO、codegen 或 production root。
- 新模块不导入 Agent Host、Runtime/Application 实现、Owner 数据库、internal 或 fakes。

## 基线处理

旧基线 origin/main@f6cf6805 曾因 agent-host 的 golang.org/x/text indirect lock 漂移阻断测试。该问题已由独立 PR #25 修复；本分支已 rebase 到 origin/main@f6014b93（含 AgentPackage PR #26/#29/#31、独立 model-invoker/CI 修复与 Tool-only Context PR #32）。本 PR 新增独立无外部依赖的 agent-run-service go.mod，不修改任何既有 go.mod/go.sum。

## 后续 Design Delta

Builder/AgentPackage exact refs 合入后，另开 PR 冻结 Build/Run/Create/Start。codegen/TypeScript backend、Console transport binding、production composition root 与多 Owner replay 均为独立后续 PR。

## 实际验证

- go test ./...：通过；
- go test -race ./...：通过；
- go vet ./...：通过；
- go test -count=100 ./contract ./transport/jsonv1：通过；
- go mod tidy -diff：空；
- go list -deps ./...：无外部模块依赖。
