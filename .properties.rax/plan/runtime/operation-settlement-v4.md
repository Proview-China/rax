# Runtime Operation Settlement V4实施计划

## 1. 状态与授权边界

- 对应设计：[Operation Settlement V4](../../design/runtime/operation-settlement-v4/README.md)；
- 合同版本：additive `4.0.0`；
- 当前状态：P0—P5实现、Owner自测、中央独立复验与最终Review均已完成，最终裁决为YES；本计划作为已完成历史计划保留；
- 实现结果：additive V4公共合同、Gateway、同Owner reference store、Conformance与完整测试矩阵均已落地；
- 当前阶段门禁：不选择生产后端/SLA，不把fake提升为生产实现；后续G6A仍须独立design/plan与联合YES，不由本计划自动解冻。

## 2. 已完成产物

已按批准范围产出：

1. V4强类型Evidence Binding、Submission、Settlement/Inspection Ref和DomainResult Fact Ref；
2. V4 Fact/Governance Port及kind-routed DomainResult current Reader；
3. Runtime Effect/Settlement Owner内原子Settlement+Association+shared terminal guard+V4 terminal projection；
4. V3/V4共享终态互斥，且不改变V3公共合同或digest；
5. Evidence consumed但Settlement未写的可Inspect恢复链；
6. 单元、白盒、黑盒、故障、并发、race、vet和Conformance测试。

不产出Provider调用、生产数据库/RPC/队列、跨Owner事务、Action Gateway、Run/Session/Turn/Action/Context Reader或新的组件业务语义。

## 3. 实施波次

### P0：资产与版本冻结（本计划）

状态：已完成。

- 冻结`4.0.0` discriminator、对象名、canonical和兼容规则；
- 冻结prepare/execute两份exact Evidence Binding；
- 冻结DomainResult exact Fact ref与kind-routed current Reader；
- 冻结两Owner事务边界、shared terminal guard与V4 terminal projection；
- 冻结activation first slice和unsupported矩阵；
- 完成README、drawio、合同、Port Delta、测试矩阵与本计划。

验收：链接、XML、术语扫描与diff-check全绿；无实现文件变化。

### P1：公共合同与canonical

状态：已完成。

- 新增V4独立类型，不修改V3/Evidence V3/Dispatch V4.0/Enforcement 4.1；
- 实现strict Validate、Seal、Clone和stable canonical；
- prepare/execute集合固定两项、canonical唯一且拒绝重复；
- typed ref逐字段交叉校验；
- 单元与fuzz/property覆盖nil/empty、排序、type-pun、tamper和跨版本ID冲突。

### P2：Effect Owner终态存储

状态：已完成。最终shared guard按`(TenantID, EffectID)`分区，V3-first/V4-first对称，跨Tenant独立；历史Guard按Settlement ID读取，不依赖current索引。

- 新增V4 Settlement、Association、shared terminal guard、terminal projection存储；
- 一次Commit四项全有或全无；
- V3 terminal CAS在同一Owner锁内复用guard，但不改变V3公共结构/digest；
- 既有V3 settled逻辑占用guard；V4不写V3 sidecar；
- lost-reply、同内容幂等、换内容Conflict、64并发与V3/V4 race全覆盖。

### P3：Evidence V3强类型关联

状态：已完成。

- 读取每phase exact Consumption、issued/final Qualification、Record、Candidate digest、Handoff、Attempt、4.1 phase与full Scope；
- 只接受final `consumed_current`；late/observation零资格；
- Evidence已消费但Settlement缺失时按原refs恢复；
- 禁止重新Issue/Consume或调用Provider；
- 逐字段漂移、phase swap/missing/duplicate/extra与Reader unavailable零写测试。

### P4：DomainResult Reader与Governance Gateway

状态：已完成。

- 由Binding V2可信装配`EffectKind + DomainResultKind`到唯一Reader；
- Reader只返回authoritative current Fact投影；
- Gateway重读Effect revision、immutable owner、guard、DomainResult与Evidence exact关联；
- Owner Commit前最后一次复读；
- 历史Permit/Policy过期不阻止truthful settlement，也不授新dispatch；
- unknown kind、多Owner、NotFound、lost-reply、观察租约过期与Fact漂移全部fail closed。

### P5：Conformance与activation first slice

状态：已完成。

- 发布只依赖公共Port的V4 Conformance testkit；
- 首批仅允许三元矩阵：`OperationScopeKind=activation_attempt`、`PolicyProfile=praxis.runtime/activation-evidence`、`EffectKind in {praxis.sandbox/allocate, praxis.sandbox/activate, praxis.sandbox/open}`；
- backend-discovery/cancel/rollback/close/release/termination/run/admin/custom/recovery全部拒绝；
- 验证Settlement/Inspect路径Provider计数为零；
- ordinary、race、vet、gofmt、diff-check和必要fuzz/property全绿。

### P6：联合接线门

状态：Settlement V4门已通过；G6A Action matrix/router转入独立design/plan联合评审，不在本计划内实现。

P0—P5中央验收后，只能解冻pre-run activation slice的Application编排。Action Gateway仍需单独完成：

1. Run/Session/Turn/Action/Context required Applicability current Reader；
2. 单Call Action的DomainResult authoritative Reader；
3. Action closed matrix、Owner和Settlement策略联合冻结；
4. 再按`Action Gateway -> per-turn Context refresh -> Checkpoint`顺序接线。

## 4. 候选实现路径

以下实现落点已完成：

| 路径 | 未来内容 |
|---|---|
| `ExecutionRuntime/runtime/ports/operation_settlement_v4.go` | 公共V4合同与Port |
| `ExecutionRuntime/runtime/control/operation_settlement_v4.go` | Fact Validate、terminal guard和projection |
| `ExecutionRuntime/runtime/kernel/operation_settlement_gateway_v4.go` | current复读与受治理提交 |
| `ExecutionRuntime/runtime/fakes/operation_settlement_store_v4.go` | 确定性线程安全测试Store |
| `ExecutionRuntime/runtime/conformance/operation_settlement_v4.go` | 公共Conformance testkit |
| `ExecutionRuntime/runtime/tests/{ports,control,fakes}/operation_settlement_v4_test.go` | 测试族 |
| 各领域Owner批准的`runtimeadapter/**` | kind-routed authoritative DomainResult Reader |

## 5. 测试与验收

### 5.1 单元/白盒

- 全部Validate、canonical、Seal、Clone、enum、phase集合和typed ref；
- prepare/execute exact完整性、full Scope重算与所有ref错配；
- 四对象单事务、shared guard、V3/V4 race；
- DomainResult Reader路由与Owner最终复读；
- lost-reply、idempotency、Conflict、64并发、时钟与TTL边界。

### 5.2 黑盒/Conformance

- 公共Port外部行为；
- Observation/Receipt/Claim/type-pun不能产生Settlement；
- 自定义namespaced kind必须先可信注册Reader，不能自授Owner；
- fake不能宣称production；
- legacy V3不自动升级，V4不伪装V3 settled。

### 5.3 集成/故障

- Evidence consumed后、Runtime Commit前崩溃；
- DomainResult current读后漂移；
- Owner Commit成功回包丢失；
- V3/V4并发与不同Settlement ID同Effect冲突；
- Association/guard/projection半写注入必须全部回滚；
- historical Permit/Policy到期后的truthful settlement；
- late observation永不进入Settlement。

实际执行并通过的基础验证命令：

```text
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
gofmt -l .
git diff --check -- .
```

关键canonical与并发状态机已运行定向高重复测试；中央记录为`count=100` PASS（127.334s）、`race count=20` PASS（238.537s）。full ordinary/shuffle、full race、Vet、gofmt与diff-check亦由Owner和中央独立复验通过。

## 6. 完成条件

- 设计、计划、drawio、代码对象和测试矩阵一一对应；
- V3 Settlement、Evidence V3、Dispatch V4.0与Enforcement 4.1 digest均未变化；
- 每phase exact Evidence V3关系、DomainResult authoritative current Fact和full Scope全部复读；
- 两Owner非原子窗口可恢复，Runtime Owner四对象原子；
- shared terminal guard阻止V3/V4双终态与sidecar；
- lost-reply、type-pun、phase错配、all-ref mismatch、reader unavailable、V3/V4 race与late反例全绿；
- Runtime+Harness联合review最终为YES；后续Action Gateway仍按独立接线门禁推进。

## 7. 完成声明与保留限制

Operation Settlement V4已经完成合同、reference Owner、Gateway、Conformance和测试闭环。Runtime Owner四对象单次publish、lost reply恢复、V3/V4共享terminal guard、历史四对象按Settlement ID复读、跨Tenant隔离均有实际反例。

本完成声明不包含生产数据库、跨进程事务、RPC、availability或SLA，也不授权G6A、Context Refresh、Continuation或真实Provider能力启用。
