# Agent Activation V2 八步 Owner 合同候选

## 事件

对Application V1 activation coordination与live Runtime/Sandbox/Harness ports进行只读审计后确认：新增full coordination reader只能证明Application因果链，不能证明八步权威current，V1无法接真实production Activation。

新增待用户审核的 `agent-activation-v2` 设计与计划候选，逐步冻结Preflight、Snapshot、Identity/Budget、Sandbox Allocate、Activation Commit、Sandbox Activate、Execution Open和Ready Inspect的Owner、typed result、exact Reader、Intent/Fence及unknown inspect-only恢复边界。

## 当前门

- Application不签Runtime/Sandbox/Harness事实；
- generic Observation不能升级为authoritative current；
- V1保留reference兼容，production Host必须等待additive V2联合审核与实现；
- 当前未修改Go，也未解锁production Activation。
