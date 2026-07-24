# Runtime Context Local Transition V1裁决

状态：**联合Review候选；未获Runtime/Context/Application三方YES前不得实现或接入production root。**

## 1. 裁决

CTX-D09首切面中的Context Refresh、Frame/Generation构造、Domain Result与Generation expected-CAS均是Context Owner本地State Plane事务；它们不调用Provider、不触发外部副作用，也不创建Runtime Operation Effect。因此：

1. 本地Context Generation CAS **不需要新的Runtime Settlement**；
2. 不得把Context本地Domain Result提交给Operation Settlement V4；
3. 不得复用上游Tool Effect的Settlement ID、Effect ID或terminal guard来“结算”Context；
4. Runtime不为此新增Fact、FactPort、Permit、Evidence或Outcome；
5. Context Owner必须把上游settled Tool Result的current V4 Inspection、Association、DomainResult exact ref与本次RefreshAttempt绑定进自己的Context transition candidate；candidate/pending不改变Generation current pointer。只有S2 fresh Owner-current reads与TTL全部通过后，Context Owner才可在同一原子事务提交ApplySettlement与expected Generation current CAS；Application只编排与保存exact refs。

Operation Settlement V4仍只证明其所属Operation Effect已由Runtime Settlement Owner终态化。它不是通用跨域事务提交器，也不替代Context Owner的CAS。

## 2. 唯一合法顺序

```text
Application Inspect upstream Tool V4 current closure
  -> Context Owner S1 current reads
  -> create-once Context RefreshAttempt
  -> Context Owner builds pending DomainResult/Frame/Generation candidate
  -> Context Owner S2 fresh Owner-current reads + fresh TTL boundary
  -> atomic Context ApplySettlement + expected Generation current CAS
  -> applied_current | waiting_inspect | conflict
```

S2失败、TTL crossing或current drift时，Generation current pointer必须保持原值；pending candidate只能作为诊断/重试输入，不能交付Harness。回包丢失、cancel、deadline或CAS结果未知时，只允许按RefreshAttempt/Generation exact ref Inspect；不得重跑Tool、重用Runtime Settlement、创建第二Generation或推进Turn。

## 3. 外部动作边界

未来Context remote source、provider cache、披露、远程评估或外部存储写入属于真实Operation Effect，必须走对应版本的Admission、Review、Permit、Begin、Enforcement、Evidence、DomainResult与Runtime Operation Settlement。该路径不得借本裁决绕过治理，也不得把本地Refresh合同扩大为通用Action Gateway。

## 4. TTL与currentness

- RefreshAttempt与Generation expiry取请求NotAfter、上游V4 current closure、Session/Turn、ParentFrame、Recipe、Authority、Binding、Source和Artifact全部可读上界的严格最小值；
- `checked >= expires`即Fail Closed；
- 上游V4历史Fact仍可审计，不等于当前可用于新Refresh；
- S1/S2 closure digest必须一致；S2必须使用fresh clock并发生在ApplySettlement/current CAS之前；漂移只产生Conflict/Residual，不修补Owner事实、不发布current pointer。

## 5. 兼容与反例

- Operation Settlement V3/V4、OperationScope Evidence V3、Dispatch V4.0和Enforcement 4.1不改字段、摘要或语义；
- legacy Context缓存、Receipt、Observation、summary或Application DTO不能升格为Context Fact；
- Context不得写Runtime Effect、terminal guard、Outcome、Evidence cursor或Run Settlement；
- Runtime不得提交Context Generation CAS。

硬反例：本地Refresh创建V4 Settlement；复用Tool Effect ID提交Context DomainResult；上游V4已stale仍Apply；S2失败后Generation pointer可见；ApplySettlement成功但Generation CAS未写或反向半写；CAS回包丢失后创建第二Generation；Context将Observation解释为settled Tool Result。

## 6. 实施依赖

本裁决不新增Runtime Port。后续实现仅依赖Context/Application Owner冻结的三段Port、settled-action current Reader以及现有Runtime V4只读Inspection/Association入口。production root、Harness Continuation与Turn推进仍需独立联合验收。
