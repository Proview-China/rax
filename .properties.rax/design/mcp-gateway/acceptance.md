# MCP Gateway验收合同

## 1. 协议Conformance

- Initialize必须是首个交互，版本协商与initialized通知顺序正确；
- Tools/Resources/Prompts只在Server声明Capability后调用；
- 2025-11-25普通取消与Task取消严格分流；
- stdio消息边界、stdout/stderr隔离通过真实进程测试；
- Streamable HTTP的Origin、Session、Protocol Header、POST/GET/SSE和断线恢复通过真实Server测试；
- 标准层在未启用Praxis扩展时保持可互操作；
- 扩展失败不破坏标准Session。
- process page严格绑定Connection full digest/Epoch/Snapshot，按exclusive source sequence有界拉取；
  乱序、跨流、wrong digest、limit超界和Journal index漂移拒绝，空页不升级为no-effect结论。
- Snapshot V3逐Page绑定Command/Receipt/Apply/Material Set并逐项绑定Tool/Resource/Prompt Material；
  缺失、重复、跨Namespace/Receipt、V2 type-pun或S1/S2漂移全部拒绝；
- 显式MCP Tool Mapping只能在Snapshot V3 exact-current与Material exact闭合后Admission；
  Capability/Tool/Mapping三Record必须共享一个successor RegistryRevision，generic MCP Tool
  transition必须Fail Closed。

## 2. 安全与恶意Server

必须覆盖：超大Schema/结果、重复JSON-RPC ID、重复或乱序响应、非法UTF-8/JSON、工具名冲突、注解伪造、Prompt Injection、跨Epoch迟到响应、Session劫持、Origin绕过、错误Credential Scope、list_changed风暴和无限分页。

预期：全部Fail Closed或进入有界Residual；不写Authority/Review/Runtime/Harness事实，不泄漏Secret。

## 3. 生命周期与Unknown

- Register/Connect/Initialize/Discover/Bind/Refresh/Call/Cancel/Inspect/Drain/Close正常链；
- 每个阶段前后丢回包注入；
- Prepare丢回包只Inspect本地Prepared；
- Begin后Call丢回包只Inspect原Attempt；
- 无远端查询能力时保持Unknown并降低Conformance；
- Task Inspect本身作为独立Operation Effect；
- Connection Epoch变更后旧响应只能进入历史Evidence；
- Close/Disconnect不声称外部Effect回滚。
- PendingActionDigest错误、同Ref换Params/Capability、Candidate或Reservation冒充Permit时，MCP Server收到的请求数为0；
- Runtime V4.1对应phase Enforcement、Evidence V3 current/handoff任一未持久化、引用漂移或五维required事实不可读时，MCP Server收到的请求数为0；
- Evidence Candidate内嵌Handoff、prepare/execute复用Qualification/Handoff/EventID/SourceSequence/ConsumptionID，或Consume换Candidate digest时Fail Closed；
- `N > 1`、批量或聚合Call保持NO-GO，零状态变化且零Transport写；
- Call lost reply/Unknown只Inspect原Attempt，不复用JSON-RPC ID重发，也不创建新Attempt；
- Provider Observation不能跳过Tool Owner独立Inspect；G6A链止于`ToolDomainResultFactV2 -> Runtime Settlement V4 -> current/readonly Association Inspect -> ApplySettlementV4 -> settled ToolResultV2 + current Inspection`，且Context/Harness/能力启用/Continuation/Turn计数均为零；G6B不得跳过`ContextTurnRefreshPortV1 -> pending Context DomainResult -> S2 -> Context Owner local atomic ApplySettlement+Generation current CAS -> new exact Frame`直接进入Harness，Context链不创建Runtime Settlement；
- Settlement V4缺typed DomainResult/任一Evidence Binding，或current Inspection的Settlement/Association/Guard/Projection/DomainResult不exact/过期，或ToolResult未关闭这些refs与Apply时，Apply/Continuation零写；
- Runtime Settlement出现领域Outcome/Disposition字段或选择applied/not-applied/failed的实现必须拒绝；领域语义只由Tool DomainResult/Apply表达；
- legacy `ActionPort`/`ToolPort`/`MCPPort`、`GovernedExecutionProviderV2`、Evidence V2、Settlement V3包装路径必须拒绝；
- 同一Call至少32并发竞争、lost-reply普通重复100轮、race重复20轮，Provider Execute最多一次且所有恢复只Inspect原Attempt。
- 无Context已settled新Frame、Frame S2漂移或Tool试图构造Continuation时，Harness continuation写次数为0。
- G6A未执行Application-facing只读Association Inspect、Association缺prepare/execute任一phase或Attempt不一致时，Apply/Result均零写。
- G6B PASS前，真实MCP Provider能力启用、Continuation与Turn推进次数均为0；Context/Harness缺失不阻塞G6A的内存/本地test transport测试。

## 4. 真实兼容对象

系统测试至少包含：

1. 官方/公开标准stdio MCP Server；
2. 官方/公开Streamable HTTP MCP Server；
3. 一个支持Tools/Resources/Prompts的Server；
4. 一个支持2025-11-25 Tasks的Server或官方Conformance fixture；
5. 一个不支持远端Inspect的Server，证明Unknown/Residual不会被Fake掩盖。

`2025-06-18`另以official Go SDK public协议类型覆盖initialize、tools/list和tools/call严格
codec往返；由于SDK v1.6.1没有public旧版本Client option，该项不得写成真实Session或
production Transport兼容证明。

Fake只用于单元和故障注入，不能作为生产兼容证据。

MCP Discovery Material另覆盖：Tool/Resource/Prompt完整canonical JSON与各自Snapshot摘要逐项
一致；wrong object/description/schema/arguments/annotations/meta digest；duplicate Tool/Prompt
name、duplicate Resource URI；非法schema、超限、exact Ref漂移、deep-copy、nil/canceled context、
observed幂等、lost-reply inspect-only与64并发exact read。材料本身不得触发Capability、Context、
Prompt authority、Registry或Provider写入。

## 5. Go验证矩阵

```bash
go test -count=1 -shuffle=on ./...
go test -count=20 ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
go test -run '^TestMCPConformance' ./...
```

官方Go SDK Discovery与actual-point Call第一切片已执行in-memory真实Client/Server、
分页/cursor/duplicate/超限、协议/Session/TTL/clock、nil/canceled context、真实
`tools/call`、64同key单effect与lost-reply inspect-only测试，并通过targeted
ordinary×100、race×20、Tool full ordinary/race和vet。受治理Call另已穿过official SDK
真实子进程stdio与loopback Streamable HTTP Server fixture并通过ordinary×100、race×20；
该证据覆盖真实协议Transport，不是production Network/Credential/backend/root。Task/SSE、
远端回包owner-local安全门已执行：official Call Result的typed-nil、sampling-only Content、
非object structured content、循环/非有限JSON、双重输出上限、Unknown inspect-only及CLI不输出
raw bytes均已覆盖；Result canonicalizer Fuzz运行5秒。完整Result Content Artifact/背压及
production系统验收仍未执行。JSON-RPC Decoder、Schema、canonical Call Arguments与Pagination
Fuzz也已分别运行5秒；仍需SSE Event、Artifact Store/背压和Lifecycle状态机Fuzz。

`list_changed` owner-local测试另覆盖：official SDK真实通知、wrong/unbound Session、
typed-nil sink、nil/canceled context、clock rollback、同旧Snapshot coalescing、pending换
Snapshot冲突、successor ack、历史exact Inspect及64并发单Fact。测试明确Handler不调用
Discovery/Provider，未把通知升级为Runtime Effect或Snapshot current。

process Observation有界读取另覆盖：三项跨页顺序/upper bound、空exact stream、page digest
篡改、API/SDK deep-copy、typed-nil、nil/canceled context、limit超界、CLI零raw payload及64并发
Record后有序读取；该测试不声明production durable Journal或streaming Watch。
