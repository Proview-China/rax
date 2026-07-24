# Agent Assembler V1

## 1. 当前裁决

- 状态：设计已确认，允许进入详细 Plan；尚未授权写实现代码。
- Owner：独立 `agent-assembler`，位于 Definition 与 Harness/Runtime 之间。
- 输入：sealed `AgentDefinitionV1` 和一组 exact current resolution facts。
- 输出：sealed `ResolvedAgentPlanV1`、Runtime `BindingPlanV2`、Harness `AssemblyInputV1`。
- 不启动 Agent，不申请 Sandbox，不调用 Provider，不创造组件事实。

现有 Runtime `ResolvedAgentPlan` 仅含少量摘要和旧 `ComponentRequirement`，保留为兼容/reference 路径，不能升级为完整生产计划。新路径使用独立版本并显式复用 Runtime Binding V2 与 Harness Assembly V1。

## 2. 确定性装配链

```text
AgentDefinitionRefV1
  + ResolutionFactsSnapshotV1
  + ComponentReleaseCatalogSnapshotV1
  -> validate exact current facts
  -> resolve version/capability/dependency/locality/owner/residual
  -> ResolvedAgentPlanV1
  -> runtime.ports.BindingPlanV2
  -> harness.assemblycontract.AssemblyInputV1
  -> Harness Compiler
```

解析阶段必须为零外部副作用。Catalog Reader 可以读取已发布事实，但不能下载 artifact、解析 secret 或动态探测 Provider。

## 3. 公共入口

```go
type AgentAssemblerPortV1 interface {
    Resolve(context.Context, ResolveRequestV1) (ResolveResultV1, error)
}

type ResolutionFactsReaderV1 interface {
    InspectExactResolutionFactsV1(context.Context, ResolutionFactsRefV1) (ResolutionFactsSnapshotV1, error)
}

type ComponentReleaseCatalogReaderV1 interface {
    InspectExactComponentReleaseCatalogV1(context.Context, ComponentReleaseCatalogRefV1) (ComponentReleaseCatalogSnapshotV1, error)
}
```

接口签名中的 DTO 由 `agent-assembler` 公共 contract 包拥有；组件 Owner 通过 adapter 发布 `ComponentReleaseV1`。Assembler 不接收实现句柄，也不 import 任一组件实现包。

## 4. 6+1 与自定义组件

首版 Definition 中的 6+1 requirement 均必须解析到 `support_mode=production` 的 exact Component Release。以下状态只能参与开发诊断，不能产生可启动计划：

| support mode | 含义 | 可启动完整 Agent |
|---|---|---|
| disabled | 明确关闭 | 否 |
| reference_only | 参考合同/fake/testkit | 否 |
| standalone | 领域服务可独立运行，但未接 Host 全链 | 否 |
| production | 生产 Port、State Plane、Inspect/Cleanup、Conformance 全闭合 | 是 |

自定义组件通过 Governance Catalog 注册 namespaced kind/capability/schema/extension 后，发布同样的 Component Release；Assembler 不增加 switch，也不允许声明外能力。

## 5. AssemblyInput 映射

Harness `AssemblyPlanRefsV1` 的十个 ref 全部必填：

| Harness ref | Resolved Plan 来源 |
|---|---|
| ResolvedAgentPlan | `ResolvedAgentPlanV1.Ref` |
| HarnessBootstrapPlan | Harness Release 的 bootstrap artifact |
| Profile | Profile Owner exact ref |
| RuntimePolicy | Runtime policy exact ref |
| HarnessStack | Harness stack exact ref |
| SemanticRoute | Route Owner exact ref |
| ContextPlan | Context Owner exact plan ref |
| ToolSurface | Tool Owner exact surface ref |
| CapabilityGrant | Authority/Capability exact grant ref |
| ExpectedInjectionManifest | Context/Injection Owner exact ref |

内容为空不等于 ref 缺失。合法的“不适用”必须由对应 Owner 发布 sealed empty artifact，Assembler 不能用空字符串占位。

`AssemblyInputV1.CreatedUnixNano` 取自 sealed Resolution Facts/Resolved Plan 的冻结时间；相同 Definition 和 facts 的重试必须字节一致，不能读取本次 wall clock。

## 6. Runtime 交接

- `BindingPlanV2` 由 resolved exact releases 派生，包含 artifact、contract、capability 与 governance digest。
- Runtime Binding Fact Owner 重新验证 plan/current manifest，不信任 Assembler 自报。
- Harness 编译产物经 Runtime Generation/Binding Association 绑定后，Host 才能构造组件。
- Sandbox 是 Runtime Activation 的硬前置；即使 Harness 某 slot 可选，完整 Agent 的 production plan 也必须含 Environment Provider。
- Plan 不授予 dispatch、Permit、Effect、Review Verdict 或 Outcome。

## 7. Owner 边界

Assembler 拥有：版本解析、依赖图、确定性选择、plan canonical/digest、映射诊断。

Assembler 不拥有：组件 Manifest/Release、Capability current、事实 TTL、秘密值、Provider 探测、Binding Fact、Assembly Generation、Instance/Run 或业务 Settlement。

详细计划对象见 [resolved-agent-plan-v1.md](resolved-agent-plan-v1.md)，发布目录见 [component-release-catalog-v1.md](component-release-catalog-v1.md)，验收见 [acceptance.md](acceptance.md)，图见 [architecture.drawio](architecture.drawio)。

## 8. 进入 Plan 的门

- [x] 用户确认 Resolved Plan、Component Release 与三输出模型；
- [x] 用户确认首版 6+1 全部 production 才允许启动；
- [x] 用户确认旧 Runtime plan 只作兼容路径；
- [x] agent-host 生命周期和唯一 Composition Root 同时确认；
- [ ] 各组件 Owner 在实施波次分别确认发布 Component Release 的 additive Delta。
