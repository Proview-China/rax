# Agent Builder / AgentPackage V1

本模块是代码式与声明式 Agent 构建入口的最小公共合同。声明式 YAML/JSON 和代码式 Builder 都只产生现有 `AgentDefinitionSourceV1`；它们不会创建第二套 AgentDefinition，也不能自签 Definition。

纯编译链固定复用：

```text
AgentDefinitionV1 -> ResolvedAgentPlanV1 -> AssemblyInputV1
  -> CompiledHarnessGraphV1 -> AgentPackageV1 + lock manifest
```

`AgentPackageRefV1` 是下游最小不可变引用：package ID、revision、digest、contract version 和 schema version。`AgentPackageLockManifestV1` 锁定 Definition、Resolution Facts、Catalog、Component Releases、Resolved Plan、Binding Plan、Assembly Input 以及 Harness Generation/Manifest/Graph/Handoff 的 exact refs 或 digest。相同输入产生相同 package；任一上游 ref、digest 或 compiler version 漂移都 Fail Closed。

Package ID 使用完整规范化 lock SHA-256 hex，不截断。代码式 `AddComponent`、`AddSecretRef`、`AddExtension` 在接收时即深拷贝输入，调用方后续修改 slice、map 或 payload 不会改变 Builder 内部状态。

当前只提供解析、代码式 Source Builder、上游 ResolveResult 适配和纯 Harness 编译/封装。它不提供 Definition Owner、Catalog/Plan Store、Host、Runtime、Loop、Provider 调用、Sandbox 激活、Secret 读取、发布或运行资格。

验证：

```bash
cd ExecutionRuntime/agent-builder
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
```
