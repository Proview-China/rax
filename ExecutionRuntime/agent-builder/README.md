# Agent Builder / AgentPackage V1

本模块是代码式与声明式 Agent 构建入口的最小公共合同。声明式 YAML/JSON 和代码式 Builder 都只产生现有 `AgentDefinitionSourceV1`；它们不会创建第二套 AgentDefinition，也不能自签 Definition。

纯编译链固定复用：

```text
AgentDefinitionV1 -> ResolvedAgentPlanV1 -> AssemblyInputV1
  -> CompiledHarnessGraphV1 -> AgentPackageV1 + lock manifest
```

`AgentPackageRefV1` 是下游最小不可变引用：package ID、revision、digest、contract version 和 schema version。`AgentPackageLockManifestV1` 锁定 Definition、Resolution Facts、Catalog、Component Releases、Resolved Plan、Binding Plan、Assembly Input、Harness `AssemblyPublicationRefV2` 以及 Generation/Manifest/Graph/Handoff 的 exact refs 或 digest。相同输入产生相同 package；任一上游 ref、digest 或 compiler version 漂移都 Fail Closed。

Compiler 必须调用 Harness `NewAssemblyPublicationBundleV2`，从真实 Publication Bundle 同时取得 Publication ref 与四个 artifact refs；AgentPackage 不自行推导半套发布对象。这样 Package Loader 能直接消费 Harness Owner 已提交的历史发布资产。

Package ID 使用完整规范化 lock SHA-256 hex，不截断。代码式 `AddComponent`、`AddSecretRef`、`AddExtension` 在接收时即深拷贝输入，调用方后续修改 slice、map 或 payload 不会改变 Builder 内部状态。

本模块还提供 create-once SQLite WAL Package Repository 与 exact Loader。Repository 只拥有 Package：完全相同写入幂等，同坐标不同 body/digest 冲突；写回复丢失只能 Inspect 原 Package ref。Loader 只依赖一个结构兼容 Harness `HistoricalReaderV2` 的窄接口，按锁定的 exact Publication ref 重读完整 Bundle，再由 Harness 合同重算并验证 Publication、四个 artifact、input/compiler/handoff closure。未提交、缺失或拼接的 Publication 均 Fail Closed。Loader 返回的 verified closure 仍不是 executable 或 authorized runtime object。

Agent Builder 不实现 Harness Publication Store；黑盒直接使用 Harness 自有 SQLite Publisher/HistoricalReader。当前仍不提供 Definition Owner、Catalog/Plan Store、Host、Runtime、Loop、Factory 实例化、Provider 调用、Sandbox 激活、Secret 读取、Console/TS DTO 或运行资格。

验证：

```bash
cd ExecutionRuntime/agent-builder
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
(cd tests/blackbox && go test -count=1 ./... && go test -count=1 -race ./... && go vet ./...)
```
