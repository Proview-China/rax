# Sandbox 公共合同 Port Delta

状态：Sandbox lifecycle、Workspace、Checkpoint/Restore 与 Assembly release 所需公共合同已落
live；当前只保留 Snapshot terminal 与宿主部署的跨Owner Delta。

## 已关闭

| Delta | live结果 |
|---|---|
| Runtime Enforcement 4.1 exact-current | Sandbox Reader逐Owner复读Attempt/Reservation/Lease/Policy/Placement/Backend/Slot/Generation，prepare/execute独立 |
| OperationScope Evidence | activation lifecycle、独立 Inspect、Workspace commit、Checkpoint/Restore使用各自typed profile |
| Lifecycle Domain | DomainResult→Runtime opaque Settlement ref→Sandbox Apply CAS |
| Checkpoint/Restore | typed scope、Evidence、Settlement、Participant/Restore governance、Sandbox Provider/current/Apply |
| Assembly release | Component Release、Factory/Port/Slot descriptor、readiness S1/S2 |

关闭不表示 Runtime/Continuity/Agent Host 权威转移给 Sandbox。

## SD-ART-01 Retention/Legal Hold exact current

- **用例**：Snapshot purge前证明同一Artifact Subject/coverage/jurisdiction/hold-kind在当前
  Hold Index中没有active Legal Hold。
- **Owner**：Retention/Legal Hold。
- **请求**：Subject exact ref、Coverage exact ref、ExpectedIndex exact ref；
  不含caller RequestedNotAfter。
- **输出**：
  - `RetentionIndexCurrentExactRefV1`
  - `NoActiveLegalHoldCurrentProjectionV2`
  - `CoverageCarryProofExactRefV1`
  - From/To Index exact refs
- **字段**：ContractVersion、TypeURL、ID、Owner、Kind、Revision、DigestDomain、Digest、
  generation、sequence、coverage、jurisdiction、hold kinds、checked/expires。
- **不变量**：projection watermark.index_generation == index generation；S1→S2按
  `(generation,sequence)`词典序；跨代必须有连续carry proof；不同caller bound读取同一natural
  sealed projection。Sandbox只在Deletion Attempt TTL中再取min。
- **错误**：Invalid/NotFound/Conflict/Unavailable/Unknown分离；NotFound不等于NoActive。
- **恢复**：lost reply只Inspect原request+expected index；history按full exact ref读取。
- **反例**：nil LegalHoldRef证明无hold、generation回退、跨代无carry、coverage漂移、caller
  RequestedNotAfter进入proof digest。
- **兼容**：additive Retention-owned Reader；不得扩义Continuity retention。

## SD-ART-02 Runtime governed Snapshot purge

- **用例**：对available Snapshot发起唯一物理删除Effect
  `praxis.sandbox/snapshot-artifact-purge`；cleanup是独立
  `praxis.sandbox/snapshot-artifact-purge-cleanup`。
- **Owner**：Runtime拥有Operation/Intent/Admission/Permit/Enforcement/Settlement；Evidence Owner
  拥有Issue/Handoff/Record/Consume；Sandbox拥有Deletion Reservation/Request/Attempt、
  DomainResult/Apply。
- **Runtime neutral exact ref**：只含可由Sandbox exact对象逐字段无损映射的
  ContractVersion、TypeURL、ID、Owner、Kind、Revision、Schema、DigestDomain、Digest、
  Tenant、Expires；禁止Runtime反向导入Sandbox。
- **创建链**：DeletionReservation→PurgeRequest→PurgeAttempt。Request不含Attempt exact ref；
  Attempt单向绑定PurgeRequestDigest，避免摘要环。
- **Evidence**：Qualification/Handoff/Record/Chain/Cursor/Consumption exact refs；
  Record+Chain+Cursor+Consumption+Qualification-consumed由Evidence Owner同一原子提交。
- **Settlement**：Purge和cleanup各自拥有Submission、Association、Guard、Projection、
  EffectTerminal、CommitBundle、InspectRequest/Inspection的完整ContractVersion/TypeURL/
  DigestDomain/canonical/Expires。
- **不变量**：prepare/execute各自双Enforcement；Request不等于deleted；Provider NotFound不证明
  deletion；lost reply只Inspect原Attempt/CommitBundle。
- **反例**：扩义Checkpoint Settlement V5、Request↔Attempt digest环、purge隐式cleanup、
  cleanup成功推进deleted、Runtime→Sandbox import SCC。
- **兼容**：新增Runtime-owned additive sibling，不改现有Checkpoint V5 Reader。

## SD-ART-03 Management terminal DTO

- **用例**：在purge Settlement与Sandbox Apply后发布deleted；无法判定时发布indeterminate。
- **Owner**：Management terminal fact；Sandbox Artifact Owner只消费exact ref并更新自身current。
- **CurrentIndex**：type/version/ID/revision/state、HeadEnvelope、activeAttempt/tombstone presence、
  closure/time/TTL；canonical排除own ref/digest。
- **Tombstone**：terminal state、pre-terminal exact ref、cause/settlement/residual/previous
  presence、TTL；terminal不可回退。
- **不变量**：active Attempt与tombstone不能并存；available/terminal不可因历史Fact过期复活；
  terminal续期保持lineage/state。
- **反例**：delete request直接deleted、deleted→indeterminate切换、清空tombstone、Fact TTL杀current。
- **兼容**：additive Management DTO/Reader；不由Provider或Sandbox自签。

## SD-HOST-04 Agent Host注册与production readiness

- **用例**：把 Sandbox exact release 中的Factory/Port/Provider/Phase descriptors实例化为
  `hostroot.ProductionHostV1`并监督Rust Data Plane。
- **Owner**：Agent Host拥有registry/process lifecycle；Deployment/Management拥有attestation与
  certification；Sandbox只提供factory product和readiness contract。
- **输入**：FactoryID、release/artifact/manifest exact refs、Runtime/Application公共Port、SQLite、
  reverse-current socket、Provider transport、deployment/certification facts。
- **输出**：independent `SandboxProductionReadinessProjectionV1`。
- **不变量**：所有readiness角色使用不同exact proof；S1/S2稳定且TTL current；缺任一项只能
  standalone；`/readyz`只在current socket已绑定时成功。
- **反例**：descriptor自行执行、Sandbox自签attestation、Fake作为Provider proof、one proof alias
  multiple roles、ready后current server已停。
- **兼容**：不要求Agent Host反向暴露实现包；通过可信factory registry装配。

## DAG

```text
Retention Owner ----------------------+
Runtime/Evidence purge sibling -------+--> Management terminal
                                      |        |
Sandbox Snapshot Artifact exact refs -+        v
                                         Sandbox current index

Assembler Release -> Agent Host registry -> Sandbox hostroot -> Rust Data Plane
                           |
                           `-> Deployment attestation/certification
```

不存在Runtime→Sandbox、Retention→Sandbox实现包或Management→Sandbox实现包反向依赖。
