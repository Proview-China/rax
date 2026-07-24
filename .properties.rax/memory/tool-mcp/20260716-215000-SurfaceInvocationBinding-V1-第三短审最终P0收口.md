# SurfaceInvocationBinding V1 第三短审最终P0收口

时间：2026-07-16 21:50 CST

## 事件

Tool Owner继续只修资产，未写Go、未修改Model/Harness/Application/Runtime、未stage/commit。本记录追加第三短审最终P0收口真值，不改写旧memory。

## 收口结论

- Prepared Fact Ref/Historical Fact与Current Ref/Projection直接引用Model唯一public nominal；无`ToolPrepared*`影子DTO或Kind翻译；
- Registry exact Ref Authority是Registry Owner。Model Historical只无损携带`Owner + ContractVersion + ID + Revision + Digest`，Tool Binding保存完整Ref并与`SurfaceCurrent.Manifest.RegistrySnapshotDigest`交叉；
- V1无独立Registry current资格，Surface Current以sealed Manifest/expiry承担Surface与该RegistrySnapshot组合资格；每个actual boundary复读Surface Current并核完整Ref，禁止裸digest/latest；
- Writer保持`Ensure`。Harness只实现Model Gate并在内部调用注入的Ensure；Fact/Created/NotAfter/Digest/private Commit均由Tool Owner clock与唯一Repository生成；
- Ack具有exact Ref并与Binding一同由唯一Repository Inspect。每个provider attempt/Stream/Open/continuation复读Prepared Current、Binding/Ack、Surface Current和Assembly composite；
- Tool Manifest expected只与Model ActualToolSurfaceDigest按public canonical强等，ActualProviderInjectionDigest独立保存；Context Frame/field链不进入；
- V1 TTL只有六项共同min：Historical NotAfter、Prepared Current Expires、Surface Current Expires、Assembly composite Expires、required RequestedNotAfter、caller deadline；无默认cap。

## 门禁

最近一次有计数的官方短审仍为NO（P0=4/P1=1/P2=0）。Tool已修已知本地字段/语义；仍待Model public Go nominal、Harness public composite/Gate、逐字段mapping及全Provider路径证明的联合独立复核。Tool Go、PD-TM-04 P4、system与production继续hard-block。

## 入口

- [SurfaceInvocationBinding V1](../../design/tool-engine/surface-invocation-binding-v1.md)
- [PD-TM-04第七设计修正](../../design/tool-engine/pd-tm-04-seventh-candidate.md)
- [Tool/MCP计划](../../plan/tool-mcp/tool-mcp-v1.md)
