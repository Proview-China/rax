# Harness Exact Model Turn Adapter V2 Prepared/ACK S1/S2 返修

PR #62 独立终审发现唯一 P1：原候选只重读 InvocationMaterial 与 Context/Tool
Pair，Prepared Historical、Prepared Current 和 Commit ACK 没有进入 S1/S2。

本次原地收紧尚未合并的 V2：

- Envelope 新增 exact `PreparedModelInvocationCommitAckRefV1`，并绑定
  Command Prepared/Current 与 ACK TTL；
- Harness 内定义只读 structural ACK exact reader，只复用 Model 公共 ACK
  Ref/Fact 类型；不修改 Model public contract，现有 CommitGate 实现自然满足；
- 每次 S1/S2 exact-read Prepared Historical/Current/ACK/InvocationMaterial/
  Context Pair/Tool Pair，并复用 Model dispatch receipt guard 验证 lineage、
  currentness 和共同最短 TTL；
- SQLite canonical 行新增 `ack_ref_digest` 物理交叉绑定，旧 V2 schema ledger
  无法被 silent repair；
- 新增 Prepared/Current/ACK splice、revision/digest/TTL、Gate drift、
  unavailable、typed-nil 与 S1→S2 每轴漂移负例。

边界不变：不调用 Provider、不执行 Tool、不创建 PendingAction、不推进 Turn 或
next-turn；真实 Context/Tool Pair Reader composition 与 production root 仍为
NO-GO。
