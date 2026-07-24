# Agent Host H3 Owner Adapter Delta V1

## 裁决

本 Delta 只实现 Definition -> Assembler -> Harness Compile 的第一条 production-owner 纵切。Runtime Binding、Activation、Application、Readiness 和 6+1 Factory 实例仍保持 Host 窄 Port，不在本 Delta 内伪接线。

`HostConfigV1` 已冻结，不增加字段、不改变 JSON/canonical digest。Host Journal 继续只保存 generic exact refs/digests；Owner typed artifacts每次调用和重启都通过Owner public exact readers重建，不缓存第二份真值。

## Additive current readers

新增两个Host-owned public Port，精确签名见 `port-deltas/h3-owner-current-readers-v1.md`：

1. `DefinitionSourceCurrentReaderV1`：把opaque `definition_source_ref`原子映射到exact AgentDefinition ref。source stable ID与DefinitionID是不同类型，禁止same-string type-pun。
2. `ResolutionInputsCurrentReaderV1`：把opaque `catalog_ref + resolution_facts_ref`原子映射到同一projection内的exact Catalog/Facts refs。

两个projection均含 `ContractVersion/ObjectKind/stable IDs/exact refs/revision/checked/expires/projection digest`，使用带domain/version/type discriminator的canonical digest。Reader不接受caller `now`；Host以自己的fresh clock校验。

## 读取顺序

1. Definition：Source Current S1 -> Definition Owner Current S1(active且exact==source映射) -> exact Definition -> Source Current S2 -> Definition Owner Current S2 -> finalNow。两个current全字段无漂移，Definition effective window与Source TTL在finalNow仍有效才返回。
2. Assembler：复读上述双current链；读取ResolutionInputs S1，转换为Owner typed exact refs并调用真实 `AgentAssemblerPortV1.Resolve`；Resolve后读取ResolutionInputs S2和两条Definition current S2，finalNow重验TTL/effective window，再exact校验Plan/BindingPlan/AssemblyInput linkage。
3. Harness：读取ResolutionInputs S1、exact Plan/Facts/Catalog，从exact Catalog选回Plan声明的exact releases，调用Owner mapper重建sealed AssemblyInput并与Host InputRef exact比较，再调用真实 `assemblycompiler.Compiler`；读取ResolutionInputs S2并在finalNow重验所有TTL。

## ConstructionGraph 映射

- 每个Manifest factory生成一个Host node；NodeID=FactoryID。
- FactoryKey由Factory -> Module -> exact ComponentManifest映射；artifact/contract/capability来自factory公开字段。
- component dependency使source component的每个factory依赖目标component的全部factories；optional目标缺失才跳过。
- explicit DependencySpec：known-known连边；one-known Fail Closed；both-unknown required Fail Closed；both-unknown optional才略过。
- Graph/Manifest/Handoff refs使用真实compiler digest和GenerationID派生的稳定artifact ID。

## 禁止

- 不复制Owner DTO；不读取fakes/internal/testkit；
- 不缓存Definition、Plan、Facts、Catalog、AssemblyInput或CompileResult；
- 不从未知ref、模块名、字符串路径猜factory/dependency；
- 不将本纵切写成H3完成或production Agent Ready。
