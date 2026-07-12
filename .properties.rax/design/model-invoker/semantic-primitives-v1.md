# Model Invoker 统一语义原语 v1候选

## 1. 状态与版本

- 契约 ID：`praxis.model-invoker.semantic/v1candidate`
- Runtime 常量：`modelinvoker.SemanticPrimitivesCandidateVersion`
- 状态：候选；等待 Profile、Cache及未来 Runtime/Context消费者联合审核，不得称最终冻结
- 作用域：`ExecutionRuntime/model-invoker/` 根包的 Provider-neutral 公共语义
- 不包含：Provider SDK 类型、Runtime Kernel、Context Engine、Model Profile、缓存策略、Agent 编排

本文件记录当前候选调用原语和兼容规则，不冻结模型清单、Provider证据、Endpoint、套餐状态或实现数量。C阶段已经补齐原语×协议×Route/Adapter机器矩阵和未来扩展判断；当前版本作为供Profile、Cache、Context和Runtime联合审核的最终候选。

## 2. v1 原语面

| 原语 | v1职责 | 不得承担 |
|---|---|---|
| `Request` | 表达模型、输入、指令、工具、输出约束、推理、状态、预算、元数据和 Provider扩展 | Route选择、Credential值、缓存策略、上下文编排 |
| `Response` | 表达统一输出、状态、用量、请求标识、Provider元数据、映射报告和受控 Raw | 持久化、调度、模型画像 |
| `Stream` / `StreamEvent` | 有序增量、终态、错误与显式关闭 | 后台 goroutine所有权、自动重放、跨 Route续接 |
| `CapabilityContract` / `MappingReport` | 对请求所需能力执行 exact/transformed/degraded/rejected判断 | 猜测未声明能力、静默降级 |
| `Error` | 稳定错误分类、Provider/operation/code、重试信号与安全 unwrap | 暴露 SDK对象、Credential、未经批准的原生 cause |
| `State` | 绑定 Provider与 Protocol的 server/provider continuation | Prompt Cache、跨 Provider会话、Context Engine状态 |
| `Usage` | input/output/reasoning/cache-read/cache-write/total计数 | 计费结论、缓存命中策略、重复汇总 |
| `ProviderOptions` | 以所选 Runtime Adapter ID隔离并验证 Provider专属 JSON | 通用逃生舱、跨 Provider字段复用 |
| `RawPayload` | 受控、可复制、默认脱敏的协议审计载荷 | 普通日志、秘密容器、缓存存储 |

`Provider`、`Registry`与基础 `Invoker`继续是语义执行边界；新增 RouteID门面只绑定选择与策略，不改变上述原语含义。

## 3. 候选不变量

1. 公共签名只能出现 Go标准库和 Praxis自有类型，厂商 SDK不得穿透。
2. `Request`中的语义字段不能被静默删除；无法忠实表达时必须拒绝，部分能力只有显式 `AllowDegradation`才可执行。
3. `ProviderOptions`命名空间必须等于已选 Runtime Adapter ID。
4. `State.Provider`与 `State.Protocol`必须等于当前绑定，跨绑定 continuation在网络前拒绝。
5. 流式调用不自动重放；非流式重试仍由基础 `Invoker`单点拥有。
6. Cache read/write是用量明细，不得在业务侧再次与 total相加。
7. Raw、错误和流分片继续服从统一脱敏与响应体硬上限。
8. v1允许增加可选字段、能力值和新 Provider实现；删除字段、改变既有字段含义、放宽安全边界或让 SDK类型进入公共面，需要新的契约版本。

## 4. 与 RouteID 门面的关系

RouteID门面使用同一个 `Request`，但调用方必须把 `Provider`、`Protocol`和 `Endpoint`留空。门面从 Catalog绑定这三个选择器，完成 evidence/callable/policy/entitlement检查后再调用基础 `Invoker`。

Policy/Audit层成功返回 `RouteResponse{Response, Route}`；流式返回 `RoutedStream`。Provider-neutral `Response`和 `StreamEvent`当前不扩字段，Route审计信息保存在并列的 `RouteSelection`中；这些结构是否足以承载未来内容块、动作和CacheIntent，仍由C阶段逐项判断。

## 5. 自动化门禁

完整Route/协议/Adapter能力交叉矩阵、公共结构扩展判断和延期清单见[语义并集矩阵v1候选](./semantic-matrix-v1candidate.md)；780行机器资产见[CSV](./semantic-matrix-v1candidate.csv)。`tests/semanticmatrix`不只检查文档，而是用真实Gateway和内建工厂逐Route调用`Capabilities`并与Catalog逐项对照。

- `tests/core/public_api_test.go`：根包不得导入 Provider SDK；
- `tests/core/public_sdk_boundary_test.go`：所有包外可达签名不得暴露 SDK类型；
- `tests/routefacade/route_invoker_test.go`：版本 ID、Route绑定、不变性、订阅授权和流选择均有黑白盒反例；
- `scripts/verify-offline.sh`：统一执行格式、静态、普通、shuffle、race、integration仅编译和资产校验。
