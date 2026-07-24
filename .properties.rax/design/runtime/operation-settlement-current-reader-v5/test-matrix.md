# Operation Settlement Current Reader V5测试矩阵

状态：**全矩阵通过Owner门禁与第二次独立代码短审YES（P0/P1/P2=0/0/0）**。

| ID | 类别 | 断言 |
|---|---|---|
| `OSCR5-C01` | compile | 既有Governance实现可赋值给`OperationSettlementCurrentReaderV5` |
| `OSCR5-C02` | capability | reader-only实现可满足Reader，但不能满足Governance或Fact Port |
| `OSCR5-C03` | method set | Reader恰有一个方法，签名与live current Inspect完全相同 |
| `OSCR5-C04` | compatibility | Governance仍保留原六方法；V5对象、JSON tags、canonical与digest冻结hash不变 |
| `OSCR5-C05` | wiring provider | Kernel Gateway facade可满足provider；raw Fact Port与普通单方法Reader均不能满足provider marker |
| `OSCR5-R01` | positive | exact Operation+Effect返回完整current Inspection并通过`Validate()` |
| `OSCR5-R02` | exact | Operation、Effect、Attempt、Phase、DomainResult、Association、Guard、Projection任一漂移Fail Closed |
| `OSCR5-R02A` | malicious backend | Fact Port返回结构有效但属于另一Operation/Effect的Inspection时Gateway返回Conflict，不泄露closure |
| `OSCR5-R02B` | order | malformed Inspection先因Validate失败；结构有效的request drift才返回Conflict，两者均返回零Inspection |
| `OSCR5-R02C` | operation value | 同ID但Tenant/Scope/operation kind或nested ref不同，`SameOperationSubjectV3`值语义拒绝 |
| `OSCR5-R03` | absence | exact current ID不存在只返回NotFound，不触发Settle或重建 |
| `OSCR5-R04` | errors | Unavailable/Indeterminate不转NotFound；Provider/Commit/Apply调用数均为0 |
| `OSCR5-R05` | typed nil | nil/typed-nil Reader在backend前`ComponentMissing` |
| `OSCR5-R06` | tenant | 跨Tenant相同EffectID不串读；wrong tenant不泄露他租户closure |
| `OSCR5-R07` | backend spy | invalid request或typed-nil依赖在Fact read前失败；malicious return后Settle/Commit/Provider/Apply调用均为0 |
| `OSCR5-R08` | zero result | malformed、drift、Unavailable、Indeterminate均返回与零Inspection完整`DeepEqual`的结果 |
| `OSCR5-B01` | import | Sandbox adapter只需`runtime/core`与`runtime/ports`，不能取得Fact Port或Settle方法 |
| `OSCR5-B02` | authority | current Inspection不授Provider、Evidence、DomainResult、ApplySettlement、Consistency或Restore能力 |

已执行target ordinary `count=100`、target race `count=20`、Runtime full ordinary/race、vet、gofmt、diff-check、public method-set/provider reflection与import-boundary扫描，结果均PASS。第二次独立代码短审YES（P0/P1/P2=0/0/0）。
