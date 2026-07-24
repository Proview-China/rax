# Operation Settlement Current Reader V5设计候选

状态：**第二次独立代码短审YES（P0/P1/P2=0/0/0），Runtime V5窄Reader纵切完成**。

## 1. 目标

为Sandbox Checkpoint等只需验证Runtime Operation Settlement V5 current终态的消费者提供最小只读能力，避免其持有包含`SettleCheckpointPhaseV5`的治理写面。

本候选只做接口能力收窄：复用现有V5 request、inspection、Gateway、Fact Owner、四对象闭包、shared terminal guard、canonical和digest。它不创建第二Store，不复制Sandbox事实，不改变V3/V4/V5对象或终态语义，也不授Provider、ApplySettlement、Checkpoint Consistency或Restore能力。

## 2. Owner与边界

- Runtime Settlement Owner继续唯一拥有V5 Commit和current terminal projection；
- Sandbox只持窄Reader，读取Runtime既有current inspection，再验证其自己的DomainResult/ApplySettlement因果链；
- Reader不返回Settle、Commit或任何Fact Owner写能力；
- historical Settlement、Association、Guard与Projection的公开读取仍留在现有Governance Port；本Delta不重组这些方法；
- current Inspect存在不等于Effect可再次执行，也不替代Evidence、DomainResult或Sandbox ApplySettlement。

## 3. 设计入口

- [Port Delta](./port-delta.md)
- [测试矩阵](./test-matrix.md)
- [实施计划候选](../../../plan/runtime/operation-settlement-current-reader-v5.md)

## 4. 当前裁决

Live V5治理接口已包含目标Inspect方法，Gateway、reference store和Conformance均已有真实实现；但存在两个P0：只读消费者必须依赖含Settle写能力的大接口，以及Gateway当前只验证返回Inspection自身有效，尚未将返回Bundle的Operation/EffectID与请求exact交叉。后续必须同时抽取additive Reader并补Gateway请求—回值exact门；仍不需要新对象、Reader adapter、Store或digest。

Runtime Owner资产已把上述live缺口冻结为获授权后的实施义务，而非未决设计：

- `P0-I1`：抽取窄Reader，Sandbox等消费者不得持有Settle或Fact Port；
- `P0-I2`：Gateway必须对request Operation/Effect与returned Bundle执行exact交叉，错误backend回值返回零值+Conflict；
- `P1-I1`：reader-only Conformance、method-set、typed-nil、跨Tenant零泄露与malicious backend反例。

联合设计Review已裁决YES。首轮独立代码短审随后发现：raw Fact Port在Go结构上可满足单方法Reader、malformed Inspection与request drift的验证顺序未冻结、同Operation ID的Tenant/Scope/nested ref漂移及Unavailable/Indeterminate透传反例不足。Runtime已完成以下返修：

- 新增Gateway-backed provider marker与Kernel facade constructor，composition只接受该provider，防止误把raw Fact Port装入consumer；该marker只防误装配，不宣称语言级阻止蓄意伪装；
- public Conformance改为只接收Gateway-backed provider；
- Gateway先验证完整Inspection，再执行request Operation/Effect exact交叉；malformed返回InvalidArgument，合法结构的漂移返回Conflict，均为零Inspection；
- 同ID不同Tenant/Scope/nested ref、Unavailable/Indeterminate原样透传、完整DeepEqual零值及Settle/Provider/Commit/Apply计数为0均有反例。

返修已通过Owner门禁与第二次独立代码短审，P0/P1/P2=0/0/0。本裁决只确认Runtime窄Reader纵切完成，不授production backend/root/durability/SLA。

本实现不授权Sandbox直接持有raw Fact Port；composition只能向消费者注入Kernel Gateway构造的`OperationSettlementCurrentReaderProviderV5`窄能力，再以其内嵌Reader方法读取。
