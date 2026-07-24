# Agent Host H4 Production Lifecycle V2 设计候选

## 事件

H3真实Definition -> Assembler -> Harness Compile纵切完成后，live审计确认HostV1的`binding -> constructing -> verifying`无法容纳真实production Activation与Generation-Binding Association：Activation依赖已经构造的零副作用Control Plane adapters，Association又依赖current Activation。

因此形成additive HostV2设计候选，明确：

- HostV1字段、JSON、摘要、Journal阶段和行为不变；
- HostV2新增`constructing_control -> activating -> associating_generation -> verifying`；
- Application拥有启动协调Journal，Runtime拥有Binding/Activation/Run事实，Harness拥有Assembly artifacts，组件拥有production current；
- Host只聚合exact refs与S1/actual-point/S2 Readiness，不自签production；
- 当前等待用户确认，未新增HostV2代码或公共Port，H4/H5与SYSTEM_READY继续NO-GO。

## 资产

- [Design](../../design/agent-host/h4-production-lifecycle-v2.md)
- [DrawIO](../../design/agent-host/h4-production-lifecycle-v2.drawio)
- [Plan](../../plan/agent-host/h4-production-lifecycle-v2.md)

