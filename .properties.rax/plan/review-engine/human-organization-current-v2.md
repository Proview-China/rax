# REV-H12 Human Organization Current Consumer V2 实施计划

## 当前状态

- Review-owned public consumer Port、只读 adapter、测试与 reusable conformance 已实现并通过最终独立复审；真实Organization production backend/root仍NO-GO。
- Organization public Reader 已复用，未改 Organization Owner。
- Runtime V5/current Authorization 与 `agent-host` production composition root 仍是发布阻塞；本计划不把 owner-local 完成写成 production GO。

## 文件落点

| 文件 | 产出 |
|---|---|
| `ExecutionRuntime/review/ports/human_organization_current_v2.go` | Review request、Owner exact ref receipt、set cut、seal/validate、窄 Reader Port |
| `ExecutionRuntime/review/multisigcurrent/organization_v2.go` | Resolve 新 S1、exact S1/S2、fresh clock、min TTL、deep clone、closed recovery |
| `ExecutionRuntime/review/conformance/human_organization_current_v2.go` | reusable consumer conformance |
| `ExecutionRuntime/review/multisigcurrent/organization_v2_test.go` | unit/whitebox/blackbox/fault/concurrency 反例 |
| `ExecutionRuntime/review/go.mod` | 只依赖 Organization public module；无 implementation reverse dependency |

## 完成门

1. targeted `OrganizationCurrentV2` ordinary100；
2. targeted race20；
3. Review full ordinary/race/vet；
4. gofmt/diff/import scan；
5. 独立审查 P0/P1/P2；
6. Runtime V5 与 production root 单独联合验收，未通过前保持 NO-GO。

## 下一依赖

```text
Organization Current V1 public Reader
        -> Review Human Organization Current V2 cut
        -> Review Multi-Sign external-current aggregate
        -> Human Verdict V2
        -> Runtime Review Authorization V5
        -> agent-host production root / Gateway
```
