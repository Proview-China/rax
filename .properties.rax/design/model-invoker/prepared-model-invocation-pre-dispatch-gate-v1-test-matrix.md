# PreparedModelInvocation / PreDispatch Gate V1测试矩阵

## 1. 验收目标

测试必须证明四件事：

1. Historical Fact、Current Projection和ACK的identity/canonical/current语义无歧义；
2. Gate未返回并通过current ACK验证前，所有外部调用计数严格为0；
3. Tools、schema、顺序、policy、profile、route、capability、registry或actual injection任何漂移都fail closed；
4. sync、stream、retry、continuation、Harness、RouteGateway和Realtime不能形成旁路。

本矩阵是设计验收标准，不代表测试代码已经实现或通过。

Surface第六审确认Model本地合同已基本闭合。Runtime asset candidate及`RegistrySnapshotExactReaderV1`合同已经落盘；当前external P0为1项：候选public Go nominal/Reader尚未实现。不得据此写Go。

## 2. Contract与canonical

| ID | 场景 | 断言 |
|---|---|---|
| C01 | 合法Fact seal | Ref/Fact字段闭合，`InvocationDigest == UnifiedRequestDigest` |
| C02 | ID派生 | 相同Invocation坐标得到同ID；不依赖Fact Digest |
| C03 | Fact digest | 修改任一非Digest字段必改变Digest |
| C04 | Current digest | Prepared Ref、snapshot、两个tool digest、Checked/Expires任一变化必改变Digest |
| C05 | ACK digest | Prepared/Current Ref、Gate ref、完整SurfaceBindingRef、时间任一变化必改变Digest |
| C06 | strict JSON | 顶层/嵌套重复键、非法UTF-8、trailing bytes、非canonical schema全部拒绝 |
| C07 | clone/no alias | Reader返回值、Raw schema和ref均不可回写Store |
| C08 | empty/zero | 空ID、零revision、零digest、空snapshot ref全部拒绝 |
| C09 | digest loop反例 | Fact不含Ref，Ref Digest只复制sealed Fact Digest；无递归依赖 |
| C10 | terminal coordinate | ToolCall Ref的InvocationID/Digest与Prepared Ref exact一致 |
| C11 | CurrentRef exact | Contract/ID/Revision/Digest/PreparedRef/Checked/Expires/NotAfter逐字段复制并可重算 |
| C12 | AckRef exact | 完整Prepared/Current/Binding refs和时间逐字段复制ACK，无digest闭环 |
| C13 | Runtime Registry exact ref | `runtimeports.RegistrySnapshotRefV1`的Owner/Version/ID/Revision/Digest缺一拒绝；裸RegistrySnapshotDigest拒绝 |
| C14 | 三Owner public nominal | Model/Harness/Tool直接使用Runtime Registry Ref与Assembly composite concrete types；alias、wrapper、私有缩水DTO编译/AST或conformance拒绝 |
| C15 | Gate method set | 公共接口exact只有`Commit(ctx, PreparedRef, CurrentRef)->ACK`与`InspectExactAck(ctx, AckRef)->ACK`；Harness实现签名漂移时编译失败 |
| C16 | Gate完整输入 | 删除CurrentRef、交换Prepared/Current、传ID/digest缩水DTO或返回私有ACK全部拒绝 |
| C17 | Binding Ref同形 | Model neutral Ref与Tool BindingRef均为Owner/ContractVersion/ID/Revision/Digest，字段类型、JSON tag和Validate逐项一致 |
| C18 | live Kind Ref反例 | `Kind+ID+Revision+Digest`旧Ref不能转换、补全或进入Model ACK |

## 3. Tool Surface与Provider Injection分离

| ID | 变化 | 结果 |
|---|---|---|
| T01 | Tools交换顺序 | RequestToolsDigest、Tool公共Expected canonical及Fact Digest变化，旧身份Conflict |
| T02 | tool name变化 | 同上 |
| T03 | input schema byte/canonical语义变化 | 同上 |
| T04 | output schema变化 | 同上 |
| T05 | strict/owner/side-effect/approval/timeout变化 | 同上 |
| T06 | ToolPolicy变化 | 同上 |
| T07 | mapped ToolChoice变化 | 只改变ActualProviderInjectionDigest，dispatch拒绝；不得改变/混比Tool expected digest |
| T08 | ParallelToolCalls nil/false/true变化 | 只改变ActualProviderInjectionDigest，dispatch拒绝 |
| T09 | hosted/builtin tool option变化 | 只改变ActualProviderInjectionDigest，dispatch拒绝 |
| T10 | provider tool extension变化 | 只改变ActualProviderInjectionDigest，dispatch拒绝 |
| T11 | 普通Input/State变化但Surface不变的continuation | 允许复用Historical Fact、同一Current/ACK；必须重新Inspect并生成Validation Receipt |
| T12 | continuation改变Surface | 必须新Invocation epoch；旧ACK拒绝 |
| T13 | Tool public canonical | ActualToolSurfaceDigest与`ComputeExpectedInjectionDigest(entries)`逐字节相等 |
| T14 | richer digest误比较 | ActualProviderInjectionDigest不得与Tool ExpectedInjectionDigest比较 |
| T15 | Context链误混 | Context Expected/Actual Manifest或Conformance不能满足Surface Binding等式 |
| T16 | Surface Registry ref | Model Historical/Current完整Registry Ref的Digest必须等于Tool Surface Current.Manifest.RegistrySnapshotDigest |
| T17 | Registry exact交叉 | Historical.RegistryRef == Current.RegistryRef，且Ref.Digest == Tool Surface Current Manifest裸digest |
| T18 | 裸digest反推 | 只有RegistrySnapshotDigest时不得构造Owner/Version/ID/Revision或调用Gate |

## 4. Repository、Reader与恢复

| ID | 场景 | 断言 |
|---|---|---|
| R01 | 首次Ensure Fact | 只创建1条记录 |
| R02 | 同canonical重复Ensure | 幂等，记录数仍为1 |
| R03 | 同ID换内容 | Conflict，原记录不变 |
| R04 | 同Invocation坐标换内容 | Conflict，原记录不变 |
| R05 | lost Fact Ensure reply | 同sealed Fact最多恢复一次，调用数可观测，记录数1 |
| R06 | 第二次Indeterminate | fail closed，不外呼 |
| R07 | exact Reader | 完整Ref读取完整clone并重算全部digest |
| R08 | weak/latest读取 | 公共API不存在或明确拒绝 |
| R09 | authoritative NotFound | 只由同线性化Repository证明；不得采信另一个Store |
| R10 | unknown/retention NotFound | 不重投、不外呼 |
| R11 | Current create-once | 同Current幂等；同Prepared Ref换Checked/Expires/content Conflict，不能创建第二Current |
| R12 | lost Gate reply | 同Prepared+Current refs最多重试一次，只接受exact ACK |
| R13 | split store/wrapper | 不能被canonical producer表达或构造期拒绝 |
| R14 | typed-nil | New/Preflight/guard都在任何外部调用前失败 |
| R15 | 同Invocation第二Binding | Conflict，不更新原Binding，不外呼 |
| R16 | attempt Validation Receipt | ID按Prepared+DispatchSequence派生；包含AttemptRequestDigest与两个tool digest；不可作为下一guard输入或Authority/Permit |
| R17 | ACK扩TTL | Ack.Expires大于Current.Expires或NotAfter不等于Fact/Current时拒绝 |
| R18 | Binding裸digest | 缺Owner/Version/ID/Revision任一字段时Gate/ACK拒绝 |
| R19 | Harness双Reader | Gate必须分别exact Inspect Historical Fact与Current Projection，payload副本不能替代Reader |
| R20 | Registry Authority Reader | PurePrepare前用完整candidate Ref exact Inspect Registry Owner；返回deep clone逐字段exact后才能pin |
| R21 | Registry owner/drift | Owner/Version/ID/Revision/Digest任一漂移或Reader NotFound/Unavailable/Indeterminate均fail closed |
| R22 | PurePrepare不回读Registry | verified Ref pin完成后PurePrepare只消费clone，Registry Reader调用数不再增加 |
| R22A | Registry Reader资产合同 | Runtime asset candidate必须存在`RegistrySnapshotExactReaderV1`完整Ref请求、Authority验证、historical/current分离、deep clone与closed errors；任一缩水时Model保持NO-GO |
| R22B | Runtime type identity | Prepared Historical/Current、Harness composite、Tool Binding字段的Go类型必须直接等于Runtime concrete nominal，不允许alias/copy |
| R22C | Harness私有同形Ref | Harness声明五字段相同的本地named struct并传入Prepared/Gate；compile或type identity conformance失败，不能靠逐字段复制接线 |
| R22D | Tool私有同形Ref | Tool声明五字段相同的本地named struct、alias、wrapper或wire mirror；compile/AST/type identity conformance失败，必须直接使用`runtimeports.RegistrySnapshotRefV1` |
| R22E | Runtime public Go缺口 | asset candidate虽存在，但`runtime/ports`中任一Registry/Assembly public nominal或Reader未实现时，compile/conformance失败且Provider调用数为0 |
| R23 | Tool actual-point单Ref Reader | 只传完整BindingRef，返回Binding+ToolAck deep clones；调用计数恰好1 |
| R24 | 预传ToolAckRef | Model ACK出现ToolAckRef或Tool Reader要求AckRef第二参数时contract/conformance失败 |
| R25 | Binding返回漂移 | Binding.Ref或ToolAck.BindingRef与输入Ref任一字段不等时Provider调用数0 |
| R26 | Tool Reader弱输入 | Invocation/latest/ID-only/digest-only/private request DTO全部不可表达或拒绝 |

## 5. 时间与current

| ID | 场景 | 断言 |
|---|---|---|
| K01 | 正常时序 | `Fact.Created <= Current.Checked <= Ack.Checked < Ack.Expires <= Current.Expires <= Fact.NotAfter`且三方NotAfter相等 |
| K02 | Fact current上界过期 | Historical Reader仍可读；NotAfter不作为retention TTL；Current/ACK不可用 |
| K03 | Current过期 | dispatch count=0 |
| K04 | ACK过期 | dispatch count=0 |
| K05 | context deadline更早 | 采用更早上界 |
| K06 | clock rollback | `ClockRegression`，所有外部计数0 |
| K07 | clock jump超过expiry | `Expired`，所有外部计数0 |
| K08 | retry跨expiry | 原attempt不重放；下一attempt count=0 |

## 6. 全路径no-bypass

每个用例都配置独立计数器：`providerCapabilities`、`backendResolve`、`secretResolve`、`poolLease`、`processStart`、`initialize`、`adapterOpen`、`providerInvoke`、`providerStream`、`backendInvoke`、`backendOpenStream`、`realtimeOpen`、`hostedToolDispatch`。

| ID | 路径 | ACK前必须为0的计数 |
|---|---|---|
| P01 | `model-invoker/execution.Runtime` -> Direct Preflight | backendResolve及其后全部 |
| P02 | `model-invoker/execution.Runtime` -> Claude Preflight | processStart/initialize/session/prompt全部 |
| P03 | `model-invoker/execution.Runtime` -> Qwen Preflight | 同P02 |
| P04 | `model-invoker/execution.Runtime` -> Codex App Server Preflight | 同P02 |
| P05 | `model-invoker/execution.Runtime` -> ACP Preflight | 同P02 |
| P06 | `model-invoker/execution.Runtime` -> Gemini/Kimi内置Harness | process/session/provider全部 |
| P07 | root Invoker sync | providerCapabilities/providerInvoke全部 |
| P08 | root Invoker stream | providerCapabilities/providerStream全部 |
| P09 | RouteInvoker | root provider全部 |
| P10 | RouteGateway | secretResolve/poolLease/factory/provider全部 |
| P11 | Adapter.Open | adapterOpen/provider全部 |
| P12 | Invoke retry | 每个attempt前复验；ACK失败时下一attempt为0 |
| P13 | Direct sync continuation | backendInvoke为0 |
| P14 | Direct stream continuation | backendOpenStream为0 |
| P15 | Realtime Open with tools | realtimeOpen为0 |
| P16 | Hosted/builtin tool | hostedToolDispatch/provider为0 |
| P17 | unknown opaque realtime config | fail closed，realtimeOpen为0 |
| P18 | valid NoToolSurfaceProof | 只允许明确无tool路径，proof drift则0 |

P01-P18分别注入Gate unavailable、NotFound、Indeterminate、Conflict、Drift、Expired和ClockRegression；每个错误都必须保持对应计数为0。

## 7. Capability因果

| ID | 场景 | 断言 |
|---|---|---|
| A01 | sealed local capability snapshot | PurePrepare成功，providerCapabilities=0 |
| A02 | Pure EvaluateCapabilities | 无网络/进程/secret/pool调用 |
| A03 | Provider.Capabilities实现为本地 | 仍只允许ACK后调用 |
| A04 | Provider.Capabilities外呼 | ACK前为0；ACK后受独立外部发现策略约束 |
| A05 | Gate后Capabilities与snapshot一致 | 可继续dispatch |
| A06 | Gate后Capabilities改变tool support/mapping | Drift，Invoke/Stream=0 |
| A07 | capability snapshot过期/不可读 | Gate前fail closed |
| A08 | mapping依赖未sealed discovery | PurePrepare拒绝，不能形成Fact |

## 8. retry、continuation与epoch

| ID | 场景 | 断言 |
|---|---|---|
| E01 | sync retry同Surface | 同Historical Ref与同一Current/ACK，每attempt重新Inspect并生成Receipt |
| E02 | retry改变任一tool digest | Conflict/Drift，下一attempt=0 |
| E03 | continuation只增加tool result/Input/State | 同Historical Ref与同一Current/ACK；重算两个tool digest相同，AttemptRequestDigest变化 |
| E04 | continuation改变Tools | 旧epoch拒绝；新Invocation可重新准备 |
| E05 | continuation改变ToolChoice | 同E04 |
| E06 | duplicate continuation | create-once/idempotency，不多发dispatch |
| E07 | cancellation before gate | 所有外部计数0 |
| E08 | cancellation after ACK before call | 紧邻调用复验失败，外部计数0 |
| E09 | retry/continuation跨TTL | fail closed，不刷新Current、不创建第二Binding |
| E10 | ParallelToolCalls/provider映射漂移 | 必须新Invocation epoch，同Invocation内拒绝 |

## 9. Realtime与Hosted Tool

| ID | 场景 | 断言 |
|---|---|---|
| L01 | adapter strict extractor发现Tools | 分别生成Tool Surface canonical和richer Provider Injection digest并Gate |
| L02 | extractor证明无Tools | 产生canonical NoToolSurfaceProof |
| L03 | opaque config无法分类 | fail closed |
| L04 | ClientEvent不改变surface | 允许发送 |
| L05 | ClientEvent新增/替换Tools | 必须新epoch并Commit |
| L06 | hosted search/code/computer tool option | 纳入ActualProviderInjectionDigest；只有公共Surface entry进入ActualToolSurfaceDigest |
| L07 | provider偷偷添加native tool | Model tool-surface/provider mapping重验失败，prompt/invoke=0；不得借Context Conformance放行 |

## 10. 并发、故障与门禁

- 64并发Ensure同Fact：一个canonical记录，全部返回exact clone；
- 64并发同身份不同内容：至多一个赢家，其余Conflict；
- 32并发Current/Gate：不出现过期ACK放行、alias或数据竞争；
- fault injection覆盖Fact Ensure、Current Ensure/Read、Gate、clock、snapshot reader和每个外部边界；
- targeted ordinary `-count=100`；
- targeted race `-race -count=20`；
- Model Invoker full ordinary/race/vet/gofmt；
- `git diff --check`、relative-link扫描、trailing whitespace扫描；
- AST/import conformance：Model公共合同不import Tool/Harness/Application/Runtime implementation/internal/vendor；
- Runtime-port conformance：Model只import Runtime public `ports`中的Registry Ref/Reader与所需Assembly neutral type，不定义第二套Registry/Assembly nominal；
- Harness/Tool identity conformance：Prepared、Harness composite和Tool Binding字段的Registry Ref静态类型必须exact为`runtimeports.RegistrySnapshotRefV1`；同形named type、alias、wrapper与wire mirror全部拒绝；
- cross-owner shape conformance：Model neutral SurfaceBindingRef与Tool public BindingRef逐字段同形，禁止Kind旧Ref、alias或缩水镜像；
- actual-point conformance：Tool exact Reader只有完整BindingRef一个业务参数，返回Binding+ToolAck；Model ACK wire中不存在ToolAckRef；
- composition conformance：production root不存在未注入Gate的Model dispatch入口。

## 11. 阶段裁决

- P0合同测试全部设计完成后，才允许写公共types/codec/repository；
- 候选Runtime public Go nominal/Reader未实现前，Model Go、integration composition和production root均为NO-GO；不得用Model私有占位type解阻；
- P1 no-bypass迁移未全部通过前，不得宣称Surface桥可用于production；
- production persistence/root/retention仍是后续Delta，不阻塞内存reference implementation，但必须保持NO-GO标记。

## 12. 关联资产

- [主设计](./prepared-model-invocation-pre-dispatch-gate-v1.md)
- [实施计划](../../plan/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1.md)
