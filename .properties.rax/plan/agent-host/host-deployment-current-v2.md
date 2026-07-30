# HostDeploymentCurrentV2 实施计划

## 已实施范围

1. additive V2 contract、public reader/owner 与 raw repository seam；
2. Builder Selection/Closure、Runtime Resource 与 Host Service 的窄只读依赖；
3. S1/S2 currentness、最小 TTL 与 fresh seal；
4. SQLite v7 history/current CAS、lost-reply Inspect 与 restart；
5. exact import allowlist，只允许本切片使用的 public contract/ports。

## 验收

- create、advance、idempotency、stale predecessor、ABA；
- 64 并发争夺一个 current；
- restart、lost reply、superseded history、Selection 全轴 splice；
- strict JSON、schema ledger drift、弱表、伪注释约束、额外 index/trigger、`synchronous=FULL`；
- typed nil、clock regression、`now == expiry`；
- ordinary、race、vet、import boundary 与 `git diff --check`。

## 后续阻塞

只有 Builder Selection production reader、Resource/Service production current readers 与 HostV3 typed factory 全部独立冻结后，才允许新增 Composition 注入；本计划不提前实现。
