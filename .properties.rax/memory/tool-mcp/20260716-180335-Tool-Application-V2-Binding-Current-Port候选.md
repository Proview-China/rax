# Tool Application V2 Binding Current Port联合候选

时间：2026-07-16 18:03（Asia/Shanghai）

Application G6A V2设计第二审确认Tool Owner缺少一个从稳定Application Request、Identity/ExecutionScope与Model Projection exact坐标解析Action/Capability/Tool/Provider/canonical arguments，并在Provider Boundary前再次复读current的公开只读Port。Tool Owner已在`design/tool-engine/port-delta.md`追加`PD-TM-04`，冻结`SingleCallToolActionBindingCurrent*V1`的联合候选字段、JSON discriminator/domain、Reader签名、Owner、TTL、S1/S2、typed-nil、zero-effect、Boundary再Inspect、import边界与硬反例。

本事件不改变既有Tool G6A V2 Owner-local隔离实现第三轮独立审计YES的历史结论，但该YES不覆盖Application G6A V2 Binding Current接线。当前P0为：

1. Application Owner尚未落盘`SingleCallToolActionRequestV2`；
2. Application Owner尚未落盘`SingleCallToolActionInputCurrentProjectionV2`；
3. 对应V2窄Input Current Reader正式类型名/签名尚未冻结；
4. `PD-TM-04`尚未通过Application/Tool/Runtime/Model独立设计终审。

因此该候选compile-blocked，未写Go，不自判YES。禁止退回Application V1、alias/wrapper/JSON重编码/type-pun，也禁止在Tool domain/kernel下沉Application协调器类型。P0解除前，相关Watermark、Candidate、Reservation、Boundary、Gateway、Provider、DomainResult与Settlement均保持零。

同步范围仅为Tool独占design/plan/module及本条新增memory；未修改Application、Harness、Runtime、Model、Go代码或既有历史memory。
