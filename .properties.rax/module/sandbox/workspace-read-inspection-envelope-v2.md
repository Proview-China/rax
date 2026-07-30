# Workspace Read Inspection Envelope V2 Module

该模块提供 Sandbox Owner 的 additive exact inspection：

```text
Admission -> immutable origin AttemptRef rev1
          -> InspectBoundedWorkspaceReadV2(exact origin)
          -> sealed Envelope(origin + latest current rev1/rev2)
```

它解决 V1 response 只有 latest current、却没有响应级 origin lineage proof 的
缺口。SQLite 在单个只读事务中验证 origin、Reservation、Command、Admission
binding和current全部闭合；不会按stable key接受 caller 查询。

Reader 在事务读取前后分别取 initial/fresh owner clock，最终 Envelope 必须
按 fresh 封存。started current同时受全部已封存 execution TTL最短上界约束；
历史事实即使已过期，terminal current仍可用独立30秒内的inspection证明审计，
但不能据此恢复执行。Unknown和lost reply只能Inspect原Attempt，物理文件不会
重读。

V1保持兼容。当前模块仅为 Sandbox owner-local reusable public reader；
Tool adapter/full composition、Runtime Settlement、Host root及Console仍不在本
切片，不得标记为完整production闭环。
