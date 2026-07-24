# SurfaceInvocationBinding V1 Tool合同候选冻结

时间：2026-07-16 21:00 Asia/Shanghai

## 结论

用户已接受三Owner Surface桥方向。Tool Owner已把`SurfaceInvocationBinding V1`推进为可联合冻结的资产合同；未写Go、未修改Model/Harness/Application/Runtime。官方跨Owner门仍为NO（P0=2/P1=1/P2=0），Tool Go与PD-TM-04 P4继续hard-block。

## Tool Owner冻结内容

- Tool-owned neutral Prepared/Assembly exact coordinates，不冒充Model/Harness live nominal；
- canonical Binding完整覆盖Prepared、SurfaceCurrent、Assembly ToolSurface/ExpectedInjectionManifest、Expected==Actual Injection、Profile/RegistrySnapshot、Generation/Handoff/BindingSet watermarks、Created/NotAfter；
- stable ID只由完整Invocation coordinate派生；same canonical幂等，同Invocation任一其他字段漂移Conflict；
- 唯一Repository实例以一个线性化事务维护Binding ID和Invocation coordinate两个唯一索引；窄Writer、Reader、Ack分权，禁止第二仓；
- lost reply只按Invocation Inspect winner；TTL、clock rollback、Unavailable/Indeterminate/NotFound与exact drift全部Fail Closed；
- terminal Model Projection只提供Invocation coordinate，不能证明pre-dispatch injection；后续S1必须Inspect Binding并复读Prepared/Surface/Registry/InputContract/CandidateV3/BindingV2链；
- 新增`SIB-V1-*` unit/idempotency/conflict/fault/read/race/conformance/boundary/import矩阵；原26反例与schema 12项保持原编号和原义。

## 剩余联合Delta

- Model：最终public Prepared Ref/Projection/Reader与PreDispatchGate；
- Harness/宿主：所有Provider路径首次调用前统一Gate、Assembly exact refs/watermarks到Tool neutral coordinates的mapping与Tool Ack接线；Harness不创建Tool Fact；
- Application：不改；
- 三Owner逐字段mapping、独立终审P0/P1/P2归零及用户P4 Go授权。

## 入口

- [Tool SurfaceInvocationBinding V1](../../design/tool-engine/surface-invocation-binding-v1.md)
- [PD-TM-04第七设计修正候选](../../design/tool-engine/pd-tm-04-seventh-candidate.md)
- [Port Delta](../../design/tool-engine/port-delta.md)
- [实施计划](../../plan/tool-mcp/tool-mcp-v1.md)
