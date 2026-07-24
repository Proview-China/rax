# Memory + Knowledge模块说明

> 实现状态：**backend-neutral component framework + cross-owner reference integration + production closure verifier implementation_software_test_yes**。Owner-local与reference integration为P0/P1/P2=0；新增non-HA/HA双Profile生产证明、四个durable Resource exact Inspect、外部Owner current关联、S1/S2和Release readiness适配。实际资源与独立Certification未注入前，deployment production仍NO-GO。

## 1. 作用与Owner边界

本模块提供Memory与Knowledge两个相互隔离的领域Owner。Memory拥有Candidate/Admission、Record/Correction/Tombstone、View/Watermark及其正式Commit；Knowledge拥有Source/Package、Record/Snapshot/Pointer、Withdraw、License/Trust/Conflict及其正式Commit。Retrieval Result只是持久Observation，不能直接成为Memory/Knowledge正式事实或Context Frame。

## 2. 当前组成

| 实现位置 | 作用 |
|---|---|
| `ExecutionRuntime/memory-knowledge/contract` | Owner-neutral Ref、canonical digest、Citation/Coverage、DomainResultAssociation与opaque Runtime Settlement ref |
| `ExecutionRuntime/memory-knowledge/memory` | Memory Owner领域store、CAS、current projection与Settlement Apply |
| `ExecutionRuntime/memory-knowledge/knowledge` | Knowledge Owner领域store、CAS、current Snapshot/Pointer与Settlement Apply |
| `ExecutionRuntime/memory-knowledge/memory/contextsource` | Memory Owner-local `InspectAttempt/InspectForTurn/ReadContentExact` |
| `ExecutionRuntime/memory-knowledge/knowledge/contextsource` | Knowledge Owner-local `InspectAttempt/InspectForTurn/ReadContentExact` |
| `ExecutionRuntime/memory-knowledge/reference` | 线程安全内存reference store，仅用于reference实现与测试 |
| `ExecutionRuntime/memory-knowledge/retrieval` | 本地确定性检索与Citation/Coverage，不是远程Gateway |
| `ExecutionRuntime/memory-knowledge/projection/{skill,lexical,vector,graph}` | 四类可替换Projection与确定性reference backend |
| `ExecutionRuntime/memory-knowledge/consolidation` | 已Settlement Continuity输入到Memory Proposal；不直接写正式事实 |
| `ExecutionRuntime/memory-knowledge/knowledge/sync*.go` | Knowledge阶段Journal与Settlement-gated两阶段Snapshot发布 |
| `ExecutionRuntime/memory-knowledge/{sdk,api,cli,cmd}` | 公共Go SDK、严格reference HTTP handler与CLI命令层 |
| `ExecutionRuntime/memory-knowledge/{conformance,observability}` | 扩展实现一致性门与diagnostic metrics schema |
| `ExecutionRuntime/memory-knowledge/production` | non-HA/HA生产证明Bundle、Resource Binding/Handle exact复读与Release readiness适配 |

## 3. Current Reader闭环

两个Reader只读本Owner本地State Plane。`InspectAttempt`在Owner RLock内读取fresh clock，精确绑定原Attempt、Request、Idempotency、Execution Scope、Run/Turn、Observation与Result，在Inspection回传Run/Turn，并区分ID不存在与同ID exact ref漂移；`InspectForTurn`同样在Owner锁域内读取fresh clock；`ReadContentExact`在同一RLock执行S1→Get→S2双fresh clock、双binding/current/closure复读，通过后才返回bytes。

ContractVersion/ObjectKind、RunID/TurnID、Owner-local StatePlaneBinding exact ref与TTL全部进入canonical Projection/Closure/Observation。Items按Score降序、Record ID升序、Revision降序、Digest升序派生Rank；duplicate semantic Record及任一无效nested Source/Evidence/Projection ref均拒绝。StatePlane reader接口封闭，两个Owner各自提供零网络内存参考store。

## 4. Delta 10/11 Owner V2与cross-owner reference接线

- V1与V2 Reader均已在各自Owner package落地；V2是同一`context-source-current-reader`合同族的加法实现，复用各自唯一Store/Journal/current，不建立第二DTO、public facade、Store或current；
- Memory与Knowledge保持独立public nominal、canonical discriminator与Reader；V2已实现Identity/Session/SourceTurn exact coordinate、stable/fresh Projection、AssociationDigest、bounded local content和ctx-aware可取消锁；
- V2 `ReadContentExact`在同一Owner一致性锁域执行S1 current/binding→bounded `GetExact`→S2 fresh current/binding/stable closure，任何cancel、TTL、eviction、poison、binding/current漂移均返回零Observation与nil body；
- exact Turn等式为`SourceTurnRef/Ordinal == Tool.Execution.Turn == ExpectedCurrent.Turn`；V2中的`SourceTurnRef`是`contract.Ref`，由Harness committed PendingAction current经public Adapter无损传递，不在Ref上伪造字段。Memory/Knowledge只验证/回显，任何Owner不得从uint32或字符串补ID/revision/digest；
- Context是TransitionProof唯一Owner；Application只协调pre-frame request。固定链为`Owner S1 -> Context pending outputs seal -> Context final proof seal -> Owner S2 -> Context atomic ApplySettlement+Generation CAS -> publish`；Target/ContextTurn=T+1及childExecution/new Frame/Generation只进入Context final proof；
- stable ClosureDigest排除`AttemptInspectionRef`、CheckPhase、OwnerCheckedAt、ExpiresAt与Projection self ref；fresh Projection保留Inspection ref并包含phase/time/expiry/closure。重新Inspect可改变fresh ref/digest，但S1/S2 stable closure和ordered exact集合必须相同；
- Context Owner已发布加法kind `knowledge_reference`；Knowledge只提供exact refs，禁止创建Context Fact或冒充`memory_recall`、Artifact、Instruction；
- Application只协调exact refs与Attempt；Harness只消费Context Owner已经发布并Inspect为current的exact Frame，不调用Owner Reader或接收Owner body；
- 详细字段与门禁：[Memory冻结设计](../../design/memory-engine/context-refresh-neutral-delta10-11-v1.md)、[Knowledge冻结设计](../../design/knowledge-engine/context-refresh-neutral-delta10-11-v1.md)、[V2冻结合同](../../plan/memory-knowledge/context-source-reader-v2-frozen.md)、[联合Port Delta](../../plan/memory-knowledge/port-delta.md)。

原External reference P0五项已经闭合：Harness exact Turn无损映射、Context-owned TransitionProof、Application三阶段Port、双Owner Adapter/nonzero fixture及`knowledge_reference` exact chain。production root仍不得把reference fixture当作部署授权。

## 5. Fail Closed与能力边界

- `now >= ExpiresAt`、historical state、Watermark/Pointer/Projection漂移、Tombstone/Withdraw、Knowledge License漂移、poisoning未清除、错误DomainResultAssociation均不可current；
- canonical payload tamper、本地正文evicted或bytes摘要不一致均拒绝；
- caller旧CheckedAt不能授current；clock rollback、锁等待/Get跨TTL、Get期间binding revision/RemoteLocator漂移均Fail Closed且不返回body；
- Reader没有`Retrieve`、Provider、Resolver、网络或远程正文路径；
- Harness/Application/Context reference Adapter链已存在；没有production composition root，也没有远程Gateway；
- reference fixture为`MemorySources=1`、`KnowledgeSources=1`；production root未装配时生产来源数仍为0；
- 内存store不是生产Backend、持久State Plane或SLA承诺。

## 6. 验证

2026-07-17 当前框架验证实际通过（最终全门结果以最新memory事件为准）：

```bash
cd ExecutionRuntime/memory-knowledge
go test ./memory/contextsource ./knowledge/contextsource ./tests -run 'V2|ContextSourceV2PublicNominals' -count=100
go test -race ./memory/contextsource ./knowledge/contextsource ./tests -run 'V2|ContextSourceV2PublicNominals' -count=20
go test ./...
go test -race ./...
go vet ./...
go test -count=100 ./...
go test -race -count=20 ./...
```

Cancel/TTL/Evict/Poison/Binding/LostReply与cross-owner S1/S2/proof/atomic CAS关键组另以ordinary100/race20重复执行并全部通过；Application、Context与Harness exact Turn定向组通过，`gofmt -d`、diff-check、import-boundary与零网络扫描通过。验证包含reference integration，不包含production root或真实远程Effect。

Component Release/readiness V1直接产出Assembler公共`ComponentReleaseV1`。Production closure V2 verifier软件侧已闭合：`non_ha`强制单写者Fence、恢复与备份恢复证明且禁止HA声明；`ha`额外强制ReplicaCount >= 3、多数写Quorum、复制/仲裁/故障转移/单调current证明。四个durable Resource Handle经Runtime公开Reader逐项exact Inspect，外部Authority/Policy/Credential、Index/Context、Settlement/Purge/Cleanup、Deployment/Certification只保存opaque exact Ref。

该结论不等于某一环境已经生产部署。实际Owner资源和独立Certification未注入，Assembler/Host production root尚未发布`SupportProductionV1`，因此deployment production继续NO-GO。
