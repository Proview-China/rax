# 全上游统一原语层 Code Review 计划 v1

- 状态：陈旧计划（已完成）
- 日期：2026-07-14
- 设计与结论：[全上游统一原语层Code Review](../../design/model-invoker/upstream-primitive-code-review-20260714.md)

## 计划清单

- [x] 盘点Direct、Cloud、Subscription、Harness、Relay、Local和外围入口；
- [x] 对齐39条Callable LLM Route与Builtin Factory；
- [x] 对齐6个Representative Profile与Execution Runtime；
- [x] 建立Operation/Realtime/Local/Relay Canonical Surface Registry；
- [x] 建立LLM矩阵和非主链Surface矩阵的联合证明；
- [x] 补齐Realtime Registry/Invoker；
- [x] 将Operation流生命周期收回Invoker；
- [x] 拒绝LLM、Operation、Realtime和Localcompat身份/事件漂移；
- [x] 隔离Operation/Realtime调用方请求与Provider持有结果，消除跨原语内存别名；
- [x] 增加矩阵缺行、重复、结果漂移、事件漂移和生命周期测试；
- [x] 运行普通、Shuffle、Race、Vet和integration-tag门禁；
- [x] 同步design、plan、module和memory资产。

## 预期产物

完成后，调用方可以根据生命周期选择LLM Route、Execution、Operation、Resource、Job或Realtime公共原语；新增已登记上游若没有进入Canonical Registry、公共调用边界或测试矩阵，将在自动化门禁中失败。
