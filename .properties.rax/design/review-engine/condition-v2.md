# Review Condition V2 精确条件集与准入当前性设计

## 1. 状态与目标

- 最高业务输入：`tmp.document/Review.md`，尤其“Conditional 不能只写自由文本”“每个 Condition 都必须可独立证明”。
- 本资产状态：**Review-owned资产最终独立复审YES（P0/P1/P2=0）；尚未授权 Go 实现**。
- production 状态：**NO-GO**。Runtime Policy/Binding/Authority 的 live current Reader 可以复用，但缺少 Policy Owner 对“某个 exact Condition tuple 是否允许”的公共窄聚合 Reader，也没有宿主 composition root。
- 本切片只补 Review 自有设计、计划和测试 oracle；不修改 Runtime/Harness/Application/Model，不创建 production adapter/root。

目标是让 Auto、单人 Human、企业 Human 多签最终都以同一组 `runtimeports.ReviewConditionV2` 表达 Conditional，而不是只传一个无法审计的 `ConditionsDigest`。

## 2. Owner 与非 Owner

| 对象/动作 | 唯一 Owner | Review 的权限 |
|---|---|---|
| `runtimeports.ReviewConditionV2`、`DigestReviewConditionsV2` | Runtime public contract Owner | 直接复用；不复制、不改名、不建私有 Condition 类型 |
| Condition proposal、Attestation/Verdict 内的 exact Condition set | Review Verdict Owner | 校验、排序、持久化、绑定 Case/Target/Assignment，并将 exact set带入Verdict |
| Condition 是否被当前 Policy 允许 | Policy Owner | Review 只消费 current projection；不从名称、Schema或Owner猜测 |
| `SatisfactionOwner` Binding/Capability current | Runtime Binding Owner | Review 只按 exact Source+Assignment/Target subject复读 |
| Condition `Authority` current | Authority Owner | Review 只按 exact ref复读并校验Run/Scope/ActionScope |
| `ConditionSatisfactionFactV2`、其Create/CAS/current index | Runtime Owner | Review 不创建、不CAS；只把Verdict与ConditionsDigest交给既有Runtime链 |
| `ReviewConditionProofV2`所引用的领域source fact | 各`SatisfactionOwner`领域Owner | 领域Owner只创建/Inspect/CAS自己的source fact；Runtime Owner exact复读后才可形成正式Satisfaction |
| production wiring | 宿主composition Owner | 最后注入只读Reader；Review不得私建root或Owner fake冒充production |

Review只拥有“提议了哪些Condition、这些Condition是否被允许进入Verdict”的领域判断。Condition是否已经满足的正式事实唯一归Runtime Owner；各领域Owner只拥有Proof所引用的source fact，不能写Runtime Satisfaction。

## 3. 唯一类型裁决

### 3.1 直接复用的公共类型

所有生产路径必须直接使用：

```go
[]runtimeports.ReviewConditionV2
runtimeports.DigestReviewConditionsV2
```

`ReviewConditionV2`的live完整字段为：

| 字段 | 语义 |
|---|---|
| `ID` | namespaced稳定Condition身份；同一集合中唯一 |
| `Revision` | exact Condition revision，必须非零 |
| `Schema` | 完整`SchemaRefV2`，不是自由文本 |
| `ConstraintDigest` | 条件约束的canonical digest |
| `SatisfactionOwner` | 完整`ReviewComponentBindingRefV2`，指定独立证明Owner |
| `ScopeDigest` | Condition适用的Action Scope digest |
| `Authority` | 完整`AuthorityBindingRefV2` |
| `ExpiresUnixNano` | Condition自身TTL上界 |

集合必须按`ID`严格递增、数量不超过`runtimeports.MaxReviewConditionsV2`、不得出现同ID不同revision的双项；digest只调用`DigestReviewConditionsV2`生成。

### 3.2 不允许的替代

- Review私有`ReviewConditionV1`或`ConditionDTO`；
- 只传`ConditionsDigest`而没有exact set；
- 用Finding、ReasonCode、自然语言备注或平台评论冒充Condition；
- 用`ReviewerBinding`代替`SatisfactionOwner`；
- 用Actor/Reviewer Authority角色投影type-pun Condition Authority；
- Review创建`ConditionSatisfactionFactV2`或把Provider自报升级为Satisfaction。

## 4. 对live V1对象的兼容加法

以下live对象在未来获批实现时增加同名字段：

```go
Conditions []runtimeports.ReviewConditionV2 `json:"conditions,omitempty"`
```

落点：

1. `AutoReviewerStructuredOutputDraftV1`；
2. `AutoReviewerStructuredOutputV1`；
3. `AttestationV1`；
4. `VerdictV1`。

### 4.1 JSON与digest兼容

- 字段必须使用`omitempty`；nil和空切片在旧对象canonical JSON中均不出现，因此无Condition的旧对象digest保持可读。
- Provider Draft不得携带caller自封的`ConditionsDigest`；host seal只从exact set生成digest。
- sealed Auto Output、Attestation与Verdict若caller提供非空digest，必须与`DigestReviewConditionsV2(Conditions)`完全一致；空digest只允许进入各对象的Owner `Seal`，由Owner生成后再持久化。任何production持久对象都必须携带非空exact digest。
- 非Conditional对象必须同时满足`len(Conditions)==0 && ConditionsDigest==""`。
- 新production Conditional必须同时满足`len(Conditions)>0`且digest exact。

### 4.2 legacy digest-only隔离

历史`ResolutionConditionalV1/VerdictConditionalV1 + ConditionsDigest + empty Conditions`继续允许historical exact Inspect和低层Store兼容读取，但：

- 不得通过production Service、Auto Attestation Owner、Verdict Owner或Runtime Adapter；
- 不得产生新的Verdict revision、Runtime current projection或Authorization；
- 不得通过“从digest反推Condition”升级；
- 只有工作域形成新Candidate/Case/Round并提交exact Conditions后，才能进入新production Decide。

因此实现时必须区分：

1. `Validate`/historical decode：保留旧对象可读；
2. production strict validator：Conditional要求非空exact set；
3. Owner mutation/Decide/adapter：只能调用strict validator。

## 5. Owner-local canonical与绑定不变量

### 5.1 Auto structured output

- Provider输出始终是Observation；它可以提议完整Condition，但不能决定admissibility或Satisfaction。
- builtin JSON Schema必须为`conditions`冻结`additionalProperties:false`的完整字段形状，最多64项；Conditional要求至少1项，非Conditional禁止该字段。
- host seal重新排序、重算digest并拒绝未知字段、重复key、同ID双revision、坏Schema/Authority/Binding或条件digest不一致。
- Rubric只允许某类结构化输出；Rubric允许Conditional不等于Policy允许具体Condition tuple。

### 5.2 Attestation

- exact Conditions必须从已验证的Auto output或Human input无损复制，不能只复制digest。
- 每项`ScopeDigest == Target.ActionScopeDigest`。
- 每项TTL必须晚于实际检查点，且不得超过Attestation、Target、Case、Round、Assignment与Rubric的适用上界。
- Auto Attestation仍必须先完成`applied ApplySettlement -> exact ReviewerInvocationResultFact`全链复读；Condition不能绕过该链。

### 5.3 Verdict

- 单人/Auto Verdict的Conditions与唯一source Attestation必须逐字段完全相等，digest相等；禁止Verdict Owner改写、缩减或补充。
- Human多签产生Conditional Verdict时采用唯一canonical union：只有计入accept quorum的`Conditional Acceptance`票贡献Condition，普通`Accept`贡献空集；按Condition `ID`做union并按ID严格排序。同ID出现在多票时八字段必须逐字段完全相等，否则整个Quorum Decision `Conflict`且零写；不同ID直接并集。结果digest由Owner重算，`QuorumDecisionV2 -> HumanVerdictV2`两层Conditions和digest逐字段完全相等。任一计入Conditional票的Conditions为空、digest不exact或TTL已过期均Fail Closed；禁止intersection、last-writer-wins、任选一票或自由文本合并。
- 多签不得再创建`VerdictV1`或第三个Review终态：V1单Reviewer字段不能表达quorum，synthetic panel/group reviewer禁止。现有Runtime V5 adapter只读exact复读QuorumDecisionV2与HumanVerdictV2，验证其Conditions/digest完全相等后映射到现有`OperationReviewQuorumCurrentProjectionV5`；该projection不是Review Store mutation，也不复制Review终态。Conditional Runtime projection继续通过existing Satisfaction的`ConditionsDigest`绑定exact set。
- Verdict expiry取既有Target/Case/Round/Assignment/Attestation/Policy/Authority/Scope/Binding/Evidence最短TTL，并进一步纳入每个Condition自身TTL与Condition admissibility projection TTL。
- Condition、Policy、Binding、Authority或Scope在S1/S2间漂移时，Verdict CAS零写。

## 6. REV-D12：`ConditionAdmissibilityCurrentReaderV2`公共窄Port Delta

### 6.1 缺口与唯一Owner

live已有：

- `ReviewDecisionPolicyCurrentReaderV2`；
- `ReviewBindingAuthoritativeCurrentReaderV1`；
- `DispatchAuthorityCurrentReaderV3`；
- `ReviewConditionV2`与`DigestReviewConditionsV2`。

但`ReviewPolicyFactV2`没有“允许哪些Condition Schema、SatisfactionOwner、Capability、Authority组合”的字段。Review不能自行把`Policy.Active`解释为任意Condition均允许。因此需要一个由**Runtime public ports Owner冻结、Policy Owner提供admissibility语义、trusted host adapter聚合Binding/Authority current**的只读窄Port。各底层事实Owner不变。

### 6.2 Policy Owner exact tuple decision窄Reader

REV-D12不能用裸`PolicyDecisionDigest`代替Policy current。Runtime public ports Owner与Policy Owner必须先冻结以下名义类型；它们属于Policy Owner current truth，不由trusted host或Review生成：

```go
type ReviewConditionPolicyDecisionRefV2 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"`
    Digest   core.Digest   `json:"digest"`
}

type ReviewConditionPolicyDecisionSubjectV2 struct {
    PolicySubject runtimeports.ReviewDecisionPolicyApplicabilitySubjectV2 `json:"policy_subject"`
    Target        runtimeports.ReviewDecisionTargetRefV1                   `json:"target"`
    CaseID        string                                                   `json:"case_id"`
    CaseRevision  core.Revision                                            `json:"case_revision"`
    CaseDigest    core.Digest                                              `json:"case_digest"`
    RoundID       string                                                   `json:"round_id"`
    RoundRevision core.Revision                                            `json:"round_revision"`
    RoundDigest   core.Digest                                              `json:"round_digest"`
    Assignment    runtimeports.ReviewDecisionAssignmentRefV1               `json:"assignment"`
    Condition     runtimeports.ReviewConditionV2                           `json:"condition"`
}

type ReviewConditionPolicyDecisionCurrentProjectionV2 struct {
    ContractVersion  string                                      `json:"contract_version"`
    Ref              ReviewConditionPolicyDecisionRefV2          `json:"ref"`
    Subject          ReviewConditionPolicyDecisionSubjectV2      `json:"subject"`
    Policy           runtimeports.ReviewDecisionPolicyCurrentProjectionV2 `json:"policy"`
    Allowed          bool                                        `json:"allowed"`
    State            runtimeports.ReviewDecisionGovernanceProjectionStateV1 `json:"state"`
    Current          bool                                        `json:"current"`
    CheckedUnixNano  int64                                       `json:"checked_unix_nano"`
    ExpiresUnixNano  int64                                       `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                                 `json:"projection_digest"`
}

type ReviewConditionPolicyDecisionCurrentResolveRequestV2 struct {
    Subject ReviewConditionPolicyDecisionSubjectV2 `json:"subject"`
}

type ReviewConditionPolicyDecisionCurrentReaderV2 interface {
    ResolveCurrentReviewConditionPolicyDecisionV2(context.Context, ReviewConditionPolicyDecisionCurrentResolveRequestV2) (ReviewConditionPolicyDecisionRefV2, error)
    InspectCurrentReviewConditionPolicyDecisionV2(context.Context, ReviewConditionPolicyDecisionSubjectV2, ReviewConditionPolicyDecisionRefV2) (ReviewConditionPolicyDecisionCurrentProjectionV2, error)
    InspectHistoricalReviewConditionPolicyDecisionV2(context.Context, ReviewConditionPolicyDecisionRefV2) (ReviewConditionPolicyDecisionCurrentProjectionV2, error)
}

func (r ReviewConditionPolicyDecisionRefV2) Validate() error
func (s ReviewConditionPolicyDecisionSubjectV2) Validate() error
func (p ReviewConditionPolicyDecisionCurrentProjectionV2) Clone() ReviewConditionPolicyDecisionCurrentProjectionV2
func (p ReviewConditionPolicyDecisionCurrentProjectionV2) Validate() error
func (p ReviewConditionPolicyDecisionCurrentProjectionV2) ValidateCurrent(expected ReviewConditionPolicyDecisionRefV2, subject ReviewConditionPolicyDecisionSubjectV2, now time.Time) error
func DeriveReviewConditionPolicyDecisionIDV2(subject ReviewConditionPolicyDecisionSubjectV2) (string, error)
func DigestReviewConditionPolicyDecisionCurrentProjectionV2(p ReviewConditionPolicyDecisionCurrentProjectionV2) (core.Digest, error)
func SealReviewConditionPolicyDecisionCurrentProjectionV2(p ReviewConditionPolicyDecisionCurrentProjectionV2) (ReviewConditionPolicyDecisionCurrentProjectionV2, error)
```

唯一语义：stable Projection ID由`Policy.Ref + Tenant + Target ID/revision + Case ID + Round ID + Assignment ID/revision + Condition ID/revision`派生；Subject的其余exact字段同ID漂移为Conflict。Owner状态变化时append-only revision严格`+1`并以full Ref CAS更新current index；Checked/Expires/Digest在projection创建时sealed，Inspect exact返回同一deep clone，时间流逝只使`ValidateCurrent(expected, subject, now)`失败，不重封projection。`Allowed=true && State=active && Current=true`才可成功消费；deny返回`Forbidden`，不得返回可被误用的软成功。`ProjectionDigest`domain为`praxis.runtime.review-condition-policy-decision-current`、contract为`praxis.runtime.review-condition-policy-decision-current/v2`、body为完整projection且计算时只清空`Ref.Digest/ProjectionDigest`。`0 < Checked < Expires <= Policy.Expires`。

Policy Owner Reader的`ResolveCurrent`按exact Subject取得current full Ref；`InspectCurrent`必须原子核对current index仍等于expected Ref并重验底层Policy current full Ref。historical Inspect只按exact Ref读取history，不借current index。所有方法closed error与6.6相同；reply-loss只允许同canonical request做只读恢复，不发布新revision。

新Policy tuple contract的closed Category+Reason固定如下，不允许Owner/adapter改映射：

| 方法/校验 | 条件 | 唯一Category + Reason |
|---|---|---|
| `Ref.Validate` / `Subject.Validate` | 缺字段、零revision、坏namespace/shape | `InvalidArgument + InvalidReference` |
| `Subject.Validate` | digest非法或Conditions非canonical | `InvalidArgument + InvalidDigest/InvalidCanonicalForm` |
| `ResolveCurrent` | 该stable Subject ID从未有history/current index | `NotFound + ReviewVerdictMissing` |
| `ResolveCurrent` | history存在但无active current、terminal或TTL已失效 | `PreconditionFailed + ReviewVerdictStale` |
| `InspectCurrent` | exact ID从未存在 | `NotFound + ReviewVerdictMissing` |
| `InspectCurrent` | expected历史存在但current index不是expected、same-ID drift或ABA | `Conflict + BindingDrift` |
| `InspectCurrent` | projection revoked/expired/superseded、TTL crossing | `PreconditionFailed + ReviewVerdictStale` |
| `InspectHistorical` | exact Ref不存在 | `NotFound + ReviewVerdictMissing` |
| `InspectHistorical` | ID存在但revision/digest不exact | `Conflict + RevisionConflict/InvalidDigest` |
| Policy deny | tuple不允许 | `Forbidden + ReviewConditionUnsatisfied` |
| `Validate/ValidateCurrent` | Ref/Subject/Policy/digest/current truth漂移 | `Conflict + BindingDrift/InvalidDigest` |
| `ValidateCurrent` | zero/rollback clock | `PreconditionFailed + ClockRegression` |
| 任一Reader | ctx取消、deadline、unknown backend/outcome | `Indeterminate + InspectCoverageIncomplete` |
| 任一Reader | 已知Policy Owner不可用 | `Unavailable + OwnerMissing` |

Reason的斜线只表示由表中客观失败字段唯一选择：revision不符=`RevisionConflict`，digest不符=`InvalidDigest`；不得由实现任意二选一。`NotFound`只能表示从未存在，不能表示terminal、deny、backend unknown或retention不明。

### 6.3 trusted-host aggregate exact request/result候选

以下是提交相应Owner联合冻结的候选形状；它只存在于资产，不是Review私有Go接口：

```go
type ReviewConditionAdmissibilitySubjectV2 struct {
    TenantID          core.TenantID                          `json:"tenant_id"`
    CaseID            string                                 `json:"case_id"`
    CaseRevision      core.Revision                          `json:"case_revision"`
    CaseDigest        core.Digest                            `json:"case_digest"`
    RoundID           string                                 `json:"round_id"`
    RoundRevision     core.Revision                          `json:"round_revision"`
    RoundDigest       core.Digest                            `json:"round_digest"`
    Target            runtimeports.ReviewDecisionTargetRefV1 `json:"target"`
    Assignment        runtimeports.ReviewDecisionAssignmentRefV1 `json:"assignment"`
    Policy            runtimeports.ReviewPolicyBindingRefV2  `json:"policy"`
    ActorAuthority    runtimeports.AuthorityBindingRefV2      `json:"actor_authority"`
    Scope             core.ExecutionScope                    `json:"scope"`
    CurrentScope      runtimeports.ExecutionScopeBindingRefV2 `json:"current_scope"`
    ActionScopeDigest core.Digest                            `json:"action_scope_digest"`
    Conditions        []runtimeports.ReviewConditionV2        `json:"conditions"`
    ConditionsDigest  core.Digest                            `json:"conditions_digest"`
}

type ReviewConditionAdmissibilityItemV2 struct {
    Condition                    runtimeports.ReviewConditionV2             `json:"condition"`
    PolicyDecision               runtimeports.ReviewConditionPolicyDecisionCurrentProjectionV2 `json:"policy_decision"`
    SatisfactionOwnerBinding     runtimeports.ReviewBindingCurrentProjectionV1 `json:"satisfaction_owner_binding"`
    ConditionAuthority           runtimeports.DispatchAuthorityFactV3       `json:"condition_authority"`
    ExpiresUnixNano              int64                                      `json:"expires_unix_nano"`
}

type ReviewConditionAdmissibilityCurrentProjectionV2 struct {
    ContractVersion  string                                           `json:"contract_version"`
    Subject          ReviewConditionAdmissibilitySubjectV2            `json:"subject"`
    Policy           runtimeports.ReviewDecisionPolicyCurrentProjectionV2 `json:"policy"`
    Items            []ReviewConditionAdmissibilityItemV2              `json:"items"`
    CheckedUnixNano  int64                                            `json:"checked_unix_nano"`
    ExpiresUnixNano  int64                                            `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                                      `json:"projection_digest"`
}

type ConditionAdmissibilityCurrentReaderV2 interface {
    InspectReviewConditionAdmissibilityCurrentV2(
        context.Context,
        ReviewConditionAdmissibilitySubjectV2,
    ) (ReviewConditionAdmissibilityCurrentProjectionV2, error)
}
```

候选中的Runtime类型只作引用；最终公共类型必须落在Runtime public ports，由相应Owner串行合入。Review不得复制这组候选到自身production包。

### 6.4 Validate与digest

- canonical domain：`praxis.runtime.review-condition-admissibility-current`；contract：`praxis.runtime.review-condition-admissibility-current/v2`；body：`ReviewConditionAdmissibilityCurrentProjectionV2`。
- `ProjectionDigest`对完整Subject、Policy projection、全部Item、Checked、Expires计算；计算时仅清空`ProjectionDigest`。
- Items顺序必须与Conditions顺序完全一致；每个Item.Condition与Subject对应Condition逐字段相等。
- `PolicyDecision`由Policy Owner exact Reader返回；其Subject必须与aggregate Subject、Target/Case/Round/Assignment和对应Condition逐字段相等，且其嵌入Policy projection必须与aggregate Policy逐字段相等。Review与host不生成、不补签其digest。
- `SatisfactionOwnerBinding.Source == Condition.SatisfactionOwner`，Subject必须精确绑定同一Tenant/Assignment/Target。
- `ConditionAuthority.Ref == Condition.Authority`，其Run/Scope/ActionScope必须与Subject相等，且State active。
- success只表示全部Condition被当前Policy允许；不返回`Admissible=false`的“软成功”。Policy拒绝必须是closed `Forbidden`。
- Subject完整交叉不变量：`TenantID == Target.TenantID == Assignment.TenantID == Scope.Identity.TenantID == Policy.Subject.TenantID`；Target ID/revision/digest/Run必须来自Review Store exact Target；Case与Round必须来自同一Target current链；Assignment必须来自同一Case/Round/Target且`Assignment.ReviewerID`与stored Assignment相等；Policy/ActorAuthority/CurrentScope/ActionScope必须与Target exact事实一致。trusted host不得只做shape Validate，Review Verdict Owner必须从同一Review-owned snapshot构造Subject并在调用前后重验这些关系。
- `Item.ExpiresUnixNano = min(Condition.Expires, PolicyDecision.Expires, SatisfactionOwnerBinding.Expires, ConditionAuthority.Expires)`，不得任意封值；aggregate `ExpiresUnixNano = min(Policy.Expires, every Item.Expires)`。
- success必须deep clone所有slice、嵌套projection及`ExecutionScope.SandboxLease`等指针字段。

### 6.5 S1 -> Owner current reread -> S2

一次调用必须在同一Reader内部完成：

1. 取非零`baseline`，校验Subject、exact Conditions、digest、`Condition.ScopeDigest == ActionScopeDigest`；
2. S1：
   - Policy按exact applicability subject resolve current full Ref并Inspect；
   - 对每个Condition，Binding按`Source=Condition.SatisfactionOwner + exact Assignment/Target subject` resolve current full Ref并Inspect；
   - Authority按`Condition.Authority` full Ref Inspect current；
   - Policy Owner对每个完整Condition tuple先`ResolveCurrent`取得full Ref，再`InspectCurrent(subject, ref)`取得sealed decision projection；
3. S2只按S1保存的full refs重新Inspect current，并原子验证各current index仍等于S1 ref；不得重新resolve并追随新revision；
4. 取fresh `now`，若零值或`now < baseline`立即ClockRegression；
5. 逐项`ValidateCurrent(expected, now)`，并逐字段比较S1/S2 projection/ref/digest/subject；
6. 每项按6.4的完整closure计算Item expiry；aggregate expiry取Policy与全部Item的真实min；`CheckedUnixNano=now`；
7. seal aggregate projection并返回deep clone。

Review Verdict Owner在CAS实际点还必须把该projection TTL与Review自身Target/Case/Round/Assignment/Attestation/Evidence等TTL再次取min。Reader projection不是Authority、Satisfaction、Evidence或Permit。

### 6.6 closed errors

| Category | 条件 | 写入 |
|---|---|---|
| `InvalidArgument` | shape、JSON/canonical、empty conditional、bad digest、unsorted/duplicate、坏full ref | 零Review mutation |
| `NotFound` | exact Policy/Binding/Authority current source确实不存在 | 零Review mutation |
| `Conflict` | S1/S2 ref/digest/index/subject漂移、cross-tenant、ABA、same-ID drift | 零Review mutation |
| `Forbidden` | 当前Policy不允许Condition Schema/Owner/Capability/Authority tuple | 零Review mutation |
| `PreconditionFailed` | revoked/expired/superseded、scope不符、TTL crossing、clock rollback | 零Review mutation |
| `Indeterminate` | ctx canceled/deadline、unknown backend/result、无法证明S1/S2 | 零Review mutation |
| `Unavailable` | 已知底层Owner不可用 | 零Review mutation |

禁止把Unknown/Unavailable降级为NotFound或Policy deny；也禁止把Policy deny当作可授权的空Conditions。

### 6.7 lost reply与ctx/clock

- Reader只读，无mutation重派问题；调用结果丢失时，Review最多用同一canonical Subject做一次fresh read recovery。detached ctx必须以宿主注入的`ReadRecoveryTimeout`裁剪，要求`0 < timeout <= 2s`且不得晚于Subject/调用方最短TTL；只允许一次retry。使用`context.WithoutCancel`后必须立即`context.WithTimeout`，不得无限等待。
- recovery返回的是新的完整current snapshot，不宣称恢复第一次未知结果；两次均无法证明时返回原closed error。
- 不缓存旧projection，不在TTL crossing后使用S1数据，不接受clock rollback。
- Auto invocation、Attestation、Decide等mutation reply-loss继续只Inspect原Attempt/Attestation/Case/Verdict，绝不因Condition reader重试而重新调用Provider或重新Decide。

## 7. 迁移顺序

1. 先由Runtime public ports/Policy Owner冻结并实现exact tuple-decision Reader，再由Runtime/Policy/Binding/Authority联合冻结REV-D12聚合Reader及conformance；
2. Review实现exact Conditions `omitempty`加法、strict production validator和schema；
3. Auto Owner、Human Service、多签入口全部写exact set；
4. Verdict Owner接入REV-D12，在CAS前完成S1/S2与min TTL；
5. Runtime Adapter拒绝legacy digest-only Conditional，只接受exact set与既有Satisfaction current；
6. 最后由宿主composition root注入真实Owner Readers；
7. 历史digest-only对象只读保留，不批量升级、不反推Condition、不恢复旧Permit。

任何一步缺失都保持production Conditional NO-GO；无条件Accept/Reject不因本切片自动获得新的production结论。

## 8. Effect、Unknown、Settlement与边界

- Condition proposal、canonical seal与current Inspect均是事实/只读操作，不是外部Effect。
- Policy/Binding/Authority Reader不得暴露write Port给Review。
- Satisfaction可能触发领域检查或外部Effect时，仍由对应Owner走独立Intent/Permit/Begin/Observation/Evidence/Settlement，不在Review内执行。
- Review Verdict只判定；Condition满足后Runtime Gate仍重验Verdict、Satisfaction、Fence、Authority、Budget、Scope、Binding。
- 本切片不触发pre-run Evidence，不创建Timeline，不把Trace当Evidence。

## 9. 发布结论

Review-owned资产最终独立复审YES（P0/P1/P2=0）；Go实现尚未授权。以下证据全部齐备前production Conditional保持NO-GO：

1. REV-D12公共类型/Reader由相应Owner冻结；
2. Policy对exact Condition tuple的sealed admissibility语义可验证；
3. Binding/Authority S1/S2 conformance通过；
4. Review exact-set实现、target100/race20/full/race/vet通过；
5. Runtime Adapter证明legacy digest-only零授权；
6. trusted host composition/root集成通过。
