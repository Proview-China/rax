# Praxis AgentDefinition V1

本模块把严格声明式 YAML 封存为不可变、可摘要、可审计的
`AgentDefinitionV1`。它只表达 Agent 需要什么，不解析组件、不读取秘密、
不创建 Instance/Run，也不启动 Harness、Provider 或 Sandbox。

## 公共包

| 包 | 用途 |
|---|---|
| `contract` | Source、Definition、exact Ref、current projection、Validate/Seal/canonical |
| `decoder` | 安全 YAML/严格 JSON 到规范化 Source |
| `ports` | Owner Repository、下游只读 Reader、Approval current Reader |
| `store` | Memory reference repository 与单节点 SQLite WAL durable repository |
| `conformance` | 第三方 Repository 可复用 fixture 与合同测试 |

作者 YAML 必须显式声明首版七个核心 kind，全部为 required + production。
自定义 namespaced kind/capability 通过注入的 `ValidationCatalogV1` 接入；
扩展声明注册统一由 `RegisteredExtensionKeys` 表达，不使用组件 switch。可信
Secret 字段只允许 `SecretRefV1`；opaque extension 的明显 secret/path 仅做粗粒度
fail-closed 扫描，unknown optional 仍是 untrusted opaque，不能据此证明“不含秘密”。

最小调用顺序：

```text
DecodeYAMLV1
  -> ValidateSourceV1
  -> ApprovalCurrentReaderV1
  -> SealDefinitionV1
  -> DefinitionRepositoryV1.CreateDefinitionV1
  -> AgentDefinitionRefV1
```

回包丢失后，`ServiceV1` 只 Inspect 原 DefinitionID/Revision；同 source
恢复原对象，换 source 返回 Conflict。历史 revision 不变，current 指针通过
CAS 单调推进；撤销和过期不会改写历史对象。Current reader 持久保存最高
`CheckedUnixNano`，跨过 NotAfter 后任何较低 checked 都拒绝，不能 active ABA。

新建 Definition 在写入前对同一 exact ApprovalRef 执行 S1/S2 双读；S1/S2 必须
全字段一致，S2 返回后的 fresh clock 不得回拨且必须仍在 TTL 内，Owner 时间使用
该 S2 clock。

验证入口：

```bash
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
```

`store.SQLiteRepositoryV1`在不改变既有 Definition digest/Memory 语义的前提下，
持久化 immutable history、current exact ref、current revision/state、最高
`CheckedUnixNano`与 revoke closure；使用 schema digest、history/current 外键、
row digest 和严格 JSON decode。它只提供单节点本机 crash durability，不提供
HA、远程副本、RPC 或 SLA。Agent Assembler 与 Host Composition Root 不在本模块内。
