# Operation Settlement V4 Port Delta

## 1. Delta原因

现有Operation Settlement V3只接受V2 Evidence Record引用。V2引用不能表达OperationScope Evidence V3的Qualification、Consume终态、Provider Handoff、Attempt、Enforcement 4.1 phase和完整OperationScope关系。把V3字段静默塞入旧结构会改变旧digest并制造跨版本type-pun，因此必须新增additive `4.0.0`合同。

## 2. 新增公共面

| Delta | Owner | 调用方 | 权威输出 |
|---|---|---|---|
| `OperationSettlementEvidenceBindingV4` | Runtime公共合同Owner | Settlement Gateway | phase级Evidence V3强类型绑定 |
| `OperationSettlementSubmissionV4` | Runtime公共合同Owner | Application/Runtime Reconciler提出候选 | 非权威提交候选 |
| `OperationSettlementRefV4` | Runtime Effect/Settlement Owner | V4消费者 | immutable Settlement V4事实引用 |
| `OperationInspectionSettlementRefV4` | Runtime Settlement Gateway | Application/Reconciler | current复读快照引用，不授写权 |
| `OperationSettlementDomainResultFactRefV4` | 对应领域Owner定义Fact，Runtime定义中立引用 | kind-routed Reader/Gateway | exact authoritative DomainResult ref |
| `OperationSettlementDomainResultCurrentReaderV4` | 领域Adapter实现，Runtime可信装配 | Settlement Gateway | authoritative current投影 |
| `OperationSettlementFactPortV4` | Runtime Effect/Settlement Owner | Runtime Gateway | 原子Settlement+Association+guard+projection |
| `OperationSettlementGovernancePortV4` | Runtime Settlement Gateway | Application Coordinator/Reconciler | 受治理Settle/Inspect入口 |

## 3. 不变公共面

以下合同不改字段、不改摘要、不自动升级：

- Operation Settlement V3；
- OperationScope Evidence V3；
- Operation Dispatch V4.0 Admission/Permit；
- Operation Dispatch Enforcement 4.1；
- V2 Evidence Ledger；
- Run Settlement V2。

V3 Settlement实现只需要在既有Owner内部终态CAS路径复用shared terminal guard；这是Owner线性化实现约束，不是V3公共对象或digest变更。

## 4. 精确读写顺序

```text
1. Evidence Owner原子Consume prepare资格
2. Evidence Owner原子Consume execute资格
3. Runtime Gateway读取Effect revision与immutable owner
4. Runtime Gateway读取shared terminal guard
5. kind-routed Reader读取exact DomainResult current Fact
6. Runtime Gateway逐项读取两份Consumption、issued/final Qualification、Record、Candidate digest、Handoff、Attempt、4.1 phase、full Scope
7. 构造并Seal Submission V4
8. Runtime Effect/Settlement Owner再次复读并原子Commit四项终态事实
9. 回包丢失只Inspect原Settlement/effect identity
```

步骤1—2与步骤8属于不同Owner事务，不宣称原子。步骤8前崩溃时，Evidence保持已消费，恢复沿原exact refs继续。

## 5. 版本与兼容策略

- 新domain/version/type discriminator；
- 同一Settlement ID跨V3/V4使用时由Owner返回Conflict与IdempotencyPayloadMismatch，首记录不变；
- 同一Effect即使使用不同version-specific Settlement ID，也受shared terminal guard约束；
- 旧V3 settled视为guard占用，不补写V4 sidecar；
- V4 settled不会把V3 Fact或V3 Effect状态改成settled；V4消费者读取独立terminal projection；
- 未识别V4 schema/capability/DomainResult kind/Reader一律fail closed。

## 6. 首批适用范围

V4首批只允许与Evidence V3相同的closed matrix：

```text
OperationScopeKind = activation_attempt
PolicyProfile = praxis.runtime/activation-evidence
EffectKind in {
  praxis.sandbox/allocate,
  praxis.sandbox/activate,
  praxis.sandbox/open
}
```

prepare与execute都必须存在。backend-discovery、cancel、rollback、close、release、termination、run、admin、custom和recovery profile保持unsupported；Action Gateway不随本Delta解冻。

## 7. 独占实现路径候选

下列路径已在Runtime+Harness联合review为YES后按既有用户授权完成；它们仍是reference/fake实现，不代表生产后端：

| Owner | 候选路径 | 内容 |
|---|---|---|
| Runtime | `ExecutionRuntime/runtime/ports/operation_settlement_v4.go` | V4公共类型与Port |
| Runtime | `ExecutionRuntime/runtime/control/operation_settlement_v4.go` | Validate、terminal projection与Owner事务 |
| Runtime | `ExecutionRuntime/runtime/kernel/operation_settlement_gateway_v4.go` | current复读与Governance Gateway |
| Runtime | `ExecutionRuntime/runtime/fakes/operation_settlement_store_v4.go` | 确定性CAS测试Store，不宣称生产 |
| Runtime | `ExecutionRuntime/runtime/conformance/operation_settlement_v4.go` | 公共Conformance testkit |
| Runtime | `ExecutionRuntime/runtime/tests/{ports,control,fakes}/operation_settlement_v4_test.go` | 单元、白盒、黑盒、故障与并发测试 |
| Domain adapters | 各组件Owner批准的`runtimeadapter/**` | kind-routed authoritative current Reader |

不得由6+1组件修改Runtime公共合同，不得跨组件导入实现包，不得把`foundation/fakes/kernel`作为生产依赖。

## 8. Action Gateway解冻边界

本Delta闭合的是pre-run activation slice的Evidence→Operation Settlement强类型关联。Action Gateway仍需独立冻结并实现Run/Session/Turn/Action/Context required的Applicability current Reader、单Call Action领域DomainResult Reader与其closed matrix。Settlement V4通过中央验收只是必要条件，不是Action Gateway自动授权。

## 9. 最终Owner实现约束

- V3 CAS与V4 Commit使用同一Runtime Effect Owner实例、同一锁和同一`(TenantID, EffectID)` shared terminal guard；
- V4四对象及其historical/current索引在一次copy-on-write publish边界全有或全无；
- historical Guard Inspect按`GuardRef.Settlement.ID`读取历史闭包；current-by-Effect只服务current查询，不能替代历史事实源；
- V3-first/V4-first对称，不同OperationDigest/Settlement ID不能绕过；跨Tenant相同Effect ID互不冲突。
