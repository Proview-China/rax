# Context Model Input Pair Adapter V2

## 作用

本模块是Context Owner到Model公开Pair V2端口的production read-only
适配层。它证明完整Material、durable current Frame和immutable Owner
binding仍一致，并验证RouteCall Context body确实来自冻结的Context
OrderedSegments子集。

## 组成

- `contract/model_input_source_current_v1.go`：完整source request/projection与
  Context-domain digest。
- `ports/model_input_source_current_v1.go`：current source窄Reader。
- `kernel/model_input_source_current_v1.go`：Material exact/current、Frame
  exact-current、Owner的S1/S2和TTL闭包。
- `modelinvokeradapter/invocation_material_context_pair_v2.go`：严格lowering、
  Model canonical body digest和Pair projection。

## 使用边界

构造kernel reader时必须注入同一Context Owner下的Material exact/current
Reader、暴露相同immutable Owner的durable Frame current Reader、fresh clock和
不超过30秒的TTL。随后把该Reader注入Model adapter。

调用方必须提供固定Kind的neutral Model Frame/Material exact refs及Model
ContextBody digest。adapter只读Context并返回Model projection，不执行Provider、
routegateway、Harness或Tool调用。

import AST门禁扫描本slice全部production文件和共享Owner adapter。每个文件使用
独立精确白名单，并先拒绝网络、OS进程、database/sql、syscall、plugin、
unsafe、外部Provider SDK及Model provider/routegateway/internal/runtimeadapter。

## 已知限制

仅支持instruction UTF8、input_message UTF8及function_call canonical JSON
object。FunctionResult、reference、artifact-ref及其他组合是明确的Fail Closed，
不是production能力。

## 验证

- focused ordinary ×100
- focused race ×20
- Context module full ordinary/race
- `go vet ./...`
- `go mod verify`
- gofmt、diff-check及adapter import AST门禁
