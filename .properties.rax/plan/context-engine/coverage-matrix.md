# Context&Cache业务终点覆盖矩阵

状态时间：2026-07-23。业务事实源：[Context&Cache定稿文档](../../../tmp.document/Context&Cache.md)。本矩阵只陈述live证据，不把Owner-local/B-cross/reference-only完成扩大为production完成。

## 1. 业务要求与live证据

| 业务要求 | 当前证据 | 裁决 | 下一闭环 |
|---|---|---|---|
| Context Engineering开发面 | 六个Offline入口、PromptAsset Service、Engineering SDK/API/CLI与Prompt Provenance seal/verify均完成；七家官方Prompt/T3Code边界有exact审计证据；target100/race20及full ordinary/race/vet通过 | Owner-local开发面与Provenance离线核完成 | 具体Prompt pre-release候选、Model Invoker exact Profile ref/current reader、远程Evaluator与production发布仍需独立治理 |
| Context Source/Fragment合同 | typed Candidate/Fragment合同已完成；Memory/Knowledge唯一public V2 Reader已通过Application中立Port进入B-cross Frame | Owner-local+B-cross部分完成 | Prompt/Profile/Human/MCP/Continuity/Sandbox/Skill/Index仍需各Owner唯一公共Reader；不得复制nominal |
| Candidate Admission | `kernel/compiler.go`已有稳定排序、required/optional、trust、currentness、dedupe、预算、Residual与Manifest reason | Owner-local完成 | 真实source currentness依赖各Owner公共Reader与Application装配 |
| Recipe→Manifest→Frame | `contract/recipe.go`、`frame.go`、`kernel/compiler.go`与staged compile/inspect已完成确定性冻结、内容寻址和回放检查 | Owner-local核心完成 | production Frame binding是否直接携CachePlan/ExpectedInjection，或由Assembly binding对象关联，尚需联合合同冻结 |
| Stable/SemiStable/Dynamic布局 | compiler与refresh保持Stable/SemiStable exact ContentRef复用，只变Dynamic Tail；prefix digest/cache identity在Refresh合同中exact | Owner-local完成 | Harness/Model实际物化仍无公共exact Frame bridge |
| Provider-neutral Cache规划 | `contract/cache.go`已有Profile/Partition/Plan/Entry、TTL、invalidation、经济性；第六Offline入口只读Inspect Plan/Profile | 离线核完成 | Provider Profile公共current projection、真实cache create/read/write/invalidate均未接；usage不等于hit |
| Artifact Anchor/Delta | `contract/artifact.go`已有Anchor/Delta与unchanged/delta/rematerialize、RetainedAnchor exact检查 | 纯模型完成 | CTX-D05 Artifact Owner exact-current Snapshot/Delta Resolver未发布；Context不得自行读文件或把调用方字符串冒充current |
| Compaction/Generation | `contract/compaction*.go`、`kernel/compaction*.go`、同backend Store完成Prepare→pending不可见→S2→atomic current CAS→Inspect | Owner-local完成 | Continuity Event/Checkpoint/Fork/Rewind projection仍等待公共Reader/root |
| Outcome/Evaluation/Feedback | `contract/outcome.go`、`kernel/outcome.go`、`outcomestore`完成不可变exact事实链与Put-once/Inspect；Owner-local Engineering API/CLI已完成full gate | Owner-local事实链与开发入口完成 | 真实Model/Tool/User/Task owner refs由协调层提供；远程评测是Effect；production ingress仍未冻结 |
| Recipe反馈与pre-release | Recipe immutable + `draft→validated→evaluated→review_pending|rejected` head CAS已完成 | Owner-local pre-release完成 | CTX-D07未裁决，production publish/rollback/revoke明确unsupported |
| Prompt资产与反馈式Prompt管理 | PromptAsset直接持有规范化片段规格与exact ContentRef；immutable Put/exact Inspect、distinct AssetRef、Candidate projection、pre-release lifecycle-head CAS已完成，target100/race20/full ordinary/race/vet通过 | Owner-local完成 | developer ingress未冻结；production发布仍等待CTX-D07 |
| Expected/Actual Injection | `contract/injection.go`、`kernel/injection.go`完成Expected、Provider Observation、Harness Actual、Conformance分层与时间/Route/Attempt/Frame/sequence/fidelity反例 | 离线合同完成 | 不等于Tool Surface；Model/Harness公共Adapter与actual-point materialization未闭 |
| per-turn N=1 Refresh | CTX-D10、CTX-D09-R1、Application三段Port/Context Adapter及Memory/Knowledge B-cross fixture均已完成；Tool=1、Memory/Knowledge各<=1、Continuity=0 | A/B-cross完成 | production composition、Capability、Harness Continuation/Turn仍NO-GO |
| Review Context投影 | Context已实现Review公共Publisher/Current Reader；Memory reference store与SQLite WAL单节点durable repository均覆盖exact history/current CAS、重启与lost-reply Inspect | 单节点production-shaped参考能力完成 | 不声明多节点HA、备份、远程持久性、宿主root或SLA |
| Restore Context物化 | Context Owner materialization kernel/store及Application/Runtime公共只读Adapter已完成；exact source/current requirements、create-once、lost-reply Inspect和并发CAS均有测试 | reference闭环完成 | 不负责Runtime Activation、Provider执行或production trusted root |
| Component Release | Agent Assembler公共合同上的`reference_only` builder/readiness/publisher已完成；未知Ensure回包只Inspect exact ref | assembly candidate完成 | durable state/cache、Provider current、Harness injection/continuation、cleanup与deployment proof缺失，production promotion不存在 |
| Continuity联动 | Continuity `TimelineOwnerFactRefV1`与Checkpoint V2已有exact Context refs | 引用形状完成 | typed Owner-current Reader/Projection/Router仍在`continuity/runtimeadapter`，不是公共Port；CTX-D06仅保留这一Delta |
| 不同Harness诚实差异 | Context Expected/Actual/Fidelity合同已完成 | 离线合同完成 | Harness `ModelTurnCandidateV2`只有ContextRef+Digest，Continuation无new exact FrameRef；不能宣称已接线 |

## 2. live公共合同阻塞

| Owner | live证据 | 唯一需要的additive Delta | Context侧行为 |
|---|---|---|---|
| Application | 已发布`ContextTurnRefreshPortV1`、`ContextOwnerSourceReaderV1`及协调DTO；Context Adapter/B-cross fixture已通过 | 已闭A/B-cross；production composition仍需宿主接线 | Context Adapter只依赖公共contract/ports；Application不import Context实现 |
| Harness | `ModelTurnCandidateV2`只有`ContextRef string + ContextDigest`；`ContinuationRefV2`只绑定Pending/Settlement/Evidence | 发布namespaced exact Frame/Generation/Manifest continuation binding与materialization/currentness合同 | 不依赖Harness私有ContextPort，不构造或推进Continuation |
| Model Invoker | `union.ContextReference`只有ID/Kind/Reference/Snapshot/Access/Visibility/Required | 发布revision/digest/expiry/current projection及Actual Injection公共Observation/Reader；Route仍走RouteID+public execution union | 不导入internal或厂商SDK；不可物化Route Fail Closed/Residual |
| Continuity | typed Owner-current Reader/Projection/Router仍定义于`continuity/runtimeadapter` | 提升到`continuity/ports`或提供唯一无损public facade | 只返回Context exact-current投影；不写Event/Evidence/Timeline/RocksDB |
| Artifact/Sandbox/Tool | 无统一Artifact Snapshot/Delta public Resolver | CTX-D05 exact owner/version/digest/range current Reader与bounded Snapshot/Delta/Unchanged projection | 不自行读路径、不缓存快照冒充current |
| Review/Runtime | 非Run Recipe/Prompt发布Review subject未冻结 | CTX-D07由Review/Runtime Owner裁决admin/custom发布Review与Operation关联 | production动作保持unsupported；不扩现有V4或复用上游Settlement |

## 3. 不能被局部测试替代的完成证据

- Owner-local ordinary/race/vet只能证明Context内部算法、合同和参考Store，不能证明production root、Provider、Harness或Continuity接线；
- G6B A/B-cross已经看到Application公共Port与test-only cross-module fixture；完整production完成仍必须看到composition root、Capability启用门、Harness continuation exact new Frame与Turn推进全链证据；
- ContextFrame发送成功、Provider usage/cached tokens、Receipt或Observation均不能单独证明实际注入一致或cache hit；
- Prompt发布、远程评测、Provider Cache操作和远程Source必须出现对应Runtime Operation治理与Owner Settlement，不能由Offline SDK结果替代；
- 只有本矩阵所有“未完成/部分完成”项有直接live证据且全套门通过后，`Context&Cache.md`业务终点才可标记完成。
