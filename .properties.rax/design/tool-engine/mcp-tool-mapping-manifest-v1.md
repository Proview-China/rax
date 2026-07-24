# MCP Tool Mapping Manifest V1

## 1. 裁决

用户已确认：MCP Tool不能按名称、Schema、annotations或模型猜测自动成为Praxis Capability/Tool；
必须由版本化显式Mapping Manifest进入Tool Registry Admission。自动候选与snapshot-only均不是V1。

状态：`implementation_software_test_yes`。Tool Owner的Manifest、exact Reader及Mapping-aware
三Record单RegistryRevision Admission已实现，并通过定向ordinary×100、race×20及模块full
ordinary/race/vet。V1只允许`submitted -> admitted`，不自动`active/enable`，不授Runtime
Authority、Review、Fence、Credential或Provider执行权。

## 2. 历史Mapping Manifest

`MCPToolMappingManifestV1`字段：

| 字段 | exact语义 |
|---|---|
| `Ref` | Tool-owned create-once immutable Ref，revision 1 |
| `Owner` | 映射提交Owner，必须与目标Tool Owner exact |
| `Snapshot` | exact `MCPCapabilitySnapshotV3` Ref |
| `Source` | Snapshot中的完整`MCPToolObservationV2` |
| `SourceMaterial` | exact `MCPToolDiscoveryMaterialRefV1` |
| `Capability` | exact `CapabilityDescriptor` ObjectRef |
| `Tool` | exact `ToolDescriptor` ObjectRef |
| `MappingPolicy` | 版本化namespaced policy profile |
| `MappingPolicyDigest` | policy内容摘要，不用布尔allowed代替 |
| `CreatedUnixNano` | Tool Owner历史形成时间 |

ID由`Snapshot + SourceMaterial + Tool`完整坐标派生；Digest覆盖完整Manifest。相同ID同内容幂等，
任一source/target/policy变化Conflict并要求新ID，不原位改写。

`ValidateAgainst`必须证明：

- Snapshot V3包含exact Source及SourceMaterial，且Page/Material provenance完整；
- Material canonical JSON与Source摘要逐字段一致；
- Tool `Mechanism == mcp`、`ArtifactDigest == SourceMaterial.Digest`；
- Tool exact绑定Capability，二者Input/Output Schema ContentDigest分别等于MCP Source摘要；
- Tool Owner等于Mapping Owner；Effect/Risk/Review/Authority/Budget/Sandbox由显式Capability/Tool
  Descriptor给出，禁止从MCP annotations推断。

## 3. Registry Admission

唯一强写流程：

```text
Submit exact Capability + Tool + Mapping -> all submitted
  -> S1 exact-current Snapshot V3 + exact Material + Mapping/Capability/Tool
  -> S2 same refs, fresh clock, requested deadline
  -> one Registry lock/CAS:
       Capability submitted -> admitted
       Tool submitted -> admitted
       Mapping submitted -> admitted
```

三条Record使用同一successor RegistryRevision与Updated time，任一expected RegistryRevision、
Object Ref、state或dependency漂移则零写。generic `Transition`不得把`MechanismMCP` Tool推进
admitted/active；必须走Mapping-aware Port。V1不做active，后续Enable需独立治理和Reconcile。

Snapshot/Material是immutable source，current资格由Snapshot V3窗口与caller deadline共同最早上界
约束。S1/S2后形成的Admission current proof只证明本次Tool Registry CAS输入，不授执行权。

## 4. Repository与Reader

Tool唯一Registry维护Mapping history/current Record；公开exact Reader只按`MCPToolMappingManifestRefV1`
读取，不提供name/latest/source弱查询。SDK可提交、Inspect及调用Mapping-aware Admission；API/CLI首批
只读，写命令仍需宿主治理与production composition。

## 5. 失败与恢复

- Submit/Admission回包丢失：Inspect exact Mapping与三条Registry Record，不创建第二Mapping；
- NotFound只表示exact local事实不存在；Unavailable/Indeterminate不得当NotFound；
- Snapshot过期或S1/S2 drift：零Registry写；
- generic MCP Tool Transition：PreconditionFailed；
- Mapping admitted不等于Tool active，不等于Provider可调用。

## 6. 硬反例

- 同名MCP Tool跨Server/Snapshot拼接；
- Snapshot Source与Material ObjectDigest、Schema digest或Page Receipt漂移；
- SourceMaterial合法但不在Snapshot V3 provenance；
- Tool ArtifactDigest不等于Material digest，或Mechanism不是MCP；
- Capability/Tool schema与MCP source不一致；
- annotations自动生成Risk/Review/Authority；
- generic Transition绕过Mapping；
- 三Record部分提交或使用三个不同RegistryRevision；
- expired Snapshot、clock rollback、typed-nil、nil/canceled context仍Admission；
- same ID换policy/target、64并发多winner、lost reply后重复Admission；
- admitted后自动active/enable或直调Provider。

测试门同Snapshot V3，并增加Registry多记录原子性、generic绕过、same/different key并发与SDK import
boundary。
