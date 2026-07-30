# Application Result Durable Exact V2

状态：owner-local durable Application V2 result store/index 已实现；完整 Tool settled Context handoff B1 继续 BLOCKED/NO-GO。

## 已冻结边界

- 写入只发生在现有 `ApplicationResultStoreV2.CreateSingleCallApplicationResultV2` create-once seam；该 seam 由 `SingleCallToolActionAdapterV2` 在真实 Tool settlement close 后调用。
- 不提供 handoff caller whole-fact Ensure、镜像表或旁路写 API。
- 同一 durable canonical row 同时支持既有 request inspect key 与 original `SingleCallToolActionResultRefV2` exact ref 读取。
- row 保存 Application request/result canonical body、两组 exact secondary coordinates 与独立 row digest。
- original ref exact read 只有 SQLite 读取能力，不调用 Tool execute 或 Provider。

## 完整 B1 的真实阻塞

- production Tool path 使用 `ToolDomainResultFactV2`，禁止用旧 `DomainResultFact` 冒充。
- production 尚无 durable `SettledToolResultProjectionV1` producer/store/current reader。
- `Projection.Tool` provenance、Classification、inline/artifact materialization policy 与 payload source 尚未冻结。
- 因此本子切片不定义 public handoff contract/Inspector，不保存 Tool closure，不制造 projection，也不生成任何 Context-owned字段。
