# Memory + Knowledge backend-neutral framework

> 状态：**backend-neutral component framework + cross-owner reference integration + production closure verifier implementation_software_test_yes**。Memory与Knowledge领域闭环、V2 Current Reader、检索/治理/SDK及reference集成已闭合；production verifier已支持`non_ha`与`ha`双Profile并复读全部exact资源/current。实际部署资源、外部Owner current与独立Certification未注入前，production deployment仍NO-GO。

这是Memory与Knowledge的独立Go 1.25后端中立框架。实现不预选生产DB、Vector DB、Graph DB、RPC、进程拓扑或SLA；所有reference backend只用于合同与Conformance。

## 包边界

- `contract`：版本化值对象、opaque Ref、Canonical Digest、严格JSON解码、Citation/Coverage、`DomainResultFact`与opaque `RuntimeSettlementRef`；
- `reference`：线程安全的内存content-addressed reference store；它只用于Wave 1/reference测试，不是生产持久Backend或SLA声明；
- `projection/{skill,lexical,vector,graph}`：四类可替换Projection及确定性reference backend；
- `retrieval`：本地lexical与四通道Hybrid/RRF、过滤、Citation、Coverage与绑定Cursor；
- `consolidation`：只消费已Settlement exact输入并产出Memory Proposal，不直接Commit；
- `memory`：Memory Candidate/Admission/Record/Correction/Tombstone/View/Projection的唯一Owner；
- `knowledge`：Knowledge Source/Package/Candidate/Record/Snapshot/View/Projection/Withdraw的唯一Owner；
- `memory/contextsource`：Memory Owner-local Current Reader与版本化本地Attempt/current projection store；
- `knowledge/contextsource`：Knowledge Owner-local Current Reader与版本化本地Attempt/current projection store；
- `production`：不选择后端的non-HA/HA生产证明Bundle、durable Resource exact复读、S1/S2与现有Release readiness适配；
- `sdk`、`api`、`cli`、`cmd/praxis-memory-knowledge`：不直连map的公共开发者入口；
- `internal/testkit`：仅测试使用的确定性Clock和Ref辅助；生产包不得导入它；
- `tests`：只通过公共包执行黑盒与Conformance验证。

## 硬分层

```text
Memory或Knowledge Owner CAS
  -> DomainResultFact(result_ready)
  -> Runtime Operation Settlement opaque ref + exact DomainResultAssociation
  -> Domain ApplySettlement CAS
  -> settled投影
```

本模块不能创建、解释或复制Runtime Operation Settlement；只保存其`ID + Revision + Digest`。ApplySettlement还必须携带组件可验证的`DomainResultAssociation`，并精确匹配当前DomainResult的ID、revision与canonical digest。类型中不存在Runtime Outcome、Binding、Policy、Trust或Disposition字段。`DomainResultFact`存在不代表已经Settlement。

Memory与Knowledge分别使用独立Controller、ID namespace、幂等索引、权威Record和Settlement投影。跨Owner Ref、Candidate或Settlement必须Fail Closed。

## 当前支持

- Candidate exact-idempotency与Admission；
- 显式`expect_absent`/expected revision CAS；
- Memory Record、Correction、Pin、Archive、Forget/Tombstone、Legal Hold门禁和当前View；
- Memory Merge、Decay、Expiry、Export、Watch、Reindex与metadata-only Purge Intent；
- Knowledge Source、Package、Record、不可变Snapshot、Correction、Conflict、Deprecate、Withdraw和当前View；
- Projection引用、版本/水位、TTL/currentness；
- Skill/Lexical/Vector/Graph Projection、Hybrid RRF、Citation、Coverage、Cursor currentness；
- 可回放Consolidation Batch，未Settlement或不可验证自动候选Fail Closed；
- Knowledge Acquire→Parse→Normalize→Validate→Index→Snapshot→Publish Journal及两阶段Sync Controller；
- Sync Prepare停在DomainResult，Runtime Settlement经Owner Apply后才允许Projection/Snapshot/current Publish；
- Go SDK、严格HTTP reference handler及文档列出的CLI命令面；
- 可插拔Retriever、Indexer、Consolidator、Admission Policy、Telemetry与Source Connector的Observation-only Conformance合同；
- 容量、延迟、召回质量、无结果、冲突、陈旧、权限拒绝、Context采用和任务效果的diagnostic metrics schema；
- 原Attempt Inspect、CAS后丢回包恢复、Domain ApplySettlement幂等；
- in-memory reference store及Conformance testkit。
- non-HA单写者/恢复/备份证明与HA复制/多数仲裁/故障转移/单调current证明的严格生产退出验证。

## Owner-local Current Reader

两个Owner各自实现独立的`InspectAttempt`、`InspectForTurn`与`ReadContentExact`。Reader只复读本Owner已经持久化的Attempt、DomainResult association、Settlement application、current state及本地exact bytes；所有持久对象和返回Observation均携带ContractVersion/ObjectKind并重算canonical digest。RunID/TurnID进入Attempt、请求、Projection、Closure与Content Observation，跨Turn replay拒绝。

Store使用expected-revision CAS保存不可变版本，并在返回时深拷贝slice与正文。`InspectAttempt`和`InspectForTurn`均进入Owner一致性锁域后读取fresh owner clock；Inspection携带并exact回传RunID/TurnID。`ReadContentExact`在同一锁域执行S1 fresh clock/current/binding、Get、S2 fresh clock/current/binding/closure复读。锁等待或Get跨TTL、clock rollback、binding revision/remote locator漂移均返回空body并Fail Closed。

LocalContentReader是包内封闭接口，只接受canonical `StatePlaneBinding`且禁止RemoteLocator；两个Owner分别提供无网络`StatePlaneContentStore`参考实现。Binding exact ref与TTL进入Projection、Closure及Content Observation。

Current Reader没有`Retrieve`方法，也没有网络、Provider、Resolver或远程正文能力。reference integration fixture已用两个真实V2 Owner Reader各提供一个本地来源，经Application S1→pending→S2→Context atomic Apply/CAS发布exact Frame；它不等于Context production root。

## 明确unsupported / 外部门

当前框架不提供以下已部署能力；不得用Fake或stub宣称支持：

- 无Run admin Operation和pre-run Evidence；
- 真实Connector、远程Query/Embedding/Graph、外部写入、物理Purge执行、Provider保留或任何自动网络/费用/披露Effect；
- 生产Vector/Graph/Remote Index Backend；
- Context/Application/Harness production composition root与真实per-turn启用；reference三阶段链、双Owner Adapter和`knowledge_reference`已实现并测试；
- 生产API部署、数据库、RPC、进程拓扑和SLA；
- Rust实现。

`production`包只验证并关联外部Owner已经发布的事实：四个durable Resource Handle、Authority/Policy/Credential、Retrieval Index、Context Source、Settlement、Purge/Cleanup、Deployment和Certification。它不创建这些事实，也不把non-HA提升成HA。Application负责协调，Assembler/Host负责绑定真实资源，Context只消费current source，Harness只消费exact Frame/Release。

需要上述能力时必须由对应公共Owner提供已批准Port并进入下一Wave；本模块不得创建私有兼容接口。

## 验证

在本目录运行：

```text
go test ./...
go test ./... -run 'Test.*(WhiteBox|StateMachine|CAS)'
go test ./... -run 'Test.*BlackBox'
go test ./... -run 'Test.*Fault'
go test ./... -run 'Test.*Conformance'
go test -race ./...
go vet ./...
```
