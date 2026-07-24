# Tool Package离线Verify与强Admission软件门YES

时间：2026-07-23 21:30:00 +08:00

## 事件

Tool/MCP Owner完成Package owner-local离线验证与verification-aware强Admission：

- Runtime public `SupplyChainArtifact/Trust V1` neutral refs、exact Readers与Trust Policy current
  被直接消费，没有复制共享nominal；
- 使用官方OCI image-spec、in-toto attestation和Sigstore Go验证State Plane中已有的exact材料；
- Verification按`Observation -> Fact -> Current`持久，lost reply只Inspect同一canonical记录；
- Package从`submitted -> admitted`在同一Registry锁/CAS内复读Package current、
  Verification current、Trust Policy与Artifact exact；generic Package admitted/active路径Fail Closed；
- SDK提供Verify/Observation/Fact/Current Inspect与强Admission，transport-neutral API提供exact
  双读，模块内CLI提供`package verify --request-json=<sealed exact request>`；
- same Verification但改变Registry CAS source被拒绝，Admission回包丢失按exact winner恢复。

## 实际验证

- Package targeted ordinary×100：PASS；
- Package targeted race×20：PASS；
- Tool/MCP `go test ./... -count=1`：PASS；
- Tool/MCP `go test -race ./... -count=1`：PASS；
- Tool/MCP `go vet ./...`：PASS；
- Runtime Supply Chain ports ordinary×100、race×20、vet：PASS；
- official Sigstore离线key Bundle正向Conformance及Artifact tamper反例：PASS。

## 边界

本事件只表示`implementation_software_test_yes`，不表示production闭环。Package Fetch、Install、
Enable、在线透明日志freshness/撤回、production Artifact/Trust/Registry backend、credential和
composition root仍为NO-GO。
