# Agent Assembler V1 实施计划

## 1. 状态和预期产物

- 状态：计划已审核通过，P1-P3/P5 实现已授权并开始；P4 由各 Owner 分波次实施。
- 代码候选根：`ExecutionRuntime/agent-assembler/`。
- 完成后产物：Resolved Plan 合同、Resolution Facts/Catalog Readers、确定性 resolver、Runtime BindingPlan 与 Harness AssemblyInput mapper、Component Release conformance。

## 2. 包与导入 DAG

```text
agent-definition/contract
runtime/core + runtime/ports(Binding V2)
harness/assemblycontract
             |
             v
agent-assembler/contract -> ports -> resolver -> mapper
             |
             v
agent-host (consumer only)
```

Assembler 禁止导入 Runtime foundation/kernel/fakes/internal、Harness kernel/fakes/internal 和任意 6+1 实现。

## 3. 阶段

### P1 合同与 Readers

- [ ] ResolvedAgentPlanV1/Ref/current、ResolveRequest/Result；
- [ ] ResolutionFactsSnapshot/Ref/Reader；
- [ ] ComponentRelease/Ref/CatalogSnapshot/Reader；
- [ ] support mode 与 production certification；
- [ ] plan create-once repository/current reader。

### P2 Resolver

- [ ] SemVer、contract、artifact、capability、schema、locality；
- [ ] dependency DAG 和唯一 Provider；
- [ ] Owner/credential/residual/cleanup/extension 规则；
- [ ] 全 6+1 production 硬门；
- [ ] 自定义组件基于 Governance Catalog 通用解析。

### P3 三输出映射

- [ ] sealed ResolvedAgentPlanV1；
- [ ] Runtime BindingPlanV2 exact 映射；
- [ ] Harness AssemblyInputV1 全字段映射；
- [ ] 十个 Plan Ref 和 sealed empty artifacts；
- [ ] `CreatedUnixNano` 来自冻结 facts，重试确定性；
- [ ] 使用现有 Harness Compiler 做实际兼容验收。

### P4 Owner Component Release 波次

以下 Delta 由对应 Owner 实施，Assembler 任务只提供合同/testkit：

| Owner | 必须发布 |
|---|---|
| continuity | timeline/checkpoint/restore ports、participant/factory、state/cleanup refs |
| tool-mcp | surface/action/MCP/provider transport、credential/effect/inspect refs |
| memory-knowledge | retrieve/candidate/commit/forget/context-source、state refs |
| sandbox | Environment/lease/policy/current/enforcer/provider、cleanup refs |
| review | current/review/verdict/human-auto provider、effect refs |
| context-engine | prepare/refresh/frame/cache/prompt/injection refs |
| harness | stack/bootstrap/execution/model bridge/route/factory refs |
| runtime/application/model | 必要的中立 adapter/factory release，不拥有 6+1 语义 |

每个 Owner 在自己的 design/plan 增加 additive Delta，不由 Assembler 跨目录代写。

### P5 验收

- [ ] resolver unit/property/fuzz；
- [ ] Catalog lost reply/current drift/TTL crossing；
- [ ] 64 并发 Resolve；
- [ ] import boundary；
- [ ] ordinary100/race20/full ordinary/race/vet；
- [ ] 全 6+1 production release 正反例。

## 4. 故障硬门

- unknown catalog/current 只 Inspect，不切换 latest；
- Resolve 创建回包丢失只检查 deterministic PlanID；
- required release 非 production、certification 缺失、TTL crossing 时零 Assembly output；
- Assembler 不下载 artifact、不读 secret、不探测 Provider；
- 旧 Runtime ResolvedAgentPlan 不可自动提升。

## 5. Plan 完成门

用户已审核本 Plan。当前先做 P1-P3/P5；P4 可由 6+1 Owner 并行，但最终 Host Ready 依赖全部 P4 通过。
