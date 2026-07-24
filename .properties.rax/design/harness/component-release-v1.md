# Harness Component Release V1

## 1. 裁决

Harness可以向Agent Assembler发布完整的声明式`ComponentReleaseV1`候选，但当前只能是`reference_only`。

现有Assembly current、Controlled Operation Provider Route current和Model PreDispatch CommitGate均已达到owner-local实现/软件测试可用；它们不能替代生产持久Store、Model actual-point全路径no-bypass、真实Tool/Application/Context接线或production composition root。

## 2. 发布闭包

| 对象 | 固定值 | 边界 |
|---|---|---|
| Component | `components/harness` | Harness Owner |
| Kind | `praxis/harness` | 不创建Host kind分支 |
| Capability | `praxis.harness/run-loop` | reference-only |
| Module | `module/harness` | 单一声明式模块 |
| Port | `port/harness/run-loop` | stale/route/gate/current漂移Fail Closed |
| Factory | `factory/harness` | 只有`ModuleFactoryDescriptorV1`，没有可执行Factory |
| SupportMode | `reference_only` | 无本地升级API |
| Conformance | `restricted_controlled` | Residual=`inspectable` |

Release必须携带完整九项计划资产：Harness Bootstrap、Profile、Runtime Policy、Harness Stack、Semantic Route、Context Plan、Tool Surface、Capability Grant、Expected Injection Manifest。缺失或重复role直接拒绝。

## 3. Owner-local事实与生产事实分离

`ConformanceV1`只承认三个owner-local事实：

- Assembly current Reader/Publisher已实现；
- Route Declaration/Conformance/Current已实现；
- Model公开CommitGate的Harness concrete Gate与ACK Repository已实现。

同一Conformance固定否认：持久Store、production Route、Model actual-point guard、production Continuation、可执行Factory和SLA。调用方不能通过改字段、改SupportMode或替换proof集合完成自提升。

## 4. 发布恢复

`PublisherV1`只依赖Agent Assembler公共`ComponentReleasePublisherV1`和`ComponentReleaseReaderV1`。`Ensure`返回indeterminate时，只用`context.WithoutCancel`按完整Release Ref执行一次exact Inspect；禁止重试发布。发布后重新采时并校验TTL、时钟单调性和返回Release exact一致性。

## 5. Production P0

1. 生产持久Session/Event、Assembly current及Route current Store；
2. production Route wiring current与ProviderTransport/actual Provider双层no-bypass；
3. Model所有actual-point的ACK guard、inventory、receipt和纯Preparation闭环；
4. Tool Consumer与Application生产协调current；
5. Context Refresh、Harness Continuation及真实Turn推进；
6. 可执行Factory的可信绑定、Cleanup Conformance和部署证明；
7. production composition root真实接线与验收。

以上proof未全部由各Owner提供并由宿主root复读前，Harness Release不得进入`standalone`或`production`。
