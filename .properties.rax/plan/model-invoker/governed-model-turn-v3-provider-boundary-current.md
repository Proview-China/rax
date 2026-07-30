# Governed Model Turn V3 Provider Boundary Current Plan

## 实施范围

1. 定义 Model-owned Provider Boundary V3 exact Ref、immutable Fact、create-once Repository。
2. 返回闭集 persistence disposition：`created`、`existing`、`recovered_unknown`，但不产生调用许可。
3. 在 Model SQLite 增加 V5 history-only schema、逐列索引重算和 exact physical schema verifier。
4. 实现既有 `ModelProviderBoundaryCurrentReaderV1` 的只读 adapter。
5. 以真实 V3 Turn winner、Prepared current、ACK 和 Runtime request 完成 owner-local 黑盒。

## 验收

- restart 后 exact Inspect 与 Runtime projection 不漂移。
- 64 并发同 identity 只有一个 `created`，其余只能是 `existing` 或 `recovered_unknown`，且历史行仍只有一个。
- same identity 异 ACK、receipt、Runtime request、Provider 或 TTL 必须 Conflict。
- commit lost reply 与 Conflict recovery 只 Inspect 原 exact Ref，并返回 `recovered_unknown`。
- initial exact Inspect 命中返回 `existing`；repository 谎报 Applied 或返回异 body 必须 Fail Closed。
- Turn exact Reader 必须返回请求的 exact Ref，不能用另一个合法 Turn 替代。
- fresh/current、clock rollback、`now == expiry`、typed nil、cancel、Unavailable 均不返回 current winner。
- strict JSON、indexed row、DDL、CHECK、index、trigger、migration drift 均 Fail Closed；schema Rows 必须检查迭代错误并显式处理 Close 错误。
- V5 ledger 声明后的缺表/缺索引不可自动修复；无 ledger 的 partial V5 拒绝；真实 V4 且无 V5 对象才允许升级。
- Provider、routegateway、Runtime guard、Harness、Context、Tool 调用计数为零。
- focused ordinary×100、race×20、full ordinary/race/vet/diff/import boundary 全绿。

## 后续硬阻塞

真正 Provider dispatch 必须另开 routegateway PR，并等待 Context authoritative lowering 与 Harness V2 exact closure；本计划不提前实现第二半。
