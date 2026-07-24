# Runtime Model Pre-Dispatch Assembly Current V1设计候选

- 时间：2026-07-16 21:53:00 +08:00
- 状态：Runtime ports asset-only候选已闭合，等待Runtime/Harness/Model/Tool联合设计Review；未授权Go实现。

本事件冻结了中立`RegistrySnapshotRefV1`及`ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1`候选。Runtime ports只拥有Go nominal类型；Harness Assembly Owner仍拥有语义、发布、revision/current index和Reader实现。完整Generation复用`GenerationArtifactRefV1`，Projection exact绑定Handoff、BindingSet、Manifest、Conformance、ToolSurface、Profile、Registry、Semantic、Currentness、Checked/Expires和ProjectionDigest。

Harness未来应直接发布Runtime public DTO；Model只无损携带；Tool直接嵌入同一Projection，禁止alias、echo、generic ObjectRef或裸digest补全。Runtime不实现Harness Store/publisher/CAS，不持有Surface/Prepared事实，也不把该Projection当Provider gate。当前无Go、production root/backend/durability/SLA，不stage、不commit。
