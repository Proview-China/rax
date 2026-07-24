# Praxis Sandbox Rust Data Plane

这是 Sandbox 的真实执行面。Go 仍拥有领域状态、Sandbox current projection 和
Runtime V4 适配；本 crate 只在每个实际执行点重新读取 current 后调用 Provider。

已实现：

- versioned length-prefixed JSON over Unix domain socket，双方校验 `SO_PEERCRED` UID；
- durable `Started -> Completed` journal；`Started` 后丢回包永久拒绝重放，只允许新的
  Inspect Effect；
- Runtime V4 `prepare` / `execute` 与 Sandbox EffectKind 分离，二者同时绑定请求摘要；
- bwrap Host Workspace：root-owned workspace/tool opaque binding、tmpfs root、只读toolchain、
  独立user/PID/IPC/UTS/network namespace、clearenv、exact process start-time identity、
  独立Fence/Release/Cleanup；真实workspace写入与逃逸反例已通过；
- containerd 2.x gRPC + OCI/runc v2：allocate、activate/open、inspect、fence、release、
  cleanup；rootfs 只能来自 root 配置中的 binding，容器使用独立 PID/IPC/UTS/Mount/
  Network/Cgroup namespace、只读 rootfs、空 capability、`noNewPrivileges`、cgroup limits、
  masked paths 和 seccomp denylist；
- QEMU `microvm` + KVM：kernel/initramfs均来自root-owned digest-pinned binding，固定无默认设备、
  无网络、QEMU sandbox参数；以PID+`/proc` start-time形成进程identity，运行中Release拒绝，
  Fence确认identity消失，Cleanup对仍存活进程只报Residual；
- Wasmtime 41 Component Model：WIT world/export、artifact digest、空 host-import allowlist、
  fuel、epoch deadline、memory/table/instance limits、targeted fence 和 release；
- Provider 输出始终只是 Observation/Receipt；本进程不写 Runtime Lease/Fence/Outcome，
  也不生成 Runtime Enforcement Receipt。

## 进程边界

```text
Go dispatch client
  -> dispatch.sock
  -> Rust validates sealed request
  -> current-reader.sock
  -> Go re-reads Runtime Enforcement V4 + Sandbox Reservation/current
  -> Rust durable Begin
  -> Provider method
  -> Rust durable Complete
  -> Observation/Receipt response
```

`prepare` 只做该 Effect 的本地/Provider准备；真实动作只在相同 attempt、相同 payload 的
`execute` current enforcement 后发生。`release` 不会顺带取消 active task；`cleanup` 是独立
Effect，才允许 fence 后清理 residual。

## 构建与验证

```bash
cargo fmt --check
cargo test --all-targets
cargo clippy --all-targets --all-features -- -D warnings

# 需要已运行且当前用户可访问的 /run/containerd/containerd.sock
cargo test --test containerd_backend -- --ignored --nocapture

# 需要可读的真实内核镜像与当前用户可访问/dev/kvm
PRAXIS_TEST_KERNEL=/path/to/pinned/vmlinuz \
  cargo test --test microvm_backend -- --ignored --nocapture
```

本机若 containerd socket 只允许 root，可先以普通用户完成编译，再只用受控权限运行已编译
的测试二进制；不要用 root 执行 Cargo，以免污染 target ownership。

## 部署前提

- Data Plane 使用独立低权限账号；该账号只能访问 containerd socket 和受控的
  `/var/lib/praxis/sandbox`；
- Go current-reader 与 Rust dispatch socket 使用专用 Unix group，不开放 other 权限；
- `config.example.json` 中Host workspace/tool、container rootfs与WASM component所有binding都是
  控制面配置，不接受Dispatch caller传裸路径；
- containerd namespace 与 socket group 由宿主运维创建；本进程不会修改 containerd 配置；
- Wasmtime 组件不得导入未注册 capability；当前首切片不提供 WASI filesystem/network。
- Host Workspace首切片不提供network allow-list、Secret、device或caller bind mount；Workspace
  binding必须是Owner预先创建的overlay/staging view，Provider不顺带commit。
- MicroVM首切片不提供guest agent、块设备、Snapshot、Secret或network allow-list；不得因KVM不可用
  而静默降级TCG并沿用原Admission。

`deploy/praxis-sandbox-dataplane.service` 是安装模板，不会由测试自动安装或启用。
