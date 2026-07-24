# Harness Route绑定与公共引擎接线

> 状态：已废止的早期草案。该草案把本地Loop与Runtime Adapter组合误作主接线路线，无法覆盖定稿要求的Slot、HookFace、Assembly SDK与三段装配链。不得据此实现；后继设计见[Harness Assembly公用接线总设计](../assembly/README.md)。

## 1. 状态与目标

本切面在已经验收的Harness公共合同、Run内Kernel和Runtime Adapter之上增量落地，不重写既有合同，也不选择具体Codex、Claude、ACP或第三方生产Route。

目标是补齐一条可自动验证的本地参考接线：调用方提交已经编译完成的`Manifest`和实际Route表面，公共引擎先比较Expected/Actual摘要与治理能力，再同时产生Data Plane的Interaction Loop和宿主侧Runtime `ExecutionPort` Adapter。Fake只用于测试，不代表生产Backend、进程拓扑或SLA。

## 2. Route绑定输入

`RouteBinding`只表达已完成选择后的实际绑定，不负责Profile合成或Route选择。它必须提供：

- Semantic Route、Harness Stack、Injection Manifest、Context Plan和Tool Surface实际摘要；
- 实际Conformance和Control能力；
- 已绑定的`ContextPort`、`ModelTurnPort`和`EventCandidatePort`；
- 本地参考引擎的显式事件与Turn上限。

上述摘要必须与`BootstrapPlan`逐项相等；Conformance必须与Harness Manifest一致且满足最低要求；Control能力不得暗增或缺失。任一漂移在创建Loop、调用Context或派发Model Turn之前Fail Closed。

`RouteBinding`是组装输入，不是Runtime权威事实，也不把Provider自报升级为Attestation或Authoritative Fact。生产Route仍必须由领域Owner提供独立Probe、Evidence和Inspect。

## 3. 公共引擎产物与部署边界

公共引擎只提供本地参考组合：

```text
compiled Manifest + verified RouteBinding
  -> Data Plane Interaction Loop
  -> host-facing Runtime ExecutionPort Adapter
```

这不预设生产环境必须同进程。远程Execution Surface可以通过同一Runtime `ExecutionPort`合同提供独立实现；不得为了复用本地参考组合而绕过Sandbox、Secret Broker、宿主Gateway或实际执行点门禁。

公共引擎不拥有Runtime Run、ExecutionOutcome、Identity、Sandbox Lease、Review Verdict、Budget事实或Effect终态。

## 4. 线性化补强

本切面同时修复两个不改变公共Schema的实现缺口：

1. `AgentRunID`在整个Loop内不可跨Execution Scope并发复用；Context准备后的最终提交必须再次检查RunID和Scope单活跃索引，避免并发覆盖Session；
2. Model失败等路径即使向Runtime返回错误，只要Harness已经建立Run Session，Runtime Adapter仍保存该Run关联，使后续`Inspect(state/events)`能够看到失败Claim，而不是误报idle。

Harness失败Claim仍只是Observation；Runtime或领域Owner必须独立Inspect并CAS后才能形成正式Outcome。

## 5. 验收合同

- 单元/白盒：Route摘要、Conformance、Controls、Port和上限逐项漂移均在外部调用前拒绝；
- 黑盒：只通过公共引擎返回的Runtime `ExecutionPort`完成Preflight、Open、Start、Inspect与Close；
- 故障注入：Event写失败不得派发Model；Model失败后Runtime边界仍可Inspect失败Run；
- 并发：不同Scope可以并发执行，但同一RunID只能由一个Scope提交；
- Conformance：`fully_controlled`与`restricted_controlled`均通过，同一绑定不得降级或虚报；
- 最终门禁：普通、20轮稳定性、Race、Vet和跨包覆盖率。

## 6. 本切面不做

- 不导入或修改`ExecutionRuntime/model-invoker/**`实现包；
- 不选择真实模型、账号、Secret、Tool/MCP、数据库、RPC或进程拓扑；
- 不接管六条领域组件的语义或复制其类型；
- 不实现Checkpoint/Restore、跨Run Session或Unknown Outcome自动重试；
- 不修改Runtime Core/Ports。Runtime Cleanup独立Inspect缺口见本组件提交的Port Delta。
