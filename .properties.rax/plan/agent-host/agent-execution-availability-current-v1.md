# AgentExecutionAvailabilityCurrentReaderV1 实施计划

## 已实施切片

1. 新增 `HostStartPackageSelectionBindingV1` contract、exact/ForClaim readers 与
   原子复合 Host port；
2. 新增 same-lock memory reference store 与 Unknown exact-Inspect admission；
3. SQLite schema v8、原子 Claim+InputV3+Binding 写入、exact/ForClaim Inspect；
4. Availability Config 删除 caller `DeploymentV2Ref`，改为
   Claim→Binding→DeploymentV2→Selection/Closure；
5. Binding/Selection/Closure 纳入完整 S1/S2 水印、fresh clock 与最小 TTL；
6. 保持 Runtime projection、V1/V3 对象、旧 V3 port、Console、Provider、Model、
   Harness 与 Runtime guard 不变。

## 验收矩阵

- Binding contract exact closure、逐轴 splice、TTL 与 `now==expiry`；
- lost reply exact Inspect once、restart、旧 V3 Claim 不升级、association conflict；
- SQLite sidecar fault 三行全回滚、row splice、v7→v8 migration；
- 8 handles/64 same Claim 与 Availability 64 并发读；
- caller Config 无 DeploymentV2Ref；Binding/Deployment/Selection/Closure/
  Claim/Input/SystemReady/Availability/Cleanup splice；
- Selection S1/S2 drift、clock rollback、typed nil、unavailable、cancel；
- targeted、ordinary×100、race×20、full ordinary/race/vet、import boundary 与
  `git diff --check`。

## 冻结边界

production 接线不在本计划内。只有 HostV3 production Start 在调用任何旧 Claim
mutation 前完成 authoritative DeploymentV2/Selection/Closure 校验，并调用新复合
端口，且 Runtime actual-point guard 另行冻结 Host availability 输入后，才可重新
评估 production GO。
