# Model Tool Injection Pair Current V2 落地计划

状态：已按用户确认边界实现并通过独立敌手复审 P0/P1/P2=0，保留为陈旧计划；ready PR 发布已获授权。

## 范围

- Tool Owner 从同一个 authoritative closure reader 读取 `CompiledModelToolsV1` 与 `ModelToolInjectionMaterialV1`，再读取 exact current `ToolSurfaceManifestCurrent`。
- S1/S2 全量复读，逐项闭合 Owner、Kind、ID、revision、digest、Surface、ExpectedInjectionDigest 和 CompiledToolsDigest。
- 直接接收真实 `RouteCall.Request.Tools`，调用 Model PR #60 已合并的 `DigestGovernedModelTurnRequestToolSetV2`；不复制 Model canonical 算法，不构造假 `RouteCall`。
- 新增 request-scoped Model adapter，实现 `InvocationMaterialToolPairExactReaderV2`；Model 接口中的 caller digest 只做相等性检查，不作为事实源。
- ToolChoice、Provider 调用、Tool execution、Model 实现、Sandbox、Runtime、Harness、Context 与 durable action SQLite 不在范围内。

## 产物

- Tool-owned `ModelToolInjectionPairCurrentProjectionV2`。
- authoritative pair/current reader。
- Model-neutral exact-reader adapter。
- focused、conformance、splice、TTL、race、并发和零副作用测试。

## 验收

- add/remove/reorder/schema/strict/alias 与 stored/actual mismatch fail closed。
- surface/material/owner/kind/ref/digest splice fail closed。
- 清空并重算 ProjectionDigest 也不能让 source/expected/compiled/request 角色替代通过 Seal、Validate 或 adapter。
- S1/S2 drift、最小 TTL、`now == expiry`、clock rollback、typed nil、cancel、64 并发覆盖。
- focused `-count=100`、focused race `-count=20`、full ordinary、full race、vet、diff/import 全部通过。
