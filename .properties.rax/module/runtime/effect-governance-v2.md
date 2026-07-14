# Runtime Effect治理V2模块说明

## 作用

该模块把“准备调用外部Provider”拆成可恢复、可审计且不会重复派发的事实链：Effect Journal、短TTL单attempt Permit、宿主最终Gateway、执行点二次验真、Settlement与独立Inspect/Compensation/Cleanup。

## 组成

- 公共Envelope：`ports/effect_v2.go`
- 当前治理事实读取面：`ports/governance_v2.go`
- Effect与Permit Journal合同：`control/effect_fact_v2.go`
- Budget绑定事实：`control/budget_fact_v2.go`
- 宿主派发门禁与恢复规划：`control/governance_gateway_v2.go`
- 测试事实Owner：`fakes/effect_store_v2.go`
- 自定义执行点Conformance：`conformance/effect_v2.go`

## 可预期行为

- Permit Issue丢回包：Inspect Effect与Permit Fact，不重复签发另一个attempt。
- Gateway Begin在最终时刻重读Binding、Identity、Authority、Policy、Review、Budget、Credential与CurrentScope/Run投影；任一revision/state/TTL/digest漂移均不消费Permit。
- Begin丢回包：Inspect到`begun`后创建独立Inspect Effect，不重派原Effect；裸Store Begin不对Application开放。
- Enforcement写回丢回包：先Inspect已Begin Permit，确认精确绑定的Receipt已持久，才允许接收Provider Receipt。
- Provider回包后的Effect CAS丢回包：Inspect Effect Fact决定是否已进入`dispatched`。
- 同一Permit并发Begin：只一次成功。
- 同Permit/Attempt/Effect ID但TTL、Fence或摘要不同：`IdempotencyPayloadMismatch`，不能静默复用。
- 同一Settlement重复写：幂等；不同结果：Conflict并保留为后续Evidence输入。
- 过期、旧revision、旧epoch、Binding/Review/Budget/Policy/Credential/CurrentScope/Run/Fence漂移：机器可判定`DomainError`并失败关闭。
- 旧Instance的unknown仍占tenant稳定冲突域；Restore、新Instance或新Run不能绕过。
- Settlement可先完成，Residual/Cleanup可随后独立CAS闭合；Compensated后仍可完成Cleanup。

## 当前实现边界

- CurrentScope是Activation/Instance/Sandbox/Authority/Binding/Run权威事实的一致只读投影，不是第二主库；静态fake不宣称生产一致快照能力。
- P0.2只允许绑定Provider作为Enforcement Point；Verifier Receipt必须精确匹配Provider Binding，不能用自由字符串冒充。独立Verifier委派留给新版本。
- P0.3已经引入Condition Satisfaction Fact；conditional只有精确Verdict与Satisfaction Ref/Digest/Revision在Issue、Begin和执行点均current时才具备派发资格。
- Settlement的Runtime语义使用封闭Disposition；自定义领域结果只通过受治理Opaque/Evidence引用承载。

## 非职责

模块不运行Provider业务、不计算Budget、不做Review判断、不解释Opaque Payload、不宣称Task/Goal/Artifact成功，也不自动把Harness Observation提升为Runtime Outcome。
