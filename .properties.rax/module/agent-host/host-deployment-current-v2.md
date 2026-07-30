# HostDeploymentCurrentV2 模块

## 代码入口

- `contract/deployment_current_v2.go`：V2 request、exact Ref、Current 与 Service Current projection；
- `ports/deployment_current_v2.go`：Builder/Resource/Service readers、raw repository、public Owner；
- `deployment/current_v2.go`：Host-owned publish/inspect currentness boundary；
- `storage/sqlite/deployment_current_v2.go`：append-only history/current CAS；
- `storage/sqlite/deployment_schema_v2.go`：v7 physical schema proof。

## 边界

Raw SQLite Store 的方法名故意不同于 public Reader，因而不能绕过 Builder/Resource/Service fresh revalidation。模块只形成 reference executable Owner；没有 Factory、Provider、Runtime write、Application workflow 或 HostV3 lifecycle 权力。
