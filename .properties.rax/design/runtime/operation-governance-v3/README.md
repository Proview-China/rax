# Runtime Operation与Execution治理V3设计

## 1. 定位

Operation V3把Run内与Run外外部动作统一成受治理、可恢复的执行协议，同时不放宽既有Run Effect V2。`OperationSubjectV3`封闭支持activation attempt、run、termination attempt、admin，并为严格namespaced custom kind保留独立CustomOperationID；任何一种subject都必须有唯一、可摘要的稳定身份。

Runtime只治理Intent、Permit、Fence、Delegation、Enforcement、Observation、Settlement与Evidence引用，不解释Context、Model Turn、Tool、Memory、Review或其他领域payload。

## 2. 唯一公共时序

1. Intent Owner经`OperationEffectAdmissionPortV3`提交不可变Intent；Application重启可以先Inspect accepted receipt。
2. Application调用`OperationGovernancePortV3.IssueOperationDispatchV3`和`BeginOperationDispatchV3`。Gateway两次复读Identity、Binding、Authority、Review、Budget、Policy、Credential和CurrentScope，并返回含完整Permit与Fence的公共授权Envelope。
3. `ExecutionDelegationGovernancePortV2`声明Host relay→Data Provider delegation。Host只关联和转发，不替Provider出具Enforcement。
4. Provider按预分配attempt执行Prepare；实际执行点再次读取current governance与dispatch projection。Runtime先持久Enforcement，再把Delegation提交为prepared。
5. Application把exact prepared refs交给Harness-owned状态Port；Provider随后ExecutePrepared。Prepare或Execute回包丢失只Inspect同一attempt，Inspect不得再次Execute。
6. `OperationObservationGovernancePortV3`只接收与prepared attempt、source sequence和Evidence Ledger Record逐字段一致的Observation。
7. Settlement Owner通过`OperationSettlementGovernancePortV3`提交封闭Disposition与可选Schema+Digest领域结果。Observation本身不能产生领域Commit、Harness terminal或Runtime Outcome。
8. Begin后无法证明是否触达Provider时，只能`MarkOperationDispatchUnknownV3`并独立治理Inspect Operation；不能转成pre-dispatch rejected或盲目重派。

首次Prepare的恢复顺序固定为：先按exact Operation、Delegation与Permit调用`InspectPreparedExecutionV2`；合法declared且尚无Preparation时返回`ErrorNotFound`，Application才可执行Prepare并提交Commit。错误Operation、Permit或identity始终返回`PreconditionFailed`，不能被误当成“尚未准备”而继续触达Provider。

所有Application-facing mutation与Inspect都先验证Operation subject、Effect/Permit/Attempt identity、revision、Fence和完整请求关系，再触达Fact/Evidence/backend。`MarkOperationDispatchUnknownV3`额外要求EffectID、ExpectedEffectRevision与Permit attempt的Operation digest/EffectID exact一致；非法Envelope不会产生任何后端读取。

## 3. 双Binding与Delegation

宿主ExecutionPort Adapter与Data-plane Provider分别拥有Binding ref和Capability。二者可以属于同一ComponentID，但Host capability不能冒充Provider capability；隔离由exact Binding、Capability、Effect、Permit、Attempt和Delegation证明，不靠组件名不同。

Delegation使用有界、无环hop chain并绑定冻结BindingSet水位、Operation、payload、endpoint/runtime session、Permit与TTL。Model Provider每个Turn仍必须拥有独立Effect/Permit/Attempt，不能复用Harness Provider Permit。

## 4. 精确引用与恢复

`GovernedExecutionAttemptRefsV2`是Harness可嵌入的公共不可变水位，只含`runtime/ports`类型。Admission、Permit、Delegation、Prepared、Enforcement、Observation与Settlement逐级交叉绑定；Settlement引用Schema+ContentDigest，并将unknown Inspect provenance压平成单层专用Ref，拒绝递归、循环或深链。

Operation、Fence与Enforcement中的ExecutionScope一律按canonical值语义比较；`SandboxLeaseRef`深拷贝产生不同Go指针不会造成漂移，但lease ID或epoch变化必然使Permit交叉绑定失败。

Provider Observation绑定source registration/epoch/sequence和Evidence Record，同序换内容Conflict。Provider Observation只进入waiting settlement；Harness只有在读取exact Operation Settlement后才能解释Domain Result并继续action/input/terminal。

## 5. Application边界

Application只能依赖`runtime/core`和`runtime/ports`。Raw Effect、Permit、Delegation、Enforcement、Observation、Settlement Fact Store属于Runtime Fact Owner实现细节。`conformance.CheckApplicationRuntimeImportsV3`拒绝Application导入`runtime/control`、`kernel`、`foundation`或`fakes`。

Command/Desired State/Outbox的稳定入口为`ApplicationCommandFactPortV2`。旧`control`同名类型只是deprecated/restricted兼容别名；Outbox dispatched只表示已安全移交持久Application Operation Journal，不表示动作已执行或完成。

## 6. 当前限制

- 当前只提供公共合同、Gateway、确定性内存fake、故障注入与Conformance，不选择生产Backend、RPC、签名、Scheduler、进程拓扑或SLA；
- 跨Fact Owner复读不是隐含原子事务，exact revision/digest/TTL会被固化并在每个门禁重新验证；
- Runtime公共入口已可供Application与Harness接线，但Application Step Journal和最终组合闭环仍在独立任务完成；
- 未完成Runtime/Application/Harness组合门禁前，不据此解锁6+1领域实现。
- 默认Provider Conformance不会并发调用真实Effect。64并发create-once只在显式isolated/fixture-only profile执行，并分别报告method entries、distinct attempt observations与observed logical effects；这不等于物理exactly-once，且不会产生production资格。
