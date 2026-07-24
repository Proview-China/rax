# Tool Call候选观测与公共投影v1设计

## 1. 状态

- 模块：`model-invoker`
- 合同：`ToolCallCandidateObservationV1`、`ToolCallCandidateObservationProjectionV1`
- 当前状态：Observation/Projection构造、strict codec与direct事件输出已实现并通过中央门禁；Projection Publisher/Exact Reader仍是待联合评审的additive Delta
- 完成日期：2026-07-16
- 所有者：Model Invoker只拥有Provider调用结果的候选观测及其公共Union投影

本合同把模型返回的完整Tool Call批次变成可校验、可引用的Observation。它不是`PendingAction`、`ActionCandidate`、审批、派发或执行授权，也不拥有Runtime Operation Settlement。

## 2. 公共合同

`ToolCallCandidateObservationV1`描述一个`completed/tool_call`响应中的全部调用：

| 字段 | 约束 |
|---|---|
| `contract_version` | 固定为Tool Call候选观测v1 |
| `invocation_digest` | 由调用方提供并校验，Model Invoker不得伪造来源 |
| `response_status` / `stop_reason` | 必须严格为`completed/tool_call` |
| `calls` | 保持终态Output顺序；ordinal从0连续；Call ID唯一 |
| `canonical_arguments` | 必须是严格、规范化的JSON Object |
| `digest` | 覆盖调用来源、终态、调用顺序和规范参数 |

`ToolCallCandidateObservationProjectionV1`是公共Union中唯一可供后续Gateway读取的权威投影：

| 字段 | 约束 |
|---|---|
| `contract_version` | 固定为公共投影v1 |
| `ref.id/revision/digest` | Observation投影的精确身份、版本和摘要 |
| `ref.invocation_id/digest` | 精确绑定Invocation身份与请求摘要 |
| `ref.observation_digest` | 精确绑定完整Observation摘要 |
| `ref.source.response_id` | 最终绑定的Provider Response ID |
| `ref.source.source_sequence` | 公共Union事件的精确来源序号 |
| `observation.calls[]` | 完整保留ordinal、Call ID、name和canonical arguments |

公共载体固定为`ModelEvent.Kind = model_tool_call_candidate_observation`。读取方必须使用严格公共Codec解码和校验整个投影，不能只读取逐Call事件拼装权威输入。

## 3. sync与stream原子边界

sync和stream共用同一终态finalizer，当前事件发出顺序为：

```text
completed/tool_call终态
  -> 整批Validate/finalize
  -> 生成完整Observation及exact ref
  -> 校验整批Tool Call
  -> 恰好一次构造并发出完整Projection事件
  -> 可选发出逐Call兼容事件
```

- stream中的start、arguments delta和complete都只是provisional证据，终态前不对外发出权威Observation事件；
- N>1时仍只发出一个完整Projection事件，不允许把其中一个Call先升级；
- 任一Call非法、重复、冲突或无法关联时，整个批次零Projection、零兼容Call、零Pending升级；
- 取消、EOF、缺失终态和流错误同样零Projection；
- finalizer成功后，完整Projection必须先于逐Call兼容事件出现。

这里的“发出”只指当前direct session的Union事件输出。live尚无Projection Store、Publisher或Exact Reader，因此不等于Projection已经持久发布并可按Ref复读。该前置缺口由[Projection发布与Exact Reader V1 Delta](./tool-call-observation-projection-publish-reader-v1.md)承接；中央联合`YES`前不修改代码。

## 4. Response ID绑定

stream的来源身份遵循唯一绑定规则：

1. 非空`StreamEvent.ResponseID`会绑定流来源；后续非空ID必须完全一致；
2. 终态内嵌`Response.ID`非空时，必须与已绑定ID一致；
3. 终态`Response.ID`为空时，只能继承已经唯一绑定的Response ID；
4. 终态及此前事件都没有可绑定ID时拒绝finalize；
5. 公共Projection必须使用finalizer最终封存的Response ID，不使用未经校验的终态字段。

因此，Response ID冲突或无绑定空ID不会产生部分投影，也不会升级为后续动作。

## 5. 严格JSON边界

- Tool Call参数只接受JSON Object；array、scalar、null、非法UTF-8、trailing document、超限、重复键和partial JSON全部拒绝；
- 公共Projection Codec使用仓库`core.DecodeStrictJSON`；
- 顶层和任意嵌套对象的重复JSON key均拒绝；
- 未声明字段、trailing document和超限载荷均拒绝；
- 失败时不返回可被误用的部分Projection。

## 6. 兼容事件与所有权

逐Call `model_tool_call`只为现有读取方保留，并明确携带：

- `authority = compatibility_only_not_gateway_authority`；
- `gateway_authoritative = false`；
- 完整Observation ref；
- 对应连续ordinal。

后续Harness可以基于N=1的完整Observation建立自己的`PendingAction`，Tool领域可以再形成`ActionCandidate`，但这些转换不属于Model Invoker，也不能由Provider Observation自行完成。

## 7. 验收结论

- targeted ordinary：100轮通过；
- targeted race：20轮通过；
- 中央full ordinary、full race、vet、gofmt和diff检查通过；
- sync/stream、N=1/N>1、失败零投影、Response ID冲突/继承/缺失、顶层/嵌套重复键均有正反例；
- 独立Review最终结论：`YES`。

对应计划见[Tool Call候选观测v1完成计划](../../plan/model-invoker/tool-call-candidate-observation-v1.md)，使用说明见[模块说明](../../module/model-invoker/tool-call-candidate-observation-v1.md)。

后续additive计划见[Projection发布与Exact Reader V1实施计划](../../plan/model-invoker/tool-call-observation-projection-publish-reader-v1.md)。
