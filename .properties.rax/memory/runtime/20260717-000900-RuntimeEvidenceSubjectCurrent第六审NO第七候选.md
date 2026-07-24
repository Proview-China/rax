# Runtime Evidence Subject Current V1 第六审NO与第七候选

- 时间：2026-07-17 00:09:00 +08:00
- 状态：第六次独立资产审计NO（P0=1/P1=1/P2=2）；第七机械候选已落盘，asset-only等待第七次双独立资产审计。
- 边界：未授权Go、Continuity adapter、production backend/root、durability或SLA；不改Evidence V2、OperationScope Evidence V3或Checkpoint Evidence V1。

## 第七候选修正

1. consumer权威改为host composition注入的bound Assembly/Binding association current proof；Lookup/Validation request只携`ExpectedConsumer`，不携association/principal且不提供discovery。
2. Readability Policy canonical新增exact `SubjectKeyDigest`、`ExecutionScopeDigest`、Consumer与`AllowRead`；明确live `ProviderBindingCurrentProjectionV2`不带tenant/scope，不得单独用于scope授权。
3. Record+Registration与Presence+Readability各自冻结具名Request/Result/Reader，方法集不与raw `EvidenceLedgerFactPortV2`或Tombstone write port兼容。
4. Reader Binding live映射改为含`BindingID`的九项exact坐标，并统一“命名derive input”措辞。
5. 测试矩阵新增bound association漂移/不可用、caller自报权威、两个窄Reader capability narrowing、Policy subject/scope/consumer/allow逐字段反例。

## 后续门禁

- 第七次Continuity+Runtime双独立资产审计未执行；
- 审计YES前不写Go，Continuity不得注入raw Evidence Fact/Governance Port。
