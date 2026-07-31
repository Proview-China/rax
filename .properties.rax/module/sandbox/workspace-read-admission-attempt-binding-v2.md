# Workspace Read Admission Attempt Binding V2 Module

本模块提供Sandbox-owned exact历史反查：

```text
full Runtime Attempt
  -> WorkspaceReadAdmissionAttemptBindingV2
  -> Admission + original WorkspaceRead Attempt
```

公开能力只有
`WorkspaceReadRuntimeAttemptAdmissionReaderV2`。它不接受AttemptID或stable key，
也不授予current或执行权。实际创建由Sandbox kernel在完整S1 current closure后，
通过Go internal nominal capability进入SQLite同事务writer；外部Owner不能导入
该capability，也不能caller-mint Owner Fact。

SQLite schema v18持久化完整Runtime Attempt（含Delegation）、V1 binding、
Authorization/Association/Command/Admission/original Attempt及canonical digest。
Reader在单一只读事务中复读全部引用和denormalized列；V2 winner存在后的引用
缺失、canonical漂移或stable digest协同篡改都按corruption/Conflict处理。

lost reply与restart只能使用同一full Runtime Attempt反查original Attempt，然后
Inspect；不会再次读取物理文件。64并发创建只有一个winner，64并发读取不修改
任何相关SQLite行。

当前状态为`implemented-candidate / owner-local / software gates green`。
Tool full composition、跨Owner product root和production readiness仍NO-GO。
