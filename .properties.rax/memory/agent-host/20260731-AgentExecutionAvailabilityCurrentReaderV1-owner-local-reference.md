# AgentExecutionAvailabilityCurrentReaderV1 owner-local/reference

- P1 裁决已落实：Availability Config 删除 caller `DeploymentV2Ref`。
- 新增 Host-owned immutable `HostStartPackageSelectionBindingV1`，绑定 exact
  Claim/InputV3 BindingDigest/DeploymentV2/Selection/Verified Closure digest，
  revision=1，TTL 为完整最小值。
- 新复合端口在 SQLite schema v8 同一 transaction 写 Claim、InputV3 sidecar 与
  Binding；旧 V3 port 不变，旧 Claim 不补写升级。
- Unknown outcome 只 exact Inspect 原 BindingRef 一次；不重试 mutation、不替换
  DeploymentV2/Selection。
- Availability 每次按 Claim→Binding→DeploymentV2→Selection exact/current→
  Verified Closure 读取，并把全链纳入 S1/S2、fresh clock 与 TTL。
- owner-local/reference tests 覆盖 lost reply、restart、v7→v8、8 handles/64 same
  Claim、association conflict、splice、Selection drift、expiry 与只读 Config。
- production 仍 NO-GO：HostV3 production Start 尚未调用新复合端口，Runtime
  actual-point guard 尚未接 Host availability；不得宣称 Availability closed。
