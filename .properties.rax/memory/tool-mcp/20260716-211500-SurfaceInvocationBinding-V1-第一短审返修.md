# SurfaceInvocationBinding V1 第一短审返修

时间：2026-07-16 21:15 CST

## 事件

三Owner Surface桥第一短审结论为NO（P0=4/P1=1/P2=0）。Tool Owner仅修订资产，未写Go、未修改Model/Harness/Application/Runtime、未stage/commit。

返修合同：

- Model Prepared historical Fact Ref与Current Projection分层；RequestTools/Route/Profile/ActualInjection/Checked/Expires只来自未来Model public Current Projection，历史Fact retention不授current资格；
- Harness Assembly采用一个sealed composite current watermark，内部证明Generation/Handoff/BindingSet并输出共同min；Tool只存exact composite与closed-kind refs；
- public Writer只接exact EnsureRequest；Tool internal Owner clock生成Created/NotAfter、构造private CommitRequest并由唯一Repository create-once；
- lifetime不冻结任意秒数。RequestedNotAfter必需，NotAfter取Owner currents、caller deadline与版本化policy可选cap共同min；policy缺失Fail Closed；
- 每Invocation单Binding。同Invocation内Tools/ToolChoice/ParallelToolCalls/ActualInjection不变，变化必须新Invocation epoch；
- Binding/Ack只证明Invocation到Surface的历史因果Fact，不授Provider执行权。每个provider attempt/Stream/Open/continuation边界重新InspectExact+ValidateCurrent，跨TTL Fail Closed。

## 当前门

P0=4/P1=1/P2=0保持不变，等待Model/Harness最终public nominal、逐字段mapping、所有Provider路径统一Gate与独立复核归零。Tool Go、PD-TM-04 P4、system与production继续hard-block。

## 入口

- [SurfaceInvocationBinding V1](../../design/tool-engine/surface-invocation-binding-v1.md)
- [PD-TM-04第七设计修正](../../design/tool-engine/pd-tm-04-seventh-candidate.md)
- [Tool/MCP计划](../../plan/tool-mcp/tool-mcp-v1.md)
