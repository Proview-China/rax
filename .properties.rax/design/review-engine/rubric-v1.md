# Review Rubric V1 冻结设计

## 1. 裁决与边界

- 当前状态：Review Owner 实现、机械门与最终独立复审完成，`P0/P1/P2=0`；跨组件 production root 仍 NO-GO。
- 唯一语义 Owner：Review Owner。
- Review Owner 拥有 Rubric 的权威版本、criteria、rules、输出 Schema、只读 capability 集、终止 ceiling、append-only history、current index、撤销和替代。
- Policy Owner 只选择 `ExactResourceRefV1{ID,Revision,Digest}` 及适用条件；Policy 不复制、修改或重新解释 Rubric 内容。
- Rubric 不授予 Reviewer 身份、Authority、Scope、Binding、Evidence、Budget、Permit、Begin、Dispatch、Commit 或 Provider 调用权。
- Rubric 不是万能 Prompt。Reviewer Context 只能消费结构化 criteria/rules 和闭集只读 capability；任何写入、执行、Spawn 或网络能力均不属于本合同。
- 本切片只形成 Review-owned 单机内存 reference Store 与 SQLite WAL 持久化；未创建跨组件 production composition root、外部 Provider 或 SLA。

## 2. 权威对象

`RubricDefinitionV1` 使用 `praxis.review/v1`，字段如下：

| 字段 | 类型/约束 | 权威语义 |
|---|---|---|
| `FactIdentityV1` | Tenant、稳定 ID、Revision、Digest、Created/Updated | ID 在租户内稳定；首建 revision=1；续版严格 `+1` |
| `Kind` | 七个闭集 kind | 选择专用判断协议，不选择 Route/Profile |
| `Name` | 非空有界文本 | 人类可读名，不参与授权 |
| `Criteria[]` | ID 排序且唯一 | objective、priority、required evidence kinds、失败 resolution |
| `Rules[]` | ID 排序且唯一 | 每条规则 exact 引用已存在 criterion；RuleKind 为闭集 |
| `OutputSchema` | 固定 Attestation/ Finding 结构 | Resolution 闭集、完整 Finding 字段、reason/evidence/conditional digest 必需 |
| `AllowedReadOnlyCapabilities[]` | 排序唯一闭集 | 仅 inspect 类能力；无 write/execute/dispatch/commit/spawn/network |
| `Termination` | MaxRounds/Duration/Tokens/重复阈值 | v1 baseline 为 3 rounds、10 min、64k tokens、重复 Finding=2、重复 Reject=2；Policy 可选择更严格预算，但不得放宽 Rubric ceiling |
| `State` | `active` / `revoked` | 只有 current index 指向的 active revision 可 admission |
| `ExpiresUnixNano` | `Created < Expires` | current TTL 上界；纯时间流逝不创建新 revision |

Digest domain 为 `praxis.review / praxis.review/v1 / RubricDefinitionV1`，绑定全部字段。所有返回值 deep clone。

## 3. 七类 baseline

| Kind | 核心 criteria | 规则重点 | 允许的只读能力 |
|---|---|---|---|
| `action_safety` | Authority、Scope | exact Authority；Scope containment | Target、Evidence、Policy inspect |
| `code_change` | Correctness、Verification | Diff grounding；relevant test signal | Diff、Source、Evidence、Test Result inspect |
| `work_state` | Intent fidelity、Unresolved risk | Intent digest exact；风险必须有 Evidence | Target、Source、Evidence inspect |
| `artifact_quality` | Artifact integrity、Quality evidence | exact Artifact ref；质量 Claim grounding | Artifact、Source、Evidence inspect |
| `outcome_acceptance` | Acceptance coverage、Claim grounding | criteria-to-claim coverage；逐 Claim Evidence | Target、Artifact、Evidence、Test Result inspect |
| `legal_compliance` | Legal authority、Source attribution | accountable approval；可归属来源 | Organization、Policy、Source、Evidence inspect |
| `finance_control` | Financial authority、Financial evidence | accountable approval；来源与计算 Evidence | Organization、Policy、Source、Evidence inspect |

这些 baseline 是最小结构化协议。业务域定制必须发布同一 ID 的新 revision，不能改旧历史，也不能把自由文本 Prompt 当规则替代品。

## 4. Store 状态机与原子性

```text
absent
  -- Publish(expected=nil, revision=1, active) --> active R1

active Rn
  -- Publish(expected=full Rn, active Rn+1) --> active Rn+1
  -- Revoke(expected=full Rn, revoked Rn+1) --> revoked Rn+1

revoked Rn -- no publish/revive --> terminal
```

同一 Owner 锁/SQLite generation CAS 内原子更新：

1. `history[ID][revision]`；
2. `current[ID] = full ExactResourceRefV1`；
3. `highestRevision[ID] = revision`。

三者全有全无。`highestRevision` 与 full-ref current index 共同防止 ABA；同 ID 同 revision 换 digest、revision rollback/gap、stale expected、terminal revive 全部 Conflict。历史 exact Inspect 只读 `(Tenant,ID,Revision,Digest)`，不借 current index。

### 方法

- `PublishRubricV1(ctx, PublishRubricMutationV1)`：首建或 active supersede；同 canonical replay 幂等。
- `RevokeRubricV1(ctx, RevokeRubricMutationV1)`：只允许下一 revision，除 State/Revision/Updated/Digest 外不得改定义内容。
- `InspectRubricExactV1(ctx, tenant, exactRef)`：历史只读 deep clone。
- `InspectRubricCurrentV1(ctx, tenant, exactRef, now)`：原子验证 current full ref + highest + history，并执行 `ValidateCurrent`。

## 5. Request Admission 的 S1/S2

`ReviewRequestV1.Rubric` 只携带 exact ref，不携带 caller current 布尔值或 payload。

```text
S1 fresh baseline
  -> InspectRubricCurrent(exact Request.Rubric)
  -> validate active + exact digest + TTL
  -> validate Request/Target/Result Bundle
S2 fresh clock (must be >= baseline)
  -> InspectRubricCurrent(same exact ref)
  -> exact compare S1/S2
  -> CreateTargetCase compound mutation
       -> under Store lock recheck same current full ref/highest/history
       -> validate Request expiry <= Rubric expiry
       -> publish Request+Target+Case+optional ResultBundle+Trace atomically
```

因此 Rubric 在 S1/S2 之间或 S2/Store 线性化点之前被 supersede/revoke 时，Case/Target/Request/Trace 全部零写。Rubric TTL 是 Request/Case TTL 的硬上界；后续 Reviewer Round 仍须 exact Inspect 同 Rubric revision，不能 by-name/latest。

## 6. 时间、撤销与恢复

- `ValidateCurrent(expected, now)` 要求 exact ref、`State=active`、非零 now、`now >= Created`、`now < Expires`。
- Service 在 current read、Publish、Revoke 和 Admission 读取前后使用 baseline/fresh clock；`fresh < baseline` Fail Closed。
- 纯时间过期只使 `ValidateCurrent` 失败，不创建 `expired` revision，也不改历史。
- mutation 返回 `Unavailable/Indeterminate` 时，只用 `context.WithoutCancel` exact Inspect 原 `Next.ExactRef()`；禁止再次调用 Publish/Revoke、换 ID、换 revision 或重新 resolve current。
- historical exact Inspect 不受后来 supersede/revoke/expiry 影响。

## 7. Closed errors

| 条件 | Error category / reason | 写入 |
|---|---|---|
| 空/坏 ref、未知 kind/rule/capability、非 canonical 排序 | InvalidArgument | 0 |
| current ID 不存在 | NotFound / InvalidReference | 0 |
| same revision 换 digest、stale expected、current/highest/history 漂移 | Conflict / RevisionConflict 或 IdempotencyPayloadMismatch | 0 |
| revoked/expired | PreconditionFailed / ReviewVerdictStale | 0 |
| clock zero/rollback | PreconditionFailed / ClockRegression | 0 |
| ctx cancelled/deadline、SQLite outcome unknown | Unavailable/Indeterminate | 只 exact Inspect 原 mutation |

## 8. 兼容与迁移

- `ReviewRequestV1.Rubric` 字段形状不变，旧 caller 不需要新的 DTO；语义由“仅 shape-valid exact ref”收紧为“必须命中 Review-owned active current”。
- Snapshot 通过可选 `rubrics_v1` 子快照增量扩展；旧 snapshot 缺该字段等价为空 Rubric Store，不被伪造为 current。
- SQLite 继续复用现有 tenant snapshot + generation CAS，无新跨 Owner schema、无新数据库产品承诺。
- 未预发布 Rubric 的旧请求现在 Fail Closed；迁移顺序必须是 Owner 发布 Rubric，再让 Policy 发布/选择 exact ref，最后提交 Request。

## 9. Effect / Settlement / Root

Rubric publish/revoke 是 Review State Plane 内部事实 mutation，不是外部 Provider Effect；Provider 调用数为 0，不产生 Runtime Operation Settlement。Reviewer 若用任何远程或真实执行能力，仍走既有 Runtime Governance/Settlement，Rubric 只限制允许的只读 capability。当前 production root 仍由宿主联合线负责，本资产不声明 root GO。
