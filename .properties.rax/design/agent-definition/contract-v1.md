# AgentDefinition V1 合同

## 1. 版本和 Owner

| 项 | 冻结值 |
|---|---|
| 语义 Owner | `agent-definition` |
| ContractVersion | `praxis.agent.definition/v1` |
| ObjectKind | `AgentDefinitionV1` |
| 作者输入 | `AgentDefinitionSourceV1` |
| sealed 对象 | `AgentDefinitionV1` |
| exact ref | `AgentDefinitionRefV1` |

Source 不含 `Digest`、Owner 创建时间或审批结果。sealed 对象不可由作者自签。

## 2. 最小字段集

```text
AgentDefinitionSourceV1
|-- contract_version
|-- definition_id
|-- revision
|-- identity_ref
|-- profile_selection_ref
|-- components[]
|-- policy_refs
|-- secret_refs[]
|-- provenance_ref
|-- approval_ref
|-- effective_window
|-- extensions[]
`-- change_reason

AgentDefinitionV1
|-- <全部 Source 字段的规范化值>
|-- created_unix_nano
|-- source_digest
`-- digest
```

### 2.1 ComponentRequirementV1

| 字段 | 语义 |
|---|---|
| component_id | 严格 namespaced 稳定 ID |
| kind | namespaced kind；允许治理目录注册的自定义 kind |
| semantic_version | SemVer 范围，不是浮动 `latest` |
| contract_name / contract_version | 版本化公共合同约束 |
| required_capabilities | 稳定排序、去重的 namespaced capability 集合 |
| required | 首版 6+1 固定为 true |
| support_mode | `production`；其他模式只允许开发诊断，不满足首版运行 |
| locality_constraint | 允许的部署区，不指定进程拓扑 |
| residual_policy | residual 是否允许及其 Inspect/Cleanup Owner 要求 |
| dependency_ids | 只引用本 Definition 中的稳定 requirement ID |

### 2.2 PolicyRefsV1

必须显式携带 Runtime、Authority、Review、Budget、Sandbox、Context、Continuity、Tool/MCP、Memory/Knowledge 的策略引用。`operation_not_required` 也必须引用显式 Policy Fact，不能靠空值推断。

### 2.3 SecretRefV1

一等 Secret 字段只允许 `secret_id + class + requested_scope_digest`。明显 token、password、private key、环境变量值和本机绝对文件路径 fail closed；该粗筛不覆盖任意 opaque 内容，也不构成无秘密证明。

### 2.4 ExtensionV1

扩展必须有 namespaced key、required 标志、schema ref、bounded opaque payload。未知 required 扩展 fail closed；未知 optional 扩展仅以 opaque/untrusted 形式保留，不能影响治理、获得能力或进入 trusted production resolution。

`RegisteredExtensionKeys` 只表达声明期 key 注册。当前 `SchemaRefV1` 与 `ContentDigest` 校验能证明 payload 自洽，不能证明 schema 已被治理目录 exact 绑定，也不能替代专属语义 validator。registered required extension 在进入生产解析前，仍必须由未来的治理目录 exact schema binding 与 Extension Validator Port 验证；V1 owner-local 候选不宣称该生产门已完成。

## 3. Ref 和摘要

`AgentDefinitionRefV1` 至少包含：

```text
DefinitionID + Revision + Digest
```

摘要域固定为：

```text
domain  = praxis.agent.definition
version = v1
type    = AgentDefinitionV1
```

`digest` 排除展示 annotation，包含全部治理字段、版本、有效窗口、extension 内容摘要和 `source_digest`。`created_unix_nano` 由 Owner 注入并进入摘要；同一 create-once 请求回包丢失后必须按 exact ref Inspect，不得以新时间重建同 ID 内容。

新 revision 写入采用 Approval S1/S2 门：先用 `now1` 验证 S1，再按 Source 中同一 exact `ApprovalRef` 复读 S2；S1/S2 全字段必须一致。S2 返回后读取 fresh `now2`，要求 `now2 >= now1`，并以 `now2` 再验证 S2 currentness；`created_unix_nano = now2`。任一步 drift、TTL crossing 或 clock rollback 都必须零写。

## 4. 状态和变更

- Definition revision immutable；修改产生同 DefinitionID 的更高 revision。
- 同 ID + revision + 同 canonical 内容为幂等；换内容为 Conflict。
- 旧 revision 可历史读取，但不能被当作 current。
- 撤销/过期由独立 current 指针或状态事实表达，不修改历史对象。
- 不允许默认迁移、字段补全、隐式组件降级或跨版本 type-pun。

## 5. 失败类别

| 场景 | 结果 |
|---|---|
| YAML 不在安全子集 | InvalidArgument |
| 同 ID/revision 换内容 | Conflict |
| 未知 required extension/kind/capability | PreconditionFailed |
| required 6+1 条目缺失或非 production | PreconditionFailed |
| 引用不存在 | NotFound |
| Reader 不可用 | Unavailable |
| current 无法判定 | Indeterminate |

错误复用 Runtime `core.DomainError` 类别，不创建平行错误体系。
