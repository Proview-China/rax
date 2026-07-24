# Evidence Subject Current V1 实施计划候选

状态：**第七候选第一独立资产审计YES（P0/P1/P2=0/0/0）；asset candidate等待Continuity第二独立资产审计，implementation仍NO-GO，未授权Go实现**。

设计入口：[README](../../design/runtime/evidence-subject-current-v1/README.md)、[合同](../../design/runtime/evidence-subject-current-v1/contracts.md)、[Port Delta](../../design/runtime/evidence-subject-current-v1/port-delta.md)、[测试矩阵](../../design/runtime/evidence-subject-current-v1/test-matrix.md)。

## P1：设计冻结

- [x] 盘点`EvidenceSourceRecordReaderV2`、Source Registration/Policy、Tombstone和reference store同Owner锁；
- [x] 明确Record双读可复用但不足以证明current/readability；
- [x] 冻结通用Evidence subject类型，不type-pun checkpoint专用projection；
- [x] 冻结exact Tombstone或sealed absence watermark二选一；
- [x] 冻结historical Inspect与current Validate分离；
- [x] 冻结source/policy/tombstone/retention mutation与index/watermark同Owner原子publish；
- [x] 冻结old ref historical可读、current失效和no-ABA；
- [x] 冻结Continuity只持窄Reader、禁止Fact Port；
- [x] 冻结natural min-TTL且caller不能重封；
- [x] 冻结ProjectionID只由domain/version+SubjectKeyDigest派生，同ID revision+1且禁止watermark换ID；
- [x] 冻结完整Projection canonical清Ref.Digest/ProjectionDigest并将同一digest写回两者；
- [x] Lookup不要求expected current refs；Validation携S1派生的Registration/Reader Binding/Capability exact refs并在S2逐字段回扣；
- [x] 冻结domain/version/object discriminator、readability/presence闭表；
- [x] 冻结Owner mutation stable key、lost-reply progressed recovery与no-ABA反例；
- [x] 第二次独立只读资产复审YES（P0/P1/P2=0/0/0）。
- [x] 旧候选Reader Capability/Binding含无live来源字段；第四审后按`ProviderBindingCurrentProjectionV2`冻结映射表唯一派生两个nominal Ref；
- [x] 冻结stable ProjectionID、首次revision=1、后续严格`+1`及Ref/body同digest；
- [x] 冻结MutationKey精确字段与lost-reply同内容/换内容/不可判定三分；
- [x] 冻结C01/R02/R07/T05/T06成对正反例；
- [x] 第三次独立资产短审完成：NO（P0=3/P1=4），进入review-pending返修；
- [x] 删除Projection body中的Current Index Ref/digest，冻结先seal Projection、后CAS Index的单向摘要依赖；
- [x] 冻结首次Projection/Index revision=1+previous nil、后续stable ID/revision+1及immutable history；
- [x] 冻结完整expected Index/Projection refs CAS、no-current首创哨兵与no-ABA；
- [x] 第四候选曾冻结Record+Registration、Source Policy、Reader Binding+Capability、Presence+Readability四组Owner S1/S2 exact；第六候选已扩展为含Scope/Producer/Authority current的七组闭包；
- [x] 冻结closed errors、first/CAS lost-reply与half-publish/ABA adversarial反例；
- [x] 第四次独立资产审计完成：NO（P0=2/P1=4），停止Go patch map；
- [x] 冻结`InspectEvidenceSubjectCurrentV1`只读bootstrap lookup/snapshot：Subject+ExpectedConsumer+Scope+Record Policy定位，不要求caller预持current refs，NotFound不create；
- [x] 删除不存在的Binding currentness与Capability自由revision/authority字段，逐字段映射live `ProviderBindingCurrentProjectionV2`；
- [x] 冻结Key/Tombstone/Absence/ReadabilityPolicy/Projection/ValidationRequest完整Go shape、JSON、canonical与Clone；
- [x] 冻结derive domain/version/discriminator/input/output与golden；
- [x] 冻结immutable MutationCommit与lost-reply Commit+ancestor proof；
- [x] closed errors加入Forbidden并冻结四项零写反例；
- [x] 第五次独立资产审计完成：NO（P0=3/P1=2），未解锁Go；
- [x] S1/S2真实接入live `ExecutionScopeFactReaderV2`、Producer `ProviderBindingCurrentnessPortV2`与`AuthorityFactReaderV2`，三者exact current与expiry进入Projection/natural min-TTL；
- [x] 第六候选曾让Lookup/Validation携Consumer；第七候选已改为只携ExpectedConsumer并与bound association exact比对；
- [x] 冻结immutable `EvidenceSubjectMutationRequestV1`完整shape/domain/discriminator、four Kind exact one-of payload及Request→Key→Commit三组exact equality；
- [x] 补齐`EvidenceTombstonePresenceV1`、`EvidenceSubjectReadabilityV1`、`EvidenceSubjectMutationKindV1`、`EvidenceSubjectMutationKeyV1`公开Go shape与snake_case tags；
- [x] 冻结ProjectionID/IndexID/MutationStableKey命名derive input struct，且`MutationID == string(StableKeyDigest)`；
- [x] 第六次独立资产审计完成：NO（P0=1/P1=1/P2=2），未解锁Go；
- [x] Gateway冻结host composition注入的consumer association current proof；Lookup/Validation仅携ExpectedConsumer，不携association/principal也不提供discovery；
- [x] Readability Policy增加exact SubjectKeyDigest、ExecutionScopeDigest、Consumer与AllowRead，明确live Binding projection不能单独证明tenant/scope；
- [x] Record+Registration和Presence+Readability分别冻结具名Request/Result/Reader，方法集不与raw `EvidenceLedgerFactPortV2`兼容；
- [x] Reader Binding live映射文字从八项修正为含BindingID的九项，并统一为“命名derive input”措辞；
- [x] 第七候选第一独立资产审计YES（P0/P1/P2=0/0/0）；
- [ ] Continuity第二独立资产审计YES；
- [ ] 双独立资产审计完成并获得用户Go实现授权。

## P2：获授权后的公共合同

- [ ] 实现additive ports types、Validate、canonical和digest；
- [ ] 实现historical/current只读Reader与capability narrowing；
- [ ] 实现同Owner原子Store扩展和staged failpoints；
- [ ] 实现Gateway S1/S2/fresh current闭包；
- [ ] 实现public-only Conformance；
- [ ] 完成unit/whitebox/blackbox/fault/race/import tests。

## P3：门禁与交接

- [ ] target ordinary `count=100`；
- [ ] target race `count=20`；
- [ ] Runtime full ordinary/race/vet；
- [ ] gofmt、diff-check、assets links；
- [ ] 独立代码Review YES；
- [ ] Continuity adapter另行授权并只注入Reader。

## 非目标

本Plan不实现Go、不修改Continuity、不选择production backend/root，不扩Evidence V2/V3 closed schema，不授Timeline、Provider、Fact或Settlement权威。
