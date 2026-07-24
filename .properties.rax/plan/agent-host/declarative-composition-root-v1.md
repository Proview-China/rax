# Declarative Agent Composition Root V1 实施计划候选

## 1. 状态

- 状态：候选，待用户设计审核；所有实施项未授权。
- 独立设计反审：`YES（P0/P1=0）`；实施门仍关闭。
- 设计：[Declarative Composition Root V1](../../design/agent-host/declarative-composition-root-v1.md)。

## 2. 候选产物

```text
ExecutionRuntime/agent-host/
|-- bootstrap/
|-- composition/root_v1.go
|-- cmd/praxis-agent/
|-- api/
`-- tests/system/
```

不预选远程 RPC、生产数据库品牌或进程拓扑；首个实现可使用已验证的本机 SQLite，但不得宣称 HA/SLA。

## 3. 波次

### R0 公共门

- [ ] 用户审核 Activation V2、CleanupClosure V2 与本 Root 候选；
- [ ] 冻结 additive Host Service Contract V3、既有HostStartClaimV1同key/store的InputV3原子sidecar与V1/V2 reference兼容边界；
- [ ] 冻结 production 与可选 governed-local 版本策略；
- [ ] 冻结 bootstrap schema、API、错误 closed set 与 import DAG。

### R1 Owner factories/readers

- [ ] Definition Approval/Source current；
- [ ] Resolution Facts/Catalog persistent Owner；
- [ ] Definition/Assembler/Compiler 的 production StartOrInspect/Inspect operations；
- [ ] Definition/Plan/Catalog additive read-only exact Readers，禁止向Host泄漏Ensure/CAS；
- [ ] Host Service V3 Stage Inputs Assembler；
- [ ] HostCleanupClosure Fact/Plan Template/typed cleanup envelope；
- [ ] Application Agent Activation V2八步Owner合同；
- [ ] Runtime/Application/Harness/Sandbox adapters；
- [ ] 至少七个核心组件及额外catalog组件的release/current/executable factory/cleanup；
- [ ] custom component kind/capability/schema trust/certification/deployment/cleanup registration conformance。

### R2 Root 与 CLI/API

- [ ] strict bootstrap validation；
- [ ] `HostBootstrapConfigV1`、`HostDeploymentCurrentV1`与Deployment Owner typed Shutdown/Inspect port；
- [ ] Definition create/publish/current与simple-config `run`入口；
- [ ] Deployment/Bootstrap Owner负责durable store open/migrate/recover/health并发布typed ResourceHandle/Deployment current；Root只消费；
- [ ] sealed registry and resource bindings；
- [ ] `validate`只做strict decode/Validate零写；`assemble`只接受exact DefinitionSourceCurrent并通过production Assembler operation持久化/Inspect Plan，不触发Host lifecycle或Provider；`run`可先发布Definition配置事实，再映射Host Service `StartV3`；
- [ ] Host control API在Ready前开放Start/Inspect，业务Run/Provider surface仅SystemReady后启用；
- [ ] StartV3/InspectV3/StopV3，以及V2/V3同HostID+StartID的共享Claim冲突；
- [ ] ClaimV1+InputBindingV3同事务create-once、lost reply、缺sidecar indeterminate与V1/V2/V3双向竞争；
- [ ] CLI stable IDs到StartClaim/CleanupClosure exact refs的只读解析；
- [ ] restart recovery、Closure前/后的first/second signal、Deployment Owner shutdown、退出码0/2/75、unknown可Inspect、diagnostics；
- [ ] no string constructor/no raw provider/import boundary。

### R3 系统测试

- [ ] YAML 与 JSON strict decode；
- [ ] 至少七个核心production components并允许catalog额外组件的Definition；
- [ ] non-test Build -> Start -> Ready -> Inspect -> Stop；
- [ ] Claim-before-Definition/Assembler/Compiler owner-call与双版本同ID竞争；
- [ ] Inspect digest/Ready/Closure splice与Closure-derived Stop；
- [ ] bootstrap open/migrate unknown恢复、Root zero-open-call；
- [ ] 命令隔离：`validate`零写、`assemble`仅产生/恢复ResolvedPlan Effect、`run`是唯一Host StartV3入口；
- [ ] first/second signal、Closure前/后、cleanup unknown与退出码0/2/75矩阵；
- [ ] crash at every write/call boundary；
- [ ] TTL/clock/revoke/drift/unknown/residual；
- [ ] 64 root concurrency；
- [ ] custom component positive/negative；
- [ ] ordinary100/race20/full ordinary/race/vet/gofmt/import/diff；
- [ ] binary smoke、restart smoke、zero fake/internal/testkit scan。

## 4. 当前门

R0-R3 前，仓库只具备库级声明/装配与 HostV2 参考纵切，不得发布 `praxis-agent` binary 或声明简单配置已可运行完整 Agent。
