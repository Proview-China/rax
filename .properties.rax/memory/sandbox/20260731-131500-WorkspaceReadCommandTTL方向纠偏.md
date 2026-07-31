# WorkspaceReadCommand TTL 方向纠偏

## 现状

`WorkspaceReadCommandV1` 的 owner-local TTL 不变量已纠正为：

`Meta.ExpiresUnixNano <= RequestedNotAfterUnixNano`

Meta expiry 是全部已验证权威上界的自然最小值；caller 请求的 not-after 只能是
上界，不能要求 Meta Fact 活得更久。`now` 到达任一 expiry 或 owner clock
回退时，current 资格立即 Fail Closed。

后续敌手核查确认 Sandbox kernel 与 Tool 的
`RequestedNotAfter <= Runtime UnifiedNotAfter` 不是旧方向残留，而是独立的
caller-bound authority 门；不得弱化成只比较 Meta。完整闭包为
`Meta.Expires <= RequestedNotAfter <= UnifiedNotAfter`。owner-local public
Executor fixture 已覆盖 `Meta < Requested == Unified` 的成功路径与
`Requested > Unified` 的 admission 前拒绝。

## 兼容与恢复

- 现有 Sandbox 与 Tool caller 都以 equal TTL 创建 Command，调用面不变；
- wire、digest domain、SQLite schema 与 public Port 均未改变；
- 不新增 Command Factory，不修改 Runtime、Tool 或 Provider；
- 既有 `Meta.Expires > RequestedNotAfter` Fact 被视为不安全历史数据：
  exact/current读取拒绝，且不得 reseal、backfill 或静默修复；
- 过期但满足新不变量的安全 Command 仍可 exact 历史复读，不能恢复执行资格。

## 验收状态

实现与 owner-local 反例已落入独立 worktree。focused ordinary×100、
race×20、Sandbox full ordinary/race/vet/module verify 与 Tool caller 回归均
通过；独立终审为 P0=0、P1=0、P2=0。生产代码仅修改一行 Command shape
不变量，kernel caller-bound authority 门保持不变，其余变更仅测试与 Sandbox
自有资产。

当前仍保持 uncommitted，等待提交授权；physical/provider/Runtime write 为零，
production 仍为 NO-GO。
