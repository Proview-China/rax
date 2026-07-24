# PreparedModelInvocation / PreDispatch Gate V1实施计划

## 1. 状态

- Owner：`Model Invoker`
- 计划版本：`v1`
- 当前阶段：M0/M1主审P1返修已通过双独立短审
- 当前裁决：两次独立短审均为`YES(P0=0/P1=0/P2=0)`；M0/M1定向与全量门禁全部通过
- 代码授权：Model M0/M1已接受；仅解锁Harness M2，Model M2-M5、Tool与production root仍为NO-GO、未授权、未实施
- 允许范围：未来授权后只改`ExecutionRuntime/model-invoker/**`及批准的Model资产
- 禁止范围：不得吞并Harness、Tool、Application、Runtime、Context或Operation Settlement所有权

## 2. 预期产物

完成全部计划后，Model Invoker将提供：

1. 不可变`PreparedModelInvocationFactV1/RefV1`和strict canonical codec；
2. session-level、create-once、短时`PreparedModelInvocationCurrentProjectionV1`；
3. Historical/Current原子Repository、Exact Reader和线程安全内存reference store；
4. public `PreparedModelInvocationCommitGateV1`及中立ACK；
5. Model-owned canonical dispatch guard和非权威attempt validation receipt；
6. `model-invoker/execution` Model bridge、Direct、内置Harness Adapter、root Invoker、RouteInvoker、RouteGateway、retry、continuation和Realtime的no-bypass接线；
7. 普通100轮、race20轮、全量ordinary/race/vet/gofmt和故障注入证据。

这些产物只证明Model Preparation与Surface的因果绑定和调用顺序，不提供Provider执行权，不创建PendingAction/Action/Effect/Settlement。

## 3. 当前P0与后续波次边界

本轮M0/M1没有遗留P0。以下事项属于M2-M5接线前置或相邻Owner职责，不得因M0/M1完成而宣称已闭合：

1. **完整ProfileDigest**：Model Profile Compiler必须公开并seal完整Effective Profile digest；禁止用live `ProfileKeyDigest`替代。
2. **Runtime public Port落地**：asset candidate已有唯一`runtimeports.RegistrySnapshotRefV1`、`RegistrySnapshotExactReaderV1`及Assembly Current Ref/Projection/Reader；仍须实现这些public Go nominal/Reader。Registry Authority仍是Ref.Owner，Model/Harness/Tool直接使用Runtime concrete types，不alias/复制。
3. **PurePrepare边界**：Request/Plan/Route/Profile/Capability/Registry读取和provider request映射必须证明零Provider、Backend、secret、pool、session、process、network调用。
4. **Gate时点**：Gate必须早于`Adapter.Preflight`；Direct Resolve、Harness process initialize、root Capabilities、RouteGateway secret/pool等都不能留在Gate前。
5. **digest分链**：ActualToolSurfaceDigest严格使用Tool公共Expected canonical且只与Tool expected比较；ActualProviderInjectionDigest覆盖richer provider controls；Context Frame注入链完全独立。
6. **Capability因果**：PurePrepare只使用sealed capability snapshot；Gate后的`Provider.Capabilities`不得改变两个sealed tool digest。
7. **Historical/Current/ACK canonical**：ID、Digest、完整CurrentRef/AckRef/BindingRef、时间上界、create-once、lost reply、typed-nil和clock rollback语义必须逐字段冻结且无闭环。
8. **唯一Binding**：一个Invocation epoch只允许一个Current和一个create-once Surface Binding/ACK；retry/continuation只复验，跨TTL fail closed。
9. **统一Guard**：所有tool-bearing真实dispatch必须调用同一个Model-owned guard；不得只依赖Harness/Application构造器。
10. **Realtime/opaque配置**：必须有strict tool extractor或`NoToolSurfaceProofV1`；无法证明时fail closed。
11. **Binding Ref/Reader**：Model neutral SurfaceBindingRef与Tool BindingRef必须同为Owner/ContractVersion/ID/Revision/Digest；Tool actual-point Reader只收BindingRef并返回Binding+ToolAck，Model Ack不得携ToolAckRef。

## 4. 实施波次

### 4.0 编译前唯一清单

external P0闭合前只维护此清单，不创建占位Go类型：

- [x] `PreparedModelInvocationCommitGateV1`的方法集exact只有`Commit(ctx, PreparedRef, CurrentRef)`与`InspectExactAck(ctx, AckRef)`；
- [x] Prepared Historical/Current字段直接使用`runtimeports.RegistrySnapshotRefV1`，Authority仅为`Ref.Owner`；
- [x] Model包内不存在Registry Ref/Reader或Assembly composite的struct、alias、wrapper、mirror和私有JSON DTO；
- [x] Model生产包不import Harness、Tool、Application或其internal/implementation；只允许Runtime public core/ports；
- [x] `PreparedModelInvocationSurfaceBindingRefV1`固定Owner/ContractVersion/ID/Revision/Digest，并与冻结的Tool public exact Ref逐字段同形；
- [ ] actual-point Tool Reader只接受完整BindingRef一个业务参数并返回Binding+ToolAck；
- [x] Model ACK/AckRef wire中不存在ToolAckRef、Tool Ack payload或Tool私有类型；
- [ ] Harness/host拥有Model ACK create-once Repository与Inspect；Model只拥有nominal/canonical/dispatch guard；
- [ ] retry、Open、Stream、continuation逐次复验同一Current/Ack，跨TTL不刷新、不续租。

### 4.1 external P0

| 编号 | Owner | 必须先落地的公共Delta | Model等待方式 |
|---|---|---|---|
| P0-1 | Runtime Go | 实现candidate全部public nominal/Reader：Registry Ref/Exact Reader与Assembly Current Ref/Projection/Reader | 已完成并通过两条独立代码审计；Model直接import Runtime public ports，无fallback |

任一external P0未落地时：Model实现、integration composition和production root全部NO-GO。Registry Owner、Tool与Harness后续实现/conformance仍是相邻Owner接线条件，但不计入本轮external P0数量。

### 4.2 Delta落地后的必跑conformance反例

- [ ] 旧`CommitPreparedModelInvocationV1`或旧`InspectExactPreparedModelInvocationCommitAckV1`实现不能满足接口；
- [ ] Gate删除CurrentRef、交换Ref顺序、返回私有ACK或添加第二套生产Writer时失败；
- [ ] Model本地Registry/Assembly struct、type alias、wrapper或JSON mirror被AST/import门禁拒绝；
- [ ] Prepared字段不是Runtime concrete Registry type时compile/conformance失败；
- [ ] Registry Owner、Version、ID、Revision、Digest任一漂移，或从裸digest/latest反推Ref时fail closed；
- [ ] Harness/Tool私有Registry Ref与Runtime Ref即使字段值相同也不得通过type identity门禁；
- [ ] Tool旧`Kind+ID+Revision+Digest`BindingRef不能进入Model ACK；
- [ ] Tool Reader要求ToolAckRef第二参数、弱ID/latest输入或返回Binding/ToolAck错Ref时Provider调用数为0；
- [ ] Model ACK/AckRef出现ToolAckRef或Tool Ack payload时strict codec/shape conformance失败；
- [ ] Model直接import Harness/Tool、或Assembly composite在Harness/Tool重新定义时依赖/AST门禁失败；
- [ ] Current/Ack过期、clock rollback、retry/continuation跨TTL时不创建第二Binding且Provider调用数为0。

### Wave M0：合同与canonical内核

- [x] 冻结公共nominal types、常量和错误分类；
- [x] 实现Prepared Fact/Ref strict Validate、seal、encode/decode、deep clone；
- [ ] 实现RequestToolsDigest、ActualToolSurfaceDigest、ActualProviderInjectionDigest和identity derivation；
- [ ] 通过获批Tool公共canonical Port生成ActualToolSurfaceDigest，不复制Tool算法、不import internal；
- [x] Runtime asset candidate已冻结`runtimeports.RegistrySnapshotRefV1`和Assembly composite唯一neutral types；
- [x] Runtime asset candidate已冻结`RegistrySnapshotExactReaderV1`完整合同；
- [x] Runtime Registry/Assembly public Go nominal/Reader已由Runtime Owner实现并通过独立审计；
- [x] 直接消费Runtime Registry concrete type并验证snapshot ref，不定义Model alias/wrapper/copy；Assembly Current留在Gate实现边界，不由Model直接消费；
- [x] 在PurePrepare前通过Runtime public Exact Reader完成Registry Authority exact Inspect与pin；
- [x] 将Historical Fact/Ref/Reader与Current Ref/Reader冻结为Model public nominal；Registry只引用Runtime public nominal；
- [x] 实现Current Projection/Ref、ACK/AckRef和完整SurfaceBindingRef canonical；
- [x] 冻结`PreparedModelInvocationSurfaceBindingRefV1`五字段prototype；跨Tool编译接线仍属相邻Owner后续波次；
- [x] 冻结三Owner唯一`PreparedModelInvocationCommitGateV1`方法集：只含`Commit(ctx, PreparedRef, CurrentRef)`与`InspectExactAck(ctx, AckRef)`；
- [x] 实现非权威Dispatch Validation Receipt；
- [x] 证明Fact不内嵌Ref、Stable ID覆盖ContractVersion/InvocationID/InvocationDigest且不依赖Fact Digest、各层无digest闭环。

进入条件：external P0全部落地、唯一public nominal可编译、联合conformance通过，并取得明确Go授权。

退出条件：contract/canonical测试、重复键/非法UTF-8/alias反例全部通过。

### Wave M1：原子Repository与内存Reference

- [x] Historical单方法atomic Ensure；
- [x] ID和Invocation坐标双索引create-once；
- [x] Historical Exact Reader；
- [x] Current create-once Repository和Exact Reader；
- [x] 同canonical幂等、同key换内容Conflict；
- [x] Repository lost reply对同sealed输入最多一次恢复；Gate Indeterminate禁止二次Commit，只允许用错误同时返回的完整稳定ACK Ref做一次Exact Inspect；
- [x] 第二次atomic Ensure的Conflict/Unavailable/Indeterminate作为唯一最终分类，不与首次Indeterminate做`errors.Join`；
- [x] public Ensure在Repository调用前拒绝nil/canceled context，typed-nil固定归一为Invalid；
- [x] Exact Reader在取得读锁后和解锁后复查context，取消不得成功返回；
- [x] typed-nil、Authoritative/Unknown NotFound、Unavailable、Indeterminate分类；
- [x] deep clone/no alias、64并发同identity异content唯一canonical赢家、stored wire篡改重算拒绝和race conformance。

退出条件：reference store只作为Fake/Conformance；明确无production driver/root/SLA。

#### M0/M1实现候选验证证据

- [x] `go test ./tests/preparedinvocation -count=100`；
- [x] `go test -race ./tests/preparedinvocation -count=20`；
- [x] `go test ./...`；
- [x] `go test -race ./...`；
- [x] `go vet ./...`；
- [x] `gofmt -d`、`git diff --check`、trailing whitespace、relative links与import/type-identity conformance；
- [x] 当前P1返修已获双独立短审`YES(P0=0/P1=0/P2=0)`；仅Harness M2解锁，Model M2-M5、Tool与production root继续NO-GO，不据此宣称production no-bypass完成。

### Wave M2：PurePrepare与CommitGate

- [ ] 从完整`UnifiedExecutionRequest`重算`UnifiedRequestDigest`；
- [ ] 从有序Tools+ToolPolicy计算RequestToolsDigest；
- [ ] 从Tool公共Expected canonical计算ActualToolSurfaceDigest；
- [ ] 从mapped provider request计算richer ActualProviderInjectionDigest；
- [ ] 消费sealed Profile/Route/Capability及Runtime Registry snapshot；
- [ ] 验证Historical.RegistryRef == Current.RegistryRef及Ref.Digest == Tool Surface Current Manifest裸digest；
- [ ] Ensure Historical Fact和唯一Current Projection；
- [ ] 调用public CommitGate，验证exact/current ACK；
- [ ] 增加compile-time interface conformance，拒绝Harness改名、删CurrentRef、私有ACK或第二套生产Gate签名；
- [ ] 增加Tool actual-point单Ref Reader conformance：输入BindingRef，返回Binding+ToolAck；拒绝ToolAckRef第二参数和Model ACK ToolAckRef字段；
- [ ] 实现canonical attempt guard：Harness双Reader Inspect Historical/Current、重算两个tool digest、检查clock/current、产生含AttemptRequestDigest的Receipt；
- [ ] 明确ACK/Receipt不是Authority/Permit。

退出条件：Gate失败时PurePrepare之后的全部外部计数为0。

### Wave M3：Model Invoker execution/Direct/内置Harness前置接线

- [ ] 将Model bridge Gate放到`ExecutionRuntime/model-invoker/execution.Runtime.Start`的`Adapter.Preflight`之前；
- [ ] Direct Preflight将Route映射拆为纯部分，Gate后才`Backend.Resolve`；
- [ ] Claude/Qwen/Codex/ACP/Kimi/Gemini在process start/initialize前guard；
- [ ] Preflight后、发送prompt前重新验证Model tool-surface/provider mapping；Context ActualInjectionManifest/Conformance保持独立，不参与Surface Binding等式；
- [ ] `Adapter.Open`入口再次Inspect同一Current/ACK；
- [ ] Cleanup保证Gate后Preflight失败不遗留process/session。

退出条件：`ExecutionRuntime/model-invoker/execution/harness/**`六类Adapter与Direct的process/resolve/open/prompt no-bypass用例全部通过；外部Runtime/Harness Owner只登记Port Delta，不在本计划修改。

### Wave M4：root Invoker、Route与Provider接线

- [ ] root `Invoker.prepare`改为使用sealed capability snapshot做PurePrepare；
- [ ] `Provider.Capabilities`后移到ACK后，只做验证；
- [ ] sync/stream紧邻Provider调用前统一guard；
- [ ] retry每个attempt Inspect同一Current/ACK，不创建第二Binding；
- [ ] 每个actual-point从Model Ack取SurfaceBindingRef，调用Tool单Ref Exact Reader并核Binding.Ref/ToolAck.BindingRef exact；
- [ ] RouteInvoker只传播exact refs，不复制Gate逻辑；
- [ ] RouteGateway拆分离线Route/request映射与Gate后的secret/pool/factory/provider；
- [ ] 低层legacy/ungoverned入口从production composition root剔除或fail closed。

退出条件：Capabilities、secret、pool、factory、Invoke/Stream的ACK前计数均为0。

### Wave M5：Direct continuation、Realtime与Hosted Tool

- [ ] Direct continuation重算两个tool digest并要求与session-level Fact exact一致，同时Receipt携本次AttemptRequestDigest；
- [ ] Input/State-only变化可复用同一Binding；Tools/ToolChoice/ParallelToolCalls/provider mapping变化要求新Invocation epoch；
- [ ] continuation跨TTL fail closed，不refresh Current/Binding；
- [ ] Realtime adapter提供strict tool extractor或NoToolSurfaceProof；
- [ ] Realtime Open和可能改变surface的ClientEvent进入guard；
- [ ] hosted/builtin/code/search/computer tool options纳入ActualProviderInjectionDigest；只有Tool公共Surface entry进入ActualToolSurfaceDigest；
- [ ] Batch/background等tool-bearing路径进入同一规则或由合同证明无Tools。

退出条件：Realtime opaque unknown和surface-changing event反例全部fail closed。

### Wave M6：全量验证与资产同步

- [ ] targeted ordinary `-count=100`；
- [ ] targeted race `-race -count=20`；
- [ ] Model Invoker full ordinary/race/vet/gofmt；
- [ ] fault/count/race/no-bypass矩阵；
- [ ] AST/import边界与production composition conformance；
- [ ] `git diff --check`、links和stale wording；
- [ ] 更新module说明、memory完成事件和项目索引；
- [ ] 保留no production driver/root/SLA残余并交独立Review。

## 5. 推荐最小实施顺序

```text
M0 public data
  -> M1 reference repository
  -> M2 PurePrepare/Gate/guard
  -> M3 model-invoker execution+Direct+内置Harness
  -> M4 root+Route+Provider
  -> M5 continuation+Realtime+hosted
  -> M6 full gates
```

不得并行修改公共Model合同和多个dispatch路径；先由M0/M1冻结可引用边界，再分路径迁移。Harness、Tool和Application只能在external P0落地且Model public Port conformance通过后接线。

## 6. 测试入口

完整用例见[测试矩阵](../../design/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1-test-matrix.md)。未来最少需要：

- types/codec/repository白盒单元；
- 公共API黑盒与strict wire反例；
- PurePrepare外部调用spy；
- 每条no-bypass路径的计数断言；
- clock、expiry、lost reply、typed-nil、split store和concurrency fault；
- sync/stream/retry/continuation/realtime集成；
- ordinary100/race20/full ordinary/race/vet/gofmt/diff。

## 7. 不做事项

- 不在本计划内实现Tool Surface/Binding Repository；
- 不创建PendingAction、ActionCandidate、Tool Dispatch或Tool Result；
- 不拥有外部Runtime Operation Settlement、Evidence、Context Expected/Actual Injection或Conformance；
- 不引入真实API、订阅、secret或公网测试；
- 不宣称production persistence、Continuity adapter、composition root或retention SLA存在；
- 不把ACK/Validation Receipt当Authority或Permit。

## 8. 编译前联合复核输入

Reviewer应优先裁决：

1. P0字段Owner、Runtime Registry/Assembly neutral Port Delta和Profile前置Delta是否无串台；
2. Gate早于Preflight/Resolve/Capabilities/secret/pool的顺序是否可实现；
3. exact Gate方法集、session-level唯一Binding与attempt-levelReceipt是否清晰；
4. root/Route/Direct/Harness/Realtime no-bypass是否覆盖全部真实路径；
5. terminal ToolCall Observation是否被正确限制为历史回链；
6. 是否仍存在Model吞并Tool/Runtime执行权的措辞。
7. Registry Authority Reader、Model无损carrier与Tool Surface Current裸digest交叉是否无反向推断。
8. SurfaceBindingRef同形、Tool单Ref Reader与Model ACK不携ToolAckRef是否闭合。

## 9. 关联资产

- [主设计](../../design/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1.md)
- [测试矩阵](../../design/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1-test-matrix.md)
- [Model设计索引](../../design/model-invoker/README.md)
