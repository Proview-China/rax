# MCP Cancel、Inspect、Drain与Close受治理边界V1候选

状态：Tool/MCP Owner候选，等待用户与Runtime/Application联合冻结；当前Go门关闭。

精确Runtime additive字段、Tool active-call current、Drain落点及实施顺序见
[Controlled MCP Lifecycle Port Delta V1](controlled-mcp-lifecycle-port-delta-v1.md)。live审计另发现
Discovery Page公开矩阵尚未进入Runtime通用Matrix/Policy subject注册闭集；该既有缺口必须在
Cancel/Close新增前由Runtime Owner修正，Tool不私建兼容注册表。

## 1. 当前裁决

四个词不能共用一个“生命周期操作”接口：

| 动作 | 是否外部Effect | 当前可复用能力 | V1裁决 |
|---|---:|---|---|
| 本地Inspect | 否 | exact Command/Receipt/Connection/Snapshot Reader | 已实现；NotFound不证明未执行 |
| 普通Call Cancel | 是 | official SDK以原Call context发送JSON-RPC cancellation | 新Runtime Action矩阵与actual-point Port；不等于Effect未发生 |
| Drain | 否 | Tool Owner Connection current/CAS与活跃Attempt索引 | 只关闭新准入并等待现有Attempt可解释；不接触Transport |
| Close | 是 | official SDK Session `Close()` | 新Runtime Run/Session矩阵与actual-point Port；不回滚历史Call |
| Task Inspect/Cancel | 是 | official SDK v1.6.1无可复用Task public nominal | 保持NO-GO，不用私有字符串状态代替 |

live official SDK v1.6.1代码确认：`ClientSession.CallTool(ctx, ...)`在ctx取消时由底层发送
`notifications/cancelled`；该通知可能被Server忽略，迟到响应也不应被当作成功取消。Tool
owner-local Conformance已用真实in-memory SDK Session证明“admission后取消→Unknown→同key重投
只Inspect、Provider调用仍为1”。这只是标准行为证据，不是受治理Cancel写口。

上游依据：[MCP Cancellation 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/cancellation)、
[MCP Tasks 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/tasks)、
[official Go SDK](https://github.com/modelcontextprotocol/go-sdk)。Task取消必须走`tasks/cancel`；
普通context cancellation不能冒充Task取消或终态事实。

普通`tools/call`没有标准远端结果查询能力。丢Call回包后，本地Inspect找不到Receipt时仍是
Unknown；不得把Cancel、Close、断流或NotFound升级成`confirmed_not_applied`。

## 2. Owner边界

| 对象 | Owner | 非Owner限制 |
|---|---|---|
| Cancel/Drain/Close Intent、Receipt、Connection lifecycle、活跃Attempt索引、Residual | Tool/MCP | 不写Runtime Outcome/Review/Authority |
| Operation、Intent/Permit/Begin、Evidence、Enforcement、Settlement | Runtime | 不决定MCP领域结果 |
| Cancel/Close跨域调度及原Attempt关联 | Application Coordinator | 不直接写Tool或Runtime事实 |
| ordinary cancellation context handle、official SDK Session、actual transport close | Tool physical executor | 不导出raw handle/Session |
| Review/Identity/Authority/Scope/Budget/Fence | 对应治理Owner与Runtime Gateway | Tool只能读取public current投影 |

## 3. 普通Call Cancel

### 3.1 Tool领域Intent

候选`MCPOrdinaryCallCancelIntentV1`必须包含：

- `ContractVersion, Ref{ID,Revision=1,Digest}, Owner`；
- 原`MCPExecutionCommandRefV1`、`OperationDispatchAttemptRefV3`、Delegation、Prepared Attempt；
- exact Connection Fact/Connection Epoch、Capability Snapshot Ref、JSON-RPC Request ID；
- `CancelReasonDigest`，不保存不受限原因文本；
- Cancel自身`OperationSubjectV3/OperationDigest/EffectID/Revision/IntentDigest/Attempt`；
- `CreatedUnixNano, NotAfterUnixNano`。

ID只从原Command/Attempt与Cancel Operation稳定坐标派生，不含fresh时间。Intent create-once；
same ID changed reason/target/attempt必须Conflict。它不授执行权。

### 3.2 Runtime Port Delta

候选矩阵：

| OperationScopeKind | EffectKind | PolicyProfile | Run | Session | Turn | Action | Context |
|---|---|---|---|---|---|---|---|
| `run` | `praxis.mcp/cancel` | `praxis.mcp/ordinary-call-cancel-v1` | required | required | required | required | required |

Runtime需新增中立`ControlledMCPOrdinaryCallCancelPhysicalAuthorizationV1`与
`ControlledMCPOrdinaryCallCancelPhysicalPortV1`，绑定Cancel Intent、原Command/Attempt、当前
Connection/Epoch、原JSON-RPC Request ID、prepare/execute Enforcement、两项Evidence Handoff、
Provider/Transport及统一NotAfter。Tool不得包装Action V2/V3或Connect/Discovery Port。

actual point必须由同一Tool executor持有原Call的取消handle，在自身fresh clock下复读
authorization、Cancel Intent、原Command/Attempt和Connection current后，原子create-once admission，
再触发official SDK cancellation。handle不存在、原Call已terminal、Attempt/epoch漂移或TTL过期均零写
Provider。取消通知回包丢失后只Inspect同Cancel Entry；绝不重新发送或新建Cancel Attempt。

### 3.3 Receipt与结论

`MCPOrdinaryCallCancelReceiptV1`只证明取消请求在何时被actual point接纳/发送，以及SDK/Transport
观察到的有限结果。它不能证明原工具副作用未发生。原Call仍由原Command/Attempt的Observation、
Tool DomainResult与Runtime Settlement闭合；Cancel自身另行Settlement。

## 4. Drain

Drain是Tool Owner本地准入状态，不是Provider调用。候选`MCPConnectionDrainFactV1`绑定：

- exact `MCPConnectionFactRefV2`、Connection Epoch、current Availability与Snapshot Ref；
- expected current lifecycle Ref、reason digest、Requested/Created/NotAfter；
- 状态闭集`requested|draining|drained|residual`及canonical Digest。

create/CAS后，所有新的Connect复用、Discovery Page和Call在Tool actual point前都必须复读Drain
current；`draining|drained|residual`时零Provider。`drained`只在活跃Attempt集合为空且所有已开始
Attempt均已有terminal/Unknown+Residual解释时形成。Unknown Attempt不能被Drain抹掉；等待超时进入
Residual而不是伪造drained。Drain不调用official SDK、不关闭Session、不修改Runtime Outcome。

## 5. Close

候选矩阵复用Connect的Run/Session维度形状但使用独立类型：

| OperationScopeKind | EffectKind | PolicyProfile | Run | Session | Turn | Action | Context |
|---|---|---|---|---|---|---|---|
| `run` | `praxis.mcp/close` | `praxis.mcp/run-connection-close-v1` | required | required | forbidden | forbidden | forbidden |

`MCPConnectionCloseIntentV1`绑定exact Connection Fact/Epoch、Drain Fact（除紧急policy明确允许外必须
`drained`）、initialized Session Binding、Server/Transport/Provider、Close自身Operation/Attempt及
NotAfter。Runtime需新增独立Close matrix、authorization和physical Port。actual point fresh复读后
create-once admission并直接调用同一official SDK Session的`Close()`；不得验证后转发给raw Session
wrapper形成新的调度间隙。

`MCPConnectionCloseReceiptV1`只记录本地Session/Transport关闭Observation。Streamable HTTP的DELETE、
stdio进程退出或Close error按有限摘要保存；回包丢失进入Unknown并Inspect原Close Entry。Close成功后
Tool Owner独立Inspect/CAS新的Connection lifecycle事实；旧Epoch迟到消息只进历史。Close不声称远端
Effect回滚，也不删除Descriptor、Snapshot、Command、Receipt或Residual历史。

## 6. SDK、API与CLI

- 已实现的Command/Receipt/Connection/Snapshot exact Inspect继续可用；NotFound保持未知语义；
- Go SDK未来`Cancel`/`Drain`/`Close`只能消费Application/Runtime公共受治理Port，不接受raw Session；
- API写请求必须携Idempotency Key、exact target Ref、CAS revision与caller deadline；
- CLI `mcp cancel|drain|close`在对应公共Port、composition root和联合Conformance前继续unsupported；
- Task Inspect/Cancel等待官方SDK/标准public nominal，不与普通Cancel复用。

## 7. 恢复序列

```text
Cancel: persist Intent -> Runtime governance -> actual-point cancel -> Receipt/Observation
        -> Tool Inspect -> Cancel DomainResult -> Runtime Settlement -> Tool Apply

Drain:  persist/CAS Drain -> reject new admission -> inspect active Attempts
        -> drained OR Residual

Close:  require Drain/current -> persist Close Intent -> Runtime governance
        -> actual-point Session.Close -> Receipt/Observation -> Tool Inspect/CAS lifecycle
```

任何Provider边界之后的Unavailable/Indeterminate均不等于NotFound。恢复只使用原Intent/Entry/Attempt；
不续租旧Permit，不换JSON-RPC Request ID，不建立新Connection Epoch掩盖旧Unknown。

## 8. 硬反例

1. Cancel Intent未持久或Runtime Permit/Enforcement/Evidence任一缺失仍取消：零Provider；
2. 用Cancel成功生成原Call `confirmed_not_applied`：拒绝；
3. 原Call handle/Attempt/Request ID/Connection Epoch漂移仍发送取消：拒绝；
4. Cancel回包丢失后重复发送notification：拒绝；
5. 用Task cancel替代普通cancel，或反向替代：拒绝；
6. Drain直接调用Session Close：拒绝；
7. 有active/Unknown Attempt仍标记drained：拒绝并保留Residual；
8. draining/drained后新Discovery/Call仍进入actual point：Provider次数为0；
9. 未drained且无紧急policy证明即Close：零Transport；
10. Close复用Connect/Action/Discovery矩阵或Port：拒绝；
11. Close回包丢失后重新Close或创建新Epoch：只Inspect原Entry；
12. Close删除历史Command/Receipt/Snapshot/Residual或声称回滚：拒绝；
13. nil/canceled context、typed-nil reader/clock/session、clock rollback、TTL crossing：零Provider；
14. 64同canonical Cancel/Drain/Close竞争：单Intent、单admission、最多一次physical动作；
15. 同ID换target/reason/epoch/session：Conflict；
16. Application/Harness/Runtime import Tool实现，或Tool import其kernel/fakes/internal：拒绝。
17. official SDK普通Call在admission后context取消返回错误：Entry必须为Unknown，同key重投
    Provider调用保持1；不得产出Receipt或`confirmed_not_applied`。

## 9. 当前P0与实施门

| 缺口 | Owner | 状态 |
|---|---|---|
| Cancel Action矩阵、public physical authorization/Port | Runtime | P0，未设计终审/未落Go |
| Close Run/Session矩阵、public physical authorization/Port | Runtime | P0，未设计终审/未落Go |
| Discovery Page矩阵进入通用Validate/Policy subject注册闭集 | Runtime | P0，live public类型存在但通用注册缺失 |
| 原Call cancellation handle与active Attempt exact index的持久/恢复语义 | Tool/MCP + Runtime | P0，需联合终审 |
| Cancel/Close跨域调度与Settlement编排 | Application | P0，production root不存在 |
| Task public nominal与official SDK支持 | Model Context Protocol/SDK | external P0，保持NO-GO |
| Drain local Fact/Store/Reader | Tool/MCP | 候选可实现，但须与Cancel/Close时序一并终审 |

在上述P0归零前，只允许现有exact Inspect与测试fixture；Cancel/Drain/Close写入口、真实Provider能力、
CLI/API写命令和production启用均NO-GO。
