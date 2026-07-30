# 2026-07-31 Workspace Read Exact Handoff V1

- PR #45 已合并，exact current Query/Projection/Reader 已成为 main 基线。
- 新切片开始闭合 actual-point query wiring 与 AdmissionReceipt 到 original AttemptRef 的 durable binding。
- 唯一 lookup 输入是完整 Runtime AdmissionReceipt；StableKey 和 AuthorizationDigest 不作为 lookup key。
- current 过期后可以读取历史 original AttemptRef，但不能恢复或重放执行。
- actual-point handoff 已升级为 additive V2：Base V1 + exact Reservation + original Attempt + Sandbox AdmissionReceipt。
- V1-only execute Fail Closed；Reservation/Attempt 漂移必须在 Rust `openat2/pread` 前形成 `physical_read_count=0`。
- exact Reservation Reader 与 original-Attempt-to-latest-current Reader 独立于 broad OwnerStore。
- workspace-read Execute 已在 Go 构造器、Go CurrentServer 与 Rust 合同三处强制 V2；V1、普通 Runtime V4 或空 query 均在 journal Begin 和 Provider 前拒绝。
- Go CurrentServer 对 SandboxProjection revision、digest、expiry 三轴逐项 exact 比较；V2 在 started 到 S1 或 S1 到 S2 任一时钟回拨时拒绝。
- query 构造失败改为确定性 failed/no-effect；只有跨过 actual point 后的不确定结果才能进入 Unknown/Inspect。
- 真实公共 UDS 黑盒已闭合，并覆盖 64 并发单次物理读取、Reservation/Attempt 漂移零读取、symlink/special/oversize/non-UTF8 Fail Closed。
- 2026-07-31 最终 ordinary、vet、race、Rust all-targets、clippy、fmt、公共链路与 diff-check 全绿。
- 当前仍为 Sandbox owner-local 能力；Tool production seam、Settlement/ToolResult、Host factory 和 Console 合同不在本切片内。
