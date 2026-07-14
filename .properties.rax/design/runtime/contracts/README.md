# Runtime端口、Capability与Harness Conformance

## 1. 端口族

| Port | 典型所有者 | Runtime只依赖的语义 |
|---|---|---|
| `ExecutionPort` | Harness、Model Invoker | Describe、Preflight、Open、Observe、Control、Close |
| `ContextPort` | Context Engine | ContextPackage、CachePlan、摘要、来源与失败 |
| `EffectPort` | Model/Tool/MCP/Network/Provider | Intent、Authorization、Fence、Receipt、Inspect |
| `StatePort` | Memory、Knowledge、Asset | Candidate、Review引用、Commit Intent与Receipt |
| `GovernancePort` | Organization、Review、Management | Authority、Verdict、ControlIntent |
| `EnvironmentPort` | Sandbox | Lease、隔离、Attach、Inspect、Fence、Release |
| `BudgetAuthorityPort` | 绑定的预算事实所有者 | Reserve、Commit、Release、Reconcile |
| `EvidencePort` | 权威Intent Store、Evidence系统 | write-ahead、Source Observation、Projection、Watch |

Budget是必要能力，不预设为独立模块。Application Facade也不是Runtime内部Port所有者。

## 2. 共同生命周期语义

每种Port按适用范围提供：

```text
Describe
Validate
Prepare
Open / Allocate
Inspect / Observe
Command
Close / Revoke / Release
Evidence
```

必须区分声明能力、探测能力、认证能力、本次绑定能力和已撤销能力。外部调用成功不等于状态成功，关键状态必须Inspect。

## 3. Mount Descriptor

至少包含：

- component ID/kind/version/digest与contract version；
- Required/Optional和允许Residual；
- Capability、Conformance和证据TTL；
- endpoint/locality/deployment mode；
- Authority、Fence、tenant和Secret类型；
- DAG依赖、Ready独立验证、超时和取消；
- 重试、Effect、Compensation和释放所有者；
- 输入输出Schema摘要与供应链证据。

## 4. Harness共用合同

每条Harness Route必须等价提供：Bootstrap、Run Controller、Interaction Loop、Run内Session、Model/Tool协调、Context Ingress、Result Egress、Event、Control、Error/Backpressure、Health Observation和Cleanup Observation。官方Harness可以内部实现这些能力，但Adapter必须忠实报告不可控和不可观察部分。

## 5. HarnessConformance（Harness准入等级）

| 等级 | 最低保证 |
|---|---|
| `fully_controlled` | 全部持久Effect受权且可Fence；Manifest、取消、结构化Observation和独立终态验证完整 |
| `restricted_controlled` | Effect仍受权且可Fence，但缺少Pause、Checkpoint或完整事件之一，只暴露实际命令 |
| `contained_observe_only` | 内部动作不可完全截获；无持久凭据、开放网络和宿主写权，只允许审核后的产物导出 |
| `rejected` | 存在不可Fence持久Effect、隐藏凭据、不可控网络或身份/版本不可确认 |

Pause和Checkpoint不是所有Harness的强制能力；一旦声明支持，就必须满足对应合同。不能把`contained_observe_only`宣传为受治理Agent。

## 6. Ready与事实可信度

Harness自报Ready/Cleaned/EffectFailed都是Observation。Runtime必须结合Sandbox Inspect、Binding验证、Effect Receipt和领域事实决定权威状态。来源冲突时产生`EvidenceConflict`；安全相关冲突进入Fenced/Indeterminate，Optional能力才可按Plan降级。

## 7. 失败与反例

- `CONTRACT-01`：Provider自报Capability但认证证据过期，不能作为bound能力；
- `HARNESS-01`：官方CLI内置不可拦截网络和长期凭据，Conformance必须为rejected；
- `HARNESS-02`：Harness无Pause但全部Effect可Fence，可为restricted controlled，API不得暴露Pause；
- `HARNESS-03`：Harness签名Cleaned但远程Batch仍运行，不能形成Cleanup Complete；
- `CONTRACT-02`：未知扩展命名空间不能由Runtime猜测或跨Provider透传。
