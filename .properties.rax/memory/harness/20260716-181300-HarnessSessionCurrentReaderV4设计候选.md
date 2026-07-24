# Harness SessionCurrentReaderV4设计候选

时间：2026-07-16 18:13（Asia/Shanghai）

## 事件

为Harness-owned Application P3 Assembler补充最小Session V4只读能力候选：新增public `SessionCurrentReaderV4`，只含既有`InspectSessionV4`；`SessionFactPortV4`兼容嵌入该Reader并保留Create/CAS。该候选等待独立设计短审，尚未写Go或执行实现测试。

## 冻结候选边界

- 不修改`GovernedSessionV4`、`SessionCASRequestV4`、canonical/digest、Store、冲突域或既有方法语义；
- 只实现Inspect的Reader无需Create/CAS；既有FactPort实现因方法集不变而天然兼容；
- P3 Assembler构造器只接收Harness public `SessionCurrentReaderV4`，禁止接收`SessionFactPortV4`、Store/fake具体类型或私有跨Owner接口；
- nil/typed-nil Reader必须`Unavailable/ComponentMissing`并在零下游读取、零Request Seal处Fail Closed；
- Application V2 public contract/ports实际落盘并compile前，P3 Go继续等待。

## 状态

既有Owner-current V3/V4最终代码审计`YES(P0/P1/P2=0)`不回退。本候选不代表P3、Tool Consumer、system G6A/G6B或production root通过，不自判设计短审YES。

## 关联资产

- [Owner-current Port Delta](../../design/harness/port-deltas/committed-pending-action-owner-current-inputs-v2.md)
- [Identity冻结反例矩阵](../../design/harness/assembly/model-tool-call-pending-action-identity-v1-test-matrix.md)
- [Identity实施计划](../../plan/harness/model-tool-call-pending-action-identity-v1.md)
