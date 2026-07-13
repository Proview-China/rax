# Praxis 执行语义并集 v1 设计候选

## 1. 状态与目的

- 契约候选ID：`praxis.execution-semantic-union/v1candidate`
- 创建时间：2026-07-12
- 当前状态：v1语义设计已收口，并已由独立实施计划落地为`praxis.execution-union/v1`离线Runtime合同；本文继续作为设计事实源
- 授权演进：本文形成时仅授权设计；2026-07-13用户另行授权并完成`ExecutionRuntime/model-invoker/{union,profile,effect,execution}`实现，真实账号/付费验证仍未授权执行
- 依赖事实：现有[`model-invoker`统一语义原语](./semantic-primitives-v1.md)、[39 Route语义矩阵](./semantic-matrix-v1candidate.md)和[全量上游/Profile计划](../../plan/model-invoker/upstream-support-and-profile-union-v1.md)
- 三层语义详细合同：[Intent、Mechanism、Effect与Profile路由v1](./intent-mechanism-effect-profile-routing-v1-draft.md)
- 上游行为与注入依据：[官方Agent行为与HarnessDelta研究](./upstream-official-agent-behavior-and-harness-delta-research-20260713.md)
- 纸面编译与一致性依据：[代表Route纸面编译与跨Route一致性合同v1](./representative-route-paper-compilation-and-union-conformance-v1.md)

本设计解决一个比`model-invoker.Request/Response/StreamEvent`更上层的问题：

> 上层如何用同一套Praxis语义调用直连模型、Claude Agent SDK、Codex app-server、Copilot SDK/ACP、Grok Build和Antigravity CLI，同时不掩盖它们在上下文、工具、会话、审批、文件操作、用量和事件上的真实差异。

## 2. 核心结论

### 2.1 不是替换现有Model Invoker语义

现有`model-invoker`合同已经稳定表达：

- 文本消息与Instruction；
- function tool定义、调用与结果；
- 结构化输出；
- reasoning意图与摘要；
- Provider continuation；
- 模型流、错误、用量、Raw和MappingReport。

它继续作为`Direct Model Route`的模型级语义内核。新并集层位于其上方：

```text
Runtime / Context / Agent上层
  -> Praxis Execution Semantic Union
       -> Semantic Route Profile
            +-> model-invoker Request/StreamEvent
            `-> Official Harness SDK/CLI/App Server/ACP
```

### 2.2 统一的是“意图和可观测结果”，不是内部实现

- 统一请求表达调用者想完成什么；
- Profile把意图编译为精确Route能够表达的原生语义；
- 统一事件表达Praxis能够观察和控制的执行过程；
- Harness内部不可见的system prompt、工具循环或模型调用保持“未知/不可控”；
- 无法等价的语义通过映射报告、残余差异或明确拒绝暴露。

### 2.3 v1采用五个顶层原语

| 原语 | 职责 |
|---|---|
| `UnifiedExecutionRequest` | 一次执行或一轮会话的Provider-neutral意图 |
| `PreparedExecutionPlan` | Semantic Route Profile编译后的不可变执行计划 |
| `UnifiedExecutionEvent` | 模型与Agent执行过程的带类型事件并集 |
| `ExecutionCommand` | 运行中审批、拒绝、取消和补充输入 |
| `UnifiedExecutionResult` | 从终态事件确定性投影出的最终结果 |

### 2.4 三层内部语义

基于[Codex、OpenCode、OpenClaw调用原语研究](./codex-opencode-openclaw-semantic-primitives-research-20260712.md)，模型与Agent原生载荷不采用平面大枚举，而分成：

```text
ModelEvent       模型内容、reasoning、tool call/result、usage、finish
ExecutionItem   Agent实际动作与可持久状态
ControlEvent    审批、用户输入、steer、interrupt和session控制
```

模型流、Agent执行和控制面共享统一身份、顺序与Profile来源，但保留不同所有权和生命周期。

这三个原生载荷层位于统一八事件family中的`model/item/control`；`lifecycle/intent/mechanism/effect/diagnostic`由Praxis编译、观测和投影，二者不是互斥分类。

## 3. 总体数据流

```text
UnifiedExecutionRequest
  |
  v
Resolve SemanticRouteProfile
  |
  v
Compile PreparedExecutionPlan
  |-- Policy / Entitlement Gate
  |-- Capability Plan
  |-- Context Envelope
  |-- Tool Ownership Plan
  |-- State / Session Plan
  |-- Request Mapping Plan
  `-- Capability Residuals
  |
  +--> Direct Model Route --> model-invoker
  |
  `--> Harness Route ------> Prepare Harness --> SDK/CLI/App Server/ACP
  |
  v
Ordered UnifiedExecutionEvent stream
  |-- IntentEvent
  |-- MechanismEvent
  |-- ModelEvent
  |-- ItemEvent<ExecutionItem>
  |-- EffectEvent
  |-- ControlEvent
  `-- DiagnosticEvent
  |                ^
  |                `-- ExecutionCommand
  v
UnifiedExecutionResult + MappingReport + ContextEnvelopeReport + Residuals
```

`PreparedExecutionPlan`必须可以在不触达上游的情况下生成和审核。只有计划通过策略与能力检查后，才允许创建网络连接或Harness进程。

## 4. 统一身份与关联模型

每个请求、事件、命令和结果都使用同一套关联标识：

| 字段 | 含义 | 不变量 |
|---|---|---|
| `execution_id` | 一次执行生命周期 | 全局唯一；事件和命令必须匹配 |
| `session_id` | 可续接会话 | 可空；只在同一Profile允许的范围内续接 |
| `turn_id` | 会话中的一轮 | 同一Session内唯一 |
| `item_id` | 内容、动作、审批或产物 | 同一Execution内唯一 |
| `parent_id` | 事件因果父项 | 只能引用当前Execution已有项 |
| `action_id` | 一次工具/命令/外部动作 | 请求、审批、执行和结果共用 |
| `sequence` | 事件序号 | 从1开始严格单调且不重复 |
| `profile_id/version` | 实际Semantic Route Profile | 执行开始后不可漂移 |
| `route_id/version` | 实际上游Route | 执行开始后不可漂移 |

Harness原生thread、conversation、response或tool ID保存在带命名空间的`NativeIdentity`中，不能替代Praxis ID，也不能跨Profile复用。

## 5. UnifiedExecutionRequest

### 5.1 顶层结构

```text
UnifiedExecutionRequest
  - semantic_version
  - profile_selector
  - execution_kind
  - input[]
  - instructions[]
  - context[]
  - tools[]
  - tool_policy
  - output_contract
  - reasoning_intent
  - session_intent
  - execution_policy
  - budget
  - degradation_policy
  - metadata
  - extensions
```

### 5.2 Profile选择

`profile_selector`有两种合法方式：

1. **精确选择**：上层传稳定`semantic_route_profile_id`；
2. **约束解析**：上层传模型意图、Offering、执行面偏好和用途，由Resolver确定唯一Profile。

解析结果不唯一时必须拒绝并返回候选，不允许根据环境变量、余额或“最便宜”隐式选路。请求中不得直接携带Endpoint、OAuth token或API Key。

解析键至少包含Provider、model/revision、Deployment、Protocol、Offering、Auth Route、Execution Surface、Harness和Harness Version。只按模型名命中Profile属于非法模糊选择。

`execution_kind`：

| 值 | 含义 |
|---|---|
| `auto` | 由已选Profile决定，但结果仍报告真实kind |
| `model` | 必须落到模型级Route |
| `agent` | 必须落到Official Harness Route |

### 5.3 Input与ContentPart

`InputItem`是带类型并集：

| 类型 | 含义 | v1设计状态 |
|---|---|---|
| `message` | system之外的对话消息 | 核心 |
| `tool_result` | 对已知`action_id`的工具结果 | 核心 |
| `artifact_reference` | 已存在产物的引用 | 核心引用入口已实现；Route传输与物化按Adapter分期 |
| `native_extension` | Profile命名空间内无法统一的输入 | 受控扩展 |

`ContextReference`在落地合同时已独立为`UnifiedExecutionRequest.Context[]`，用于表达文件、URL、资源和工作区上下文，不再作为`InputItem` kind。

每个`message`由有序`ContentPart[]`组成：

| ContentPart kind | 内容 |
|---|---|
| `text` | UTF-8文本 |
| `json` | 结构化JSON值和可选Schema身份 |
| `image_ref` | 图像资产引用、媒体类型和尺寸元数据 |
| `audio_ref` | 音频资产引用、媒体类型和时长元数据 |
| `video_ref` | 视频资产引用、媒体类型和时长元数据 |
| `file_ref` | 文件资产引用、媒体类型、名称和用途 |
| `artifact_ref` | Praxis产物引用及版本 |

v1不在公共请求中直接嵌入任意二进制；资产层负责内容寻址、权限、生命周期和实际传输。Profile决定Route支持原生引用、上传、内联还是拒绝。

### 5.4 Role与Instruction

消息Role只表示对话参与者：`user`、`assistant`、`tool`。

高权威指令单独表达为`Instruction`：

```text
Instruction
  - id
  - authority: runtime_policy | developer | task
  - scope: execution | session | turn | action
  - content[]
  - conflict_policy: reject | higher_authority_wins | append
```

调用者不能伪造`profile`、`harness`或`vendor_mandatory`来源。这三层由Profile编译器写入`ContextEnvelopeReport`，而不是混入调用者请求。

建议的有效指令顺序：

```text
Vendor不可见/不可控策略
  > Vendor/Harness强制公开配置
  > Praxis Runtime Policy
  > Semantic Profile注入
  > Developer Instruction
  > Task Instruction
  > User Message
```

如果某Harness真实优先级不同，Profile必须报告`semantic_shift`，不能假装顺序一致。

### 5.5 ContextReference

Context不等同于Message历史：

```text
ContextReference
  - id
  - kind: conversation | workspace | file | artifact | memory | resource
  - reference
  - snapshot/version
  - access: read | write | execute
  - visibility: model | harness | praxis_only
  - required
```

- `required=true`但Route不能提供时拒绝；
- 工作区默认使用快照或明确cwd，不允许Harness偷偷扩展到父目录；
- memory只传引用和经过Context层编译的内容，不让模型Provider直接拥有Praxis记忆存储。

### 5.6 ToolDefinition与执行所有权

Tool必须同时表达“能力是什么”和“谁真正执行”：

```text
ToolDefinition
  - id / name / description
  - kind
  - input_schema / output_schema
  - execution_owner
  - side_effects
  - approval_policy
  - timeout
  - extension
```

`kind`候选并集：

| kind | 例子 | 可能的owner |
|---|---|---|
| `function` | 业务函数 | Praxis/external |
| `mcp` | MCP Server Tool | Praxis/Harness |
| `shell` | 命令执行 | Praxis/Harness |
| `filesystem` | 读写文件 | Praxis/Harness |
| `computer` | UI/浏览器操作 | Praxis/Harness/Provider |
| `hosted` | Web Search、Code Execution | Provider/Harness |
| `agent` | 子Agent或委派 | Praxis/Harness |

`execution_owner`：`praxis`、`harness`、`provider`、`external`。

所有权是不可丢失语义：

- `praxis`工具由统一层接收action request后执行或转交；
- `harness/provider`工具可能只暴露观察事件，Praxis不能重复执行；
- Harness不暴露内部tool call时，Profile只能报告`unobservable`，不能合成虚假调用事件；
- 同名工具但owner不同，不是同一个工具。

### 5.7 ToolPolicy与ApprovalPolicy

```text
ToolPolicy
  - allowed_tool_ids
  - default_approval: always | on_side_effect | on_risk | never
  - parallelism
  - max_actions
  - network_policy
  - workspace_policy
```

Profile只能收紧策略，不能放宽Runtime Policy或厂商不可变限制。Harness只有粗粒度审批时，细粒度审批请求必须拒绝或经调用者允许显式降级。

### 5.8 OutputContract

```text
OutputContract
  - accepted_content_kinds[]
  - text_required
  - json_schema
  - artifact_kinds[]
  - patch_format
  - completion_mode: final | incremental | either
```

- Direct Model Route的JSON Schema可映射为原生Structured Output；
- Harness只支持自然语言约束时属于`transformed`或`degraded`，不能标成exact；
- 文件、补丁和其他产物用Artifact事件返回，不塞进Text假装普通文本。

### 5.9 ReasoningIntent

```text
ReasoningIntent
  - effort
  - budget_tokens
  - summary: none | auto | concise | detailed
  - observable: summary_only | provider_supported
```

Praxis不要求或存储隐藏chain-of-thought。Route只返回厂商明确允许暴露的reasoning summary或公开增量；Harness内部分析不可见时标记`unavailable`。

### 5.10 SessionIntent与State

```text
SessionIntent
  - mode: stateless | new | continue | resume | fork
  - session_id
  - turn_id
  - expected_profile_id/version
  - expected_route_id/version
```

State分为：

| kind | 示例 | 可移植性 |
|---|---|---|
| `model_continuation` | `previous_response_id` | 只在同一Route/Offering/Protocol |
| `provider_continuation` | 厂商opaque payload | 只在同一精确Profile |
| `agent_session` | Codex thread、Claude session、ACP session | 只在同一Harness Profile及兼容版本 |
| `praxis_history` | Praxis重放的显式历史 | 可经Profile重新编译，但不保证行为等价 |

禁止把Agent Session ID降格为普通模型continuation，也禁止在API与Harness之间隐式迁移会话。

### 5.11 ExecutionPolicy

```text
ExecutionPolicy
  - stream
  - sandbox
  - cwd_reference
  - environment_allowlist
  - network_policy
  - user_presence
  - foreground
  - interaction_mode
  - max_concurrency
```

这些字段同时服务合规门禁和Harness准备。`user_presence/foreground/interaction_mode`必须由可信宿主证明，不能只接受普通调用方布尔值。

### 5.12 Budget与DegradationPolicy

`Budget`候选：

- 最大输入/输出token；
- 最大墙钟时间；
- 最大步骤数；
- 最大工具动作数；
- 最大费用及币种；
- 最大订阅计量单位（若厂商公开）。

费用和订阅额度无法可靠预估时，Profile必须报告不可执行的硬预算或降为软告警，不能把token估算冒充真实账单。

原有单一`AllowDegradation bool`不足以表达跨Harness策略。新候选为：

```text
DegradationPolicy
  - default: reject | allow_reported
  - allowed_paths[]
  - forbidden_actions[]
  - require_preflight_ack
```

安全、身份、权限、用途和secret边界永远不可降级。

## 6. PreparedExecutionPlan

Profile编译器输出不可变计划：

```text
PreparedExecutionPlan
  - request_digest
  - semantic_route_profile
  - exact_route_selection
  - execution_kind
  - capability_plan
  - context_envelope
  - harness_preparation
  - tool_ownership_plan
  - state_plan
  - request_mapping_plan
  - expected_event_contract
  - policy_decision
  - entitlement_decision
  - capability_residuals
  - evidence_digest
```

### 6.1 HarnessPreparation

`HarnessPreparation`记录而不是隐藏二开结果：

- 官方组件与版本；
- 采用`vendor_default`、`semantic_stable`或`custom_explicit`模式；
- system prompt/preset的替换或追加方式；
- settings sources、skills、MCP、tools、cwd和sandbox；
- 使用官方扩展点、wrapper、窄补丁、sidecar或fork；
- 补丁digest、许可证和条款审核结果；
- 不可关闭项；
- Expected/Actual InjectionManifest、HarnessDelta与opaque fields；
- Tool的discovered/registered/model-visible/executable/permission/owner状态；
- RouteFingerprint、Harness组件栈、sanitized environment、instruction digest与tool schema digest；
- 版本升级策略和回退版本。

二开等级按风险从低到高：

```text
official configuration
  < public extension point
  < wrapper / adapter
  < narrow maintained patch
  < sidecar
  < long-lived fork
```

每个Profile选择最低足够等级。OAuth身份、allowlist和产品身份不属于可二开项。

### 6.2 ContextEnvelopeReport

```text
ContextEnvelopeReport
  - caller_instructions[]
  - profile_instructions[]
  - harness_configured_instructions[]
  - vendor_mandatory_documented[]
  - settings_sources[]
  - skills_agents[]
  - tools_mcp[]
  - workspace_context[]
  - session_history
  - hidden_context: documented | observed | unknown
  - controllability: full | partial | none
  - expected_injection_manifest
  - actual_injection_manifest
  - harness_delta
  - opaque_fields[]
  - harness_stack_digests[]
  - sanitized_environment_digest
  - field_evidence_quality{}
  - route_fingerprint
```

报告描述“我们能控制和观察什么”，不声称能够证明Provider内部没有隐藏策略。

每次Execution都必须在Provider/model触达前持久化ContextEnvelope/Manifest audit事件。普通用户流可以只返回摘要，但Result必须携带digest、漂移判定与Residual。

### 6.3 MappingReport v2候选

当前model-invoker的Capability级报告继续保留。并集层增加字段路径级决策：

```text
MappingDecision
  - source_path
  - target_path
  - action
  - detail
  - evidence
```

`action`：

| action | 含义 |
|---|---|
| `exact` | 语义和所有权无损保持 |
| `transformed` | 形状变化但语义保持 |
| `configured` | 通过Harness配置实现 |
| `synthesized` | 由Praxis根据权威typed event或Observer确定性投影；不得合成隐藏tool call、approval或usage |
| `degraded` | 已获授权的部分保留 |
| `retained_extension` | 无法统一但在命名空间保留 |
| `rejected` | 执行前拒绝 |
| `unobservable` | 上游行为存在但不可观察 |

不存在`dropped`。任何请求语义都不能静默丢失。

### 6.4 CapabilityResiduals

```text
CapabilityResidual
  - path/capability
  - kind
  - severity
  - impact
  - mitigation
```

`kind`候选：`mandatory_context`、`unknown_context`、`semantic_shift`、`ownership_difference`、`unobservable`、`unsupported`、`version_bound`、`nonportable_state`、`usage_unavailable`。

## 7. UnifiedExecutionEvent

### 7.1 公共事件头

每个事件包含：

```text
EventHeader
  - event_id
  - semantic_version
  - execution_id / session_id / turn_id
  - item_id / parent_id / action_id
  - intent_id / mechanism_plan_id / mechanism_attempt_id
  - effect_id / verification_id / approval_id
  - sequence / source_sequence
  - timestamp / source_timestamp / ingested_at
  - causation_id / correlation_id
  - origin: praxis | model | provider | harness | external
  - family: lifecycle | intent | mechanism | model | item | effect | control | diagnostic
  - visibility: model_visible | user_visible | progress_only | audit_only | private_runtime
  - security_classification: public | internal | sensitive | restricted
  - execution_kind: model | agent
  - profile_id/version
  - route_id/version
  - native_identity
```

### 7.2 ModelEvent

ModelEvent只表达一次模型调用的Provider-neutral流：

| 事件 | 含义 |
|---|---|
| `model_step_started` | 一次模型step开始 |
| `content_started/delta/completed` | 文本或其他ContentPart增量 |
| `reasoning_summary_started/delta/completed` | 厂商允许公开的推理摘要；携带disclosure class |
| `tool_input_started/delta/completed` | 模型生成工具参数 |
| `model_tool_call` | 完整模型工具调用意图 |
| `model_tool_result` | Provider、Harness或Praxis交付给模型的结果；必须报告result origin与是否真实执行 |
| `model_tool_error` | Provider工具或工具结果错误 |
| `model_usage` | 当前模型调用计量 |
| `model_step_completed` | step终止及原因 |
| `model_completed/model_failed` | 模型调用终态 |

ModelEvent不表示命令已经运行、文件已经修改或用户已经批准；这些属于ExecutionItem或ControlEvent。

`model_tool_result`至少携带`result_origin`、`execution_item_id`、`executed`和可选`synthetic_reason`。为协议配对而生成的synthetic/skipped结果只能说明“没有执行”，不得生成Mechanism完成或Effect。

thinking/reasoning事件必须声明`disclosure_class: public_summary | provider_exposed | private_native | unavailable`。只有前两类在厂商政策与RuntimePolicy同时允许时进入普通用户输出；Praxis不采集或推断隐藏chain-of-thought。

### 7.3 ExecutionItem与ItemEvent

`ExecutionItem`是Agent执行过程的权威、可持久化Tagged Union：

| Item kind | 作用 |
|---|---|
| `message` | 用户/Agent消息及phase |
| `reasoning_summary` | 可公开推理摘要快照 |
| `plan` | 计划内容与步骤状态 |
| `tool_action` | 实际工具执行，而非仅模型tool call |
| `command` | 命令、cwd、输出、退出码和时长 |
| `file_change` | proposed/applied diff和文件状态 |
| `mcp_call` | Server、tool、arguments、result/error |
| `hosted_action` | Web Search、Code Execution等上游动作 |
| `artifact` | 文件、图像、报告、patch等产物引用 |
| `subagent` | 子Agent创建、状态与结果引用 |
| `compaction` | 上下文压缩及边界 |
| `extension` | 经Profile命名空间注册的扩展Item |

每个Item拥有自己的合法状态子集，通用超集为：

```text
proposed | pending | running | completed | failed | declined | cancelled | indeterminate
```

可能产生副作用的Item还必须携带`side_effect_state: none | possible | observed | reconciled | unknown`。断流、超时或取消时，只要副作用仍可能存在，就不能把Item直接标成普通`failed/cancelled`。

流式attempt还携带`attempt_id`、`retry_of`、`superseded_by`和`authoritative`。失败或已被替代attempt的文本、tool delta和临时结果不得混入最终authoritative output。

ItemEvent只有三种基本动作：

- `item_started`：创建初始权威Item；
- `item_delta`：对指定Item执行有类型、可顺序重放的增量；
- `item_completed`：给出最终权威Item快照。

delta服务实时UI，不是持久化事实源。恢复、审计和Result投影使用completed Item或显式checkpoint。

### 7.4 ControlEvent与DiagnosticEvent

ControlEvent：

- `approval_requested/resolved`；
- `input_requested/received`；
- `steer_accepted/rejected`；
- `interrupt_requested/dispatched/acknowledged`；
- `cancel_requested/dispatched/acknowledged/cancellation_quiesced`；
- `session_created/resumed/forked/checkpointed`。

Approval必须绑定`approval_id`、`action_id`、`mechanism_attempt_id`、`input_digest`、`action_revision`、scope、authority、expiry与idempotency key。允许决定只对该参数revision有效；参数变化后必须重新审批。取消开始时，所有pending approval失效。

DiagnosticEvent：

- `usage_updated`；
- `mapping_report`；
- `context_envelope`；
- `capability_residual`；
- `route_terminal_observed`；
- `warning`；
- 受控、脱敏和限长的`native_event`。

### 7.5 事件不变量

1. 每个Execution严格有一个`execution_started`；
2. `sequence`严格单调；
3. 每个`item_id/action_id`生命周期顺序合法；
4. 只有一个终态事件；
5. 终态后不得出现新事件；
6. `native_event`不能代替已有统一事件；
7. Harness内部动作不可见时不合成ModelEvent或ExecutionItem；
8. 文件已变更与仅提出变更必须是不同事件；
9. stdout文本、模型文本和命令输出不能全部映成同一个`content_delta`；
10. 事件原始顺序必须保留，跨来源合并时由Praxis分配全局sequence。
11. `model_tool_call`与`tool_action`不能互相替代；只有执行所有权明确后才能从前者触发后者；
12. `item_delta`不能在缺少`item_started`时出现；`item_completed`后同一Item不得再更新；
13. `audit_only/private_runtime`事件不得进入模型上下文或普通用户输出；
14. 已知公共语义不得退化为`native_event`。
15. 上游Model/Harness终态只形成`route_terminal_observed`候选；Effect reconcile与Verification结束后，Praxis才能发唯一Execution终态；
16. 取消请求不等于取消完成；只有进程、工具、审批和后台工作均已quiesced且副作用已reconcile时，才能发`execution_cancelled`；
17. `source_sequence/source_timestamp`只保存原生顺序证据；全局`sequence`按Praxis接收与因果约束分配，不能按跨机器时间戳重排；
18. 唯一Execution终态之后不得产生late observer事件，因此finalize必须等待所有必需Observer与Verifier收口。

## 8. ExecutionCommand

长运行Harness不能只靠Request/Response。统一控制面候选：

| command | 作用 |
|---|---|
| `approve_action` | 批准指定`approval_id/action_id` |
| `deny_action` | 拒绝并带结构化原因 |
| `provide_input` | 回答`input_requested` |
| `cancel_execution` | 请求取消整个Execution |
| `interrupt_execution` | 请求安全暂停，若Route支持 |
| `continue_execution` | 从已暂停状态继续 |
| `provide_tool_result` | 向正在等待的Harness提交与action绑定的工具结果 |

命令必须携带预期Execution状态和幂等键。Profile不支持某控制时，不能假成功；应返回`unsupported_control`。

工具结果交付已经冻结为双路径：Harness实时等待时使用`provide_tool_result`命令；模型级多轮调用仍可在下一轮Request中传`tool_result`。每个Profile必须明确选择恰好一种，不能同时提交。

## 9. UnifiedExecutionResult

Result是终态事件的确定性投影，不是第二套事实源：

```text
UnifiedExecutionResult
  - execution/session/turn identity
  - status / verification_status / stop_reason
  - route_terminal_summary
  - intent_satisfaction[]
  - mechanism_trace[]
  - effects[] / verification_summary
  - final_content[]
  - actions[]
  - artifacts[]
  - workspace_changes[]
  - state_checkpoint
  - usage_metrics[]
  - mapping_report
  - context_envelope_report
  - capability_residuals[]
  - pending_background_work
  - error
```

```text
status: succeeded | partial | failed | cancelled | indeterminate
verification: verified | partially_verified | unverified | contradicted
```

任何字段无法从事件流得到时保持`unavailable`，不以空字符串或零冒充已报告值。`pending_background_work`只有为0且副作用已reconcile时，结果才可进入已确认终态。

## 10. Usage与计量语义

现有模型Token用量继续映射，但跨订阅/Harness需要显式质量：

```text
UsageMetric
  - kind
  - value
  - unit
  - scope
  - source
  - quality: reported | derived | estimated | unavailable
```

候选kind：input/output/reasoning/cache token、API request、premium request、subscription quota、wall time、tool action、cost。

- `reported`：上游明确返回；
- `derived`：由公开确定性字段计算；
- `estimated`：只能用于展示，不能执行硬账务；
- `unavailable`：不能用零代替。

API token、Copilot premium request、订阅quota和费用不是同一种单位，禁止相加形成伪造`total`。

## 11. Error并集

在现有模型错误之上补充执行层分类：

| kind | 例子 |
|---|---|
| `profile_resolution_failed` | 没有唯一合法Profile |
| `profile_incompatible` | Profile组成不相容 |
| `harness_unavailable` | 官方二进制/SDK缺失 |
| `harness_version_mismatch` | 版本不在验证范围 |
| `login_required` | 官方登录缺失或过期 |
| `session_not_found` | Harness会话丢失 |
| `session_incompatible` | 会话与Profile/版本不匹配 |
| `approval_denied` | 用户或策略拒绝动作 |
| `interaction_required` | Route需要用户输入 |
| `tool_failed` | 工具执行失败 |
| `workspace_conflict` | 文件版本或patch冲突 |
| `control_unsupported` | Harness不支持某运行控制 |
| `protocol_violation` | 事件序列或payload违反合同 |
| `semantic_mapping_failed` | 无法忠实编译或解码 |

Error同时记录`phase`：resolve、prepare、encode、connect、execute、decode、control、finalize。未经批准的SDK error、token、环境变量、完整命令输出和敏感文件内容不能进入公共unwrap链。

## 12. 不同执行面的映射示例

| 统一语义 | Direct API | Claude Agent SDK minimal | Codex app-server | ACP Harness | `agy --print` |
|---|---|---|---|---|---|
| Developer Instruction | 原生system/developer或协议转换 | 配置`systemPrompt`并报告SDK强制上下文 | 作为Codex可表达指令，保留Codex固定上下文残余 | ACP prompt/配置，按能力协商 | 拼入单次任务；通常是transformed |
| Function Tool | 模型tool call，Praxis执行 | SDK tool/MCP或Praxis回调，owner显式 | Codex工具配置或MCP；内部工具不重复执行 | ACP能力协商后的动作 | 若CLI不开放结构化工具则rejected/degraded |
| Session | Provider continuation | Claude session | Codex thread/turn | ACP session | conversation能力按CLI实测；否则单次 |
| Approval | 不属于模型API；Praxis外层实现 | SDK权限回调 | app-server审批事件 | ACP approval | 无结构化审批则不宣称支持 |
| File change | 只能由Praxis工具产生 | Harness事件/工作区diff | Codex事件/工作区diff | ACP动作事件 | 仅最终文本时不可合成applied事件 |
| Usage | token reported/unknown | SDK已暴露usage/model_usage/cost，按source/quality记录 | app-server限额/usage分别记录 | 按Agent能力 | 通常unavailable |
| Context controllability | request-controlled，不保证Provider内部无策略 | semantic-stable但仍有SDK残余 | 取决于所选Codex模式与Manifest | 取决于Agent及preflight | opaque vendor-default CLI |

## 13. Semantic Profile的双向合同

每个Semantic Profile至少实现设计上的十二项合同：

1. `Match`：精确匹配Provider、模型/revision、Offering/Auth Route、Deployment/Region、Protocol、ExecutionSurface和Harness组件栈；
2. `ValidateIntent`：检查请求字段、用途和策略；
3. `ProbeCapabilities`：读取静态与动态能力；
4. `BuildExpectedManifest`：形成预期InjectionManifest；
5. `PreflightHarness`：读取实际组件栈、sanitized environment、注入、工具状态、事件保真与opaque项并比较漂移；
6. `PrepareHarness`：形成官方配置/扩展/patch准备计划；
7. `CompileContext`：形成ContextEnvelope与RouteFingerprint；
8. `EncodeRequest`：统一请求到原生请求；
9. `DecodeEvent`：原生事件到统一事件；
10. `EncodeCommand`：统一运行控制到Harness控制；
11. `FinalizeResult`：从事件确定性生成Result；
12. `ExplainResiduals`：报告不可消解差异。

Profile版本升级必须重新验证请求映射、事件序列、上下文、工具所有权、会话续接、审批和错误分类。只改变prompt也属于行为版本变化。

## 14. v1纳入与延期

### 14.1 v1核心设计纳入

- 文本与结构化JSON内容；
- function tool及工具所有权；
- 公开reasoning summary；
- model continuation与agent session；
- 流式内容、动作、审批、工作区、产物、会话、用量和终态事件；
- 运行中审批、输入和取消；
- Profile编译、ContextEnvelope、MappingReport和Residuals；
- Direct Model与Official Harness两种执行目标。

### 14.2 设计保留、首版实现可分期

- 图像、音频、视频和文件ContentPart；
- MCP、shell、filesystem、computer、hosted和agent Tool kind；
- fork/resume/interrupt；
- 费用与订阅quota硬预算；
- patch和长期fork形式的Harness二开；
- 跨设备/远程Harness。

### 14.3 明确不纳入

- 隐藏chain-of-thought抽取；
- 不同Provider/Harness Session的透明迁移；
- 任意Raw事件自动提升为公共语义；
- 使用Profile绕过Credential、Offering、条款或产品身份；
- 把所有Agent动作压成function tool；
- 把CLI最终文本反推成未观察到的工具和文件事件；
- Agent调度策略、多Agent协作算法和长期记忆存储本身。

## 15. 与现有model-invoker的兼容关系

| 现有类型 | 并集层处理 |
|---|---|
| `Request` | 由Direct Model Semantic Profile编译生成；现有字段语义不变 |
| `Response` | 投影为内容、状态、usage、state和诊断事件 |
| `StreamEvent` | 顺序映射为`ModelEvent`；保留原Sequence相对顺序 |
| `CapabilityContract` | 成为PreparedExecutionPlan的模型能力来源之一 |
| `MappingReport` | 嵌入新的字段路径级MappingReport，不删除旧报告 |
| `Error` | 映射到执行层Error并保留安全分类 |
| `State` | 映射为`model_continuation/provider_continuation`，继续禁止跨Route |
| `Usage` | 映射为`quality=reported`或按字段缺失标记unavailable |
| `ProviderOptions` | 仍只属于Adapter命名空间，不能承载Profile prompt或Harness配置 |
| `RawPayload` | 继续只做受控审计，不作为上层语义 |

因此本设计不要求立即修改现有Go公共类型。实现时应先在新的上层合同完成适配，再由独立计划决定哪些加法字段需要下沉。

## 16. 设计验证要求

### 16.1 静态合同

- 每个Tagged Union恰好一个payload；
- 请求和命令可版本化、确定性序列化；
- ID、Sequence和终态不变量可机器验证；
- Profile命名空间外的扩展被拒绝；
- secret和SDK类型不进入公共语义。

### 16.2 Profile契约用例

每个Profile至少具备：

- 同一请求的预期PreparedExecutionPlan golden；
- ContextEnvelope golden；
- 原生请求encode golden；
- 原生事件decode序列golden；
- MappingReport和Residuals golden；
- 非法工具owner、非法State、未知事件和不允许降级反例；
- 版本升级diff。

### 16.3 跨执行面对照

选择同一简单任务分别经Direct API和Harness执行，只比较：

- 请求语义是否被忠实表达；
- 事件合同是否完整；
- 上下文差异是否被报告；
- 工具所有权是否正确；
- 终态和错误是否可解释。

不以文本输出相似度证明“模型纯净”或语义等价。

## 17. v1已闭合决定

本文件原17项开放问题已经由[代表Route纸面编译与跨Route一致性合同v1](./representative-route-paper-compilation-and-union-conformance-v1.md#15-v1设计决定闭合)逐项闭合。关键结果是：

1. 保留五个顶层原语；
2. `execution_kind=auto`只在唯一Route已经解析后使用；
3. v1类型冻结多模态reference tagged union，首个实现切片可只实现text/json/reference；
4. organization规则进入RuntimePolicy provenance，不新增调用者可伪造Role；
5. Tool kind冻结七类并保留命名扩展；
6. 不可控制的Harness内部工具只有在Policy允许、owner已知和Effect可验证时才接受；
7. `provide_tool_result`纳入ExecutionCommand；
8. Usage采用带unit/scope/source/quality的通用Metric；
9. `synthesized`只允许权威observer/typed event的确定性投影；
10. ContextEnvelope每次持久化audit事件；
11. Artifact与Workspace Change属于v1核心；
12. 公共合同逻辑上位于model-invoker之上，不归任一Harness所有；
13. ExecutionItem是Agent持久权威状态；
14. visibility与security classification分开；
15. Direct称`request_controlled`，Harness报告HarnessDelta；
16. 上下文固定三种模式；
17. P0/P1 Manifest漂移在Provider/model触达前fail closed。

## 18. 设计完成与实现前门槛

- [x] 顶层原语和双平面兼容关系已闭合；
- [x] Request字段与Instruction层级已闭合；
- [x] Tool kind、执行所有权和审批语义已闭合；
- [x] Event family、来源顺序、取消与唯一终态不变量已闭合；
- [x] ModelEvent、ExecutionItem、ControlEvent与EffectEvent边界已闭合；
- [x] Session/State不可移植边界已闭合；
- [x] MappingReport、ContextEnvelope、InjectionManifest、HarnessDelta和Residuals已闭合；
- [x] v1核心与实现分期边界已闭合；
- [x] Direct API、Codex、Claude、Gemini、当前Kimi Code和Qwen SDK纸面编译已完成；
- [x] 物理模块名、目录、Go/TypeScript职责与IPC已由独立实现计划确认；本切片为现有Go module内包，不新增Sidecar；
- [x] 代码实现已获得单独授权并完成离线验收；
- [ ] 真实账号或付费验证仍须在用户提供环境后逐Route单独授权。
