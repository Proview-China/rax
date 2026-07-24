# 官方MCP list_changed软件验收YES

时间：2026-07-18 00:19（Asia/Shanghai）

## 事件

Tool/MCP新增`MCPListChangedObservationV1`、owner-local in-memory journal与
`OfficialSDKListChangedBridgeV1`。Bridge安装official SDK Tools/Resources/Prompts
notification handlers，绑定exact initialized Session、Connection与旧Snapshot；同一
namespace/旧Snapshot的重复通知合并为一个pending Observation，只有新Snapshot已存在后
才接受successor acknowledgement。

## 验证

- official SDK in-memory真实list-changed通知通过；
- targeted ordinary×100、race×20通过；
- 64并发同pending单Fact、wrong/unbound Session、typed-nil、nil/canceled context、
  clock rollback、pending换Snapshot、wrong ack与exact history Inspect反例通过；
- Tool full ordinary、full race、vet与gofmt通过。

## 边界

状态为`implementation_software_test_yes`，仅表示owner-local通知消费。Handler不调用
Discovery、不修改Snapshot、不连接网络，也不授Runtime Effect权。通知到受治理新Discovery
Operation的Application调度、production State Plane/root和真实Connect仍NO-GO。
