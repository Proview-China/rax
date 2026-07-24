# Runtime OperationScope Evidence V3设计

## 1. 状态与目标

- 合同族：`praxis.runtime.operation-scope-evidence/v3`；
- 当前阶段：Runtime二审文件级全YES，六项实现前决策已冻结；**实现代码尚未授权**；
- 目标：让Run创建前及其他非Run Operation在不伪造`RunID`的前提下，按精确`OperationScope`登记、复读并一次性消费来源有序Evidence资格；
- 直接缺口：live Evidence Ledger V2保守要求active `running|stopping` Run，不能承载真实pre-run `allocate`、`activate`或`open`；`backend-discovery`当前只有Sandbox资产候选名，live Sandbox合同尚未实现该Effect；
- 非目标：不修改Evidence V2、Operation V3、Dispatch V4.0、Enforcement 4.1、Run Settlement V2或各领域终态语义。

本设计只定义公共合同、Owner边界和恢复语义，不选择数据库、RPC、队列、进程拓扑、Provider、生产后端或SLA。

## 2. 核心裁决

1. Evidence V3在同一Evidence Owner下新增独立`Operation ledger scope`；不修改V2 Run scope、不伪造Run、不建立第二Ledger Owner。V2/V3可以由同一个生产Evidence Owner实现，但禁止双写和自动升级。
2. Evidence V3只证明某个来源Candidate在某个精确Operation范围内满足摄取资格并已进入Evidence Ledger；它不签发Authority、Review、Budget、Permit或Enforcement，也不证明Provider已执行、领域结果成立或Runtime已Settlement。
3. `Issue → InspectCurrent Qualification+4.1 → Provider → Consume`是唯一主链。`Issue`只原子创建Qualification并保留精确source/event key，绝不推进Ledger cursor、chain或ledger sequence；`Consume`由同一Evidence Owner原子完成Record、chain、cursor、Qualification consumed和消费关联，禁止先调V2 Append再CAS拼接伪原子。
4. 每个真实外部prepare/execute phase分别取得独立资格；prepare资格不能用于execute，旧phase、旧attempt、旧epoch、旧generation或旧context一律fail closed。
5. Run/Session/Turn/Action/Context使用结构化Applicability tagged union，并由`OperationScopeKind + EffectKind + PolicyProfile`矩阵声明`required | forbidden`；不得用空值、默认值、自由副本或虚构Run替代。Applicability Policy Ref只出现于Scope顶层。
6. Provider回包、Sandbox Receipt、Harness Event、Context Observation和组件自报始终只是Candidate/Observation；Evidence Owner摄取后也不自动升级为领域Fact。Provider回包跨越Qualification TTL时，只能在Evidence Policy绑定的bounded ingest window内降级为Observation，禁止成为Claim、Authoritative Fact、Permit或Settlement输入。

## 3. Owner与非Owner

| 对象/动作 | 唯一Owner | 其他参与方 | 明确禁止 |
|---|---|---|---|
| Operation ledger scope、Qualification、source/event key保留、chain/cursor、Ledger Record、Consumption Association | 唯一Evidence Owner | Runtime/Application调用公共Port；可与V2同一生产Owner | 第二Ledger Owner、V2/V3双写、Application/组件自行分配ledger sequence或写Record |
| OperationSubject、OperationScope与current operation projection | Runtime Operation Owner | Evidence Gateway精确复读 | Evidence Owner改写Operation或生成占位Run |
| Authorization V4、Admission V4、Permit V4、Begin与Enforcement 4.1 phase | Runtime Governance Owner | Evidence资格只绑定精确Ref | 把Evidence当Admission、Permit、Begin或Enforcement Receipt |
| Sandbox内部Reservation/Lease/Placement等事实 | Sandbox Owner与Runtime Lease Owner | 由4.1/Sandbox Reader按精确phase ref复读 | 把这些事实复制进V3并形成可漂移自由副本；Evidence Gateway调用Provider |
| Session/Turn/Action/PendingAction current投影 | Harness/Runtime对应Owner | 未来Harness Adapter实现`OperationScopeEvidenceApplicabilityCurrentReaderV3` | Harness证明Runtime OperationScope、写Evidence Fact或伪造TTL/restore epoch |
| Context Frame/Manifest/Generation/Actual Injection current投影 | Context/Harness/Model Invoker对应Owner | 未来窄Context current Reader只读投影；现有Ref不足 | Cache hit、Frame或Actual Injection自升为Authority；V3声称现有Reader已实现 |
| Candidate/Receipt/Inspect与领域DomainResultFact | 对应组件Owner | Evidence只保存既定TrustClass和PayloadDigest | Provider Receipt直接成为DomainResultFact |
| 跨域步骤与恢复推进 | Application Coordinator | 只编排Issue/Inspect/Consume和各Owner调用 | 创作Owner Fact、绕过Gateway或盲重试Provider |

## 4. 公共对象

所有V3公共类型统一使用`OperationScopeEvidence...V3`前缀，不扩大旧V2结构：

- `OperationScopeEvidenceLedgerScopeV3`：Evidence Owner下独立于V2 Run scope的Operation分区与chain scope；
- `OperationScopeEvidenceScopeV3`：精确Operation与受治理引用的canonical封装，不复制上游领域事实；
- `OperationScopeEvidenceApplicabilityPolicyRefV3`：只允许出现在Scope顶层的Applicability Policy精确引用；
- `OperationScopeEvidencePolicyRefV3`：只允许出现在Qualification顶层的Evidence Policy精确引用；
- `OperationScopeEvidenceApplicabilityMatrixKeyV3`：`OperationScopeKind + EffectKind + PolicyProfile`稳定键；
- `OperationScopeEvidenceApplicabilityRefV3`：封闭`required | forbidden` tagged union；Applicability Policy Ref只在Scope顶层出现一次；
- `OperationScopeEvidenceQualificationFactV3`与`OperationScopeEvidenceQualificationRefV3`：issued资格及精确引用；
- `OperationScopeEvidenceSourceReservationV3`：保留source/event key；在Policy维度只绑定Evidence Policy digest，不复制Policy Ref；
- `OperationScopeEvidenceProviderHandoffRefV3`：Evidence Owner在InspectCurrent之后、Provider之前create-once的调用交接；
- `OperationScopeEvidenceCandidateV3`：来源有序Candidate；
- `OperationScopeEvidenceConsumptionFactV3`与`OperationScopeEvidenceRecordRefV3`：Consume结果与Ledger引用；
- `OperationScopeEvidenceFactPortV3`：create-once/CAS/历史Inspect；
- `OperationScopeEvidenceGovernancePortV3`：Issue、InspectCurrent、Consume的受治理入口。
- `OperationScopeEvidenceApplicabilityCurrentReaderV3`：可选、中立、只读current投影Port；不拥有任何领域事实。

合同版本固定`3.0.0`。有领域TTL的治理Ref采用`ID + Revision + Digest + ExpiresUnixNano`值语义；Session/PendingAction等没有领域TTL的对象只保留其真实`ID + Revision + Digest`，由Reader另给观察租约，禁止伪造`ExpiresUnixNano`。Canonical必须禁止`map`，所有集合稳定排序并拒绝重复，nil slice归一为空集合，`required | forbidden`使用显式tagged union；TTL、phase、source key、event key、Policy digest全部进入对应对象digest。结构体必须strict decode、canonical seal、clone-on-write/read，禁止隐式默认和未知required字段。P1只覆盖单条Issue/Inspect/Consume；Watch、Batch、Retention与跨scope查询全部后移。

## 5. 必须进入Scope Digest的精确引用

### 5.1 主体与操作

- Tenant与`OperationSubjectV3` exact ref、Operation ID/kind/revision/digest、Effect ID/revision、Attempt exact ref；
- 独立`OperationScopeEvidenceLedgerScopeV3` ref，不携带V2 Run partition key；
- Scope顶层唯一`ApplicabilityPolicyRef`及`OperationScopeEvidenceApplicabilityMatrixKeyV3`；
- `OperationScopeEvidenceApplicabilityRefV3`中Run、Session、Turn、Action、Context逐项为`required | forbidden` tagged union；required项绑定对应Owner exact ref，forbidden项不携带维度Ref；
- Idempotency/Conflict domain与Causation/Correlation根。

### 5.2 Assembly、Generation与Context

- 精确Generation/Generation-Binding Association Ref；Reader据此复读BindingSet及相关current事实，不把它们复制为V3自由字段；
- Context为required时绑定未来Context Current Projection Ref。现有Context Ref不足以证明Frame、Manifest、Generation、Expected/Actual Injection同一时刻current，必须先落地窄Reader Delta；
- Context为forbidden时该维度union不携带Reader或Context Ref；其合法性由Scope顶层Applicability Policy Ref与矩阵共同证明，不得携带空Context Digest；
- Context/Generation Reader只返回观察租约，不改变源Fact TTL。

### 5.3 Runtime治理与执行面

- 精确Authorization Fact V4、Dispatch Admission V4、Permit V4与Enforcement 4.1 phase ref；
- 精确Generation/Generation-Binding Association；Evidence Policy Ref只在Qualification顶层出现，Applicability Policy Ref只在Scope顶层出现；
- Enforcement phase=`prepare|execute`；execute phase ref必须已经绑定exact prepare ref与`PreparedProviderAttemptRef`，且属于同一Attempt；
- Reservation、Lease、Placement、Authority、Policy、Budget、Capability、Credential与Fence不得作为V3自由副本。Evidence Gateway只通过上述exact refs调用对应current Reader复读，任一漂移即fail closed。

### 5.4 中立Applicability current Reader与后续Adapter

可选`OperationScopeEvidenceApplicabilityCurrentReaderV3`只服务required维度：对应维度为forbidden时Reader可以为nil；任一required维度缺Reader、Reader未由可信Binding/Policy装配或返回漂移时，Issue必须在零写之前fail closed。Reader输出的观察租约不是领域TTL，也不能延长源Fact寿命。

Harness当前只能提供Run内Session/Turn exact-current投影，不能证明Runtime OperationScope，也不能产Evidence、Permit或Enforcement。未来Harness Adapter可实现上述中立Reader：

- 请求：精确RunRef/ExecutionScope、SessionID/revision/phase/turn、可选Candidate/PendingAction、调用方`NotAfter`；
- 算法：S1读Session → 完整校验Candidate/PendingAction → S2复读Session；S1/S2 revision与canonical digest必须exact一致；
- TTL：Session/PendingAction没有领域TTL，禁止伪造。Projection只返回观察租约，等于`min(调用上界, Evidence Policy cap, Candidate/Reservation真实expiry)`；
- Restore：只引用完整ExecutionScope digest，其中覆盖Instance、SandboxLease与Authority epoch；不得自建Harness restore epoch；
- 输出不含Runtime OperationScope结论，只供Evidence Gateway核对required的Run内坐标。首批activation矩阵将Run/Session/Turn/Action全部设为forbidden，因此本Reader不进入首批实现；后续Action Gateway解冻前仍需单独联合冻结。

Context现有Ref不足以证明Frame、Manifest、Generation、Expected/Actual Injection同一时刻current。首批activation矩阵把Context设为forbidden；未来若矩阵将Context改为required，必须先由Context Owner实现同一中立Reader合同的窄Adapter。

### 5.5 来源、Event key与载荷

- Source Registration、Producer identity、Source epoch/sequence；
- `OperationScopeEvidenceSourceReservationV3`保留source/event key、reservation expiry和Evidence Policy digest；它不复制Evidence Policy Ref、Scope、phase或治理事实；
- Issue时保留Event ID与Expected SchemaRef；Provider返回后Candidate才携带实际TrustClass、PayloadDigest与Provenance；
- Causation、Correlation、适用的Domain Fact Ref；
- 同source epoch+sequence同摘要幂等；同序换内容返回EvidenceConflict，不能分配新Event ID绕过。

## 6. 状态机与API语义

```text
不存在
  -> Issue(create-once)
issued
  -> InspectCurrent Qualification + exact 4.1/current Readers
  -> Evidence Owner create-once ProviderHandoff
  -> Provider
  -> ConsumeCurrent(同Owner原子Record + chain + cursor + consumed)
consumed_current

issued且Provider已在current窗口内调用、Qualification current TTL已越过
  -> response跨Qualification TTL但未越过Policy IngestNotAfter
  -> ConsumeObservation(同Owner原子提交，TrustClass固定observation)
consumed_observation

issued -> ingest_only（Qualification TTL已越过但IngestNotAfter未到）
issued | ingest_only -> expired（IngestNotAfter已到） | revoked
```

### 6.1 Issue

`IssueOperationScopeEvidenceV3`必须：

1. 复读Operation、exact Authorization/Admission/Permit/4.1 phase、Generation、Evidence Policy、Applicability及所需Reader投影；
2. 校验预期source registration/policy、source key、event key与Expected SchemaRef；实际PayloadDigest只能在Provider返回后由Consume校验；
3. Scope顶层只封存Applicability Policy Ref；Qualification顶层只封存Evidence Policy Ref，不在其他层重复这两个Ref；
4. 计算Qualification TTL和Policy限定的`IngestNotAfter`并写create-once Qualification；
5. 在Qualification同一原子创建中保留精确source+event key，SourceReservation在Policy维度只绑定Evidence Policy digest；
6. 不调用Provider、不Append任何V2/V3 Record、不生成chain/ledger sequence、不推进cursor、不授予dispatch。

Issue回包丢失后，调用方只能用Qualification ID或source key `Inspect`，不得换ID、sequence、attempt或payload重发。

### 6.2 Inspect

- `InspectOperationScopeEvidenceV3`返回历史Fact，不声称current；
- `InspectCurrentOperationScopeEvidenceV3`必须同时复读Qualification、exact 4.1 phase及所有required Reader，逐字段核对revision/digest/TTL；
- `now >= ExpiresUnixNano`即非current；时钟回拨不能延长已观察过的资格寿命；
- not found、不可访问、漂移、撤销、过期、unknown required extension均返回零current projection并fail closed。

### 6.3 Provider Handoff

Runner只有在InspectCurrent成功后、Provider调用前，才能向Evidence Owner create-once取得`OperationScopeEvidenceProviderHandoffRefV3`。它精确绑定Qualification、4.1 phase、Attempt、`CheckedAt`与`NotAfter`并进入自身digest；同ID换任一字段Conflict。Handoff只证明Runner在该时点完成过current检查并获得一次调用交接，不证明Provider副作用发生、不授予重派，也不替代4.1 Enforcement。回包跨Qualification TTL时，Consume必须以该Handoff证明调用在current窗口内开始。

### 6.4 Consume

`ConsumeOperationScopeEvidenceV3`必须在Evidence Owner同一逻辑提交中：

1. current路径再次执行InspectCurrent；跨TTL路径必须以同Owner `OperationScopeEvidenceProviderHandoffRefV3`证明Runner在current窗口内完成检查并取得调用交接，同时验证Policy bounded ingest window；
2. Candidate的唯一治理绑定是Qualification Ref；Candidate不得重复携带Scope、Policy、phase、Attempt、Handoff或Applicability Ref。Consume通过Qualification与同Owner Handoff反查并精确校验Operation、phase、Attempt、source/event key和PayloadDigest；
3. 在同一Evidence Owner原子提交内写Record、更新chain/cursor、将Qualification置为对应consumed状态并创建不可变Consumption Association；
4. 禁止调用V2 Append后再写V3 CAS，禁止跨Store补偿拼成伪原子；
5. 跨TTL路径Record的TrustClass固定为`observation`，并携带`late_within_ingest_window`分类；它不得关联Claim、Authoritative Fact、Permit、Enforcement资格或Settlement。

同内容重放Inspect并返回原Record；不同内容、不同phase、不同attempt或不同scope冲突。跨Fact Owner的current复读不宣称分布式原子；Evidence Record事务必须由唯一Evidence Owner完成。V2/V3可共用该Owner的生产实现，但不能双写、串联两个Append或产生两条chain。

## 7. TTL、漂移与恢复

- Evidence Policy由独立Evidence Policy Owner持有，Runtime/Evidence Gateway只能消费精确Policy Ref，不能自发策略；
- `MaxOperationScopeEvidenceIngestGraceV3 = ports.MaxDispatchPermitTTL`。live值为30秒，只是安全上限，不是生产SLA；
- `IngestNotAfter = min(Qualification.Expires + GrantedIngestGrace, Qualification.Expires + MaxOperationScopeEvidenceIngestGraceV3, EvidencePolicy.Expires, SourceReservation.Expires)`；
- Qualification TTL等于exact Authorization/Admission/Permit/4.1 phase、Generation、Evidence Policy、Applicability与所有required Reader观察租约的最小值；上游Reservation/Lease/Authority等事实由Reader复读，不复制进V3；
- 任一字段进入Qualification digest，TTL本身也进入digest；同ID但TTL或任一字段不同必须Conflict；
- Begin后、Provider调用前或回包后发生漂移，都不能靠已有Evidence继续执行；执行点必须按4.1及对应Owner合同处理；
- Provider回包未知时，Inspect原Provider Attempt。Evidence资格或正式Record都不产生重派权；
- Consume回包丢失时，先Inspect Qualification、Consumption Association与exact Ledger Record；
- Provider必须在Qualification current且4.1 current时被调用。若回包跨Qualification TTL，只能在Evidence Policy于Issue时封存的bounded ingest window内降级摄取为Observation；越过`IngestNotAfter`直接拒绝。降级Record不能成为Claim、Authoritative Fact、Permit、Enforcement或Settlement输入，也不能复活Qualification currentness。

## 8. 与Action Gateway的严格顺序

### 8.0 首批支持矩阵

首批只注册以下矩阵键：

| OperationScopeKind | EffectKind | PolicyProfile | Generation | Run/Session/Turn/Action/Context |
|---|---|---|---|---|
| `activation_attempt` | `praxis.sandbox/allocate`、`praxis.sandbox/activate`、`praxis.sandbox/open` | `praxis.runtime/activation-evidence` | required | 全部forbidden |
| `activation_attempt` | `praxis.sandbox/inspect` | `praxis.runtime/activation-inspection-evidence` | required | 全部forbidden |

以下边界已经冻结：

- `praxis.sandbox/backend-discovery`作为Sandbox资产候选名与backend discovery语义的命名映射为**YES**，但live Sandbox `EffectKind`、领域Fact与Provider合同尚未实现，因此不进入首批矩阵且保持**NO-GO/零Provider调用**；
- pre-run `rollback`没有已批准的Sandbox EffectKind；`praxis.sandbox/cancel`是Run-scoped执行控制，禁止把rollback映射或降级为cancel；
- `praxis.sandbox/close`与`praxis.sandbox/release`在live Sandbox合同中属于termination scope，与`activation_attempt`冲突，保持NO-GO；未来pre-run recovery必须由用户批准独立Operation kind/profile，不能复用termination动作；
- `praxis.sandbox/inspect`窄行只服务于已治理原Provider Attempt的独立远端Inspect；必须使用新Effect/Attempt/Permit/双Enforcement并exact绑定原Effect/Attempt/ProviderAttempt/request/payload digest。它不能签发原allocate/activate/open动作，不能递归创建Inspect链，也不能从Provider NotFound推导not_applied；
- `run`、`admin`、`custom`、Run termination及Action Gateway全部unsupported；未知矩阵键在Issue零写前fail closed。

后续新增矩阵键必须走新Policy/Profile与联合评审，不能用optional Reader、自定义kind、别名或动作映射绕过。

### 8.1 代码解冻顺序

```text
Runtime Enforcement 4.1 + Sandbox current Reader联合YES
-> OperationScope Evidence V3公共合同/Store/Gateway/Conformance
-> Sandbox/Harness/Context exact-current Reader与Adapter
-> pre-run lifecycle集成
-> 单Call Action Gateway
-> per-turn Context Refresh
-> Checkpoint接线
```

不得用P3b万能Hook、Harness私有Port、Application临时表或组件私有Evidence Gate跨越上述顺序。

### 8.2 单次外部动作顺序

```text
Operation V3 Intent/Reservation
-> Authorization V4 -> Permit -> Begin
-> Enforcement 4.1 prepare phase
-> Evidence V3 Issue -> InspectCurrent -> ProviderHandoff
-> Provider Prepare/Inspect -> Candidate Observation
-> Evidence V3 Consume
-> Enforcement 4.1 execute phase（绑定exact prepare + PreparedProviderAttempt）
-> 新Evidence V3 Issue -> InspectCurrent -> 新ProviderHandoff
-> Provider Execute/Inspect -> Candidate Observation
-> 新Evidence V3 Consume
-> Domain Owner Inspect/CAS
-> Runtime Operation Settlement
-> Domain ApplySettlement
```

Runner实际调用Provider前仍须复读current Permit、Enforcement、Sandbox及Credential。Evidence不替代任何一步。Run内Model `ToolCallCandidateObservationV1`当前仍走Evidence V2；除非另行批准迁移，不自动双写V3。

## 9. 版本与兼容策略

- 新合同版本固定为`3.0.0`，采用独立type discriminator、Port和Operation ledger scope；全部V3公共类型统一`OperationScopeEvidence...V3`前缀；
- Evidence V2保持原样，不新增可选Run、不放宽active Run约束、不把V2 Record升级为V3 Qualification；
- Operation V3、Dispatch V4.0、Enforcement 4.1均保持原Digest和状态机；V3只绑定其精确Ref；
- V2/V3可由同一生产Evidence Owner实现；使用相同ID但不同内容或版本必须Conflict，首个记录保持不变；禁止第二Ledger Owner、静默双写或跨版本自动信任升级；
- 自定义组件通过Binding V2登记namespaced kind/capability/schema，未知required schema或治理扩展fail closed；
- 组件Adapter只能依赖Runtime公共`core/ports`，不得导入`foundation/fakes/kernel`内部实现。

## 10. 独占实现边界

未来获代码授权后：

- Runtime Owner独占公共V3合同、Fact/Gateway、store、Conformance与Runtime测试；
- Sandbox Owner只实现Sandbox current Reader/Adapter，不实现Evidence Store/Gateway；
- Harness Owner只实现Session/Turn/Action/Generation current Reader及Application-facing适配，不导出私有Loop Port；
- Context Owner只实现Frame/Manifest/Generation/Injection current Reader，不拥有Action Gateway；
- Application只编排公共Port与step journal，不复制任一Owner事实；
- Provider/组件只产生Candidate/Receipt及领域Inspect，不获得Evidence写权限。

## 11. 硬反例

以下任一出现即拒绝实现或验收：

1. 为pre-run allocate编造占位`RunID`，或把应为forbidden的Run坐标留空而无精确Applicability Policy Ref；
2. Issue后直接调用Provider，未InspectCurrent，或把Issue当Permit/Enforcement；
3. 一个Qualification同时覆盖prepare和execute，或execute不绑定exact prepare与PreparedAttempt；
4. Qualification过期后以新的TTL重Seal同ID，或时钟回拨使其重新current；
5. source同序换payload，通过换Event ID、Qualification ID或Operation ID规避Conflict；
6. Consume分别写Fact、Ledger和cursor，崩溃后出现“资格已用但Record不存在”或双Record；
7. Provider Receipt、Evidence Record或Sandbox Observation直接推进DomainResult、Run Outcome或Runtime Settlement；
8. lost reply后换attempt/sequence盲重派，或把Provider NotFound解释为confirmed_not_applied；
9. Context/Generation/Binding/Sandbox/Credential漂移后仍消费旧资格；
10. 组件私建`EvidenceGate`、万能Hook或内部Runtime类型绕过公共V3 Port。
11. 把Reservation/Lease/Placement/Authority/Policy/Budget/Capability/Credential/Fence复制到V3，由caller声称current；
12. Provider回包跨TTL后仍产Claim/Authoritative Fact，或以bounded ingest Observation继续Permit/Settlement。
13. Candidate重复绑定Scope/Policy/phase/Handoff，或SourceReservation保存完整Policy Ref而不是Policy digest；
14. required维度缺Reader仍写Qualification，或forbidden维度伪造空Reader/空Ref；
15. 未取得create-once ProviderHandoff就调用Provider，或把Handoff当副作用/重派证明；
16. 用首批activation矩阵承载run/admin/custom、Run termination或Action Gateway。

## 12. 已冻结的实现前决策

1. 全部公共类型使用`OperationScopeEvidence...V3`前缀，合同版本`3.0.0`，canonical规则按本设计第4节冻结；
2. Applicability Policy Ref只在Scope顶层，Evidence Policy Ref只在Qualification顶层，SourceReservation只绑定Policy digest，Candidate只绑定Qualification Ref；
3. Applicability矩阵键与首批activation slice按第8.0节冻结；backend-discovery、rollback及termination close/release保持NO-GO，其他kind全部unsupported；
4. Evidence Policy Owner独立，ingest grace上限与`IngestNotAfter`公式按第7节冻结；
5. `OperationScopeEvidenceProviderHandoffRefV3`由Evidence Owner create-once，语义按第6.3节冻结；
6. 可选`OperationScopeEvidenceApplicabilityCurrentReaderV3`按第5.4节冻结；Harness/Context具体Adapter继续后移，不属于首批activation实现。

## 13. 配套资产

- [执行顺序图](./operation-scope-evidence-v3.drawio)
- [实施计划](../../../plan/runtime/operation-scope-evidence-v3.md)
- [Evidence Ledger V2](../evidence-ledger-v2/README.md)
- [Operation与Execution治理V3](../operation-governance-v3/README.md)
- [Sandbox SD-EVID-04](../../sandbox/port-delta.md#sd-evid-04operationscope-aware-pre-run-evidence外部生命周期必需的-runtime-p0)
- [Harness单Call Action Gateway](../../harness/assembly/port-deltas.md#ha-x01a单call-action-gateway)
- [Context pre-run Evidence裁决](../../context-engine/README.md#111-pre-run-evidence裁决)
- [Runtime G6A Action Matrix/Router V1候选](../g6a-action-matrix-router-v1/README.md)
