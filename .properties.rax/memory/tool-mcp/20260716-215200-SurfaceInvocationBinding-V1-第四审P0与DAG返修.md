# SurfaceInvocationBinding V1第四审P0与DAG返修

## 时间

2026-07-16 21:52 +08:00

## 粗粒度事件

Tool Owner只同步设计、计划和模块资产，未写Go、未修改Model/Harness/Application/Runtime/Context，未stage/commit。Surface第四审P0返修与最新DAG裁决已进入候选：

- Tool BindingRef固定`Owner + ContractVersion + ID + Revision + Digest`，可与Model neutral SurfaceBindingRef无损对应；
- `InspectExactToolSurfaceInvocationBindingV1(ctx, BindingRef)`只接BindingRef并返回Binding+ToolAck，AckRef由Reader内部验证；
- Ack canonical计算时将`Ack.Ref.Digest`置空且排除`Ack.Digest`，computed digest一次性写入两处，禁止digest回流；
- Prepared全部直接使用Model public nominal；Registry Ref Authority是Registry Owner，Model Historical只是无损carrier；
- Assembly composite不import Harness也不定义Tool echo。Runtime ports需additive发布唯一`ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1 + RegistrySnapshotRefV1`，Harness是Assembly语义/发布Owner，Registry Ref Authority是Registry Owner，Tool/Harness直接共享nominal；
- 每个attempt/Open/Stream/continuation actual-point必须复读Prepared Current、Binding/ToolAck、Surface Current和Assembly composite。

当前不沿用旧P0/P1计数，不自评YES。live Runtime ports尚无上述nominal/Reader，该项是external P0。Runtime ports Delta、独立终审与用户P4 Go授权前，Tool Go继续hard-block，P4/system/production均NO-GO。
