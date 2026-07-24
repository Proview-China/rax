# Review Condition V2 实施计划

## 1. 状态

- 设计输入：`../../design/review-engine/condition-v2.md`。
- 当前结果：Review-owned exact Condition set、Human/Auto/Quorum/Verdict与Runtime V4/V5只读投影已实现并纳入本域全门；Policy tuple current与host production root仍NO-GO。
- Owner-local设计：最终独立复审YES（P0/P1/P2=0）。
- production：**NO-GO**，等待REV-D12公共窄Reader、各Owner conformance与宿主root。
- 技术：未来默认Go；无benchmark证据，不规划Rust。

## 2. 范围与不做事项

本计划只负责：

1. Auto output、AttestationV1、VerdictV1的exact Condition set `omitempty`加法；
2. canonical sort/unique/digest、Target scope/TTL、production strict validator；
3. Auto/Human/Verdict Owner无损传递exact set；
4. 消费获批的`ConditionAdmissibilityCurrentReaderV2`；
5. legacy digest-only只读隔离和Runtime零授权；
6. unit/whitebox/blackbox/fault/conformance/race/vet。

不做：

- 不复制`ReviewConditionV2`或`ConditionSatisfactionFactV2`；
- 不实现Policy/Binding/Authority Owner；
- 不创建/CAS Satisfaction；
- 不改Runtime/Harness/Application/Model；
- 不建production adapter/root；
- 不把Trace/Fake/Provider回包当Timeline、Evidence或Satisfaction。

## 3. 依赖DAG

```text
Runtime public ReviewConditionV2 / DigestReviewConditionsV2
                     |
                     v
Policy Owner exact tuple-decision Reader + append-only current index
                     |
                     v
REV-D12 trusted-host aggregate Reader + Policy/Binding/Authority conformance
                     |
                     v
Review exact-set contracts/schema/strict validators
                     |
          +----------+-----------+
          v                      v
Auto Attestation Owner       Human Service/Multisig
          +----------+-----------+
                     v
Verdict Owner S1/S2 + min TTL + CAS
                     v
Runtime read-only projection rejects legacy digest-only
                     v
trusted host composition/root + integration/system tests
```

无SCC import：Review只import Runtime public `core/ports`；Runtime public Port不import Review；trusted host root在最终composition层注入。

## 4. 文件级未来落点

| 阶段 | Review文件 | 产物/验收 |
|---|---|---|
| contract | `contract/auto_reviewer_v1.go`、`reviewer.go`、`verdict.go`、新exact-set helper | `Conditions,omitempty`、clone/sort/digest、legacy/strict分层 |
| schema | `contract/auto_output_schema_v1.go` | full Condition JSON schema、strict unknown/duplicate拒绝 |
| Auto owner | `autoattestation/owner.go`、memory/SQLite mutation校验 | Observation→Attestation exact无损；settlement链不变 |
| Human | `service`/`api`/`sdk`相关Review-owned文件、`multisigowner/owner.go`、`memory/multisig_v2.go`、`storage/sqlite/multisig_v2.go` | exact Conditions入参；canonical union；QuorumDecisionV2→HumanVerdictV2逐层exact；staged conflict零写；不创建VerdictV1 |
| Verdict | `verdictowner/owner.go`、`decisioncurrent`消费面 | REV-D12 S1/S2、scope、min TTL、zero CAS |
| Store | `memory`、`storage/sqlite` | clone/persist/exact Inspect；旧对象历史可读 |
| Adapter | `runtimeadapter` | legacy digest-only Conditional Fail Closed；不写Authorization |
| tests | `contract`、`tests`、`conformance` | CND-01..61 |
| assets | Review design/plan/module/memory/README | 双轴真值与真实命令同步 |

Runtime公共Port候选只记录于Review Port Delta；不得在上述Review路径创建兼容接口。

## 5. 分阶段执行

### C0 公共合同联合冻结

- Runtime public ports Owner裁决REV-D12 request/result/method；
- Policy Owner冻结exact Condition tuple resolve/exact Inspect/current-index与sealed immutable decision projection；
- Binding/Authority Owner确认现有Reader可按Condition subject S1/S2；
- 冻结closed errors、deep clone、ctx/clock/lost reply和min TTL。

验收：CND-21..35、49..60；公共签名、Category+Reason closed表与Owner conformance 0/P0/P1，宿主root仍可保持NO-GO。

### C1 Review contract与schema

- 为四个V1对象增加`Conditions,omitempty`；
- 抽取唯一canonical helper；
- 历史Validate与production strict validator分层；
- builtin Auto schema加入完整Condition shape。

验收：CND-01..14、36、40、43..45；旧无Condition literal golden digest不变。

### C2 Auto/Human producer

- Auto output exact set经Rubric shape与REV-D12 policy admissibility；
- Attestation无损复制exact set；
- Human Service/SDK/API只允许完整set；
- `multisigowner`对所有计入Conditional票执行唯一canonical union；memory/SQLite在一个事务stage QuorumDecisionV2、HumanVerdictV2与Trace，任何同ID字段冲突全零写；lost reply只Inspect原Quorum/HumanVerdict/exact Trace。
- 多签路径禁止创建`VerdictV1`；`runtimeadapter/reader_v5.go`随后只读复读Quorum/HumanVerdict exact Conditions/digest并投影到现有Runtime V5 current对象，不参与Review Store事务。

验收：CND-15..18、46..50、61；Auto/Human producer无digest-only降级，多签两层逐字段相同且无第二Review终态。

### C3 Verdict Owner与Store

- Decide前构造exact subject并调用REV-D12；
- fresh baseline/now、S1/S2、scope与min TTL；
- Verdict exact复制Conditions；
- memory/SQLite deep clone、history、lost reply exact Inspect。

验收：CND-19..20、27..35、40..42、51..54；并发只有一个CAS winner；detached retry归Reader/Verdict recovery门，不归producer门。

### C4 Runtime只读投影与集成

- Adapter对legacy digest-only fail closed；
- exact Conditional只有current Satisfaction时才输出相应basis；
- 宿主注入真实Policy/Binding/Authority Readers；
- 验证漂移后旧Permit不复活。

验收：CND-36..39、43；不得在Review创建Authorization Fact或Satisfaction。

## 6. 测试门

| 门 | 命令/证据 | 通过标准 |
|---|---|---|
| targeted ordinary | `go test -count=100 -run 'ConditionV2|ConditionalExact|ConditionAdmissibility' ./...` | 100次全绿 |
| targeted race | `go test -race -count=20 -run 'ConditionV2|ConditionalExact|ConditionAdmissibility' ./...` | 20次全绿无race |
| full | `go test ./...` | 全绿 |
| full race | `go test -race ./...` | 全绿无race |
| vet | `go vet ./...` | 零问题 |
| conformance | Runtime Owner reusable suite + Review consumer suite | Policy/Binding/Authority/TTL/ABA/lost reply覆盖 |
| mechanical | gofmt、`git diff --check`、asset links/stale/import scan | 全绿 |

只有实际运行后才能写PASS。当前计划不声称任何Go门已执行。

## 7. 迁移与回退

- schema加法使用`omitempty`，旧非Conditional JSON/digest保持可读；
- digest-only Conditional不删除、不覆盖、不批量升级，只能historical Inspect；
- 新写路径一律exact set；无法满足REV-D12时Fail Closed；
- 回退仅停用新producer/consumer capability，不删除历史对象、不恢复旧Permit；
- Runtime/Harness/Application/Model接口变化只由各Owner合入，Review不跨目录补洞。

## 8. 完成定义

Owner-local Go完成需同时满足：C1-C3代码、CND矩阵、target100/race20/full/race/vet与独立复审。

production完成还必须满足：C0公共Reader、各Owner真实conformance、C4宿主root与系统测试。两者必须分开报告；owner-local YES不能消除production NO-GO。
