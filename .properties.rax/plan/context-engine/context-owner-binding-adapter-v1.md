# Context Owner Binding Adapter V1 实施计划

## 状态

用户已冻结语义并授权直接施工；本计划已执行，作为陈旧计划保留。

## 范围与产物

- `ExecutionRuntime/context-engine/modelinvokeradapter/context_owner_binding_v1.go`；
- 同包ordinary、failure、blackbox、64并发和import-boundary测试；
- Context Engine `go.mod`对Model Invoker公开合同的直接依赖；
- 同名design、plan、module与memory资产。

不修改Model、Runtime、Harness、Tool、Sandbox、Application、Host或Provider，不新增
Store、production root、Provider调用或Context/Model写路径。

## 执行清单

1. 将Model request严格绑定为Context固定Material Kind和exact source；
2. 调用同一Context lineage current reader完成S1/S2；
3. 验证完整Owner、Material/Frame两个Kind、全部exact coordinate、TTL/current/digest；
4. 对完整S1/S2 projection做值相等检查；
5. 逐字段无损映射Context Owner与Material/Frame；
6. 直接绑定Context authoritative lineage digest；
7. 使用fresh clock和最小expiry调用Model公开sealer；
8. 覆盖字段漂移、角色互换、history/current、时钟、错误与并发窗口；
9. 执行focused ordinary×100、race×20、模块full ordinary/race/vet、tidy diff、
   verify、gofmt、diff与import范围门。

## 完成条件

- 所有成功结果可由Context与Model公开合同独立复算；
- 所有不确定、过期、漂移和依赖缺失路径只返回零projection；
- Model/Provider/Harness/Tool实际调用计数为0，除Model公开DTO映射/sealer外无Model行为；
- 改动只位于Context Owner允许范围和同名`.properties.rax`资产；
- 完成后保持未提交，交给独立审计；
- Frame durable current reader已由独立PR #57合并；本计划只验证经lineage窄Reader共存，
  不承担RouteCall lowering或Harness production composition，dispatch继续`NO-GO`。
