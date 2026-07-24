# Agent Host V1 验收

## 1. 组合验收

- 使用一份 YAML 经 Definition -> Assembler -> Harness Compile -> Runtime Binding -> Host Start 形成一个完整 Agent。
- 6+1 各至少一条真实 public-port 路径接入；不以 fake、internal、testkit、standalone CLI 代替。
- 自定义 namespaced 组件通过 Release + public factory 接入，不修改 Host switch。
- CLI 五个命令调用同一 Host API，不能直写 Store/Gateway。

## 2. 白盒/单元

- Factory Registry exact key、alias、duplicate、artifact/version/capability drift。
- 生命周期状态机、reverse-DAG shutdown、typed nil、partial construction cleanup。
- Host journal create-once/CAS/lost reply/restart。
- Ready projection 对 Plan/Generation/Handoff/Binding/6+1 current refs 的完整 exact 校验。
- import boundary：禁止 foundation/fakes/internal/testkit/raw provider SDK。

## 3. 黑盒

| 场景 | 期望 |
|---|---|
| `validate` 合法配置 | 输出 sealed Definition ref；Provider/Sandbox 调用 0 |
| `assemble` 合法配置 | 输出 Plan/BindingPlan/Generation refs；业务 Effect 0 |
| `run` 完整生产配置 | Runtime Admission 后才分配 Sandbox/调用 Provider |
| 缺任一 6+1 release | Start 前 fail closed |
| 配置含 secret 明文/自由 factory | InvalidArgument，零写入 |
| stop | fence/reconcile/settle/cleanup 后报告各维度，不伪造全回滚 |

## 4. 故障与并发

- 64 并发同 Start：Command/Activation/Instance 只线性化一次。
- resolve/compile/binding/construct 回包丢失：按 exact ref Inspect；不产生第二实例。
- Provider/Environment unknown：隔离并 Inspect，禁止盲重试。
- current TTL 在启动途中跨界、clock rollback、Binding drift：Ready 前或实际执行点 fail closed。
- 构造第 N 个组件失败：反向释放已构造资源；unknown/residual 分维度保留。
- Host crash/restart：不依赖进程内 map 恢复事实。

## 5. 系统门

```text
Definition parser unit/fuzz
  -> Assembler unit/property/conformance
  -> Harness compiler ordinary/race/vet
  -> Runtime/Application ordinary/race/vet
  -> each 6+1 owner ordinary/race/vet
  -> all-6+1 public-port fixture
  -> production-root smoke + crash recovery
```

最终 `SYSTEM_READY` 需要上述全部通过、生产后端配置已明确、真实进程可启动并完成最小任务。仅 owner-local 或 test-only fixture 通过时只能标为组合候选。
