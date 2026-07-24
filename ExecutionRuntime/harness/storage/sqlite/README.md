# Harness Session/Event SQLite State Plane

本包是Harness唯一Fact Owner下的单节点持久后端，同时实现：

- `ports.SessionFactPortV4`：Session完整history、current pointer、highest revision及Execution Scope active唯一性；
- `ports.EventCandidateJournalPort`：按`source_component_id + epoch + sequence`精确追加/读取，序列从1连续单调；
- `DurableSessionEventCurrentReaderV1`：只证明当前数据库身份、schema、Session/Event计数与TTL窗口。

数据库固定启用WAL、foreign keys和FULL synchronous。Create/Append同坐标同内容幂等，换内容冲突；Session CAS绑定完整previous revision+digest；提交回包未知后，调用方只能按原Session/Event坐标Inspect。所有JSON行均strict decode并绑定独立row digest，schema版本集合和digest在每次打开时复核。

边界：Event里的completed/cancelled/failed仍只是Harness Claim/Observation。本包没有Runtime Settlement/Outcome写口，也不提供Harness factory、cleanup、deployment、production root或SLA；整体Component Release仍为`reference_only`。
