# Run Lineage Association V1 Port Delta

状态：**Runtime Owner additive公共候选已吸收首轮独立审计`NO（P0=1/P1=2/P2=0）`并返修；READY等待不同agent复审，不自标YES，未授权Go**。

## 1. 用例

宿主需要创建一个普通Runtime child Run，并精确证明它与既有parent Run的关系。Detached Review只是首个可能消费者；未来其它受治理child workflow可复用相同关系，不得为每个组件创建第二Run生命周期或重复Run类型。

## 2. 语义Owner

- 类型、Fact、history/current/highest、compound create、S1/S2与terminal projection：Runtime Owner；
- parent/child Run lifecycle：既有Runtime Run V3 Owner；
- workflow等待与Review语义：Application/Review Owner，不进入本Port；
- Harness source：Harness Owner，不进入本Port；
- production composition：受信Agent Host/Assembler，当前NO-GO。

## 3. Additive public types

精确字段与JSON tags见[contracts](./contracts.md)：

- `RunLifecycleRecordExactRefV1`；
- `RunLineageAssociationSubjectV1`；
- `RunLineageAssociationRefV1`；
- `RunLineageAssociationFactV1`；
- `RunLineageAssociationCurrentIndexV1`；
- `CreatePendingChildRunRequestV1`；
- `RunLineageCreateReceiptV1`；
- `RunLineageAssociationCurrentRequestV1`；
- `RunLineageAssociationExactRequestV1`；
- `RunLineageAssociationTerminalRequestV1`；
- `RunLineageAssociationHistoricalRequestV1`；
- `RunLineageAssociationCurrentProjectionV1`；
- `RunLineageAssociationTerminalProjectionV1`；
- `RunLineageAssociationStoreReaderV1`、`RunLifecycleCurrentReaderV1`；
- `RunLineageAssociationReadRecoveryPolicyV1`、`RunLineageAssociationReaderDependenciesV1`。

公共接口：

```go
type TrustedChildRunAssemblerPortV1 interface {
    CreatePendingChildRunV1(
        context.Context,
        CreatePendingChildRunRequestV1,
    ) (RunLineageCreateReceiptV1, error)
}

type RunLineageAssociationCurrentReaderV1 interface {
    ResolveRunLineageAssociationCurrentV1(
        context.Context,
        RunLineageAssociationCurrentRequestV1,
    ) (RunLineageAssociationCurrentProjectionV1, error)
    InspectRunLineageAssociationCurrentV1(
        context.Context,
        RunLineageAssociationExactRequestV1,
    ) (RunLineageAssociationCurrentProjectionV1, error)
    ResolveRunLineageAssociationTerminalV1(
        context.Context,
        RunLineageAssociationTerminalRequestV1,
    ) (RunLineageAssociationTerminalProjectionV1, error)
    InspectRunLineageAssociationTerminalV1(
        context.Context,
        RunLineageAssociationExactRequestV1,
    ) (RunLineageAssociationTerminalProjectionV1, error)
    InspectRunLineageAssociationHistoricalV1(
        context.Context,
        RunLineageAssociationHistoricalRequestV1,
    ) (RunLineageAssociationFactV1, error)
}

type RunLineageAssociationReadRecoveryPolicyV1 struct {
    ReadRecoveryTimeoutNanos int64 `json:"read_recovery_timeout_nanos"`
}
type RunLineageAssociationReaderDependenciesV1 struct {
    Store RunLineageAssociationStoreReaderV1
    Lifecycle RunLifecycleCurrentReaderV1
    Clock func() time.Time
}
func NewRunLineageAssociationCurrentReaderV1(
    RunLineageAssociationReadRecoveryPolicyV1,
    RunLineageAssociationReaderDependenciesV1,
) (RunLineageAssociationCurrentReaderV1, error)
```

Assembler与Reader必须分离；consumer只获得Reader，不取得child create或Runtime lifecycle mutation。

## 4. 输入输出与不变量

| 能力 | 输入 | 输出 | 不变量 |
|---|---|---|---|
| child create | parent exact ref + ordinary V3 child request + NotAfter | child lifecycle + rev1 association + index receipt | 同事务全有全无；stable ID；same canonical幂等 |
| Resolve current | full Subject + caller NotAfter | S1/S2 current projection | 只由Subject派生index；不支持latest/by-name |
| Inspect current | full Subject + exact Association Ref | 同Ref仍current的projection | index/ref/history/parent/child全量双读 |
| Resolve terminal | full Subject + caller NotAfter | current index指向的truthful termination_closed projection | Subject唯一bootstrap；S1/S2 index/history/Run closure；不新增ProjectionRef |
| Inspect terminal | full Subject + exact terminal Association Ref | immutable exact termination_closed history projection | 不读取current index；child Decision/Report exact；不选择Review/Task Outcome |
| Inspect historical | exact Subject + full Ref | immutable history Fact | 不借current index；过期仍可审计 |

## 5. Owner compound mutation

现有`CreatePendingRunV3`由bundle与EffectIndex多步实现，不能单靠consumer外层调用获得跨对象原子性。实现必须新增Runtime Owner内部compound transaction：

```text
CreatePendingChildRunV1
  -> 在Owner事务内exact复读parent
  -> stage existing child Run bundle/Plan certification/empty EffectIndex
  -> stage association rev1/history/highest/current/receipt
  -> one commit
```

后续child Run record revision或lifecycle phase变化也必须在Runtime Owner同一事务中追加association revision；例如Completion Claim导致Run revision变化，即使phase不变也要推进association。EffectIndex或其它非Run-record sidecar在同phase内变化时不增association revision，但current Reader的完整V3 envelope S1/S2必须捕获读间漂移。若某production backend不能提供该原子边界，则该backend对child Run能力返回`CapabilityUnavailable/ComponentMissing`，不得退化为“先建Run、再补association”。

## 6. Effect与Recovery

- 关联与Run Fact写入是Runtime State Plane mutation，不是Provider Effect；
- 创建child Run的真实执行仍由既有execution-start Operation治理，association不授Permit/Begin；
- lost create/lifecycle reply只能Inspect原ID、receipt、history与current，不重调mutation；
- current Reader只读；Resolve lost reply最多一次同Subject新S1，exact Inspect最多一次同full Ref retry；具名recovery policy要求`0<timeout<=2s`并按caller deadline/request TTL及current路径association TTL裁剪；
- Unknown、Unavailable、TTL crossing、clock rollback均Fail Closed；
- Cleanup/Residual仍归child普通Run lifecycle/各Participant Owner，不由association吞并。

## 7. 兼容影响

- additive：不改V3 structs、JSON、digest、接口签名或ordinary Run行为；
- existing implementations不会自动满足新Assembler/Reader；
- 无法原子扩展的backend保持unsupported；
- Application未来只依赖`runtime/core`与`runtime/ports`；Review/Harness不得import Runtime实现；
- 不迁移legacy Run，不从Session/Case/Target反推association。

## 8. Import DAG（无SCC）

```text
runtime/core
  <- runtime/ports
  <- runtime/control
  <- runtime/kernel

application/contract+ports
  -> runtime/core + runtime/ports

review runtimeadapter（未来）
  -> review public + runtime/core + runtime/ports

harness（无新增依赖）
agent-host composition（最后）
  -> public ports only
```

禁止`runtime/ports -> application|review|harness`，禁止`application -> runtime/kernel|control|fakes`，禁止Review专用类型进入Runtime。

## 9. Production准入证据

必须全部满足后才可GO：

1. Runtime ports/contracts双独立0/0/0；
2. 同Owner原子memory reference store与SQLite/目标production store conformance；
3. create/phase/terminal staged failure零泄漏；
4. target ordinary100、race20、full ordinary/race/vet/gofmt/diff/import全绿；
5. Application中立association适配与Review detached lineage消费独立审计；
6. Agent Host trusted assembler/composition root实装并通过重启恢复；
7. 未用Fake声称production durability/SLA。

当前上述实施证据均未形成，因此Go与production root继续NO-GO。
