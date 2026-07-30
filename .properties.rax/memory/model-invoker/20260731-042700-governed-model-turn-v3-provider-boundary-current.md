# Governed Model Turn V3 Provider Boundary Current owner-local 完成

## 当前事实

- 基线：`origin/main@8969b2159ea646e202f25e49f942ed12a028ad8b`
- 分支：`agent/governed-model-turn-v3-provider-boundary-current`
- Model-owned Provider Boundary V3 exact Ref、immutable Fact、SQLite V5 history 和 Runtime read-only current adapter 已实现。
- history 表为 `governed_model_turn_v3_provider_boundary_history`；create-once、无 current pointer。
- persistence result 明确区分 `created`、`existing`、`recovered_unknown`；64 并发恰有一个 `created`，丢回复/Conflict recovery 不会产生调用权。
- exact Turn Reader 输出必须与请求 Ref 完全相等；合法 sibling 也会 Fail Closed。
- V5 `table_list/table_xinfo/index_list` 和共享 `index_xinfo` 均检查迭代错误并显式处理 Rows Close 错误。
- V5 migration 已分流：existing ledger 只 verify、绝不 repair；无 ledger 且零 V5 objects 才创建；无 ledger 的 partial V5 拒绝；真实 V4 可升级。
- TTL crossing、clock rollback、Unknown lost reply、same identity splice、strict JSON、physical schema drift、V4→V5 migration 和 64 并发均有黑盒。

## 验证

- focused ordinary×100：PASS，229.865s（包含 V5 no-repair migration 反例）
- focused race×20：PASS，276.261s（包含 V5 no-repair migration 反例）
- Model Invoker full ordinary：PASS
- Model Invoker full race：PASS
- `go vet ./...`：PASS
- `git diff --check`：PASS
- import boundary：PASS；Provider、routegateway、Runtime guard、Harness、Context、Tool 调用为零
- 独立敌手审计：P0=0、P1=0

## NO-GO

本切片不是 production Provider dispatch。Context authoritative lowering、Harness exact closure、routegateway S3 与 Runtime guard 尚未组合；不得把 boundary Fact 或 persistence disposition 当作 Provider authority 或 Provider execution receipt。
