# Model Route工具兼容Delta

时间：2026-07-18 16:40 CST

## 事件

Tool Owner完成PD-TM-05 Model Route Tool Compatibility V1联合候选及current-truth索引同步。

## 已确认事实

- Tool Owner的portable compile已经能从exact current Surface确定性产生Model Invoker公开
  neutral `Tool`，且不复制厂商DTO；
- live Model Invoker没有公开Prepared-scoped Route Tool compatibility historical/current/
  Association事实，粗粒度Capability、Request Validate、RouteSelection或Provider回包不能替代；
- 候选采用Model Owner historical Fact、current Projection、create-once Prepared Association与
  actual-point exact Readers，逐项表达`exact|transformed|degraded|rejected`及Residual；
- Tool只消费Model public Reader；Model不import Tool实现，Runtime只拥有neutral Assembly，Harness
  与宿主只负责composition/Gate，不创建事实；
- lost reply只Inspect同一Prepared Association/Fact/current，不重新Prepare、换Route或重派Provider。

## 当前状态

- PD-TM-05：`joint_candidate_no_go`；
- owner-local portable compile：`implementation_software_test_yes`；
- production Route可调用Surface：NO-GO；
- 未修改Model、Runtime、Harness、Application或Context实现。

## 后续门

Model/Runtime/Harness联合审定public nominal、Route/Catalog exact-current Evidence以及Open/Stream/
continuation所有Provider路径统一Gate后，才允许进入跨Owner实现。Tool侧不得提前写Model
compatibility Go或把portable profile升级成production保证。
