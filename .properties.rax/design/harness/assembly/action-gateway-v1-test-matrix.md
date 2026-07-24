# Harness Assembly N=1 Action Gateway V1测试矩阵

## 1. 分阶段前置门

Owner-local测试可在各自合同门通过后创建。真实Provider G6A cross-module fixture还必须等待Runtime V2、Tool Adapter及[HA-X02强类型Route](./controlled-operation-provider-route-v2.md)实现/Conformance PASS，并满足Model exact Reader、Settlement V4、closed Action矩阵、Runtime nominal router + Harness applicability current Reader + sealed Binding、ToolResult V4、Application窄Port/DTO与Harness Assembly接口校验。Harness Adapter必须先按完整`ToolCallCandidateObservationRefV1`复读Projection并验证`Calls==1`；Binding不得包含观察时间；每次Inspect使用fresh clock。G6A环境不得注入Context/Continuation实现，输出止于settled ToolResult + current V4 Inspection + public Association Inspect。当前不宣称生产composition root存在。

Context Owner-local实现可先使用本Owner fixture隔离，不要求production root。G6A的unit、whitebox、blackbox、fault、Conformance与race全部PASS后，才允许创建G6B test-only跨模块fixture；该fixture手工注入Application公共Port与各Owner Adapter，验证Context Refresh/Settlement/Apply/Generation CAS/S2/new Frame和Continuation Adapter合同，但不是production root。G6B完整验收及真实production root接线Conformance PASS前，Capability注册/激活、生产Continuation与Turn推进必须被拒绝。

P3b、`N>1`与Checkpoint保持NO-GO；除N=1 settled-action Refresh外的通用per-turn refresh继续冻结。

## 2. 单元与白盒

| ID | 层级 | 用例 | 预期 |
|---|---|---|---|
| AG1-U01 | unit | Calls恰好为1且完整Observation digest正确 | 允许进入Evidence append候选 |
| AG1-U02 | unit | Calls为0或2以上 | 保存完整Observation；不创建PendingAction |
| AG1-U03 | unit | committed PendingAction projection canonical与TTL | 同输入同digest，TTL不超过30秒且受最短current上界约束 |
| AG1-U04 | unit | Session/Turn/PendingAction任一revision或digest漂移 | Reader Fail Closed |
| AG1-U05 | unit | closed Action matrix五维全部required | 唯一矩阵通过；其他组合拒绝 |
| AG1-U06 | unit | `ActionContinuationBindingV1`canonical | 所有exact refs进入digest，换一字段即漂移 |
| AG1-U07 | unit | Session/Turn source coordinates分别seal | 不同静态类型、canonical domain与digest；完整Session/PendingAction变化即漂移 |
| AG1-U08 | unit | Reader unavailable、projection expired或forged ExecutionScope | Fail Closed，不返回current projection、不Issue Evidence |
| AG1-U09 | unit | source coordinate到`OperationScopeEvidenceApplicabilityFactRefV3`的nominal projection | `Kind/ID/Revision/Digest`逐字段相等；重新seal或改写任一字段拒绝 |
| AG1-U10 | unit/model-input | 已终审YES的Model公共exact Reader按完整Ref复读Projection | Ref全字段、lineage/digest exact且Calls恰好为1才形成Application Request；Application/Harness无Model写口 |
| AG1-U11 | unit/binding | `CommittedPendingActionApplicabilityBindingV1` canonical seal与deep clone | 稳定Subject、Session/Turn coordinates进入digest；CheckedAt/Checked/Expires不进入identity；调用方后改原对象不影响Adapter |
| AG1-U12 | unit/binding-map | 构造器接收重复Binding | 同exact键+同binding digest幂等；同键换Subject/coordinate/digest立即Conflict且无半成品Adapter |
| AG1-U13 | unit/binding-map | 公共ref exact lookup | 四字段全相等命中；unknown ref或任一字段漂移Fail Closed |
| AG1-U14 | unit/projection | lookup后底层Reader返回current projection | fresh生成Request CheckedAt；Reader返回自己的Checked/Expires；二次fresh clock满足`now>=checked && now<expires`，公共Expiry不晚于底层Expiry |
| AG1-U15 | unit/time | Binding构造后延迟并使用递增真实时钟调用 | 不因构造期时间冻结而stale；每次调用获得新的观察租约 |
| AG1-U16 | unit/time | Reader返回后时钟回拨或跨过TTL | Fail Closed，不返回公共current projection |
| AG1-W01 | whitebox | S1/S2间Session CAS变化 | committed PendingAction Reader拒绝 |
| AG1-W02 | whitebox | ApplySettledTurn只接受绑定Model Settlement | Observation或Application payload不能直写PendingAction |
| AG1-W03 | whitebox | Continuation先create Candidate后CAS | 同ID同内容幂等；同ID换内容Conflict |
| AG1-W04 | whitebox | Tool Unknown发生在waiting_action | Session保持waiting_action，不进入model reconciling |
| AG1-W05 | whitebox/G6A | G6A只注入Application G6A Port与Tool Adapter | 输出三元闭包后停止；Context/Continuation/Turn调用均为零 |
| AG1-W06 | whitebox/binding | Binding构造后调用方改写Subject/coordinate或尝试运行期注册 | deep clone保持原sealed内容；Adapter无注册/删除/替换写口 |
| AG1-W06 | whitebox/G6A | G6A验收前注册或激活Action Capability | 拒绝，不产生可运行Slot |
| AG1-W07 | whitebox/G6B | 未提供G6A PASS记录即构造G6B | Fail Closed，不调用Context或Harness Continuation |

## 3. typed exact refs与Conformance

| ID | 层级 | 用例 | 预期 |
|---|---|---|---|
| AG1-C01 | conformance | V4 Inspection持Settlement/Association/Guard/Projection四类typed exact refs及DomainResult/Owner/Effect revision | current Inspection通过 |
| AG1-C02 | conformance | public Inspect完整Association后，prepare或execute缺失/交换/重复 | Runtime/Harness协调均拒绝 |
| AG1-C03 | conformance | 两份Evidence来自不同Attempt/Scope/Effect | 拒绝，不调用Tool ApplySettlement |
| AG1-C04 | conformance | `consumed_observation`、late或expired current projection | 不具Settlement资格 |
| AG1-C05 | conformance | ToolResult的Action/DomainResult/Settlement与current V4 Inspection及完整Association exact | 允许继续；任一漂移拒绝 |
| AG1-C06 | conformance | Context Frame Run/Session/Turn/ScopeDigest/AuthorityDigest/Generation exact | 允许；恢复后Scope/epoch漂移或Reader不可用拒绝 |
| AG1-C07 | conformance | 不经current Inspection与public Association Inspect，直接传prepare/execute pair或字符串Evidence ref | Harness合同拒绝 |
| AG1-C08 | conformance/import | Application只依赖自有contract/ports与Runtime公共类型 | import图中无Harness/Tool/Context |
| AG1-C09 | conformance | settled ToolResult进入`ContextTurnRefreshPortV1`，pending DomainResult→S2→Context Owner本地原子Apply/Generation CAS后返回new Frame | 只有new Frame exact ref/digest可进入Continuation；Runtime Settlement调用为0 |
| AG1-C10 | conformance/import | Tool、Context、Harness各Owner Adapter实现Application公开窄Port | 允许import Application contract/ports；不得import coordinator/kernel/实现 |
| AG1-C11 | conformance/import | Harness Assembly接收已注入Application Port接口 | 只校验版本/能力/Owner-Slot/sealed摘要；不import Tool/Context实现、不实例化Adapter |
| AG1-C12 | conformance/import | G6A显式test composition/fixture手工构造并注入Owner Adapters | Harness Assembly只校验已注入接口；不宣称production root存在；无新增Harness跨域Composition模块或Go package cycle |
| AG1-C13 | conformance/schema | Application中立DTO canonical/exact | 不复制Owner struct；字段漂移拒绝；opaque JSON不能替代typed ref |
| AG1-C14 | conformance/owner | Harness Reader返回Session/Turn distinct source coordinates | 真实可复读；Reader不返回公共ref、不授Evidence资格 |
| AG1-C15 | conformance/runtime | Runtime G6A router将Harness source coordinate nominal projection为公共ref | 四字段exact；不创建新ID/Digest或Runtime Fact；`OperationScopeEvidenceFactPortV3`不新增方法 |
| AG1-C16 | conformance/schema | Application Request携带Session/Turn source-coordinate中立镜像 | 不预塞公共applicability refs；Session/Turn互换或镜像type-pun均拒绝 |
| AG1-C17 | conformance/owner-reader | Harness `runtimeadapter`实现`OperationScopeEvidenceApplicabilityCurrentReaderV3` | 公共ref四字段exact lookup sealed Subject Binding；fresh Request→Reader→fresh verify，验证source/ExecutionScope/S1/S2/短租约并返回公共current projection |
| AG1-C18 | conformance/model-boundary | Harness Adapter只依赖已终审YES的Model公共只读Projection Reader | 不设计Model写口/Store/Repository/Ensure实现；不从PendingAction/event/compat payload复原Projection |
| AG1-C19 | conformance/binding-boundary | Binding仅作为Harness Adapter构造配置 | 不进入Application DTO/Runtime FactPort，不授Fact/Authority/Evidence；不新增Runtime接口 |
| AG1-C20 | conformance/G6B-public-port | Application `ContextTurnRefreshPortV1`接收settled ToolResult、current V4 Inspection与full Association，Source基数`Tool=1 / Memory=0 / Knowledge=0 / Continuity=0` | 正向通过；Memory/Knowledge Reader调用数为0；不得要求Memory/Knowledge非空 |
| AG1-C21 | conformance/G6B-version | Memory/Knowledge source collection与G6B settled-action Refresh的type/version/domain | 两种语义不得共享`ContextTurnRefreshPortV1` nominal；无私有兼容映射 |

## 4. 黑盒与集成

| ID | 层级 | 用例 | 预期 |
|---|---|---|---|
| AG1-B01 | blackbox/V1兼容 | V1 equality链独立运行 | 返回`system_identity_incomplete`；不得声称完整G6A、不得产生next Candidate或推进Turn；只有V2 Identity链联合PASS后才可升级验收 |
| AG1-B02 | blackbox | Provider Receipt先到、DomainResult未提交 | 保持waiting_action，不产生ActionResult/Continuation |
| AG1-B03 | blackbox | V4 Settlement完成、Tool ApplySettlement未完成 | 保持waiting_action，只Inspect Tool Owner |
| AG1-B04 | blackbox | Tool ApplySettlement完成但Context Refresh/Settlement/Apply/S2未完成 | 保持waiting_action，不CAS Continuation |
| AG1-B05 | blackbox/V1兼容 | 未注入V2 Identity/DomainResult/Session atomic Binding链 | 返回`system_identity_incomplete`；即使已有settled ToolResult、current V4 Inspection和public Association Inspect，也不得冒充G6A完整验收 |
| AG1-B06 | blackbox/model-input | Model Reader unavailable、返回changed ref或Calls不等于1 | Application Request不形成，Application dispatch调用为零 |
| AG1-I01 | integration | Model Observation→Evidence V2→Model Settlement→PendingAction | exact lineage不丢失 |
| AG1-I02 | integration | PendingAction→Tool Candidate/Reservation→V4/4.1/Evidence V3 | 五维scope与同一Attempt闭合 |
| AG1-I03 | integration | Tool DomainResult→V4 Settlement→Tool Apply→Context Refresh→Context Settlement/Apply/S2→Continuation | 一个current Inspection + 一次完整Association Inspect + 一个settled new Frame，turn仅增加1 |
| AG1-I04 | system | 纯文本Run不声明Action capability | 不经过Action链且不受影响 |
| AG1-I05 | integration/G6B fixture | G6A PASS后由test-only composition手工注入Tool/Context/Continuation Adapter | Application只见自有Port/DTO；Harness Assembly只见已注入接口；全链无循环依赖；Capability/生产Continuation/真实Turn调用均为0 |
| AG1-I06 | production enablement gate | G6B完整验收后，宿主Owner另行设计、实现、验收production composition root并执行真实接线Conformance | root不是fixture；接线exact且无循环后才可GO Capability、production Continuation和Turn推进 |

## 5. 故障、并发与漂移

| ID | 类别 | 注入点 | 必须行为 |
|---|---|---|---|
| AG1-F01 | lost reply | Evidence V2 append | Inspect同source/record；不得换sequence |
| AG1-F02 | lost reply | Model Settlement V3 | Inspect原Operation/effect；不得重复解释Observation |
| AG1-F03 | lost reply | PendingAction Session CAS | Inspect同Session/revision/PendingAction；同ID换内容Conflict |
| AG1-F04 | lost reply | Tool Candidate/Reservation create | Tool Inspect原ID/digest；不得从Observation新建 |
| AG1-F05 | lost reply | V4 Permit/Begin/Enforcement phase | Inspect原Operation/Effect/Attempt/phase；不得换attempt |
| AG1-F06 | crash window | Evidence V3 consumed、Settlement V4 absent | Inspect两phase与DomainResult，重试同canonical V4；不调用Provider |
| AG1-F07 | lost reply | Settlement V4 commit | Inspect current四类typed refs并public Inspect完整Association；同ID换内容Conflict |
| AG1-F08 | lost reply | Tool ApplySettlement | Tool Inspect exact Action/DomainResult/Settlement |
| AG1-F09 | lost reply | Context Refresh/Frame/Generation/Apply CAS | Inspect原Attempt/Frame/Generation/Settlement；不得换ID或重跑Tool |
| AG1-F10 | lost reply | Harness Continuation CAS | Inspect exact Candidate/Session；不得创建第二Candidate |
| AG1-R01 | race | 两Coordinator消费同PendingAction | 同canonical幂等且只有一个CAS胜者 |
| AG1-R02 | race | V3/V4争用同Effect terminal guard | 最多一个终态；失败方Inspect胜者 |
| AG1-R03 | race | Session/Turn在五维current读取期间变化 | S1/S2或Runtime current Reader拒绝 |
| AG1-R04 | race/binding-map | 多goroutine并发读取同一sealed Binding map | 只读结果一致、无data race、无运行期注册或map写 |
| AG1-D01 | drift | Run/Binding/Generation或恢复后的Execution Scope/epoch变化 | 原Operation Fail Closed，不重封旧ref |
| AG1-D02 | drift | Action/Context/ToolResult owner或digest变化 | 不Issue、不Settle或不Continuation |
| AG1-D03 | drift | Context S2发现Session/ToolResult/ParentFrame/Generation/Binding漂移 | new Frame不可交付，保持waiting_action |
| AG1-D04 | drift/binding | Binding digest、Request、coordinate或底层Reader结果漂移 | 不返回公共current projection，不Issue Evidence |

## 6. NO-GO反例

| ID | 反例 | 裁决 |
|---|---|---|
| AG1-N01 | `Calls>1`取首项 | 拒绝并保留完整Observation/Evidence |
| AG1-N02 | Receipt/Observation直接成为PendingAction或ActionResult | 拒绝 |
| AG1-N03 | Action维度引用PendingAction而非Tool ActionCandidate | 拒绝 |
| AG1-N04 | 缺Context仍使用single-call profile | 拒绝 |
| AG1-N05 | 用V3 Tool Settlement代替V4 | 拒绝 |
| AG1-N06 | Harness私封Evidence bundle或传裸pair | 拒绝；复用V4 current Inspection + public Association Inspect |
| AG1-N07 | Application/Hook写Runtime、Tool或Context事实 | 拒绝 |
| AG1-N08 | P3b万能Hook联网或直调Tool | 拒绝 |
| AG1-N09 | Tool Apply后直接读取旧Frame或未settled new Frame进入Continuation | 拒绝；必须完成Context Apply/Generation CAS与S2 |
| AG1-N10 | Application import Harness/Tool/Context | 拒绝；协调器只能依赖Application Port/DTO |
| AG1-N11 | Owner Adapter依赖Application coordinator/kernel/实现 | 拒绝；只允许依赖Application公开contract/ports |
| AG1-N12 | Harness Assembly import Tool/Context实现、创建跨域Composition模块或承担具体wiring | 拒绝；G6A/G6B test-only fixture只手工注入公共Port，生产wiring属于G6B完整验收后经宿主Owner验收的production root |
| AG1-N13 | 复制Owner类型或用opaque JSON逃避中立DTO | 拒绝；复用Runtime公共typed refs与中立坐标 |
| AG1-N14 | Harness Reader直接返回`OperationScopeEvidenceApplicabilityFactRefV3`，或router重封/改写source四字段 | 拒绝；Reader只seal own source coordinate，router只做无损nominal projection |
| AG1-N15 | 缺`OperationScopeEvidenceApplicabilityCurrentReaderV3`、缺Kind路由或source ref漂移仍Issue | Fail Closed；Evidence Gateway必须先完成Owner-current校验 |
| AG1-N16 | Session coordinate、Turn coordinate或公共ref互相type-pun | 拒绝；distinct类型、Kind、canonical domain和digest必须保持 |
| AG1-N17 | Application Request预塞公共applicability ref | 拒绝；Request只承载distinct Harness source-coordinate中立镜像 |
| AG1-N18 | 从PendingAction payload、event JSON或compat tool calls反推Model Projection | 拒绝；必须按完整Ref经Model公共只读Reader复读 |
| AG1-N19 | 对source ID做可逆编码/解码以恢复稳定Subject | 拒绝；只允许构造期sealed Binding exact lookup |
| AG1-N20 | 使用进程级全局mutable registry或运行期注册/替换Binding | 拒绝；Adapter构造完成后immutable、零写 |
| AG1-N21 | unknown ref、Binding缺失、同键换Subject/coordinate或Binding digest漂移 | Fail Closed；不得回退到按Kind猜测或扫描Store |
| AG1-N22 | Binding digest包含观察时间、重放构造期CheckedAt、二次时钟回拨或跨过Expiry | Fail Closed；不得返回或Issue公共current projection |
| AG1-N22 | Binding进入Application DTO、Runtime FactPort或被解释为Fact/Authority/Evidence | 拒绝；Binding只是Harness Adapter配置 |
| AG1-N23 | 为满足错误的live G6B Port而伪造Memory/Knowledge Envelope，或从两类source DTO反推settled ToolResult/V4 Association | 拒绝；零Context写、零Continuation、零Turn推进 |

## 7. 实现后的命令门

按现有总授权进入对应实现阶段后，必须实际运行并记录结果；本次文档修订未运行这些代码验证，不得在本次回报中声称通过：

```text
go test -count=1 -shuffle=on ./...
go test -count=100 ./...
go test -count=20 -race ./...
go test -run 'ActionGateway|PendingAction|Continuation|SettlementV4' ./...
go test -fuzz <target> -fuzztime <approved-duration>
go vet ./...
```

联合集成与系统测试必须覆盖Runtime、Application、Model Invoker、Harness和Tool Owner的公开Port；Fake只能用于故障注入与Conformance，不证明真实Backend或SLA。
