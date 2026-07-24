# Runtime Evidence Subject Current V1设计候选

- 时间：2026-07-16 21:37:01 +08:00
- 状态：R-CTY-06 Runtime Owner设计候选已闭合，等待联合设计Review；未授权Go实现。

本事件冻结了Evidence subject current/readability的最小Runtime Owner边界：继续复用`EvidenceSourceRecordReaderV2`做immutable Record双读；新增候选sealed projection覆盖source registration/policy、readability/retention、exact Tombstone或Owner absence watermark及subject-current index。相关mutation必须与index/watermark在同一Evidence Owner锁/事务原子推进；旧Projection Ref可historical Inspect但current验证失败，禁止ABA。

Continuity只能消费窄Reader，不能注入`EvidenceLedgerFactPortV2`；本候选不type-pun checkpoint专用projection，不选择production backend/root/durability/SLA，也未写Go。
