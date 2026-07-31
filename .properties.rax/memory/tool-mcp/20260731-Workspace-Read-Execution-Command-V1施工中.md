# 2026-07-31 Workspace Read Execution Command V1 Ready PR #78

- Root确认P0：generic `SourceToolCommand`不能唯一连接Tool Request链与Runtime Attempt链。
- 新增Tool-owned nominal create-once command，位于任何Sandbox/Provider effect之前。
- Tool internal Owner Producer只接收expected exact坐标；Claim、State、Binding、Candidate、Runtime Effect和
  Prepared Current均由Owner readers派生并做S1/S2。
- Runtime Attempt reverse key采用完整canonical digest，包含Delegation与Permit，不使用
  AttemptID或指针相等。
- SQLite以command ID为主键，RequestKey、Tool ExecutionAttemptID、Runtime Attempt三条外部
  因果轴分别唯一；并发候选可有不同真实Created/digest，但只返回stable closure一致的winner。
- 历史Fact只保存稳定语义；Registry/Effect/Prepared的fresh current proof只进Current。
- Binding/Input是create-once固定winner，可以进入历史闭包，但目前仅InMemory，restart fresh
  current无法保证，必须Fail Closed。
- Contract、Owner Producer、SQLite及真实时钟、restart/lost-reply、64并发、splice门禁已闭合；
  focused ordinary×100、race×20、full ordinary/race、vet、diff与import均通过。
- owner-local Command已以ready PR #78发布，PR尚未合并。
- additive typed `WorkspaceReadSandboxRequestV2`/adapter仍pending；Start必须读取Command
  exact/current并做current S1/S2/S3，recovery只读exact历史，再映射Sandbox opaque
  raw-lowercase Ref。legacy V1仅reference/test-only，不得进入production composition。
- 当前切片不写Payload Source/Material、Classification、Context、Application、Runtime或
  Sandbox事实，不声明production。
- Payload Source/Material与Classification仍pending/NO-GO，不属于PR #78。
