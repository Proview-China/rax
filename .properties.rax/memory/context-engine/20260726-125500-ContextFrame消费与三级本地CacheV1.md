# ContextFrame消费与三级本地Cache V1

- 基于Design PR #6落地严格模型中立的Frame Consumption Descriptor。
- 落地Fragment、Frame与Projection三层进程内参考Cache，具备TTL、create-once、Inspect-only恢复、invalidation-generation CAS与no-ABA。
- 落地非权威StructuralValueEvaluatorV1与CompressionEvidenceV1，压缩许可仍由Context不变量和Owner状态机决定。
- Provider专属序列化、Prefix/KV Cache与命中事实仍归Model Invoker。
- Tool settled result回填尚未接线；等待Tool PR #7公共投影合并后直接消费，不复制Tool合同。
- 软件门：target100、race20、full ordinary/race、vet与diff-check全部通过。
