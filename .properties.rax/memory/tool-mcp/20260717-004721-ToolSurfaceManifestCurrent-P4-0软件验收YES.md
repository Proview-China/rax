# ToolSurfaceManifestCurrent P4-0软件验收YES

## 事件

- Tool Owner在`ExecutionRuntime/tool-mcp`内完成C2 P4-0：`ToolSurfaceManifestCurrent*V1`公共合同、唯一内存Repository、测试fixture与unit/whitebox/blackbox/fault/conformance测试；
- public Ref无损复用Manifest/Plan ToolSurface `ID/Revision/Digest`，`ProjectionDigest`保持独立；
- 唯一Repository原子维护history/current，闭合revision 1 create、successor full Ref/current+1 CAS、history winner current门、lost-reply恢复、ABA/回退拒绝、deep clone、TTL/clock/cancel与per-ID并发；
- 53项关键测试ID唯一，lost-reply、ABA、full CAS及64并发反例通过；
- 实际验证通过：targeted ordinary×100、targeted race×20、full ordinary、full race、`go vet ./...`、gofmt、import/zero-network和diff/trailing检查。

## 当前门

- P4-0状态：`implementation_software_test_yes`；该YES仅代表Tool Owner纯软件实现与测试验收；
- P4-1+继续锁定，不实施SurfaceInvocationBinding、InputContract、ActionCandidateV3、BindingV2或完整Action Adapter；
- Harness M2、跨Owner actual-point接线、system、production root/backend、能力启用与SLA仍为NO-GO；
- 本事件只同步Tool自有module/memory，未继续修改Go，未stage/commit。
