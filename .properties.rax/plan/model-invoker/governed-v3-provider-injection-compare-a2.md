# Governed V3 Provider Injection Compare A2 实施计划

## 1. 输入

- 基线：包含`PreparedProviderInjectionShapeV1`的最新`main`；
- 既有入口：`InvokeGovernedModelTurnActualBoundaryV3`；
- 既有纯函数：`ComputeActualProviderInjectionDigestV1`；
- 既有目标：`PreparedModelInvocationFactV1.ActualProviderInjectionDigest`。

## 2. 实施步骤

1. 在`prepareAt`成功并安装lease retire defer后计算actual digest；
2. 与S2 exact Prepared事实中的target digest比较；
3. Compute失败或不等时返回governed Conflict；
4. 保留后续Route/Model校验与Credential HARD NO-GO；
5. 将测试fixture的占位provider digest替换为真实concrete mapped request计算值；
6. 增加逐轴漂移、canonical等价、release与零副作用黑盒；
7. 运行高重复、race和全量门禁。

V3 Material当前禁止`ProviderOptions`进入RouteCall。不得为A2制造不可达的ProviderOptions正向fixture；其canonical正等价由A1套件证明，A2只保留absent→present/body漂移反例。

## 3. 完成条件

- A2比较发生在任何Boundary/guard/Provider动作之前；
- exact match不会解锁Provider；
- mismatch不会写事实或调用Provider；
- lease release错误仍可观察且不泄露内部材料；
- A1、V1/V2和相邻Owner目录零修改。

## 4. 后续残余

本计划完成后仍不具备真实Provider执行资格。以下工作继续独立冻结：

- exact Credential current；
- durable Provider Attempt/Observation；
- Runtime actual-point guard后的same-stack physical invoke。
