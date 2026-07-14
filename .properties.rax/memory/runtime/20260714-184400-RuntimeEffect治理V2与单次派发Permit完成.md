# Runtime Effect治理V2与单次派发Permit完成

## 事件

Runtime执行底座P0.2已在最终宿主门禁、执行点二次验真和正交收口完成后闭合：包含Effect Fact Journal、Budget Binding Fact、宿主Governance Dispatch Gateway、单attempt短TTL Dispatch Permit、Begin write-ahead、PermitVerifier/EnforcementReceipt、独立Inspect/Compensation/Cleanup及恢复规划。

## 已闭合事实

- Gateway在Issue与最终Begin两次读取BindingSet、Identity、Authority、Dispatch Policy、Review、Budget、全部Credential及CurrentScope/Run一致投影；Fence由Gateway根据当前投影构造并持久化，不接受调用方Fence。
- Dispatch Policy由版本化权威Fact提供，调用方不再直接提交可信Policy摘要。
- Owner只从当前BindingSet解析；自定义组件仍使用namespaced Kind、SchemaRef、ContentDigest和Opaque Payload接入。
- Permit必须`Issue→Gateway.Begin→执行点Enforcement CAS`；Begin成功后的崩溃或丢回包只能Inspect，不允许盲重派。裸Store Begin仅是Fact Owner原子原语。
- Permit TTL受全部治理事实与最短Credential Lease约束；Credential集合摘要进入Permit，Begin和执行点分别重读。
- P0.2将Enforcement Point确定绑定为Provider自身；Receipt精确绑定BindingSet revision、Manifest、Artifact、Capability、Permit、Attempt，未持久Enforcement不能进入`dispatched`。
- Provider回包保持Observation/Receipt；Settlement Owner通过封闭Disposition CAS形成权威Settlement Fact，领域结果保持受治理Opaque。
- unknown只能由独立settled Inspect收敛；补偿、Residual Inspect、Cleanup均为独立Effect并双向绑定原revision。
- Conflict/Idempotency使用tenant稳定域，跨Restore/新Instance继续阻塞；Settlement、Residual、Cleanup、Compensation正交推进，全部required维度闭合后才释放。
- `conditional` Review在P0.3 Condition Satisfaction Fact落地前失败关闭。

## 验证覆盖

定向测试覆盖全部当前事实的Begin后漂移、exact TTL、Credential revoke/epoch/TTL、Binding/Authority/Budget/CurrentScope/Run撤销或替换、并发双Begin、lost issue/begin/enforcement/provider/CAS reply、跨Restore稳定冲突域、同Permit ID内容冲突、Receipt精确绑定、无Enforcement绕过、Unknown Inspect与Compensation双向关系、正交Cleanup恢复，以及自定义执行点Verifier/Manifest/Artifact/Capability漂移。`go test -count=1 ./...`、`go test -count=1 -race ./...`、`go vet ./...`与`git diff --check`均通过。

CurrentScope静态夹具只验证合同，不代表生产一致快照后端、数据库、拓扑或SLA。生产Adapter必须提供同一State Plane一致读取或版本水位Conformance。

## 下一步

进入P0.3 Review Verdict V2；保留`DispatchReviewFactV2`为Gateway窄投影，不原地扩大旧`VerdictObservation`语义。
