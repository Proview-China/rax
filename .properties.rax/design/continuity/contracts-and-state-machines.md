# Continuity 合同与状态机

状态：**Runtime Checkpoint-first V2、Continuity Manifest/Seal、C-01及Restore最小公共参考纵切已落地**。纵切已覆盖Intent、Reservation/Eligibility、Admission/Review/Permit/Begin、双重Enforcement、Sandbox Stage、Evidence/Settlement、Context物化与Activation；production trusted Assembler、跨Owner全量Participant、远程Provider与root仍NO-GO。

## 1. 合同状态

本文定义Continuity领域合同候选，不代表Runtime/Harness公共Port已经具备这些能力。所有跨Owner部分必须先通过`port-delta.md`联合评审；组件不得用私有接口伪装公共合同。

建议初始领域合同版本：`praxis.continuity/v1alpha1`。进入实现前由联合评审冻结最终version、Schema Digest和Capability名称。

## 2. 共同引用

### 2.1 `ContinuityScopeV1`

必须携带：

- Tenant；
- Identity ID+Epoch；
- Lineage ID+Plan Digest；
- Instance ID+Epoch；
- 可适用的SandboxLease ID+Epoch；
- 可适用的Run ID与Run identity digest；
- Authority Epoch；
- canonical `execution_scope_digest`。

查询可以选择较宽分区，但写入必须携带形成事实时的完整Execution Scope。Scope漂移只能形成新revision或迟到历史，不能覆盖旧记录。

### 2.2 `OwnerBindingV1`

字段：BindingSet ID/revision、Component ID、Manifest Digest、Artifact Digest、Capability、Fact kind。它标识语义Owner，不授予Authority或Dispatch权。

### 2.3 `VersionedFactRefV1`

字段：ID、revision、digest、schema ref、Owner binding、Scope digest、created/updated time。所有CAS都使用`expected_revision + next_fact`，不接受last-write-wins。

### 2.4 `ResidualRefV1`

字段：ID、kind、owner、scope、subject digest、state、inspection ref、conflict domain、created/updated time。Residual与Settlement、Cleanup正交，不能用单个`failed`字符串折叠。

### 2.5 Governance V2跨Owner exact ref闭包

V2不新造各领域事实类型，只冻结调用方必须携带并由Owner Reader复读的最小引用封套：

| 字段 | 不变量 |
|---|---|
| `contract_version/schema_ref` | 由对象Owner发布；Continuity不得把legacy schema解释为V2 |
| `owner_binding` | 绑定Component、Manifest/Artifact digest、Fact kind；不授予Authority |
| `id/revision/digest` | 精确历史身份；同ID/revision换digest为Conflict |
| `scope_digest` | 必须与Checkpoint/Restore Execution Scope闭包一致 |
| `currentness_ref`（按需） | TTL、水位或current projection只能由Owner定义；历史ref本身不伪造current |

Manifest必须保存这些字段的canonical闭包及`frozen_ref_set_digest`。读取时由对应Owner以exact ref Inspect；Continuity不得复制Owner状态枚举、Runtime Outcome、Review Verdict或Provider语义来替代复读。

## 3. Timeline对象

### 3.1 `TimelineProjectionRequestV1`

| 字段 | 约束 |
|---|---|
| `attempt_id/idempotency_key` | 唯一stable create-once身份；lost reply后不得更换 |
| `evidence_source_key` | registration+source epoch+source sequence请求坐标；实际Record/sequence/digest由Runtime Reader返回 |
| `expected_evidence_record_ref` | 可选expected Evidence V2精确Record Ref；只作期望比较，不授Trust/current |
| `owner_fact_ref` | 仅`authoritative_fact`必需的Owner exact请求坐标；其余五类不得用generic OwnerFact升权 |
| `projection_policy_ref` | 权限、脱敏、Retention策略 |
| `scope` | caller声明的目标Execution Scope期望；必须由Owner Readers复核 |
| `requested_not_after` | `0`=caller不加上限；`<0`非法；`>0`只能缩短Owner current上限，不能续期 |

这是production Request允许的完整字段闭集；不得增加caller semantic kind、custom class、Parent/Causation/Correlation/Object refs、payload ref/schema/revision、Observed/Recorded time或任何可信ledger sequence、ledger/record/candidate/chain/payload digest、Trust、Owner Fact内容、Runtime Outcome、Evidence admission、Owner current/production current。Event语义、因果关系、payload、sequence、digest、Trust和时间全部从Runtime Evidence Record及按需Owner Fact sealed projection派生。live Wave 1 `TimelineProjectionCandidate/EvidenceAdmission`中的sequence/digest/Trust、`AdmittedByLedger`和`InspectedByOwner`只可作为reference测试fixture；不得经Adapter复制到新Request或生产Attempt。

### 3.2 `TimelineEventRecordV1`

字段：immutable Event ID/revision/digest、exact Projection Attempt ref、Runtime Reader派生的Evidence Record Ref/Source Key、ledger scope digest/sequence、Record/candidate/chain digest、六值TrustClass、Observed/Ingested Unix Nano、Payload ref/schema/digest/revision、Reader派生的semantic/parent/causation/correlation/object refs、Projection Policy ref、Projection revision与created time。Event不得保存可变`Visibility`或`TombstoneRef`字段。

不变量：

1. `ledger_sequence`必须与Evidence Record一致；
2. 同Evidence Record/source binding只能产生一个语义相同的可见Projection；
3. 投影内容漂移必须新revision并保留旧revision；
4. Event一旦create即永久immutable；Tombstone、Retention、Rebuild或current-index变化不得改写其任何字段或digest；
5. Provider/Model Invoker的stream/completed/cache usage/Provider状态只是Observation：只有它已由Runtime Evidence Owner摄取为精确Evidence Record后，Timeline才可把它投影成明确标记为Observation的TimelineEventRecord；该投影不得冒充Fact、Settlement或终态。若要投影权威领域结论，则必须等待对应领域Owner独立Inspect并形成Fact/Settlement，并同时引用原Observation Evidence。

`TimelineEventRecordV1`是不可变历史记录，不携`current=true`，也不因可被读取而自动获得TTL/current资格。payload只能通过exact Evidence Record公开projection形成；仅`authoritative_fact`再关联Owner Fact projection。Continuity不解析Model/Tool私有identity，也不成为其Owner。

### 3.2.1 `TimelineProjectionTombstoneFactV1`

Tombstone是独立immutable Fact，不是Event mutation。字段：Tombstone ID/revision=1/digest、Continuity Owner Binding、Tenant/Scope、exact Event ref、Runtime Evidence Tombstone/ref或Retention policy basis、reason、created time。相同ID同canonical内容幂等，同ID换Event/reason/policy为Conflict；lost reply只Inspect原Tombstone。

Continuity的`TimelineProjectionVisibilityIndexV1`以结构化subject key指向当前Event和可选Tombstone exact ref，并按expected revision CAS。Create Tombstone必须在同一Continuity Owner事务中create immutable Tombstone Fact并推进Visibility/current index；Event history保持原digest。Query/Watch通过Event history与current Visibility/Tombstone overlay组合输出，不能把overlay写回Event。Tombstone存在时payload默认不可读，只允许Policy允许的历史metadata；不得删除Tombstone后静默复活Event。

Runtime `EvidenceTrustClassV2`六类路由与输出权限固定如下，禁止合并或隐式升级：

| Ledger TrustClass | generic Owner Fact current Reader | Timeline输出权限 | 硬禁止 |
|---|---|---|---|
| `observation` | 禁止 | 同TrustClass历史Event与短时readability projection | 升级为Fact/Settlement/current authority |
| `late_observation` | 禁止 | 同TrustClass迟到历史Event；保持Runtime迟到限制 | 冒充current run/effect Evidence |
| `receipt` | 禁止 | 同TrustClass Receipt投影 | Provider回包成为领域Fact |
| `attestation` | 禁止 | 同TrustClass Attestation投影 | generic OwnerFact升级为authoritative |
| `claim` | 禁止 | 同TrustClass Claim投影 | 直接成为Run终态或Settlement |
| `authoritative_fact` | **必需** | exact Owner Fact current S1/S2通过后输出同TrustClass Event/current-binding projection | 缺Owner Fact或Reader仍visible |

某领域若要为`attestation`或`claim`增加current证明，只能发布该领域独立typed Delta/Fact projection并与原Event关联；原Ledger TrustClass保持不变，Continuity和generic OwnerFact Reader都不得改写Runtime trust语义。

### 3.3 `TimelineProjectionAttemptFactV1`

字段：contract/schema、Continuity Owner Binding、Tenant/Execution Scope digest、Attempt ID/revision/digest、idempotency key、Request闭集坐标digest、Runtime Evidence Owner exact Record/Source refs与immutable sealed S1/S2 projections、仅`authoritative_fact`的领域Owner Fact exact ref与immutable sealed S1/S2 current projections、Projection Policy exact/current projection、Reader派生的Event canonical digest、共同`checked_at/not_after`、状态、结果Event ref、Diagnostic/Residual refs、created/updated time。Attempt不保存caller `EvidenceAdmission`、caller semantic/payload/causal内容或caller sequence/digest/Trust/current副本。

Attempt current/history使用结构化`TenantID + ScopeDigest + AttemptID + Continuity Owner Binding`键；同ID同canonical内容幂等，同ID换Evidence/source/Owner/Policy/Scope/payload任一binding为Conflict。历史revision不可覆盖，current CAS必须携expected exact ref，lost reply只Inspect原Attempt。

### 3.4 `TimelineProjectionCurrentV1`

这是唯一可向生产caller表达“在短窗口内metadata/current bindings及payload readability状态”的密封projection，字段至少包含exact Attempt/Event ref、Evidence subject key与Projection Ref、Evidence S1/S2 projection digest、subject-current index revision/digest、closed readability state、exact Tombstone ref或Owner-defined absence watermark/current revision、Retention/readability policy current ref、按需Owner Fact S1/S2 projection digest、Projection Policy current ref/digest、Reader binding set digest、checked-at、not-after与projection digest。它不是Event字段，也不得持久为无限期资格；`current`不等于Trust升级。

`RequestedNotAfter == 0`表示caller不加上限，`< 0`为`invalid_argument`，`> 0`只能截短。它只属于Continuity aggregate请求，不传给任何Owner Reader，也不进入Evidence/Owner sealed ProjectionDigest。Owner Readers先返回与caller无关的自然Checked/Expires/Digest；Continuity完成S1/S2 exact比较并以fresh now执行`ValidateCurrent`后，计算基础`not_after = min(Projection Policy current上限, Runtime Evidence source/policy/readability/tombstone-absence sealed projection上限, authoritative_fact Owner Fact sealed projection上限, readers返回的Authority/Scope/Binding上限)`，最后仅当`RequestedNotAfter > 0`时再截短Attempt共同`not_after`。空Tombstone ref不是absence证明；存在Tombstone时可以保留历史审计metadata，但payload readability必须关闭。任何Owner没有提供可证明的上限时不得补默认TTL；Cursor TTL、Event RecordedAt、Retention window与immutable Evidence存在均不能替代current reader。`now >= not_after`、时钟回拨、reader typed-nil/unavailable或S1/S2漂移全部Fail Closed。

Runtime Evidence Owner必须在同一线性化边界内原子推进会改变subject current/readability的source registration/policy、Tombstone、已接纳Retention/readability binding，以及subject-current projection index与tombstone absence watermark。旧Projection Ref和sealed内容仍可exact Inspect作历史审计；但`ValidateCurrent(subject, expected_projection_ref, now)`还必须匹配current index revision/digest与absence watermark，任一mutation推进后旧ref即使未自然过期也返回`precondition_failed`。current index revision单调，禁止回到旧ref/digest形成ABA。

### 3.5 `TimelineOwnerFactCurrentReaderV1`

该Port是Continuity仅为`authoritative_fact`消费的窄、只读、按Fact kind路由的合同形状，不是通用Hook或通用Fact Store。输入为完整Owner Fact exact ref、expected Owner Binding、Execution Scope digest与reader capability/binding ref；禁止输入`RequestedNotAfter`。输出为Owner natural immutable sealed projection：Fact exact ref、Owner Binding、Scope、revision/digest、closed state/currentness、Authority/Policy/Binding水位以及稳定`CheckedUnixNano/ExpiresUnixNano/ProjectionDigest`。

`observation | late_observation | receipt | attestation | claim`禁止调用该generic Reader取得Trust升级；它们只能保持Evidence Ledger原值。只有`authoritative_fact`由Application/Assembler按`owner contract/schema/fact kind`选定Owner发布的Reader。调用方不能注入任意Reader，S1/S2不得切换Reader binding。Reader重复Inspect同一projection必须返回相同Checked/Expires/Digest；fresh now只由consumer调用`ValidateCurrent(now)`，不得促使Owner每读重封。

### 3.6 `TimelineCursorV1`

字段：ledger scope digest、after sequence、query digest、authority/policy watermarks、projection revision、page limit、issued/expires time、cursor digest。

Cursor状态：`active -> expired | invalidated | exhausted`。Authority缩小、query改变、projection schema不兼容或ledger gap均显式Invalidated，不静默跳过。

## 4. Projection写入状态机

### 4.1 `ProjectionAttemptV1`

```text
proposed
  -> inspecting
  -> admitted
  -> visible

proposed | inspecting -> rejected | expired | indeterminate
proposed | inspecting | admitted的持久回包不确定 -> reconcile_required
reconcile_required -> Inspect原Attempt -> admitted | visible | indeterminate
```

规则：

- Create-once与每次状态转移都使用Attempt exact ref/revision CAS；同ID同canonical请求幂等，同ID换内容Conflict。Repository写口不得绕过Governance Controller。
- Controller唯一生产序列固定为：`Create Attempt -> S1 Record双读 -> R-CTY-06 Evidence current/readability/tombstone -> authoritative_fact按需Owner current -> S2 exact复读 -> fresh ValidateCurrent/aggregate TTL -> atomic Event + Attempt visible + Continuity projection current index`。不得跳步、换序或把任一Reader移到原子提交之后。
- **S1 Record双读**：先调用Runtime `EvidenceSourceRecordReaderV2.InspectBySource(SourceKey)`定位Record，再以返回的exact Ref调用`InspectRecord`；两份完整Record必须exact相同。caller optional expected Ref只作期望比较。ledger scope/sequence、Record/candidate/chain digest、Trust、payload schema/ref/revision/digest、Observed/Ingested time、semantic/causal refs与Execution Scope全部从Reader结果派生；随后R-CTY-06密封source/policy/current、readability、exact Tombstone或absence watermark。只有`authoritative_fact`再由对应Owner Reader完成Owner Fact S1。
- **S2 exact**：使用与S1相同的全部Reader bindings重复Record双读、R-CTY-06及按需Owner current，并逐字段exact比较完整Record/Source、candidate/chain/payload/semantic/causal内容、readability/Tombstone/absence watermark、Evidence subject-current index，以及按需Owner Fact/Owner Binding、Authority/Policy/Scope/Binding、Checked/Expires/ProjectionDigest。任一缺失或漂移Fail Closed。
- **fresh**：S2 exact通过后才读取一次fresh now，依次对S2 sealed Evidence/Owner/Policy projections执行`ValidateCurrent(expected Ref, now)`，然后聚合Owner自然TTL并最后应用`RequestedNotAfter`截短。fresh now不重封Owner projection，也不能复用S1之前或上一Attempt的时间。
- **atomic publish**：`TimelineEventRecordV1 + Attempt visible revision + Continuity projection current index`必须在同一Continuity Owner事务中全有或全无；Event canonical内容只能来自S2结果。后端不能证明原子性时保持`admitted/reconcile_required`且查询不可见。查询不得把`proposed | inspecting | admitted | reconcile_required`当已提交事实。
- Evidence/Owner Fact由各自Owner独立Inspect；Continuity不能信任调用方附带的反序列化对象、`AdmittedByLedger`、`InspectedByOwner`、`current`或accepted标志。
- 同source key重放同digest幂等，换内容`conflict`；Projection失败不回滚Evidence，可重建投影失败形成Continuity Residual。
- Create/CAS/commit回包不确定只Inspect原Attempt exact ref；`unavailable | indeterminate`不能降为NotFound、换Attempt ID或盲重试。CAS identity绑定完整previous Fact digest；current已前进时重放旧expected必须Conflict，禁止ABA。

### 4.2 Rebuild治理

Production Rebuild输入只能是有界`TimelineProjectionRequestV1`列表或可复读Request refs；不得接收`TimelineProjectionCandidate`、caller构造的Event或Store record。Rebuild只是逐项调度同一Governance Controller：每项使用自己的stable Attempt/idempotency，完整执行S1/S2/fresh/atomic publish。不得直接调用`PutProjection`、`ReplaceLedgerScope`或任何bulk Store写口，也不得因“重建来源可信”跳过R-CTY-06或Owner current。

每项结果独立Inspect并保留exact Attempt/Event/Residual；某项`unavailable | indeterminate`只Inspect该原Attempt，不能换ID或用批次重试覆盖history。Rebuild不得清空/替换Ledger Scope历史，不得覆盖Tombstone/Visibility overlay，也不得让晚到旧Record把current index倒退。批次完成只是一组Item Result refs，不形成新的Ledger sequence或全局权威Fact。

### 4.3 Tombstone与历史不可变

Production Tombstone必须经Continuity Controller create `TimelineProjectionTombstoneFactV1`并CAS Visibility/current index；禁止Store直接修改`TimelineEventRecordV1.Visibility/TombstoneRef`。Tombstone create、index推进和回包恢复遵循create-once/exact Inspect/no-ABA；Rebuild与Projection publish都必须复读current Tombstone/Visibility index，不能把tombstoned Event恢复为visible。

历史Event exact Inspect在Tombstone前后必须返回相同revision/digest/bytes。Query/Watch的可见性和payload读取权限来自独立overlay projection；删除overlay、改写Event或重建Scope均不得静默复活已Tombstone的Event。

### 4.4 C-01 closed errors与恢复

公开分类固定为：`invalid_argument | not_found | conflict | precondition_failed | unavailable | indeterminate | unsupported`。Runtime Adapter只做该闭集到`core.ErrorCategory`的明确映射，不复制Runtime reason或Owner状态枚举。

| 分类 | 条件 | 恢复 |
|---|---|---|
| `invalid_argument` | schema、canonical、ref/scope形状非法或`RequestedNotAfter < 0` | 修正新请求；不得重用为current证据 |
| `not_found` | 线性化Owner Reader明确证明exact identity不存在 | 终止原Attempt；不得把unknown当absent |
| `conflict` | 同identity换内容、CAS revision、S1/S2或reader binding漂移 | Inspect原Attempt/current后重新决策 |
| `precondition_failed` | expired/revoked/not-current、`now >= not_after`、Authority/Scope/Policy水位失效或时钟回拨 | 重新取得Owner current facts并创建新治理尝试；不得续旧资格 |
| `unavailable` | typed-nil、Reader/backend暂不可用或无所需coverage | 保持原Attempt，稍后Inspect同identity |
| `indeterminate` | 持久回包丢失或无法证明写入结果 | 只Inspect原Attempt/Event，不换ID、不重派 |
| `unsupported` | Reader版本、Fact kind、production backend或装配能力未获准 | Provider/外部调用为零，等待Owner能力闭合 |

## 5. 内容寻址与跨存储Journal

### 5.1 `ObjectManifestV1`

字段：Object ID、schema/version、content digest、total length、ordered Chunk refs、compression envelope、encryption envelope、classification、Owner、Scope、Retention、created time。

Chunk key至少绑定schema version、digest与length。读取后必须重算digest；RocksDB key存在不是内容正确的证明。

### 5.2 `ContinuityWriteJournalV1`

状态：

```text
proposed
  -> metadata_pending
  -> content_staged
  -> reference_committed
  -> visible
  -> closed

异常：unknown_write | orphan_content | dangling_reference | corrupt_content | cleanup_pending
```

CAS不变量：

- `reference_committed`前内容必须可Inspect且digest匹配；
- `visible`前SQLite current ref和RocksDB Object Manifest必须互相引用同digest；
- `orphan_content`可回收前必须证明无Checkpoint/Fork/Review/Legal Hold引用；
- `dangling_reference/corrupt_content`Fail Closed，不返回正文、不宣称Checkpoint可恢复；
- Unknown只Inspect同Journal/Object ID。

### 5.3 `ArtifactRefV1`与不可变因果关系

`ArtifactRefV1`是Artifact Owner密封的历史描述引用，不是Continuity创建的Artifact业务事实。它至少包含Artifact Owner exact Fact ref、opaque storage ref+digest、可选parent revision exact ref、origin Evidence Record ref+digest及source projection digest。Parent必须与当前Artifact同Tenant、同Owner、同contract/schema、同Artifact ID且revision更低；不得从路径、时间或caller布尔值推导这些关系。

`ArtifactRelationFactV1`只由Continuity Owner拥有，表示一个经过精确复读的Artifact与`context_frame | workspace_change | review | tool_result | effect | checkpoint`之一的历史关系。Fact固定revision=1、create-once、immutable，按`TenantID + ExecutionScopeDigest + RelationID`结构化隔离；same-ID/same-idempotency exact重放返回原Fact，换内容Conflict，lost reply只Inspect原Relation ref。

production create请求只能携带stable Relation/Idempotency坐标、期望Execution Scope、Artifact Fact exact ref、Related Fact exact ref、Relation kind和Evidence Record ref；不得携带可信storage digest、parent、source projection digest或预制Fact digest。Controller固定执行：

```text
create-once coordinates
  -> S1 Timeline exact Event + typed Artifact Owner source projection
  -> exact compare Artifact/Related/Evidence/Scope
  -> S2重读同一Event与同一Owner source projection
  -> S1/S2全字段一致
  -> create immutable ArtifactRelationFactV1 + artifact/related indexes
```

Owner source projection只证明该Owner声明的exact历史关系，不授current、Authority或Effect执行权。Continuity不复制Artifact正文、Context Frame、Workspace Change、Review Verdict、Tool Result或Effect业务状态。真实typed Owner Router/Application root未装配时，只允许reference fake/conformance，不宣称production `AttachArtifact`。

### 5.4 `ContentIntegrityAuditFactV1`诊断切面

`ContentIntegrityAuditFactV1`是Continuity Content Owner对**调用方明确列出的Object/Journal坐标**形成的不可变诊断Fact，不是全库完整性证明、孤儿回收证明或删除授权。请求只允许携带stable Audit ID、Idempotency Key、Execution Scope、Object ID、Journal ID及可选expected Manifest digest；caller不能提交`healthy`、Journal state、Chunk存在性、摘要结果或任何Cleanup结论。

Controller对每个Subject执行两轮同构Inspect：读取Object Manifest/visibility与Journal，逐Chunk执行`Has + Get + length/digest`校验，再重读同一组坐标。两轮canonical结果完全一致后，才创建revision-1、create-once、immutable Audit Fact；任一Manifest/Journal/Chunk漂移形成`indeterminate`且不宣称healthy。closed classification为`healthy | write_incomplete | metadata_absent | journal_absent | dangling_reference | corrupt_content | indeterminate`，聚合结论只允许`healthy | attention_required | indeterminate`。

审计Fact按`TenantID + ExecutionScopeDigest + AuditID`结构化隔离，same-ID/same-idempotency exact重放返回原Fact，换Subject或expected digest时Conflict；commit回包丢失只Inspect原Audit ID，不重新扫描、更换ID或猜测结果。`healthy`只覆盖本次请求列出的坐标与读取时点；没有Content keyspace枚举、跨Owner引用闭包和current readers时，不得声称“无孤儿”。本切面不推进Journal、不写Retention/Tombstone、不物理删除Chunk、不调用remote Provider。

### 5.5 `ContentDeltaFactV1`结构共享关系

`ContentDeltaFactV1`是Continuity Content Owner在两个**已经可见且完整可读**的Object之间形成的不可变结构共享关系，不是可执行binary patch、Compaction结果或删除旧Object的资格。caller Request只携stable Delta ID、Idempotency Key、Execution Scope、Base/Target Object ID与各自expected Manifest digest；Chunk集合、reuse/add/remove分类、字节统计及Fact digest必须由Controller读取Owner Manifest/Content派生。

Controller固定执行Base/Target Object Manifest+visibility+全部Chunk bytes的S1→S2同构读取；每轮都重算Chunk length/digest与完整Object content digest。两轮canonical projection全量一致且两个Object同Execution Scope、均visible后，按`schema version + digest + length`精确比较Chunk，生成ordered target recipe和normalized reused/added/removed集合。相同digest但length/schema不同不得复用；缺失、损坏、不可用或S1/S2漂移均Fail Closed且零Delta Fact。

Fact固定revision=1、create-once、immutable，按`TenantID + ExecutionScopeDigest + DeltaID`隔离；same-ID/same-idempotency exact replay，换Base/Target/expected digest Conflict，lost reply只Inspect原Delta。Fact只能证明内容寻址结构共享关系；不创建Target、不改变Object visibility/current、不执行Compactor/Indexer/Consolidator、不授权Retention/Purge。

### 5.6 `HistoryDerivationCandidateFactV1`派生历史候选

`HistoryDerivationCandidateFactV1`只表示“一个已可见Content Object被声明为一组immutable Timeline Event的`projection | summary | index`候选输出”。它不证明内容语义正确，不成为Timeline current、领域Fact、Memory/Knowledge、Review Verdict或Run终态，也不允许替换、压缩、Tombstone或删除任一source Event。

caller Request闭集仅含stable Candidate ID、Idempotency Key、Execution Scope、closed kind、ordered source Event坐标（Evidence Record ref+expected Record/Projection digest）及output Object ID+expected Manifest digest。Controller对每个source Event和output Object/Chunk执行S1/S2 exact读取；source必须属于同Execution Scope，Event历史bytes/digest不变，output必须visible且完整。两轮全量canonical一致后才创建revision-1 immutable Candidate Fact。

Fact按`TenantID + ExecutionScopeDigest + CandidateID`结构化隔离；same-ID/same-idempotency exact replay，changed Conflict，lost reply只Inspect原Candidate。Candidate Repository没有publish-current、bulk replace、Event mutation、Compaction execute或Purge方法；真正的Compactor/Indexer/Consolidator调度、算法证明、资源预算与production root后置。

## 6. Checkpoint对象与状态机

### 6.1 `SnapshotBindingV1`

字段：Participant Binding、Snapshot ID/revision/digest、coverage schema/digest、storage ref、encryption envelope ref、source epoch/sequence、Receipt/Evidence ref、Inspect fact ref、Residual refs。

Participant Report状态候选：`prepared`、`unsupported`、`partial`、`unknown`、`committed`、`aborted`。只有Participant Owner的独立Inspect Fact可支持`prepared/committed`；Provider Receipt不能升级状态。

### 6.2 `CheckpointManifestV1`与Governance V2 Delta

至少包含：

- Checkpoint ID、epoch、Barrier ID/revision；
- exact Execution Scope、Runtime State Ref、Run/Session Ref；
- Plan/Profile/Binding/Context Generation/Tool Surface/Authority digests；
- Evidence/Event watermark；
- Effect Cut：accepted、per-attempt disposition、dispatch/settlement/remote/cleanup水位和稳定Conflict Domain；
- Required/Optional Participant policy与全部Report；
- Snapshot bindings与coverage；
- Pending Review、Remote Continuation、Provider Retention、Secret rotation和不可逆Effect缺口；
- Residual refs、manifest revision/digest、created time。

Manifest Fact与Manifest Seal都是Continuity领域事实，不等于Runtime资格：

```text
collecting
  -> participants_inspecting
  -> verified_candidate
  -> diagnostic_partial | diagnostic_indeterminate | rejected

verified_candidate --Continuity create-once--> immutable revision 1 ManifestSeal
ManifestSeal --Runtime独立复读--> atomic Consistency + Attempt consistent + Barrier closed
diagnostic_* | rejected --Runtime Finalize--> Attempt incomplete|aborted|indeterminate + Barrier closed; NO Consistency
```

Continuity不设置`consistent`或`restore_eligible`。Runtime `CheckpointConsistencyFactV2`是immutable revision 1且只允许`consistent`，只证明该冻结点在形成时一致；`partial | indeterminate | rejected`属于Attempt Finalization并且不生成Consistency。Runtime现独立拥有short-TTL Eligibility；它必须复读Plan与requirements current，且不包含accepted Verdict/Authorization/Permit。

#### 6.2.1 `CheckpointManifestV2` exact-ref闭包

V2不再接受跨Owner裸字符串。每个引用必须使用对应Owner公开、版本化的Fact Ref；wire最低字段是`contract/schema`、`owner binding`、`TenantID`、`id`、`revision`、`digest`、`ScopeDigest`，需要currentness时再携带Owner定义的TTL/水位。内部exact identity必须使用包含完整`OwnerBinding`全部字段的可比较结构键；禁止用可被字段内`|`等字符碰撞的字符串拼接，也禁止只使用`Owner.ComponentID`。Continuity不得复制Runtime/Harness/Context/Memory/Knowledge状态枚举来替代Owner Inspect。

Manifest V2至少冻结：

- Runtime `CheckpointAttempt`、Barrier与immutable Effect Cut exact refs；
- Timeline Cut：`ledger_scope_digest + ledger_sequence + evidence_record_ref/digest`，sequence仍由Evidence Ledger Owner分配；
- Context Owner的exact Generation Ref与Frame Ref，且Generation必须证明包含该Frame；
- Application governed Attempt refs、Runtime Operation Attempt refs与opaque Operation Settlement refs；
- 每个已Begin Attempt必须关联同一Attempt的Settlement；没有Settlement只能记录unknown/inspection/residual，Manifest不得成为verified candidate；
- Memory Owner的Watermark/View/Projection refs与Knowledge Owner的Snapshot/View/Projection refs；
- Required/Optional Participant Fact refs、Snapshot refs、coverage、Evidence与Residual refs；
- frozen-ref-set digest、Manifest revision/digest与创建时间。

Manifest递归校验Context Generation/Frame、Memory、Knowledge、Attempt、Settlement/Inspection、Participant Fact/Snapshot/Coverage/Evidence、Diagnostic及全部Residual exact refs都与Manifest `TenantID + ExecutionScopeDigest`一致。Residual必须从Manifest顶层、所有Attempt（包括已有Settlement）、所有Participant以及所有severity Diagnostic聚合；`verified_candidate`要求聚合结果为空。current与history按结构化`TenantID + ScopeDigest + ManifestID`分区，current Reader还必须携带受验Continuity Owner Binding，跨Tenant同ID独立且禁止混读。

V2禁止内联Context正文、Memory/Knowledge正文、Snapshot blob、Provider native session、Runtime Outcome、Review Verdict语义副本、执行函数或控制句柄。历史exact ref可继续审计，但不能自动证明Restore时仍current。

#### 6.2.2 `CheckpointManifestSealFactV2`

Continuity在Manifest Fact达到`verified_candidate`后，通过`CheckpointManifestGovernancePortV2.CreateCheckpointManifestSealV2`create-once immutable Seal。Seal revision固定为1，并exact绑定：Manifest ID/revision/digest、CheckpointAttempt、Barrier、EffectCut、frozen-ref-set digest、required Participant set digest、Runtime Participant Set digest、每个Runtime typed closure的完整Owner exact ref、canonical Participant closure refs、Context Closure digest及Artifact Closure digest。Context Closure唯一算法为exact Generation+normalized Frame refs；Artifact Closure唯一算法为normalized Memory/Knowledge/Snapshot/Coverage refs。跨Owner SHA-256 spelling只经Runtime公开normalizer转换，raw Owner digest仍保留在exact lookup中。

`InspectCheckpointManifestSealV2`只能返回该immutable Seal的中立projection。Repository在create事务内重新读取current Manifest，重验exact revision、`verified_candidate`、Manifest/Seal Owner、Attempt/Barrier/EffectCut、frozen/required/runtime participant set、Context/Artifact closure digest及Participant closure，不能由直接Repository调用绕过Controller。Runtime只读Adapter按完整Continuity exact coordinate Inspect，要求每个Runtime closure与Seal内`RuntimeClosureRef`一一对应且Participant ID/Owner/digest均exact；Runtime Gateway用同一request在CAS前S1/S2复读。相同Seal ID与相同canonical内容幂等；同ID换Manifest revision/digest、Attempt、Barrier、EffectCut、frozen-ref-set或任一Participant closure均Conflict。Create或Inspect回包丢失只Inspect原Seal ref；历史Seal不可CAS、覆盖、删除后重建或由mutable Manifest candidate冒充。Manifest CAS回包丢失后若current已继续前进，旧expected→next重放必须Conflict，不得误判幂等或形成ABA。Runtime Consistency唯一允许绑定的Continuity对象是该Seal ref。

### 6.3 Runtime CheckpointAttempt（P1 Delta）

Runtime持久状态不包含孤立`proposed`或独立Barrier acquisition：

```text
atomic Attempt+Barrier bundle: barrier_acquired
  -> cut_frozen
  -> collecting
  -> success: consistent + Barrier closed + immutable Consistency
  -> failure: incomplete | aborted | indeterminate + Barrier closed + NO Consistency
```

`CreateCheckpointAttemptV2`由同一Runtime Checkpoint Fact Owner在一个事务中写Attempt+Barrier；不存在独立`AcquireBarrier`或可观察的半对象。Barrier与Begin线性化后，Runtime把所有in-flight Effect分类为not_dispatched、begun、observed、settled、unknown、remote、cleanup_pending。聚合数字不足以证明无洞，必须绑定可Inspect Effect Cut摘要。

### 6.4 第一阶段事实等级与恢复纪律

| 输入/结果 | 等级 | 可支持的结论 |
|---|---|---|
| Provider/Harness/Participant回包、Snapshot Report、Receipt | Observation/Receipt | 只能触发原Attempt Inspect，不能证明Snapshot committed |
| Runtime Evidence Record | Ledger authoritative record | 权威证明Observation被记录；不升级Observation内容 |
| Participant Snapshot/coverage Inspect+CAS Fact | Participant authoritative fact | 仅证明该Participant自身Snapshot与coverage |
| Continuity Manifest CAS Fact | Continuity authoritative fact | 仅证明exact-ref闭包和`verified_candidate/diagnostic_*`分类；不等于Seal |
| Continuity immutable ManifestSeal Fact | Continuity authoritative fact | revision 1 exact冻结Manifest/Attempt/Barrier/EffectCut/Participant closures；不等于Runtime consistent |
| Runtime immutable `CheckpointConsistencyFactV2` | Runtime authoritative fact | 结论只能`consistent`；与Attempt consistent、Barrier closed原子提交；无TTL，不代表Restore current资格 |
| Runtime `RestoreEligibilityFactV2` | Runtime authoritative short-TTL fact | 仅支持Admission前资格读取；不授Authorization、Permit、Begin、Stage或Activate |

Checkpoint happy path固定为：Runtime原子create-once Attempt+Barrier bundle→逐Attempt Effect Cut→Participant Reserve/治理/Inspect/CAS→Continuity Manifest CAS→immutable ManifestSeal create-once→Runtime复读→原子Commit Consistency+CloseBarrier。任一crash或回包丢失只Inspect原bundle、Participant Fact、Manifest或Seal ref；不得生成替代Checkpoint/Seal ID。任一required缺口或无法证明的结果只能走Runtime Finalize Attempt+CloseBarrier；Partial/Unknown永远只作诊断且不生成Consistency。

`CheckpointConsistencyFactV2`最小公开Delta：`contract/schema/version`、Runtime owner binding、fact ID/revision/digest=revision1、exact CheckpointAttempt/Barrier/EffectCut/ManifestSeal refs、Required Participant closure-set digest、frozen-ref-set digest、`consistent`结论和created-at。它create-once且不可修改；回包丢失按fact ID+digest Inspect，不以CAS更新TTL/currentness，也不允许`partial|indeterminate|rejected`实例。

## 7. Fork、Rewind与Restore合同

### 7.1 `ForkPlanV1`

字段：source node/checkpoint、parent lineage/session、new lineage/session intent、desired context generation、required profile/binding/tool/sandbox/review revalidation、Authority ceiling、inherited Effect/Residual refs、plan digest/TTL。

状态：`draft -> admitted -> review_pending? -> approved | rejected | expired`。Approved只表示计划可提交给Application，不创建Instance。

### 7.2 `RewindPlanV2`（首版Workspace-only）

Continuity只拥有计划Fact，不拥有Workspace View、ChangeSet、Review、Runtime Operation或实际文件写入。首版请求只携stable Plan/Idempotency、Tenant/Execution Scope、target `CheckpointConsistencyFactV2`与`CheckpointManifestSealV2` exact refs、source Workspace View exact ref、ordered keep/drop Workspace ChangeSet exact refs、expected current Workspace revision、dependency inspection exact refs、不可逆Effect/Residual exact refs、Review requirement exact refs、stable Conflict Domain和自然TTL。调用方不得携accepted Verdict、Permit、Fence、Provider binding、可信current或文件payload。

Plan Owner复读Checkpoint/Manifest与Sandbox Workspace current/historical Reader，形成immutable dependency closure与planned Workspace ChangeSet coordinate；计划不得复制Workspace正文或自称ChangeSet Owner。只有依赖闭合、无冲突且无未知Workspace outcome时才能进入`admitted`；`approved/submitted`也只表示Application收到exact Plan ref，不授执行资格。

状态：

```text
draft
  -> dependencies_inspected
  -> conflicts_classified
  -> admitted
  -> review_pending（按Policy）
  -> approved | rejected | expired
  -> submitted_to_application
```

Rewind执行不是Continuity状态迁移；首版只由Sandbox Workspace Owner在Runtime Operation治理下执行new ChangeSet Effect。

首版把上句收窄为Sandbox Workspace Owner：Application从exact submitted Plan派生新的`praxis.sandbox/workspace-commit` Intent，随后走Reservation/Admission/Review/Authorization/Permit/Fence/Begin、actual-point Enforcement、Provider Observation、Evidence、Runtime Settlement(ref only)和Sandbox ApplySettlement。Begin或Provider回包丢失后只Inspect原Attempt；不得换Plan、ChangeSet或Operation identity重派。Tool与外部系统补偿仅保留候选，不进入首版执行链。

### 7.3 `RestorePlanV1`

字段：immutable Runtime `CheckpointConsistencyFactV2` Ref、Continuity `CheckpointManifestSealRefV2`、new Instance proposal、required Participant set与Compatibility/Authority/Profile/Binding/Context/Tool/MCP/Secret requirement shapes、Residual policy、Recovery Credential、TTL。Plan requirements不是Runtime current `RestoreEligibilityFact`；只有Runtime create-once Reservation后才能Issue/Bind该Fact。

状态：`draft -> checkpoint_inspected -> compatibility_inspected -> admitted | rejected | expired -> submitted`。`submitted`只表示Application收到exact Plan ref；Restore Review/Authorization必须发生在Runtime Reservation→Eligibility Issue/Bind→Action Admission之后，不是Plan状态。

#### 7.3.1 `RestorePlanV2`到Runtime Reservation/Eligibility

Continuity拥有的`RestorePlanV2`只保留Plan字段：Plan exact ref/digest/TTL、Runtime immutable `CheckpointConsistencyFactV2` ref、`CheckpointManifestSealRefV2`、frozen-ref-set digest、source Instance ref、fresh Instance ID/higher Epoch proposal、Required Participant set digest、Context Generation/Frame requirement refs、compatibility/currentness requirement refs、Review/Authority/Budget/Binding prerequisite refs、stable tenant Conflict Domain、Residual policy与Recovery Credential ref。Plan中的Identity始终是proposal；Runtime create-once成功后才形成authoritative Reservation。

`admitted/submitted`只允许保存和交付exact Plan ref；Runtime必须独立create-once Attempt/Reservation并Issue/Bind Eligibility。当前运行边界固定为：

```text
historical CheckpointConsistencyFactV2 + CheckpointManifestSealRefV2
  -> exact submitted RestorePlanV2 current Reader
  -> Runtime create-once Attempt + Identity Reservation
  -> Runtime short-TTL Eligibility
  -> Application immutable Intent
  -> Action Admission
  -> Review / Authorization bound to exact Attempt + Eligibility
  -> Permit / Fence -> Begin
  -> Runtime Prepare/Execute actual-point Enforcement
  -> Sandbox Stage/Inspect -> Evidence -> Runtime Settlement(ref only) -> Sandbox ApplySettlement
  -> Context materialize new Generation/Frame
  -> Runtime Activate reserved new Instance/high Epoch/new Lease
```

`RestoreGovernancePortV2`只公开create/Inspect Attempt、Issue/Inspect/CAS Eligibility；Eligibility保存requirements而非accepted结论。后续Action route、Review/Dispatch、Restore Stage Evidence/Settlement、Context materialization与Activation使用各Owner的专用公开Port，Continuity不实现这些Port。任一current门、exact binding或Residual检查失败必须Fail Closed；Host-Local参考Stage不授远程Provider或production root资格。

### 7.4 Restore后置恢复纪律

- historical Consistency或ManifestSeal不得被解释为current Restore资格；
- Plan TTL/shape有效不表示Admission、Authorization、Permit或production运行资格已经存在；
- legacy `RestoreRequest`、`RestoreCheckpoint`、Foundation协调器、`activation_attempt`或checkpoint phase Evidence/Settlement不得升级为Restore；
- Restore必须创建fresh Instance/higher Epoch/new Lease且不得宣称外部世界回滚；当前参考纵切已冻结TTL/currentness、CAS/lost-reply和调用顺序，production trusted Assembler与跨Owner全量Participant仍需独立验收；
- 旧unknown继续占稳定tenant Conflict Domain，不能因Plan、Fork或换ID而释放。

## 8. Recovery Credential

`RecoveryCredentialV1`是短期、最小Scope、可撤销的引用集合，不包含Secret正文。字段：credential ID/revision/digest、checkpoint/manifest/restore plan digest、subject scope、Authority/Policy/Review refs、allowed stage/inspect actions、Participant set digest、issued/expires time、revocation ref。

Credential只约束Restore Plan输入，不单独授current资格或Dispatch。`RestoreGovernancePortV2`最小运行面只到Attempt/Identity Reservation与Eligibility；Credential过期或漂移时Plan/Eligibility current复读失败，不自动续签。未来实际Dispatch仍须后续独立Review冻结Admission、Authorization、Permit、Fence与执行点重验。

## 9. Retention状态机

对象可见性状态：

```text
active
  -> retention_expired
  -> tombstoned
  -> purge_planned
  -> purge_reviewed（按Policy）
  -> purge_dispatched
  -> purged | purge_unknown | purge_failed
```

旁路状态：`legal_hold`阻止普通expiry/purge；`privacy_erasure_required`触发独立受治理计划。Physical Purge是Effect；回包丢失只能Inspect provider/object ref。链式Evidence Record本身不原地改写，按Runtime Evidence V2 Tombstone语义处理。

## 10. 外部动作完整顺序

所有Continuity相关外部、破坏性或可能计费动作必须保持：

```text
领域Intent / Reservation
  -> Admission
  -> Review / Authorization
  -> Permit
  -> Begin
  -> Delegation / Prepare
  -> Enforcement持久化
  -> Execute 或 Inspect
  -> Observation / Evidence
  -> Continuity DomainResultFact
  -> Runtime Operation Settlement
  -> Continuity ApplySettlement
```

每个领域Owner必须独立Inspect精确Attempt/Evidence并CAS自身`DomainResultFact`。Checkpoint phase Settlement V5与Restore Stage专用Settlement由Runtime Settlement Owner治理并返回唯一opaque ref，领域Owner再以精确Settlement Ref执行ApplySettlement；Begin后丢回包只能Inspect原Attempt。Continuity不得重造私有Permit链或解释其他Owner的Settlement语义。

## 11. Effect、Conflict Domain与Unknown矩阵

下表列出Continuity V1全部计划内Effect。`tenant stable scope`使用Runtime live `EffectStableScopeTenantV2`；更窄作用域只有未来Runtime Policy明确支持后才能采用。

| Effect kind | Effect/Settlement Owner | Conflict Domain | Review/Budget | Unknown恢复 |
|---|---|---|---|---|
| `continuity/remote-content-put` | Continuity | `continuity/content-object` + tenant stable scope | 按披露/成本Policy；Budget必须绑定或显式not-required事实 | Inspect原provider operation/object digest；禁止换Object ID重传 |
| `continuity/remote-content-delete` | Continuity | `continuity/content-object` + tenant stable scope | Retention/Legal Hold/Privacy Policy；通常需Review | Inspect原delete attempt；未知时保留引用与Residual |
| `continuity/remote-archive` | Continuity | `continuity/archive` + tenant stable scope | 披露、区域、Retention与成本Policy | Inspect原archive attempt；不得假设归档成功 |
| `continuity/retention-purge` | Continuity | `continuity/retention` + tenant stable scope | Policy决定Review；Budget绑定 | Inspect原purge attempt；unknown阻止Cleanup闭合 |
| `praxis.runtime/restore-workspace-stage` | Sandbox Participant；Continuity只关联 | tenant stable Restore Attempt conflict domain | exact Eligibility/Admission/Review/Authority/Budget/Scope/Permit/Fence与实际执行点双验 | Inspect原Enforcement/Workspace attempt；Unknown不换ID、不重做Effect |
| Runtime Restore Activation | Runtime Restore/Instance Owner | exact Restore Attempt + reserved target identity | 仅在Stage Settlement、ApplySettlement与Context current全部复读后 | 按stable Restore Attempt Inspect；不创建第二Instance/Epoch/Lease |
| `continuity/rewind-apply` | Sandbox/Tool Owner；Continuity只保存计划 | `continuity/rewind` + tenant stable scope | ChangeSet Review/Authority/Budget | Owner Inspect原ChangeSet Effect；不可逆Effect只继承 |
| `continuity/inspect` | 被Inspect领域的Settlement Owner | 与原Effect相同 | 独立Inspect Policy/Budget；relation绑定原Effect | Inspect Effect自身Unknown按Runtime非递归规则收口 |

本地SQLite/RocksDB Projection/Fact CAS不通过第二套Operation链：Evidence append使用Evidence Governance专用链；Continuity领域Fact由自身Admission+Fact CAS负责。只有外部、破坏性、披露、计费或Provider持久动作进入上表。

## 12. RunSettlementRequirement

Continuity只能声明自身可提供的RunSettlementRequirement，由Trusted Run Assembler复读current BindingSet和声明后决定最终Run Settlement Plan；组件不能自行创建/删减Requirement或签发Certification。Restore等单次外部动作使用OperationScope；启动前置条件若存在则另行声明RunStartRequirement，三者不得混写。

| Requirement ID候选 | Phase | Owner Fact | 成功条件 | Unknown/失败 |
|---|---|---|---|---|
| `continuity/timeline-durability` | completion | Continuity | Run所需Evidence watermark已被精确Inspect；全部required Projection Journal已visible/closed或由Policy明确not-required | unknown阻止CompleteRun；只InspectJournal/Evidence ref |
| `continuity/checkpoint-attempts-classified` | completion | Continuity | Run内Runtime Checkpoint attempts全部为consistent/incomplete/aborted/indeterminate，且关联Manifest诊断与Seal/Finalization refs可Inspect | 未分类或unknown按Policy阻止/形成显式Participant disposition；partial/rejected只属于Manifest诊断，不冒充Runtime Attempt terminal |
| `continuity/content-residuals` | termination_report | Continuity | dangling/orphan/corrupt/remote retention均已closed、policy-not-required或精确Residual化 | unknown进入Termination residual，不能谎报Cleanup complete |

这些是组件声明候选；是否Required、允许`operation_not_required`及Policy由管理线和Trusted Assembler决定。

## 13. pre-run Evidence裁决

Timeline Append、Checkpoint第一波和Run Settlement不触发Restore pre-run Evidence。Checkpoint phase Evidence V1绑定current Run/Attempt/Barrier/EffectCut，不属于pre-run Restore。

Restore Stage使用专用typed Operation scope与`RestoreStageEvidenceGovernancePortV1`；Evidence只在Sandbox DomainResultFact形成并可current Inspect后发布，丢回包按原Publish request Inspect。Eligibility、Admission、Authorization、Permit与Begin的pre-execute current门仍独立存在；`activation_attempt`和transport kind都不能授Restore资格，也不得私建第二Evidence链。

## 14. Model Invoker与Context边界

- Continuity不调用Model Invoker Provider或厂商SDK。
- 若需要关联模型事实，只接收通过`RouteID + routegateway + 公开execution union`形成的Owner Fact/Evidence refs；禁止依赖`model-invoker/internal`、Raw/Native event或Provider SDK类型。
- Tool Call只是一项Action Candidate；stream/completed/cache usage/Provider状态都是Observation，不能直接成为Tool Result、Review Verdict、Cache命中、领域Fact或Run终态。它们只有先进入Runtime Evidence Ledger，才可被Timeline按原Observation TrustClass忠实投影。
- `ContextReference`并非所有Route都已完成物化。Checkpoint/Restore遇到不可物化Context Reference时必须Fail Closed或写入显式Residual；不能声明普遍可恢复。

## 15. 稳定错误语义候选

| Reason | 含义 |
|---|---|
| `continuity/evidence_not_inspectable` | 无法复读精确Evidence Record |
| `continuity/timeline_projection_conflict` | 同一Evidence ref投影内容冲突 |
| `continuity/cursor_invalidated` | Query/Authority/Projection水位漂移 |
| `continuity/content_digest_mismatch` | Content/Chunk摘要错误 |
| `continuity/cross_store_indeterminate` | SQLite/RocksDB关系无法判定 |
| `continuity/checkpoint_partial` | 仅诊断，不可自动恢复 |
| `continuity/checkpoint_indeterminate` | Participant/Effect Cut存在Unknown |
| `continuity/restore_incompatible` | Profile/Binding/Authority/Context等漂移 |
| `continuity/pre_run_evidence_unavailable` | Restore Stage所需公共Delta未闭合 |
| `continuity/retention_blocked` | Legal Hold/引用闭包阻止删除 |
| `continuity/effect_unknown` | Begin后结果未知，只能Inspect |
