# Runtime Effect治理V2

## 定位

Effect治理V2负责外部动作的线性化事实、最终派发门禁、未知结果恢复和治理维度收口。Runtime只解释身份、版本、摘要、权限、Fence、预算、审核、Owner、冲突域与状态迁移，不解释自定义组件的领域Payload，也不把Provider回包提升为领域Commit。

## 权威时序

1. Effect Owner提交`EffectIntentV2`，事实进入`proposed`。
2. 当前冲突域空闲时CAS进入`accepted`；`unknown_outcome`持续占用冲突域。
3. 宿主`GovernanceDispatchGatewayV2`重新读取BindingSet、Identity Lease、Authority、Dispatch Policy、Review、Budget、全部Credential Lease，以及由Activation/Instance/Sandbox/Authority/Binding/Run来源事实形成的`ExecutionScopeCurrentFactV2`一致只读投影。投影携带来源revision/digest与水位，不是第二个Instance、Fence或Run事实Owner。
4. Gateway原子写入`dispatch_intent`和单Effect revision、单BindingSet revision、单Provider、单attempt的短TTL `DispatchPermitFactV2(issued)`。
5. Gateway根据当前投影构造并持久化Effect专属`ExecutionFence`；调用方不能提供或替换Fence。Permit TTL取Intent及所有治理事实、CurrentScope和每个Credential Lease的最早到期时间。
6. Application只能调用`GovernanceDispatchGatewayV2.Begin`作为最终宿主门禁。它在Issue后再次读取全部当前事实并原子调用Fact Owner的`BeginDispatch`；裸Store Begin只是内部线性化原语，不是公开治理入口。成功即写成`begun`，从这一刻起只能Inspect，禁止盲重派。
7. 执行点再次读取CurrentScope、BindingSet capability grant和Credential集合，返回`EnforcementReceiptV2`并CAS写回已Begin Permit。P0.2保守限定Enforcement Point为Permit绑定的Provider自身，Receipt必须精确绑定其BindingSet revision、Component、Manifest、Artifact和Capability；未来独立Verifier委派需新增版本合同。Receipt只证明二次验真，不证明Provider或领域操作成功。
8. Provider只返回精确绑定Permit/Attempt/Intent/Provider且不早于Enforcement的Observation/Receipt。Settlement Owner形成封闭通用Disposition的`EffectSettlementFactV2`并CAS；领域结果只能放受治理SchemaRef+Digest+Opaque/Evidence引用，Runtime不解释。
9. `unknown_outcome`只能由双向绑定原Effect revision的独立、已settled Inspect Effect收敛。Compensation、Remote Residual Inspect和Cleanup同样必须引用各自独立settled Effect。
8. Compensation、远程Inspect、Cleanup、Cancel或Release只要触达外部世界，均是新的独立Effect。原Effect只能引用已独立settled的相关Effect后关闭对应维度。

## 安全不变量

- `issued→begun`最多一次线性化；Permit不是可无限重放的Bearer字符串。
- Gateway Begin必须先于任何可能触达Provider的动作；执行点Enforcement Receipt必须先于`dispatched`。
- Begin前拒绝是安全的`rejected`；Begin后失败是`unknown_outcome`，Provider自报“未尝试”不能恢复自动重试权。
- Permit过期/撤销通过CAS线性化；恢复规划使用注入Clock，未Begin且过期可确定性拒绝，已Begin只能Inspect。
- Owner由当前BindingSet解析；调用方不能自选Effect、Settlement或Cleanup Owner。
- Review与Budget的`operation_not_required`必须引用显式Policy事实；零值不代表“不需要”。
- `conditional` Review在P0.3提供Condition Satisfaction Fact前一律失败关闭。
- Dispatch Policy本身是版本化权威Fact，并精确绑定Effect候选摘要、Scope、Kind、Risk与最大Permit TTL；请求方不能自选策略摘要。
- Budget算法属于Budget Owner；Runtime只验证reservation/consumption/release或显式not-required权威事实。
- Observation、Enforcement Receipt、Provider Receipt和Settlement Observation均非领域权威结果。
- Conflict与Idempotency采用tenant稳定域，不含Run/Instance epoch；Restore或新Instance不能绕过旧`unknown_outcome`。更窄作用域必须由未来显式Policy合同证明。
- Settlement、Remote Residual、Cleanup与Compensation为正交CAS维度；主Effect settled/compensated不等于Cleanup完成，只有全部required维度闭合才释放Conflict Domain。
- 时间来自注入Clock；TTL边界时刻即过期，时钟回拨失败关闭。

## 公共接入面

- `runtime/ports`: `EffectIntentV2`、`DispatchPermitV2`、`ExecutionScopeCurrentFactV2`、`DispatchCurrentFactsV2`、`PermitVerifierPortV2`及Authority/Policy/Review/Credential只读事实Port。
- `runtime/control`: `EffectFactPortV2`、`BudgetFactPortV2`、Effect/Permit/Budget状态机、`GovernanceDispatchGatewayV2`与`PlanEffectRecoveryV2`。
- `runtime/conformance`: `CheckPermitVerifierV2`。通过只说明验真Receipt合规，不授予生产资格、派发结果权威性或领域Commit资格。
- `runtime/fakes`: 原子内存事实Owner和故障注入，仅用于测试，不能宣称生产Conformance。

生产`ExecutionScopeFactReaderV2`必须从同一State Plane提供一致读取或明确版本水位，并通过Conformance证明来源revision/digest不会混读。当前静态测试投影不代表生产原子快照后端、进程拓扑或SLA。

## 保留边界

- 旧`core.EffectIntent`、旧Effect Port与Observation seam保持原语义，不原地升级为权威Fact。
- P0.3将提供完整Review Verdict V2状态机；本阶段的`DispatchReviewFactV2`只是Gateway读取的窄权威投影。
- 不选择签名算法、RPC、生产数据库、进程拓扑、Scheduler或SLA。
