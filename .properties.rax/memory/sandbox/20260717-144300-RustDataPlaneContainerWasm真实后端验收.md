# Rust Data Plane、Container与WASM真实后端验收

时间：2026-07-17 14:43（Asia/Shanghai）  
状态：`implementation_software_and_live_backend_test_yes / end_to_end_composition_pending`

## 本次事件

用户解除旧的production Backend限制并明确选择：保留Go Control/Domain Plane；Rust作为独立
Data Plane进程；Container采用containerd 2.x + OCI/runc v2；WASM采用Wasmtime Component
Model/WIT。Go/Rust使用版本化Unix-domain IPC，不使用FFI。

已在`ExecutionRuntime/sandbox/**`落地：

- `protocol/v1` strict wire schema与Go/Rust golden digest；
- Go `dataplaneadapter`：Runtime Enforcement V4 dispatch无损映射、实际执行点reverse-current
  Reader、Runtime/Sandbox Owner exact current独立复读；
- Rust `dataplane`：peer credential、4 MiB strict frame、durable Started/Completed journal、
  actual-point Enforcer、Provider SPI与production root；
- containerd Provider：root配置绑定rootfs、OCI spec、namespace/cgroup/seccomp/capability/
  no-new-privileges/readonly-rootfs约束，以及allocate/activate/open/inspect/fence/release/cleanup；
- Wasmtime Provider：Component Model/WIT、artifact digest、空host import、fuel/epoch/memory/table/
  instance限制、fence/release；
- Provider只返回Observation/Receipt；不生成Evidence、DomainResult、Settlement、Runtime Outcome，
  不写Lease/Fence/Instance epoch。

## 实际验证

- Rust debug全目标测试、release全目标测试、strict clippy与fmt：PASS；
- Rust enforcer/journal/reverse-current IPC/golden/WASM故障矩阵连续100轮：PASS；
- 64路相同Attempt并发：唯一Provider winner；lost reply只Inspect语义与journal重启恢复：PASS；
- 真实Wasmtime Component：normal、unknown import、trap、fuel exhaustion、digest drift：PASS；
- 真实containerd：allocate→activate→inspect stopped→cleanup：PASS；测试后`ctr`的container与
  task列表均为空；
- Go adapter及领域核ordinary 100轮、race 20轮：PASS；Sandbox full ordinary/race/vet：PASS；
- gofmt、Rust fmt、禁止跨Owner import、JSON、`git diff --check`：PASS。

## 当前边界

这次验收证明真实执行面可用，不等于Praxis端到端Sandbox已投产。仍未闭合：

- Application/Coordinator到Data Plane的production composition；
- Provider Observation后的Evidence摄取、Sandbox Inspect/DomainResult、Runtime Settlement与
  Sandbox ApplySettlement完整链；
- Checkpoint/Restore Provider与公共接线；
- Host/MicroVM/Remote Provider、WASI host capability、containerd image/snapshot供应链；
- production部署认证、持久State Plane与SLA。

SnapshotArtifactOwnerV2 owner-local、Checkpoint Participant候选及其feature状态不因本事件被
扩大；Provider Observation/Receipt仍不是任何Owner权威事实。
