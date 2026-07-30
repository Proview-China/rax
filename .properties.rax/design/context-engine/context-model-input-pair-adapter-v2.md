# Context Model Input Pair Adapter V2 设计

## 目标

由唯一 Owner `ExecutionRuntime/context-engine` 证明一个完整
`ContextModelInputMaterialV1` 与其 durable current Frame 仍为同一
authoritative current source，并把冻结的最小子集无损映射为 Model
`Instructions/Input`，实现公开
`modelinvoker.InvocationMaterialContextPairExactReaderV2`。

## 冻结范围

| Context Channel | Role | Encoding | Model映射 |
|---|---|---|---|
| `instruction` | `system` / `developer` | `utf8`且非空白 | `Instruction{Role,Text}` |
| `input_message` | `user` / `assistant` | `utf8`且非空白 | `MessageInput(Role,Text)` |
| `function_call` | `assistant` | `canonical_json`且根为object | `FunctionCallInput(CallID,Name,Content)` |

`function_result`、`reference`、`artifact_ref_json`、其他Role/Encoding、
空白文本和非object function arguments全部Fail Closed。

## Authoritative流程

1. Context current source reader以raw `OwnerRef`和Material/Frame exact
   coordinate为输入。
2. 每次读取均执行S1/S2：Material exact、Material current、durable Frame
   exact-current、Frame reader immutable Owner binding。
3. projection包含完整Material、Frame current projection、raw Owner、
   checked/expires及独立Context-domain digest。
4. Model adapter双读该projection；两次分别lower，并只调用Model公开
   `DigestGovernedModelTurnContextBodyV2`。
5. expected mapped digest必须与S1、S2均一致，最后由Model公开sealer产生
   neutral Pair projection。

## 边界

- 不修改`ContextModelInputMaterialV1` wire/digest。
- 不复制Model canonical digest算法，不伪造RouteCall。
- 不导入Model实现子包。
- production文件逐文件使用精确import白名单，并显式拒绝网络、进程、SQL、
  unsafe、外部Provider SDK及Model provider/routegateway实现包。
- Provider、routegateway、Harness、Tool不写Context或Model事实。
- Context projection digest与Model mapped-input digest保持不同domain。

## 当前限制

这是明确的严格子集，不授权FunctionResult、reference或artifact lowering；
这些能力仍需各自公开合同后另行设计。
