# Runtime Evidence Ledger V2与RunClaim精确关联完成

## 事件

2026-07-14，Runtime P0.4完成Evidence Ledger V2、独立Source Policy治理、单主摘要链、Tombstone、late observation及V2 Run Claim精确持久关联的代码与测试收口。

## 已落地事实

- Ledger Owner原子提交Source cursor与Record，并独占ledger sequence；
- 封闭TrustClass与namespaced自定义class/kind分离，claim/authoritative资格由独立current Policy授予；
- Register、Renew、Append重读Binding、Authority、CurrentScope、Policy Owner/Authority、Run/Effect；
- Record绑定完整Scope、source key、因果、payload、Producer/Authority及治理水位；
- Append丢回包按source key Inspect，高位epoch/sequence无Unicode键碰撞；
- Timeline被限定为V2 Ledger只读投影，旧Evidence/Timeline不自动升级也不双写；
- RunClaimGatewayV2先落Evidence，再落create-once Association；Association恢复时复读exact Record；
- Claim不完成Run、不生成Outcome，P0.5仍需独立Settlement与CompleteRun。

## 保留限制

- 当前所有Evidence分区保守要求active running/stopping Run，pre-run证据仍未开放；
- 跨Fact Owner复读与Ledger append不宣称生产原子事务；
- 生产Backend、RPC、Scheduler、签名、retention和SLA未选择；
- Runtime+Harness+6+1执行基座尚不能解锁：P0.5 Run Settlement与P0.6 Application闭环仍未完成。
