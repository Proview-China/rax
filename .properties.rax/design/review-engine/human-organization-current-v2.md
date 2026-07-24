# REV-H12 Human Organization Current Consumer V2

## 状态与边界

- 业务来源：`tmp.document/Review.md` 与已冻结 `human-multisig-v2.md`。
- Organization 唯一拥有 Identity、RoleGrant、Delegation、Responsibility 与 `ReviewEligibilityCurrentProjectionV1`；Review 只消费公开 `ReviewEligibilityCurrentReaderV1`。
- 本切片不持有 Organization Store/写口，不签发或续期外部事实，不把 RoleGrant 当 Runtime Authority，不创建 Verdict/Authorization。
- Review owner-local consumer/reference/conformance 可评审；Runtime V5 与 `agent-host` production root 未闭合前，Human Multi-Sign production integration 仍 NO-GO。

## Review 具名输入

`HumanOrganizationCurrentRequestV2` 绑定完整 sealed `Panel`、`Assignment`、`ReviewerSubjectID`、可选 `DelegatorSubjectID`、`ActionScopeDigest`。Subject ID 只是 Owner lookup coordinate，必须通过 Organization 稳定 ID 派生反证：

| 输入 | exact 证明 |
|---|---|
| ReviewerSubjectID | `DeriveIdentityIDV1 == Assignment.ReviewerIdentity.Ref` |
| DelegatorSubjectID | `DeriveIdentityIDV1 == Assignment.DelegatorIdentity.Ref` |
| Delegation source | `DeriveDelegationIDV1 == Assignment.DelegationFact.Ref` |
| Responsibility source | kind=`review-target`、ID/Digest=`Panel.Target.ID/Digest`，派生 ID 等于 Panel Responsibility ref |
| Scope | 等于 delegated Assignment 的 `DelegationScopeDigest`；调用者不能换 scope |

Review 复用 Organization public contract 类型，不复制 Identity/Role/Delegation/Responsibility fact/ref。

## 唯一读取协议

```text
fresh baseline
-> ResolveCurrentReviewEligibilityV1(source)
-> unknown reply: detached new S1（不宣称恢复旧 Resolve）
-> exact Inspect(ref) S1
-> fresh clock + ValidateCurrent + Review exact mapping
-> exact Inspect(same ref) S2
-> fresh clock + ValidateCurrent + full projection equality
-> Review cut seal(deep clone, checked, min expires, digest)
```

exact Inspect 的 `Indeterminate/Unavailable` 只用原 ref 在 `context.WithoutCancel` 下恢复；不得退回 Resolve、by-name/latest 或 mutation。每位 reviewer 的 projection expiry 已是 Organization closure 的真实 min TTL；Review set expiry再取所有 reviewer 的最短正 TTL。任一 clock rollback、TTL crossing、current drift 或同 Identity 重复均 Fail Closed。

## 映射与不变量

1. Projection Identity 必须逐字段等于 Assignment ReviewerIdentity。
2. 全部 required Role 必须完整、同 scope、同 Identity；`CanVeto` 聚合必须等于 Assignment 当前 veto eligibility。
3. Delegation 与 Delegator/Delegate/Role/Scope 必须逐字段等于 Assignment；直接 Assignment 不得携带 Delegation。
4. Responsibility fact 与 author Identity 必须逐字段等于 Panel ResponsibilitySubject；production self-review 由 Organization Owner Forbidden，Review再做 exact separation。
5. sealed Owner projection 的 Checked/Expires/ProjectionDigest 不重封；Review cut 只形成自己的只读 receipt。
6. 返回值、Owner ref 中所有 slice/pointer deep clone；并发读取无 mutable alias。

## Closed errors

| 类别 | Review 处理 |
|---|---|
| InvalidArgument | request shape/canonical/subject 不完整，zero output |
| NotFound | Owner exact fact/ref 不存在，Fail Closed |
| Conflict | tenant/case/round/target/assignment/identity/role/scope/digest/ABA 漂移，zero output |
| PreconditionFailed | terminal、TTL、lease、clock rollback，zero output |
| Forbidden | production self-review 或无当前 Delegation，zero output |
| Indeterminate/Unavailable | Resolve 只能新 S1；Inspect 只能原 ref detached exact 恢复 |

## 验收反例

- reviewer/delegator subject 换值、跨 tenant、Target/Responsibility drift；
- role/scope/veto drift、Role revoke、Delegation revoke、self-review；
- exact Inspect lost reply、Resolve unknown、S1/S2 drift、TTL crossing、clock rollback；
- duplicate Assignment、duplicate actual Identity、deep-clone alias、64 并发一致读取；
- reusable conformance、ordinary100、race20、full ordinary/race/vet。

