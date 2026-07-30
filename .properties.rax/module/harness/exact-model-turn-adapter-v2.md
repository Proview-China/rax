# Harness Exact Model Turn Adapter V2 模块说明

## 用途

本模块把一个已由 Model Owner 封存的 `InvocationMaterialV2` 转换为 Harness
可恢复的 Model Turn dispatch 过程。它只保存 attempt/outcome 引用，不保存
Prompt、Provider 回包或 Tool payload。

## 组成

- `bridgecontract/model_turn_dispatch_v2.go`：Envelope、DispatchRef、Fact。
- `ports/model_turn_exact_v2.go`：执行 Port 和持久化 Port。
- `modelinvokeradapter/exact_model_turn_v2.go`：四源 S1/S2 与 Model V3 调用。
- `modelinvokeradapter/sqlite_model_turn_dispatch_v2.go`：单节点 SQLite sidecar。
- `tests/modelinvokeradapter/exact_model_turn_v2_test.go`：合同、故障和并发测试。

## 输入输出

输入是精确 `ModelTurnExactEnvelopeV2`。输出是
`ModelTurnDispatchFactV2`：

- revision 1：`attempt_bound`；
- revision 2：`outcome_bound`。

## 使用边界

构造 Adapter 时必须注入：

- Model `InvocationMaterialReaderV2`；
- Model neutral Context Pair Reader；
- Model neutral Tool Pair Reader；
- Harness `ModelTurnDispatchRepositoryV2`；
- Model `GovernedModelTurnPortV3`；
- 单调、可信的时钟。

Unknown 恢复 Inspect 会移除 caller cancel，但始终重新附加对应
Envelope/S1/共同最短 TTL deadline。SQLite Open 同时验证 schema ledger 的精确
版本集合、完整 DDL、列/index 元数据、无额外 trigger 和回滚约束 probe。
outcome Bind 即使已经 durable，也必须在正常返回或 exact Inspect 恢复后重取
fresh trusted clock；clock regression、共同 TTL 到点或 Outcome 不再 current
时只返回 fail-closed 错误，不把 durable fact 伪装成当前可用结果。

当前仓库没有上述 Pair Reader 的 production composition adapter，因此这里只能
用于 owner-local/测试组合，不能作为 production root。

## 验证

本轮已实际通过：

- 定向普通测试 `count=100`；
- 定向 race 测试 `count=20`；
- Harness 全量 ordinary；
- Harness 全量 race；
- Harness 全量 vet；
- gofmt、diff-check 与 import 边界扫描。

当前状态是代码候选，仍等待独立终审；未提交、未创建 PR。

详细设计见
[design](../../design/harness/exact-model-turn-adapter-v2/README.md)，落地计划见
[plan](../../plan/harness/exact-model-turn-adapter-v2.md)。
