# MCP Process Observation有界读取 V1

## 1. 结论

MCP `notifications/progress`与`notifications/message`已经由official Go SDK Handler进入
Tool/MCP Owner的immutable `MCPProcessObservationV1` Journal。本切片在同一Journal上增加
exact Inspect与有界pull-page读取，供Go SDK、transport-neutral API和可嵌入CLI暴露过程摘要。

它不是长连接Watch协议，不创建goroutine、Webhook、SSE或Provider调用；空页也不能证明
Provider未执行。真正的follow/Webhook、Task Watch和production持久Backend继续NO-GO。

## 2. Owner与边界

- Tool/MCP Owner拥有Observation合同、source sequence、唯一Journal、exact Reader和page Reader；
- official MCP SDK只提供标准notification Handler，回调内容仍是不可信Observation；
- SDK/API/CLI只消费`MCPProcessObservationReadPortV1`，不能接触Session或Provider handle；
- Runtime Evidence、Timeline、Review、ToolResult、Action Settlement与Authority均不由该Port创建；
- 不新增Application/Harness/Context/Runtime写口，也不选择HTTP、gRPC、SSE、数据库或进程拓扑。

## 3. 公共合同

### 3.1 Reader

```go
type MCPProcessObservationExactReaderV1 interface {
    InspectMCPProcessObservationV1(context.Context, MCPProcessObservationRefV1) (MCPProcessObservationV1, error)
}

type MCPProcessObservationPageReaderV1 interface {
    ReadMCPProcessObservationPageV1(context.Context, MCPProcessObservationPageRequestV1) (MCPProcessObservationPageV1, error)
}

type MCPProcessObservationReadPortV1 interface {
    MCPProcessObservationExactReaderV1
    MCPProcessObservationPageReaderV1
}
```

### 3.2 Page坐标

`MCPProcessObservationPageRequestV1`固定包含：

- exact Connection `ObjectRef{ID,Revision,Digest}`与`ConnectionEpoch`；
- exact Capability Snapshot `ObjectRef`；
- exclusive `AfterSourceSequence`；
- `Limit`，闭集为`1..256`。

Connection的compact `ObjectRef.Digest`已经绑定完整`MCPConnectionRef`，读取方无需复制
Tenant/Session字段。Journal索引仍按Connection full Ref+Epoch+Snapshot exact分流，不允许只按ID、
Session ID、名称或latest读取。

### 3.3 Page投影

`MCPProcessObservationPageV1`包含原Request、有界Observations、
`NextAfterSourceSequence`、本次锁内快照的`UpperBoundSourceSequence`、`HasMore`与`PageDigest`。
Observations必须严格source-ordered、全部大于after并逐项回扣相同Connection/Epoch/Snapshot；
`PageDigest`覆盖完整投影且排除自身。

Page是读取投影而非持久Fact。并发append可以使后续同Request看到更高upper bound；每个已返回
Page本身仍由digest自洽。调用方继续拉取时只使用前页`NextAfterSourceSequence`，不得把
`HasMore=false`解释成Provider完成或Action成功。

## 4. 唯一Journal与恢复

- `InMemoryMCPProcessObservationJournalV1`同时实现Sink、exact Reader与page Reader；
- 每个exact Connection Ref+Epoch维护单调source sequence；每个Connection+Snapshot维护有序Ref索引；
- Record在同一锁内分配sequence、Seal Observation、写history并追加stream index；
- Page在同一读锁中固定upper bound并复制有界结果；index缺记录或Ref漂移为Conflict；
- exact NotFound只表示该Observation Ref不在本Journal，不等于Effect未发生；
- 回包丢失、Unknown或进程重启不能据空页重派Provider，只能沿原Attempt/Entry/Receipt Inspect链恢复。

当前Repository是owner-local内存实现；没有production durable backend、retention SLA或跨进程
resume保证。未来Backend必须保持相同source order、exact Ref和page digest语义。

## 5. SDK/API/CLI

- Go SDK `MCPProcessV1`：immutable exact Observation双读一致性；page单次有界读取并验证自包含digest；
- API `MCPReadV1`：新增同一page方法，不选择transport；
- CLI `mcp process`：显式接收Connection/Snapshot exact坐标、epoch、after与limit，只输出有界
  Observation摘要和digests，不输出原始log data或Provider payload；
- CLI没有`--follow`、Webhook、Cancel、Call或Provider入口，未注入Process Reader时保持unsupported。

## 6. 失败与反例

以下情况必须零Provider、零新Effect、零Evidence/Timeline/ToolResult：

1. nil/typed-nil Reader、nil/canceled context；
2. Connection/Snapshot Ref或Epoch不完整，limit为0或大于256；
3. page内跨Connection、跨Epoch、跨Snapshot、乱序、重复或小于等于after；
4. Observation、cursor、upper bound、HasMore或PageDigest漂移；
5. Journal stream index指向缺失/换Ref history；
6. 调用方把空页、NotFound或`HasMore=false`当作confirmed-not-applied；
7. CLI输出raw notification data，或SDK/API启动后台follow、网络或Provider；
8. 用process Observation替代Runtime Evidence V3 Consumption、Review Verdict或Settlement。

## 7. 状态

Owner-local contract/Journal/SDK/API/CLI与测试已实现；production durable Journal、retention、
streaming follow/Webhook、Task Watch和跨Owner总装未实现，不构成production GO。
