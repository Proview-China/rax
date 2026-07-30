# Workspace Read Exact Current V1 Plan

## 已完成

- Sandbox-owned exact Query/Projection/Reader；
- sealed Physical Authorization 对 StableKey、Authorization、Association 与 DomainCommand 的证明绑定；
- Association、Command、WorkspaceView、Runtime current 的双读语义一致性；
- 自然最小 TTL、完整 ProjectionDigest 与稳定 SemanticDigest；
- Data Plane CurrentServer 的可选 exact handoff；
- 64 并发、fresh Runtime envelope、Owner 轴漂移、lost reply 与 reader rebuild 测试。

## 后续

下一切片必须在 actual-point caller 显式传入 sealed Authorization 与 Association exact ref 后启用 exact handoff。不得从 Runtime current、label、digest 字符串或 caller 自报字段反推这些证明。
