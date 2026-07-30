# Harness Exact Model Turn Adapter V2 代码候选

时间：2026-07-31 03:55（Asia/Shanghai）

## 事件

最初基于 `origin/main@85339b6e` 开始施工，并已安全 rebase 到
`origin/main@8969b215`。依赖的 Model
`GovernedModelTurnPortV3`，Harness owner-local exact adapter 已完成代码候选：

- 独立 Envelope/DispatchRef；
- SQLite `attempt_bound -> outcome_bound` durable sidecar；
- Context/Tool 四源 S1/S2；
- fresh clock 与共同最短 TTL；
- create/bind 回包丢失后的 exact Inspect 恢复；
- 重启与 64 个独立 Adapter 并发。

## 测试事实

实际通过定向普通100、race20、Harness全量ordinary/race/vet，以及
gofmt/diff/import边界检查。

## 当前边界

状态是 **owner-local代码候选，等待独立终审**。

当前没有 Context/Tool Owner 事实库到 Model 公共 Pair Reader 的 production
composition adapter，也没有本切面的 production root；不得以测试 Reader 或
SQLite单节点sidecar冒充生产闭环。

本次未调用Provider、未执行Tool、未创建PendingAction、未推进Turn，且未修改旧
Harness Loop/ModelTurnPort。
