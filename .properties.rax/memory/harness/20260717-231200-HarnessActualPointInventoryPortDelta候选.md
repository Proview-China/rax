# Harness Actual-Point Inventory Port Delta候选

## 事件

Harness共用接线Goal完成live依赖DAG复核，并把Model PreDispatch现状分为两层：

- Harness Owner-local `A2+B1+C2` Assembly Current、Runtime neutral Current Reader、concrete Model Gate与同实例ACK create-once Repository已经实现、测试并通过对应独立代码审计；
- Model实际Provider执行路径仍缺可被Harness Assembly独立穷举核验的公开actual-point inventory；现有`model.dispatch.before` Phase、Gate类型或ACK绿测不能证明production no-bypass。

## 新增资产

- `design/harness/port-deltas/model-predispatch-actual-point-inventory-v1.md`；
- 状态为`candidate`，等待Model Owner与Harness Assembly联合设计审定；未授权新增Model/Harness Go。

Delta要求Model只发布versioned sealed candidate/Observation，Harness仍以AssemblyInput、Component Manifest、Module/PortSpec/Factory/ProviderCandidate及production wiring inventory独立交叉并seal Conformance。Receipt仍只是Observation，不成为Permit、Evidence、Binding Fact或进入权。

## 同步真值

Harness current design/plan/index已从旧`M2 candidate-NO-GO`纠正为`owner-local implementation_software_test_yes`，并继续明确以下系统门未通过：

- Model actual-point全路径guard inventory与代码审计；
- Harness专用required Capability/Factory/Binding/no-bypass Conformance；
- Tool V2 Consumer、system G6A/G6B与production root。

## 机械验证

- Markdown relative links：PASS；
- draw.io XML：PASS；
- trailing whitespace：0命中；
- scoped `git diff --check`：PASS；
- Port Delta SHA-256：`c5a3da474588afe10f8883dcd59b72b9424400f16097261e9baaacf82d344d44`。

本事件不声称production capability、真实Backend、SLA或system闭环。
