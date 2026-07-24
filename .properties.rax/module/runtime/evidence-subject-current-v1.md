# Evidence Subject Current V1

状态：`implementation_software_test_yes`。

## 作用

该纵切把Evidence Ledger V2中的不可变Record转换为可历史读取、可验证current、且受Consumer/Scope/Policy约束的稳定Projection。它不创建第二份Evidence事实，也不把Projection升级为Domain Fact、Timeline、Settlement或Provider执行凭证。

## 实现组成

| 位置 | 职责 |
|---|---|
| `ExecutionRuntime/runtime/ports/evidence_subject_current_v1.go` | V1 DTO、closed enum、canonical/derive/seal/Validate、窄Reader与Owner Fact Port能力分离 |
| `ExecutionRuntime/runtime/control/evidence_subject_current_v1.go` | 从现有Owner事实派生typed ref，构造单向Projection→Index→Commit发布bundle并验证单调后继 |
| `ExecutionRuntime/runtime/kernel/evidence_subject_current_gateway_v1.go` | public current Gateway；bound Consumer association与七组Owner-current依赖S1/S2复读、自然TTL及clock regression门禁 |
| `ExecutionRuntime/runtime/fakes/evidence_ledger_v2.go` | 扩展既有Evidence Owner同锁reference store；原子发布historical Projection、Current Index与Mutation Commit |
| `ExecutionRuntime/runtime/conformance/evidence_subject_current_v1.go` | 仅依赖public Reader的historical/current/conformance验证，不取得Fact Owner写能力 |
| `ExecutionRuntime/runtime/tests/{ports,control,fakes}` | golden/canonical、能力收窄、lost reply、half-write、no-ABA、64并发与changed-content反例 |

## 关键语义

1. `ProjectionID`和`IndexID`只由稳定`SubjectKeyDigest`派生；首次revision为1，后继严格`+1`，旧Projection保持historical可读；
2. Projection先seal，Index只绑定完整Projection Ref，消除双向digest环；
3. raw `EvidenceSubjectCurrentFactPortV1`的方法集不满足public `EvidenceSubjectCurrentReaderV1`，消费者只能经Kernel Gateway读取current；
4. Owner publish在既有Evidence Owner锁内使Projection、Index、Commit全有或全无；同immutable内容幂等，同stable key换内容Conflict；
5. lost reply仅Inspect原Mutation/Projection/Index；合法progressed successor可恢复，revision rewind、gap、旁支与ABA拒绝；
6. Gateway在S1/S2复读bound Consumer、Record/Registration、Source Policy、Execution Scope、Producer Binding、Authority、Reader Binding/Capability与Presence/Readability，任一漂移Fail Closed；
7. current TTL取所有真实Owner上限的最小值，caller不能传自由TTL或重封Projection。

## 软件验证

Runtime Owner实现期与中央独立纯软件验收共同覆盖：

- target ordinary `count=100`：PASS；
- target race `count=20`：PASS；
- Runtime full ordinary与full race：PASS；
- `go vet ./...`、gofmt、import-boundary与diff-check：PASS；
- 64并发同内容单一逻辑发布、changed-content一胜、lost-reply、staged failure与no-ABA反例：PASS。

## 未解锁边界

当前只有Go公共合同、Kernel Gateway、reference in-memory Owner扩展、Conformance与软件测试。尚无production persistence backend、composition root、跨进程耐久性、availability、SLA或Continuity生产Adapter；因此不得将`implementation_software_test_yes`解释为production readiness。
