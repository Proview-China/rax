# Organization Engine Component Release V1 实施计划候选

## 1. 状态

- 状态：设计与本计划已获用户确认；P0/P1 Owner实现开始，Review Owner的P2独立推进。
- 设计：[Component Release V1](../../design/organization-engine/component-release-v1.md)。
- 产物：Organization声明式Release、local/production Readiness、Publisher/Conformance、Human Multi-Sign Review依赖variant及测试；不建立production root。

## 2. 实施清单

### P0 合同与资产

- [x] 冻结Component/Capability/Port/Schema/Owner身份；
- [x] 冻结reference-only/standalone/production Readiness与同revision不可提升；
- [x] 冻结Human Multi-Sign Review variant的Organization required dependency；
- [x] 冻结ResourceBinding/cleanup/deployment/certification exact refs；
- [x] canonical、TTL、lost-reply、64并发与import DAG矩阵。

### P1 Organization Owner实现

- [x] `ExecutionRuntime/organization-engine/release/{contract,ports,publisher,conformance}.go`；
- [x] memory缺readiness时固定reference-only；
- [x] SQLite exact local readiness只允许standalone；
- [x] production readiness只绑定Organization自身Local/ResourceBinding/cleanup/deployment/executable-factory/certification exact current并fail closed；
- [x] Organization readiness不反向依赖Review consumer；consumer current只进入Review variant/SystemReady；
- [x] descriptor factory明确不等于executable factory；后者仍是H4 production P0。

### P2 Review依赖variant

- [ ] Review Owner独立发布Human Multi-Sign Release variant；
- [ ] manifest/capability/assembly三层required edge一致；
- [ ] automatic variant不暗含Organization；
- [ ] missing/drift/expiry/unavailable不得降级。

### P3 验收

- [x] Organization Owner unit/whitebox/Assembler blackbox/fault/conformance；
- [x] Organization Owner ordinary100/race20/full ordinary/race/vet；
- [ ] Assembler解析两种Review profile；
- [ ] H4 Binding/SystemReady条件依赖系统反例；
- [x] Organization module/memory同步；Agent Host readiness table由Host Owner在独立审计后更新。

## 3. 完成门

Organization P0/P1候选与Owner软件门已完成，当前等待独立代码审计；P2 Review variant和P3跨Owner/H4门仍未完成。即使独立审计通过，也只能标记`assembly-candidate`或在exact SQLite local readiness下标记`standalone`。只有H4真实ResourceBinding、executable factory、deployment/certification和production current闭合后，才能发布production；不得把SQLite owner-local完成直接写成系统GO。
