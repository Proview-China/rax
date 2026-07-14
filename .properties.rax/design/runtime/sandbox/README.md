# Runtime与Sandbox、Placement摘要

## 1. V1基线

- 一个具体AgentInstance独占一个active SandboxLease；
- 一个Identity只有一个活跃Lineage拥有执行权；
- Harness默认运行在Lease内；
- Runtime Kernel位于Agent无法取得控制权的边界外；
- Memory、Knowledge、正式Asset、权威Intent和Identity位于可销毁Sandbox外；
- 不承诺跨后端透明热迁移。

## 2. Sandbox Provider合同

```text
DescribeCapabilities
ValidateRequirement
Allocate
Inspect
Attach
StartEndpoint
Fence / Freeze / Thaw（按Capability）
Checkpoint（按Capability）
Revoke
Release
AttestIsolation
```

公共合同不出现Docker、Pod、VM、PID或宿主路径专属对象。每个Provider发布隔离、网络、Snapshot、GPU、迁移和远程执行能力及证据TTL。

## 3. Lease与隔离

Sandbox分配请求绑定已经预留但尚未active的Instance ID/epoch和Sandbox Requirement Digest。Provider返回的Lease依次处于：

```text
requested -> reserved_quarantined -> active -> releasing
          -> released | release_failed | release_indeterminate
```

`reserved_quarantined`必须强制禁止Harness启动、开放网络、挂载业务凭据和取得冲突Effect Domain；它可在ActivationCommit前安全占位。ActivationCommit把实际SandboxLeaseRef写入Instance并激活Identity Lease后，Provider才可以依据同一Fence把SandboxLease转为`active`。这不是跨系统原子事务；中途失败由Admission恢复协议Fence、Inspect和释放。

active SandboxLease绑定Instance ID、instance epoch、lease epoch和Fence。默认不继承宿主文件、环境变量、网络或凭据。Secret按组件、用途和时间最小暴露。Release必须分别报告进程、挂载、网络、设备、Secret路径和残留；无法确认时为indeterminate。

## 4. Placement与未来集群

Placement使用ResourceClaim、NodeRef、Fault Domain、Data Location、Affinity和Capability。节点失联时Control Plane不能假设资源死亡；必须Fence、Quarantine并由ReplacementPermit决定冲突能力。未来Agent Cluster可以提供相同Lease合同，但V1不设计调度器实现。

## 5. Provider远程状态不归Sandbox清理推导

Sandbox Release不结束Model Session、Batch、Hosted Tool、Prompt Cache或远程Sidecar。它们由[RemoteContinuation合同](../effects/README.md)单独追踪。Cleanup报告必须区分本地资源与Provider长期状态。

本地SandboxLease在进程、网络、挂载、设备与Secret路径均有充分证据时可以Release；不要求等待无冲突的远程Continuation结算。仍可能影响冲突域的远程残留继续由独立记录追踪并占用该域，不能因Sandbox Release而签发对应ReplacementPermit。

## 6. 最低反例

- `SBX-01`：容器删除但网络撤销无法确认，Cleanup不能Complete；
- `SBX-02`：一个Lease试图绑定两个活跃Instance，必须拒绝；
- `SBX-03`：节点失联后新实例未取得ReplacementPermit，只能Quarantined；
- `SBX-04`：第三方后端不支持必要网络隔离，不能因官方默认而静默降级。
- `SBX-05`：reserved_quarantined Lease在ActivationCommit前尝试启动Harness，Provider必须拒绝；
- `SBX-06`：ActivationCommit失败且Release响应丢失，Lease保持release indeterminate并阻断冲突域。
