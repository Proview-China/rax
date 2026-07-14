# Praxis Runtime V1执行治理核心计划

## 1. 状态与替换声明

- 状态：已通过独立文件复审、具备正式用户审核条件的V1候选；Plan尚未获批，12项决策仍待用户确认；
- 实现授权：无；
- 取代：本文件完全取代2026-07-14早期Runtime V1草稿、首次文件复审前候选及二次复审修正前候选；旧草稿中的单线状态、循环Activation顺序、Binding事务/原子回滚、Harness自报Ready、ActionPort局部Effect和技术/目录选型提案均不再有效。

## 2. 目标

构建一个实现中立、可通过fake/stub独立验证的Runtime V1：它按无循环协议创建proposed Lineage/Instance，以稳定proposed ID执行Bounded Preflight，再冻结ActivationSnapshot、依次取得reserved Identity Lease、适用Budget Reservation与reserved_quarantined SandboxLease，并以ActivationCommit授予一般执行权；随后执行Binding Saga、监督Harness、线性化命令、Fence旧执行面、治理全部外部Effect、保存权威Intent和证据、真实表达Unknown/Remote状态并确定性协调清理。

Runtime V1不等待相邻引擎完成内部实现，但所有相邻能力必须通过版本化Port和合同测试接入。

## 3. 设计依据

- [总体架构](../../design/runtime/architecture.md)
- [对象与所有权](../../design/runtime/concepts/README.md)
- [三维状态与Kernel](../../design/runtime/kernel/README.md)
- [命令与CAP](../../design/runtime/control-plane/README.md)
- [Admission与Saga](../../design/runtime/admission/README.md)
- [Fence与替换](../../design/runtime/safety/README.md)
- [统一Effect与远程状态](../../design/runtime/effects/README.md)
- [Evidence](../../design/runtime/evidence/README.md)
- [Checkpoint与Cache](../../design/runtime/continuity/README.md)
- [Port与Harness](../../design/runtime/contracts/README.md)
- [扩展供应链](../../design/runtime/extensions/README.md)
- [Application Facade](../../design/runtime/interfaces/README.md)
- [Profile交接](../../design/runtime/profile-assembly/README.md)
- [Sandbox](../../design/runtime/sandbox/README.md)
- [反例矩阵](../../design/runtime/scenarios/README.md)
- [设计门禁](../../design/runtime/acceptance/README.md)

## 4. V1范围

### 4.1 必须产出

1. AgentIdentity引用、IdentityExecutionLease、InstanceLineage、AgentInstance/epoch、SandboxLease和单活跃Run合同，包含proposed/reserved/quarantined/active/aborted状态；
2. LifecyclePhase、ExecutionCertainty、CleanupStatus三维状态的完整合法谓词、迁移前置条件、失败落点、迟到隔离、受限恢复Effect白名单和certainty收敛验证；
3. Command Log、Desired State、revision、线性化、安全支配和CAP fail-closed；
4. Static Admission、Bounded Preflight、proposed身份、ActivationSnapshot、两类reserved Lease、ActivationCommit和live fact漂移处理；
5. Binding Saga、CompensationIntent、unknown effect和残留协调；
6. ExecutionFence、RevocationPolicy、Conflict Effect Domain和ReplacementPermit；
7. Harness共用合同、四级Conformance和至少两种不同fake Harness合同样例；
8. 统一`effect_intent_id/revision`、Authorization、Receipt、UnknownOutcome和幂等分级；ActionRef仅是可选子类型引用；
9. BudgetAuthorityPort能力、fake原子Reservation事实源和Hard Budget可执行有限cap验证；
10. RemoteContinuation、ProviderOperation、Cancel/Retention/Close和分维度终止报告；
11. Artifact/Memory Candidate、Review引用、Commit Intent和迟到提交拒绝；
12. Source身份、Provenance/Observation/Attestation/Authoritative Fact、Write-ahead Evidence、Outbox和安全Projection；
13. Checkpoint/Restore统一Effect Intent/Dispatch/Settlement/Remote水位合同（实现可在用户审核后选择unsupported或最小一致实现）；
14. Cache Partition、本地读取证据、父模型Intent内Provider命中、独立远程Cache Effect与污染拒绝合同；
15. Extension Manifest、Trust、供应链、资源限制和合同测试框架；
16. Transport-neutral Application Facade资源、目标、epoch/revision前置条件、幂等范围、命令、查询、Operation、Watch和稳定TypedError合同；
17. fake Sandbox、Harness、Effect、Budget、Evidence、Authority和状态端口；
18. 反例矩阵对应的自动化验证、中文模块说明和项目状态同步。

### 4.2 明确不做

- 不实现相邻Context、Tool、Memory、Review、Budget价格或Sandbox后端内部算法；
- 不新增未经用户确认的独立模块；Budget只作为必要能力Port；
- 不选择具体语言、代码根、数据库、RPC、进程拓扑或生产后端；
- 不接入真实账号、凭据或生产外部系统；
- 不支持Identity多活跃Lineage、并发Run、跨Run Session或集群副本；
- 不支持透明热迁移、跨租户缓存共享或Exactly Once；
- 不自动补偿不可逆效果；
- 不承诺生产SLA、容量或成本精度。

## 5. 可预见产物

### 公共合同

- 版本化对象、状态、命令、错误、Effect、Evidence、Checkpoint、Cache和RemoteContinuation Schema；
- Port、Capability、Conformance、Risk和Failure语义；
- Application Facade的Transport-neutral合同。

### Runtime核心行为

- Admission/Activation；
- Lineage/Instance Registry和Identity Lease协调；
- 单Instance Reconcile；
- Binding Saga与Compensation协调；
- Command线性化和Desired State；
- Fence、替换和三维状态；
- Effect、Budget、Evidence和Remote状态协调。

### 验证基础

- fake/stub端口和确定性测试夹具；
- Provider/Harness/Extension合同测试工具；
- 反例矩阵自动化映射；
- 模块说明、诊断和测试证据。

### 用户可见结果

在不接真实Provider的条件下，用户能够通过获批的一个入口完成：解析计划、查看风险和Residual、观察proposed/reserved/quarantined/active激活过程、执行单Run、观察事件、审批/撤销、处理UnknownOutcome、停止并查看本地/远程/清理分维度报告。具体入口和语言由用户审核决定，默认不实施。

## 6. 阶段计划

### 阶段0：复审和用户决策

- 独立线程逐文件复审文本、图、矩阵和本Plan；
- 修正所有P0/P1冲突；
- 用户确认未决选项；
- 冻结V1 Schema版本和设计门禁。

完成条件：设计复审通过，用户明确批准Plan和实现边界。

### 阶段1：公共对象、所有权与失败模型

- 实现Identity/Lineage/Instance/Lease/Run引用、proposed/reserved/active状态与摘要；
- 实现每个Effect一个Intent所有者和一个Settlement归并所有者、Review Verdict与Commit所有者拆分；
- 实现三维状态合法谓词、迁移前置条件/触发/失败落点和迟到事件隔离；
- 实现`unknown/lost/fenced`下Inspect、原Intent结算、Emergency Safety、受权Cleanup/Release和独立授权Compensation的白名单判定；禁止对Unknown原效果猜测补偿；
- 实现权威Inspect覆盖范围验证与`unknown/lost→confirmed`原地收敛；terminal只更新certainty/settlement而不复活Lifecycle，fenced旧Instance不可恢复运行；
- 实现Fence、Risk、Secret类型和ReplacementPermit对象；
- 实现Typed Error、迟到结果和Residual；
- 建立Schema round-trip、golden、非法输入和fuzz测试。

完成条件：相同输入确定性产生相同摘要和验证结果；循环Schema、双主所有者和非法状态组合被拒绝；旧epoch不能改变新对象；恢复白名单拒绝越权Effect，同时不会把合法Cleanup/Release永久锁死。

### 阶段2：线性化控制与权威写前记录

- 实现抽象Command/Desired State/Identity Lease事实Port；
- 实现revision、CAS、命令支配和Superseded/Invalidated；
- 实现Transport-neutral Target ResourceRef、Identity/Lineage/Instance/Lease/Authority epoch前置条件和幂等范围；
- 实现权威Intent与Outbox合同；
- 实现分区stale read和推进fail-closed；
- 实现Emergency Safety与Evidence Gap。

完成条件：双主、证据不可用和命令竞态反例全部关闭。

### 阶段3：Admission、Activation与Binding Saga

- 实现纯Static Admission；
- 实现proposed Lineage/Instance身份预留，二者不授予执行权；
- 实现关联proposed Instance的声明式Probe Budget和Preflight记录，未知/残留不得进入Snapshot；
- 实现不含真实Lease ID的ActivationSnapshot与live fact重验；
- 实现reserved IdentityExecutionLease和reserved_quarantined SandboxLease；
- 实现Snapshot只冻结Budget Policy/requested cap，之后再取得真实Budget Reservation；
- 实现最终重验与ActivationCommit逻辑提交，再激活SandboxLease；
- 实现每个中断点的abandon/abort、Inspect、Fence、Release和Unknown Quarantine恢复；
- 实现Binding DAG、Step Intent、Inspect和Saga状态；
- 实现CompensationIntent、unknown effect和残留协调。

完成条件：对象建立图无循环；Commit前Sandbox不能启动Harness；所有强制失败都落入可恢复状态；Required unknown effect不能Ready。

### 阶段4：Sandbox、Harness与Kernel

- 实现fake SandboxLease requested/reserved_quarantined/active/release状态和隔离Attestation；
- 实现两种内部机制不同的fake Harness；
- 实现Conformance、Ready独立验证和单活跃Run；
- 实现Kernel Reconcile、Stop、Lost/Fenced和分维度终止；
- 实现ReplacementPermit与Quarantined新实例。

完成条件：Harness自报、节点失联、旧Token、Secret和替换反例全部关闭。

### 阶段5：统一Effect、Budget与远程状态

- 实现覆盖所有类别的`effect_intent_id/revision`、Authorization、Fence和派发状态；
- 实现幂等等级、Receipt、UnknownOutcome和Reconcile；
- 实现fake BudgetAuthorityPort的原子Reserve/Commit/Release/Reconcile及有限cap验证；
- 实现本地Cache读证据、父模型Intent内Provider命中和独立远程Cache Effect三种路径；
- 实现RemoteContinuation、Cancel、Retention和Cleanup关联；
- 实现Artifact/Memory Candidate与Commit协调。

完成条件：模型外发、Hosted Tool、预算超卖、重复派发、Batch残留和迟到Commit反例关闭。

### 阶段6：Evidence、Checkpoint、Cache与扩展

- 实现Source身份、至少一次、重复去重、Source sequence Gap和Command逻辑提交破裂恢复；
- 实现Restricted Evidence、Projection、Redaction和Retention合同；
- 实现Checkpoint的Effect Intent Accept/Dispatch/Settlement/Remote Barrier/Watermark合同或明确unsupported；
- 实现Cache Partition与Provider Cache Effect验证；
- 实现Extension Trust、Digest、TTL、撤销、资源限制和Observer权限。

完成条件：伪报Ready、Source伪造、Secret Payload、半恢复、缓存污染和恶意扩展反例关闭。

### 阶段7：Application Facade与获批入口

- 实现Transport-neutral资源、Command/Query/Operation/Watch；
- 验证REPL/API/SDK入口对相同请求产生相同领域结果；
- 只实现用户明确批准的首个入口和语言；
- 未批准Remote API或SDK时只交付合同与fake客户端。

完成条件：入口不泄漏后端、不拥有领域语义，分区和旧epoch结果一致。

### 阶段8：加固、说明与验收

- 运行单元、白盒、黑盒、Provider合同、故障注入、并发/race、fuzz、安全和集成测试；
- 建立Admission、Reconcile、Command、Effect、Ledger和Saga基准；
- 对照反例矩阵逐ID关闭；
- 在用户批准的模块说明落点生成中文入口，并同步测试证据、memory和properties；具体目录在用户决策前不创建；
- 未获单独授权不接真实Sandbox或Provider。

完成条件：强制矩阵全部有证据，无未解释高风险Residual。

## 7. 细粒度检查清单

### 身份与状态

- [ ] IdentityExecutionLease单活跃保证；
- [ ] proposed→reserved→ActivationCommit→active无循环顺序和逐步恢复；
- [ ] Lineage固定Plan Digest；
- [ ] 重建新Instance ID/epoch；
- [ ] V1单活跃Run；
- [ ] 三维状态和分维度终止；
- [ ] 合法组合谓词、迁移前置条件、失败落点与迟到隔离；
- [ ] Execution不等于Task/Artifact成功。

### 安全与控制

- [ ] Fence覆盖全部最终效果边界；
- [ ] 非Action Effect同样使用统一Effect Intent ID/revision；
- [ ] 离线策略缺失即禁用；
- [ ] 长期明文Secret拒绝受治理Harness；
- [ ] Command线性化和安全支配；
- [ ] 分区推进fail closed；
- [ ] ReplacementPermit阻止冲突能力。

### Effect与证据

- [ ] 模型、网络、资源、Tool、Hosted Tool、Provider Cache、Credential和Commit全部进入Effect；本地纯查找按CacheAccessEvidence合同处理；
- [ ] Intent-before-dispatch；
- [ ] UnknownOutcome不盲重试；
- [ ] Budget原子Reservation；
- [ ] Hard Budget只有存在可执行有限cap时成立；
- [ ] RemoteContinuation独立清理；
- [ ] Harness自报不成为权威事实；
- [ ] Evidence不可用时阻断新Effect；
- [ ] Restricted Evidence和安全Projection分离。

### 扩展与入口

- [ ] Trust Root、Publisher、Digest、SBOM、TTL和撤销；
- [ ] Observer无权威Append；
- [ ] Provider专属字段命名空间化；
- [ ] Application Facade不吞并领域；
- [ ] Command target、epoch/revision前置条件、幂等范围和TypedError跨Transport一致；
- [ ] 入口跨Transport行为一致。

## 8. 测试矩阵

| 测试层 | 必测内容 |
|---|---|
| 单元 | 循环Schema拒绝、摘要、状态谓词/迁移、命令支配、Fence、Budget cap、Cache Key |
| 合同 | Sandbox、两类Harness、Effect、Budget、Evidence、Extension |
| 白盒 | Reconcile幂等、Saga、epoch隔离、Outbox、未知结果、迟到Receipt |
| 黑盒 | Plan到分维度终止，客户端只观察公共合同 |
| 故障注入 | 每个Activation步骤和外部调用前后、响应丢失、Close失败、Evidence故障、分区 |
| 并发/race | 双主、Identity Lease、命令竞态、预算超卖、Watch重连 |
| fuzz | 非法/循环Plan、扩展Schema、事件重复/缺口、DAG、三维状态和Effect帧 |
| 安全 | Authority撤销、Secret暴露、网络越界、Source伪造、缓存污染 |
| 集成 | fake全链；真实后端仅经独立授权 |
| 性能 | Admission、Reconcile、Command线性化、Effect/Ledger、Saga回放 |

具体覆盖率、延迟和容量指标等待用户基于实现技术和风险确认，当前不写死。

## 9. 回退与停止条件

- 任一实现发现所有权或概念合同冲突，停止并返回设计；
- Provider无法满足Fence、Evidence或UnknownOutcome合同，不修改Runtime伪装兼容；
- 真实后端未获批准，保持fake/stub；
- Authority扩大、Route/Harness变化产生新Plan/Lineage，不热改活跃实例；
- 检测到Secret泄漏、权限扩大、Source伪造、孤儿Effect或审计缺口时阻断下一阶段；
- 技术选项未确认时默认不执行。

## 10. 依赖与风险

| 风险 | 控制 |
|---|---|
| 相邻引擎未完成 | fake Port与合同测试，不复制内部算法 |
| Provider最低公分母 | 公共Capability + 命名空间扩展 + Residual |
| 分布式不确定性 | Fence、线性化事实、UnknownOutcome和分维度状态 |
| 第三方扩展恶意 | Trust、隔离、资源限制、撤销和证据TTL |
| 预算/价格漂移 | Budget Authority Port与Activation重验 |
| Cache节省污染权限 | 强Partition、命中重鉴权、敏感Route禁用 |
| 远程状态残留 | RemoteContinuation与Retention报告 |
| API成为万能层 | Facade路由，不改变领域所有权 |

## 11. 用户审核问题

这些选择没有安全默认值；未确认时对应能力保持不实现或禁用：

1. Runtime代码位置与技术语言；
2. 核心、Facade、REPL/API/SDK的进程与Transport边界；
3. 首个用户入口及语言；
4. 首个真实Sandbox Provider；未确认时只做fake；
5. 权威Command/Lease/Intent事实存储的实现；
6. 首批Risk Class及每个部署的`max_revocation_lag`、`max_clock_skew`和允许的离线Effect；未确认时离线Effect禁用；
7. V1 Checkpoint是明确unsupported还是实现最小一致版本；
8. Restricted Evidence的保留、删除、加密和Redaction策略；
9. BudgetAuthorityPort由现有哪个责任域承载；
10. 是否在V1交付Remote API和User SDK实现，还是只冻结合同；
11. 是否以及何时进行真实Sandbox/Model Invoker集成；
12. 关键覆盖率、性能和容量验收值。

## 12. 最终完成条件

只有以下全部成立，Runtime V1实施才可标记完成：

1. 设计、图、矩阵和Plan通过独立复审及用户批准；
2. 所有获批范围的公共合同和fake端口可运行；
3. 反例矩阵每个ID有自动化结果；
4. 单元、合同、白盒、黑盒、故障注入、并发、fuzz和安全验证通过；
5. 未知效果、远程残留和证据缺口没有被伪装成成功；
6. 中文模块说明和项目状态同步完成；
7. 未批准能力保持禁用或明确unsupported；
8. 用户完成最终验收。
