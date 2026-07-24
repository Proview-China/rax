# AgentDefinition V1 实施计划

## 1. 状态和预期产物

- 状态：计划已审核通过；P1-P4 owner-local/reference 实现候选及终审返修已完成，等待独立终审。生产持久 Backend、跨进程 SLA 与 trusted extension validator 未完成。
- 代码候选根：`ExecutionRuntime/agent-definition/`。
- 完成后产物：严格 YAML decoder、public contract、Definition repository/current reader、CLI/API 可复用的 validate/seal 服务、conformance testkit。

## 2. 代码包

```text
ExecutionRuntime/agent-definition/
|-- contract/        Source/Definition/Ref/current DTO、Validate/Seal/canonical
|-- decoder/         YAML 安全子集 -> strict semantic tree
|-- ports/           Repository/CurrentReader/Service
|-- store/           线程安全 reference backend；production adapter 只实现 ports，不预选数据库
|-- conformance/     第三方实现 testkit
|-- tests/           unit/blackbox/fuzz/fault/import
`-- README.md
```

禁止依赖 Runtime/Harness/组件实现；仅可依赖稳定 canonical/error 基础包。若直接复用 Runtime core 会造成模块拓扑耦合，实施前由集成 Owner 选择抽取独立公共基础包或保留单向依赖，禁止复制摘要算法。

## 3. 阶段

### P1 公共合同

- [x] 冻结全部 JSON 字段、domain/type discriminator、nil/empty、集合排序规则；
- [x] 实现 Source、Definition、Ref、current pointer 和 Runtime `core.DomainError`；
- [x] 实现 component/policy/secret/extension 声明校验；
- [x] 首版 6+1 required+production 矩阵；
- [x] 自定义 namespaced 项不走 hardcoded kind switch；
- [ ] trusted production extension 的 exact schema governance 与专属 validator Port。

### P2 严格解析

- [x] 选择支持 parser event/node 检查的 YAML 库并锁版本；
- [x] 在构造语义树前拒绝 duplicate/merge/anchor/alias/tag/non-string key；
- [x] scalar allowlist 和整数范围；
- [x] unknown field、multi-document、trailing content 拒绝；
- [x] 输出 strict JSON semantic tree 和 source digest。

### P3 Owner Repository

- [x] Definition create-once、revision、history/current、revoke/expire；
- [x] lost create reply exact Inspect；
- [x] immutable history + current pointer CAS 与 highest checked 防 ABA；
- [x] 线程安全 reference store 与扩展后的 owner 语义 conformance；
- [ ] production 持久 adapter、跨进程 CAS、durability/availability/SLA 认证；
- [x] Reader 只读最小接口，写口不注入 Assembler/Host consumer。

### P4 服务与验收

- [x] `ValidateSourceV1`、`SealDefinitionV1`、`InspectExactDefinitionV1`；
- [x] Approval S1/S2 TOCTOU 门、fixture config 与结构化错误输出；
- [x] ordinary100、race20、full ordinary/race/vet/gofmt/import/diff；
- [x] fuzz/property 覆盖 decoder 与 canonical；
- [ ] ID/version 专项 fuzz 尚未单独建立；
- [x] module/memory/design/plan 同步。

## 4. 测试矩阵

| 类别 | 必测 |
|---|---|
| unit | 所有 Validate/Seal/Clone、状态机、错误类别 |
| whitebox | parser node/event、duplicate key、canonical bytes、CAS |
| blackbox | YAML 文件到 sealed JSON/ref，全程零网络/Provider/Sandbox |
| fault | lost reply、reader unavailable、clock rollback、expired approval |
| concurrency | 64 same-content、64 changed-content、revision race |
| security | secret 明文、绝对路径、自定义 tag、alias bomb、oversize |
| compatibility | V1 reader/writer、unknown optional preserved、required rejected |

## 5. Plan 完成门

用户已审核本 Plan，允许按 P1-P4 实现；owner-local/reference 候选完成不等于 production release。任何新增公共 canonical 基础模块、Extension Validator Port 或治理目录 exact schema binding Delta 仍必须先单独确认。
