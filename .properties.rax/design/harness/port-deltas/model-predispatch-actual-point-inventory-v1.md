# Model PreDispatch Actual-Point Inventory V1 Port Delta

## 1. 状态与用例

- 状态：`candidate-p0`，等待Model Owner与Harness Assembly联合设计审定；本文件不授权修改Model、Runtime或Harness Go；
- 用例：Harness已经实现Model公开`PreparedModelInvocationCommitGateV1`的concrete Gate与ACK Repository，但live Model的`routegateway/direct/operation/provider/stream/continuation/realtime`实际执行路径没有一个公开、版本化、可由Harness Assembly穷举核验的actual-point inventory；
- 目标：让Model发布完整actual-point**候选声明**，Harness以AssemblyInput、Component Manifest、Module/PortSpec/Factory/ProviderCandidate及production wiring inventory独立Inspect并封存Conformance；缺项、别名或旁路一律Fail Closed；
- 非目标：不新增通用Hook，不改变`ModelTurnPort`，不让Harness调用Provider，不让Runtime拥有Model语义，不把Model自报声明升级为正式接线事实。

现有可复用合同：

- Model Gate与ACK：[prepared_model_invocation_v1.go](../../../../ExecutionRuntime/model-invoker/prepared_model_invocation_v1.go)中的`PreparedModelInvocationCommitGateV1`、`PreparedModelInvocationCommitAckV1/RefV1`、`InspectPreparedModelInvocationCommitAckV1`；
- actual-point Observation：[prepared_model_invocation_v1.go](../../../../ExecutionRuntime/model-invoker/prepared_model_invocation_v1.go)中的`PreparedModelInvocationDispatchValidationReceiptV1`与`SealPreparedModelInvocationDispatchReceiptAgainstV1`；Receipt不授进入权；
- Harness Assembly结构：[types.go](../../../../ExecutionRuntime/harness/assemblycontract/types.go)中的`ModuleDescriptorV1/CapabilityDescriptorV1/SlotSpecV1/PortSpecV1/ModuleFactoryDescriptorV1/ProviderBindingCandidateV1/DependencySpecV1`；
- 已冻结专用Capability：`praxis.harness/model-predispatch-surface-commit-gate-v1`，`one_active_binding`；`model.dispatch.before` Phase声明本身不能证明runtime Gate。

## 2. Owner与非Owner

| 对象 | 语义Owner | 允许 | 禁止 |
|---|---|---|---|
| actual-point实现与候选inventory | Model Owner | 在Model公开根包发布sealed nominal、Reader与完整候选集合 | import Harness实现包；把inventory当Gate ACK或进入权 |
| Gate/ACK Repository | Harness/host Owner | 实现Model公开两方法Gate；create-once ACK与exact恢复 | 生成Model Prepared Fact；拥有Provider调用语义 |
| Assembly Conformance | Harness Assembly Owner | 独立交叉核验候选、全图与production inventory后seal | 信任Model自报；只看Phase/Capability名字 |
| Runtime | Runtime Owner | 继续承载既有中立Binding/Generation/current事实 | 新增Model语义枚举或成为inventory Owner |
| Provider/Receipt | 各Provider/Model Observation Owner | 提供Observation | 生成Conformance、ACK或production资格 |

## 3. 请求的Model additive公共合同

建议由Model公开根包发布下列**nominal候选**；名称与字段须经Model/Harness联合审定后冻结，Harness不得先建私有兼容类型：

```go
type PreparedModelInvocationActualPointKindV1 string

type PreparedModelInvocationActualPointCandidateV1 struct {
    Kind                     PreparedModelInvocationActualPointKindV1
    ComponentID              runtimeports.ComponentIDV2
    ComponentManifestDigest  core.Digest
    ArtifactDigest           core.Digest
    ModuleRef                string
    PortSpecRef              string
    FactoryRef               string
    ProviderCandidateRef     string
    GateCapability           runtimeports.CapabilityNameV2
    GatePortSpecRef          string
    GateFactoryRef           string
    GateBindingRef           string
    AckInspectRequired       bool
    AttemptCurrentRequired   bool
    DispatchReceiptRequired  bool
    BoundaryContractVersion  string
    Digest                   core.Digest
}

type PreparedModelInvocationActualPointInventoryV1 struct {
    ContractVersion string
    ID              string
    Revision        core.Revision
    Publisher       runtimeports.ComponentIDV2
    Candidates      []PreparedModelInvocationActualPointCandidateV1
    Digest          core.Digest
}

type PreparedModelInvocationActualPointInventoryReaderV1 interface {
    InspectExactPreparedModelInvocationActualPointInventoryV1(
        context.Context,
        PreparedModelInvocationActualPointInventoryRefV1,
    ) (PreparedModelInvocationActualPointInventoryV1, error)
}
```

字段语义：

1. `Kind`必须来自联合冻结的closed集合，至少覆盖`execution_runtime_start`、`direct_resolve`、`direct_invoke`、`direct_stream`、`generic_capabilities`、`generic_invoke`、`generic_stream`、`routegateway_capabilities`、`routegateway_invoke`、`routegateway_stream`、`operation_composite`、`provider_continuation`、`realtime`及每个hosted/remote/provider adapter入口；
2. Component/Manifest/Artifact/Module/PortSpec/Factory/ProviderCandidate必须是完整exact结构坐标，禁止只传package名、Kind、自由字符串路径或裸digest；
3. Gate capability固定为`praxis.harness/model-predispatch-surface-commit-gate-v1`；Gate Port/Factory/Binding必须指向同一active binding；
4. 三个布尔硬门必须全为`true`：每个attempt前exact ACK Inspect、全部Owner current复读、dispatch receipt Observation；任一false不允许进入受治理Generation；
5. Inventory必须canonical seal、deep clone、exact Reader；同ID/revision换内容Conflict，旧revision只可历史读取且不得current；
6. Model发布的Inventory仍只是candidate/Observation。Harness必须从Assembly全图和production wiring反向枚举所有可到达Provider actual-point，再与Inventory做双向exact闭集比较。

### 3.1 Inventory以外仍需Model Owner冻结的两个公开能力

live核验确认Inventory只能证明“哪些actual point应存在”，不能把已提交ACK送到actual point，也不能保存每次dispatch检查的Observation。因此Model Owner还须与Harness联合冻结下列两个additive公共能力；本Delta只冻结语义下限，不先发明type名、canonical或Repository：

1. **actual-point guard carrier**：从受治理Preparation无损携带完整`PreparedModelInvocationCommitAckRefV1`到每个actual point，并exact绑定closed boundary kind、dispatch sequence、provider attempt ordinal和request digest。`execution.Invocation`、`RouteCall`、operation/realtime Request当前均没有这些字段；不得通过context value、裸string、latest ACK或包内全局表补齐；
2. **dispatch receipt sink/reader**：Model Owner在每次Provider/Backend调用前完成ACK/Prepared/Current复读后，seal现有`PreparedModelInvocationDispatchValidationReceiptV1` Observation，并通过公开create-once writer与exact reader保存/复读。Receipt仍不授进入权；sequence/ordinal只能由Model attempt Owner产生，Harness不得代发。

两个能力的字段、Owner Repository、canonical、lost-reply和兼容方案必须由Model Owner设计并联合审定。Inventory nominal/Reader单独落地不能解锁actual-point代码或production Conformance。

另有一个既有路径拆分硬门：`execution/direct.Adapter.Preflight`当前在ACK前调用`Backend.Resolve`。Phase A必须先改为纯Preparation，任何Backend Resolve/Capabilities/Provider调用在Gate Commit前计数均为0；不能通过在现有调用之后补一次ACK Inspect来声明符合。

## 4. Harness独立Admission与输出

Harness Assembly收到exact Inventory后固定执行：

```text
Inventory exact Inspect
-> Model publisher ComponentManifest/Artifact exact
-> required Capability + exactly one active Gate Slot/PortSpec/Factory/Binding
-> scan Modules/Ports/Slots/Factories/Dependencies/ProviderCandidates/Phases
-> scan direct/hosted/remote/realtime/continuation production wiring inventory
-> 双向closed-set比较：declared == reachable actual points
-> 每个actual point验证Gate binding + pre-attempt Ack/current guard + Receipt Observation
-> seal Harness-owned Conformance
```

Harness Conformance至少输出：Inventory exact Ref、Assembly Input/Manifest/Graph/Generation/Handoff摘要、Gate active binding exact坐标、closed actual-point集合摘要、production wiring inventory exact Ref、Checked/Expires、Conformance digest与`conformant|rejected`状态。它不包含raw Provider句柄，不授dispatch authority。

## 5. 不变量、Effect与Recovery

- Phase A保持纯Preparation，Provider/Backend/动态Capabilities调用数为0；
- `Commit`成功只形成Model ACK；每个actual-point attempt仍须fresh `InspectExactAck`并复读Prepared/Assembly/Surface current；一次ACK不是永久或单次执行许可；
- `PreparedModelInvocationDispatchValidationReceiptV1`仅是Observation，不是Permit、Evidence、Binding Fact或Runtime Outcome；
- ACK/Inventory/Conformance任何Read为Unavailable、Indeterminate、过期或漂移时，actual-point调用数必须为0；
- Commit丢回包只Inspect原ACK；actual-point未知结果由Model/Provider既有attempt Inspect恢复，禁止二次Provider执行；
- Inventory Reader丢回包不适用写恢复；只允许原exact Ref重读，不得按ID取latest替代；
- Context ExpectedInjection、Tool Result、Runtime Settlement不进入本Inventory；它们保持各Owner独立链。

## 6. 硬反例

1. 只有`model.dispatch.before` Phase Gate，没有Gate Port/Factory/Binding：拒绝；
2. Inventory漏掉routegateway Stream、direct Resolve、continuation、realtime或任一provider adapter：拒绝；
3. Inventory声明完整，但Assembly存在额外raw ProviderCandidate、Factory output、Slot/Dependency alias或production wiring edge：拒绝；
4. 同一个Provider actual-point通过另一个Module/PortSpec/Factory别名可达：拒绝；
5. Gate与actual-point使用不同active binding、不同Component Manifest/Artifact或旧revision：拒绝；
6. ACK存在但未在当前attempt前fresh Inspect，或current窗口已过期：Provider调用数0；
7. Capabilities/Resolve在ACK前发生，或动态Capabilities改变Tools/ActualToolSurface/ActualProviderInjection：拒绝当前Invocation；
8. Receipt缺失、错误Attempt ordinal/request digest/boundary kind，或被当作进入权：拒绝；
9. Model Inventory自报与全图相同，但production composition未提供sealed wiring inventory：production资格拒绝；
10. Harness私建Model DTO/Reader、Model import Harness实现包、Runtime新增Model kind：import/conformance失败；
11. Inventory完整但Invocation/RouteCall/operation/realtime载体没有exact ACK ref，或以context value/裸string/latest补齐：Provider调用数0；
12. ACK/current复读成功但没有Model Owner create-once Receipt sink，或sequence/attempt ordinal由Harness生成：拒绝production Conformance；
13. direct Preflight在Gate前调用`Backend.Resolve`，即使随后Commit/Inspect成功：拒绝，Backend调用数必须为0。

## 7. 兼容与迁移

- additive V1，不修改既有Prepared Fact/Current/ACK/Receipt canonical或方法集；
- 未发布Inventory的旧Model路径仍可用于不含production资格的测试，但受治理Generation与production Capability必须Fail Closed；
- Inventory上线后先以reference/test composition验证，再由Model actual-point代码审计与Harness Assembly Conformance联合验收；二者缺一不可；
- 不引入Runtime Port变化，不改变Tool/Context/Application合同；
- Model public nominal实际落盘并compile前，Harness不得实现私有等价Reader或用静态源码扫描冒充production Conformance。

## 8. 当前裁决

当前P0共有三组：Model public exact Inventory nominal/Reader与closed `Kind`集合、ACK exact actual-point carrier、dispatch Receipt create-once sink/reader；此外direct Preflight仍须移除Gate前`Backend.Resolve`。Harness可继续维护Owner-local Gate/M2，但专用Assembly Capability/Conformance与production no-bypass保持`NO-GO`。这些P0不阻塞Tool V2 Consumer或Context Owner-local开发，但阻塞Model受治理Generation、system production资格与production root。
