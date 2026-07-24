# Tool/MCP G6A真实Owner Flow闭环完成

时间：2026-07-16 09:17（Asia/Shanghai）

## 本次闭合

- V2 Tool Owner事实已落地：ActionCandidate、ApplicationAttempt中立Reservation、Provider Observation/prepare+execute Enforcement/Consumption绑定、三种合法Outcome/Disposition组合、DomainResult短current lease、ApplySettlement与ToolResult；
- Coordination Watermark完成request→candidate→reservation→Runtime Attempt→Provider boundary→Observation→DomainResult→Apply→Result单调CAS；Candidate/Reservation currentness与TTL不得被重试延长；
- Application Adapter实现公共N=1 Port。Model exact Projection验证成功后才创建Watermark；相同canonical request使用按key协调，不同key不被全局锁串行；所有长调用后的current判断使用fresh clock并拒绝回拨；
- 新增production-neutral `ToolOwnerSingleCallFlowImplV1`，依赖仅为注入的公开Reader/Repository、Runtime controlled Provider Port和Settlement V4 Governance Port，不包含生产Provider/backend/root；
- Provider boundary CAS前复读同Attempt的execute Enforcement与Evidence Handoff。CAS成功后即按“可能已调用”处理；Provider丢回包或Observation暂不可用只Inspect原Attempt，绝不重新dispatch；
- Tool Owner独立Inspect Provider Observation后形成typed authoritative DomainResult；Runtime Settlement V4回包丢失只从current Inspection恢复，Association public Inspect闭合后才Tool Apply/Result；
- Provider Boundary与Tool DomainResult current adapter均逐字段无损映射Runtime公共exact类型，不扩权、不包装V3；
- 主闭环测试已改为真实Owner flow fixture：64并发同canonical单Provider、Provider/Settlement lost reply恢复、boundary unknown inspect-only、DomainResult current exact drift拒绝。原测试stub只保留Application keyed gate/TTL隔离测试，不再证明领域主闭环。

## 实际验证

- `go test -count=100 ./action ./applicationadapter ./runtimeadapter ./tests/conformance ./tests/fault`：通过；
- `go test -race -count=20 ./action ./applicationadapter ./runtimeadapter ./tests/conformance ./tests/fault`：通过；
- `go test ./...`：通过；
- `go test -race ./...`：通过；
- `go vet ./...`：通过；
- `gofmt -l .`：无输出；
- import boundary扫描未发现Application coordinator/kernel/fakes、Harness internal/kernel/fakes、Model internal或Runtime foundation/kernel/fakes依赖。

## 保留边界

- 当前没有production composition root、Provider/backend/transport/Credential、持久Store或SLA；内存Store和Provider/Settlement fixture只用于测试；
- Tool不调用Context/Harness、不Build Continuation、不启用Capability；G6B通过前禁止Turn推进；
- N大于1、batch和custom effect继续NO-GO。
