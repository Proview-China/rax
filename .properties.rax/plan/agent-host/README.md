# Agent Host V1 与全 6+1 系统集成计划

## 1. 状态和预期产物

- 状态：计划已审核通过；H1-H3、P4组件声明候选与HostV2 Start/Inspect参考纵切已完成。live 11个目标域均已有Release/ReleaseCandidate包，但没有任何production readiness真实发布。真实Activation、Cleanup Closure、H5及production Runtime/Application/6+1接线仍为NO-GO。
- 代码候选根：`ExecutionRuntime/agent-host/`。
- 完成后产物：唯一 production Composition Root、Host API、`praxis-agent` CLI、生命周期 journal、factory registry、readiness/no-bypass 检查和全 6+1 系统验收。

## 2. 代码布局

```text
ExecutionRuntime/agent-host/
|-- contract/        HostConfig/API/Result/Journal/Ready DTO
|-- ports/           仅 Host 生命周期所需窄接口
|-- registry/        build-time exact public factory registry
|-- composition/     唯一 DI root
|-- lifecycle/       start/recovery/drain/shutdown
|-- cmd/praxis-agent thin CLI
|-- tests/           unit/fault/import
|-- tests/system/    all-6+1 public-port/production smoke
`-- README.md
```

## 3. 实施波次

### H1 Host contract 与零副作用入口

- [x] HostConfigV1、HostV1、journal、ready projection；
- [x] validate/assemble/start/inspect/stop；
- [x] secret refs only、endpoint exact refs 与 sealed exact factory registry；
- [x] typed nil、closed errors、clock/revision/canonical、Port panic 隔离。

### H2 Factory Registry 与 Composition

- [x] exact ComponentID/artifact/contract/capability key；
- [x] Graph order construction；
- [x] extra factory 隐藏、alias/duplicate fail closed；
- [x] Factory Effect 采用 write-ahead attempt + canonical start-or-inspect，不允许隐藏不可追踪重试；
- [x] reverse-DAG partial cleanup、constructed progress 失败接管与清理有效 handle。

H1-H2与H3第一纵切的完成不表示production Composition Root或 `SYSTEM_READY`。H3第一纵切只覆盖Definition -> Assembler -> Harness Compile；后续接线继续未完成。

### H3 Runtime/Application/Harness 接线

- [x] Definition/Assembler/Compiler outputs exact 串联（第一纵切；见 [h3-owner-adapter-v1.md](h3-owner-adapter-v1.md)）；
- [ ] Binding/Generation Association/Conformance；
- [ ] Command/Outbox/Application Coordinator 唯一入口；
- [ ] Runtime Activation/Run/Settlement/Cleanup；
- [ ] Harness ExecutionPort/Model bridge/Action Gateway/Context refresh/Checkpoint 顺序；
- [ ] 不 import foundation/reference coordinator。

### H4 全 6+1 production 接线

HostV2 Start/Inspect参考纵切已经实现，但当前完整正向测试仍注入Application/Runtime fake，不能证明生产Activation。additive HostV2主计划见 [h4-production-lifecycle-v2.md](h4-production-lifecycle-v2.md)；[Cleanup Closure V2](cleanup-closure-v2.md)、[Application Activation V2计划](../application/agent-activation-v2.md)与[Declarative Composition Root V1](declarative-composition-root-v1.md)已完成独立设计复审（P0/P1=0），仍须用户审核后才可实施。

必须按领域 Owner 提供的 Release/Factory/Adapter 接入：

1. Sandbox（Activation 硬前置）；
2. Review/Authority/Budget/Credential current readers；
3. Tool/MCP actual-point 与 Provider transport；
4. Context/Cache + model injection/refresh；
5. Memory/Knowledge retrieve/commit/context source；
6. Continuity checkpoint/restore/timeline；
7. Harness 完整 run/session/event/continuation。

该顺序是启动依赖，不改变七个领域的语义独立性。各 Owner 的首轮Release声明候选已形成；Organization Release、真实readiness producer、executable factory、production adapters仍需并行补齐。descriptor factory不等于executable factory，Host不得据此提前构造。

### H5 CLI/API 与运行验收

唯一可执行Root与CLI候选见 [Declarative Composition Root V1](declarative-composition-root-v1.md)；独立设计复审已通过，但用户审核前不得落生产入口。

- [ ] `praxis-agent validate|assemble|run|inspect|stop`；
- [ ] CLI 仅调用 Host API；
- [ ] `validate/assemble` 业务 Effect 计数为 0；
- [ ] `run` 通过 Runtime admission 后才进入 Sandbox/Provider；
- [ ] inspect/stop 的 unknown/residual/cleanup 分维度报告；
- [ ] no fake/internal/testkit/raw Provider import。

## 4. 系统测试矩阵

现状与 Owner 准入见 [component-readiness-v1.md](component-readiness-v1.md)，可执行矩阵见 [system-test-matrix-v1.md](system-test-matrix-v1.md)。

| 波次 | 白盒 | 黑盒 | 故障/并发 |
|---|---|---|---|
| Definition | parser/canonical/CAS | YAML->sealed ref | fuzz、64并发、lost reply |
| Assembler | resolver/DAG/mappers | Definition->CompileResult | facts drift、TTL crossing |
| Binding | exact associations | compile->Runtime binding | CAS/lost reply/restart |
| Activation | admission journal | sandbox ready | unknown allocate/open |
| Run | command/outbox/claim/settlement | model turn/action/context | claim loss、Effect unknown |
| 6+1 | each owner state machine | minimal real path per domain | provider loss、cleanup residual |
| Host | registry/journal/reverse DAG | CLI five commands | crash/restart、64 Start |

## 5. 最终系统场景

一个测试 Agent 必须实际声明并使用：

- context/cache 生成首帧；
- model 调用产生一次受治理 tool/MCP action；
- review/authority/budget/credential 门禁；
- sandbox 承载 Harness；
- tool 结果触发 context refresh；
- memory/knowledge 各完成一次 retrieve 与受治理 candidate/commit；
- continuity 完成 checkpoint，并在新 Instance/更高 epoch 上 restore；
- Runtime 独立 settlement/cleanup 后关闭。

外部世界不可回滚、unknown effect、cleanup residual 必须被真实报告。

## 6. 验收命令层级

```text
各模块 ordinary100 + race20 + full ordinary/race/vet
-> Definition/Assembler/Harness compiler integration
-> Runtime/Application/Harness/Model/Tool fixture
-> all-6+1 public-port system fixture
-> production backend smoke
-> crash/restart/clock/TTL/concurrency matrix
```

只有最后一层通过才标记 `SYSTEM_READY`。test-only fixture 通过不等于 production root 可用。

## 7. 禁止项

- 不把 Runtime foundation、fakes、internal 或 testkit 注入生产 root；
- 不让 Host 成为 Fact/Permit/Verdict/Settlement Owner；
- 不从配置动态 import 任意包、脚本或 URL；
- 不在 constructor 隐藏 Provider/网络/资源副作用；
- 不以单组件 standalone CLI 代替全链接线；
- 不预选未评审的数据库、RPC、Sandbox backend 或 SLA。

## 8. Plan 完成门

用户已审核本Plan。H1-H2与H3第一纵切已完成；其余Runtime/Application/Harness接线及各Owner production Release继续推进，H4-H5由唯一Host集成任务串行接线。H4-H5未通过前不得声明production root可用。
