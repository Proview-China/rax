# TurnContinuationCoordinatorV1闭合与跨进程硬门

## 事件

Application新增最小续轮协调器，将Harness持久pending、Context refresh和ActiveContext CAS按固定顺序组合。真实Context fixture与Harness SQLite覆盖Begin/Context Apply/Commit丢回复、重启恢复和多handle并发。

## 裁决

- Coordinator不拥有Model、Tool、Provider执行权，也不执行下一Model Turn；
- Harness mutation unknown只Inspect原Attempt；Context失败不由Application盲重试；
- same Attempt不同Tool-only payload在进入Owner前拒绝；stale ActiveContext CAS、TTL过期和clock regression保持Fail Closed；
- owner-backed Memory/Knowledge refresh无法在final replay早退前从当前请求独立证明完整Prepare/Attempt，V1固定CapabilityUnavailable，等待后续Start绑定exact S1 projection；
- 多Application coordinator和多个Harness SQLite handle可以共享一个Context coordinator并发收敛；
- Context coordinator当前gate是进程内实现。多个独立Context composition root/跨进程same-Attempt并发暴露Prepare后Inspect的瞬时NotFound，当前明确NO-GO，不得用Application重试掩盖。

## 后续解锁

由Context Owner设计durable同Attempt claim/create-or-inspect与exact Inspect可见性；通过独立合同审核和跨进程黑盒后，才可移除该NO-GO。production Host、下一Model执行和Console接线仍分别等待自身Owner合同。
