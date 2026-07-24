# Context Engine兼容与迁移

## 1. 当前live基线

- Runtime legacy `ContextPort.Prepare/Inspect`只支持一次窄准备，没有Run Turn、Frame Generation、Source Admission、Cache事实或Outcome；
- Harness私有`ContextPort`只允许消费已经物化、治理并结算的Snapshot；
- Governed Harness `ModelTurnCandidateV2`已携带精确`ContextRef + ContextDigest`，可作为未来每Turn Frame绑定点；
- Harness Assembly Generation/CompiledGraph、`context.frame` Slot、Context/Turn phases与Generation-Binding Association已是live公共基线；
- Runtime Binding V2、Operation Effect V3、Review V2、Evidence V2和Settlement V3可复用，但不是Context领域Commit；
- Model Invoker已有Expected/Actual Manifest、ContextReference、cache read/write usage与内部Cache facts，但缺少完整跨模块ProviderCacheProfile/Frame治理合同。

## 2. 兼容策略

1. 不扩写或私自解释legacy Runtime `ContextPort`；保留为静态一次Prepare兼容面；
2. 新Context领域合同使用独立SemVer和SchemaRef，通过Application namespaced Step接入；
3. legacy返回只能映射为`restricted_controlled`或更低，不能自动认证per-turn、Cache或Actual Injection能力；
4. Governed Harness继续只携带Frame Ref/Digest，不接收可变Frame指针或隐式Resolver；
5. Provider-specific字段只在Model Invoker Adapter，Context核心只保存Provider-neutral Profile Ref；
6. 旧PromptPack/Context Compact/CachePlan只作为migration input，经显式转换、预览和对比后产生新Recipe/Frame，不能直接标记published。

## 3. 迁移阶段

### M0：合同冻结

联合评审对象、Owner、Port Delta、Schema与状态机；不接线。

### M1：离线编译/预览

旧资产转换为draft Recipe；比较旧输出与新Manifest/Frame，不调用Provider、不写远程Cache。

### M2：影子Frame

真实Run仍用旧路径，新引擎只生成不可消费shadow Frame与差异报告。任何Effect禁止。

### M2.5：per-turn Refresh接线门禁

`CTX-D09-R1`已冻结为零Runtime Settlement，A/B-local、Application公共Port Adapter及Memory/Knowledge B-cross test-only fixture均已完成。本组件fixture验证`settled Tool exact chain + Owner V2 sources → Prepare/pending DomainResult → S2 → atomic ApplySettlement+Generation current CAS`；A/B不得使用V4/additive Runtime settlement/Tool settlement，不调用Harness/推进Turn。C层仍需production composition root、Capability与Harness Continuation/Turn验收。

### M3：Direct API受控切换

只在`fully_controlled` Route启用exact FrameRef/Digest；Actual Manifest可观测并通过Conformance。

### M4：官方Harness受限切换

仅当Harness Capability、Residual和Actual Injection证据满足策略时启用；不可见区域保持restricted/observe-only。

### M5：Cache与反馈发布

ProviderCacheProfile、Effect/Inspect/Settlement、Review发布链完成联合验收后单独开启。

## 4. 回退

回退通过Binding/Current Recipe切换到上一已发布版本，不修改历史Frame、Outcome或Cache Fact。存在Unknown Effect时先Inspect/Settlement；回退Recipe不能代替远程Cache Cleanup。
