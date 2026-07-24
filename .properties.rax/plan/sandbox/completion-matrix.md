# Sandbox Documentary 完成矩阵

事实源：`tmp.document/Sandbox.md`。状态只按 live 代码与实际测试更新。

| 文档能力 | Sandbox-owned 状态 | production 外部门 |
|---|---|---|
| 逻辑 Sandbox、Requirement/Policy/Placement | 完成 | 无 |
| Runtime Lease/Fence/Instance 边界 | exact-current 与双 Enforcement 完成 | Runtime 继续唯一写 Owner |
| Host Workspace | bwrap 真实后端完成并实测 | 部署 binding/attestation |
| Container | containerd/OCI 真实后端完成并实测 | 镜像与 registry credential 供应链 |
| MicroVM | QEMU/KVM 独立 kernel、Fence/Cleanup 完成并实测 | guest agent、块 Snapshot、Secret 供应链按 capability 保持 unsupported |
| WASM | Wasmtime Component/WIT 与 Capability Gateway 完成并实测 | Tool Owner 的组织级部署与认证 |
| Remote | neutral SPI、UDS connector、exact Inspect 完成 | 厂商 connector/certification |
| Workspace View/Overlay/Diff/Commit | 完成 | Workspace Gateway 的部署证明 |
| Lifecycle/Evidence/Settlement/Cleanup | 完成 | 宿主 Runtime/Application 注入与 attestation |
| Sandbox Owner State Plane | SQLite 单节点持久闭环完成 | 分布式/HA 不在当前能力承诺 |
| Checkpoint Participant | prepare→commit XOR abort、Provider、Evidence/Settlement/Apply、Participant/Coverage 完成 | Runtime/Continuity 宿主协调与认证 |
| Snapshot capture | Provider artifact→canonical bundle→AES-GCM content→reserved→available 完成 | Retention/Legal Hold terminal lifecycle |
| Restore | fresh Instance/epoch/Lease/Fence、bundle current、host-local stage、Settlement/Apply 完成 | Runtime Activation/Ready 系统编排 |
| Snapshot purge/delete/cleanup | Sandbox Port Delta 完成，未实现 | Runtime sibling、Retention proof、Management terminal DTO |
| SDK/API/CLI | 完成 | transport credential/secret rotation |
| Host root | API/current Reader 同生命周期、`livez/readyz` 完成 | Agent Host factory/provider/phase 注册 |
| Assembly release | exact descriptor/readiness/Pub lost-reply 完成 | deployment attestation 与独立 Certification Fact |
| SLA/升级/跨机恢复 | 测试与 readiness 输入完成 | 真实部署环境验收 |

## 当前裁决

Sandbox 规划内可由本组件独立实现的生产接线已经完成。剩余项均需要唯一外部 Owner
产生真实 current fact 或部署证明，Sandbox 不得以私有 DTO、Fake 或自签结果补齐：

```text
Retention/Legal Hold Owner
  + Runtime Snapshot purge/cleanup sibling
  + Management terminal DTO
  + Agent Host registration
  + Deployment attestation/certification
  -> production release
```

在该链完成前：

- `FeatureSnapshotArtifactCapture=true`；
- terminal `FeatureSnapshotArtifactOwner=false`；
- Component Release 为 `standalone`；
- 不声称 production deployment 或 SLA。

## 已完成的收口顺序

1. 普通 lifecycle 与 Workspace governed commit；
2. Checkpoint Provider/Participant/Evidence/Settlement/Apply；
3. Provider artifact→Snapshot content→Artifact available；
4. Snapshot current→fresh Instance Restore stage；
5. SQLite、SDK/API/CLI、Host root、Assembler release/readiness；
6. Host/containerd/QEMU-KVM/Wasmtime 实机黑盒；
7. 全量 ordinary/race/vet/clippy/fmt 与资产真值同步。
