# Application Next Model Turn Eligibility V1 设计候选

状态：前半切片实现候选；仅覆盖只读 eligibility、稳定 derived dispatch ref 与 exact Inspect 恢复坐标。production Dispatch 继续 HARD-BLOCKED。

## 目标

在既有 `TurnContinuationCurrentV1(context_current)` 与未来 Harness V2 Model dispatch adapter之间增加 Application 中立只读接缝：

```text
TurnContinuation exact Inspect
  -> 校验 ActiveContext / Run / Session / TargetTurn exact
  -> 绑定未来 Runtime Model actual-point exact request digest
  -> 返回短期 eligibility projection + stable derived dispatch ref
```

该切片不等待 Harness V2，但也不替代 Harness V2。它不调用 Runtime actual-point guard、不接收或暴露 guard projection、不调用 Model/Provider、不推进 Turn、不创建 Store、不写 Application/Harness/Runtime/Model Owner 事实。

## 中立合同

`NextModelTurnEligibilityRequestV1` 仅绑定：

- TurnContinuation 原 Attempt ref 与 current digest；
- Harness ActiveContext exact ref；
- Run revision/digest、Session revision/digest/TTL、TargetTurn；
- Runtime actual-point guard 的 public exact request及其 canonical digest；
- 排他 `RequestedNotAfterUnixNano`；
- Application request digest。

Application 不携带 Harness Session/Run Envelope，不复制 Model Prepared/ACK Fact，不引入第二套 current Store。

## 校验顺序

1. fresh clock 与 request currentness；
2. 用原 `TurnContinuationAttemptRefV1` 只读 Inspect一次；该结果仅为 advisory快照，不是dispatch时的current证明；
3. 要求 current 为 `context_current`，并逐字段闭合 current digest、ActiveContext、Run、Session、TargetTurn；
4. fresh clock复检 request与continuation；
5. 取 request、Session、continuation与future ModelBoundary四者最小 NotAfter；
6. 返回短期 projection。

`now == expiry`（含ModelBoundary排他expiry）、时钟回拨、pending、current digest splice、ActiveContext/Run/Session/TargetTurn漂移、future Fence坐标无效、Continuation current Reader不可用或取消均 Fail Closed。Eligibility projection不得拓宽其绑定的ModelBoundary expiry。Fence与ModelBoundary是否仍current只能由紧贴物理Provider调用的Runtime actual-point guard及Harness/Runtime fresh read裁决，本切片不得提前判断。

## Derived ref 与恢复

`NextModelTurnDerivedDispatchRefV1` 只由 sealed request digest派生。相同 exact request在进程、并发和重试之间产生同一 ID/ref；它不是 Permit、Fence、Authorization或已发生 dispatch 的证明。

所有回包丢失都由调用方用原 exact eligibility request重新 Inspect。Application不生成新 Attempt、不重派 mutation，也不保存 recovery row。

## 硬边界

- production Dispatch等待 Harness V2 exact adapter；
- 真正dispatch必须重新fresh-read Harness/Runtime currentness，不得复用本切片的单次Continuation advisory快照；
- Runtime actual-point guard只能在Model boundary CAS winner与Model S3后、同一call stack紧贴物理Provider调用，projection不得缓存或返回Harness/Application；
- Provider物理调用仍必须在 Model provider boundary接线后发生；
- 本切片不声明 Agent Loop、两轮Agent、Host root或production闭合；
- Console合同、假Provider、FFI、跨Owner事实写入均为零；
- actual-point projection在其排他 NotAfter后不可复用。
