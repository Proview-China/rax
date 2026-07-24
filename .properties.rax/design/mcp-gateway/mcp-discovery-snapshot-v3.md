# MCP Discovery Snapshot V3来源闭包

## 1. 裁决与目标

用户已确认采用Snapshot V3，而不是另立Snapshot外部来源索引。V3在不修改V2历史canonical的
前提下，把每个受治理Discovery Page、Receipt、Apply、Material Set及每个Tool/Resource/Prompt
Material exact Ref纳入Snapshot canonical。V2保持只读兼容，不包装成V3。

状态：`implementation_software_test_yes`。Tool/MCP Owner的immutable snapshot、Repository及
SDK/API/CLI exact Inspect已实现，并通过定向ordinary×100、race×20及模块full ordinary/race/vet；
不代表production持久Backend、自动映射或能力启用。

## 2. 强类型对象

`MCPCapabilitySnapshotV3`保留V2的Server、Connection、协议、Server Info/Capabilities、三类
Observation、Conformance、Residual和时间窗口，并增加：

- `Pages []MCPDiscoveryPageProvenanceV3`：Namespace、PageOrdinal、Command、ProtocolReceipt、
  ApplySettlement、ResponsePageDigest、对应typed Material Set Ref；
- `ToolMaterials []MCPToolMaterialProvenanceV3`：完整Tool Observation、Page Receipt、
  `MCPToolDiscoveryMaterialRefV1`；
- `ResourceMaterials []MCPResourceMaterialProvenanceV3`；
- `PromptMaterials []MCPPromptMaterialProvenanceV3`。

Page按`Namespace + PageOrdinal`排序且ordinal从0连续；三类Material分别按Name、URI、Name排序。
每个Snapshot Observation必须恰有一个同内容Material provenance，不能缺失、重复或跨Namespace；
Material的Page Receipt必须存在于同一Snapshot的对应Page。ValidationDigest覆盖Observation与完整
provenance，Snapshot Digest覆盖所有字段。

V3 ID由`Server + Connection + ConnectionEpoch`派生，前缀与V2分离；Revision仍由调用方显式
提供并走current+1 full-Ref CAS。V2/V3不得共享ID或type-pun。

## 3. 聚合与原子边界

聚合固定执行：

```text
S1 Connection/Connect Receipt
  -> each Command/Applied/Receipt/Observation/typed Material Set exact Inspect
  -> validate cursor chain and terminal page
S2 same exact closure
  -> reject drift/clock rollback/expiry
  -> Seal Snapshot V3
  -> Repository create revision 1 or current+1 CAS
```

Material Set必须与Page Receipt的Command、Connection、ResponsePageDigest以及Observation entries
逐字段相等。V3只保存已由Page physical Repository原子提交的Material Ref，不复制raw JSON。
聚合回包丢失只按exact V3 Ref Inspect；Unavailable/Indeterminate不等于NotFound。

## 4. Reader与边界

公开只读口：

```go
type MCPCapabilitySnapshotExactReaderV3 interface {
    InspectMCPCapabilitySnapshotV3(context.Context, ObjectRef) (MCPCapabilitySnapshotV3, error)
}

type MCPCapabilitySnapshotCurrentReaderV3 interface {
    InspectCurrentMCPCapabilitySnapshotV3(context.Context, string) (MCPCapabilitySnapshotV3, error)
}
```

SDK/API按exact Ref双读同一immutable Snapshot并用fresh clock验证；不得按tool name、URI、latest或
不完整digest查询。CLI只可输出有界typed provenance，不输出Material raw JSON。

## 5. Owner与非Owner

- MCP Gateway：Page/Receipt/Observation/Material/Snapshot V3语义与Repository；
- Tool Engine：后续Mapping Manifest，只消费V3与Tool Material exact Ref；
- Runtime：Discovery Effect治理，不解释Snapshot语义；
- Application/Harness：未来调度与总装，不创建Snapshot或Material；
- Provider：只产生Observation，不能声明V3或Mapping事实。

V3不授Authority、Review、Fence、Admission或执行权；Resource不变成Context Fact，Prompt不变成
系统指令。

## 6. 硬反例与测试

- V2 Ref包装成V3、同ID跨版本复用；
- Page Material Set缺失、跨Receipt/Connection/Namespace、ResponsePageDigest漂移；
- Observation有条目但Material provenance缺失/重复/换ObjectDigest；
- Page ordinal断链、terminal cursor缺失；
- S1/S2换Command/Apply/Receipt/Material Set；
- old revision回退current、ABA、same revision换digest；
- typed-nil Reader、nil/canceled context、clock rollback、TTL crossing；
- 64同Ref并发多winner或不同lineage被全局锁串行；
- SDK/API返回slice可修改Store。

门：unit、whitebox、blackbox、fault、Conformance、ordinary×100、race×20、full ordinary/race、
vet、gofmt、import boundary与diff-check。
