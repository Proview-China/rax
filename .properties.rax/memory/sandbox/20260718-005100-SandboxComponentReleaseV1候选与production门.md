# Sandbox ComponentReleaseV1候选与production门

时间：2026-07-18 00:51（Asia/Shanghai）

Sandbox新增`ExecutionRuntime/sandbox/release`，直接发布Agent Assembler公共
`ComponentReleaseV1`，完整携带Manifest、Capability、Module、Port、Host adapter Factory、
Effect/Settlement/Cleanup Owner、artifact、TTL、evidence与certification闭包。

production资格不接受bool或fixture自报。新增readiness projection逐项绑定持久Fact/Current
Store、Lease/Policy/Sandbox current、Enforcement、Evidence、Settlement、Provider transport/
inspect、journal、cleanup、deployment attestation与独立Certification Fact；Publisher在Catalog
写入前S1/S2复读。缺readiness只发布`standalone`，Unavailable/漂移/过期均Fail Closed；
promotion需要更高release revision。

当前仓库仍没有宿主持久State Plane proof、deployment attestation与Host factory/transport注册，
所以Sandbox的production release继续NO-GO。testkit/memory只用于合同、lost-reply、TTL、并发和
Conformance测试，不能进入production Catalog。
