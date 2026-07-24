# Operation Settlement Current Reader V5 Port Delta

状态：**第二次独立代码短审YES（P0/P1/P2=0/0/0），已实施并冻结**。

## 1. 精确公共签名

```go
type OperationSettlementCurrentReaderV5 interface {
    InspectCheckpointPhaseSettlementCurrentV5(
        context.Context,
        InspectCurrentOperationCheckpointRestoreSettlementRequestV5,
    ) (OperationCheckpointRestoreSettlementInspectionV5, error)
}

type OperationSettlementCurrentReaderProviderV5 interface {
    OperationSettlementCurrentReaderV5
    GatewayBackedOperationSettlementCurrentReaderV5()
}

type OperationCheckpointRestoreSettlementGovernancePortV5 interface {
    OperationSettlementCurrentReaderV5

    SettleCheckpointPhaseV5(
        context.Context,
        OperationCheckpointRestoreSettlementSubmissionV5,
    ) (OperationCheckpointRestoreSettlementRefV5, error)

    InspectCheckpointPhaseSettlementHistoricalV5(
        context.Context,
        InspectOperationCheckpointRestoreSettlementRequestV5,
    ) (OperationCheckpointRestoreSettlementCommitBundleV5, error)

    InspectCheckpointPhaseSettlementAssociationV5(
        context.Context,
        OperationSubjectV3,
        OperationCheckpointRestoreSettlementAssociationRefV5,
    ) (OperationCheckpointRestoreSettlementAssociationV5, error)

    InspectCheckpointPhaseTerminalGuardV5(
        context.Context,
        OperationSubjectV3,
        OperationCheckpointRestoreTerminalGuardRefV5,
    ) (OperationCheckpointRestoreTerminalGuardV5, error)

    InspectCheckpointPhaseTerminalProjectionV5(
        context.Context,
        OperationSubjectV3,
        OperationCheckpointRestoreTerminalProjectionRefV5,
    ) (OperationCheckpointRestoreTerminalProjectionV5, error)
}
```

Reader的方法集严格只有一个current Inspect方法。Governance通过嵌入保持既有六个方法完全不变。consumer composition只接收`OperationSettlementCurrentReaderProviderV5`；Kernel的`OperationSettlementCurrentReaderFacadeV5`由有效Gateway构造并实现marker。raw Fact Port和普通单方法Reader都不能误满足provider wiring。marker不构成密码学或语言级不可伪造证明，只是明确、可测试的装配能力边界。

## 2. 复用证据

Live `ports/operation_checkpoint_settlement_v5.go`已经冻结并实现：

- `InspectCurrentOperationCheckpointRestoreSettlementRequestV5`；
- `OperationCheckpointRestoreSettlementInspectionV5`及其`Validate()`；
- `OperationCheckpointRestoreSettlementGovernancePortV5.InspectCheckpointPhaseSettlementCurrentV5`；
- 同名Fact Port读取方法。

Live Kernel Gateway先验证request和依赖，再委托Fact Owner并验证完整Inspection；reference store按`(TenantID, EffectID)` current索引返回同一V5 Commit Bundle。故本Delta不新增实现路径、状态、ID、canonical、digest、TTL或错误类别。

但公开Reader前必须修复一个现有Owner边界：`inspection.Validate()`只证明Bundle内部自洽，不证明错误或恶意backend返回的Bundle属于本次request。Gateway必须在返回前额外要求：

```go
SameOperationSubjectV3(inspection.Bundle.Submission.Operation, request.Operation)
inspection.Bundle.Submission.EffectID == request.EffectID
inspection.Bundle.Settlement.EffectID == request.EffectID
```

任一不等返回`Conflict`并且不泄露其他Operation/Tenant的closure。该检查只闭合既有读取语义，不改任何V5对象或digest。

冻结的Gateway伪代码为：

```go
inspection, err := facts.InspectCheckpointPhaseSettlementCurrentV5(ctx, request)
if err != nil {
    return OperationCheckpointRestoreSettlementInspectionV5{}, err
}
if err := inspection.Validate(); err != nil {
    return OperationCheckpointRestoreSettlementInspectionV5{}, err
}
bundle := inspection.Bundle
if !SameOperationSubjectV3(bundle.Submission.Operation, request.Operation) ||
    bundle.Submission.EffectID != request.EffectID ||
    bundle.Settlement.EffectID != request.EffectID {
    return OperationCheckpointRestoreSettlementInspectionV5{},
        checkpointGatewayConflictV2("checkpoint Settlement V5 current request drifted")
}
return inspection, nil
```

`SameOperationSubjectV3`使用现有canonical digest值语义，覆盖Operation完整结构及Tenant/Scope；不得改用指针相等、只比OperationDigest字符串或仅依赖backend索引。必须先执行完整`inspection.Validate()`，再比较request，且Conflict返回零值，禁止携带他Operation的sidecar。

V3已有同型先例：`OperationSettlementCurrentReaderV3`只抽取既有`InspectOperationSettlementV3`，Governance兼容嵌入。该先例只证明additive形状可行，不自动构成V5代码授权。

## 3. 调用与错误边界

Sandbox侧调用顺序固定为：

```text
持有exact OperationSubjectV3 + EffectID
-> OperationSettlementCurrentReaderV5.InspectCheckpointPhaseSettlementCurrentV5
-> Inspection.Validate()
-> exact比较Operation/Effect/Attempt/Phase/DomainResult/Association/Guard/Projection
-> Sandbox Owner自己的ApplySettlement current/CAS
```

- Invalid request继续返回既有`InvalidArgument`；
- current V5不存在继续返回`NotFound`；
- wrong operation、ref或closure漂移继续返回`Conflict`；
- backend不可用继续返回`Unavailable`或`Indeterminate`，不得降级为NotFound；
- Reader为nil/typed nil时消费者或public Conformance必须以`ComponentMissing` fail closed；
- Fact backend返回结构有效但属于其他Operation或Effect的Inspection时必须`Conflict`，不得原样透传；
- Reader不允许调用Settle、Fact Commit或重建不存在的Settlement。

## 4. 兼容与非目标

- 不改`OperationCheckpointRestoreSettlementGovernancePortV5`既有方法签名或方法总数；
- 不改`OperationCheckpointRestoreSettlementFactPortV5`，它仍是Runtime内部Owner能力；
- 不改V3/V4/V5 shared `(TenantID, EffectID)` terminal guard；
- 不改V5 Settlement/Association/Guard/Projection/EffectTerminal对象和digest；
- 不新增Sandbox adapter、production root、backend、durability或SLA声明。

冻结method-set：

- `OperationSettlementCurrentReaderV5`：恰好1个方法；
- `OperationCheckpointRestoreSettlementGovernancePortV5`：嵌入Reader后仍恰好6个方法；
- `OperationCheckpointRestoreSettlementFactPortV5`：完全不变；
- 既有实现无需adapter即可同时满足Governance与Reader；reader-only consumer在静态类型上无法调用Settle。

## 5. 实施结果

联合设计Review YES和用户代码授权后，仅修改Runtime ports、Kernel Gateway、public Conformance及测试：

- additive Reader已落地并嵌入Governance；
- Gateway已执行Inspection Validate后Operation/Effect exact回值门；
- reader-only Conformance、method-set、typed-nil、import边界与恶意backend零泄露反例均已落地；
- Store、Fact Port、V5对象、digest与其他Owner未修改。

首轮独立代码短审NO后的返修已补Gateway facade/provider marker、malformed-before-drift、同ID跨Tenant/Scope/nested ref、Unavailable/Indeterminate原样透传、DeepEqual零Inspection与零副作用计数。Owner验证已通过target ordinary `count=100`、target race `count=20`、Runtime full ordinary/race、vet、gofmt与diff-check；第二次独立代码短审YES（P0/P1/P2=0/0/0）。本纵切不声明production backend/root/durability/SLA。
