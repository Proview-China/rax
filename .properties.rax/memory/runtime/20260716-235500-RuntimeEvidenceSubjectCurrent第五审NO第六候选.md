# Runtime Evidence Subject Current V1 第五审NO与第六候选

- 时间：2026-07-16 23:55:00 +08:00
- 状态：第五次独立资产审计NO（P0=3/P1=2）；第六机械候选已落盘，asset-only等待第六次双独立资产审计。
- 边界：未授权Go、Continuity adapter、production backend/root、durability或SLA；不改Evidence V2、OperationScope Evidence V3或Checkpoint Evidence V1。

## 第六候选修正

1. S1/S2从四组扩展为七组真实current闭包：新增live `ExecutionScopeFactReaderV2`、Producer `ProviderBindingCurrentnessPortV2`、`AuthorityFactReaderV2`；source/policy Authority均复读，所有current expiry进入natural min-TTL。
2. Lookup/Validation request携exact `ProviderBindingRefV2` consumer；Readability Policy canonical冻结Consumer与`AllowRead`，consumer不同或deny可证为`Forbidden`。
3. 新增immutable `EvidenceSubjectMutationRequestV1`，固定domain/discriminator与four Kind exact one-of payload；Request→Key→Commit精确绑定RequestDigest、Subject/Kind、expected Index/Projection与new refs。
4. 补齐`EvidenceTombstonePresenceV1`、`EvidenceSubjectReadabilityV1`、`EvidenceSubjectMutationKindV1`、`EvidenceSubjectMutationKeyV1`公开Go shape、closed const与snake_case JSON tags。
5. 冻结ProjectionID/IndexID/MutationStableKey命名erive input struct；`StableKeyDigest`exact等于stable-key canonical输出，`MutationID = string(StableKeyDigest)`，same key换RequestDigest为Conflict。

## 后续门禁

- 第六次Continuity+Runtime双独立资产审计未执行；
- 审计YES前不写Go，Continuity不得注入raw Evidence Fact/Governance Port。
