# MCP Cancel、Drain、Close最小Port Delta候选

时间：2026-07-18 12:59:00 +08:00

## 事件

- 基于live official MCP Go SDK v1.6.1、Runtime Connect/Discovery矩阵及Tool Call actual-point合同，
  将Inspect、普通Cancel、Drain、Close和Task操作拆分；
- 本地exact Inspect已闭合；普通Cancel与Close都是新的外部Effect，分别需要Action五维与Run/Session
  Runtime矩阵/actual-point Port；Drain只做Tool Owner本地准入CAS；
- 普通Call没有通用远端Inspect，Cancel/Close/NotFound都不能证明原Effect未发生；
- Task因SDK没有可复用public nominal继续NO-GO。

## 状态

该资产是待用户与Runtime/Application联合冻结的Port Delta候选，不是实现授权或production GO。
当前Tool Go未新增Cancel/Drain/Close写路径。
