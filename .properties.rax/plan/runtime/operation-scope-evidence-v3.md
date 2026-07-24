# Runtime OperationScope Evidence V3实施计划

## 1. 状态与授权

- 设计事实源：[OperationScope Evidence V3设计](../../design/runtime/operation-scope-evidence-v3/README.md)；
- 当前状态：Runtime二审文件级全YES，六项实现前决策已冻结；**尚未授权实现代码**；
- 当前允许：依据冻结合同细化测试与未来实现切面；
- 当前禁止：创建或修改Runtime/Harness/Sandbox/Context/Application代码，选择生产后端/SLA，stage或commit。

## 2. 计划完成后将产出什么

另行获实现授权后，产出一个与Evidence V2并存但不互相升级的V3切面：

1. versioned OperationScope Evidence公共对象、strict canonical与Validate；
2. create-once Qualification、历史/current Inspect、一次性Consume；
3. 同一Evidence Owner、独立Operation ledger scope内原子完成Record、chain、cursor、Qualification consumed与Consumption Association；不调用V2 Append、不双写；
4. Runtime Gateway对exact Authorization/Admission/Permit、4.1 phase、Generation、Evidence/Applicability Policy与所需Reader的第三次复读；
5. Sandbox/Harness/Context只读Adapter与Application step-journal编排边界；
6. 单元、白盒、黑盒、故障注入、并发、race、vet、fuzz/property及跨模块集成证据。

不产出生产数据库/RPC/队列/Provider、进程拓扑、SLA，也不实现Action Gateway、per-turn Context Refresh或Checkpoint业务语义。

## 3. 严格实施波次

### P0：Runtime决策与版本冻结（资产已完成）

- 冻结独立`OperationScopeEvidenceLedgerScopeV3`、Qualification内source/event key保留及唯一Evidence Owner事务schema；
- 冻结Applicability/Evidence Policy Ref唯一层级、bounded ingest公式、首批activation矩阵、中立Reader和ProviderHandoff；
- 冻结`3.0.0` discriminator、ID/revision/digest/TTL规则和V2/V3同ID冲突语义；
- 冻结`OperationScopeKind + EffectKind + PolicyProfile`矩阵；首批精确限定为`activation_attempt + praxis.runtime/activation-evidence + praxis.sandbox/{allocate,activate,open}`，Generation required，Run/Session/Turn/Action/Context全部forbidden。
- `praxis.sandbox/backend-discovery`命名映射已确认，但live Sandbox合同未实现该Effect，保持NO-GO；rollback不得映射`praxis.sandbox/cancel`；`praxis.sandbox/{close,release}`因termination scope冲突保持NO-GO。

验收：无隐式默认、无占位Run、无V2扩写、无Evidence授予dispatch的路径。

### P1：公共合同与canonical

- 落地统一`OperationScopeEvidence...V3`前缀的Scope、Matrix Key、Applicability、Qualification、SourceReservation、ProviderHandoff、Candidate、Consumption、Record Ref、Reader与Port；
- strict JSON、duplicate key、unknown required、nil/empty逐字段规则；
- canonical禁止map；集合稳定排序并拒绝重复；nil slice归一；required/forbidden为tagged union；TTL/phase/source/event/policy进入对应digest；
- fuzz/property验证稳定摘要、排序、clone与tamper拒绝。
- Watch、Batch、Retention、跨scope查询与V2迁移全部后移，不进入首切面。

### P2：Fact Store与Governance Gateway

- 实现create-once Issue、历史/current Inspect、expire/revoke、Consume；
- 将Record、chain、cursor、Qualification consumed、association形成唯一Evidence Owner单一逻辑提交；禁止先V2 Append再V3 CAS；
- Gateway按`Issue→InspectCurrent Qualification+4.1→ProviderHandoff→Provider→Consume`复读exact refs和required Reader；prepare/execute使用不同资格；
- lost reply只Inspect原ID/source key，Unknown不换attempt重派。

### P3：Conformance与Runtime门禁

- 发布只依赖公共Port的V3 Conformance testkit；fake固定不具生产Conformance资格；
- 验证跨版本ID冲突、source gap/同序换内容、TTL/时钟回拨、phase/attempt漂移、并发Consume一次线性化；
- ordinary、race、vet、diff-check及定向fuzz/property全部通过。

### P4：只读Adapter

- Sandbox沿4.1 Reader从phase ref复读Attempt/Lease/Instance/Fence/Scope/GenerationBinding/Placement/Backend/Slot/Provider，不向V3复制自由字段；
- 首批仅装配Generation所需的`OperationScopeEvidenceApplicabilityCurrentReaderV3`；required缺Reader在Issue零写前fail closed，forbidden维度Reader可nil；
- Harness/Context Adapter继续后移：首批矩阵将Run/Session/Turn/Action/Context设为forbidden；未来required时再实现同一中立Reader，观察租约不冒充领域TTL；
- Adapter零写、零Provider、零事实升级；丢回包返回零current projection并fail closed。

### P5：pre-run lifecycle集成

- 只对冻结矩阵中的`activation_attempt + praxis.runtime/activation-evidence + praxis.sandbox/{allocate,activate,open}`接入Issue→Inspect→Handoff→Provider→Consume；
- prepare和execute各用独立Qualification；
- Unknown先Inspect原Provider Attempt；DomainResult、Runtime Settlement和ApplySettlement仍由原Owner链完成；
- Provider回包跨TTL仅在Policy bounded ingest window内降级为Observation；禁止Claim、Authoritative Fact、Permit、Enforcement或Settlement；
- `praxis.sandbox/backend-discovery`在live Sandbox合同落地并重新联合评审前保持资产候选/NO-GO；rollback不得映射cancel；close/release继续走termination scope，不进入本activation slice；
- run/admin/custom、Run termination、Action Gateway及其他未注册kind保持unsupported、零Provider调用。

### P6：后续接线解冻门

在P0—P5联合验收后，才允许按以下顺序单独申请实现授权：

```text
单Call Action Gateway
-> per-turn Context Refresh
-> Checkpoint
```

P3b万能Hook、N>1 Tool Call、隐式Context联网刷新及Checkpoint外部世界回滚不随本计划解冻。

## 4. 未来独占路径

以下均为获实现授权后的候选落点，当前不创建：

| Owner | 独占路径 | 责任 |
|---|---|---|
| Runtime | `ExecutionRuntime/runtime/ports/operation_scope_evidence_v3.go` | 公共合同与Port |
| Runtime | `ExecutionRuntime/runtime/control/operation_scope_evidence_fact_v3.go` | Fact状态机与Validate |
| Runtime | `ExecutionRuntime/runtime/control/operation_scope_evidence_gateway_v3.go` | Issue/InspectCurrent/Consume治理 |
| Runtime | `ExecutionRuntime/runtime/fakes/operation_scope_evidence_store_v3.go` | 确定性线程安全测试Store，不宣称生产 |
| Runtime | `ExecutionRuntime/runtime/conformance/operation_scope_evidence_v3.go` | 公共Conformance testkit |
| Runtime | `ExecutionRuntime/runtime/tests/{ports,control,fakes}/operation_scope_evidence_v3_test.go` | Runtime测试族 |
| Sandbox | `ExecutionRuntime/sandbox/runtimeadapter/**`中经Owner批准的新V3 Reader文件 | 只读Sandbox current投影 |
| Harness | `ExecutionRuntime/harness/**`中未来经Owner批准的V3 Reader Adapter | 首批不实现；未来required时只读Run/Session/Turn/Action投影 |
| Context | Context Engine模块未来经Owner批准的`runtimeadapter/**` | 首批不实现；未来required时只读Frame/Manifest/Injection投影 |
| Application | `ExecutionRuntime/application/**`中经Owner批准的V3 Coordinator step | 只编排公共Port和恢复Journal |

禁止跨Owner并发修改Runtime公共类型，禁止组件复制V3结构，禁止导入Runtime `foundation/fakes/kernel`或Harness私有Loop Port。

## 5. 测试计划

### 5.1 单元与白盒

- 每个Validate、canonical、seal、clone、状态跃迁、TTL最小值和applicability分支；
- source key唯一性、cursor/gap、phase与Attempt exact绑定；
- Issue不推进Ledger，Consume一次性同时产生Fact/Record/cursor/association；
- Issue同一创建保留source/event key；不得生成chain/sequence，Consume不得调用V2 Append；
- Scope/Qualification/SourceReservation/Candidate的Policy与引用层级逐字段反例；
- ProviderHandoff create-once、checked/not-after/phase/attempt漂移、同ID换内容冲突；
- `now == expiry`拒绝、时钟回拨不复活、同ID重Seal冲突；
- unknown required schema/capability/kind fail closed。

### 5.2 黑盒与Conformance

- 自定义namespaced组件只通过Binding V2公共Ref接入，不含内部句柄；
- V2/V3双向同ID冲突，首记录不变，第二版本NotFound；
- Provider/组件自报不能获得Evidence写权限或dispatch资格；
- V2/V3由同一Owner实现时仍只有一条目标scope chain，零双写、零自动升级；
- fake、external adapter、contained observer均不能自报production或fully controlled。

### 5.3 故障与并发

- Issue、Consume、Ledger append、association回包丢失后的Inspect恢复；
- 两Coordinator并发Issue/Consume只线性化一次；同source sequence换内容稳定Conflict；
- 每个current Reader逐字段漂移、TTL、revocation、NotFound与read lost-reply；
- Provider prepare/execute unknown只Inspect，零盲重派；
- Provider回包刚好跨TTL、bounded ingest window内/边界/越界三组反例；window内只产生Observation且不能进入Claim/Permit/Settlement；
- ingest grace逐项验证`qualification expiry+grant`、30秒安全上限、policy expiry、source reservation expiry的最小值；
- 首批矩阵精确限定`activation_attempt + praxis.runtime/activation-evidence + praxis.sandbox/{allocate,activate,open}`，Generation required、其余五维forbidden；backend-discovery、rollback、close、release及其他unsupported kind在Issue零写前拒绝且Provider计数为零；
- 反例固定覆盖rollback→cancel映射拒绝，以及close/release以activation_attempt进入矩阵时的scope冲突；
- Consume提交边界故障不得产生半Record、双cursor或双association。

### 5.4 集成顺序

1. Runtime公共V3合同→Store→Gateway→Conformance；
2. Runtime + Sandbox Reader；
3. Runtime + Harness/Context Reader；
4. pre-run lifecycle单kind；
5. 单Call Action Gateway；
6. per-turn Context Refresh；
7. Checkpoint。

每级必须普通测试、`-race`、`go vet`及边界反例全绿后进入下一级；不得用fake成功冒充生产Backend/SLA。

## 6. 完成门禁

- Runtime二审冻结的六项实现前决策全部被公共对象、状态机与测试覆盖；
- design、plan、drawio与代码对象一一对应；
- Issue/Inspect/Consume、Owner、TTL、漂移、并发和lost-reply测试充分；
- V2/V3兼容与自定义组件扩展反例通过；
- Runtime/Sandbox/Harness/Context/Application联合审计YES；
- 用户另行明确授权下一接线波次。

## 7. 已冻结的实现前决策

1. 公共类型前缀、`3.0.0`和canonical规则已冻结；
2. Scope/Qualification/SourceReservation/Candidate的引用层级已冻结；
3. 首批activation矩阵及unsupported范围已冻结；backend-discovery、rollback与termination close/release不属于首批；
4. 独立Evidence Policy Owner、30秒安全上限和IngestNotAfter公式已冻结；
5. Evidence Owner create-once ProviderHandoff已冻结；
6. 中立Applicability Current Reader已冻结；Harness/Context具体Adapter继续后移。
