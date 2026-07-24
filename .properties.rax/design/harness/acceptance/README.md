# Harness首切面验收门禁

> 历史验收快照：测试数量和覆盖率仅代表当时基座，不作为Assembly V1当前完成面。新的公用接线验收见[Assembly验收设计](../assembly/acceptance.md)。

## 1. 正常闭环

- 两种内部路径通过相同Manifest、Kernel与Runtime Port合同：直接完成；Action请求→Gateway结果→完成；
- Runtime Foundation完成Activation、Ready、Run关联、Harness Control、Stop和Cleanup；
- Event source sequence单调，Harness不伪造Ledger sequence；
- Completion Claim不直接成为Runtime ExecutionOutcome。

## 2. 失败与安全

- 配置/Profile/Harness stack/Requirement Digest漂移在外部调用前拒绝；
- Capability过期或Conformance不足拒绝绑定；
- 错误ActionRef、重复Start、终态复活和迟到Model结果拒绝；
- Event持久化/背压失败时不派发下一Model Turn；
- Cancel传播并保持单调；
- 未支持Checkpoint明确unsupported；
- 未经Review的Action只产生候选，不执行真实Tool/MCP；
- 过期/错误Fence和未持久化Intent拒绝Model Turn。

## 3. 自动化门禁

测试分为三层：

| 层级 | 位置 | 关注点 |
|---|---|---|
| 单元测试 | `contract`、`ports`、`fakes`包内 | 所有公共值对象、摘要、TTL、Conformance、Intent/Fence和测试替身 |
| 白盒测试 | `kernel`、`runtimeadapter`包内 | 内部状态、上限、锁/隔离、故障注入、序列化、Endpoint和Capability分支 |
| 黑盒测试 | `tests/contract`、`tests/kernel`、`tests/runtimeadapter` | 只经公开API验证direct/action/input/cancel与Runtime Foundation完整生命周期 |

最终门禁：

```text
go test -count=1 -shuffle=on ./...
go test -count=20 ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
go test -count=1 -coverpkg=./contract,./ports,./kernel,./runtimeadapter,./fakes -coverprofile=<path> ./...
```

当前39个顶层测试全部通过，生产包跨包语句覆盖率89.7%。同时复跑Runtime与model-invoker普通/Race/Vet完整回归。任何真实Harness、Model、Tool/MCP或账号验证必须另获授权。

## 4. 已覆盖故障矩阵

- 配置：缺失Port、零上限、错误摘要、过期证据、低于最低Conformance、重复Residual、Manifest绑定漂移；
- 输入输出：Opaque内容替换、错误Schema、非法JSON、解码丢字段、非法Context/Model结果；
- 状态：同Scope第二活跃Run、Run ID复用、错误ActionRef、终态继续Input、重复Cancel、缺失Run；
- Effect：未持久化Intent、过期/错配Fence、Control缺失Fence；
- 故障：Context失败、Model失败、Event首写/中途/终态写失败、Turn/Event上限、取消阻塞调用、迟到结果；
- 隔离：不同ExecutionScope并发互不阻塞，关闭/陈旧/异属Endpoint全部Fence；
- 黑盒：fully/restricted两种Conformance，direct、Action Gateway、Input continuation、运行中Cancel，以及Runtime Activation/Ready/Stop/Cleanup。
