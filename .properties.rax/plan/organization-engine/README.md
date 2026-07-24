# Organization Engine 实施计划

## 状态

- Review Human Multi-Sign 所需最小 Organization current Owner 已完成 Owner-local 实现和验证。
- Review consumer Port与Organization adapter已在Review Owner公开合同中冻结；历史Delta见 [review-consumer-port-delta-v1.md](../../design/organization-engine/review-consumer-port-delta-v1.md)。production composition root仍未建立。
- 声明式Release见 [component-release-v1.md](component-release-v1.md)：Organization Owner P0/P1代码候选与普通100/race20/full门已完成，等待独立代码审计；Review条件依赖variant与production root仍未完成。

## 实施 DAG

```text
contract -> ports -> memory ---------+
                  -> storage/sqlite -+-> current reader -> conformance/tests -> module/memory
                  -> release contract/readiness/publisher -> Assembler catalog candidate
```

## 文件级落点

| 阶段 | 路径 | 完成条件 |
|---|---|---|
| Contract | `ExecutionRuntime/organization-engine/contract/**` | 四类 immutable facts、exact refs、seal/validate/current |
| Ports | `.../ports/**` | publish CAS、historical exact、current resolve、Review eligibility exact reader |
| Reference | `.../memory/**` | append-only、atomic history/highest/current、deep clone |
| Production backend | `.../storage/sqlite/**` | WAL、schema digest、single tx CAS、restart/integrity |
| Reader | `.../current/**` | S1/S2、stable projection、min TTL、clock rollback、closed errors |
| Conformance | `.../conformance/**` | reusable Store/Reader suites |
| Tests | `.../tests/**` 与包内测试 | unit/whitebox/blackbox/fault/concurrency/count/race |
| Handoff | `module/organization-engine`、timestamp memory | 说明能力、限制、真实命令结果 |

## 测试门

1. `go test ./...`；
2. targeted `-count=100`；
3. targeted `-race -count=20`；
4. `go test -race ./...`；
5. `go vet ./...`；
6. `gofmt`、diff/import scan。

## 明确不做

- 不导入或复制 Review V2 类型；不签发 Runtime Authority、Review Verdict 或 Evidence。
- 不提供网络 API/UI、远程 Provider、HA/SLA 或 production root。
- 不将 memory backend 宣称为生产；SQLite 仅单机 v1 State Plane。
- 不在其他外部 Owner 和 root 未关闭前宣称 Review Human Multi-Sign 整体 production GO。
