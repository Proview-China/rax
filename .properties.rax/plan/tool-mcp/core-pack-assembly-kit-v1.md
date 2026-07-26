# Core Pack Assembly Kit V1 Plan

## 1. 目标

在不实现真实Effect的前提下，为官方Core Tool Pack提供Tool-owned确定性装配入口，让Harness
快速取得reference preview或verification-aware admitted closure。

## 2. 文件边界

Implementation PR预计只修改：

    ExecutionRuntime/tool-mcp/corepack/assembly_kit_v1.go
    ExecutionRuntime/tool-mcp/corepack/assembly_kit_v1_test.go
    ExecutionRuntime/tool-mcp/corepack/conformance_kit_v1_test.go
    ExecutionRuntime/tool-mcp/corepack/harness_fixture_v1_test.go

若现有SDK公开方法足够，不修改contract、Harness或Sandbox。确需Tool内部helper时仍留在
corepack或sdk，不得升级为跨Owner公共合同。

## 3. 实施阶段

### P0：Design PR

- [x] 冻结preview与admitted状态机；
- [x] 冻结Package verification/admission复用边界；
- [x] 冻结non-executable与Provider-call-zero要求；
- [x] 冻结Harness test-only fixture边界；
- [ ] 主控审查并合并Design PR。

### P1：Preview facade

- [ ] 组合Catalog、Register、Snapshot与Surface；
- [ ] Package必须保持submitted；
- [ ] seal reference-only/non-executable Result；
- [ ] deterministic、clone、typed-nil、cancel、TTL测试。

### P2：Admitted facade

- [ ] 复用现有PackageVerificationV1与Admission；
- [ ] lost reply只Inspect；
- [ ] active Package exact复读；
- [ ] S1/S2 Snapshot与PackageAssembly closure；
- [ ] seal admitted/non-executable Result。

### P3：Conformance与Harness fixture

- [ ] canonical五工具golden；
- [ ] Package到Tool、Capability、Schema、Material和Surface exact closure；
- [ ] Harness现有公开合同test-only compile；
- [ ] execution unsupported且Provider call count=0；
- [ ] stale、drift、ABA、Unknown、cancel与64并发反例。

### P4：软件门

- [ ] targeted ordinary count=100；
- [ ] targeted race count=20；
- [ ] full ordinary、race、vet；
- [ ] gofmt、forbidden-import、diff-check；
- [ ] Draft Implementation PR，不合并。

## 4. NO-GO

- Design PR未合并前不写Go；
- Package未active不得产出admitted；
- Sandbox被冻结，不新增依赖或临时Effect；
- 不修改Harness公共合同或生产文件；
- 不把fixture、in-memory Registry或unsupported Factory称为production实现。
