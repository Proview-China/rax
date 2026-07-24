# Harness Assembly对象、版本与状态

## 1. 统一Envelope

跨边界对象至少携带：`ContractVersion`、`ObjectID`、`Revision`、`OwnerRef`、`ScopeRef`、`CreatedUnixNano`、`Digest`、`EvidenceRefs`。需要当前性控制时增加`Epoch`、`ExpiresUnixNano`、`PolicyRef`和`CapabilityGrantDigest`。

统一Envelope只解决身份、版本、范围与追踪，不合并Candidate、Observation、Attestation、Decision、Permit、Settlement和Fact的权威等级。

## 2. 声明对象

| 对象 | 必备字段 | Owner与用途 |
|---|---|---|
| `ModuleDescriptorV1` | ContractVersion、ModuleID、Namespace、SemanticVersion、ArtifactDigest、Publisher/SourceRef、ComponentManifestV2Ref、Compatibility、Capabilities、Schemas、Locality、Residual、Owners、CredentialRequirements | 模块发布者声明；不授予Binding |
| `CapabilityDescriptorV1` | Capability、Version、Schemas、Required/Provided、TTL、EffectClass、OwnerCapability、Conformance | 描述能力，不是Grant |
| `SlotSpecV1` | SlotID、ContractVersion、LifecycleScope、Cardinality、Required、OwnerCapability、ContributionKinds、Input/OutputSchema、EffectClass、Concurrency/Failure/DegradationPolicy、Dependencies、Digest | Harness Assembly拥有Slot合同与公共Catalog版本 |
| `SlotContributionV1` | ContributionID、ModuleRef、SlotRef、Kind、CapabilityRef、PortSpecRef/ProviderCandidateRef、Priority、Dependencies、Digest | 模块贡献材料，不获得Slot所有权；不得携带HookFaceRef或WriteSet |
| `PortSpecV1` | PortID、ContractVersion、OwnerCapability、Request/ResponseSchema、OperationClass、EffectKind/ConflictDomain规则、Review/Fence/Authority/Scope/Budget要求、Idempotency、OperationScopeRef、InspectContractRef、CleanupContractRef、DomainResult/Runtime Operation Settlement/ApplySettlement合同、可选RunStartRequirementRefs、可选RunSettlementRequirementRefs、FailureSemantics、Compatibility | 领域Owner发布稳定Port语义；三类Requirement互不替代 |
| `PortBindingV1` | PortSpecRef、ProviderBindingRefV2、BindingSetRef、CapabilityGrantRef、Endpoint/Locality、Evidence/Expiry、ActualSchemaDigests | post-binding实际绑定Observation；不进入pre-binding Manifest/Graph，也不等于Permit |
| `HookFaceSpecV1` | HookFaceID、PhaseID、Kind、Input/OutputSchema、AuthorityCeiling、MutationMask、EffectClass、Timeout/Failure/Concurrency/ReceiptPolicy、Digest | Harness Assembly定义公共Phase/HookFace Catalog与扩展权力上限 |
| `PhaseContributionV1` | ContributionID、HookFaceRef、HandlerDescriptorRef、ModuleRef、Capability（observer/filter/gate/port）、Dependencies、Priority、WriteSet、Async、Digest | 只能在HookFace上限内运行；HookFaceRef与WriteSet只属于Phase贡献 |
| `DependencySpecV1` | FromRef、ToRef、Relation、Required、VersionRange、Capability、FailureMode | 构成确定性DAG |
| `ModuleFactoryDescriptorV1` | FactoryID、ModuleRef、ArtifactDigest、ConstructionMode、InputSchema、OutputCapability、Lifecycle、CleanupContract、TrustRef | 只描述构造方式；Manifest不保存原始函数指针 |

所有ID采用namespaced稳定名称；所有Schema使用Runtime `SchemaRefV2`或经联合评审后的后继版本；大Payload只保存受限Envelope或Artifact Ref。

对象Owner不会随装配而转移：Harness Assembly只拥有Slot/Phase Catalog、Assembly Generation/Manifest/Graph和报告；模块发布者拥有Module/Contribution声明；领域Owner拥有Port语义、领域Manifest、Run Requirement、Inspect/CAS/Settlement；Runtime拥有Binding、Operation、Policy/Trust、Evidence、Run与Outcome；Application拥有跨域协调过程。

## 3. 编译输入与产物

### `AssemblyInputV1`

必须原样带入：

- ResolvedAgentPlan Ref/Digest；
- HarnessBootstrapPlan Ref/Digest；
- Profile、RuntimePolicy、HarnessStack、SemanticRoute、ContextPlan、ToolSurface、CapabilityGrant摘要；
- Expected InjectionManifest Ref/Digest；运行期Actual Injection不属于编译输入；
- 已存在的Runtime Policy/Capability/Schema current facts；初次编译不得要求本次Generation尚未产生的BindingSet；
- RouteBinding、ComponentManifestV2、ModuleDescriptor、Slot/Port/Phase Contribution集合；
- 编译Policy、目标Contract版本和允许Residual。

Compiler不得补写、重算或宽松合并这些上游事实。

编译与绑定采用单向两阶段协议，禁止循环依赖：

1. `AssemblyInputV1`在pre-binding阶段生成sealed但不可执行的Generation/Manifest/Graph；
2. `AssemblyHandoffV1`携带Generation/Manifest/Graph Digest与Provider Binding Candidate交给Runtime；
3. Runtime独立形成`BindingSetV2`后，`AssemblyBindingConformanceV1`只读核对Generation、Binding、Capability、Schema与currentness；
4. Model Turn执行后，Harness Model Turn Adapter才形成`ActualInjectionManifest`，Context Owner再形成`InjectionConformanceFact`。Actual不得反向写回原AssemblyInput或改变其Digest。

### `AssemblyGenerationV1`

字段：GenerationID、Revision、InputDigest、CompilerVersion、ContractVersion、CreatedAt、State、ManifestDigest、GraphDigest、ConformanceReportDigest、ResidualReportDigest、PreviousGenerationRef（迁移时）和EvidenceRefs。

同一规范化输入、Compiler版本与Policy必须得到同一Manifest/Graph Digest。Generation ID不能仅使用时间或随机数参与语义摘要。

### `AssemblyManifestV1`

保存全部Module、Capability、Slot、Contribution、PortSpec/Provider Binding Candidate、HookFace、Phase Order、Owner、Schema、Locality、Residual、上游摘要和Runtime handoff输入。pre-binding Manifest不得保存本次Generation尚未产生的BindingSetRef或PortBindingV1。Manifest可序列化、签名、缓存和跨进程传输；它不是运行时对象图。

### `CompiledHarnessGraphV1`

保存热路径直接消费的：

- 固定Slot索引和唯一Owner绑定；
- 预排序HookFace Handler数组与依赖拓扑；
- 类型化Port调用表和Schema validator；
- 常量预算/上限/失败策略引用；
- Manifest/Generation摘要；post-binding的BindingSet摘要只存在于`AssemblyBindingConformanceV1`，不得反向改变Graph Digest；
- 只读SessionServices模板。

Graph不可序列化原始Secret、Credential或跨模块内部指针；进程内函数引用只在验证后的实例化阶段产生，并受Graph Digest约束。

### 报告对象

- `AssemblyDiagnosticV1`：severity、code、object/field path、owner、expected/actual、evidence、remediation；
- `ConformanceReportV1`：Expected/Actual、Capability、Schema、Locality、Control、Owner和expiry结论；
- `ResidualReportV1`：Residual class、owner、scope、inspect/cleanup contract、是否允许、阻断阶段；
- `RuntimeProviderBindingV1`：Assembly产出的Runtime handoff候选，携带Generation/Manifest/Graph/BindingSet/Provider摘要；不得冒充Runtime Binding Fact或dispatch资格。

## 4. Assembly状态机

Builder是进程内临时对象，不是持久权威事实。持久状态由不可变Attempt/Generation报告表达：

```text
collecting
-> validating
   -> rejected
   -> compiled
      -> conformance_checked
         -> sealed
            -> runtime_handoff_ready
               -> invalidated (currentness漂移，只能产生新Generation)
```

规则：

- 每一步输出新revision或新不可变对象，不原地修改sealed Generation；
- rejected必须含结构化Diagnostic，不产生可执行Graph；
- conformance失败或不允许Residual不得进入sealed；
- Runtime handoff只表示可提交认证，不表示已Activation；
- ComponentManifest、Route、Schema、权限、HookFace顺序或核心Slot变化必须产生新Generation；
- Plan已经声明的动态能力启停只可在Capability/Degradation包络内发生，不改变Graph结构。

## 5. 错误与CAS语义

- `Rejected`：尚未进入外部执行；修复输入后重新编译；
- `Failed`：本地编译器或受控构造失败，未产生外部Effect；
- `Indeterminate`：仅用于编译阶段确实调用了受治理外部Probe且结果未知；必须Inspect该Probe Effect；
- `Expired`：证据或Binding currentness过期；重新取证，不复用旧Generation；
- `Conflict`：摘要、Owner、Schema、Slot cardinality、write-set或CAS冲突；Fail Closed；
- `Unavailable`：必需依赖不可读；无显式离线Policy时Fail Closed；
- `Residual`：允许降级也必须进入ResidualReport，不能消失。

所有可重试命令携带Idempotency Key；状态更新携带ExpectedRevision；任何外部Probe使用独立Governed Effect，不把编译重试当Effect重试。
