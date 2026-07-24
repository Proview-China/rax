# Sandbox 可实施计划

状态：`implemented / sandbox-owned closure complete / external production gates pending`。

最高业务输入：`tmp.document/Sandbox.md`。

## 1. 已完成阶段

| 阶段 | 产物 | 验收 |
|---|---|---|
| P0 合同/Owner | Requirement、Policy、Backend、Placement、Lease binding、Effect/Attempt、Unknown/Residual/Cleanup | strict shape/current、canonical、TTL、Owner 边界 |
| P1 Owner Core | Reservation、Observation、Inspect、DomainResult、opaque Settlement ref、Apply CAS | lost reply/no-ABA/64 路 CAS/历史 exact |
| P2 actual-point | Runtime current Reader、prepare/execute 双 Enforcement、Rust Enforcer/journal | 旧 epoch/drift/缺门 Provider=0 |
| P3 Backend | bwrap、containerd/OCI、QEMU/KVM、Wasmtime/WIT、Remote connector | unit/conformance/真实后端黑盒 |
| P4 Workspace | View/Overlay/diff/blob/governed commit | symlink/base/host drift、crash recovery |
| P5 Checkpoint | prepare→commit XOR abort、Provider artifact、Evidence/Settlement/Apply、Participant/Coverage | unknown Inspect、failed/not_applied 无后继 |
| P6 Snapshot | canonical Workspace bundle、AES-GCM Store、Artifact reserved→available | exact reread、TTL、digest、executable bit |
| P7 Restore | Snapshot current、fresh Instance/epoch/Lease/Fence、host-local stage、Apply closure | 旧 Lease/host drift/lost reply fail closed |
| P8 使用面 | SQLite、SDK、API/Watch、CLI、Host root/livez/readyz | 重启恢复、auth、idempotency、同生命周期关闭 |
| P9 Assembly | Component Release、Factory/Port/Slot descriptor、production readiness | S1/S2 drift、TTL、lost reply、独立 certification |
| P10 收口 | README/module/memory/completion matrix 与 live 代码同步 | ordinary100/race20/full race/vet/clippy/fmt |

## 2. Owner 与依赖 DAG

```text
Runtime Governance (Instance/epoch/Lease/Fence/Operation/Settlement)
    |
    v
Sandbox Go Owner Core + SQLite
    |
    +--> reverse-current UDS --> Rust Enforcer --> Backend
    |
    +--> Checkpoint artifact --> Snapshot content/artifact
    |                               |
    |                               v
    +-------------------------- Restore fresh Instance stage
    |
    +--> SDK/API/CLI --> Host root --> Agent Host factory registration
```

Sandbox 不导入 Runtime/Application/Harness/Assembler 实现包。跨域只依赖版本化公共
contract/ports。Provider Observation/Receipt 必须经 Owner Inspect/CAS 才能成为 Sandbox
DomainResult；Runtime Settlement 仍为 opaque exact ref。

## 3. 文件级落点

- `contract/**`：公共对象、版本、digest、TTL、clone 与状态机。
- `ports/**`：Owner/Reader/Provider/Conformance 窄接口。
- `kernel/**`：Admission、CAS、Workspace、Checkpoint、Snapshot、Restore。
- `runtimeadapter/**`、`applicationadapter/**`：公共 Runtime/Application 映射。
- `dataplaneadapter/**`：Go/Rust wire、current server、content/restore Host adapter。
- `dataplane/**`：Rust Enforcer、journal 与真实后端。
- `storage/sqlite/**`：Sandbox-owned 持久 State Plane。
- `workspacefs/**`：真实 Overlay capture。
- `sdk/**`、`api/**`、`apihandler/**`、`cmd/**`：使用面。
- `hostroot/**`：可信宿主组合与健康门。
- `release/**`：Assembler release/readiness。

## 4. 当前外部门

以下仍是 production release 的必要条件，但不属于 Sandbox 可独立写入的语义：

| 门 | 唯一 Owner | Sandbox 只提供 |
|---|---|---|
| no-active-legal-hold/current retention | Retention/Legal Hold | exact Reader/Port Delta |
| Snapshot purge/cleanup governance | Runtime/Evidence | neutral sibling mapping Delta |
| deleted/indeterminate terminal | Management | Artifact/current/tombstone Delta |
| factory/provider/phase 注册 | Agent Host/Assembler | exact release descriptor |
| deployment attestation/certification | Deployment/Management | readiness projection输入 |
| Tool gateway、image/guest artifact、remote vendor | 对应部署 Owner | backend capability binding |

这些门未落地时不得：

- 把 Snapshot delete request 当 deleted；
- 用 Checkpoint V5 扩义 Snapshot purge；
- 用 Provider NotFound 推导 cleanup/deletion；
- 让 Sandbox 反向拥有 Runtime/Continuity/Retention/Management 事实；
- 把 standalone candidate 宣称 production。

## 5. 测试矩阵

Required：

1. unit：strict decode、digest、TTL、clone/no-alias、状态机；
2. whitebox：依赖/import/Owner/零 Provider 门；
3. blackbox：lifecycle、workspace、checkpoint、snapshot、restore、API/host；
4. fault：lost reply、Unknown、CAS、restart、host/current drift、residual；
5. conformance：Host/Container/MicroVM/WASM/Remote；
6. concurrency：ordinary 100、race 20、full race；
7. static：gofmt、go vet、cargo fmt、strict clippy、diff/import scan；
8. live：bwrap、Wasmtime、containerd/OCI、QEMU/KVM。

精确命令与结果写入
`.properties.rax/memory/sandbox/20260723-213300-Sandbox生产接线最终收口.md`。

## 6. 完成定义

本计划的 Sandbox-owned 实施部分在上述 required 门全绿后结束。production release 仍必须由
外部 Owner 提供 current facts、宿主注册、deployment attestation 与独立 certification；
那是跨 Owner 部署验收，不得通过继续向 Sandbox 私增模块来“补完”。
