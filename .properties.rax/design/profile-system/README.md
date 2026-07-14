# Profile System与Runtime交接

## 1. 状态

- 当前阶段：Runtime-facing分层、不可覆盖和策略缺省语义已修正；Profile存储与继承算法仍待单独设计；
- 实现授权：无。

## 2. 三种视角

| 视角 | 产物 |
|---|---|
| 使用者 | 简洁、可组合的AgentProfile |
| 开发者 | ComponentProfile、Schema、Capability和扩展命名空间 |
| 执行系统 | 完全展开、版本固定的ResolvedAgentProfile |

Runtime只消费Resolved结果。Profile不能成为隐藏改变AI行为、权限、Provider合同或组织职权的万能配置。

## 3. 不可覆盖优先级

```text
厂商不可变合同
> Route/Deployment固定约束
> Authority、Policy、Entitlement
> 用户组合Profile
> 单次Invocation
> Provider扩展默认值
```

低层只能收紧权限。强制能力缺失确定性拒绝；允许降级必须预先声明并产生Residual。

## 4. Runtime策略字段

Profile Schema必须能够表达Risk Class、online/offline验证、撤销延迟、时钟偏差、Conflict Effect Domain、Harness Conformance、Cache分区、Remote Continuation、Budget Authority引用和Evidence Policy。具体运行值必须显式配置、由用户审核并通过合同测试；未配置时禁用相关高风险能力。

## 5. Model Profile

最终执行继续使用精确Route选择和`ModelBehaviorProfile × HarnessCapabilityProfile × RuntimePolicy`。新Model/Harness扩展必须提供Manifest、MappingReport、Residual和证据TTL，不能只注册名称。

## 6. 进入Profile自身Plan的门槛

- Schema、版本不可变与来源解释；
- 继承、覆盖、null/delete和冲突算法；
- Secret引用与敏感字段；
- Runtime风险、Cache和Effect策略的缺省拒绝语义；
- 用户确认首批Profile类型和迁移规则。

