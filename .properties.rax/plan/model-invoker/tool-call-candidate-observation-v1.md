# Tool Call候选观测与公共投影v1实施计划

## 1. 状态

- 模块：`model-invoker`
- 当前状态：陈旧计划（已完成）
- 完成日期：2026-07-16
- 设计事实源：[Tool Call候选观测与公共投影v1](../../design/model-invoker/tool-call-candidate-observation-v1.md)
- 实现范围：`ExecutionRuntime/model-invoker/`

本计划只负责Model Invoker生成Tool Call候选Observation及公共Union投影，不创建`PendingAction`、`ActionCandidate`、执行授权、Tool Apply、Harness Continuation或Runtime Settlement。

## 2. 已完成产物

- [x] `ToolCallCandidateObservationV1`完整批次合同；
- [x] sync/stream共用的确定性finalizer；
- [x] 严格JSON Object规范化、digest和不可变copy；
- [x] `ToolCallCandidateObservationProjectionV1`及exact source/ref；
- [x] 公共严格Codec与`model_tool_call_candidate_observation`事件；
- [x] 逐Call兼容事件的非Gateway权威标记；
- [x] direct sync/stream终态后一次性构造并发出Union事件；
- [x] N>1完整批次与任一Call失败整批零产出；
- [x] Response ID冲突、空ID继承和无绑定拒绝；
- [x] 单元、白盒、黑盒、故障、并发和direct session测试。

## 3. 原子完成条件

- [x] 只有`completed/tool_call`终态可形成Observation；
- [x] 完整Projection事件在同一批兼容Call之前恰好发出一次；
- [x] stream partial、取消、EOF、lost terminal、重复complete和终态冲突均零投影；
- [x] N>1不会部分升级或因map覆盖丢失Call；
- [x] source ref包含精确Response ID和SourceSequence；
- [x] Call ref包含连续ordinal、唯一ID、name和canonical arguments；
- [x] Codec拒绝顶层及嵌套重复键；
- [x] Projection保持Observation性质，不获得动作或派发资格。

## 4. 验收记录

- [x] targeted ordinary `count=100`；
- [x] targeted race `count=20`；
- [x] 中央full ordinary；
- [x] 中央full race；
- [x] `go vet ./...`；
- [x] `gofmt`与diff检查；
- [x] 独立Review最终结论`YES`。

计划已经全部完成并保留为历史依据。后续Gateway把N=1 Observation转换为Harness PendingAction，属于Harness/Tool/Runtime的独立设计与授权范围，不回写本计划所有权。
