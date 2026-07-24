# ToolSurfaceManifestCurrent V1 Repository P0实施计划

状态：**C2 design/test matrix已冻结，Go未实施；不解锁完整PD-TM-04 P4、Harness M2、system或production。**

设计：[ToolSurfaceManifestCurrent V1](../../design/tool-engine/tool-surface-manifest-current-v1.md)
测试：[测试矩阵](../../design/tool-engine/tool-surface-manifest-current-v1-test-matrix.md)

## 1. 目标产物

1. Tool-owned完整Manifest Current公共contract；
2. 一个concrete Repository嵌入只读Reader并实现`EnsureExact`；Reader方法固定为`InspectExactToolSurfaceManifestCurrentV1`；
3. Harness只消费的窄Reader接口；
4. 内存/本地test fixture，不包含production backend/root；
5. unit、whitebox、blackbox、fault、conformance、race和vet门。

## 2. 文件级落点

| 阶段 | 候选文件 | 内容 |
|---|---|---|
| P0.1 contract | `ExecutionRuntime/tool-mcp/contract/surface_manifest_current_v1.go` | Ref/EnsureRequest/Projection/Reader、Validate/Seal/canonical/clone |
| P0.2 repository | `ExecutionRuntime/tool-mcp/surface/manifest_current_repository_v1.go` | 唯一history/current索引、per-key gate、EnsureExact/InspectExact |
| P0.3 fixture | `ExecutionRuntime/tool-mcp/surface/manifest_current_repository_v1_test.go` | unit/whitebox、typed-nil、post-lock、deep-copy、64并发 |
| P0.4 blackbox | `ExecutionRuntime/tool-mcp/tests/blackbox/tool_surface_manifest_current_v1_test.go` | 仅公共Reader/Repository行为 |
| P0.5 fault | `ExecutionRuntime/tool-mcp/tests/fault/tool_surface_manifest_current_v1_test.go` | lost reply、Unknown、clock/TTL crossing |
| P0.6 conformance | `ExecutionRuntime/tool-mcp/tests/conformance/tool_surface_manifest_current_v1_test.go` | Harness窄Reader、import、zero-network、scope guard |
| P0.7 testkit | `ExecutionRuntime/tool-mcp/internal/testkit/tool_surface_manifest_current_v1.go` | 仅测试Manifest/clock与Harness Reader计数，不宣称生产SLA |

禁止创建Application/Harness/Runtime/Model Adapter、Provider seam、production root、network transport或第二Repository。

## 3. 实施顺序

- [x] 中央裁决C采用Tool-owned完整`ToolSurfaceManifestCurrent`；
- [x] 冻结Owner/非Owner、完整字段、canonical、ID/revision/digest；
- [x] 冻结唯一Repository与Harness窄Reader方法集；
- [x] 冻结Manifest canonical/ExpectedInjection重算；Registry exact权威仍由M2独立公共Reader闭合，不回显进Projection；
- [x] 冻结S1/S2、ExpectedCurrent full Ref、current+1 CAS、history-hit current门、lost reply、deep clone、ctx/clock和错误闭集；
- [x] 冻结53项测试矩阵及ordinary100/race20/full/race/vet门；
- [x] Reader只含`InspectExactToolSurfaceManifestCurrentV1`；Repository嵌Reader再加Ensure；Harness M2只接Reader且EnsureCalls=0；M2闭集为A2+B1+Handoff+C2+Registry，Prepared Historical/Current留给未来M3 Gate；
- [x] current ID直接等于Manifest.ID；Ref ID/Revision/Digest无损等于Manifest/Plan ToolSurface；ProjectionDigest独立；
- [ ] 独立设计审计P0/P1/P2归零；
- [x] Runtime ports唯一Assembly Current/RegistrySnapshot public Go nominal与Reader已落盘；
- [ ] Harness M2只读注入与C2独立设计复审允许实现；
- [ ] 获得本slice Tool Go实施授权；
- [ ] 实现contract与唯一Repository；
- [ ] 实现`internal/testkit`与unit/whitebox；
- [ ] 实现blackbox/fault/conformance；
- [ ] targeted ordinary100与race20全绿；
- [ ] full ordinary/race/vet、gofmt/import/diff全绿；
- [ ] 独立代码审计P0/P1/P2归零；
- [ ] module/memory同步实现真值。

## 4. 实施不变量

1. 生产contract不import Harness/Application/Model或Runtime实现包。
2. Repository外部Reader调用不持全局锁；相同ID用per-key gate，不同ID并行。
3. same Request仅在winner仍为current时返回持久Checked/Expires；history winner非current不得返回或回退index。
4. current Ref必须full exact等于Manifest/Plan ToolSurface ID/Revision/Digest；不提供latest/name或ProjectionDigest lookup。
5. Projection不回显Registry；Tool不拥有Registry事实，M2另行读取Registry Owner exact current。
6. UnknownOutcome只读原key；不得重Seal或新revision。
7. Fake只在testkit；无production backend、network、credential或SLA。
8. Provider/Harness ACK/Action/Binding调用计数恒为0。
9. Ensure必须携`ExpectedCurrent` full Ref；revision 1 create要求它为严格零值且current权威NotFound。
10. successor只允许`Manifest.Revision == ExpectedCurrent.Revision+1`并对current full exact Ref执行CAS；跳跃、倒退、same-ID换digest与ABA均零提交。
11. revision 2成为current后重投revision 1必须Conflict/Precondition；不得返回历史rev1或回退current index。

## 5. 验收

设计验收：

- design、plan、test matrix相互链接；
- 53个测试ID唯一；
- P0/P1/P2自检无未解释字段；Manifest digest与ProjectionDigest两类错误、Plan ObjectRef golden与跨Owner Conflict均有独立反例；
- Markdown links、trailing whitespace、stale wording、diff-check通过。

代码验收以测试矩阵第7节命令为准。未实际运行的命令不得标记完成。
