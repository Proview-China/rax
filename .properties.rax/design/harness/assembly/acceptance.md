# Harness Assembly验收设计

## 1. 设计验收门

进入实现前必须由Harness、Runtime、Application、Agent Assembler、Model Invoker和六组件Owner确认：

1. 公共对象、Owner、公共/私有Port边界；
2. Slot/Phase Catalog、Contribution权限和确定性合并；
3. Agent Assembler输出、Binding V2与CompiledGraph映射；
4. Effect/Review/Fence/ConflictDomain/Run Requirement/Unknown矩阵；
5. Checkpoint、Action Gateway、per-turn refresh的延期或正式接线；
6. Port Delta处置及兼容/迁移策略。

任何一项未冻结，只能实现与其无关的纯合同验证切片，不能宣称公共装配闭合。

## 2. 合同验收用例

| 类别 | 正例 | 必须拒绝的反例 |
|---|---|---|
| 确定性 | 输入换序仍产生相同Manifest/Graph Digest | Digest依赖map顺序、时间、地址或注册先后 |
| Slot | 唯一Owner + 合法N Source/Provider | 缺Owner、多Active Binding、组件自造公共Slot |
| Phase | 只引用Catalog中的Observer/Filter/Gate/Port | 万能Hook、未知Phase、Observer阻断、Filter联网/写Fact |
| DAG | 依赖拓扑稳定且可解释 | 环、未满足required依赖、同层无稳定排序 |
| Schema/版本 | compatible Contract/SchemaRef | 主版本不兼容、摘要漂移、内部类型进入Graph |
| 权威边界 | Candidate/Observation/Receipt保持原等级 | Tool Call变Tool Result、Provider completed变Run终态 |
| Model桥 | RouteID + public execution union | internal/vendor SDK/Raw/Native事件泄露 |
| ContextReference | Route支持则物化 | 必需但不支持仍继续；可选却不记Residual |
| Effect治理 | 完整EffectKind/ConflictDomain/Inspect/Settlement | 缺Permit、缺二次Fence、Unknown盲重试 |
| Run收口 | RunSettlementRequirement Participant Fact齐备后由Runtime CompleteRun | 用Operation/Domain Settlement或Harness terminal/close直接写Outcome |
| currentness | Binding/Authority/Review/Budget过期Fail Closed | 用sealed Graph绕过运行时重验 |
| pre-run Evidence | compile只生成Report | 把Conformance Report冒充权威Evidence |
| 单Call升级 | N=1 Observation经Runtime Evidence与Settlement Owner形成SettledTurnResult，再由Runtime Settlement驱动PendingAction CAS | Observation/Application/Hook直接创建PendingAction |
| Tool Candidate | 只从已提交PendingAction和当前Tool binding创建 | 从Tool Call Observation直接创建或自行补Risk/Effect/Owner |
| V4 Enforcement | Begin后经Delegation/Prepare与实际执行点重验，prepare/execute各消费一份Evidence V3 | 把Begin当最终授权、缺任一phase或用V3 Tool路径冒充V4闭合 |
| Action矩阵 | `run + praxis.tool/execute + praxis.tool/single-call-action-v1`且五维全部required | 缺Run/Session/Turn/Action/Context、借用activation profile或开放枚举 |
| Context Refresh | settled ToolResult + current V4 Inspection +完整Association进入`ContextTurnRefreshPortV1`，经pending Context DomainResult→S2→Context Owner本地原子ApplySettlement/Generation CAS产生new Frame；零Runtime Settlement | Tool Apply后读取旧Frame、未settled Frame或绕过Context Owner直接Continuation |
| Continuation | exact Pending/ToolResult/current V4 Inspection/完整Association与已Apply、S2通过的new Frame闭合后CAS | Receipt、旧Frame、裸Evidence pair、字符串ref或未结算Observation直接续行 |
| 多Call | N>1完整保留Observation/Evidence并NO-GO | 取首项、静默串行/并行、部分升级PendingAction |

## 3. 故障与恢复验收

- Permit前失败：不产生Begin，可修复输入后新尝试；
- Model Turn Begin后请求成功但回包丢失：Session进入`reconciling`，只Inspect原attempt；Tool Action发生在`waiting_action`后，Unknown必须保持`waiting_action`并Inspect原Tool attempt；
- Provider返回重复/冲突Observation：同source coordinate同Digest幂等，换Digest冲突；
- Filter panic、Gate超时、Port断连：按Effect边界区分Failed与Unknown，不破坏Session CAS；
- 必需RunSettlementRequirement Participant Fact缺失：Completion Gate拒绝；
- Sandbox Cleanup未知：保留Residual和Inspect条件，不把Endpoint close当Cleaned；
- Binding/Route/Review/Fence在compile后漂移：Activation或dispatch currentness检查拒绝，生成新Generation或重新Admission；
- 异步Observer超载：有界降级并记录Residual，不能改变主流程决定。
- Evidence append、Settlement、PendingAction CAS、Tool Candidate create或Continuation CAS丢回包：只Inspect exact source/ID/revision/digest；同ID换内容冲突；
- Context Refresh/Frame/Generation/Apply CAS丢回包：只Inspect原Attempt/Frame/Generation/Settlement；不得换ID、重跑Tool或推进Harness Turn；
- 两个Application Coordinator并发消费同一PendingAction：同ID同内容幂等，换ID或内容Fail Closed，只允许一个Session CAS胜出；
- Execute回包未知：保持原attempt为UnknownOutcome，只允许受治理Inspect，不得发起替代attempt。

## 4. 测试层次要求

- 单元：对象校验、规范化、Digest、Slot cardinality、DAG、排序、write-set、Gate合并、Residual；
- 白盒：Compiler阶段状态、错误分类、currentness、CAS、Run Requirement覆盖、私有Port不可见；
- 黑盒：只经Go公共SDK编译/inspect/explain/diff，不导入internal；
- 故障注入：lost reply、timeout、panic、stale fence、expired review/budget、duplicate/conflict、partial cleanup；
- Conformance：Runtime Binding/Operation/ExecutionPort、Model public route union、六组件贡献Schema；
- race：并发Observer、并行Contribution收集、Session/Graph只读访问；
- vet：所有计划新增Go包；
- 集成：Agent Assembler→Assembly→Runtime Binding，以及Model/Context/Action/Checkpoint分阶段接线；
- 系统：纯文本Run、Action Run、Unknown恢复、取消/cleanup、完整Settlement后CompleteRun。

测试详细文件级落点与阶段门见plan。Fake仅用于测试，不构成生产Backend、账号、SLA或认证。

### 4.1 Action Gateway分阶段实现门

#### G6A实现门

下列条件联合确认后，可先创建并隔离测试G6A代码；不必等待G6B：

1. Model Owner未来公开只读Projection exact Reader可按完整`ToolCallCandidateObservationRefV1`复读`ToolCallCandidateObservationProjectionV1`；Harness Adapter在形成Application Request前验证Ref全字段、lineage/digest与`Calls==1`，随后才进入完整Observation Evidence映射和exact Inspect；Reader unavailable、Ref变化或Calls不等于1时Application dispatch为零；
2. Model-turn Settlement Owner的解析、current binding检查及`SettledTurnResultV2`产出责任；
3. committed PendingAction只读投影、Tool ActionCandidate create-once/Inspect；
4. Settlement V4 public实现的中央ordinary/race/vet/conformance已通过，且不回退为V3 Tool路径；
5. Runtime登记唯一Action矩阵`run + praxis.tool/execute + praxis.tool/single-call-action-v1`，Run/Session/Turn/Action/Context全部required；
6. Delegation/Prepare后宿主Gateway与实际执行点Enforcement双重重验，prepare/execute Evidence V3各一份且同Attempt；
7. Run、Tool Action、Context ParentFrame Owner-current Readers与Harness Session/Turn source-coordinate Reader通过Conformance；Runtime G6A router只将exact `Kind/ID/Revision/Digest`无损投影为公共applicability ref。Harness `runtimeadapter`构造时接收只封存稳定Subject、不含观察时间的`CommittedPendingActionApplicabilityBindingV1`；收到公共ref后fresh生成Request CheckedAt，复读底层Reader，再采第二次fresh clock校验`now>=checked && now<expires`、公共expiry不超过底层projection；
8. Tool DomainResultFact→Runtime Settlement V4(ref only)→Tool ApplySettlement→settled ToolResult/current V4 Inspection/public Association Inspect的exact字段与ToolResult V4 current合同冻结；
9. Application Owner发布G6A窄Port/中立DTO；Tool Adapter在`tool-mcp`内实现该Port，允许依赖Application公开contract/ports，但不依赖Application coordinator/kernel/实现；
10. G6A仅允许显式test composition/fixture手工注入Binding与已构造Adapter；Harness Assembly对已注入接口的版本、能力、Owner/Slot、sealed摘要校验冻结且不承担具体wiring；当前不宣称生产composition root存在；Application与Harness Assembly都不import Tool实现；Reader直返公共ref、nominal映射改写字段、unknown ref/Binding缺失、同键换Subject/coordinate、Binding digest或Reader漂移、Binding封存观察时间、clock rollback/TTL crossing、可逆ID、全局mutable registry、Session/Turn type-pun、Application Request预塞公共ref/Binding或从PendingAction/event/compat calls反推Model Projection的反例全部拒绝。

G6A验收必须证明输出严格止于settled ToolResult + current `OperationInspectionSettlementRefV4` + public Association Inspect；任何Context Refresh、Continuation、Turn推进、Capability注册/激活均失败。Context及其他Owner-local实现可独立使用本Owner fixture；真实Provider G6A cross-module fixture必须先通过Runtime V2、Tool Adapter及[HA-X02强类型Route](./controlled-operation-provider-route-v2.md)实现/Conformance门。G6A的unit、whitebox、blackbox、fault、Conformance和race全部PASS后，才可创建G6B test-only跨模块fixture。

#### G6B三层实现与能力启用门

三层门固定为：

1. Context等Owner-local实现可使用本Owner fixture隔离，不要求production root；
2. G6A隔离验收PASS后，G6B test-only跨模块fixture可手工注入Application公共Port与各Owner Adapter，验证完整链；它不是production root，不注册/激活Capability，不生产调用Continuation或推进Turn；
3. 只有G6B完整验收、宿主Owner完成production composition root真实设计/实现/验收并通过接线Conformance后，production Capability、Continuation和Turn推进才可GO。

G6B test-only跨模块fixture前必须满足：

1. G6A隔离验收PASS并有可复查结果；
2. Context `ContextTurnRefreshPortV1`、pending DomainResult、S2、本地原子ApplySettlement/Generation CAS与new Frame current Reader通过联合门禁；不得创建、请求或消费Runtime Settlement；
3. Application Owner发布G6B Context/Continuation窄Port与中立DTO；Context Adapter在`context-engine`内、Continuation Adapter在`harness`内实现这些Port；
4. 各Owner Adapter只依赖Application公开contract/ports与本Owner公开合同，不依赖Application coordinator/kernel/实现；Application不import Harness/Tool/Context；
5. test-only composition手工注入的公共Port、Owner Adapter与sealed Binding边界冻结；不得把fixture冒充production root。Harness Assembly只校验/组装已注入接口，不承担具体wiring、不import Tool/Context实现、不新建跨域Composition模块；
6. current `OperationInspectionSettlementRefV4`四类typed exact refs及DomainResult通过公开Association Inspect核对完整prepare/execute；
7. settled ToolResult/V4 Inspection进入Context Refresh，且pending Context DomainResult、S2、Context Owner本地原子ApplySettlement/Generation CAS后才交付new Frame exact ref/digest；
8. lost-reply、并发CAS、同ID换内容、Scope/Binding/Owner/Evidence/Frame/Generation漂移及Unknown original-attempt Inspect反例全部覆盖；
9. `N>1`保持Observation/Evidence但NO-GO，且无`action.batch.completed`隐式绕行。

G6B test-only fixture可验证Continuation Adapter合同与CAS，但不得对生产Session调用或推进真实Turn。G6B完整验收及真实production root接线Conformance PASS前，不得注册或激活Action Capability，不得生产调用Continuation，不得推进Turn。P3b、`N>1`与Checkpoint继续NO-GO。两阶段通过都不等于真实Tool Backend、生产Enforcement或SLA认证；详细用例以[Action Gateway V1测试矩阵](./action-gateway-v1-test-matrix.md)为准。

## 5. 性能验收边界

首轮只建立benchmark基线，不预设生产SLA：

- 规范化/编译随Module、Contribution、DAG边数的增长曲线；
- Graph热路径Slot lookup与Phase dispatch的alloc/op和ns/op；
- 大量Observer、Diagnostic、Residual对内存和尾延迟的影响；
- 同一输入重复编译的Digest一致性。

只有profile定位到热点并经用户确认目标后才能优化；Conformance、race、vet或benchmark通过都不能被表述为生产可用性证明。
