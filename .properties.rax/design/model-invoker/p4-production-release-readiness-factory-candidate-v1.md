# Model Invoker P4 Production Release / Readiness / Factory Candidate V1

## 裁决

本Delta只允许Model Invoker Owner产出一个可被Assembler读取的声明式 `ComponentReleaseV1` 候选、一个Owner-owned readiness投影和一个conformance报告。候选固定为 `reference_only`，不得自升production，不得把Provider配置、内存repository、owner-local测试或fixture写成production证据。

## Live公共边界

- Prepared Historical/Current：根包已有exact Ref/Fact/Current Projection、Reader/Repository及Seal/Validate。
- Registry：Prepared链使用Runtime public `RegistrySnapshotExactReaderV1`，但Model P4没有独立部署current readiness投影。
- Route/Profile：Prepared Fact只携带 `RouteDigest/ProfileDigest`，没有Owner exact current Ref/Reader可供release readiness S1/S2复读。
- Provider：RouteGateway可构造Provider，但Provider endpoint/credential/entitlement/deployment没有统一Owner exact current readiness链。
- CommitGate ACK：Model拥有ACK合同，Harness拥有CommitGate实现；当前ACK repository实现不是production durable proof。
- Harness bridge：公共bridge存在，但没有与本Model release exact绑定的deployment/readiness/certification fact。

## 最小候选

- ComponentID: `components/model-invoker`
- Kind: `praxis/model-invoker`
- Capability: `praxis.model-invoker/prepared-invocation-current`
- Factory: `factory/model-invoker/prepared-invocation-current`
- SupportMode: `reference_only`
- Readiness: canonical sealed，`assembly_candidate + production_eligible=false`，Required/Missing P0必须完全一致。

## 真实P0

1. durable Prepared Historical store conformance；
2. durable Prepared Current store conformance；
3. Route exact current projection/reader；
4. Profile exact current projection/reader；
5. Registry deployment current与Prepared RegistrySnapshot exact一致性；
6. Provider endpoint/credential/entitlement/deployment exact current；
7. durable CommitGate ACK repository与lost-reply恢复；
8. Harness CommitGate/bridge deployment exact association；
9. provider dispatch actual-point/no-bypass conformance；
10. cleanup/settlement conformance；
11. deployment root/certification fact。

缺少任一P0都必须Fail Closed，候选仍可用于Assembler结构验证，但不能成为production readiness或Provider调用授权。
