# Application与Memory/Knowledge跨Owner Refresh闭环

时间：2026-07-18 00:08（Asia/Shanghai）

live Application已发布`ContextTurnRefreshPortV1`、`ContextOwnerSourceReaderV1`及协调DTO；`ExecutionRuntime/context-engine/applicationadapter`已实现三段Prepare/Apply/Inspect映射，只依赖Application公共`contract/ports`。Memory与Knowledge各自唯一public V2 Reader通过Owner Adapter返回S1/S2 exact current projection和正文Observation，Context没有定义第二套Owner nominal，也不依赖其concrete Store/internal。

当前首切面为一个settled Tool action，Source基数`Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`。Memory/Knowledge分别映射为DynamicTail受限`memory_recall`/`knowledge_reference`；Context Adapter在正文读取前执行每Owner64KiB聚合预检，S2复读stable association与exact content。Owner Reader的Unknown/Unavailable/cancel/deadline原样返回，不能降格为Conflict；任何失败均保持Generation current不变。

B-cross test-only fixture已把Application协调器、Context Adapter、Memory V2与Knowledge V2 public adapters手工注入，并验证最终Frame exact包含ToolResult、MemoryRecall与KnowledgeReference。定向普通100轮与race20轮PASS；S2 drift、lost reply、proof/settlement association漂移、64并发、Unknown/Unavailable/cancel及超限零读取/零current发布均覆盖。该完成不等于production State Plane/root，不注册Capability，不调用Harness Continuation，不推进Turn；C层继续NO-GO。

本轮full ordinary/race/vet、gofmt、Markdown links、draw.io XML、diff-check与import-boundary均PASS；Go闭集hash=`e37aa9059b78916fba0c98c1e70e50758ff8bbdc1d06e14144fa4ae8900bd7bb`。
