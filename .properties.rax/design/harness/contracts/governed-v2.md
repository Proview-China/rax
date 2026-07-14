# Harness Governed V2合同

## 目的

把legacy“Control后立即调用Model”的路径拆成可持久、可审核、可恢复的事实链，使Harness能够接入Runtime Operation Governance，同时不把Runtime/组件权威性移入Sandbox。

## Harness唯一拥有的事实

- `GovernedSessionV2`：Run内Session状态和revision；
- `ModelTurnCandidateV2`：不可变模型调用候选，不是Effect/Permit；
- `PendingActionV2/PendingInputV2`：精确绑定来源Candidate与请求摘要；
- Event Candidate：source-ordered观察，不是Runtime Ledger序号。

## 状态机

```text
creating
-> waiting_model_dispatch
-> model_dispatch_reserved
-> model_in_flight
-> waiting_settlement | reconciling

reconciling
-> waiting_settlement | terminal failed after independent Inspect settlement

waiting_model_dispatch
-> terminal failed only when Runtime proves pre-Prepare not applied

model_dispatch_reserved
-> model_in_flight
| terminal failed only when the exact undispatched settlement proves no Provider dispatch occurred

waiting_settlement
-> waiting_action | waiting_input | terminal

waiting_action | waiting_input
-> fresh waiting_model_dispatch candidate
```

状态只通过`SessionFactPortV2` CAS；一次CAS只推进一个revision。模型Provider出现不确定结果时只能进入`reconciling`并走独立Inspect，不得再次ExecutePrepared。

## 精确绑定

- Endpoint绑定完整Execution Scope、BindingSet revision、Component/Manifest/Artifact/Capability；
- 同一自定义ComponentID可同时提供Harness与Model能力，但exact Capability必须不同；
- Candidate绑定Run、Endpoint、Session expected revision、turn、Context digest、Continuation settlement/evidence和Model Provider Binding；
- Session进入`model_in_flight`必须嵌入Runtime公开的Admission/Permit/Delegation/Prepared/Enforcement exact refs；Provider Observation只能推进到`waiting_settlement`；进入action/input/terminal还必须嵌入同一attempt的Runtime Settlement ref，并让Settlement精确绑定规范化DomainResult schema+digest；
- Action/Input continuation只能消费当前Pending一次，并产生新的Candidate；
- 所有时间由调用者预分配后进入事实摘要，重启重放不能因Clock变化产生新身份。
- `ModelTurnOperationBindingFactV3`是Application attempt到Harness Run/Session/Candidate的create-once/CAS关联；prepared保存完整声明Delegation，unknown保存精确Authorization，observed保存精确Observation，settled要求Application/Runtime/Harness三份Settlement完全一致；每个状态都从内部持久事实重算Application Basis。
- `ModelDispatchReservationRefV2`属于Harness核心、传输无关且不导入Application；Application专用的完整Reservation/Binding类型位于`bridgecontract`。同一Harness Fact Owner用一个原子Commit同时写入Session `model_dispatch_reserved`和Attempt索引，禁止Session与索引任一单独成功。
- 首次Reservation必须在注入Clock下复读Candidate/Intent current，并绑定Session revision、Candidate digest、Intent digest、exact Adapter和最短expiry。同Session+Candidate仅一个Attempt获胜；lost reply按Attempt Inspect。历史Inspect不因自然过期丢失事实，但Application/Runtime dispatch门禁必须拒绝过期Reservation。
- Candidate的Endpoint、Session、Data Provider和Host Relay必须与Delegation逐字段一致；同Provider和payload不能掩盖换Endpoint、换Session或换Relay host。

## Runtime接线路径

```text
Application Step Journal
-> Harness生成Candidate
-> Runtime Operation Effect + Gateway Issue/Begin
-> Execution Delegation
-> Data Provider Prepare并自验current facts
-> Runtime持久Enforcement Receipt
-> ExecutePrepared
-> Provider Observation
-> Harness Session CAS(waiting_settlement)
-> 独立Inspect/Runtime Settlement + schema-bound DomainResult
-> Harness ApplySettledTurn CAS(action | input | terminal)
```

宿主Adapter只relay；Data Provider才可产生Enforcement。Model Turn必须使用自身Effect/Permit/Attempt，不能复用Harness Open或Control Permit。

这些不可变引用只证明Harness Claim的依据链存在；它们不让Harness直接提交Operation Settlement、Run Outcome或领域Commit。

Application入口通过`OperationDomainRouterV3`按StepKind/Descriptor解析。内建Model Turn Adapter只接受配置的精确StepKind；用户未来新增组件必须注册自己的Domain Adapter和状态Owner，禁止复用Harness Model Turn入口取得领域语义。

## 扩展边界

用户自定义Harness/Provider只实现公共Port和namespaced Capability；不能导入Harness kernel内部、Runtime foundation/fakes或其他6+1实现包。Conformance通过仍不产生生产认证、Binding、dispatch或Outcome资格。

`CheckGovernedProviderV2`只验证Provider公共Port可观察到的窄合同：同一精确请求的并发`Prepare`必须收敛到同一create-once内容，`InspectPrepared`必须读回完全相同的事实；`ExecutePrepared`只会被调用一次，后续只能由`InspectLocalAttempt`读回完全相同的结果。它不会测试性地重派Execute，也不会授予Binding、Dispatch、Settlement或Completion资格。仅凭进程内测试不能证明Inspect物理上没有访问远程、生产持久性或SLA，因此报告中的Locality与Production Durability证明固定为false，必须由独立部署认证补齐。

宿主Relay对Unavailable、Indeterminate、context cancelled和deadline exceeded一律按“调用结果可能未知”处理，只允许一次本地Inspect。Provider返回的Permit、payload、attempt、provider binding、delegation任一漂移都会在宿主边界失败关闭。
