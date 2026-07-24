# ToolSurfaceManifestCurrent V1测试矩阵

状态：C2 Repository P0 slice测试合同已冻结，Go未实施。主合同见[ToolSurfaceManifestCurrent V1](tool-surface-manifest-current-v1.md)。

| ID | 级别 | 场景 | 强制Oracle |
|---|---|---|---|
| C2-001 | P0 | valid Manifest首次Ensure | history=1、current=1、完整Projection返回 |
| C2-002 | P0 | same canonical重投且winner仍为current | 同Ref/Checked/Expires/Digest，write仍1 |
| C2-003 | P0 | lost Ensure reply后同Request重试 | Inspect winner并返回，零第二写 |
| C2-004 | P0 | same ID/revision换Manifest字段 | conflict，原winner不变 |
| C2-005 | P0 | Manifest digest tamper | zero write |
| C2-006 | P0 | wrong ExpectedInjectionDigest | zero write |
| C2-007 | P0 | Entry顺序/ModelName/EffectKinds tamper | zero write |
| C2-008 | P0 | Owner、revision、expiry重复字段漂移 | conflict/invalid，zero write |
| C2-009 | P0 | wrong Manifest digest或Ref.Digest != Manifest.Digest | Ref lookup/Validate conflict，zero Ensure |
| C2-010 | P0 | current index Ref与expected Ref漂移 | conflict，zero Ensure |
| C2-011 | P0 | expired Manifest/current | precondition_failed，zero write |
| C2-012 | P0 | clock rollback | precondition_failed，zero write |
| C2-013 | P0 | TTL在lock/CAS前穿越 | zero commit |
| C2-014 | P0 | nil context | invalid_argument，零Reader/lock/write |
| C2-015 | P0 | pre-canceled context | preserve `context.Canceled` cause，零Reader/lock/write；不伪造core category |
| C2-016 | P0 | post-lock cancel/deadline | preserve标准context cause，zero commit |
| C2-017 | P0 | constructor typed-nil clock/store | error，调用不panic |
| C2-018 | P0 | Reader typed-nil注入Harness | constructor拒绝，zero Ensure |
| C2-019 | P0 | Harness M2正常读取 | InspectExact=1，EnsureCalls=0 |
| C2-020 | P0 | Harness M2 NotFound/Unavailable/Indeterminate | 分类原样返回，EnsureCalls=0 |
| C2-021 | P0 | Harness拿Repository实例但静态Reader | method-set无Ensure；EnsureCalls=0 |
| C2-022 | P0 | InspectExact exact hit | fresh full clone；零写/零Ensure |
| C2-023 | P0 | InspectExact unknown ref | not_found；零写/零Ensure |
| C2-024 | P0 | same Manifest ID/revision换Manifest digest的ABA | conflict；零写/零Ensure |
| C2-025 | P0 | deep-clone Request后篡改源Manifest | persisted winner不变 |
| C2-026 | P0 | deep-clone return后篡改Entries/EffectKinds/Residuals | 后续Inspect不变 |
| C2-027 | P0 | RegistrySnapshotDigest被调用方当latest selector | 合同无该Reader/字段；compile/import反例 |
| C2-028 | P0 | C2 Projection增加Registry/Assembly/Prepared echo，或M2构造器/DTO/canonical/Reader集合引入Prepared | schema/conformance失败；M2闭集仅A2+B1+Handoff+C2+Registry |
| C2-029 | P0 | Inspect路径内部调用Ensure | EnsureCalls必须0，测试失败 |
| C2-030 | P0 | Reader method名或签名漂移 | compile-time interface assertion失败 |
| C2-031 | P0 | Repository未嵌Reader | compile-time method-set assertion失败 |
| C2-032 | P0 | Ensure允许nil/empty nested slice canonical漂移 | same canonical或invalid，不产生第二摘要 |
| C2-033 | P0 | 64 goroutine同canonical Ensure | 单winner、单history、单current |
| C2-034 | P0 | 64 goroutine同ID换内容 | 单winner，其余conflict |
| C2-035 | P0 | 64 goroutine不同stable ID | 可并行，不被全局锁串行 |
| C2-036 | P0 | lost reply并发64重试 | 单winner，全部同Ref |
| C2-037 | P0 | revision+1携full exact ExpectedCurrent合法CAS | 新current，旧history immutable |
| C2-038 | P1 | revision跳跃/倒退 | conflict，current不变 |
| C2-039 | P1 | Unavailable/Indeterminate被当NotFound | zero create |
| C2-040 | P1 | zero fresh clock | invalid/precondition，zero write |
| C2-041 | P1 | Expires != Manifest.Expires | reject |
| C2-042 | P1 | Ref revision != Manifest revision | reject |
| C2-043 | P1 | wrong ProjectionDigest但Manifest Ref正确 | Projection Validate拒绝；Reader不得据此改查另一对象 |
| C2-044 | P1 | Manifest digest与Projection digest混用、相等断言或互猜 | reject；Ref仍只按Manifest exact坐标查询 |
| C2-045 | P1 | Harness import Tool实现/surface/store | import-boundary失败 |
| C2-046 | P1 | Harness构造器收Repository而非Reader | compile/review失败 |
| C2-047 | P1 | Repository调用网络/生产backend | zero-network/static scan失败 |
| C2-048 | P1 | Inspect返回内部slice别名 | race/deep-clone失败 |
| C2-049 | P0 | Plan.ToolSurface ObjectRef→Current Ref golden | ID/Revision/Digest逐字段无损相等，可直接InspectExact |
| C2-050 | P0 | same Manifest.ID跨Owner Ensure | conflict，history/current仍为原winner |
| C2-051 | P0 | same Manifest.ID但尝试派生prefix/第二lineage | schema/canonical失败，zero write |
| C2-052 | P0 | rev2已成为current后重投rev1 | conflict/precondition；current不回退，绝不返回rev1 |
| C2-053 | P0 | successor ExpectedCurrent same ID换revision/digest、ABA或CAS间漂移 | CAS失败、zero successor commit、current不变 |

验证门：

```bash
go test ./contract ./surface -run 'ToolSurfaceManifestCurrent' -count=100
go test -race ./contract ./surface -run 'ToolSurfaceManifestCurrent' -count=20
go test ./... -count=1
go test -race ./...
go vet ./...
gofmt -w <本slice新增Go文件>
git diff --check -- ExecutionRuntime/tool-mcp .properties.rax/design/tool-engine .properties.rax/plan/tool-mcp
```

这些是未来实现门；本轮资产冻结未运行Go命令。
