# Application G6A SingleCallToolActionPortV1测试矩阵

状态：V1 Application owner-local协调实现与本矩阵对应测试已完成；但缺少Identity V2、Assembler及system链，不能计为系统G6A或production GO。

## 1. Unit

| ID | 对象 | 必须证明 |
|---|---|---|
| APP-G6A-U01 | Request canonical | 全字段进入digest；同subject确定性同ID；换ID、revision、Scope、Step或任一坐标即拒绝 |
| APP-G6A-U02 | ExecutionScope | full Scope重算digest；Instance/SandboxLease/Authority epoch漂移拒绝 |
| APP-G6A-U03 | distinct neutral coordinates | Run/Session/Turn/PendingAction/Observation/Generation/Frame/Result不能互相type-pun |
| APP-G6A-U04 | N=1 | `CallCount==1`；0、2、溢出在零Store/Reader/Provider调用前拒绝 |
| APP-G6A-U05 | PendingAction | Capability、Schema、PayloadDigest、SourceCandidate、RequestDigest/ProjectionDigest任一缺失或漂移拒绝；DTO无payload bytes |
| APP-G6A-U06 | TTL | Request取全部current窗口最小值；过期、未来CheckedAt、时钟回退拒绝 |
| APP-G6A-U07 | Result canonical | Request、ToolResult、Inspection、Association、checked/expires全部入digest；Context/Continuation/NextTurn字段不存在 |
| APP-G6A-U08 | Result closure | ToolResult.Settlement、Inspection.Settlement、Inspection.Association、Result.Association必须exact |
| APP-G6A-U09 | Model Projection coordinate | Projection ID/Revision/Digest、Invocation ID/Digest、Observation Digest、ResponseID/SourceSequence逐字段映射；EvidenceRecordRef为独立字段 |
| APP-G6A-U10 | Input current projection | S1/S2 sealed projection绑定Request/Scope和全部冻结输入坐标；缺字段、Owner struct或TTL越界拒绝 |
| APP-G6A-U11 | Session source coordinate | Kind/ID/Revision/Digest全部入Session专属canonical domain；缺失、换Kind或摘要漂移拒绝 |
| APP-G6A-U12 | Turn source coordinate | Kind/ID/Revision/Digest全部入Turn专属canonical domain；缺失、换Kind或摘要漂移拒绝 |
| APP-G6A-U13 | source type-pun | Session/Turn nominal types与canonical domain不同；逐字段内容相似也不可互换 |
| APP-G6A-U14 | ParentFrame CTX-D10 source | Kind/ID/Revision/Digest进入ParentFrame source专属canonical domain和Request digest；missing、摘要漂移或与metadata/source/public ref互换均拒绝 |

## 2. Whitebox

| ID | 场景 | 必须证明 |
|---|---|---|
| APP-G6A-W01 | write-ahead | `dispatch_intent` CAS成功前Port Execute调用数为0 |
| APP-G6A-W02 | canonical command recovery | Coordinator先Inspect；只有权威NotFound+Input current exact才重投同ID/revision/digest/scope command |
| APP-G6A-W03 | current closure | Result返回后先读current V4 Inspection，再public Inspect完整Association，最后才CAS completed |
| APP-G6A-W04 | Association内容 | prepare/execute独立、同Attempt且Ref等于Inspection.Association；裸pair/string拒绝 |
| APP-G6A-W05 | Owner隔离 | Application只直接写CoordinationFact；Application对Tool/Runtime/Harness/Context Fact的直接写次数为0 |
| APP-G6A-W06 | G6A硬停 | Context、Harness Continuation、Turn推进、Capability activation调用次数全部为0 |
| APP-G6A-W07 | Reader unavailable | 保持`waiting_inspect`；不生成Result、不推进Journal completed、不重派 |
| APP-G6A-W08 | Tool start-or-inspect | 重复canonical Execute先Inspect/恢复Tool watermark；Provider边界后Provider调用计数不增加 |
| APP-G6A-W09 | Input S1/S2 source reread | Harness Adapter逐字段复读两个source current；Session/Turn交换或任一字段漂移零command/零completed |
| APP-G6A-W10 | Model Projection S1/S2 | Input Reader每次均调用已终审YES的Model公共只读Reader，按完整Projection Ref逐字段复读完整Projection、重算全部digest并验证Calls恰为1；S1闭合前Tool command为0 |
| APP-G6A-W11 | ParentFrame CTX-D10 S1/S2 | Harness/Input Adapter逐字段调用Context Owner Reader复读source四元组与scope；S1/S2任一漂移零command或不完成G6A；Expires取CTX-D10 projection最小值 |

## 3. Blackbox

| ID | 场景 | 必须证明 |
|---|---|---|
| APP-G6A-B01 | V1兼容链 | 输入一个V1 exact Request只能返回`system_identity_incomplete`；不得把V1 equality冒充完整G6A，V2 Identity链联合PASS后另行验收 |
| APP-G6A-B02 | Observation为0/N>1 | Fail Closed；无Application Attempt、Tool Candidate、Reservation、Provider动作 |
| APP-G6A-B03 | Owner-current漂移 | Session/Turn/PendingAction/Observation/Generation/Binding/Authority/ParentFrame任一漂移，零Execute或停留Inspect |
| APP-G6A-B04 | Provider Receipt先到 | Receipt不能成为Result；必须等待Tool DomainResult→Settlement V4→ApplySettlement |
| APP-G6A-B05 | V2 Identity缺失 | 即使不注入Context/Continuation且已有Tool settled输出，仍返回`system_identity_incomplete`，不完成G6A |
| APP-G6A-B06 | source-to-public-ref | Router仅逐字段投影Kind/ID/Revision/Digest；公共ref没有新ID/digest且不授Evidence资格 |
| APP-G6A-B07 | ParentFrame双坐标 | metadata coordinate与CTX-D10 source并列且均exact；缺任一项、互换或Context Reader不可用均Fail Closed |

## 4. Fault injection

| ID | 断点 | 恢复要求 |
|---|---|---|
| APP-G6A-F01 | Coordination create回包丢失 | Inspect原RequestID；同内容继续，换内容Conflict |
| APP-G6A-F02 | dispatch_intent CAS回包丢失 | Inspect原Fact确认水位；不提前Execute |
| APP-G6A-F03 | crash after dispatch_intent before first Port call | 恢复先Inspect；同Owner权威NotFound+current exact后重投同canonical command，并只形成一个Tool watermark |
| APP-G6A-F04 | Tool Result已写、Application未见 | Inspect取得exact Result，再复读Runtime closure |
| APP-G6A-F05 | Settlement current Inspect回包丢失 | 重读同Effect/Settlement；不改ToolResult |
| APP-G6A-F06 | Association Inspect unavailable | 保持waiting_inspect；不接受Inspection内Ref代替完整Fact读取 |
| APP-G6A-F07 | completed CAS回包丢失 | Inspect原CoordinationFact；只接受exact completed successor |
| APP-G6A-F08 | clock regression/TTL crossing | Fail Closed；已存在事实只供历史Inspect，不启用能力 |
| APP-G6A-F09 | lost reply after Provider call | Tool Inspect恢复原attempt；Application即使重投canonical command也不得再次调用Provider |
| APP-G6A-F10 | Tool Inspect unavailable/indeterminate | 保持waiting_inspect且零command重投；恢复后仍先Inspect原ID |
| APP-G6A-F11 | Session/Turn source在S2漂移 | 保持waiting_inspect；不接受旧公共ref、不完成G6A |
| APP-G6A-F12 | Model Projection Reader不可用或漂移 | Reader unavailable、Ref/Observation digest漂移、Calls不等于1均Fail Closed；Tool command为0或保持waiting_inspect且不完成G6A |
| APP-G6A-F13 | ParentFrame source missing/drift | Kind/ID/Revision/Digest缺失，S1/S2漂移，或CTX-D10 Reader NotFound/Unavailable/Conflict时，零Tool command或保持waiting_inspect |

## 5. Conformance与import boundary

| ID | 检查 | 必须证明 |
|---|---|---|
| APP-G6A-C01 | Application public API | `SingleCallToolActionPortV1/RequestV1/ResultV1/InspectRequestV1`只依赖Application contract与Runtime core/ports |
| APP-G6A-C02 | Tool Adapter | 仅`tool-mcp/applicationadapter`依赖Application public contract/ports；Tool domain/kernel不依赖Application |
| APP-G6A-C03 | Harness Adapter | 位于Harness Owner目录，只投影公开Owner事实；Application不import Harness |
| APP-G6A-C04 | Application imports | 无Harness、tool-mcp、context-engine、Runtime kernel/control/fakes/foundation导入 |
| APP-G6A-C05 | Runtime权限 | Coordinator没有`OperationSettlementFactPortV4`或Commit能力，只使用current/Association Inspect窄接口 |
| APP-G6A-C06 | DTO schema | 无`map[string]any`、owner struct、opaque JSON、payload bytes或通用Object union |
| APP-G6A-C07 | composition | 测试组合不得宣称生产root/Backend/SLA；生产root仍是G6B残余 |
| APP-G6A-C08 | custom provider | 更换namespaced Tool Provider Binding不修改Application kind switch，仍须同一closed N=1矩阵 |
| APP-G6A-C09 | Input Reader | `SingleCallToolActionInputCurrentReaderV1`只返回Application sealed neutral projection，无Owner写口/struct |
| APP-G6A-C10 | Runtime Reader | `SingleCallOperationSettlementCurrentReaderV1`只有current/Association Inspect，无Settle/FactPort/Commit |
| APP-G6A-C11 | Applicability输入 | Request/Projection schema不含`OperationScopeEvidenceApplicabilityFactRefV3`，只含Session、Turn、ParentFrame三个distinct Application source coordinates |
| APP-G6A-C12 | Applicability Owner | Runtime FactPort无Applicability Create/Inspect依赖；Harness-backed current Reader只读source current |
| APP-G6A-C13 | Model Reader依赖 | Application只消费Harness Adapter聚合后的中立Projection；Model公共只读Reader/atomic Ensure已终审YES，Application无Model publish/write口且不发明Reader/Repository实现 |
| APP-G6A-C14 | Context Reader依赖 | ParentFrame source由Context Owner CTX-D10 Reader验证；Application不import Context、不实现Reader、不持有Tool Boundary proof |

## 6. Race与重复验证

| ID | 并发 | 必须证明 |
|---|---|---|
| APP-G6A-R01 | 64 goroutines同Request | 一个Coordination Fact与一个Tool Owner线性化结果；其余Inspect同结果 |
| APP-G6A-R02 | 64 goroutines同ID不同内容 | 仅一个canonical内容可线性化，其余`idempotency_payload_mismatch` |
| APP-G6A-R03 | 同subject不同ID | 全部在Request Validate阶段拒绝，backend调用为0 |
| APP-G6A-R04 | completed与waiting_inspect竞态 | exact completed胜出或保持可Inspect，不产生两个Result |

实施阶段至少执行：

```bash
go test -count=1 -shuffle=on ./...
go test -count=100 ./tests -run 'SingleCallToolAction'
go test -count=1 -race -shuffle=on ./...
go vet ./...
```

跨模块G6A测试组合只允许在各Owner合同完成后执行，不以Fake通过宣称生产接线完成。

## 7. 硬反例

1. DTO直接包含`PendingActionV2`、`ToolCallCandidateObservationV1`、`ToolResultV2`或`ContextFrame`；
2. 用opaque JSON、owner package别名或reflection绕过中立DTO；
3. 取Observation `Calls[0]`忽略完整基数；
4. 未先得到同Owner权威NotFound或未复读Input current，就在`dispatch_intent/waiting_inspect`后重投command；
5. canonical command重投时更换Request ID、Revision、Digest、Scope或任一内容；
6. Tool Adapter把command重投解释为再次调用Provider，或宣称exactly-once transport；
7. Tool Result回包直接完成Application，跳过Input S2/current Inspection/public Association Inspect；
8. Application构造Tool DomainResult、Settlement、Outcome或Continuation；
9. G6A测试组合被命名或记录为生产composition root；
10. G6B未验收却启用Capability、Context Refresh、Continuation或Turn推进。
11. Application Request预载Runtime Applicability ref，或Runtime/Tool router为公共ref新建ID/Digest；
12. 把公共Applicability ref当作Evidence Qualification、Permit或Enforcement；
13. Session/Turn source coordinate互换、共用canonical domain或退化为通用Object ref。
14. 从PendingAction payload、Harness event JSON、compat tool calls或Evidence Record反推/重建Model Projection；
15. Model Projection Reader不可用、Ref或Observation digest漂移、Calls不等于1时仍提交Tool command；
16. Application定义Model Reader实现、缓存Projection权威副本或持有Model publish/write口。
17. 缺失ParentFrame CTX-D10 source，或用`SingleCallParentFrameCoordinateV1`、Session/Turn source、Runtime公共ref替代；
18. Router为ParentFrame source公共ref新建ID/Revision/Digest，或跳过Context Owner Reader；
19. 把Tool Provider Boundary proof装入Application Request、InputCurrentProjection、Result或协调Fact。

## 8. 链接

- [主设计](./single-call-tool-action-v1.md)
- [实施计划](../../plan/application/single-call-tool-action-v1.md)
