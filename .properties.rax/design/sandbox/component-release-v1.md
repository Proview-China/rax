# Sandbox Component Release V1 与 production readiness 门

## 1. 状态

状态：`implemented-candidate / production-deployment-no-go`。

本切面让 Sandbox Owner 直接发布 Agent Assembler 公共
`ComponentReleaseV1`，不复制 Assembler/Harness/Runtime 类型。当前仓库已具备
Rust Data Plane、containerd/Wasmtime 与代码级 Application/Runtime composition，
但没有可由宿主复读的持久 State Plane 事实和部署认证。因此当前 live 环境只能发布
`support_mode=standalone` 的完整 assembly candidate，不能进入首版 Agent 的
全 6+1 production 解析结果。

## 2. 完整构造闭包

同一 release revision 封存以下对象：

| 对象 | Sandbox 发布内容 | Owner边界 |
|---|---|---|
| Manifest | `praxis/sandbox`、lifecycle V4、locality、三Owner、artifact、TTL、conformance、residual | Binding/Assembler只读取，不构造Sandbox事实 |
| Capability | `praxis.sandbox/lifecycle-v4`及request/result schema | 只描述能力，不授执行权 |
| Module | exact Manifest ref、artifact、publisher/source、owner集合 | 不携带Go/Rust句柄 |
| Port | start-or-inspect、Scope/Inspect/DomainResult/Runtime Settlement/ApplySettlement合同 | Permit/Enforcement/Evidence仍由各Owner治理 |
| Factory | trusted Go host adapter descriptor与cleanup contract | `agent-host`实现factory registry；Sandbox不反向import Host |
| Evidence/Certification | production时绑定全部readiness evidence与独立Certification Fact | Sandbox不能自签production |

Remote Provider仍只能位于Factory背后的受控transport；Factory descriptor不是raw Provider
句柄，Host composition root必须注入真实transport factory、Runtime Gateway与durable Store。

## 3. 无boolean捷径的readiness

`SandboxProductionReadinessProjectionV1`必须逐项封存：

- release ID/revision、artifact digest、production Manifest digest；
- production composition；
- durable Fact Store与Current Store；
- Lease current、Policy current、Sandbox current；
- Enforcement Gateway、Evidence Governance、Settlement current；
- Provider transport、Provider inspect、Data Plane durable journal；
- Cleanup Owner、deployment attestation、独立Certification Fact；
- Checked/Expires与canonical digest。

这些角色不得复用同一proof ID。Publisher执行S1/S2复读；任一Ref、revision、digest、TTL
漂移或时钟回退均在Catalog写入前Fail Closed。Certification Fact必须精确等于
Agent Assembler对完整production release payload计算的certification digest；Publisher
不能自行生成一个不存在的认证事实。

readiness明确NotFound时可发布`standalone` candidate；Unavailable/Indeterminate不能降级为
NotFound。promotion必须使用更高release revision，同revision换artifact/readiness/content一律Conflict。

## 4. 恢复与并发

Sandbox Publisher只调用Assembler Owner的exact Catalog Port：

```text
Inspect readiness S1
  -> build exact release closure
  -> Inspect readiness S2
  -> EnsureExact ComponentRelease
  -> lost reply: InspectExact same ReleaseRef
```

lost reply后不换ID/revision/content；64个Publisher共享Catalog时只能线性化一份exact release。
返回值必须deep clone，caller修改不能污染Catalog事实。

## 5. 生产NO-GO

当前production阻断是宿主尚未发布完整readiness Projection：Sandbox SQLite State Plane和
Host root已实现，但Agent Host factory/transport注册、deployment attestation与独立
Certification Fact仍不存在。`internal/testkit`、单元测试reader或本地进程存活不能生成这些事实，
因此release仍只能从`standalone`开始，不能由Sandbox自升`production`。
