# Runtime Review Verdict V2模块说明

## 作用

该模块把“Reviewer说可以”拆成可验证事实链，使Observation不能直接授权外部Effect，并保证Review在Permit Issue、最终Begin和实际执行点三次复读。

## 代码组成

- 公共类型与Port：`ports/review_v2.go`
- Review只读派发投影：`ports/governance_v2.go`
- Case/Verdict/Satisfaction状态机：`control/review_fact_v2.go`
- 投影构造：`control/review_projection_v2.go`
- 唯一Application入口：`control/review_governance_v2.go`
- 测试Fact Owner：`fakes/review_store_v2.go`
- 自定义Reviewer Conformance：`conformance/review_v2.go`
- 执行点复读兼容Delta：`conformance/effect_v2.go`

## 可预期行为

- 必须先持久Effect，再用Effect ID与Expected Fact Revision创建Case；调用方不能提交未持久Intent冒充审核对象；
- Case/Decide丢回包后Inspect同一Fact，不重复创建或改判；
- 并发相反Verdict只线性化一个，冲突决定保留为Evidence输入；
- accepted旧Verdict不能批准新Effect revision或漂移后的payload/policy/scope/binding/run；
- conditional只有全部exact Satisfaction Proof闭合才可派发；
- 自动Reviewer的网络调用走独立Effect，unknown不得重复调用；
- Verdict或Satisfaction在Issue后撤销，Begin失败且Permit不消费；
- 同一Verdict不替换Satisfaction；证明失效后必须新建Effect revision与整条Review/Permit链，旧Permit不可复活；
- Begin后、Provider触达前任一Review/Budget/Authority/Policy漂移，执行点Verifier拒绝且Provider零调用。

## 非职责与限制

Review只判定，不dispatch、不commit。Runtime不解释Reviewer业务逻辑、Condition正文或领域结果。当前Evidence仍为稳定引用，等待P0.4 Ledger Adapter；当前无生产Review后端、签名算法、RPC、离线策略或SLA。
