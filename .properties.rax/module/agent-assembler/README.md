# Agent Assembler 模块入口

## 当前真值

- `Agent Assembler V1` 的 owner-local 实现已经落在 `ExecutionRuntime/agent-assembler/`。
- 已实现三输出：`ResolvedAgentPlanV1`、Runtime `BindingPlanV2`、Harness `AssemblyInputV1`。
- 已实现 exact Facts/Catalog readers、Component Release 公共合同/testkit、确定性 resolver、create-once plan repository、revisioned current projection CAS。
- current projection 使用 exact projection ref CAS，revision 单调递增，并拒绝历史 Plan 回滚/ABA。
- production Component Release 必须具备 Manifest/CapabilityDescriptor/Module/Factory/Port exact 构造闭包；remote 组件同样必须提供 host adapter factory。
- CertificationRef digest 绑定完整 release payload；Catalog 对同一 ReleaseID 只允许一个 current revision。
- 6+1 七个核心组件和 namespaced 自定义组件走同一条 Catalog/Release 解析路径。
- 现有 Harness Compiler 已实际接受生成的 AssemblyInput，并产出 sealed Generation/Manifest/Graph/Handoff。
- `repository.SQLiteV1`已持久化ResolvedPlan history与current CAS/history，具备schema/row digest、严格decode、复合外键、重启/lost reply、64独立Store与ABA验证；仅声明单节点本机crash durability，不声明HA/SLA。
- Resolution Facts与Component Release Catalog没有公开Owner Repository写口，本轮没有另造第二Owner；production仍需外部exact Reader。

## 边界

```text
AgentDefinitionV1 + ResolutionFactsV1 + ComponentReleaseCatalogV1
                               |
                               v
                    deterministic resolver
                               |
              +----------------+----------------+
              |                |                |
              v                v                v
      ResolvedAgentPlanV1  BindingPlanV2  AssemblyInputV1
                                                |
                                                v
                                        Harness Compiler
```

Assembler 只拥有解析、canonical plan 和映射；不拥有 Release 事实、Provider、secret、Sandbox lease、Binding Fact、Generation、Instance 或 Run。

## 软件验收

| 门 | 结果 |
|---|---|
| 普通定向 100 轮 | PASS |
| race 定向 20 轮 | PASS |
| 七核心 production 正向 + Harness 实编译 | PASS |
| 64 并发确定性 / create-once conflict | PASS |
| Catalog 结构 key / future-created release | PASS |
| current stale CAS / lost reply / ABA rollback | PASS |
| production 缺 Factory/Port、capability/module splice、认证漂移、多 current revision | PASS |
| full ordinary / race / vet | PASS |
| 双 fuzz 2s（189 + 190 execs）/ import boundary / gofmt / diff-check | PASS |

这表示 Assembler owner-local/reference-test 已完成；整体 production/SystemReady 仍依赖各组件 Owner 发布真实 production Component Release，并由 Host/Runtime 完成后续 activation 链，不能把本模块完成写成整体 GO。

## 入口

- 实现说明：`ExecutionRuntime/agent-assembler/README.md`
- 公共合同：`ExecutionRuntime/agent-assembler/contract/`
- Resolver：`ExecutionRuntime/agent-assembler/resolver/`
- 黑盒与故障验收：`ExecutionRuntime/agent-assembler/tests/`
- 设计：`.properties.rax/design/agent-assembler/`
- 计划：`.properties.rax/plan/agent-assembler/`
