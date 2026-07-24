# Sandbox Rust Data Plane生产设计

状态：`implemented / software-and-live-backend-test-yes / production-wiring-implemented / deployment-pending`  
日期：2026-07-17

## 1. 裁决

用户确认保留现有Go Control Plane与领域Owner状态核，Rust独立进程实现Data Plane Enforcer、
containerd/OCI Container Provider和Wasmtime Component Model Provider。Go/Rust不使用FFI；本地
版本化IPC是唯一接线。原首切片不含MicroVM；2026-07-18后续已增加QEMU/KVM MicroVM Adapter。
Remote Provider、数据库、远程RPC与SLA仍不在已实现范围。

## 2. Owner与非Owner

| 对象 | Owner | Rust Data Plane权限 |
|---|---|---|
| Runtime Lease/Instance/Fence/Permit/Enforcement Journal | Runtime | 只读exact current；不能创建、续期或推进 |
| Sandbox Reservation/Attempt/DomainResult/ApplySettlement | Go Sandbox Owner | 只消费坐标；只返回Observation/Receipt |
| Host process、containerd task/container/snapshot、MicroVM process、WASM prepared component/instance | 对应Rust Provider | 可执行与Inspect；不得升级为领域事实 |
| Evidence/Settlement/Outcome | Runtime/Evidence Owner | 不能Issue、Consume、Settle或选择Outcome |

## 3. 进程与DAG

```text
Runtime V4 Gateway
  -> Application SandboxLifecyclePortV4
      -> Go Sandbox production composition / current Reader / dispatch adapter
      -> Rust Data Plane Unix socket
          -> Rust Enforcer
              -> Go read-only current socket (exact re-read)
              -> Host Workspace Provider -> bwrap
              -> containerd Provider -> /run/containerd/containerd.sock -> OCI/runc
              -> MicroVM Provider -> QEMU microvm/KVM
              -> Wasmtime Provider -> WIT Component
```

依赖单向：Runtime public ports -> Go Sandbox adapter -> wire schema <- Rust Data Plane。Runtime不导入
Sandbox；Rust不链接Go；Provider不访问Fact Store。reverse current socket只提供读取，不提供CAS或
dispatch。任何回调环都以一次request ID结束，禁止Rust在current reader响应期间递归调用Provider。

Go composition root只组合宿主注入的Runtime公共Gateway、Sandbox持久Owner Store、Application
Plan/Result Store与UDS坐标；不会构造Runtime实现或把testkit当State Plane。每个原动作在Provider
Observation后必须进入独立governed Inspect：新Effect/Attempt/Permit/双Enforcement、独立Evidence、
Inspect DomainResult/Settlement/Apply，再形成原动作Inspection与最终DomainResult/Settlement/Apply。

## 4. 状态机

```text
absent -> prepared -> executing -> observed_terminal
                 \-> fenced
unknown -> inspect(original attempt) -> prepared|observed_terminal|indeterminate
observed_terminal -> release_requested -> released_observed -> cleanup_inspected
```

Prepare与Execute是两个独立Runtime Enforcement phase。Begin不授执行；Prepare receipt不授Execute。
lost reply只Inspect原Attempt。Provider NotFound仅是Observation，不能证明not_applied。Fence只缩权；
Release不证明Cleanup；Cleanup七维全部confirmed前slot不得复用。

## 5. Container精确边界

- image必须使用digest-pinned reference；tag-only请求拒绝。
- OCI spec由Sandbox Policy投影生成，Provider只允许缩减：rootfs readonly、capability drop、seccomp、
  namespaces、cgroup limits、mount allowlist、network deny/allow模式、no-new-privileges与masked paths。
- 首批runtime handler固定`io.containerd.runc.v2`，socket默认`/run/containerd/containerd.sock`，二者
  由Owner配置并进入Descriptor digest，不接受每请求覆盖。
- Container ID与task ID从`tenant + Runtime Lease ID/Epoch + Instance epoch`稳定坐标确定性派生，
  使allocate与后续activate独立Effect仍指向同一受租约约束环境；禁止caller指定raw ID。
- Inspect必须同时读取container metadata、task status与snapshot existence；进程死亡不等于Release/
  Cleanup。containerd不可用或重启时进入Unknown/Inspect，不自动重派。

## 6. WASM精确边界

- WIT package首批为`praxis:sandbox/capability@1.0.0`；guest只导入显式授予的host capability。
- component bytes以sha256 digest绑定；digest、WIT world、export、Policy revision或limit变化必须新Attempt。
- 使用Wasmtime 41.x以兼容仓库Rust 1.90；禁用default features，只启用component-model、cranelift、
  pooling、runtime、std与wat。guest调用在Tokio`spawn_blocking`中执行，不启用Wasmtime async ABI。
- 每次调用强制fuel、epoch deadline、linear-memory、table、instance与result-size上限；trap/interrupt/OOM
  形成Observation，不直接成为Sandbox失败Fact。
- 默认无WASI filesystem/network/process/environment。未来WASI或自定义host capability必须通过
  Runtime governed Effect Gateway，不在Linker里直连宿主副作用。

## 6A. MicroVM精确边界

- 公共Surface保持`microvm`产品中立；QEMU、KVM、binary/kernel/initramfs digest只属于Adapter配置与
  Conformance证据，不进入Policy枚举。
- kernel/initramfs只由root-owned opaque binding解析并复核内容digest；caller不能传路径、QEMU参数、
  kernel cmdline、device或TCG fallback。KVM不可读写时fail closed。
- 首切片固定QEMU `microvm,accel=kvm`、无默认设备、无网络、无monitor/display与sandbox deny参数；
  不提供guest agent、块设备、host share、Secret、Snapshot或network allow-list。
- PID与`/proc` start-time共同组成实际进程identity。Release拒绝任何仍存活identity；Fence等待identity
  消失后才确认；Cleanup若identity仍存活返回Residual，不能因状态目录删除而证明回收。
- 真实KVM黑盒已证明独立内核启动、serial标记、Inspect running、Release Conflict、Fence与Cleanup；
  该证据不替代宿主production root binding、deployment attestation或SLA。

## 7. 安全与恢复

- socket目录0750、socket0660，限定专用Unix group与`SO_PEERCRED` UID allowlist；4 MiB帧上限、
  未知字段拒绝且other权限必须为0。单用户部署可进一步收紧为0700/0600。
- 所有request/result canonical digest包含contract version、phase、Attempt、Provider binding、payload
  与TTL；`now == expires`失效。
- Provider attempt使用append-only Started/Completed journal；不以caller current pointer授权。
  同exact attempt replay：Completed由exact Inspect返回持久完整Result，Started返回Unknown并要求Inspect，
  绝不再次调用Provider。独立远端Inspect用`ProviderInspectionTargetV1`绑定原Effect/Attempt、revision-2
  ProviderAttempt、原request digest与payload digest；NotFound保持Unknown，不推导not_applied。
- Rust panic不得跨边界；顶层捕获转closed internal error并写审计日志，不含Secret或payload原文。
- shutdown先停止接收新请求，再Fence/Inspect active Attempts；未收敛项输出Residual，不伪报clean。

## 8. 验收

单元、白盒、黑盒、故障、Conformance必须覆盖：双Enforcement顺序、零门禁Provider调用、UID/frame/
typed-nil/digest/TTL拒绝、64并发单赢家、lost reply、containerd restart/task death/stale snapshot、Fence/
Release/Cleanup分离、WASM未知import/digest漂移/fuel/epoch/memory/trap、Go/Rust golden wire、进程重启
journal恢复。Rust执行`cargo test`、`cargo clippy -- -D warnings`、`cargo fmt --check`；Go继续ordinary/
race/vet；真实containerd与真实WASM component集成必须通过后才能标production-supported。
