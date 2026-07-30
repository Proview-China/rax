# Application Next Model Turn Dispatch V1 设计候选

状态：早审 P0 重构后的中立合同与 Harness durable derived-binding 前半切片候选；Application production dispatch继续 HARD-BLOCKED。

## 目标

在已合并 Eligibility 与未来 Harness ExactModelTurnPortV2 之间，仅固化 Application 自己拥有的恢复坐标：

```text
existing Eligibility request + projection
  -> Harness S1 Continuation exact Inspect
  -> Harness S2 Continuation exact Inspect
  -> append-only attempt_bound sidecar
  -> exact Inspect / restart recovery
```

## Application 中立合同

`NextModelTurnDispatchRequestV1` 只能包含：

- existing `NextModelTurnEligibilityRequestV1`；
- existing `NextModelTurnEligibilityProjectionV1`；
- 排他 `RequestedNotAfterUnixNano`；
- canonical `RequestDigest`；
- projection内既有 stable `NextModelTurnDerivedDispatchRefV1`。

Eligibility已完整绑定 Continuation exact Attempt/current digest、ActiveContext exact、Run/Session/TargetTurn，以及 future Runtime Model actual-point guard的public sealed exact request coordinate。Dispatcher只复用该合同，不复制Runtime字段、不调用guard、不保存guard projection。

Application不得出现 Model Prepared/current/InvocationMaterial/Authorization/SourceLineage DTO，不硬编码Context/Tool Owner或Kind，不携带RouteCall、Tool digest、Provider ordinal、Harness Envelope或任何Model/Harness私有身份。

## Harness sidecar

Harness adapter仅依赖：

- `TurnContinuationCurrentReaderV1`；
- 本切片唯一 SQLite derived-binding repository；
- fresh clock。

StartOrInspect先按原 `DerivedDispatchRef + RequestDigest` Inspect durable current；未命中才执行S1/S2。两次读取必须完整一致并逐轴闭合 Continuation Attempt/current digest、ActiveContext、Run/Session/TargetTurn与Eligibility坐标。

最终 NotAfter是caller上界、Eligibility request/projection、Session、future Runtime ModelBoundary与Continuation S1/S2 expiry的最小值。`now == expiry`、clock rollback、pending、splice、cancel或任一轴drift均Fail Closed。

持久化使用不晚于该最小NotAfter的bounded context；获取owner锁后、事务mutation前以及commit紧前都必须fresh-read clock并重新验证排他TTL。锁等待或事务内延迟跨过TTL时整笔回滚、写入数为0。

## SQLite 权威

SQLite只保存 `attempt_bound` 的 Application `DerivedDispatchRef`、`RequestDigest`、canonical fact digest与TTL：

- schema ledger；
- append-only history；
- current exact index；
- stable ID由Eligibility request digest派生；
- 同ID同payload幂等、同ID异payload冲突；
- lost reply只以原 exact Inspect恢复；
- restart与64并发保持单winner；
- 全新空库的64 mixed-payload同stable identity首创race只形成一个durable winner/current；
- 无更新、删除或ABA路径。

打开数据库时必须在同一owner锁和初始化事务内先分类完整物理闭集与schema ledger：

- 仅当ledger、两张数据表、exact index及trigger闭集全部不存在时，才允许一次原子fresh migration；
- 完整对象与正确ledger只做exact verify，禁止repair；
- partial/no-ledger、correct-ledger缺表/缺index、弱schema或任何额外trigger均Conflict且不得补齐；
- history/current的全部metadata与canonical payload必须逐字段一致；
- durable fact只允许结构完整的`attempt_bound`，`outcome_bound`、未知state或额外Outcome字段均Fail Closed。

DerivedDispatch结构验证统一覆盖contract version、revision、Eligibility request digest、Continuation Attempt exact validity、各轴digest、ref digest，以及由Eligibility request digest按公开唯一算法派生的stable ID。Application Inspect、Harness写入/读取与IntegrityCheck共用该验证。

禁止调用面证据由测试自动解析实际production source imports并执行Owner allowlist，覆盖Model、Runtime guard、Provider、Harness dispatch与Console；不以手工import列表或默认零字段充当调用证据。

未来Harness V2负责把自己的 exact Model dispatch envelope与该Application ref显式关联；本切片不提前复制或代持Model/Context/Tool正文。

## 硬边界

- 未合并 Harness ExactModelTurnPortV2仍是后半切片阻塞；
- 不实现或伪造 `outcome_bound`；
- 不调用 Runtime guard、Model Port或Provider；
- 不修改 Harness `modelinvokeradapter/ports/bridgecontract`；
- 无 Console合同；
- 当前 sidecar不是 Permit、Authorization、Model Attempt、Harness Dispatch或production GO。
