# Context Engine Plan入口

状态：规划内无需production composition root的Owner-local/B-cross/reference-only切片均已完成软件验证：`CTX-D09-R1`、`CTX-D10`、Application Adapter、Memory/Knowledge B-cross、Offline/Engineering SDK与API/CLI、Compaction/Generation、Outcome、Recipe/PromptAsset pre-release、Prompt Provenance、durable Reviewer Context、Restore Context materialization及Component Release候选。target100、race20、full ordinary/race/vet通过。production Recipe/Prompt发布仍等待CTX-D07；Model Profile current绑定、production State Plane/Cache、Capability、Harness Continuation与Turn推进未启用。

- [Context Engineering与Cache v1实施计划](./context-engine-v1.md)
- [测试矩阵](./test-matrix.md)
- [Context&Cache业务终点覆盖矩阵](./coverage-matrix.md)
- [per-turn Refresh最小接线](../../design/context-engine/integration.md)
- [CTX-D10 ParentFrame Reader与G6A/G6B Port Delta](../../design/context-engine/port-delta.md)
- [per-turn状态机与Unknown恢复](../../design/context-engine/state-machines.md)
- [N=1 Refresh验收合同](../../design/context-engine/acceptance/README.md)
- [Context Owner-local Offline SDK V1](../../design/context-engine/sdk-api.md)
- [Context Engineering SDK V1](../../design/context-engine/engineering-sdk.md)
- [模型专属 Coding Agent Prompt 上游审计](../../design/context-engine/prompt-upstream-audit.md)
- [Prompt Upstream Provenance V1](../../design/context-engine/prompt-provenance.md)
- [多模型预埋 Prompt 候选架构](../../design/context-engine/prompt-family-candidates.md)
- [OperationScope Evidence V3参考](../runtime/operation-scope-evidence-v3.md)
- [ComponentReleaseV1 Delta](../../design/context-engine/component-release-v1.md)：已实现reference-only owner publisher/readiness；production state/cache/provider/injection/continuation/cleanup/root仍NO-GO。

设计事实源：[Context Engine设计入口](../../design/context-engine/README.md)。`CTX-D09-R1`已按零Runtime Settlement完成Context Owner原子ApplySettlement+expected Generation current CAS；本组件A/B-local与Offline SDK独立软件验收均已完成。跨模块G6B与production composition不在本组件实现范围。
