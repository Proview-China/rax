# Package离线验签与强Admission Owner闭环

时间：2026-07-23 21:27:50 CST

## 事件

- Runtime public `SupplyChainArtifact/Trust V1` neutral nominal与exact Readers已经落盘，包含
  Artifact、Trust Material、Trust Policy Document及immutable Trust Policy Current。
- Tool Owner直接复用Runtime public类型，完成OCI content-addressed Artifact、官方Sigstore Go
  Bundle与in-toto Statement离线验证；证书模式与key PEM模式分开校验，没有自造签名或证明协议。
- `packageverify`维护create-once Observation、authoritative Fact与immutable current projection；
  lost reply只Inspect相同stable subject，Unknown/Unavailable不触发Fetch或换Attempt。
- verification-aware强Admission在同一Registry锁/CAS内复读Package current、Verification
  Fact/current、Trust Policy/Policy Document与Artifact exact；generic Package
  `admitted|active`生产transition Fail Closed，Admission不自动active/enable。
- SDK已提供sealed exact Verify、Observation/Fact/current Inspect和强Admission；transport-neutral
  API提供exact双读；可嵌入CLI提供`package verify --request-json`且严格拒绝unknown/trailing JSON。

## 验证真值

- 临时overlay诊断门通过：targeted ordinary×100、race×20、Tool full ordinary/race、vet；
  Runtime Supply Chain ports targeted ordinary×100/race×20及ports/kernel ordinary/race/vet。
- 官方Sigstore/in-toto离线key-bundle正向Conformance与tampered artifact反例通过。
- live门仍被Runtime目录内未完成的`agent_activation_v2.go`外部编译错误阻断；本事件不能写成
  live独立审计YES，也不代表production GO。

## 保留边界

- Fetch/Install/Enable、在线Transparency查询、production Artifact/Trust backend与composition
  root未实现。
- Package Verify是纯离线计算；任何material网络获取必须作为独立受治理Effect。
- 本模块继续使用Go；没有Rust热点基准证据。
