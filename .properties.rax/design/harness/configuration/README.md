# Harness配置编译与Bootstrap交接

## 1. 配置所有权

Harness不拥有Profile合成，也不在启动时重新猜测Model Family、Route、工具面或权限。上游按既有合同编译：

```text
EffectiveProfile
  = ModelBehaviorProfile
  × HarnessCapabilityProfile
  × RuntimePolicy
```

Profile System与Assembler拥有合成、映射和冲突解释；Harness只消费不可变`HarnessBootstrapPlan`。同一模型只要Execution Surface、Harness stack、版本、协议或Manifest不同，就必须形成不同Profile/Plan Digest。

## 2. BootstrapPlan固定内容

- ResolvedAgentPlan、Profile、RuntimePolicy和Harness stack摘要；
- Semantic Route引用与精确Model Route摘要；
- Expected Injection Manifest摘要；
- Context Plan、Tool Surface和MCP Surface摘要；
- Capability Grant、Review Policy、Evidence Policy摘要；
- Conformance最低要求、控制能力和允许Residual；
- 证据到期时间与配置版本。

BootstrapPlan只包含SecretRef或Brokered Capability引用，不包含长期明文Secret。Harness运行中不得热改上述摘要；配置漂移必须拒绝或创建新Plan/Lineage。

## 3. 配置优先级

```text
厂商不可变合同
> Route/Deployment固定约束
> RuntimePolicy与Entitlement
> 用户选择的组合Profile
> 单次调用覆盖
```

Harness只能在Plan显式允许的字段上做单次覆盖。未知字段、互斥Feature、被禁止的原生Tool、未声明的MCP或额外设置源默认拒绝，不做宽松合并。

## 4. Expected/Actual Manifest

Preflight必须形成Actual Manifest证据并与Expected Manifest比较。实际表面不得删除、替换或暗增Required组件、模型可见工具、强制指令、Workspace root、Secret路径和opaque boundary。允许的低风险漂移进入Residual；身份、权限、Secret、Sandbox和执行所有者漂移直接拒绝。

## 5. 当前最小实现

首个组件中立骨架只接受已经编译并带摘要的BootstrapPlan，不实现Profile数据库、Model Family私有映射、配置文件发现或厂商preset。测试使用固定Plan和fake依赖证明配置Digest、TTL、Capability和Manifest门禁。
