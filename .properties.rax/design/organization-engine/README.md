# Organization Engine：Review Human Current Owner V1

## 1. 冻结状态

- 最高业务输入：`tmp.document/Review.md`。
- 用户已明确授权本最小模块，用于关闭 Review 企业多签的 Identity、Role、Delegation、Responsibility current 生产依赖。
- 实现语言 Go 1.25；State Plane v1 为单机 SQLite WAL，明确不承诺 HA、跨节点线性一致或 SLA。
- 本设计只冻结 Organization Owner 本地域；不建立 production composition root，不修改 Runtime Authority、Review Verdict、Harness 或 Application。
- 声明式Component Release见 [component-release-v1.md](component-release-v1.md)：Organization Owner P0/P1代码候选与软件门已完成，等待独立代码审计；Human Multi-Sign条件依赖variant、production readiness和Host root仍未完成。

## 2. Owner 边界

| 对象 | 唯一 Owner | 本模块行为 |
|---|---|---|
| Human Identity | Organization | immutable fact、append-only history、current full-ref CAS |
| Role Grant / veto role | Organization | 明确授予；Responsibility 不隐式推导 Role |
| Delegation | Organization | 明确 Delegator/Delegate/Role/Scope/TTL；撤销发布新 revision |
| Responsibility Subject | Organization | 绑定候选作者/发起者 exact Identity，用于禁自审 |
| Runtime Authority | Runtime/Authority Owner | 只由调用方以公开 nominal ref 另行复读；Organization 不签发 |
| Review Panel/Vote/Verdict | Review Verdict Owner | Organization 只提供 current proof，不计票、不判定 |

禁止跨域复制 Review 的 `HumanIdentityProofRefV2`、`HumanDelegationFactRefV2`、`HumanResponsibilitySubjectRefV2`。Organization 输出自己的具名 exact refs；宿主 Adapter 仅逐字段映射公开 nominal carrier。

## 3. 领域事实与稳定身份

所有事实携带 `ContractVersion/TenantID/ID/Revision/Digest/CreatedUnixNano/UpdatedUnixNano/ExpiresUnixNano/State`。Digest 使用 `praxis.organization.review-current` 域和具名 body seal。

| Fact | 稳定 ID 输入 | 关键字段 |
|---|---|---|
| `IdentityFactV1` | Tenant + SubjectID | SubjectKind、SubjectID、DisplayHandle、State |
| `RoleGrantFactV1` | Tenant + IdentityID + Role + ScopeDigest | Identity exact ref、Role、ScopeDigest、CanVeto、State |
| `DelegationFactV1` | Tenant + DelegatorSubjectID + DelegateSubjectID + Role + ScopeDigest | 两侧 Identity exact ref、Role、Scope、State |
| `ResponsibilityFactV1` | Tenant + SubjectKind + SubjectID | Author Identity exact ref、SubjectDigest、State |

规则：

1. revision 首建为 1，续版严格 `+1`；同 revision 换 digest Conflict。
2. history 只按 `(kind, tenant, ID, revision, digest)` exact 读取，永不借 current index。
3. current index 保存 full Ref；发布在一个 Owner 原子事务内完成 history insert、highest revision 验证与 current full-ref CAS。
4. 同 canonical lost reply 只 exact Inspect；不得重发另一个 mutation。generic NotFound 不授重投。
5. 纯时间到期只让 `ValidateCurrent(now)` 失败，不自动发布 expired revision。
6. terminal fact 保留在历史/current；它不能通过 active current 校验。撤销/替代必须发布新 immutable revision。

## 4. Review Eligibility Current Projection

`ResolveCurrentReviewEligibilityV1` 的唯一输入是具名 source coordinate：

```text
TenantID
ReviewerSubjectID
RequiredRoles[]
ScopeDigest
ResponsibilitySubjectKind + ResponsibilitySubjectID + SubjectDigest
DelegatorSubjectID (仅委托路径)
RequireDelegation
Production=true
```

Owner 在一个线性化 snapshot 中解析完整 exact refs，并返回 sealed immutable `ReviewEligibilityCurrentProjectionV1`：

```text
Identity exact Ref
all required RoleGrant exact Refs
optional Delegation exact Ref
Responsibility exact Ref
ReviewerSubjectID / DelegatorSubjectID
RequiredRoles / ScopeDigest / Responsibility coordinate
Current=true
CheckedUnixNano = 本 closure 最新事实更新时间（固定，不因读取时间重封）
ExpiresUnixNano = Identity、全部 Role、Delegation、Responsibility 的最短 TTL
ProjectionDigest
```

`InspectCurrentReviewEligibilityV1(ctx, exact ProjectionRef)` 只按 exact ref 读取同一 sealed projection source，再复读 current indexes。Reader 不提供 by-name/latest 弱 Inspect。

### S1/S2

```text
baseline fresh clock (>0)
 -> S1 owner snapshot: resolve/inspect exact Identity + all Role + optional Delegation + Responsibility
 -> validate tenant, active state, role/scope, delegate/delegator、Responsibility identity、production self-review
 -> seal stable projection
 -> S2 owner snapshot: same exact refs + current full-ref indexes unchanged
 -> fresh clock; now < baseline => ClockRegression
 -> ValidateCurrent(now), including min TTL
 -> deep clone result
```

生产禁自审：`Responsibility.Identity.SubjectID == ReviewerSubjectID` 必须 `Forbidden`。委托只能在 `RequireDelegation=true` 时接受 current exact Delegation，且 Delegate 必须是 Reviewer，Delegator 必须匹配请求，Role/Scope 必须同时匹配；显示名和调用方布尔值不能替代证明。

## 5. Closed errors 与恢复

| Category | 原因 | 行为 |
|---|---|---|
| InvalidArgument | 空字段、未排序角色、非法时间、坏 digest | zero write |
| NotFound | exact history 或稳定 coordinate 确实不存在 | Fail Closed |
| Conflict | revision/digest/current CAS/tenant/scope/ABA 漂移 | zero write |
| PreconditionFailed | terminal、TTL crossing、必需角色/委托缺失 | Fail Closed |
| Forbidden | production 自审、delegate/delegator 不匹配 | zero write |
| Indeterminate | ctx cancel/deadline、commit/lost reply outcome unknown | exact Inspect 原 ref；不盲重试 mutation |
| Unavailable | 已知 SQLite/Owner 不可用 | Fail Closed |

Reader 在 ctx 取消后不得伪造 NotFound；lost reply 恢复只允许 `context.WithoutCancel` 下 exact Inspect 原 ref。没有 expected ProjectionRef 的 Resolve unknown 只能开启新的 S1，不能宣称恢复原结果。

## 6. 持久化与 import DAG

```text
runtime/core primitives
        ^
organization-engine/contract <- ports <- memory / storage/sqlite
                                      <- current <- conformance/tests

Review public nominal carrier <- host adapter (未来 composition root)
```

- 生产包只依赖 Organization 公开包与 `runtime/core`；不导入 Review/Runtime ports/实现包。
- SQLite 开启 WAL、foreign_keys、busy_timeout、`BEGIN IMMEDIATE`；schema 记录 digest，facts/history/current 分离。
- memory 仅 reference/test backend，不宣称生产 State Plane。
- 当前不建立 host adapter/root；这仍是 Review production 总链的集成门禁。

## 7. 验收

- unit：canonical、ID/digest、revision、state、TTL、deep clone。
- whitebox：history/current/highest 同事务、staged failure zero leak、bad current 不影响 historical exact。
- blackbox：direct reviewer、delegated reviewer、required roles、veto role、production self-review拒绝。
- fault：ctx cancel、lost commit reply exact Inspect、S1/S2 drift、clock rollback、TTL crossing、SQLite restart/integrity。
- concurrency：64 publishers/CAS 只产生一个 current next revision；不同 tenant 独立。
- reusable conformance：Store 与 ReviewEligibilityCurrentReader factory suite。
- repeat：targeted ordinary100、race20、full ordinary/race、vet、gofmt、diff/import scan。

## 8. 发布结论

本模块完成后只代表 Organization Owner-local production backend 可用。Review 的 Policy、Runtime Authority/Binding/Evidence/Scope、Runtime Authorization V5 与最终 composition root 仍由各自 Owner 关闭；不得据此宣称 Human multi-sign 整体 production GO。
