# 模型专属 Coding Agent Prompt 官方上游审计

状态时间：2026-07-18。状态：七个目标家族与T3Code兼容边界已获用户确认；官方来源、证据等级与离线`PromptUpstreamProvenanceV1`设计冻结。本文不复制上游正文，不授予任何候选Prompt Authority、Review、published或production资格。

官方原始文件与许可证已按本次不可变commit逐字节留存，见[上游原件留存清单](upstream-sources/README.md)。

## 1. 总裁决

优先级固定为：**厂商官方开源 Coding Agent 明文 > 厂商官方 Agent SDK preset引用 > 厂商官方模型chat-template/profile > 非厂商开源工程对照**。来源等级只决定“可借鉴什么”，不自动证明对任意模型版本适用。

| 目标 | 官方来源 | 等级 | 当前裁决 |
|---|---|---:|---|
| Codex/OpenAI | `openai/codex`公开base instructions | A1 Coding Agent明文 | 可进入离线提取、删改、参数化候选；必须保留Apache-2.0来源闭包 |
| Gemini | `google-gemini/gemini-cli` PromptProvider及modern/legacy snippets | A1 Coding Agent明文 | 可进入候选；必须审计完整composer+snippets，禁止只取facade |
| Kimi | `MoonshotAI/kimi-code` Agent Core V2 system template | A1 Coding Agent明文 | 可进入候选；模板中的OS、shell、时间、workdir、project与skills必须从stable closure剥离 |
| Grok/xAI | `xai-org/grok-build`公开prompt template；`grok-prompts`安全前缀 | A1/A2 Coding Agent明文+Policy | 明文template可候选；安全前缀必须独立Policy层；加密prompt材料禁止冒充可审计正文 |
| Claude | `anthropics/claude-agent-sdk-*`的`claude_code` preset nominal | A2 SDK preset引用 | 只做SDK preset/profile兼容；官方preset正文未公开，不复制、不逆向、不声称拥有Claude Code官方Prompt |
| DeepSeek | `deepseek-ai/DeepSeek-Coder` instruct/chat template | A3 模型模板 | 只作模型家族chat-template/profile证据；不是完整Coding Agent loop Prompt |
| MiniMax | `MiniMax-AI/MiniMax-M2.5`公开默认system提示及Claude Code评测说明 | A3 模型模板 | 只作profile/default template证据；CLI不是Coding Agent，且本次审计未闭合可导入license，正文不导入 |
| T3Code | `pingdotgg/t3code` provider/model公开contracts | Consumer | 仅作消费端兼容目标；不是Prompt来源、Owner、currentness或Authority |
| OpenCode | `anomalyco/opencode`多模型适配 | B 工程对照 | 只作差异Evidence；不得标注为对应厂商官方Prompt |

## 2. 不可变来源记录

所有digest均为原始文件bytes的SHA-256；bytes/digest只证明本次读取内容，不表示已导入、已适配或已获production资格。

### 2.1 OpenAI Codex

- 官方仓库：[`openai/codex`](https://github.com/openai/codex)；审计commit：`7c4aaf28c253161f1ed9a4fccc6229b1a4510891`；
- `codex-rs/protocol/src/prompts/base_instructions/default.md`：`20903 bytes`，SHA-256 `ac8ae107a0d72fe3476b430afb161ea4e67da2e446d778aefc44828160559807`；
- `LICENSE`：Apache-2.0，SHA-256 `d17f227e4df5da1600391338865ce0f3055211760a36688f816941d58232d8dc`；
- 该文件是公开默认base instructions，不代表线上Model Catalog或隐藏指令。

### 2.2 Google Gemini CLI

- 官方仓库：[`google-gemini/gemini-cli`](https://github.com/google-gemini/gemini-cli)；审计commit：`3ff5ba20fc1ad7d867218bbdb34756eb54d6eccb`；
- `packages/core/src/prompts/promptProvider.ts`：`11751 bytes`，SHA-256 `8dc79837635be5b2288a40a331c20c24024929068aa3f0e74239ebe3a90efa78`；
- `packages/core/src/prompts/snippets.ts`：`68680 bytes`，SHA-256 `7411a475e57bf80d01269cb1740ee76d9df0529dc583b4045ccd4a7ce6d4b776`；
- `packages/core/src/prompts/snippets.legacy.ts`：`56560 bytes`，SHA-256 `efed3124be0c59bbf029001dab2e2d04167f13e3ddb4eebd9a8451d7e77be416`；
- `docs/cli/system-prompt.md`：`4634 bytes`，SHA-256 `46d42b94733e625d548b3022d99a3a01211ae6f73e7554413e9da45444528241`；
- `LICENSE`：Apache-2.0，SHA-256 `58d1e17ffe5109a7ae296caafcadfdbe6a7d176f0bc4ab01e12a689b0499d8bd`；
- `packages/core/src/core/prompts.ts`只是facade，单独读取必须Fail Closed。

### 2.3 Moonshot Kimi Code

- 官方仓库：[`MoonshotAI/kimi-code`](https://github.com/MoonshotAI/kimi-code)；审计commit：`7d393b56fb324773fad0af58c7da52b254365cb4`；
- `packages/agent-core-v2/src/app/agentProfileCatalog/system.md`：`20726 bytes`，SHA-256 `68662e7d9bb565d6c3606b58a9c13dd5377bd47474e10ed8b39ef347a053219c`；
- `packages/agent-core-v2/src/app/agentProfileCatalog/promptPrefix.ts`：`878 bytes`，SHA-256 `44afe91ba53db33bc4ea1b158e062b02f3d5a0520bbe3a747cb935a7fb7d4c93`；
- legacy `packages/agent-core/src/profile/default/system.md`与V2 system正文digest相同；
- `LICENSE`：MIT，SHA-256 `23cc68e17992e0b512ae2e80afc5787d7d8e0fbfbdb4fff54ec0245508fa400e`。

### 2.4 xAI Grok

- 官方Coding Agent仓库：[`xai-org/grok-build`](https://github.com/xai-org/grok-build)；审计commit：`98c3b2438aa922fbbe6178a5c0a4c48f85edc8ce`；
- `crates/codegen/xai-grok-agent/templates/prompt.md`：`4638 bytes`，SHA-256 `c805ee840c5550f501432bf27ae454c7f59f3de4e331ae5107913b4c9f49dafb`；
- `crates/codegen/xai-grok-agent/src/prompt/template.rs`：`34990 bytes`，SHA-256 `61074de6dbdd95cff34c9482414359a28f11714060116a8d3783331d4a0f1e3a`；
- `prompt_encrypted.rs`：`140898 bytes`，SHA-256 `1745b9cc16915ad2038408c838658d4458a3363b4d6f1a41613fe4ec23b459a4`，只记录存在与digest，禁止当作可审计明文；
- `LICENSE`：Apache-2.0，SHA-256 `116f7778b9802e569b7fa3a532b17bd80eb13c67837def01eed093d4ea472f28`；
- 官方政策仓库[`xai-org/grok-prompts`](https://github.com/xai-org/grok-prompts)，commit `a7c186f5ccac95875c0041aed60398f6ecb6d6c7`；`grok_4_code_rc1_safety_prompt.txt`：`1083 bytes`，SHA-256 `c59f95de2fdd1e6954d1f332b9364f6b9ccf5ebdce082ea524cd6427cba920d6`；它是独立Policy来源，不能拼接后冒充Coding Agent base prompt。

### 2.5 Anthropic Claude Agent SDK

- 官方Python SDK：[`anthropics/claude-agent-sdk-python`](https://github.com/anthropics/claude-agent-sdk-python)，commit `f604b8c5dd18d2503c8daf52da495ad8db3aa92e`；
- `src/claude_agent_sdk/types.py`：`75580 bytes`，SHA-256 `06c1215aa27806a5221dc44c639afc47f4ef06b872dc3f94384d30d69fa92362`；只公开`{type:"preset", preset:"claude_code"}`等nominal shape；
- `examples/system_prompt.py`：`2553 bytes`，SHA-256 `5e4aadeb8002b409f8955284328267cae2858db0997d98dcab695a0fa055ba45`；
- `LICENSE`：MIT，SHA-256 `cebdde8a8fb9ee59e5eaaed19578bf8085aa7047562259c31f38f225c26f6812`；
- SDK许可不等于Claude服务条款，也不公开`claude_code` preset正文。Context只能保存preset exact引用和适配Evidence。

### 2.6 DeepSeek Coder

- 官方仓库：[`deepseek-ai/DeepSeek-Coder`](https://github.com/deepseek-ai/DeepSeek-Coder)；审计commit：`2f9fd85927c669dae3c0fbb2d607274023af243e`；
- `README.md`：`20346 bytes`，SHA-256 `aa0a95ca037f1d2c641421417abcaabaca230696cfc79b84cfea235153b8f6f2`；包含官方instruct/chat template和最小programming-assistant system声明；
- `LICENSE-CODE`：MIT，SHA-256 `6e4c38e1172f42fdbff13edf9a7a017679fb82b0fde415a3e8b3c31c6ed4a4e4`；
- 未发布完整Coding Agent harness prompt，不能从模型chat template推导工具、权限或宿主语义。

### 2.7 MiniMax

- 官方[`MiniMax-AI/cli`](https://github.com/MiniMax-AI/cli)，commit `f48a4e7703af484d412aacba0d06ccb7b70eaa79`；`src/utils/prompt.ts`为终端交互helper而非LLM system prompt：`3024 bytes`，SHA-256 `b0f26ce3ab5c5aeb542911d8ab8b5adca05e076ac8d7522427d3ce0c5a21f3a9`；
- 官方[`MiniMax-AI/MiniMax-M2.5`](https://github.com/MiniMax-AI/MiniMax-M2.5)，commit `0fe00c843c16e7081a9631daeafc11288f5f871c`；`README.md`：`20087 bytes`，SHA-256 `0b64d5b63f22cf1afda49c4fd6c189791c2dd68324a00e7c1df3debd672bdf3c`；
- 本次commit未发现可闭合的根LICENSE，因此只记录profile证据，不导入正文、不继承Claude Code评测脚手架。

### 2.8 T3Code兼容目标

- 官方仓库：[`pingdotgg/t3code`](https://github.com/pingdotgg/t3code)；审计commit：`8b5469863ae1dd696e696de30240ec3da607962d`；
- `packages/contracts/src/provider.ts`：`4489 bytes`，SHA-256 `45501bda98271090f3c624806697144069d26ee75746d168e32e61d3a42fc0bb`；
- `packages/contracts/src/model.ts`：`8142 bytes`，SHA-256 `75e74fa9493f49a8613d03129365789f58e731e7936cd75ca54793cc7052ecdf`；
- `LICENSE`：MIT，SHA-256 `935d8f2af0c703f9c39517ee57cc4930b19d02d533be930b63f0e82f93614b43`；
- T3Code Adapter未来只消费Praxis exact PromptAsset/Profile/closure refs并映射到其`ProviderDriverKind`/`ModelSelection`。`promptInjectedValues`只是UI/provider option metadata，不授予Authority、currentness或已注入事实。

### 2.9 OpenCode（只作B级对照）

- 仓库：[`anomalyco/opencode`](https://github.com/anomalyco/opencode)；审计commit：`69a80663a2ed7d671d2b4d5dd6f2d605714675a5`；
- `packages/opencode/src/session/system.ts`：`6127 bytes`，SHA-256 `3d190119fafac513049fc3bbf97e7bc0291cd345bfc1a03650ec1ddab8713717`；
- `prompt/anthropic.txt`=`8324e4cf58eb45d4d9d6fd120f5e8da59e0548de48e7e6aefcdfbf2923f40b4e`，`prompt/gpt.txt`=`83a66a46a5febbc21454161d5f053638b22d25d95e09d77b8f6da33debc848ad`，`prompt/gemini.txt`=`921750803b0314b88b8adc996e2afcf1a61fd7d9dd6dfcf812baeadac1468cf3`，`prompt/codex.txt`=`c30bca40693a47965e25ceac3f02d3709712af7abeab1278bba53a9efcffa928`；
- `LICENSE`：MIT，SHA-256 `625f0f619133f89bbbb2abe37369613dfa1885eba1e50d02170deb62bb42cb6b`。

## 3. 冻结变换链

```text
OfficialExactArtifact(s)
  -> verify immutable commit/path/bytes/license
  -> ExtractRangeManifest
  -> remove upstream host/product/tool/permission claims
  -> parameterize only approved Praxis public capabilities
  -> split StablePrefix / SemiStable / DynamicPlaceholder
  -> canonical ContentRef(s)
  -> PromptAsset candidate
  -> independent review (still not published)
```

每步必须seal `TransformID + Revision + Kind + InputDigest + RulesDigest + ToolDigest + OutputDigest`。Stable closure排除time、cwd、OS/shell observation、runtime permission、dynamic tool list、route、session/turn和self digest；SemiStable只放exact、可current复读且在Frame内冻结的内容；其余进入Dynamic Tail。

`PromptUpstreamProvenanceV1`只拥有来源与变换事实，不拥有目标模型选择。模型适用性必须绑定Model Invoker Owner发布的nominal exact `SemanticRouteProfileRef + current reader`；live只有可计算digest的Profile对象而无公共exact ref/reader，因此这是production Port Delta。Context不得复制`ModelFamily/RouteID/Profile` nominal。

## 4. T3Code映射边界

```text
Context PromptAssetRef + provenance/closure digests
              |
Model Invoker exact profile/route ref (future public reader)
              |
host composition adapter
              v
T3Code ProviderDriverKind / ModelSelection
```

Context不import T3Code类型；T3Code不mint PromptAsset、Provenance、Expected/Actual Injection或current事实。兼容层不得按字符串猜模型家族，也不得把T3 provider option当已注入Receipt。

## 5. 硬反例

- 把OpenCode的`anthropic.txt`标为Anthropic官方Prompt；
- 把Claude SDK preset名字当作可复制的preset正文；
- 把MiniMax CLI交互helper或M2.5的Claude Code benchmark说明当作MiniMax官方Coding Agent Prompt；
- 解密、推断或模型复述Grok encrypted prompt后声称官方明文；
- 只读取Gemini facade，或忽略Kimi动态placeholder；
- 用`HEAD/main`浮动链接、文件名或片段ID代替原始bytes+exact range digest；
- 上游Prompt直接取得Praxis Authority/Review/published；
- 把SDK preset、模型chat-template与完整Coding Agent Prompt混为同级；
- StablePrefix包含time/cwd/tool availability/route/session；
- Context定义第二套ModelFamily/Profile nominal，或T3Code `promptInjectedValues`充当current/actual injection；
- license未闭合仍复制正文，或变换时删除notice/归属；
- 不同模型家族共用同一结果却没有exact compatibility Evidence。

## 6. 实施门

- **GO**：Context Owner-local离线provenance DTO、canonical seal、原始bytes/digest核验、transform chain连续性、closure分层、fixture与conformance；Codex/Gemini/Kimi/Grok明文只作为审计fixture输入。
- **CONDITIONAL**：Claude只做preset reference；DeepSeek/MiniMax只做profile evidence，不生成“官方Coding Agent Prompt”。
- **NO-GO**：网络抓取、自动license裁决、production Prompt发布、Model Invoker/Harness/Application/T3Code Adapter、route current绑定和真实注入；等待对应Owner公共合同与composition root。
