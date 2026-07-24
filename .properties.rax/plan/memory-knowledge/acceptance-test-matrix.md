# Memory + Knowledge v1 验收与测试矩阵

本矩阵同时记录已执行真值与未启用边界。Memory/Knowledge V1/V2 Current Reader与backend-neutral framework已完成软件测试；Harness exact Turn映射、Application三阶段公共Port、Context TransitionProof/`knowledge_reference`、双Owner Adapter、Memory=1/Knowledge=1 reference fixture、atomic Apply+Generation CAS及并发/Unknown/故障组也已YES。Owner-local与cross-owner reference integration均为P0/P1/P2=0。production root与远程Retrieval Gateway仍未启用；Fake/reference只用于测试，不代表生产Backend、容量或SLA。

## 1. 测试层级

| 层级 | Memory重点 | Knowledge重点 | 通过标准 |
|---|---|---|---|
| 单元 | Candidate/Admission/Record状态机、Correction/Forget、View过滤、CAS | Source/Package/Record/Snapshot状态机、Conflict/Withdraw、Publish CAS | 合法迁移完备；非法迁移无写；错误稳定 |
| 合同/白盒 | Canonical Digest、严格解码、Idempotency、Owner Journal、Inspect | 版本/水位、Citation、Coverage、Projection重建、Owner Journal | golden fixture稳定；重复/未知字段Fail Closed |
| 黑盒 | 仅经Memory公共Port提交、查询、纠错、遗忘 | 仅经Knowledge公共Port注册、发布、查询、撤回 | 不导入内部实现；观察结果与Owner Fact一致 |
| 故障注入 | 各治理阶段丢包、CAS冲突、Purge残留、Projection损坏 | Connector超时、Publish丢包、Source撤回传播、Reference不支持 | Unknown只Inspect；Residual/Partial不被吞掉 |
| Conformance | Owner/non-owner、Effect顺序、双Fence、Run Requirement | 同左，另含Asset边界、Citation/License | 违反一项即不通过，不靠集成Stub翻译 |
| Race | 并发Candidate、同Record CAS、View切换、Query/Reindex | 并发Acquire/Publish/Withdraw、Snapshot/Projection切换 | `go test -race`无报告；线性化不变量成立 |
| Vet/Fuzz | 编解码、Digest、Cursor、过滤器 | Parser输出合同、Manifest、Citation、Cursor | `go vet`通过；Fuzz不崩溃且保持上限 |
| 集成 | Runtime Operation/Application/Review/Evidence/Context/Assembler | 同左，另含Asset/Route Reference能力 | 只使用公开Port；精确引用可回读 |
| 系统 | 本地State Plane闭环；远程Retrieval只测unsupported/provider=0 | 本地Source→Snapshot→Query→Context Candidate；远程Resolver只测unsupported/provider=0 | 不外推远程Backend或SLA；专用三版本冻结前不跑远程成功路径 |

## 2. 必测反例

| 编号 | 输入/故障 | 期望 |
|---|---|---|
| N1 | 模型总结直接请求永久写入 | 只产生Candidate；无Record Fact |
| N2 | Provider/Connector返回success但Owner Inspect无Fact | 不提交权威Evidence；保持Unknown/Residual |
| N3 | Review绑定旧Candidate/Payload/Scope或已撤销 | Fail Closed；Begin不得发生 |
| N4 | Permit后Authority/Policy/Fence/Scope/Budget变化 | 执行点Enforcement拒绝；无CAS |
| N5 | Begin后dispatch/execute/CAS回包丢失 | 只Inspect原Attempt，不创建新Attempt |
| N6 | 同Idempotency换Payload | Evidence Conflict；不覆盖原语义 |
| N7 | Expected Revision竞争 | 仅一个CAS成功；输家返回revision conflict |
| N8 | Projection ready但Coverage partial | Result显式Partial，不宣称全量 |
| N9 | ContextReference在目标Route不可物化 | 必需内容Fail Closed；可选内容Residual |
| N10 | Forget成功但远程索引删除超时 | Tombstone有效；Purge Residual保留 |
| N11 | Knowledge来源冲突 | 返回多Claim及Conflict/Citation，不无来源选真 |
| N12 | Source withdrawn但旧Snapshot仍被历史Run引用 | 新View不可见；历史Evidence仍可解释 |
| N13 | Harness私有ContextPort被尝试绑定为公共Port | Assembly/Conformance拒绝 |
| N14 | Component Settlement尝试写Runtime Outcome | 合同拒绝；无Runtime状态变化 |
| N15 | 无Run管理Operation尝试伪造Run Evidence | 拒绝；只允许已批准OperationScope Evidence，且顺序为DomainResultFact→Evidence→Runtime Settlement→ApplySettlement |
| N16 | Provider Cache usage被提交为Knowledge/Memory Fact | 只保留Observation；Admission拒绝事实升级 |
| N17 | RetrievalResult被直接转换为Candidate/Frame | Conformance拒绝；必须先得到Owner Observation/Result exact refs并Inspect |
| N18 | 同Observation ref更换Result/Items顺序/Coverage/NextCursor | canonical复算拒绝；无Frame |
| N19 | 同DomainResult ref换内容、错Association digest/Owner/Subject/CASAfter | Owner Inspect拒绝；Context不得自行修复 |
| N20 | Memory watermark或Knowledge Published Snapshot/Source/Projection在freeze前漂移 | 当前Frame attempt失败；不得混合新旧Owner贡献 |
| N21 | ContentRef相同但bytes/length/media/digest被篡改 | exact read复算失败；无Candidate/Frame |
| N22 | `now == Expires`或freeze前任一Owner TTL到期 | 按过期Fail Closed |
| N23 | 已选高rank项失效后在同attempt用低rank项补位 | 拒绝；新检索/贡献/Frame attempt |
| N24 | Gateway Retrieval/远程正文或Frame freeze回包丢失后重新执行同身份不同内容 | Gateway只`InspectOriginalAttempt`；Frame只Inspect原Frame Attempt |
| N25 | 压缩summary存在但exact anchor未保留，或anchor保留但Owner事实漂移 | rematerialize并复读current；summary不能作事实 |
| N26 | Knowledge使用`memory_recall`或未冻结的私有fragment kind | Conformance拒绝；Knowledge注入NO-GO |
| N27 | Harness直连Owner Store，或接收字符串拼接revision的Frame ref | 公共边界拒绝；只消费结构化exact Frame ref |
| N28 | Route不能精确物化但继续Model dispatch | 必需内容Fail Closed；可选项只按Recipe产生Residual |
| N29 | production root未装配同一批双Owner Adapter、Application三阶段Port与Context ports就配置非零来源 | 配置/Conformance拒绝；生产Reader和两个Gateway调用数均为0 |
| N30 | 单个Owner Review YES后同时启用Memory与Knowledge | 未获YES的Owner保持0且零调用；禁止合并授权 |
| N31 | 用Current Reader或Retrieval Gateway实现Checkpoint Prepare/Restore Stage | 合同拒绝；Checkpoint/Restore保持独立NO-GO |
| N32 | 未启用的0来源被记录成空检索成功或Complete Coverage | 拒绝；状态必须是明确未启用，不生成Observation/Result |
| N33 | Current Reader签名包含Retrieve、Provider、网络或远程Resolver | Conformance拒绝；Reader只能本地Inspect/current/exact-read |
| N34 | 本地Content evicted后Reader自动远程读取或用Context缓存替代 | 返回`evicted/remote_required`；零外部Effect |
| N35 | Gateway Retrieve缺Operation/Attempt/Permit或prepare+execute Enforcement坐标 | dispatch前拒绝；Provider调用数为0 |
| N36 | Provider回包丢失后调用Reader Inspect或换Provider重试 | 拒绝；只Gateway `InspectOriginalAttempt`原治理坐标 |
| N37 | 远程结果已持久化但Evidence/Runtime Settlement/ApplySettlement未闭表 | Current=false；不得进入Context或宣称Coverage |
| N38 | 远程正文直接流入Frame，未形成Owner本地exact ref | 拒绝；先DomainResult/Settlement/Apply，再由Reader本地读取 |
| N39 | S1后、S2前把pending Context DomainResult或Generation发布为current | 拒绝；两者保持不可见、非current |
| N40 | S2 current/TTL失败仍执行Context ApplySettlement或发布Generation | 原子提交不发生；无current Frame/Generation |
| N41 | Context ApplySettlement成功但Generation current CAS失败并留下可见半状态 | 合同拒绝；两者必须在单个本地原子边界线性化 |
| N42 | S2通过后、原子提交前TTL到期仍发布current | 提交前fresh deadline校验失败；无current发布 |
| N43 | 使用Tool G6A closed matrix或Checkpoint V5批准Retrieval | 不适用；Gateway `unsupported`且Provider/Resolver调用数为0 |
| N44 | Retrieval Evidence获批但Applicability或Settlement版本未冻结便探测Provider | 拒绝；三个retrieval-specific additive版本必须全部YES |
| N45 | Adapter以IdentityID字符串代替完整Identity exact ref/epoch，或任一Owner/Application补造Session coordinate | `coordinate_conflict`；两个Owner来源数保持0 |
| N46 | `TurnOwnerFact(SourceTurnRef).Ordinal`、`SourceTurnOrdinal`、Tool `Execution.Turn`、`ExpectedCurrent.Turn`任一不等 | Fail Closed；不得进入Context pending DomainResult |
| N47 | S1 projection有效，但S2复用不同Session/SourceTurn、Item集合或ClosureDigest | 整个Refresh Attempt失败；无current Frame |
| N48 | Context未发布Knowledge kind却改用`memory_recall`、`artifact_reference`或`instruction` | `unsupported_fragment_kind`；Knowledge注入Fail Closed |
| N49 | Knowledge Source/License/Conflict在Get或S2期间漂移 | 零body；pending Context DomainResult不可Apply |
| N50 | Memory/Knowledge创建Context Candidate、Fragment Fact、pending DomainResult或Generation | Owner/Conformance拒绝；无Context Fact写入 |
| N51 | Application把Owner DTO复制成Context Fact、补写Citation/License或解释closed reason为成功 | Conformance拒绝；Application只协调exact refs |
| N52 | Harness直接调用Owner Reader、消费Owner body或接收candidate/pending Frame | 绑定拒绝；Harness只消费Context current exact Frame |
| N53 | Context ApplySettlement成功而Generation current CAS失败，随后暴露Frame | 原子边界拒绝；只能Inspect原Refresh Attempt，无current发布 |
| N54 | Frame refresh回包丢失后新建Attempt、重跑Reader并得到不同Item集合 | 拒绝；只Inspect原Context Refresh Attempt |
| N55 | closed reason被编码为自由文本、成功空列表或无序map | canonical/strict decode拒绝；失败输出零body |
| N56 | 本地已settled Owner refs被错误强制走远程Gateway，或Gateway unsupported时偷偷探测Provider | 本地Reader路径独立；远程调用拒绝且Provider/Resolver=0 |
| N57 | Memory/Knowledge Reader携带TargetTurn、执行`SourceTurnOrdinal+1`或构造TransitionProof | 合同拒绝；Owner输出只能回显Source字段 |
| N58 | 从live uint32或legacy TurnID生成SourceTurn ID/revision/digest，未复用Harness committed PendingAction exact current | exact coordinate无效；整个refresh Fail Closed |
| N59 | Application而非Context seal final proof，或final proof未sealpre-frame request、Source/Target、Session、childExecution、新Frame/Generation、version/digest/expiry | Context refresh Fail Closed；无current Frame |
| N60 | Memory SourceTurn=T、Knowledge SourceTurn=T+1，或任一Owner自行补Session后混帧 | coordinate conflict；整个refresh失败，不降级为单Owner成功 |
| N61 | final proof在pending DomainResult/Manifest/Frame/Generation seal前产生 | 时序拒绝；不得使用未来ref或占位ref |
| N62 | ClosureDigest包含CheckPhase、OwnerCheckedAt、ExpiresAt或Projection self ref | canonical拒绝；stable closure必须与fresh digest分层 |
| N63 | S1/S2 fresh Projection/Observation refs不同但stable closure与ordered exact集合相同 | 允许继续；仍执行fresh TTL/currentness检查 |
| N64 | S1/S2 fresh ref相同但stable closure或ordered exact集合漂移 | Fail Closed；无Apply/CAS/publish |
| N65 | V2 JSON缺required tag、使用未知ObjectKind、跨Owner nominal/type-pun或required字段`omitempty`消失 | strict decode/Validate拒绝；零Projection/body |
| N66 | V1缺Session/Turn Owner evidence却由Adapter、Context缓存或legacy字符串补齐后迁移V2 | migration拒绝；V1仅历史Inspect，必须新建V2 Attempt |
| N67 | AssociationDigest同ID换DomainResult revision/digest、错Owner、canonical tamper或把SettlementApplicationRef混入算法 | Verify拒绝；不进入stable Closure或Context pending |
| N68 | ctx在等待Owner锁、Get期间或S2前取消，但实现继续等待/返回body | 必须返回兼容context错误、零Observation/nil body且无goroutine/锁泄漏 |
| N69 | ContentRef.Length或Get实际bytes超过MaxBodyBytes，或实现截断后返回成功 | Fail Closed；零Observation/nil body，不联网fallback |
| N70 | Memory或Knowledge任一V2 public struct字段名、Go type、JSON tag、声明顺序与冻结schema不一致，或`CurrentProjectionV2`省略具名current/governance/set digest/`NextCursor`/`ResultDigest`/`EvidenceDigest`字段 | reflection/golden Conformance拒绝；不得启用V2 Binding |
| N71 | StableClosure或任一set digest的domain/version/ObjectKind/body/正规化算法漂移，set digest只验非空、使用map顺序、OrderedItemSet摘要完整ProjectionItem，或遗漏attempt/request/idempotency/current/set digest/`NextCursor`/`ResultDigest`/`EvidenceDigest`字段 | golden/tamper拒绝；S1/S2不得比较该closure |
| N72 | S1后重新Inspect使`AttemptInspectionRef`因`OwnerCheckedAt/ExpiresAt`变化，stable集合未变；实现却改变StableClosureDigest或要求S2携带S1 Inspection ref | 拒绝该实现；fresh Projection digest应变化，stable closure必须不变，S2仍执行fresh TTL裁决 |
| N73 | Coverage与Items不变但NextCursor、ResultDigest或EvidenceDigest任一变化，CurrentProjection或StableClosure canonical却保持不变 | golden/tamper拒绝；三字段必须按冻结顺序进入两个摘要 |
| N74 | 仅改变item ExpiresAt/Digest便改变OrderedItemSetDigest，或篡改任一set digest字符串而底层stable items不变；空集合编码为null、Ref/Content/String集合顺序不同导致摘要不同 | 拒绝；OrderedItemSet只取stable item body，所有set按冻结正规化重算且空集合编码`[]` |

## 3. Effect/Unknown故障注入点

每个Effect kind至少覆盖以下切点：Reservation前、Admission拒绝、Permit前后、Begin前后、Delegation/Prepare后、dispatch Enforcement、Provider执行中、执行成功回包丢失、DomainResultFact CAS前后、Evidence追加失败、Runtime Operation Settlement失败、Domain ApplySettlement失败、Cleanup部分成功。

验证：

- Begin前失败可证明Not Applied；
- Begin后所有不确定性绑定原Operation/Intent revision/Permit/Attempt/Provider；
- Inspect确认未应用后，再执行必须取得新治理Attempt和当前Policy；
- Inspect不完整保持`reconciliation_required`并占用稳定Conflict Domain；
- Evidence、业务Commit、Cleanup与Residual分别结算。

## 4. Owner/Port/装配Conformance

1. `go list -deps`证明domain/kernel只依赖本组件合同；runtimeadapter只依赖runtime/core与runtime/ports公开包；
2. 静态检查禁止Runtime foundation/fakes/kernel内部、Harness internal/kernel/fakes、model-invoker/internal、厂商SDK和其他组件实现包；
3. Slot/Phase golden graph只含公共namespaced版本化对象，Phase kind仅Observer/Filter/Gate/Port；
4. CompiledGraph合并顺序确定，重复Owner、未满足依赖、Binding V2不匹配、per-turn refresh缺失必须诊断；
5. Harness只消费已物化Snapshot；Run内Event/Completion Claim不能被断言成Memory/Knowledge Fact或Runtime Outcome。
6. Memory与Knowledge分别运行Current Reader Conformance：接口只含本地`InspectAttempt/InspectForTurn/ReadContentExact`，禁止Provider/网络/Resolver依赖，也禁止共享Owner实现、状态、Score归一化或预算借用；
7. 每Owner检索顺序固定为Score降序、Record ID升序、Revision降序、Digest升序；exact bytes和固定Estimator共同约束N/Bytes/Tokens；
8. Context Assembler freeze前必须再次调用两个Owner current Inspect，并重算content length/media/digest；任一漂移使Frame attempt失败；
9. 两个Retrieval Gateway Conformance要求retrieval-specific additive Applicability/Evidence/Settlement版本全部已冻结，每次effectful调用精确绑定Operation/Attempt/Permit、prepare+execute Enforcement，并验证Provider Observation→DomainResult→Evidence→Runtime Settlement→ApplySettlement全链；缺一项保持unsupported且Provider/Resolver=0。Tool G6A与Checkpoint V5必须产生不适用结果。
10. per-turn executor、Knowledge fragment kind、结构化Frame ref、freeze `InspectAttempt`或Route exact-reference能力缺一项，联合Context Refresh测试必须标记NO-GO，而不是跳过后宣称通过。
11. reference fixture断言`MemorySources=1`、`KnowledgeSources=1`且两个Gateway调用数为0；production root未装配时生产来源与Reader调用数为0；
12. 静态与接口Conformance证明两个Current Reader均不含Retrieve/远程/Checkpoint/Restore方法，未来Participant Reader不能由类型别名或Adapter扩权；Gateway也不得成为Checkpoint Participant。
13. Context Refresh/Apply/Inspect、single backend/lock、TransitionProof、atomic Apply+Generation CAS与nonzero fixture记为YES；Conformance顺序固定为`S1 -> pending outputs seal -> Context final proof seal -> S2 -> atomic Apply/CAS -> publish`。
14. 两个Owner V2只验证/回显Identity、Session/Turn证据与SourceTurnOrdinal；`SourceTurnRef`由Harness committed PendingAction current经public Adapter无损传递。任何Owner从uint32/legacy字符串补exact ref、携带Target/+1/proof都必须拒绝。
15. Context fragment Conformance把Knowledge专属`knowledge_reference`作为加法枚举严格解码，禁止映射到Memory/Artifact/Instruction。
16. Application三阶段Port Conformance验证pre-frame request、apply/advance、inspect原attempt的调用顺序与exact ref透传；不能seal proof或构造Owner/Context Fact。Harness输入不得出现Owner projection/body/Reader接口。
17. stable Closure与fresh Projection/Observation digest必须分别做canonical golden/tamper测试；S1/S2以stable closure和ordered exact集合比较，而非强求fresh self ref相等。
18. 现有AttemptStatus与`errors.Is` error set必须保持闭合、确定性；任何失败返回零body，不能以空命中或Partial Coverage降级成功，不建立第二`ClosedReason` DTO。
19. V2 nominal/tag golden必须逐Owner验证ContractVersion/ObjectKind、required tag、unknown/duplicate/tail字段、跨Ownertype-pun及untrusted输入不panic。
20. Owner evidence persistence测试必须证明Session/Turn evidence原样落Attempt canonical、Inspect只回显、V1缺字段不可迁移且单一Binding generation无双current。
21. AssociationDigest golden固定domain/version/kind/Owner/DomainResult exact ref；同ID换revision/digest、错Owner与tamper全部拒绝，SettlementApplicationRef单独由stable Closure绑定。
22. ctx-aware reader测试覆盖调用前取消、锁等待取消、Get取消、S2前取消、deadline crossing、body cap与copy isolation；均不得泄漏锁/goroutine/partial body。
23. V2 schema Conformance逐Owner固定七个public struct的字段名、Go type、JSON tag和声明顺序；`CurrentProjectionV2`分别固定Memory Query/View/Watermark与Knowledge Query/View/Snapshot/Pointer、逐项governance和具名set digests。
24. StableClosure golden固定两Owner独立domain/version/ObjectKind常量与canonical body字段顺序；证明V1含`ExpiresAt`的helper不可复用，`AttemptInspectionRef`及fresh字段变化不改stable digest，而任何stable exact ref/set/budget变化必改digest。
25. TTL/复检反例固定：同一Attempt与stable集合重新Inspect，新的Inspection ref只改变fresh `CurrentProjectionV2` digest；S2 request不携S1 Inspection ref，按stable closure与ordered exact集合比较并以fresh Owner clock/TTL决定通过或拒绝。
26. Retrieval envelope golden逐Owner固定`Coverage -> NextCursor string -> ResultDigest string -> EvidenceDigest string -> Items`字段/JSON顺序；三字段任一变化必须同时改变fresh CurrentProjection digest与StableClosureDigest。
27. Set digest golden逐Owner固定set-digest domain/version、每个ObjectKind、canonical body字段顺序和正规化算法；覆盖乱序、exact duplicate、空集合、tamper及fresh item ExpiresAt/Digest变化，证明只有stable item body/集合变化会改变set digest。

## 5. 兼容、迁移和持久化恢复

| 场景 | 核验 |
|---|---|
| v1 strict decode | 未知治理字段、重复键、尾随文档拒绝；Opaque Extension显式版本化 |
| Backend迁移 | Snapshot导出+增量对账+双读Digest；切水位时只有一个Owner |
| Projection升级 | 新旧版本并存；旧Snapshot可复现；切换不改Record |
| Crash recovery | 重启后从State Plane Journal恢复Candidate/Attempt/Watermark；Sandbox本地盘丢失不丢权威Fact |
| Cursor/View漂移 | 旧Cursor显式失效，不跨Authority/Policy/Snapshot水位读取 |
| Purge recovery | 每Backend状态可Inspect；Legal Hold与Residual可回读 |

## 6. 性能、race与工具验收

性能只记录基准，不预设SLA：Candidate/Admission/Commit吞吐，Query fan-out与合并成本，Citation/过滤开销，Projection构建CPU/内存，Snapshot publish与恢复时间。先优化Go的批量读取、Cursor、并发上限、内容寻址和结构共享。

Owner-local Reader已执行的最低真实命令集与后续获批范围的复验基线：

```text
go test ./...
go test -race ./...
go vet ./...
go test -run Conformance ./...
go test -run FaultInjection ./...
go test -fuzz=<approved targets> -fuzztime=<reviewed duration> ./...
```

最终报告必须逐条列实际命令、退出码、失败用例和未覆盖项。未执行的测试不得写“通过”。Rust仅在Go基准和批准目标形成可复现实证后另行计划，并补FFI/进程隔离、崩溃、超时和内存所有权测试。

## 7. 集成与系统退出条件

- Runtime Operation V3/Application链全序可观测且无组件私有兼容层；
- Operation Review与条件性pre-run Evidence若启用，已通过对应Owner合同测试；
- ContextReference在支持与不支持Route均有正确结果；
- Memory/Knowledge Slot/Phase贡献通过Assembler/Harness公共Conformance；
- Run内domain-commits参与者能Inspect/Settlement，组件不修改Runtime Outcome；
- 本地State Plane重启可恢复；远程Retrieval在专用三版本冻结前只验证unsupported且Provider/Resolver=0，不宣称lost-reply恢复成功；
- 无未解释高风险Residual，已知Residual有Owner、范围和处理状态。
- Context Refresh reference四组Delta已由Memory/Knowledge、Application、Context、Harness通过定向Conformance；production root与远程路径保持unsupported。
