# Sandbox 模块说明

状态：`sandbox-owned-production-wiring-complete / external-production-certification-pending`。

## 作用

Sandbox 为 AgentInstance 提供临时、受治理的执行权和隔离边界。它根据
ExecutionRequirement、SandboxPolicy、Backend capability、风险等级与当前 Runtime
Authority/Review/Budget/Scope/Lease/Fence 选择并绑定 Host、Container、MicroVM、WASM 或
Remote 后端。

Sandbox 不拥有 Runtime Instance/epoch、Runtime Lease/Fence、Run Outcome、Continuity
Manifest/RestorePlan、Retention/Legal Hold 或 Management terminal facts。Provider/Enforcer
只能产生 Observation/Receipt。

## 组成与当前能力

| 层 | 当前产物 |
|---|---|
| Go Owner Core | Requirement/Policy/Placement、Lifecycle、Workspace、Checkpoint Participant、Snapshot capture、Restore、Cleanup/Residual 状态机 |
| Go State Plane | SQLite WAL/FULL sync、append-only history/current、exact CAS、lost-reply/restart 恢复 |
| Runtime/Application 接线 | 双 Enforcement、Evidence、DomainResult→Settlement ref→ApplySettlement、actual-point current Reader |
| Rust Data Plane | durable journal、strict UDS、bwrap Host、containerd/OCI、QEMU/KVM、Wasmtime/WIT、Remote connector |
| Workspace/Continuity bridge | overlay/diff/commit；Checkpoint artifact→canonical bundle→加密 Snapshot；fresh Instance Restore stage |
| 使用面 | governed SDK、异步 API/Watch、CLI、Host root `/livez`/`/readyz` |
| Assembly | exact Component Release、Factory/Port/Slot descriptors、独立 production readiness contract |

固定链：

```text
Reserve
  -> current Admission/Review/Auth/Permit
  -> prepare Enforcement
  -> Provider Prepare
  -> execute Enforcement
  -> Provider Execute
  -> Inspect/Evidence
  -> DomainResultFact
  -> Runtime Settlement exact ref
  -> Sandbox ApplySettlement CAS
```

UnknownOutcome 只 Inspect 原 Attempt。Close 不顺带 Cancel active Run，Release 不证明
Cleanup，Restore 必须创建 fresh Instance、更高 epoch 和新 Lease/Fence。

## Backend 能力边界

| Backend | 已证明 | 未声明 |
|---|---|---|
| Host Workspace | bwrap、默认断网、受控挂载、进程 identity、Fence/Cleanup | raw host shell |
| containerd/OCI | real lifecycle、read-only rootfs、resource/network/process limits、Inspect/Cleanup | registry/credential 供应链 |
| QEMU/KVM | fixed artifact digest、独立 kernel、Inspect/Fence/Release/Cleanup | guest agent、块 Snapshot、Secret 注入 |
| Wasmtime | Component/WIT、explicit grant、fuel/epoch/memory、trap fail-closed | 完整 Linux 环境 |
| Remote | neutral SPI、短 Credential、exact Inspect/lost-reply | 厂商 RPC、远端权威事实 |

## 完成与外部门

Sandbox 自有的代码、持久化、真实后端、Checkpoint→Snapshot→Restore 内容桥、SDK/API/CLI、
Host root 和 release descriptor 已落地。

仍不能由 Sandbox 单独关闭的 production 门：

1. Retention/Legal Hold current proof；
2. Runtime Snapshot purge/cleanup sibling 与 Management terminal DTO；
3. Agent Host 最终 factory/provider/phase 注册；
4. deployment attestation、Certification Fact、密钥/镜像/guest artifact 供应链；
5. vendor Remote connector、分布式 State Plane、升级与 SLA 认证。

因此 `FeatureSnapshotArtifactCapture=true`，完整 terminal
`FeatureSnapshotArtifactOwner=false`。这些门未齐时 Component Release 只能发布
`standalone`，不能由 Sandbox 自证 production。

## 验证证据

- Go：full ordinary、ordinary 100、race 20、full race、vet；
- Rust：all-targets、strict clippy、fmt；
- 实机：bwrap、Wasmtime、containerd/OCI、QEMU/KVM；
- 反例：旧 epoch/current drift/TTL crossing/typed-nil/64 路 CAS/lost reply/no-ABA/
  Provider NotFound/cleanup residual/Checkpoint XOR/Restore fresh Instance。

命令与最新结果记录在
`.properties.rax/memory/sandbox/20260723-213300-Sandbox生产接线最终收口.md`。
