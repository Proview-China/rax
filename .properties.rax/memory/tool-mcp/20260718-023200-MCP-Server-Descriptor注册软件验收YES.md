# MCP Server Descriptor注册软件验收YES

时间：2026-07-18 02:32 +08:00

## 结果

- 新增Tool-owned唯一`MCPServerDescriptorRepositoryV1`，原子维护immutable history/current。
- revision 1只允许create；successor必须携current full Ref且revision=current+1。
- current前进后重投旧revision、wrong expected、same revision换内容与ABA全部Conflict。
- Go SDK新增`RegisterMCPServerV1`、exact Inspect与current Inspect；返回值deep clone。

## 实际门

- targeted ordinary x100：PASS。
- targeted race x20：PASS。
- 64同successor并发、lost reply exact Inspect、typed-nil、nil/canceled context、future Descriptor：PASS。

## 边界

Register只登记Descriptor，不Connect、不Initialize、不Discover，不创建Connection、进程、网络、
Credential、Provider authority、production backend/root或SLA。
