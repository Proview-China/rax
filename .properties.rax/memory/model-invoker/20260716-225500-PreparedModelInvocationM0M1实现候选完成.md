# PreparedModelInvocation M0/M1实现候选完成

## 时间

2026-07-16 22:55:00 +0800

## 粗粒度事件

Runtime neutral Registry/Assembly Go合同在两条独立审计均为YES后，Model Invoker按授权完成`PreparedModelInvocation / PreDispatch Gate V1`的M0/M1实现候选。本轮只修改`ExecutionRuntime/model-invoker/**`和Model自有plan/module/memory资产，没有修改Runtime、Harness、Tool、Application、Context或production root，没有stage或commit。

M0落地了Historical Fact/Ref、Current Projection/Ref、ACK/AckRef、五字段`PreparedModelInvocationSurfaceBindingRefV1`、非权威Dispatch Validation Receipt、strict canonical codec和公共短Gate。Prepared Stable ID的canonical body固定覆盖`ContractVersion + InvocationID + InvocationDigest`；ACK与Receipt分别强制完整Prepared/Current/Ack因果链和`Checked < Expires <= NotAfter`时间上界。Historical/Current直接携带`runtimeports.RegistrySnapshotRefV1`，PurePrepare通过`runtimeports.RegistrySnapshotExactReaderV1`按完整Ref exact读取并pin；Model没有定义Runtime alias、wrapper或第二Registry nominal。

Gate的UnknownOutcome恢复已收紧：`Commit`只调用一次。只有Indeterminate错误同时返回完整、可验证、与输入exact的ACK时，才按其稳定Ack Ref调用一次`InspectExactAck`；没有稳定Ref、Inspect不可用或返回漂移时全部fail closed，绝不盲目第二次Commit。

M1落地了公共Historical/Current Exact Reader和原子Repository接口，以及线程安全内存reference Store。Store以Historical ID+Invocation坐标、Current ID+完整Prepared Ref做双索引create-once；相同canonical内容幂等，同key换内容Conflict，exact读取重算并返回clone。Repository原子Ensure的lost reply只允许对同一个sealed canonical输入最多恢复一次；typed-nil、AuthoritativeAbsent、UnknownAbsent、RetentionUnreadable、Unavailable和Indeterminate保持名义分类。内存Store只是Fake/Conformance，不是生产持久化driver、composition root、retention service或SLA。

## 验证

- `go test ./tests/preparedinvocation -count=100`：通过；
- `go test -race ./tests/preparedinvocation -count=20`：通过；
- `go test ./...`：通过；
- `go test -race ./...`：通过；
- `go vet ./...`：通过；
- `gofmt`：目标Go文件与测试无差异；
- import/type identity conformance：通过，Model只依赖Runtime public`core/ports`，Registry字段静态类型exact为`runtimeports.RegistrySnapshotRefV1`。

## 当前边界

- 当前状态是M0/M1 reference implementation候选，等待独立代码审计；
- 没有实现Harness Gate/ACK Repository、Tool Surface Binding Reader或Tool Ack；
- 没有把Gate接入root Invoker、RouteGateway、Direct、Harness Adapter、retry、continuation、Realtime或hosted tool路径；
- 没有宣称provider-before-ACK no-bypass、production persistence、production root或SLA已经存在；
- M2-M5只有在后续明确授权和相邻Owner合同就绪后才能实施。

## 关联资产

- [实施计划](../../plan/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1.md)
- [主设计](../../design/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1.md)
- [测试矩阵](../../design/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1-test-matrix.md)
- [模块入口](../../module/model-invoker/README.md)
