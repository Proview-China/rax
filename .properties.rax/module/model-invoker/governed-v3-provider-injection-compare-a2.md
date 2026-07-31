# Governed V3 Provider Injection Compare A2 模块说明

## 1. 作用

本模块在V3 routegateway完成可逆Provider准备后，重新计算实际目标工具注入形状，并与Prepared历史事实封存的`ActualProviderInjectionDigest`逐字节比较。

它证明“准备发送给目标Provider的工具注入控制面没有漂移”，但不授权调用Provider。

## 2. 数据流

```text
preparedProvider.request
→ ComputeActualProviderInjectionDigestV1
→ exact Prepared digest compare
→ existing Route/Model validation
→ credential current HARD NO-GO
```

## 3. 错误与清理

- 非canonical shape或digest mismatch返回governed Conflict；
- adapter lease始终由既有retire defer回收；
- release失败与主错误合并为可观察证据；
- ProviderOptions正文不进入错误文本。

## 4. 非能力

本模块不：

- Ensure Provider Boundary；
- 调用Runtime guard；
- 调用Provider；
- 写Provider Observation；
- 实现Credential、Provider Attempt或Console合同。

## 5. 验证入口

测试位于`ExecutionRuntime/model-invoker/tests/routegateway/`，覆盖真实fixture、逐轴漂移、canonical等价、release failure和零Boundary/Provider副作用。

V3 Material上游当前禁止ProviderOptions，因此本模块不增加生产helper或测试钩子伪造该正向路径；ProviderOptions canonical正等价由A1套件负责，A2验证其Prepared absent→present/body漂移必然拒绝。
