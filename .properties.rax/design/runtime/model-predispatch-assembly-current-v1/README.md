# Model Pre-Dispatch Assembly Current V1 Runtime中立Port

状态：**Surface neutral Go双独立代码审计YES（P0/P1/P2=0/0/0）；Runtime中立DTO、Reader与public Conformance完成，全门PASS**。

## 1. 目标

本Delta在`runtime/ports`中提供一组中立、版本化、可exact比较的nominal DTO和单方法Reader，消除Harness与Tool互相导入实现包形成的SCC，也禁止Tool把Harness current projection降级成有损echo、alias、裸digest或自由map。

Runtime是这些Go类型的type owner，但不是Assembly事实的semantic/publisher/current owner。Harness Assembly Owner继续唯一负责发布、单调CAS current revision、实现Reader并解释Manifest/Conformance/Handoff语义。Runtime不实现Store、publisher、CAS、canonical事实Owner或production composition。

## 2. 唯一边界

- `RegistrySnapshotRefV1`：Runtime中立的Registry exact ref，Registry Owner仍是语义Owner；Model、Harness和Tool只能携带与exact比较；
- `RegistrySnapshotExactReaderV1`：以完整`RegistrySnapshotRefV1`为唯一请求坐标，由`Ref.Owner`指向的Authority Repository执行exact读取、current验证并返回deep clone；
- `ModelPreDispatchAssemblyCurrentRefV1`：Harness已发布current projection的exact key；
- `ModelPreDispatchAssemblyCurrentProjectionV1`：完整Generation、Handoff、BindingSet、Manifest、Conformance、Tool Surface、Profile、Registry、Semantic、Currentness和TTL闭包；
- `ModelPreDispatchAssemblyCurrentReaderV1`：按exact Ref读取current projection的单方法接口；
- `GenerationArtifactRefV1`直接复用Runtime live完整结构，不定义缩水版；
- Tool只能嵌入同一Runtime public Projection，禁止定义Tool alias/echo或重封摘要。

## 3. Owner裁决

| 对象 | type owner | semantic/publisher/current owner | consumer |
|---|---|---|---|
| `RegistrySnapshotRefV1` | Runtime ports | Ref中`Owner`指向的Registry Owner | Model/Harness/Tool exact carry |
| `RegistrySnapshotExactReaderV1` | Runtime ports | Ref中`Owner`指向的Registry Owner实现 | Model在PurePrepare前验证并pin clone |
| Assembly Current Ref/Projection/Reader interface | Runtime ports | Harness Assembly Owner | Model/Tool/Runtime Gateway只读 |
| Generation | Runtime既有Generation Owner | Runtime Generation Owner | Harness current复读 |
| Manifest/Conformance/Handoff | Runtime neutral DTO；事实类型不在Runtime | Harness Assembly Owner | exact carry/compare |

“type owner”不转移事实语义、发布权、current index或CAS所有权。

## 4. 当前裁决

Current Ref完整结构、own-ref/digest排除、唯一Registry exact Reader、Authority Owner、deep clone、historical/current和closed error语义均已落入Runtime Go。实现包含七个public declarations、Validate/canonical/digest/JSON shape、两个窄Reader、public-only Conformance、typed-nil与import-boundary反例；双独立代码审计均为YES（P0/P1/P2=0/0/0），target ordinary `count=100`、race `count=20`、Runtime full ordinary/race、vet、gofmt与diff-check均PASS。

完成不改变Owner边界：Harness publisher/CAS/current index、Model/Harness/Tool适配和system/production composition root仍为NO-GO；Runtime仍不持有Surface/Prepared事实，不实现Harness Store，不建立production backend、durability或SLA，也不把compile HookFace或Runtime Effect当作Provider gate。

## 5. 入口

- [精确合同](./contracts.md)
- [公共Port Delta](./port-delta.md)
- [测试矩阵](./test-matrix.md)
- [实施计划](../../../plan/runtime/model-predispatch-assembly-current-v1.md)
