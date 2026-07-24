# Application G6A SingleCallToolAction V2 additive实施计划

## 1. 状态

状态：**owner-local design终审 YES；Application owner-local P2代码第四独立终审 YES；Harness P3 Adapter已实现，2026-07-16 live合同漂移与并发P1返修独立复审 YES（P0/P1/P2=0）**。ToolResult Owner真实性、可信生产提交时钟、Tool P4、P5跨模块fixture、系统G6A与production composition root继续`BLOCKED/NO-GO`。

Harness Route V2第八轮独立审计和Harness Owner-current V3/V4最终独立代码审计均已`YES(P0/P1/P2=0)`，其ordinary100/race20/full机械门全绿。上述事实只解除Harness依赖，不代表Application V2、系统G6A或production root完成。旧V1只算Application owner-local实现/测试。

设计源：

- [Application V2设计](../../design/application/single-call-tool-action-v2.md)
- [Application V2测试矩阵](../../design/application/single-call-tool-action-v2-test-matrix.md)
- [V2调用图](../../design/application/single-call-tool-action-v2.drawio)
- [Harness Identity设计](../../design/harness/assembly/model-tool-call-pending-action-identity-v1.md)
- [Owner-current Port Delta](../../design/harness/port-deltas/committed-pending-action-owner-current-inputs-v2.md)

## 2. 冻结后的实施产物

owner-local终审已解除并完成P1-P3实施；P4-P5仍未获准实施。当前已经产生：

1. Application-owned V2 neutral coordinates、去除旧V1重复坐标的ActionCoordinate、Request、Tool Owner Result ref/Application Result双身份、Ref/Inspect；
2. Fact Owner exact Identity Current Reader、`HarnessOwnerCurrentProofV3 + AuthorityCurrentProofV2`与InputCurrent Reader V2；
3. Coordination Fact/CAS/Port V2及Coordinator恢复逻辑；
4. Harness-owned Application Assembler Adapter：SessionV4→Fact Reader→Model Projection exact Reader→CurrentReaderV3→Authority Reader S1/S2；
5. Application/Harness owner-local单元、并发、全量ordinary/race/vet与定向高重复测试。

以下产物尚未产生：

1. Tool-owned V2 start-or-inspect consumer（等待Tool Binding public Port Delta）；
2. test-only cross-module system fixture。

不产生production backend/root/SLA、Policy proof、Context Refresh、Continuation、Turn推进、Capability、Checkpoint、Transformation或N>1。

## 3. 依赖顺序

```text
P0 独立设计终审
-> P1 Application neutral contract/ports V2
-> P2 Application coordination fake/coordinator V2
-> P3 Harness Assembler Adapter owner-local实现
-> P4 Tool Adapter V2 owner-local实现
-> P5 test-only cross-module fixture
-> 联合实现验收
-> production composition root（另门）
```

## 4. 设计阶段清单

- [x] Route V2第八轮独立审计真值已同步；
- [x] 版本闭包改为BindingV2 + Session/CAS V4 + Subject/Request/Current/Reader V3；
- [x] neutral Binding coordinate覆盖Base四事实、OwnerInputs五项、Binding version/digest；
- [x] old six proofs替换为Harness Current V3 proof + Authority proof，并增加Fact Owner exact Identity读取；
- [x] ActionCoordinate先于Authority，Authority绑定ActionCoordinate digest；
- [x] Request TTL删除无输入的Policy；
- [x] Workflow及其余非必要V1坐标从Action V2删除；
- [x] S1/S2完成后fresh nowS2计算时间并只Seal Request一次；
- [x] ResultCoordinate/Result/Ref/Inspect/Tool Port/Coordination/CAS V2 exact字段、domain、状态机与恢复候选已写明；
- [x] Identity补`CreatedUnixNano`并进入canonical；
- [x] 独立设计终审`YES(P0/P1/P2=0)`；

以下清单保留为分层实施记录。Application owner-local P1/P2与Harness P3已落地；Tool Owner、system fixture或production root仍保持未勾选，P4-P5继续阻断。

## 5. 实施任务

### P1 Application contract/ports

- [x] 实现所有V2 struct的strict Validate、Seal、canonical、digest、ID派生；
- [x] 实现IdentityRef/DomainResultFactRef neutral coordinate与Fact Owner exact Current Reader；
- [x] Harness Adapter仅注入Model根包`ToolCallCandidateObservationProjectionReaderV1`，按full Ref exact读取唯一Call；canonical arguments必须是Reader原bytes，最大长度唯一取`runtimeports.MaxOpaqueInlineBytes`，digest=`core.DigestBytes(originalReaderBytes)`，禁止重序列化；
- [x] Reader Adapter、Seal、Clone/返回路径对canonical arguments slice逐层deep-copy，覆盖输入/返回修改不污染、空bytes+非空digest及oversize反例；
- [x] 禁止map、unknown required、opaque JSON与Owner struct；除已exact验证并deep-copy的ProjectionProof canonical arguments外，所有其他payload bytes禁止进入neutral coordinate/proof；
- [x] 实现删除旧V1重复坐标后的ActionCoordinate与Authority顺序，ValidateCurrent用fresh clock；
- [x] 实现ResultCoordinate/Result/ResultRef/InspectKey/ToolActionPortV2与只读Settlement/Association Reader；
- [x] 保留Tool Owner原始Result ID/revision/digest，Application只创建独立ResultCoordinate wrapper；
- [x] 实现HarnessOwnerCurrentProofV3、AuthorityCurrentProofV2和InputCurrentReaderV2；
- [x] 用AST/import测试保证Application只依赖自身与Runtime core/ports。

### P2 Coordination与Coordinator

- [x] 实现FactV2、CASRequestV2、FactPortV2、thread-safe fake；
- [x] Create按V2自身`Scope+ID`实现create-once、same-exact幂等、changed-content Conflict和lost reply Inspect；跨版本不共享ID，统一使用stable semantic key与atomic VersionClaim；
- [x] 同一Coordination Owner/Store/原子线性点create-once写`VersionClaim + initial prepared Fact`；禁止先claim后fact或sidecar补齐；
- [x] 每次从Request重算ConflictKey；`ClaimedActionVersion == Fact.Request.ContractVersion`且只允许V1/V2常量；`CoordinationID == Fact.ID == Request.ID`、`CoordinationDigest == revision=1 prepared Fact.Digest`、`Created == Fact.Created == Request.Created`；
- [x] Create/Inspect/CAS每次复读同一Claim；wrong version/key/id/initial digest/created或Claim缺失全部zero write/zero transition/zero Execute；Claim digest不随current Fact变化；V1未接Claim前系统Route固定拒绝V1；
- [x] CAS exact绑定ExpectedRevision+ExpectedDigest+Next+request digest；
- [x] CASRequest不携完整Completion、不用caller的Next时间伪造current；FactPort只验structural exact Next，Tool Owner result current与可信提交时钟留给P4/system；
- [x] 实现`prepared→dispatch_intent→waiting_inspect→completed`；
- [x] 先把StartClaim CAS到waiting_inspect且收到exact成功回包，再允许一次Execute；64并发仅一个调用权；
- [x] waiting_inspect/lost reply/Conflict/Unavailable/Indeterminate永久Inspect-only；
- [x] 每个current/commit/return边界fresh clock，覆盖rollback与TTL crossing。

### P3 Harness Adapter

- [x] exact读取SessionV4并取得完整BindingV2；
- [x] 逐字段映射Application neutral Binding，不构造/回传Harness struct；
- [x] 注入只读`SettledTurnDomainResultReaderV3`并按FactRef读取full Fact/Identity，验证Created/NotAfter与SourceKey；
- [x] 注入Model根包Projection Reader；验证Projection Validate/ref exact/Calls==1/ordinal0/arguments bytes+digest，Retention不可读fail closed且不伪造TTL；
- [x] 调用CurrentReaderV3而非ReaderV2；
- [x] 使用AuthorityFactReaderV2按ref/scope/ActionCoordinateDigest做S1/S2；
- [x] CurrentV3与SessionV4/BindingV2任一漂移零Request；
- [x] Adapter不持有Tool/Runtime commit口，不创建production root。

### P4 Tool Adapter

- [ ] **阻断**：等待Tool Owner冻结并实现公开Binding exact current Port Delta；当前P4/system不得解冻；
- [ ] 接收V2 Request并复读InputCurrent；
- [ ] same-canonical start-or-inspect，Provider unknown不重派；
- [ ] Result只含ToolResult、V4 Inspection、public Association；
- [ ] G6B/Continuation/Turn/Capability调用保持零。

### P5 测试与fixture

- [x] 已执行当前Application/Harness owner-local适用测试矩阵；APP-V2-31/32涉及Tool P4/system，继续未执行；
- [ ] 执行APP-V2-31/32 Tool P4/system测试；
- [x] targeted count100、race20与64并发；
- [x] Application/Harness full ordinary、full race、vet与gofmt；
- [ ] P5跨模块fixture的diff/import联合门；
- [ ] fixture只注入公开Port，不直接Seal Request；
- [x] 当前system G6A与production root保持`BLOCKED`，未越过准入门；
- [ ] system G6A与production root各自完成独立验收。

## 6. 候选代码落点与当前真值

```text
ExecutionRuntime/application/contract/single_call_tool_action_v2.go
ExecutionRuntime/application/ports/single_call_tool_action_v2.go
ExecutionRuntime/application/fakes/single_call_tool_action_v2.go
ExecutionRuntime/application/single_call_tool_action_coordinator_v2.go
ExecutionRuntime/application/conformance/single_call_tool_action_v2.go
ExecutionRuntime/application/tests/single_call_tool_action_v2_test.go

ExecutionRuntime/harness/applicationadapter/single_call_tool_action_assembler_v2.go
ExecutionRuntime/tool-mcp/applicationadapter/adapter_v2.go
ExecutionRuntime/tool-mcp/tests/system/g6a_identity_v1_test.go
```

上述Application contract/ports/fake/coordinator/conformance与V2单元测试已通过owner-local P2第四独立代码终审；P3 Harness Adapter已实现并通过独立复审，P4 Tool Adapter与P5跨模块system fixture不得伪报已实现。Harness BindingV2、SessionV4、CurrentV3及其Reader是live外部依赖，不在Application代码落点中重复实现。各Owner只改自己的目录。

## 7. 验收门

- 独立设计终审先达到P0/P1/P2=0；
- Application owner-local实现测试全门；
- Harness/Tool owner adapter独立审计；
- cross-module fixture只能证明G6A组合，不证明production root；
- 输出在settled ToolResult + current V4 Inspection + public Association处硬停。
