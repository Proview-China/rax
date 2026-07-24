# Agent Host 模块说明

## 作用

`ExecutionRuntime/agent-host` 是唯一进程级 Composition Root 的实现位置。它只负责 Host API、依赖注入、进程编排 Journal、Factory exact 选择、构造顺序和反向清理，不拥有 Definition、Plan、Binding、Permit、Verdict、Settlement、领域结果或 Runtime Outcome。

## H1-H2 当前产物

- HostConfig/API/Journal/SystemReady 公共合同；
- Definition、Assembler、Harness Compiler、Runtime Binding/Readiness 的 Host-owned 窄反转接口；
- exact `ComponentID + ArtifactDigest + Contract + Capability` Factory Registry；
- sealed Registry、稳定拓扑、构造进度、重附着与 reverse-DAG cleanup；
- Host `Validate/Assemble/Start/Inspect/Stop` 生命周期骨架；
- typed nil、摘要漂移、别名/重复、DAG环、CAS/lost reply、部分构造、并发 Start 与严格 import boundary 测试。
- Host Journal 每次迁移要求宿主时钟严格高于上一持久水位；时钟回拨或同 tick 均以 `clock_regression` Fail Closed，不合成伪时间。
- Runtime Binding 和组件 Construction 均先持久化 deterministic Attempt，再调用 canonical start-or-inspect；lost reply 只能 Inspect 原 Attempt，无法证明结果时持久化 Unknown。
- Ready 只在 Journal phase 为 Ready 时返回，并重新做 fresh clock 校验及 exact ReadyRef 校验；Closed/Draining/Reconciling/Indeterminate 只返回 Journal。
- 生命周期锁按 `HostID + StartID` 分片：同一 Start 串行，不同 Start 可并行；任何 planned/unknown attempt 都禁止 Stop/Reconcile 误收口为 Closed。
- Journal successor 强制 Binding 首次 planned、Construction 单尾 planned append；直写 outcome 和批量 append 均 Fail Closed。
- Factory 有效 handle/ref 在 constructed progress 前先进入 cleanup 集；progress 持久失败必须回传组合错误，commit-then-panic/CAS lost reply 立即 Inspect exact desired。
- Decoder/Assembler/Compiler/Binding/Readiness/Journal/Clock 的 panic 均封装为 closed typed error；Binding start panic 仍只 Inspect 原 attempt。Ready fresh clock 不得落后于 Journal Updated watermark。

## 当前边界

H3第一纵切及H4 Assembly Publication子切面已完成；H4其余Runtime/Application接线、全6+1 production factories、CLI/API transport仍未落地。live HostV1又缺少control-adapter构造之后的显式Activation与Generation Association阶段，因此不能产生生产 `SystemReady`。

H4 Assembly Publication已经具备两层真实产物：Harness Owner提供SQLite WAL durable `OwnerStoreV2`，以单事务提交publication marker与scope current；Host新增additive `CompiledAssemblyArtifactsV2`和`AssemblyPublicationAdapterV2`，保留同一次H3 compile的四个sealed对象，并把Generation/Manifest/Graph/Handoff/commit分别写入HostJournalV2 fixed attempt。lost reply或重启只能Inspect原staged/historical/current，不从refs重建对象。该子切面通过ordinary100/race20/full ordinary/race/vet，但尚未接入HostV2 production root。

## H3 第一纵切完成

`ExecutionRuntime/agent-host/owneradapter` 已提供Definition、Agent Assembler、Harness public Owner adapters，并保持 `HostConfigV1` 冻结。新增Source-current与ResolutionInputs-current窄读Port，所有exact artifacts由Owner reader重建。public struct测试声明已证明Definition store/current -> Resolver/Snapshots/Plan repository -> mapper -> real Harness compiler -> Host ConstructionGraph真实链；这些测试声明不是production release证据。这里的“完成”严格限于H3第一纵切，Runtime Binding/Activation/Readiness所属H4/H5仍为NO-GO。
