# AgentDefinition V1 终审返修完成

- 时间：2026-07-18 00:06 +08:00
- 范围：`ExecutionRuntime/agent-definition/**`
- 公共 API：`ValidationCatalogV1` 的两套 required/optional 扩展列表收敛为单一 `RegisteredExtensionKeys`。
- 语义：已出现且 `required=true` 的扩展必须注册；unknown optional 保留；注册项不要求必须出现。
- 安全：opaque extension 对秘密字段、常见 token/PEM/Bearer 值、file URI、路径遍历和 Unix/Windows 路径 fail closed。
- Current：Reference store 持久最高 checked 水位，跨过 NotAfter 后拒绝较低 checked，消除 active ABA。
- 隔离：Service 与 Memory store 构造时 deep clone catalog；Service 对 typed nil 依赖 fail closed；Definition clone 改为结构化深拷贝。
- Gate：收紧 production import boundary，并补 catalog mutation、expiry ABA、typed nil、revision race 与秘密矩阵反例。
