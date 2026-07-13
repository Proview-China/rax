# Codex、OpenCode、OpenClaw语义研究完成

- 时间：2026-07-12 18:50:02 CST
- 模块：`model-invoker`
- 阶段：执行语义并集设计研究完成，草案已修订

## 本次进展

1. 核验Codex、OpenCode、OpenClaw官方仓库当前HEAD和本机发布标签基线；
2. 读取Codex Responses item、Thread/Turn/ThreadItem、App Server通知与审批控制；
3. 读取OpenCode LLM ContentPart/LLMEvent、PreparedRequest、Session Message/Part和请求准备层；
4. 读取OpenClaw llm-core、agent-core和Gateway AgentEvent三层合同；
5. 确认三者共同收敛为“模型流 → Agent Turn/Item → 独立控制面”；
6. 将Praxis`UnifiedExecutionEvent`修订为ModelEvent、ExecutionItem/ItemEvent、ControlEvent和DiagnosticEvent分层；
7. 确认ExecutionItem为可持久化权威状态，delta只用于顺序流和实时UI；
8. 增加visibility五级和模型tool call/实际tool action分离；
9. 没有实现Runtime代码或修改现有Go公共合同。

## 当前资产

- [`codex-opencode-openclaw-semantic-primitives-research-20260712.md`](../../design/model-invoker/codex-opencode-openclaw-semantic-primitives-research-20260712.md)
- [`execution-semantic-union-v1-draft.md`](../../design/model-invoker/execution-semantic-union-v1-draft.md)

## 下一步

与用户确认三层内部语义、ExecutionItem状态机、visibility、工具所有权和公共合同模块归属；确认后才能形成实现计划。
