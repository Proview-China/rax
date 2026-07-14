# Harness公共对象、Port与所有权

## 1. 对象

```text
HarnessManifest
  -> HarnessBootstrapPlan
  -> HarnessEndpoint（绑定Runtime Instance）
  -> HarnessRunSession（只属于当前Run）
  -> HarnessEventCandidate[]
  -> CompletionClaim / CleanupObservation
```

Harness引用Runtime的Identity、Lineage、Instance、Run和Fence，不创建替代身份。Native thread/session/tool ID只能作为命名空间化引用，不能替代Praxis ID。

## 2. 窄依赖Port

- `ContextPort`：返回已授权、带摘要和证据的Run输入快照；
- `ModelTurnPort`：执行一次已授权Model Turn，不拥有完整Interaction Loop；
- `EventCandidatePort`：按Source epoch/sequence保存Harness候选事件；
- Tool/MCP：首切面不直接执行。Harness只产生`action_requested`候选，等待Runtime/Review/Tool Gateway返回结果；
- Checkpoint：Capability可选，首切面明确`unsupported`，不得伪造快照。

Model Invoker继续拥有Route、协议、Provider和单次调用语义；全局Harness只通过`ModelTurnPort`窄桥接，不导入或复制其Provider实现。

## 3. Effect边界

每次模型外发、Hosted Tool、Provider Session推进、Context外发或正式提交都必须携带已持久化EffectIntent和当前Fence。Harness无权自造Intent、扩大Capability或绕过Review。

纯本地状态迁移和只读内存检查可无Effect；只要调用可能越出本地无副作用边界，就必须在最终dispatch前重新校验Intent/Fence。

## 4. Event与Observation

Harness只分配自己的`source_sequence`，不分配Runtime Ledger sequence。事件至少携带Source组件、Source epoch、Run ID、事件类型、ObservedAt和Payload Digest。

`ready`、`completed`、`cancelled`、`cleaned`、`effect_failed`均是Observation或Claim；Runtime结合Identity Lease、Sandbox、Effect结算和独立Inspect决定权威状态。

## 5. Conformance

- `fully_controlled`：模型外发、Action、取消、输入、事件和清理均可拦截/验证；
- `restricted_controlled`：存在显式Residual，但所有允许的持久Effect仍可治理和Fence；
- `contained_observe_only`：只能在外部隔离与只读观察下运行，不授予一般持久Effect；
- `rejected`：不可拦截网络、长期明文Secret、不可观测高风险工具或无法清理。

首切面只实现前两级fake路线；其余两级只冻结拒绝/降级合同。
