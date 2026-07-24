# 模型调用器设计

## 设计状态

- 模块名称：`model-invoker`
- 当前阶段：P4 assembly candidate已完成Owner-owned release/readiness/conformance reference implementation；固定`reference_only`且`production_eligible=false`
- 最近更新：2026-07-18
- 进入计划阶段：第一、第二阶段于2026-07-10获授权；执行并集Runtime于2026-07-13获授权并完成；Tool Call Projection Delta于2026-07-16获授权并完成reference implementation
- 代码实现：统一 Go内核、十四个 Runtime Provider、波次 A上游基础、B完整协议层、C动态订阅控制面、D云托管、E1全部路线，以及`union/profile/effect/execution`与六条代表Route Adapter均已离线验收，位置为 `ExecutionRuntime/model-invoker/`
- 当前候选：默认16条订阅Route为`implemented_offline + callable=false + blocked_by_host_trust`；只有可信宿主激活后才可调用
- 后续边界：E2、Sidecar、第三方首批名单、真实 API与生产评审仍需新的设计和单独授权
- 当前设计候选：`PreparedModelInvocation / PreDispatch Gate V1`的Owner-local合同及P4声明式候选已落盘；11项production P0仍全部显式缺失，因此只可供Assembler结构验证，不宣称production root、Provider readiness或调用授权存在

## 目标

构建 Praxis 的模型调用核心：先忠实实现每个厂商的原生调用逻辑，再由上层语义映射系统形成 Runtime 可稳定使用的官方统一调用层。

统一层不是把所有厂商压缩成最低公共功能，而是形成所有厂商能力的语义并集，并明确每项能力是原生支持、兼容支持、部分支持还是不支持。

## 已确认的设计决定

1. 采用 Route First：完整身份由 Model Family、Provider、Offering、Deployment、Protocol、Endpoint 与 Credential Profile组成。
2. 厂商直连、Token/Coding Plan、云托管和第三方托管分别建路由，不能只靠更换 Base URL。
3. SDK按路由选择：区分厂商第一方 SDK、协议方 SDK调用兼容端点和社区 SDK；官方没有 Go 方案时允许隔离的非 Go执行器。
4. Go 是调用核心、语义映射、路由注册、能力策略和 Runtime 接口的所有者。
5. TypeScript是首选 Sidecar；Python只为确有官方 SDK独占能力的获批路线启用，任何 SDK类型都不能泄漏到 Go Runtime。
6. OpenAI Responses/Chat Completions、Anthropic Messages 和其他官方协议分别维护能力方言，不把“兼容”理解为完全等价。
7. 订阅计划的允许工具、客户端身份、Key、配额和生产边界必须受控；禁止伪造身份绕过条款。
8. 所有映射必须可解释、可审计、可测试；不允许静默丢弃语义或跨 Offering自动扣费。
9. Rust 不属于本模块当前设计范围，只有经过性能测量确认的计算热点才考虑接入。

## 当前范围

- 文本与多模态模型调用；
- 非流式与流式响应；
- 推理内容；
- 工具调用；
- 结构化输出；
- 服务端会话状态；
- 用量、错误、取消、超时和重试；
- Provider 原生能力与统一语义之间的映射。
- 媒体生成、Embedding/Rerank、Files/Stores、Video/Batch Job与Realtime Session；
- 本地OpenAI-compatible、Ollama、llama.cpp及企业HTTPS自建端点。

## 暂不纳入核心范围

- 微调、评测、模型部署和账号管理；
- 浏览器WebRTC、SIP、ephemeral token签发及厂商全部长尾资源的强类型builder；
- Agent Run Engine、记忆系统和多 Agent 编排；
- Rust 计算层。

这些能力可以复用模型调用器的 Provider 基础设施，但必须作为独立能力域设计，不能塞进一个无限扩张的调用接口。

## 总体流程

```text
Runtime Request
      |
      v
Praxis Semantic API
      |
      v
Capability Router and Smart Mapper
      |
      v
Provider Adapter
      |
      +--> Official Go SDK
      +--> Official TS Sidecar
      +--> Compatible SDK
      `--> Native HTTP
      |
      v
Provider Native API
```

## 设计资产

- [P4 Production Release / Readiness / Factory Candidate V1](./p4-production-release-readiness-factory-candidate-v1.md)
- [P4实施计划与门禁真值](../../plan/model-invoker/p4-production-release-readiness-factory-candidate-v1.md)
- [PreparedModelInvocation / PreDispatch Gate V1](./prepared-model-invocation-pre-dispatch-gate-v1.md)
- [PreparedModelInvocation / PreDispatch Gate V1测试矩阵](./prepared-model-invocation-pre-dispatch-gate-v1-test-matrix.md)
- [PreparedModelInvocation / PreDispatch Gate V1实施计划](../../plan/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1.md)
- [ToolCall Projection发布与Exact Reader V1设计Delta](./tool-call-observation-projection-publish-reader-v1.md)
- [ToolCall Projection发布与Exact Reader V1测试矩阵](./tool-call-observation-projection-publish-reader-v1-test-matrix.md)
- [Tool Call候选观测与公共投影v1](./tool-call-candidate-observation-v1.md)
- [全上游统一原语层Code Review（2026-07-14）](./upstream-primitive-code-review-20260714.md)
- [外围上游能力官方研究（2026-07-14）](./upstream-peripheral-capability-research-20260714.md)
- [外围能力并集与本地上游设计v1](./peripheral-union-and-local-upstream-v1.md)
- [外围能力实施计划v1](../../plan/model-invoker/peripheral-union-and-local-upstream-v1.md)
- [外围能力并集与本地上游模块说明v1](../../module/model-invoker/peripheral-union-and-local-upstream-v1.md)
- [架构与语义映射](./architecture.md)
- [第三方中转站兼容 Route v1](./third-party-relay-compat-v1.md)
- [Praxis执行语义并集v1设计草案](./execution-semantic-union-v1-draft.md)
- [Intent、Mechanism、Effect与Profile路由v1设计草案](./intent-mechanism-effect-profile-routing-v1-draft.md)
- [上游官方Agent行为与HarnessDelta研究（2026-07-13）](./upstream-official-agent-behavior-and-harness-delta-research-20260713.md)
- [代表Route纸面编译与跨Route一致性合同v1](./representative-route-paper-compilation-and-union-conformance-v1.md)
- [执行语义并集 Runtime 实施计划 v1](../../plan/model-invoker/execution-semantic-union-runtime-v1.md)
- [执行并集 Runtime v1 模块说明](../../module/model-invoker/execution-semantic-union-runtime-v1.md)
- [并集语义编译与一致性设计图源](./grounding/union-route-compilation-and-conformance-v1.drawio)
- [并集语义编译与一致性设计图预览](./grounding/union-route-compilation-and-conformance-v1.png)
- [Codex、OpenCode、OpenClaw调用原语研究（2026-07-12）](./codex-opencode-openclaw-semantic-primitives-research-20260712.md)
- [统一语义原语 v1候选](./semantic-primitives-v1.md)
- [语义原语×六协议×39默认Route/14活跃Adapter矩阵v1候选](./semantic-matrix-v1candidate.md)
- [语义矩阵机器CSV](./semantic-matrix-v1candidate.csv)
- [官方订阅调用面设计卡v1候选](./subscription-route-cards-v1.md)
- [订阅调用与官方Harness路由研究（2026-07-12）](./subscription-and-official-harness-research-20260712.md)
- [Route Policy/Audit Invoker v1候选](./route-invocation-facade-v1.md)
- [上游调用最终候选设计](./route-gateway-final-candidate.md)
- [Provider 缓存传输边界 v1](./provider-cache-transport-boundary-v1.md)
- [Provider缓存事实机器CSV v1候选](./provider-cache-facts-v1candidate.csv)
- [最终候选审核清单](./final-candidate-review.md)
- [Route Gateway信任闭合修正设计](./route-gateway-trust-closure.md)
- [宿主激活与十家上游再验证设计](./host-activation-and-upstream-revalidation.md)
- [Factory A/B双层信任矩阵设计](./factory-trust-matrix-v1.md)
- [Factory信任矩阵机器CSV v1候选](./factory-trust-matrix-v1candidate.csv)
- [Factory信任矩阵展开Markdown v1候选](./factory-trust-matrix-v1candidate.md)
- [厂商与 SDK 调查矩阵](./provider-matrix.md)
- [第二阶段 Anthropic 与 Gemini Provider 设计](./provider-phase-2.md)
- [第三阶段完整上游生态设计](./provider-phase-3-upstream-ecosystem.md)
- [第三阶段波次 E1 MiniMax按量设计卡](./provider-phase-3-wave-e1-minimax-payg.md)
- [第三阶段波次 E1 Xiaomi MiMo按量设计卡](./provider-phase-3-wave-e1-mimo-payg.md)
- [第三阶段波次 E1 Qwen/百炼按量设计卡](./provider-phase-3-wave-e1-qwen-payg.md)
- [第三阶段波次 E1 xAI按量设计卡](./provider-phase-3-wave-e1-xai-payg.md)
- [第三阶段完整上游生态执行计划](../../plan/model-invoker/phase-3-upstream-ecosystem.md)
- [Agent 核心结构图](./grounding/agent-core-overview.png)

## 设计门槛

第一阶段进入 `plan/model-invoker/` 的决定：

- [x] 第一版语义字段由 `plan/model-invoker/README.md` 固定；
- [x] 第一阶段不引入 TypeScript Sidecar，IPC 协议延期到确有 Provider 需要时再设计；
- [x] OpenAI 为第一 Provider，其余 Provider 逐个设计和计划；
- [x] 第一阶段只覆盖 Agent 所需能力；
- [x] 使用四级能力契约，`Partial` 必须由调用者显式允许降级；
- [x] 本轮只做无密钥离线测试，真实 API 集成测试等待用户后续提供边界；
- [x] SDK 精确锁版本，升级前运行离线兼容回归并同步矩阵与 memory。

第一阶段与第二阶段已经完成离线实现与验收。第三阶段波次 A也已完成：七维 Route身份、Credential引用与约束、Catalog、版本化 Schema、证据 TTL/状态门禁、Catalog生成的 Markdown当前 Binding区块、Runtime AdapterID映射，以及统一离线脚本和 CI入口均已落地。波次 A最初四条 Binding的 AdapterID映射为 OpenAI Responses/Chat → `openai`、Anthropic Messages → `anthropic`、Gemini GenerateContent → `gemini`。

波次 A的三项 fuzz与 B1/B2/B3/B4各两项协议 fuzz均已完成验收。B0固定 SDK中立协议根；B1至 B4抽取四协议 driver；B5/B6统一安全错误与公共 SDK签名边界。波次 C新增动态订阅状态、允许/拒绝 Key前缀、到期/额度/HTTP拒绝、禁止自动 PAYG和 Savings Plan BillingPlan边界。波次 D新增 AWS Bedrock Mantle/Runtime、Google Vertex、Azure OpenAI四个云 Adapter和 Bedrock两协议。波次 E1已完成 DeepSeek Chat/Messages、Kimi/Z.AI按量 Chat、MiniMax Messages/Chat/Responses、MiMo Messages/Chat、Qwen北京/新加坡 Responses/Chat与 xAI `grok-4.5` Responses。最终候选订阅阶段实现Kimi、MiniMax、MiMo、Alibaba共16条Route；信任闭合审核后这些Route默认受宿主信任阻塞。当前Catalog为62条：39条默认callable、16条已实现但host-blocked、7条研究/控制记录；真实烟测仍延期。
