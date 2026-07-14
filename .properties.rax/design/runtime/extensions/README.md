# Runtime扩展体系与供应链合同

## 1. 目标

开发者可以替换Harness、Sandbox、Tool、Invoker、Context/Cache、Memory/Knowledge、Observer和未来Scheduler，而不修改Runtime Kernel。扩展自由度不能绕过Authority、Fence、Effect、Evidence和生命周期。

## 2. 稳定结构

```text
公共语义
-> Capability Descriptor
-> 责任域专属Provider/Adapter Contract
-> 命名空间扩展
-> 隔离运行的具体实现
```

Extension SDK按Harness、Sandbox、Tool、Context、Cache、Invoker、Observer等责任域拆分，不创建万能Provider接口。具体语言和进程拓扑尚未决定。

## 3. Capability事实

能力状态分为`declared`、`probed`、`certified`、`bound`和`revoked`。自报能力不能直接成为本次绑定事实。强制能力缺失或证据过期时拒绝；Optional能力只有Plan明确允许才可Degraded并产生Residual。

## 4. Extension Package

至少包含：

- 扩展ID、类型、版本、协议和Artifact Digest；
- 配置Schema、默认值语义和兼容规则；
- Capability Descriptor；
- Publisher、Trust Root和授权范围；
- 依赖锁、SBOM和构建来源；
- 认证范围、证据TTL和撤销状态；
- 运行隔离、Secret、文件和网络需求；
- 合同测试和失败证据；
- 升级、回滚和弃用信息。

签名只证明发布者，不证明扩展可信。安装、装配和Activation前都要检查Trust、Digest、撤销和证据TTL。

## 5. 运行时隔离与资源限制

- 第三方扩展必须满足“故障、资源耗尽和恶意输入不能破坏Kernel权威状态”的隔离属性，并提供可验证的等价隔离证明；是否采用独立地址空间、进程、容器或其他机制由后续获批拓扑决定；
- 限制CPU、内存、文件、网络、事件率、日志量、Payload大小和Schema深度；
- Secret按用途和时间最小暴露；
- 恶意或故障Health/Event不能拖垮Control Plane；
- 扩展不能写权威Ledger、分配sequence、决定终态或扩大Authority；
- Trust撤销后禁止新Activation，并按Risk Policy Fence活跃能力。

## 6. 升级与回滚

扩展升级必须形成新版本、Digest、Resolved Plan和Lineage，采用并存验证而非原地替换。回滚到先前Digest仍需新Plan/Lineage及当前Trust复验，不能复活旧Instance。

## 7. 命名空间

公共字段表达跨Provider稳定语义；高级能力进入扩展ID命名空间并参与摘要、审计和兼容检查。未知命名空间不能被Runtime猜测解释或跨Provider透传。

## 8. 失败与反例

```text
extension_trust_root_missing
publisher_not_authorized
artifact_digest_mismatch
certification_expired
extension_revoked
resource_limit_exceeded
unknown_extension_namespace
```

- `EXT-01`：合法签名但Publisher不在部署Trust范围，必须拒绝；
- `EXT-02`：扩展持续产生无限Health事件，必须限流、隔离并标记Unhealthy；
- `EXT-03`：Observer试图直接写权威Ledger，必须拒绝；
- `EXT-04`：扩展Digest变化但版本号不变，必须视为新供应链事实并拒绝旧Plan。
