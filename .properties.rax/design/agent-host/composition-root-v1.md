# Agent Host Composition Root V1

## 1. 唯一职责

Composition Root 是整个进程唯一可以同时看见下列公共接口的地方：

- Agent Definition decoder/reader；
- Agent Assembler；
- Harness Assembly compiler/adapter；
- Runtime Control/Kernel 的公共 Port；
- Application Coordinator 的公共 Port；
- 各 6+1 Owner 的 public factory/adapter；
- production State Plane、Sandbox 与 Remote Provider adapter。

“可以看见”只用于构造和注入，不授权 Host 调用领域写口或绕过 Application/Runtime 路径。

## 2. HostConfigV1

Host 配置与 AgentDefinition 分离：

```text
HostConfigV1
|-- host_id
|-- definition_source_ref
|-- state_plane_bindings[]
|-- provider_endpoint_refs[]
|-- secret_broker_ref
|-- catalog_ref
|-- resolution_facts_ref
|-- runtime_service_refs
|-- listen_ref
`-- diagnostics_policy_ref
```

HostConfig 只携 refs 和受 Schema 约束的 endpoint IDs，不携 secret values、自由 Go package path、factory symbol 或任意动态代码 URL。

## 3. HostV1 接口

```go
type HostV1 interface {
    Validate(context.Context, ValidateRequestV1) (ValidateResultV1, error)
    Assemble(context.Context, AssembleRequestV1) (AssembleResultV1, error)
    Start(context.Context, StartRequestV1) (StartResultV1, error)
    Inspect(context.Context, InspectRequestV1) (InspectResultV1, error)
    Stop(context.Context, StopRequestV1) (StopResultV1, error)
}
```

- `Validate`：仅解析/验证配置和引用形状，零外部业务 Effect。
- `Assemble`：可读取 authoritative facts，产 sealed plan/compile artifacts，不启动 Provider。
- `Start`：消费 exact plan/handoff/binding，进入 Runtime command/admission 路径。
- `Inspect`：聚合只读 current projections，不创造权威结论。
- `Stop`：提交 Runtime command，由各 Owner 执行 cleanup，Host 仅等待/报告。

## 4. Factory Registry

Factory Registry 必须是构建时注册、版本化和 closed-by-plan：

- key 为 exact ComponentID + artifact digest + contract + capability；
- value 是符合公共 Factory Port 的构造函数；
- 只有 Resolved Plan + Harness Graph 选中的 factory 能被调用；
- registry 中额外存在但未绑定的 factory 不可暴露；
- 同 capability 多 factory、alias、artifact drift、版本漂移均 fail closed；
- 禁止反射包名、插件路径、shell 命令或配置内任意 module loader。

自定义组件通过构建/部署时注册其 public factory 和 Component Release，不需要修改 Host 核心 switch。

## 5. 生命周期与恢复

Host 维护的只允许是进程级 orchestration journal，不是领域事实副本。每步记录 exact input/output refs：

```text
accepted -> validating -> resolving -> compiling -> binding
         -> constructing -> verifying -> ready
         -> draining -> reconciling -> closed | indeterminate
```

- 回包丢失先 Inspect 对应 Owner fact；unknown 不盲重派。
- Host 重启从 journal + authoritative facts 恢复；不得依赖进程内对象判断已完成。
- journal 不替代 Command/Activation/Run/Effect/Review/Memory 等 Owner store。
- 若某 Factory 构造产生不可逆外部动作，该动作必须作为受治理 Effect 单独执行，不能隐藏在 constructor。

## 6. Sandbox 强制门

Runtime activation 当前要求 EnvironmentPort；因此首版完整 Agent：

- sandbox release、policy、provider、lease/current reader 必须 production；
- Environment Allocate/Activate/Open 仍走 Runtime admission/intent/inspect 路径；
- Host 不能用本地进程存活、临时目录或 Harness slot optional 推导 Sandbox Ready；
- remote sandbox 必须同样有 fence、inspect、cleanup、residual 和 credential scope。

## 7. 不选定事项

本设计不预选生产数据库、RPC 框架、对象库、向量库、MCP 实现、Sandbox 后端、部署进程数量或 SLA。具体选择进入 Plan，并必须满足相同 public Port/conformance。
