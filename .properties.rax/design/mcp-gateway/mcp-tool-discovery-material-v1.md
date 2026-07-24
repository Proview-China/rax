# MCP Discovery Material V1（Tool / Resource / Prompt）

## 1. 结论

`MCPCapabilitySnapshotV2`继续只保存有界摘要；新增Tool/MCP Owner的
`MCPToolDiscoveryMaterialV1`、`MCPResourceDiscoveryMaterialV1`与
`MCPPromptDiscoveryMaterialV1`，原样保存official MCP SDK在一个已受治理
`tools/list|resources/list|prompts/list` page中返回的canonical JSON。Tool材料解决schema和
描述原文丢失；Resource/Prompt材料保留标准对象，但继续保持不同类型，绝不自动升级成Tool、
Context事实或系统指令。

本合同是Provider Observation材料，不是Capability、ToolDescriptor、Registry Admission、
Review Verdict、Authority、Permit或ToolResult。MCP annotations同样只是不可信提示。

## 2. 对象与Owner

### 三类exact Ref

- Tool：`praxis.tool-mcp.mcp-tool-discovery-material/v1`，ID绑定Page Command full Ref、
  `Tool.Name`与`Tool.ObjectDigest`；
- Resource：`praxis.tool-mcp.mcp-resource-discovery-material/v1`，ID绑定Page Command full Ref、
  `Resource.URI`与`Resource.ObjectDigest`；
- Prompt：`praxis.tool-mcp.mcp-prompt-discovery-material/v1`，ID绑定Page Command full Ref、
  `Prompt.Name`与`Prompt.ObjectDigest`；
- `Revision = 1`；
- `Digest`绑定完整Material canonical body。

### `MCPToolDiscoveryMaterialV1`

| 字段 | 语义 |
|---|---|
| `Ref` | Tool/MCP Owner exact历史材料坐标 |
| `Command` | 产生该材料的exact `MCPDiscoveryPageCommandV1.Ref` |
| `Connection` | 同一page绑定的exact `MCPConnectionFactRefV2` |
| `Source` | 与命名空间一致的`MCPToolObservationV2`、`MCPResourceObservationV2`或`MCPPromptObservationV2` |
| `CanonicalObject` | official SDK对象的有界、严格、canonical JSON；最大1 MiB |

`CanonicalObject`必须重算并逐项匹配`ObjectDigest`、description、input/output schema、
annotations与`_meta`摘要；name/title也必须与`Source`逐字相等。Input schema必须是有界JSON
object schema。Icons等标准字段保留在完整canonical object中并由`ObjectDigest`覆盖。
Resource另逐字段闭合URI/name/title/MIME type/size/description/annotations/meta；Prompt闭合
name/title/description/arguments/meta。Resource/Prompt标准扩展字段和icons由ObjectDigest保真，
不能从这些不可信字段推断权限、Review、Context优先级或Prompt authority。

## 3. 持久与恢复

`InMemoryMCPDiscoveryPagePhysicalRepositoryV1`是当前唯一owner-local物理仓：

1. Provider返回page后，adapter先构造摘要和canonical Tool JSON；
2. Page Receipt seal成功后，在同一仓锁内一次提交Receipt、Observation和对应命名空间的完整Material集合；
3. 每类page的Material数量、排序键（Tool/Prompt name，Resource URI）和每个`Source`必须与
   Observation逐项exact；跨命名空间Material必须为空；
4. 同stable page已observed时，Receipt与Material集合必须完全一致才可幂等返回；
5. 回包或提交结果不确定后只Inspect原Page Entry，不重发`tools/list`；
6. 三类`InspectExact*DiscoveryMaterialV1(ctx, exactRef)`只按完整Ref读取并deep-copy返回。

为使调用方无需读取内部物理Entry，三类`MCPDiscoveryPage*MaterialSetV1`按exact Page Receipt
返回该页的`{typed Observation, typed Material Ref}`有界、排序、唯一集合，并绑定Command、
Connection与ResponsePageDigest。只支持exact Receipt Reader，没有latest/name/URI弱查询。

当前实现是内存/测试State Plane，不宣称production durable backend或retention。

## 4. 后续映射边界

未来`MCP Tool -> Praxis Capability/Tool`映射必须同时复读：

- exact/current `MCPCapabilitySnapshotV2`中的`MCPToolObservationV2`；
- exact `MCPToolDiscoveryMaterialV1`，且全部摘要与Snapshot Tool逐字段相等；
- 由用户、组织Policy或已验证Package发布的版本化语义Mapping Manifest。

Effect、Risk、Review、Authority、Budget、Sandbox、Evidence、Conflict Domain与Scope不得从工具名、
schema或MCP annotations推断。用户已确认采用Tool Owner显式版本化
[MCP Tool Mapping Manifest V1](../tool-engine/mcp-tool-mapping-manifest-v1.md)；自动候选与
snapshot-only均不属于V1。

## 5. 验收

- official SDK Tool完整JSON可canonical round-trip；
- description/schema/annotations/meta/object任一换字节均Conflict；
- duplicate Tool name、超1 MiB、非法/非object input schema均Fail Closed；
- duplicate Resource URI或Prompt name、Resource字段/Prompt arguments摘要漂移均Fail Closed；
- wrong exact Ref、nil/canceled context、deep-copy tamper均不得泄漏或写入；
- same page 64并发最多一次Provider调用，observed重试只读；
- lost reply/Unknown只Inspect，Material缺失不得生成可映射Capability；
- full ordinary/race/vet与import boundary通过。

## 6. 当前状态

Owner-local三类contract、official SDK capture、Page物理仓原子保存、Go SDK/transport-neutral
API exact Reader及模块内CLI六种`tool|resource|prompt-material`与对应`*-material-set` kind已实现；Page
Receipt可exact枚举对应Material。Snapshot V3 provenance与显式Tool Mapping Manifest到Registry
Admission的owner-local软件闭环已完成；Resource/Prompt Context消费、active/enable、production
durable backend与能力启用仍未完成。
