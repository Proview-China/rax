# 2026-08-01 Workspace Read Post-Actual Durability V2

## 事件

PR #79本地集成分支在v19 Publication/current/physical gate之后发现一个actual-point后结果
持久性P0：物理读取已经发生时，V1 Attempt/Reservation/OwnerCurrent TTL跨界会使Observed或
Unknown均无法可靠落盘。用户授权自行裁决后，Root批准additive V20，不再尝试延长V1 TTL
或重新Seal旧Attempt。

本轮已经冻结并实现V20合同层：pre-actual Qualification历史、neutral Rust journal exact
lookup/ref、post-actual Terminal历史，以及Sandbox-internal writer capability。Runtime Attempt
与Sandbox origin Attempt保持不同Owner坐标，由AdmissionAttemptBinding digest显式关联；
Completed后、kernel outcome S2前崩溃固定为Indeterminate无content，只有S2成功并形成exact
Observation与完整可重算S2 proof后才可Observed。Qualification新增exact Association与Workspace
Lease digest；S2 proof逐轴回证S1 closure；Indeterminate stage由journal state/error class确定性
推导，caller-sealed public proof不能直接取得Owner写能力。

## 当前真值

- contract/ports/internal capability：已实现，focused ordinary/race通过；
- SQLite schema V20与repository：已实现，append-only/strict schema/restart/8 handles门禁通过；
- kernel/Data Plane/Rust historical Inspect：已接通，公开Client raw workspace.read dispatch已关闭，
  物理IPC与journal evidence只存在于Kernel私有边界；
- full Sandbox ordinary/race/vet、Rust test/fmt/clippy、diff-check：通过；
- 8-handle natural-clock压测曾暴露late contender在写锁队列中跨过短TTL的真实liveness缺陷；
  已改为同一read-only SQLite snapshot检查已持久的完整triple并收敛到winner，
  ordinary×20与race×5通过；
- 独立P0/P1终审已通过：P0=0、P1=0、无实质P2；确认Storage→Kernel
  opaque terminal token是同一Sandbox Owner内的capability边界，无裸Fact writer、import cycle或physical旁路；
- PR #79远端仍未更新，本地V20尚未提交；
- V1/V2 wire/digest未修改；
- Tool typed handoff及两Turn纵切尚未解锁。

## 下一验收

完成SQLite V20与Rust/kernel接线后，必须证明S2 exact-expiry、Completed后S2前崩溃、
Terminal commit跨TTL、restart/lost reply、64并发、8 handles、Unknown无content、v19→v20与
ordinary/race/vet/Rust test/clippy全部通过，再更新PR #79。
