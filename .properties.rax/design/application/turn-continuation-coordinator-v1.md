# Application Turn Continuation Coordinator V1

## 状态

- 状态：owner-local实现候选；不代表production composition root完成。
- 目标：把已结算ToolResult之后可预先密封证明的Tool-only Context refresh与Harness ActiveContext CAS组成一个fail-closed续轮门。
- 非目标：不调用Model、Tool或Provider，不创建Console API，不修改HostV3或AgentPackage。

## 固定顺序

    Harness Begin(pending)
      -> ContextTurnRefreshCoordinatorV1
      -> Harness Commit(Expected ActiveContext CAS)
      -> ModelTurnAllowed(exact context_current)

Application只拥有跨Owner顺序。Harness拥有pending/current、Attempt、ActiveContext CAS与Model门禁；Context拥有Prepare/Apply/Inspect和Context事实。

## Unknown与并发

- Harness Begin/Commit回包为Indeterminate时，只Inspect原Attempt，不换key、不重派mutation。
- Context错误保持Harness pending；Application不盲重试Context mutation。
- 同一Application coordinator用本地single-call gate去重。
- 多Application coordinator与多Harness SQLite handle可共享同一个Context coordinator；Harness SQLite负责最终CAS，Context coordinator负责同Attempt的进程内single-call。
- 多个独立Context coordinator composition root或跨进程同Attempt并发目前没有durable Context claim/gate，明确为NO-GO。若要解锁，必须由Context Owner提供durable create-or-inspect/claim语义，不能在Application加盲重试。

## 安全门

- exact source/session/turn/scope/run/attempt必须一致；
- Tool-only payload在Begin前重算sealed Prepare/Attempt，same Attempt不同payload拒绝；
- Memory/Knowledge owner-backed refresh在V1固定CapabilityUnavailable，避免final replay在未证明完整refresh payload时早退；
- stale ActiveContext CAS保持Conflict，不Inspect成成功、不开放Model；
- TTL过期和clock regression均Fail Closed；
- context_current、Context result、Commit digest任一不exact均拒绝。

## 当前完成边界

完成owner-local Tool-only协调器、真实Context adapter/fixture与Harness SQLite黑盒、lost-reply/restart、多handle并发、payload splice/final replay splice、TTL/clock/CAS负例。尚未闭合Memory/Knowledge owner-backed续轮、production Host composition、跨进程Context coordination、真实下一Model执行或Console合同。
