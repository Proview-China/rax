# Runtime Evidence Subject Current V1 设计候选

状态：**第七候选第一独立资产审计YES（P0/P1/P2=0/0/0）；asset candidate等待Continuity第二独立资产审计，implementation仍NO-GO，尚未授权Go实现**。

## 1. 目标

本Delta为Continuity等只读消费者提供一个Runtime中立的Evidence subject current/readability证明。它复用现有`EvidenceSourceRecordReaderV2`完成immutable Record的`InspectBySource + InspectRecord`双读，但不把“Record仍存在”误当成source、policy、retention或payload readability仍current。

新增能力只表达：某个exact `EvidenceRecordRefV2 + EvidenceSourceKeyV2`在一个Runtime Evidence Owner密封水位上，对应哪个source registration、source policy、execution/current-scope、producer/authority binding、retention/readability状态，以及存在的exact Tombstone或Owner密封的absence watermark。

## 2. Owner边界

- Runtime Evidence Owner唯一拥有source lifecycle、Ledger Record、Tombstone、readability/retention overlay、subject-current index和absence watermark；
- Source Policy仍由其现有Policy Owner提供，Runtime Evidence Owner只复读并把exact ref/currentness密封进projection；
- Continuity只持有新增只读Reader；不得持有或注入`EvidenceLedgerFactPortV2`、`EvidenceGovernancePortV2`或任何Tombstone/Source mutation能力；
- 本合同不复制`CheckpointEvidenceSourceCurrentProjectionV1`，不把checkpoint专用类型type-pun成通用Evidence current证明；
- Projection不是Fact、Settlement、Trust升级或payload内容授权，只证明Owner在该水位给出的readability/current结论。

## 3. 核心不变量

1. `EvidenceSourceRecordReaderV2`只负责immutable Record exact双读；R-CTY-06不得复制第二份Record DTO或放宽为弱ID查询；
2. `Tombstone == nil`永远不是absence证明；无Tombstone时必须携Owner-issued sealed absence watermark；
3. source registration/policy、Tombstone、已接纳retention/readability binding的任何mutation，必须与subject-current index和absence watermark在同一个Runtime Evidence Owner锁/事务中全有或全无；
4. sealed projection的`CheckedUnixNano/ExpiresUnixNano/ProjectionDigest`稳定；重复Inspect不得按读取时刻重封；
5. 历史`ProjectionRef`永久可exact Inspect；current验证必须比较subject-current index与absence watermark，旧ref即使未过自然TTL也必须Fail Closed；
6. current index revision严格单调；相同subject不得回到旧revision/ref/digest，禁止ABA；
7. `RequestedNotAfter`、Cursor TTL、Event时间或调用方默认值不得进入Owner projection或伪造自然TTL；
8. typed-nil、无法区分absent与unknown、任一Owner reader不可用或S1/S2漂移均Fail Closed，不降级为“可读”。
9. `ProjectionID`只由固定domain/version与`SubjectKeyDigest`派生，Owner watermark、revision、TTL和digest不得进入ID；同一subject只能revision单调推进；
10. 首次lookup不要求caller预持Projection/Index/Registration/Reader Binding/Capability；S1从live `ProviderBindingCurrentProjectionV2`唯一映射Reader Binding/Capability refs，S2 Validation再携这些S1 exact refs逐字段回扣；
11. 四个核心Ref的字段、类型与JSON tag已经冻结；`ProjectionID`稳定、revision只允许`+1`，Projection Ref与完整body共享同一digest；
12. Owner mutation使用稳定`MutationKey`；回包丢失只允许“同immutable或合法后继恢复 / same key换内容Conflict / 无权威结论Indeterminate或Unavailable”三分，绝不盲重写。
13. Projection body不含Current Index Ref或Index digest；Owner先seal immutable historical Projection，再以其完整Ref原子CAS Current Index，摘要依赖严格单向；
14. 首次seal固定Projection/Index revision=1、previous=nil；后续同稳定ID严格`+1`并以完整旧Index Ref作CAS expected，历史Projection永久不可覆盖；
15. current验证必须对Record+Registration、Source Policy、Execution Scope Fact、Producer Binding current、Authority current、Reader Binding+Capability、Presence+Readability七组Owner current依赖执行S1/S2 exact双读，任一漂移均不可见。
16. public bootstrap是Reader上的只读`InspectEvidenceSubjectCurrentV1`：请求只含ContractVersion、Subject、expected Scope与来自S1 Record的SourcePolicy；按稳定IndexID返回sealed Snapshot，不存在current时NotFound且不得create。
17. 每次Owner mutation发布一个immutable `MutationCommit`，把RequestDigest、expected refs与新Projection/Index绑定在一起；lost reply只能用exact Commit+immutable history ancestor proof恢复。
18. Lookup/Validation request只携`ExpectedConsumer`；真实consumer必须来自Gateway构造时绑定的Assembly/Binding association current proof，caller不能自报或换association。Readability Policy把SubjectKeyDigest、ExecutionScopeDigest、consumer与`AllowRead`冻结进canonical，不得假设live Binding projection自带tenant/scope。
19. Gateway真实调用live `ExecutionScopeFactReaderV2`、`ProviderBindingCurrentnessPortV2`和`AuthorityFactReaderV2`；三个current结果进入Projection canonical与S1/S2 full-equality，各自expiry进入natural min-TTL。
20. Owner mutation的immutable Request、derived Key与Commit形成三段exact链：RequestDigest、expected Index/Projection及Subject/Kind必须逐段相等；Commit新Projection必须full-equal新Index的CurrentProjection。
21. Record+Registration与Presence+Readability各自使用具名Request/Result/Reader，方法集不与raw `EvidenceLedgerFactPortV2`兼容；Continuity/Gateway不得持写权Fact Port。

## 4. 入口

- [精确合同](./contracts.md)
- [公共Port Delta](./port-delta.md)
- [测试矩阵](./test-matrix.md)
- [实施计划候选](../../../plan/runtime/evidence-subject-current-v1.md)

## 5. 当前裁决

Live Runtime已有Record双读、Source Registration/Policy、append-only Tombstone、`ExecutionScopeFactReaderV2`、`ProviderBindingCurrentnessPortV2`与`AuthorityFactReaderV2`，但没有subject-current index、sealed absence watermark、readability/retention current projection，也没有让相关mutation与这些current水位同Owner原子推进的合同。第六次独立资产审计结论为NO（P0=1/P1=1/P2=2），第七候选第一独立资产审计已YES（P0/P1/P2=0/0/0）。第七候选保留已通过的七依赖、单向摘要、stable ID/revision+1、full-ref CAS/no-ABA与mutation链，并改为bound association-derived consumer、Policy exact subject/scope与两个具名窄Reader。当前仅为asset candidate，仍等待Continuity第二独立资产审计，implementation NO-GO，Continuity不得接到raw Evidence Store。

不选择production backend、数据库、RPC、durability或SLA。
