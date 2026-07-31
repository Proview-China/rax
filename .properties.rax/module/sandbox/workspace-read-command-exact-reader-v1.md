# Workspace Read Command Exact Reader V1 Module

该模块发布 Sandbox-owned `WorkspaceReadCommandExactReaderV1`，按完整 exact Ref
返回原始 `WorkspaceReadCommandV1`。

## 保证

- exact ID、revision、digest 与 decoded body 全量一致；
- V17 owner-local row seal绑定完整exact Ref与full canonical body；
- Command与seal同事务create-once；legacy无proof且禁止迁移或retry自签；
- expired Command 在 restart 后仍可做历史复读；
- Command TTL 固定为 `Meta.Expires <= RequestedNotAfter`；任一 exact expiry
  边界与 owner clock rollback均Fail Closed；
- caller-bound authority 继续要求
  `RequestedNotAfter <= Runtime UnifiedNotAfter`；public Executor 纵切证明
  `Meta < Requested == Unified` 时 Admission/Reservation成功且Effective TTL取
  Meta，不能以 Meta 比较替代该纵深门；
- 旧实现下 `Meta.Expires > RequestedNotAfter` 的不安全持久 Fact 即使
  canonical body/row seal自洽，也拒绝历史与current读取且不会被重封或回填；
- 只执行 `ValidateShape`，不刷新 TTL、不恢复执行权；
- 64 个并发 Reader 返回同一 immutable winner；
- typed nil、取消、Unavailable、Ref/body/digest及Created/Updated splice均
  Fail Closed；
- seal表额外/隐藏列、type/null/default/PK漂移使Store启动失败；
- Reader 路径写入计数、Provider 调用与 physical read 均为零。

## 边界

该模块只是历史因果 Reader。Tool 的 production workspace.read recovery 仍需
同时消费 Runtime Attempt-to-Admission、Sandbox Admission-to-Attempt 与
Sandbox exact Attempt inspection；本模块单独不构成 production Agent Loop。
