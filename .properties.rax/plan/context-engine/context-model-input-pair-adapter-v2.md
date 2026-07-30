# Context Model Input Pair Adapter V2 落地计划

状态：已实现并通过门禁，保持未提交等待独立审计。

## 产物

- Context-owned full Material + durable Frame current request/projection合同。
- Context current source窄Reader port及kernel S1/S2实现。
- Context `modelinvokeradapter`严格子集lowering与Model Pair V2实现。
- contract、kernel、adapter、durable SQLite黑盒、并发、race和import测试。

## 执行清单

1. 保持Context Material V1 wire不变，复用现有exact/current SQLite Reader。
2. 新增projection，digest绑定raw Owner、完整Material、Frame current proof和TTL。
3. 构造时拒绝typed nil及Frame/Source Owner splice。
4. 实现三类已冻结lowering；所有未授权组合Fail Closed。
5. 通过Model公开ContextBody helper产生唯一mapped digest。
6. adapter执行两次完整Context source read，拒绝source、lowering、TTL和clock drift。
7. 对全部slice production Go文件执行逐文件精确import白名单及高风险denylist。
8. 验证ordinary ×100、race ×20、full ordinary/race、vet、diff和import边界。

## 完成条件

- ordinary路径Material exact/current和Frame均完成两层S1/S2闭包。
- Unknown、unavailable、cancel、now==expiry、clock rollback返回零projection。
- 64并发稳定窗口一致，flip窗口无授权成功。
- Provider、routegateway、Harness、Tool事实写入和调用为0。
- worktree保持未提交，交由独立审计。

以上条件已完成。
