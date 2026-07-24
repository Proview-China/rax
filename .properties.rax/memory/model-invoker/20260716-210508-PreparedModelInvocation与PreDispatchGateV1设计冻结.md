# PreparedModelInvocation与PreDispatch Gate V1设计冻结

## 时间

2026-07-16 21:05:08 +0800

## 粗粒度事件

Model Invoker已收口additive、neutral的`PreparedModelInvocation / PreDispatch Gate V1`合同设计、测试矩阵和实施计划。本轮只同步`.properties.rax`资产，没有写Go、没有修改Harness/Tool/Application/Runtime、没有stage或commit。

设计冻结为两阶段：先在严格零Provider/Backend/secret/pool/session/process/network调用的`PurePrepare/Map/Seal`阶段封存完整Unified Request、ordered Tools/ToolPolicy、Plan、Route、Profile、Capability/Registry snapshot、`ActualToolSurfaceDigest`和`ActualProviderInjectionDigest`；随后create-once Historical Fact与session-level Current Projection，以固定`PreparedRef + CurrentRef`调用public CommitGate并验证current ACK，最后才允许Preflight、Resolve、Capabilities、secret/pool、Open、Invoke/Stream等外部路径。

中央语义P0已闭合到资产：`ActualToolSurfaceDigest`严格使用Tool公共`ComputeExpectedInjectionDigest(entries)`canonical，只与`ToolSurfaceManifest.ExpectedInjectionDigest`exact比较；`ActualProviderInjectionDigest`是Model richer body，覆盖ToolChoice、ParallelToolCalls、hosted/provider extensions，不与Tool expected或Context Frame注入digest混比。Context Expected/ActualInjectionManifest与InjectionConformance保持独立链。

Surface DAG补充裁决后，Registry exact Ref从Model nominal提升为Runtime ports唯一neutral `runtimeports.RegistrySnapshotRefV1{Owner,ContractVersion,ID,Revision,Digest}`；其asset candidate及Assembly composite Ref/Projection/Reader candidate现已落盘，Authority仍是Ref.Owner。Model Prepared Historical/Current、Harness composite、Tool Binding必须直接使用未来Runtime concrete type，不alias/复制；Model不依赖Harness/Tool。

Model在PurePrepare前通过Runtime public Authority Exact Reader按完整candidate Ref读取、逐字段校验并pin，Historical与Current只无损携带；Gate再验证Ref.Digest等于Tool Surface Current Manifest中的`RegistrySnapshotDigest`，禁止从裸digest反推完整Ref。当前asset candidate已含`RegistrySnapshotExactReaderV1`，但Runtime public Go nominal/Reader尚未实现，因此Model Go与production root保持NO-GO。

`PreparedModelInvocationCommitGateV1`方法集冻结为三Owner唯一签名：`Commit(ctx, PreparedRef, CurrentRef)->ACK`和`InspectExactAck(ctx, AckRef)->ACK`。Harness只能实现并注入该接口，不能删CurrentRef、改方法名、使用缩水DTO或另建生产Gate签名。

第四审继续冻结：Model neutral `PreparedModelInvocationSurfaceBindingRefV1`与Tool BindingRef必须逐字段同形为`Owner/ContractVersion/ID/Revision/Digest`；live Tool `Kind/ID/Revision/Digest`旧Ref登记为Tool P0 Delta，Model不得推断补全。actual-point只用该完整Ref单参调用Tool Exact Reader并返回Binding+ToolAck；Model ACK不预携ToolAckRef。Tool owns Binding/ToolAck，Harness/host owns Model ACK create-once Repository与Inspect，Model只拥有nominal/canonical/dispatch guard。

一个Invocation epoch只允许一个Surface Binding/ACK。retry、Stream/Open和Direct continuation在每个真实边界前重新Inspect/Validate同一个Current与ACK，并产生非权威attempt validation receipt；receipt和ACK都不是Authority/Permit。跨TTL或Tools、ToolChoice、ParallelToolCalls、provider mapping漂移必须fail closed并创建新Invocation epoch，禁止同Invocation更新Binding。

## 第五审状态

Surface第五审裁决为`NO`。短Gate `Commit/InspectExactAck`与ACK canonical已通过；该轮清除了Registry旧名和陈旧阶段状态，唯一公共类型固定为`runtimeports.RegistrySnapshotRefV1`，并补入Harness、Tool各自定义同形named type/alias/wrapper/wire mirror时必须失败的type identity反例。此段保留为历史裁决。

## 第六审状态

Surface第六审确认Model本地合同已基本闭合，并纠正此前笼统的Runtime资产状态：Runtime asset candidate已经落盘。第六审开始时资产尚缺`RegistrySnapshotExactReaderV1`，但Runtime Owner在本轮并发补齐并冻结其exact单Ref合同；当前live external P0已从2项收敛为1项：在Runtime public ports实现已冻结candidate的Registry/Assembly nominal与Reader。该项未闭合且无明确Go授权前，Model继续不写Go。

## 当前P0

1. Model Profile Compiler提供完整sealed ProfileDigest；
2. Runtime Go实现Registry/Assembly public nominal与Reader；Authority仍是Ref.Owner，Model无损carrier并与Tool Surface Current裸digest交叉；
3. PurePrepare与Gate后外部行为物理拆分；
4. Gate早于Adapter.Preflight和全部Resolve/Capabilities/secret/pool/process调用；
5. sealed capability snapshot闭合映射因果；
6. Historical/Current/CurrentRef/ACK/AckRef/SurfaceBindingRef canonical、clock、create-once与恢复合同待external P0落地后实现并跑conformance；Historical NotAfter只作为current资格上界，不作为retention TTL；
7. 一个Invocation唯一Binding与统一Model guard待明确Go授权后实现；
8. Tool BindingRef同形版本、单Ref Exact Reader与Model ACK无ToolAckRef属于Tool/Harness相邻Owner实现与conformance条件，不计入当前external P0；
9. Realtime/opaque配置必须有strict tool extractor或NoToolSurfaceProof。

## 残余边界

- 当前没有Go实现、production driver或composition root；
- live Plan尚不能诚实提供完整ProfileDigest；Runtime asset candidate及Registry Exact Reader合同虽已落盘，但Runtime public Go nominal/Reader尚未实现，Registry Owner实现及Model pin接线也尚未开始；
- root Invoker、RouteGateway、Direct、`model-invoker/execution/harness`、retry、continuation和Realtime尚未迁移统一guard；
- terminal ToolCall Observation只能做Invocation历史回链，不能替代pre-dispatch proof；
- 等待当前一项external P0闭合并通过联合conformance后再申请Go授权。

## 编译前等待状态

Surface合同方向已收口，Model Owner当前只维护编译前清单，不写Go。当前external P0为1项：Runtime public ports实现Registry/Assembly nominal与Reader。Registry Owner、Tool与Harness的后续实现/conformance属于相邻Owner接线条件，不计入本轮external P0数量。该external P0未落地时Model implementation、integration composition与production root均为NO-GO；禁止以Model私有neutral DTO、alias或wrapper解阻。

Delta落地后必须先跑旧Gate方法、缺CurrentRef、本地Registry/Assembly type、Runtime type identity漂移、裸digest反推、Tool Kind旧Ref、双Ref Tool Reader、Model ACK携ToolAckRef、跨Owner import和TTL crossing反例，再决定是否申请Go授权。

## 资产

- [主设计](../../design/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1.md)
- [测试矩阵](../../design/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1-test-matrix.md)
- [实施计划](../../plan/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1.md)
