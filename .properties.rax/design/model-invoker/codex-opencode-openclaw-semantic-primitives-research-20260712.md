# Codex、OpenCode、OpenClaw调用原语研究（2026-07-12）

## 1. 研究目的与边界

本研究只回答三个问题：

1. Codex、OpenCode、OpenClaw分别怎样封装模型输入、模型流、工具、Agent执行和会话；
2. 哪些结构值得进入Praxis执行语义并集；
3. 哪些设计只属于单个产品，不能直接照搬。

本轮读取官方文档和官方仓库源码，没有登录账号、没有执行模型调用、没有修改上游代码。结论是架构研究，不是许可证或法律意见。

## 2. 证据快照

### 2.1 当前HEAD

| 项目 | 官方仓库 | 核验HEAD | 核心文件 |
|---|---|---|---|
| Codex | `openai/codex` | `9e552e9d15ba52bed7077d5357f3e18e330f8f38` | `app-server-protocol/v2/{thread,turn,item}.rs`、`protocol/{models,items}.rs` |
| OpenCode | `anomalyco/opencode` | `34e58090595d44e3e7cc37498f16753a98627456` | `schema/v1/session.ts`、`llm/schema/{messages,events}.ts`、`session/llm/request.ts` |
| OpenClaw | `openclaw/openclaw` | `ff4b4517d2ef00a2150a04b880e34a3d679784af` | `llm-core/types.ts`、`agent-core/{types,agent-loop}.ts`、`infra/agent-events.ts` |

### 2.2 本机发布标签基线

| 项目 | 本机源码快照 | 标签提交 | 与当前HEAD的相关差异 |
|---|---|---|---|
| Codex | `0.144.1` | `db75c19352d29ef29c17dbcf73a7244f1b1a8d10` | 本轮检查的v2 item/turn核心类型没有结构性变化 |
| OpenCode | `1.17.18` | `b1fc8113948b518835c2a39ece49553cffe9b30c` | 本轮检查的Session Part和LLMEvent核心类型没有结构性变化 |
| OpenClaw | `2026.6.11` | `08d1bbad1bd6ee5700082e1c0f65f63f07600d1f` | HEAD新增requestId、context usage质量、runtime context carrier、工具进度可见性和私有audit事件 |

官方来源：

- Codex App Server：<https://learn.chatgpt.com/docs/app-server>
- Codex源码：<https://github.com/openai/codex/tree/9e552e9d15ba52bed7077d5357f3e18e330f8f38/codex-rs>
- OpenCode源码：<https://github.com/anomalyco/opencode/tree/34e58090595d44e3e7cc37498f16753a98627456>
- OpenClaw源码：<https://github.com/openclaw/openclaw/tree/ff4b4517d2ef00a2150a04b880e34a3d679784af>

## 3. Codex语义模型

### 3.1 Codex实际有三层语义

```text
Responses模型层
  ResponseInputItem / ResponseItem
        |
        v
Codex Core执行层
  Thread / Turn / TurnItem
        |
        v
App Server产品协议层
  JSON-RPC request/response/notification + approval server request
```

### 3.2 模型调用原语

`ResponseInputItem`不是简单字符串：

- `message`：role + `ContentItem[]`；
- `function_call_output`；
- `mcp_tool_call_output`；
- `custom_tool_call_output`；
- `tool_search_output`。

`ContentItem`当前公开核心包含input text、input image和output text。`ResponseItem`进一步包含：

- message与agent message；
- reasoning summary/content/encrypted content；
- function call/output；
- local shell；
- MCP、custom/dynamic tool；
- tool search；
- web search等Hosted能力。

重要结论：Codex在模型层已经使用Tagged Union，不用一个通用`type + any payload`逃生结构承载全部内容。

### 3.3 Agent执行原语

官方App Server把三种对象作为核心：

```text
Thread
  `-- Turn
        `-- ThreadItem[]
```

- `Thread`：持久对话、cwd、模型Provider、来源、父子线程、历史模式和状态；
- `Turn`：一次用户请求及随后Agent工作，有明确状态和时间；
- `ThreadItem`：一项可显示、可持久化的输入、输出或动作。

当前`ThreadItem`覆盖：

- user message、agent message；
- plan、reasoning；
- command execution、file change；
- MCP tool call、dynamic tool call；
- collaboration agent tool call、sub-agent activity；
- web search、image view、image generation；
- sleep、review mode、context compaction、extension item。

Item具有完整状态，而delta是Item的流式增量。例如agent message、plan、reasoning和command output都有自己的delta通知；Item完成后，completed Item才是权威快照。

### 3.4 控制面原语

Codex没有把运行控制塞进普通消息：

- `thread/start/resume/fork/read/list/archive`管理会话；
- `turn/start/steer/interrupt`管理一轮执行；
- approval通过Server发起的JSON-RPC request完成；
- `command/exec/write/resize/terminate`是独立进程控制；
- 工具、文件和MCP审批使用不同的结构化决定。

`turn/start`能覆盖model、effort、summary、output schema、cwd、sandbox/permissions、approval和collaboration mode。这证明Harness Profile不能只保存模型参数，必须同时编译工作区、权限、审批和预置协作语义。

### 3.5 Codex对Praxis的价值

应吸收：

1. Conversation/Thread → Turn → Item层级；
2. Item作为权威可持久化状态；
3. delta只更新Item，不成为第二事实源；
4.审批和运行控制独立于消息；
5. command、file、MCP、subagent不压成同一种function tool；
6. schema按版本生成，experimental字段需要能力协商。

不直接照搬：

- Codex专属Thread配置和协作模式；
- Responses原生item名称；
- Codex固定prompt、skills、MCP、sandbox和审批实现；
- 将所有Route都假设为可提供完整ThreadItem。

## 4. OpenCode语义模型

### 4.1 OpenCode明确分成模型层与Session层

```text
LLM层
  Message / ContentPart / LLMEvent / PreparedRequest
        |
        v
Session层
  User|Assistant Message + Part[]
        |
        v
产品事件层
  message.updated / part.updated / part.delta / session.status
```

### 4.2 OpenCode LLM层

模型消息`ContentPart`包含：

- text；
- media；
- tool-call；
- tool-result；
- reasoning。

`LLMEvent`是细粒度顺序流：

- step-start；
- text start/delta/end；
- reasoning start/delta/end；
- tool-input start/delta/end；
- tool-call、tool-result、tool-error；
- step-finish、finish、provider-error。

`PreparedRequest`保存route、protocol、model、原生body和metadata，使“统一意图编译后的真实请求”可以预览。这与Praxis的`PreparedExecutionPlan`高度一致。

### 4.3 OpenCode Session层

OpenCode不直接持久化全部LLMEvent，而是把Message拆成Part：

- text、reasoning、file、tool；
- step-start、step-finish；
- snapshot、patch；
- agent、subtask；
- retry、compaction。

Tool Part使用状态机：`pending → running → completed|error`，并保存input、output、attachments、metadata和时间。

这种结构的重要价值是：模型流适合传输，Part适合UI、持久化、恢复、重放和审计。两者不应混为一个类型。

### 4.4 OpenCode请求准备

`LLMRequestPrep.prepare`明确做以下工作：

1. 组合agent prompt、Provider system prompt、调用system和user system；
2. 允许插件执行system transform、params和headers变换；
3. 合并model、agent、variant和Provider options；
4. 按权限和用户选择过滤tools；
5. 为不同Provider调整strict/tool replay等兼容行为；
6. 形成最终system、messages、tools、params、headers。

对OpenAI OAuth路线，OpenCode把组合后的system写入`options.instructions`而不是普通system message。此事实证明其模型并非“无预设裸调用”，而是有明确的准备/转换层。

### 4.5 OpenCode对Praxis的价值

应吸收：

1. `PreparedRequest`/预执行编译产物；
2. ModelEvent与持久Part分层；
3. content/tool input的start-delta-end；
4. 工具结果支持text/json/error/content，而不是只有string；
5. Provider executed标记；
6. provider metadata与opaque replay signature保留；
7. prompt/system/params/header转换通过显式准备阶段完成。

需要改进而不是照搬：

- Session Part中的任意metadata必须在Praxis采用命名空间和安全Schema；
- OpenCode部分兼容逻辑会合成空tool或文本占位，Praxis必须进入MappingReport；
- system plugin transform非常自由，Praxis Profile需要版本、digest和审计；
- 不能用OpenCode OAuth实现自动推导Praxis的授权。

## 5. OpenClaw语义模型

### 5.1 OpenClaw同样是三层

```text
llm-core
  Context / Message / AssistantMessageEvent
        |
        v
agent-core
  AgentContext / AgentEvent / AgentTool
        |
        v
Gateway
  AgentEventPayload(stream + runId + seq + session/agent context)
```

### 5.2 llm-core模型语义

`Context`非常小：

- `systemPrompt`；
- `messages`；
- `tools`。

Message分为user、assistant、toolResult。Assistant content分为text、thinking、toolCall，并保留provider/api/model、response ID、usage、stop reason和诊断。

`AssistantMessageEvent`使用：

- start；
- text/thinking/toolcall start-delta-end；
- done或error终态。

Stream合同规定：调用之后的运行错误进入流中的最终error message，而不是随意throw。这给Praxis提供了稳定的终态设计参考。

### 5.3 agent-core语义

`AgentEvent`增加：

- agent start/end；
- turn start/end；
- message start/update/end；
- tool execution start/update/end。

AgentTool独立定义label、参数准备、execute、progress和并发模式。Agent loop拥有工具执行循环，而llm-core只拥有模型输出工具调用。

这进一步证明“模型tool call”和“Agent实际执行工具”必须是不同语义。

### 5.4 Gateway事件

Gateway使用统一外壳：

```text
AgentEventPayload
  - runId
  - seq
  - stream
  - ts
  - data
  - sessionKey/sessionId/agentId
```

stream分为lifecycle、tool、assistant、error、item、plan、approval、command_output、patch、compaction和thinking。HEAD还新增私有audit event通道与工具进度可见性控制。

优点：跨不同Agent/Harness容易接入。缺点：`stream + Record<string, unknown>`类型强度不足，长期容易产生非正式payload方言。

### 5.5 OpenClaw对Praxis的价值

应吸收：

1. 模型层、Agent loop层、Gateway投影层分离；
2. runId + seq + session/agent上下文；
3. tool call与tool execution生命周期分离；
4.进度可见性和audit可见性分离；
5. provider-billed cost与derived usage区分；
6. stale lifecycle generation拒绝旧运行事件污染新会话。

需要改进：

- Praxis公共合同不用`Record<string, unknown>`承载已知事件；
- 空usage不能统一理解为真实零；
- Gateway投影事件不能反向替代Agent Core权威状态；
- 公开进度、模型可见内容和审计内容必须有不同visibility。

## 6. 三者结构对照

| 语义层 | Codex | OpenCode | OpenClaw | Praxis结论 |
|---|---|---|---|---|
| 模型输入 | ResponseInputItem | LLM Message/ContentPart | Context/Message | `ModelInvocationIntent` |
| 模型流 | ResponseItem/Core events | LLMEvent | AssistantMessageEvent | `ModelEvent` |
| Agent轮次 | Turn | Session assistant step | turn_start/end | `Turn` |
| 可持久动作 | ThreadItem | Message Part | AgentEvent + Gateway投影 | `ExecutionItem` |
| 工具模型请求 | function/dynamic/MCP item | tool-call | ToolCall | `ModelToolCall` |
| 工具真实执行 | command/MCP/dynamic item | ToolPart state | tool_execution_* | `ActionExecution` |
| 会话 | Thread | Session | sessionKey/sessionId | `Conversation/Session` |
| 控制 | steer/interrupt/approval RPC | Permission/session APIs | approval/Gateway control | `ControlEvent/Command` |
| 编译产物 | thread/turn config | PreparedRequest | Context + config | `PreparedExecutionPlan` |
| 诊断 | warning/usage/items | metadata/error/parts | diagnostics/audit events | typed DiagnosticEvent |

## 7. 对Praxis并集语义的确定修订

### 7.1 内部三层，不做平面大事件枚举

```text
UnifiedExecutionEvent
  = LifecycleEvent
  | ModelEvent
  | ItemEvent<ExecutionItem>
  | ControlEvent
  | DiagnosticEvent
```

### 7.2 ModelEvent只表达模型流

纳入：content、reasoning summary、tool input、tool call/result/error、step、usage、finish、provider error。

不纳入：命令已执行、文件已修改、审批、子Agent、会话迁移。这些属于Item或Control。

### 7.3 ExecutionItem是权威执行状态

候选Item：message、reasoning summary、plan、tool action、command、file change、MCP call、hosted action、artifact、subagent、compaction、extension。

生命周期：`proposed/pending → running → completed|failed|declined|cancelled`。不同Item可以只允许其中子集。

### 7.4 Control独立

approval、user input、steer、interrupt、resume、cancel、session fork不属于普通Item，也不属于模型Event。

### 7.5 delta不是事实源

`item/delta`只用于实时UI和流处理；completed Item或持久快照才是恢复与审计事实。Result由权威Item和终态确定性投影。

### 7.6 visibility必须进入合同

至少区分：

- `model_visible`；
- `user_visible`；
- `progress_only`；
- `audit_only`；
- `private_runtime`。

避免OpenClaw目前需要用`suppress/hideFromChannelProgress`补丁式控制显示。

### 7.7 工具所有权必须保持

借鉴OpenCode的`providerExecuted`和OpenClaw的模型tool call/agent tool execution分层，Praxis继续固定：`praxis | harness | provider | external`。只有owner是Praxis时统一层才执行动作。

## 8. 不采用的共同缺陷

1. 不把任意metadata或`Record<string, unknown>`当长期公共合同；
2. 不以空usage/零token表示未知；
3. 不把系统prompt转换藏在插件hook而没有Profile版本和digest；
4. 不把流式delta当持久状态；
5. 不把provider tool call和本地工具执行混成一个事件；
6. 不把所有工具压成function；
7. 不把Harness内部不可见动作合成虚假事件；
8. 不继承上游产品OAuth身份、allowlist或条款资格。

## 9. 研究结论

Praxis当前五个顶层原语方向正确，但`UnifiedExecutionEvent`内部必须采用“模型事件、执行Item、控制事件、诊断事件”分层。最值得复用的不是任何一家具体字段，而是三家共同收敛出的结构：

```text
准备/编译
  -> 模型流
  -> Agent Turn/Item状态
  -> 独立控制面
  -> 持久快照与结果投影
```

这套结构能够同时承载直连API和不纯净Harness，又不会把Harness方言泄漏给上层。
