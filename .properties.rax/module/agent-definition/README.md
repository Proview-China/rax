# AgentDefinition V1 模块说明

## 1. 作用

AgentDefinition 是 Praxis 声明式 Agent 的最上游事实 Owner。它把作者 YAML
约束为严格 JSON 语义树，验证首版 6+1、身份、Profile、策略、Secret Ref、
扩展和有效窗口，再生成不可变 `AgentDefinitionV1` 与 exact ref。

它不解析 Component Release，不构造 Harness，不绑定 Runtime，不读取 Secret
值，也不创建 Instance、Run、Lease、Permit 或 Settlement。

## 2. 组成

| 代码包 | 内容 |
|---|---|
| `contract` | V1 DTO、字段校验、集合规范化、canonical digest、Seal、current projection |
| `decoder` | 有界 YAML 安全子集与严格 JSON decoder |
| `ports` | Owner Repository、只读 current/exact Reader、Approval current Reader |
| `store` | Memory reference store 与单节点 SQLite WAL durable store |
| `conformance` | 第三方 Repository 共用 fixture 与行为测试入口 |
| 模块根 | `ServiceV1` 的 Approval S1/S2 TOCTOU 门、approval-before-seal、lost-reply Inspect 恢复 |

## 3. 输入输出

输入是 `AgentDefinitionSourceV1` 或等价 YAML。YAML 只允许 null、boolean、
有界 int64、string、array 和 string-key object；duplicate、merge、anchor、
alias、custom tag、float、timestamp、多文档、未知字段全部拒绝。

输出为：

- immutable `AgentDefinitionV1`；
- `DefinitionID + Revision + Digest` exact ref；
- current projection（active/revoked/expired）；
- Runtime `core.DomainError` 分类错误。

## 4. 首版不变量

- 七个核心 kind 全部存在、`required=true`、`support_mode=production`；
- 自定义组件通过 `ValidationCatalogV1` 注册，不增加 kind switch；扩展只使用单一 `RegisteredExtensionKeys`，unknown required 拒绝、unknown optional 原样保留；
- components、capabilities、dependency、secret 和 extension 集合稳定排序；
- nil/empty 集合规范为相同语义；
- 同 ID/revision/同 source 幂等，换 source Conflict；
- history 永不改写，current 只通过 revision CAS 推进，并持久保存最高 `CheckedUnixNano` 防止过期后 active ABA；
- create 回包丢失只 Inspect 原坐标；
- clock rollback、过期 approval、过期 effective window fail closed；
- 新建写入前同一 exact ApprovalRef 的 S1/S2 必须完整一致；S2 后 fresh clock 不回拨、S2 仍 current，Definition Owner 时间取 fresh S2 clock；
- 可信 Secret 字段只保存引用；明显 secret/token/password/private key/API key/authorization/credential/cookie 字段、常见秘密值、file URI、遍历路径和 Unix/Windows 本机路径粗粒度 fail closed，但黑名单不证明任意 opaque payload 都不含秘密；
- unknown optional extension 只 opaque/untrusted 保留，不能进入 trusted production resolution；当前 key 注册与 payload 自洽摘要不等于 exact schema trust，生产扩展仍需治理目录 exact schema 绑定与专属 validator Port。

## 5. 依赖和边界

本模块只单向依赖 Runtime 稳定 `core` 的 canonical、Digest、SemVer 和
DomainError，以及 YAML parser。Runtime 不反向依赖本模块，因此不存在 SCC。
Assembler 只能依赖本模块 public contract/reader，不能依赖 store 或 Service
实现。Harness、Application 和6+1组件不读取 Definition。

## 6. 使用

```go
service, _ := agentdefinition.NewServiceV1(repository, approvals, catalog, clock)
result, err := service.CreateYAMLV1(ctx, yamlBytes)
ref := result.Definition.RefV1()
```

生产宿主必须提供真实 Approval current Reader。`SQLiteRepositoryV1`已按现有
`DefinitionRepositoryV1`持久化history/current/highest checked/revoke，使用schema
digest、外键、row digest与严格JSON decode，并覆盖重启、lost reply、64独立Store、
clock rollback、ABA、损坏和typed nil。它是单节点本机crash-durable实现，不认证
HA、远程副本、availability/SLA或trusted extension resolution。

## 7. 验证

模块测试覆盖 strict decoder、canonical/nil-empty、custom component、
history/current/revoke、lost reply、64并发、clock/expiry、深拷贝、fault、
黑盒 YAML、fuzz、revision race、catalog mutation isolation、typed nil、clock ABA、vet 和 import boundary。

```bash
cd ExecutionRuntime/agent-definition
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
```

## 8. 当前限制

- 已有单节点SQLite WAL持久Backend，但没有HA、远程副本、RPC、网络服务或availability/SLA；
- 不拥有 Agent Assembler、Harness Assembly、Runtime admission 或 Host；
- Definition 完成不等于 Agent 已装配或可生产运行。
