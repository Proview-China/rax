# Governed V3 Provider Injection Compare A2 设计冻结

## 1. 目标与 Owner

- 唯一 Owner：`Model Invoker`。
- 本切片只把 A1 的 `ActualProviderInjectionDigest` canonical 合同接入现有 V3 pre-invoke reference path。
- 本切片不创建 Provider Boundary、不调用 Runtime guard、不调用 Provider、不写 Provider Observation。
- Credential exact current、durable Provider Attempt/Observation与真实Provider执行仍为独立 HARD NO-GO。

## 2. 唯一执行顺序

```text
V3 S1/S2 exact inputs
→ prepareAt成功
→ 安装并保留adapter lease retire defer
→ ComputeActualProviderInjectionDigestV1(preparedProvider.request)
→ exact比较prepared2.ActualProviderInjectionDigest
→ 现有Route/Model校验
→ credential current HARD NO-GO
```

计算输入必须是`prepareAt`返回的`preparedProvider.request`。不得使用仍为RouteID-owned、Provider为空且Protocol为Auto的`material2.Call.Request`。

## 3. Fail Closed

- Compute失败或digest不等统一返回governed `Conflict`；
- 错误不暴露ProviderOptions正文；
- 任何失败均通过既有defer回收adapter lease，并保留release错误证据；
- Provider Boundary history、Runtime guard、Provider Invoke与Observation写入必须保持0；
- canonical等价的JSON key order、whitespace及A1已冻结的Unicode scalar不得形成误报。

当前V3 Invocation Material合同禁止`ProviderOptions`进入RouteCall，因此A2真实full-fixture只验证Tool Parameters的key order、whitespace和paired-surrogate/literal-scalar等价。ProviderOptions的正向canonical等价由A1永久套件证明；A2保留Prepared absent→present empty/body的反向漂移用例，不为不可达正向路径增加测试钩子或生产helper。

## 4. 接线边界

允许修改：

- `ExecutionRuntime/model-invoker/routegateway/governed_v3_actual_boundary.go`；
- 对应`tests/routegateway`黑盒；
- 本切片同名设计、计划、模块与memory资产。

禁止修改：

- A1 canonical合同；
- Provider、Harness、Runtime、Context、Tool与Console；
- routegateway公共执行权或production composition；
- B/C后续合同。

## 5. 验收

- fixture使用真实concrete Provider/Protocol request计算Prepared digest；
- exact match继续命中Credential HARD NO-GO；
- Provider、Protocol、Tools、ToolChoice、Parallel与ProviderOptions逐轴漂移均在Boundary前拒绝；
- Parameters canonical等价输入通过A2真实比较；ProviderOptions正向等价继续由A1证明；
- mismatch与match路径都能正确回收lease；
- Boundary/guard/provider/observation为0；
- targeted ordinary×100、race×20及Model full ordinary/race/vet/import/diff全部通过。
