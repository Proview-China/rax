# Profile、Model Route与Runtime交接

## 1. 三阶段产物

```text
AgentProfile（使用者）
+ ComponentProfile（开发者）
+ 组织、权限、凭据和能力事实
       |
       v
Profile Compiler + Agent Assembler
       |
       v
ResolvedAgentProfile + ResolvedAgentPlan
       |
       v
Runtime Static Admission / Activation
```

Runtime只读取完全展开、版本固定、可摘要且不含Secret明文的Resolved结果；不在启动时重新合并Profile。

## 2. Plan与Lineage

InstanceLineage固定绑定一个Resolved Plan Digest。Agent Definition、最终Profile、Harness、Model Route、Provider、Execution Surface、强制Sandbox能力、Context语义或工具可见面变化创建新Plan和新Lineage。

## 3. Model Profile

```text
ProfileSelectionKey
  = provider + model_id/revision + deployment/region
  + protocol + offering/auth_route + execution_surface
  + harness/harness_version

EffectiveProfile
  = ModelBehaviorProfile
  × HarnessCapabilityProfile
  × RuntimePolicy
```

Resolved Plan必须携带Route fingerprint、Expected Injection Manifest、MappingReport、CapabilityResiduals、Harness stack digests和缓存能力事实。Direct API、SDK、CLI和App Server不能仅因模型名相同复用最终Profile。

## 4. Runtime-facing领域

| 领域 | 交接内容 | 所有者 |
|---|---|---|
| identity | Identity/Authority引用和最大范围 | Organization/Authority |
| harness | Conformance、Manifest、控制与不透明边界 | Harness Profile |
| invoker | 精确Route、语义映射、Effect和远程能力 | Model Invoker |
| context/cache | ContextPackage、CachePlan、分区和Retention | Context Engine |
| tools/mcp | Capability、Schema、Effect Domain | Tool/MCP |
| state | Candidate/Commit端口 | Memory/Knowledge/Asset |
| runtime | 生命周期、Fence、Risk、Failure Policy | Runtime Policy |
| sandbox | 隔离、资源、网络和Secret类型要求 | Sandbox Profile |
| extension trust | Publisher授权、Trust、撤销和证据 | 本次部署绑定的唯一Extension Trust所有者 |
| extension resolution | 命名空间、版本、Digest和Resolved引用 | Agent Assembler |

## 5. Activation事实

Resolved Plan是不可变设计事实；Authority epoch、Entitlement、Credential状态、证据TTL、Provider健康、Pricing和Budget Policy/requested cap是Snapshot前的live facts。真实Budget Reservation在Snapshot冻结后以EffectIntent取得，并在ActivationCommit前再次验证。Live fact缩小可以Fence当前实例；任何语义扩张需要新Plan。

## 6. 参数策略

`max_revocation_lag`、`max_clock_skew`、risk class、允许的offline效果和首个后端等值必须在对应Deployment/Profile/Security Policy设计中显式配置并审核。未配置即禁用相关能力，Runtime不得填隐式默认。

## 7. 最低反例

- `PROFILE-01`：同模型换Harness仍复用旧Route Profile，必须拒绝；
- `PROFILE-02`：Provider专属字段试图覆盖Authority门禁，必须拒绝；
- `PROFILE-03`：离线撤销参数缺失，离线Effect能力必须禁用；
- `PROFILE-04`：Profile alias解析到新Digest，必须创建新Plan/Lineage。
