# Review Consumer Port Delta V1

## 状态

状态更新：Organization Owner 的 `ReviewEligibilityCurrentReaderV1` 已实现；Review Owner 已在live `ExecutionRuntime/review/ports/human_organization_current_v2.go`冻结公共consumer Port，并由`review/multisigcurrent/organization_v2.go`完成缺失/陈旧fail-closed适配。本文件保留为已完成Delta的历史依据；Organization仍不创建反向依赖Review的私有Adapter。

## 用例与 Owner

- 用例：Review Panel/Attestation/Quorum actual-point 需要复读 Reviewer Identity、全部 required Role、显式 Delegation、Responsibility Subject，并验证 production self-review 禁令。
- 事实 Owner：Organization Engine。
- 聚合/计票/Verdict Owner：Review Engine。
- composition Owner：`agent-host`；只负责接线和逐字段 nominal mapping，不签发事实。

## 已有 Organization 输出

```go
type ReviewEligibilityCurrentReaderV1 interface {
    ResolveCurrentReviewEligibilityV1(context.Context, contract.ReviewEligibilitySourceV1) (contract.ReviewEligibilityCurrentProjectionV1, error)
    InspectCurrentReviewEligibilityV1(context.Context, contract.ReviewEligibilityProjectionRefV1) (contract.ReviewEligibilityCurrentProjectionV1, error)
}
```

Projection 已包含：

- Reviewer `IdentityFactV1` exact ref；
- 每个 required role 的 `RoleGrantFactV1` exact ref、Role、ScopeDigest、CanVeto；
- 可选 `DelegationFactV1` exact ref、Delegator/Delegate Identity exact refs、Role、ScopeDigest；
- `ResponsibilityFactV1` exact ref、author Identity exact ref、subject kind/id/digest；
- fixed `CheckedUnixNano`、真实 closure min `ExpiresUnixNano`、`ProjectionDigest`、`Current=true`。

## Review Owner 最小 Delta

Review 应冻结一个只读 consumer Port，参数使用 Review 自有 expected Panel/Assignment/Target nominal coordinates；返回值只承载 Organization projection 的 exact refs/current window，不复制 Organization fact body。建议方法语义：

```text
ResolveHumanOrganizationCurrentV2(ctx, exact Assignment source) -> sealed expected projection ref
InspectHumanOrganizationCurrentV2(ctx, exact projection ref) -> deep-cloned current projection
```

逐字段 nominal mapping：

| Organization source | Review public carrier |
|---|---|
| Identity `{TenantID,ID,Revision,Digest}` | `HumanIdentityProofRefV2 {TenantID,Ref,Revision,Digest}` |
| Delegation `{TenantID,ID,Revision,Digest}` | `HumanDelegationFactRefV2 {TenantID,Ref,Revision,Digest}` |
| Responsibility exact + Identity exact | `HumanResponsibilitySubjectRefV2` |
| RoleGrant.Role / CanVeto / ScopeDigest | Assignment roles/veto eligibility 的 current proof；不得写入 Authority ref |
| Checked/Expires/ProjectionDigest | Review S1/S2/min TTL 输入 |

## 不变量

1. Review 不得按 display handle/by-name/latest 查询；S1 Resolve 后 S2 必须 exact Inspect 同 ProjectionRef。
2. mapping 不改变 ID/revision/digest，不重封 Organization digest。
3. Role Grant 不是 Runtime Authority；Review 仍须独立复读 Runtime Authority current。
4. Organization self-review Forbidden 是前置 proof；Review 仍需对 Assignment/Attestation exact binding 再校验。
5. 任一 missing reader、unknown role、terminal/expired/drift/clock rollback Fail Closed。

## Effect / Recovery

- current read 无外部 Effect；projection 是 Organization Owner create-once derived fact。
- exact Inspect lost reply：使用原 ProjectionRef detached read-only recovery；不重调 mutation。
- 无 expected ref 的 Resolve unknown：开启新 S1，不宣称恢复原结果。

## 反例

- Review 用 Identity display name 构造 `HumanIdentityProofRefV2`；
- 把 RoleGrant `CanVeto=true` 当 Runtime Authority；
- 把 Delegation nominal ref 当 current，不执行 S2；
- Responsibility author revision 漂移后仍允许旧 Reviewer；
- Review 或 Organization 互相导入 implementation，形成 SCC；
- Adapter 每次读取重新 seal Checked/Digest。

## 兼容影响

- 对 Human Multi-Sign V2 是版本化加法；不改变 V1 单 Reviewer 历史合同。
- Organization public Reader 已稳定，未来 Review consumer Port 只需 host adapter 逐字段投影。
- Review consumer Port已冻结；production composition root、Organization Release与Human Multi-Sign Review Release依赖variant未闭合，因此Human multi-sign production integration仍为NO-GO。
