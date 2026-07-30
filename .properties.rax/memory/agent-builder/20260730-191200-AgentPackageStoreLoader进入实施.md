# AgentPackage Store + Loader 进入实施

## 当前真值

- PR #26 已合并，基线为 `main@13f63712`；
- 本切片在独立分支实现 durable Package store 与 exact Loader；
- Package store 只拥有 `AgentPackageV1`，Harness 四份 artifact 仍由其 Owner Reader 提供；
- Loader 必须重读并验证四份 artifact，不能把 lock 的存在当作闭包证据；
- lost reply 只能 Inspect 原 Package ref，缺证据返回 NotFound/Indeterminate；
- Host/Runtime/Factory/Console/TS 与 production/SystemReady 继续 NO-GO。

## 实施结果

- SQLite WAL/FULL create-once store、exact Inspect 与原 ref recovery 已实现；
- Loader 已实现 Package + 四份 Harness artifact 的重新读取、canonical digest 与交叉闭包验证；
- restart、32-way concurrent ensure、同坐标冲突、lost reply、ref splice、body/lock drift、lock-only NotFound 与 typed-nil 黑盒已通过；
- ordinary、race、vet、diff-check 已通过；等待独立 Draft PR，不合并。
