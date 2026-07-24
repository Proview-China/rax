# Application G6A SingleCallToolActionPortV1实施计划

## 1. 状态

状态：V1 Application owner-local协调实现与测试已完成；但缺少Identity V2、Assembler及system链，不能计为系统G6A或production GO。

兼容说明：本V1计划只覆盖局部协调合同；完整G6A系统闭环还必须通过[additive V2 Identity计划](./single-call-tool-action-v2.md)。V1 equality不能替代Identity/current/Assembler链。

设计事实源：

- [主设计](../../design/application/single-call-tool-action-v1.md)
- [测试矩阵](../../design/application/single-call-tool-action-v1-test-matrix.md)
- [调用与依赖图](../../design/application/single-call-tool-action-v1.drawio)

## 2. 预期产物

实施完成后只产出：

1. Application拥有的`SingleCallToolActionPortV1`、`RequestV1`、`ResultV1`、`InspectRequestV1`；
2. distinct neutral coordinate types、canonical/Validate/Derive ID实现，包括Session/Turn两个独立Applicability source nominal type；
3. Application自己的`SingleCallToolActionCoordinationFactV1`与FactPort/Fake；
4. write-ahead→先Inspect→条件重投同canonical command→Tool start-or-inspect→current V4/public Association复读→completed的Coordinator；
5. import-boundary Conformance、unit/whitebox/blackbox/fault/race测试；
6. 可供Tool与Harness Owner实现Adapter的稳定公共接口。

不会产出Context Refresh、Continuation、Turn推进、Capability启用、Checkpoint、`N>1`、真实Provider、生产Backend、RPC、Scheduler或生产composition root。

## 3. 候选文件

```text
ExecutionRuntime/application/contract/single_call_tool_action_v1.go
ExecutionRuntime/application/ports/single_call_tool_action_v1.go
ExecutionRuntime/application/single_call_tool_action_coordinator_v1.go
ExecutionRuntime/application/fakes/single_call_tool_action_v1.go
ExecutionRuntime/application/conformance/single_call_tool_action_v1.go
ExecutionRuntime/application/tests/single_call_tool_action_contract_v1_test.go
ExecutionRuntime/application/tests/single_call_tool_action_coordinator_v1_test.go
ExecutionRuntime/application/tests/single_call_tool_action_conformance_v1_test.go
ExecutionRuntime/application/tests/single_call_tool_action_imports_v1_test.go
```

本计划不在Application目录创建Tool/Harness Adapter；它们分别由`tool-mcp`与`harness` Owner实现。生产composition root目前不存在，也不是G6A、Owner-local实现或G6B test-only跨模块fixture的前置。G6B fixture只手工注入Application公共Port/各Owner Adapter，不启用Capability、不生产调用Continuation或推进Turn；root的具体落点、进程拓扑和生命周期由宿主Owner在G6B完整验收后的production enablement前确定、实现、验收并完成真实接线Conformance。

## 4. 实施波次

### A. 公共合同

- [ ] 固定`praxis.application.single-call-tool-action/v1`；
- [ ] 实现Workflow/Run/Session/Turn/PendingAction/Observation/Assembly/ParentFrame/ToolResult distinct neutral coordinates；Observation完整覆盖Model Projection Ref的ID/Revision/Digest、Invocation ID/Digest、Observation Digest与ResponseID/SourceSequence，并独立携带EvidenceRecordRef；
- [ ] 新增`SingleCallSessionApplicabilitySourceCoordinateV1`与`SingleCallTurnApplicabilitySourceCoordinateV1`，各自精确含Kind/ID/Revision/Digest并使用不同canonical domain；
- [ ] 新增`SingleCallParentFrameApplicabilitySourceCoordinateV1`，与ParentFrame metadata并列，精确含CTX-D10 Kind/ID/Revision/Digest并使用独立canonical domain；
- [ ] 三个source coordinate进入Request digest、InputCurrentProjection S1/S2与Expires最小值；Harness/Input Adapter逐字段映射/复读，互换、与metadata/public ref type-pun或漂移拒绝；
- [ ] Request不含Runtime Applicability Fact Ref；Tool/Runtime router只允许逐字段无损投影公共ref，不新建ID/digest；
- [ ] Harness-backed Applicability CurrentReader复读source current；公共ref不授Evidence资格，Runtime FactPort不新增Applicability Create/Inspect依赖；
- [ ] ParentFrame公共ref只由Router无损投影CTX-D10四字段，并由Context Owner Reader验证；Application不import Context、不预载Runtime公共ref；
- [ ] DTO仅导入Runtime `core/ports`及Application自有合同；
- [ ] Request无Calls slice、payload bytes、opaque JSON或Owner struct；
- [ ] full ExecutionScope与digest、exact Contract/Step、全部坐标和TTL进入canonical；
- [ ] Request ID按canonical subject确定性派生；同subject换ID零backend拒绝；
- [ ] Result只含ToolResult坐标、current Inspection、Association ref与验证时间；
- [ ] 定义只读Settlement current/Association reader最小接口，不暴露Fact Commit；
- [ ] 定义`SingleCallToolActionInputCurrentReaderV1`与sealed neutral projection，由Harness Owner Adapter实现，供Coordinator S1/S2复读全部输入；
- [ ] Observation S1/S2必须由Harness Adapter调用已终审YES的Model Owner公共只读Projection Reader，按完整Projection Ref逐字段复读完整Projection、重算Projection/Invocation/Observation digest并验证Calls恰为1；
- [ ] 将Model Reader unavailable、Ref/Observation digest漂移、Calls不等于1固定为Tool command前Fail Closed；Application不定义该Reader实现、不持有Model publish/write口；

### B. Application协调Fact

- [ ] `SingleCallToolActionCoordinationFactV1`只保存Application协调水位；
- [ ] 状态固定为`prepared -> dispatch_intent -> waiting_inspect|completed`；
- [ ] 每次CAS只推进一个状态且revision精确`+1`；
- [ ] create/CAS回包丢失Inspect原RequestID；
- [ ] 同ID换RequestDigest/Scope/Workflow/Step/Result全部Conflict；
- [ ] Fake线程安全、深拷贝、确定性，不宣称生产持久性。

### C. Coordinator

- [ ] public request在任何Reader/Store调用前Validate；
- [ ] Harness Adapter聚合已终审YES的Model公共只读Reader与Harness/Tool Owner Readers，确认Observation完整Projection exactly one与PendingAction exact投影；禁止从PendingAction payload、event JSON或compat tool calls反推Projection；
- [ ] S1/S2通过InputCurrentReader复读Session/Turn及其distinct applicability source、PendingAction、Observation、Generation/Binding、Authority、ParentFrame metadata与CTX-D10 applicability source currentness；
- [ ] `dispatch_intent`恢复先Inspect Tool Owner；权威NotFound且Input current exact后才重投同ID/revision/digest/scope command；
- [ ] Tool Adapter Execute按start-or-inspect实现并先线性化/Inspect其内部watermark；Provider边界后只恢复原attempt；
- [ ] Execute任一Unavailable/Indeterminate先转`waiting_inspect`并Inspect原ID/digest/scope；Inspect不可用时不得重投；
- [ ] 明确不承诺exactly-once transport，只承诺canonical command幂等与Provider未知不盲重派；
- [ ] exact Result后执行InputCurrentReader S2，再复读Runtime current V4 Inspection与public完整Association；
- [ ] current/Association exact闭合后才CAS completed；
- [ ] completed后硬停止，不调用Context/Harness、不推进Turn、不启用Capability。

### D. Adapter与依赖Conformance

- [ ] 发布Tool/Harness Adapter可实现的公共Port，不在Application实例化Adapter；
- [ ] Application生产代码import图只有自身与Runtime `core/ports`；
- [ ] Tool domain/kernel、Context、Harness实现包不进入Application go.mod/import；
- [ ] Tool Adapter只允许在`tool-mcp/applicationadapter`依赖Application public contract/ports；
- [ ] Harness Adapter只允许在Harness Owner目录依赖Application public contract/ports；
- [ ] Model Projection Reader合同、Repository原子Ensure与实现已由Model Owner终审YES；Application只声明其为Input Reader的外部只读前置，不新增Model adapter、缓存、Repository或写口；
- [ ] Context CTX-D10 Reader合同与实现由Context Owner负责；Application仅携带中立source并消费Harness/Input Adapter sealed projection；
- [ ] Conformance证明Application Request、Projection、Result和协调Fact均不包含Tool Provider Boundary proof；
- [ ] 测试组合显式标记fixture，不出现Production/Certified/SLA声明。

### E. 测试与收口

- [ ] 完成[测试矩阵](../../design/application/single-call-tool-action-v1-test-matrix.md)全部Unit；
- [ ] 完成write-ahead、current复读、G6A硬停Whitebox；
- [ ] 完成N=1成功、N>1零写、G6B缺失仍可验收Blackbox；
- [ ] 完成所有lost-reply与TTL/clock/drift Fault injection；
- [ ] 完成64并发同内容/不同内容线性化Race；
- [ ] 执行import Conformance并保存实际结果；
- [ ] 仅在全部通过后向中央提交G6A技术PASS候选。

## 5. 严格调用顺序

```text
Harness Owner Adapter产生neutral Request
-> Application Validate + N==1 + S1
-> create prepared CoordinationFact
-> CAS dispatch_intent
-> Tool Owner Inspect original request
-> [authoritative NotFound + Input current exact] submit same canonical command
-> Tool Owner Adapter start-or-inspect
-> [unknown/unavailable] waiting_inspect -> Inspect original request first
-> [authoritative NotFound + current exact] resubmit same canonical command
-> settled ToolResult neutral coordinate
-> InputCurrentReader S2
-> Runtime InspectCurrentOperationSettlementV4
-> Runtime public InspectOperationSettlementEvidenceAssociationV4
-> Application CAS completed
-> G6A hard stop
```

## 6. 验证命令

实施阶段必须实际运行并记录：

```bash
cd ExecutionRuntime/application
go test -count=1 -shuffle=on ./...
go test -count=100 ./tests -run 'SingleCallToolAction'
go test -count=1 -race -shuffle=on ./...
go vet ./...
gofmt -l .
```

资产阶段运行Markdown relative links与`git diff --check`；draw.io运行XML校验。

## 7. 风险与Fail Closed

| 风险 | 处理 |
|---|---|
| 中立DTO退化为Owner struct副本 | distinct nominal coordinates；import/AST/schema Conformance拒绝 |
| 无payload bytes导致Adapter无法执行 | Tool Adapter按PendingAction/Observation exact refs从Owner Reader取值；Reader不可用Fail Closed，不在DTO塞JSON |
| crash发生于dispatch_intent后、首次Port调用前 | 恢复先Inspect；同Owner权威NotFound+current exact后允许重投同canonical command |
| lost reply导致重复Provider动作 | Tool Adapter start-or-inspect；Provider边界后仅Inspect原attempt，canonical command重投不增加Provider调用 |
| Inspect unavailable | 保持waiting_inspect，不重投command，不宣称NotFound |
| Session/Turn source coordinate互换或漂移 | InputCurrentReader拒绝；不投影公共ref、不提交command或completed |
| 公共Applicability ref被误当资格 | Conformance拒绝；资格仍由Evidence Owner Policy/Qualification/current Reader判定 |
| Inspection ref被当完整Association | 必须额外调用public Association Inspect并核对完整Fact |
| Result过期后仍完成 | completed CAS前复读current Inspection；TTL crossing保持waiting_inspect |
| G6B反向阻塞G6A | G6A测试组合不注入Context/Continuation；三元结果闭合即验收候选 |
| 测试组合被误称生产root | fixture命名和Conformance固定否定Production/Binding/Dispatch资格 |
| 自定义Tool Provider要求Application switch | 只按namespaced Provider Binding与公共Port解析，不写kind switch |

## 8. 完成条件

1. Contract/Port/Coordinator/Fake/Conformance与测试均落在Application独占目录；
2. Request绑定全部冻结坐标，Result严格只有G6A三元闭包；
3. N=1、lost reply先Inspect、权威NotFound后同canonical重投、Provider未知不重派、current/Association复读和G6A硬停均有正反测试；
4. Application import图无Harness/Tool/Context及Runtime内部Owner包；
5. ordinary、count100、race、vet、gofmt全部通过；
6. 结果只提交中央联合验收，不启用Capability，不进入G6B。
