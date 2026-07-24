# Operation Settlement V4公共合同与原子边界

## 1. 版本与canonical

- Domain：`praxis.runtime.operation-settlement`；
- Contract version：`4.0.0`；
- 所有V4对象使用独立discriminator、独立type name和独立digest；
- 禁止修改V3 Settlement、Evidence V3、Dispatch V4.0、Enforcement 4.1的字段、Validate、canonical或digest；
- canonical禁止`map`与隐式默认；集合稳定排序、拒绝重复；nil slice逐字段归一；所有ID、revision、digest、phase、Attempt和Scope digest进入摘要；
- strict decode拒绝重复键、未知required字段、尾随文档与错误version/type；clone-on-read/write，调用方不得通过slice或pointer篡改Store内容。

## 2. `OperationSettlementEvidenceBindingV4`

| 字段 | 语义 | 硬校验 |
|---|---|---|
| `Phase` | `prepare`或`execute` | 封闭枚举；Submission中各一次 |
| `Consumption` | exact `OperationScopeEvidenceConsumptionRefV3` | 必须Inspect到同一Consumption Fact |
| `IssuedQualification` | Handoff/Consumption绑定的历史Qualification ref | ID同final，revision演进合法，内容与Issue一致 |
| `FinalQualification` | Consume后的current Qualification ref | 状态只能`consumed_current`；拒绝late/observation |
| `Record` | exact `OperationScopeEvidenceRecordRefV3` | 与Consumption、source/event、payload一致 |
| `CandidateDigest` | Candidate完整canonical digest | 不得只用PayloadDigest；必须由Record/Consumption反查一致 |
| `Handoff` | exact `OperationScopeEvidenceProviderHandoffRefV3` | 属于同Qualification、Attempt和phase |
| `Attempt` | Provider Attempt exact ref | prepare/execute属于同一Attempt链 |
| `EnforcementPhase` | Enforcement 4.1 exact phase ref | prepare/execute类型与Phase一致 |
| `OperationScopeDigest` | normalized canonical full Scope digest | Gateway从Qualification Scope重算，不信调用方自由值 |

Binding自身必须Seal。任一子ref重Seal、type-pun、revision/digest/expiry漂移、phase交换或Scope重算不一致均拒绝。

## 3. `OperationSettlementSubmissionV4`

Submission至少绑定：

- Settlement ID、Tenant、Effect ID与expected Effect revision；
- immutable Settlement Owner binding ref；
- exact OperationSubject、Operation ID/kind/revision/digest；
- canonical full OperationScope digest；
- `OperationSettlementDomainResultFactRefV4`；
- 两项canonical排序且唯一的Evidence bindings：prepare、execute；
- expected shared terminal guard revision；
- idempotency/conflict domain；
- Submission canonical digest。

Submission不得携带自由Permit、Policy、Authority、Budget、Credential、Fence或Provider Receipt副本。这些历史治理关系只通过exact Evidence/Enforcement引用核对；DomainResult由领域current Reader核对。

## 4. DomainResult强类型引用与Reader

`OperationSettlementDomainResultFactRefV4`必须包含稳定typed identity：

- Owner component ref；
- namespaced DomainResult kind；
- Fact ID、revision、canonical digest；
- exact Tenant、Effect ID/revision、Operation ID/digest、Attempt ref；
- SchemaRef仅是typed Fact的一部分，不能单独授权。

`OperationSettlementDomainResultCurrentReaderV4`由可信Binding V2装配按`EffectKind + DomainResultKind`路由。Reader只能投影领域Owner的authoritative current Fact，不签发Settlement、不调用Provider、不改领域Fact。未知kind、多个Owner、reader不可用、NotFound、过期观察租约或返回ref漂移均在Runtime写入前fail closed。

## 5. Fact与Port

### 5.1 `OperationSettlementFactPortV4`

候选方法：

```text
CommitOperationSettlementV4(request) -> OperationSettlementRefV4
InspectOperationSettlementV4(settlement_id) -> immutable fact
InspectOperationSettlementByEffectV4(effect_ref) -> immutable fact
InspectOperationSettlementCurrentV4(effect_ref) -> terminal projection + guard
```

只有Runtime Effect/Settlement Owner实现该Port。`Commit`不是通用CAS拼装器：Owner必须从公共Reader/Store重新读取并重建expected事实，逐摘要比较后在同一事务内写入四个对象。

### 5.2 `OperationSettlementGovernancePortV4`

候选方法：

```text
SettleOperationV4(submission) -> OperationSettlementRefV4
InspectOperationSettlementV4(ref) -> historical fact
InspectCurrentOperationSettlementV4(effect_ref) -> OperationInspectionSettlementRefV4
```

Application只能调用Governance Port，不能持有Fact Store或shared guard写入口。

### 5.3 `OperationInspectionSettlementRefV4`

Inspection ref绑定一次完整复读的canonical快照：Effect revision/owner、guard、DomainResult、prepare/execute Evidence binding、V4 terminal projection及观察上界。它是复读证据，不授予dispatch、Provider调用或Settlement写权；Commit仍由Owner最后一次复读。

## 6. Runtime Owner单事务

一次成功`CommitOperationSettlementV4`必须在同一Effect Owner事务内同时完成：

1. create-once `OperationSettlementFactV4`；
2. create-once `OperationSettlementEvidenceAssociationV4`；
3. 占用`OperationSettlementTerminalGuardV4`；
4. create-once `OperationSettlementTerminalProjectionV4`。

四项任一已存在时：同一canonical内容幂等返回；不同内容Conflict。不得出现Settlement存在但Association/guard/projection缺失，或guard已占用但无可Inspect终态事实。

## 7. V3/V4共享terminal guard

- shared guard以完整Tenant/Effect identity分区，不依赖version-specific Settlement ID；
- 既有V3 terminal CAS与新V4 Commit在同一Owner锁/事务边界内竞争同一个guard；
- V3已settled即逻辑占用guard，即使历史Fact创建时没有显式V4 guard记录；实现可在同一事务判定旧V3终态，但不得修改旧Fact摘要；
- V4胜出后，V3 CAS返回Conflict/terminal occupied；V4不写V3 sidecar、不改V3 Effect state；
- 两版本并发最多一个线性化，胜者可由公共Inspect确定。
- V3-first与V4-first必须对称：guard按`(TenantID, EffectID)`分区，不得因OperationDigest或version-specific Settlement ID变化绕过；不同Tenant的相同Effect ID各自独立。
- 历史Guard/Association/Projection读取必须先按typed ref中的Settlement ID读取真实历史四对象闭包，再逐ref比较；不得用current-by-Effect索引代替历史读取。

## 8. Evidence Owner与Runtime Owner边界

Evidence Consume先完成单Owner原子提交：Record、chain、cursor、Qualification `consumed_current`、Consumption Association。Runtime随后只读这些exact事实并提交自己的单Owner事务。

两事务之间的崩溃窗口保留为显式恢复状态：

```text
Evidence consumed_current
Settlement absent
  -> Inspect exact Evidence facts
  -> Inspect exact DomainResult current fact
  -> Inspect shared terminal guard
  -> retry same canonical Settlement V4 commit
```

禁止补偿性删除Evidence、重新Issue/Consume、重新调用Provider或宣称跨OwnerExactly Once。

## 9. 状态与终态投影

V4公开状态最小为：`absent -> settled`；Unknown/NeedsInspect属于协调过程，不伪造成Settlement Fact。`OperationSettlementTerminalProjectionV4`是V4消费方的权威终态读取面，包含Settlement ref、Association ref、DomainResult ref、Effect/Operation identity和terminal guard ref。

legacy V3 reader不会自动看到或解释V4 projection。需要V4语义的调用方必须显式依赖V4 Port；跨版本Adapter只能做受限只读展示，不能生成V3 settled。
