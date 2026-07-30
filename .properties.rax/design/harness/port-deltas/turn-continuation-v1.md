# TurnContinuation V1公共合同Port Delta

## 1. 结论与范围

- 状态：`public_contract_candidate`；本PR只冻结Go Runtime内部跨Owner合同，不宣称Harness实现、真实Tool执行、production root或SLA完成。
- 目标顺序：`Settled ToolResult -> ContextTurnRefresh applied_current -> immutable ContextFrame -> Harness CAS ActiveContextRef -> next Model Turn`；本PR只冻结其中continuation CAS/recovery公共合同。
- 不属于Console合同：Runtime/Review/Timeline/Sandbox正文均未冻结，本PR不新增TypeScript ViewModel、Event或命令，不授权前端据此接线。
- 证明边界：它只co-seal两个independently sealed exact refs（settled ToolResult ref与Context prepare Attempt ref），不证明Context prepare内容来自SettledToolResult payload。在这两个ref已由调用方提供后，本合同只证明pending、applied-attempt防splice、ActiveContext CAS、Inspect recovery和Model前置门；不声称Application coordinator、Harness owner实现或完整真实Loop已形成。

## 2. Owner与公共对象

| 对象 | Owner | 本合同只消费/定义 |
|---|---|---|
| `SingleCallToolActionResultRefV2` | Tool/Application既有协调面 | 已settled的精确结果ref；不复制Tool DTO |
| `ContextTurnRefreshResultV1` | Context | 仅接受`applied_current`及既有Manifest/Frame/Generation/ApplySettlement/CurrentPointer精确ref |
| `TurnContinuationStartRequestV1` | Application协调 | 并列绑定Source Session/Turn、settled ToolResult、旧ActiveContext和预先冻结的Context prepare Attempt四元组；不证明Tool内容到prepare内容的语义派生 |
| `TurnContinuationCurrentV1` | Harness | `continuation_pending`或`context_current`的Owner-local current事实 |
| `HarnessActiveContextRefV1` | Harness | Session内活动不可变ContextFrame的CAS值 |
| `TurnContinuationPortV1` | Harness实现、Application调用 | `Begin`、`Commit`、`Inspect`三个公共方法 |

Application只排序和绑定精确坐标，不取得Tool、Context或Harness领域事实所有权。精确ref也不授予currentness或执行权限。

## 3. 固定顺序与P0绑定

```text
1. Application取得settled SingleCallToolActionResultRefV2
2. 由稳定Tool/Session/Turn/ActiveContext坐标派生唯一TurnContinuation AttemptID
3. Context prepare request使用该AttemptID完成seal
4. Begin前把prepare的 Kind + ID + Revision + Digest
   冻结为ExpectedContextRefreshAttempt
5. Harness Begin原子发布continuation_pending；此时ActiveContext仍是旧值
6. Context Owner在自己的事务内Apply，返回applied_current
7. Harness Commit要求AppliedContextRefresh.AttemptRef
   与ExpectedContextRefreshAttempt四元组完全相等
8. Harness在一个Owner-local事务内CAS旧ActiveContextRef为目标Frame，
   同时发布context_current
9. 只有ModelTurnAllowedV1通过后，才允许下一Model Turn
```

只匹配Attempt ID或Revision不够：同ID/Revision但不同prepare digest必须拒绝，避免把另一份Context请求的applied结果splice进当前continuation。

Start的canonical digest只防止Begin后的坐标替换；它不是`SettledToolResult -> Context prepare content`的派生证明。该内容绑定必须由后续Application coordinator合同与真实纵切测试单独闭合。

## 4. 状态、不变量与恢复

- `continuation_pending`：必须仍持有`ExpectedActiveContext`，不得带applied refresh，下一Model Turn一律拒绝。
- `context_current`：revision必须从1变2，`PreviousDigest`必须绑定pending事实，ActiveContext revision只加1，Turn只加1，并精确复制applied结果的Manifest/Frame/Generation/CurrentPointer refs。
- ContextFrame只作为不可变精确ref切换；Harness不得改写Frame内容。
- Begin或Commit出现未知/丢回包后，只能使用原`TurnContinuationAttemptRefV1`调用Inspect；不得生成新Attempt或盲目重发。
- Attempt ID不含TTL，因此currentness窗口变化仍落在同一Attempt ID；不同请求digest会冲突并迫使Inspect原Attempt。

## 5. 事务语义

这不是跨Tool、Context、Harness的分布式事务：

- Tool只在自己的settlement边界提交结果；
- Context只在自己的Apply settlement + generation-current CAS事务内提交Frame；
- Harness的Begin与`ActiveContextRef + continuation state` Commit分别是Harness-local原子事务；
- Application崩溃恢复依赖各Owner的exact Inspect和同一Attempt，不使用跨库rollback，也不把任一Owner事实伪装为另一Owner事实。

## 6. 当前已知阻塞

### 6.1 Context零Source不兼容

当前`ContextTurnRefreshPrepareRequestV1`与coordinator都要求`Memory != nil || Knowledge != nil`。因此`Tool=1 / Memory=0 / Knowledge=0`不能通过现有candidate。本PR不修改该充分性语义；需要Context Owner单独冻结零Source时的Frame/Manifest/Generation语义后再开delta。

### 6.2 永久失败/终止状态未冻结

V1只有`continuation_pending`和`context_current`，没有永久失败或abort terminal。Context永久失败时，Harness只能保持fail-closed并按原Attempt Inspect；它不能恢复旧Session phase、开启下一Turn或自行宣告Run终止。该出口必须由后续Failure/Run termination公共合同冻结，当前两状态合同不能解读为完整可运行Loop。

### 6.3 Sandbox Coding Execution / workspace.read

本PR不伪造`workspace.read`执行。当前Sandbox有`WorkspaceView`和写/commit actual-point，但没有公开的有界read Port；#23已加固Unix socket路径回归测试，但production host plane与actual-point read仍未形成。

下一PR的最窄Port必须至少精确绑定：

1. 复用Tool `WorkspaceReadInputV1`与`WorkspaceExactRefV1{ID,Revision,Digest}`，不造第二套read输入；
2. 将WorkspaceRoot精确映射到Sandbox `WorkspaceView.Meta.Ref()`，并在actual point重新验证`Meta` current；
3. 同时绑定`BaseArtifactRef`、`BaseRevision`、`OverlayRef`、`PolicyRef`、`RuntimeLeaseBinding{InstanceEpoch,LeaseEpoch,FenceEpoch,ScopeDigest,ObservedRevision,Expires}`和`FileScopeDigest`；
4. 路径必须命中canonical `ReadScopes`且不命中`HiddenScopes`，`StartByte/MaxBytes/RequestedNotAfter`保持原界限；
5. read receipt至少绑定request digest、WorkspaceView exact ref、lease/fence坐标、relative path、byte range、observed length/content digest和actual-point inspection ref；
6. 真正读取只能发生在Sandbox Coding Execution actual point，未知结果按原Attempt Inspect，不能以Go fake或Tool catalog冒充执行成功。

## 7. 本PR验收门

- 单元：pending/final seal、严格Frame CAS、Model前置门；
- fault：prepared-only、Frame splice、同ID/Revision不同Context Attempt digest全部拒绝；
- currentness：checked/expires边界fail-closed；
- black-box：Commit丢回包后只Inspect原Attempt，Begin/Commit调用数不增加；
- race/vet/diff-check通过；#24已修复旧AgentActivation candidate flags。rebase到`origin/main@13f63712`后，Application full ordinary/race/vet、Context full ordinary/vet、Harness相关ordinary/race/vet均通过。
- 上述验收只支持continuation CAS/recovery合同，不构成完整真实Loop或ToolResult到Context prepare内容绑定证据。
