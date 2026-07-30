# Agent Builder → AgentPackage V1 首合同计划

## 状态

本计划已按用户冻结方向进入首个公共合同实现；完成后保留为历史计划。

## 产物

- [x] 代码式 `DefinitionSourceBuilderV1`；
- [x] 严格 YAML/JSON 到现有 `AgentDefinitionSourceV1` 的声明式入口；
- [x] `AgentPackageRefV1`、`AgentPackageLockManifestV1`、`AgentPackageV1`；
- [x] 只消费 `ResolveResultV1` 的纯编译/封装 API；
- [x] canonical、digest、clone、Fail Closed validation；
- [x] 单元与真实 Definition→Assembler→Harness→Package 黑盒测试；
- [ ] Host/Runtime/Loop/Console/TS Wire：明确留给后续独立合同，不在本计划实现。

## 验收门

1. ordinary、race、vet 全绿；
2. 同 lock 重复 Seal 与伪造不同调用方时间得到完全相同 Package；
3. Release omit/add/splice、Facts/Catalog/Binding/Assembly splice 均拒绝；Release 重排保持相同 Package；
4. Manifest/Graph/Handoff swapped ID 即使 digest 合法也拒绝；
5. 真实 Assembler fixture 经现有 Harness Compiler 产出完整 exact Package closure；
6. `gofmt`、`git diff --check` 与范围检查通过。
