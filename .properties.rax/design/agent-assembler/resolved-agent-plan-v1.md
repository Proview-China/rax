# ResolvedAgentPlan V1

## 1. 版本

| 项 | 值 |
|---|---|
| Owner | `agent-assembler` |
| ContractVersion | `praxis.agent.assembler.plan/v1` |
| ObjectKind | `ResolvedAgentPlanV1` |
| 状态 | immutable sealed object + 独立 current pointer |

## 2. 字段

```text
ResolvedAgentPlanV1
|-- plan_id / revision / digest
|-- definition_ref
|-- identity_ref
|-- profile_ref
|-- policy_refs
|-- sandbox_requirement_ref
|-- component_releases[]
|-- binding_plan
|-- assembly_plan_refs
|-- harness_bootstrap_ref
|-- resolution_facts_ref
|-- catalog_ref
|-- residuals[]
|-- evidence_refs[]
|-- created_unix_nano
`-- valid_until_unix_nano
```

`component_releases` 为 exact refs + public manifest projections；Release 正文由 Owner Reader 提供。`binding_plan` 复用 Runtime `BindingPlanV2`；`assembly_plan_refs` 与 Harness 十 ref 逐字段 exact。

## 3. 不变量

1. Definition、Resolution Facts、Catalog Snapshot 均通过 exact ref 读取并在 S1/S2 复读 current。
2. `valid_until` 不超过所有 required release、capability、policy、approval、catalog current 的最小到期时间。
3. 六个公共组件域加 Harness 共七条 requirement 均为 required + production；缺一 fail closed。
4. 每个 required capability 唯一解析到一个 Provider；多解且无显式优先规则为 Conflict。
5. dependency graph 无环，locality 可满足，Effect/Settlement/Cleanup Owner 完整且唯一。
6. residual 必须显式允许，并携 exact Inspect/Cleanup 合同；首版 required 6+1 不允许以 residual 代替生产能力。
7. 同输入 exact refs 产生同 plan ID/digest、BindingPlan、AssemblyInput；诊断时间不进入语义摘要。
8. Plan 不携 secret value、Provider handle、factory instance、网络地址自由文本或组件私有 DTO。

## 4. 状态与恢复

- create-once：同 PlanID/revision 同内容幂等，换内容 Conflict。
- resolve 回包丢失：按 deterministic PlanID + exact inputs Inspect，不重新读取新版 Catalog。
- current facts 过期或漂移：旧 Plan 保留历史真实性，但不可用于新 activation；必须重新 Resolve 产生新 revision/Plan。
- Assembler 不原地修订 sealed Plan。

## 5. 与旧类型兼容

Runtime `ports.ResolvedAgentPlan`、`ComponentRegistry` 和 v1alpha1 descriptor 只允许 reference/test compatibility。它们不能自动升级为 `ResolvedAgentPlanV1`，也不能声称 production conformance。
