# G6A Identity V1测试矩阵

状态：Identity V1、Owner-current V3/V4、`SessionCurrentReaderV4`及Harness P3 Assembler/InputCurrent Reader均完成实现与对应独立终审；P3最终代码审计为`YES(P0/P1/P2=0)`。Tool Consumer/P4、system G6A/G6B/root仍保持`NO-GO`。

本矩阵是Model Projection→SettledTurn DomainResult→Harness PendingAction→Committed PendingAction Request V3→未来Application Request→Tool V2 Watermark的统一反例事实源。任一反例命中时，Tool Watermark、ActionCandidate、Reservation、Gateway和Provider调用均必须为零；已经产生的历史Projection/DomainResult/Settlement只允许exact Inspect。

| # | 反例 | 预期 |
|---:|---|---|
| 1 | `Calls==0`或`Calls>1` | N=1门前Fail Closed；不创建Identity/PendingAction |
| 2 | Projection Ref任一ID/revision/digest/invocation/source字段漂移；SourceKey与Identity/DomainResult顶层Projection、ordinal、Candidate、SettlementOwner发生splice；或SourceKey scope/run/session/turn与Current发生跨Run/Session/Turn splice | `Conflict`；逐字段exact比较，零DomainResult/Request |
| 3 | Call ordinal缺presence、encoding version未知、Value不是0，或CallID/CallName与Projection唯一Call不一致 | `Conflict`；零Identity；不得用uint32零值冒充present |
| 4 | canonical arguments未对Model canonical bytes使用live `core.DigestBytes`，或结果不等于PendingAction Payload digest | `Conflict`；Owner自签摘要无效，零Settlement/Tool写 |
| 5 | Identity与PendingAction不在同一个`SettledTurnDomainResultFactV3` body/content digest，或Harness Session FactRef与Fact Reader漂移 | `PreconditionFailed`；Runtime只绑定schema+content digest，FactRef不伪装成Settlement字段且Runtime不settle |
| 6 | PendingAction SourceCandidate不等于SettledTurn Candidate | `Conflict`；Harness不CAS |
| 7 | 同稳定SourceKey换Projection内容、Call、PendingAction、Capability、Schema、TTL或digest，并试图按完整内容派生新ID | FactID/IdentityID/SourceKeyDigest三索引同一线性点`Conflict`；首记录不变 |
| 8 | DomainResult `EnsureExact`回包丢失后换Repository/切Reader猜结果，或Runtime Settlement回包丢失后换ID/内容重建 | 同Repository以同canonical Fact重调原子`EnsureExact`并compare；下游Reader只读，禁止新建 |
| 9 | Harness `SessionCASV3`回包丢失后只比较revision/PendingAction/Identity，或Inspect到的完整V3 successor在Session Digest/Revision、Binding.PendingAction、IdentityRef、DomainResultFactRef、ModelTurnSettlementRef任一字段不同 | `Conflict`；必须完整exact successor才可恢复，不得当幂等成功 |
| 10 | `RequestedNotAfter<0`、正数`<=fresh now`、正数晚于natural expiry却延长结果，或Candidate/Identity.NotAfter/Association/Generation/任一role Provider Binding/Route/Context/caller bound/30s任一上界在S1/S2间跨越 | 负数`InvalidArgument`；其余`PreconditionFailed`；0表示不增加上界，正数只能缩短；Model Projection/历史Settlement无TTL；零Request/Tool写 |
| 11 | 任一Reader Checked时间晚于调用方fresh time，或clock rollback | `Indeterminate`/`PreconditionFailed`；零写 |
| 12 | Application Request V2 Identity coordinate与Harness Current V2不同，或InputCurrent把Model/Harness/Context/Route/Generation+Binding/Authority压成汇总摘要/互换proof | versioned exact proof逐项`Conflict`；Coordinator不dispatch |
| 13 | test fixture直接Seal Request V2，未调用Assembler及S1/S2 | Conformance失败；不能计为system PASS |
| 14 | Tool V2未复读Identity，或在Watermark后才复读 | Conformance失败；Watermark必须为零 |
| 15 | V1 equality通过但缺Identity/current/Assembler链 | 只能算Tool局部拒绝门；系统G6A保持NO-GO |
| 16 | 64并发同内容发布得到多个revision/ref，或64个不同内容竞争同SourceKey同时成功 | 仅一个canonical successor；同内容幂等，不同内容63个Conflict |
| 17 | 从PendingAction payload、event JSON、compat call或Evidence反推Model Projection | `Forbidden`/Conformance失败；零写 |
| 18 | 以schema/default转换放宽摘要相等，或未审Transformation Fact进入N=1 | `InvalidArgument`；当前版本不支持 |
| 19 | V3 Create不是revision 1/creating，或`ApplicationBinding!=nil` | `InvalidArgument`/`PreconditionFailed`；零Create |
| 20 | 同完整Run/Scope+SessionID键Create同canonical、换内容、或已有V2同键事实 | 同canonical幂等；换内容Conflict；V2/V3共享冲突域且禁止隐式迁移 |
| 21 | V3 Create回包丢失后换键、换版本、换内容重建 | 只能按完整Run/Scope+SessionID exact Inspect；匹配首记录才恢复 |
| 22 | Session/CAS复用canonical domain/version/subject，CAS缺ContractVersion/Run/SessionID/ExpectedRevision/ExpectedDigest/Next/Digest任一字段，Request digest遗漏Next.Digest，Next revision不连续，或Run/ID变化 | `InvalidCanonicalForm`/`InvalidArgument`/`Conflict`；两套明确常量与字段集合必须exact，零CAS |
| 23 | `waiting_action`缺PendingAction或ApplicationBinding，二者不同；非waiting_action携Binding；或DomainResult schema/content不等于Execution.Settlement | `PreconditionFailed`/`Conflict`；一次CAS不得部分写入 |
| 24 | 将GovernedSessionV2翻译、默认填充或重摘要后冒充V3；或把“唯一新增转换”实现成V3只允许waiting_settlement→waiting_action，导致creating及V2既有合法转换不可达 | `Conflict`/Conformance失败；V2/V3无隐式迁移、无双写，V3继承V2既有合法转换 |
| 25 | Content body遗漏顶层完整ModelProjection Ref、把Projection压进Identity后省略顶层字段、或加入IdentityRef/ContentDigest/FactDigest | canonical/digest Conformance失败；IdentityRef只能在ContentDigest完成后派生 |
| 26 | 把SettlementOwner/Schema/Created编码进Content而非Fact envelope，或FactDigest未覆盖它们 | `InvalidCanonicalForm`/`InvalidDigest`；零Ensure |
| 27 | IdentityID或FactID使用另一方domain/subject/prefix、两ID相同、截前缀比较后缀或从一方推断另一方 | `InvalidReference`/Conformance失败；必须分别调用两个helper |
| 28 | active binding注入两个Repository capability、Fact/Identity/SourceKey索引分属不同实例、Reader切到另一实例，或Application定义/持有Repository | Owner/Binding Conformance失败；零写、零dispatch |
| 29 | `CommittedPendingActionSubjectV2`自行加入/默认`ContractVersion`，或Current只比较汇总摘要而未逐字段核完整Subject/ApplicationBinding | `InvalidCanonicalForm`/`Conflict`；不得type-pun |
| 30 | import `model-invoker/internal`、`execution/direct`、store/publisher/writer/event/provider类型，或使用Model写接口 | import-boundary/Conformance失败；唯一允许公开根包只读Ref/Projection/Reader |
| 31 | `Request.Validate`、`Current.Validate`、`ValidateAgainst`职责互换；`RequestedNotAfter==0`被当作过期；Checked>=Expires、rollback、expired projection或requested>0时Expires>requested仍通过 | `InvalidArgument`/`PreconditionFailed`；三组Validate与时间关系必须分别成立 |
| 32 | S1后未复读Model/DomainResult/Settlement等Owner、Owner复读后无S2、返回前无第二次fresh clock，或S2/时钟/TTL漂移 | Fail Closed；零Application Request/Tool写 |
| 33 | Store/Reader返回共享的PendingAction payload、Execution内部slice/指针、ApplicationBinding或Session嵌套对象，调用方可变更已存Fact | no-clone Conformance失败；所有跨Port指针/slice必须deep clone |
| 34 | `CommittedPendingActionOwnerCurrentInputsV1`被缩写/alias，或其与Binding V2、Session V4、Current V3复用旧version/domain/subject、遗漏字段/Digest，或Binding V2复制Base四字段形成双真值 | `InvalidCanonicalForm`/`InvalidDigest`；零V4 CAS/Current V3 |
| 35 | Session V4同时保存V1/V2 Binding，Reader V2返回Binding V2/Current V3，或V2/V3/V4间默认翻译、迁移、双写 | `Conflict`/Conformance失败；旧版本只读兼容，新版本显式选择 |
| 36 | RouteMatrix只携digest、字段不等于closed Action Matrix，或Route Reader返回另一Ref/matrix/projection | `Conflict`；必须以Binding内完整`(ref,matrix)`调用并逐字段exact |
| 37 | ModelTurnOperation非run，RunID/Scope digest/完整Scope与Session漂移，或Context adapter未复读Run/Session/Turn/Frame sealed subject | `EffectFenceStale`/Conformance失败；零Current V3 |
| 38 | role闭集漏Endpoint/Candidate/Identity/Route任一角色、加入扫描成员、binding不属Association exact set、Provider current Ref虽exact但BindingSetDigest/SemanticDigest被splice，或BindingSet五字段漂移 | `BindingDrift`；全部role-tagged ref及set digests exact current前零Current |
| 39 | Association按ID读回后未Fact Validate/Ref exact/active/fresh，向Generation Reader误传整个`Candidate.Generation` projection，或返回Projection与完整Candidate.Generation字段/digest/current漂移 | `Conflict`/`Expired`；必须以`Candidate.Generation.Generation` Artifact Ref读取，不得使用漂移Candidate内容 |
| 40 | Harness Current Reader构造器接收含Settle的Runtime Governance Port、以宽接口变量/adapter绕过窄能力、import非Runtime公开Port、自建私有Settlement Reader，或Inspect返回同ID换Attempt/DomainResult | capability/import/Owner Conformance失败；构造器只允许Runtime public `OperationSettlementCurrentReaderV3`并逐字段exact；Application后续才可持Governance Port |
| 41 | TTL min加入伪造Model/历史Settlement TTL，或漏Candidate/Identity/Association/Generation/任一role Binding/Route/Context/30s/caller bound | `PreconditionFailed`；只使用live真实expiry，取最小值 |
| 42 | V2/V3/V4 Session在同一完整Run/Scope+SessionID key并存，或V4 Store/Port不共享同一线性冲突域 | `Conflict`/Owner Conformance失败；只能存在一个版本的Session事实 |
| 43 | `SessionCASRequestV4`复用V3 version/domain/subject、缺ExpectedDigest/完整Next，或Create接受非rev1/creating/非nil Binding V2 | `InvalidCanonicalForm`/`InvalidArgument`；零V4 Create/CAS |
| 44 | V4 CAS正常返回或lost-reply Inspect到合法但非expected successor，或Binding Base/`CommittedPendingActionOwnerCurrentInputsV1`任一字段漂移仍按幂等成功 | `Conflict`；必须逐字段及canonical exact匹配完整`Next GovernedSessionV4` |
| 45 | Request V3的`RequestedNotAfter<0`、`==0`、`>0 && <=fresh now`或晚于natural expiry分别被误作可用/过期/可用/延长 | `InvalidArgument`、无额外上界、`PreconditionFailed`、仍取natural min；不得从V2猜语义 |
| 46 | `ValidateAgainst`只比较Current/Subject/Binding汇总digest，未逐字段exact比较Base Subject V2、Binding Base四字段和完整`CommittedPendingActionOwnerCurrentInputsV1` | `Conflict`/Conformance失败；canonical digest只辅助，零Current V3 |
| 47 | `CommittedPendingActionOwnerCurrentInputsV1`/Binding V2/Session V4/CAS Request V4/Subject/Request/Current V3的Clone/Seal共享ExecutionScope/SandboxLease、Settlement Evidence/Delegation/DomainResultSchema/PendingAction payload，或Reader注入依赖是typed nil | no-alias Conformance失败或`Unavailable/ComponentMissing`；不得panic、不得污染Store/Reader |
| 48 | `ContextApplicability.Kind`是run/session/turn/action或任意其他合法namespaced Kind，OwnerInputs只做通用Ref.Validate并继续调用Context Reader | `InvalidArgument`/Conformance失败；必须在OwnerInputs Validate及调用前双重exact锁死`OperationScopeEvidenceContextParentKindV3`，零Context Reader、零Current V3 |
| 49 | 一个类型只实现`InspectSessionV4`，不实现Create/CAS | 必须满足`SessionCurrentReaderV4`并通过compile/conformance；不得要求写能力 |
| 50 | 既有`SessionFactPortV4`实现保持原三个展开方法，通过嵌入Reader后重新编译 | 必须天然兼容；对象、digest、Store和方法集不变 |
| 51 | P3 Assembler构造器参数或字段为`SessionFactPortV4`、Store/fake具体类型、私有同形接口，或代码可选择Create/CAS | import/capability Conformance失败；构造器只能接收public `SessionCurrentReaderV4`，生产代码零Session写调用 |
| 52 | `SessionCurrentReaderV4`为nil或typed nil | `Unavailable/ComponentMissing`；zero Session/Fact/Model/Runtime read、zero Request Seal、不得panic |
| 53 | Assembler或InputCurrent Reader为nil receiver，或ctx为nil | fail closed；nil ctx在clock及所有Owner read前拒绝，zero Seal、不得panic |
| 54 | Current V3的S2 expiry大于S1，或S2收窄值未进入Request/Harness/InputCurrent proof | 扩大立即拒绝；不变/收窄可用且最终对象采用S2最短租约 |
| 55 | `RequestedNotAfter`为负数、零或正数 | 负数在clock/read前拒绝；零不增加上界；正数只能缩短Current/Authority自然最小值 |

## 分层测试

| 层级 | Owner | 必测 |
|---|---|---|
| Unit | Settlement Owner/Harness/Application | 两类ID helper、Content/envelope canonical、strict JSON、Current V3三组精确Validate签名、RequestedNotAfter、TTL、nominal type-pun、deep clone/Seal，以及`SessionCurrentReaderV4` nil/typed-nil零调用 |
| Whitebox | Harness | 单实例三索引Repository；DomainResult→Settlement(schema+digest only)→Session FactRef CAS/current Reader；ExpectedDigest；V2/V3/V4共享冲突域；V4继承V3 transition、`waiting_settlement→waiting_action`一次写完整Binding V2；create/CAS lost reply按完整Next exact恢复 |
| Blackbox | Application/Tool | Assembler/InputCurrent S1/S2与Request V2已闭合；Tool Identity reread、Watermark前Fail Closed留P4 |
| Fault | 各Owner | lost create/settle/CAS/read reply、Unavailable、Indeterminate、clock rollback、TTL crossing |
| Race | Harness | 64并发同内容幂等、不同内容单一线性化、三索引无ABA、并发读只见deep clone |
| Conformance | Assembly | Model import boundary、单Repository capability、Reader同实例、Owner/Binding/Generation/Route exact；Harness构造器仅依赖Runtime public窄Settlement Reader和Harness public `SessionCurrentReaderV4`，不接受Governance/Session FactPort、Store/fake具体类型、私有同形接口或raw bypass；只实现Inspect的Reader与既有FactPort均做compile-time assertion |
| System | `tool-mcp/tests/system` | 手工注入公开Ports完成最小G6A组合；不使用production root |
