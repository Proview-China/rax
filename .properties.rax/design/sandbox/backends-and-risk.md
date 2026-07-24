# Sandbox v2 后端、隔离与风险路由

## 1. 三个正交维度

Sandbox 不用单一枚举同时表示产品、隔离强度和位置。Placement 必须分别记录：

1. `ExecutionSurfaceClass`：`host_workspace`、`container`、`microvm`、`wasm_capability`；
2. `IsolationProfile`：实际可强制的文件、网络、进程、Secret、资源、内核、设备、系统调用、Checkpoint 与 Cleanup 属性；
3. `Locality`：`host`、`instance_data_plane`、`remote_provider`。

Remote Provider 可以承载 Container、MicroVM 或 WASM，不自动获得更高隔离等级。Docker、gVisor、Kata、Firecracker、Wasmtime 等产品名只能出现在 Provider Adapter 或证据中，不能成为公共策略语义。

## 2. 执行面裁决

| 执行面 | 适用工作 | 必须证明 | 固有限制/Residual |
|---|---|---|---|
| Host Workspace | 本地 coding、真实工具链、真实项目读取 | 受控 executor、WorkspaceView、overlay/diff、路径/网络/Secret scope、Fence、独立 Cleanup Inspect | 无内核隔离；任意 Shell 可绕过门禁时不能 fully controlled |
| Container | 常规 Linux 工具链、可信或中等风险任务、共享 Worker Pool | 进程树、mount、网络、Secret、resource、Fence、slot reuse cleanup；共享内核事实必须显式 | 共享内核；强化运行时能力由 Descriptor 证明，不能靠“容器”名称推断 |
| MicroVM | 不可信代码、强多租户、敏感数据、独立内核需求、长任务 | 独立内核、启动/停止/Fence、网络/块设备、Secret、Snapshot/Residual、VM 级 Cleanup | 启动和资源成本较高；Snapshot/远端残留仍需独立 Inspect |
| WASM Capability | 纯函数工具、Skill、Policy module、数据转换、细粒度能力调用 | 默认无文件/网络/进程权限、显式 capability import、fuel/time/memory、module digest、deterministic/side-effect declaration | 不是完整 Linux；不能满足任意 Shell、浏览器、系统工具链需求 |

WASM 可以嵌入 Host/Container/MicroVM，作为第三方 Tool/Skill 的第二层能力隔离；也可以单独承载只需能力合同的任务。它不能成为最终权限 Owner，仍需读取当前 Runtime Lease/Fence/Scope 与 Runtime Operation Permit。

## 3. Capability 状态

每个 Backend Capability 只允许：

- `enforced`：实际执行点可以阻止越界，并有当前证据；
- `observed_only`：只能观察，不能阻止；
- `unsupported`：不提供。

Descriptor 不能把 `observed_only` 暴露为可执行 grant。Conformance 仍使用治理合同的四级：`fully_controlled`、`restricted_controlled`、`contained_observe_only`、`rejected`。

## 4. 后端 Capability 矩阵

下表是 Admission 最低语义，不是对任何具体产品的生产认证；每个 Adapter 必须用 Conformance Report 证明。

2026-07-18 Host Workspace首切片合同已冻结：Rust Data Plane使用受控`bwrap` executor；
workspace与tool只能来自root配置的opaque binding，caller不得传宿主路径；根文件系统由tmpfs构造，
仅只读挂载被Owner允许的toolchain目录及精确Workspace View，默认新建user/PID/IPC/UTS/network
namespace并清空environment。Host Provider只执行当前V4双Enforcement绑定的普通lifecycle attempt，
只返回Observation/Receipt；workspace commit仍是独立受治理Effect，不能由executor顺带提交。

Host列已完成bwrap真实执行、写入精确overlay、进程Inspect/Fence/Release/Cleanup与逃逸反例；
Container列已有containerd 2.x + OCI/runc v2首切片，真实本机allocate/activate/inspect/cleanup
通过；WASM列已有Wasmtime 41 Component Model/WIT首切片，真实Component正常/故障矩阵通过。
MicroVM列已有QEMU `microvm` + KVM真实独立内核首切片；真实启动、Inspect、运行中Release拒绝、
Fence等待进程identity消失、Cleanup与artifact/KVM降级反例已经通过。它仍缺guest agent、块设备、
Snapshot、Secret与宿主部署认证，不能把首切片扩大表述为完整production能力。

| Capability | Host | Container | MicroVM | WASM Capability |
|---|---:|---:|---:|---:|
| `sandbox.execution.controlled` | 必须；任意 Shell 绕过则降级/拒绝 | 必须 | 必须 | 仅 capability call |
| `sandbox.files.view` | 必须 | 必须 | 通过 mount/block/agent | 仅显式 preopen/ref |
| `sandbox.files.overlay` | 本地 coding 必须 | 必须或显式 unsupported | Provider/guest agent 证明 | 仅受限 virtual FS |
| `sandbox.network.deny_all` | 必须可强制或拒绝 | 必须 | 必须 | 默认必须 |
| `sandbox.network.allow_list` | 可选但声明外不得开放 | 可选 | 可选 | 显式 host capability |
| `sandbox.process.fence` | 受控 executor 必须 | namespace/cgroup 等效果必须 | VM stop/fence 必须 | 不适用，实例调用终止 |
| `sandbox.secret.ephemeral` | 必须引用化和可撤销 | 必须 | 必须 | 显式 import，禁止长期明文 |
| `sandbox.resource.limit` | 必须声明实际可强制维度 | 必须 | 必须 | fuel/time/memory 必须 |
| `sandbox.inspect.prepared-local` | 必须 | 必须 | 必须 | 必须 |
| `sandbox.inspect.attempt-local` | 必须 | 必须 | 必须 | 必须 |
| `sandbox.cleanup.coverage` | 七维覆盖；不足为 restricted/unknown | 七维覆盖 | 七维覆盖含 VM/volume | module state、host capability、remote continuation |
| `sandbox.checkpoint.workspace` | 可选 | 可选 | 可选 | 可选 |
| `sandbox.checkpoint.environment` | 通常 unsupported | Provider 可选 | Provider 可选 | module state 可选 |
| `sandbox.restore.new-instance` | 必须通过新 Runtime Instance/Lease | 同左 | 同左 | 同左 |
| `sandbox.remote.inspect` | Remote locality 时必须 | Remote locality 时必须 | Remote locality 时必须 | Remote locality 时必须 |

任何 Backend 若不能证明 persistent effect 可 Fence、网络可控、Secret 不长期明文、Unknown 可 Inspect，则对应高风险 Requirement 直接 `rejected`。

## 5. 风险路由

风险路由先匹配 Requirement，再检查 Policy、Capability、Conformance、Residual 与容量；风险标签不能单独选产品。

| 风险/需求 | 首选候选 | 可接受降级 | 禁止 |
|---|---|---|---|
| 纯函数、无外部 Effect、边界清晰 | WASM capability | 受控 Container | 为每次调用启动 MicroVM 作为默认；给予隐式文件/网络 |
| 本地 coding、真实 Workspace、可信仓库 | Host Workspace + overlay/diff | Container 挂载受限 View；必须在 Plan 中显式 | 直接宿主任意 Shell却声明 fully controlled |
| 常规 Linux 工具链、中风险 | Container | 强化 Container 或 MicroVM，按容量/成本策略 | 静默切换 Host 无隔离 |
| 不可信代码、外部高风险输入、强多租户 | MicroVM | 只有 Plan 明确且能力等价时的强化 Container | 普通共享 Container 静默降级 |
| 敏感数据、独立内核、长任务 | MicroVM | 受批准的 Remote MicroVM | 未证明数据域/Residual 的 Remote Provider |
| 细粒度第三方 Tool/Skill | WASM 嵌入当前执行面 | 受控 Tool Provider | 因主任务已在 MicroVM 就绕过 capability gateway |

### 路由不变量

- `PlacementCandidate` 可有多个；只有 Sandbox Allocation Admission 的精确结果可成为 `AllocationFact` 输入。
- Allowed downgrade 必须在 Requirement/Plan 中有序列出，并为每个候选重新执行 Admission/Review；容量不足不能修改原 Decision。
- Scheduler 只提供容量和槽位 Candidate；Runtime 拥有 SandboxLease，Sandbox 不签发 Lease。
- 选择 Remote Provider 时，短期凭据、最小 Scope、Inspect、Settlement、Residual 和数据驻留策略均为 Admission 必填。

## 6. 默认策略仍需管理线裁决

设计只冻结机制，不替管理线决定：

1. Host 任意 Shell 在默认产品策略中是 `restricted_controlled` 还是直接 `rejected`；
2. 哪些 namespaced RiskClass 强制 MicroVM，哪些允许强化 Container；
3. Remote Provider 的数据驻留、最大 TTL 和最小 Inspect 覆盖；
4. ~~WASM Runtime 是首批实现还是第二批独立里程碑。~~ 2026-07-17用户已裁决首批同时实现
   Container与WASM；Container绑定containerd/OCI，WASM绑定Wasmtime Component Model。

## 7. 2026-07-17生产Provider技术裁决

- Go继续拥有Sandbox领域合同、Controller、Owner Store、current Reader及Runtime Adapter；不重写
  已通过ordinary/race/vet的软件状态核。
- Rust独立Data Plane进程拥有实际执行点Enforcer与Provider Adapter实现，但不拥有Lease、Fence、
  Permit、Evidence、DomainResult、Settlement或Runtime Outcome。
- Container Provider使用containerd 2.x的版本化gRPC API和OCI runtime。首批只认证Linux本地
  `containerd-shim-runc-v2`，不因containerd支持其他runtime就自动声明Kata/gVisor/MicroVM能力。
- WASM Provider使用与Rust 1.90兼容的Wasmtime 41.x、Component Model与WIT。首批只承载
  tool/skill/policy/data-transform capability；不提供任意Shell、浏览器或完整Linux语义。
- 产品版本、socket、runtime handler和artifact digest必须进入Provider Descriptor/Conformance Evidence；
  不能进入公共Policy枚举，也不能由caller覆盖Owner配置。

## 8. 2026-07-18 Host Workspace Provider首切片

- wire payload只携`workspace_binding_id`、`tool_binding_id`、argv、受限environment、相对working
  directory、resource limits和`network_deny_all=true`；binding由Data Plane root配置解析并复核真实目录/
  文件、无symlink与owner policy，payload不携宿主绝对路径。
- `prepare`只验证binding、bwrap能力与命令形状；`execute`才创建隔离进程。每个attempt使用独立
  state目录和process identity token；进程PID与`/proc/<pid>/stat` starttime共同标识，PID复用不得
  被Inspect/Fence误认。
- workspace输入绑定必须指向Owner预先创建的overlay或staging view；Host Provider不创建
  authoritative WorkspaceView，不生成ChangeSet，不执行commit，也不把进程退出推导成cleanup完成。
- Fence只终止同一process identity；重启后identity无法确认、lost reply或`/proc`漂移时返回unknown，
  只能由独立Inspect/cleanup Effect继续，不能重放原execute。
- 首切片不开放网络allow-list、Secret注入、设备、宿主home、任意bind mount或caller选择的shell。
  这些能力保持unsupported，不能在BackendDescriptor中声明`enforced`。

## 9. 2026-07-18 MicroVM Provider实施裁决

- 公共`ExecutionSurfaceClass=microvm`与Policy仍保持产品中立；首个可验证Adapter选用QEMU
  `microvm` machine + KVM，产品名、binary/kernel/initramfs digest只进入Provider配置与Conformance
  Evidence，不进入公共策略枚举。KVM不可用时不得静默降级TCG并沿用原Admission。
- kernel/initramfs只能由Data Plane root配置的opaque binding解析；payload仅携binding ID、digest、
  vCPU/memory/wall-clock与network-deny-all，不携宿主路径、任意QEMU参数、device或kernel cmdline。
- 固定`-nodefaults/-no-user-config/-nographic/-net none`与QEMU sandbox deny选项；首切片无块设备、
  Secret、host share、guest agent或Snapshot。未实现的能力必须在Descriptor中保持unsupported。
- process identity、lost-reply、Fence/Release/Cleanup与Host Provider同等要求；Provider只返回
  Observation/Receipt，VM启动/退出、serial输出或QMP状态都不能直接成为Sandbox/Runtime事实。
- 首切片实测使用宿主当前内核的只读副本与静态BusyBox initramfs：独立guest输出启动标记；运行中
  Inspect为`running`，Release为Conflict，Fence仅在PID+`/proc` start-time identity消失后返回
  `fenced`，Cleanup若identity仍存活只能返回`residual_present`，不得删除状态目录并误报完成。
