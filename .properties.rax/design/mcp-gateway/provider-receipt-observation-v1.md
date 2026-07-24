# MCP Provider Receipt 到 Runtime Observation 协调 V1

## 1. 状态与边界

状态：`implementation_software_test_yes`，仅覆盖owner-local/reference-test接线。

本合同把Tool/MCP Owner已持久的`MCPProtocolReceiptV1`无损投影为Runtime中立
Provider Receipt，随后复用live `EvidenceGovernancePortV2`与
`OperationObservationGovernancePortV3`形成正式
`ProviderAttemptObservationRefV2`。它不调用Provider、不生成Tool DomainResult、不执行
Settlement，也不授Authority/Review/Fence/Budget/Scope。

production仍缺宿主root、持久Evidence Source provisioning、持久Tool receipt/session store
与Credential/Transport部署；本切片不得解释为production GO。

## 2. Owner与公共Port

| 对象 | Owner | 公共边界 |
|---|---|---|
| `MCPProtocolReceiptV1`及canonical response | Tool/MCP | Tool exact Reader |
| `OperationProviderReceiptRefV1/ProjectionV1` nominal | Runtime | Tool `runtimeadapter`实现只读Reader |
| Evidence Source/Record/sequence | Runtime Evidence Owner | 既有V2 Governance Port |
| Provider Observation与Effect dispatched投影 | Runtime | 既有V3 Observation Gateway |
| 正式Observation到Tool inspection投影 | Tool/MCP | `runtimeadapter.MCPProviderObservationReaderV1`只读exact join |
| 调用顺序与依赖注入 | Application/宿主 | 不import Tool/Runtime实现 |

```go
type OperationProviderReceiptReaderV1 interface {
    InspectOperationProviderReceiptV1(
        context.Context,
        OperationProviderReceiptRefV1,
    ) (OperationProviderReceiptProjectionV1, error)
}

type OperationProviderReceiptObservationPortV1 interface {
    RecordOperationProviderReceiptObservationV1(
        context.Context,
        OperationProviderReceiptObservationRequestV1,
    ) (ProviderAttemptObservationRefV2, error)
    InspectOperationProviderReceiptObservationV1(
        context.Context,
        ExecutionDelegationRefV2,
        string,
    ) (ProviderAttemptObservationRefV2, error)
}
```

Tool projection精确绑定：`Owner/Kind/Receipt ID/Revision/Digest`、Operation/Digest、
Prepared、Dispatch Attempt、Provider Binding、ProviderOperationRef、bounded Payload
Ref/Digest/Schema/Revision、Observed与Checked。它是immutable historical receipt的exact
read，不设置资格TTL，也不把Checked解释成Authority。

## 3. 专属Evidence Source与固定sequence

V1不增加第二套Ledger或游标Reservation。每个Prepared provider attempt必须在执行前获得
一个专属Evidence V2 Source Registration：

- `SourceID=praxis.runtime/provider-receipt-source`；
- 初始Registration Revision固定为1，`NextSourceSequence=1`；
- Ledger partition固定为`effect`并exact绑定Operation Run与Effect ID；
- Producer exact等于Prepared Provider Binding；
- Allowed Kind包含`praxis.runtime/provider-receipt`；
- Observation Class包含`praxis.runtime/provider-observation -> observation`；
- 一个Source只允许该Prepared Attempt的一条Provider Receipt，source sequence固定为1。

因此lost append reply不需要重新分配sequence：始终以
`{RegistrationID, SourceEpoch, SourceSequence=1}`执行exact Inspect。若sequence 1已被不同
Candidate占用则Conflict；若未写且Source不再等于原始Revision/FactDigest或
`NextSourceSequence != 1`则Fail Closed。

## 4. 精确顺序

```text
Tool exact MCPProtocolReceipt
  -> Tool runtimeadapter returns Runtime-neutral historical Receipt Projection
  -> Runtime inspect dedicated Evidence Source exact configuration
  -> Inspect source key sequence=1
  -> NotFound only: AppendGoverned with original registration revision
  -> lost append reply: Inspect same source key only
  -> build ProviderAttemptObservationV2 from exact Receipt + Evidence Record
  -> RecordGovernedProviderObservationV3
  -> return ProviderAttemptObservationRefV2
  -> Tool reader joins exact command + formal Observation + immutable Receipt
  -> return existing SingleCallToolProviderInspectionV1
```

Evidence Candidate由Runtime coordinator构造，Tool不能自报Evidence Ref、SourceSequence、
Producer或Authority。Candidate的EventID等于ProviderOperationRef，Correlation等于Prepared
ID，唯一Causation绑定Delegation ID与同Ledger scope digest。

## 5. 恢复与反例

- Receipt/Source/Append/Observation任一`Unavailable|Indeterminate`不等于NotFound；
- append丢回包仅Inspect sequence 1；绝不重新调用MCP Server；
- Observation Gateway失败可重投同canonical协调请求，但只能重做Evidence/Observation
  exact Inspect与幂等提交，不能重派Provider；
- same Receipt 64并发只形成一个Evidence Record；
- same Receipt ID换digest、跨Attempt、换Provider、换Operation、换Source config、sequence非1、
  wrong class/kind、clock rollback、nil/canceled context全部零Evidence新写/零Observation；
- typed-nil Reader/Gateway依赖在constructor拒绝；
- Provider Receipt仍是Observation；Tool Owner必须独立Inspect Observation与prepare/execute
  两项Enforcement/Consumption后才可CAS `ToolDomainResultFactV2`。
- Tool只读join以Runtime Attempt为入口，反查同Attempt唯一`MCPExecutionCommandV1`，再按
  `Observation.ProviderOperationRef`精确读取immutable Receipt；换Attempt、Prepared、Receipt、
  payload或digest全部Conflict，不能形成DomainResult输入。
- MCP `ToolError=false`只映射为`succeeded+confirmed_applied`；`ToolError=true`只映射为
  `failed+confirmed_applied`。这个映射仍是Tool Owner inspection投影，不是DomainResult事实。

## 6. 兼容与后续门

- additive V1，不修改Evidence V2、OperationScope Evidence V3或Observation V3 canonical；
- `OperationScopeEvidenceConsumptionRefV3`继续用于prepare/execute Settlement closure，不能与
  本V2 Evidence Record互换或type-pun；
- production启用前必须由宿主/Application在provider execute前持久创建专属Source，并把
  exact Registration Ref交给协调Port；当前仓内未提供production root；
- 正式Observation到既有Tool Owner inspection投影的只读join已经完成owner-local软件门；
  Application/宿主调用既有Tool Owner flow后，才可由Tool Owner复读全部current inputs并生成
  DomainResult，再进入Runtime Settlement V4。UnknownOutcome仍只Inspect原Attempt。
