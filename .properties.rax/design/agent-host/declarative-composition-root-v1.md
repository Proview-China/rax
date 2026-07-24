# Declarative Agent Composition Root V1 设计候选

## 1. 状态与目标

- 状态：候选，待用户设计审核；未授权实现。
- 独立设计反审：`YES（P0/P1=0）`；该结论不替代用户审核。
- 目标：新增唯一 Agent Host service，并由既有命名 `praxis-agent` CLI 作为薄入口，把 AgentDefinition、Assembler、Harness、Runtime、Application 与 6+1 public factories 组合为一个可启动、可检查、可停止的 Agent。
- 基线：[Composition Root V1](composition-root-v1.md) 与 [H4 Lifecycle V2](h4-production-lifecycle-v2.md)。

当前 `HostV2` 是 injectable reference coordinator，仓库没有非测试 `NewHostV2` 调用、`agent-host/cmd`、daemon/API 或 production root。本设计不得把现有测试组合改名为生产入口。

## 2. 两层声明

### 2.1 AgentDefinition

用户声明 Agent 语义：Identity/Profile、至少七个核心 required components、Policy/Secret refs、版本/能力/locality、effective window 与 extensions；Catalog可注册额外production components。它不含 DSN、Go symbol、raw provider URL、secret value 或动态代码路径。

### 2.2 BootstrapConfig

部署者声明 Host 资源绑定：

- HostID、State Plane binding IDs；
- Definition source、Catalog、Resolution Facts current IDs；
- Secret Broker、Credential/Provider endpoint registry IDs；
- Runtime/Application/Harness service binding IDs；
- listen、diagnostics、shutdown policy refs；
- enabled control API surface。

Bootstrap 只能引用 build/deployment-time 注册的 typed factories/readers；禁止把字符串反射为 constructor、package、shell、plugin 或 raw Provider。

新增严格`HostBootstrapConfigV1`公共合同：固定ContractVersion/ObjectKind、stable HostID、上述binding IDs、Created/NotAfter与ContentDigest；禁止自由map。Deployment/Bootstrap Owner读取该声明，打开资源并发布`HostDeploymentCurrentV1`，其exact Ref绑定Bootstrap digest、全部typed ResourceHandle/ServiceBinding refs、Checked/Expires和ProjectionDigest。

现有`HostConfigV1/StartRequestV2`不含该exact deployment current，不能原地扩大。Production facade使用additive Host Service Contract V3：`StartRequestV3`绑定`HostDeploymentCurrentRefV1 + HostConfigV1 + DefinitionSourceCurrent`；HostV2继续是reference coordinator。

V3不得创建平行Claim事实或第二仓。它复用live version-neutral、永久冲突域`HostStartClaimV1`的同一`HostID+StartID` key/store，并做additive扩展：

- 新`HostStartClaimInputV3`平铺保存StartRequestV3中的HostDeploymentCurrent exact ref、HostConfig digest、DefinitionSourceRef、RequestedOperation、Created/Expires并有独立canonical digest；
- `HostStartClaimV1.ConfigDigest`在V3严格等于该InputV3 digest，`DefinitionSourceRef`逐字段相等；现有Claim V1 canonical/digest不改，只把其`HostContractVersion`closed validation additive接受V3；
- 同一Claim Owner/Store提供`ClaimOrInspectHostStartV3`，在一个事务/线性点create-once既有ClaimV1与`HostStartClaimInputBindingV3` sidecar；sidecar绑定Claim exact Ref与完整InputV3，供lost reply/restart Inspect，不能独立授Start；
- V1/V2/V3双向使用同一key和永久Conflict；Claim存在但sidecar缺失或漂移时V3为indeterminate并只Inspect/repair同一事务语义，禁止创建新Claim或换StartID盲重派。

## 3. 唯一进程入口

候选落点：`ExecutionRuntime/agent-host/cmd/praxis-agent`。它只创建唯一 Host service/composition root；CLI 本身不获得领域写权限。

对外最小命令/API：

```text
validate <definition>
assemble <definition-current>
run --definition <agent.yaml|json> --bootstrap <host.yaml|json>
inspect <host-id> <start-id>
stop <host-id> <start-id>
```

命令语义固定如下：

- `validate`只对本地Definition做strict decode、schema/semantic Validate与报告，零持久写、零Owner operation、零Provider；
- `assemble`只接受已经发布的exact DefinitionSourceCurrent，调用Assembler production StartOrInspect/Inspect operation并持久化ResolvedPlan；它是显式配置/解析Effect，但不占Host lifecycle Claim、不构造组件、不调用Provider；
- `run`是唯一启动命令；它可以先通过Definition Owner create-once发布本地文件为配置事实，然后Seal `StartRequestV3`。进入Host lifecycle后，第一项写必须是shared HostStart Claim，随后才允许Definition exact read、Assembler/Compiler lifecycle operation；
- `inspect`、`stop`只经同一Host Service V3；不再暴露独立`start`命令或第二套启动语义。

简单配置路径中，Root先调用Definition Owner创建/发布definition source current，再读取exact current并Seal`StartRequestV3`。Definition publication是独立配置事实，不属于Host lifecycle；Resolve/Compile不得在HostStart Claim前预跑。`Host.StartV3`内部第一步必须占用shared Claim，随后才调用Definition/Assembler/Compiler owner operations。

Definition/Start/Stop的digest、时间、StartClaim与CleanupClosure refs由对应Owner service读取/Seal；用户不手写摘要、revision、内部AttemptID或CleanupPlanRef。面向自动化控制面的低层API可接受exact refs，但必须经过同一Reader与Host facade，不能成为CLI旁路。

Production Host Service还需additive `InspectRequestV3/InspectResultV3`和`StopRequestV3/StopResultV3`：所有request有Seal/Digest/TTL；Inspect返回exact StartClaim、Journal、Ready/Availability与CleanupClosure current；Stop由service按HostID+StartID读取这些refs并构造exact request。现有V2 Inspect缺少完整digest/Ready语义，只保留reference兼容。

## 4. 注入闭包

### 4.1 Durable State Plane

- Definition、ResolvedPlan；
- Host StartClaim/Journal/SystemReady/Operation/CleanupClosure/CleanupAttempt；
- Runtime Command/Desired/Outbox/Identity/Activation/Run/Effect/Evidence/Settlement；
- Application coordination；
- Harness Session/Event/Assembly/Route/Gate；
- 各组件 owner facts/current。

所有生产写口必须有单一Owner；Memory/fake/testkit不进入root。Deployment/Bootstrap Owner负责open/migrate/recover/health并发布ResourceHandle/Deployment current；Root只接收typed handles和read-only health/current，不自行打开或迁移Store。

### 4.2 Readers 与 publishers

- Approval、DefinitionSource、ResolutionInputs；
- persistent ResolutionFacts 与 ComponentReleaseCatalog；
- 七个组件 release/readiness/current publisher；
- Runtime/Application/Harness/Sandbox exact current Readers；
- credential、authority、policy、budget、provider、deployment、cleanup、certification。

### 4.3 Executable Factory Registry

每个factory key精确绑定ComponentID、artifact、contract、capability、Binding与Generation。FactoryDescriptor只是声明；root必须注册真实typed constructor，并验证no-raw-provider-bypass、zero-effect construction、resource handles和cleanup contract。注册闭包还包括namespaced kind/capability catalog、schema trust、release/current、deployment/certification与cleanup adapter；只注册Factory不足以接入自定义组件。

自定义组件只通过build/deployment-time注册完整public闭包接入；Host核心不增加kind switch。

## 5. 启动、恢复与退出

```text
deployment/bootstrap Owner opens durable stores and prebound resources
  -> publishes exact ResourceHandle/Deployment currents
  -> Root only receives typed handles/readers
  -> load registered factories/readers
  -> open Host control API/IPC (agent traffic disabled)
  -> optional Definition create/publish as configuration fact
  -> Seal production StartRequestV3
  -> HostStart claim
  -> Definition exact read
  -> resolve/persist Plan
  -> compile/publish Harness assembly
  -> Binding
  -> CleanupClosure
  -> construct zero-effect control adapters
  -> Activation V2
  -> Generation association
  -> SystemReady
  -> enable agent run surfaces
```

打开数据库、socket或其他资源是deployment/bootstrap Owner的显式部署动作，不得藏在组件Factory或Host Start中。Root只能消费已经发布current的typed handle；关闭时也只能调用Deployment Owner的typed Shutdown/Inspect port，不得直接close、迁移或替换底层handle。领域cleanup仍由Owner链完成。Host control API必须在Agent Ready前可用，以便Start、Inspect和恢复；Provider/业务Run surface只能在SystemReady后启用。

重启必须先从Host Journal、Owner facts与CleanupClosure Inspect恢复，不根据进程对象重派。收到第一次shutdown signal后，Host Supervisor持久记录draining intent、拒绝新Start/Run但保持control API可Inspect：Closure已result_recorded时进入partial/normal Stop；Closure之前只允许Inspect/取消尚未dispatch的Host intent，并调用Deployment Owner Shutdown/Inspect回收bootstrap resources，Host保持indeterminate而不伪Closed。第二次signal只请求加速Inspect/Reconcile，不能绕过Cleanup、Fence或Owner settlement。正常Stop与Deployment shutdown均终态时进程退出码为0；配置/合同错误在任何Owner写前退出码为2；indeterminate、cleanup unknown或外部强制中断退出码为75，重启后仍按持久事实恢复。不得用本地超时把unknown升级为Closed。

## 6. Execution profile 与版本策略

`AgentDefinitionV1`继续严格要求至少七个核心组件都是`production`，不得静默放宽。

若要先交付本机可用整体，应另行设计additive `governed_local` nominal version：

- 只允许单机 SQLite、显式 provider allowlist、无 HA/SLA claim；
- 每个组件仍须 durable Owner facts、typed factory、cleanup、actual-point 和 conformance；
- 不允许 reference_only、fake、testkit 或 raw provider；
- 不能升级为 production 或与 V1 同 StartID 双写。

是否新增该版本由用户单独审核。本候选的Bootstrap和production Start中没有自由`execution_profile`字符串，也不预先修改AgentDefinition V1。

## 7. 当前 NO-GO 与系统反例

当前缺失：非测试Approval/Resolution/Catalog owners、HostV2/V3 stage input assembler、Definition/Assembler/Compiler production operations、真实Activation V2、CleanupClosure、全部required production release/current/factories，以及CLI/root。

必须覆盖：

- 配置字符串不能触发任意 constructor/provider；
- 缺/过期/drift release/current/factory Fail Closed；
- Definition/Bootstrap secret value 拒绝；
- duplicate/alias capability/factory/provider 拒绝；
- lost create/start/activation/stop 只 Inspect；
- 64 个 root 共享 State Plane 只产生一个 Start；
- restart 后不重复 external Effect；
- SIGTERM 中 unknown cleanup 不伪 closed；
- custom component 只能经 namespaced public registration；
- V1 production 与未来 local profile 不发生同 ID 双活。
- Claim必须先于Definition/Assembler/Compiler生命周期Owner调用；
- Inspect request digest/Ready/Closure splice与Closure-derived Stop；
- bootstrap migration/open unknown由Deployment Owner恢复，Root调用计数为零；
- production V3与reference V2双向同ID竞争；
- V3 Claim的DeploymentCurrent/Input sidecar与既有ClaimV1同事务lost-reply/partial-write/第二仓拒绝；
- composition只注入read-only Plan/Definition/Catalog能力，不把Ensure/CAS写口泄漏给Host coordinator。
- `validate`零写，`assemble`只产生显式Definition/Plan配置事实且Provider调用为零，`run`进入唯一StartV3；
- 第一次/第二次signal、Closure前/后、cleanup unknown与退出码0/2/75矩阵。

## 8. 完成定义

只有一份真实YAML/JSON能通过非测试入口完成Definition -> Claim -> Plan -> Assembly -> Binding -> Closure -> Activation -> Ready，并能在重启后Inspect/Stop，且全链无fake/internal/testkit，才能标记declarative Agent SYSTEM_READY。CLI `run`映射同一production Host `StartV3`，不另建启动语义。

## 9. Import DAG 与能力最小化

Composition package只导入各Owner public contract/ports和具体bootstrap adapters；Owner组件不得反向importcomposition。Host Definition/Assembler/Compiler adapters只接收read-only窄接口，例如`ResolvedAgentPlanExactReaderV1`，不得持有同时暴露Ensure/CAS的Repository能力。若现有公共Port只有读写合并接口，必须先由Owner发布additive窄Reader，不能在Root用约定自限替代类型约束。
