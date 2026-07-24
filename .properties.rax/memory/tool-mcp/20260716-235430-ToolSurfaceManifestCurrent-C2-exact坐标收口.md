# ToolSurfaceManifestCurrent C2 exact坐标收口

## 事件

- 中央M2复审要求C2 public Ref必须直接复用Manifest/Plan ToolSurface `ID/Revision/Digest`，不得以ProjectionDigest作为lookup坐标；
- current stable ID固定为Manifest.ID，删除Owner/prefix派生和第二lineage；同Manifest ID跨Owner必须Conflict；
- Projection返回独立`ProjectionDigest`，Manifest digest错误与Projection digest错误分别Fail Closed；
- 公共Reader只含`InspectExactToolSurfaceManifestCurrentV1`，Repository嵌Reader再加`EnsureExactToolSurfaceManifestCurrentV1`；
- Harness M2只import Tool contract、构造器只接Reader，所有M2读取路径EnsureCalls=0；
- 51项矩阵补齐Plan ObjectRef golden、跨Owner冲突、method-set、typed-nil/cancel、tamper、TTL、deep-clone与zero-Ensure反例。
- Runtime ports唯一Assembly Current/RegistrySnapshot public Go nominal与Reader已落盘；C2按live状态消费。
- context取消保留`context.Canceled/context.DeadlineExceeded`标准cause并由Tool私有边界分类，不伪造runtime/core错误类别；测试fixture落在`internal/testkit`。
- C2 create只允许revision 1；successor必须携`ExpectedCurrent` full Ref并对current+1做CAS。history hit只有winner仍为current才可幂等返回，rev2 current后重投rev1或ABA均不得回退current。

## 当前门

- 本轮只同步Tool自有design/plan/module/memory资产，未写Go；
- 等待C2独立设计复审，PD-TM-04 P4、Harness M2、system和production继续NO-GO。
