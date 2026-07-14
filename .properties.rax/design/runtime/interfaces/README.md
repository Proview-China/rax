# Application Facade、REPL、API与SDK

## 1. 边界

Application Command API是跨领域应用门面，不是Runtime内部模块，也不拥有Definition、Profile、Review、Artifact或Cache语义。

```text
Application Facade
├── Definition命令 -> Definition所有者
├── Profile命令 -> Profile所有者
├── Assembly命令 -> Assembler
├── Instance/Run命令 -> Runtime
├── Review命令 -> Review所有者
├── Artifact/Memory命令 -> 对应所有者
└── Cache查询 -> Context/Cache所有者
```

REPL、Remote API和User SDK共享同一资源、命令、查询、Operation、Watch和错误语义，不各自实现授权、状态机或重试。

## 2. Transport-neutral合同

```text
ResourceRef(kind, tenant, id, version_or_revision)
ExecutionPreconditions(
  identity_epoch?, lineage_id?, instance_epoch?,
  sandbox_lease_id?, lease_epoch?, authority_epoch?,
  aggregate_revision, effect_intent_revision?
)
CommandEnvelope(
  target: ResourceRef,
  actor, authority_ref, reason,
  command_kind, canonical_payload_digest,
  preconditions: ExecutionPreconditions,
  idempotency_key, idempotency_scope,
  submitted_at, expires_at
)
QueryEnvelope
OperationRef
WatchCursor(scope, watermark, schema_version)
TypedError
```

- Instance/Run命令必须同时携带Identity、Lineage、Instance和适用Lease的epoch前置条件；非执行资源只携带其领域所需集合；
- 幂等范围固定为`tenant + actor + target ResourceRef + command_kind + idempotency_key`，Payload Digest变化返回冲突，不能复用旧结果；
- 长操作返回OperationRef；
- 修改使用乐观并发revision；
- 分页Token不透明并绑定版本和Scope；
- Watch Cursor绑定Ledger Scope；
- 写命令遇到未知语义字段默认拒绝；
- 查询和事件允许兼容性新增字段；
- Transport错误与领域错误分离。

错误类型至少包括：`invalid_argument`、`unauthenticated`、`forbidden`、`not_found`、`conflict`、`precondition_failed`、`capability_unavailable`、`indeterminate`、`rate_limited`、`unavailable`和`internal`。Transport只映射这些稳定领域类别；具体错误还携带稳定`reason_code`，例如`stale_instance_epoch`、`stale_lease_epoch`、`revision_conflict`和`idempotency_payload_mismatch`，不同Transport不得改写领域含义。

## 3. REPL

REPL是User SDK之上的交互客户端，不拥有后门。它支持定义/选择Agent、解析Profile、展示来源/Residual/风险/预算/缓存、Preflight、控制Instance/Run、Watch、审批、产物和诊断。它不得直接读取Secret明文、修改Kernel状态或绕过Effect Intent。

## 4. User SDK与Extension SDK

User SDK面向Agent使用者；Extension SDK面向Provider开发者。二者共享基础资源引用和错误，但不共享权限。首期语言、Transport、目录和交付顺序必须由用户审核，当前设计不预设。

## 5. CAP与一致性

无法访问线性化事实源时，Facade只能提供带watermark的stale read；不得接受推进命令、新授权、续租或新Effect。不同入口对同一命令必须产生相同领域结果。

## 6. 最低反例

- `API-01`：REPL和Remote API对旧epoch Stop必须同样返回precondition failed；
- `API-02`：Facade不得直接把Review Verdict写成Runtime状态；
- `API-03`：分区侧API不得接受Resume，即使本地投影显示Paused；
- `API-04`：未知写字段不能被静默忽略。
- `API-05`：同一幂等Key换Payload Digest，所有入口必须返回同一conflict/reason code；
- `API-06`：命令缺少适用的lease epoch前置条件时必须precondition failed，不得读取本地投影补齐。
