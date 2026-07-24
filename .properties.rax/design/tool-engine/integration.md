# Tool/MCP公共接线、贡献与冲突检查

## 1. 公共与私有Port边界

| 边界 | 性质 | Tool/MCP使用方式 |
|---|---|---|
| `runtime/core`、`runtime/ports` | 公共合同 | 允许：live Operation V3、Dispatch V4.0、Enforcement 4.1、Evidence V3、Settlement V4与Inspection公开值/Port；不导入实现 |
| `application/contract`、`application/ports` | Application-owned公共协调合同 | 允许：Tool内独立Adapter实现`OperationDomainStatePortV3`及`SingleCallToolActionPortV1`；只依赖公共合同/Port，不依赖Application实现 |
| `harness/contract`中公开Governed对象 | 公共值合同 | Tool不直接依赖；由Harness Assembly Adapter投影成Application-owned窄DTO，Harness不import Tool实现 |
| `harness/ports.ContextPort` | Harness私有装配缝隙 | 禁止依赖或实现 |
| `harness/ports.ModelTurnPort` | Harness私有装配缝隙 | 禁止依赖或实现 |
| `harness/ports.EventCandidatePort`及Journal | Harness私有事实缝隙 | 禁止用于Tool/MCP过程事件 |
| Runtime/Harness kernel、foundation、fakes/internal | 实现内部 | 生产包禁止导入 |
| Model Invoker公开Projection exact Reader | 公共Reader与producer-side atomic Ensure已终审YES | Tool `applicationadapter`只按完整Ref读取`ToolCallCandidateObservationProjectionV1`；不调用Ensure、不持有publish/write，禁止internal/厂商SDK/Raw/Native事件 |
| Runtime `OperationProviderBoundaryCurrentReaderV1`冻结合同 | Runtime-neutral公共只读Port | Tool实现`InspectCurrentOperationProviderBoundaryV1(ctx, exactRef)`；Runtime不import Tool，Projection不授Authority |
| Tool/MCP自身Registry/Action/MCP Fact Port | 组件公共合同 | 由组件Owner定义并实现；不承担其他Owner事实 |
| Store、Clock、Transport、Executor、Credential materializer | 组件私有Port | 只在内部装配；不能作为跨域治理接口 |

## 2. Slot/Phase贡献声明

Tool/MCP只声明语义贡献，不定义公共Slot、Hook或Phase枚举。所有贡献必须引用未来Agent Assembler/CompiledGraph输出的namespaced、版本化`SlotRef`、`PhaseRef`和合并Policy。

| 贡献类型 | Tool/MCP语义贡献 | 限制 |
|---|---|---|
| Observer | Registry/Snapshot/Connection/Action/Progress/Receipt/Residual只读过程投影 | 只观察或输出有界Event Candidate；不写其他Owner Fact |
| Filter | 根据已冻结Surface、Capability Grant和Schema过滤模型可见条目 | 不联网、不刷新Snapshot、不扩大权限、不重排未授权条目 |
| Gate | Capability/Schema/Descriptor当前性Admission | 只返回通过/拒绝及理由；不签发Review、Authority、Budget或Permit |
| Port | Application Domain Adapter、Runtime Relay/Provider Adapter、Registry/Discovery/Action公开Port | 只走版本化请求/响应；不能成为通用Hookface |

明确拒绝：`func(ctx, mutableContext) any`式通用Hook、可任意联网的Filter、可写任意Fact的Observer、组件自定义Phase字符串、通过Hook直接触碰Harness Session或Runtime Kernel。

### 2.1 需要Assembler分配的语义位置

以下只是需求，不是本组件定义的Phase名称：

1. Plan编译时：Surface Compiler读取精确Profile/Registry/Snapshot并产出Surface候选；
2. Context物化前：Surface Filter校验Expected Surface与Capability Grant；
3. Harness进入waiting_action后：Action Admission Gate验证已Settlement PendingAction与Surface Entry；
4. Runtime Provider Prepare/Execute接触前：各自完成V4.1 Enforcement与Evidence V3 current/handoff实际点门禁；
5. Provider过程：Observer暴露Progress/Partial/Residual；
6. Settlement后：Action Result Port向联合Action Bridge提供精确Result Ref。

实际SlotRef/PhaseRef、顺序、重复贡献和冲突合并规则由公共CompiledGraph冻结后回填。

## 3. 依赖DAG

```text
Agent Assembler / CompiledGraph / Binding V2
  -> Profile + Harness Capability Profile
  -> Tool Registry + MCP Capability Snapshot
  -> Tool Surface Manifest
  -> Context Expected/Actual Injection Association
  -> Harness PendingActionV2
  -> Harness Assembly Adapter -> Application-owned SingleCallToolActionRequestV1
  -> Application N=1 Action Gateway Coordinator -> SingleCallToolActionPortV1
       -> Model Owner readonly exact Reader -> Projection Ref/digest/Calls == 1
       -> Tool Owner create/Inspect canonical-command Coordination Watermark
       -> Tool Candidate / Reservation current CAS
       -> Runtime Intent / Admission / Permit / Begin
       -> Runtime Enforcement 4.1 prepare
       -> Evidence V3 prepare current / handoff
       -> controlled Provider Prepare
       -> Evidence V3 prepare candidate / consume
       -> Runtime Enforcement 4.1 execute
       -> Evidence V3 execute current / handoff
       -> Tool Watermark boundary CAS binds same-Attempt current execute Enforcement + Handoff
       -> Runtime seam reads neutral boundary current Projection
       -> controlled Provider Execute or Inspect original Attempt
       -> Evidence V3 execute candidate / consume
       -> Tool typed DomainResult Fact
       -> Runtime Settlement V4
       -> Runtime Application-facing readonly Association Inspect
       -> Tool ApplySettlement / settled ToolResult + Runtime current closure
       == G6A hard stop: zero Context/Harness/capability enable ==
  == G6B starts after G6A acceptance ==
  -> Application calls ContextTurnRefreshPortV1
       -> pending Context DomainResult -> S2
       -> Context Owner local atomic ApplySettlement + Generation current CAS
       -> new exact FrameRef / Digest
  -> Application calls Harness Continuation Port
  -> Harness Owner CAS
```

依赖方向固定为：Tool domain/kernel只依赖自身合同，不依赖Application；仅`applicationadapter`可实现Application发布的窄Port，并只依赖Application公共合同。Application/Harness不import Tool实现。live当前无production composition root；G6A由test fixture手工注入，未来root由宿主Owner在G6B前闭合。Tool不依赖Harness/Context/Application实现，也不负责总装。

`Execute`的跨Owner原子边界固定为：Application只投递中立Request DTO及Model Projection exact Ref；Tool `applicationadapter`先调用Model公开只读Reader，复读完整Projection并验证Ref全字段、Observation digest与`Calls == 1`；通过后Tool Owner才create/Inspect `SingleCallToolActionCoordinationWatermarkV1`，再以各阶段独立CAS推进。Tool不得从DTO、PendingAction payload、event JSON或compat tool calls反推Model事实，不持有Model publish/write Port。Reader unavailable/drift/基数错误时零Watermark/零Gateway。Provider边界前恢复还必须得到Watermark current exact和下一阶段权威NotFound；Provider边界后只Inspect原Attempt及其Observation。

## 4. 当前公共装配硬依赖

| 公共依赖 | Tool/MCP需求 | 当前处理 |
|---|---|---|
| Agent Assembler最终输出 | 冻结Surface、Provider/Domain Adapter Binding与贡献清单 | 仅声明Input/Output，不私建Assembler |
| Assembly SDK/CompiledGraph | 分配Slot/Phase、检查DAG与冲突 | plan设为实现前置Gate |
| Slot/Phase合并规则 | 同位置多Observer/Filter/Gate/Port的顺序、唯一性、失败语义 | 不创建组件枚举或默认顺序 |
| Binding V2映射 | Tool Engine、MCP Gateway、Host Relay、Data Provider、Domain Adapter的精确Capability Binding | 只给候选Manifest需求，等待Owner确认映射 |
| Single Call Action Gateway接线 | G6A从Application DTO exact Projection Ref经Model Reader到settled ToolResult/current Inspection；`Execute`为same-canonical-command start-or-inspect | Model Reader通过后才维护Watermark；G6A test fixture手工注入；未来production root在G6B前闭合 |
| Evidence V3 Action矩阵与Reader路由 | run/tool execute/single-call profile；Run/Session/Turn/Action/Context全required，S1/S2 current reread | Runtime公共矩阵/router及Tool narrow reader消费已闭合；任一current漂移时V2 Gateway零admission，不私建聚合Reader |
| V4.1/V2受控Provider关联 | prepare/execute各自exact Enforcement与Evidence handoff后才能调用对应Provider phase | Runtime public V2 Route/Gateway与Tool Adapter隔离接线已实现；legacy `GovernedExecutionProviderV2`不得包装或作为降级路径 |
| Tool boundary current Reader | Runtime实际点复读已CAS Watermark boundary的Operation/Scope/Attempt/execute refs/Stage/current窗口 | Runtime中立Port已发布、Tool runtimeadapter已实现；Reader/Projection任一失败零Gateway |
| Runtime Settlement V4 | Submission exact绑定typed DomainResult与prepare/execute两项Evidence Binding；current Inspect及Application-facing只读Association Inspect证明Settlement/Association/Guard/Projection/DomainResult closure | public closure已由V2隔离Owner flow接入；Runtime只投影settled、不拥有Disposition，V3不得包装 |
| ContextTurnRefreshPortV1 | G6B消费G6A的settled ToolResult与Runtime V4 current closure；Context Owner完成pending DomainResult→S2→本地原子ApplySettlement+Generation current CAS→new exact Frame | Context链不创建Runtime Settlement；不是G6A写码前置，Tool不导入Context实现、不跳过Refresh |
| Harness continuation公开入口 | G6B中Application仅在Context产生已settled、S2-current的新Frame后传Tool Settlement/Association与Frame exact refs | 不是G6A写码前置；Tool无BuildContinuation操作；Harness Owner独立验证/CAS |
| per-turn refresh | Snapshot/Surface变化与下一Turn Context的安全切换 | 当前Run不无声刷新；等待公共接线 |
| ContextReference物化 | Tool Definition/Result/Resource Artifact进入Context | Route不支持时Fail Closed或显式Residual |
| Checkpoint接线 | Connection/Action过程与Checkpoint/Restore协调 | 只报告Snapshot/Residual需求，不自建公共Barrier/Phase |

Tool领域事实、Application Adapter、Runtime V2 Adapter与Owner start-or-inspect flow已经完成隔离实现和测试；V2 physical executor在自身入口fresh-clock原子校验Provider/Prepared current/Enforcement/Handoff/Boundary/NotAfter后直接执行，V1 compatibility仍在`runtime_attempt_bound`后Fail Closed且不得包装。当前production G6A端到端仍因Identity/Assembler、production root/backend及G6B总装未闭合而保持NO-GO。

## 5. Effect、Review、Fence与Unknown矩阵

| 动作类 | Scope/Run Requirement | Review | Fence/二次门禁 | Unknown恢复 |
|---|---|---|---|---|
| Tool execute/MCP call | Run | 精确Verdict或`operation_not_required`正式投影 | Host Gateway + actual Runner | Begin后只Inspect原Attempt；远端Inspect为独立Effect |
| Tool/MCP cancel | Run，Relation绑定原Effect | 按风险Policy重新判定，不能继承已漂移Verdict | 同上 | 取消回包不证明原Effect未发生；Inspect原Attempt |
| Remote inspect | Run，Relation绑定原Effect | 独立Policy/Review；不得复用过期Permit | 同上 | Inspect本身Unknown时按其自身Attempt处理，不形成递归盲重试 |
| MCP connect/discover/refresh | Admin或Run | 根据网络/披露/费用Policy产生正式投影 | 接触Transport前二次门禁 | 原Connection/Request ID+Epoch Inspect；不自动重连替代 |
| Resource read/Prompt get | Run | 数据披露/Injection风险需正式Policy结果 | Scope、Credential、Network、Size/Fence | 只Inspect原请求；未知内容不进入Context |
| Package fetch/register/revoke | Admin | 供应链/正式提交Review | Network/Registry actual point | 原下载/注册Attempt Inspect；不重复正式提交 |
| Cleanup/drain/close | Run/Termination/Admin | Cleanup Policy与必要Review | 当前Fence/Authority/Scope | Unknown保留Residual和Conflict Domain |

## 6. Settlement、Cleanup与Residual矩阵

| 事实 | Owner | 形成条件 | Cleanup/Residual |
|---|---|---|---|
| Provider Candidate/Observation/Receipt | Provider + Runtime Observation Governance | exact phase/Attempt和Evidence Candidate/Consumption | 只是Observation；不能直接生成DomainResult或Settlement |
| ToolDomainResultFactV2 | Tool/MCP领域Owner | 复读`ProviderAttemptObservationRefV2`、两项`OperationDispatchEnforcementPhaseRefV4`、两项`OperationScopeEvidenceConsumptionRefV3`及Action/Reservation/ApplicationAttempt因果链 | 历史权威Fact；首次绑定Runtime Attempt；Reservation后来过期不抹除truth |
| Runtime OperationSettlementRefV4 | Runtime Settlement Owner | exact typed DomainResult与两项`OperationSettlementEvidenceBindingV4` | 只投影settled；不选择applied/not-applied/failed，不拥有领域Outcome/Disposition |
| OperationInspectionSettlementRefV4 | Runtime Settlement Owner的公共Inspect投影 | exact Settlement、Association、Guard、Projection、DomainResult及current window | 作为Apply/Continuation的V4 public closure；不创造自由Evidence Bundle |
| ToolApplySettlementFactV2/ToolResultV2 | Tool/MCP领域Owner | fresh Inspection/Association后CAS合法Outcome/Disposition组合 | `succeeded+confirmed_applied`、`failed+confirmed_applied`、`failed+confirmed_not_applied`；Unknown/indeterminate禁止Apply；Result关闭全部V4 refs与Apply |
| Cleanup Fact | 对应Tool/MCP Cleanup Owner | 独立Cleanup Operation Settlement后 | Unknown cleanup继续占用冲突域，不标记closed |
| Remote Residual | Tool/MCP领域Owner记录，外部系统实际拥有 | Inspect证据确认或保持unknown | 不能因本地close而清零 |

## 7. Runtime/Harness/Application映射

| Tool/MCP对象 | Harness | Application | Runtime |
|---|---|---|---|
| ActionCandidate | 来源为PendingAction，但不归Harness | Attempt + Domain Reservation | OperationSubject/Intent Admission输入 |
| Coordination Watermark | 不可见、不改变PendingAction | Request/canonical command水位；形成Tool boundary source ref | 保存exact Runtime Attempt、execute Enforcement/Handoff及后续Observation；不拥有Runtime事实 |
| ToolProviderBoundarySourceRefV1 / neutral current Projection | 不可见 | Tool exact source与Adapter实现 | Runtime只消费中立Projection；不import Tool、不获得Authority |
| ActionReservation | 不可见控制权，仅通过Ref关联 | `ApplicationAttemptRefV1`、Intent/Subject/Session坐标；后续用`RuntimeAttemptCausalityV1`证明Runtime Attempt来源 | 不拥有；Reservation不含Runtime Dispatch Attempt |
| Prepare/Execute governed | Session不直接吸收Tool执行 | 只编排phase与引用 | V4.1 Enforcement + 独立Evidence V3 current/handoff/consume |
| Receipt/Observation | 不能直接回注 | 传递原Attempt观察，不形成领域Fact | Provider Observation + Evidence Consumption |
| Unknown | Harness继续waiting_action | Domain unknown水位 | unknown Authorization；只Inspect |
| Tool DomainResult/Runtime Settlement V4/ToolResult | 不能直接形成Continuation；先作为Context Refresh输入 | 编排Context Refresh | Tool effect的Runtime Settlement V4；不选择Run Outcome |
| Context settled Frame | Harness Candidate绑定new exact FrameRef/Digest | pending DomainResult完成S2后，由Context Owner本地原子ApplySettlement+Generation current CAS，再调用Harness Port | Context链无Runtime Settlement；不归Tool Runtime Effect |
| Completion | Tool不产生Harness Completion Claim | Coordinator继续工作流 | Runtime独立Run Settlement/Outcome |

## 8. 协调冲突检查

1. 同Request ID/Revision且Digest、Scope、CanonicalCommandDigest完全一致：只Inspect或从Watermark最后已提交阶段继续；不得重建Candidate/Reservation。
2. 同Request ID/Revision但Digest、Scope或CanonicalCommandDigest变化：Tool Owner冲突，零新写、零Provider。
3. crash-before-first-provider-call：仅在Watermark current exact且下一阶段Owner事实权威NotFound后继续同canonical command；Unavailable、Indeterminate、超时和读模型缺失全部停止。
4. Provider边界后lost reply或Application重投：只Inspect Watermark中的原`OperationDispatchAttemptRefV3`/`ProviderAttemptObservationRefV2`；禁止新Attempt或再次Provider调用。
5. 该边界不承诺transport exactly-once；Application、Tool、Runtime、Provider仍是独立原子域。
6. Model Reader unavailable/indeterminate、Projection Ref任一字段或Observation digest漂移、`Calls != 1`：零Tool Watermark/Candidate/Reservation/Provider；不得以Application DTO、PendingAction或event JSON替代读取。
7. Provider boundary CAS前，execute Enforcement/Handoff必须current、exact且绑定同一Attempt/phase；CAS单调保存两public refs。CAS成功即可能已调用，崩溃inspect-only。
8. prepare/execute Consumption只在Provider响应/Observation后进入DomainResult；boundary预填、换Handoff、换phase或跨Attempt全部冲突。
9. Boundary Reader NotFound/unknown/unavailable/drift/expired、sameID换digest、crossAttempt、其他三字段Ref type-pun：Runtime实际点零Provider；Projection不得替代Enforcement/Handoff。

### 7.1 Single Call Owner边界

| Owner | 拥有 | 在单Call链中明确不拥有 |
|---|---|---|
| Harness | `PendingActionV2`、`RequestDigest`、waiting_action Session、`ContinuationRefV2`与new Frame验证/CAS | ActionCandidate、Reservation、Provider执行、Runtime Settlement、ToolResult、Context Frame生成 |
| Application | 单Call基数检查、跨域编排、Tool Apply后调用Context Refresh、Context S2后调用Harness Port | 不生成或改写Tool/Context领域事实，不签发Permit/Enforcement，不直写Harness Session |
| Runtime | Intent/Admission/Review Authorization关联、Dispatch V4 Permit/Begin、Enforcement 4.1、Evidence V3、Operation Settlement V4 | 不拥有Tool Candidate/Reservation/DomainResult/ToolResult，不替Harness推进Continuation |
| Tool/MCP | PendingAction投影校验、ActionCandidate/Reservation、Action current reader、Provider Observation独立Inspect、typed DomainResult、ApplySettlement、`ToolResultV2 + OperationInspectionSettlementRefV4` Refresh输入 | 不签发执行权，不实现Runtime Evidence router，不调用Context/Harness，不构造Continuation，不写其他Owner事实 |

跨Owner只传版本化不可变Ref/Digest。Harness current reader拥有Run/Session/Turn，Tool current reader拥有Action/Reservation，Context current reader拥有generation/frame/manifest/injection/conformance；Runtime applicability router按Fact kind组合，Observation lease只覆盖一次handoff。Runtime V4.1 Enforcement到受控Provider seam若没有精确公共绑定，Single Call实现必须保持unsupported。

### 7.2 Owner原子边界与恢复点

| 原子边界 | 唯一Owner | lost-reply恢复 |
|---|---|---|
| PendingAction/waiting_action CAS | Harness | exact Session/Pending ref+digest Inspect |
| Candidate/Reservation CAS | Tool | exact ID/revision/digest Inspect；Candidate取全部来源expiry最小值，Reservation不越界；过期重试零写 |
| Intent/Admission/Permit/Begin/Enforcement | Runtime | 对应Operation/Attempt/phase Inspect，不重发不同内容 |
| Evidence Issue/Handoff/Consume | Runtime Evidence | 同Qualification/HandoffID/ConsumptionID与Candidate digest Inspect/幂等重放 |
| Provider Prepare/Execute | 受控Provider实际点 | 只Inspect原provider attempt；不得重新调用 |
| ToolDomainResultFactV2 CAS/current lease | Tool | Fact丢回包按exact key Inspect；current projection每次复读因果链后签发，TTL最大30秒，late truth不因Reservation已过期丢失 |
| Settlement V4 CAS | Runtime | `InspectCurrentOperationSettlementV4`返回exact Settlement/Association/Guard/Projection/DomainResult closure |
| ApplySettlement/ToolResult CAS | Tool | exact Apply/Result ref Inspect |
| Context Refresh/pending DomainResult/S2/local atomic ApplySettlement+Generation current CAS | Context | exact Context attempt/facts Inspect；无new exact Frame不向Harness推进 |
| Continuation CAS | Harness | Application在Context S2后调用公开Port；exact Session/Continuation/new Frame refs Inspect |

## 8. 冲突检查

1. Owner冲突：Manifest的Effect/Settlement/Cleanup Owner与Binding/Intent/Settlement Ref逐一一致；
2. Capability冲突：同一namespaced Capability+Version只能有一个精确Surface Entry；多Provider由Plan显式选择；
3. Slot冲突：等待CompiledGraph合并规则，不以文件顺序或注册顺序决定；
4. Phase冲突：组件不定义Phase；未知PhaseRef直接拒绝装配；
5. Schema冲突：同Version不同Digest拒绝，同Digest不同语义Version拒绝；
6. Effect冲突：Dialect/Provider不能降低Capability Effect/Risk/Scope；
7. Binding冲突：Host Relay、Data Provider、Domain Adapter必须是同BindingSet精确且角色相符的Capability Binding；
8. Attempt冲突：同Action revision只能一个Reservation和获胜Attempt；
9. Epoch冲突：旧Connection/Instance/Lease/Authority Epoch只作迟到Evidence；
10. Result冲突：同Settlement不同DomainResult Digest或同source sequence换内容拒绝。
11. Pending冲突：PendingAction Ref相同但RequestDigest、Payload、Capability或SourceCandidate任一不同，拒绝且零Reservation写入。
12. Cardinality冲突：`N != 1`、数组输入或成员聚合请求直接NO-GO，不拆分、不选首项、不部分执行。
13. Evidence冲突：Candidate内嵌Handoff、prepare/execute复用Qualification/Handoff/phase，或Consumption未绑定同一Candidate digest，全部拒绝。
14. Settlement冲突：V4 Submission未绑定typed DomainResult/两项Evidence Binding，或Inspection的Settlement/Association/Guard/Projection/DomainResult任一不exact/过期时，Tool ApplySettlement零写。
15. Legacy冲突：`ActionPort`/`ToolPort`/`MCPPort`、`GovernedExecutionProviderV2`、Evidence V2或Settlement V3不得包装扩权。
16. Context顺序冲突：Tool构造Continuation、Application跳过Context Refresh、pending Context DomainResult未完成S2、Context Owner未本地原子提交ApplySettlement+Generation current CAS，或没有new exact Frame仍调用Harness，全部零Continuation写。
17. Outcome冲突：`succeeded+confirmed_not_applied`或Unknown/indeterminate进入Apply/Result，全部零写；Runtime不得替Tool选择组合。
18. Attempt冲突：Reservation携带Runtime Attempt，或DomainResult的`OperationDispatchAttemptRefV3`不能证明源自同Reservation/ApplicationAttempt，拒绝。
19. Receipt冲突：字符串/Opaque JSON替代`ProviderAttemptObservationRefV2`、两phase Enforcement或两Consumption，拒绝。
20. Current冲突：Candidate未取全部来源expiry最小值、Reservation越界、DomainResult lease超过30秒或签发前因果复读漂移，拒绝；历史late truth仍保留。
