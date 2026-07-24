# Runtime Evidence Subject Current V1 第四审NO与第五候选

- 时间：2026-07-16 23:26 +08:00
- 状态：第四次独立资产审计NO（P0=2/P1=4）；第五候选已完成Runtime自有asset-only返修，等待Continuity+Runtime双独立第五审。
- 范围：仅`.properties.rax/design/runtime/evidence-subject-current-v1/**`与`.properties.rax/plan/runtime/evidence-subject-current-v1.md`；未写Go，未改Continuity，未选择production backend/root。

## 本轮冻结候选

1. `EvidenceSubjectCurrentReaderV1`新增只读`InspectEvidenceSubjectCurrentV1` lookup：只以ContractVersion、exact Subject、Scope和S1 Record Policy按稳定IndexID返回sealed Snapshot；caller不预持current refs，NotFound不得create。
2. Reader Binding/Capability nominal refs按live `ProviderBindingCurrentProjectionV2`冻结映射表唯一派生；删除无来源的Binding currentness字段与Capability自由revision/digest/authority。
3. 冻结Key、Presence/Policy refs、完整Projection、Lookup/Snapshot/Validation request的完整Go shape、snake_case JSON、canonical与Clone规则。
4. 冻结SubjectKey/ProjectionID/IndexID/first Tombstone Mutation ID的derive domain/version/discriminator与四个literal golden。
5. 新增immutable MutationCommit，将RequestDigest、完整expected refs与新Projection/Index纳入同Owner原子发布；lost reply只接受exact Commit及immutable history ancestor proof。
6. closed errors增加`core.ErrorForbidden`语义及缺capability、binding/grant不匹配、跨scope、readability policy拒绝四项零写反例。

## 保持不变

- Projection摘要不读取Index，Index只读取已seal Projection Ref，保持单向断环。
- stable Projection/Index ID、首次revision=1、后续严格+1、immutable history/full-ref CAS/no-ABA不回退。
- Tombstone present与sealed absence one-of、readability/presence闭表不回退。

## 边界

当前仍是asset candidate；Go实现、Continuity Adapter、production persistence/durability/SLA均未授权。第五双审YES前不得按此候选落码。
