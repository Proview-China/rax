# Application SingleCallToolAction V2测试矩阵

状态：**owner-local design终审 YES；Application owner-local P2代码第四独立终审 YES（P0/P1/P2=0）**。CAS schema、本地schema/version/context kind与反例已闭合，targeted ordinary100、race20及full ordinary/race/vet全绿。ToolResult权威current与可信生产提交时钟属于Tool P4/system门，不由P2 FactPort fixture证明；Tool P4、P5跨模块fixture、系统G6A与production composition root继续`BLOCKED`。

统一18项跨Owner反例仍以[Harness Identity测试矩阵](../harness/assembly/model-tool-call-pending-action-identity-v1-test-matrix.md)为来源；下表补足Application additive V2门。

| ID | 级别 | 用例 | 预期 |
|---|---|---|---|
| APP-V2-01 | P0 | 把BindingV2/SessionV4/Subject或CurrentV3塞进ReaderV2 | 装配fail closed，零Tool调用 |
| APP-V2-02 | P0 | BindingV2 Base四事实任一漂移 | Conflict，零Request |
| APP-V2-03 | P0 | OwnerInputs五项、HarnessContractVersion/HarnessDigest或BindingVersion/HarnessBindingDigest漂移 | Conflict；不得重算“等价”摘要 |
| APP-V2-04 | P0 | Application DTO携Harness struct、非Model exact Reader来源的bytes或import Owner实现 | AST/import/strict contract失败 |
| APP-V2-05 | P0 | 从IdentityRef猜Created/Owner/Call/NotAfter，未调用Fact Owner Reader | fail closed，零Request |
| APP-V2-06 | P0 | Fact Reader或Model `InspectExactProjectionV1`返回Ref/SourceKey/Projection/Candidate/PendingAction/Owner任一漂移，Calls!=1/ordinal!=0；canonical arguments为空但digest非空、超过`runtimeports.MaxOpaqueInlineBytes`、重序列化，或输入/返回slice修改污染sealed proof | Invalid/Conflict，零Request；Reader Adapter/Seal/Clone/返回路径deep-copy且digest只取original reader bytes |
| APP-V2-07 | P0 | Fact.Created不等于Identity.Created，或Created不小于NotAfter | PreconditionFailed |
| APP-V2-08 | P0 | Action V2出现旧Workflow/Observation/Assembly/Route/ParentFrame/Applicability字段 | strict decode失败 |
| APP-V2-09 | P0 | Action Scope与Run/ModelOperation不exact，或Subject/Binding/Identity splice | Conflict，零Authority/Tool |
| APP-V2-10 | P0 | Authority缺失、scope/epoch/ref漂移、ActionScopeDigest不等于Action digest、过期/撤销 | S1/S2 fail closed |
| APP-V2-11 | P0 | Authority倒灌Harness Binding，或Action含Authority/TTL/Request ID | contract/canonical失败 |
| APP-V2-12 | P0 | Policy进入Request TTL但没有versioned ref/proof/input | contract失败；必须另立Delta |
| APP-V2-13 | P0 | Identity缺Created、ordinal presence/version或SourceKey顶层字段发生splice | Invalid/Conflict |
| APP-V2-14 | P0 | Runtime Settlement被伪装成携DomainResult FactRef | Conflict；FactRef只取BindingV2并经Fact Reader复读 |
| APP-V2-15 | P0 | Request/Result/ResultCoordinate ID使用错误domain/prefix/discriminator | InvalidReference/Conflict |
| APP-V2-16 | P0 | Application重派Tool Owner Result ID/revision/digest，或Tool/Application Result双身份type-pun | strict Owner ref/ContractVersion拒绝 |
| APP-V2-17 | P0 | Tool Owner ref、Tool Inspect、Application Coordinate、Inspection、Association的Settlement/Operation/Effect/DomainResult不exact | Conflict，不能返回/完成 |
| APP-V2-18 | P0 | ToolResult Schema/Payload或Inspection scope与Request Action不exact | Conflict |
| APP-V2-19 | P0 | Coordinator未用fresh clock完成Result/Settlement/Association复验并由完整Result派生Ref，或completed Next绑定另一Request/Action | Conflict，不能completed；FactPort只验structural exact Next，不冒充Tool Owner current证明 |
| APP-V2-20 | P0 | Create absent / same canonical / same ID换内容 | absent写一次；same exact幂等；changed content Conflict |
| APP-V2-21 | P0 | Create回包丢失 | 只Inspect Scope+ID并exact compare，不二次创作 |
| APP-V2-22 | P0 | V1/V2不同Request ID但同scope/run/session/turn/PendingAction语义；ClaimedActionVersion为unknown/wrong version；ConflictKey不是从Request重算或发生splice | 同一Owner原子CrossVersionKey+VersionClaim拒绝；zero write/zero Execute；V1 system route调用计数0 |
| APP-V2-23 | P0 | dispatch_intent→waiting_inspect CAS尚未exact成功就Execute | execute/provider调用计数0 |
| APP-V2-24 | P0 | StartClaim CAS lost reply/Conflict/Unavailable/Indeterminate或Inspect已waiting_inspect | 永久Inspect-only，Execute计数0 |
| APP-V2-25 | P0 | 64并发StartClaim | 唯一CAS成功回包持有一次Execute权，其余Inspect-only |
| APP-V2-26 | P0 | Claim与Fact不同原子线性点，或CoordinationID/Fact.ID/Request.ID、Claim.CoordinationDigest/initial prepared Fact.Digest、Claim.Created/Fact.Created/Request.Created任一不exact；Create/Inspect/CAS遇到Claim缺失/漂移 | Conflict/Unavailable，zero write/zero state transition/zero Execute；Claim digest永久绑定revision=1 prepared Fact，不与current Fact digest比较 |
| APP-V2-27 | P1 | RequestedNotAfter小于0、等于0、大于0 | `<0` Invalid；`0`无caller cap；`>0`只缩短 |
| APP-V2-28 | P1 | S1/S2间Session/Binding/Fact/CurrentV3/Authority变化 | Conflict/PreconditionFailed，零Tool command |
| APP-V2-29 | P1 | S2前先Seal、S2后补时间/二次Seal、clock rollback、Created非fresh nowS2、Created>=Expires或途中跨TTL | 拒绝；只允许S2后计算时间并Seal一次 |
| APP-V2-30 | P1 | Application重算Harness内部TTL或扩大CurrentV3/Authority/caller窗口 | PreconditionFailed；Expires只取三者min |
| APP-V2-31 | P2 | 直接Seal Request作为system fixture输入，或不存在root却宣称GO | system/acceptance失败 |
| APP-V2-32 | P2 | Tool Binding public Port Delta未闭合却启用P4/system，或N>1、Context Refresh、Continuation、Turn、Capability、Checkpoint调用非零 | G6A硬停失败 |

实施后最小门：contract/whitebox/blackbox/fault/conformance；targeted count100、race20；64并发；full ordinary/race/vet/gofmt/diff/import。最小system fixture落点保持`ExecutionRuntime/tool-mcp/tests/system/g6a_identity_v1_test.go`，但当前不得实施。
