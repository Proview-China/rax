# Agent Builder → AgentPackage V1 公共合同

## 1. 已冻结方向

Agent Builder 是 Go Agent Framework 的代码式与声明式构建入口。它不新增平行 `AgentDefinition`：YAML/JSON 与代码式 Builder 均只生成现有 `AgentDefinitionSourceV1`，随后继续复用权威链：

```text
AgentDefinitionV1
  -> ResolvedAgentPlanV1
  -> AssemblyInputV1
  -> CompiledHarnessGraphV1
  -> AgentPackageV1 + AgentPackageLockManifestV1
```

## 2. 首个公共合同切片

- `AgentPackageRefV1`：`package_id + revision + digest + contract_version + schema_version`；
- `AgentPackageLockManifestV1`：锁定 Definition、Resolution Facts、Catalog、全部 selected Component Release、Resolved Plan、Binding Plan、Assembly Input；
- Harness 产物闭包：Generation、Manifest、Graph、Handoff 均提供 `ID + revision + digest` exact ref；
- `CompilerV1` 只接受已验证的现有 `ResolveResultV1`，所有 lock 字段从该闭包派生，调用方不能另传 release/facts/catalog/binding；
- Package ID、revision、digest 与 lock 一一确定；冻结时间来自 lock，不读取编译时 wall clock。

## 3. Fail Closed

Definition/Plan/Assembly、BindingPlan、四份 Harness 产物任一 exact ref、digest、revision、compiler version 或交叉绑定漂移均拒绝。集合按 canonical 规则排序，nil/empty 语义固定；重复 Release ID 拒绝。

## 4. 明确不做

本切片不实现 Host、Runtime、Loop、Instance、Run、Provider、Sandbox 激活、Secret 读取、TS Wire DTO、Praxis Console 接线、Canvas `x/y/viewport`、静态 label/capability 枚举或 Draft 保存。

AgentPackage 只证明编译与 provenance 闭包，不授予 activation、dispatch、Permit、Effect、Settlement 或 production/SystemReady 资格。
