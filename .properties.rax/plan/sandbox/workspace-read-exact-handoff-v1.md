# Workspace Read Exact Handoff V1 Plan

## 施工清单

1. 为 kernel actual-point request 增加 sealed additive `WorkspaceReadCurrentQueryV2`；
2. 在 Physical Executor 内部从已复读 Base V1、Reservation、original Attempt 与 Sandbox AdmissionReceipt 构造 Query；
3. execute DispatchInput 只注入 V2 Query，并保持 prepare phase独立、V1 inspect兼容；
4. 新增 immutable Admission-to-Attempt binding 合同与 exact Reader port；
5. SQLite schema 与 Reserve 事务保存 binding；
6. SQLite Reader 只按完整 Runtime AdmissionReceipt exact identity 读取；
7. 增加 query splice、Reservation/Attempt S1-S2 漂移、receipt splice、V1-only拒绝、64并发、restart、current推进与过期后的历史 original ref 测试；
8. 运行 Go ordinary/race/vet、Rust fmt/test/clippy 与 diff-check。

## 产物

- Rust actual point 通过 UDS 消费 Sandbox kernel 生成的 V2 exact current query；
- Sandbox Reservation/Attempt 漂移在物理读取前以可信 no-effect evidence 关闭；
- Tool 后续可以从 Runtime AdmissionReceipt 获取权威 original WorkspaceReadAttemptRef；
- 不产生 Runtime/Tool/Console 新语义。
