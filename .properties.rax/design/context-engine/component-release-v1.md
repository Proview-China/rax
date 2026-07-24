# Context Engine ComponentReleaseV1 Delta

状态：**reference-only assembly candidate / production NO-GO**。

## 本轮闭合

- 使用Agent Assembler与Harness/Runtime公共类型发布声明式`ComponentReleaseV1`，不复制公共DTO。
- Manifest、Module、Capability、Port、Factory descriptor、effect/settlement/cleanup owners、artifact、candidate certification、evidence与TTL精确闭合。
- owner-local publisher的Ensure未知回包只按同一exact ref Inspect，不重派mutation；任何返回漂移失败关闭。
- Factory仅descriptor；不导入Host、Assembler repository、Harness实现或其他Owner实现。

## production门禁

当前真实能力包括Offline SDK、Frame/Manifest/Generation exact current Reader、N=1 owner-local Refresh、Application公共Adapter和Memory/Knowledge test-only B-cross。`refreshstore`、cache/ref/outcome/release/prompt stores仍是进程内reference；没有production durable state/cache、真实Provider Cache current、Harness per-turn Injection/Continuation、Turn推进、cleanup或deployment root。

production必须从同一current/certification cut获得durable state/cache、source/provider current、injection/refresh/continuation、cleanup和deployment attestations。当前缺口不能由offline conformance、memory store、test fixture或Application Adapter存在性替代，因此没有production promotion API。

唯一后续Delta是production State Plane/Cache与Source/Provider current、Harness per-turn injection/continuation、cleanup及composition/deployment qualification。
