# 受治理 MCP Discovery Page V1 Port Delta

## 1. 裁决与状态

- 状态：`implementation_software_test_yes`。Runtime public Page ports/kernel gate、Tool Page Command/Receipt、official SDK单页actual-point、正式Observation/Evidence、typed DomainResult、Runtime Settlement V4、Tool ApplySettlement、Capability Snapshot聚合及只读SDK/API/CLI均已落盘并通过owner-local软件门；
- 原因：live `ControlledOperationPhysicalExecutionAuthorizationV3`只接受`praxis.tool/execute`，`ControlledMCPConnectPhysicalAuthorizationV1`只接受`praxis.mcp/connect`。二者均不得包装扩权为Discovery；
- 旧`OfficialSDKDiscoveryV1`继续只作注入式兼容测试；真实owner-local分页链只接受Runtime专属`praxis.mcp/discover` physical authorization并消费已settled Connection Availability。两者均不代表production root或Backend；
- 当前Connection前置已经闭合到：`Connect Intent -> Runtime Connect authorization -> official SDK initialize -> Protocol Receipt -> formal Observation/Evidence -> MCPConnectDomainResult -> Runtime Settlement V4 -> MCPConnectApplySettlement -> Connection Availability`。

## 2. 用例与Effect粒度

MCP分页协议中的每一次`tools/list`、`resources/list`或`prompts/list`都是独立外部调用，因此必须是独立Effect。一个“发现全部能力”的Application工作流只是顺序编排多个Page Effect，不得用一个Permit覆盖未知数量的Provider写入。

固定首版矩阵：

| 字段 | V1值 |
|---|---|
| OperationScopeKind | `run` |
| EffectKind | `praxis.mcp/discover` |
| PolicyProfile | `praxis.mcp/discovery-page-v1` |
| Run | required |
| Session | required |
| Turn/Action/Context | forbidden |
| Conflict Domain | tenant + run + session + connection epoch + namespace + cursor |

pre-run/admin Discovery继续NO-GO；未来若需要，必须由Runtime Owner先提供真实Admin OperationScope，不能伪造Run。

## 3. Owner与非Owner

| 对象/动作 | Owner | 非Owner约束 |
|---|---|---|
| Page Command/Intent、Protocol Receipt、Page DomainResult、Snapshot聚合 | Tool/MCP | 不写Runtime Outcome/Review/Evidence |
| Operation/Intent/Admission/Permit/Begin/Enforcement/Fence/Settlement | Runtime | 不解释MCP业务字段 |
| 多Page/多namespace顺序编排 | Application | 不直接调用SDK或写Tool事实 |
| SDK List调用与wire/pagination响应 | official MCP SDK/Provider | 仅Observation |
| Capability Snapshot active/admitted | Tool/MCP + 后续宿主治理 | 原始Page响应不能直接active |

## 4. Tool侧候选对象

### `MCPDiscoveryPageCommandV1`

字段：

- `Ref{ID,Revision=1,Digest}`；
- `Owner`；
- exact `MCPConnectionFactRefV2`、`MCPConnectionAvailabilityCurrentProjectionV1` source ref；
- exact `MCPConnectApplySettlementFactV1` ref；
- `Namespace`闭集：`tools|resources|prompts`；
- `CursorDigest`与有界opaque cursor payload；空cursor是第一页的canonical值；
- `PageOrdinal`，从0开始且只由上一页settled receipt推进；
- `Operation/OperationDigest/EffectID/EffectRevision/IntentDigest/Prepared/Attempt`；
- `Provider`、`CreatedUnixNano`、`NotAfterUnixNano`。

ID覆盖Connection、Epoch、Namespace、PageOrdinal、CursorDigest与Attempt；同ID换cursor/connection/attempt必须Conflict。

### `MCPDiscoveryPageProtocolReceiptV1`

字段：Command、Runtime admission receipt、namespace、request/cursor digest、response page digest、next cursor digest、item count、Provider operation ref（若有）、ObservedUnixNano。原始SDK响应只在有界owner store保存；公开Receipt为ref/digest投影。

### `MCPDiscoveryPageDomainResultFactV1`

仅在formal Provider Observation、execute Evidence Consumption和Page Receipt exact闭合后由Tool Owner Inspect/CAS。Runtime Settlement V4只引用typed DomainResult。Application只有在前一页ApplySettlement后才能构造下一页Command；UnknownOutcome永不推进cursor。

## 5. Runtime-neutral additive Port Delta

Runtime Owner候选新增：

```go
type ControlledMCPDiscoveryPageAuthorizationRequestV1 struct {
    Route                  ControlledMCPDiscoveryRouteCurrentRefV1
    Execute                ExecutePreparedRequestV2
    Attempt                OperationDispatchAttemptRefV3
    ExecuteEnforcement     OperationDispatchEnforcementPhaseRefV4
    PrepareConsumption     OperationScopeEvidenceConsumptionRefV3
    ExecuteHandoff         OperationScopeEvidenceProviderHandoffRefV3
    Association            PreparedDomainCommandAssociationRefV1
    DomainCommand          OperationDomainCommandRefV1
    ConnectionAvailability MCPConnectionAvailabilityNeutralRefV1
    Namespace              NamespacedNameV2
    CursorDigest           core.Digest
    PageOrdinal            uint32
    CallerDeadlineUnixNano int64
}

type ControlledMCPDiscoveryPagePhysicalAuthorizationV1 struct {
    // existing Runtime operation/attempt/enforcement/evidence/association fields
    StableKeyDigest         core.Digest
    UnifiedNotAfterUnixNano int64
    ProviderTransport       ProviderBindingRefV2
    Provider                ProviderBindingRefV2
    Operation               OperationSubjectV3
    OperationDigest         core.Digest
    OperationScopeDigest    core.Digest
    EffectID                core.EffectIntentID
    EffectRevision          core.Revision
    Prepared                PreparedProviderAttemptRefV2
    Attempt                 OperationDispatchAttemptRefV3
    ExecuteEnforcement      OperationDispatchEnforcementPhaseRefV4
    PrepareConsumption      OperationScopeEvidenceConsumptionRefV3
    ExecuteHandoff          OperationScopeEvidenceProviderHandoffRefV3
    Association             PreparedDomainCommandAssociationRefV1
    DomainCommand           OperationDomainCommandRefV1
    ConnectionAvailability MCPConnectionAvailabilityNeutralRefV1
    Namespace              NamespacedNameV2
    CursorDigest           core.Digest
    PageOrdinal            uint32
    IssuedUnixNano          int64
    Digest                  core.Digest
}
```

`MCPConnectionAvailabilityNeutralRefV1`只携Tool Owner、Connection Ref、Apply Ref、DomainResult Ref和projection digest；Runtime通过注入的只读current Reader复读，不import Tool实现。最终字段名需Runtime Owner联合终审，Tool不得先复制私有兼容类型。

## 6. 调用序列

```text
Application chooses namespace/cursor
  -> Tool Ensure Page Command (no Provider)
  -> Runtime Intent/Admission/Permit/Begin
  -> prepare Enforcement + Evidence issue/current/handoff/consume
  -> execute Enforcement + independent Evidence issue/current/handoff
  -> Runtime Discovery Page physical authorization
  -> Tool actual-point re-read Command + Connection Availability + official Session
  -> SDK one List page call
  -> Tool Page Protocol Receipt (Observation only)
  -> Runtime formal Observation + execute Evidence consume
  -> Tool Inspect/CAS Page DomainResult
  -> Runtime Settlement V4
  -> Tool ApplySettlement
  -> Application may schedule next cursor or aggregate sealed Snapshot
```

## 7. Effect/Recovery

- physical admission CAS成功即视为“可能已调用”；
- lost admission reply：Inspect entry，Provider调用必须为0；
- lost Provider/page reply：只Inspect原Entry/Attempt，不重发同页；
- `Unavailable`/`Indeterminate`不等于`NotFound`；
- Provider返回next cursor只作Observation；必须等本页settled+applied后才能推进；
- cursor cycle、页数/对象数超限、namespace漂移、Connection/Apply/Session过期：Fail Closed，不产Snapshot；
- cancellation不能把Unknown改写为confirmed_not_applied。

## 8. 硬反例

1. 用Action V3或Connect V1 authorization包装Discovery：零SDK调用；
2. 未Apply的Connection、换Connection Epoch、换Apply/DomainResult：零SDK调用；
3. prepare/execute Qualification、Handoff、Consumption复用：零SDK调用；
4. same Command ID换namespace/cursor/page ordinal：Conflict；
5. admission丢回包重投：单entry、零或单SDK调用，禁止第二次；
6. Provider回包丢失/timeout：Unknown、Inspect-only、不得推进cursor；
7. 本页未settled就请求下一页：PreconditionFailed；
8. next cursor循环或超过上限：不生成Snapshot；
9. raw SDK Result直接Seal Snapshot：拒绝；
10. 64同key并发：单entry/单physical admission/最多单SDK page call；不同key允许并行。

## 9. 兼容与实施门

- additive；不修改V1 Discovery canonical，不扩大Runtime V3/Connect V1；
- Runtime public ports/kernel gate与Tool physical adapter已经实现并通过软件测试；V1不得回退到Action/Connect授权或旧注入式Discovery；
- 多namespace/多page Application工作流、list_changed到新Operation调度与production装配仍未落盘，不得把owner-local单页闭环写成系统Discovery GO；
- production仍依赖durable State Plane、Credential/backend、composition root和Application工作流。
