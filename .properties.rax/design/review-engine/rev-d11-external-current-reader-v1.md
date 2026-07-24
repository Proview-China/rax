# REV-D11 Production External Current Reader V1（Review 侧冻结设计）

## 1. 状态与边界

- Review 侧状态：**FROZEN FOR JOINT REVIEW**。本文件冻结 Review Owner 对输入、输出、一致性、TTL、错误与恢复的要求。
- production 状态：**NO-GO**。Binding、Evidence、Policy、Authority、Scope Owner 的公开 exact-current Reader/Projection 尚未全部冻结，production composition root 也未授权。
- 五类Owner与root的live公开类型、缺字段、最小Delta、准入证据和文件指纹见 [REV-D11外部Owner live依赖盘点](rev-d11-external-owner-live-inventory.md)；Binding唯一公共窄Port的第三候选见 [REV-D11 Binding Authoritative Current Port Delta V1](rev-d11-binding-authoritative-current-port-delta-v1.md)。两者都不授予Go或production GO。
- 本合同是只读聚合合同。它不创建 Binding/Evidence/Policy/Authority/Scope Fact，不产生 Runtime Authorization/Permit/Begin，不 Dispatch、不 Commit。
- `memory.Store`、`internal/testkit.ExternalCurrentReader` 和未来 conformance fixture 均不是 production Owner 或 production Backend。
- live `ReviewerBindingCurrentV1` / `DecisionEvidenceCurrentV1` 只有 `Current + ExpiresUnixNano`，没有 Owner-sealed `CheckedUnixNano + ProjectionDigest`，不能作为 production S1/S2 的可验证输入。

本轮不定义其他 Owner 的 Go Port，不复制共享类型，不选择数据库、RPC、进程拓扑、超时常量或 SLA。

## 2. Owner 与非 Owner

| 对象/动作 | 唯一 Owner | Review 侧权限 |
|---|---|---|
| `ReviewPolicyFactV2` currentness | Policy Owner | exact Inspect、比较、取 TTL；无写口 |
| Actor/Reviewer Authority currentness | Authority Owner | exact Inspect、比较、取 TTL；不签发或延长 |
| Scope currentness | Scope Owner | exact Inspect、比较、取 TTL；不改 Scope |
| Reviewer Binding currentness | Binding Owner | 使用 full `ReviewComponentBindingRefV2` exact Inspect；不合成 Binding |
| Review Evidence 到 authoritative Owner Fact 的 applicability/current | Evidence Owner | 使用 full `ReviewEvidenceRefV2` exact Inspect；不创建 Owner Fact 或 Evidence |
| `DecisionExternalCurrentRequestV1` 归一化、S1/S2、aggregate projection、min TTL、closed error | Review Owner | 只读聚合与验证 |
| production adapter 注入与生命周期 | production composition Owner，待联合冻结 | Review 不创建 root 或全局 registry |

## 3. Review 侧公开读模型

Review 继续使用 live seam：

```go
type DecisionExternalCurrentReaderV1 interface {
    InspectDecisionExternalCurrentV1(
        context.Context,
        DecisionExternalCurrentRequestV1,
    ) (DecisionExternalCurrentProjectionV1, error)
}
```

该签名只冻结 Review-owned 聚合面；它不代表各 Owner 的生产 Reader 已闭合。

### 3.1 输入来源

`DecisionExternalCurrentRequestV1` 只能由 Store 中 exact stored Review facts 组装：

- `TargetSnapshotV1`；
- `ReviewerAssignmentV1`；
- `AttestationV1`；
- canonical 去重后的 `[]ReviewEvidenceRefV2`。

调用前必须 deep clone 并 canonical seal。caller 提交的 `Current`、Owner Fact、TTL 或派生 Authorization 一律不是输入。

### 3.2 Binding exact ref

Binding 请求身份必须无损使用 live `ReviewComponentBindingRefV2` 全字段：

1. `BindingSetID`；
2. `BindingSetRevision`；
3. `ComponentID`；
4. `ManifestDigest`；
5. `ArtifactDigest`；
6. `Capability`。

任何字段缺失、同 ID 换 revision/digest/capability、或 Owner 返回不同 full ref，均 Fail Closed。production Binding Owner projection还必须携带immutable ProjectionID/Revision、exact Subject、固定`CheckedUnixNano`、`ExpiresUnixNano`与`ProjectionDigest`，并提供`ValidateCurrent(expectedProjectionRef, expectedRef, expectedSubject, now)`。Review不派生新的Binding ID/Digest，不补签Owner projection digest，不把Binding当Authority，也不缓存旧`Current=true`。

### 3.3 Evidence exact refs

每个 Evidence 输入必须保留 `ReviewEvidenceRefV2{Ref, Classification, Digest}` 全字段。Evidence Owner 的成功输出必须逐项携带并允许 Review 复核：

- 原 exact `ReviewEvidenceRefV2`；
- authoritative `EvidenceOwnerFactRefV2` 全字段：`Owner`、`FactKind`、`FactID`、`Revision`、`FactDigest`、`PayloadSchema`、`PayloadDigest`、`PayloadRevision`；
- `Current`；
- Evidence Owner公开合同冻结的 exact Subject；
- `CheckedUnixNano`；
- `ExpiresUnixNano`。
- `ProjectionDigest`。

`ReviewEvidenceRefV2 -> EvidenceOwnerFactRefV2`的applicability映射与Subject由Evidence Owner公开Reader负责。Owner projection必须是create-once/sealed immutable对象并提供`ValidateCurrent(expectedProjectionRef, expectedRef, expectedSubject, now)`；Owner current Reader原子验证subject-current index，纯值`ValidateCurrent`由Owner canonical domain验证full ref/subject、`0 < CheckedUnixNano <= now < ExpiresUnixNano`和`ProjectionDigest`。Review要求两层都成功并比较完整projection，不从`Ref`字符串、classification或digest猜测Owner Fact，不新建Applicability Fact，也不补签Owner digest。

输入按 `(Ref, Classification, Digest)` canonical 排序。完全相同项只 Inspect 一次；同 `Ref` 但 classification/digest 不同是 `Conflict/EvidenceConflict`。输出必须和输入一一对应，禁止多项合并、缺项、额外项或顺序依赖。

### 3.4 输出

live `DecisionExternalCurrentProjectionV1` 只包含：

- exact current `ReviewPolicyFactV2`；
- exact current Actor Authority、Reviewer Authority 与 Scope `OperationGovernanceFactRefV3`；
- `ReviewerBindingCurrentV1`；
- 每项 `DecisionEvidenceCurrentV1`；
- aggregate `Current` 与 `ExpiresUnixNano`。

projection 必须 deep clone；不得返回可变 alias。它只证明本次 Review 决策读模型完整，不授予 Runtime Evidence、Authority 或执行资格。

live `ReviewerBindingCurrentV1` / `DecisionEvidenceCurrentV1` 缺少 `CheckedUnixNano` 与 `ProjectionDigest`，所以只能服务现有reference/test读模型，不能直接升级为production合同。production候选必须由各Owner冻结versioned public projection。每个Owner使用自己的nominal类型，但都必须无损表达以下共同语义：

```text
ContractVersion
ProjectionID
ProjectionRevision
ProjectionDigest
ExactRef
ExactSubject
Current
CheckedUnixNano
ExpiresUnixNano
ValidateCurrent(expectedProjectionRef, expectedRef, expectedSubject, now)
```

Binding/Evidence projection还必须包含上文各自的full public fields。`ProjectionDigest`由Owner在自己的canonical domain封存，覆盖ContractVersion、ProjectionID/Revision、ExactRef、ExactSubject、Current、Checked、Expires及Owner权威payload；Review只调用公开`ValidateCurrent`并保存/比较完整projection，不生成、重写或延长该digest。

### 3.5 Immutable current projection 生命周期

1. Owner只在权威状态、applicability或current pointer发生变化时create-once一个新projection，并以Owner CAS发布新的subject-current index；不得覆盖旧projection。
2. `ProjectionID + ProjectionRevision + ProjectionDigest`、ExactRef、ExactSubject、Current、Checked、Expires和payload一经seal全部不可变。
3. `CheckedUnixNano`是Owner封存该projection时的固定时点，不是每次Inspect的调用时点；`ExpiresUnixNano`与`ProjectionDigest`同样固定。
4. historical exact Inspect按`ProjectionID/Revision/Digest`返回同一内容的deep clone。current Inspect还必须先验证subject-current index仍精确指向expected projection；若已切换到新projection，返回drift/stale，不返回新值替代expected。
5. fresh currentness由Owner current Reader与Review传入`ValidateCurrent`的fresh `now`共同完成：Reader原子验证subject-current index；纯值`ValidateCurrent`只验证expected refs/subject、sealed digest、`Checked <= now < Expires`及Owner状态，不重封Checked/Expires/Digest，也不得宣称独立验证index。
6. 仅因调用时间前进不得产生新projection；到期后同一projection仍可historical exact Inspect，但`ValidateCurrent`必须失败。

因此S1/S2比较的是同一个Owner-sealed immutable projection。任何Reader在第二次读取时使用fresh now重写Checked、延长Expires或重算ProjectionDigest，均违反合同并Fail Closed。

### 3.6 S1 ProjectionRef 的唯一来源

REV-D11 v1不允许caller或stored Review facts预带/指定Owner current ProjectionRef。唯一合法路径是：

1. Review从exact stored Target、Assignment、Attestation和Evidence refs派生每个Owner的full nominal `ExactSource + ExactSubject`；不得丢字段或把不同Owner subject type-pun为通用字符串。
2. S1以该full key调用Owner线性化current index。此查询是read-only exact resolve，不是by-name/latest/list/filter；它原子返回当前条目的完整`ProjectionRef{ID, Revision, Digest}`，或closed NotFound/Conflict。
3. Owner current index只可通过状态变化时的CAS前进到新create-once ProjectionRef；已被替代的旧Ref不得再次发布为current，禁止ABA。
4. S1取得完整ProjectionRef后，立即按该Ref执行Owner current exact Inspect，由Reader原子验证current index仍指向该Ref；随后调用纯值`ValidateCurrent`验证sealed expected/current/TTL。resolve与Inspect之间发生漂移时，S1失败，不追随新Ref。
5. Review保存S1取得的full ProjectionRef和完整projection。S2不得再次resolve latest；它只按S1保存的exact Ref复读，并由Owner current Reader原子校验同一ExactSource+ExactSubject的current index仍等于该Ref。
6. S2发现index漂移、Ref不完整、NotFound或新current Ref时整批Fail Closed。上层若重启整个Decide Inspect，必须从新的S1开始，不能把新Ref拼入旧snapshot。

若未来stored facts需要预带Owner ProjectionRef，必须另提版本化合同并证明ref由Owner写入且与exact Source/Subject/current index一致；不属于REV-D11 v1。

## 4. 一致性算法：S1 -> Owner current reread -> S2

跨 Owner 没有可声明的全局事务。本设计冻结一个可验证的乐观一致 cut；任何无法证明一致的情况直接失败，不用旧值拼接。

### 4.1 基线与 S1

1. 读取 non-zero `baseline`；失败或零值返回 `PreconditionFailed/ClockRegression`。
2. 校验并deep clone request；从stored Review facts派生每个Owner的full ExactSource+ExactSubject，确定唯一canonical Owner read plan。
3. 按确定性顺序对Policy、Actor Authority、Reviewer Authority、Scope、Binding、Evidence[N]执行线性化current-index resolve，取得完整ProjectionRef后立即exact Inspect；禁止by-name/latest和caller/stored preloaded ref。
4. 每次调用前后读取 fresh clock；若任何新时间小于前一已观察时间，立即 `PreconditionFailed/ClockRegression`。
5. 对每个S1结果调用Owner public `ValidateCurrent(expectedProjectionRef, expectedRef, expectedSubject, now)`，校验full ref/subject、projection identity/revision/digest、tenant/scope/target/reviewer关联、`Current=true`、`0 < CheckedUnixNano <= now < ExpiresUnixNano`；subject-current index必须已由同次Owner current Reader原子证明，不能由纯值方法冒充。
6. deep clone并保存完整、Owner-sealed S1 projection，包括`CheckedUnixNano`、`ExpiresUnixNano`与`ProjectionDigest`；不保存Owner可变对象引用。

可并行化 Owner read 不是本合同承诺；任何实现若并行读取，仍必须产生与 canonical read plan 等价的确定性结果和错误优先级。

### 4.2 Owner current reread 与 S2

1. 所有S1成功后，S2不得重新resolve current/latest；只对S1保存的同一组full exact ProjectionRef+Source+Subject执行Owner current Inspect，原子验证index仍等于该Ref；不得换ID、使用weak query或追随新projection。
2. S2先调用同一Owner的`ValidateCurrent(expectedProjectionRef, expectedRef, expectedSubject, now)`，再与保存的完整S1 projection逐字段比较；ProjectionID/Revision/Digest、ExactRef、ExactSubject、Current、Checked、Expires及Owner payload必须完全相同。
3. Binding、Policy、Authority、Scope 或任一 Evidence 发生漂移、消失、撤销、过期、tombstone 或 owner change 时，整个 projection 失败。
4. S2 不允许自动追随新 revision，也不在内部循环直到“读稳”。上层可重新发起一轮新的 Decide Inspect，但不得复用本轮部分结果。
5. S2 成功后读取 final fresh clock；`final < baseline` 或小于任一中间 clock 均为 rollback；`final >= minTTL` 为过期。

只有 S1/S2 全量一致才可 seal 单个 immutable projection。不存在“部分 current”“多数 current”或默认 current。

## 5. TTL 合同

external projection 的 `ExpiresUnixNano` 必须精确等于以下所有成功 S2 输入的最小正 TTL：

```text
min(
  Policy,
  ActorAuthority,
  ReviewerAuthority,
  Scope,
  Binding,
  Evidence[0..N-1]
)
```

- 任一 TTL 缺失、非正或在 actual point 满足 `now >= expires`，整个结果 Fail Closed；相等即过期。
- Review 不延长、四舍五入或以 aggregate 配置覆盖 Owner TTL。
- `DecisionCurrentReaderV1` 后续还必须把 external min TTL 与 Target、Case、Round、Assignment expiry/lease、Attestation、Finding 等 Review-owned TTL 再取一次全局最小值。
- 每个 Owner 和每一项 Evidence 必须分别成为唯一最短值接受测试；不能只用“所有 TTL 相同”的 fixture 代替。

## 6. Closed error set

production adapter 必须把 Owner-specific error 归一到下表 closed set；调用方只按 Category/Reason 分支，不解析 message。

| Category | 允许 Reason | Review 语义 |
|---|---|---|
| `InvalidArgument` | `InvalidReference`、`InvalidDigest`、`InvalidCanonicalForm` | request/ref/subject/digest/canonical 输入不合法；零 Owner mutation |
| `Unauthenticated` / `Forbidden` | `OwnerConflict` | tenant/identity/owner 边界不成立；Fail Closed |
| `CapabilityUnavailable` | `ComponentMissing` | 必需 Owner Reader 未注入或合同版本不支持；production NO-GO |
| `NotFound` | `OwnerMissing`、`EvidenceSourceMissing` | exact Binding/Evidence/Policy/Authority/Scope 不存在；不使用 current index 替代 |
| `Conflict` | `RevisionConflict`、`InvalidDigest`、`DuplicateCanonicalKey`、`BindingDrift`、`BindingSetConflict`、`EvidenceConflict`、`EvidenceScopeConflict`、`OwnerConflict` | S1/S2 full projection、exact Ref/Subject或ProjectionDigest漂移，或strict JSON存在重复key；整批失败 |
| `PreconditionFailed` | `BindingExpired`、`BindingNotCertified`、`EvidenceSourceStale`、`ReviewVerdictStale`、`ClockRegression` | inactive/expired/tombstone/clock rollback；整批失败 |
| `RateLimited` | `EvidenceUnavailable` | Owner 明确限流；不降级为 current |
| `Unavailable` | `EvidenceUnavailable`、`ComponentMissing` | transient read unavailable；只允许下节的同请求恢复 |
| `Indeterminate` | `EffectUnknownOutcome` | exact Inspect 是否完成未知且恢复后仍不能证明；不返回 projection |
| `Internal` | `InvalidCanonicalForm`、`InvalidDigest` | Owner adapter 返回不可能的 shape/alias/digest；不得伪装 NotFound |

不在 closed set 的 Owner error 必须 Fail Closed，并归一为 `Internal/InvalidCanonicalForm`；不得吞错、返回 zero projection 或把 Unknown 降为 NotFound。

## 7. Lost reply 与 clock rollback

- 聚合Port及Policy/Authority/Scope/Evidence子Port是read-only exact Inspect；它们没有create/CAS/dispatch重试权限。
- Binding是唯一已冻结的S1特化：首次S1可按exact Source+Subject调用`ResolveCurrentReviewBindingV1`取得当时的full ProjectionRef，随后立即以该Ref执行exact `InspectCurrentReviewBindingV1`。Resolve lost reply后的同canonical读取若返回不同current Ref，只能废弃旧部分结果并开启全新S1，不能称为恢复原结果；S2和任何exact Inspect recovery仍绝对不得重新Resolve或追随新Ref。
- `Unavailable` 或 `Indeterminate` 可对**同一个 canonical Owner request**执行至多一次 recovery Inspect。不得换 ref、转 weak query、读 current-by-name 或调用 mutation。
- `context.Canceled`、`context.DeadlineExceeded` 与无类型 Unknown 必须归一为 `Indeterminate/EffectUnknownOutcome`，不能伪装 authoritative NotFound。
- recovery Inspect 必须使用不受原请求取消影响的 context，并由宿主已批准的边界提供独立上限；本设计不选择固定 timeout/SLA。
- recovery前后都读取fresh clock并对Owner返回的同一sealed projection调用`ValidateCurrent`；fresh now不得用于重封Checked/Expires/Digest。若时钟回拨、TTL crossing、S1/S2漂移或第二次仍未知，返回closed error，丢弃本轮全部部分结果。
- lost reply 恢复不会产生“exactly-once transport”声明；它只说明重复同一只读 Inspect 不改变 Owner 事实。

## 8. 依赖 DAG 与 production 门禁

```text
Policy current Reader ------------------┐
Actor/Reviewer Authority current Reader ├─> Review external-current aggregator
Scope current Reader -------------------┤        -> S1/S2 sealed projection
Binding exact-current Reader -----------┤        -> DecisionCurrentReader
Evidence applicability/current Reader --┘        -> Verdict Owner Decide/CAS
```

production implementation前必须同时满足：

1. 五类Owner冻结公开、版本化、只读exact-current Reader/Projection、`ValidateCurrent(expectedProjectionRef, expectedRef, expectedSubject, now)`与closed errors；
2. 每个Owner projection状态变更时create-once/sealed，包含immutable ProjectionID/Revision、exact Ref/Subject、Checked/Expires与Owner-sealed ProjectionDigest；Binding Reader接受full `ReviewComponentBindingRefV2`，Evidence Reader接受full `ReviewEvidenceRefV2`并返回authoritative full Owner Fact；
3. Owner Reader明确subject-current index、historical exact Inspect、`0 < Checked <= now < Expires`、tombstone/revocation、retention、lost-reply与deep-clone语义；fresh now只验证，不重封projection；
4. production composition 只注入 Reader capability，无 Owner write Port、Fake 或 mutable global registry；
5. Review conformance 套件通过；
6. 另行获得 production adapter/root 实现授权。

当前以上条件未全部满足，因此只允许资产冻结；不实现 production Adapter/root。

### 8.1 Binding authoritative closure 特化

Binding S1的`ResolveCurrent`以及S1/S2每次`InspectCurrent`不能只验证Owner-sealed projection index。它们必须按 [Binding第三候选](rev-d11-binding-authoritative-current-port-delta-v1.md) 在Runtime Binding Owner同一snapshot中重新复读当前BindingSet、selected及全部member BindingFact、Set/Fact两侧完整Grant和全member TTL closure，再与projection history/current full Ref/highestRevision交叉验证。底层renew/revoke/drift但projection未推进时仍须Fail Closed。historical exact Inspect不提供current结论。

该特化不增加第二个public Raw Fact Reader：唯一公共面仍是Runtime Binding Owner nominal authoritative-current Reader；Raw snapshot接口、Binding Fact与Grant material保持`runtime/control`内部。Review只消费窄projection，不能导入Runtime实现包。

## 9. 兼容与迁移

- live `DecisionExternalCurrentReaderV1` seam 保持reference/test兼容；production必须以versioned加法Owner projections补齐Checked/ProjectionDigest，不能静默把旧read model升权。本文件不扩Runtime/Harness/Application合同。
- `internal/testkit.ExternalCurrentReader` 继续只服务 reference/test，不能迁移为生产注入。
- 未来 Owner Port 必须版本化加法；不以 Review 私有兼容接口替代公共合同。
- 旧 Verdict 历史不重写；production Reader 启用只影响新 Decide 的 currentness 检查。
- 回退只停用 production Reader capability；不得恢复 caller-supplied current 或旧缓存。
