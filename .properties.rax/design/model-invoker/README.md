# 模型调用器设计

## 设计状态

- 模块名称：`model-invoker`
- 当前阶段：第三阶段当前授权范围完成；A-E1离线实现与验收完成，F未触发，G/H明确延期，总计划已转为陈旧计划
- 最近更新：2026-07-11
- 进入计划阶段：第一、第二阶段均已于 2026-07-10 获得用户明确实施授权
- 代码实现：统一 Go内核、十四个 Runtime Provider、波次 A上游基础、B完整协议层、C动态订阅控制面、D云托管与 E1全部路线均已离线验收，位置为 `ExecutionRuntime/model-invoker/`
- 下一阶段：当前无已授权实施波次；E2、Sidecar、第三方首批名单、真实 API与生产评审需新的设计和单独授权

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

## 暂不纳入核心范围

- 图像、视频、音乐等生成产品的完整统一；
- 语音实时通信的完整实现；
- 微调、评测、模型部署和账号管理；
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

- [架构与语义映射](./architecture.md)
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

波次 A的三项 fuzz与 B1/B2/B3/B4各两项协议 fuzz均已完成验收。B0固定 SDK中立协议根；B1至 B4抽取四协议 driver；B5/B6统一安全错误与公共 SDK签名边界。波次 C新增动态订阅状态、允许/拒绝 Key前缀、到期/额度/HTTP拒绝、禁止自动 PAYG和 Savings Plan BillingPlan边界。波次 D新增 AWS Bedrock Mantle/Runtime、Google Vertex、Azure OpenAI四个云 Adapter和 Bedrock两协议。波次 E1已完成 DeepSeek Chat/Messages、Kimi/Z.AI按量 Chat、MiniMax Messages/Chat/Responses、MiMo Messages/Chat、Qwen北京/新加坡 Responses/Chat与 xAI `grok-4.5` Responses；reasoning、continuation、区域/Workspace、流、缓存与专属终态均有离线门禁，订阅 Offering继续隔离。当前 Catalog为62条，其中39条 callable、23条控制记录。F审计未发现非 Go独占能力触发证据，未生成 Sidecar空壳；第三方首批名单与真实烟测按当前授权延期。统一离线脚本和最终资产一致性审计已通过，总计划已转为陈旧计划；当前不声明真实可用或生产支持。
