# Agent Builder 模块入口

`ExecutionRuntime/agent-builder` 提供现有 AgentDefinition 的代码式/声明式作者入口，以及把现有 Assembler/Harness 编译闭包封装成不可变 AgentPackage 的纯 Go API。

公共输出 `AgentPackageRefV1` 可作为后续 Host 合同的 exact 输入；lock 可追溯 Definition、Facts、Catalog、Releases、Plan、Binding、Assembly、Harness Publication 与 Generation/Manifest/Graph/Handoff。模块提供 Package SQLite Repository 和 exact historical Publication Loader，但不拥有 Harness Store，也不包含生命周期、运行面或生产资格。

代码说明见 `ExecutionRuntime/agent-builder/README.md`，设计见 `.properties.rax/design/agent-builder/README.md`，验收计划见 `.properties.rax/plan/agent-builder/README.md`。
