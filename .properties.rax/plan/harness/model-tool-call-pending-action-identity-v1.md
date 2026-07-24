# Harness G6A Model Tool Call → PendingAction Identity V1实施计划

## 1. 状态

状态：第二独立设计短审、Owner-current Port Delta及对应V3/V4实现、Harness P3 Assembler/InputCurrent Reader最终独立代码审计均为`YES(P0/P1/P2=0)`。H-ID-P0/P1/P2/P4与Harness Owner-local Phase1/2完成；Tool Consumer/P4、system fixture及G6A/G6B/production root保持`NO-GO`。

事实源：

- [Identity主设计](../../design/harness/assembly/model-tool-call-pending-action-identity-v1.md)
- [统一冻结反例矩阵](../../design/harness/assembly/model-tool-call-pending-action-identity-v1-test-matrix.md)
- [Identity链路图](../../design/harness/assembly/model-tool-call-pending-action-identity-v1.drawio)
- [Application V2设计](../../design/application/single-call-tool-action-v2.md)
- [Application V2计划](../application/single-call-tool-action-v2.md)

## 2. 预期产物

联合评审通过后的实现产物固定为：

1. Settlement Owner拥有的稳定`IdentitySourceKeyV1`、FactID/IdentityID/SourceKeyDigest三唯一索引、`ModelToolCallPendingActionIdentityV1`及同一`SettledTurnDomainResultRepositoryV3.EnsureExact/InspectExact` capability；
2. Harness `PendingActionApplicationBindingV1`、`GovernedSessionV3/SessionCASV3`与`CommittedPendingActionCurrentV2`/Reader；
3. Application Request/InputCurrent/Identity Reader/Assembler Port V2；
4. Harness-owned Application Assembler Adapter；
5. Tool V2 Consumer pre-watermark Identity reread；
6. owner-local测试和`ExecutionRuntime/tool-mcp/tests/system/g6a_identity_v1_test.go`测试组合。

## 3. 顺序与Owner独占

```text
Model Owner：Projection exact Reader保持只读
-> Model-turn Settlement Owner：Identity + PendingAction同DomainResult
-> Runtime Owner：exact Settlement
-> Harness Owner：Session CAS + Current V2 Reader
-> Application Owner：neutral V2 contracts/coordinator
-> Harness Owner：Assembler Adapter
-> Tool Owner：V2 consumer
-> Integration Owner：test-only system fixture
```

任何Owner不得跨目录代写另一Owner代码。Harness Assembly只校验注入Port/Binding/Generation/Route，不实现Tool、Context、Runtime或production root。

## 4. 工作包

### H-ID-P0：合同冻结

- [x] 审核稳定SourceKey、presence/versioned ordinal、Identity/Ref字段、三唯一索引、独立ID canonical domain/subject/prefix、strict JSON和deterministic ID；
- [x] 审核SourceKey与Identity/DomainResult顶层Projection、ordinal、Candidate、SettlementOwner逐字段exact相等及splice反例；
- [x] 审核`SettledTurnDomainResultFactV3/Ref/Repository/Reader`与V2兼容；
- [x] 审核canonical arguments只对Model canonical bytes使用live `core.DigestBytes`；
- [x] 审核NotAfter与historical truth分离；
- [x] 审核无Transformation/N>1隐式扩展。
- [x] A：冻结`GovernedSessionV3/SessionCASRequestV3`完整字段、两套独立canonical domain/version/subject与Digest字段集合、ExpectedDigest、Create/Inspect/CAS语义、完整Run/Scope+SessionID键、V2/V3共享冲突域及V3继承V2既有合法转换；
- [x] B：冻结DomainResult Content body为Candidate+顶层完整Projection Ref+PendingAction+完整Identity，并排除IdentityRef/ContentDigest/FactDigest；
- [x] C：冻结IdentityID/FactID独立derive helper、domain、subject、prefix、ID必须不同及禁止后缀推断；
- [x] D：确认`CommittedPendingActionSubjectV2`保持无ContractVersion；
- [x] E：Model依赖只允许公开根包的Projection full Ref/public exact Reader，禁止internal/execution/direct/store/publisher/writer/event/provider与payload/event反推；
- [x] F：冻结公共类型位于Harness contract、唯一Repository写口位于Harness ports、exact SettlementOwner provider adapter单实例三索引及Application零Repository；
- [x] G：冻结RequestedNotAfter的0/负数/正数语义、三组Validate签名及fresh S1→Owner复读→S2→返回前fresh clock顺序。

H-ID-P0中央冻结值已经全部落盘并通过第二独立设计短审；[Owner-current exact输入Port Delta](../../design/harness/port-deltas/committed-pending-action-owner-current-inputs-v2.md)及对应V3/V4实现也已通过最终独立代码审计。旧Binding V1/Session V3保持兼容，新增Binding V2/Session V4/Current V3闭合Owner-current读取；Repository三键单线性、S1→全部Owner复读→S2/30秒cap、全链exact字段与deep clone均已进入实现与反例门。

### H-ID-P1：Harness Current V3

- [x] Reader按完整RunRef/重算Scope digest/Session/Turn/Settlement Subject exact复读Session、Projection、DomainResult Fact、Settlement、Association、Generation、Route、10-role Provider Binding与Context；
- [x] Identity/PendingAction/SourceCandidate及Model唯一Call的ordinal/CallID/Name/canonical arguments逐字段交叉验证；
- [x] SourceKey scope/run/session/turn与Current完整RunRef/Session/Turn逐字段exact，跨Run/Session/Turn splice拒绝；
- [x] Current Request严格执行0=不增加上界、负数非法、正数只缩短；Reader自产Checked/Expires，S1→Owner复读→S2→返回前fresh clock，natural TTL最小值与30秒cap生效；
- [x] `SessionCASV4`沿`waiting_settlement/reconciling→waiting_action`一次原子写`PendingActionApplicationBindingV2`；lost reply只接受完整V4 exact successor；
- [x] typed-nil、clock rollback、TTL crossing、valid owner splice、10-role分组单读、no-alias与并发反例通过。

### H-ID-P2：Application Assembler

- [x] `SessionCurrentReaderV4`通过独立设计短审并在Harness public ports additive落盘；`SessionFactPortV4`兼容嵌入且展开方法集不变；
- [x] P3 Assembler构造器静态只接收`SessionCurrentReaderV4`，不接收`SessionFactPortV4`、Store/fake具体类型或私有同形接口；typed nil为`Unavailable/ComponentMissing`且zero read/zero Seal；
- [x] Application V2 public contract/ports稳定compile后实现`single_call_tool_action_assembler_v2.go`；
- [x] 实现Application Request Assembler与public `SingleCallToolActionInputCurrentReaderV2`；
- [x] 直接复读Session/DomainResult/Model/CurrentV3/Authority，Context/Route/Generation/Binding/Settlement current由CurrentV3聚合并exact承载；
- [x] 固定`S1→S2→fresh nowS2→single Request/Proof Seal`，S2租约不得扩大；
- [x] no raw payload/no Owner struct/no direct dispatch；唯一Projection bytes来自Model exact Reader并deep-copy；
- [x] Assembler和Reader通过compile/import/capability Conformance；实际Assembly PortSpec与production wiring留给P4/system，不使用万能Hook。

### H-ID-P3：Tool消费与系统组合

- [ ] Tool V2在Watermark前复读Identity；
- [ ] Identity ref进入command/watermark digest；
- [ ] V1 equality保留但system gate固定不完整；
- [ ] system fixture只手工注入公开Ports；
- [ ] fixture直接Seal Request必须失败；
- [ ] Context/Continuation/Turn/Capability计数为零。

### H-ID-P4：Governed Session V3/V4落点与迁移

- [x] 完整V3镜像、独立canonical常量与Digest字段集合已实现；V4以单一Binding V2替换旧Binding字段，不产生双真值；
- [x] V2/V3/V4共享完整Run/Scope+SessionID冲突域，Create/Inspect/CAS/lost-reply均按exact版本对象执行；
- [x] ports只暴露版本化Session Create/Inspect/CAS，不暴露Store内部句柄；
- [x] thread-safe fake覆盖lost reply、immutable replay、反向占键、ABA与deep clone；
- [x] `waiting_settlement/reconciling→waiting_action`一次CAS写完整Binding V2并证明本地attempt lineage；Owner Fact复读仍由Current Reader独立完成；
- [x] V2/V3保持历史兼容且不能冒充V4 system identity complete。

## 5. 测试要求

| 类型 | 要求 |
|---|---|
| Unit | SourceKey/三索引、两类独立ID helper/domain/prefix、presence ordinal、`core.DigestBytes`、Content/envelope分层、三组Validate、strict JSON、真实TTL、nominal refs、N=1；新增Reader nil/typed-nil零调用 |
| Whitebox | 同实例Repository DomainResult Fact Ensure/Inspect、Runtime Settlement schema+digest绑定、SessionCASV3 ExpectedDigest/atomic FactRef binding/current Reader、V2/V3共享冲突域 |
| Blackbox | Assembler S1/S2、Tool pre-watermark reread、G6A硬停 |
| Fault | lost reply、Unavailable、Indeterminate、clock rollback、TTL crossing |
| Race | 64并发同/不同内容，仅一个canonical successor |
| Conformance | Owner/import/PortSpec/Binding/Generation/Route exact、无raw bypass；只实现Inspect的`SessionCurrentReaderV4`可编译，既有FactPort天然兼容，P3静态不可见Create/CAS |
| System | `tool-mcp/tests/system/g6a_identity_v1_test.go`，公开Ports、test-only |

统一执行[冻结反例矩阵](../../design/harness/assembly/model-tool-call-pending-action-identity-v1-test-matrix.md)。

## 6. 完成门

1. Runtime/Model/Harness/Application/Tool联合设计评审YES；
2. V1对象与摘要未改；
3. Identity/PendingAction同DomainResult，Harness CAS恢复精确；
4. Application Request只能由Assembler系统路径产生；
5. Tool在Watermark前复读Identity；
6. owner-local与system测试全部通过；
7. 无production root、Provider能力启用、Context/Continuation/Turn推进；
8. Future Transformation Fact继续后置。
