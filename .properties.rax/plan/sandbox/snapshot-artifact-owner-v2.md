# SnapshotArtifactOwnerV2 联合审查与实施计划

> 2026-07-18 current truth：用户已授权`Workspace Snapshot + Host Local`首切面。
> 立即实施Reservation之后的immutable Artifact Fact、aggregate rev2/current CAS、Owner
> current/historical Reader与Host Local加密Content Store；复用现有Checkpoint-first V2
> 公共链。Workspace Restore已实现；Purge/Delete terminal与production certification继续NO-GO。

## 当前实施波次：Workspace capture

1. 冻结并测试`SnapshotStorageArtifactRefV2`与`SnapshotArtifactFactV2`canonical/exact binding；
2. Owner内部以expected current exact ref原子提交Artifact Fact、Entry、Envelope rev2和CurrentIndex rev2；
3. lost reply只Inspect原Reservation/Aggregate，same key换内容Conflict，64路CAS单赢家且无ABA；
4. Host Local只保存加密内容并返回opaque storage ref，不在DTO暴露path/key/credential；
5. Participant通过现有Application/Runtime Checkpoint链发布Snapshot/Coverage exact ref，
   Continuity只聚合Manifest/Seal；Partial只诊断；
6. Restore已按新Instance/高Epoch/新Lease、Workspace Stage + Context Refresh边界实现。

Checkpoint capture当前不需要新增Runtime schema；若实现发现live exact ref无法表达字段，停止并
只提交additive Port Delta，不复制共享类型、不改V3/V4/V5闭表。

状态：`workspace-capture + Workspace Coverage/Participant + evidence/settlement + restore implemented / terminal purge external-NO-GO`。

2026-07-18接线增量：Sandbox自有
`GovernedCheckpointParticipantApplicationAdapterV1`已经强制
`prepare/capture -> Owner S1 -> commit -> Owner S2`；prepare/commit unknown只Inspect原
Attempt/Participant，commit不得替换prepared closure。Runtime Checkpoint capture合同无需新增schema
Delta；Workspace Coverage/Participant Owner Fact、Snapshot Aggregate exact-current闭包与Application
Owner-current Adapter、Evidence V1/Settlement V5 Lifecycle映射与Restore已经实现并通过高重复门。

对应设计：`../../design/sandbox/snapshot-artifact-owner-v2.md`。

## 1. 当前裁决

Owner方向、零Provider/零Runtime写边界已确认；Sandbox owner-local DTO、状态机、stable key、
aggregate CAS与Snapshot purge Delta已经形成字段级候选。Retention/Legal Hold公共Index/Carry/
Reader、Runtime Settlement V5 additive sibling以及Management最终DTO仍待各Owner联合冻结并落地。
本计划的Workspace capture owner-local部分已经落地；以下Retention/Purge审查顺序继续作为
未实施历史计划，不得被解释为已解锁。

## 2. 依赖与联合Review顺序

1. Retention/Legal Hold治理域确认并落地Index ExpectedCurrent、NoActive、Carry、From/To Index全部
   full exact Ref，以及逐方法closed errors/current/history/lost-reply已闭合的四项Reader；
2. Sandbox Owner确认Artifact-local Retention Application、Deletion、history/current aggregate；
3. Runtime Owner确认并落地Snapshot purge专用Evidence Issue/Handoff/atomic RecordAndConsume与
   Settlement V5，公共DTO只使用Runtime-owned neutral refs；Sandbox Request/Subject/Aggregate/Attempt
   exact源真实携Owner/Kind/Revision/Schema/Expires，runtimeadapter只做逐字段等值映射且DAG无SCC；
   `OperationSnapshotPurgeSettlementCurrentReaderV5`与Gateway-backed provider marker；现有
   `OperationSettlementCurrentReaderV5`保持Checkpoint-only一方法不变；
4. Runtime同时冻结purge cleanup独立Operation/Evidence/Settlement/current Reader，Sandbox冻结只
   更新CleanupFact/Residual的Apply闭包；三方冻结stable Subject identity与versioned exact/current
   SubjectRef分层、SubjectRef→FactRef→EntryRef→EnvelopeRef四层canonical、StorageArtifactRef/FactRef完整exact DTO、只读/Reserve公共Port、future
   typed command、HoldIndex exact ref+watermark lex/carry、CurrentIndex/Tombstone全presence/TTL
   canonical、state-active TTL、Owner clock与terminal lineage；
5. Management冻结CurrentIndex/Tombstone最终DTO命名、持久化与terminal续期策略，管理线把
   测试矩阵从`review_pending`改为approved并明确授权实现；
6. 才进入纯Go Store切片。

## 3. 获批后的最小文件落点

| 路径 | 责任 | 禁止 |
|---|---|---|
| `contract/snapshot_artifact.go` | Subject、Storage/Fact exact refs、Entry/Envelope、CurrentIndex/Tombstone、Attempt/Aggregate纯DTO及type URL/version/digest domain/canonical/clock/TTL | 导入外部实现、混用digest domain、遗漏presence/TTL或选择backend |
| `ports/snapshot_artifact.go` | 首切片外部仅Reserve与aggregate/entry exact Inspect | 公共Apply/raw CAS、generic mutate、future command偷跑、Provider方法、caller-mint Fact |
| `kernel/snapshot_artifact_owner.go` | create-once Reserve；未来获批后包内seal/CAS committer与typed governed command handler | 导出/跨包注入committer；推导Evidence/Settlement；nil hold当负证明或删除成功 |
| `internal/testkit/snapshot_artifact.go` | append-only内存history/current、lost-reply seam | 生产Backend/SLA |
| `contract/snapshot_artifact_test.go` | shape/current/digest/key/TTL单元测试 | 外部集成假证明 |
| `kernel/snapshot_artifact_owner_test.go` | CAS、history/current、no-ABA、clone、retention/deletion状态核 | Provider调用 |
| `tests/snapshot_artifact_blackbox_test.go` | 公共Port create-once/Inspect行为 | Runtime/Continuity私有包 |
| `tests/snapshot_artifact_fault_injection_test.go` | lost-reply、并发、stale TTL/current | 真实外部Effect |
| `tests/snapshot_artifact_conformance_test.go` | 零Provider/零Runtime写/feature false | production认证 |

首切片已从Reserve/Inspect推进到coordinate-only Commit：Artifact Fact/Entry/Envelope rev2/
CurrentIndex rev2只能经Owner S1/S2与expected-current CAS形成。Retention/Deletion仍属未来独立
切片；实际seal/CAS committer始终留在Owner实现包内且不可导出/注入。任何情况下都不存在
公共`Apply*CAS`或raw CAS。

## 4. 阶段与验收

### SA-P0：合同联合冻结

- 关闭Retention双主、Deletion治理链、exact Ref消费边界；确认delete request不等于deleted、
  purge是唯一物理删除Effect、purge cleanup是独立Effect；
- 冻结无Revision/TTL的stable `SnapshotArtifactSubjectIdentityV2`与带Revision/TTL的versioned
  exact/current `SnapshotArtifactSubjectRefV2`分层；后者进入BindingSet最短TTL；冻结
  SubjectRef→payload FactRef→EntryRef→EnvelopeRef四层canonical、StorageArtifactRef/
  FactRef exact DTO/digest domain、统一entry/envelope、CurrentIndex/Tombstone exact DTO、
  AttemptState/AggregateState、one-active key、ExpectedAggregateRef、state-active TTL、Owner clock、
  HoldIndex exact revision/digest、watermark generation等值/词典序与跨代coverage carry；
- 冻结Purge单向创建链DeletionReservation→Request→Attempt；Request不携Attempt exact ref，Attempt
  单向绑定PurgeRequestDigest，lost reply不得回填或重封Request；
- 验收：Runtime/Continuity/Sandbox/管理线全部Review YES，无`future`或`candidate`冒充live。

### SA-P0R：外部公共Port落地

- Retention落地ExpectedCurrent/NoActive/Carry/From-To Index full exact refs与四项Reader，逐方法锁定
  closed Category/Reason、权威NotFound、Unknown、history/current和lost reply；
- Runtime以additive sibling落地Snapshot purge Evidence Issue/Handoff/atomic RecordAndConsume、
  structured neutral BindingSet、完整Settlement Ref/Submission/Association/Guard/Projection/
  EffectTerminal/CommitBundle/InspectRequest/Inspection/current Reader/Gateway，
  保持现有Checkpoint V5 Reader方法集不变；
- Runtime sibling采用设计第10.5节closed error集合，所有错误零Inspection、零Settle/Commit/
  Provider/Apply；未列backend错误归一为`internal/execution_inspection_invalid`；
- Runtime落地名义独立的purge cleanup ProviderHandoff、neutral CleanupBindingSet、两阶段
  EvidenceBindingSet、Settlement Submission/Association/Guard/Projection/EffectTerminal/
  CommitBundle/Inspection/current Reader；全部shape/canonical/TTL闭合。Sandbox Apply只更新
  cleanup/Residual，不推进deleted或Runtime Outcome；
- 验收：public Port conformance锁定一方法Reader、Gateway marker、raw Fact Port/plain Reader不满足
  production wiring，malicious tenant/scope/nested exact ref及typed-nil全部fail closed。

### SA-P1：纯合同

- 实现Shape/Current分离、逐层canonical body、自排除Ref/Digest、stable ArtifactSubject、
  StorageArtifactRef/FactRef完整type URL/version/revision/digest domain、CurrentIndex/Tombstone全部
  presence/TTL、Owner clock/RequestedNotAfter、deep clone；
- 验收：同source attempt换Schema/content/policy/ID/TTL只能Conflict；同内容不同TTL digest不同；
  payload/Entry/Envelope/Index/Tombstone自引用、presence/TTL篡改旧digest与Ref/domain混型拒绝。
- 首切片AggregateCurrent只校验Store对象TTL，不出现Participant/Lease/Fence/Scope/Credential
  或执行资格min声明。

### SA-P2：Owner Store

- 实现create-once Reservation、统一aggregate entry/envelope、append-only exact history、
  CurrentIndex、ExpectedAggregateRef CAS与按AggregateState选择的active TTL闭集；
- 验收：64路不同内容单赢家；exact replay幂等；Reservation NotFound只同request重放；CAS lost
  reply返回原winner；历史Fact expiry不杀available，terminal index不可ABA/复活。

### SA-P3：Retention/Deletion状态核

- 仅在未来typed governed command与Owner closure获批后实现Retention/Deletion handler；seal/CAS
  committer保持包内不可导出；
- 验收：Fact不含RetentionApplicationRef；Retention后续revision关联Fact；Deletion confirmed
  必须绑定HoldIndex exact revision/digest与generation-equal watermark；S1→S2按generation/sequence
  词典序，跨代carry链连续且coverage exact；S2仍NoActive；proof Reader/canonical不含caller RequestedNotAfter，不同caller bound复用
  同一exact proof；Sandbox只在Attempt/Aggregate min应用bound；one-active Attempt；failed关闭后只允许fresh stable key，confirmed/
  indeterminate终态；Provider Unknown只Inspect原Attempt。

### SA-P4：验证与资产

- unit/whitebox/blackbox/fault/conformance；ordinary100、race20、full ordinary/race/vet；
- gofmt、diff、禁止import与Provider=0扫描；
- 同步module/memory，仍不宣称production Backend/root。

## 5. 候选测试矩阵

| 类别 | 正例 | 反例 | 通过标准 |
|---|---|---|---|
| create-once | exact replay返回原Reservation | 同stable key换ID/schema/content/policy/TTL | 单一current，内容漂移Conflict |
| canonical | stable SubjectIdentity（无TTL）→versioned SubjectRef（带revision/TTL）→payload FactRef→EntryRef→EnvelopeRef | stable identity携TTL/current；SubjectRef换revision/TTL复用旧digest；自含Ref/Digest；StorageArtifactRef/FactRef混型 | identity/ref分层；后四层digest无环、分型拒绝 |
| storage ref | 完整Storage exact DTO | type/version/digest domain错；revision/namespace/content/schema/TTL变化复用digest；raw locator | Shape/digest拒绝；与FactRef隔离 |
| index/tombstone | 全presence/TTL canonical与terminal lineage | absent带value、漏TTL、own ref入body、active+tombstone、terminal换state/断链 | canonical拒绝；terminal不回退 |
| CAS | exact ExpectedAggregateRef追加统一Envelope/CurrentIndex | 只给revision、stale ref、同revision不同digest、ABA | 单winner，history不覆盖，current单调 |
| lost reply | Reservation replay/CAS winner/Provider Unknown分别恢复 | 非权威NotFound换ID；CAS盲重放；Provider NotFound换Attempt | 同request/原winner/原Provider Attempt三分 |
| TTL/clock | Sandbox-owned对象RequestedNotAfter、Owner min、pre/post | clock rollback、`now==expires`、post漂移、历史Fact杀current、caller bound进入cross-owner proof | 零写或零projection；proof exact稳定；terminal不复活 |
| current | state-active TTL闭集与terminal index | available仍min Reservation/历史Fact；expired tombstone当absent | current可续期、terminal不可回退 |
| retention | exact Policy Owner current应用 | nil hold当无hold、Policy revision/digest/TTL漂移 | 零写，旧Fact不改 |
| retention refs | ExpectedCurrent/NoActive/Carry/From-To Index full exact；四Reader逐方法closed errors | expected只给revision；NotFound当NoActive；Unknown当absent；lost reply换read key | current/history分离；同request幂等；所有失败零写零Provider |
| deletion | HoldIndex exact+generation-equal watermark+carry后按Reservation→Request→Attempt进入purge治理链 | Request携Attempt exact ref且Attempt反含RequestDigest；lost reply重封Request；request accepted/Begin/Receipt/NotFound直接deleted；index/watermark generation错；same revision换digest；lex回退；跨代无carry/断链/coverage漂移 | canonical无SCC；Request/Attempt分别Inspect；exact index；lex单调；carry连续；S2 NoActive；只在Settlement current+Apply CAS后deleted |
| neutral source | Request/versioned SubjectRef/Aggregate/Attempt由Sandbox Owner seal完整Owner/Kind/ID/Revision/Digest/Tenant/Schema/Expires | 无TTL stable SubjectIdentity被Adapter伪造成neutral current ref；缺字段后由Adapter推导；Subject stable identity相同却revision/TTL篡改复用digest；Aggregate TTL越过Head/current closure；source/neutral任一字段不等 | Shape/canonical拒绝；Adapter零输出；八字段逐项等值且BindingSet TTL只取四个versioned exact源最短值 |
| Purge Evidence | prepare/execute分别Issue→Handoff→atomic RecordAndConsume current | Record/Chain/Cursor/Consumption/Qualification-consumed部分写；混Attempt/phase/sequence；Handoff延长TTL；lost reply换Commit ID | 同Evidence Owner原子closure；exact双phase binding；同write key Inspect；Unknown不重派 |
| Evidence DTO identity | 主Purge与cleanup各18个命名Request/Result，Result绑定RequestID+RequestDigest | anonymous参数；Issue返回Ref而非QualificationCurrent；typed-nil；presence/value矛盾；同RequestID换digest；Result绑定另一Request | canonical拒绝并返回零Result；lost reply只Inspect原request identity |
| Effect拆分 | purge与purge-cleanup分别完整Operation/Evidence/Settlement/current Reader/Apply | cleanup复用purge Effect/Attempt/Permit/Evidence/Settlement；cleanup成功推进deleted；purge隐式cleanup | Conflict/unsupported；两个Conflict Domain、Reader、Apply与结果维度独立 |
| Purge Settlement parity | 主Purge全族与cleanup同等级version/type/domain/canonical/TTL | 缺Expires；Bundle部分字段；重复SettlementRef；Inspection延寿；wrong nested ref/domain | Shape/TTL拒绝、零Inspection；唯一SettlementRef；全部嵌套exact且min TTL |
| Runtime sibling | additive Snapshot purge one-method Reader+Gateway marker；Sandbox四个exact源逐字段映射neutral DTO | Adapter推导/默认填充Owner/Kind/Revision/Schema/Expires；source/neutral任一字段不等；反向import/SCC；role交换；扩大Checkpoint Reader；typed-nil | closed error、零Inspection、零副作用；Owner/Kind/ID/Revision/Digest/Tenant/Schema/Expires全部等值；原Checkpoint V5方法集不变 |
| cleanup完整性 | 独立Handoff/Binding/Evidence/CommitBundle/Inspection的TypeURL、domain、presence、TTL均闭合 | purge/cleanup互填；占位字段；部分Evidence commit；Bundle/Inspection延长TTL；Apply推进deleted | Shape/TTL拒绝；零Inspection/零Apply；只更新CleanupFact/Residual |
| closed errors | 每个允许Category/Reason逐项注入 | backend开放错误、secret或Provider payload透传 | 未列项归一`internal/execution_inspection_invalid`，message不泄漏 |
| boundary | coordinate-only Reserve/Commit/Inspect；Owner S1/S2 | 公共Apply/raw CAS、跨包committer、caller payload直接成Fact | 静态无导出写缝隙 |
| history/current | exact旧revision审计+最新CurrentIndex | old revision reopen、current回退 | append-only、no-ABA |
| clone/race | 读写值无alias，64路并发 | pointer/slice外部突变 | race clean、单winner |
| boundary | 零Provider、零Runtime/Continuity写 | import其他Owner实现、backend handle泄漏 | 静态扫描零结果 |

## 6. 当前停止条件

Workspace capture owner-local切片已经实现并验证；Checkpoint Participant生产接线仍停在联合
Review，不能以本轮测试关闭跨Owner门禁或Restore/Purge SA-P0。

## 7. 本轮既有模块回归证据

2026-07-16在`ExecutionRuntime/sandbox`实际通过：

```text
go test -count=100 -shuffle=on ./...
go test -count=20 -race -shuffle=on ./...
go test -count=1 -shuffle=on ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
```

这些命令没有SnapshotArtifactOwnerV2实现对象，因此只证明本轮资产编辑未破坏现有模块。

## 8. review_pending实施顺序

1. Retention/Legal Hold Owner独立审计全部full exact refs、四Reader逐方法errors、canonical、TTL、
   history/current/lost-reply、S1/S2与反例；
2. Runtime Owner与Harness facts双审Snapshot purge structured neutral DTO、Sandbox source字段Owner证明、
   逐字段等值映射、Evidence
   Issue/Handoff/atomic RecordAndConsume、
   Settlement/current Reader、cleanup sibling、closed errors与lost-reply，并确认additive sibling而非修改Checkpoint V5；
3. Management独立审计CurrentIndex/Tombstone DTO与terminal持久化/续期；
4. 三个外部门均YES后，Sandbox重新做无SCC import DAG与字段exact映射审计；
5. 只有新的用户实现授权后才进入SA-P1/SA-P2；purge/cleanup执行、Provider、production backend/root
   仍属于后续独立授权切片。

第四候选账本：第三候选第二独立审输入`P0=1/P1=2/P2=2`；本轮补齐主Purge Settlement全族，
Purge/cleanup命名Evidence Request/Result与request identity，冻结source→neutral单向映射，删除重复
SettlementRef语义并清理memory whitespace。owner-local候选`P0/P1/P2=0/0/0`；external `P0=3`
不变；结论`review_pending / implementation-NO-GO`，等待Harness facts双审。

第五候选账本：第四候选第一独立审输入`P0=1/P1=1`；Purge Request删除
`DeletionAttemptRefV2`并固定Reservation→Request→Attempt单向摘要链；Subject拆为无Revision/TTL的
stable identity与带Revision/TTL的versioned exact/current ref，BindingSet只消费后者。第三审指定项
保持闭合；owner-local候选`P0/P1/P2=0/0/0`、external `P0=3`不变；结论
`review_pending / implementation-NO-GO`，等待第五候选独立审计。
