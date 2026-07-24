# Harness Model PreDispatch Gate第一短审P0返修

时间：2026-07-16 21:12（Asia/Shanghai）

## 事件

Model/Tool/Harness联合第一短审判定原Gate设计`NO(P0=4)`。本次只修Harness设计、计划、图与索引，没有写Go，也没有修改Model、Tool、Application或Runtime。

## 已纠正真值

- Prepared Historical Fact Ref与Current Ref/Reader严格分层：Historical携Plan/RequestTools/Route/Profile/Registry exact Ref与资格绝对NotAfter；Current携Tool/Provider双actual digest及Checked/Expires；
- Gate输入必须显式携Fact Ref和Current Ref，禁止按Fact查询latest Current；Current/Tool窗口不得超过Historical NotAfter；
- Assembly公共面统一为single sealed composite current：Harness内部只复读Generation/Handoff/BindingSet，Tool完整保存Projection而不拆解currentness；
- Model路径冻结为纯本地Preparation→Prepared publish→Harness Gate→actual-point；Phase A禁止Provider/Backend动态调用；
- Tool Writer改为Owner-owned Commit request语义：Harness只提交exact坐标和requested上界，Tool Clock生成Created/NotAfter并Seal Fact/Ack；
- Binding/Ack不授Provider进入权。每次Provider attempt、Open、Stream、direct continuation与realtime Open前重新Inspect exact Binding/Ack及全部Owner current；
- Tool Surface `ExpectedInjectionDigest`只与Model `ActualToolSurfaceDigest`相等；richer `ActualProviderInjectionDigest`独立保存；
- Context ExpectedInjection属于Frame/field Injection Conformance，已从Surface Binding等式、TTL和Reader依赖中移除；
- continuation若改变Tools、Surface或任一actual digest，当前session拒绝并要求新Invocation epoch；

## live旁路证据

已确认当前Model路径包含Gate前实际点：`Runtime.Start→Adapter.Preflight`、direct `Backend.Resolve`、generic/operation `provider.Capabilities`、routegateway Invoke/Stream、direct continuation Backend Invoke/OpenStream及realtime WebSocket Open。后续Conformance必须逐路径证明同一Ack guard，不能只依赖上层ModelTurn构造注入。

## 当前状态

- Harness资产：联合短审P0返修候选，等待下一轮三Owner短审；
- Harness Go：NO-GO；
- Model Historical/Current双Ref双Reader及Tool Owner Commit request：联合Port Delta，未实现；
- Tool P4、system G6A、Capability、production root：继续NO-GO；
- 未stage、未commit。

## 资产

- [设计](../../design/harness/assembly/model-predispatch-surface-commit-gate-v1.md)
- [测试矩阵](../../design/harness/assembly/model-predispatch-surface-commit-gate-v1-test-matrix.md)
- [实施计划](../../plan/harness/model-predispatch-surface-commit-gate-v1.md)
- [流程图](../../design/harness/assembly/model-predispatch-surface-commit-gate-v1.drawio)
