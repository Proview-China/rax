# Agent Host 组件准入真值表 V1

## 1. 裁决口径

本表记录 Agent Host H4 当前可读取的声明式组件产物，不把“代码支持某种发布模式”写成“该模式已经发布”。截至 2026-07-18，11 个目标域都已有真实 `release/` 或 `releasecandidate/` 包；它们仍未形成任何 live production publication。

| 术语 | 精确含义 |
|---|---|
| `assembly-candidate` | 已能形成 Agent Assembler 可校验的 `ComponentReleaseV1` 声明；只证明声明闭包，不证明可执行构造或生产资格 |
| `reference_only` | live Release 的支持模式固定或默认停在 reference-only；只能用于结构、合同和装配验证 |
| `standalone` | Owner-local 软件闭包可独立运行或可在 exact local readiness 下发布 standalone；仍未进入 Agent Host production root |
| `production` | 真实 durable backend、Provider/current、可执行 Factory、cleanup、deployment 和 certification 在同一 current cut 闭合 |
| `not-published` | 尚无可供 Assembler 读取的 Component Release；不具备组件准入资格 |

必须区分两个对象：

```text
ModuleFactoryDescriptorV1
  = 声明“需要怎样构造、输出什么能力、由谁清理”
  != 可调用的 Go/TS/Rust constructor
  != 已绑定 backend/provider 的 executable factory
  != production composition root
```

所以，`FactoryDescriptors` 非空只表示 descriptor closure；没有可信 executable factory binding、真实依赖注入和 cleanup conformance 时，Host 不得构造组件。

## 2. Live 11 项准入表

| # | 域 / live 包 | 当前最高可证层级 | live 真值 | production P0 |
|---:|---|---|---|---|
| 1 | Continuity / `continuity/releasecandidate` | `assembly-candidate` + `reference_only` | 固定 reference-only；descriptor factory，不含 Host 构造或 promotion API | durable checkpoint/timeline/artifact/history/restore stores；remote blob provider；Participant capture；Restore execute；cleanup/purge/archive；deployment/root attestation |
| 2 | Tool + MCP / `tool-mcp/release` | `assembly-candidate`；exact local readiness 下可 `standalone`，无 local readiness 时 `reference_only` | 具备两项 Capability/Port/Factory descriptor 和三模式发布合同；仓内没有真实 production readiness 发布 | durable Action/Binding/Surface/MCP stores；Credential current；Provider transport/current；controlled actual-point；Evidence/Settlement；MCP lifecycle/Inspect；cleanup；deployment；独立 certification |
| 3 | Memory + Knowledge / `memory-knowledge/release` | `assembly-candidate`；exact local readiness 下可 `standalone`，无 local readiness 时 `reference_only` | Memory/Knowledge 保持两个独立 Capability/Port/Factory descriptor；reference store 与 fixture 不是 durable proof | durable Memory/Knowledge fact+content stores；Authority/Policy；Credential；Index/Context current；Settlement；Purge Effect；cleanup；deployment；独立 certification |
| 4 | Sandbox / `sandbox/release` | `assembly-candidate` + `standalone` | base publication 为 standalone；production 路径必须读 exact readiness，当前没有真实 readiness 发布 | durable State Plane；真实 Environment/remote Provider transport；placement/enforcement current；checkpoint compatibility；Host executable factory；deployment attestation；独立 certification |
| 5 | Review / `review/releasecandidate` | `assembly-candidate` + `reference_only` | 固定 reference-only，无 production promotion API；已有服务/SQLite切面不等于 production root | Decision/Verdict/Policy/Evidence/Authority/Scope 同一 current cut；durable store；remote review Effect；Human intervention；cleanup；external-current composition；deployment certification |
| 6 | Context + Cache / `context-engine/releasecandidate` | `assembly-candidate` + `reference_only` | 固定 reference-only；offline SDK、owner-local refresh 与 reference stores 不构成生产状态面 | durable state/cache；Source/Provider current；Harness per-turn injection/continuation；Turn推进；cleanup；composition/deployment qualification |
| 7 | Organization / `organization-engine/release` | `assembly-candidate`；exact local readiness下可`standalone`，无local readiness时`reference_only` | Release publisher、Capability/Port/Factory descriptor与三模式合同已存在；仓内没有真实production readiness发布或Host root注入 | durable organization facts、Authority/Review consumer current、cleanup、executable factory、deployment/certification |
| 8 | Harness / `harness/releasecandidate` | `assembly-candidate` + `reference_only` | 固定 reference-only；Assembly/Route/CommitGate 已是 owner-local软件事实，但 Factory 仍只是 descriptor | durable session/event/assembly/route/gate stores；production Route与Provider no-bypass；真实 Application/Tool/Context/Model actual-point接线；Continuation/Checkpoint；cleanup；executable factory；root/deployment proof |
| 9 | Model Invoker / `model-invoker/releasecandidate` | `assembly-candidate` + `reference_only` | 固定 reference-only；Prepared Invocation、Projection与Gate ACK是候选事实，不授 Provider 调用或production资格 | durable prepared/current/ACK repositories；production profile/route/credential/current；全部 Provider path actual-point guard；Harness bridge；cleanup；executable factory；deployment/certification |
| 10 | Runtime / `runtime/releasecandidate` | `assembly-candidate` + `reference_only` | 固定 reference-only；部分 SQLite 与大量公共 Fact/Gateway 不能替代完整 Runtime production闭包 | Command/Desired/Outbox；Identity/Activation；Run/Effect/Evidence/Settlement；Checkpoint/Restore 全持久后端；scheduler/supervision workers；cleanup；executable factory；deployment/root attestation |
| 11 | Application / `application/release` | `assembly-candidate`；exact local readiness 下可 `standalone`，无 local readiness 时 `reference_only` | 六项协调能力有独立 Capability/Port/Factory descriptor；Effect/Settlement Owner 明确属于 Runtime，Application 只编排且拥有 cleanup | durable command/outbox、journal、attempt、run、G6A、context、checkpoint stores；Outbox/Recovery workers；Runtime gateways；cleanup；executable root；deployment；独立 certification |

### 2.1 当前统一结论

- 11 个目标域都已有 live Release/ReleaseCandidate 声明面；是否进入 Catalog 仍取决于Owner真实publication与current。
- Sandbox 当前可证到 standalone；Tool+MCP、Memory+Knowledge、Application 的代码支持“读取 exact local readiness 后发布 standalone”，但这不等于 Agent Host 已持有相应 live publication。
- Continuity、Review、Context、Harness、Model Invoker、Runtime 固定为 reference-only assembly candidate。
- **当前 11 项中没有任何一个真实发布了 production readiness，也没有任何一个获得 production support mode 的 live catalog 准入。** 测试构造的 production projection 只验证合同反例与状态机，不能成为生产事实。

## 3. Agent Host 的准入算法

```text
Release/ReleaseCandidate
  -> 校验 ComponentManifestV2 / artifact / contract / schema / capability
  -> 校验 support_mode 与当前 readiness/certification exact 一致
  -> 校验 dependency DAG / locality / residual / credential / owner
  -> 校验 FactoryDescriptor 与 PortSpec 只形成声明闭包
  -> 查找可信 executable factory binding
  -> 复读 production current + cleanup + deployment + certification
  -> 全部通过后才允许 H4 production construction
```

任一步缺失都 Fail Closed。Host 不得：

- 把 `assembly_candidate` 状态翻译成 `production`；
- 把 standalone CLI、owner-local store、fixture、fake、测试 projection 或文件存在性当生产证明；
- 根据 Factory ID 动态 import 包、脚本、URL 或 raw Provider；
- 替领域 Owner 签发 readiness、Effect、Settlement、Verdict 或 cleanup 事实。

## 4. 共同 production 硬字段

每个 production Release 最少必须在同一 current/certification cut 闭合：

- exact ComponentManifestV2、artifact、contract、schema、capability、locality；
- module/slot/phase/port/factory/provider contribution 与依赖 DAG；
- 真实 executable factory binding，不是仅有 descriptor；
- Effect/Settlement/Cleanup Owner 与可恢复的 durable facts；
- credential、Authority、Policy、Budget、Scope、Fence 和 Provider current；
- residual/unknown/cleanup 的 Inspect 与 settlement；
- deployment attestation、no-raw-provider-bypass 与独立 certification；
- production root 对上述事实的 fresh reread，而不是测试时一次性注入。

## 5. 后续波次

```text
P4a  已完成：11个目标域的Release/ReleaseCandidate声明面
P4b  待完成：各Owner真实local/production readiness producer与durable backend
P4c  待完成：可信executable factory registry + owner adapter
H4   待完成：Agent Host按依赖顺序构造、激活、恢复、清理
H5   待完成：CLI/API + all-6+1 production backend/system gates
```

P4a 完成只代表“可以开始声明式装配验证”，不代表可从简单配置直接构造并运行 production Agent。P4b-P4c、H4-H5 全绿之前，`SYSTEM_READY` 与 production root 均为 NO-GO。
