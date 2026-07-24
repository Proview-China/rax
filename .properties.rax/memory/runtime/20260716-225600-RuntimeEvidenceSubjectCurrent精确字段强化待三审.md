# Runtime Evidence Subject Current V1精确字段强化待三审

- 时间：2026-07-16 22:56 CST
- 状态：R-CTY-06第二次独立资产复审后的机械强化已落盘；asset-only，等待第三次独立资产短审，未授权Go实现。

本次冻结`EvidenceSubjectProjectionRefV1`、`EvidenceSourceRegistrationRefV1`、`EvidenceSubjectReaderCapabilityRefV1`和`EvidenceSubjectReaderBindingRefV1`四个Ref的精确Go候选字段、类型与snake_case JSON tag；ProjectionID只由固定domain/version和SubjectKeyDigest派生，首次revision为1、后续严格+1，Projection Ref与完整body共享同一digest。

Registration、Binding、Capability在request、projection与Gateway S1/S2必须逐字段exact；readability/presence保持closed enum与合法pair。Owner内部`EvidenceSubjectMutationKeyV1`固定稳定identity与RequestDigest，lost reply只允许exact/合法后继恢复、same key换内容Conflict、无法形成权威结论Unavailable或Indeterminate三分。C01/R02/R07/T05/T06 paired tests已写入矩阵。

本事件不授权Go、Continuity adapter、production backend/root、durability或SLA，不修改Surface Go、Evidence V2/V3或其他Owner资产。
