# Sandbox v2 设计总览

状态：`implemented / sandbox-owned closure complete / external production gates pending`。

最高业务输入：`tmp.document/Sandbox.md`。

## 1. 定位

Sandbox 是逻辑执行治理层：

- Agent 是 Identity/状态/记忆；
- Sandbox 是临时执行权与隔离边界；
- Host、Container、MicroVM、Remote 是后端；
- WASM 是 capability/tool 隔离载体；
- 业务 namespace/policy domain 与计算 worker pool 分离。

AgentInstance 进入执行态时消费 Runtime 线性化的当前 SandboxLease；Sandbox 不签发、不续期、
不迁移该 Lease。

## 2. Owner

| 事实 | 唯一 Owner | Sandbox 角色 |
|---|---|---|
| Instance/epoch、SandboxLease、Fence、Operation、Run/Outcome | Runtime | exact current 消费、提交 DomainResult ref |
| 跨域编排与 settlement 请求 | Application Coordinator | 提供版本化 Adapter |
| Requirement/CompiledGraph | Agent Definition/Assembler | 校验不可变需求 |
| Authority/Review/Budget/Policy source | 各治理 Owner | 消费 revision/digest/TTL |
| Placement、环境、Workspace、Checkpoint Participant/Compatibility/Restore domain、Cleanup/Residual | Sandbox | Inspect 后 CAS |
| Provider local attempt | Backend Provider | 仅 Observation/Receipt |
| actual-point enforcement | Rust Enforcer | current 复读与执行 |
| CheckpointAttempt/Barrier/Effect Cut/consistent/restore eligibility | Runtime | Sandbox 仅绑定 exact ref |
| Manifest/RestorePlan | Continuity | Sandbox 提供 Participant/Artifact ref |
| Retention/Legal Hold | Retention Owner | Sandbox 只读 exact proof |
| Snapshot terminal deleted/indeterminate | Management + Runtime governed purge | Sandbox 等待公共 sibling 合同 |

## 3. 架构

```text
Runtime/Application
    |
    | public versioned ports
    v
Go Sandbox Owner Core --------> SQLite Owner State Plane
    |
    | strict UDS + exact current coordinates
    v
Rust Data Plane Enforcer
    |
    +--> bwrap Host Workspace
    +--> containerd/OCI
    +--> QEMU/KVM
    +--> Wasmtime Component/WIT
    `--> Remote neutral connector
```

Go/Rust 不使用 FFI。Rust 不写 Runtime/Sandbox 权威 Fact。Provider 名称不蕴含 isolation
保证；每个 backend/artifact/contract 必须单独 Conformance。

## 4. 治理链

```text
Requirement
  -> Placement Candidate
  -> Reservation
  -> InspectCurrent
  -> Admission
  -> Review/Auth/Budget/Scope
  -> Permit
  -> Begin
  -> persisted prepare Enforcement
  -> Provider Prepare
  -> persisted execute Enforcement
  -> Provider ExecutePrepared
  -> Observation/Receipt
  -> independent Inspect
  -> Evidence
  -> DomainResultFact
  -> Runtime Settlement exact ref
  -> Sandbox ApplySettlement CAS
```

Begin 不授执行权。prepare/execute 任一 current 门失败时 Provider=0。UnknownOutcome 只
Inspect 原 Attempt；Provider NotFound、进程死亡、超时不恢复重派权。

## 5. Workspace、Checkpoint、Snapshot、Restore

Workspace：

```text
View -> Overlay -> S1/S2 Diff -> Blob -> governed Commit -> Inspect/Settlement
```

Checkpoint：

```text
prepare -> commit XOR abort
failed/not_applied -> no successor
unknown -> Inspect/Reconcile -> prepared|failed|not_applied|indeterminate
```

Checkpoint Provider artifact 必须由 Sandbox 独立复读，重新生成 canonical
WorkspaceSnapshotBundle，写入 AES-256-GCM content store，再将 Snapshot Artifact
`reserved -> available`。

Restore 必须：

- exact 复读 Snapshot Fact/current/content；
- 创建 fresh Instance、更高 epoch、新 Lease/Fence；
- 在新 staging root 写入 canonical bundle；
- 独立 DomainResult/Settlement/Apply；
- 明确保留不可回滚外部 Effect 与 Residual。

## 6. Backend capability

| Backend | 当前强制能力 | 显式 unsupported |
|---|---|---|
| Host | bwrap、mount scope、default-deny network、PID identity、Fence/Cleanup | raw shell bypass |
| Container | OCI rootfs、resource/pid/network、Inspect/Cleanup | registry/credential 供应链 |
| MicroVM | fixed artifact digest、KVM kernel boundary、Fence/Cleanup | guest agent、block snapshot、Secret |
| WASM | WIT imports、grant、fuel/epoch/memory | Linux workspace |
| Remote | credential current、request/result binding、Inspect | authority ownership、vendor RPC |

未提供的能力以 `unsupported/observe_only` 进入路由，不静默降级。

## 7. 使用面与装配

- SDK/API/CLI 只能经公共 Controller/Application Port 提交意图；
- API 提供 async Operation、idempotency、CAS、Watch、Cancel、Inspect-only reconcile；
- Host root 同时监督 API 与 reverse-current server，并提供 `/livez`、`/readyz`；
- Component Release 声明 `sandbox.execution`、effectful Port、Factory、Cleanup、Owner 与
  production readiness；
- Harness 只消费装配后的 endpoint/scope，不持有 Backend handle，不导入 Sandbox 实现。

## 8. production 外部门

Sandbox 已完成可独立实现的代码闭环。以下仍由外部 Owner 关闭：

1. Retention/Legal Hold exact current/no-active proof；
2. Runtime Snapshot purge/cleanup sibling 与 Evidence/Settlement；
3. Management CurrentIndex/Tombstone terminal DTO；
4. Agent Host factory/provider/phase 注册；
5. deployment attestation、Certification Fact、密钥/镜像/guest artifact 供应链；
6. vendor Remote connector、分布式 State Plane、升级/SLA。

这些门未完成前：

- Snapshot capture 可用，但 terminal Artifact Owner 不标 supported；
- release 只能 `standalone`；
- 不自签 production、deployment 或 SLA。

详细合同见 `contracts.md`、`interfaces.md`、`state-machines.md`、
`workspace-checkpoint.md`、`workspace-restore-v1.md`、`port-delta.md` 与
`acceptance.md`。
