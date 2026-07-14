# Harness与Runtime ExecutionPort接入

## 1. Adapter职责

Harness Runtime Adapter实现现有`ports.ExecutionPort`：

- `Describe`：返回Harness Descriptor、Conformance、版本、Artifact Digest和证据TTL；
- `Preflight`：验证BootstrapPlan、Requirement Digest、Manifest和有界Probe；
- `Open`：在Activation Commit后以EffectIntent/Fence创建Endpoint；
- `Inspect`：返回Ready、Run状态、事件或Cleanup Observation；
- `Control`：处理StartRun、ProvideInput、ProvideActionResult和Cancel；
- `Close`：取消活跃Run、关闭Endpoint并返回Cleanup Observation。

Adapter翻译合同，不拥有Runtime状态机，也不把Harness Claim提升为权威事实。

## 2. Control Payload

可能触发新Model Turn的Start/Input/ActionResult必须在Payload中携带对应EffectIntent，并由`ExecutionControlRequest.Fence`绑定。Cancel只做单调收紧；若未来取消本身触发Provider Effect，则也必须升级为显式Intent合同。

## 3. Runtime完整闭环

```text
Registry绑定Harness Descriptor
-> Foundation Activation/Preflight/Open/independent Ready
-> Runtime StartRun
-> ExecutionPort.Control(start_run)
-> Harness Interaction Loop事件/Action Gateway
-> Harness Completion Claim
-> Runtime Stop/Close/Fence/Release
-> independent Cleanup evidence
```

当前Foundation的Run Record与Harness Run Session通过同一个Run ID关联，但保持不同事实所有者。

## 4. Application V3持久Domain接线

真实接线不再让Application直接调用Harness kernel。`OperationDomainRouterV3`按冻结的namespaced StepKind、Descriptor和Domain Adapter Binding选择`ModelTurnDomainAdapterV3`，然后依序持久：

```text
Application BindPrepared
-> strict ModelTurnEffectEnvelope decode
-> exact Candidate/Run/scope/provider/Delegation route cross-check
-> Harness Session model_in_flight
-> ModelTurnOperationBinding prepared

BindObserved -> waiting_settlement -> binding observed
MarkUnknown -> reconciling -> binding unknown
ApplySettlement -> action/input/terminal -> binding settled
```

Binding不是Application或Runtime的第二权威Owner。它只证明某个Application attempt已经被Harness唯一Session/Candidate精确吸收；`BasisDigest`必须由Binding内部保存的RuntimeAttempt、Delegation、UnknownAuthorization、Settlement和DomainResult重算。

pre-prepared unknown没有RuntimeAttempt/Delegation，只允许精确Candidate的failed terminal；post-prepared unknown必须携带独立Inspect Effect与Inspect Settlement provenance。任何回包丢失只Inspect Session和Binding，不重派不确定外部动作。

## 5. Claim与Run终态

Harness terminal Event仍是Claim Candidate。Application `RunCoordinatorV3`先把精确Candidate持久为自己的`claim_planned`水位，再调用Runtime Claim Gateway形成Evidence Association；回包丢失按Run和source coordinates Inspect。Association不选择Outcome，也不把Harness成功提升为Runtime事实。

Runtime Settlement Owner独立复读Execution、Effect、Claim Policy与全部Participant后提交terminal Run。清理维度未闭合时，Application水位保持`terminal_cleanup`；只有显式Termination Inspect/Reconcile返回新的权威Envelope后才能进入`termination_closed`。该公共协调合同支持组件开发，但测试Port、测试Assembler和fake State Plane不代表生产部署。

## 6. 不修改既有Adapter

`ExecutionRuntime/model-invoker/execution/harness/**`继续作为既有单次执行Route实证。首切面不移动、不重命名、不修改这些代码；未来通过单独的`ModelTurnPort`或受审Adapter接入。
