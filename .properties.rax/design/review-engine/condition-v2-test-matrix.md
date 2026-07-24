# Review Condition V2 测试与反例矩阵

本矩阵是未来实现oracle，不表示当前已存在Go测试或production root。

| ID | 层级 | 场景 | Oracle |
|---|---|---|---|
| CND-01 | contract | Conditional携带1个完整`ReviewConditionV2` | seal成功，exact set与digest一致 |
| CND-02 | contract | 64个不同ID且严格排序 | seal成功；65个拒绝 |
| CND-03 | contract | 未排序Conditions | `InvalidArgument/InvalidCanonicalForm`，零写 |
| CND-04 | contract | 同ID同revision重复 | duplicate/invalid canonical，零写 |
| CND-05 | contract | 同ID不同revision双项 | duplicate ID，不能解释为两个Condition |
| CND-06 | contract | Conditions任一字段改动后沿用旧digest | Conflict，zero Attestation/Verdict |
| CND-07 | contract | Conditional只有digest没有exact set | historical read可用；production strict拒绝 |
| CND-08 | contract | Draft、sealed Output/Attestation/Verdict分别遇到空/错误digest | Draft由host Seal唯一生成；sealed对象空值只可由其Owner Seal生成，错误值拒绝；持久对象必须非空exact |
| CND-09 | contract | 非Conditional携带Conditions或digest | Conflict，零写 |
| CND-10 | contract | Condition坏Schema/Constraint/Binding/Authority/TTL | InvalidArgument，零写 |
| CND-11 | contract | `ScopeDigest != Target.ActionScopeDigest` | PreconditionFailed，零写 |
| CND-12 | contract | Condition TTL超过Target/Attestation最短上界 | PreconditionFailed，零写 |
| CND-13 | auto schema | exact Condition JSON完整且无未知字段 | strict decode成功，host重新seal |
| CND-14 | auto schema | 顶层/嵌套重复key、未知字段、自由文本condition | strict decode拒绝，零Observation升级 |
| CND-15 | auto owner | output exact set与ApplySettlement/Result exact链一致 | Attestation无损携带set与digest |
| CND-16 | auto owner | output Condition在settlement后被换字段并重digest | exact Observation/Result漂移，零Attestation |
| CND-17 | human service | Human提交exact set | Attestation逐字段保存；平台评论本身不是Condition |
| CND-18 | multisig | Accept空集+多个Conditional票包含不同ID | 只对Conditional票做按ID union、排序、重算digest，QuorumDecisionV2与HumanVerdictV2逐字段相同 |
| CND-19 | verdict | Attestation与Verdict Condition set完全相同 | CAS成功，Verdict expiry纳Condition TTL |
| CND-20 | verdict | Verdict删项/加项/改序/改字段后重digest | Conflict，zero Verdict/CAS |
| CND-21 | policy | current Policy允许exact Schema/Owner/Capability/Authority tuple | Resolve full Ref + exact Inspect success并返回sealed Policy decision projection |
| CND-22 | policy | Policy不允许Schema/Owner/Capability/Authority任一项 | `Forbidden`，zero Verdict/CAS |
| CND-23 | binding | SatisfactionOwner Binding active/current且exact Subject匹配 | S1/S2成功，纳true min TTL |
| CND-24 | binding | Binding在S1/S2间revoke/supersede/renew到新ref | Conflict/PreconditionFailed，不追随新ref |
| CND-25 | authority | Condition Authority exact current且Run/Scope/ActionScope匹配 | S1/S2成功，纳TTL |
| CND-26 | authority | revoked/epoch drift/cross-run/cross-scope | PreconditionFailed/Conflict，zero Verdict |
| CND-27 | current | Policy在S1/S2间ABA回到同ID旧payload | full ref/digest/index检测Conflict |
| CND-28 | current | 单个Condition是唯一最短TTL | aggregate expiry精确等于该Condition expiry |
| CND-29 | current | Policy/每个Binding/每个Authority分别为唯一最短TTL | table-driven逐项证明min TTL |
| CND-30 | current | Inspect期间TTL crossing | fresh actual-point validation失败，zero CAS |
| CND-31 | current | reader内部clock从T2回拨T1但仍晚于facts | ClockRegression，zero Verdict/CAS |
| CND-32 | error | ctx canceled/deadline/unknown | 保留Indeterminate，不降级NotFound |
| CND-33 | error | 已知Owner backend outage | Unavailable，不降级Policy deny |
| CND-34 | recovery | 首次只读reply-loss，detached同canonical fresh retry成功 | 接受新的完整snapshot；不声称恢复旧结果 |
| CND-35 | recovery | retry发生drift/TTL crossing/仍unknown | Fail Closed；无Provider/Decide重复调用 |
| CND-36 | history | legacy digest-only exact Inspect | 历史可读；production adapter零Authorization |
| CND-37 | runtime | exact Conditional但无current Satisfaction | defer/deny；不进入Permit |
| CND-38 | runtime | Satisfaction owner自报但无Owner Inspect/CAS | 不升级为Satisfaction/Authorization |
| CND-39 | runtime | Satisfaction满足后Condition/Policy/Authority过期 | 旧Permit不复活，需重新Review |
| CND-40 | clone | 修改Reader返回的Conditions/Items/Scope lease | 不污染Owner state或后续Inspect |
| CND-41 | concurrency | 64个Decide共享同Case，Condition current稳定 | 仅1个Verdict CAS winner |
| CND-42 | concurrency | 64个Decide且任一current在中途漂移 | 最多1个合法winner；漂移后全部zero write |
| CND-43 | import | Review production包扫描 | 只依赖Review公开包与Runtime public core/ports，无Runtime control实现import |
| CND-44 | ownership | Review尝试Create/CAS Satisfaction | compile-shape无该write Port；Runtime唯一拥有Satisfaction，领域Owner只写source proof fact |
| CND-45 | compatibility | 四个V1对象nil/empty Conditions的canonical JSON与旧digest literal golden | `omitempty`均不出现字段；旧digest逐字不变 |
| CND-46 | multisig | 两个Conditional票同ID八字段exact相同 | 去重为一项；union deterministic |
| CND-47 | multisig | 两票同ID但revision/Schema/Constraint/Owner/Scope/Authority/TTL任一漂移 | Conflict；QuorumDecisionV2/HumanVerdictV2/Trace全零写；零Runtime V5 current projection |
| CND-48 | multisig | Conditional票空set/坏digest/过期；或Quorum/HumanVerdict集合漂移 | Fail Closed，两层不得降级digest-only或Accept；零Runtime current projection |
| CND-61 | ownership | 多签路径尝试创建`VerdictV1`或synthetic panel/group Reviewer | compile/owner边界拒绝；唯一Review终态为HumanVerdictV2，Runtime V5仅只读投影 |
| CND-49 | subject | cross-tenant、cross-Case、cross-Round、cross-Assignment、cross-Target、Run漂移逐项 | aggregate与Policy decision均Conflict，zero Verdict/CAS |
| CND-50 | policy | Policy decision Ref不变但Subject/Allowed/Checked/Expires/Digest任一漂移 | Conflict/PreconditionFailed，zero Verdict |
| CND-51 | policy | Policy current indexABA或同ID同revision换payload | exact Ref+index+digest检测Conflict；不得追新ref |
| CND-52 | ttl | 任一Item expiry伪大/伪小，或aggregate未取全部Item min | Validate失败，zero Verdict/CAS |
| CND-53 | recovery | detached retry没有timeout、超过2s/Subject TTL、二次retry | Fail Closed；调用次数最多2次且无goroutine泄漏 |
| CND-54 | history | 时间自然越过Expires | exact historical projection不增revision/不重封；ValidateCurrent失败 |
| CND-55 | policy publisher | stable ID首建 | revision=1、history/highest/current full Ref同事务全有全无 |
| CND-56 | policy publisher | 64并发发布同Previous与不同next payload | 仅一个CAS winner；loser零history/highest/current泄漏 |
| CND-57 | policy publisher | initial revision非1或续版非严格+1 | Conflict；三索引零写 |
| CND-58 | policy publisher | staged digest/policy/current-index failure | history/highest/current全零写；旧exact仍可读 |
| CND-59 | policy publisher | publish成功但reply丢失 | 只Inspect exact proposed Ref+Subject；同canonical已存在即恢复，不重复publish新revision |
| CND-60 | policy reader | Resolve/InspectCurrent/Historical逐项触发closed error表 | Category+Reason逐字相同；NotFound不吞terminal/deny/unknown |

## 未来门禁命令

```text
go test -count=100 -run 'ConditionV2|ConditionalExact|ConditionAdmissibility' ./...
go test -race -count=20 -run 'ConditionV2|ConditionalExact|ConditionAdmissibility' ./...
go test ./...
go test -race ./...
go vet ./...
gofmt / diff-check / import scan
```

公共Owner还必须提供可复用conformance：Policy publish/allow/deny/current-index/history、Binding S1/S2、Authority actual-point、min TTL、ABA、lost reply、clock rollback。Review测试不能用私有fake伪造这些production证据。
