# 多模型预埋 Prompt 候选架构

状态：设计候选，等待用户对“公共Praxis Core + 官方派生Family Overlay”结构做业务确认；未生成production PromptAsset，未发布、未接Model Route。来源证据见[官方上游审计](prompt-upstream-audit.md)，完整性合同见[Prompt Provenance V1](prompt-provenance.md)。

## 1. 推荐结构

不维护七份彼此漂移的整段system prompt。每个最终候选由四层exact refs组合：

```text
Praxis Core Stable Prompt
  + Official-derived Model Family Overlay
  + Harness Compatibility Overlay (exact Profile current)
  + Run/Turn Dynamic Tail
```

| 层 | 内容 | Cache区域 | Owner/current |
|---|---|---|---|
| Praxis Core | 任务完成、指令优先级、事实/currentness、scope、工具与验证、沟通 | StablePrefix | Context PromptAsset，独立Review/publish |
| Family Overlay | 从厂商官方Coding Agent或模型template提取的模型偏好 | StablePrefix或SemiStable | Context PromptAsset + Upstream Provenance |
| Harness Overlay | 该Route真实支持的system/instruction/tool/plan/session能力与Residual | SemiStable | Model Invoker exact Profile current projection |
| Dynamic Tail | cwd、时间、OS/shell、项目规则、tools、skills、scope、Run/Session/Turn、当前任务 | Dynamic | 各Owner exact-current refs，经Frame冻结 |

这样既借鉴官方工程经验，又避免把某个产品的工具名、审批模式、Session实现或隐藏权限带进Praxis。Family Overlay不能覆盖Praxis治理、Runtime/Tool/Sandbox/Review强制门禁。

## 2. Praxis Core Stable语义块

| Block ID | 稳定语义 | 主要官方证据 | 禁止项 |
|---|---|---|---|
| `core.role-goal` | 以完成用户当前目标为主；先理解事实再行动；不编造完成 | Codex task execution、Kimi ultimate reminders、Grok user_query | 厂商品牌自述、具体模型名 |
| `core.instruction-authority` | 按系统/开发者/项目/用户与Owner事实的冻结优先级执行；冲突Fail Closed | Codex AGENTS.md、Kimi system-reminder/project info | 把普通tool output升级为system指令 |
| `core.scope-safety` | 只改授权范围，保留脏工作树，危险/外部动作走治理 | Codex scoped changes、Grok action_safety | 自报审批、伪造Permit |
| `core.context-truth` | 优先live/exact current refs；Unknown只Inspect；引用必须有版本/digest | Kimi fact checking/context management + Praxis合同 | 模型记忆充当Owner current |
| `core.tool-discipline` | 选择公开能力；工具调用与用户沟通分离；Observation不冒充Fact | Codex/Grok tool guidance | 写死厂商tool name、绕过Gateway |
| `core.change-quality` | 根因、最小完整改动、遵循代码库惯例、不顺手改无关内容 | Codex/Kimi coding guidelines | 过度抽象、无授权重构 |
| `core.verify-deliver` | 按风险运行测试/检查，真实报告命令与结果，不把红灯说成完成 | Codex validating work、Kimi verify before done | 伪造测试、隐藏阻断 |
| `core.communication` | 过程简短可核验，最终先结论后证据；语言由用户/项目规则决定 | Codex responsiveness、Grok output efficiency、Kimi language | 固定英语、强制厂商格式 |

这些block是Praxis自有表达，不逐句复制上游；每个block必须保存`TransformID/RulesDigest/Output ContentRef`和独立Review Evidence。

## 3. Family Overlay

### 3.1 Codex/OpenAI

- 来源：Codex公开base instructions；
- 保留：短preamble、适度plan、直接执行、`rg`/patch式小改动、验证与简洁handoff的行为偏好；
- 参数化：工具选择、approval/sandbox、计划接口、文件链接格式；
- 删除：Codex品牌/产品自述、固定tool名称、宿主权限和UI renderer假设；
- 建议Overlay blocks：`codex.progress-contract`、`codex.plan-thresholds`、`codex.edit-verify-style`。

### 3.2 Gemini

- 来源：Gemini CLI `PromptProvider + modern/legacy snippets`完整composer；
- 保留：core mandates、research/strategy/implementation/verification workflow、上下文与skills/memory的显式管理；
- 参数化：interactive/yolo/plan、sub-agent、task tracker、sandbox、git repo、tools、memory/skills；全部必须来自Harness/Profile或Dynamic refs；
- 禁止：只取facade、把modern snippets用于不支持的模型/Profile、把Gemini CLI memory文件当Praxis Memory Owner事实；
- 建议Overlay blocks：`gemini.workflow`、`gemini.context-efficiency`、`gemini.feature-gated-capabilities`。

### 3.3 Kimi

- 来源：Kimi Code Agent Core V2 system template；
- 保留：语言跟随、Prompt/Tool纪律、最小且完整的代码变更、事实核对、压缩后继续当前目标；
- Dynamic化：`KIMI_OS/SHELL/NOW/WORK_DIR/DIRS/AGENTS_MD/SKILLS`及role additions；
- 禁止：将模板里的`system-reminder`权威规则直接复刻到Praxis普通Tool内容；权威必须由Praxis注入层决定；
- 建议Overlay blocks：`kimi.language-and-directness`、`kimi.minimal-complete-change`、`kimi.compaction-continuity`。

### 3.4 Grok/xAI

- 来源：Grok Build公开template；安全prompt另作Policy source；
- 保留：dangerous/external action safety、专用工具优先、后台任务与用户可见沟通分离、精炼输出；
- Dynamic化：system label、interactive/non-interactive、tool-by-kind、background monitor与user guide；
- 禁止：解密/推断`prompt_encrypted.rs`；Policy prefix不得与base prompt合并后失去独立来源；
- 建议Overlay blocks：`grok.action-safety`、`grok.tool-specialization`、`grok.background-observability`。

### 3.5 Claude Agent SDK

- 只保存official SDK `SystemPromptPreset{type:"preset", preset:"claude_code"}` exact来源引用；不生成“官方Claude Code正文”ContentRef；
- 若exact Harness Profile证明支持preset+append/custom，Praxis Core作为独立PromptAsset注入并在Expected/Actual Manifest中核验；若opaque或不支持，返回Residual/Fail Closed；
- T3Code Claude driver不得把其本地option值升级为preset已生效事实。

### 3.6 DeepSeek

- 只使用官方DeepSeek Coder instruct/chat template和programming-assistant role作为A3 profile证据；
- Agent行为来自Praxis Core，不从模板虚构plan/tool/approval/session能力；
- exact model/template revision必须由Model Invoker Profile current绑定后才可选用。

### 3.7 MiniMax

- 只使用MiniMax M2.5公开默认system说明作为A3 profile证据；当前license闭包未完成，不复制正文；
- README中“在Claude Code scaffolding评测”只是benchmark setup，不授予Claude prompt或工具语义；
- license与exact Profile current未闭合时只可Residual，不能生成“MiniMax官方Coding Agent Prompt”。

## 4. T3Code兼容投影

未来宿主Adapter只消费：

```text
PromptAssetRef
PromptUpstreamProvenanceRef
Stable/SemiStable/Dynamic ClosureDigest
Model Invoker SemanticRouteProfileRef + CurrentProjection
ExpectedInjectionManifestRef
```

然后映射为T3Code `ProviderDriverKind/ModelSelection`所需的外部参数。Context不import T3Code types；T3Code的`promptInjectedValues`不进入上述exact闭包，也不形成ActualInjection Observation。

## 5. Candidate生成顺序

```text
official exact bytes + license
  -> provenance verify
  -> semantic extraction candidate
  -> remove host/product/tool/permission claims
  -> Praxis Core / Family Overlay split
  -> Stable/Semi/Dynamic closure
  -> PromptAsset validate + preview
  -> independent semantic review
  -> pre-release lifecycle only
```

首批建议按`Codex -> Gemini -> Kimi -> Grok -> Claude preset reference -> DeepSeek profile -> MiniMax profile`推进。前四家各自保留独立Overlay，不共享一个未经Evidence证明的“通用模型Prompt”。

## 6. 硬反例

- 七家各复制一份完整长Prompt，公共修复产生漂移；
- 为提高cache命中把cwd/time/tools/profile current塞入StablePrefix；
- Family Overlay覆盖Praxis Authority/Effect/Review门禁；
- Gemini modern能力在legacy/不支持Profile上启用；
- Kimi环境placeholder未materialize或materialize后仍计入stable digest；
- Claude preset正文缺失却从第三方/模型复述补造；
- MiniMax复用Claude Code benchmark scaffolding后标成官方MiniMax Prompt；
- DeepSeek chat template被宣称拥有Agent tool loop；
- Grok safety Policy与base prompt混合后丢失独立digest；
- T3Code provider字符串或UI option决定Prompt current/ActualInjection。

## 7. 当前唯一业务确认点

建议正式采用“**一份Praxis Core Stable Prompt + 四份官方Coding Agent派生Overlay + 三份preset/profile兼容层**”，而不是维护七份完整复制Prompt。确认后再生成具体pre-release PromptAsset候选正文；在确认前只保留本设计，不把文字写入Go fixture或production资产。
