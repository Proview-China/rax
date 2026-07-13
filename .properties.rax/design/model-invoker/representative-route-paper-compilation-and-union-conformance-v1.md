# Praxis代表Route纸面编译与跨Route一致性合同v1

## 1. 状态、范围与结论

- 合同候选ID：`praxis.route-paper-compile-conformance/v1candidate`
- 完成时间：2026-07-13
- 当前状态：v1语义设计收口资产；其六路编译与跨Route一致性合同已在`tests/conformance/semantic_union_routes_test.go`完成离线实现验证，仍不是生产或live API声明
- 授权演进：本文形成时只授权设计；2026-07-13用户另行授权并完成Runtime代码与离线集成；凭据、账号、真实模型和付费调用仍未执行
- 上位合同：[Praxis执行语义并集v1](./execution-semantic-union-v1-draft.md)
- 三层合同：[Intent、Mechanism、Effect与Profile路由v1](./intent-mechanism-effect-profile-routing-v1-draft.md)
- 上游事实：[官方Agent行为与HarnessDelta研究](./upstream-official-agent-behavior-and-harness-delta-research-20260713.md)
- 设计图源：[并集语义编译与一致性.drawio](./grounding/union-route-compilation-and-conformance-v1.drawio)
- 设计图预览：[并集语义编译与一致性.png](./grounding/union-route-compilation-and-conformance-v1.png)

本合同完成六条代表Route的纸面编译：

1. OpenAI Responses Direct caller-hosted；
2. Codex app-server；
3. Claude Agent SDK；
4. Gemini CLI ACP/headless；
5. 当前Kimi Code ACP；
6. Qwen Code TypeScript SDK。

最终结论是：

> Praxis统一`Intent`、审批、安全、可观测`Effect`、Verification和终态；Profile负责把同一意图编译为每条Route最适合的Mechanism。原生工具名、Agent loop、上下文注入、执行所有权和事件可见度必须保留，不能伪造为相同。

六条纸面编译均使用同一个IntentGraph和RuntimePolicy。它们不比较生成文本是否相似，只检查：计划是否可确定生成、差异是否完整报告、Effect是否能真实观测、失败与取消是否能安全收口。

## 2. 规范性语义

### 2.1 两类执行面

```text
request_controlled
  = 调用者控制公开请求、工具集合和工具loop
  != Provider内部没有任何隐藏策略

harness_composed
  = 模型 + Provider framing + Harness指令/工具/上下文/权限/会话/重试
  != 不可统一
```

Direct API只能称为`request_controlled`。SDK、CLI、ACP和App Server只要拥有Agent loop或注入，就属于`harness_composed`。

### 2.2 Profile注册命名空间与运行时合成

既有八类Profile继续作为存储和引用命名空间：

```text
Credential / Entitlement / Deployment / Model
Harness / Semantic / Invocation / Policy
```

运行时不把八份对象平铺覆盖，而按以下方式派生：

```text
RouteEnvelope
  = CredentialProfile + EntitlementProfile + DeploymentProfile

ModelBehaviorProfile
  = ModelProfile + versioned BehaviorEvidence

HarnessCapabilityProfile
  = HarnessProfile + pinned component stack + preflight probe

RuntimePolicy
  = PolicyProfile + organization/user/workspace/task scoped constraints

EffectiveProfile
  = Compile(ModelBehaviorProfile, HarnessCapabilityProfile, RuntimePolicy)

SemanticRouteProfile
  = RouteEnvelope + EffectiveProfile + SemanticCodec + InvocationOverrides
```

这消除了“八类Profile”与“三部分EffectiveProfile”之间的表面冲突。

### 2.3 完整Route选择键

```text
ProfileSelectionKey
  - provider
  - model_id / model_revision
  - deployment / region / endpoint identity
  - protocol / protocol schema version
  - offering / auth_route
  - execution_surface
  - harness_stack[]
```

`harness_stack[]`中的每个组件都记录：

```text
component / version / executable_path / binary_digest / protocol_schema_digest
```

SDK与其实际启动的CLI必须分别记录。只写一个SDK版本不能证明实际Harness版本。

### 2.4 Profile作用域合并

只有字段合同明确声明为`overridable`时，才允许按`organization → user → workspace → task`逐层细化。合并不是普通last-write-wins：

| 字段类型 | 合并规则 |
|---|---|
| allow集合 | 取交集 |
| deny集合 | 取并集 |
| 预算上限 | 取更小值 |
| Verification要求 | 取更强值 |
| 行为偏好 | 最具体作用域优先，但不能越过硬约束 |
| Credential/Offering/Provider身份 | 不可覆盖，必须重新选Route |
| secret | 只保留引用，不进入导出与事件 |

厂商强制合同、Entitlement与RuntimePolicy拥有硬否决权。

## 3. 统一纸面编译输入

### 3.1 工作区夹具

本合同使用抽象工作区，不依赖真实仓库：

```text
/workspace/go.mod                              exists, read-only for this task
/workspace/internal/config/config.go          exists, sha256 = H_CONFIG_0
/workspace/internal/config/config_test.go     absent
```

符号Hash是golden占位符，不伪装成实际文件摘要。真正实现时由Fixture生成真实内容和Hash。

### 3.2 IntentGraph

| Intent | kind | 规范 | 依赖 | required | accepted fidelity |
|---|---|---|---|---|---|
| `I1` | `ModifyFile` | 在`config.go`中把唯一目标常量由`legacy`改为`strict`，其他字节保持不变；before hash必须是`H_CONFIG_0` | 无 | 是 | exact |
| `I2` | `CreateFile` | 创建`config_test.go`，内容为调用者给定的精确UTF-8字节；目标必须原先不存在 | `I1` | 是 | exact |
| `I3` | `ExecuteCode` | 运行项目声明的Go package test，目标`./internal/config`，网络关闭，成功条件exit code 0 | `I1,I2` | 是 | exact |
| `I4` | `ProduceStructuredOutput` | 产生满足`S_SUMMARY`的最终JSON；允许native、harness-enforced或Praxis emulated strict | `I3` | 是 | exact或transformed |

上层Intent没有出现`apply_patch`、`Edit`、`Write`、`replace`、`Bash`或`run_shell_command`。

### 3.3 RuntimePolicy

```text
filesystem:
  readable_roots  = [/workspace]
  writable_paths  = [config.go, config_test.go]
  follow_symlink  = false
  max_file_size   = 128 KiB
  max_total_delta = 64 KiB
  delete/move     = denied

process:
  allowed_argv    = [["go", "test", "./internal/config"]]
  shell_meta      = denied
  cwd             = /workspace
  network         = denied
  timeout         = 120 s

tool:
  exact action classes = file read, constrained file edit/create, exact test command
  all other executions = denied
  extra model-visible but non-executable tools = only if explicitly degraded and reported

approval:
  pre-authorized  = only the exact compiled actions above
  changed input/action revision = requires new approval
  timeout/unknown = deny

verification:
  filesystem snapshot + hash + unified diff
  Go syntax/semantic predicate
  process exit code and captured output
  JSON parse + S_SUMMARY validation

fallback:
  max one predeclared fallback per Intent
  only after side_effect_state=none or reconciled
  model/Route switch is never a Mechanism fallback

session:
  fresh only; no resume/fork/continuation
```

### 3.4 结构化输出Schema

`S_SUMMARY`要求一个JSON object：

```text
status          = "succeeded"
changed_files   = exactly [config.go, config_test.go]
verification:
  runtime       = "go"
  target        = "./internal/config"
  exit_code     = 0
additionalProperties = false
```

传输层JSON、CLI JSONL envelope和业务结果JSON必须分开；`stream-json`不等于业务Schema已满足。

### 3.5 预期统一Effect

| Effect | 必需证据 | 权威来源 |
|---|---|---|
| `E1 FileChanged` | before/after hash、精确diff、文件metadata | Praxis filesystem observer |
| `E2 FileCreated` | before absent、after hash、精确字节、metadata | Praxis filesystem observer |
| `E3 CodeExecutionCompleted` | argv/runtime identity、exit code 0、stdout/stderr、时长 | process supervisor或等价可信Harness事件，再由Verifier确认 |
| `E4 StructuredOutputProduced` | raw/parsed ref、schema digest、validation result、repair chain | Praxis schema verifier |

Harness diff、自然语言“完成”、计划勾选、`end_turn`、`ResultMessage`或`turn/completed`都不能单独生成这些Verified Effect。

## 4. 纸面编译算法

### 4.1 编译阶段

```text
1 Normalize IntentGraph
2 Resolve unique ProfileSelectionKey
3 Validate Credential/Entitlement/Purpose without reading secret
4 Resolve harness_stack and evidence freshness
5 Build ExpectedInjectionManifest
6 Run local Harness preflight contract
7 Compare Expected vs Actual/Reported/Inferred/Opaque
8 Enumerate Mechanisms and actual ownership
9 RuntimePolicy hard filter
10 Require Observer and Verifier for every required postcondition
11 Rank by evidenced model/harness affinity
12 Select primary and safe fallback graph
13 Freeze PreparedExecutionPlan and all digests
14 Only then allow Provider/model invocation
```

纸面编译不实际执行第14步。每个样例的`ActualManifestProbe.status`均为`not_run`，只冻结“必须如何探测和比较”，不伪造live结果。

### 4.2 ToolSurfaceManifest

工具不能再用一个`available=true`表示。每个工具记录：

```text
discovered
registered
model_visible
executable
permission_mode
auto_approved
execution_owner
fallback_owner
schema_digest
capability_probe
```

`allowed_tools`、permission allowlist、registered tools和model-visible tools不是同一事实。

### 4.3 Manifest证据质量

每个Actual字段携带：

```text
source = reported | observed | inferred_from_config | inferred_from_source | opaque
confidence
evidence_ref
```

源码推导不能伪装成Harness运行时回显。`opaque`不能按空集合处理。

### 4.4 Manifest漂移等级

| 等级 | 例子 | `semantic_stable`处理 |
|---|---|---|
| P0 | auth、Provider、sandbox、执行owner、secret来源变化 | 一律fail closed |
| P1 | system prompt、工具Schema、额外可执行工具、context source变化 | 一律fail closed |
| P2 | 额外但被硬拒绝的model-visible工具、事件可见度下降 | 只有DegradationPolicy明确允许时继续，并形成Residual |
| P3 | 显示文案、非语义telemetry | 审计记录，可继续 |

### 4.5 RouteFingerprint

```text
RouteFingerprint = hash(
  ProfileSelectionKey
  + harness_stack digests
  + ModelBehaviorProfile version
  + Expected/Actual Manifest digests
  + tool registry/schema digests
  + sanitized environment digest
  + RuntimePolicy digest
  + SemanticCodec version
  + context mode
)
```

Request与workspace snapshot有独立digest，不混入可复用Route身份。

### 4.6 排序与tie-break

硬过滤之后才评分：

```text
model/harness affinity
+ semantic fidelity
+ effect observability
+ verification strength
+ determinism
- transformation cost
- opaque harness delta
- operational/fallback risk
```

同分时固定按：保真度、Verification、风险、Profile偏好rank、稳定Mechanism ID排序。相同输入与digest必须得到相同Plan。

## 5. Route一：OpenAI Responses Direct caller-hosted

### 5.1 Route身份

```text
provider          = openai
model             = binding时从已授权Catalog解析的精确ID/revision
deployment        = OpenAI Platform API
protocol          = Responses
offering          = platform_payg
auth_route        = platform_api_key_reference
execution_surface = direct_api
harness_stack     = []
context_mode      = semantic_stable/request_controlled
```

它不消费ChatGPT订阅，也不经Codex。跨到Codex不属于fallback，必须重新解析Route。

### 5.2 Expected Manifest

- Praxis编译的developer/task instructions；
- 精确caller function tool Schema；
- 不启用provider hosted tools；
- Praxis拥有tool loop、审批、文件和process executor；
- 每轮重新编译instructions；不能依赖`previous_response_id`继承上一轮instructions；
- Provider内部安全与路由保持`opaque`。

### 5.3 MechanismPlan

| Intent | primary | fallback | origin/owner | Verification |
|---|---|---|---|---|
| I1 | caller-hosted `apply_patch`方言 | caller filesystem exact text edit | `caller_hosted/praxis` | snapshot、hash、diff、semantic predicate |
| I2 | caller-hosted `apply_patch`创建 | caller filesystem atomic write | `caller_hosted/praxis` | absent→present、exact bytes |
| I3 | caller sandbox exact argv | 无 | `caller_hosted/praxis` | process supervisor |
| I4 | Responses strict JSON Schema | Praxis本地repair/retry | `native/provider`；fallback为`emulated/praxis` | local parse+Schema |

模型function call只是请求。Praxis在Policy与approval通过后执行，观测Effect，再回送`function_call_output`。

### 5.4 事件映射

| Responses | Praxis |
|---|---|
| `response.created` | `model_step_started` |
| output text delta/done | content ModelEvent |
| function arguments delta/done | tool-input ModelEvent；done后形成`model_tool_call` |
| output item add/done | typed model/hosted item；按原类型映射 |
| caller executor lifecycle | MechanismEvent + `tool_action` Item |
| observer/verifier | EffectEvent |
| `response.completed/failed/incomplete` | route terminal candidate；不是Execution终态 |

### 5.5 Fail-closed

- actual model或Endpoint与Profile不符；
- tool call ID或tool result pairing错误；
- 模型调用未产生合法终态；
- 请求了未声明的provider hosted tool；
- before hash冲突；
- strict schema被静默降级；
- caller executor可能已产生副作用但无法reconcile。

## 6. Route二：Codex app-server

### 6.1 Route身份

```text
provider          = openai
model             = thread/start实际回显的精确模型
deployment        = codex_managed
protocol          = app-server v2 + internal Responses
offering          = ChatGPT/Codex subscription
auth_route        = Codex official managed ChatGPT login
execution_surface = app_server
harness_stack     = [codex app-server binary + protocol schema]
context_mode      = semantic_stable
```

### 6.2 Expected Manifest与准备

- Praxis专用`CODEX_HOME`、固定配置与binary digest；
- fresh ephemeral thread，不resume/fork；
- exact cwd、workspace-write sandbox、network false；
- `project_doc_max_bytes=0`关闭AGENTS发现；
- `baseInstructions`为Praxis最小桥接指令，`developerInstructions`为任务合同；
- `instructionSources`必须回显空或完全匹配Expected；
- preflight读取effective config、requirements、skills、plugins/apps、MCP和hooks；
- `dynamicTools`只表示Praxis追加的动态工具，不能声称是全部Codex工具allowlist；
- Codex仍可能加入shell、core utility、collaboration、permissions、personality、skills、extensions和hosted tool说明，必须逐项Manifest或标opaque；
- 禁止使用不继承thread sandbox的`thread/shellCommand`完成模型动作。

### 6.3 MechanismPlan

| Intent | primary | fallback | origin/owner | Verification |
|---|---|---|---|---|
| I1 | Codex `apply_patch` | Codex受控shell替换，仅在可证明无副作用后 | `harness_hosted/harness` | Praxis filesystem observer |
| I2 | Codex `apply_patch`创建 | Codex受控whole write | `harness_hosted/harness` | Praxis observer、exact bytes |
| I3 | Codex turn内sandbox shell | 无 | `harness_hosted/harness` | command Item +独立Verifier |
| I4 | `turn/start.outputSchema` | 本地校验失败则整个Intent失败；不静默prompt-only | `native/provider`经Harness passthrough | parse+Schema |

`outputSchema`只对当前turn生效，不继承到后续turn。

### 6.4 事件映射

Codex稳定app-server暴露Agent Thread/Turn/Item，不暴露稳定模型step边界：

| Codex | Praxis |
|---|---|
| `thread/started` | `session_created` |
| `turn/started` | Agent turn lifecycle；不伪造`model_step_started` |
| `item/started/delta/completed` | 权威ExecutionItem lifecycle |
| command/file/MCP/agent-message Item | 对应typed Item；按actual owner生成MechanismAttempt |
| `turn/diff/updated` | provisional Mechanism evidence，不是File Effect |
| approval server request/decision | Approval lifecycle |
| `turn/completed` | route terminal candidate |
| internal `rawResponseItem` | v1不依赖；只可受控native audit |

### 6.5 Fail-closed

- CODEX_HOME、config requirements、binary或protocol schema漂移；
- instructionSources、skills、MCP、plugin/app或工具面与Expected不符；
- dynamicTools被误当总allowlist；
- EOF且没有`turn/completed`；
- interrupt已受理但未观察turn终止与副作用静止；
- 只有provisional diff，没有真实文件快照；
- actual model/provider变化。

## 7. Route三：Claude Agent SDK

### 7.1 Route身份

```text
provider          = anthropic
model             = init/assistant实际回显的精确模型
deployment        = claude_code_managed
protocol          = Agent SDK stream-json + control
offering          = Claude subscription
auth_route        = official local Claude login
execution_surface = agent_sdk
harness_stack     = [Agent SDK, resolved Claude CLI]
context_mode      = semantic_stable
```

SDK版本、resolved CLI path/version/digest必须分别固定。CLI最低版本检查只有告警能力，不能代替Praxis fail-closed。

### 7.2 Expected Manifest与环境

```text
system_prompt          = Praxis最小桥接指令，不是空字符串
setting_sources        = []
skills                 = []
tools                  = exact [Read, Edit, Write, Bash]
allowed_tools          = only pre-authorized subset
strict_mcp_config      = true
mcp_servers            = {}
plugins                = []
agents                 = none
session                = fresh
include_partial        = true
include_hook_events    = true
cwd                    = /workspace
```

`tools`决定真实可用性，`allowed_tools`只表示预授权。Praxis保留必要的PreToolUse/PostToolUse审计hook；空hooks不是安全优势。

SDK默认继承几乎全部父进程环境。preflight必须形成sanitized environment digest，并移除或拒绝冲突的API Key、base URL、Bedrock/Vertex/Foundry、proxy、helper与`CLAUDE_CONFIG_DIR`。订阅Route禁止使用跳过OAuth/keychain的`--bare`。

### 7.3 MechanismPlan

| Intent | primary | fallback | origin/owner | Verification |
|---|---|---|---|---|
| I1 | Claude `Edit` | `Write`，仅在exact content与before hash允许时 | `harness_hosted/harness` | Praxis observer |
| I2 | Claude `Write` | 无 | `harness_hosted/harness` | exact bytes |
| I3 | Claude `Bash` exact command | 无 | `harness_hosted/harness` | hook/item + process evidence/独立Verifier |
| I4 | Agent SDK `output_format=json_schema` | Praxis emulated repair | `native/provider`经CLI Harness | `ResultMessage.structured_output` + local Schema |

### 7.4 事件映射

| Claude SDK | Praxis |
|---|---|
| `SystemMessage(init)` | session/Actual Manifest candidate |
| stream `message_start` | `model_step_started` |
| text/thinking/tool delta | ModelEvent；thinking带严格disclosure class |
| `AssistantMessage` | 当前model step权威快照 |
| `can_use_tool` | approval request |
| Pre/PostToolUse hook | Mechanism/Item evidence |
| `ToolResultBlock` | model tool result；不直接证明Effect |
| Task消息 | subagent/background Item |
| `ResultMessage` | route terminal candidate、usage、structured output candidate |

SDK parser跳过未知message type，因此版本或Schema未知时，strict Route必须fail closed，不能悄悄继续。

### 7.5 Fail-closed

- resolved CLI与SDK声明不符；
- sanitized environment表明实际将走API Key、云Deployment或代理Route；
- init tools/MCP/model/cwd与Expected不符；
- 自动允许规则绕过必须逐次审批的Policy；
- structured output缺失、refusal或max-token终止；
- stream没有`ResultMessage`；
- `is_error/subtype`组合无法解释；
- interrupt后存在未收口工具或后台任务。

## 8. Route四：Gemini CLI ACP/headless

### 8.1 Route身份

```text
provider          = google
model             = init/actual config解析的精确模型
deployment        = code_assist | gemini_api | vertex_ai
protocol          = ACP JSON-RPC/NDJSON（canonical）或headless stream-json
offering/auth     = 与deployment精确绑定
execution_surface = official_cli/acp_agent
harness_stack     = [gemini-cli binary, ACP schema]
context_mode      = semantic_stable
```

不同登录、AI Studio与Vertex不得共用Profile。

### 8.2 Expected Manifest

- 独立`GEMINI_CLI_HOME`；
- `GEMINI_SYSTEM_MD`完整替换主system prompt；
- `context.includeDirectoryTree=false`、includeDirectories空、禁止额外memory目录；
- auto memory、agents、skills、hooks关闭；
- `tools.core`为`read_file/replace/write_file/run_shell_command`精确内建集合；
- discovery/call command未设置，MCP为空，启动`-e none`；
- fresh session；
- 首条`<session_context>`仍可能包含日期、OS、临时目录与session memory，必须进入Manifest；
- `tools.allowed`若存在只表示免确认，不当作注册工具allowlist。

### 8.3 MechanismPlan

| Intent | primary | fallback | origin/owner | Verification |
|---|---|---|---|---|
| I1 | `replace` | `write_file` only after reconcile | ACP工作区内可为`caller_hosted/praxis`，否则Harness；按Attempt记录 | Praxis observer |
| I2 | `write_file` | 无 | 同上 | exact bytes |
| I3 | `run_shell_command` | 无 | `harness_hosted/harness` | command evidence + Verifier |
| I4 | schema-bearing SubmitResult或本地repair | 无native CLI业务Schema承诺 | `caller/harness hosted`或`emulated/praxis` | local Schema |

CLI的`json/jsonl/stream-json`只描述传输，不等于strict structured output。

### 8.4 事件映射

| Gemini | Praxis |
|---|---|
| headless init/session context | diagnostic + Manifest candidate |
| headless message | content event |
| headless tool_use/result | 只有协议明确为模型block时映射ModelEvent，同时生成独立Item |
| ACP agent message/thought | content；thought默认private，不自动当公开reasoning summary |
| ACP tool call/update | Mechanism/Item lifecycle；是否能提升model_tool_call由event fidelity决定 |
| ACP requestPermission | approval lifecycle |
| diff content | provisional evidence，不是File Effect |
| result/stopReason | route terminal candidate |

当前ACP会顺序执行多个tool request；Profile不得把模型并行建议伪装成Harness并行能力。同一文件的修改固定串行。

### 8.5 Fail-closed

- 首用户session context或memory来源漂移；
- extension/MCP/discovery/agent/tool registry额外扩展；
- ACP filesystem capability与预期owner不符；
- malformed stream被Harness投影为`end_turn`；
- stopReason存在但tool仍未终态；
- diff没有真实observer证据；
- required schema只返回传输JSON。

## 9. Route五：当前Kimi Code ACP

### 9.1 新旧Route必须分开

当前官方主项目是`MoonshotAI/kimi-code`。旧Python `MoonshotAI/kimi-cli`已明确进入逐步停止阶段。

```text
kimi_code_current
  = Kimi Code CLI / ACP 0.23 / stream-json / current tools

kimi_cli_legacy_pinned
  = legacy Agent spec / Wire 1.10 / legacy tools
```

旧版`--agent-file`、Wire、StrReplaceFile等能力不能写入当前Route。Legacy只能作为固定版本兼容Profile单独维护。

### 9.2 Route身份

```text
provider          = moonshot
model             = Kimi Code config/ACP实际解析的精确模型
deployment        = kimi_code_managed | moonshot_open_platform
protocol          = ACP 0.23（canonical）
offering/auth     = official device OAuth或明确API credential route
execution_surface = official_cli/acp_agent
harness_stack     = [kimi-code CLI, ACP adapter, embedded SDK]
context_mode      = semantic_stable with reported residuals
```

### 9.3 Expected Manifest与已知残余

- 独立`KIMI_CODE_HOME`；必要时同时隔离OS HOME/容器；
- fresh session；
- `--skills-dir`指向受控空目录；
- KIMI_CODE_HOME与workspace内plugins、MCP、hooks、AGENTS、skills清空或精确hash；
- `~/.agents`与项目AGENTS来源必须预检；单独KIMI_CODE_HOME不会自动隔离它们；
- 关闭cron和不需要的后台能力，完成前drain全部background work；
- fixed provider/model/config/version；
- manual permission，经ACP approval桥接；禁止使用print mode隐式auto审批；
- 当前公开CLI没有完整system prompt override，也没有精确tool registry allowlist；这些形成HarnessDelta。

当前Route只有在DegradationPolicy明确允许“额外model-visible工具、但所有未批准执行都fail-closed”时才能编译。若调用者要求实际registry精确等于Expected，则在执行前拒绝。

### 9.4 MechanismPlan

| Intent | primary | fallback | origin/owner | Verification |
|---|---|---|---|---|
| I1 | Kimi `Edit` | `Write` only after reconcile | ACP workspace FS由client/Praxis；否则Harness，按Attempt记录 | Praxis observer |
| I2 | Kimi `Write` | 无 | ACP FS为`caller_hosted/praxis` | exact bytes |
| I3 | Kimi `Bash` | 无 | 终端reverse-RPC未连接，当前为`harness_hosted/harness` | process evidence + Verifier |
| I4 | final text/SubmitResult本地校验修复 | 无公开CLI output schema | `emulated/praxis` | local Schema + repair chain |

### 9.5 事件映射

当前ACP主要暴露Agent语义：

| Kimi ACP | Praxis |
|---|---|
| assistant/thought chunk | content/private reasoning item |
| tool call/update | Mechanism + tool_action Item；不默认伪造raw ModelEvent |
| tool progress/result | Item progress/terminal |
| plan | plan Item |
| request_permission | approval lifecycle |
| `turn.ended` | route terminal candidate |

Kimi ACP可能把普通failed压成`end_turn`并只在日志保存错误，所以终态必须结合错误、pending tools、background work与Effect验证判断。

### 9.6 Fail-closed

- 在current Route请求legacy Wire/agent-file；
- KIMI_CODE_HOME隔离但真实HOME下`.agents`未受控；
- 实际tool registry无法证明且调用者要求精确registry；
- print mode隐式auto与RuntimePolicy冲突；
- 未批准plugin/MCP/AgentSwarm/background/cron可执行；
- ACP terminal到达但工具或后台任务未drain；
- cancel后Bash或文件动作仍可能运行；
- CLI/ACP没有业务Schema却被标成native strict。

## 10. Route六：Qwen Code TypeScript SDK

### 10.1 Route身份

```text
provider          = alibaba或实际配置Provider
model             = SDKSystemMessage.actual model
deployment        = 实际base URL/region
protocol          = Qwen SDK bidirectional stream-json
offering/auth     = PAYG或Coding Plan + actual authType
execution_surface = official_sdk
harness_stack     = [@qwen-code/sdk, bundled Qwen CLI]
context_mode      = semantic_stable.bare_fixed
```

Qwen SDK允许多种auth/provider形状。provider、authType和offering必须取实际值，不能只按“Qwen模型”选Profile。

### 10.2 `bare_fixed`与`controlled_nonbare`

当前源码中`--bare`会忽略`coreTools`覆盖，固定注册：

```text
read_file / edit / notebook_edit / run_shell_command
```

因此两种合法模式是：

| 模式 | 工具面 | 上下文 |
|---|---|---|
| `semantic_stable.bare_fixed` | 固定bare集合，可用exclude做减法 | 最小隐式配置，默认 |
| `controlled_nonbare` | `coreTools`精确注册 | 必须额外隔离memory、extensions、hooks、skills、MCP、agents |

`coreTools + extraArgs=[--bare]`是非法设计组合，Profile Compiler必须拒绝。

### 10.3 Expected Manifest

Canonical使用`bare_fixed`：

- `systemPrompt`为Praxis最小桥接指令，完整覆盖主prompt；
- `coreTools`不设置；
- `excludeTools=[notebook_edit]`；
- 最终工具面为read/edit/shell，须由首个`SDKSystemMessage.tools`回读确认；
- MCP、managed memory、skills、hooks、agents、extensions关闭/空；
- fresh session、partial messages开启；
- model fallback关闭；
- max tool calls/turns显式限制；
- 启动日期、OS、cwd、目录树与git status仍可能注入，进入Manifest；
- user memory与隐式QWEN规则必须确认为禁用。

### 10.4 MechanismPlan

| Intent | primary | fallback | origin/owner | Verification |
|---|---|---|---|---|
| I1 | Qwen `edit` | 无安全自动fallback | `harness_hosted/harness` | Praxis observer |
| I2 | `edit`且`old_string`为空以创建 | 无 | `harness_hosted/harness` | exact bytes |
| I3 | `run_shell_command` exact argv | 无 | `harness_hosted/harness` | tool result + Verifier |
| I4 | SDK最终结果 + Praxis校验/repair | Qwen headless `--json-schema`属于另一Route，不在SDK Route冒充native | `emulated/praxis` | local Schema |

SDK的`fallbackModel`保持关闭。任何模型切换必须重新解析Profile并生成新RouteFingerprint。

### 10.5 事件映射

| Qwen SDK | Praxis |
|---|---|
| `SDKSystemMessage` | session + Actual Manifest candidate |
| partial message/content block | model content/reasoning/tool-input events |
| `ToolUseBlock` | `model_tool_call` |
| `can_use_tool` | approval lifecycle；timeout/exception自动deny |
| Harness tool lifecycle | Mechanism + tool_action Item |
| `ToolResultBlock` | model tool result，带actual action correlation |
| `SDKResult` | route terminal candidate、usage、denials |
| interrupt/abort | control request；随后必须reconcile |

### 10.6 Fail-closed

- bare与coreTools同时配置；
- SDKSystemMessage的cwd/tools/model/version不匹配；
- actual auth/provider与Profile不匹配；
- model fallback被意外启用；
- fixed bare工具被新增或无法通过exclude缩减；
- stream/CLI退出时存在未配对tool use；
- cancellation未确认quiescence；
- SDK Route被错误标记为支持CLI headless `--json-schema`。

## 11. 跨Route一致性矩阵

### 11.1 同一Intent的Mechanism差异

| Route | ModifyFile | CreateFile | ExecuteCode | Structured Output |
|---|---|---|---|---|
| OpenAI Direct | caller apply-patch | caller patch/write | caller sandbox | provider native strict |
| Codex app-server | Harness apply_patch | Harness patch/write | Harness sandbox shell | provider native strict passthrough |
| Claude SDK | Edit | Write | Bash | provider native strict passthrough |
| Gemini CLI ACP | replace | write_file | run_shell_command | emulated/schema tool |
| Kimi Code ACP | Edit | Write | Bash | emulated |
| Qwen SDK bare | edit | edit-create | run_shell_command | emulated |

差异全部进入MechanismTrace、Origin、Owner、NativeIdentity和Residual；Effect Schema保持一致。

### 11.2 可见性与事件保真

| Route | 模型step | Agent Item | Tool执行 | 原生终态 |
|---|---|---|---|---|
| OpenAI Direct | 明确typed ModelEvent | caller生成Item | Praxis | Responses terminal candidate |
| Codex app-server | stable面不暴露，禁止伪造 | 明确typed Item | Codex/Harness或dynamic callback | turn terminal candidate |
| Claude SDK | partial/raw stream可见 | hooks/Task/Result | Claude CLI或SDK MCP | ResultMessage candidate |
| Gemini headless/ACP | 取决于事件合同；ACP偏Agent | ACP tool/plan | ACP client或CLI | stopReason/result candidate |
| Kimi Code ACP | 不保证raw model step | 明确ACP tool/plan | ACP client或Harness | turn.ended candidate |
| Qwen SDK | partial blocks可见 | Harness tool lifecycle | Qwen Harness | SDKResult candidate |

Profile必须声明`event_fidelity`。缺失的模型step、retry、compaction或usage事件保持unavailable，不能补造。

### 11.3 统一Tool Call链

```text
ModelToolCall
  -> ApprovalRequest(input_digest + action_revision)
  -> MechanismAttempt
  -> ToolExecutionItem
  -> ToolResult(result_origin + executed)
  -> EffectObservation
  -> Verification
```

每个tool call最终必须有真实、declined、cancelled、skipped或synthetic结果之一。synthetic结果只用于协议配对，必须`executed=false`，不得产生Effect。

### 11.4 统一取消链

```text
cancel_requested
  -> cancel_dispatched
  -> cancel_acknowledged
  -> cancellation_quiesced
  -> effect_reconciliation_started
  -> effect_reconciliation_completed
  -> execution_cancelled | execution_indeterminate
```

取消开始时所有pending approval立即失效。Harness确认interrupt只表示控制已受理，不表示工具、子Agent或后台任务已停止。

### 11.5 统一终态投影

| 条件 | Execution status | Verification |
|---|---|---|
| 所有required Intent satisfied且verified，无禁止Effect | `succeeded` | `verified` |
| 已有部分verified Effect，但required Intent缺失 | `partial`或`failed`，按Policy | `partially_verified` |
| approval denied且无fallback | `failed` | 保留已有验证状态 |
| 取消且quiescence已确认 | `cancelled` | 保留取消前Verified Effects |
| 断流/取消/超时且副作用未知 | `indeterminate` | `unverified/partially_verified` |
| 禁止Effect真实发生 | `failed` | `contradicted` |
| transport失败但全部required Effect已验证 | 由Intent决定，可成功；transport error作为诊断保留 | `verified` |

上游terminal永远只是candidate。所有Observer、Verifier和background drain完成后，Praxis才发唯一Execution终态。

## 12. 首批详细原语的最终补充

### 12.1 文件操作

文件Effect只由实际文件系统前后状态产生。Harness diff是Mechanism evidence。`EffectRecord`必须支持`supersedes_effect_ids[]`，以表达后续reconcile纠正早期观测。

### 12.2 结构化输出

```text
transport_json
provider_native_schema
harness_enforced_json_schema
schema_bearing_tool
json_object
emulated_json_validated
prompted_json
```

Origin、Fidelity、raw output、repair attempt与最终Schema validation必须完整报告。

### 12.3 Tool Call

Tool manifest拆分发现、注册、模型可见、可执行、许可和owner。审批绑定参数revision；执行owner按每个MechanismAttempt记录，不能只在Route上静态声明。

### 12.4 代码执行

`provider_hosted`、`harness_hosted`与`caller_hosted`分别保留。缺失exit code、runtime identity或网络观测时字段为unavailable。测试命令输出不能混成模型content。

### 12.5 Computer Use

Computer Use不在本次canonical coding IntentGraph中，但必须遵守同一编译合同：

| Route | 当前纸面Profile |
|---|---|
| OpenAI Direct | 只有精确模型/Route声明provider computer tool时才可编译 |
| Codex app-server | 不从Codex一般工具面推定；需实际Manifest |
| Claude SDK | 只有tools显式包含Computer且Policy允许时 |
| Gemini CLI | Gemini API Preview能力不能自动继承给CLI Route |
| Kimi Code/Qwen SDK | 当前代表Profile为unavailable |

无外部状态回读时最多`unverified`。不可逆动作始终审批，坐标机制优先级最低。

## 13. 统一事件与结果协议收口

### 13.1 EventHeader

```text
event_id
execution/session/turn IDs
intent/mechanism plan/attempt/effect/verification/approval IDs
global sequence
source sequence + source timestamp
ingested_at
causation_id + correlation_id
origin
family
visibility
security_classification
profile/route/fingerprint
native_identity
payload
```

`family`固定为：

```text
lifecycle | intent | mechanism | model | item | effect | control | diagnostic
```

### 13.2 Attempt与retry

流式attempt必须有：

```text
attempt_id
retry_of
superseded_by
authoritative
side_effect_state
```

失败attempt的文本或tool delta不能混入最终authoritative output。Legacy Kimi StepRetry、Provider retry和Harness auto-continue都按此表示。

### 13.3 Reasoning disclosure

每个thinking/reasoning事件携带：

```text
disclosure_class = public_summary | provider_exposed | private_native | unavailable
```

只有厂商允许公开且Profile声明的内容进入普通用户输出。隐藏chain-of-thought不采集、不推断。

### 13.4 ContextEnvelope发布

每次Execution开始都必须持久化`ContextEnvelope/Manifest` audit事件；普通用户流可隐藏全文，但Result必须包含摘要与digest。P0/P1漂移必须发生在Provider/model调用前。

### 13.5 UnifiedExecutionResult

Result是事件确定性投影，至少包含：

- Execution与route terminal状态分别记录；
- IntentSatisfaction；
- authoritative MechanismTrace与全部失败attempt；
- Verified/Unverified/Contradicted Effects；
- final content与structured output；
- artifacts与workspace changes；
- pending background work必须为0或明确indeterminate；
- usage metric的source与quality；
- Expected/Actual Manifest摘要、RouteFingerprint、MappingReport和Residual；
- error与stop reason。

## 14. Negative golden与验收

### 14.1 必需negative cases

| ID | 输入/故障 | 预期 |
|---|---|---|
| N01 | Route selector匹配多个Profile | `profile_resolution_failed`，不触达上游 |
| N02 | Manifest出现额外可执行工具 | semantic_stable fail closed |
| N03 | before hash不符 | `workspace_conflict` |
| N04 | approval后参数revision变化 | 原approval失效，重新审批 |
| N05 | 文件动作后断流，副作用未知 | reconcile；无法确定则`indeterminate` |
| N06 | invalid structured output | 按Policy repair；耗尽后Intent失败 |
| N07 | cancel已受理但进程未静止 | 不得发cancelled terminal |
| N08 | 当前Kimi Route请求legacy Wire | `profile_incompatible` |
| N09 | Qwen bare同时配置coreTools | compile-time reject |
| N10 | Harness synthetic tool result | `executed=false`，无Effect |
| N11 | Codex provisional diff无文件变化 | File Intent unsatisfied |
| N12 | Gemini/Kimi `end_turn`但required verifier失败 | Execution failed/contradicted |
| N13 | 自动模型fallback发生 | 终止当前Route，重新解析Profile/Fingerprint |
| N14 | unavailable Computer Use Intent | capability reject，不转shell伪装等价 |

### 14.2 每个Route的离线fixture与后续live fixture

2026-07-13首个实现切片已经为六条代表Route落地可重复的离线合同与fake fixture，覆盖PreparedExecutionPlan、Expected/Actual Manifest、native request或CLI参数、native event decode、统一事件序列、Effect/Verification投影及上述negative cases的适用子集。

仍属于后续真实联调或版本维护范围的是：

1. 由固定版本官方二进制和真实账号产生的Actual Manifest probe；
2. 官方组件升级前后的semantic diff；
3. 真实Offering、entitlement、quota与付费黑盒证据。

## 15. v1设计决定闭合

| ID | 已闭合决定 |
|---|---|
| D01 | 因果主轴固定为`IntentGraph → MechanismPlan/Attempt → Effect/Verification` |
| D02 | 五个顶层原语固定为Request、PreparedPlan、Event、Command、Result |
| D03 | 成功由required Intent的Verified Effect决定 |
| D04 | CapabilityOrigin与SemanticFidelity分开 |
| D05 | ModelBehaviorProfile只保存带证据、归因、TTL的偏好，不把偏好冒充能力 |
| D06 | HarnessCapabilityProfile必须声明Effect可观测性、事件保真和实际owner |
| D07 | RuntimePolicy拥有最终硬否决权 |
| D08 | 模型可在Runtime暴露的受控Mechanism集合内自主选择 |
| D09 | 未知副作用时禁止自动fallback，先reconcile |
| D10 | 文件Effect由真实快照、hash和diff生成 |
| D11 | structured output完整区分native、harness、tool、emulated和transport JSON |
| D12 | Tool Call、Tool Execution、Tool Result与Tool Effect分离 |
| D13 | Computer无外部状态证据时最多unverified |
| D14 | Event固定八个family，执行状态与验证状态分离 |
| D15 | Profile按完整SelectionKey与Harness组件栈选择 |
| D16 | Direct API称`request_controlled`，不作“裸模型”承诺 |
| D17 | 官方Agent证据拆成ModelBehaviorCandidate与HarnessDelta |
| D18 | Harness执行前必须Expected/Actual InjectionManifest preflight |
| D19 | context mode固定`semantic_stable/vendor_default/custom_explicit` |
| D20 | opaque字段不能当不存在 |
| D21 | `execution_kind=auto`保留，但只能在唯一Route已经解析后决定model/agent，禁止隐式选便宜路线 |
| D22 | v1类型冻结text/json和多模态reference tagged union；首个实现切片可只实现text/json/reference |
| D23 | 调用者Instruction保留runtime_policy/developer/task；organization规则进入RuntimePolicy provenance，不新增可伪造role |
| D24 | Tool kind冻结function/mcp/shell/filesystem/computer/hosted/agent，并保留Profile命名扩展 |
| D25 | Harness内部工具不可控制时，只有Policy允许、owner已知且Effect可验证才可继续 |
| D26 | `ExecutionCommand`纳入`provide_tool_result`；Profile在command与next Request两种交付方式中二选一 |
| D27 | Usage采用通用强类型Metric ID+unit+scope+source+quality，不跨单位相加 |
| D28 | Mapping `synthesized`只允许由权威observer/typed event确定性投影；禁止合成隐藏tool call、approval、usage |
| D29 | ContextEnvelope每次持久化audit事件，普通用户只需摘要 |
| D30 | Artifact与Workspace Change属于v1核心Item/Effect |
| D31 | ExecutionItem是Agent持久权威状态；ModelEvent与delta是顺序流 |
| D32 | visibility与security classification分开；secret payload禁止进入普通事件 |
| D33 | 上游terminal是candidate；唯一统一终态必须晚于reconcile与Verification |
| D34 | 公共合同逻辑上属于model-invoker之上的Execution Semantic Union，不由任何单一Harness拥有 |
| D35 | Profile作用域使用约束合并而不是last-write-wins |
| D36 | 实现模块名、物理目录与Go/TypeScript IPC属于独立实现计划门，不影响本语义设计完成 |

## 16. 设计完成边界与下一门槛

本部分现在已经完成：

- 上游正式路线与Harness差异研究；
- Profile职责、选择、合成和漂移策略；
- Intent/Mechanism/Effect类型主轴；
- 五类首批能力原语；
- 统一事件、取消、终态和Result投影；
- 六条代表Route纸面编译；
- cross-route conformance与negative golden要求；
- v1设计决定闭合。

设计完成后的实施演进已经闭合前三项原实现门槛：2026-07-13独立实施计划确认物理边界为现有Go module内的`union/profile/effect/execution`及受控进程协议，并完成Direct与首批Harness Adapter；本切片不需要TypeScript sidecar。

仍未执行且不属于离线完成声明的事项：

1. 使用真实账号、OAuth、API Key或订阅额度；
2. 对固定版本官方组件执行live Manifest probe与真实黑盒测试；
3. 生产容量、条款、安全和运维评审。

后续新增Harness仍需独立切片、版本锁定和用户授权；现有离线实现不自动授权任何真实调用。

## 17. 官方证据基线

本轮固定源码快照：

| 上游 | commit |
|---|---|
| OpenAI Codex | `9e552e9d15ba52bed7077d5357f3e18e330f8f38` |
| Claude Agent SDK Python | `528265fa09da954f0a0da1bf31e16db32b510138` |
| Claude Agent SDK TypeScript | `79b6350e13cf24af94a8d2e696a0883fd8cc55fe` |
| Gemini CLI | `f354eebaf43b25bacb176007e449bb9a638fd101` |
| Kimi Code current | `ceb158dc54586f254819edbc83c27e21dca1ecf6` |
| Kimi CLI legacy | `2c34efbbc6c7cfe40770623281e87c138ff8eb6c` |
| Qwen Code | `92b47a4e014611007bbd11ca6b6707e17f103f05` |

公开官方入口：

- OpenAI：[Codex app-server](https://learn.chatgpt.com/docs/app-server)、[Function calling](https://developers.openai.com/api/docs/guides/function-calling)、[Responses streaming](https://developers.openai.com/api/docs/guides/streaming-responses)
- Anthropic：[Claude Agent SDK Python](https://github.com/anthropics/claude-agent-sdk-python)、[Structured Outputs](https://platform.claude.com/docs/en/build-with-claude/structured-outputs)
- Google：[Gemini CLI](https://github.com/google-gemini/gemini-cli)、[ACP mode](https://geminicli.com/docs/cli/acp-mode)、[Headless](https://geminicli.com/docs/cli/headless)
- Moonshot：[Kimi Code current](https://github.com/MoonshotAI/kimi-code)、[Kimi CLI legacy](https://github.com/MoonshotAI/kimi-cli)
- Alibaba：[Qwen Code](https://github.com/QwenLM/qwen-code)、[Qwen TypeScript SDK](https://qwenlm.github.io/qwen-code-docs/en/developers/sdk-typescript/)

所有高漂移事实都必须在具体实现计划开始时重新拉取、固定版本并比较semantic diff。
