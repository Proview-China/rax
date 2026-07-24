# SurfaceInvocationBinding V1第五审NO返修

## 时间

2026-07-16 21:57 +08:00

## 粗粒度事件

Surface第五审结论为NO。Tool Owner只同步设计、计划与模块资产，未写Go、未修改Model/Harness/Application/Runtime/Context，未stage/commit。

本地返修将Prepared Historical/Current、Assembly与Binding内的Registry exact Ref全部统一为候选`runtimeports.RegistrySnapshotRefV1{Owner,ContractVersion,ID,Revision,Digest}`；禁止任何旧Model私有Registry nominal、alias、type-pun、JSON重编码或缩水digest。Registry Owner仍是Authority，Model Historical仅为无损carrier。Tool Binding继续直接嵌入候选Runtime Assembly Ref/Projection；exact Reader仍为单BindingRef入参返回Binding+ToolAck；Ack canonical仍按own Ref.Digest置空、顶层Digest排除、computed digest一次写入两处。

live Runtime ports尚无`RegistrySnapshotRefV1 + ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1`，Runtime neutral Port Delta仍是external P0。第五审复审和Delta落盘前不自评YES；Tool Go、P4/system/production继续NO-GO。
