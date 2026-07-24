# Sandbox 测试矩阵

## Required

| 层 | 正例 | 反例/故障 | 通过标准 |
|---|---|---|---|
| contract unit | canonical ref/digest/TTL/clone | unknown field、typed-nil、presence/value、now==expires | fail closed、无 alias |
| admission | Requirement/Policy/Backend exact match | capability 缺失、raw bypass、未批准降级 | rejected/observe_only 明确 |
| lifecycle | allocate→activate→open→inspect→fence→release→cleanup | stale Lease/Fence/Scope、lost reply、NotFound | 独立 Effect；Unknown 只 Inspect |
| actual-point | prepare/execute current 一致 | 两门之间 Authority/Review/Budget/Binding 漂移 | Provider=0 |
| workspace | view→overlay→diff→commit | symlink、base/host drift、swap crash | 零越界写；cleanup 可收敛 |
| checkpoint | prepare→commit XOR abort | failed/not_applied 后继、unknown replay、双 closure | branch guard/no-ABA |
| snapshot | artifact→bundle→encrypted content→available | digest/TTL/executable-bit/current drift | exact replay、CAS 单赢家 |
| restore | new Instance/epoch/Lease/Fence→stage→Apply | 旧 Lease、host drift、lost reply | 不覆盖旧实例，不回滚外部世界 |
| persistence | restart/history/current/CAS | lost write reply、Unavailable、64 并发 | append-only、current 不回退 |
| API/host | auth/idempotency/watch/livez/readyz | bad payload、typed-nil、current server failure | 同生命周期关闭 |
| assembly | release/readiness/catalog | S1/S2 drift、TTL、missing attestation | standalone，不误升 production |
| import/layer | public ports only | Runtime/Harness/Application internal、raw side-effect import | 白盒扫描零违规 |

## Backend Conformance

| Backend | 自动/实机门 |
|---|---|
| bwrap Host | real execute/inspect/fence/cleanup；默认断网与 symlink escape |
| containerd/OCI | privileged local daemon：allocate/activate/inspect/cleanup，无残留 |
| QEMU/KVM | pinned kernel/initramfs/qemu digest；独立 kernel；fence/release/cleanup |
| Wasmtime | real Component；unknown import/trap/fuel/epoch/memory/digest drift |
| Remote | credential TTL、cross-request reply、lost reply inspect original |

## 必跑命令

```bash
cd ExecutionRuntime/sandbox
go test -count=100 -shuffle=on ./...
go test -count=20 -race -shuffle=on ./...
go test -race ./...
go vet ./...
gofmt -l .
git diff --check -- ExecutionRuntime/sandbox .properties.rax/design/sandbox \
  .properties.rax/plan/sandbox .properties.rax/module/sandbox .properties.rax/memory/sandbox

cd dataplane
cargo test --all-targets
cargo clippy --all-targets --all-features -- -D warnings
cargo fmt --all -- --check
```

受控本机实机门：

```bash
PRAXIS_TEST_KERNEL=<trusted-readable-kernel> \
  cargo test --test microvm_backend -- --ignored --nocapture

<privileged-containerd-test-binary> --ignored --nocapture
```

每次最终验收必须记录命令、退出码、ignored 原因、临时系统状态及恢复结果。Fake 与
software-only test 不替代 production deployment attestation 或 SLA。
