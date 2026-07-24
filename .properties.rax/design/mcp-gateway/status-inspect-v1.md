# MCP Status/Inspect V1

本切片只暴露现有Tool-owned MCP Lifecycle事实的exact只读状态，不触发Connect、
Discover、Refresh、Call、Drain或任何外部Effect。

SDK输入`Connection ObjectRef{ID,Revision,Digest}`，通过注入的Lifecycle Reader按ID做
S1/S2两次读取，逐字段确认同一Record、同一Connection exact ref及单调Owner clock。
NotFound、digest/revision漂移、读间变化、clock rollback、nil/typed-nil/canceled context
全部Fail Closed。返回值为deep-copy `mcp.ConnectionRecord`；Snapshot只显示已持久事实，
不把stored active冒充当前可执行资格，也不授予Provider权。

CLI仅新增`mcp status --id --revision --digest`，通过注入SDK Port读取后一次性输出JSON。
既有构造器不注入MCP Port时该命令仍unsupported；`mcp discover/connect/call`继续拒绝。
无根二进制、网络、进程、Credential、official SDK Session或production root。

实现状态：`implementation_software_test_yes`。SDK/CLI定向ordinary×100、race×20、
Tool full ordinary/race与vet通过；64并发exact读取、S1/S2 drift、typed-nil、
canceled context、wrong digest和clock rollback均Fail Closed。
