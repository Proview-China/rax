# Evidence Subject Current V1 测试矩阵

状态：**第七候选第一独立资产审计YES（P0/P1/P2=0/0/0）；asset candidate等待Continuity第二独立资产审计，implementation仍NO-GO**。

| ID | 类别 | 断言 |
|---|---|---|
| `ESCR1-C01` | canonical pair | exact完整Projection Seal后`Ref.Digest == ProjectionDigest`；清两digest重算稳定。same Ref换任一body字段、只换一侧digest、digest回流或错误非零派生字段重Seal均Conflict |
| `ESCR1-C01A` | stable ID pair | 同SubjectKeyDigest首次ID/revision=1成功；watermark变化只推进同ID revision+1。将watermark/revision/TTL/digest带入ID、换ID、revision gap/rewind均拒绝 |
| `ESCR1-C01B` | frozen shape | 四Ref与TombstonePresence/SubjectReadability/MutationKind/MutationKey字段数、字段类型、closed const、snake_case JSON tag与contracts exact；漏字段、alias、自由map及nil/empty歧义拒绝 |
| `ESCR1-C01C` | one-way seal | revision1先Seal Projection再Seal Index成功；Projection包含本次Index Ref/ID/revision/digest必须拒绝 |
| `ESCR1-C01D` | nested digest rejection | Index引用已Seal Projection成功；nested digest fixed-point、迭代Seal或任一环回填拒绝 |
| `ESCR1-C01E` | ref/body digest pair | exact Projection Ref.Digest等于完整body digest；任一侧换digest/body字段Conflict |
| `ESCR1-C01F` | derive golden | Subject/ProjectionID/IndexID/first Tombstone Mutation ID四个golden逐字节固定；字段顺序/domain/version/discriminator变化必须不同 |
| `ESCR1-C01G` | full frozen shapes | Key/Presence refs/Projection/Lookup/Snapshot/Validation/MutationRequest/MutationKey/MutationCommit字段数、Go类型、snake_case tag、strict JSON与Clone逐字段exact；漏ClaimKind/OwnerFact/HistoricalSource/Causation同样拒绝 |
| `ESCR1-C01H` | named derive inputs | ProjectionID/IndexID/MutationStableKey三个命名input struct字段与tags exact；anonymous/map/alias与`MutationID != string(StableKeyDigest)`拒绝 |
| `ESCR1-C01I` | association shape | Consumer Association Ref/Current Projection字段、snake_case tags、canonical/self digest与Reader单方法签名exact；request中出现association/principal字段拒绝 |
| `ESCR1-C01J` | owner-reader shapes | RecordRegistration与PresenceReadability两组Request/Result字段、tags、strict JSON、canonical/clone及Reader方法集exact；raw Fact Port结构不满足 |
| `ESCR1-ID01` | stable derive | 固定golden Subject得到四个固定digest/ID；TTL/watermark/revision改变Projection/Index ID必须拒绝 |
| `ESCR1-ID02` | monotonic revision | revision严格1、2、3；首次非1、gap、rewind、same revision换body拒绝 |
| `ESCR1-C02` | capability | Continuity只持Reader；无法调用Append/Renew/Tombstone/Fact CAS |
| `ESCR1-R00A` | bootstrap lookup | 只给ContractVersion+Subject+ExpectedConsumer+Scope+S1 Record Policy可取得首个current Snapshot；Gateway从构造时bound association proof获得真实consumer，caller无需且不能预知/替换association |
| `ESCR1-R00B` | bootstrap closed shape | Lookup JSON携Projection/Index/current/Binding/Capability/TTL字段时strict decode拒绝；current不存在只NotFound且不create |
| `ESCR1-C03` | boundary | 不引用或type-pun checkpoint-specific projection |
| `ESCR1-R01` | record exact | `InspectBySource`与按返回Ref的`InspectRecord`完整Record exact一致 |
| `ESCR1-R02` | source | registration ID/revision/full/config digest/state/TTL任一漂移Fail Closed |
| `ESCR1-R02A` | expected registration pair | exact ID/revision/fact digest/configuration digest/source ID/source epoch在request、projection、S1/S2全部一致时通过；逐字段fresh同ID漂移均Fail Closed |
| `ESCR1-R02B` | live binding mapping | `ProviderBindingCurrentProjectionV2`九项唯一映射得到Binding/Capability refs并在S1/S2一致时通过；Ref/Set digests/BindingID/revision/grant/projection/issued/expires任一换值拒绝 |
| `ESCR1-R02E` | deleted fields | Capability自由Revision/Digest/Authority与Binding Currentness/Projection/Checked/重复Expires字段不得出现在Go shape/JSON |
| `ESCR1-R02F` | mapping exact | 同一live projection只导出唯一nominal refs；换Binding revision、Grant/Projection digest、Issued/Expires任一值拒绝 |
| `ESCR1-R02C` | seven-owner S1/S2 | Record+Registration、Source Policy、Execution Scope、Producer Binding、Authority、Reader Binding+Capability、Presence+Readability七组S1/S2全部full-equal时通过；第1/7组必须走具名窄Reader；每组分别在S1/S2间换revision/digest/body/TTL均Fail Closed且Continuity publish计数0 |
| `ESCR1-R02G` | bound consumer association | Gateway构造时bound association ref与current Reader返回exact principal/consumer/scope时通过；stable ID只由principal+consumer component/capability+scope派生且revision+1；request换ExpectedConsumer、same ID换revision/body/digest、scope漂移、reader typed-nil/unavailable均零publish |
| `ESCR1-R02H` | no caller authority | request JSON携association ref/principal/current projection或自由consumer权威字段时strict decode拒绝；Gateway不提供association discovery |
| `ESCR1-R02I` | narrow owner readers | `EvidenceSubjectRecordRegistrationCurrentReaderV1`与`EvidenceSubjectPresenceReadabilityCurrentReaderV1`具名Request/Result成功；raw `EvidenceLedgerFactPortV2`与Tombstone Fact Port方法集不能满足两窄Reader |
| `ESCR1-R02D` | presence/readability current | exact Tombstone/Absence、OwnerWatermark、Readability state与Policy current通过；same presence ID换body、absence→present、readability/policy/TTL漂移逐项拒绝 |
| `ESCR1-R03` | policy | source policy ref/revision/digest/state/TTL或Owner/Authority漂移Fail Closed |
| `ESCR1-R04` | scope/binding | Execution/Ledger Scope、CurrentScope、Producer、Authority、Binding水位任一漂移Fail Closed |
| `ESCR1-R04A` | live governance closure | live `ExecutionScopeCurrentFactV2`/Producer `ProviderBindingCurrentProjectionV2`/`DispatchAuthorityFactV2`在S1/S2全部相同通过；任一Reader typed-nil、ref/body/digest/state/expiry漂移拒绝 |
| `ESCR1-R04B` | natural TTL | Expires分别被Execution Scope、Producer Binding、Authority current截断时exact取最小；忽略任一上限、用caller/Projection TTL或非正上限拒绝 |
| `ESCR1-R05` | tombstone | exact Tombstone存在时不可返回payload readable |
| `ESCR1-R06` | absence | Tombstone nil/NotFound不能替absence；必须exact sealed absence watermark |
| `ESCR1-R07` | retention | readability policy ref/state/TTL漂移Fail Closed |
| `ESCR1-R07A` | closed enums pair | `readable/policy_denied/retention_expired/source_inactive + absence`及`tombstoned + present`逐项合法；unknown/alias/大小写变体、任一非tombstoned状态+present、tombstoned+absence及矛盾one-of均拒绝 |
| `ESCR1-R07B` | five-by-two pair | 表中五个合法readability/presence pair逐项通过；其余组合、nil absence、present+非tombstoned拒绝 |
| `ESCR1-R07C` | consumer authorization | association/Lookup/Validation/Projection/ReadabilityPolicy的exact Consumer且`AllowRead=true`通过；Policy SubjectKeyDigest、ExecutionScopeDigest、consumer任一与当前坐标不等，或`AllowRead=false`均Forbidden且零publish；不使用live Binding projection假验tenant/scope |
| `ESCR1-R08` | stable seal | 重复historical/current读取返回同Checked/Expires/ProjectionDigest，不按读取时刻重封 |
| `ESCR1-R08A` | no reseal | historical/S1/S2/fresh返回同Seal；fresh调用产生新Checked/Digest拒绝 |
| `ESCR1-R09` | historical/current | 旧`(ProjectionID,Revision)`仍可historical exact Inspect；Current Index只指向新完整Ref，旧ref ValidateCurrent即使TTL未过也PreconditionFailed；新ref current成功 |
| `ESCR1-R10` | full-ref CAS/ABA | CAS仅在完整expected Index Ref和expected Projection Ref exact时成功；仅revision相同但digest/current/previous/watermark漂移、revision rewind/gap、旧ref复活、删除重建均Conflict |
| `ESCR1-R11` | typed nil | Record/Source/Policy/Binding/Index/Clock任一nil或typed-nil在backend mutation前Fail Closed |
| `ESCR1-R12` | closed errors | malformed/非法首seal为InvalidArgument；从未存在的historical或mutation前current absent为NotFound；same-ID/body/full-ref/ABA漂移为Conflict；旧current/撤销/过期为PreconditionFailed；shape合法但无reader capability、跨scope访问、装配不授权或readability policy拒绝为Forbidden；typed-nil/unavailable为Unavailable；S1/S2或半写不可判定为Indeterminate |
| `ESCR1-R12A` | forbidden four | 缺`praxis.runtime/read-evidence-subject-current`grant、Binding capability与grant不等、bound association consumer/scope与request expected不等、readability policy的Subject/Scope/Consumer/Allow不授权四族分别Forbidden且零projection/index/mutation |
| `ESCR1-E01` | closed categories | InvalidArgument/NotFound/Conflict/PreconditionFailed/Forbidden/Unavailable/Indeterminate逐项闭表；NotFound吞并Unavailable/Indeterminate拒绝 |
| `ESCR1-E02` | forbidden vs stale | capability/authority从未授予或不匹配为Forbidden；曾有效但已过期/撤销为PreconditionFailed，不互相改写 |
| `ESCR1-T01` | atomic source | source renew/revoke/expire与projection/index/absence全有或全无 |
| `ESCR1-CAS01` | first atomic publish | Owner事务内absent→revision1成功；外部NotFound冒充sealed absence拒绝 |
| `ESCR1-CAS02` | exact old index CAS | exact旧Index full Ref CAS成功；只比revision、旧digest、旧watermark均拒绝 |
| `ESCR1-ABA01` | immutable history | history永久可读、current单调；删除重建旧Index/Projection拒绝 |
| `ESCR1-T02` | atomic policy | accepted source-policy/readability binding推进与projection/index/absence全有或全无 |
| `ESCR1-T03` | atomic tombstone | Tombstone、non-readable projection、index与absence branch切换全有或全无 |
| `ESCR1-T04` | staged fault | 每个publish阶段失败后historical/current/index/absence均无半写；相同canonical可恢复 |
| `ESCR1-T04A` | one-way publish | Projection可先在事务内seal，但Index CAS失败时不得发布任何historical/current对象，Owner回滚为全无；故障注入/恶意backend出现“只有Projection”或“只有Index”均判Owner原子性失败，current不可见并返回Indeterminate |
| `ESCR1-T05` | lost reply three-way | 成功丢回包后exact immutable或合法progressed successor只Inspect恢复；same stable key换RequestDigest/body为Conflict；历史/index不可形成权威结论为Unavailable/Indeterminate且零重写 |
| `ESCR1-T05A` | stable mutation key pair | 同domain/version/subject/kind/full expected refs在不同new watermark/revision/TTL/time下导出同MutationID/StableKeyDigest；改变任一expected Ref字段必须导出不同key，随机caller ID不得影响key |
| `ESCR1-T05G` | immutable mutation request | four Kind分别只允许对应one-of payload；missing/duplicate/extra/type-pun、自由domain/discriminator、非零错RequestDigest重Seal全拒绝 |
| `ESCR1-T05H` | request-key-commit exact | RequestDigest、Subject/SubjectKeyDigest、Kind、expected Index/Projection三段exact且NewIndex.CurrentProjection==NewProjection通过；逐字段splice均Conflict |
| `ESCR1-T05I` | mutation ID relation | exact named stable-key input导出`StableKeyDigest`且`MutationID == string(StableKeyDigest)`；same stable key换RequestDigest是Conflict而非新ID |
| `ESCR1-T05B` | recovery lineage pair | 连续合法revision后继可恢复；revision rewind、gap、ABA、旁支、同revision换digest均Conflict |
| `ESCR1-T05C` | first-create lost reply | no-current稳定key首次create丢回包后，historical+index exact全有则幂等成功；全无或只一半不得盲重写，分别按权威读结果返回Indeterminate/Unavailable；same key换body Conflict |
| `ESCR1-T05D` | mutation commit proof | exact Commit存在且NewIndex仍current或在immutable ancestor链时lost reply幂等 |
| `ESCR1-T05E` | unrelated successor | Commit不存在但无关mutation已推进不得成功；返回Conflict/Indeterminate且零补写 |
| `ESCR1-T05F` | request drift | Commit存在但RequestDigest不同为Conflict |
| `ESCR1-T06` | concurrency pair | 64并发同immutable/full expected refs仅一次线性化且全部Inspect同一结果；64并发same expected refs换内容仅一胜，其余Conflict，Projection/Index ID不变且revision只推进一次 |
| `ESCR1-T06A` | commit concurrency | 64同内容只产生一个Projection/Index/Commit；64换内容仅一胜，其余Conflict |
| `ESCR1-S12` | same reader S1/S2 | S1/S2同typed Reader实例且live映射exact成功；换Reader实例或Binding ProjectionDigest时零Continuity publish |
| `ESCR1-STRICT` | strict JSON | Lookup/Validation/Projection未知字段、alias、重复JSON key、trailing document全部拒绝 |
| `ESCR1-T07` | tenant | 跨Tenant相同Record坐标不串读；subject key digest包含完整scope identity |
| `ESCR1-B01` | no authority upgrade | Projection不升级Trust、不创建Fact/Settlement/Timeline Event |
| `ESCR1-B02` | no caller TTL | RequestedNotAfter/Cursor/Event time不能改变Owner seal或自然Expires |
| `ESCR1-B03` | no production claim | fake/conformance不声明production backend、durability或SLA |

未来实现门禁：target ordinary `count=100`、race `count=20`、Runtime full ordinary/race/vet、gofmt、diff-check、import-boundary和public-only Conformance。当前未执行，因为本轮不写Go。
