# 订阅调用与官方 Harness 路由研究（2026-07-12）

## 1. 研究目的与边界

本研究回答两个不同问题：

1. 某个订阅权益能否由 Praxis 直接按模型 API 调用；
2. 不能直接调用时，是否存在厂商公开支持的 CLI、SDK、App Server 或 ACP 执行面，可由 Praxis 作为外部 Agent Harness 调用。

本文件是设计阶段的事实与建议，不是已批准计划，也不是法律意见。本轮没有新增代码、没有登录账号、没有读取真实凭据、没有执行真实付费调用。

研究时间为 2026-07-12。厂商订阅条款、支持工具名单和 OAuth allowlist 均属于高漂移事实，进入实现前必须重新刷新。

## 2. 总结论

不能把所有订阅都做成一种“兼容 API Provider”。当前应明确分成两类：

```text
Praxis Runtime
  |
  +-- Direct Model Route
  |     `-- model-invoker：Praxis持有官方允许的独立Key并执行模型协议
  |
  `-- Official Agent Harness Route
        `-- agent-runtime：厂商CLI/SDK/App Server拥有认证、会话、工具循环和额度
```

两类不能互相伪装：

- Direct Model Route 返回模型级流、工具调用和 usage，由 Praxis 拥有模型调用循环；
- Official Agent Harness Route 返回 Agent 级事件或最终结果，由厂商 Harness 拥有内部模型调用、会话和一部分工具循环；
- 禁止抽取消费者 OAuth token 后绕过官方 Harness 直连未公开后端；
- 禁止复制其他项目的 OAuth client ID、User-Agent 或官方客户端身份；
- “中转一次”应定义为 Praxis 启动或连接一个官方公开执行面一次，不是通过多层代理把订阅伪装成通用模型 API。

## 3. 当前分级矩阵

| 路线 | 当前建议 | Praxis执行面 | 核心理由 |
|---|---|---|---|
| Kimi Code会员 | `direct_conditionally_allowed` | `model-invoker`现有订阅Route | 官方允许主流Coding Agent和OpenClaw/Hermes类Agent框架，要求真实客户端身份 |
| MiniMax Token Plan | `direct_allowed_with_entitlement` | `model-invoker`现有订阅Route | 官方提供Subscription Key、API测试示例及其他工具入口，允许Agent/Coding工作流 |
| Xiaomi MiMo Token Plan | `direct_coding_only` | `model-invoker`现有订阅Route | 仅允许编程工具；明确禁止自动脚本、非Coding请求和自定义应用backend |
| Alibaba Coding/Token Plan | `direct_coding_only` | `model-invoker`现有订阅Route | 允许第三方编程工具/OpenClaw，但禁止自动化平台、API测试器和自定义backend |
| GLM Coding Plan | `official_tool_only` | 不进入默认Callable Catalog | 仅限官方支持名单；SDK式自接入、非支持工具、自有应用、bot、SaaS和代理均受限 |
| xAI按量API | `direct_payg` | `model-invoker`现有xAI Route | 官方Responses API |
| SuperGrok/X Premium | `official_harness` | 优先Grok Build ACP；可评估官方点名的OpenClaw/OpenCode | xAI正式支持若干第三方Agent，但OAuth客户端受注册/allowlist约束；Praxis尚未被点名 |
| OpenAI按量API | `direct_payg` | `model-invoker`现有OpenAI Route | 官方Responses/Chat API |
| ChatGPT/Codex订阅 | `official_harness` | Codex app-server | OpenAI公开app-server并让其拥有ChatGPT OAuth；未公开授权Praxis复刻Codex后端HTTP |
| Anthropic按量API | `direct_payg` | `model-invoker`现有Anthropic Route | 官方Messages API |
| Claude Pro/Max订阅 | `official_harness` | Claude Agent SDK或`claude -p` | Anthropic当前仍承认Agent SDK、`claude -p`和基于Agent SDK的第三方应用消耗订阅额度；OpenCode式token直连被其自身文档标为禁止 |
| GitHub Copilot订阅 | `official_sdk` | GitHub Copilot SDK，ACP为备选 | GitHub公开多语言SDK、用户OAuth、CLI server和ACP，明确允许应用按用户Copilot订阅调用 |
| Gemini Developer API/Vertex | `direct_payg` | `model-invoker`现有Google Route | 官方API/云认证 |
| Google个人登录/Gemini Code Assist | `official_harness` | Gemini CLI ACP或headless stream-json | 官方开源CLI明确支持Google Sign-In、程序化输出与ACP；权益按实际账号类型判断 |
| Google消费者/Antigravity权益 | `official_cli_only` | Antigravity CLI `--print`一次调用 | 没有找到面向任意第三方应用的订阅OAuth/SDK合同；第三方复用Gemini/Antigravity OAuth有封号报告 |

## 4. 国内与兼容订阅路线

### 4.1 Kimi Code

官方事实：

- 提供独立会员API Key和OpenAI/Anthropic兼容Endpoint；
- 官方列出Claude Code、Roo Code、OpenCode，并明确支持OpenClaw、Hermes等通用Agent框架；
- 固定模型ID为`kimi-for-coding`；
- 要求保留工具真实身份，篡改User-Agent可能导致会员权益停用；
- 社区指南限定为交互式使用；企业集成和商业服务应转Kimi开放平台。

结论：现有`kimi-code` Adapter方向成立，但只能由可信宿主证明“个人、单租户、前台、交互式”后激活。Praxis必须发送自己的真实身份，不能冒充Kimi CLI、OpenCode或Claude Code。

官方来源：

- <https://www.kimi.com/code/docs/en/>
- <https://www.kimi.com/code/docs/en/kimi-code/community-guidelines.html>

### 4.2 MiniMax Token Plan

官方事实：

- 每个用户/Team拥有Subscription Key，与PAYG Key不可互换；
- Token Plan面向Agent、Coding和多模态工作流；
- 官方列出OpenClaw、Claude Code、Cursor、TRAE、Hermes等，并提供“Other Tools”；
- Quick Start直接给出Anthropic SDK调用示例和可选API测试；
- 额度耗尽后可由用户主动换PAYG Key，或使用已购买Credits；Praxis不能静默替用户切换计费来源。

结论：这是当前订阅路线中直接调用依据最强的一类。现有`minimax-token-plan` Adapter可继续保留，但Credential Profile必须把Subscription Key、Credits/PAYG状态和用户选择分开。

官方来源：

- <https://platform.minimax.io/docs/token-plan/intro>
- <https://platform.minimax.io/docs/token-plan/quickstart>

### 4.3 Xiaomi MiMo Token Plan

官方事实：

- 官方提供`tp-*`专属Key以及中国、新加坡、欧洲的OpenAI/Anthropic兼容地址；
- 支持Claude Code、OpenClaw、OpenCode、Kilo Code、Cline、Hermes等主流编程工具和模型框架；
- 套餐额度只能用于编程工具；自动脚本、自定义应用backend和明确非Coding请求被禁止；
- 用户协议同时禁止通过未获授权的第三方软件使用服务，因此“主流工具/框架”授权不能被扩张为任意应用授权。

结论：现有`mimo-token-plan`只可在Praxis真实处于交互式编程工具模式时激活。通用Agent、后台任务、定时任务、API服务、批处理和非Coding请求必须在Provider触达前拒绝。

官方来源：

- <https://mimo.mi.com/docs/tokenplan/subscription>
- <https://mimo.mi.com/docs/en-US/quick-start/faq/api-integration>
- <https://mimo.mi.com/docs/quick-start/terms/user-agreement>

### 4.4 Alibaba Coding Plan / Token Plan

官方事实：

- 官方允许支持自定义Endpoint的第三方编程工具，并为OpenCode、OpenClaw等提供配置说明；
- Coding Plan使用`sk-sp-*`专属Key和`coding...`专属Endpoint；
- 官方FAQ强调不能把通用PAYG凭据与套餐凭据混用，额度耗尽不会自动切PAYG；
- 自动化平台、API测试器、自定义应用和backend不属于套餐允许范围。

结论：现有`alibaba-plan`只能服务真实交互式编程产品面。不能因为OpenClaw被允许，就把任意Praxis后台Agent都解释成OpenClaw类场景。

官方来源：

- <https://help.aliyun.com/en/model-studio/coding-plan>
- <https://help.aliyun.com/en/model-studio/more-tools>
- <https://help.aliyun.com/en/model-studio/coding-plan-faq>

### 4.5 GLM Coding Plan

官方事实：

- 套餐仅限官方支持的指定工具；当前名单包括Claude Code、OpenCode、OpenClaw、Kilo、Cline等；
- 未授权或不支持的工具、SDK式接入、第三方集成可能被限制；
- 自有应用、bot、网站、SaaS、模型能力代理和向第三方提供能力均被禁止，除非另有书面协议；
- Praxis不在当前官方支持工具名单。

结论：维持`official_client_only + research_only + callable=false`。即使Praxis可以生成与OpenCode相同的HTTP请求，也不构成授权。若通过OpenCode启动任务，只能被设计为用户可见的外部工具交接，不能把OpenCode隐藏成通用模型代理；在实现前仍应向Z.AI取得Praxis或该交接模式的书面确认。

官方来源：

- <https://docs.z.ai/legal-agreement/subscription-terms>
- <https://docs.bigmodel.cn/cn/coding-plan/overview>
- <https://docs.bigmodel.cn/cn/coding-plan/tool/others>

## 5. Grok、OpenCode与OpenClaw

### 5.1 官方支持边界

xAI在2026年5月至6月已正式发布以下订阅集成：

- Hermes Agent；
- OpenClaw；
- OpenCode；
- Kilo Code；
- Warp；
- xAI自己的Grok Build CLI。

因此SuperGrok并非只能在grok.com中使用，也不能再笼统归类为“第三方全部禁止”。但授权是按产品/集成给出的，不能自动外推到Praxis。

官方来源：

- <https://x.ai/news/grok-openclaw>
- <https://x.ai/news/grok-opencode>
- <https://x.ai/news/grok-kilocode>
- <https://x.ai/news/grok-warp>
- <https://x.ai/news/grok-build-cli>

### 5.2 OpenCode如何实现

核查OpenCode提交`34e58090595d44e3e7cc37498f16753a98627456`（2026-07-11）：

- `packages/opencode/src/plugin/xai.ts`使用PKCE、loopback callback或device code；
- 使用xAI注册的Grok CLI OAuth client ID和`grok-cli:access api:access` scopes；
- OAuth token由插件刷新，并作为Bearer发送到xAI SDK默认API地址；
- 源码明确说明非allowlist客户端的loopback OAuth会被xAI拒绝，redirect URI也必须完全匹配注册值。

这条实现对OpenCode是可接受的，因为xAI已经官方宣布支持OpenCode。Praxis不能复制该client ID；正确路径是向xAI注册Praxis，或调用官方Harness。

### 5.3 OpenClaw如何实现

核查OpenClaw提交`8d39c1baa52439c339a2b27f3fa4b6df43775eac`（2026-07-12）：

- `extensions/xai/xai-oauth.ts`同样使用注册的xAI OAuth client、OIDC discovery、PKCE/device-code和token刷新；
- xAI官方文章明确给出OpenClaw onboarding和VPS device-code用法；
- OpenClaw还把xAI认证复用于模型、搜索、图像、视频、语音等多个Provider能力面。

这证明xAI为获批开源Agent提供的是产品级OAuth集成，而不是任意程序都能复用的消费者token接口。

### 5.4 Praxis建议

优先使用Grok Build：

- xAI官方Grok Build公开headless `-p`和完整ACP支持；
- ACP适合Praxis保持会话、事件和权限边界；
- xAI仍拥有OAuth、额度、模型目录和内部工具循环；
- Praxis不需要读取或转存SuperGrok refresh token。

只有xAI为Praxis分配独立OAuth client/书面确认后，才重新评估SuperGrok直连Route。xAI PAYG API继续由现有`model-invoker` Route承担。

## 6. Codex与Claude Code

### 6.1 ChatGPT/Codex订阅

OpenAI官方事实：

- Codex包含在ChatGPT计划中；
- `codex app-server`是OpenAI公开的、用于驱动丰富界面的JSON-RPC接口；
- app-server支持ChatGPT managed OAuth、device code、token自动刷新、账号/计划类型和限额读取；
- app-server拥有Codex线程、turn、审批、工具、技能、Apps和MCP等语义。

OpenCode的实现则直接复制Codex OAuth client，重写请求到`https://chatgpt.com/backend-api/codex/responses`。这说明技术可行，但OpenAI没有在已找到的官方资料中授权Praxis或任意第三方复用该未公开后端。

结论：

- OpenAI API Key路线继续直连`model-invoker`；
- ChatGPT/Codex订阅路线通过`codex app-server`，不进入模型Provider；
- Praxis只保存app-server本地身份引用和会话绑定，不读取OAuth明文；
- 不复制OpenCode OAuth实现，不直接调用`chatgpt.com/backend-api`。

官方来源：

- <https://help.openai.com/en/articles/11369540-using-codex-with-chatgpt>
- <https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md>

### 6.2 Claude Pro/Max订阅

Anthropic当前事实：

- 2026-06-15原计划把Agent SDK与`claude -p`改为独立月度credit，随后暂停；
- 暂停期间“没有变化”：Agent SDK、`claude -p`和第三方应用仍消耗Claude订阅额度；
- 官方明确把“基于Claude Agent SDK并通过Claude订阅认证的第三方应用”列为支持场景；
- 共享生产自动化应使用Claude Platform API Key；订阅面属于个人实验、交互和有限自动化。

OpenCode当前文档明确写明：旧版曾内置Claude Pro/Max插件，但Anthropic明确禁止这种OpenCode直接订阅插件，因此1.3.0后不再内置。

结论：

- Claude API Key路线继续直连`model-invoker`；
- Claude订阅路线必须通过官方Claude Agent SDK或`claude -p`；
- 不读取Claude Code凭据后自行构造Messages请求；
- 不复用OpenCode社区认证插件；
- 生产、多用户、服务端共享自动化直接要求PAYG/商业合同。

官方来源：

- <https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan>
- <https://docs.anthropic.com/en/docs/claude-code/getting-started>

## 7. GitHub Copilot

GitHub已经给出完整官方路径：

- 2026-01正式宣布Copilot支持OpenCode；
- Copilot SDK公开支持TypeScript、Python、Go、.NET、Java，Rust为技术预览；
- SDK使用Copilot CLI server和JSON-RPC，支持本地子进程或外部server；
- 支持已登录用户、GitHub OAuth App、环境变量和BYOK；
- 官方OAuth指南明确请求代表每个用户发起，并消耗该用户Copilot订阅；
- Copilot CLI ACP公开允许第三方IDE、自动化系统、自定义前端和多Agent系统。

结论：Praxis不应复制OpenCode的`api.githubcopilot.com`私有协议适配器。优先采用官方Go Copilot SDK；若需要更强隔离或统一Agent协议，再使用Copilot CLI ACP。该路线属于Official Agent Harness，不伪装成OpenAI/Anthropic模型API。

当前限制：Copilot SDK仍为Public Preview，生产批准前需要固定SDK/CLI版本、验证并发与多租户、权限回调、模型目录、premium request计量和升级兼容。

官方来源：

- <https://github.blog/changelog/2026-01-16-github-copilot-now-supports-opencode/>
- <https://github.com/github/copilot-sdk>
- <https://docs.github.com/en/copilot/how-tos/copilot-sdk/setup/github-oauth>
- <https://docs.github.com/en/copilot/reference/copilot-cli-reference/acp-server>

## 8. Gemini与Antigravity

### 8.1 直接订阅OAuth不采用

当前没有找到Google面向任意第三方Agent公开的消费者Gemini/Antigravity订阅OAuth或SDK合同。OpenClaw当前Google插件在认证前主动警告：

- 这是非官方集成，Google不背书；
- 有用户报告第三方Gemini CLI/Antigravity OAuth客户端导致账号限制或停用。

因此不能把OpenClaw的Google OAuth实现当作Praxis合规依据。Gemini Developer API与Vertex AI仍按官方API/云路线直连。

### 8.2 Gemini CLI官方Harness

本轮刷新后，Gemini CLI仍是活跃的Google官方开源Agent，并明确提供：

- Google Sign-In，面向个人开发者和Gemini Code Assist License；
- Gemini API Key与Vertex AI认证；
- headless `text/json/stream-json`；
- ACP stdio JSON-RPC，覆盖session、cancel、approval mode、模型切换和客户端文件系统代理；
- 完整system prompt覆盖、工具allowlist、GEMINI.md、skills、hooks、extensions、MCP和memory配置。

因此先前“个人/免费Gemini CLI已整体迁移到Antigravity CLI”的归纳不再作为当前事实。Gemini CLI必须作为独立Official Harness Route和Gemini编码行为样本。它的Google Sign-In/Code Assist权益不能自动等同于Google AI Pro/Ultra或Antigravity消费者权益。

官方来源：

- <https://github.com/google-gemini/gemini-cli>
- <https://geminicli.com/docs/cli/headless>
- <https://geminicli.com/docs/cli/acp-mode>

### 8.3 Antigravity CLI中转

核查官方Linux amd64二进制与公开资料：

- 版本：`1.1.1`；
- 官方manifest SHA-512：`fcebad8247e0453097e2b26d839371c88df040e6fb18fac3fd8072851194739cf4624041469b57f529bc9c2e1140a6ce4cd5e7144e64e6efc5605a9f10a862b7`；
- 支持`--print`/`-p`单次非交互调用和`--print-timeout`；
- 支持Google Sign-In和远程/SSH登录；
- 当前没有公开ACP子命令，官方仓库仍有ACP功能请求。

结论：Antigravity消费者权益仍只设计为一次`agy --print`外部Harness调用，保持真实二进制、真实登录和真实身份。因为当前缺少公开结构化流协议，该路线先限定为单次任务/最终结果，不宣称完整工具事件、token usage或可续接模型流。

官方来源：

- <https://antigravity.google/docs/cli-overview>
- <https://github.com/google-antigravity/antigravity-cli>
- <https://developers.googleblog.com/en/an-important-update-transitioning-gemini-cli-to-antigravity-cli/>

## 9. 后续设计收口后的实现边界

### 9.1 保留在`model-invoker`的路线

- PAYG：OpenAI、Anthropic、Gemini Developer API、Vertex、xAI及现有其他厂商API；
- 受限订阅直连：Kimi、MiniMax、MiMo、Alibaba；
- GLM继续`callable=false`；
- 所有订阅Route继续由可信宿主提供ClientIdentity、Entitlement、用途、前后台、单租户和非生产证明。

### 9.2 不应塞进`model-invoker`的路线

- Codex app-server；
- Claude Agent SDK / `claude -p`；
- GitHub Copilot SDK / Copilot CLI ACP；
- Gemini CLI ACP/headless；
- 当前Kimi Code ACP/stream-json；旧Kimi CLI仅作`legacy_pinned`兼容Route；
- Qwen Code Agent SDK/ACP/daemon；
- Grok Build ACP；
- Antigravity CLI `--print`。

这些是Agent Harness，不是模型Provider。它们需要独立的上层运行时合同，但在用户确认前不创建新模块、不写计划、不实现代码。

### 9.3 统一安全要求

1. Harness进程必须使用厂商真实二进制和真实身份，不复制OAuth client或改写User-Agent。
2. Praxis只存Credential引用、profile ID和本地登录状态，不复制refresh token到Catalog或审计。
3. 用户账号、订阅和进程必须单用户隔离；禁止共享、池化或多租户复用消费者权益。
4. 后台、批处理、SaaS和生产用途按厂商条款分别拒绝；不能用“Agent”一词绕过用途限制。
5. 额度耗尽、401/403/429和套餐到期必须终止，不自动切PAYG、其他账号或其他Region。
6. Harness拥有内部工具循环时，Praxis不能再次把其内部tool call当普通模型tool call重放。
7. 每个官方Harness需要固定版本、能力探测、超时/取消、事件规范化、审批回传、工作区隔离和真实黑盒验收。

## 10. 后续设计已经闭合的决定

本研究当时留下的决定已由[Profile并集计划](../../plan/model-invoker/upstream-support-and-profile-union-v1.md)与[代表Route纸面编译合同](./representative-route-paper-compilation-and-union-conformance-v1.md)闭合：

1. [x] 确认双平面；`model-invoker`拥有模型级Route，官方CLI/SDK/App Server进入上层Harness执行面；
2. [x] 审核顺序更新为Codex → Claude → Gemini/Qwen → current Kimi/Copilot → Grok/MiMo → Antigravity；
3. [x] GLM Coding Plan继续`official_client_only/callable=false`，直到获得支持名单或书面许可；
4. [x] MiMo与Alibaba受限Offering继续由可信宿主按用途、交互、前后台和租户硬门禁；
5. [x] 禁止复制OpenCode/OpenClaw OAuth client、token、身份或消费者私有后端直连实现。
