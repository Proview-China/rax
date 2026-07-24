# Praxis Sandbox

状态：`sandbox-owned-production-wiring-complete / external-production-certification-pending`。

Sandbox 是 Praxis 的执行治理与隔离层，不是 Docker 或 Firecracker 的产品封装。Go 1.25
实现 Controller、Owner State Plane、Runtime/Application Adapter、SDK/API/CLI 与宿主组合；
Rust 1.90 实现实际执行点 Enforcer、durable journal 和真实后端。Go/Rust 只通过严格版本化
UDS IPC 交互，不使用 FFI。

## 当前可以做什么

| 能力 | live 实现 | 边界 |
|---|---|---|
| Requirement/Policy/Placement | exact ref、revision、digest、TTL、risk/capability 路由 | Requirement 不是 Authority；无批准不降级 |
| 普通 lifecycle | allocate/activate/open/inspect/cancel/close/fence/release/cleanup 独立 Operation、双 Enforcement、Evidence、DomainResult、Runtime Settlement ref、Apply CAS | Provider 回包只是 Observation/Receipt |
| Host Workspace | bwrap、只读工具链、精确 workspace binding、默认断网、进程 identity、独立 fence/cleanup | 不开放绕过 Gateway 的 raw host shell |
| Container | containerd 2.x / OCI runc v2、只读 rootfs、资源/进程/网络约束、Inspect/Cleanup | 镜像、registry credential 由部署 Owner 提供 |
| MicroVM | QEMU/KVM、固定 kernel/initramfs/qemu digest、独立内核、Inspect/Fence/Release/Cleanup | guest agent、块快照、Secret 注入未声明支持 |
| WASM | Wasmtime Component Model、WIT、fuel/epoch/memory 限制、显式 capability gateway | WASM 是能力隔离，不冒充完整 Linux |
| Remote | vendor-neutral UDS connector、短 Credential current、exact Inspect/lost-reply | 远端 Provider 不拥有权威事实 |
| Workspace | View→Overlay→S1/S2 diff→content-addressed blob→独立 governed commit | overlay 不等于 committed |
| Workspace Rewind composition | exact View+keep/drop ChangeSet refs→新的 staged ChangeSet+immutable Composition Fact；SQLite create-once/lost-reply/history | 只做结构组合；实际提交仍复用既有 governed workspace commit |
| Checkpoint | prepare→commit XOR abort、双 Enforcement、Provider artifact、Evidence/Settlement/Apply、Workspace Participant/Coverage Fact | failed/not_applied 无后继；unknown 只 Inspect |
| Snapshot capture | Checkpoint artifact 独立复读→canonical Workspace bundle→AES-256-GCM content store→reserved→available Artifact Fact | terminal retention/purge 依赖外部 Owner |
| Restore | exact Snapshot bundle→fresh Instance/高 epoch/新 Lease/Fence→host-local stage→DomainResult/Settlement/Apply | 不复用旧 Instance，不回滚外部世界 |
| State Plane | SQLite WAL、FULL sync、append-only history/current、CAS、重启恢复 | 只保存 Sandbox-owned facts |
| SDK/API/CLI | governed Go SDK、异步 Operation、幂等/CAS/Watch/Cancel、HTTP auth、operator CLI | 不暴露 Provider handle 或 raw Fact CAS |
| Host root/release | API 与 actual-point current Reader 同生命周期；`/livez`、`/readyz`；Assembler exact Component Release/readiness contract | production 证书必须由独立宿主事实产生 |

## 固定权威链

```text
Runtime current governance
  -> Sandbox Reservation
  -> prepare Enforcement persisted
  -> Provider Prepare
  -> execute Enforcement persisted
  -> Provider Execute
  -> independent Inspect
  -> Evidence
  -> Sandbox DomainResultFact
  -> Runtime Operation Settlement (opaque exact ref)
  -> Sandbox ApplySettlement CAS
```

缺任一 Authority/Review/Budget/Scope/Lease/Fence/Binding/current/TTL 门时，Provider 调用数
必须为 0。Begin 不授执行权；丢回包后只能 Inspect 原 Attempt。`execution-quiesced`、
`environment-closed`、fenced、released、cleanup-complete 分别证明，不能互相推导。

## 包结构

- `contract`、`ports`、`kernel`：Sandbox-owned DTO、Port、状态机与 CAS。
- `runtimeadapter`、`applicationadapter`：只依赖 Runtime/Application 公共合同。
- `dataplaneadapter`：Go/Rust wire、reverse-current server、Host-local content/restore。
- `dataplane`：Rust Enforcer、journal、Host/containerd/QEMU-KVM/Wasmtime/Remote Provider。
- `workspacefs`：受信路径 binding、overlay diff、blob capture。
- `storage/sqlite`：单节点持久 Owner Store。
- `sdk`、`api`、`apihandler`、`cmd/praxis-sandbox`：公共使用面。
- `hostroot`：可信宿主工厂产物与健康门。
- `release`：Agent Assembler Component Release 与独立 readiness 复读。
- `internal/testkit`：仅测试，绝不作为生产 Backend 或 SLA 证据。

## 精确 unsupported

以下不是可由 Sandbox 私建的“兼容实现”：

1. Retention/Legal Hold Owner 的 current Index/Carry/negative no-hold proof；
2. Runtime governed Snapshot purge/cleanup sibling、Evidence/Settlement 与 Management terminal
   DTO；因此 `FeatureSnapshotArtifactCapture=true`，而完整
   `FeatureSnapshotArtifactOwner=false`；
3. Agent Host 对 factory/provider/phase handler 的最终注册，以及 deployment attestation、
   Certification Fact、密钥轮换、组织级 Tool Gateway、镜像/guest artifact 供应链；
4. 特定云厂商 Remote connector、分布式 State Plane、跨机升级与 SLA 认证。

这些外部事实缺失时，release 只能是 `standalone`，不能自证 `production`。Sandbox 已提供
精确 Port Delta/readiness 输入，但不会复制其他 Owner 类型或伪造证明。

## 验证

```bash
cd /home/proview/Desktop/property/praxis/ExecutionRuntime/sandbox
go test -count=100 -shuffle=on ./...
go test -count=20 -race -shuffle=on ./...
go test -race ./...
go vet ./...

cd dataplane
cargo test --all-targets
cargo clippy --all-targets --all-features -- -D warnings
cargo fmt --all -- --check
```

本机真实黑盒还包括：

- bwrap Host 与 Wasmtime：普通 `cargo test --all-targets` 自动执行；
- containerd：临时启动本机 daemon 后运行 ignored OCI lifecycle；
- QEMU/KVM：以受信 kernel artifact 运行两个 ignored 独立内核与 fence/cleanup 黑盒。

测试通过只证明代码与本机后端行为；production 标记仍必须经过 release readiness 中列出的
独立 current facts、deployment attestation 与 certification。
