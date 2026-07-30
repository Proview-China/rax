# HostV3 可执行参考生命周期 V1 实施记录

## 状态

- [x] additive `HostV3OwnerPipeline` 窄接口；
- [x] additive `CoordinatorV2.EnsureAcceptedV3`，V2行为不变；
- [x] concrete `lifecycle.HostV3` 实现 Start/Inspect/Stop；
- [x] SQLite Claim/Input sidecar与Journal真实黑盒；
- [x] 32并发跨8个Host/State Plane handles；同进程reference pipeline mutex下只执行一次Owner pipeline；
- [x] Start/Stop lost reply只Inspect原操作；
- [x] 8个Host/SQLite handles并发、关闭旧pipeline后由test-only SQLite projection store恢复、同请求重放、V2/V3冲突、splice、expiry、unknown；
- [x] Ready/Availability同坐标验证；
- [ ] production Owner pipeline与非测试进程入口；
- [ ] production multi-process Owner operation intent/journal linearization；
- [ ] Model Loop、全6+1 executable factories、真实Cleanup execution。

## 验收命令

```bash
cd ExecutionRuntime/agent-host
go test -count=1 ./...
go test -race -count=1 ./lifecycle ./journal ./storage/sqlite
go vet ./...
```

本计划完成只表示reference lifecycle可执行和可恢复，不表示production GO。
