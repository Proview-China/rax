# Workspace Read Command Exact Reader V1

## 目标

该 additive 切片由 Sandbox Owner 唯一持有，为已经创建的
`WorkspaceReadCommandV1` 发布按完整 exact Ref 读取的历史 Reader。

它只解决 lost reply、进程重启或执行资格过期后的因果复读：

- Tool 可以用 Admission-to-Attempt binding 中的 exact Command Ref 读取原始
  Sandbox Command；
- Reader 返回 Sandbox 已持久化的原 Fact，不复制、不重封、不生成新的事实；
- Reader 不授予执行权，也不恢复已经过期的 Runtime、Sandbox 或 Provider
  current。

## 历史语义

`WorkspaceReadCommandV1` 以 `command_id` create-once 保存。现有表名包含
`current`，但 Command 没有 advance/update 路径；历史 Reader 仍必须通过完整
`ID + Revision + Digest` 核对唯一 immutable winner。

SQLite V17 增加 Sandbox-owned `workspace_read_command_body_seal`。新 Command
只允许在同一事务中同时 create-once 写入 Command 行和
`exact Ref + full canonical body` 的 domain-separated row seal；seal 写入或
commit 失败时 Command 行不可见。

V16 及更早版本的既有 Command 没有可信 create-time full-body seal。迁移只创建
空 seal 表，禁止从现存 body 自动回填，也禁止通过同 Fact `Create` retry 补
proof；这类 legacy Command 的 exact read 固定返回 Conflict。

读取算法固定为：

1. 校验 context 与 exact Command Ref；
2. 只按 Command ID JOIN 唯一 Command 与 body seal；
3. 要求 seal 存在并逐字段比较 revision、digest 与 exact Ref；
4. strict decode body并要求原始 bytes 等于 canonical re-encode；
5. 重算完整 canonical body row seal并exact比较；
6. 只执行 `ValidateShape()` 并要求 `Meta.Ref() == exact`；
7. 返回原始 Fact。

历史读取禁止调用 `ValidateCurrent`，禁止读取 stable key，禁止 fallback 到
mutable current，禁止刷新 `ExpiresUnixNano` 或
`RequestedNotAfterUnixNano`。Command 已过期仍可被读取，但它保留原始过期
时间且不能据此重新执行。

`WorkspaceReadCommandV1` 的 TTL 方向冻结为
`Meta.ExpiresUnixNano <= RequestedNotAfterUnixNano`。Meta expiry 是创建时所有
已验证权威上界的自然最小值，不能被 caller 的 requested bound 延长；
caller bound 本身仍必须满足
`RequestedNotAfterUnixNano <= Runtime Authorization.UnifiedNotAfterUnixNano`，
因此完整闭包是 `Meta.Expires <= RequestedNotAfter <= UnifiedNotAfter`。
Tool 与 Sandbox kernel 的 caller-bound authority 复验不得改成只比较 Meta；
`now == Meta.ExpiresUnixNano` 或 `now == RequestedNotAfterUnixNano` 均不再
current，owner clock 回退也 Fail Closed。既有持久 Fact 即使 body 与 row seal
在旧实现下可自洽，只要 `Meta.ExpiresUnixNano >
RequestedNotAfterUnixNano`，历史 exact read 也必须返回 Conflict，且禁止
重封、回填或静默修复。

## Owner 边界

公开接口属于 `ExecutionRuntime/sandbox/ports`：

`InspectWorkspaceReadCommandExactV1(ctx, exactRef)`

Tool、Runtime、Harness、Application 与 Host 只能注入该只读 Port；不得读取
Sandbox SQLite、推导 Command、复制 Sandbox DTO 或自行签发历史证明。

NotFound、Unavailable、typed nil、取消、存储损坏或任何 Ref/body/digest
splice 都 Fail Closed。该 Reader 不调用 Provider、不执行物理读取、不写
Owner store。

V17 seal 表通过 `PRAGMA table_xinfo` 严格核对列顺序、名称、类型、NOT NULL、
default、PK与hidden状态；额外普通列、generated/hidden列或任何物理形状漂移都
阻止 Store 打开。
