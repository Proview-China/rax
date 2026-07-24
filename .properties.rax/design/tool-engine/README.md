# Tool Engine 可实施设计

## 1. 状态与业务输入

- 设计域：`tool-engine`；与`mcp-gateway`分离设计、共同组成`tool-mcp`组件线。
- 最高业务输入：`tmp.document/Tool&MCP.md`。
- 当前阶段：既有Tool G6A V2 Owner-local隔离实现第三轮独立审计最终YES保持不变；[ToolSurfaceManifestCurrent V1](tool-surface-manifest-current-v1.md) P4-0、[SurfaceInvocationBinding V1](surface-invocation-binding-v1.md) P4-1与PD-TM-04 P4-2均已通过software test。MCP侧Runtime V3 actual-point与official SDK `tools/call` owner-local闭环也已通过；正式Evidence/Observation、Harness M2、G6B、system/production继续NO-GO。
- 实现语言：Go。当前没有经基准证明的计算稠密热点，不规划Rust、FFI或独立Rust进程。
- 体系图：[architecture.drawio](architecture.drawio)。对象、状态机、Single Call Tool侧Port和调用链见[contracts.md](contracts.md)，Application/Runtime/Harness Owner映射见[integration.md](integration.md)，验收见[acceptance.md](acceptance.md)，公共缺口见[port-delta.md](port-delta.md)，PD-TM-04 live字段修正见[第七设计修正候选](pd-tm-04-seventh-candidate.md)，M2 C slice见[ToolSurfaceManifestCurrent V1](tool-surface-manifest-current-v1.md)，现有生态供应链组装见[Tool Package Offline Verification与Admission V1](package-offline-verification-v1.md)，MCP语义进入Registry的唯一接法见[MCP Tool Mapping Manifest V1](mcp-tool-mapping-manifest-v1.md)，开发者只读对象面见[Registry Catalog Exact Inspect V1](catalog-exact-inspect-v1.md)，Model Route实际工具兼容缺口见[PD-TM-05联合候选](model-route-tool-compatibility-v1.md)。

## 2. 目标

Tool Engine把本地函数、远程Tool、MCP Tool、Hosted Tool、App Server或WASM载体归一为稳定的Semantic Capability，使模型看到的表达、实际执行机制和治理事实彼此可追溯但不混同。

它必须保证：

1. 能力先注册、验证和版本冻结，再进入Agent Plan与Tool Surface；
2. Model-visible、allowed和pre-approved三个集合分离；
3. 每个调用先形成不可变Action Candidate，再经Application与Runtime现有Governed Operation链；
4. Provider回包只形成Observation/Receipt，不能直接成为领域Settlement或Runtime Outcome；
5. Unknown Outcome只允许Inspect原Attempt，禁止盲目创建新Attempt；
6. SDK、CLI和API与Harness调用走同一治理链，不提供直连Provider后门。
7. 当前Action Gateway只支持`N == 1`与`praxis.tool/execute`；PendingAction摘要必须exact，Candidate/Reservation不授执行权，`N > 1`、batch与custom effect保持NO-GO。
8. prepare与execute分别使用Dispatch V4.0/Enforcement 4.1和独立Evidence V3 Qualification/Handoff/Consumption；Provider事实只是Observation，Tool Owner独立Inspect后才可形成typed DomainResult。
9. Runtime Settlement V4/current Inspection及Application-facing只读Association Inspect证明prepare/execute完整闭合后，Tool才可ApplySettlement；G6A权威输出严格止于settled `ToolResultV2 + OperationInspectionSettlementRefV4`，不调用Context/Harness且不启用能力。G6B才把它交给`ContextTurnRefreshPortV1`；pending Context DomainResult完成S2后，Context Owner本地原子提交ApplySettlement+Generation current CAS并产出new exact Frame，Application才可调用Harness continuation Port。Context链不创建Runtime Settlement。
10. Tool自有Outcome只允许`succeeded|failed`，Disposition只允许`confirmed_applied|confirmed_not_applied`；仅三种合法组合，Unknown/indeterminate只能Inspect/Reconcile。
11. Reservation只绑定中立`ApplicationAttemptRefV1`、Intent/Subject/Session，不绑定Runtime Attempt；DomainResult才首次绑定`OperationDispatchAttemptRefV3`并证明因果来源。
12. DomainResult只接受Runtime公开Provider Observation、两phase Enforcement与两Consumption强类型ref；current projection由Owner复读后签发最长30秒短租约。30秒不是生产SLA，late truth不因Reservation已过期而失真。
13. `SingleCallToolActionPortV1.Execute`在Tool侧是同canonical command的start-or-inspect：Model exact Reader门通过后才create/Inspect Tool Owner Watermark；Provider边界前仅在权威NotFound/current exact后继续，Provider边界后只Inspect原Attempt/Observation。合同不承诺transport exactly-once。
14. Watermark前必须经Model Owner已公开只读Reader exact复读完整`ToolCallCandidateObservationProjectionV1`，验证Ref全字段、Observation digest和`Calls == 1`；Reader unavailable/drift/基数错误时零Tool写、零Gateway/Provider。
15. Runtime受控Provider seam必须调用`InspectCurrentOperationProviderBoundaryV1(ctx, OperationProviderBoundaryRefV1)`复读Tool boundary current proof；Projection只证明CAS/current，不授Authority且不替代Enforcement/Handoff。

## 3. Owner与非Owner

### 3.1 Tool Engine拥有

- `CapabilityDescriptor`、`ToolDescriptor`、`ToolPackageManifest`的语义、版本和兼容规则；
- Tool Registry中Descriptor、Package、Alias解析、弃用和撤回的领域事实；
- `ToolSurfaceManifest`的稳定排序、名称、Schema、Dialect、Guidance和Expected/Actual关联；
- `ActionCandidate`、Tool领域Reservation、`ToolExecutionReceipt`、`ToolResult`和Residual的领域状态；
- Tool级Schema校验、并发/超时/取消/幂等声明、输出规范化、大小与背压规则；
- Provider Observation的独立Inspect语义，以及Runtime Settlement完成后的领域CAS投影；
- Tool过程事件的分类、脱敏、source sequence和领域读模型。

### 3.2 Tool Engine不拥有

- Runtime的Operation Subject、Effect Intent、Admission、Dispatch Permit、Fence、ExecutionOutcome或Settlement Fact；
- Review Case、Verdict、Condition Satisfaction及Human/Auto/Not Required决策；
- Identity、Authority、Budget、Credential Lease、Sandbox Lease或Scope的签发与续租；
- Harness Run Loop、Session、PendingAction或Completion Claim；
- Application Workflow、Outbox、跨域编排或Run终态；
- MCP连接、传输、协议Session和Server能力发现；这些归`mcp-gateway`；
- Context排序、Prompt Cache、Memory/Knowledge正式提交和Sandbox强制执行。

## 4. 运行分区

| 区域 | Tool Engine职责 | 禁止事项 |
|---|---|---|
| Host Control Plane | Registry查询、Surface编译、Application Domain Adapter、只读过程视图 | 不自行签发Permit，不替Review作Verdict |
| Instance Data Plane | 受控Tool Runner、Runtime Provider Adapter、实际执行点二次校验 | 不以Sandbox本地盘作为唯一权威事实，不缓存长期明文Secret |
| External State Plane | Descriptor、Package、Surface、Action Reservation、Receipt、Result、Residual和过程索引 | 不预选具体数据库/RPC |
| Remote Provider | Tool实现或Hosted Tool | 自报只能是Observation/Receipt；不可Fence的持久Effect不得声明受控执行 |

同一Go module可以包含不同分区的适配器，但部署是否拆进程由后续生产设计决定，当前合同不绑定拓扑。

## 5. 与现有公共基座的唯一接法

### 5.1 Application接法

- Application拥有并发布`SingleCallToolActionPortV1`及窄Request/Result DTO；Tool/MCP的`applicationadapter`可实现该Port，只依赖Application公共`contract/ports`，不得依赖Application实现；
- Tool domain/kernel不依赖Application；Application/Harness不import Tool实现；live当前无production composition root，G6A用test fixture手工注入，未来root由宿主Owner在G6B前闭合；
- Tool/MCP的`applicationadapter`还可实现Application公开的版本化领域Port；现有`OperationDomainStatePortV3`只能表达其live字段，不得包装Settlement V4或Evidence V3；
- 在任何Runtime mutation前，`ReserveOperationIntentV3`必须create-once保存Action、Session、Candidate、Intent和Attempt精确关联；
- `BindPrepared`、`BindObserved`、`MarkUnknown`只吸收其公开合同能够精确表达的不可变引用；N=1 `ApplySettlementV4`走Tool Port并等待Application公开V4关联，不自行推进Application状态；
- 回包不确定时只调用对应Inspect，不重复Reserve或CAS不同内容。
- `applicationadapter`在任何Watermark/Candidate写入前，先通过Model已公开只读Reader exact复读完整Projection；不得从Application DTO、PendingAction payload、event JSON或compat tool calls反推Model事实，也不持有Model publish/write Port；
- Reader验证Ref全字段、Observation digest与`Calls == 1`后才create/Inspect `SingleCallToolActionCoordinationWatermarkV1`；相同Application Request ID/Revision/Digest/Scope只Inspect或继续，Reader或Watermark的`Unavailable`/`Indeterminate`不得伪装成`NotFound`。

### 5.2 Runtime接法

- N=1 Action路径只消费Runtime公开Dispatch V4.0、Enforcement 4.1、OperationScope Evidence V3与Settlement V4；不重造治理链；
- `OperationScopeKind=run`、`EffectKind=praxis.tool/execute`、`PolicyProfile=praxis.tool/single-call-action-v1`，Run/Session/Turn/Action/Context五维全部`required`；
- prepare先持久化对应V4.1 Enforcement，再完成Evidence prepare Issue/current/handoff，才能接触受控Provider Prepare；execute必须exact绑定prepare receipt/prepared attempt，并重新走V4.1 execute Enforcement与独立Evidence execute Issue/current/handoff；
- Evidence Candidate只包含单一Qualification与Observation内容；Consume独立携带Handoff。prepare/execute不得复用Qualification、Handoff、EventID、SourceSequence或ConsumptionID；
- Provider Execute前，Tool Watermark必须CAS绑定同Attempt current execute `OperationDispatchEnforcementPhaseRefV4`与`OperationScopeEvidenceProviderHandoffRefV3`；CAS成功即可能已调用。Consumption不得提前进入boundary；
- Runtime Owner已冻结`OperationProviderBoundaryCurrentReaderV1`；Tool `runtimeadapter`把Watermark ID/Revision/Digest无损映射为`OperationProviderBoundaryRefV1`并实现Reader；Runtime不import Tool；
- Runtime Settlement V4 Submission必须exact绑定typed Tool DomainResult与prepare/execute两项`OperationSettlementEvidenceBindingV4`，且公共Inspect返回同一Settlement的Association/Guard/Projection/DomainResult closure；Runtime只投影settled，不拥有领域Outcome/Disposition；
- legacy `ActionPort/ToolPort/MCPPort`是窄Observation骨架；`GovernedExecutionPortV2`/`GovernedExecutionProviderV2`绑定V3/V2证据，均不能作为本路径的V4.1 Provider seam。缺少公开精确关联时真实Provider调用保持unsupported；
- `InspectPrepared`与`InspectLocalAttempt`只读原Attempt事实；远端Inspect是关联原Effect的独立Operation，lost reply/Unknown不得新建Attempt。
- V2领域切片只消费`ProviderAttemptObservationRefV2`、两项`OperationDispatchEnforcementPhaseRefV4`和两项`OperationScopeEvidenceConsumptionRefV3`；不新增字符串ReceiptRef，不用Opaque JSON冒充typed refs；
- `OperationDispatchAttemptRefV3`只从DomainResult开始绑定，Reservation不得包含或回填它；Apply另需fresh `OperationInspectionSettlementRefV4`与Association closure。

### 5.3 Harness接法

- Harness只产生并持有`PendingActionV2`，不导入Tool/MCP实现包；
- Harness Assembly Adapter先把`PendingActionV2`投影成Application DTO；`Application Port -> Tool Candidate/Reservation -> Runtime V4治理/Evidence -> typed DomainResult -> Settlement V4 -> InspectCurrent/Association closure -> Tool Apply`是G6A，输出后硬停；`ContextTurnRefreshPortV1 -> pending Context DomainResult -> S2 -> Context Owner local atomic ApplySettlement+Generation current CAS -> new exact Frame -> Harness Continuation`是G6B并由Application协调；Context链不创建Runtime Settlement；
- Tool/MCP不能修改Harness Session，也不能把Tool Result直接塞入下一轮Context。

## 6. 内部组成

| 包/边界 | 责任 | 允许依赖 |
|---|---|---|
| `contract` | Tool/MCP公共对象、枚举、校验、摘要和错误 | `runtime/core`、`runtime/ports`公共值 |
| `registry` | Descriptor/Package/Alias/Revocation事实机 | 自身contract与持久化Port |
| `surface` | Capability映射、Dialect编译、稳定排序和Surface Diff | 自身contract；Model Profile只以版本化输入Ref进入 |
| `action` | Action Reservation、Receipt、Result、Residual、Inspect/CAS | 自身contract与事实Port |
| `applicationadapter` | 实现Application-owned领域Port与Single Call窄Port；消费Model Owner公开exact Reader；维护Tool Owner canonical-command Watermark与start-or-inspect恢复 | 自身contract、`application/contract`、`application/ports`、Model公开只读Reader合同、Runtime公共值；禁止Application/Model实现依赖及Model publish/write Port |
| `runtimeadapter` | 实现Runtime-neutral boundary current Reader、SingleCall current projection与公开ref映射 | 自身contract、`runtime/core`、`runtime/ports`；Runtime不import Tool；不得包装legacy V2/V3扩权 |
| `runner` | 未来受控Provider实际执行点、并发/超时/取消/背压 | 仅在公共V4.1执行关联闭合后实现；Sandbox能力以Port注入 |
| `sdk` | 注册、解析、编译、调用、Inspect、Watch的Go API | 只调公开Port |
| `cli` | 人类/自动化入口 | 只调SDK/API，不直连Runner |
| `api` | transport-neutral服务合同 | 不预选HTTP/gRPC/RPC实现 |

当前Owner-local Go `sdk`与只读`api`除Registry/Surface/Catalog首批切片外，已增加Package
Offline Verification专项面：sealed exact Verify、Observation/Fact/current Inspect、verification-aware
强Admission、transport-neutral exact双读与CLI `package verify`。该强写口不暴露generic Transition，
也不执行Fetch/Install/Enable；Provider/Runner、production Backend/root与`call/connect`生产入口仍无。

CLI首批只提供可嵌入Runner的`tool list|inspect`，消费上述SDK/API并在完整成功后一次性输出JSON；没有根命令、进程/网络依赖或Provider命令。`call/cancel/watch/mcp discover/package install`必须等待各自治理Port，当前均Fail Closed。

生产包禁止导入`runtime/kernel`、`runtime/foundation`、`runtime/fakes`、Harness kernel/fakes/internal和其他组件实现包。

## 7. 核心不变量

1. Descriptor、Surface、Candidate、Intent、Permit、Attempt和Result都以版本+摘要绑定；任一漂移产生新Revision或拒绝。
2. Alias只在装配阶段解析；Run内只使用精确Version+Artifact Digest+Capability Snapshot。

Tool Alias V1的stable ID、current+1 CAS、唯一Registry与SDK装配边界见
[Tool Alias装配期解析 V1](tool-alias-v1.md)。V1只做Tool Alias；Package Alias、semver range、
runtime latest与production Assembler/Reconcile不在本切片。
3. Tool Dialect只能改变模型表达，不能改变Effect/Risk/Scope/Review/Budget/Credential/Sandbox要求。

Model Tool内容物化与neutral function-tool组装见
[model-tool-materialization-v1.md](model-tool-materialization-v1.md)：复用Model Invoker
公开`modelinvoker.Tool`及其现有多厂商adapter，不新增厂商DTO或业务Tool。V1 portable
profile已把名称与strict JSON Schema限制为live OpenAI/Anthropic/Gemini表达交集；某条
Route的实际compatibility仍由[PD-TM-05联合候选](model-route-tool-compatibility-v1.md)
交回Model Owner。候选要求Prepared historical/current/Association与actual-point exact复读，
不用neutral Validate、粗粒度Capability或Provider回包冒充production兼容事实；当前状态为
`joint_candidate_no_go`，不是Model Owner已冻结合同。
4. 每个Action拥有独立`ActionID + Revision + AttemptID + IdempotencyKey`；Batch总状态不能覆盖成员状态。
5. 同一Conflict Domain的写Effect串行或CAS；无冲突只读调用才可并行。
6. Progress/Partial只作过程Observation；只有Final Receipt经Inspect、Runtime Settlement和领域CAS后才能形成稳定Tool Result。
7. Cancel是关联原Attempt的独立Effect；Provider确认取消不能证明原Effect未发生。
8. Secret只以Credential Ref/Lease进入实际执行点，禁止进入Schema、Context、日志、Package或MCP内容。
9. Fake只用于测试，不声明生产Backend、兼容性或SLA。
10. 既有ActionCandidateV2六项current仅属兼容历史；P4候选使用Surface Repository Current、Capability/Tool Registry Object Current、ActionCandidateV3、immutable InputContract与BindingV2，Model SourceCandidate只作historical lineage。
11. DomainResult Fact是历史truth；Owner current projection每次签发前复读完整因果链，V1 TTL最大30秒，不能把该上限解释为生产SLA。
12. `succeeded+confirmed_not_applied`非法；Unknown/indeterminate不能Apply或形成ToolResult。
13. 相同Application Request ID/Revision/Digest/Scope只能对应一个CanonicalCommandDigest；Provider边界后所有重投只Inspect原Runtime Attempt/Provider Observation，不得再次调用Provider。
14. Model Projection Reader失败、Ref/Observation摘要漂移或`Calls != 1`时，Watermark/Candidate/Reservation全部零写；Application DTO或PendingAction不能替代Model Owner复读。
15. `provider_boundary_crossed`必须单调绑定同Attempt current execute Enforcement/Handoff；boundary CAS成功即“可能已调用”并转inspect-only。Consumption只在响应后进入DomainResult。
16. boundary Projection的Ref、Operation/Scope、Attempt、execute refs、Stage/窗口任一不exact均零Provider；不得把其他三字段Ref type-pun为Boundary Ref。

## 8. 设计裁决

- v1统一能力面以`CapabilityDescriptor`为语义中心，Tool和MCP只提供不同Mechanism；
- Tool Engine与MCP Gateway保持不同Owner和状态机，通过`CapabilitySnapshot`与映射事实连接；
- Runtime Operation V3是唯一执行治理链，不另造私有Intent/Permit/Gateway；
- Review的`operation_not_required`也是正式Verdict投影，不能用布尔`ReviewRequired=false`代替；
- Go承担全部首版实现。Rust仅在未来同一基准下证明Go热点无法满足目标后另行设计；
- MCP Plus只作为显式扩展层输入，不进入标准兼容声明，也不在v1暗中改变Tool Surface。
- 最新Evidence V3采用Candidate单一Qualification、Consume独立Handoff；旧的“Candidate内嵌Handoff/phase”映射作废。

## 9. 尚需管理线裁决

1. 全局Capability/StepKind命名空间最终分配；本设计使用`praxis.tool/*`与`praxis.mcp/*`作为候选。
2. 团队/市场Package的production Trust Policy、Artifact Store、透明日志freshness和漏洞撤回Owner；
   owner-local离线Verify已实现OCI+官方Sigstore Go Bundle+in-toto组合，但不替Owner作生产信任决策。
3. 首个生产State Plane、Credential Broker、Sandbox Provider、API Transport和SLA；本计划不预选。
4. 是否在v1启用对外MCP Server；若启用，哪些组织Policy允许暴露Tool Surface。
5. MCP Plus扩展何时进入独立评审，不与标准MCP首版绑定。

## 10. Component Release/readiness

[component-release-v1.md](component-release-v1.md)定义Agent Assembler可直接消费的Tool/MCP发布面。当前live能力最多形成`standalone`候选；durable store、Credential、真实Provider transport/current、actual-point部署、MCP lifecycle cleanup、deployment attestation与独立Certification全部闭合前，`production`保持NO-GO。
6. Tool G6A V2 Owner-local发布批次：Application窄Port/DTO、Model exact Reader、Settlement V4只读闭包、Evidence V3矩阵、Runtime public V2 actual-point Gateway与Tool V2 Adapter已闭合，隔离实现/测试第三轮独立审计最终YES（P0/P1/P2=0）；V1持续不得包装升权。该结论不代表系统G6A或production GO。
7. G6B发布批次：Context Refresh settled Frame链、Harness公开入口、Harness Assembly Adapter与未来宿主production composition root；live尚无该root，未闭合前Provider生产能力启用、Continuation与Turn推进均NO-GO，但不阻塞G6A test fixture隔离实现。
8. Tool Owner字段与Runtime公开ref兼容性已在隔离实现中逐字段验证；Outcome/Disposition组合、Reservation/Runtime Attempt分层、强类型Observation/两phase refs及30秒current lease均已实现并通过模块门禁。
9. `SingleCallToolActionCoordinationWatermarkV1`及Application `Execute` start-or-inspect映射已实现；其语义不改变四项领域合同，也不扩大N=1。
10. Model Owner公开exact Reader已由Tool消费门接入；Tool只持有只读能力，不定义其publish/write或实现合同。Reader失败、漂移或`Calls != 1`保持零Tool写、零Gateway。
11. Runtime public V2 actual-point Gateway与`OperationProviderBoundaryCurrentReaderV1`已经落盘，Tool V2 Adapter已实现；Boundary Reader仍只证明CAS current，不授执行权。raw Provider、production Backend/root与Identity/Assembler未闭合前不得启用生产能力。
