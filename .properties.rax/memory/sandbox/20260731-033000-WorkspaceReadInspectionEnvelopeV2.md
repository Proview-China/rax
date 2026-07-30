# 2026-07-31 Workspace Read Inspection Envelope V2

- 用户授权直接实现 additive V2，不重启既有设计。
- V1保持不变；V2 Envelope同时封存 requested immutable origin Attempt ref和
  latest current projection。
- Sandbox SQLite复用现有 origin、Reservation、Command、Admission binding、
  current与Observation记录，在一个只读事务中完成 exact lineage closure；
  未新增stable lookup或第二真值表。
- 过期历史terminal仍可Inspect；Envelope独立短TTL不恢复执行资格。
- Unknown/lost reply/restart只Inspect original Attempt，Provider和物理读取为零。
- owner-local实现与验收完成后可供Tool新adapter消费；Tool full composition、
  Runtime/Application/Harness/Host/Console仍NO-GO。
