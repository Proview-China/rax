# Context Tool-only Application V1

## 结论

Context per-turn Refresh 允许仅携带一个已结算 Tool Result，而不要求 Memory 或 Knowledge 来源存在。

    exact settled Tool Result
    -> ContextTurnRefresh prepare
    -> immutable Manifest / Frame / Generation
    -> current CAS

Memory 与 Knowledge 是可选增强来源。二者存在时，原有 S1/S2 stable association anti-splice 必须继续执行；二者均不存在时，stable source set 与 association set 使用既有空集合规范摘要，不能跳过摘要绑定或 current CAS。

## 精确边界

- Context kernel/contract 继续要求 Cardinality.Tool == 1，并继续由 Tool exact-current reader验证来源。
- Application Prepare/Apply 合同接受 Memory/Knowledge 均为空。
- Application Coordinator 只强制要求 Context Port；Owner Reader仅在对应请求存在时必需。
- Context Application Adapter 只强制要求 Context Service、Owner Backend、Transition Proof Store与Content Store；空 Owner来源不得被伪造为零值贡献。
- Apply仍绑定 Prepared中的稳定来源集合摘要，并为S2空集合生成确定摘要。

## 不变项

- 不修改 AppendSettledToolResult API。
- 不改变 Memory/Knowledge 存在时的 S1/S2 exact request、current inspection、stable digest与association digest校验。
- 不创建 Console、RPC、页面ViewModel或公共TS合同。
- 不声明 Harness continuation、下一轮Model调用或production composition已经完成。

## 验收

1. Tool=1、Memory=0、Knowledge=0的真实Application Coordinator调用成功。
2. 生成的Manifest至少包含精确ToolResult，且不包含Memory/Knowledge fragment。
3. Result中的Manifest/Frame/Generation exact ref可分别读取，并相互精确绑定。
4. current指向同一Generation exact ref与ordinal。
5. Inspect原Attempt返回同一已应用结果。
6. 既有Memory/Knowledge S1/S2 drift测试继续通过。
7. Application与Context相关包的ordinary/race/vet及diff-check通过。
