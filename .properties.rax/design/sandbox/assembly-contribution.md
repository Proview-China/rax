# Sandbox Assembly Contribution

## 原则

- 不定义公共 Slot/Hook/Phase 枚举；
- 不实现 Harness 私有 ContextPort/ModelTurnPort/EventCandidatePort；
- 只发布 Observer/Filter/Gate/Port；
- 任何 effectful Port 都绑定 Runtime Operation/Inspect/DomainResult/Settlement/Apply；
- Harness 不获得 Backend handle、Lease/Fence 写权或 Provider Receipt。

## live release贡献

| 公共面 | Sandbox贡献 | 状态 |
|---|---|---|
| `sandbox.execution` | Slot Owner/Provider/Reference、effectful PortSpec、Provider candidate | exact Component Release 已实现 |
| lifecycle | allocate/activate/open/inspect/fence/release/cleanup public Port | descriptor 与 production readiness 已实现 |
| Checkpoint | typed participant Driver、before Gate、after Observer 所需 Port ref | Sandbox handler 已实现；最终 phase 注册由 Agent Host |
| Restore | typed Restore stage participant Port | Sandbox handler 已实现；不复用 activation/run.resume |
| Workspace | capture/commit/rewind governed Port | Sandbox SDK/API 使用 |

## 依赖 DAG

```text
Agent Definition
  -> Agent Assembler Component Release/Catalog
  -> Binding V2 / CompiledGraph
  -> Agent Host factory/provider/phase registry
  -> Sandbox hostroot
  -> Runtime/Application public ports
  -> Rust Data Plane
```

Sandbox `release` 已发布 Module/Capability/Port/Factory/Owner/TTL/Cleanup/readiness exact
descriptors，但 descriptor 不会实例化进程。Agent Host 必须根据 FactoryID
`praxis.sandbox/factory/lifecycle-v4`与`praxis.sandbox/factory/execution-v1`注册可信构造器，
并注入真实 Runtime/Application/Owner readers。

## fail-closed

- 未注册 factory/provider/phase：assembly 或 host startup 失败；
- Binding/Generation/Artifact/Manifest drift：不构造；
- readiness 缺 deployment attestation/certification：只发布 standalone；
- Gate 缺 current Authority/Review/Budget/Scope/Lease/Fence：Provider=0；
- Observer 只产 bounded candidate，不写 Context/Evidence/Runtime Fact；
- 未冻结的新 Phase 保持缺失，不以私有 Hook 替代。

## 外部完成项

Agent Host/Assembler Owner仍需完成真实 registry、merge/conflict/failure policy、deployment
attestation 与 certification。Sandbox 只提供 exact descriptor 和 hostroot factory product，
不反向导入 Agent Host 或自签 production。
