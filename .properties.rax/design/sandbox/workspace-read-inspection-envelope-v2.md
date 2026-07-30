# Workspace Read Inspection Envelope V2

## 目标

该 additive Sandbox Owner 合同解决一个单一问题：消费者持有
Admission 映射得到的 original `WorkspaceReadAttemptRefV1`（通常为 rev1），
而 terminal current 已推进为 rev2；V1 Inspect 只返回 current projection，
无法在响应内证明该 current 确由 caller 请求的 original exact ref 派生。

V1 不修改。V2 新增：

`InspectBoundedWorkspaceReadV2(ctx, exactOriginalAttemptRef)`

输出 `WorkspaceReadInspectionEnvelopeV2`：

- `RequestedOriginAttemptRef`：caller 提供的完整 immutable origin ref；
- `CurrentProjection`：Sandbox Owner 当前保存的 execution projection；
- `CheckedUnixNano / ExpiresUnixNano`：本次只读 inspection 的自然时间窗；
- `ProjectionDigest`：封存上述完整对象；
- namespaced `ContractVersion / TypeURL`。

## Owner 与读取边界

Sandbox 是 Envelope、Attempt origin/current、Reservation、Command 和
Admission-to-Attempt binding 的唯一 Owner。Runtime Admission 仍只是被
Sandbox immutable binding 引用的外部 exact ref；Tool 不拥有或推导这些事实。

公开读取只能使用完整 `WorkspaceReadAttemptRefV1{ID,Revision,Digest}`。
StableKey 只作为 Owner 内部索引，不出现在 V2 Request，不允许 stable lookup。

SQLite Reader 在一个只读事务中：

1. 按 exact origin ID 读取 history，并逐项验证 revision/digest/body；
2. 从 origin 内部取得 stable coordinate；
3. 复读 exact Reservation 与其 Command；
4. 按完整 origin tuple 复读 Admission binding；
5. 复读 current pointer及其完整 body；
6. 验证 origin 必为 create-once `started`，current 只能是同一 origin 或唯一
   rev+1 terminal successor；
7. 比较 stable/request/payload/Reservation/Command/WorkspaceView/
   Authorization/Admission 全闭包；
8. 读取完整 current projection并 seal Envelope。

没有 Provider、Rust Data Plane、workspace bytes 或 Runtime writer 调用。

## TTL、Unknown 与恢复

Reader 在读取任何 SQLite 对象前取得 initial clock；完成全部只读事务读取及
projection 验证后必须再次取得 fresh clock。fresh 为零、非正、早于 initial
均 Fail Closed。`CheckedUnixNano` 必须使用 fresh，禁止用事务开始前的旧时钟
自证返回时 current。

Envelope TTL 固定不超过 30 秒，只描述 inspection freshness，不续租、不授权、
不恢复任何已过期的 execution fact。若 current 仍为 started，Envelope expiry
还必须不晚于 origin、Reservation/TTLClosure、Command/requested-not-after、
Admission binding/receipt和current projection的最短可验证执行 TTL；fresh
到达该边界即拒绝。过期 historical origin 仍可读取 terminal current，其
Envelope 使用独立、从 fresh 起算的短 inspection TTL，不会令执行资格复活；
`now == envelope expiry` 必须拒绝。

UnknownOutcome 只允许用 original Attempt ref Inspect。lost reply、restart和重复
请求不得再次执行物理 read。Started recovery仍由现有显式 owner-incarnation
流程负责；V2 Inspect 本身始终只读。

## 兼容与 NO-GO

- V1 port、wire shape和行为不变；
- V2 是 Sandbox-only additive public reader；
- Tool 新 adapter 可消费 V2，但 Tool full composition不在本切片；
- Runtime、Application、Harness、Host、Console均不修改；
- 本切片不扩大 production Provider、Host root 或跨 Owner readiness 结论。
