# Tool/MCP Component Release V1

## 1. 目标与真值

`ExecutionRuntime/tool-mcp/release`把Tool与MCP现有owner-local能力发布为Agent Assembler可直接消费的`ComponentReleaseV1`。发布只描述构造合同，不拥有Host root、Provider、Credential、Runtime事实或生产认证。

当前live事实分三档：

| 模式 | 必需证明 | 当前结论 |
|---|---|---|
| `reference_only` | 发布请求、完整Manifest/Module/Capability/Port/Factory descriptor | 可发布 |
| `standalone` | 再有G6A P4、Surface Current/Binding、Input Contract、Controlled Provider Adapter、MCP Discovery/Lifecycle/official SDK Call八项exact current | owner-local软件可发布 |
| `production` | 再有四类durable store、Credential current、Provider transport/current、controlled actual-point、Evidence/Settlement、MCP lifecycle/Inspect、Cleanup、Deployment Attestation和独立Certification Fact | 当前NO-GO |

## 2. 公共合同

- `LocalReadinessProjectionV1`：八项owner-local exact Ref，全部非空、互不alias，摘要与TTL封存。
- `ProductionReadinessProjectionV1`：十五项生产exact Ref；任何缺项、漂移、过期或Reader不可用都Fail Closed，不能降级发布。
- `PublicationRequestV1`：一个不可变Release revision；相同ID/revision换内容必须Conflict。
- `PublisherV1`：S1读取readiness，构造release，S2复读同一projection，再`EnsureExact`写Catalog；丢回包只`InspectExact`。
- `ConformanceCandidateV1`：仅作为审核候选，不是Certification Fact。

Factory只描述构造输入、输出Capability、生命周期和cleanup contract；不携带Go函数、Provider句柄、Secret或生产root。

## 3. Manifest闭包

一个组件提供两个独立Capability与两个effectful Port：

1. `praxis.tool/single-call-action-v2`：N=1、start-or-inspect、Runtime actual-point治理；
2. `praxis.mcp/governed-tools-call-v1`：exact MCP command与official SDK `tools/call`。

两者都声明Effect/Settlement/Cleanup Owner、Credential requirement、inspect-only unknown语义；MCP lifecycle成功不替代Runtime Settlement，Provider receipt也不替代Tool DomainResult。

## 4. 生产硬门

以下任一未闭合时不得发布`production`：durable Action/Binding/Surface/MCP store；Credential Broker current；真实Provider transport和Provider current；controlled actual-point；Evidence/Settlement current；MCP lifecycle与Inspect；cleanup owner；deployment attestation；独立release certification。

当前内存Repository、测试Provider、official SDK in-memory Session和test composition均不满足上述门。

## 5. 验收

- 黑盒：Assembler Repository能按exact Ref读取公开候选；
- 白盒：三种模式不互相升级，Manifest/Capability/Port/Factory闭包完整；
- 故障：lost reply、S1/S2 drift、TTL crossing、clock rollback、Unavailable、typed-nil、同ID换内容；
- 并发：64个Publisher共享Catalog仅一次线性化；
- 工程：ordinary 100、race 20、full ordinary/race/vet、gofmt、import/diff。

