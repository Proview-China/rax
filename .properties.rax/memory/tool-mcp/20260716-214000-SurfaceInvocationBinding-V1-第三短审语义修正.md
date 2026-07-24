# SurfaceInvocationBinding V1 第三短审语义修正

时间：2026-07-16 21:40 CST

## 事件

Tool Owner仅修订资产，吸收Surface bridge第三短审实时P0；未写Go、未修改Model/Harness/Application/Runtime、未stage/commit。本记录覆盖此前候选中的Surface Binding TTL policy与Context注入混义，但不改写旧memory。

## 当前合同

- Binding直接无损保存Model public Prepared Fact Ref/Historical Fact与Current Ref/Projection；Tool不复制Prepared或Registry第二套nominal。`InjectionRegistrySnapshotRefV1`保留Registry Owner签发的`Owner + ContractVersion + ID + Revision + Digest`；
- Surface Registry exact来源仅为Model public Prepared Reader返回的完整Registry Ref，且其Digest必须等于`ToolSurfaceManifest.RegistrySnapshotDigest`；
- Tool public `ComputeExpectedInjectionDigest`重算的Manifest expected只与Model `ActualToolSurfaceDigest`强等；`ActualProviderInjectionDigest`原样保存但不与Tool expected比较；Context Frame/field注入链完全移出；
- Binding/Ack由Tool Owner clock、唯一Repository、EnsureRequest→internal Commit create-once形成，不授Provider执行权；
- V1无默认秒数或额外TTL cap。`NotAfter=min(Historical NotAfter, Prepared Current Expires, Surface Current Expires, Harness composite Expires, required RequestedNotAfter, caller deadline)`；
- 每Invocation单Binding；旧Invocation不得第二Binding或续租，变化/跨TTL必须新Invocation epoch。每个provider attempt/Stream/Open/continuation边界重新InspectExact+ValidateCurrent。

## 当前门

最近一次有计数的官方短审仍为NO（P0=4/P1=1/P2=0）。Tool本轮已修已知字段/语义项，但Model public Go nominal、Harness public composite/Gate nominal、逐字段mapping及全Provider路径证明仍需联合独立复核；Tool Go、PD-TM-04 P4、system与production继续hard-block。

## 入口

- [SurfaceInvocationBinding V1](../../design/tool-engine/surface-invocation-binding-v1.md)
- [PD-TM-04第七设计修正](../../design/tool-engine/pd-tm-04-seventh-candidate.md)
- [Tool/MCP计划](../../plan/tool-mcp/tool-mcp-v1.md)
