# 上游官方 Agent 行为与 HarnessDelta 研究（2026-07-13）

## 1. 状态、目标与边界

- 资产类型：正式设计研究，供 Profile 与并集调用原语继续讨论；不是冻结合同
- 研究时间：2026-07-12 至 2026-07-13
- 授权演进：本文形成时只授权研究与设计；2026-07-13后续独立实施计划已完成离线Runtime、Direct/Harness Adapter与测试资产；真实账号、官方二进制和付费调用仍为`not_run`
- 上位设计：[Intent、Mechanism、Effect 与 Profile 路由 v1](./intent-mechanism-effect-profile-routing-v1-draft.md)
- 路由事实：[订阅调用与官方 Harness 路由研究](./subscription-and-official-harness-research-20260712.md)
- 对照研究：[Codex、OpenCode、OpenClaw 调用原语研究](./codex-opencode-openclaw-semantic-primitives-research-20260712.md)

本研究回答一个更精确的问题：

> Praxis 应如何利用每个厂商自己的官方 Agent、CLI、SDK、App Server、评测脚手架和提示词，提取模型最擅长的行为方式；同时又不把 Harness 注入误判为模型天性，并把 API 与不同 Harness 的行为差异编译成稳定的并集语义？

本轮只采用厂商官方文档、官方仓库和本机已安装官方 CLI 的只读检查。没有以社区体验帖代替正式事实，也没有用少量主观试跑推导模型本性。

## 2. 核心结论

### 2.1 不再使用“纯净/不纯净”二元判断

直接 API 也不是“裸模型权重”：Provider 仍可能拥有服务端安全策略、模型路由、隐藏实现和 Hosted Tool 说明。它更准确的名称是：

```text
request_controlled
```

即调用者能够明确控制请求可见的 instructions、messages、tools、tool loop、session 与验证逻辑。

官方 Agent Harness 则是：

```text
harness_composed
```

模型最终行为由以下因素共同产生：

```text
ObservedBehavior
  = ModelBehavior
  + ProviderProtocolFraming
  + HarnessInstructions
  + ToolSchemasAndDescriptions
  + ContextDiscovery
  + SessionAndMemoryPolicy
  + PermissionAndRuntimePolicy
  + RetryCompactionOrchestration
```

该式表达归因关系，不表示这些因素可以简单数值相加。

### 2.2 官方 Agent 是高价值行为样本，但不是“模型本性转储”

Codex、Claude Code、Gemini CLI、当前Kimi Code与Legacy Kimi CLI、Qwen Code、Grok Build、MiMo Code、ZCode、MiniMax Code和Copilot CLI/SDK的价值在于：

1. 厂商通常已针对模型后训练习惯设计提示词和工具方言；
2. 厂商已经替我们验证大量工具描述、权限、压缩和循环策略；
3. 开源实现可以直接看见哪些规则来自 Harness；
4. 闭源实现至少可以通过公开配置、事件与检查命令建立 Manifest；
5. 同一模型在不同官方脚手架中的差异，可帮助剥离 Harness 诱导行为。

因此 Praxis 必须从每个官方样本同时产出两个不同资产：

```text
OfficialAgentEvidence
  |-- ModelBehaviorCandidate
  `-- HarnessCapabilityProfile + HarnessDelta
```

只有跨 Harness、官方模型文档或官方评测配置共同支持的行为，才可以提升为稳定的 `ModelBehaviorProfile` 规则。

### 2.3 并集稳定性来自“因果与效果统一”，不是提示词强行拉齐

Praxis 不要求 GPT、Claude、Gemini、Kimi、Qwen 都调用同名 Edit 工具。正确做法是：

```text
同一 Intent
  -> Route 专属 Mechanism
      -> Runtime 真实观测
          -> 同一 Effect Schema 与 Verification
```

例如 `ModifyFile`：

| Route | 官方偏好或可见机制 | Praxis 统一结果 |
|---|---|---|
| Codex/GPT | `apply_patch` 或 shell | `FileChanged + unified_diff + hashes` |
| Claude Code | `Edit`，必要时 `Write/Bash` | 同上 |
| Gemini CLI | `replace`，必要时 `write_file/run_shell_command` | 同上 |
| Kimi Code current | `Edit/Write/Bash`；ACP文件可由client执行 | 同上 |
| Legacy Kimi CLI | `StrReplaceFile`，大文件使用分块`WriteFile` | 同上 |
| Qwen Code | bare用`edit/shell`；controlled nonbare可用`edit/write_file/shell` | 同上 |
| MiMo Code | GPT 模型暴露 `apply_patch`；其他模型暴露 `edit/write` | 同上 |

MiMo Code 的官方源码已经直接证明：成熟 Harness 会按模型方言切换工具，而不是让所有模型共用一种编辑工具。这与 Praxis 的 Profile 设计方向完全一致。

## 3. 官方行为样本的证据等级

### 3.1 来源优先级

| 等级 | 来源 | 能证明什么 | 不能单独证明什么 |
|---|---|---|---|
| `A1` | 官方开源 Agent/SDK 的当前源码 | 实际提示、工具、注入、事件、配置与循环 | 行为全部来自模型本身 |
| `A2` | 官方公开协议与配置文档 | 正式支持面、可控开关、事件合同 | 闭源内核没有其他注入 |
| `B1` | 官方模型评测脚手架与参数说明 | 厂商认可的高性能运行方式 | 通用生产最佳配置 |
| `B2` | 官方产品说明与使用指南 | 厂商推荐工作流和能力边界 | 精确底层机制 |
| `C1` | 厂商正式点名的第三方 Harness 集成指南 | 该 Offering 可在该产品中使用 | Praxis 自动继承产品授权或行为 |
| `D1` | 本机官方二进制只读 inspect/help | 当前机器真实版本、默认发现和可见开关 | 未公开内部请求完全可见 |

社区实现和主观试跑只能成为待验证线索，不能直接写入稳定 Profile。

### 3.2 BehaviorEvidence

```text
BehaviorEvidence
  - evidence_id
  - subject_kind: model | protocol | harness | tool_schema | runtime_policy
  - source_kind: official_source | official_docs | official_eval | live_inspect
  - source_uri
  - version_or_commit
  - observed_surface
  - assertion
  - attribution
  - confidence
  - counter_evidence[]
  - captured_at
  - expires_at
```

`attribution` 必须是以下集合之一或组合：

```text
model_intrinsic_claimed
official_harness_induced
tool_schema_induced
provider_protocol_induced
runtime_policy_induced
unknown
```

例如“Gemini 不应在同一轮对同一文件并发发出多个 edit”当前来自 Gemini CLI 官方 system prompt，应标为 `official_harness_induced + tool_schema_induced`，不能直接写成 Gemini 模型的不可变天性。

## 4. HarnessDelta 与 InjectionManifest

### 4.1 HarnessDelta

```text
HarnessDelta
  = ObservedOfficialAgentContract
  - CallerControlledDirectAPIContract
```

它不是文本 diff，而是以下语义维度的结构化差异：

```text
instructions
context_discovery
tool_surface
tool_guidance
permission_policy
session_history
memory
compaction
retry_and_auto_continue
subagents_and_orchestration
hooks_plugins_skills_mcp
hosted_execution
event_observability
usage_and_quota_observability
```

### 4.2 InjectionManifest

每次 Official Harness 调用前，Adapter 应形成预期 Manifest；调用开始后再形成实际 Manifest：

```text
InjectionManifest
  - harness_stack[]: component/version/path/binary/protocol digests
  - instruction_control
  - base_instructions[]
  - appended_instructions[]
  - dynamic_instructions[]
  - context_sources[]
  - tool_definitions[]
  - tool_state[]: discovered/registered/model_visible/executable/permission/owner
  - tool_guidance[]
  - permission_rules[]
  - hooks[]
  - plugins[]
  - skills[]
  - mcp_servers[]
  - subagents[]
  - memory_sources[]
  - session_state
  - compaction_policy
  - retry_policy
  - hosted_capabilities[]
  - event_contract
  - sanitized_environment_digest
  - field_evidence_quality: reported/observed/inferred/opaque
  - opaque_fields[]
```

### 4.3 可控性枚举

```text
HarnessTransparency
  = source_visible | manifest_visible | config_visible | opaque

InstructionControl
  = full_replace | section_replace | append_only | fixed | unknown

ContextDiscoveryControl
  = disabled | isolated | scoped | enumerable_only | fixed | unknown

ToolSurfaceControl
  = exact_allowlist | registered_fixed_with_exclude | execution_allow_deny_only | preset_only | fixed | unknown

EventFidelity
  = model_blocks_and_mechanism | agent_item_lifecycle | tool_lifecycle | message_stream | final_text_only
```

这些维度不能合并成一个“纯净度百分比”。例如某 Harness 可以完整替换 system prompt，却仍自动加载项目上下文和全局插件；另一个 Harness 提示词固定，但能提供完整工具与文件变更事件。

### 4.4 RouteFingerprint

```text
RouteFingerprint = hash(
  provider,
  model_id,
  model_revision,
  deployment,
  protocol,
  offering,
  auth_route,
  harness_stack_digests,
  instruction_digest,
  tool_schema_digest,
  context_manifest_digest,
  sanitized_environment_digest,
  runtime_policy_digest
)
```

同一模型只要 Deployment、Harness 或 Harness 版本不同，就不能自动复用同一最终 Profile。

## 5. Route 行为类别

| 类别 | 含义 | 默认执行所有者 | 典型路线 |
|---|---|---|---|
| `direct_api` | 调用者控制模型请求与工具循环 | Praxis | OpenAI/Anthropic/Gemini/xAI/PAYG兼容API |
| `configurable_official_harness` | 官方 Harness 可替换提示、限定工具、输出结构事件 | Harness + Praxis verifier | Codex、Claude SDK、Gemini CLI、Qwen SDK、Legacy Kimi CLI、Copilot SDK、Grok Build |
| `partially_configurable_official_harness` | 公开配置可隔离上下文和执行权限，但不能精确替换全部prompt/tool registry | Harness + Praxis verifier | 当前Kimi Code |
| `opaque_official_harness` | 官方 Harness 可调用但内部注入和事件有限 | Harness | Antigravity消费者CLI、当前 MiniMax Code 桌面面 |
| `sanctioned_third_party_harness` | Provider 明确允许指定第三方工具 | 指定 Harness | GLM Coding Plan 支持工具、MiniMax/MiMo官方集成工具 |
| `caller_authored_agent_sdk` | 官方 SDK 提供循环，但提示与工具由调用者构建 | Praxis + SDK | OpenAI Agents SDK、Antigravity SDK API/Vertex路线 |

路由选择不应永远偏向`direct_api`。对于为官方Harness深度协同训练的模型，`vendor_default`可能产生更高任务质量；对于严格可复现和Effect驱动任务，`semantic_stable`或`direct_api`更容易形成稳定合同。

## 6. 本轮官方源码快照

| 项目 | 官方仓库 | 本轮快照 |
|---|---|---|
| Codex | `openai/codex` | `9e552e9d15ba52bed7077d5357f3e18e330f8f38` |
| Claude Agent SDK Python | `anthropics/claude-agent-sdk-python` | `528265fa09da954f0a0da1bf31e16db32b510138` |
| Claude Agent SDK TypeScript | `anthropics/claude-agent-sdk-typescript` | `79b6350e13cf24af94a8d2e696a0883fd8cc55fe` |
| Gemini CLI | `google-gemini/gemini-cli` | `f354eebaf43b25bacb176007e449bb9a638fd101` |
| Antigravity CLI | `google-antigravity/antigravity-cli` | `b2cf478468731d06f13a4b02effbf40a4248aa7a` |
| Antigravity SDK Python | `google-antigravity/antigravity-sdk-python` | `dd49bbccc3aa4ecd538ec832e2e3a739bfb5ad7d` |
| Kimi Code current | `MoonshotAI/kimi-code` | `ceb158dc54586f254819edbc83c27e21dca1ecf6` |
| Kimi CLI legacy | `MoonshotAI/kimi-cli` | `2c34efbbc6c7cfe40770623281e87c138ff8eb6c` |
| Qwen Code | `QwenLM/qwen-code` | `92b47a4e014611007bbd11ca6b6707e17f103f05` |
| MiMo Code | `XiaomiMiMo/MiMo-Code` | `f056dcc60bbf2f3c574b2bd4dda55f2eb32b08f2` |
| GitHub Copilot SDK | `github/copilot-sdk` | `edbe6c662a78216147cfae1a19c6d127f1e94797` |

Claude Code 与 Grok Build 的核心运行时没有完整开源，因此主要依据官方 CLI、SDK包装层、公开文档和本机只读检查。MiniMax Code 当前官方说明其 Harness 基于 OpenCode/Pi，但公开产品页尚不足以提供可嵌入协议合同。

## 7. 各上游行为样本与最小 Profile

### 7.1 OpenAI：Direct API、Codex 与 Agents SDK

#### Direct API

- Responses API 由 Praxis 决定 instructions、items、tools、tool loop 与状态续接；
- Hosted Shell、Apply Patch、Computer、Web/File Search 等能力仍可能由 Provider 托管；
- 这是 OpenAI 模型的 `request_controlled` 路线，但不是无服务端策略的“裸权重”。

#### Codex app-server

官方源码显示：

- thread 创建可指定 model、provider、cwd、approval、sandbox、base instructions、developer instructions、动态工具与配置；
- `AGENTS.md` 等项目指令会进入运行上下文，并可通过 instruction sources 观测；
- 将 `project_doc_max_bytes` 设为 `0` 可关闭项目文档发现；
- base instructions 可以覆盖，工具与 feature 仍会产生动态运行说明；
- stable app-server事件原生区分Agent message、reasoning Item、command、file change、approval、turn与item，但不承诺暴露稳定的单次模型step边界。

`codex_semantic_stable` 建议：

```text
baseInstructions = Praxis精简桥接指令
developerInstructions = 显式任务策略
project_doc_max_bytes = 0
optional_features = 显式关闭或固定
dynamic_tools = 仅Praxis追加动态工具；不是Codex全部工具allowlist
thread = fresh + ephemeral
approval/sandbox = RuntimePolicy编译结果
```

残余差异：Codex仍会组合shell、core utility、collaboration、MCP、extensions与hosted tools。Codex是Agent Harness；沙箱、工具描述、运行时上下文和模型路由不会因此消失。stable app-server的turn/item应映射Agent Item，不得为统一而伪造ModelEvent。

#### OpenAI Agents SDK

- 官方 SDK 默认使用 Responses API，但 `Agent + Runner` 拥有工具循环、handoff、guardrail、session、tracing 与可选 sandbox；
- instructions、tools、handoffs 和 session 均由应用定义；
- 它不桥接 ChatGPT/Codex 订阅，只使用 Platform API/BYOK 类凭据；
- 它适合成为 `caller_authored_agent_sdk` 或事件设计参考，不应被当作 Codex 官方后训练行为样本。

结论：

```text
OpenAI PAYG最可控路线 = Direct Responses
ChatGPT/Codex订阅正式路线 = Codex app-server
自定义Agent循环候选 = OpenAI Agents SDK
```

### 7.2 Anthropic：Direct Messages 与 Claude Agent SDK

Claude Agent SDK 当前实现会启动官方 Claude CLI，并以 stream-json 与其通信。当前源码能够确认：

- `system_prompt=None` 会显式映射为一个空 system prompt；
- `setting_sources=[]` 会禁用文件系统 settings 来源；仅使用默认 `None` 会继承 CLI 默认来源；
- `tools` 可以是精确列表、空列表或 Claude Code preset；
- `allowed_tools` 只控制授权，不等于工具是否进入模型上下文；
- `strict_mcp_config=true` 可避免合并其他 MCP 配置；
- hooks、plugins、skills、MCP、session、fork 与 structured output 均可配置。

订阅路线不能直接使用 Claude CLI 的 `--bare`：该模式会跳过 OAuth/keychain 读取，只允许 API Key、helper 或第三方 Provider 认证。

`claude_subscription_semantic_stable` 建议：

```text
system_prompt = Praxis最小桥接指令
setting_sources = []
skills = []
tools = 精确需要集合
allowed_tools = 仅已预批准子集
strict_mcp_config = true
plugins = []
mcp_servers = {}
agents = none
hooks = Praxis审核所需的Pre/PostToolUse或空
session = fresh
cwd = 隔离且显式
```

注意：若显式传 `skills=[]` 却没有同时设置 `setting_sources=[]`，SDK可能为 skills 引入默认 user/project来源；最小 Profile 必须把两项一起控制。

SDK默认继承几乎全部父进程环境且CLI版本检查只告警。订阅Profile必须固定resolved CLI path/version/digest，并生成sanitized environment digest，拒绝冲突API Key、base URL、Bedrock/Vertex/Foundry、helper和proxy变量。

残余差异：官方 Claude Agent loop、工具 system prompt、权限与动态运行策略仍存在，因此应标为 `harness_composed`，而不是 Direct Messages 等价物。

### 7.3 Google：Gemini API、Gemini CLI、Antigravity SDK/CLI

#### Gemini API / Vertex

这是 Gemini 模型的主要 `request_controlled` 路线。Praxis拥有 function loop 和 caller-hosted工具；官方 Code Execution、Search Grounding、Computer Use 等按具体模型和 Deployment 标注来源。

#### Gemini CLI

Gemini CLI 是比 Antigravity 更适合提取 Gemini 编码行为的官方开源样本：

- 官方 system prompt 完整可见；
- `GEMINI_SYSTEM_MD` 支持完整替换，不是 append；
- 默认自动发现 `GEMINI.md`、项目与个人 memory、skills、subagents、extensions、MCP 和 hooks；
- `tools.core` 是内建工具精确 allowlist；
- headless 支持 `text/json/stream-json`，事件含 init、message、tool_use、tool_result、error、result；
- ACP 通过 stdio JSON-RPC，支持 session、cancel、approval mode、模型切换和由客户端代理的文件系统；
- 官方提示明确偏好 `replace` 做局部编辑，要求先取得足够上下文，并避免同一轮对同一文件并发多个 edit；独立工具调用则鼓励并行。

完整替换system prompt后，Gemini CLI仍会生成首条`<session_context>`，包含日期、OS、项目临时目录、可选目录树和session memory；memory也可进入system层或首用户消息。因此`GEMINI_SYSTEM_MD`不能单独证明上下文已清空。

`gemini_cli_semantic_stable` 建议：

```text
GEMINI_CLI_HOME = Praxis专属隔离目录
GEMINI_SYSTEM_MD = Praxis精简桥接指令文件
tools.core = 精确工具集合
tools.exclude = 硬拒绝集合
experimental.autoMemory = false
extensions/skills/subagents/hooks/mcp = 显式空或禁用
context.includeDirectoryTree = false
context.loadMemoryFromIncludeDirectories = false
session = fresh
transport = ACP优先，stream-json备选
```

当前实现仍默认认识 `GEMINI.md` 文件名；在工作区确有该文件时，Adapter必须把它列入 Manifest，而不能宣称零发现。若未来官方提供 bare/context-off 开关，再升级为 `disabled`。

#### Antigravity SDK

- API Key/Vertex 路线支持 `CustomSystemInstructions` 完整替换；
- `enabled_tools/disabled_tools` 控制模型可见工具；
- policies、hooks、MCP、workspaces、schema、skills、subagents、conversation/history 可配置；
- Python SDK 包装的本地 Harness 核心为编译二进制，透明度低于 Gemini CLI。

最小建议：完整替换 instructions、精确 enabled tools、空 hooks/skills/subagents/MCP、隔离 workspace 与 fresh conversation。

#### Antigravity 消费者权益 CLI

- 当前可通过官方 `agy --print` 使用本机 Google Sign-In；
- 没有确认公开 ACP 合同；
- 核心 Harness 闭源，结构事件与注入可见性较低；
- 因此只能形成 `opaque_official_harness` Profile，默认最多承诺单次任务与最终结果，不承诺完整 mechanism stream。

### 7.4 xAI：Direct API 与 Grok Build

Grok 4.5 可经官方 Responses API直接调用，也可经 Grok Build运行。

Grok Build 当前公开能力包括：

- headless plain/json/streaming-json；
- ACP：`grok agent stdio`；
- system prompt覆盖、rules追加、tools/disallowed tools；
- permission mode、session resume/fork、schema、sandbox、max turns；
- plan、subagents、memory、web 等可关闭；
- `grok inspect --json` 可枚举已加载 rules、permissions、plugins、hooks、skills、agents和兼容资产。

本机只读 inspect 证明默认 Grok Build 会发现项目指令和 Claude/Cursor兼容资产，因此不能只看启动参数判断其上下文。

`grok_build_semantic_stable` 建议：

```text
system prompt = full override
tools = exact allowlist
plan/subagents/memory/web = off unless Intent requires
config/home = Praxis专属隔离根
session = fresh
transport = ACP
preflight = grok inspect --json
```

若实际 inspect 与 Expected Manifest 不一致，应拒绝或显式降级，不静默继续。

### 7.5 Moonshot：Kimi API、当前Kimi Code与Legacy Kimi CLI

2026-07-12官方仓库发生了必须进入Profile选择键的代际变化：旧Python `MoonshotAI/kimi-cli` README已明确项目将逐步停止并迁移到新的TypeScript `MoonshotAI/kimi-code`。因此必须建立两条互不继承能力的Route：

```text
kimi_code_current
  = 当前Kimi Code CLI + stream-json + ACP 0.23

kimi_cli_legacy_pinned
  = 旧Kimi CLI + Agent spec + Wire 1.10/legacy ACP
```

当前Kimi Code事实：

- 公开CLI支持`-p --output-format stream-json`和`kimi acp`；当前公开参数没有旧版`--agent-file`、Wire或完整system prompt override；
- 当前工具方言为`Read/Edit/Write/Bash`等，并带Plan、Task、Agent/AgentSwarm、后台任务、cron、skills、plugins、MCP与hooks；
- ACP可把workspace文件读写交给client，但当前terminal reverse-RPC未连接，Bash仍由本机Harness执行；
- `KIMI_CODE_HOME`可隔离Kimi专属config、credentials、AGENTS、skills、plugins和MCP，但通用真实HOME下`.agents`及项目AGENTS仍可能被发现；
- permission rules控制allow/deny/ask，不控制工具是否registered或model-visible；
- CLI/ACP没有公开顶层业务JSON Schema参数，structured output属于emulated；
- print mode默认auto权限，不能用于要求逐次审批的Profile；
- Agent结束时后台任务可能仍存在，统一终态前必须drain与reconcile。

`kimi_code_current_semantic_stable`建议：

```text
KIMI_CODE_HOME = Praxis专属隔离目录
HOME/container = 必要时同时隔离
skills-dir = 受控空目录
workspace/global AGENTS、MCP、plugins、hooks = 清空或精确hash
cron/background/subagents = 禁止或严格门禁
permission = ACP manual + RuntimePolicy
session = fresh
transport = ACP优先，stream-json备选
```

由于当前公开CLI没有完整prompt override和精确tool registry allowlist，这条Route只能在DegradationPolicy明确允许“额外model-visible工具但执行全部fail-closed”时使用；若任务要求registry精确相等，应执行前拒绝或进入单独的官方源码二开评审。

Legacy Kimi CLI仍是有价值的固定行为样本：Agent spec允许自定义system prompt、精确tools/exclude/subagents；旧提示偏好`StrReplaceFile`与分块`WriteFile`；Wire保留turn、step、retry、compaction、tool、approval、subagent、plan、hook与steering。但这些能力只能属于`legacy_pinned`，不能继续写成current默认。

Kimi开放平台PAYG、当前Kimi Code权益与Legacy CLI兼容Route必须使用不同Route/Entitlement。即使协议形状相似，也不能共享计费和用途判断。

### 7.6 MiniMax：Direct API、MiniMax Code 与官方评测脚手架

截至本轮，MiniMax当前模型与产品已发生版本漂移：官方页面已以 MiniMax M3 和更新后的 MiniMax Code 为主，不能继续把 M2.7 结论自动当作最新默认。

官方事实：

- M3 API与 Token Plan提供正式入口；
- MiniMax Code是与M3协同设计的官方Agent产品；
- 官方说明其Harness基于OpenCode与Pi，并采用 Producer + Verifier 的反思/校验循环；
- 当前公开产品说明表示未来计划开源，尚未提供足以让Praxis稳定嵌入的公开Agent协议；
- 官方评测多处使用 Claude Code，并覆盖其默认 system prompt；其他任务使用 Terminus 2 或 Mini-SWE-Agent；
- 官方提示建议少量活跃目标、简洁system prompt、明确工具边界、合理并行和长任务分窗/压缩。

因此应拆成三类证据：

```text
MiniMax model behavior candidate
  <- 官方提示最佳实践 + 官方评测脚手架

MiniMax Code harness behavior
  <- 官方产品说明，当前仅作行为参考

Praxis executable route
  <- Direct API/Token Plan，或官方明确支持的第三方Coding Harness
```

MiniMax CLI `mmx` 是多模态服务能力CLI，可作为 Agent 工具调用，但不是 MiniMax Code 的编程 Agent 协议替代品。

当前建议：Direct Route由Praxis工具循环完成并集；若选Claude Code/OpenCode等正式支持的Harness，则使用该Harness自己的 `HarnessCapabilityProfile`，不得把其行为写成 MiniMax 模型本性。

### 7.7 Xiaomi：MiMo API、Token Plan 与 MiMo Code

MiMo Code V0.1.x 已正式开源，官方仓库显示它是 OpenCode 二开，并增加：

- 项目 memory、session checkpoint、任务进度和自动上下文重建；
- dream/distill、自定义workflows、compose与orchestrator；
- skills、plugins、MCP、subagents、LSP、权限与SQLite状态；
- headless `run --format json`、本地server与ACP；
- provider/model专属system prompt；
- 模型专属工具投影：GPT家族启用 `apply_patch`，其他模型使用 `edit/write`。

MiMo Code 默认 HarnessDelta 很大：

- base prompt之外再加入环境、skills、AGENTS/CLAUDE指令；
- main/peer actor会得到memory/checkpoint说明；
- plugin可变换system prompt和tool definition；
- dream/distill默认具有自动触发配置；
- session、task、actor和checkpoint会跨轮维护状态。

`mimo_code_semantic_stable` 建议：

```text
MIMOCODE_CONFIG_DIR = Praxis专属隔离目录
MIMOCODE_DISABLE_PROJECT_CONFIG = true
MIMOCODE_DISABLE_CLAUDE_CODE_PROMPT = true
custom agent.prompt = Praxis桥接指令
custom agent.tool_allowlist = 精确工具集合
plugin/skills/mcp/instructions = empty
dream.auto = false
distill.auto = false
session = fresh
transport = ACP优先，run JSON备选
```

当前源码对 primary/peer 的 memory instructions 没有显式总关闭开关，因此即使使用上述配置也应记录残余 HarnessDelta，不能宣传为 Direct API 等价。

MiMo PAYG和 Token Plan均有兼容Endpoint，但 Token Plan存在Coding用途与自动化/后台边界。该限制属于 `EntitlementProfile`，不是 MiMo 模型或协议自身不支持自动化、非流或Batch的能力结论。

### 7.8 Alibaba/Qwen：Direct API、Qwen Code 与 Agent SDK

Qwen Code当前已远超单一交互式CLI：

- 官方开源 Agent支持多Provider、headless、JSON/stream-json、daemon、ACP、MCP、hooks、skills、subagents、memory、worktree与自动化；
- 官方文档明确给出脚本、CI、批处理和后台工作流；
- TypeScript SDK `@qwen-code/sdk` 是官方程序化入口，并有Python/Java早期SDK；
- TS SDK启动Qwen Code stream-json进程，支持多轮、取消、权限回调、MCP、partial messages和session；
- `systemPrompt: string`完整覆盖内置prompt；
- 非bare模式下，`coreTools`限制真实注册的内建工具，不只是自动批准；
- `extraArgs`可以传入`--bare`，但bare会忽略`coreTools`覆盖；
- bare固定注册`read_file/edit/notebook_edit/run_shell_command`，可用exclude做减法；`edit`以空old string创建文件；
- bare关闭MCP、managed memory、skills与hooks，但仍注入日期、OS、cwd、目录树和git status等启动上下文。

Qwen必须拆分两个Harness子模式：

```text
semantic_stable.bare_fixed
  = 固定bare工具集 - excludeTools，最小隐式配置

controlled_nonbare
  = coreTools精确注册，但需额外隔离memory/extensions/hooks/skills/MCP/agents
```

`qwen_code_semantic_stable.bare_fixed`建议：

```text
SDK = @qwen-code/sdk
systemPrompt = Praxis桥接指令
extraArgs = ["--bare"]
coreTools = 不设置；禁止宣称生效
excludeTools = 从bare固定集合做减法
mcpServers/agents/extensions = empty
permissionMode + canUseTool = RuntimePolicy
sessionId = fresh
includePartialMessages = true
fallbackModel = disabled
```

`coreTools + --bare`是Profile编译错误，必须拒绝，不能把其中一项静默忽略。

这条路线是当前最接近“官方订阅Harness + 高可控程序化接口”的样本之一。

必须区分：

1. Qwen Code Harness能力支持自动化；
2. Alibaba Coding Plan是否允许某种自动化，由具体Offering条款决定；
3. PAYG API能力与Coding Plan权益不能互相推导。

因此“Qwen只能流式、不能自动化或Batch”不能作为模型级全局结论。正确栅栏必须绑定精确Offering和ExecutionSurface。

### 7.9 Z.AI/GLM：Direct API、ZCode 与受限支持工具

ZCode是当前GLM最重要的第一方行为样本：

- 官方自研Agent与GLM-5.2深度调优；
- 统一任务、上下文、权限、文件引用、Review与Git工作流；
- 内建general-purpose和只读Explore subagent；
- 支持自定义subagent、system prompt和工具权限；
- 支持skills、commands、plugins、hooks、LSP和MCP；
- 能从Claude Code、Codex和OpenCode导入配置资产。

这意味着ZCode默认是高注入、高编排的 `opaque/config_visible` Harness，而不是纯GLM调用。

当前公开文档未确认可供Praxis嵌入的稳定CLI、SDK、ACP或headless合同，因此：

- ZCode用于提取官方行为模式；
- GLM PAYG继续Direct API；
- GLM Coding Plan只可在官方支持工具名单范围内使用；
- Praxis本身未获批准前，不能因协议Endpoint公开就直接消费该订阅；
- 若通过OpenCode/Claude Code等指定工具交接，Profile必须同时记录该第三方HarnessDelta。

### 7.10 GitHub Copilot：Copilot SDK 与 CLI ACP

Copilot SDK 是官方、程序化、按用户订阅计量的 Agent Harness：

- SDK通过JSON-RPC连接Copilot CLI server，与Copilot CLI共用引擎；
- `mode: "empty"`关闭可选功能默认值，要求应用显式提供base directory、filesystem和tools；
- system message支持完整替换以及按section remove/replace/append/prepend/preserve；
- `availableTools/excludedTools`控制工具面；
- `skipCustomInstructions`关闭自定义指令；
- hooks、MCP、skills、custom agents、session FS、provider/BYOK、telemetry和compaction均有配置面；
- 事件区分session idle与model task_complete：前者是机械终态，后者只是模型的最佳努力判断。

`copilot_semantic_stable` 建议：

```text
mode = empty
systemMessage = replace或逐section显式配置
skipCustomInstructions = true
availableTools = 精确集合
excludedTools = 其余集合
hooks/mcp/skills/customAgents = empty
sessionFs/baseDirectory = Praxis隔离对象
permissionHandler = RuntimePolicy
```

Copilot对“机械完成”和“模型声称完成”的区分，直接支持 Praxis 将 Effect/Verification 与 CompletionClaim 分开的设计。

### 7.11 云部署、第三方托管与自托管

AWS Bedrock、Vertex、Azure等不是单纯Endpoint别名：

- Provider、认证、Region、协议版本、Hosted Tools、usage、缓存和模型目录均可能不同；
- 同一Claude/GPT/Gemini家族可复用部分ModelBehavior证据，但必须有Deployment override；
- 云middleware或代理SDK若拥有retry、tool loop、content filter或session，也产生自己的HarnessDelta；
- 第三方托管不能因为宣称OpenAI兼容就继承OpenAI全部工具、schema和事件能力；
- 自托管模型的system template、chat template、tool parser和sampling配置必须进入RouteFingerprint。

因此云与托管Route必须以精确Deployment为Profile键，不能只按模型名命中。

## 8. 官方文件操作行为对照

| 官方样本 | 局部修改 | 新建/重写 | Shell定位 | 关键诱导来源 |
|---|---|---|---|---|
| Codex | `apply_patch` | patch或shell，依大小/生成性质 | 命令、测试、生成与大范围操作 | Codex base prompt + apply_patch grammar |
| Claude Code | `Edit` | `Write` | 无专用工具时补齐移动/目录/命令 | Claude Code tools + system tool说明 |
| Gemini CLI | `replace` | `write_file` | 系统命令与验证 | 官方prompt强调低轮次、避免同文件并发edit |
| Kimi Code current | `Edit` | `Write` | `Bash`；ACP文件可由client执行 | 当前Kimi Code默认profile与工具Schema |
| Legacy Kimi CLI | `StrReplaceFile` | `WriteFile`，大文件分块 | 系统操作与批量能力 | legacy agent spec与默认prompt |
| Qwen Code | bare用`edit` | bare可用`edit`空old string创建；nonbare可有`write_file` | `run_shell_command` | bare固定工具集或controlled nonbare投影 |
| MiMo Code | GPT走`apply_patch`；其他走`edit` | 非GPT走`write` | bash | Harness按model ID动态投影工具 |
| Grok Build | 由显式tool allowlist和系统prompt决定 | 同左 | 可显式开放 | 闭源核心；inspect可审计外部注入 |
| Copilot SDK | 应用定义 | 应用定义 | 应用定义 | `empty`模式由Praxis投影工具 |

该表是Profile偏好证据，不是硬能力表。实际可用机制必须再与HarnessCapabilityProfile和RuntimePolicy求交集。

## 9. Profile 选择与编译修正

### 9.1 ProfileSelectionKey

不能再用“模型名 -> Profile”单键选择：

```text
ProfileSelectionKey
  - provider
  - model_id
  - model_revision
  - deployment
  - protocol
  - offering
  - auth_route
  - execution_surface
  - harness
  - harness_version
```

选择完成后仍保持既定三部分合成：

```text
EffectiveProfile
  = ModelBehaviorProfile
  × HarnessCapabilityProfile
  × RuntimePolicy
```

RouteEnvelope负责选择正确的三类Profile实例，不把Deployment或Entitlement偷塞进ModelBehavior。

### 9.2 三种上下文模式

| 模式 | 目标 | 默认用途 |
|---|---|---|
| `semantic_stable` | 最小已知注入、精确工具、强Effect验证 | Praxis统一调用默认 |
| `vendor_default` | 最大保留厂商官方优化、memory与编排 | 长任务或模型与Harness协同优势明显时 |
| `custom_explicit` | 用户明确选择每个上下文与扩展资产 | 高级单租户场景 |

`semantic_stable`不等于空提示词。它至少需要：

1. Praxis Intent与完成条件；
2. 可用工具的正确操作约束；
3. Runtime权限与审批边界；
4. 验证要求；
5. 不与模型后训练方言冲突的最小桥接说明。

### 9.3 预检、编译与执行

```text
ResolveSupportRoute
  -> SelectProfileComponents
  -> BuildExpectedInjectionManifest
  -> HarnessPreflightInspect
  -> CompareExpectedVsActual
  -> CompileIntentToMechanismSet
  -> RuntimePolicyHardFilter
  -> ModelPreferenceRanking
  -> ExecuteAndNormalizeEvents
  -> ObserveEffects
  -> VerifyIntentSatisfaction
```

若Manifest发生漂移：

```text
strict route      -> fail_closed
transform route   -> mark degraded + residual
vendor_default    -> allow only if change is policy-safe and version-approved
unknown secret/auth drift -> always fail_closed
```

### 9.4 Harness审计能力优先级

| Harness | 首选审计面 |
|---|---|
| Codex | thread配置、instructionSources、app-server typed events |
| Claude SDK | SDK options + CLI版本 + stream-json事件 |
| Gemini CLI | 隔离home/settings + system文件digest + ACP/headless事件 |
| Grok Build | `grok inspect --json` + ACP |
| Kimi Code current | 隔离KIMI_CODE_HOME/HOME + ACP capability/tool/permission/background事件 |
| Legacy Kimi CLI | 固定Agent spec + Wire/ACP事件 |
| Qwen Code | `bare_fixed`首个SDKSystemMessage，或controlled nonbare的QueryOptions + stream-json/daemon事件 |
| MiMo Code | config/agent spec + instructions-loaded事件 + ACP/JSON事件 |
| Copilot SDK | empty mode config + system sections + session events |
| Antigravity CLI | CLI版本、显式参数、最终结果；不可见部分列opaque |
| ZCode/MiniMax Code | 产品版本与公开配置；当前不可见部分列opaque |

## 10. 如何反向构建稳定并集

### 10.1 先保存官方优势，再消解差异

每个模型/Route先定义官方最优Mechanism候选，不先求共同交集：

```text
Intent: ModifyFile

GPT candidate      = apply_patch > shell replace > whole rewrite
Claude candidate   = Edit > Write > Bash
Gemini candidate   = replace > write_file > shell
Kimi current       = Edit > Write > Bash
Kimi legacy        = StrReplaceFile > WriteFile > Shell
Qwen bare_fixed    = edit > run_shell_command
Qwen nonbare       = edit > write_file > run_shell_command
```

然后：

1. 与当前Harness实际提供工具求交集；
2. 应用RuntimePolicy硬过滤；
3. 只保留能被Effect Observer验证的机制；
4. 选择模型最熟悉的primary和有序fallback；
5. 运行后以真实文件状态生成统一Effect。

### 10.2 不稳定差异只留在Mechanism层

以下差异不应泄漏为上层公共必选字段：

- 工具名字；
- 是否由模型、Harness或Praxis执行；
- patch、replace或whole write方言；
- Harness内部重试与压缩事件；
- 厂商特有session ID。

它们进入MechanismTrace、CapabilityOrigin、NativeIdentity和Residual。

以下信息必须统一：

- 上层Intent；
- 审批与权限结果；
- 实际副作用；
- Diff、hash和验证；
- 取消、失败、未知副作用和最终满足状态。

### 10.3 官方行为不能覆盖Runtime事实

即使官方Agent默认提示要求“运行测试”，也不能据此生成 `VerificationPassed`。只有真实测试进程、退出码、日志和必要的状态检查才能形成Verified Effect。

同理，官方Agent的 `task_complete`、自然语言“完成”或计划项勾选，都只能形成 `CompletionClaim`，不能替代 Praxis verifier。

## 11. 本轮纠正的旧假设

1. **Qwen并非全局“只能流式、不能自动化”。** 当前Qwen Code官方文档提供脚本、CI、batch、daemon、ACP和正式Agent SDK。是否允许由Coding Plan付费，仍需按Offering条款门禁。
2. **MiMo限制不是模型能力限制。** 当前Token Plan提供兼容Endpoint和官方MiMo Code；Coding/自动化/backend边界属于Entitlement，不应写入模型Profile。
3. **Gemini行为样本不能只看Antigravity。** 开源Gemini CLI具有完整prompt、tools、ACP和stream-json，是更高透明度的官方行为样本；Antigravity仍是另一条消费者权益Harness。
4. **MiniMax当前默认不能只写M2.7。** 官方已转向M3与新版MiniMax Code，Profile必须按模型版本和评测脚手架拆分。
5. **官方Harness也会按模型切工具。** MiMo Code对GPT使用apply_patch、对其他模型使用edit/write，证明统一Intent与差异Mechanism是正确方向。
6. **Kimi官方主Harness已经换代。** 当前默认必须是Kimi Code；旧Agent spec/Wire/StrReplace行为只能属于`legacy_pinned`。
7. **Qwen bare与coreTools不能组合为精确工具投影。** bare会忽略coreTools并使用固定工具集，必须拆分`bare_fixed`与`controlled_nonbare`。
8. **Gemini完整system prompt override仍不等于空上下文。** 首用户`<session_context>`必须进入Manifest。

## 12. 尚未确认与必须保留的缺口

- Antigravity消费者权益是否会公开稳定ACP或完整结构流；
- MiniMax Code何时开源，以及是否提供CLI/SDK/ACP；
- ZCode是否会公开稳定headless/CLI/SDK/ACP合同；
- Grok Build是否有公开的单开关可以关闭全部Claude/Cursor兼容发现；当前应依赖隔离配置与inspect；
- Gemini CLI能否在不改源码的前提下彻底关闭所有GEMINI.md发现；当前只能隔离、枚举和报告；
- 当前Kimi Code何时公开完整system prompt override与精确tool registry allowlist；
- MiMo Code primary/peer memory instructions是否会新增正式总关闭开关；
- 各消费者/Coding Plan对Praxis产品身份、自动化、后台、多租户和商业交付的最终授权；
- 未公开Provider服务端注入无法通过本地Harness审计消除，只能标记opaque。

这些缺口不会阻止并集设计，但会影响Route是否 `exact`、`degraded`、`research_only` 或 `unavailable`。

## 13. 已纳入IME与纸面编译合同的决定

1. Profile选择键增加Deployment、Protocol、Offering、Auth Route、Harness和Harness Version；
2. ModelBehaviorProfile中的每条偏好必须携带BehaviorEvidence与归因；
3. HarnessCapabilityProfile增加Expected/Actual InjectionManifest与HarnessDelta；
4. Official Harness执行前必须preflight，无法观测的字段显式列为opaque；
5. 上下文提供 `semantic_stable`、`vendor_default`、`custom_explicit` 三种模式；
6. Direct API统一称为`request_controlled`，不再称“纯模型”；
7. 官方Agent默认行为作为高价值样本，但不会自动提升为模型固有行为；
8. 稳定并集以Verified Effect为核心，工具方言只进入Mechanism；
9. 厂商版本漂移触发Profile证据过期和重新审计；
10. 首批纸面编译示例已完成GPT Direct、Codex、Claude SDK、Gemini CLI、当前Kimi Code、Qwen SDK和caller-hosted Direct API覆盖。

## 14. 官方来源索引

### OpenAI

- <https://github.com/openai/codex>
- <https://developers.openai.com/codex/app-server>
- <https://openai.github.io/openai-agents-js/>
- <https://openai.github.io/openai-agents-python/agents/>

### Anthropic

- <https://github.com/anthropics/claude-agent-sdk-python>
- <https://github.com/anthropics/claude-agent-sdk-typescript>
- <https://docs.anthropic.com/en/docs/claude-code/cli-usage>

### Google

- <https://github.com/google-gemini/gemini-cli>
- <https://geminicli.com/docs/cli/acp-mode>
- <https://geminicli.com/docs/cli/headless>
- <https://geminicli.com/docs/cli/system-prompt>
- <https://github.com/google-antigravity/antigravity-cli>
- <https://github.com/google-antigravity/antigravity-sdk-python>
- <https://antigravity.google/docs/sdk-overview>

### xAI

- <https://docs.x.ai/build/overview>
- <https://docs.x.ai/build/cli/headless-scripting>
- <https://docs.x.ai/build/cli/reference>

### Kimi

- <https://github.com/MoonshotAI/kimi-code>
- <https://github.com/MoonshotAI/kimi-cli>
- <https://www.kimi.com/code/docs/en/>

### MiniMax

- <https://www.minimax.io/blog/minimax-m3>
- <https://platform.minimax.io/docs/token-plan/prompting-best-practices>
- <https://platform.minimax.io/docs/token-plan/minimax-cli>

### Xiaomi MiMo

- <https://github.com/XiaomiMiMo/MiMo-Code>
- <https://mimo.mi.com/docs/en-US/news/latest/mimocode>
- <https://mimo.mi.com/docs/en-US/tokenplan/integration/mimo-code>
- <https://mimo.mi.com/docs/en-US/price/token-plan>

### Qwen

- <https://github.com/QwenLM/qwen-code>
- <https://qwenlm.github.io/qwen-code-docs/en/developers/sdk-typescript/>
- <https://qwenlm.github.io/qwen-code-docs/en/developers/tools/introduction/>

### Z.AI/GLM

- <https://zcode.z.ai/en/docs/welcome>
- <https://zcode.z.ai/en/docs/subagents>
- <https://zcode.z.ai/en/docs/plugin>
- <https://docs.z.ai/devpack/tool/others>

### GitHub Copilot

- <https://github.com/github/copilot-sdk>
- <https://docs.github.com/en/copilot/reference/copilot-cli-reference/acp-server>
