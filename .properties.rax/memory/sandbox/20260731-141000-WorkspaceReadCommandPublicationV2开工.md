# 2026-07-31 WorkspaceReadCommandPublicationV2 开工

用户在PR #78合并后，以exact基线
`3c00eb379e44d9fe1b4845a4303eaec80e21fb9e`正式授权Sandbox Owner实现
WorkspaceRead Command Publication V2 owner-local切片。

本轮冻结：

- 保持事实类型`WorkspaceReadCommandV1`，不制造CommandV2事实；
- Sandbox定义neutral Source current消费Port，保持对`tool-mcp`零import；
- neutral Source exact coordinate封`settlement` Owner、固定Kind、ID/revision/
  raw lowercase digest；Owner存在于Effect.Intent.Owners exact集合，payload
  digest使用`sha256:` lowercase，并与Effect/Prepared Provider闭合；
- Ensure只携exact Source coordinate；
- Source、Runtime Effect/Prepared与Workspace current都必须S1/S2；
- Command、immutable Publication、fresh OwnerCurrent由internal owner capability
  在SQLite v19同一事务写入；
- public interface与concrete SQLite Store均不得保留raw Command writer；
- Publication只存stable semantic closure/create-time水位；transient proof只进
  OwnerCurrent，refresh仅CAS同一Publication且不越过最初semantic NotAfter；
- 上游水位只形成EffectiveCreatedLower，Command/Publication保留真实owner
  commitNow，Created不进入stable Ref/digest/equality；
- v19 ledger只在Open/migration推进，Ensure事实事务不执行DDL或写ledger；
- v19 Open必须在`BEGIN IMMEDIATE`内重读version；existing v19零DDL strict
  verify，旧版只在v19 namespace全空时exact CREATE，partial namespace拒绝；
- v18/raw-only Command可historical exact Inspect，但V1 current/physical必须
  Fail Closed；
- v18 legacy零backfill，Unknown只bounded exact Inspect；
- Tool adapter、production Runtime Reader、Provider/physical read与product root
  继续NO-GO。

2026-08-01收口状态为
`implementation-software-test-yes / owner-local / uncommitted`。contract/ports、
kernel owner factory/internal capability、SQLite v19三事实/body-seal、history/
pointer、8-handle create/refresh、restart/lost-reply及fresh join均已落地。public
raw Command writer已移除，physical与completion必须经过Publication+OwnerCurrent
fresh资格；V1/raw-only Store不再能组成执行旁路。full ordinary/race、vet、
schema/import/no-bypass与diff门禁已通过，独立P0/P1审查已清零。

本事件仍只证明Sandbox owner-local Command Publication闭包；Tool neutral adapter、
cross-owner production composition、Provider、CorePack Executable与Console合同均未
因此解锁。
