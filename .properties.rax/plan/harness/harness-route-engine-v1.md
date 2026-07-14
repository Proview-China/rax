# Harness Route绑定与公共引擎接线v1

状态：执行中（2026-07-14）。

## 1. 范围

- 落点：`ExecutionRuntime/harness/engine`及Harness现有Kernel/Runtime Adapter的最小线性化修复；
- 资产：Harness同名design、plan、module、memory；
- 允许：Expected/Actual Route摘要门禁、本地参考引擎组装、RunID提交线性化、失败Run可Inspect、全套测试；
- 禁止：具体生产Route、model-invoker实现导入、真实Tool/MCP、Runtime Port修改、生产Topology/Backend/SLA。

## 2. 粗粒度清单

1. [x] 核验现有Harness完成面和Runtime/Harness公共合同；
2. [x] 冻结Route绑定、公共引擎和验收合同；
3. [ ] 实现RouteBinding校验与本地参考引擎；
4. [ ] 修复RunID并发提交和失败Run Inspect关联；
5. [ ] 补齐单元、白盒、黑盒、故障注入和Conformance；
6. [ ] 运行稳定性、Race、Vet、覆盖率并同步module/memory。

## 3. 细粒度任务

- Route所有摘要在构造Loop前逐项验证；
- Conformance与Control能力必须与Manifest精确一致；
- nil Port、零上限、过期Manifest和Digest漂移全部拒绝；
- 公共引擎返回Data Plane Loop与宿主Runtime `ExecutionPort`两个明确表面；
- Context/Model/Event仍只经现有窄Port，不跨组件导入实现；
- Context准备后的Session提交再次检查RunID唯一性；
- Adapter在错误返回前仅在Run实际存在时记住Run关联；
- 增加同RunID跨Scope并发、Model失败Inspect、Event故障和两级Conformance测试；
- 生成Runtime Cleanup Inspect Port Delta但不修改Runtime。

## 4. 预期产物

```text
ExecutionRuntime/harness/engine/
.properties.rax/design/harness/route-engine/
.properties.rax/design/harness/port-deltas/
.properties.rax/plan/harness/harness-route-engine-v1.md
```

以及Harness现有Kernel/Adapter对应的最小测试增量、更新后的模块说明和粗粒度Memory快照。

## 5. 完成条件

- Route漂移在任何Context/Model/Event调用前Fail Closed；
- 公共引擎经Runtime `ExecutionPort`跑通direct路径和两级Conformance；
- Event失败不派发Model；Model失败Run仍可从Runtime边界Inspect；
- 同RunID不能跨Scope并发覆盖Session；
- 普通、20轮、Race、Vet、覆盖率与`git diff --check`全部完成；
- 仅修改Harness独占实现和资产目录。
