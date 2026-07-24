# Tool Engine验收合同

## 1. 设计阶段验收

- Owner/非Owner与Runtime/Application/Harness/Review/Sandbox边界无冲突；
- 所有Effect kind、Conflict Domain、Run Requirement和pre-run Evidence裁决明确；
- Action、Registry、Surface、Package、Receipt、Result和Unknown状态机可逐状态验证；
- 公共合同缺口仅出现在`port-delta.md`，没有私有兼容接口；
- Go为唯一首版语言，未以抽象“高性能”引入Rust；
- 计划逐文件映射到本设计对象和状态机。

## 2. 实现后测试矩阵

| 层级 | 必测内容 | 通过标准 |
|---|---|---|
| 合同单元 | 严格JSON、摘要、版本、排序、上限、深拷贝、Schema、Owner、Run Requirement | 每个无效字段/组合Fail Closed |
| 状态机白盒 | Registry/Surface/Action/Package单调迁移、CAS、并发、迟到Epoch | 非法跳转、等Revision换包、终态复活全部拒绝 |
| Tool Alias | stable ID、revision 1/current+1 CAS、history/current、target exact、Snapshot S1/S2与64并发 | inactive/revoked/漂移Target、wrong expected、历史回退/ABA拒绝；repoint不改旧Surface，Run Alias Reader为零 |
| Catalog exact Inspect | kind+exact Object Ref、closed typed union、Record绑定、S1/S2与ProjectionDigest | cross-kind/type-pun、wrong digest、双读漂移、typed-nil/cancel、deep-copy污染拒绝；无写口/Transport |
| Provider白盒 | Prepare/Enforcement/Execute、Timeout、Cancel、Backpressure、Conflict Domain | 实际点二次校验，Permit/Fence/Binding任一漂移零Provider调用 |
| Application Adapter | Reserve/Inspect/BindPrepared/Observed/Unknown/Settlement exact binding | 只吸收同一Attempt；lost reply只Inspect |
| Model Projection消费门 | Model已公开Reader、完整Projection Ref、Observation digest、`Calls == 1` | Reader通过前Watermark/Candidate/Reservation/Gateway/Provider全为零；Tool无Model publish/write Port |
| Start-or-inspect Watermark | 相同Application Request ID/Revision/Digest/Scope、CanonicalCommandDigest、阶段CAS与恢复 | 首次create-once；重复只Inspect/继续；换Digest/Scope/command冲突；Provider边界后Provider调用计数不增加 |
| Provider boundary授权交接 | 同Attempt execute Enforcement/Handoff current/exact、boundary CAS、响应后Consumption | 二ref绑定同一execute phase后才CAS；CAS成功即可能已调用并转inspect-only；Consumption不预填 |
| Boundary current Reader | Tool SourceRef→Runtime exact Ref无损映射、精确方法签名、Projection字段/expiry/digest | 无Request/SourceKind/Owner/Current字段；Runtime不import Tool；不确定读取/type-pun零Provider |
| G6A Single Call Gateway | Application DTO中的settled PendingAction projection、PendingActionDigest、基数、payload/capability/schema/source exact refs及六项`OwnerCurrentRefV1` | 六项固定为PendingAction/Surface/Capability/Tool/InputSchema/SourceCandidate；Candidate expiry取全部上界最小值，Reservation不越界 |
| Reservation/Attempt | `ApplicationAttemptRefV1`、Intent/Subject/Session、后续Runtime Attempt因果证明 | Reservation无`OperationDispatchAttemptRefV3`；DomainResult才首次绑定它，且必须源自同Reservation/ApplicationAttempt |
| DomainResult typed refs | `ProviderAttemptObservationRefV2`、prepare/execute各一项Enforcement Phase Ref与Evidence Consumption Ref | 字符串Receipt、Opaque JSON、缺phase/Consumption、跨Attempt或换摘要全部拒绝 |
| DomainResult current | 历史Fact与Owner新签current projection | 签发前复读完整因果链；TTL `>0 && <=30s`；Reservation当前过期不否定truthful late result；30s不作为SLA |
| Evidence V3 | Action矩阵五维required、S1/S2 reader、prepare/execute Qualification/Handoff/Candidate/Consumption | Candidate不含Handoff；两phase完全独立；对应handoff前Provider调用数为0 |
| G6A Settlement V4 | typed DomainResult、两项Evidence Binding、current Inspection/Association、Outcome/Disposition、Apply/ToolResult | Runtime只投影settled；只允许三种合法Tool组合；Unknown/indeterminate或任一V4 ref缺失/过期均零Apply/Result |
| G6A actual-point Gate | Runtime V2 exact Provider/Prepared current/Enforcement/Handoff/Boundary/UnifiedNotAfter | public V2隔离路径逐项验证并进入Gateway；V1及缺任一V2 current closure的路径unsupported且调用为零，Wrapper、额外Clock或锁不能通过 |
| 黑盒 | SDK/CLI/API创建Action并经真实公共Port完成/拒绝/Unknown | 不暴露内部handle，不绕过Review/Runtime/Sandbox |
| 故障注入 | Admission/Permit/Begin、两phase Enforcement/Issue/Handoff/Consume、Provider、DomainResult/Settlement/Apply/Continuation各点丢回包 | Provider phase最多一次；恢复按原Qualification/Handoff/Consumption/Attempt/Fact Inspect |
| Conformance | fully/restricted/observe-only/rejected与Residual | 声明外能力不可用；Fake不获得生产等级 |
| 公共接线回归 | production Go AST/import扫描 | 不定义generic Hookface/Slot/Phase，不导入Harness private/ports、Context实现、Model internal或Runtime私有实现；公开assemblycontract允许 |
| Race | Registry、Surface、Action、Batch、Watch并发 | `go test -race`无数据竞争且事实线性化 |
| Vet/Fuzz | `go vet`、合同/Decoder/状态转换Fuzz | 无panic、无越界、无宽松解析绕过 |
| G6B集成 | 未来production composition root + Application + Tool + ContextTurnRefreshPortV1 + Harness Assembly Adapter/Harness | live当前无production root；G6A只用test fixture。G6B闭合root及Context链后才可启用/回注 |
| 系统 | 真实Sandbox与至少一个真实Tool、一个真实MCP Server | 真实Effect门禁、Unknown/Residual、审计和Cleanup可复查 |

## 3. 必须覆盖的反例

1. Model输出伪造Tool Result或Review字段；
2. Tool Dialect改变Effect Class、Scope或Credential；
3. Surface顺序、Schema或Description在Run中无Revision漂移；
4. 同Action ID换Payload、同source sequence换内容；
5. Begin后网络断开导致盲重派；
6. Cancel被当作“确认未执行”；
7. Provider成功回包直接成为Settlement/Run Outcome；
8. ContextReference无法物化却继续调用；
9. Secret进入Schema、日志、Context或Package；
10. 被撤回Package仍进入新Plan；
11. 跨Tenant Session、幂等键或Conflict Domain复用；
12. 大输出绕过Artifact化和限流；
13. PendingActionDigest缺失、错误，或同Ref替换Payload/Capability/SourceCandidate；
14. Candidate或Reservation被当作Permit，或仅凭current authorization调用Provider；
15. Runtime未持久化对应V4.1 phase Enforcement或未完成对应Evidence handoff就调用Provider，或Tool私造V4.1到旧执行引用的兼容映射；
16. Evidence Candidate内嵌Handoff，prepare/execute复用Qualification/Handoff/EventID/SourceSequence/ConsumptionID，或Consume绑定不同Candidate digest；
17. Provider成功回包绕过`ToolDomainResultFactV2 -> Runtime Settlement V4 exact ref -> ApplySettlementV4`；
18. Provider Receipt/Observation直接写authoritative DomainResult，未由Tool Owner复读current并独立Inspect；
19. Settlement V4 Submission未引用typed DomainResult或prepare/execute两项Evidence Binding，或current Inspection的Association/Guard/Projection/DomainResult不exact；
20. Runtime Settlement携带或选择领域Disposition，ToolResult未关闭DomainResult/Settlement/Association/Guard/Projection/Apply，或Context完成后Application未按Tool Inspection的Settlement/Association与new Frame映射Continuation；
21. lost reply/Unknown后重新Prepare、重新Execute或创建新Attempt；
22. 用legacy `ActionPort`/`ToolPort`/`MCPPort`、`GovernedExecutionProviderV2`、Evidence V2、Settlement V3包装扩权；
23. `N > 1`、batch、custom effect被拆分、选首项、聚合或发生任何部分写入/Provider调用。
24. Tool Port定义或调用`BuildContinuation*`，Application跳过Context Refresh，或没有已settled且S2-current的新Frame仍写Harness Continuation。
25. G6A未调用`InspectOperationSettlementEvidenceAssociationV4`，或Association缺prepare/execute任一phase、引用不同Attempt仍Apply/Result。
26. 绕过Runtime public V2 Route/Gateway exact closure，通过V1 Wrapper、额外Clock或锁调用Provider；或G6A调用Context/Harness、注册/启用能力、构造Continuation、推进Turn。
27. Application或Harness import `tool-mcp`实现，Tool domain/kernel依赖Application，Tool adapter依赖Application实现，或Tool承担宿主总装。
28. G6B未PASS就启用Provider生产能力、写Continuation或推进Turn。
29. `succeeded+confirmed_not_applied`，或把Unknown/indeterminate写入Apply/ToolResult。
30. Reservation携带/回填Runtime Dispatch Attempt，或DomainResult的Runtime Attempt无法证明源自同Reservation/ApplicationAttempt。
31. 用字符串Receipt/Opaque JSON替代Provider Observation、prepare/execute Enforcement或两项Consumption。
32. Candidate忽略任一来源current expiry、Reservation晚于最小上界、DomainResult current TTL为零/超过30秒或因果复读漂移。
33. truthful late DomainResult仅因历史Reservation当前已过期被删除/拒绝，或把30秒租约上限宣称为生产SLA。
34. `Execute`跳过Tool Owner Watermark，或相同Request ID/Revision换Digest、Scope、CanonicalCommandDigest仍继续。
35. crash-before-first-provider-call在Watermark非current、下一阶段未权威NotFound，或读取Unavailable/Indeterminate/超时时继续写入。
36. Watermark已到Provider边界后，Application重投触发新Runtime Attempt或再次调用Provider。
37. 宣称消息/RPC/Provider transport exactly-once，而不是canonical command幂等与Provider未知不盲重派。
38. 未调用Model公开exact Reader，或从Application中立DTO、PendingAction payload、event JSON、compat tool calls反推/复制Model事实。
39. Model Reader unavailable/indeterminate、Projection Ref任一字段/Observation digest漂移或`Calls != 1`仍创建Watermark/Candidate/Reservation。
40. Tool持有Model Projection publish/write Port，或自行实现兼容Reader。
41. boundary缺execute Enforcement/Handoff、换摘要/跨Attempt/非current，或Handoff Fact未绑定同一execute phase仍CAS/调用Provider。
42. boundary CAS前预填prepare/execute Consumption，或CAS成功后崩溃再次Execute。
43. Runtime seam跳过`InspectCurrentOperationProviderBoundaryV1(ctx, exactRef)`，或Reader NotFound/unknown/unavailable/indeterminate仍调用Provider。
44. Boundary Ref同ID/Revision换Digest、cross-Attempt、其他三字段Ref type-pun、Projection字段/摘要漂移仍通过。
45. 把Boundary Projection当Authority/Fence/Permit，或用它替代execute Enforcement/Handoff独立验证。
46. Model唯一Call的`CanonicalArgumentsDigest`与PendingAction/Candidate `PayloadDigest`不相等仍创建Watermark、Candidate或进入Runtime Gateway；V1不得用未定义schema transformation解释该漂移。

## 4. 定向并发与故障强度

- G6A对同一PendingAction、Candidate、Reservation、DomainResult与Tool Apply分别执行至少32并发竞争，只允许各Owner一个create-once/CAS获胜且跨域调用计数为零；G6B另测Context Generation与Harness Continuation竞争；
- not-yet-current、expired、expired-retry与请求Reservation TTL超过Candidate寿命均验证零写；
- prepare/execute每个lost-reply点执行`count=100`普通重复，确保Provider phase调用计数不增加；
- race定向重复至少20轮，覆盖current reader S1/S2之间的revoke、supersede、expiry与revision漂移；
- `N != 1`、未知Effect、错误Profile、任一required applicability缺失均在Candidate/Provider前Fail Closed。
- Outcome/Disposition三种合法组合各执行`count=100`，非法组合及Unknown/indeterminate始终零Apply/Result；
- DomainResult current lease在`30s`边界、`30s+1ns`、Observation/Enforcement/Consumption漂移及Reservation过期late truth场景覆盖普通与race；
- 同一Reservation/ApplicationAttempt并发绑定不同Runtime Attempt至少32竞争，只允许exact内容幂等，换Attempt冲突且不覆盖。
- 同一Application Request canonical command至少32并发/重复投递只允许一个Watermark与各阶段一个CAS赢家；同ID/Revision换Digest/Scope/command全部冲突。
- crash-before-provider、provider-boundary-after-call-before-reply、Watermark Inspect Unavailable/Indeterminate分别执行`count=100`；前者只在current exact+权威NotFound后继续，后两者Provider调用计数保持不增加。
- Model Reader unavailable/indeterminate、Ref字段/Observation digest漂移、Calls=0/2分别执行`count=100`及32+并发；Watermark/Candidate/Reservation/Provider计数始终为零。
- Model canonical arguments与PendingAction payload拼接攻击执行`count=100`及并发反例；digest不相等时Model Reader仅允许首次exact读取，零Watermark/Candidate/Runtime Gateway/Provider/Settlement。
- execute Enforcement/Handoff换ref、跨Attempt、过期、boundary CAS回包丢失与CAS后崩溃分别`count=100`；非法交接零Provider，CAS成功场景Provider重派计数为零。
- Boundary Reader同source 32+并发只返回同一current projection；sameID换digest、crossAttempt、expired与type-pun执行`count=100`，Provider调用计数为零；race覆盖读取与Watermark supersede竞争。

## 5. 验证命令规划

实现阶段在`ExecutionRuntime/tool-mcp`独立Go module运行：

```bash
go test -count=1 -shuffle=on ./...
go test -count=20 ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
go test -run '^TestConformance' ./...
```

跨模块集成和系统测试必须由联合评审批准后运行；当前设计阶段不执行这些命令，也不声明已通过。
