# Application Adapter V2第二独立复审NO

时间：2026-07-18 12:27 +08:00

## 当前事实

- Tool Owner内部返修已闭合持久claim、重启只Inspect、同请求Binding漂移冲突、Result Store保存并验证Request+Result exact record、ToolResult到DomainResult/Runtime Settlement/ApplySettlement确定性因果、settled ToolResult Owner重读、SandboxLease deep clone、单调时钟与lost result create权威错误。
- 定向ordinary×100：PASS，511.883s。
- 定向race×20：PASS，1186.629s。
- Tool模块full ordinary/shuffle、full race、vet、gofmt、go mod tidy diff与git diff-check：PASS。
- 第二次独立代码复审：`NO(P0=1/P1=1/P2=0)`。

## 唯一P0

Application在入口、inspect-only与finalize仍要求Request/Input/Settlement current。原Tool attempt进入unknown后若TTL过期，现有Application不能只读恢复历史truth。

最小跨Owner Delta必须由Application Owner冻结并实现：

1. 注入只读`SingleCallOperationSettlementHistoricalReaderV2`，调用Runtime既有`InspectOperationSettlementClosureV4`；
2. 为Application Result增加`ValidateHistoricalFor(request, closure)`；
3. 增加`CompleteSingleCallToolActionCoordinationRecoveredV2`，只允许历史`waiting_inspect`完成CAS；
4. 固定顺序为`waiting_inspect -> Tool.Inspect(exact key) -> Runtime historical V4 closure -> exact historical validation -> completion CAS`；
5. 全程不得重读Binding/Input current，不得Execute。

## P1与边界

`NewToolOwnerSingleCallFlowV2`仍是source-compatible fixture-only便捷构造器并使用进程内claim store。production composition前必须以import/conformance或强类型profile保证只注入`NewToolOwnerSingleCallFlowWithStoresV2`及持久Owner store。

本事件只记录Tool Owner软件返修与跨Owner阻塞，不授production backend/root、durability、Capability、Provider、网络/RPC或SLA，也不把整体G6A写成完成。
