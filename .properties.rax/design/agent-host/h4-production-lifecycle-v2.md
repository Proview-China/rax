# Agent Host H4 Production Lifecycle V2 设计候选

## 1. 状态与结论

- 状态：用户已于2026-07-18确认设计；P0公共合同实施已授权，尚未实现完成。
- 结论：生产启动必须新增 additive `HostV2` 生命周期，保留 `HostV1` 的字段、JSON、摘要、阶段和行为不变。
- 原因：live `HostV1` 只有 `binding -> constructing -> verifying`。真实 Runtime Activation 必须在零副作用 Control Plane adapters 已构造之后、System Ready 之前发生；Generation-Binding Association 又必须消费已经 current 的 Activation。把这些写动作藏入 Binding、Factory 或 Readiness 都会违反 Owner、Effect 与恢复合同。
- 图形资产：[H4 Production Lifecycle V2](h4-production-lifecycle-v2.drawio)。

`HostV1` 继续承担已经验证的 Definition -> Assembler -> Harness Compile 与 owner-local Host 生命周期；不得把它标记为 production root。

## 2. 冻结时序

```text
HostConfig + AgentDefinition source
  -> shared HostStart Admission claim (V1/V2 conflict domain)
  -> Definition exact current / decode
  -> Agent Assembler exact facts + catalog / resolve
  -> Harness compile + atomic immutable artifacts/current publish
  -> Runtime Binding admission + BindingSet commit
  -> construct Control Plane adapters only (zero external Effect)
  -> Application start Command + Desired State + Outbox
  -> Application Activation coordinator journal
       -> Runtime Admission + Harness ExecutionPort preflight
       -> freeze Activation snapshot + reserve Identity + resolve Budget
       -> Sandbox allocate as reserved_quarantined
       -> Runtime ActivationCommit atomically activates Identity/Instance binding
       -> Sandbox activate
       -> Harness ExecutionPort open + Sandbox/Harness ready inspect
  -> Harness Assembly current reread
  -> Runtime Generation-Binding Association
  -> all required Owner production current S1
  -> component actual-point current inspect
  -> all required Owner production current S2
  -> SystemReady Fact commit
  -> Ready
```

Production stop 顺序固定为：

```text
Application stop Command
  -> Run stop / independent settlement
  -> cleanup explicitly requiring live Execution, if any, in Owner-declared DAG order
  -> Harness close / current inspect
  -> Sandbox fence
  -> each fenced-lease-dependent domain cleanup / residual inspect
  -> Sandbox release / current inspect
  -> Runtime cleanup aggregation
  -> Host reverse-DAG handle cleanup
  -> closed | indeterminate
```

Cleanup 不能由 Host 猜测固定位置。每个 Owner 的 Cleanup Contract 必须声明 `requires_live_execution`、`requires_fenced_sandbox_lease` 或 `sandbox_independent` 三者之一，并形成无环依赖；缺失、冲突或成环一律 Fail Closed。任何 `unknown` 或 residual 都不得被进程退出、Sandbox release 或 Go handle 回收洗成 `closed`。

## 3. HostV2 阶段

```text
accepted
  -> validating
  -> resolving
  -> compiling
  -> binding
  -> constructing_control
  -> activating
  -> associating_generation
  -> verifying
  -> ready
  -> draining
  -> reconciling
  -> closed | indeterminate
```

每一阶段先写 Host orchestration journal，再调用唯一 Owner Port。Host journal 只保存 exact refs 和步骤状态，不复制 Binding、Activation、Run、Verdict、Memory、Checkpoint 或组件领域事实。

## 4. 必需 additive Port Delta

### 4.0 跨版本 HostStart Admission

Owner：Host orchestration，且是 V1/V2 共用的单一事实 Owner。

新增 version-neutral `HostStartClaim`，稳定键为 `HostID + StartID`，内容至少绑定请求的 Host ContractVersion、HostConfig digest、Definition source exact ref、requested operation和created/expiry。所有 production-facing V1/V2 facade 在创建各自 Journal 前都必须调用同一原子 `ClaimOrInspect`；同 exact 内容幂等，不同版本或任一内容漂移返回 Conflict。

现有 `HostV1` contract/JSON/digest保持不变，但未注入共享 Admission 的旧 library constructor继续固定为reference-only，不得注册为production入口。CLI/API只暴露共享Admission之后的唯一 facade。必须覆盖V1先/V2后、V2先/V1后、64并发和lost-reply；禁止用“两个独立Journal互相Inspect”伪装原子仲裁。

### 4.1 Harness Assembly Artifact Current

Owner：Harness Assembly。

H3当前只在内存中调用Compiler并返回四个refs，尚未发布完整artifact；因此不能只新增Reader。新增 Harness-owned `CompileAndPublishAssemblyV2`（或等价的atomic Ensure）必须把完整 Generation、Manifest、Graph、Handoff 与一个共同 current projection作为同一create-once发布闭包。四个历史对象逐一不可变；current只允许单调CAS并绑定全部exact refs、共同Checked/Expires和projection digest。

发布合同必须具备单一visibility barrier：`PublicationID = Derive(InputDigest, GenerationID)`；先写不可读的staged historical objects，再以expected current revision/digest一次CAS提交publication current。只有current commit成功后，Historical Reader才允许通过该PublicationID读取四对象；partial staged rows永不对consumer可见且不能被当作current。相同PublicationID+相同内容幂等，不同内容Conflict；current predecessor不匹配永远Conflict，不能因desired相同返回成功。backend可以使用单事务，也可以使用等价的commit marker，但必须通过每个写点崩溃、lost reply、64并发和ABA测试证明相同外部原子可见性。

Compile或publish回包丢失只按稳定Generation/Input identity Inspect原对象与current；禁止重新编译后接受不同refs。Reader必须分离 Historical exact 与 Current exact，并按完整ref读取；S1/S2漂移、过期和时钟回退全部Fail Closed。H3现有`CompileHarnessV1`保持owner-local兼容，不能直接供HostV2 production使用。

Host 不缓存第二份 Assembly，不从 generic ref 反推缺失字段。

### 4.2 Application Production Start Coordination

Owner：Application。

新增窄 Port 只允许：

```text
StartOrInspectAgentActivationV1(exact request) -> exact activation result
InspectAgentActivationV1(exact request)        -> exact activation result
StopOrInspectAgentV1(exact request)            -> exact termination result
```

Application 拥有 write-ahead coordination Fact；它通过 Runtime 公共 Port 驱动 Command/Outbox、Admission、Activation、Run 与 Settlement，但不直接写 Runtime Store，也不成为 Binding、Activation 或 Run Fact Owner。

激活次序不得由实现自行重排：`Preflight -> Snapshot -> Identity/Budget -> Sandbox Allocate(reserved_quarantined) -> ActivationCommit -> Sandbox Activate -> Execution Open -> independent Ready Inspect`。`ActivationCommit` 之前 Sandbox 不得 active，之后才允许使用已提交的完整 `ExecutionScope` 创建 activate/open EffectIntent。任何一步回包未知均先 Inspect 原 intent/attempt，禁止跨 Commit 盲重派。

Start request 必须绑定 Definition、Resolved Plan、Assembly current、BindingSet、Authority/Policy/Budget/Credential refs、预期 Sandbox/Execution adapter bindings和幂等 ID。Result 必须返回完整 ExecutionScope、Activation current ref、Sandbox lease ref、Execution ready observation ref及共同 TTL。

### 4.3 Runtime Binding Admission

Owner：Runtime Control Plane。

需要一个只暴露 `StartOrInspect/Inspect` 的治理入口，把 exact Resolved Plan、production Component Releases、独立 certification/current facts 转成 Binding Facts 与原子 BindingSet。Host/Application 不得直接串行调用裸 `CreateBinding/CAS/CommitBindingSet` 来伪造统一事务。

Binding request必须绑定固定AttemptID、Definition/Plan/Assembly-current exact refs、Catalog/Resolution Facts current、全required Release/Certification/Deployment-readiness current refs、Authority/Policy适用水位、expected BindingSet identity、request digest与requested not-after。此处的Deployment-readiness只证明Release在目标部署可绑定，不得包含尚未存在的constructed component、Binding、Generation Association或Activation坐标。

所有zero-I/O factory所需的预置资源由Resource Owner先发布sealed `ResourceBindingSet`：每项绑定exact ResourceHandleCurrent ref、Owner、kind、scope、cleanup contract、deployment attestation和共同TTL。Binding request必须携完整ResourceBindingSet ref，BindingFact/BindingSet必须exact引用它；Gateway在Issue/Commit复读，Result返回同一ref。Factory只能从BindingSet引用的ResourceBindingSet与sealed process registry解析句柄，不得接受自由ref。Result其余字段为完整BindingSet ref/revision/digest、各Binding refs、Checked/Expires和result digest。Gateway在Issue与Commit前分别复读全部pre-binding current；TTL取所有输入最小值。

未知结果只按原 Binding attempt Inspect；不得换 ID、换内容或重新 certification。

### 4.4 Component Production Current

Owner：各组件。

Host 只依赖统一的中立投影：

```text
domain + exact release ref + exact constructed component ref
+ binding/generation/activation coordinates
+ checked/expires + projection digest
```

每个Owner adapter只能在Binding、control construction、Activation和Generation Association全部完成后，从自己的production Readiness/Deployment/Current Reader产生该投影。它与4.3的pre-binding Deployment-readiness是两个不同名义类型和阶段，禁止互换。Host不接受调用方自签`Production=true`，不读取fixture/internal/testkit，也不把Factory返回成功当作production current。

自定义组件必须注册 namespaced domain、public release/current adapter、exact factory 与 cleanup/inspect owner；未知 required domain Fail Closed，未知 optional domain 只能保留为 residual，不能进入首版全 6+1 production Ready。

### 4.5 SystemReady Fact

Owner：Host orchestration。

SystemReady Fact只聚合已经由Owner证明的exact current refs。至少必须包含Definition current、Plan current、Assembly current、BindingSet current、Activation current、Generation-Binding Association current、Application start-coordination/result current、Sandbox lease/active current、Harness Execution ready current，以及所有required component production current。创建前对这些事实执行完整S1/actual-point inspect/S2；TTL取全部current与HostStart Claim的最小值，并必须满足显式Supervision Policy给出的`minimum_ready_window`，没有Policy不得使用默认窗口。任一Observation/Claim本身不得替代Owner current。Create回包丢失只按固定Ready ID/摘要Inspect。

Host还拥有独立`SystemReadyCurrent`，它只指向immutable SystemReady Fact并记录state、revision、availability epoch、checked/expires和projection digest。Host supervisor在Policy规定的续检点重跑完整current闭包：仍一致时发布更高revision current；任一依赖expired/revoked/drift/unavailable或时钟回退时，必须先把同一availability epoch单调CAS为fenced，再把Host从`ready`转入`draining`或`indeterminate`，绝不让已过期Fact继续表示当前可用。Current回包未知只Inspect原revision；不得修改历史Fact或ABA回到旧ready ref。

为避免Runtime反向import Host，新增Runtime公共中立`AgentExecutionAvailabilityRef/Projection/CurrentReader`类型，事实语义Owner仍是Host。Host adapter发布并实现Reader；Runtime Command/Run admission、Application发起新Run，以及每个尚未dispatch的外部Effect实际点都必须携exact availability ref+epoch并fresh复读。旧epoch、非ready、过期或Reader不可用全部Fail Closed；已经dispatch的Effect只允许Inspect/Settlement/Cleanup，不因fence盲重派。supervisor fence CAS与新Run/Effect admission共用同一线性化epoch，因此检测和并发dispatch不能穿越失效边界。

### 4.6 Organization 条件硬依赖

Organization是shared engine，不新增AgentDefinition V1 core kind；它通过selected Release dependency DAG进入。全局默认是profile-dependent optional，但resolved Review route一旦选择`human_multi_sign_v2`，必须选用独立Human Multi-Sign Review Release/variant，该variant在`ComponentManifestV2.Dependencies`、`RequiredCapabilities`和Harness `DependencySpecV1`中显式声明Organization `review-eligibility-current`为required。Base automatic Review Release不得暗含或伪造该依赖。

Organization缺Release、非production、current过期/漂移、consumer不可用时，Human Multi-Sign route必须在Binding/SystemReady前Fail Closed，禁止自动降级automatic/bypass。Host/Profile不得在Release图外暗中注入Organization。当前Organization只有owner-local SQLite current backend、没有ComponentRelease；其Release Delta须独立设计确认后实施。

## 5. Factory 边界

- `constructing_control`只允许对已经由process bootstrap打开并发布current的资源句柄进行纯内存adapter/controller装配。constructor禁止DB/file/socket open或write、网络、goroutine/process/service start、credential/environment read、Model/Tool/MCP调用、Sandbox allocate、Memory commit、Cache write和任何远程provider接触。
- process bootstrap是独立部署职责：它可以打开资源，但必须先由对应Resource Owner发布exact `ResourceHandleCurrent`、cleanup contract与deployment attestation；Host factory只能按Binding中已有的exact ref从sealed registry取得预置handle，不得自由发现或新建。Bootstrap失败/unknown由其Owner恢复，不能藏入Agent Start。
- HostV2使用独立typed `ControlAdapterFactoryV2`和`ControlAdapterConformanceV2`。Factory必须声明exact Component/Artifact/Contract/Capability/Binding/Generation与全部ResourceHandle refs，`EffectClass=none`，输出只读/current或治理Port句柄；V1 generic factory不得直接提升为V2 control factory。
- Production certification必须含zero-effect构造的静态import/no-raw-provider扫描、Provider/Environment/Effect调用计数为0、typed-nil/alias/extra factory反例和构造前后Owner current一致性。Conformance只是门禁证据，不把构造结果升级为领域Fact。
- Sandbox/Data Plane worker 的真实创建只能发生在 Application/Runtime governed Activation/Effect 路径。
- Module `FactoryDescriptor` 只是声明，不是可执行 factory；唯一 Composition Root 必须注册 exact executable factory，并验证 ComponentID、Artifact、Contract、Capability、Binding 和 Generation。
- Factory `StartOrInspect` 的 unknown outcome 必须 Inspect；Cleanup 为独立治理链，不因 Go object 被回收而视为完成。

## 6. Typed Cleanup DAG

`CleanupPlanV2`是sealed有向无环图。每个`CleanupNodeV2`至少包含：NodeID、OwnerComponentID、exact CleanupContractRef、ResourceClass（`live_execution | fenced_sandbox_lease | sandbox_independent | host_control_handle`）、RequiredBarrierIDs、InspectPort binding、request schema digest、result schema digest和node digest。固定barrier node包括`harness_close`、`sandbox_fence`、`sandbox_release`、`runtime_cleanup_aggregate`；普通Owner节点通过edge显式声明其位置，Host不按名称猜顺序。

执行时每个node生成稳定`CleanupAttemptV2`，绑定Host/Start、Plan digest、NodeID、AttemptID、request digest、predecessor revision和barrier current refs。先写Intent，再调用Owner cleanup/inspect，最后CAS exact result；回包未知、result CAS丢失和重启都只Inspect原attempt。`sandbox_release`必须证明所有`fenced_sandbox_lease`节点settled或显式residual；`harness_close`之前只允许`live_execution`节点。缺失Inspect、资源类漂移、edge成环或barrier current不一致全部Fail Closed。

## 7. 恢复与硬反例

必须覆盖：

1. Binding commit 回包丢失后只 Inspect 原 BindingSet；
2. Control adapters 构造中途崩溃，按 Host attempt 复读，不重复外部 Effect；
3. Command/Outbox 接纳回包丢失，按 Application Fact 恢复；
4. Sandbox Allocate、Sandbox Activate、Execution Open unknown outcome 只 Inspect；
5. ActivationCommit 成功但回包丢失，禁止新 Instance/epoch 重派；
6. Generation Association 成功但回包丢失，按同 Association ID Inspect；
7. 任一 Owner current 在 S1/S2 间 drift/expire/revoke，SystemReady 零写；
8. Factory descriptor 存在但 executable factory 未注册，Fail Closed；
9. extra/raw provider、fixture、internal/testkit、第二 Store、同 capability alias 全拒；
10. 64 个 HostV2 worker 共享 State Plane，只允许一个 Start/Activation/Ready 事实线性化；
11. stop 中任一 unknown effect 或 cleanup residual，终态必须 `indeterminate` 或 residual，禁止伪 `closed`；
12. V1/V2 同 HostID+StartID 冲突，禁止双写或隐式迁移。
13. Harness四个artifact已编译但publish回包丢失，只恢复同一Generation/current，禁止重编译换ref；
14. Generation Association、Application result、Sandbox active或Execution ready任一在S1/S2漂移/过期，SystemReady零写；
15. 每次Owner调用前Host Journal先持久化`OperationKind + AttemptID + RequestDigest + exact input refs`，回包后另一步CAS写exact result；Journal回包未知只Inspect同successor；
16. cleanup依赖成环、未声明lease需求、release前仍有lease-dependent cleanup或Execution close后出现live-execution cleanup，均不得进入`closed`。
17. pre-binding Deployment-readiness与post-activation ComponentProductionCurrent互换或相互伪装，Binding/SystemReady均零写；
18. Ready依赖在Fact提交后过期/revoke，supervisor必须阻止新Run并单调退出Ready；旧Ready current不得ABA；
19. Assembly staged四对象任一写点崩溃均不可读，current commit前所有consumer必须NotFound/Indeterminate而非partial success；
20. constructor尝试open文件/DB/socket、读credential/environment或启动goroutine/process，Conformance拒绝且Owner调用计数为0。

## 8. Journal、Claim保留与兼容门禁

Host Journal的step key固定为`HostID + StartID + HostContractVersion + Phase + OperationKind + AttemptID`。successor只允许：`intent_recorded -> result_recorded | outcome_unknown`；`outcome_unknown -> result_recorded | reconciliation_required`；`reconciliation_required -> result_recorded | reconciliation_required`。后两种状态都只能Inspect原Owner attempt，禁止重调Start；每次CAS绑定expected revision/digest。owner调用成功但result CAS前崩溃时，恢复必须先Inspect journal，再Inspect原Owner attempt；无法判定时保持/推进reconciliation_required而非伪失败或换ID。Stop DAG每个node独立attempt，不允许批量“全部成功”。

HostStart Claim是永久占用的稳定冲突墓碑：TTL只控制是否仍可继续启动，不释放`HostID+StartID`供新内容或新Host版本复用。相同exact claim在保留期内可恢复；过期后只能Inspect/Stop/Cleanup，重新启动必须使用新StartID。压缩只能把完整Fact变成保留stable key、contract version和content digest的tombstone；不得删除唯一性索引。续期只能在未过期且全current复读一致时由同Owner单调CAS，不能改请求内容。

- `HostConfigV1`、`HostV1`、现有canonical digest和H1-H3公共合同不修改；H3 owner-local编译入口保留。
- `HostV2` 使用独立ContractVersion、Journal和Request/Result，但V1/V2的`HostID+StartID`必须先占用同一个共享HostStart Claim冲突域；不得把V2事实伪装为V1 Ready，也不得依赖两个Journal事后对账。
- production只暴露一个共享Admission后的HostV2 facade；HostV1没有production facade。为兼容/冲突测试提供的V1 governed wrapper同样先占共享Claim，但仍固定reference-only，不能被CLI/API或Factory Registry注册为production入口。
- 当前 6+1 Release 多数仍为 `reference_only` 或 `standalone`；在 durable State Plane、actual-point、cleanup、deployment attestation 和独立 certification 完成前，H4 production Start 必须 Fail Closed。
- 本设计已确认；实现仍须严格按P0-P3门禁推进，未通过前不得声明HostV2或production root可用。
