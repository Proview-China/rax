# Run Lineage Association V1 测试矩阵

状态：**已吸收首轮独立审计`NO（P0=1/P1=2/P2=0）`并完成oracle返修；READY等待不同agent复审，不自标YES，未实现测试**。

## 1. 正向与结构

| ID | 层级 | 场景 | Oracle |
|---|---|---|---|
| `RLA-C01` | compile | Reader-only consumer | 不能调用create、V3 lifecycle mutation或Fact write |
| `RLA-C02` | canonical | literal golden | exact Run Ref（含Scope/Session）、Subject/ID/Fact/Index/Receipt/Projection固定JSON与digest逐字一致 |
| `RLA-C03` | create | exact parent + ordinary child pending | child bundle/EffectIndex/association rev1/highest/index/receipt同事务成功 |
| `RLA-C04` | idempotency | same canonical create replay | 返回同一receipt/deep clone；无第二history/index |
| `RLA-C05` | lifecycle | pending→running、Claim revision、stopping→terminal_cleanup→closed | 每个Run record revision或phase变化同事务严格+1；history全保留 |
| `RLA-C06` | current | Resolve full Subject | S1/S2相等后返回短TTL full projection |
| `RLA-C07` | exact | Inspect current full Ref | 仅该Ref仍current时成功 |
| `RLA-C08` | terminal Resolve | Subject bootstrap到closed child（含association NotAfter已过） | current index S1/S2定位terminal Fact；fresh短TTL truthful projection，Decision/Report exact，不新增ProjectionRef |
| `RLA-C09` | historical | current index已推进 | 旧exact Ref仍按history可读，不借current |
| `RLA-C10` | clone | 修改返回scope/pointer/slice | Store后续Inspect不受影响 |
| `RLA-C11` | terminal exact | current index缺失/损坏但full terminal Ref history完整 | `InspectTerminal`仍按exact history S1/S2成功；不读取current index |
| `RLA-C12` | constructor | valid named Store/Lifecycle/Clock/recovery policy | Reader构造成功；consumer仅得到只读Reader |

## 2. 二十三个最小硬反例

| ID | 反例 | 期望 |
|---|---|---|
| `RLA-N01` | parent与child跨Tenant | `Conflict/RunConflict`，零写 |
| `RLA-N02` | parent RunID等于child RunID | `InvalidArgument/InvalidReference`，backend调用0 |
| `RLA-N03` | parent identity digest与Owner current不符 | Conflict，child/association/receipt全无 |
| `RLA-N04` | child Run identity或SessionRef换包 | Conflict，零写 |
| `RLA-N05` | parent/child execution scope digest任一漂移 | Conflict，零写/零projection |
| `RLA-N06` | caller伪造AssociationID/SubjectDigest | public request无这些字段；malformed decode拒绝 |
| `RLA-N07` | same stable ID换Subject/body/digest | Conflict；旧history/current不变 |
| `RLA-N08` | child已存在但association rev1不存在 | Create Fail Closed，不补造association |
| `RLA-N09` | association存在但child bundle/EffectIndex缺失 | Inspect为Indeterminate/Conflict，不返回current |
| `RLA-N10` | staged create在history/highest/index/receipt任一点失败 | parent不变；child与五类sidecar全部零写 |
| `RLA-N11` | revision gap、rollback或same revision换body | Conflict；highest/current/history零改动 |
| `RLA-N12` | current index回指低revision但highest更高（ABA） | Conflict；不得返回低revisionprojection |
| `RLA-N13` | current index损坏/缺失但历史Ref存在 | Historical exact仍成功；Current Fail Closed |
| `RLA-N14` | child phase已变但association未同事务推进 | Current/Terminal Conflict，不动态伪造revision |
| `RLA-N15` | parent current revision/phase相对anchor回退 | `PreconditionFailed/InvalidState`，zero projection |
| `RLA-N16` | S1/S2之间association index换Ref | Conflict，zero projection |
| `RLA-N17` | S1/S2之间Run record、EffectIndex、Claim或terminal sidecar漂移 | Conflict，zero projection |
| `RLA-N18` | S1后或S2后跨越min TTL | `PreconditionFailed/CapabilityExpired`；不增revision |
| `RLA-N19` | Reader内部clock从T2回拨T1 | `Indeterminate/ClockRegression`，zero projection |
| `RLA-N20` | create回包丢失且Inspect不能证明child+rev1 association+receipt全闭包 | 返回原Indeterminate；create调用总数仍1 |
| `RLA-N21` | parent current与anchor same revision，但RecordDigest、Phase、SessionRef或full scope任一漂移 | `Conflict/RunConflict`，zero projection；必须full exact Ref相等 |
| `RLA-N22` | parent revision更高但沿用anchor RecordDigest/phase，或新phase不可达 | 重算失败为`Conflict/InvalidDigest|RunConflict`；zero projection |
| `RLA-N23` | ResolveTerminal遇active/terminal_cleanup，或InspectTerminal exact Ref不是terminal_closed | `PreconditionFailed/InvalidState`；zero terminal projection，不返回current替代 |

## 3. 错误、恢复与并发扩展

| ID | 场景 | Oracle |
|---|---|---|
| `RLA-E01` | Store/Lifecycle nil或typed-nil、Clock nil | constructor `InvalidArgument/ComponentMissing`，backend调用0 |
| `RLA-E02` | ctx cancel/deadline | Indeterminate；Resolve最多一次同Subject新S1，exact Inspect最多一次同full Ref bounded recovery，mutation不retry |
| `RLA-E03` | backend Unavailable/Indeterminate | 原类别保留，不降NotFound |
| `RLA-E04` | exact ID从未存在 | 只有Owner线性化证明时NotFound |
| `RLA-E05` | Terminal Reader遇active/terminal_cleanup | PreconditionFailed，zero terminal projection |
| `RLA-E06` | pure time expiry | active/current ValidateCurrent失败，history/index revision不变；exact terminal truth仍可fresh Inspect |
| `RLA-E07` | recovery timeout为0、负数或大于2s | constructor `InvalidArgument/InvalidReference`，Store/Lifecycle调用0 |
| `RLA-E08` | configured timeout超过caller deadline/request TTL/current association TTL，或裁剪后<=0 | 使用真实最短remaining；<=0不retry并保留原error |
| `RLA-E09` | ResolveTerminal lost reply | 至多一次同Subject新S1，不声称恢复旧snapshot；仍验index ABA/TTL/rollback |
| `RLA-E10` | InspectTerminal lost reply | 至多一次同Subject+full terminal Ref读history；不Resolve、不读current index |
| `RLA-E11` | terminal S1后/S2后clock rollback或TTL crossing | `Indeterminate/ClockRegression`或`PreconditionFailed/CapabilityExpired`，zero projection |
| `RLA-R01` | 64 same-subject并发create | 一个canonical commit；其余same receipt或Conflict；无partial |
| `RLA-R02` | 64 same expected revision并发Run-record/phase publish | 一个N+1 winner；其余Inspect winner；无gap/ABA |
| `RLA-R03` | 跨Tenant相同local RunID并发 | 两个分区独立，无串读/冲突 |
| `RLA-R04` | race读写current | 无data race；每个返回都是单一线性化S1/S2快照 |

## 4. 未来门禁

```text
go test -count=100 -run 'RunLineage|ChildRun' ./...
go test -race -count=20 -run 'RunLineage|ChildRun' ./...
go test ./...
go test -race ./...
go vet ./...
gofmt -d <candidate files>
git diff --check -- <Runtime owned paths>
import-boundary scan
```

还必须用public-only reusable Conformance分别验证memory reference store、SQLite/production store与malicious backend；任何fixture只证明合同，不证明production root、durability或SLA。
