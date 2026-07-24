# Model Pre-Dispatch Assembly Current V1实施计划

状态：**Runtime neutral ports纵切已实现并通过双独立代码审计YES（P0/P1/P2=0/0/0）及全门；相邻Owner适配和production root继续NO-GO**。

设计入口：[README](../../design/runtime/model-predispatch-assembly-current-v1/README.md)、[合同](../../design/runtime/model-predispatch-assembly-current-v1/contracts.md)、[Port Delta](../../design/runtime/model-predispatch-assembly-current-v1/port-delta.md)、[测试矩阵](../../design/runtime/model-predispatch-assembly-current-v1/test-matrix.md)。

## P1：联合冻结

- [x] 冻结type owner=Runtime ports、semantic/publisher/current owner=Harness；
- [x] 冻结完整`GenerationArtifactRefV1`复用；
- [x] 冻结`RegistrySnapshotRefV1`为Model/Harness/Tool共享的唯一neutral exact ref；
- [x] 冻结`RegistrySnapshotExactReaderV1`唯一签名：完整Ref请求、Authority Owner验证、historical/current分离、deep clone和closed errors；
- [x] 冻结Current Ref自身与Projection均携ToolSurface/Profile/Registry/Semantic/Currentness/Checked/Expires/ProjectionDigest，并冻结单方法Reader签名；
- [x] 冻结Projection own-ref/digest无环排除及最终三摘要exact相等；
- [x] 冻结Handoff/BindingSet/Manifest/Conformance/ToolSurface/Profile/Registry/Semantic/Currentness/TTL exact闭包；
- [x] 冻结Harness直接发布Runtime DTO、Tool禁止alias/echo；
- [x] 冻结Runtime不实现Harness Store/publisher/current index；
- [x] 第二次独立只读设计复审YES（P0/P1/P2=0/0/0）。
- [x] Surface第六审Registry Reader返修及后续代码双独立审计YES（P0/P1/P2=0/0/0）。

## P2：获授权后的Runtime ports实现

- [x] additive七个public declarations、Validate、canonical、digest与JSON shape；
- [x] Registry exact Reader capability narrowing、Authority/current/deep-clone/error conformance；
- [x] Reader method-set、typed-nil和import-boundary tests；
- [x] public conformance仅验证neutral current闭包，不实现Harness事实Owner；
- [x] 冻结既有`GenerationArtifactRefV1`hash与AssemblyInput V1不变；
- [x] target ordinary `count=100`、race `count=20`、Runtime full ordinary/race、vet、gofmt与diff-check全PASS。

## P3：相邻Owner适配

- [ ] Harness publisher/CAS/current Reader由Harness Owner另行授权；
- [ ] Model无损carrier由Model Owner另行授权；
- [ ] Tool移除echo/alias并直接嵌入Runtime Projection，由Tool Owner另行授权；
- [ ] production composition root另立设计与授权。

## 非目标

本Plan只在Runtime范围落地neutral DTO、Reader、Conformance与测试；未修改Harness/Model/Tool/Application，不实现Harness Store/publisher/CAS、backend/root，不选择数据库/RPC/SLA，不把Surface/Prepared、compile HookFace或Runtime Effect升级为Provider gate。
