# Praxis Intent、Mechanism、Effect与Profile路由 v1 设计候选

## 1. 状态与定位

- 契约候选ID：`praxis.union-call-ime/v1candidate`
- 创建时间：2026-07-12
- 当前状态：v1语义设计已收口，并已由独立计划落地为`union/profile/effect/execution`离线Runtime合同；本文继续作为因果与Profile设计事实源
- 授权演进：本文形成时只授权设计；2026-07-13用户另行授权并完成代码、黑白盒与六路本地集成，真实账号联调仍为`not_run`
- 上位设计：[Praxis执行语义并集v1](./execution-semantic-union-v1-draft.md)
- 研究依据：[Codex、OpenCode、OpenClaw调用原语研究](./codex-opencode-openclaw-semantic-primitives-research-20260712.md)
- 上游行为依据：[上游官方Agent行为与HarnessDelta研究](./upstream-official-agent-behavior-and-harness-delta-research-20260713.md)
- 路由依据：[全量上游支持与Profile并集计划](../../plan/model-invoker/upstream-support-and-profile-union-v1.md)
- 纸面编译依据：[代表Route纸面编译与跨Route一致性合同v1](./representative-route-paper-compilation-and-union-conformance-v1.md)

本文件正式定义：并集类型体系、Intent/Mechanism/Effect协议、三类Profile、Profile合成算法、五类首批能力原语，以及统一事件和结果。

## 2. 不可改变的原则

1. Praxis表达上游能力并集，不取公共交集；
2. 上层表达效果，不表达`apply_patch`、`Edit`、`Write`等工具方言；
3. Profile尊重模型后训练习惯，不强迫不同模型使用相同机制；
4. Mechanism保留真实能力来源和执行所有者；
5. Effect由Runtime真实观测与验证产生，不能只相信模型文字；
6. 缺失能力允许受控模拟，但必须报告来源、修复、重试和残余差异；
7. 安全、权限、条款、身份和secret边界不可降级；
8. 只有必需Intent的后置条件通过验证，Execution才能完全成功。

## 3. 总体类型体系

```text
UnifiedExecutionRequest
  `-- IntentGraph / IntentNode[]
              |
              v
EffectiveProfile
  = ModelBehaviorProfile
  × HarnessCapabilityProfile
  × RuntimePolicy
              |
              v
PreparedExecutionPlan
  |-- IntentResolution[]
  |-- MechanismPlan[]
  |-- VerificationPlan[]
  `-- FallbackPlan[]
              |
              v
Execution
  |-- MechanismAttempt[]
  |-- ModelEvent[]
  |-- ExecutionItem[]
  |-- ControlEvent[]
  `-- EffectRecord[]
              |
              v
UnifiedExecutionResult
  |-- IntentSatisfaction[]
  |-- MechanismTrace[]
  |-- VerifiedEffect[]
  `-- VerificationSummary
```

因果链固定为：

```text
intent_id
  -> mechanism_plan_id
      -> mechanism_attempt_id
          -> effect_id[]
              -> verification_id[]
```

## 4. 公共基础类型

### 4.1 身份

`execution_id`、`session_id`、`turn_id`、`intent_id`、`mechanism_plan_id`、`mechanism_attempt_id`、`effect_id`、`verification_id`、`item_id`、`action_id`和`artifact_id`均为稳定Praxis身份。

Provider、SDK、CLI和Harness原生ID进入命名空间隔离的`NativeIdentity`，不能替代Praxis ID。

### 4.2 能力来源与保真度

`CapabilityOrigin`：

| 值 | 含义 |
|---|---|
| `native` | 模型或正式模型协议原生表达 |
| `provider_hosted` | Provider服务端执行 |
| `harness_hosted` | 官方SDK/CLI/App Server/Harness执行 |
| `caller_hosted` | Praxis或受控执行器提供 |
| `emulated` | Praxis通过校验、修复、重试或组合机制模拟 |
| `unavailable` | 当前精确Route无法提供 |

来源回答“谁提供”，`SemanticFidelity`回答“是否等价”：

```text
exact | transformed | degraded | unavailable
```

例如GPT strict schema是`native + exact`；GLM JSON Object加本地Schema验证是`emulated + transformed`。

### 4.3 执行所有权

`ExecutionOwner`：

```text
model | provider | harness | praxis | external
```

模型只产生内容、选择或工具调用意图，不能直接证明现实副作用。Tool Call和Tool Execution必须分开记录。

### 4.4 Evidence与Verification

```text
EvidenceRef
  - kind
  - source
  - digest
  - captured_at
  - sensitivity
```

Evidence可来自文件快照、stat/hash、unified diff、进程退出、stdout/stderr、Schema验证、截图、DOM/accessibility snapshot、Provider/Harness typed event、API响应或测试结果。

`VerificationStatus`：

```text
pending | verified | partially_verified | unverified | contradicted | not_applicable
```

## 5. Intent协议

### 5.1 IntentNode

```text
IntentNode
  - id
  - kind
  - target
  - specification
  - preconditions[]
  - postconditions[]
  - depends_on[]
  - atomic_group
  - required
  - idempotency
  - conflict_policy
  - accepted_fidelity
  - metadata
```

Intent不得包含厂商工具名、Endpoint、Credential或“必须使用apply_patch/Edit/Bash”等机制要求。

### 5.2 IntentGraph

```text
CreateDirectory(src)
  -> CreateFile(src/main.go)
      -> ModifyFile(go.mod)
          -> ExecuteCode(go test ./...)
```

`atomic_group`表达调用者要求的原子边界。Route无法保证时必须拒绝或显式降级。

### 5.3 前置和后置条件

Precondition可包含路径存在性、before hash、工作区版本、页面状态、用户在场或审批。Postcondition可包含文件hash/内容、JSON Schema、exit code、UI元素、Artifact存在性或禁止副作用。

## 6. Mechanism协议

### 6.1 计划与真实尝试分开

```text
MechanismPlan
  - id / intent_id
  - kind
  - origin / owner
  - selection_authority
  - capability_ref
  - preferred_rank
  - hard_constraints[]
  - expected_effects[]
  - verification_plan_id
  - fallback_plan_ids[]
  - semantic_fidelity
```

`selection_authority`：`runtime`、`model_within_set`、`harness`或`provider`。

```text
MechanismAttempt
  - id / mechanism_plan_id
  - retry_of / superseded_by / authoritative
  - actual_kind / actual_origin / actual_owner
  - native_tool_identity
  - started_at / ended_at
  - status
  - sanitized_input
  - output_refs[]
  - failure_class
  - side_effect_state
```

模型或Harness偏离preferred机制时，必须记录实际机制并重新执行Policy检查。

### 6.2 状态与Fallback

```text
planned -> selected -> awaiting_approval -> running
        -> completed | failed | declined | cancelled | indeterminate
```

`indeterminate`表示可能已有副作用但终态未知。此时禁止直接fallback，必须先reconcile。

Fallback仅在以下条件同时满足时发生：预先列入、有Policy授权、失败为`fallback_safe`、没有未知副作用、重新检查Preconditions。每次fallback产生新的Attempt，结果保留完整尝试链。

## 7. Effect协议

### 7.1 EffectRecord

```text
EffectRecord
  - id
  - intent_ids[]
  - mechanism_attempt_id
  - kind / target
  - before / after
  - evidence_refs[]
  - observation_source
  - verification_status
  - verification_refs[]
  - supersedes_effect_ids[]
  - confidence
  - occurred_at
```

`observation_source`可以是filesystem observer、process supervisor、browser observer、Provider/Harness typed event或external verifier。

模型/Harness自然语言“已完成”只能形成`CompletionClaim`诊断，不能直接形成Verified Effect。

### 7.2 IntentSatisfaction

```text
IntentSatisfaction
  - intent_id
  - status: satisfied | partially_satisfied | unsatisfied | contradicted
  - effect_ids[]
  - missing_postconditions[]
  - residuals[]
```

满足关系由确定性Verifier计算。Effect是不可变事实快照；后续纠正用新Verification或`supersedes_effect_ids`表达。

## 8. 三类Profile

### 8.1 ModelBehaviorProfile

```text
ModelBehaviorProfile
  - id / version
  - exact_model_ids[] / model_family
  - behavior_evidence[]
  - behavior_assertions[] / attribution
  - instruction_strategy
  - tool_selection_behavior
  - structured_output_behavior
  - reasoning_behavior / context_behavior
  - mechanism_preferences{}
  - known_failure_patterns[]
  - benchmark_digest
  - valid_from / evidence_ttl
```

机制偏好按Intent和条件声明，例如小文件创建偏好patch、大文件创建偏好shell/chunked write；Claude创建偏好Write、局部修改偏好Edit。偏好不是能力，必须带证据、版本和TTL。

### 8.2 HarnessCapabilityProfile

```text
HarnessCapabilityProfile
  - id / version
  - execution_surface / harness_name
  - harness_stack[]: component/version/path/binary/protocol digests
  - transparency / instruction_control / context_control
  - expected_injection_manifest / actual_manifest_probe
  - harness_delta / opaque_fields[]
  - available_mechanisms[]
  - tool_surface[]: discovered/registered/model_visible/executable/permission/owner
  - tool_definitions[] / hosted_capabilities[]
  - tool_ownership{}
  - input_output_contracts{}
  - event_observability{}
  - control/session/workspace/approval capabilities
  - limits{}
  - context_envelope
  - probe_digest
```

每个Mechanism必须声明Intent kinds、origin、owner、参数/结果Schema、流、审批、Effect可观测性、幂等性、取消、版本和模型限制。

### 8.3 RuntimePolicy

```text
RuntimePolicy
  - id / version
  - allowed_intents[] / denied_intents[]
  - filesystem / command / network / computer policy
  - approval / verification / secret policy
  - size_thresholds / timeout_limits
  - retry_fallback / concurrency / audit policy
```

RuntimePolicy拥有硬否决权。文件Policy至少包含允许根、保护路径、符号链接、文件大小、总改动量、删除/移动审批、原子写和备份要求。

### 8.4 与Route的关系

```text
RouteEnvelope
  = CredentialProfile + EntitlementProfile + DeploymentProfile

EffectiveProfile
  = Compile(ModelBehaviorProfile, HarnessCapabilityProfile, RuntimePolicy)

SemanticRouteProfile
  = RouteEnvelope + EffectiveProfile + Semantic Encoder/Decoder
```

### 8.5 ProfileSelectionKey

Profile不能只按模型家族选择。精确选择键为：

```text
ProfileSelectionKey
  - provider
  - model_id / model_revision
  - deployment / protocol
  - offering / auth_route
  - execution_surface
  - harness_stack[]
```

RouteEnvelope先解析这组身份，再选取三类Profile实例。三部分合成公式不变；Deployment、Entitlement和Credential也不会被偷塞进ModelBehaviorProfile。

### 8.6 行为归因

官方Agent呈现的是模型、Provider协议、system prompt、工具Schema、上下文发现和Runtime策略的合成行为。`behavior_assertions`中的每一条偏好必须携带证据、版本、TTL和以下归因：

```text
model_intrinsic_claimed
official_harness_induced
tool_schema_induced
provider_protocol_induced
runtime_policy_induced
unknown
```

只有官方模型文档、跨Harness对照或模型专属评测配置共同支持时，才能把候选行为提升为稳定ModelBehavior规则。单个官方Harness的system prompt不能单独证明模型天性。

### 8.7 HarnessDelta与InjectionManifest

```text
HarnessDelta
  = ObservedOfficialAgentContract
  - CallerControlledDirectAPIContract
```

每个Harness调用必须记录Expected与Actual `InjectionManifest`，至少覆盖：instructions、context sources、tool definitions/guidance、permissions、hooks、plugins、skills、MCP、subagents、memory、session、compaction、retry、hosted capabilities和event contract。

不可观测的部分进入`opaque_fields`；不能用空值表示“没有注入”。每个Actual字段声明`reported/observed/inferred_from_config/inferred_from_source/opaque`证据来源。Harness组件栈、Manifest digest、sanitized environment digest与tool schema digest进入RouteFingerprint和审计。

### 8.8 上下文模式

```text
semantic_stable | vendor_default | custom_explicit
```

- `semantic_stable`：最小已知注入、精确工具投影、fresh session和强Effect验证；作为统一调用默认；
- `vendor_default`：保留厂商官方提示、memory和编排，用于其协同优势明显的长任务；
- `custom_explicit`：由用户逐项选择上下文与扩展资产，仍受RuntimePolicy约束。

`semantic_stable`不等于空提示词，也不等于Direct API。它仍需保留工具正确使用、权限、Intent后置条件和验证所需的最小桥接指令。

## 9. Profile合成与路由算法

### 9.1 EffectiveProfile

编译结果包含component digests、intent routing table、exposed mechanism sets、compiled instructions、tool projection、verification/fallback templates、capability origins和semantic residuals。

### 9.2 确定性步骤

对每个Intent：

1. 规范化Intent和目标；
2. 解析完整ProfileSelectionKey；
3. 检查Route、Offering、Entitlement和用途；
4. 生成Expected InjectionManifest；
5. 执行Harness preflight并取得Actual Manifest；
6. 比较版本、注入、工具和策略漂移，按Policy拒绝或标记Residual；
7. 收集Harness与Praxis Runtime可用Mechanism；
8. 删除模型无法调用或Harness未提供的机制；
9. 应用RuntimePolicy硬过滤；
10. 检查Effect可观测性和Verifier；
11. 按有归因证据的ModelBehaviorProfile生成偏好；
12. 计算保真、确定性、安全、观测、验证、Harness差异和资源成本；
13. 选择primary和有序fallback；
14. 生成PreparedExecutionPlan、RoutingDecision与RouteFingerprint。

```text
Available = HarnessMechanisms ∪ RuntimeProvidedMechanisms

Feasible = Available
         ∩ ModelAddressable
         ∩ IntentCompatible
         ∩ PolicyAllowed
         ∩ VerifiableEnough
```

这只是当前精确Route为当前Intent形成的可行集合，不是所有Provider的公共交集。

### 9.3 排序

```text
score = model_affinity
      + semantic_fidelity
      + effect_observability
      + verification_strength
      + determinism + efficiency
      - transformation_cost
      - unknown_harness_delta
      - operational_risk
      - fallback_risk
```

硬过滤永远先于评分。相同输入和Profile digest必须产生相同Plan。

若模型在受控集合内选择工具，Runtime只暴露允许集合，Profile用模型熟悉的描述投影工具，实际选择进入Attempt。偏离偏好不自动算错，但必须满足Policy和可验证性。

## 10. 文件操作原语

### 10.1 Intent

```text
CreateFile | ModifyFile | RewriteFile | DeleteFile
MoveFile | CreateDirectory | DeleteDirectory
```

`FileIntentSpec`包含path/destination、content contract、expected before hash/existence、encoding、line ending、mode、metadata、atomicity、symlink和conflict policy。

`content_contract`：`exact`、`generated`、`transform`或`patch_semantics`。`patch_semantics`描述期望局部变化，不指定patch工具。

### 10.2 Mechanism

```text
apply_patch | text_edit | whole_write | chunked_write
filesystem_api | shell_redirection | shell_move | shell_remove
provider_hosted_shell | harness_hosted_editor
```

工具名只存在于Mechanism/Profile层。

| Profile | 条件 | primary | fallback |
|---|---|---|---|
| GPT | 小文件创建 | apply_patch | whole_write、shell |
| GPT | 大文件创建 | hosted/caller shell或chunked_write | whole_write、分块patch |
| GPT | 局部修改 | apply_patch | text_edit、shell |
| Claude | 创建 | Write/whole_write | Bash |
| Claude | 局部修改 | Edit/text_edit | Bash/patch |
| Claude | 移动/目录 | Bash/filesystem | 其他允许机制 |

实际候选由HarnessCapabilityProfile决定。

### 10.3 Effect与验证

```text
FileCreated | FileChanged | FileRewritten | FileDeleted
FileMoved | DirectoryCreated | DirectoryDeleted
```

Payload保存路径、before/after existence、hash、size、mode、mtime、unified diff ref、binary summary、symlink、Attempt和Verification。

Runtime在动作前后读取真实文件系统。文本且低于限制时生成统一diff；大文件/二进制使用hash和metadata。按Intent继续执行语法、Schema、lint、build或test。模型声明冲突时以真实状态为准。

## 11. 结构化输出原语

### 11.1 Intent

```text
ProduceStructuredOutput
  - schema / schema_digest
  - desired_conformance: exact_schema | valid_json_object
  - repair_allowed / max_repair_attempts
  - final_only / accepted_fidelity
```

上层不指定native或emulated。

### 11.2 Mechanism与Effect

| mechanism | origin | 含义 |
|---|---|---|
| `strict_json_schema` | native | 协议保证Schema约束 |
| `harness_enforced_json_schema` | harness_hosted | Harness使用经验证的Schema终止工具或协议合同保证输出 |
| `schema_bearing_tool` | caller/harness hosted | 通过严格工具参数提交业务结果，仍需Praxis验证 |
| `json_object` | native | 只保证JSON Object |
| `emulated_strict_schema` | emulated | JSON Mode + Praxis验证、修复、重试 |
| `prompted_json` | transformed/degraded | 只通过Instruction约束 |

Direct GPT、Claude、Gemini、Grok可优先provider native strict；具体SDK/CLI Route必须按其公开参数单独判断。传输JSON/JSONL不是业务Schema。GLM、Kimi或不暴露Schema的Harness在Policy允许时转emulated strict。

`StructuredOutputProduced`保存raw/parsed引用、schema digest、JSON/Schema验证、Mechanism、Origin、错误、修复尝试、最终digest和Verification。Emulated不得标记为native。

## 12. Tool Call原语

`ToolCapability`包含semantic tool ID、名称、input/output schema、strictness、side effect、审批、并发和超时；工具面必须拆分`discovered/registered/model_visible/executable/permission_mode/auto_approved/execution_owner/fallback_owner/schema_digest`，不能以单一available布尔值代替。

三个事实严格分离：

```text
ModelToolCall   模型请求调用
ToolExecution  执行器真实运行
ToolEffect     工具导致的真实Effect
```

事件链：tool input start/delta/end → model tool call → execution request/approval/start/progress/end → result delivery → effect observed/verified。

Approval绑定`approval_id`、action/attempt、input digest、action revision、scope、authority、expiry与idempotency key。参数变化后旧approval失效；取消开始后所有pending approval失效。

Tool Result支持text、JSON、ContentPart、ArtifactRef和Error，不再只支持字符串；同时记录`result_origin`、`execution_item_id`、`executed`和可选`synthetic_reason`。Synthetic配对结果不得暗示工具已执行或产生Effect。

## 13. 代码执行原语

`ExecuteCode` Intent包含runtime、代码/Artifact、参数、stdin、输入/输出Artifact、环境、隔离、网络、超时和成功后置条件。

Mechanism：

```text
provider_hosted_code_execution
harness_hosted_shell
harness_hosted_code_tool
caller_hosted_sandbox
caller_hosted_shell
```

GPT/Gemini/Grok可存在provider-hosted路线；Claude官方Agent工具属于harness-hosted；GLM/Kimi在未确认托管执行器时使用caller/harness hosted。

`CodeExecutionCompleted`保存runtime identity、environment fingerprint、exit status、stdout/stderr、Artifacts、文件Effects、网络观测、时长、资源和Verification。上游不报告的字段必须是unavailable，不能用0冒充成功。

## 14. Computer Use原语

`ComputerTask`表达goal、目标应用/资源、允许/禁止动作、不可逆动作Policy、用户在场、成功后置条件和Evidence要求。上层默认不指定坐标。

Mechanism：provider/harness/caller computer use、DOM、accessibility或coordinate action。坐标动作优先级最低；发送、购买、删除、考试等不可逆动作必须审批。

GPT可有provider-hosted；Claude经Harness；Gemini为Preview；Grok、GLM、Kimi未确认Route保持unavailable。

Effect：`ComputerActionObserved`、`ComputerStateChanged`、`ComputerTaskCompleted`。证据包括前后截图、DOM/accessibility snapshot、URL、窗口身份、元素和外部系统回读。模型说“成功”不构成Effect。

## 15. 统一流式事件

```text
UnifiedExecutionEvent
  - semantic/execution/session/turn identity
  - event identity / global sequence / source sequence
  - source timestamp / ingested_at / causation / correlation
  - family
  - visibility / security_classification
  - profile/route identity and digest
  - intent/mechanism/effect correlations
  - payload
```

`family`：

```text
lifecycle | intent | mechanism | model | item | effect | control | diagnostic
```

| family | 主要事件 |
|---|---|
| lifecycle | execution started/paused/resumed/completed/failed/cancelled/indeterminate |
| intent | accepted/rejected/satisfied/partial/unsatisfied |
| mechanism | planned/selected/started/progress/completed/failed/indeterminate/fallback |
| model | content/reasoning/tool-input/tool-call/usage/finish |
| item | item started/delta/completed/indeterminate |
| effect | observed/verification started/verified/unverified/contradicted |
| control | approval/input/steer/interrupt/cancel/quiescence/session |
| diagnostic | profile/mapping/context/route-terminal-candidate/residual/warning/native audit |

规则：Sequence单调；Intent accepted后才能选Mechanism；Attempt started后才能关联Effect；observed不等于verified；Item delta不是最终事实；终态后无事件；事件日志追加写。

上游终态只能先形成`route_terminal_observed`候选。Praxis必须完成取消quiescence、副作用reconcile、后台任务drain和全部必需Verification，之后才能产生唯一Execution终态。取消链固定为`requested → dispatched → acknowledged → quiesced → reconcile → terminal`；任何阶段无法证明静止且可能有副作用时，结果为`indeterminate`。

thinking/reasoning事件携带`disclosure_class: public_summary | provider_exposed | private_native | unavailable`，禁止把Harness private thought自动提升为公开reasoning summary。

## 16. UnifiedExecutionResult

Result包含execution identity、status、verification status、route terminal summary、Intent Satisfaction、Mechanism Trace、Effects、final content、structured output、Artifacts、workspace changes、state、usage、pending background work、mapping/context/residual、Expected/Actual InjectionManifest摘要、RouteFingerprint、Policy和Error。

```text
status: succeeded | partial | failed | cancelled | indeterminate
verification: verified | partially_verified | unverified | contradicted
```

所有required Intent都`satisfied + verified`、后台工作已drain且没有禁止Effect，才是`succeeded/verified`。模型有最终文本但文件未改变时，文件Intent仍unsatisfied。文件改变但连接报错时，Effect仍保留，不能因模型错误丢失副作用。

## 17. 当前模型Profile基线

| 模型基线 | Structured | Tool | Code | Computer | Profile重点 |
|---|---|---|---|---|---|
| GPT-5.6 | strict schema | strict function/MCP/hosted | provider/harness/caller | provider hosted | 按文件大小与修改形状在patch/shell间路由 |
| Claude Fable 5 | strict schema | strict client/server | Harness Bash/Text Editor | Harness Computer | Write/Edit优先，Bash补齐 |
| Gemini 3.5 Flash | strict schema | function | provider code | Preview | Preview/Region/版本门禁 |
| Grok 4.5 | strict schema | function/remote MCP | provider code | 未确认 | 搜索/RAG/代码来源分离 |
| GLM-5.2 | JSON Object | function/MCP | caller/harness | 未确认 | 本地Schema验证修复 |
| Kimi K2.7 Code | JSON Mode | 多步strict tool | caller/harness | 未确认 | 强制thinking与多步循环，Batch不推定 |

精确模型ID、Region、Offering和能力在Profile实例化时必须再次绑定Route证据。

## 18. 安全与审计不变量

1. Intent不能越权指定机制；
2. Profile不能修改Credential、Offering或身份；
3. Mechanism偏离Plan时重新过Policy；
4. 所有副作用有owner；
5. 删除、移动、网络写入和不可逆Computer动作按Policy审批；
6. fallback前证明无未知副作用；
7. Effect有Evidence或明确unverified；
8. secret不进入Intent、公开Mechanism参数、Effect、Diff和普通事件；
9. emulated不得冒充native；
10. Profile、Policy、Harness和Verifier版本进入审计digest。

## 19. 验证设计

- Profile Compiler相同输入/digest产生相同Plan；
- hard constraint先于score；
- 非法fallback被拒绝；
- 模型假完成不会生成Verified Effect；
- 文件hash/diff来自真实快照；
- structured修复链可重放；
- Tool Call/Execution/Effect不串线；
- Computer无证据不能verified；
- 同一File Intent经GPT和Claude可以机制不同，但Effect Schema一致且Satisfaction可比较。
- Expected/Actual InjectionManifest漂移能触发拒绝或显式降级；
- 同一模型经Direct API与不同Harness时不会误命中同一RouteFingerprint；
- 单个官方Harness提示规则不会在缺乏归因证据时升级为模型硬约束。

## 20. v1已闭合决定

原21项开放决定已经闭合，完整决定表见[纸面编译与一致性合同D01-D36](./representative-route-paper-compilation-and-union-conformance-v1.md#15-v1设计决定闭合)。本文件的核心结论全部确认：

1. 因果主轴固定为Intent → Mechanism Plan/Attempt → Effect/Verification；
2. required Intent的Verified Effect决定成功；
3. Origin与Fidelity分开；
4. ModelBehavior偏好必须有证据与归因；
5. Harness必须声明Effect可观测性、事件保真与实际owner；
6. RuntimePolicy拥有硬否决权；
7. 模型只在受控Mechanism集合内自主选择；
8. indeterminate时先reconcile，禁止直接fallback；
9. 文件Effect由真实快照生成；
10. structured output完整报告native/harness/tool/emulated/transport差异；
11. Tool Call、Execution、Result与Effect分开；
12. Computer无外部证据时最多unverified；
13. Event固定八个family；
14. 执行状态与验证状态分开；
15. 本设计是后续公共类型与Profile Compiler的事实源；
16. Profile按完整SelectionKey与Harness组件栈选择；
17. Direct称request-controlled；
18. 官方Agent证据拆分ModelBehaviorCandidate与HarnessDelta；
19. Harness执行前必须Manifest preflight；
20. 上下文固定三种模式；
21. opaque不能当不存在。

## 21. 设计完成与实现前门槛

- [x] 上述决定已闭合；
- [x] 文件、结构化输出、Tool Call、代码执行与Computer Use原语已闭合；
- [x] OpenAI Direct、Codex、Claude、Gemini、当前Kimi Code和Qwen SDK纸面编译已完成；
- [x] 统一事件、取消、终态、Effect与Result投影已闭合；
- [x] negative golden与Route fixture要求已定义；
- [x] 物理模块名、目录和Go/TypeScript边界已由独立实现计划确认；
- [x] v1首批代码切片、测试目录与进程协议已独立审核并完成；
- [x] 编写代码已获得单独授权并完成离线验收；
- [ ] 真实账号验证仍须用户提供环境后逐Route单独授权。
