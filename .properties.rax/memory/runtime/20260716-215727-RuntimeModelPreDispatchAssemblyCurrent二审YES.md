# Runtime Model Pre-Dispatch Assembly Current V1二审YES

- 时间：2026-07-16 21:57:27 +08:00
- 状态：asset-only第二次独立只读设计复审YES，P0/P1/P2=0/0/0；尚未授权Go实现。

首审唯一P0是`ModelPreDispatchAssemblyCurrentRefV1`为缩水结构。Runtime资产已与Harness exact DTO对齐：Ref自身完整携ToolSurface、Profile、Registry、Semantic、Currentness、Checked/Expires与ProjectionDigest；Projection逐字段闭合完整Generation/Handoff/BindingSet/Manifest/Conformance/Watermark，Registry Owner使用`core.OwnerRef`。

canonical冻结own-ref/digest排除：计算ProjectionDigest时清空`Projection.ProjectionDigest`、`Projection.Ref.Digest`和`Projection.Ref.ProjectionDigest`，完成后三者exact相等。type owner仍是Runtime ports，semantic/publisher/current owner仍是Harness；Model/Tool只无损携带，禁止alias/echo。当前不写Go，不建立production root/backend/SLA，不stage、不commit。
