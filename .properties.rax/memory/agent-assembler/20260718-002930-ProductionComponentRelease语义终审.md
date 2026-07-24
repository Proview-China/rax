# Production ComponentRelease 语义终审

## 事件

2026-07-18，对 Agent Assembler owner 范围内的 production `ComponentReleaseV1` 做终审。现场确认原合同只要求 production certification ref 非空，且除 Harness 外的 fixture release 没有 Factory/Port/Capability 构造闭包，存在把自报 production 当成可构造 production 的缺口。

## 最小修复

- production release 的 Manifest provided capabilities 必须与 provided CapabilityDescriptor、Module capabilities、Factory output capabilities、PortSpec owner projection 一一闭合；
- Module 必须 exact 绑定 Manifest digest、Release revision、Artifact、SemanticVersion、Locality、Residual 与 Owners；Factory 必须 exact 绑定 Module 和 Artifact；
- CapabilityDescriptor 的 TTL、Schemas、Version、Conformance 必须与 Manifest capability 一致；required descriptor 集合必须与 Manifest required capability 集合一致；
- 所有 capability、module、factory、port、schema identity 拒绝 duplicate 和 alias；
- remote locality 不豁免 Factory，远程组件必须通过 host adapter factory 进入构造闭包；
- `ComponentReleaseCertificationDigestV1` 覆盖 Release identity/revision、Manifest、Artifact、Contract、Conformance、构造闭包、Evidence 和 validity window；production `CertificationRef.Digest` 必须等于该摘要；
- Catalog snapshot 对同一 `ReleaseID` 只允许一个 current revision，历史 revision 不能同时作为 resolver candidate。

## 反例

- remote component 缺 host adapter factory：拒绝；
- 缺 PortSpec：拒绝；
- CapabilityDescriptor TTL splice：拒绝；
- duplicate factory capability alias：拒绝；
- Module artifact splice：拒绝；
- Certification digest 或 Manifest contract 漂移：拒绝；
- 同 ReleaseID 多 current revision：拒绝。

## 软件验收

- targeted ordinary 100：PASS；
- targeted race 20：PASS；
- full ordinary / full race / vet：PASS；
- Plan determinism fuzz 2 秒 189 execs：PASS；
- production certification fuzz 2 秒 190 execs：PASS；
- import boundary、gofmt、diff-check：PASS。

## 边界

本次只证明 Assembler 公共 Release 合同与 reference testkit 能拒绝伪 production。真实组件仍必须由各 Owner 发布对应 production adapter/factory/port/certification；该终审不替代 Owner conformance，也不授予 Host activation、authority 或 dispatch。
