# SnapshotArtifactOwnerV2 实现级合同候选

状态：`workspace-capture-and-restore implemented / terminal-purge external-NO-GO`。

2026-07-18 用户确认首个实现范围：`Workspace Snapshot + Host Local`。当前已完成
Snapshot Artifact Owner的`reserved -> available`、Host Local加密Content Store、
Checkpoint Participant exact refs、Workspace bundle与fresh Instance Restore。未解锁的是
Purge/Delete terminal、进程/Secret/网络会话/设备状态恢复与production SLA。

本切片复用live Runtime `CheckpointAttempt/Barrier/EffectCut/Participant V2`、Application
`CheckpointExternalExactRefV1`和Continuity Manifest/Seal；Checkpoint capture不新增或扩权
Runtime V3/V4/V5合同。后续Restore所需additive Runtime Delta必须在Checkpoint闭合后独立落地。

本文件把已经确认的 Owner 方向收敛为实现合同。Workspace capture与Restore已落live；
下文Deletion/Purge候选仍不是实现授权，也不得据此扩展公共能力。

## 1. 已冻结事实与当前缺口

已冻结：

- `SnapshotArtifactOwnerV2`位于Sandbox控制域，但与Sandbox Controller、Provider、
  Enforcer分离；只有它可以提交Artifact领域事实；
- Provider只给Observation/Receipt，Runtime只拥有Checkpoint治理与Operation Settlement，
  Continuity只消费`SnapshotArtifactFactRefV2`形成Manifest/RestorePlan；
- 不暴露backend handle，不选择存储产品，不调用Provider，不写Runtime/Continuity；
- Reservation、Artifact Fact、current/historical Inspect、retention/deletion必须分层，
  retention期限不能延长Checkpoint/Restore执行资格。

以下跨Owner合同已经形成字段级`review_pending`候选，但尚未由对应Owner落地，继续阻止实现：

1. Retention/Legal Hold治理域的Index/Carry/NoActive proof与current/exact Reader；
2. Runtime Snapshot purge专用Evidence Issue/Record/Consume、Settlement V5 additive sibling Reader与Gateway；
3. Management对CurrentIndex/Tombstone最终DTO命名、持久化和续期策略；
4. Runtime/Continuity消费`SnapshotArtifactFactRefV2`的中立公共Reader/Ref形状。

## 2. Owner候选闭表

| 语义 | 唯一Owner候选 | Sandbox切片权限 | 禁止 |
|---|---|---|---|
| Retention Policy、Legal Hold、法规保留决策 | Retention/Legal Hold治理域唯一Owner（公共名称待联合确认；不是Continuity Manifest/RestorePlan Owner的延伸） | 只读exact current ref、revision/digest/TTL | Sandbox或Continuity发布、改写Policy/Legal Hold或伪造negative proof |
| Artifact Reservation、不可变Artifact Fact、Artifact aggregate history/current | Sandbox控制域`SnapshotArtifactOwnerV2` | create-once、Inspect、expected-revision CAS | Provider/Controller提交Fact |
| Artifact-local retention application Fact | `SnapshotArtifactOwnerV2` | 记录外部Retention Policy exact ref及其对该Artifact的有效应用 | 冒充Retention Policy或用retention延长执行资格 |
| Deletion Reservation/Result/Residual | `SnapshotArtifactOwnerV2` | 只提交已经过治理链、Evidence与Owner Inspect确认的领域结果 | 把NotFound、进程死亡、Abort或Release当删除完成 |
| Checkpoint资格、consistent、restore eligibility、Operation/Settlement | Runtime | Sandbox只保存opaque exact ref或读current | Sandbox推进Runtime事实 |
| Manifest/RestorePlan | Continuity | Sandbox只提供`SnapshotArtifactFactRefV2` | Sandbox写Manifest/Plan或暴露StorageArtifactRef/backend handle |

## 3. DTO候选

所有对象使用`ValidateShape`与`ValidateCurrent(now)`分离。canonical编码固定type URL、schema
version、字段顺序、presence discriminator、集合排序、TTL、watermark和exact refs；每一层只
对自己的body seal，并明确排除自身Ref/Digest字段。typed-nil、nil/empty混淆、未排序或重复
集合、unknown required variant、自引用Ref均拒绝。

### 3.1 `SnapshotArtifactReservationV2`

必填候选字段：

```text
Meta(ID, Revision=1, Digest, Created, Updated, Expires)
TenantID, DataDomain
SourceOperationID, SourceEffectID, SourceAttemptRef(ID, Revision, Digest)
SchemaRef, ExpectedContentDigest
RetentionPolicyRef, EncryptionPolicyRef, ResidencyPolicyRef
ExpectedAggregateRef presence（初始Reserve必须显式absent）
RequestedNotAfter
```

create-once stable key候选为：

```text
TenantID + DataDomain + SourceAttemptRef.ID
```

`SourceAttemptRef`的revision/digest、Operation/Effect、Schema、ExpectedContentDigest或Policy
任一不同都不是新key，而是同key内容冲突。Reservation ID由Owner从stable key确定性派生；
caller不能换ID、TTL或内容创建第二份Reservation。lost reply只能按exact Reservation Ref或
stable key Inspect。

### 3.2 四层canonical与无环创建顺序

唯一允许的依赖方向是：

```text
SnapshotArtifactSubjectIdentityV2（stable、无Revision/TTL）
  -> SnapshotArtifactSubjectRefV2（versioned exact/current、带Revision/TTL）
  -> PayloadBodyDigest（排除payload自身Ref/Digest）
  -> FactRef
  -> EntryBodyDigest（排除EntryRef.Digest）
  -> EntryRef
  -> EnvelopeBodyDigest（排除Envelope自身Digest）
  -> AggregateEnvelopeRef
  -> AggregateCurrentIndex
```

1. `SnapshotArtifactSubjectIdentityV2`由Owner从Reservation stable key确定性派生，只表达永久稳定
   坐标，完整字段为：

   ```text
   TypeURL="praxis.sandbox/snapshot-artifact-subject-identity/v2", Version=2
   Owner=SnapshotArtifactOwnerV2(core.OwnerRef)
   Kind="praxis.sandbox/snapshot-artifact-subject"
   ArtifactAggregateID, TenantID, DataDomain
   ReservationID, SourceAttemptID
   DigestAlgorithm="sha256"
   DigestDomain="praxis.sandbox/snapshot-artifact-subject-identity/body/v2"
   StableSubjectDigest
   ```

   stable identity body覆盖TypeURL/Version、Owner/Kind、ArtifactAggregateID、TenantID、DataDomain、
   ReservationID与SourceAttemptID，排除`StableSubjectDigest`和任何revision、schema、TTL、current、
   Fact/Entry/Envelope/Request/Attempt字段。该对象没有`Revision`、`ExpiresUnixNano`、
   `RequestedNotAfter`或`ValidateCurrent`语义，只能作坐标；它不能进入任何执行资格TTL闭集。

   `SnapshotArtifactSubjectRefV2`是Owner基于上述stable identity发布的**versioned exact/current ref**，
   完整字段为：

   ```text
   TypeURL="praxis.sandbox/snapshot-artifact-subject-ref/v2", Version=2
   Owner=SnapshotArtifactOwnerV2(core.OwnerRef)
   Kind="praxis.sandbox/snapshot-artifact-subject"
   ArtifactAggregateID, Revision, TenantID, DataDomain
   ReservationID, SourceAttemptID
   Schema(SchemaRefV2="praxis.sandbox/snapshot-artifact-subject-ref/v2")
   DigestAlgorithm="sha256"
   DigestDomain="praxis.sandbox/snapshot-artifact-subject-ref/body/v2"
   StableSubjectDigest, SubjectDigest, ExpiresUnixNano
   ```

   `StableSubjectDigest`必须等于对同字段stable identity的重算值；`Revision/ExpiresUnixNano`只属于
   exact/current版本，不改变stable identity。`SubjectDigest=H(DigestDomain+canonical(exact subject
   body))`，body覆盖TypeURL/Version、Owner/Kind、全部stable字段、StableSubjectDigest、Revision、Schema
   与Expires，排除SubjectDigest、任何Fact/Entry/Envelope/PurgeRequest/DeletionAttempt ref或后继digest。
   current读取必须复读Owner current pointer并返回同一exact ref；stable identity相同但revision、schema
   或TTL不同必须产生不同SubjectDigest。Owner固定为Sandbox控制域独立`SnapshotArtifactOwnerV2`，
   caller、Adapter、Provider不得填充、续期或重封。
2. 每个Reservation/Artifact/Retention/Deletion payload都有Owner派生的stable `FactID`。
   `PayloadBodyDigest=H(type+version+canonical(payload body))`；body包含业务字段、presence与TTL，
   但排除payload自身FactRef及Digest。`FactRef=(FactID, Revision, PayloadBodyDigest,
   ExpiresUnixNano)`只能在body seal后构造。
3. `EntryBodyDigest=H(type+version+canonical(ArtifactSubjectRef, EntryKind, FactRef,
   PreviousEntryRef presence, RequestedNotAfter, EntryExpiresUnixNano))`；计算时排除
   `EntryRef.Digest`，再构造`EntryRef=(EntryID, Revision, EntryBodyDigest, ExpiresUnixNano)`。
4. `EnvelopeBodyDigest=H(type+version+canonical(AggregateID, Revision,
   PreviousAggregateRef presence, AppliedEntryRef, state summary, terminal presence,
   RequestedNotAfter, EnvelopeExpiresUnixNano))`；计算时排除Envelope自身Digest，再构造
   `AggregateEnvelopeRef=(AggregateID, Revision, EnvelopeBodyDigest, ExpiresUnixNano)`。

固定创建顺序为stable SubjectIdentity→versioned SubjectRef→Reservation Fact→Reservation Entry→Envelope rev1→Artifact Fact→Artifact Entry→
Envelope rev2→Retention Fact→Retention Entry→Envelope rev3。payload禁止引用自身Entry/Envelope、
任何后继payload或后继Ref；Artifact Fact不得反向含RetentionApplicationRef。相同body但不同TTL
必须产生不同digest，篡改任何TTL后复用旧digest必须拒绝。

### 3.3 `SnapshotArtifactFactV2`

必填候选字段：ReservationFactRef、ArtifactSubjectRefV2（versioned exact）、SnapshotStorageArtifactRefV2、Tenant/DataDomain、SchemaRef、
ContentDigest、Length、EncryptionFactRef、ResidencyFactRef、ProviderObservationRef、
ProviderReceiptRef、FormalEvidenceRefs、OwnerInspectionRef、SourceAttemptRef、Created、
RequestedNotAfter、FactExpiresUnixNano、State=`available`。明确禁止`RetentionApplicationRef`字段。

`SnapshotStorageArtifactRefV2`是完整、自包含的exact DTO：

```text
TypeURL = "praxis.sandbox/snapshot-storage-artifact-ref/v2"
Version = 2
StorageArtifactID, Revision
DigestAlgorithm
DigestDomain = "praxis.sandbox/snapshot-storage-artifact-ref/body/v2"
Digest
TenantID, DataDomain
StorageNamespaceExactRef(ID,Revision,Digest,ExpiresUnixNano)
ContentDigest, SchemaRef, Length
EncryptionFactRef, ResidencyFactRef
CreatedUnixNano, ExpiresUnixNano
```

`StorageArtifactBodyDigest=H(DigestDomain+canonical(body))`；body包含TypeURL/Version、stable ID、
Revision、DigestAlgorithm、tenant/domain、namespace exact ref及其TTL、content/schema/length、
encryption/residency refs、Created/Expires，排除自身`Digest`和任何包装自身的StorageArtifactRef。
Revision、TypeURL、Version、DigestDomain、namespace/content/schema/length或TTL任一变化都必须重算
Digest；同ID/revision不同Digest为Conflict。禁止raw path、bucket/key、VM/process/mount handle、
Provider Receipt或Credential字段。

`SnapshotArtifactFactRefV2`则固定使用
`TypeURL="praxis.sandbox/snapshot-artifact-fact-ref/v2"`、`Version=2`、
`DigestDomain="praxis.sandbox/snapshot-artifact-fact/body/v2"`，exact字段为
`FactID,Revision,DigestAlgorithm,Digest,ExpiresUnixNano`。它引用sealed Artifact Fact payload；
Storage DTO与FactRef的TypeURL、Revision序列、DigestDomain和Owner用途严格不同，禁止互填或
用相同digest跨domain重放。

Provider Observation/Receipt只是provenance。只有Owner复读Reservation current、source
Attempt exact、Evidence consumed current与Inspection current后，才能通过内部committer提交
Fact；该committer只能是Owner实现包内不可导出对象。第一切片固定只允许Reservation/Inspect，
禁止caller-mint Fact。

### 3.4 `SnapshotArtifactRetentionApplicationFactV2`

候选字段：ArtifactSubjectRefV2（versioned exact）、ArtifactFactRef、RetentionPolicyCurrentRef、
RetentionOwnerCurrentProjectionRef、EffectiveUntil、Decision=`retain|expiry_reached|hold_blocked`、
ExpectedAggregateRef、Evidence/Inspect refs、RequestedNotAfter、Meta。它只证明外部Policy对该Artifact的应用，
不拥有Policy或Legal Hold。

可选`LegalHoldRef=nil`、空集合或caller布尔值都不能证明“无hold”。Positive hold可由Retention
Owner current projection表达；删除所需的negative current proof另见3.5。Retention延长可晚于
Runtime Lease，但不得改变任何执行资格TTL；缩短或到期也不自动证明物理删除。

### 3.5 `SnapshotArtifactDeletionFactV2`

候选字段：ArtifactSubjectRefV2（versioned exact）、ArtifactFactRef、DeletionReservationFactRef、OperationID、EffectID、
AttemptRef、Authority/Review/Budget/Scope/Permit exact refs、Provider Observation/Receipt、
FormalEvidenceRefs、OwnerInspectionRef、RuntimeSettlementRef、ApplySettlementRef、Coverage、
ResidualRefs、`NoActiveLegalHoldS1ProjectionRef`、`NoActiveLegalHoldS2ProjectionRef`、
RequestedNotAfter、AttemptState、Meta。

`NoActiveLegalHoldCurrentProjectionV1`由Retention/Legal Hold唯一Owner seal，至少绑定：

```text
ProofSeriesID, TenantID, LegalHoldSubjectCoordinateV1
  (SubjectID=ArtifactAggregateID, SubjectDigest=StableSubjectDigest)
RetentionPolicyCurrentRef
HoldIndexExactRef(TypeURL, Version, HoldIndexID, Generation, Revision,
                  DigestDomain, Digest, ExpiresUnixNano)
Coverage(DataDomain, SubjectSelectorDigest, Jurisdictions[], HoldKinds[], CoverageDigest)
QueryWatermark(IndexGeneration, Sequence)
CoverageCarryProofRefs presence + ExactRef(TypeURL,Version,ID,Revision,DigestDomain,Digest,
                                            ExpiresUnixNano)[]
EnumeratedHoldSetDigest, EnumeratedHoldCount
NoActive=true, CheckedUnixNano, ExpiresUnixNano
```

该Projection是Retention/Legal Hold Owner按自然current窗口seal的稳定事实。Reader请求只含
ProofSeries/tenant/subject/coverage/index的exact坐标与expected current ref，**不得**包含caller
`RequestedNotAfter`、deadline或Sandbox TTL。caller bound不进入Projection body/digest，也不得
触发Retention Owner重封；不同Deletion caller bound读取同一Owner current时必须得到相同exact
Projection Ref。Owner只返回其自然seal的Checked/Expires/coverage/watermark。

`EnumeratedHoldSetDigest+Count`证明本次查询结果集合，`HoldIndexExactRef+Coverage+
QueryWatermark`证明查询针对哪个穷尽索引边界。Projection必须满足
`QueryWatermark.IndexGeneration == HoldIndexExactRef.Generation`；HoldIndex的Revision与Digest是
exact绑定，same ID/revision但digest不同、或watermark generation与index generation不同均拒绝。
只返回空集合、nil ref、NotFound或caller布尔值
均不是negative proof。Jurisdictions与HoldKinds必须canonical排序、去重且覆盖Deletion command
要求的全部jurisdiction/hold-kind，Coverage不得使用`all=true`之类未seal捷径。

S1在Owner command形成候选前复读，S2在CAS前重新复读。两份projection各自必须exact ref有效，
且`TenantID+ArtifactSubjectRef+CoverageDigest+HoldIndexID+ProofSeriesID`完全相同；S2仍须
`NoActive=true`。令`W=(QueryWatermark.IndexGeneration, QueryWatermark.Sequence)`，必须满足
`W2 >=lex W1`：`g2 > g1 || (g2 == g1 && seq2 >= seq1)`。仅当`g2 == g1`时，S2
HoldIndexExactRef.Revision不得低于S1；同ID+Generation+Revision时Digest必须exact相同。跨代
Revision不作数值比较，只由下述carry链证明连续性。不得要求S1/S2 Projection revision或digest相等，
因为合法S2应允许单调前进。

若`g2 > g1`，S2的`CoverageCarryProofRefs`必须explicit present并形成从S1
HoldIndexExactRef/Watermark到S2 HoldIndexExactRef/Watermark的连续generation链；每个carry exact
ref绑定`FromIndexExactRef、ToIndexExactRef、FromWatermark、ToWatermark、CoverageDigest、
Jurisdictions、HoldKinds、SubjectSelectorDigest、TypeURL/Version/ID/Revision/DigestDomain/Digest/
Expires`，且相邻端点完全衔接。具体external TypeURL/Version值仍由Retention Owner公共合同冻结；
Sandbox只要求它们非空、受digest覆盖且与expected exact ref一致，不私建替代类型。
同generation时carry presence必须explicit absent。缺代、端点错配、CoverageDigest或jurisdiction/
hold-kind/selector漂移、carry过期均fail closed。任一generation回退、watermark
回退、coverage/jurisdiction/hold-kind漂移、Unknown、Unavailable、NotFound、expired或S2出现active
hold均fail closed，Deletion不得进入`confirmed`。

Projection canonical body必须覆盖`CoverageCarryProofRefs`的presence、按From/To generation升序的
exact ref数组及每个ref TTL；未排序、重复边、非连续边、present空数组或absent非零值均拒绝。

Sandbox把Deletion command的`RequestedNotAfter`只应用于自己的Attempt/Entry/Envelope/
AggregateCurrent：`DeletionAttemptExpires <= min(Command.RequestedNotAfter, S2.ExpiresUnixNano,
其他Sandbox治理current最短TTL)`。它不能写回、裁剪或派生新的no-hold Projection。

Deletion Attempt state候选：`reserved -> unknown | failed | confirmed | indeterminate`；`unknown`
只Inspect原Attempt，允许同一Attempt的新revision收敛到`failed|confirmed|indeterminate`。
`failed`只终结该Attempt；aggregate经Owner提交关闭该active Attempt的Envelope后回到`available`，
才可在fresh Admission/Review/Permit、fresh S1/S2下用全新Operation/Effect/Attempt stable key重试。
`confirmed`使aggregate进入`deleted`，`indeterminate`使aggregate进入同名终态；两者均发布
不可回退terminal tombstone/current index，永久禁止新Deletion Attempt或状态复活。

### 3.6 `SnapshotArtifactAggregateCurrentProjectionV2`

首个Reserve/Inspect切片只提供Owner Store aggregate current，不提供Checkpoint/Restore执行
资格。候选字段：AggregateCurrentIndexRef、HeadAggregateEnvelopeRef、ArtifactSubjectRef、
ReservationFactRef、ArtifactFactRef presence、RetentionApplicationFactRef presence、
ActiveDeletionAttemptFactRef presence、TerminalTombstoneRef presence、AggregateState、
ActiveTTLClosureDigest、ProjectionDigest、OwnerComputedCurrent、CheckedUnixNano、ExpiresUnixNano。

#### 3.6.1 `SnapshotArtifactAggregateCurrentIndexV2` exact DTO

```text
TypeURL = "praxis.sandbox/snapshot-artifact-current-index/v2"
Version = 2
CurrentIndexID, Revision
DigestDomain = "praxis.sandbox/snapshot-artifact-current-index/body/v2"
Digest
ArtifactAggregateID, ArtifactSubjectRef, HeadAggregateEnvelopeRef
AggregateState
ReservationCurrentRefPresence + ExactRef(ID,Revision,Digest,Expires) if present
ArtifactFactRefPresence + ExactRef(ID,Revision,Digest,Expires) if present
RetentionApplicationCurrentRefPresence + ExactRef(ID,Revision,Digest,Expires) if present
ActiveDeletionAttemptFactRefPresence + ExactRef(ID,Revision,Digest,Expires) if present
TerminalTombstoneRefPresence + ExactRef(ID,Revision,Digest,Expires) if present
ActiveTTLClosureDigest
OwnerClockWatermark, CheckedUnixNano, RequestedNotAfter, ExpiresUnixNano
```

`CurrentIndexBodyDigest=H(DigestDomain+canonical(body))`；body包含TypeURL/Version、stable ID、
Revision、全部presence discriminator、present exact ref的全部字段与TTL、AggregateState、closure
digest和时间字段，但排除`Digest`以及任何包装自身的`CurrentIndexRef`。absent presence要求对应
value为零；typed-nil、presence/value矛盾、遗漏TTL或修改任一TTL后复用旧Digest均拒绝。
`CurrentIndexExactRef=(TypeURL,Version,CurrentIndexID,Revision,DigestDomain,Digest,
ExpiresUnixNano)`只能在body seal后构造。

状态presence闭表：`reserved`要求Reservation present且Artifact/activeAttempt/tombstone absent；
`available`要求Artifact present、activeAttempt/tombstone absent；`deletion_in_progress`要求Artifact
和唯一activeAttempt present、tombstone absent；`deleted|indeterminate`要求TerminalTombstone
present且activeAttempt absent，tombstone state必须与AggregateState exact相等。

#### 3.6.2 `SnapshotArtifactTerminalTombstoneV2` exact DTO

```text
TypeURL = "praxis.sandbox/snapshot-artifact-terminal-tombstone/v2"
Version = 2
TombstoneID, Revision
DigestDomain = "praxis.sandbox/snapshot-artifact-terminal-tombstone/body/v2"
Digest
ArtifactAggregateID, ArtifactSubjectRef
TerminalState = deleted | indeterminate
PreTerminalAggregateRef
CauseDeletionAttemptFactRefPresence + ExactRef(ID,Revision,Digest,Expires)
RuntimeSettlementRefPresence + ExactRef(ID,Revision,Digest,Expires)
ResidualSetRefPresence + ExactRef(ID,Revision,Digest,Expires)
PreviousTombstoneRefPresence + ExactRef(ID,Revision,Digest,Expires)
OwnerClockWatermark, CreatedUnixNano, CheckedUnixNano, ExpiresUnixNano
```

Tombstone body包含全部presence discriminator、present exact ref及其TTL；排除`Digest`和任何包装
自身的`TerminalTombstoneRef`。初次terminal提交的PreviousTombstoneRef explicit absent；续期或
审计revision explicit present且必须指向前一Tombstone exact ref、保持同一TerminalState/
ArtifactAggregateID/Subject与Cause lineage。Tombstone只引用`PreTerminalAggregateRef`，不得引用
包含自身的terminal Envelope/CurrentIndex，避免回环。它的current读取TTL到期不删除terminal
语义；新revision只能同state续期，不能清除tombstone或切换`deleted|indeterminate`。

`AggregateCurrentIndexV2`是Owner独立、append-only可续期的current索引。它不是
Envelope别名；Reservation、旧Artifact/Retention/Deletion Fact及旧Entry/Envelope都属于history，
其retention expiry只影响`ValidateCurrent`资格或完整载荷可用性，不删除Ref/Digest审计记录，也不
把aggregate退回absent/reserved。

投影TTL按`AggregateState`选择封闭的active集合，不再无条件取全部历史对象：

| AggregateState | Active TTL闭集 | 明确排除 |
|---|---|---|
| `reserved` | CurrentIndex + active Reservation current | 旧Entry/Envelope、尚不存在的Artifact Fact |
| `available` | CurrentIndex；若存在active retention decision则再含该current projection | Reservation、历史Artifact/Retention/failed Attempt Fact |
| `deletion_in_progress` | CurrentIndex + 唯一active Deletion Attempt current + 该Attempt所需治理current refs | 旧failed/closed Attempt及旧negative proof |
| `deleted` | terminal CurrentIndex + `deleted` Tombstone current | Reservation、Artifact/Retention/Deletion历史Fact |
| `indeterminate` | terminal CurrentIndex + `indeterminate` Tombstone current | 全部历史Fact；不得因其过期回退或重试 |

`ExpiresUnixNano=min(RequestedNotAfter, OwnerMaxProjectionTTL, active闭集中每个current ref的
ExpiresUnixNano)`。空闭集、缺required presence、成员Unknown/Unavailable或`now >= expires`均
返回零projection。历史Reservation/Fact retention不得永久杀死current；terminal tombstone/index
即使其current读取TTL到期也只变成“当前不可证明”，不能被当作absent或允许复活。Owner续期必须
保持同一terminal state与Head lineage，只能追加更高index revision，不能覆盖或回退。

该投影不得声称Participant、Lease/Fence、Scope、Credential或任何执行资格已current，也不得传入
Checkpoint/Restore Admission。未来若需要`SnapshotArtifactExecutionCurrentProjectionV2`，必须
新增完整exact输入与Owner readers：Participant、Runtime Lease/Instance/Fence、Operation/
Attempt、Authority/Review/Budget/Scope/Permit、Credential/Encryption、Retention Policy、
NoActiveLegalHold、Evidence/Inspection及全部TTL，并在S1/S2分别复读。该未来投影不属于
首个Reserve/Inspect切片。

## 4. 统一aggregate entry/envelope与CAS候选

统一类型：

```text
SnapshotArtifactAggregateRefV2 =
  TypeURL="praxis.sandbox/snapshot-artifact-aggregate-ref/v2", Version=2,
  Owner=SnapshotArtifactOwnerV2(core.OwnerRef),
  Kind="praxis.sandbox/snapshot-artifact-aggregate",
  AggregateID, Revision, TenantID, DataDomain,
  Schema(SchemaRefV2="praxis.sandbox/snapshot-artifact-aggregate-ref/v2"),
  DigestAlgorithm="sha256",
  DigestDomain="praxis.sandbox/snapshot-artifact-aggregate-ref/body/v2",
  Digest, ExpiresUnixNano

SnapshotArtifactAggregateEntryV2 =
  EntryRef + ArtifactSubjectRef + EntryKind + FactRef + PreviousEntryRef presence
  RequestedNotAfter + EntryExpiresUnixNano

SnapshotArtifactAggregateEnvelopeV2 =
  AggregateEnvelopeRef + RequestedNotAfter + EnvelopeExpiresUnixNano
  PreviousAggregateRef presence
  AppliedEntryRef
  ReservationFactRef
  ArtifactFactRef presence
  RetentionApplicationFactRef presence
  ActiveDeletionAttemptFactRef presence
  LastClosedDeletionAttemptFactRef presence
  TerminalTombstoneRef presence
  AggregateState

SnapshotArtifactAggregateCurrentIndexV2 = exact DTO in 3.6.1
SnapshotArtifactTerminalTombstoneV2 = exact DTO in 3.6.2
```

- Reservation、Artifact Fact、Retention Application、Deletion Attempt/Tombstone全部先按3.2
  seal独立payload，随后seal Entry，再把Entry与下一Envelope、CurrentIndex推进原子提交；payload
  不反向引用Entry/Envelope/CurrentIndex或后继payload；
- 每个非初始command必须携完整`ExpectedAggregateRef(Owner/Kind/ID/Revision/Tenant/Schema/Digest/
  Expires)`，不能只给revision；Aggregate Ref body覆盖全部字段、排除own Digest，TTL不得晚于其
  Head Envelope/CurrentIndex active closure；Owner/Kind/Schema仅由`SnapshotArtifactOwnerV2`写入；
  初始Reserve显式携ExpectedAggregateRef=absent；
- history key包含`ID+Revision+Digest`，CurrentIndex只指向最高已提交Envelope且自身revision单调；
- 同ExpectedAggregateRef并发只有一个winner；同revision不同内容Conflict，失败方只Inspect；
- Artifact Fact不可变。Retention与Deletion追加新Entry/Envelope，不能覆盖旧payload；
- CAS回包丢失按expected next Envelope exact Ref和aggregate current Inspect；一致返回原结果，
  不一致Conflict，不回退、不制造ABA；
- terminal CurrentIndex/Tombstone只允许同state续期或增加审计ref，禁止改回nonterminal、切换
  terminal variant或清除terminal presence；
- 历史Envelope/Entry过期后仍可Shape/摘要审计Inspect；首切片`InspectAggregateCurrent`只证明
  Store current index与state-active TTL闭集，不复读未来执行资格Owner。

## 5. 首切片Port、未来窄Command与包内提交器

外部候选Port只暴露Reserve/Inspect：

```text
ReserveArtifact(ctx, ReserveArtifactRequestV2)
  -> SnapshotArtifactReservationV2, Created

InspectReservation(ctx, ExactReservationRef)
InspectReservationByStableKey(ctx, StableSourceKeyV2)
InspectAggregateHistorical(ctx, SnapshotArtifactAggregateRefV2)
InspectAggregateCurrent(ctx, ArtifactAggregateID, ExpectedAggregateRef presence)
InspectEntryHistorical(ctx, EntryExactRef)
```

首切片**只有**上述Reserve/Inspect，不能接收Observation、Fact、Retention、Deletion command，
也没有任何`Apply*`、`Commit*`、raw CAS、generic mutate或caller-mint payload方法。

未来跨Owner写入即使获批，也只能另行冻结按用例收窄的governed command Port，例如Artifact
seal、Retention application、Deletion result各自独立typed command；command必须只携Operation/
Effect/Attempt、DomainResult、Evidence consumed、opaque Runtime Settlement、ExpectedAggregateRef
等exact refs，不能携caller构造的Fact/Entry/Envelope/CurrentIndex。Port返回command acceptance或
Inspect coordinate，不返回raw CAS能力。

实际seal canonical、构造Entry/Envelope、CAS CurrentIndex的committer必须是
`SnapshotArtifactOwnerV2`实现包内不可导出的函数/对象；不得出现在`ports`、不得跨包注入、不得
由testkit或其他Owner取得interface。Owner closure未冻结时，未来窄Command Port与包内committer
都不创建。

- S1：Owner接收内部command后，复读ExpectedAggregateRef、目标payload provenance及该command
  所需Owner current，形成sealed candidate；
- S2：CAS前重新复读aggregate current和所有可撤销门。Artifact/Retention committer至少重读
  自身source refs；Deletion committer必须重读Operation/Attempt、Authority/Review/Budget/
  Scope/Permit、Retention Policy及`NoActiveLegalHoldCurrentProjectionV1`；
- S1/S2 proof exact refs进入Deletion Fact canonical digest；S2必须不早于S1且CAS时fresh。
  任一Unknown/Unavailable/expired/drift都返回零写。

### 5.1 Owner clock、RequestedNotAfter与freshness

所有时间判断只使用`SnapshotArtifactOwnerV2`注入的单调Owner Clock；caller时间、Provider时间与
Receipt时间只作Observation。Owner必须持久比较`LastOwnerClockWatermark`：wall clock回退、clock
uncertain或读数小于已提交watermark时fail closed、零写，禁止用回退时间延长TTL。

| 对象 | `RequestedNotAfter`与Owner min规则 | fresh/pre-post/expiry规则 |
|---|---|---|
| `SnapshotArtifactSubjectIdentityV2` stable identity | 无Revision/TTL、不接受RequestedNotAfter；只作坐标，不授current | 不调用`ValidateCurrent`，不进入BindingSet min，不能凭稳定ID绕过任何current门 |
| `SnapshotArtifactSubjectRefV2` versioned exact/current ref | `min(request, OwnerMaxSubjectRefTTL, Owner实际复读的Reservation/source/current-index闭包)` | current pointer与exact ref的Revision/SubjectDigest/Expires必须一致；`now>=Expires`或同stable identity换revision/TTL仍复用digest均拒绝 |
| Reservation payload/Fact | `min(request, OwnerMaxReservationTTL, Reserve闭包中实际复读的exact source/policy refs)` | S1复读后seal；CAS前S2复读可撤销输入；任一缺Reader则unsupported/零写 |
| Artifact Fact payload | `min(request, OwnerMaxFactTTL, Evidence/Inspection/source closure TTL)` | S1/S2都fresh才seal；到期后只允许Shape/摘要历史Inspect，不改AggregateState |
| Retention Application Fact | `min(request, OwnerMaxFactTTL, RetentionPolicy/current decision TTL)` | Policy revision可单调前进但candidate必须重算；旧Fact expiry不杀AggregateCurrent |
| Deletion Attempt Fact | `min(command.RequestedNotAfter, OwnerMaxAttemptTTL, Operation/Attempt/Authority/Review/Budget/Scope/Permit/Evidence/Inspection及S2 no-hold natural TTL)` | S1形成candidate、S2在CAS前重读；S2变化要求Sandbox重做Deletion candidate，不能要求Retention Owner按caller bound重seal |
| cross-owner `NoActiveLegalHoldCurrentProjectionV1` | 不接受Sandbox/caller RequestedNotAfter；Retention Owner只按自身规则给natural Checked/Expires | Reader输入不含caller bound；不同caller bound返回同一current exact ref/digest |
| Entry / Envelope history | `min(request, OwnerMaxHistoryTTL, sealed payload history TTL)` | 只控制完整历史载荷保留；到期不删除Ref/Digest，不改变CurrentIndex |
| Aggregate Current projection | `min(request, OwnerMaxProjectionTTL, 3.6中state-active闭集)` | Inspect前读、返回前复读CurrentIndex；任一变化返回零projection |
| terminal Tombstone / CurrentIndex | 持久状态不可TTL删除；每次current读取有Owner限制的`RequestedNotAfter`窗口 | 窗口过期仅current unavailable；续期保持terminal state/lineage，不得复活 |

统一时序为`t_pre <= t_seal <= t_post <= t_commit`；pre/post均由Owner clock记录。所有参与当前
提交的ref必须满足`t_commit < ExpiresUnixNano`，因此`now == ExpiresUnixNano`一律expired。
Sandbox-owned command的`RequestedNotAfter`必须显式presence且大于`t_pre`；Sandbox Owner只能
取min，不能接受caller延长。该规则不改变跨Owner no-hold Projection。若post发现revision/digest/
TTL/watermark变化，丢弃Sandbox candidate并从新S1开始；不能要求Retention Owner按caller bound
重封，也不能在原candidate上局部修补。

## 6. AttemptState、AggregateState与恢复候选

两个状态维度禁止复用同一枚举：

```text
DeletionAttemptState = reserved | unknown | failed | confirmed | indeterminate
ArtifactAggregateState = reserved | available | deletion_in_progress | deleted | indeterminate
```

```text
aggregate.absent
  -> aggregate.reserved
  -> aggregate.available
       |-- retention revision(s) --------------------------> aggregate.available
       `-- reserve new deletion Attempt -------------------> aggregate.deletion_in_progress
              |-- attempt.unknown --Inspect same Attempt---> unknown|failed|confirmed|indeterminate
              |-- attempt.failed --close active Attempt----> aggregate.available
              |-- attempt.confirmed + Runtime sibling Settlement current
              |   + Sandbox Apply CAS + terminal tombstone -> aggregate.deleted
              `-- attempt.indeterminate + tombstone -------> aggregate.indeterminate

aggregate.deleted / aggregate.indeterminate: terminal, no resurrection
```

Deletion Attempt stable key为`TenantID+ArtifactAggregateID+OperationID+EffectID+AttemptID`；
aggregate one-active key为`TenantID+ArtifactAggregateID+purpose=deletion`。只有
`AggregateState=available`且ActiveDeletionAttempt presence=absent时可CAS新Attempt；unknown仍是
active，阻塞任何新Attempt。failed必须先由Owner追加closed-attempt Entry/Envelope并清空active
presence，之后重试必须使用全新Operation/Effect/Attempt stable key及fresh治理；复用任一旧坐标
Conflict。confirmed只有在Runtime sibling Settlement current与Sandbox Apply CAS闭合后才能发布
`deleted` tombstone；indeterminate发布对应terminal tombstone后，任何Reserve/CAS/TTL refresh只能返回
terminal exact ref或Conflict，不能创建Attempt。

NotFound恢复按权威边界区分：

1. **Reservation replay**：仅Owner Store对stable key的linearizable Inspect确认absent，且不存在
   pending command/expected successor时，才可用完全相同request与Owner派生ID重放create-once；
   caller换ID/TTL/content不是恢复而是Conflict。
2. **CAS winner/lost reply**：回包丢失后Inspect expected next EnvelopeRef、CommandID与CurrentIndex。
   已存在同digest successor则返回原winner；存在其他successor则Conflict；只有Owner current确认
   head仍为expected previous且CommandID未出现时才可重放同一CAS。非权威缓存NotFound无效。
3. **Provider Unknown**：Provider timeout/NotFound/进程死亡从不证明未执行。只Inspect原provider
   Attempt，保持同一Deletion Attempt active；禁止重放Provider、换Provider、换Reservation或创建
   新Attempt。无法获得权威结果时只可收敛aggregate indeterminate。

任何`reserved -> available`或Deletion结果迁移都必须来自Owner closure；零Provider、零Runtime
写的Store本身不能推导Observation、Evidence、Inspection、Settlement或删除完成。Retention或
历史Fact到期不等于deleted，也不能把terminal/current index当absent。

## 7. 实现授权门

联合Review必须逐项给YES：

1. Runtime/Continuity/Retention-Sandbox Owner表无双主，尤其Retention Policy与Legal Hold；
2. 无Revision/TTL的stable `SnapshotArtifactSubjectIdentityV2`与带Revision/TTL的versioned exact/current
   `SnapshotArtifactSubjectRefV2`分层，并冻结SubjectRef→payload FactRef→EntryRef→EnvelopeRef四层canonical及StorageArtifactRef/FactRef完整exact DTO/digest domain；
3. 统一entry/envelope/current index、terminal tombstone的exact DTO/presence/TTL canonical、ExpectedAggregateRef、history/current与外部Reserve/Inspect Port冻结；
4. 首切片严格Reserve/Inspect；未来仅typed governed command，包内committer/raw CAS不可导出；
5. NoActiveLegalHold exact index ref、watermark-generation等值、词典序S1/S2与跨代coverage carry proof冻结；
6. AggregateState active TTL闭集、terminal tombstone及Owner clock/pre-post规则冻结；
7. AttemptState/AggregateState、one-active-attempt、retry stable key与三类NotFound恢复冻结；
8. Deletion Operation/Settlement链、acceptance/test matrix获批且用户明确授权Go实现。

任一项缺失时：`FeatureSnapshotArtifactOwner=false`（若未来增加该feature）、Provider=0、
Runtime/Continuity写=0、production Backend/root=0。

## 8. 第三短审关闭账本

| 级别 | 问题 | Sandbox候选资产结论 | 跨Owner剩余门 |
|---|---|---|---|
| P0-1 | canonical层级仍可能自含digest，Storage ArtifactRef与FactRef混型 | 已关闭：固定Subject/ID→PayloadBodyDigest→FactRef→EntryBodyDigest→EntryRef→EnvelopeBodyDigest→EnvelopeRef；每层排除own Ref/Digest并区分两种Ref | 联合Review确认type URL/字段名 |
| P0-2 | no-hold proof未证明穷尽coverage、S1/S2错误要求revision exact，caller bound污染Owner projection | 已关闭：proof绑定HoldIndex generation、穷尽集合digest/count、coverage/jurisdiction/hold-kind、watermark；S1/S2 subject+coverage exact，S2单调且仍NoActive；proof Reader/canonical排除RequestedNotAfter，Sandbox只在Deletion min应用caller bound | Retention/Legal Hold Owner确认proof/Reader公共形状 |
| P0-3 | 全历史TTL参与min会永久杀死current，terminal可因过期复活 | 已关闭：按AggregateState定义active TTL闭集；历史retention不参与；deleted/indeterminate由不可回退Tombstone/CurrentIndex维持 | 联合Review确认CurrentIndex持久化/续期形状 |
| P0-4 | internal committer可能被跨包当公共写口 | 已关闭：首切片只有Reserve/Inspect；未来跨Owner仅typed governed command；seal/CAS committer包内不可导出，raw Apply/CAS永不公开 | Runtime治理链闭合后另审future Port |
| P0-5 | clock/RequestedNotAfter/freshness边界不完整 | 已关闭：Sandbox-owned对象min表、cross-owner proof例外、Owner clock watermark、pre/post、rollback fail-closed及`now==expires`已冻结为候选 | 联合Review确认Owner clock注入形状 |
| P1-1 | AttemptState/AggregateState混用与多active Attempt | 已关闭：两套枚举、one-active key、failed关闭后全新stable key、terminal阻断重试 | 联合Review确认枚举值 |
| P1-2 | NotFound恢复混淆Reservation、CAS lost reply和Provider Unknown | 已关闭：linearizable Reservation replay、CAS winner Inspect、Provider Unknown只Inspect原Attempt三分 | 联合Review确认Store/Provider Inspect能力 |
| P2 | 资产与反例矩阵一致性 | 已关闭：README/contracts/interfaces/state/workspace/port delta/acceptance/plan/test matrix/memory已同步，残留/格式/XML轻门通过；仍未勾实现、Provider=0 | 提交联合Review |

当前可冻结性：**Sandbox内部候选语义可再次短审，整体不可直接冻结为实现合同**。只有Retention/
Legal Hold Owner确认穷尽negative proof/Reader，Runtime确认Deletion独立Operation/Settlement与
future governed command边界，并由管理线确认canonical/clock/current-index DTO后，才可改为
approved。此前实现持续NO-GO。

## 9. 第四短审关闭账本

| 级别 | 问题 | Sandbox候选资产结论 | 外部剩余门 |
|---|---|---|---|
| P0-1 | watermark generation未与HoldIndex generation锁定，跨代coverage无连续证明 | 已关闭：Projection绑定HoldIndex exact TypeURL/Version/ID/Generation/Revision/DigestDomain/Digest/TTL；强制watermark generation等值；S1→S2按`(generation,sequence)`词典序，跨代必须连续CoverageCarryProof exact refs | Retention/Legal Hold Owner确认Index/Carry public DTO与Reader |
| P0-2 | CurrentIndex/Tombstone只有抽象字段，presence/TTL及own digest排除未冻结 | 已关闭：冻结两个exact DTO的type URL/version/digest domain、全部presence/ExactRef TTL、state闭表、canonical body与own Ref/Digest排除；terminal lineage不可回退 | 管理线确认DTO命名与持久化策略 |
| P0-3 | StorageArtifactRef仍是“等字段”描述，无法阻止跨domain替换 | 已关闭：冻结`SnapshotStorageArtifactRefV2`完整exact DTO、type URL/version/revision/digest algorithm/domain/body及禁止字段；FactRef使用独立type URL/digest domain | 管理线确认DTO命名；生产storage/backend仍NO-GO |
| P1 | 反例不足 | 已关闭：acceptance/plan/test matrix补watermark/index错配、跨代缺carry、presence/TTL篡改、terminal非法组合、type/version/digest-domain混型与raw locator | 提交第四短审 |
| P2 | 资产一致性 | 已关闭：第四短审残留、required语义、proof caller-bound、格式、冲突标记与Draw.io XML轻门通过；未勾实现、Provider=0 | 无 |

第四短审的历史结论已被后续Workspace capture/Restore授权覆盖；Retention/Legal Hold、Runtime
purge sibling和管理线terminal P0未全部关闭前，仍不创建Deletion governed command或Provider。

## 10. Snapshot purge治理与Runtime Settlement V5 additive sibling候选

本节状态为`review_pending`。live Runtime
`OperationSettlementCurrentReaderV5`只暴露
`InspectCheckpointPhaseSettlementCurrentV5`，其DTO、提交闭包和Conformance都绑定Checkpoint
Attempt/Phase；公共测试还锁定“一方法Reader + Gateway marker”。因此它不能被Sandbox扩大解释、
不能新增Snapshot方法，也不能作为purge Settlement current证明。Snapshot purge必须由Runtime
Owner另行落地additive sibling合同；代码落地和联合Review前，Sandbox对应命令固定
`unsupported`、Provider调用为0。

### 10.1 删除语义与Effect闭表

| 对象 | 候选名称 | 语义 | 明确不是 |
|---|---|---|---|
| delete request | `SnapshotArtifactPurgeRequestV1` | Sandbox Owner接收的删除意图和exact坐标；只绑定上游Deletion Reservation并形成Request候选 | 已删除、已Dispatch、Provider执行权或Runtime Settlement、Deletion Attempt |
| delete Operation | `praxis.sandbox/snapshot-artifact-deletion` | Runtime治理该次删除工作流的custom Operation | Checkpoint phase、termination或cleanup Operation |
| purge Effect | `praxis.sandbox/snapshot-artifact-purge` | **唯一**允许造成Snapshot物理不可逆删除的Effect | delete request、retention到期、Provider NotFound或cleanup |
| purge DomainResult | `praxis.sandbox/snapshot-artifact-purge-result` | Sandbox Owner在Evidence consumed与独立Inspect后提交的领域结果 | Runtime Outcome、Settlement或`deleted` AggregateState |
| cleanup Effect | `praxis.sandbox/snapshot-artifact-purge-cleanup` | 独立治理残留索引、临时对象、staging标记或远端残留 | purge重试、purge成功的隐含步骤或删除事实 |
| deleted | `SnapshotArtifactTerminalTombstoneV2(state=deleted)` | purge DomainResult→Runtime sibling Settlement current→Sandbox Apply CAS全部闭合后的Owner终态 | request accepted、Begin、Receipt、NotFound、进程死亡或cleanup完成 |

purge固定`RiskClass=praxis.risk/irreversible-delete`、
`ConflictDomain=praxis.sandbox/snapshot-artifact-purge`；cleanup固定
`ConflictDomain=praxis.sandbox/snapshot-artifact-purge-cleanup`并使用独立EffectID、Attempt、Intent、
Admission、Review、Permit、prepare/execute Enforcement、Evidence、DomainResult和Settlement，不得与
purge合并。cleanup的成功/失败不能把aggregate改成`deleted`，purge的`deleted`也不能证明cleanup
complete；未解决Residual继续占用自己的Conflict Domain。

`SnapshotArtifactPurgeRequestV1`固定字段：

```text
TypeURL="praxis.sandbox/snapshot-artifact-purge-request/v1", Version=1
Owner=SnapshotArtifactOwnerV2(core.OwnerRef)
Kind="praxis.sandbox/snapshot-artifact-purge-request"
RequestID, Revision=1, TenantID
Schema(SchemaRefV2="praxis.sandbox/snapshot-artifact-purge-request/v1")
ArtifactSubjectRefV2, SnapshotArtifactFactRefV2
ExpectedAggregateRefV2, ExpectedCurrentIndexRefV2
DeletionReservationFactRefV2
RequestedNotAfter, DigestAlgorithm="sha256", Digest, ExpiresUnixNano

SnapshotArtifactDeletionAttemptRefV2 =
  TypeURL="praxis.sandbox/snapshot-artifact-deletion-attempt-ref/v2", Version=2,
  Owner=SnapshotArtifactOwnerV2(core.OwnerRef),
  Kind="praxis.sandbox/snapshot-artifact-deletion-attempt",
  AttemptID, Revision, TenantID,
  OperationDigest, EffectID, PurgeRequestDigest,
  Schema(SchemaRefV2="praxis.sandbox/snapshot-artifact-deletion-attempt-ref/v2"),
  DigestAlgorithm="sha256",
  DigestDomain="praxis.sandbox/snapshot-artifact-deletion-attempt-ref/body/v2",
  Digest, ExpiresUnixNano
```

canonical domain固定为
`praxis.sandbox/snapshot-artifact-purge-request/body/v1`，body排除自身Digest并覆盖全部exact ref、
presence和TTL。Request明确**不含**`DeletionAttemptRefV2`、AttemptID、OperationDigest、EffectID、
Evidence/Settlement/Apply或任何后继ref；`DeletionReservationFactRefV2`的canonical同样不得反向引用
PurgeRequest或DeletionAttempt。唯一创建链固定为：

```text
DeletionReservationFactRefV2（upstream）
  -> SnapshotArtifactPurgeRequestV1（绑定Reservation，不绑定Attempt）
  -> SnapshotArtifactDeletionAttemptRefV2（绑定PurgeRequestDigest）
  -> Runtime Operation/Effect/Enforcement/Evidence/DomainResult/Settlement
  -> Sandbox Apply/Tombstone
```

Request create lost reply只能按`RequestID+Digest`Inspect原Request；Attempt create lost reply只能按
`AttemptID+PurgeRequestDigest`Inspect原Attempt。禁止为了补Attempt坐标重封Request，禁止以新Request/
Attempt恢复Unknown。BindingSet在Attempt创建后分别携Request与DomainAttempt exact ref，因此Request无需
也不得反向携Attempt。Operation使用`OperationSubjectV3`的custom identity，仅设置
`CustomOperationID=RequestID`，不得填Run/Activation/Termination/Admin identity。Effect Intent
必须把`ActionScopeDigest`绑定ArtifactSubject+ExpectedAggregate，`PayloadDigest`绑定Request
Digest，`Target`绑定ArtifactAggregateID，并精确绑定tenant、Owner、Provider、Authority、Review、
Budget、Scope、Credential、Idempotency、Residual/Cleanup与全部TTL。

### 10.2 Retention/Legal Hold exact Index/Carry/Reader候选

以下均为外部Retention/Legal Hold唯一Owner的`review_pending`公共候选，不由Sandbox、Continuity、
Provider或Management实现。canonical统一使用SHA-256、固定字段顺序、显式presence、排序去重集合、
UnixNano；每个body排除own Ref/Digest，message不得携raw backend locator或caller
`RequestedNotAfter`。

```text
LegalHoldSubjectCoordinateV1 =
  TypeURL, Version, TenantID, DataDomain,
  SubjectNamespace, SubjectID,
  SubjectDigest=SnapshotArtifactSubjectIdentityV2.StableSubjectDigest

LegalHoldCoverageV1 =
  DataDomain, SubjectSelectorDigest,
  Jurisdictions[], HoldKinds[], CoverageDigest

LegalHoldIndexStableRefV1 =
  TypeURL, Version, IndexID, TenantID, DataDomain, StableDigest

LegalHoldIndexExactRefV1 =
  TypeURL, Version, IndexID, Generation, Revision,
  TenantID, DataDomain, DigestAlgorithm, DigestDomain, Digest,
  ExpiresUnixNano

ExpectedLegalHoldIndexCurrentRefV1 =
  Presence=absent|present,
  LegalHoldIndexExactRefV1（present时必填；absent时必须零值）

LegalHoldCoverageCarryExactRefV1 =
  TypeURL, Version, CarryID, Revision, TenantID,
  FromIndexExactRefV1, ToIndexExactRefV1,
  FromWatermark(Generation,Sequence), ToWatermark(Generation,Sequence),
  CoverageDigest, DigestAlgorithm, DigestDomain, Digest,
  ExpiresUnixNano

NoActiveLegalHoldProjectionExactRefV1 =
  TypeURL, Version, ProjectionID, Revision, ProofSeriesID,
  TenantID, SubjectDigest=SnapshotArtifactSubjectIdentityV2.StableSubjectDigest, CoverageDigest,
  IndexExactRefV1, QueryWatermark(Generation,Sequence),
  DigestAlgorithm, DigestDomain, Digest, ExpiresUnixNano

LegalHoldIndexCurrentProjectionV1 =
  TypeURL, Version, IndexID, Generation, Revision,
  TenantID, DataDomain, IndexRootDigest, HeadSequence,
  State="active", OwnerClockWatermark,
  CheckedUnixNano, ExpiresUnixNano, Digest

LegalHoldCoverageCarryProofV1 =
  TypeURL, Version, CarryID, Revision, TenantID,
  FromIndexExactRefV1, ToIndexExactRefV1,
  FromWatermark(Generation,Sequence), ToWatermark(Generation,Sequence),
  CoverageV1, FromIndexRootDigest, ToIndexRootDigest,
  CarriedSetDigest, CarriedSetCount,
  OwnerClockWatermark, CheckedUnixNano, ExpiresUnixNano, Digest

NoActiveLegalHoldCurrentProjectionV1 =
  TypeURL, Version, ProjectionID, Revision, ProofSeriesID,
  TenantID, SubjectCoordinateV1, RetentionPolicyCurrentRef,
  IndexExactRefV1,
  CoverageV1, QueryWatermark(Generation,Sequence),
  CarryProofRefsPresence, LegalHoldCoverageCarryExactRefV1[],
  EnumeratedActiveHoldSetDigest, EnumeratedActiveHoldCount=0,
  NoActive=true, OwnerClockWatermark,
  CheckedUnixNano, ExpiresUnixNano, Digest
```

候选type URL与digest domain逐项固定为：

| DTO | TypeURL | DigestDomain |
|---|---|---|
| Subject | `praxis.retention-governance/legal-hold-subject/v1` | `praxis.retention-governance/legal-hold-subject/body/v1` |
| Coverage | `praxis.retention-governance/legal-hold-coverage/v1` | `praxis.retention-governance/legal-hold-coverage/body/v1` |
| Index stable ref | `praxis.retention-governance/legal-hold-index-stable-ref/v1` | `praxis.retention-governance/legal-hold-index-stable-ref/body/v1` |
| Index exact ref | `praxis.retention-governance/legal-hold-index-exact-ref/v1` | `praxis.retention-governance/legal-hold-index-exact-ref/body/v1` |
| Expected index current | `praxis.retention-governance/legal-hold-index-expected-current-ref/v1` | `praxis.retention-governance/legal-hold-index-expected-current-ref/body/v1` |
| Index current | `praxis.retention-governance/legal-hold-index-current/v1` | `praxis.retention-governance/legal-hold-index-current/body/v1` |
| Coverage carry exact ref | `praxis.retention-governance/legal-hold-coverage-carry-exact-ref/v1` | `praxis.retention-governance/legal-hold-coverage-carry-exact-ref/body/v1` |
| Coverage carry | `praxis.retention-governance/legal-hold-coverage-carry/v1` | `praxis.retention-governance/legal-hold-coverage-carry/body/v1` |
| NoActive exact ref | `praxis.retention-governance/legal-hold-no-active-exact-ref/v1` | `praxis.retention-governance/legal-hold-no-active-exact-ref/body/v1` |
| NoActive current | `praxis.retention-governance/legal-hold-no-active-current/v1` | `praxis.retention-governance/legal-hold-no-active-current/body/v1` |

Reader请求/结果闭表为：

```text
InspectLegalHoldIndexCurrentV1(
  TenantID, DataDomain, LegalHoldIndexStableRefV1,
  ExpectedLegalHoldIndexCurrentRefV1,
) -> LegalHoldIndexCurrentProjectionV1

InspectNoActiveLegalHoldCurrentV1(
  TenantID, LegalHoldSubjectCoordinateV1, LegalHoldCoverageV1,
  LegalHoldIndexStableRefV1, ExpectedLegalHoldIndexCurrentRefV1,
  ExpectedProjectionRefPresence, NoActiveLegalHoldProjectionExactRefV1,
) -> NoActiveLegalHoldCurrentProjectionV1

InspectNoActiveLegalHoldHistoricalV1(
  TenantID, NoActiveLegalHoldProjectionExactRefV1,
) -> NoActiveLegalHoldCurrentProjectionV1

InspectLegalHoldCoverageCarryExactV1(
  TenantID, LegalHoldCoverageCarryExactRefV1,
) -> LegalHoldCoverageCarryProofV1
```

Reader只返回Owner sealed current/exact对象，不返回Policy写权、Hold写权、raw index或CAS。Projection
的`QueryWatermark.Generation == IndexExactRef.Generation`；S1→S2按`(generation,sequence)`词典序
非递减；跨代必须用相邻、连续、coverage exact的Carry链，同代Carry必须explicit absent。S1/S2
subject、tenant、ProofSeries和Coverage exact；S2仍NoActive且fresh。同Index ID/generation/revision
换digest、跨代缺链、carry端点/coverage/selector/jurisdiction/hold-kind漂移、NotFound、Unavailable、
Unknown、expired或`now == expires`全部fail closed。Sandbox只在自身Deletion Attempt/Aggregate TTL
中应用caller `RequestedNotAfter`，不得让Retention Owner重封Projection。

四个Reader逐方法closed错误与NotFound权威边界如下；未列错误一律归一为
`internal/execution_inspection_invalid`并返回零对象：

| Reader | 允许Category/Reason | NotFound/Unknown语义 |
|---|---|---|
| Index current | `invalid_argument/{invalid_reference,invalid_digest,invalid_state}`；`not_found/invalid_reference`；`conflict/{revision_conflict,invalid_digest}`；`precondition_failed/{capability_expired,clock_regression}`；`indeterminate/inspect_coverage_incomplete`；`unavailable/evidence_unavailable`；`internal/execution_inspection_invalid` | 只有Retention Owner对stable index current pointer的linearizable absent可返回NotFound；它只表示该index current不存在，不证明无hold。读取不完整/未知必须Indeterminate，Sandbox零写、零Provider |
| NoActive current | 同上 | NotFound绝不等于NoActive；Unknown、过期、coverage不完整或expected current漂移均fail closed，不能改用nil/空集合 |
| NoActive historical | `invalid_argument/{invalid_reference,invalid_digest}`；`not_found/invalid_reference`；`conflict/{revision_conflict,invalid_digest}`；`unavailable/evidence_unavailable`；`internal/execution_inspection_invalid` | exact history不存在才可由Owner返回NotFound；过期历史仍可Shape/digest Inspect，但永不满足新purge current资格；存储未知为Unavailable而非NotFound |
| Carry exact | `invalid_argument/{invalid_reference,invalid_digest}`；`not_found/invalid_reference`；`conflict/{revision_conflict,invalid_digest}`；`precondition_failed/capability_expired`；`indeterminate/inspect_coverage_incomplete`；`unavailable/evidence_unavailable`；`internal/execution_inspection_invalid` | exact carry缺失、未知或过期均不能被当成“无需carry”；跨代purge必须零写 |

current Reader返回对象必须与ExpectedCurrent full exact ref完全一致；expected absent只允许Owner证明当前
pointer线性化absent，不能用于negative hold证明。history Reader只按full exact ref读取append-only对象，
不读取current pointer。任一Reader lost reply只按同一request digest、expected exact ref与Owner read
watermark重复Inspect；不得换stable index、ProofSeries、coverage、generation或把NotFound升级为
NoActive。相同request得到同Ref为幂等；同read key换内容为Conflict。

### 10.3 Purge Evidence Issue/Record/Consume候选

Purge Evidence的Qualification签发、Ledger Record与Consumption唯一Owner均为Runtime Evidence
Owner；Sandbox只提交绑定Provider Observation/Receipt的Candidate，Provider/Enforcer不能Issue、
Record或Consume。Runtime Gateway负责current治理复读，Owner-only Fact Port负责原子写入；
Qualification、Record或Consumption都不授Provider执行权，也不等于purge成功。

全部Purge Evidence exact对象使用`ContractVersion="1.0.0"`、
`DigestAlgorithm="sha256"`。canonical body覆盖下列全部字段和presence，排除对象自身`Digest`；
`now >= ExpiresUnixNano`即不current，派生TTL不得晚于Scope、Qualification、Handoff、Enforcement、
Candidate、Ledger chain/cursor及调用方`RequestedNotAfter`的最早到期时间。

| 对象 | TypeURL | DigestDomain |
|---|---|---|
| Scope | `praxis.runtime/operation-snapshot-purge-evidence-scope/v1` | `praxis.runtime/operation-snapshot-purge-evidence-scope/body/v1` |
| Qualification ref | `praxis.runtime/operation-snapshot-purge-evidence-qualification-ref/v1` | `praxis.runtime/operation-snapshot-purge-evidence-qualification-ref/body/v1` |
| Qualification current | `praxis.runtime/operation-snapshot-purge-evidence-qualification-current/v1` | `praxis.runtime/operation-snapshot-purge-evidence-qualification-current/body/v1` |
| Provider handoff ref | `praxis.runtime/operation-snapshot-purge-evidence-provider-handoff-ref/v1` | `praxis.runtime/operation-snapshot-purge-evidence-provider-handoff-ref/body/v1` |
| Provider handoff current | `praxis.runtime/operation-snapshot-purge-evidence-provider-handoff-current/v1` | `praxis.runtime/operation-snapshot-purge-evidence-provider-handoff-current/body/v1` |
| Record ref | `praxis.runtime/operation-snapshot-purge-evidence-record-ref/v1` | `praxis.runtime/operation-snapshot-purge-evidence-record-ref/body/v1` |
| Chain ref | `praxis.runtime/operation-snapshot-purge-evidence-chain-ref/v1` | `praxis.runtime/operation-snapshot-purge-evidence-chain-ref/body/v1` |
| Cursor ref | `praxis.runtime/operation-snapshot-purge-evidence-cursor-ref/v1` | `praxis.runtime/operation-snapshot-purge-evidence-cursor-ref/body/v1` |
| Consumption ref | `praxis.runtime/operation-snapshot-purge-evidence-consumption-ref/v1` | `praxis.runtime/operation-snapshot-purge-evidence-consumption-ref/body/v1` |
| Qualification-consumed ref | `praxis.runtime/operation-snapshot-purge-evidence-qualification-consumed-ref/v1` | `praxis.runtime/operation-snapshot-purge-evidence-qualification-consumed-ref/body/v1` |
| Atomic commit bundle | `praxis.runtime/operation-snapshot-purge-evidence-record-consumption-commit/v1` | `praxis.runtime/operation-snapshot-purge-evidence-record-consumption-commit/body/v1` |
| Atomic commit current | `praxis.runtime/operation-snapshot-purge-evidence-record-consumption-current/v1` | `praxis.runtime/operation-snapshot-purge-evidence-record-consumption-current/body/v1` |
| Phase binding set | `praxis.runtime/operation-snapshot-purge-evidence-binding-set/v1` | `praxis.runtime/operation-snapshot-purge-evidence-binding-set/body/v1` |

```text
OperationSnapshotPurgeEvidencePhaseV1 = prepare | execute

OperationSnapshotPurgeEvidenceScopeV1 =
  ContractVersion, TypeURL,
  OperationSubjectV3, OperationDigest,
  EffectID, EffectRevision, EffectKind, IntentDigest,
  Admission, ReviewAuthorization,
  DispatchAttemptRefV3, PermitID, PermitFactRevision, PermitDigest,
  AuthorizedAdmissionDigest,
  Phase, PhaseEnforcementRefV4,
  BindingSetDigest,
  Authority, EvidencePolicy,
  PayloadSchema, PayloadDigest, PayloadRevision, PayloadLength,
  EvidenceSourceKeyV2,
  CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm, DigestDomain, Digest

OperationSnapshotPurgeEvidenceQualificationRefV1 =
  ContractVersion, TypeURL,
  QualificationID, Revision,
  TenantID, OperationDigest, EffectID, EffectRevision,
  DispatchAttemptRefV3, Phase, PhaseEnforcementRefV4,
  BindingSetDigest, ScopeDigest,
  DigestAlgorithm, DigestDomain, Digest, ExpiresUnixNano

OperationSnapshotPurgeEvidenceQualificationCurrentV1 =
  ContractVersion, TypeURL,
  QualificationRefV1, ScopeDigest,
  Current=true, ConsumedPresence,
  QualificationConsumedRefV1,
  CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm, DigestDomain, Digest

OperationSnapshotPurgeEvidenceProviderHandoffRefV1 =
  ContractVersion, TypeURL,
  HandoffID, Revision=1, TenantID,
  QualificationRefV1, DispatchAttemptRefV3,
  Phase, PhaseEnforcementRefV4,
  ProviderBindingRefV2, ScopeDigest,
  CreatedUnixNano, ExpiresUnixNano,
  DigestAlgorithm, DigestDomain, Digest

OperationSnapshotPurgeEvidenceProviderHandoffCurrentV1 =
  ContractVersion, TypeURL,
  ProviderHandoffRefV1, QualificationRefV1,
  ProviderBindingRefV2, ScopeDigest,
  Current=true, CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm, DigestDomain, Digest

OperationSnapshotPurgeEvidenceRecordRefV1 =
  ContractVersion, TypeURL,
  RecordID, Revision=1, TenantID,
  QualificationRefV1, ProviderHandoffRefV1,
  EvidenceRecordRefV2(LedgerScopeDigest,Sequence,RecordDigest),
  CandidateDigest, EvidenceSourceKeyV2,
  DispatchAttemptRefV3, Phase, ScopeDigest,
  DigestAlgorithm, DigestDomain, Digest, ExpiresUnixNano

OperationSnapshotPurgeEvidenceChainRefV1 =
  ContractVersion, TypeURL,
  ChainID, Revision, TenantID, LedgerScopeDigest,
  PreviousHeadSequence, PreviousHeadRecordDigest,
  HeadSequence, HeadRecordDigest,
  DigestAlgorithm, DigestDomain, Digest, ExpiresUnixNano

OperationSnapshotPurgeEvidenceCursorRefV1 =
  ContractVersion, TypeURL,
  CursorID, Revision, TenantID, LedgerScopeDigest,
  EvidenceSourceKeyV2, Sequence, RecordRefV1,
  DigestAlgorithm, DigestDomain, Digest, ExpiresUnixNano

OperationSnapshotPurgeEvidenceConsumptionRefV1 =
  ContractVersion, TypeURL,
  ConsumptionID, Revision=1, TenantID,
  QualificationRefV1, ProviderHandoffRefV1, RecordRefV1,
  ChainRefV1, CursorRefV1,
  DispatchAttemptRefV3, Phase,
  State=consumed_current,
  EvidenceSourceKeyV2, ScopeDigest,
  DigestAlgorithm, DigestDomain, Digest, ExpiresUnixNano

OperationSnapshotPurgeEvidenceQualificationConsumedRefV1 =
  ContractVersion, TypeURL,
  ConsumedID, Revision=1, TenantID,
  QualificationRefV1, RecordRefV1, ChainRefV1, CursorRefV1,
  ConsumptionRefV1, State=consumed_current,
  DigestAlgorithm, DigestDomain, Digest, ExpiresUnixNano

OperationSnapshotPurgeEvidenceRecordConsumptionCommitV1 =
  ContractVersion, TypeURL,
  CommitID, Revision=1, TenantID, EvidenceOwnerRef,
  QualificationCurrentV1, ProviderHandoffCurrentV1,
  RecordRefV1, ChainRefV1, CursorRefV1,
  ConsumptionRefV1, QualificationConsumedRefV1,
  CandidateDigest, ScopeDigest,
  CommittedUnixNano, ExpiresUnixNano,
  DigestAlgorithm, DigestDomain, Digest

OperationSnapshotPurgeEvidenceRecordConsumptionCurrentV1 =
  ContractVersion, TypeURL,
  CommitV1, Current=true,
  CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm, DigestDomain, Digest

OperationSnapshotPurgeEvidenceBindingSetV1 =
  ContractVersion, TypeURL,
  OperationDigest, EffectID, DispatchAttemptRefV3,
  Prepare(RecordConsumptionCommitV1, PhaseEnforcementRefV4),
  Execute(RecordConsumptionCommitV1, PhaseEnforcementRefV4),
  ExpiresUnixNano,
  DigestAlgorithm, DigestDomain, BindingSetDigest
```

每个Qualification只绑定一个`OperationDispatchAttemptRefV3 + phase`；prepare/execute必须使用同一
OperationDigest/EffectID/IntentRevision/Permit/Attempt/BindingSetDigest，但Qualification、Handoff、
Record、Consumption和Enforcement Ref均独立。prepare对象不得填入execute槽，execute
Enforcement必须精确引用prepare receipt closure。Record必须绑定Qualification、Handoff、Candidate
digest、Source与Ledger exact sequence；同source同sequence换digest为EvidenceConflict。

`Record→Consume`不是两个可分离的公共写步骤。Runtime Evidence Owner必须在**同一原子提交**内：
追加Record、推进Chain head、推进Source Cursor、写Consumption、写Qualification-consumed terminal marker，
并把这些对象封入一个Commit；任一对象已写而其他对象未知时只能返回Indeterminate，Settlement资格为0。
同一Qualification只能出现一个consumed marker；同一previous chain/cursor CAS只有一个winner，重放相同
candidate返回同Commit，换candidate/source/sequence/phase为Conflict。Settlement只能接收prepare和execute
两个fresh `RecordConsumptionCommitV1`；只有Record或Observation均不满足资格。

所有public Port签名闭表如下，Request/Result均为值对象，typed-nil拒绝：

```text
OperationSnapshotPurgeEvidenceGovernancePortV1 {
IssueSnapshotPurgeEvidenceQualificationRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/issue-snapshot-purge-evidence-qualification-request/v1",
  RequestID, TenantID, ScopeV1,
  ExpectedQualificationPresence, ExpectedQualificationRefV1,
  RequestedNotAfter,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/issue-snapshot-purge-evidence-qualification-request/body/v1",
  Digest
IssueSnapshotPurgeEvidenceQualificationResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/issue-snapshot-purge-evidence-qualification-result/v1",
  RequestID, RequestDigest, QualificationCurrentV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/issue-snapshot-purge-evidence-qualification-result/body/v1",
  Digest
IssueSnapshotPurgeEvidenceQualificationV1(
  IssueSnapshotPurgeEvidenceQualificationRequestV1,
) -> IssueSnapshotPurgeEvidenceQualificationResultV1

InspectSnapshotPurgeEvidenceQualificationHistoricalRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-qualification-historical-request/v1",
  RequestID, TenantID, QualificationRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-qualification-historical-request/body/v1",
  Digest
InspectSnapshotPurgeEvidenceQualificationHistoricalResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-qualification-historical-result/v1",
  RequestID, RequestDigest, QualificationRefV1, ScopeDigest, Historical=true,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-qualification-historical-result/body/v1",
  Digest
InspectSnapshotPurgeEvidenceQualificationHistoricalV1(
  InspectSnapshotPurgeEvidenceQualificationHistoricalRequestV1,
) -> InspectSnapshotPurgeEvidenceQualificationHistoricalResultV1

InspectSnapshotPurgeEvidenceQualificationCurrentRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-qualification-current-request/v1",
  RequestID, TenantID, QualificationID, ExpectedQualificationRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-qualification-current-request/body/v1",
  Digest
InspectSnapshotPurgeEvidenceQualificationCurrentResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-qualification-current-result/v1",
  RequestID, RequestDigest, QualificationCurrentV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-qualification-current-result/body/v1",
  Digest
InspectSnapshotPurgeEvidenceQualificationCurrentV1(
  InspectSnapshotPurgeEvidenceQualificationCurrentRequestV1,
) -> InspectSnapshotPurgeEvidenceQualificationCurrentResultV1

CreateSnapshotPurgeEvidenceProviderHandoffRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/create-snapshot-purge-evidence-provider-handoff-request/v1",
  RequestID, TenantID, QualificationCurrentV1,
  ProviderBindingRefV2, DispatchAttemptRefV3,
  PhaseEnforcementRefV4, RequestedNotAfter,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/create-snapshot-purge-evidence-provider-handoff-request/body/v1",
  Digest
CreateSnapshotPurgeEvidenceProviderHandoffResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/create-snapshot-purge-evidence-provider-handoff-result/v1",
  RequestID, RequestDigest, ProviderHandoffCurrentV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/create-snapshot-purge-evidence-provider-handoff-result/body/v1",
  Digest
CreateSnapshotPurgeEvidenceProviderHandoffV1(
  CreateSnapshotPurgeEvidenceProviderHandoffRequestV1,
) -> CreateSnapshotPurgeEvidenceProviderHandoffResultV1

InspectSnapshotPurgeEvidenceProviderHandoffHistoricalRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-provider-handoff-historical-request/v1",
  RequestID, TenantID, ProviderHandoffRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-provider-handoff-historical-request/body/v1",
  Digest
InspectSnapshotPurgeEvidenceProviderHandoffHistoricalResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-provider-handoff-historical-result/v1",
  RequestID, RequestDigest, ProviderHandoffRefV1, Historical=true,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-provider-handoff-historical-result/body/v1",
  Digest
InspectSnapshotPurgeEvidenceProviderHandoffHistoricalV1(
  InspectSnapshotPurgeEvidenceProviderHandoffHistoricalRequestV1,
) -> InspectSnapshotPurgeEvidenceProviderHandoffHistoricalResultV1

InspectSnapshotPurgeEvidenceProviderHandoffCurrentRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-provider-handoff-current-request/v1",
  RequestID, TenantID, HandoffID, ExpectedProviderHandoffRefV1,
  ExpectedQualificationRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-provider-handoff-current-request/body/v1",
  Digest
InspectSnapshotPurgeEvidenceProviderHandoffCurrentResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-provider-handoff-current-result/v1",
  RequestID, RequestDigest, ProviderHandoffCurrentV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-provider-handoff-current-result/body/v1",
  Digest
InspectSnapshotPurgeEvidenceProviderHandoffCurrentV1(
  InspectSnapshotPurgeEvidenceProviderHandoffCurrentRequestV1,
) -> InspectSnapshotPurgeEvidenceProviderHandoffCurrentResultV1

RecordAndConsumeSnapshotPurgeEvidenceRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/record-and-consume-snapshot-purge-evidence-request/v1",
  RequestID, TenantID,
  QualificationCurrentV1, ProviderHandoffCurrentV1,
  Candidate, CandidateDigest, EvidenceSourceKeyV2,
  ExpectedChainRefV1, ExpectedCursorRefV1,
  ConsumptionID, RequestedNotAfter,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/record-and-consume-snapshot-purge-evidence-request/body/v1",
  Digest
RecordAndConsumeSnapshotPurgeEvidenceResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/record-and-consume-snapshot-purge-evidence-result/v1",
  RequestID, RequestDigest, RecordConsumptionCommitV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/record-and-consume-snapshot-purge-evidence-result/body/v1",
  Digest
RecordAndConsumeSnapshotPurgeEvidenceV1(
  RecordAndConsumeSnapshotPurgeEvidenceRequestV1,
) -> RecordAndConsumeSnapshotPurgeEvidenceResultV1

InspectSnapshotPurgeEvidenceRecordConsumptionHistoricalRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-record-consumption-historical-request/v1",
  RequestID, TenantID, CommitID, Revision, CommitDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-record-consumption-historical-request/body/v1",
  Digest
InspectSnapshotPurgeEvidenceRecordConsumptionHistoricalResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-record-consumption-historical-result/v1",
  RequestID, RequestDigest, RecordConsumptionCommitV1, Historical=true,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-record-consumption-historical-result/body/v1",
  Digest
InspectSnapshotPurgeEvidenceRecordConsumptionHistoricalV1(
  InspectSnapshotPurgeEvidenceRecordConsumptionHistoricalRequestV1,
) -> InspectSnapshotPurgeEvidenceRecordConsumptionHistoricalResultV1

InspectSnapshotPurgeEvidenceRecordConsumptionCurrentRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-record-consumption-current-request/v1",
  RequestID, TenantID, CommitID, ExpectedCommitDigest,
  ExpectedQualificationRefV1, ExpectedHandoffRefV1,
  ExpectedRecordRefV1, ExpectedChainRefV1,
  ExpectedCursorRefV1, ExpectedConsumptionRefV1,
  ExpectedQualificationConsumedRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-record-consumption-current-request/body/v1",
  Digest
InspectSnapshotPurgeEvidenceRecordConsumptionCurrentResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-evidence-record-consumption-current-result/v1",
  RequestID, RequestDigest, RecordConsumptionCurrentV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-evidence-record-consumption-current-result/body/v1",
  Digest
InspectSnapshotPurgeEvidenceRecordConsumptionCurrentV1(
  InspectSnapshotPurgeEvidenceRecordConsumptionCurrentRequestV1,
) -> InspectSnapshotPurgeEvidenceRecordConsumptionCurrentResultV1
}
```

上述18个命名Request/Result body均覆盖ContractVersion、TypeURL、RequestID、全部业务字段、
presence、RequestDigest（仅Result）、DigestAlgorithm和DigestDomain，排除own Digest。Result的
`RequestID+RequestDigest`必须exact绑定原Request；typed-nil、required presence/value矛盾、unknown
variant、错TypeURL/Version/Domain、重放同RequestID换Digest或Result绑定另一Request一律拒绝并返回零
Result。该identity是Issue/Handoff/RecordAndConsume及所有Inspect lost reply唯一重放坐标。

Issue lost reply只按`QualificationID + OperationDigest + EffectID + DispatchAttempt + Phase +
BindingSetDigest` Inspect原Qualification；Handoff lost reply只Inspect原Handoff ID与Qualification exact
ref；Record/Consume lost reply只按`CommitID + Qualification + Handoff + CandidateDigest +
LedgerScope + Source epoch/sequence + ExpectedChain + ExpectedCursor` Inspect原Commit。Owner NotFound只表示
该精确write key在linearizable Owner Store未提交，不证明Provider未执行；只有全部坐标和candidate
digest相同才可重放同一Owner create-once。Unknown/Unavailable保持原Attempt/phase，禁止换Qualification、
sequence、Attempt、phase、Provider或把prepare Commit复用为execute。

### 10.4 Runtime additive sibling Port Delta

Runtime候选新增下列**同版本旁路合同**，不得修改现有Checkpoint Reader方法集。Runtime public
DTO只能引用Runtime-owned neutral类型，禁止导入或复刻任何Sandbox nominal type：

```text
OperationDomainFactExactRefV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-domain-fact-exact-ref/v1",
  Owner(core.OwnerRef), Kind(NamespacedNameV2),
  ID, Revision, Digest, TenantID,
  Schema(SchemaRefV2), ExpiresUnixNano

OperationSnapshotPurgeBindingSetV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-binding-set/v1",
  TenantID,
  Request(OperationDomainFactExactRefV1),
  Subject(OperationDomainFactExactRefV1),
  ExpectedAggregate(OperationDomainFactExactRefV1),
  DomainAttempt(OperationDomainFactExactRefV1),
  ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-binding-set/body/v1",
  BindingSetDigest

OperationSnapshotPurgeSettlementRefV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-settlement-ref/v5",
  SettlementID, Revision=1, TenantID, EffectID,
  BindingSetDigest, OperationDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-settlement-ref/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeSettlementSubmissionV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-settlement-submission/v5",
  SettlementID, OperationSubjectV3, OperationDigest,
  EffectID, ExpectedEffectRevision,
  OperationSnapshotPurgeBindingSetV1,
  OperationSettlementDomainResultFactRefV4,
  OperationSnapshotPurgeEvidenceBindingSetV1,
  DispatchAttemptRefV3,
  ExecuteEnforcementRefV4, SettlementOwnerRef,
  SettledUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-settlement-submission/body/v5",
  Digest

OperationSnapshotPurgeSettlementAssociationV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-settlement-association/v5",
  AssociationID, Revision=1, SettlementRefV5,
  OperationDigest, EffectID, BindingSetDigest,
  DomainResultFactRefV4, EvidenceBindingSetDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-settlement-association/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeSettlementTerminalGuardV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-settlement-terminal-guard/v5",
  GuardID, Revision=1, SettlementRefV5,
  OperationDigest, EffectID, ExpectedEffectRevision,
  BindingSetDigest, AssociationDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-settlement-terminal-guard/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeSettlementTerminalProjectionV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-settlement-terminal-projection/v5",
  ProjectionID, Revision=1, SettlementRefV5,
  OperationDigest, EffectID, TerminalGuardRef,
  Current=true, ProjectedUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-settlement-terminal-projection/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeSettlementEffectTerminalV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-settlement-effect-terminal/v5",
  EffectTerminalID, Revision=1, SettlementRefV5,
  OperationDigest, EffectID, TerminalGuardRef,
  Closed=true, ClosedUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-settlement-effect-terminal/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeSettlementCommitBundleV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-settlement-commit-bundle/v5",
  SubmissionV5, SettlementRefV5, AssociationV5,
  TerminalGuardV5, TerminalProjectionV5, EffectTerminalV5,
  CommittedUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-settlement-commit-bundle/body/v5",
  Digest

InspectCurrentOperationSnapshotPurgeSettlementRequestV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/inspect-current-operation-snapshot-purge-settlement-request/v5",
  TenantID, OperationSubjectV3, OperationDigest,
  EffectID, ExpectedBindingSetDigest,
  ExpectedDomainAttempt(OperationDomainFactExactRefV1),
  ExpectedSettlementRefPresence, ExpectedSettlementRefV5,
  RequestedNotAfter, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-current-operation-snapshot-purge-settlement-request/body/v5",
  Digest

OperationSnapshotPurgeSettlementInspectionV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-settlement-inspection/v5",
  CommitBundleV5, Current=true,
  CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-settlement-inspection/body/v5",
  Digest
```

Settlement Ref保持opaque，不含`Disposition`、Outcome或任何授权位。CommitBundle必须原子绑定同一
tenant/OperationDigest/EffectID/neutral BindingSet/DomainResult/Evidence/dispatch execute
Enforcement/Settlement Owner；Association、TerminalGuard、TerminalProjection和EffectTerminal必须
引用唯一同一Settlement exact ref，CommitBundle内不得重复第二个同名SettlementRef字段。上述每个
body覆盖全部嵌套对象、presence与TTL并排除own Digest；CommitBundle
`ExpiresUnixNano <= min(Submission, SettlementRef, BindingSet, DomainResult, prepare/execute Evidence,
execute Enforcement, Association, Guard, Projection, EffectTerminal)`，Inspection再取CommitBundle与
Runtime current pointer最短TTL，`now >= expiry`拒绝。候选Reader与wiring marker固定为：

```text
OperationSnapshotPurgeSettlementCurrentReaderV5 {
  InspectSnapshotPurgeSettlementCurrentV5(
    InspectCurrentOperationSnapshotPurgeSettlementRequestV5,
  ) -> OperationSnapshotPurgeSettlementInspectionV5
}

OperationSnapshotPurgeSettlementCurrentReaderProviderV5 {
  OperationSnapshotPurgeSettlementCurrentReaderV5
  GatewayBackedOperationSnapshotPurgeSettlementCurrentReaderV5()
}
```

Reader只有这一项current Inspect能力，不暴露Settle、historical Fact Store、Commit、Provider或
Apply；raw Runtime Fact Port和普通one-method Reader均不能满足production wiring marker。

无损映射只允许位于未来Sandbox `runtimeadapter`：

| Sandbox Owner exact ref | Runtime neutral字段 | 映射不变量 |
|---|---|---|
| `SnapshotArtifactPurgeRequestV1` exact对象 | `BindingSet.Request` | Owner/Kind/RequestID/Revision/Digest/Tenant/Schema/Expires逐字段等值；ID=RequestID |
| `SnapshotArtifactSubjectRefV2` versioned exact/current对象 | `BindingSet.Subject` | Owner/Kind/ArtifactAggregateID/Revision/SubjectDigest/Tenant/Schema/Expires逐字段等值；ID=ArtifactAggregateID；StableSubjectDigest只校验identity，不映射为TTL |
| `SnapshotArtifactAggregateRefV2` expected exact对象 | `BindingSet.ExpectedAggregate` | Owner/Kind/AggregateID/Revision/Digest/Tenant/Schema/Expires逐字段等值；ID=AggregateID |
| `SnapshotArtifactDeletionAttemptRefV2` exact对象 | `BindingSet.DomainAttempt` | Owner/Kind/AttemptID/Revision/Digest/Tenant/Schema/Expires逐字段等值；ID=AttemptID |
| Sandbox DomainResult exact ref | `OperationSettlementDomainResultFactRefV4` | 通过既有Runtime neutral DTO表达Owner/Kind/Operation/Effect/Attempt/Schema/Payload，不新增Sandbox字段 |

四个Sandbox source exact对象都由`SnapshotArtifactOwnerV2`真实seal Owner/Kind/Revision/Schema/
Expires；Runtime Adapter只能复制，不能计算、默认填充、从type name推导或延长。BindingSet
`ExpiresUnixNano=min(Request,Subject,ExpectedAggregate,DomainAttempt)`；`now >= expiry`拒绝。
这里的`Subject`只指versioned `SnapshotArtifactSubjectRefV2`；无TTL的
`SnapshotArtifactSubjectIdentityV2`不进入BindingSet，也不得被Adapter伪造成带TTL的neutral ref。
`BindingSetDigest`覆盖四个neutral ref、各自role discriminator、TenantID与TTL；不同Sandbox输入不能
映射为同一BindingSet。adapter只能执行`Sandbox exact source -> Runtime neutral DTO`单向构造，并在
构造时逐字段等值校验；不得提供neutral→source反向映射、round-trip重建或双向转换接口。任一source
字段不存在、丢失、默认填充、role交换、digest或TTL漂移均Conflict。依赖DAG固定为：

```text
runtime/core <- runtime/ports <- Runtime Gateway
       ^             ^
       |             |
sandbox/contract <- sandbox/runtimeadapter
```

唯一跨边为`sandbox/runtimeadapter -> runtime/core|ports`；`runtime/core|ports|gateway`均不得导入
Sandbox，Sandbox contract/kernel也不得导入Runtime。禁止Runtime→Sandbox反向依赖、复制Sandbox DTO、
双向adapter或任何SCC。Runtime合同未落地前，Sandbox不得建立私有兼容interface或以Checkpoint V5
Reader代替。

### 10.5 closed error集合

Runtime sibling Gateway对外只允许以下`Category/Reason`组合；所有错误返回零Inspection，且Reader
调用期间`Settle/Commit/Provider/Apply=0`。未列出的backend错误必须在Runtime Gateway边界归一为
`internal/execution_inspection_invalid`，不得原样泄漏开放字符串、secret或Provider payload。

| Category | 允许Reason | 语义 |
|---|---|---|
| `invalid_argument` | `invalid_reference`、`invalid_digest`、`invalid_state` | request或返回bundle Shape/canonical错误 |
| `not_found` | `effect_settlement_missing` | 精确Operation+Effect+Request+Attempt尚无current Settlement；不授重派权 |
| `conflict` | `revision_conflict`、`effect_state_conflict`、`settlement_owner_mismatch` | tenant/scope/Request/Attempt/Effect/closure或current pointer漂移 |
| `capability_unavailable` | `component_missing`、`unknown_capability` | typed-nil、无Gateway facade、合同未落地或能力未装配 |
| `indeterminate` | `effect_unknown_outcome`、`effect_settlement_missing` | lost reply后无法证明current或terminal closure |
| `unavailable` | `evidence_unavailable` | current Fact Owner暂不可读；fail closed |
| `internal` | `execution_inspection_invalid` | 未知backend错误归一后的安全失败 |

### 10.6 顺序、lost reply与Apply边界

```text
ReserveDeletion
  -> Inspect Artifact/Aggregate/Retention Index + NoHold S1 current
  -> Admission -> Review/Auth -> Permit -> Begin
  -> persist prepare Enforcement
  -> Issue prepare Qualification -> create prepare Handoff
  -> Provider Prepare -> Record/Consume prepare Evidence current
  -> reread all current + NoHold S2
  -> persist execute Enforcement
  -> Issue execute Qualification -> create execute Handoff
  -> ExecutePrepared -> Record/Consume execute Evidence current
  -> Sandbox DomainResult CAS
  -> Runtime SettleSnapshotPurgeV5
  -> InspectSnapshotPurgeSettlementCurrentV5
  -> Sandbox ApplySettlement CAS -> terminal Tombstone/CurrentIndex
```

`Begin`不授执行权。Issue/Begin、Enforcement、Provider Prepare/Execute、Evidence、DomainResult、
Settlement和Sandbox Apply任一步丢回包，都只Inspect原Operation/Effect/Attempt和expected successor；
Provider NotFound、进程死亡、宿主漂移或超时不恢复自动重派权。只有Runtime sibling Reader返回完整、
exact、current CommitBundle，Sandbox才可验证其与DomainResult/DeletionAttempt绑定并尝试Apply；Sandbox
不依据本地Disposition推进Runtime事实。Apply回包丢失只InspectCurrentIndex/expected terminal
successor，禁止再purge或制造ABA。

### 10.7 独立审计矩阵

| 类别 | 正例 | 必须拒绝/故障注入 | 必须结果 |
|---|---|---|---|
| live兼容 | Checkpoint Reader仍恰好一方法 | 给它添加Snapshot方法或用Checkpoint DTO承载purge | additive sibling；原Reader零变化 |
| request/result | request→独立purge Effect→DomainResult→Settlement→Apply→deleted | request accepted/Begin/Receipt/NotFound直接deleted | terminal零写，Provider不被重复调用 |
| Effect闭表 | purge和cleanup各自完整治理链 | cleanup复用purge Effect/Attempt/Permit/Settlement；cleanup成功推进deleted | Conflict/unsupported；状态不动 |
| Retention Index | exact current index+watermark | generation错配、same revision换digest、TTL到期 | fail closed，purge Provider=0 |
| Carry | 同代absent；跨代连续exact链 | 缺代、断链、端点/coverage/jurisdiction/hold-kind漂移 | S2拒绝，DomainResult/Settlement/Apply=0 |
| no-hold | natural sealed S1/S2且S2单调NoActive | nil/empty/NotFound、caller bound入digest、S2 active/回退 | fail closed，旧proof不重封 |
| Reader wiring | Gateway marker provider返回exact current bundle | typed-nil、raw Fact Port、plain Reader、恶意tenant/scope/nested ref | closed error+零Inspection；零副作用 |
| Reader error | closed error逐项注入 | backend任意error/message/secret | 归一internal；不泄漏、不写、不调用Provider |
| lost reply | 每层Inspect原Attempt/expected successor | 新ID、新Attempt、换Provider、盲重放purge | 原attempt收敛；Unknown不重派 |
| CAS/no-ABA | 64路同ExpectedAggregate单winner | stale ref、双terminal、old revision reopen | 单一deleted或indeterminate terminal；history append-only |
| TTL/current | S1/S2/Settlement/Apply均fresh | `now==expires`、host/lease/fence/scope/budget/credential/provider drift | Provider=0或Apply=0，旧事实仅historical Inspect |
| import/boundary | Sandbox仅引用未来Runtime public `core/ports` | 复制V5 DTO、导入kernel/fakes/internal、写Runtime/Management | 静态扫描拒绝 |

### 10.8 purge cleanup独立闭包

cleanup固定为独立custom Operation
`praxis.sandbox/snapshot-artifact-purge-cleanup-operation`、Effect
`praxis.sandbox/snapshot-artifact-purge-cleanup`、Conflict Domain
`praxis.sandbox/snapshot-artifact-purge-cleanup`。它使用全新的OperationSubject、EffectID、Intent、
Admission、Review/Auth、Budget、Permit、DispatchAttempt、prepare/execute Enforcement与Attempt；不得
复用purge对象，也不得成为purge Settlement的隐含子步骤。

Sandbox Owner为neutral映射提供下列真实exact来源；这不是Runtime DTO，也不授执行权：

```text
SnapshotArtifactResidualSetExactRefV2 =
  TypeURL="praxis.sandbox/snapshot-artifact-residual-set-ref/v2", Version=2,
  ResidualSetID, Revision, TenantID,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.sandbox/snapshot-artifact-residual-set/body/v2",
  Digest, ExpiresUnixNano

SnapshotArtifactPurgeDomainResultExactRefV1 =
  TypeURL="praxis.sandbox/snapshot-artifact-purge-domain-result-ref/v1", Version=1,
  FactID, Revision, TenantID,
  OperationDigest, EffectID, DeletionAttemptRefV2,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.sandbox/snapshot-artifact-purge-domain-result/body/v1",
  Digest, ExpiresUnixNano

SnapshotArtifactPurgeCleanupRequestV1 =
  TypeURL="praxis.sandbox/snapshot-artifact-purge-cleanup-request/v1", Version=1,
  Owner=SnapshotArtifactOwnerV2(core.OwnerRef),
  Kind="praxis.sandbox/snapshot-artifact-purge-cleanup-request",
  RequestID, Revision=1, TenantID,
  Schema(SchemaRefV2="praxis.sandbox/snapshot-artifact-purge-cleanup-request/v1"),
  ArtifactSubjectRefV2, ExpectedCleanupAggregateRef(SnapshotArtifactAggregateRefV2),
  ExpectedResidualSetRef(SnapshotArtifactResidualSetExactRefV2),
  ParentPurgeDomainResultRef(SnapshotArtifactPurgeDomainResultExactRefV1),
  RequestedNotAfter,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.sandbox/snapshot-artifact-purge-cleanup-request/body/v1",
  Digest, ExpiresUnixNano

SnapshotArtifactPurgeCleanupAttemptRefV1 =
  TypeURL="praxis.sandbox/snapshot-artifact-purge-cleanup-attempt-ref/v1", Version=1,
  Owner=SnapshotArtifactOwnerV2(core.OwnerRef),
  Kind="praxis.sandbox/snapshot-artifact-purge-cleanup-attempt",
  CleanupAttemptID, Revision, TenantID,
  Schema(SchemaRefV2="praxis.sandbox/snapshot-artifact-purge-cleanup-attempt-ref/v1"),
  CleanupRequestDigest, OperationDigest, EffectID,
  ExpectedCleanupAggregateRef(SnapshotArtifactAggregateRefV2),
  ExpectedResidualSetRef(SnapshotArtifactResidualSetExactRefV2),
  DigestAlgorithm="sha256",
  DigestDomain="praxis.sandbox/snapshot-artifact-purge-cleanup-attempt-ref/body/v1",
  Digest, ExpiresUnixNano
```

四个Sandbox source body都覆盖全部字段和presence、排除own Digest；其ID/Revision/Digest/TTL由
Sandbox SnapshotArtifactOwner seal。Runtime public候选提供与purge**名义分离**的neutral合同族，
且使用10.4的Runtime-owned `OperationDomainFactExactRefV1`逐字段承载Sandbox Owner已seal的exact
source；四个source都真实携Owner/Kind/Revision/Schema/Expires，Adapter只复制、不得伪造：

```text
OperationSnapshotPurgeCleanupNeutralBindingSetV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-binding-set/v1",
  TenantID,
  CleanupRequest(OperationDomainFactExactRefV1),
  ArtifactSubject(OperationDomainFactExactRefV1),
  ExpectedCleanupAggregate(OperationDomainFactExactRefV1),
  CleanupAttempt(OperationDomainFactExactRefV1),
  ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-binding-set/body/v1",
  CleanupBindingSetDigest
```

`ExpiresUnixNano=min(CleanupRequest.Expires, ArtifactSubject.Expires,
ExpectedCleanupAggregate.Expires, CleanupAttempt.Expires, ExpectedResidualSet.Expires,
RequestedNotAfter)`。四个neutral ref逐字段复制Owner/Kind/ID/Revision/Digest/Tenant/Schema/Expires；
Owner/Kind/Schema固定值、ID映射、digest与TTL任一缺失或默认填充均Conflict。

Cleanup Evidence全部对象使用`ContractVersion="1.0.0"`、`DigestAlgorithm="sha256"`，每个body覆盖
下列全部字段和presence并排除自身Digest：

```text
OperationSnapshotPurgeCleanupEvidenceScopeV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-scope/v1",
  OperationSubjectV3, OperationDigest,
  EffectID, EffectRevision,
  EffectKind="praxis.sandbox/snapshot-artifact-purge-cleanup",
  IntentDigest, Admission, ReviewAuthorization,
  DispatchAttemptRefV3, PermitID, PermitFactRevision, PermitDigest,
  AuthorizedAdmissionDigest, Phase, PhaseEnforcementRefV4,
  CleanupBindingSetDigest, Authority, EvidencePolicy,
  PayloadSchema, PayloadDigest, PayloadRevision, PayloadLength,
  EvidenceSourceKeyV2, CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-scope/body/v1",
  Digest

OperationSnapshotPurgeCleanupEvidenceQualificationRefV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-qualification-ref/v1",
  QualificationID, Revision, TenantID,
  OperationDigest, EffectID, EffectRevision,
  DispatchAttemptRefV3, Phase, PhaseEnforcementRefV4,
  CleanupBindingSetDigest, ScopeDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-qualification-ref/body/v1",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupEvidenceQualificationCurrentV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-qualification-current/v1",
  QualificationRefV1, ScopeDigest,
  Current=true, ConsumedPresence, QualificationConsumedRefV1,
  CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-qualification-current/body/v1",
  Digest

OperationSnapshotPurgeCleanupEvidenceProviderHandoffRefV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-provider-handoff-ref/v1",
  HandoffID, Revision=1, TenantID,
  QualificationRefV1, DispatchAttemptRefV3, Phase,
  PhaseEnforcementRefV4, ProviderBindingRefV2, ScopeDigest,
  CreatedUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-provider-handoff-ref/body/v1",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupEvidenceProviderHandoffCurrentV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-provider-handoff-current/v1",
  ProviderHandoffRefV1, QualificationRefV1,
  ProviderBindingRefV2, ScopeDigest,
  Current=true, CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-provider-handoff-current/body/v1",
  Digest

OperationSnapshotPurgeCleanupEvidenceRecordRefV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-record-ref/v1",
  RecordID, Revision=1, TenantID,
  QualificationRefV1, ProviderHandoffRefV1,
  EvidenceRecordRefV2, CandidateDigest, EvidenceSourceKeyV2,
  DispatchAttemptRefV3, Phase, ScopeDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-record-ref/body/v1",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupEvidenceChainRefV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-chain-ref/v1",
  ChainID, Revision, TenantID, LedgerScopeDigest,
  PreviousHeadSequence, PreviousHeadRecordDigest,
  HeadSequence, HeadRecordDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-chain-ref/body/v1",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupEvidenceCursorRefV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-cursor-ref/v1",
  CursorID, Revision, TenantID, LedgerScopeDigest,
  EvidenceSourceKeyV2, Sequence, RecordRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-cursor-ref/body/v1",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupEvidenceConsumptionRefV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-consumption-ref/v1",
  ConsumptionID, Revision=1, TenantID,
  QualificationRefV1, ProviderHandoffRefV1, RecordRefV1,
  ChainRefV1, CursorRefV1,
  DispatchAttemptRefV3, Phase, State=consumed_current,
  EvidenceSourceKeyV2, ScopeDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-consumption-ref/body/v1",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupEvidenceQualificationConsumedRefV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-qualification-consumed-ref/v1",
  ConsumedID, Revision=1, TenantID,
  QualificationRefV1, RecordRefV1, ChainRefV1, CursorRefV1,
  ConsumptionRefV1, State=consumed_current,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-qualification-consumed-ref/body/v1",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupEvidenceRecordConsumptionCommitV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-record-consumption-commit/v1",
  CommitID, Revision=1, TenantID, EvidenceOwnerRef,
  QualificationRefV1, ProviderHandoffCurrentV1,
  RecordRefV1, ChainRefV1, CursorRefV1,
  ConsumptionRefV1, QualificationConsumedRefV1,
  CandidateDigest, ScopeDigest, CommittedUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-record-consumption-commit/body/v1",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupEvidenceBindingSetV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-evidence-binding-set/v1",
  OperationDigest, EffectID, DispatchAttemptRefV3,
  PrepareCommit(OperationSnapshotPurgeCleanupEvidenceRecordConsumptionCommitV1),
  PrepareEnforcementRefV4,
  ExecuteCommit(OperationSnapshotPurgeCleanupEvidenceRecordConsumptionCommitV1),
  ExecuteEnforcementRefV4,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-evidence-binding-set/body/v1",
  Digest, ExpiresUnixNano
```

Handoff TTL不得晚于Qualification、Enforcement、Provider Binding和CleanupBindingSet的最短TTL；
Record/Chain/Cursor/Consumption/QualificationConsumed必须由同一Runtime Evidence Owner在一个CAS中
提交，Commit TTL取这些对象、Candidate和Handoff current最短TTL。Cleanup Evidence public Port闭表
及完整Request/Result为：

```text
OperationSnapshotPurgeCleanupEvidenceGovernancePortV1 {
IssueSnapshotPurgeCleanupEvidenceQualificationRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/issue-snapshot-purge-cleanup-evidence-qualification-request/v1",
  RequestID, TenantID, CleanupEvidenceScopeV1,
  ExpectedQualificationPresence, ExpectedQualificationRefV1,
  RequestedNotAfter,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/issue-snapshot-purge-cleanup-evidence-qualification-request/body/v1",
  Digest
IssueSnapshotPurgeCleanupEvidenceQualificationResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/issue-snapshot-purge-cleanup-evidence-qualification-result/v1",
  RequestID, RequestDigest,
  OperationSnapshotPurgeCleanupEvidenceQualificationCurrentV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/issue-snapshot-purge-cleanup-evidence-qualification-result/body/v1",
  Digest
IssueSnapshotPurgeCleanupEvidenceQualificationV1(
  IssueSnapshotPurgeCleanupEvidenceQualificationRequestV1,
) -> IssueSnapshotPurgeCleanupEvidenceQualificationResultV1

InspectSnapshotPurgeCleanupEvidenceQualificationHistoricalRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-qualification-historical-request/v1",
  RequestID, TenantID, QualificationRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-qualification-historical-request/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceQualificationHistoricalResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-qualification-historical-result/v1",
  RequestID, RequestDigest, QualificationRefV1, Historical=true,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-qualification-historical-result/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceQualificationHistoricalV1(
  InspectSnapshotPurgeCleanupEvidenceQualificationHistoricalRequestV1,
) -> InspectSnapshotPurgeCleanupEvidenceQualificationHistoricalResultV1

InspectSnapshotPurgeCleanupEvidenceQualificationCurrentRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-qualification-current-request/v1",
  RequestID, TenantID, QualificationID, ExpectedQualificationRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-qualification-current-request/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceQualificationCurrentResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-qualification-current-result/v1",
  RequestID, RequestDigest,
  OperationSnapshotPurgeCleanupEvidenceQualificationCurrentV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-qualification-current-result/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceQualificationCurrentV1(
  InspectSnapshotPurgeCleanupEvidenceQualificationCurrentRequestV1,
) -> InspectSnapshotPurgeCleanupEvidenceQualificationCurrentResultV1

CreateSnapshotPurgeCleanupEvidenceProviderHandoffRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/create-snapshot-purge-cleanup-evidence-provider-handoff-request/v1",
  RequestID, TenantID,
  OperationSnapshotPurgeCleanupEvidenceQualificationCurrentV1,
  ProviderBindingRefV2, DispatchAttemptRefV3,
  PhaseEnforcementRefV4, RequestedNotAfter,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/create-snapshot-purge-cleanup-evidence-provider-handoff-request/body/v1",
  Digest
CreateSnapshotPurgeCleanupEvidenceProviderHandoffResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/create-snapshot-purge-cleanup-evidence-provider-handoff-result/v1",
  RequestID, RequestDigest,
  OperationSnapshotPurgeCleanupEvidenceProviderHandoffCurrentV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/create-snapshot-purge-cleanup-evidence-provider-handoff-result/body/v1",
  Digest
CreateSnapshotPurgeCleanupEvidenceProviderHandoffV1(
  CreateSnapshotPurgeCleanupEvidenceProviderHandoffRequestV1,
) -> CreateSnapshotPurgeCleanupEvidenceProviderHandoffResultV1

InspectSnapshotPurgeCleanupEvidenceProviderHandoffHistoricalRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-provider-handoff-historical-request/v1",
  RequestID, TenantID, ProviderHandoffRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-provider-handoff-historical-request/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceProviderHandoffHistoricalResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-provider-handoff-historical-result/v1",
  RequestID, RequestDigest, ProviderHandoffRefV1, Historical=true,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-provider-handoff-historical-result/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceProviderHandoffHistoricalV1(
  InspectSnapshotPurgeCleanupEvidenceProviderHandoffHistoricalRequestV1,
) -> InspectSnapshotPurgeCleanupEvidenceProviderHandoffHistoricalResultV1

InspectSnapshotPurgeCleanupEvidenceProviderHandoffCurrentRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-provider-handoff-current-request/v1",
  RequestID, TenantID, HandoffID, ExpectedProviderHandoffRefV1,
  ExpectedQualificationRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-provider-handoff-current-request/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceProviderHandoffCurrentResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-provider-handoff-current-result/v1",
  RequestID, RequestDigest,
  OperationSnapshotPurgeCleanupEvidenceProviderHandoffCurrentV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-provider-handoff-current-result/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceProviderHandoffCurrentV1(
  InspectSnapshotPurgeCleanupEvidenceProviderHandoffCurrentRequestV1,
) -> InspectSnapshotPurgeCleanupEvidenceProviderHandoffCurrentResultV1

RecordAndConsumeSnapshotPurgeCleanupEvidenceRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/record-and-consume-snapshot-purge-cleanup-evidence-request/v1",
  RequestID, TenantID,
  OperationSnapshotPurgeCleanupEvidenceQualificationCurrentV1,
  OperationSnapshotPurgeCleanupEvidenceProviderHandoffCurrentV1,
  Candidate, CandidateDigest, EvidenceSourceKeyV2,
  ExpectedChainRefV1, ExpectedCursorRefV1,
  ConsumptionID, RequestedNotAfter,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/record-and-consume-snapshot-purge-cleanup-evidence-request/body/v1",
  Digest
RecordAndConsumeSnapshotPurgeCleanupEvidenceResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/record-and-consume-snapshot-purge-cleanup-evidence-result/v1",
  RequestID, RequestDigest,
  OperationSnapshotPurgeCleanupEvidenceRecordConsumptionCommitV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/record-and-consume-snapshot-purge-cleanup-evidence-result/body/v1",
  Digest
RecordAndConsumeSnapshotPurgeCleanupEvidenceV1(
  RecordAndConsumeSnapshotPurgeCleanupEvidenceRequestV1,
) -> RecordAndConsumeSnapshotPurgeCleanupEvidenceResultV1

InspectSnapshotPurgeCleanupEvidenceRecordConsumptionHistoricalRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-record-consumption-historical-request/v1",
  RequestID, TenantID, CommitID, Revision, CommitDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-record-consumption-historical-request/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceRecordConsumptionHistoricalResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-record-consumption-historical-result/v1",
  RequestID, RequestDigest,
  OperationSnapshotPurgeCleanupEvidenceRecordConsumptionCommitV1,
  Historical=true,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-record-consumption-historical-result/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceRecordConsumptionHistoricalV1(
  InspectSnapshotPurgeCleanupEvidenceRecordConsumptionHistoricalRequestV1,
) -> InspectSnapshotPurgeCleanupEvidenceRecordConsumptionHistoricalResultV1

InspectSnapshotPurgeCleanupEvidenceRecordConsumptionCurrentRequestV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-record-consumption-current-request/v1",
  RequestID, TenantID, CommitID, ExpectedCommitDigest,
  ExpectedQualificationRefV1, ExpectedProviderHandoffRefV1,
  ExpectedRecordRefV1, ExpectedChainRefV1, ExpectedCursorRefV1,
  ExpectedConsumptionRefV1, ExpectedQualificationConsumedRefV1,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-record-consumption-current-request/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceRecordConsumptionCurrentResultV1 =
  ContractVersion="1.0.0",
  TypeURL="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-record-consumption-current-result/v1",
  RequestID, RequestDigest,
  OperationSnapshotPurgeCleanupEvidenceRecordConsumptionCommitV1,
  Current=true, CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-snapshot-purge-cleanup-evidence-record-consumption-current-result/body/v1",
  Digest
InspectSnapshotPurgeCleanupEvidenceRecordConsumptionCurrentV1(
  InspectSnapshotPurgeCleanupEvidenceRecordConsumptionCurrentRequestV1,
) -> InspectSnapshotPurgeCleanupEvidenceRecordConsumptionCurrentResultV1
}
```

cleanup上述18个Request/Result DTO执行与主Purge相同的canonical规则：body覆盖ContractVersion、
TypeURL、RequestID、全部业务字段、presence、Result的RequestDigest、DigestAlgorithm和DigestDomain，
排除own Digest。Issue Result必须返回已定义的
`OperationSnapshotPurgeCleanupEvidenceQualificationCurrentV1`，不能降级为Qualification Ref。
typed-nil、required presence/value矛盾、nil/empty混淆、unknown variant、错TypeURL/Version/Domain、
同RequestID换Digest或Result绑定另一Request均fail closed并返回零Result。

Runtime cleanup Settlement sibling的全部shape为：

```text
OperationSnapshotPurgeCleanupSettlementRefV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-settlement-ref/v5",
  SettlementID, Revision=1, TenantID, EffectID,
  CleanupBindingSetDigest, OperationDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-settlement-ref/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupSettlementSubmissionV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-settlement-submission/v5",
  SettlementID, OperationSubjectV3, OperationDigest,
  EffectID, ExpectedEffectRevision,
  OperationSnapshotPurgeCleanupNeutralBindingSetV1,
  OperationSettlementDomainResultFactRefV4,
  OperationSnapshotPurgeCleanupEvidenceBindingSetV1,
  DispatchAttemptRefV3, ExecuteEnforcementRefV4,
  SettlementOwnerRef, SettledUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-settlement-submission/body/v5",
  Digest

OperationSnapshotPurgeCleanupSettlementAssociationV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-settlement-association/v5",
  AssociationID, Revision=1, SettlementRefV5,
  OperationDigest, EffectID, CleanupBindingSetDigest,
  DomainResultFactRefV4, CleanupEvidenceBindingSetDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-settlement-association/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupSettlementTerminalGuardV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-settlement-terminal-guard/v5",
  GuardID, Revision=1, SettlementRefV5,
  OperationDigest, EffectID, ExpectedEffectRevision,
  CleanupBindingSetDigest, AssociationDigest,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-settlement-terminal-guard/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupSettlementTerminalProjectionV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-settlement-terminal-projection/v5",
  ProjectionID, Revision=1, SettlementRefV5,
  OperationDigest, EffectID, TerminalGuardRef,
  Current=true, ProjectedUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-settlement-terminal-projection/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupSettlementEffectTerminalV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-settlement-effect-terminal/v5",
  EffectTerminalID, Revision=1, SettlementRefV5,
  OperationDigest, EffectID, TerminalGuardRef,
  Closed=true, ClosedUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-settlement-effect-terminal/body/v5",
  Digest, ExpiresUnixNano

OperationSnapshotPurgeCleanupSettlementCommitBundleV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-settlement-commit-bundle/v5",
  SubmissionV5, SettlementRefV5, AssociationV5,
  TerminalGuardV5, TerminalProjectionV5, EffectTerminalV5,
  CommittedUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-settlement-commit-bundle/body/v5",
  Digest

InspectCurrentOperationSnapshotPurgeCleanupSettlementRequestV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/inspect-current-operation-snapshot-purge-cleanup-settlement-request/v5",
  TenantID, OperationSubjectV3, OperationDigest,
  EffectID, ExpectedCleanupBindingSetDigest,
  ExpectedDomainAttempt(OperationDomainFactExactRefV1),
  ExpectedSettlementRefPresence, ExpectedSettlementRefV5,
  RequestedNotAfter, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/inspect-current-operation-snapshot-purge-cleanup-settlement-request/body/v5",
  Digest

OperationSnapshotPurgeCleanupSettlementInspectionV5 =
  ContractVersion="5.0.0",
  TypeURL="praxis.runtime/operation-snapshot-purge-cleanup-settlement-inspection/v5",
  CommitBundleV5, Current=true,
  CheckedUnixNano, ExpiresUnixNano,
  DigestAlgorithm="sha256",
  DigestDomain="praxis.runtime/operation-snapshot-purge-cleanup-settlement-inspection/body/v5",
  Digest

OperationSnapshotPurgeCleanupSettlementCurrentReaderV5 {
  InspectSnapshotPurgeCleanupSettlementCurrentV5(
    InspectCurrentOperationSnapshotPurgeCleanupSettlementRequestV5,
  ) -> OperationSnapshotPurgeCleanupSettlementInspectionV5
}
OperationSnapshotPurgeCleanupSettlementCurrentReaderProviderV5 {
  OperationSnapshotPurgeCleanupSettlementCurrentReaderV5
  GatewayBackedOperationSnapshotPurgeCleanupSettlementCurrentReaderV5()
}
```

上述每个Settlement body覆盖全部嵌套对象/presence/TTL并排除own Digest；CommitBundle的
`ExpiresUnixNano`不晚于Submission、BindingSet、DomainResult、两阶段Evidence Commit、execute
Enforcement、Association、Guard、Projection、EffectTerminal的最短TTL，Inspection再取Bundle与
Runtime current pointer最短TTL。purge与cleanup TypeURL/DigestDomain/role均禁止互填。cleanup Reader
只有这一项current Inspect，采用10.5 closed errors且所有错误返回零Inspection、零Settle/Commit/
Provider/Apply。

Evidence write key必须包含cleanup Operation/Effect/Attempt/phase/CleanupBindingSetDigest。Issue或
Handoff lost reply只Inspect原exact key；Record/Consume lost reply只Inspect原Commit及其Record、Chain、
Cursor、Qualification-consumed closure；任何purge坐标均Conflict，部分写入永不满足Settlement。

Sandbox Owner侧Apply闭包固定为：

```text
PurgeCleanupDomainResult current
  + Prepare/Execute Evidence consumed current
  + Runtime cleanup Settlement CommitBundle current
  + ExpectedCleanupAggregateRef full exact
  + ExpectedResidualSetRef full exact
  -> ApplyPurgeCleanupSettlement CAS
  -> CleanupFact/ResidualSet/cleanup current index
```

Apply只更新cleanup/Residual维度；不得创建或修改`deleted` Tombstone，不得清除purge
`indeterminate`，也不得推进Runtime Outcome。cleanup Issue/Record/Consume/Settlement/Apply任一步
lost reply只Inspect原cleanup Operation/Effect/Attempt/phase/expected successor；purge的NotFound或
Settlement不能作为cleanup权威输入，cleanup NotFound也不能反向证明purge未执行。

## 11. 本轮review_pending账本

- Sandbox owner-local P0/P1/P2：`0/0/0`。删除语义、Effect拆分、Retention exact候选、Runtime
  sibling Port Delta、closed errors和审计矩阵在Sandbox资产内已闭表。
- 外部P0：`3`。Retention/Legal Hold Owner需冻结并落地Index/Carry/Reader；Runtime Owner需冻结并
  落地Snapshot purge sibling Evidence/Settlement/Reader/Gateway；Management需冻结CurrentIndex/
  Tombstone最终DTO与持久化/续期。以上任一未YES，整体仍`implementation-NO-GO`。
- 独立审计提交态：`review_pending`。不得把字段级候选、轻门通过或既有Checkpoint V5测试当作
  Runtime/Retention公共合同已经实现。

## 12. 第二候选返修账本

- 独立首审输入：`P0=3 / P1=2 / P2=1`，external P0保持`3`；已通过的
  delete request≠deleted、purge唯一物理Effect、Checkpoint V5不扩义均保持不变。
- P0已在Sandbox候选内关闭：Retention全部full exact Ref及history/current/lost-reply；Runtime-owned
  neutral DTO与无损单向映射DAG；Purge Evidence Issue/Record/Consume Owner和Attempt/phase闭包。
- P1已在Sandbox候选内关闭：Retention四Reader逐方法closed errors/NotFound/Unknown；cleanup独立
  Operation/Evidence/Settlement/current Reader/Apply闭包。
- P2已关闭：验收编号改为唯一连续`SA01..SA19`，同步plan/test matrix/memory且不回改旧memory。
- 第二审提交态：Sandbox owner-local `P0/P1/P2=0/0/0`；external `P0=3`；
  `review_pending / implementation-NO-GO`。

## 13. 第三候选返修账本

- 第二候选复审输入：`P0=2 / P1=1`；Retention refs/errors与`SA01..SA19`编号已闭合且本轮不回退。
- P0-1已关闭：`SnapshotArtifactOwnerV2`为Request/Subject/Aggregate/Attempt exact源真实seal
  Owner/Kind/Revision/Digest/Tenant/Schema/Expires；Owner/Kind/Schema固定、Revision单调、TTL取Owner
  current closure最短值。Runtime neutral DTO逐字段承载这些真实值，Adapter不得推导、默认填充、
  重封或延长TTL。
- P0-2已关闭：Purge Evidence全部Ref固定ContractVersion/TypeURL/DigestDomain/TTL；Handoff具有
  exact-current最短TTL；所有public Port具有Request/Result shape；Record、Chain、Cursor、Consumption
  与Qualification-consumed由Runtime Evidence Owner在一个原子CAS内提交，部分写入没有Settlement资格。
- P1已关闭：cleanup ProviderHandoff、neutral CleanupBindingSet、两阶段EvidenceBindingSet、Settlement
  Submission/Association/Guard/Projection/EffectTerminal/CommitBundle/Inspection均有完整字段、canonical
  domain、presence和TTL；purge/cleanup互填、TTL延长或占位对象全部fail closed。
- 第三审提交态：Sandbox owner-local `P0/P1/P2=0/0/0`；external `P0=3`不变；提交Runtime与Harness
  facts双审。`review_pending / implementation-NO-GO`，未授权任何Go或public Runtime/Retention/
  Management实现。

## 14. 第四候选返修账本

- 第三候选第二独立审输入：owner-local `P0=1 / P1=2 / P2=2`；external `P0=3`不变。
- P0已关闭：主Purge Settlement Ref、Submission、Association、TerminalGuard、TerminalProjection、
  EffectTerminal、CommitBundle、InspectRequest、Inspection全部补齐ContractVersion、TypeURL、
  DigestAlgorithm/Domain、canonical、完整嵌套字段与Expires；其shape和TTL不再比cleanup sibling缩水。
- P1已关闭：主Purge与cleanup Evidence各自使用18个命名Request/Result DTO；Result通过
  `RequestID+RequestDigest`绑定原Request，cleanup Issue Result返回已定义QualificationCurrent；
  typed-nil、presence/value、TypeURL/Version/Domain或request identity错误全部返回零Result。
- P2已关闭：主Purge CommitBundle只含一个唯一SettlementRef字段；adapter只允许Sandbox
  source→Runtime neutral单向构造并逐字段等值校验，不提供neutral→source；两条memory trailing
  whitespace已清理。
- 第四审提交态：Sandbox owner-local候选`P0/P1/P2=0/0/0`，等待Harness facts双审；
  `review_pending / implementation-NO-GO`，无Go或跨Owner实现授权。

## 15. 第五候选返修账本

- 第四候选第一独立审输入：owner-local `P0=1 / P1=1`；第三审指定项保持闭合，external
  `P0=3`不变。
- P0已关闭：`SnapshotArtifactPurgeRequestV1`不再携`DeletionAttemptRefV2`，Deletion Reservation
  也不得反向引用Request/Attempt；创建链固定为Reservation→Request→Attempt，只有Attempt单向绑定
  `PurgeRequestDigest`。Request/Attempt lost reply分别按自身稳定identity Inspect，禁止回填或重封Request。
- P1已关闭：stable `SnapshotArtifactSubjectIdentityV2`不含Revision/TTL/current语义；versioned
  `SnapshotArtifactSubjectRefV2`独立携Revision、Schema、StableSubjectDigest、SubjectDigest与Expires。
  BindingSet只使用versioned exact/current SubjectRef及其TTL，stable identity不进入TTL min，也不得由
  Adapter伪造成neutral current ref。
- 第五审提交态：Sandbox owner-local候选`P0/P1/P2=0/0/0`，external `P0=3`不变；
  `review_pending / implementation-NO-GO`，等待独立审计，不授权Go或跨Owner实现。
