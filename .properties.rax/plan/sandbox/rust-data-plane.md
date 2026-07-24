# Sandbox Rust Data Plane实施计划

状态：`host+container+wasm+qemu-kvm-microvm implemented / software-and-live-backend-test-yes / production-wiring-implemented / deployment-pending`  
日期：2026-07-17

## 1. 文件落点

| 路径 | 产物 |
|---|---|
| `ExecutionRuntime/sandbox/protocol/v1/**` | Go/Rust共享wire schema、golden vectors |
| `ExecutionRuntime/sandbox/dataplane/Cargo.toml` | Rust 1.90独立crate |
| `dataplane/src/contract.rs` | strict DTO、canonical digest、closed errors |
| `dataplane/src/ipc.rs` | UDS framing、peer credential、limits、server/client |
| `dataplane/src/enforcer.rs` | prepare/execute前exact current复读与fail-closed门禁 |
| `dataplane/src/provider.rs` | backend-neutral Provider trait与attempt状态 |
| `dataplane/src/containerd/**` | containerd gRPC/OCI Provider |
| `dataplane/src/wasm/**` + `dataplane/wit/**` | Wasmtime Component/WIT capability Provider |
| `dataplane/src/host.rs` | bwrap Host Workspace受控executor；root-owned bindings、process identity、Fence/Inspect |
| `dataplane/src/microvm.rs` | QEMU microvm/KVM Provider；root-owned kernel/initramfs binding、process identity、Fence/Inspect |
| `dataplane/src/bin/praxis-sandbox-dataplane.rs` | 本地production root |
| `ExecutionRuntime/sandbox/dataplaneadapter/**` | Go wire/current-reader/dispatch adapter |
| `ExecutionRuntime/sandbox/applicationadapter/**` | Application lifecycle Port、governed Inspect、Settlement/Apply与composition root |
| `*_test.rs`, Go `*_test.go`, `tests/**` | unit/whitebox/blackbox/fault/conformance/integration |

不修改Runtime/Harness/Application实现或根Go workspace/CI；若live public port无法表达，只提交Port
Delta，不在Sandbox复制Runtime语义。

## 2. 阶段

1. 冻结wire schema、closed errors、Owner/TTL/canonical与golden vectors。
2. 实现Rust Provider trait、append-only attempt journal和Enforcer current-reader抽象。
3. 实现UDS双向IPC、peer credential和shutdown/recovery。
4. 实现containerd client、OCI spec hardening、Prepare/Execute/Inspect/Fence/Release/Cleanup。
5. 实现Wasmtime/WIT compile/instantiate/call、capability deny-by-default与资源限制。
6. 实现Go adapter与Runtime V4 exact refs无损映射；缺任一门不发IPC。
7. 运行纯软件、真实containerd、真实WASM、进程重启及高重复/race门。
8. 实现Application lifecycle与Provider后Evidence/独立Inspect/DomainResult/Settlement/Apply闭环。
9. 同步README/module/memory；只有真实集成通过的capability才置supported。
10. Host Workspace首切片：strict payload与Go wire、root配置binding、bwrap prepare/execute、
    exact process identity Inspect/Fence/Release/Cleanup、重启/lost-reply/PID-reuse/symlink/path/network反例。
11. MicroVM首切片：strict payload、QEMU/KVM fixed argv、kernel/initramfs digest、allocate/boot/Inspect/
    Fence/Release/Cleanup；真实kernel+initramfs启动与KVM不可用/host drift/lost reply反例。

第11项已于2026-07-18完成真实KVM门：guest独立内核启动与退出、运行中exact Inspect、Release拒绝、
Fence确认process identity消失后才完成、Cleanup residual fail-closed均通过。guest agent、块设备、
Snapshot、Secret、network allow-list与宿主部署认证不在该首切片。

## 3. 验收命令

```bash
cd ExecutionRuntime/sandbox/dataplane
cargo fmt --check
cargo test --all-targets
cargo test --all-targets --release
cargo clippy --all-targets --all-features -- -D warnings

cd ..
go test -count=100 -shuffle=on ./contract ./kernel ./tests ./dataplaneadapter
go test -count=20 -race -shuffle=on ./...
go test -count=1 -shuffle=on ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
```

真实containerd门另执行digest-pinned image的prepare/start/inspect/fence/release/cleanup和daemon restart
故障；真实WASM门执行WIT component normal/trap/fuel/epoch/memory/unknown-import矩阵。任何Residual、
flaky、Provider调用绕门或Fake替代真实daemon均判失败。
