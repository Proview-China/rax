# 2026-07-31 Context Model Input Pair Adapter V2

## 事件

基于已包含Model canonical ContextBody公开helper的最新main，Context Owner开始
落地最小可表达production lowering slice。

## 已冻结决定

- Owner唯一为`ExecutionRuntime/context-engine`。
- 新增完整Material + durable Frame current source projection/reader，不修改
  `ContextModelInputMaterialV1` wire。
- 只授权instruction/system|developer+UTF8、input_message/user|assistant+UTF8、
  function_call/assistant+canonical JSON object。
- mapped digest只调用Model公开
  `DigestGovernedModelTurnContextBodyV2`。
- Context provenance digest与Model mapped digest分域。

## 当前状态

实现与自动化验证已完成；已rebase到同时包含#60 canonical ContextBody helper
和#63 Model Boundary V3的
`main@4f695c79f42eb9eb033f42fd33ceeadf84f6c139`，不再存在stacked依赖。
V3只新增Model-owned Provider boundary，没有修改V2 Pair port或ContextBody helper；
Context adapter继续保持Provider、routegateway导入与调用为0。
focused ordinary ×100、focused race ×20、Context module full ordinary/race、
vet、go mod verify、gofmt/diff/import门禁全部通过。

当前worktree保持未提交等待独立审计。FunctionResult、reference和artifact
lowering继续Fail Closed。

## 独立敌手审计P2收口

独立审计结论为P0=0、P1=0、P2=1。最后P2已修复：

- 删除未接入真实dependency的恒真零副作用计数器；
- import门禁改为扫描全部slice production Go文件及共享Owner adapter；
- 逐文件精确白名单与网络、进程、SQL、unsafe、Provider SDK、Model
  provider/routegateway实现denylist同时生效。

修复后focused ordinary ×100、focused race ×20及full ordinary/race已通过；
其余发布门禁随同本次发布执行。
