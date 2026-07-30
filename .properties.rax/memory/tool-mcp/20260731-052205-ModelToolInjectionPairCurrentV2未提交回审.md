# Model Tool Injection Pair Current V2 未提交回审

时间：2026-07-31 05:22:05（Asia/Shanghai）
依赖刷新复验：2026-07-31 05:36:31（Asia/Shanghai）
独立审计返修复验：2026-07-31 05:53:30（Asia/Shanghai）
发布前 latest-main 复验：2026-07-31 06:16:42（Asia/Shanghai）
Context Pair #65 合并后复验：2026-07-31 06:19:20（Asia/Shanghai）
Host #66 与 Sandbox #56 合并后复验：2026-07-31 06:50:36（Asia/Shanghai）
Sandbox causal-time fix #67 合并后最终复验：2026-07-31 06:58:45（Asia/Shanghai）

## 当前状态

- Model helper PR #60、Model Boundary V3 PR #63、#64/#62、Context Pair #65、Host #66、Sandbox #56 及 causal-time fix #67 已合并；worktree 已 rebase 到 `main@3cb68b12a6223096b042cf62c7c42fd1d03da9e6`，不再是 stacked 依赖。
- Tool Owner authoritative pair/current projection、reader 与 Model V2 adapter 已实现。
- Model paired-reader 现有字段可无损承载 actual compiled digest，未修改 Model。
- 独立敌手复审结论为 P0=0、P1=0、P2=0，已放行 ready PR 发布。

## 已验证

- focused `-count=100`：通过。
- focused race `-count=20`：通过。
- Tool/MCP full ordinary：通过。
- Tool/MCP full race：通过。
- Tool/MCP `go vet ./...`：通过。
- PR #67 合并后的 Tool Owner 与 Model paired-reader/helper 公开形状未变化；`605a5174` 的 focused×100/race×20 高重复证据继续有效。
- P1 已修复：source、expected、compiled、request 与 projection digest 角色不可替代；stored/actual compiled 仍保持同角色严格相等。
- P2 已修复：删除未接入依赖的恒真计数，改由静态 import conformance 与实际注入的 connected spy 证明 Provider 调用与 Tool execution 为 0。
- 两组清空 ProjectionDigest 的敌手矩阵均被 Seal 拒绝；伪造正确 canonical ProjectionDigest 后，Validate 与 adapter 仍拒绝。
- `main@605a5174` 上 focused×100/race×20 高重复门已通过；最终 rebase 到 `main@3cb68b12` 后，focused sanity、full ordinary、full race、vet、diff/import 已重新通过。

## 剩余边界

- ToolChoice 不属于 Tool pair。
- production Provider 组装与授权仍未开放，production 仍为 NO-GO。
- 本快照随 Tool pair 发布提交入库；ready PR 不自行合并。
