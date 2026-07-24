# Application P0执行协调器计划

## 目标产物

产出一个组件中立、可恢复的Application入口，使Command→Outbox→Workflow Journal→Runtime Governance→Harness/自定义Provider→Inspect/Settlement→Run Completion形成唯一受治理路径。

## 清单

- [x] 不可变Command Payload与Workflow Plan合同；
- [x] namespaced Step DAG、上限、排序、重复与环路校验；
- [x] Submission Fact与Workflow Journal CAS Port；
- [x] 自定义Step Catalog，未知required关闭、optional显式跳过；
- [x] Catalog暂时不可用不误判optional缺失，skipped optional按no-op解除依赖；
- [x] Step Kind精确绑定namespaced Provider Capability及Descriptor revision/digest/TTL，禁止计划换绑和过期路由；
- [x] Facade先持久Submission再接纳Runtime Command；
- [x] Outbox先交付Journal再标记dispatched；
- [x] Submission/Journal/Outbox回包丢失exact Inspect；
- [x] 并发100路交接单Journal线性化与race测试；
- [x] Submission/Journal按完整ExecutionScope分区；
- [x] 有界恢复列表、显式Policy+TTL Worker Claim、过期Fence接管和lost-reply Inspect；
- [x] 面向未来自定义模块的Catalog Conformance testkit及“不可自授Authority”反例；
- [x] Unknown只停留waiting_inspect；无Policy Fact禁止进入不可恢复indeterminate/blocked；
- [x] governed完成必须保留exact write-ahead Effect并拒绝历史洗除；
- [x] 接入Runtime Operation Governance Gateway V3；
- [x] Runtime mutation前接入Domain Reservation write-ahead，exact Inspect恢复、过期关闭和历史Fact可读；
- [x] 接入Runtime Binding Currentness只读桥，短TTL、注册/解析复读和漂移/撤销/过期关闭；
- [x] 接入Execution Delegation与Provider Prepare→Enforcement→ExecutePrepared→Inspect；
- [x] post-prepared unknown以exact持久Enforcement的RecordedRevision证明Permit Fact仅单调`+1`，无来源、错链与跨revision跳变全部拒绝；
- [x] 接入Harness governed Session/Candidate；
- [x] 接入exact Plan Certification Association、Start Confirmation、certified Run Claim V3 Evidence Association、Runtime派生Settlement/CompleteRun与terminal cleanup显式恢复；
- [x] CAS成功/恢复回值Validate、严格revision successor、并发Sibling多阶段恢复及深层值隔离；
- [x] Binding currentness、Governed Completion与Recovery List/Acquire/Release public request集中前置校验，非法输入backendCalls固定为0；
- [x] Operation链的故障断点、unknown不重派、并发CAS、换链反例及两个自定义模块/真实Harness Domain Adapter跨模块测试；
- [x] 全量普通/race/vet/fuzz及最终黑白盒验收。

## 验收红线

- Journal之前不得标记Outbox dispatched；
- Begin之后任何错误不得调用ExecutePrepared第二次；
- Receipt/Observation不得直接完成Step或Run；
- 自定义组件不得通过Descriptor、Catalog或fake自授生产/dispatch/commit资格；
- Application不得导入Runtime foundation/kernel内部实现或任何6+1实现包。
- 新增未知namespaced模块只能通过Descriptor+Domain Adapter+Port接入，不得要求Application增加kind switch。

## Additive后续计划

N=1 G6A不原地扩大本P0合同，使用独立计划：[SingleCallToolActionPortV1实施计划](./single-call-tool-action-v1.md)。该计划只增加Application窄Port、中立DTO与可恢复协调水位；G6B Context/Continuation、生产composition root及Capability启用不属于本P0完成声明。
