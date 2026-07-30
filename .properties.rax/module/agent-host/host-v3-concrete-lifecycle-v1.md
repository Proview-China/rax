# HostV3 可执行参考生命周期模块说明

## 代码入口

- `contract/host_v3_pipeline.go`：Owner exact-ref聚合投影；
- `ports/host_v3.go`：窄Owner pipeline接口；
- `journal/coordinator_v2.go`：同一Journal Owner对V3 Claim的additive acceptance；
- `lifecycle/host_v3.go`：HostV3 Start/Inspect/Stop coordinator；
- `lifecycle/host_v3_test.go`：真实SQLite reference blackbox。

## 所有权

HostV3只拥有进程级编排，不拥有Deployment、Ready、Availability、Cleanup、Runtime或6+1领域Current。Pipeline返回的每一个结果都必须是Owner exact ref；Host只能验证和聚合。

## 发布边界

当前实现可用于合同、持久化、跨8个Host/SQLite handles并发、test-only projection store重启恢复和错误语义验证。它没有production pipeline、daemon、CLI或模型执行，禁止把测试pipeline、引用ref或SQLite单机验证写成完整Agent可运行证明。

并发验收中的Owner pipeline由同一进程mutex串行；production多进程Owner operation线性化仍是明确NO-GO。
