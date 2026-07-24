# G6B跨Owner Turn映射与唯一Reader校准

时间：2026-07-16 21:36（Asia/Shanghai）

Memory/Knowledge live已各自发布public contextsource V1 Reader/DTO：`MemoryContextSourceCurrentReaderV1`与`KnowledgeContextSourceCurrentReaderV1`。Context不创建第二套平行Owner nominal/DTO；未来需求只能由对应Owner的additive V2或唯一无损public facade承载。当前Offline SDK/G6B仍固定`MemorySources=0 / KnowledgeSources=0 / ContinuitySources=0`，两Reader调用数为0；`knowledge_reference`仍是Context Owner单独候选，未冻结。

G6B Turn语义冻结为：Source Turn exact ref只来自具名Session/Turn Owner Current Reader，其`Ordinal=T`必须exact等于settled Tool Execution和ExpectedCurrent的`uint32` Turn；Target T+1只由Context `childExecution`生成并归属child Frame/Generation。Transition proof唯一Owner是Context，Application只编排不mint；Memory/Knowledge不自行`+1`、不补造Session/Turn、不产生proof。

唯一可见时序为`pending Frame/Generation seal → Context proof → S2 fresh reread → atomic ApplySettlement+expected Generation current CAS → publish`。Proof stable closure排除phase/time/self-digest，fresh observation包含phase、Checked/Expires与Owner-current projection digests；S2漂移后proof不授予publish资格。

当前truth不变：Context Owner-local Refresh/Apply/Inspect/CAS与本组件fixture已完成、二审YES；Application公共Context Refresh Port、Memory/Knowledge跨Owner source mapping、G6B跨模块fixture、production root、Capability、Harness Continuation与Turn推进仍NO-GO。本次只修订Context design/plan/memory，不写Go。
