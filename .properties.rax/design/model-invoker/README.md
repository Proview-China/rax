# 模型调用器设计

## 设计状态

- 模块名称：`model-invoker`
- 当前阶段：设计中
- 最近更新：2026-07-10
- 进入计划阶段：尚未批准
- 代码实现：尚未开始

## 目标

构建 Praxis 的模型调用核心：先忠实实现每个厂商的原生调用逻辑，再由上层语义映射系统形成 Runtime 可稳定使用的官方统一调用层。

统一层不是把所有厂商压缩成最低公共功能，而是形成所有厂商能力的语义并集，并明确每项能力是原生支持、兼容支持、部分支持还是不支持。

## 已确认的设计决定

1. 采用 Provider First：每个厂商拥有独立适配器和完整能力描述。
2. 采用 SDK First：优先使用官方 Go SDK；没有合适 Go SDK时，允许使用官方 TypeScript SDK或官方兼容 SDK。
3. Go 是调用核心、语义映射、Provider 注册和 Runtime 接口的所有者。
4. TypeScript 可以作为隔离的 Provider Sidecar，但 TypeScript 类型和 SDK 类型不能泄漏到 Go Runtime。
5. 每个厂商的认证、请求、流式事件、推理内容、工具调用、多模态、缓存、文件、Batch、错误和扩展字段都必须受控。
6. 上层提供能力感知的智能映射，不把“OpenAI 兼容”理解为完全等价。
7. 所有映射必须可解释、可审计、可测试；不允许静默丢弃语义。
8. Rust 不属于本模块当前设计范围，只有经过性能测量确认的计算热点才考虑接入。

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
- [Agent 核心结构图](./grounding/agent-core-overview.png)

## 设计门槛

进入 `plan/model-invoker/` 前仍需确认：

- [ ] Praxis Unified Model API 的第一版语义字段；
- [ ] Go 与 TypeScript Sidecar 的进程通信协议；
- [ ] 第一批 Provider 的实现顺序；
- [ ] 第一阶段是否只覆盖 Agent 所需能力；
- [ ] Provider 能力清单和降级策略；
- [ ] 每个 Provider 的真实 API Key 与集成测试边界；
- [ ] SDK 升级、协议变更和兼容回归的维护方式。

当前设计已经足以进入接口细化，但尚未批准进入代码计划。
