# 全量上游支持与 Profile 并集计划 v1

## 1. 计划状态

- 模块目录：`model-invoker`；同时覆盖尚待命名和确认的 Official Agent Harness 执行平面
- 计划版本：`v1-design-closed`
- 创建时间：2026-07-12
- 当前状态：研究与v1语义设计阶段已完成；Runtime实现已由后续[独立实施计划](./execution-semantic-union-runtime-v1.md)承接并完成离线验收，本文件保留为历史设计总计划
- 授权演进：本文件形成时只授权计划；2026-07-13用户另行授权离线代码实现；账号、凭据和真实付费调用仍未执行
- 事实基线：[`provider-matrix.md`](../../design/model-invoker/provider-matrix.md)、[`subscription-and-official-harness-research-20260712.md`](../../design/model-invoker/subscription-and-official-harness-research-20260712.md)、[`upstream-official-agent-behavior-and-harness-delta-research-20260713.md`](../../design/model-invoker/upstream-official-agent-behavior-and-harness-delta-research-20260713.md)与[`代表Route纸面编译及一致性合同`](../../design/model-invoker/representative-route-paper-compilation-and-union-conformance-v1.md)

本计划先回答“Praxis 通过什么正式路径获得每一种上游支持”，再定义这些路径如何被 Profile 组合。它不是“所有模型都伪装成 OpenAI API”的计划，也不把技术可行等同于厂商授权。

## 2. 本轮计划产物与最终可预见结果

### 2.1 本轮只产生

1. 一份覆盖直连 API、官方 SDK、CLI、App Server、ACP、云 SDK、受限订阅 Key、自托管和研究队列的总计划；
2. 一套已闭合的Profile分层与合成合同；
3. 每个主要上游的合法组合、禁止组合和待核验组合；
4. 后续设计、实现、测试和验收的粗细粒度清单；
5. 已闭合的Profile、IME、事件、Effect与执行平面决定；
6. OpenAI Direct、Codex、Claude、Gemini、current Kimi与Qwen六条代表Route纸面编译；
7. 跨Route conformance、取消/终态合同与negative golden；
8. 可编辑draw.io设计图和PNG预览。

### 2.2 首个独立实施切片的已交付与剩余范围

2026-07-13获授权的首个独立实施切片已经交付：现有`model-invoker`内的Direct Model Route桥、`execution/harness`执行平面、三因子Profile合同与组合验证器、Codex/Claude/Gemini/current Kimi/Qwen五条Harness Adapter、Manifest/Mapping/Residual审计，以及单元、白盒、黑盒、本地集成和中文模块说明。

该切片没有完成、也没有冒充完成的全量目标仍包括：

1. 覆盖全部上游而不只是六条代表Route的机器可读`SupportRouteRegistry`；
2. 每条剩余上游路线的Adapter/Harness卡、版本锁定、能力探测和证据卡；
3. Profile弃用、迁移、回滚、导出和全量组合矩阵；
4. 真实账号、OAuth、订阅、官方二进制烟测和生产评审。

## 3. 统一术语与不可合并的维度

完整支持对象暂定为：

```text
SupportRoute
  = ModelFamily
  + Provider
  + Offering
  + Deployment
  + ExecutionSurface
  + Protocol
  + Endpoint
  + CredentialProfile
  + EntitlementProfile
  + HarnessProfile
  + SemanticProfile
  + PolicyProfile
```

其中：

- `Provider`：真正运营服务并承担鉴权、可用性和数据责任的一方；
- `Offering`：按量 API、消费者订阅、Coding Plan、Token Plan、企业合同等具体产品；
- `Deployment`：直连、AWS、GCP、Azure、地区、项目、Workspace、资源或自托管实例；
- `ExecutionSurface`：`direct_api`、`provider_sdk`、`protocol_sdk`、`agent_sdk`、`app_server`、`official_cli`、`acp_agent`、`cloud_sdk`；
- `Protocol`：Responses、Chat Completions、Messages、GenerateContent、Converse、Invoke、ACP、JSON-RPC 或厂商原生协议；
- `Credential`：API Key、ADC、SigV4、Entra ID、本机官方登录、官方 OAuth 或订阅专属 Key；
- `Entitlement`：额度、套餐、用途、用户、租户、前后台和自动化限制；
- `Profile`：以精确模型、Offering和执行面为路由键，把 Praxis 并集语义双向转换为上游 API/Harness 语义的命名合同；配置和引用只是其组成部分，不是其最终目的，也不能覆盖厂商合同。

## 4. 总体架构：两个执行平面

```text
Praxis并集语义请求
  |
  +-- Direct Model Route
  |     `-- model-invoker
  |           `-- Semantic Profile <-> 官方API / 云API / 正式兼容API / 受限订阅Key
  |
  `-- Official Agent Harness Route
        `-- Semantic Profile
              `-- 经允许的Harness准备层/二开Adapter
                    `-- 官方Agent SDK / App Server / CLI / ACP
                          `-- 厂商Harness拥有登录、会话、上下文预设和部分工具循环
```

### 4.1 Direct Model Route

- Praxis 拥有模型请求、模型流、工具调用循环、状态续接和统一语义映射；
- 凭据必须被该 Offering 正式允许用于对应 Endpoint；
- SDK 只是协议实现方式，不改变 Provider、计费方或产品边界；
- 返回模型级事件、usage、request ID 和映射报告。

### 4.2 Official Agent Harness Route

- 厂商 App Server、Agent SDK、CLI 或 ACP Agent 拥有认证、会话和部分内部 Agent 行为；
- Praxis 连接的是 Agent 执行面，不假装获得一个纯模型 API；
- 返回 Agent 级事件、审批、文件变更、工具活动或最终结果；
- 必须显式报告预置 system prompt、settings、skills、tools、cwd、session 和不可关闭的厂商上下文；
- Praxis应先使用厂商公开配置点、SDK扩展点或官方开源组件完成Harness准备，再由Route专属Semantic Profile消解其可观测语义差异；
- 官方组件允许二次开发时，可以在完成许可证、条款、版本和身份边界审核后包装或维护窄补丁；不能为了“统一”修改OAuth client、伪装产品身份或绕过套餐限制；
- OpenCode、OpenClaw可作为公开实现参考，但Praxis只复用设计经验和合法代码，不继承它们的OAuth身份、allowlist资格或厂商授权；
- 不抽取消费者 OAuth token 后绕过 Harness 直连未公开后端。

双平面只描述上游执行所有权；对上层则由同一套并集语义暴露。统一不意味着抹平差异：能够忠实转换的进入统一字段，部分转换的携带`MappingReport`，不能转换的能力明确拒绝或保留为命名空间扩展。

## 5. “如何获得支持”的路径分类

| 路径 | 支持来自哪里 | Praxis如何接入 | 典型上游 | 默认归属 |
|---|---|---|---|---|
| 官方直连 API | 厂商公开 API 合同 | HTTP/gRPC 或官方 SDK | OpenAI、Anthropic、Gemini、xAI | `model-invoker` |
| 正式兼容 API | 目标 Provider 明确声明兼容协议 | 协议 SDK + Provider 专属 Adapter | DeepSeek、Kimi、MiniMax、MiMo、Qwen | `model-invoker` |
| 云原生部署 | 云厂商服务合同 | 云 SDK、云鉴权或模型原厂 middleware | Bedrock、Vertex、Azure | `model-invoker` |
| 订阅专属 Key | 套餐官方发放的专属 Key/Endpoint | 专属 Route + Entitlement 门禁 | Kimi Code、MiniMax、MiMo、Alibaba | `model-invoker`，默认受宿主门禁 |
| 官方 Agent SDK | 厂商支持的 Agent 宿主合同 | SDK/sidecar，SDK拥有内部循环 | Claude Agent SDK、Copilot SDK | Official Harness |
| 官方 App Server | 厂商公开本地服务协议 | 启动/连接官方进程 | Codex app-server | Official Harness |
| 官方 CLI Headless | 厂商公开非交互命令 | 子进程、stdin/stdout、退出码 | `claude -p`、Gemini CLI、`agy --print`、Grok Build、Kimi/Qwen/MiMo Code | Official Harness |
| ACP/Wire/Daemon | 厂商公开 Agent 协议 | session/event/approval/文件系统代理 | Gemini、Grok、current Kimi ACP、legacy Kimi Wire、Qwen、MiMo、Copilot | Official Harness |
| 厂商批准 OAuth | 厂商给具体产品注册 client/allowlist | 只用 Praxis 自己获批的 client | xAI 对已批准产品 | 独立专项设计 |
| 自托管 | 用户控制模型服务器 | 实例专属 Route | vLLM、TGI、Ollama | 后续 Deployment 类别 |
| 社区/私有逆向 | 非公开后端、复制 client/token/身份 | 不采用 | 消费者 token 直连 | 禁止或研究控制记录 |

## 6. 当前全量上游支持方案

### 6.1 厂商直连与按量 API

| 上游 | 支持获得方式 | 执行面/协议 | 当前状态与边界 |
|---|---|---|---|
| OpenAI | 官方 Platform API Key | 直接 API；官方 OpenAI SDK；Responses/Chat | 已离线实现；Agents SDK是可选 Agent Harness，不是 ChatGPT订阅桥 |
| Anthropic | 官方 Claude Platform API Key | 直接 Messages API；官方 Anthropic SDK | 已离线实现；可选择把同一 API Key交给 Agent SDK，但语义会升级为 Agent Harness |
| Google Gemini | AI Studio API Key | Gemini API/Google Gen AI SDK | 已离线实现；与 Vertex 认证和能力分离 |
| xAI | 官方 xAI API Key | Responses；未来原生 SDK/gRPC专项 | Responses已离线实现；消费者订阅不复用本路线 |
| DeepSeek | 官方平台 API Key | OpenAI Chat或Anthropic Messages兼容入口 | 已离线实现；协议 SDK不代表 OpenAI/Anthropic提供服务 |
| Moonshot/Kimi | Kimi开放平台 PAYG Key | OpenAI Chat兼容入口 | 已离线实现；与 Kimi Code会员Key分离 |
| MiniMax | 开放平台 PAYG Key | Messages、Chat、Responses | 三协议已离线实现；媒体能力另行建模 |
| Alibaba Model Studio/Qwen | Workspace/PAYG Key | Responses、Chat；DashScope原生待专项 | 北京/新加坡路线已离线实现；资源包只是计费引用 |
| Z.AI/GLM | 开放平台 PAYG Key | OpenAI Chat兼容入口 | 已离线实现；与 GLM Coding Plan分离 |
| Xiaomi MiMo | 开放平台 PAYG Key | Messages、Chat | 已离线实现；没有公开 Responses合同 |
| Meta Llama API | Meta直连服务凭据 | 原生`/v1`或兼容`/compat/v1` | `research_only + unverified`；GA、价格、服务范围和Go执行器未闭合 |

### 6.2 OpenAI 家族组合

| Offering | 凭据/登录 | 获得支持的正式路径 | 计划判断 |
|---|---|---|---|
| OpenAI Platform PAYG | API Key | Responses/Chat直连 | 主模型路线 |
| OpenAI Platform PAYG | API Key | OpenAI Agents SDK | 可选 Agent Harness；仍消耗 API，不消耗 ChatGPT订阅 |
| ChatGPT/Codex订阅 | Codex官方管理的ChatGPT OAuth | `codex app-server` JSON-RPC | 首选官方订阅路线 |
| ChatGPT/Codex订阅 | Praxis自有、OpenAI明确批准的OAuth client | `chatgpt.com/backend-api/codex/responses` | 仅保留审批研究位；获得书面授权前`callable=false` |
| ChatGPT/Codex订阅 | 复制Codex/OpenCode client或token | 私有后端直连 | 禁止纳入正式方案 |

`codex app-server` Profile 必须记录 Codex 自带 system/developer 指令、skills、MCP、Apps、审批、沙箱和线程语义。它不是“纯 GPT Responses”的等价物。

### 6.3 Anthropic 家族组合

| Offering | 凭据/登录 | 获得支持的正式路径 | 计划判断 |
|---|---|---|---|
| Claude Platform PAYG | API Key | Messages API/官方 SDK直连 | 主模型路线，配置自由度最大 |
| Claude Platform PAYG | API Key | Claude Agent SDK | 可选 Harness；适合需要其Agent循环的场景 |
| Claude Pro/Max | 本机官方Claude登录 | Claude Agent SDK | 官方订阅 Harness路线 |
| Claude Pro/Max | 本机官方Claude登录 | `claude -p` | 官方CLI Harness备选 |
| Claude Pro/Max | 抽取/代管OAuth token | 自构造 Messages 请求 | 禁止 |

Claude Agent SDK 至少规划两个 Harness Profile：

- `claude_minimal`：自定义最小 system prompt、`settingSources: []`、`skills: []`、显式 tools/MCP、隔离 cwd、显式 session；
- `claude_code_compatible`：保留 Claude Code官方 preset/settings/skills/工具语义。

两者都必须产出 `ContextEnvelopeReport`，不得把 `claude_minimal`宣传为完全“纯净模型”；SDK不可关闭的上下文仍需实测记录。

### 6.4 Google Gemini/Antigravity 家族组合

| Offering | 凭据/登录 | 获得支持的正式路径 | 计划判断 |
|---|---|---|---|
| Gemini Developer API | AI Studio API Key | 直接 API/Google Gen AI SDK | 主模型路线 |
| Vertex AI | ADC、服务账号或官方支持的Key | Google Gen AI SDK/云API | 主云路线 |
| Google个人登录/Code Assist | 官方Gemini CLI登录 | Gemini CLI ACP或headless stream-json | 高透明度官方Harness；也是Gemini编码行为主要样本 |
| API/Vertex | API Key或ADC | Antigravity SDK | 可选 Harness研究；当前不等于消费者套餐SDK |
| Google消费者/Antigravity权益 | 官方Google Sign-In | 官方 `agy --print` | 当前正式订阅桥；只声明单次任务/最终结果 |
| Google消费者权益 | 复用第三方OAuth实现 | 自行直连模型后端 | 不采用 |

当前 Antigravity CLI 没有公开 ACP 合同，因此不能预先承诺结构化流、完整工具事件、usage或会话续接。Profile 必须保留 CLI版本、模型选择、超时、sandbox、conversation/continue能力探测结果。

Gemini CLI与Antigravity必须建立不同Harness Profile：Gemini CLI开源且有ACP、stream-json、完整system prompt替换和工具allowlist；Antigravity消费者CLI的核心与事件面更不透明，不能因为同属Google就继承Gemini CLI的可观测性。

### 6.5 xAI/Grok 家族组合

| Offering | 凭据/登录 | 获得支持的正式路径 | 计划判断 |
|---|---|---|---|
| xAI PAYG | API Key | Responses直连 | 主模型路线 |
| SuperGrok/X Premium+ | 官方Grok Build登录 | Grok Build headless/ACP，`grok agent stdio` | 首选官方订阅 Harness |
| SuperGrok/X Premium+ | xAI给Praxis注册/批准OAuth client | Praxis产品级OAuth集成 | 获批后重新设计 |
| SuperGrok/X Premium+ | 复制OpenCode/OpenClaw client ID | Bearer直连 | 禁止 |

OpenCode/OpenClaw能使用Grok的原因是它们获得了产品级官方支持和OAuth allowlist；这不能自动外推给Praxis。

### 6.6 GitHub Copilot

| Offering | 凭据/登录 | 获得支持的正式路径 | 计划判断 |
|---|---|---|---|
| Copilot个人/组织订阅 | 每用户GitHub/Copilot登录 | 官方 Copilot SDK | 首选；按用户订阅计量 |
| Copilot个人/组织订阅 | Copilot CLI登录 | Copilot CLI ACP | 强隔离/统一Agent协议备选 |
| Copilot BYOK | Provider API Key + Copilot配置 | 官方 Copilot SDK | 合法但计费来自Provider，不等于Copilot订阅模型额度 |
| Copilot订阅 | 逆向`api.githubcopilot.com`私有协议 | 自制兼容Provider | 不采用 |

### 6.7 国内订阅、Coding Plan 与 Token Plan

| 上游Offering | 凭据 | 路径 | 当前门禁 |
|---|---|---|---|
| Kimi Code会员 | 会员专属Key | OpenAI/Anthropic兼容Endpoint | `blocked_by_host_trust`；真实Praxis身份；个人、单租户、前台、交互式编程 |
| MiniMax Token Plan | Subscription Key | Messages/Chat等官方入口 | `blocked_by_host_trust`；权益与PAYG分离；不得静默切换计费 |
| MiMo Token Plan | `tp-*` | 中国/新加坡/欧洲的Chat/Messages入口 | `blocked_by_host_trust`；仅交互式编程；拒绝脚本、backend、batch、非Coding |
| Alibaba Coding Plan | `sk-sp-*` | 中国/国际Chat/Messages入口 | `blocked_by_host_trust`；仅交互式编程；拒绝自动化平台、测试器、backend、batch |
| Alibaba Team Token Plan | 团队专属Key | 中国Chat/Messages入口 | `blocked_by_host_trust`；增加成员授权快照 |
| GLM Coding Plan | 套餐凭据 | 仅官方支持工具名单 | `official_client_only + callable=false`；Praxis获批前不接入 |

流式不是 MiMo/Qwen/Alibaba 合规性的替代条件。门禁检查的是用途、交互性、用户在场、前台、自动化、批处理、租户和产品身份；MiniMax官方路线也不能被错误压成“只能流式”。

官方行为与程序化执行补充：

- 当前Kimi Code提供stream-json与ACP，公开CLI没有旧版Wire/agent-file/完整prompt override；旧Kimi CLI的Agent spec/Wire只保留为`legacy_pinned`行为样本；
- Qwen Code已有正式TypeScript Agent SDK、ACP/daemon与`--bare`，Harness本身支持脚本和CI；bare会忽略coreTools并采用固定工具集，必须拆成`bare_fixed`与`controlled_nonbare`；Coding Plan能否覆盖某种自动化仍由Offering条款决定；
- MiMo Code已开源并提供ACP/JSON执行面，但默认memory/checkpoint/dream/distill注入较强；
- MiniMax Code当前主要作为官方行为样本；可执行Route仍优先Direct API/Token Plan或厂商明确支持的第三方Harness；
- ZCode当前作为GLM第一方行为样本，未确认可嵌入协议。

### 6.8 云托管与其他第一方部署

| Deployment | 支持获得方式 | 执行面 | 当前判断 |
|---|---|---|---|
| AWS Bedrock Mantle | Bedrock API Key或SigV4 | Responses、Chat、Messages；协议SDK/官方middleware | 已离线实现；与Runtime分离 |
| AWS Bedrock Runtime | AWS身份/SigV4/Bedrock Key | AWS SDK Converse/InvokeModel | 已离线实现；能力按模型和Region核验 |
| Google Vertex Gemini | ADC/服务账号/官方Key | Google Gen AI SDK/GenerateContent | 已离线实现 |
| Google Vertex Claude | Google Cloud身份 | Anthropic SDK Google middleware/rawPredict | 已离线实现；Provider和计费方是Google Cloud |
| Google Vertex OpenAI Chat | Google Cloud身份 | OpenAI形状Chat Endpoint | 已离线实现；不推定Responses |
| Azure OpenAI v1 | API Key或Entra ID | Responses/Chat，OpenAI SDK配置Azure端点 | 已离线实现；`model`是Deployment名 |
| Azure OpenAI legacy | API Key或Entra ID | dated API Version | 已离线实现；与v1独立 |
| Azure Foundry其他模型 | Azure凭据 | 按模型/Deployment专项 | `research_only + unverified` |
| Claude Platform on AWS Marketplace | 待专项鉴权 | Anthropic运营、AWS Marketplace计费 | `research_only`；不能与Bedrock合并 |

这些都是独立的第一方云部署，不叫“反代”。模型原厂 SDK在云路线上可能只是协议客户端或官方 middleware，不能改变服务运营方、数据责任和计费方。

### 6.9 第三方托管、自托管与发现队列

| 类别 | 候选 | 计划处理 |
|---|---|---|
| 第三方推理平台 | Groq、Cerebras、Together、Fireworks、OpenRouter | 逐家建立Provider/Offering/条款/协议/测试卡；不能仅凭OpenAI兼容即宣布支持 |
| 云边缘/企业托管 | Cloudflare Workers AI、NVIDIA NIM、Hugging Face Inference | 逐Deployment调查账号、区域、协议、数据和SDK |
| 其他原厂API | Meta Llama API及后续Mistral等 | 只在官方合同与执行器闭合后进入可调用Catalog |
| 自托管 | vLLM、TGI、Ollama | 独立Self-hosted Deployment Profile；由用户承担模型授权和服务运维 |

## 7. Profile语义路由与分层合同

Profile的核心目的，是让上层只面向Praxis并集语义，同时由精确Route的Profile完成“统一语义 ↔ API/SDK/CLI/App Server/ACP原生语义”的双向转换。现有 Route 中的 `Credential Profile`继续保留，但它只是基础Profile之一，不能代表完整Profile系统。

2026-07-12详细设计修订：下列八类继续作为配置、存储和引用命名空间，不再被理解为运行时平铺合并的八个输入。语义执行时先保留`Credential + Entitlement + Deployment`组成的Route Envelope，再由`ModelBehaviorProfile × HarnessCapabilityProfile × RuntimePolicy`编译`EffectiveProfile`；最后附加Semantic Encoder/Decoder形成`SemanticRouteProfile`。详细合同见[`Intent、Mechanism、Effect与Profile路由v1`](../../design/model-invoker/intent-mechanism-effect-profile-routing-v1-draft.md)。

同一个模型经直连API、Claude Agent SDK、Codex app-server或其他Harness调用时，必须使用不同的Semantic Profile。Profile的选择键至少包含：

```text
Provider + exact Model/Revision + Deployment/Region + Protocol
+ Offering/Auth Route + Execution Surface + Harness Component Stack
+ Semantic Mode
```

不能只按“Claude”“GPT”或模型名选择Profile，因为真正需要消解的是执行面附带的上下文、工具、会话、事件和状态语义。

### 7.1 八类基础 Profile

| Profile | 负责什么 | 关键字段 | 明确不负责 |
|---|---|---|---|
| `CredentialProfile` | 凭据来源与生命周期 | `kind`、secret reference、subject、scope、refresh owner、expiry、storage owner | 套餐用途、模型参数 |
| `EntitlementProfile` | 套餐和合法用途 | offering、quota、billing fallback、user presence、foreground、interactive、coding-only、automation、batch、multitenancy | Endpoint和prompt |
| `DeploymentProfile` | 服务运营与部署 | provider、region、project/workspace/resource、endpoint、protocol binding、data locality | 用户偏好 |
| `ModelProfile` | 精确模型身份和能力 | exact model、alias policy、context、modalities、tool/reasoning/structured-output能力 | 账号和登录 |
| `HarnessProfile` | SDK/CLI/App Server/ACP行为 | surface、binary/SDK version、prompt preset、settings sources、skills、tools、MCP、cwd、session、approval、sandbox | 套餐是否合法 |
| `SemanticProfile` | Praxis并集语义与上游原生语义的双向转换 | request encoder、event decoder、capability mapping、state/tool/context translation、loss policy、extension namespace | 凭据存储和厂商授权 |
| `InvocationProfile` | 单次调用可调参数 | reasoning、temperature、max output、stream、timeout、tool choice、output constraint、provider options | 凭据与产品身份 |
| `PolicyProfile` | 证据与运行门禁 | evidence state/TTL、callable、allowed purpose、vendor approval、risk、audit requirements | 保存真实secret |

### 7.2 Semantic Route Profile

```yaml
semantic_route_profile:
  id: claude.subscription.minimal.local
  credential: credential/claude-local-login
  entitlement: entitlement/claude-pro-interactive
  deployment: deployment/anthropic-official-harness-local
  model: model/claude-exact-selected
  harness: harness/claude-agent-sdk-minimal
  semantic: semantic/claude-agent-sdk-to-praxis-union-v1
  invocation: invocation/coding-balanced
  policy: policy/consumer-single-user-local
```

组合器必须验证引用是否相容，而不是把字段简单覆盖。任何非法组合都在启动上游前拒绝。上层只持有`semantic_route_profile.id`和统一请求，不需要理解Claude SDK、Codex app-server或ACP的私有字段。

### 7.3 Harness准备与语义消解流水线

```text
Praxis UnifiedExecutionRequest
  -> Resolve SemanticRouteProfile
  -> Validate Credential/Entitlement/Policy
  -> Prepare Harness
       - 选择官方preset或minimal/custom模式
       - 注入允许覆盖的system/settings/skills/tools/MCP/cwd/session
       - 关闭允许关闭的厂商默认项
       - 启用经审核的wrapper/patch/sidecar
  -> SemanticProfile.EncodeRequest
  -> Official API/Harness Execute
  -> SemanticProfile.DecodeEvents
  -> Praxis UnifiedExecutionEvent[]
       + MappingReport
       + ContextEnvelopeReport
       + CapabilityResiduals
```

语义消解必须覆盖两个方向：

1. **入站转换**：把Praxis消息、指令、工具、输出约束、推理、状态、预算和审批意图转换为上游可表达的请求；
2. **出站转换**：把模型流、Agent事件、工具活动、审批请求、文件变更、usage、错误和终态转换为带类型的Praxis并集事件。

无法忠实映射时只有三种合法结果：显式降级并报告、进入命名空间扩展、拒绝调用。禁止静默删除语义。

### 7.4 Profile约束合并优先级

```text
厂商不可变合同
  > Route/Deployment固定约束
  > Policy与Entitlement门禁
  > 用户选择的组合Profile
  > 单次Invocation覆盖
```

低层不得覆盖高层。例如单次调用不能把`coding_only=true`改成通用任务，不能把`callable=false`改成可调用，不能用`stream=true`绕过`batch=false`。

### 7.5 上下文可控性、HarnessDelta与注入报告

每个 Harness Profile 必须输出：

```text
ContextEnvelopeReport
  - model_level_system_or_developer_input
  - harness_mandatory_instructions
  - user_selected_instructions
  - settings_sources
  - skills_and_agents
  - tools_and_mcp
  - workspace_context
  - session_history
  - provider_hidden_context: unknown | documented | observed
  - controllability: full | partial | none
  - expected_injection_manifest
  - actual_injection_manifest
  - harness_delta
  - opaque_fields
  - route_fingerprint
  - harness_stack_digests
  - sanitized_environment_digest
  - field_evidence_quality
```

并只使用以下表述：

- `direct_model_route`：Praxis控制公开请求层，但不能证明Provider内部没有系统策略；
- `minimal_harness`：已关闭所有公开可关闭的预设，但仍存在Harness不可见或不可关闭部分；
- `vendor_default_harness`：主动保留厂商Agent产品上下文；
- 禁止使用未经实测支撑的“纯净模型”“裸模型”宣传。

Profile能够消解公开、可观测、可配置的差异，但不能数学上抵消未知system prompt或Provider内部策略。因此统一语义层同时返回`CapabilityResiduals`，保留“已统一什么、损失什么、仍未知什么”。

## 8. 需要形成的统一调用原语

以下逻辑原语已经由IME与纸面编译合同在设计层锁定；物理接口、语言签名和包路径仍留给独立实现计划：

1. `ResolveSupportRoute`：从模型、Offering、部署、用途和账号解析精确路线；
2. `ComposeSemanticRouteProfile`：组合八类Profile并执行相容性验证；
3. `PrepareHarness`：依据Harness与Semantic Profile形成经审核的官方配置、wrapper、patch或sidecar准备结果；
4. `InspectContextEnvelope`：在运行前形成Expected/Actual InjectionManifest，报告HarnessDelta、不可控与opaque部分；
5. `Execute`：上层统一入口，接受`UnifiedExecutionRequest`并返回带类型的`UnifiedExecutionEvent`并集；
6. `EncodeRequest`：把并集请求转换为精确上游请求；
7. `DecodeEvents`：把模型/Harness输出转换为统一事件、映射报告和残余差异；
8. `InvokeModel`：模型级非流/流调用；
9. `StartAgentSession`：官方Harness会话；
10. `ContinueAgentSession`：续接Harness会话；
11. `ApproveAgentAction`：审批工具、文件、命令和网络动作；
12. `CancelExecution`：统一取消模型或Agent执行；
13. `ProbeCapabilities`：按版本、账号、区域和模型探测真实能力；
14. `ReportUsageAndEntitlement`：区分API token、订阅quota、premium request与未知计量；
15. `AuditExecution`：保存Route/Profile版本、策略判定、身份引用和事件摘要，不保存secret；
16. `ExplainRejection`：给出条款、能力、凭据、用途或版本不相容的精确拒绝原因。

`UnifiedExecutionEvent`是带判别字段的并集，不是把所有事件压成同一结构。模型事件与Agent事件仍是两个明确的事件族；不能把Harness内部工具调用伪造成模型原始tool call。

## 9. 粗粒度执行阶段

### 阶段 A：共同审核边界

- [x] 确认双执行平面；
- [x] 确认八类注册命名空间与三部分EffectiveProfile的映射；
- [x] 确认每家上游的允许、禁止和待审批组合；
- [x] 确认首批代表Route与实现审核顺序；
- [x] 完成六条代表Route纸面编译、事件/Effect一致性和negative golden；
- [x] 确认公共语义逻辑上位于model-invoker之上；
- [x] Official Agent Harness物理边界已由独立实施计划确认为现有Go module内的`execution/harness`与受控进程协议；本切片无需Sidecar IPC。

### 阶段 B：事实注册表与Profile合同

首个切片已经实现六条代表Route的ProfileSelectionKey、三因子Profile、Expected/Actual Manifest、RouteFingerprint、MappingReport和Residual。以下任务描述全量上游注册与生命周期目标，不表示当前没有Profile代码：

- 把本计划矩阵转换为机器可读Route/Support registry；
- 定义Profile schema、引用、继承、覆盖、版本和迁移；
- 定义每条Route的request encoder、event decoder、能力映射、损失策略和命名空间扩展；
- 定义BehaviorEvidence归因、ProfileSelectionKey、Expected/Actual InjectionManifest、HarnessDelta、RouteFingerprint与权益门禁；
- 为所有状态建立证据TTL和失效规则。

### 阶段 C：Direct Model Route收口

- 复用现有`model-invoker` 62条Catalog事实；
- 对39条默认callable、16条host-blocked、7条研究/控制记录做Profile迁移；
- 保持14个活跃Adapter与18个Factory现有边界；
- 不因统一Profile而抹平Provider方言、Endpoint、Region和协议差异。

### 阶段 D：Official Agent Harness分批落地

建议审核顺序：

1. Codex app-server；
2. Claude Agent SDK与`claude -p`；
3. Gemini CLI ACP/headless与Qwen Agent SDK；
4. 当前Kimi Code ACP/stream-json、Legacy Kimi CLI兼容卡与GitHub Copilot SDK/ACP；
5. Grok Build ACP/headless与MiMo Code ACP；
6. Antigravity `agy --print`；
7. MiniMax Code与ZCode仅先完成行为证据卡，待公开嵌入合同后再进入实现评审。

其中Codex、Claude、Gemini ACP、Qwen与current Kimi已经完成离线Adapter切片；Copilot、Grok Build、MiMo Code、Antigravity、MiniMax Code、ZCode及legacy兼容卡仍属后续范围。每个后续项仍需单独设计、版本锁定和授权，不因本总计划通过而自动获得实现授权。

### 阶段 E：云、受限订阅和发现队列

- 补齐云部署Profile和逐模型能力探测；
- 受限订阅的可信宿主激活合同已在Catalog/Route Gateway层离线实现；其真实宿主身份接线与全量Execution Profile整合仍待后续；
- 对Meta、Azure Foundry其他模型、Claude Platform on AWS和第三方托管逐项闭合；
- 自托管作为独立Deployment类别设计。

### 阶段 F：真实账号黑盒验收与生产批准

- 用户明确选择账号与产生费用的路线；
- 先沙盒/最小请求，再做流、工具、会话、取消和额度测试；
- 记录账号类型、区域、模型、版本、时间和结果；
- 生产批准还需容量、数据、条款、安全和运维审核。

## 10. 细粒度任务清单

### 10.1 支持注册表

现有Catalog RouteID和六条代表Profile已经有稳定身份；以下未勾选项专指覆盖所有研究、云、自托管与Harness路线的全量SupportRouteRegistry。

- [ ] 给每个SupportRoute稳定ID；
- [ ] 记录Provider、Offering、Deployment、ExecutionSurface、Protocol和Credential类型；
- [ ] 记录官方来源、核验时间、TTL、支持状态和Praxis落地状态；
- [ ] 记录`callable`、`host_blocked`、`research_only`、`terms_blocked`或`official_client_only`；
- [ ] 把“SDK是谁的”与“服务是谁运营的”分开；
- [ ] 禁止未知兼容Endpoint自动继承完整能力。

### 10.2 Profile系统

首个切片已经实现ModelBehaviorProfile、HarnessCapabilityProfile、RuntimePolicy、organization/user/workspace/task收紧合成、调用前组合验证、Manifest、MappingReport与CapabilityResidual。以下未勾选项专指全量Profile System管理面或尚未覆盖全部Route的能力。

- [ ] 将Profile schema修订为八类基础Profile与Semantic Route Profile；
- [ ] 定义不可变字段、允许覆盖字段和禁止覆盖字段；
- [ ] 定义Profile版本、弃用、迁移和回滚；
- [ ] 定义secret reference，确保Catalog/Profile/审计不存明文；
- [ ] 在现有organization/user/workspace/task RuntimePolicy作用域之外，补齐个人、团队、项目和Deployment的全局Profile System管理语义；
- [ ] 将已实现的调用前组合验证与可解释拒绝扩展到全量SupportRoute；
- [ ] 决定全局Profile System是否保留独立`ContextEnvelopeReport`导出；当前切片使用InjectionManifest、ContextManifestSummary与Residual；
- [x] 首个切片已定义并实现`MappingReport`与`CapabilityResidual`；扩展到全量Route仍随对应Route实施；
- [ ] 为每条Route定义入站encoder和出站decoder；
- [ ] 定义官方配置点、wrapper、窄补丁与sidecar的选择和审核规则；
- [ ] 定义 Profile 导出时的脱敏格式。

### 10.3 每个Official Harness的共同任务

- [ ] 固定官方二进制或SDK版本；
- [ ] 验证官方认证路径和token所有权；
- [ ] 枚举默认/最小可控的system prompt、settings、skills、tools、MCP和cwd；
- [ ] 枚举允许二开的官方扩展点，并决定只配置、包装、窄补丁或sidecar；
- [ ] 验证会话创建、续接、并发、取消、超时和进程回收；
- [ ] 验证事件、审批、文件变更、命令、错误和最终结果；
- [ ] 验证模型选择与账号权益；
- [ ] 验证usage/quota是否真实可得，未知时明确未知；
- [ ] 验证单用户隔离和工作区隔离；
- [ ] 验证升级不兼容与能力降级；
- [ ] 禁止读取、复制或复用非Praxis OAuth client和refresh token。

### 10.4 受限订阅共同任务

- [ ] 从可信宿主接收真实ClientIdentity；
- [ ] 固定个人/团队主体和成员授权；
- [ ] 验证交互、前台、用户在场、Coding用途和单租户；
- [ ] 在Provider触达前拒绝backend、SaaS、批处理、自动脚本和非允许用途；
- [ ] 额度耗尽时停止，不静默切PAYG、账号、Region或模型；
- [ ] 记录条款版本和支持工具名单变化。

## 11. 测试与验收计划

### 11.1 单元与白盒

- Profile schema、组合、优先级和非法覆盖；
- SemanticProfile入站/出站映射、显式降级、扩展与拒绝；
- Route解析无歧义；
- secret永不进入日志、Profile导出和审计；
- 条款/用途门禁在网络调用前生效；
- 模型事件和Agent事件不会混型；
- ContextEnvelopeReport完整且稳定。
- MappingReport与CapabilityResiduals能解释所有非等价语义。

### 11.2 黑盒

- 每个Adapter/Harness用fake server、fake process或录制契约覆盖成功、拒绝、超时、取消、流中断和版本不兼容；
- CLI验证stdin/stdout/stderr、退出码、信号和孤儿进程；
- SDK/App Server验证会话、审批、工具、工作区和并发隔离；
- ACP验证握手、能力协商、事件序列和恢复。

### 11.3 集成

- `ResolveSupportRoute -> ComposeSemanticRouteProfile -> PrepareHarness -> PolicyGate -> Encode -> Execute -> Decode -> Audit`完整链路；
- Direct Model Route与Official Harness分别跑通，不共享错误的状态或工具循环；
- 同一模型家族在直连、云部署和Harness之间不发生凭据/计费/能力串线；
- 订阅与PAYG额度耗尽时没有隐式fallback。

### 11.4 真实账号验收

- 只有用户明确授权后执行；
- 每条路线只用最小计费请求；
- 必须记录真实版本、账号Offering、Region、模型和时间；
- “登录成功”不等于模型可用，“返回文本”不等于所有能力可用；
- 未完成真实烟测的路线继续标记`implemented_offline`，不能标记`live_verified`。

## 12. 风险与回退

| 风险 | 防护 | 回退 |
|---|---|---|
| 厂商条款或支持名单变化 | 证据TTL、实现前刷新、失效触发器 | 自动降为`research_only/non_callable` |
| Harness偷偷增加上下文 | 固定版本、ContextEnvelope快照与回归 | 切回上一已验证版本或直连API |
| Profile组合导致越权 | 不可变厂商合同与策略高优先级 | 拒绝组合，不做自动降级 |
| 订阅额度被用于后台/多租户 | 可信宿主与用途门禁 | 禁用该Offering，要求PAYG/企业合同 |
| CLI/SDK协议升级破坏兼容 | 版本锁定、能力探测、契约测试 | 保留旧版本Profile并停止新版本 |
| 云路线被误当模型原厂直连 | Provider/Deployment/Credential分离 | 拒绝模糊Route，要求精确选择 |
| secret泄漏 | 仅存引用、最小权限、脱敏审计 | 撤销凭据并阻断Route |

## 13. 本计划明确不做

- 不把所有上游压成一个OpenAI兼容Provider；
- 不把Agent SDK、CLI、App Server或ACP伪装为纯模型API；
- 不复制OpenCode、OpenClaw、Codex或其他产品的OAuth client ID、token、User-Agent或产品身份；
- 不通过反向代理绕过地区、套餐、用途、账号或产品限制；
- 不把“能登录”“能返回文本”“能流式”当成完整支持；
- 不在本计划审核前创建Official Harness新模块；
- 不在用户授权前执行真实账号、真实Key或付费烟测；
- 不对未核验的第三方兼容Endpoint做自动发现和自动接入。

## 14. 已闭合设计决定

详细决定表见[代表Route纸面编译与跨Route一致性合同D01-D36](../../design/model-invoker/representative-route-paper-compilation-and-union-conformance-v1.md#15-v1设计决定闭合)。本计划层已经固定：

1. 八类Profile是注册命名空间；运行时以RouteEnvelope、ModelBehaviorProfile、HarnessCapabilityProfile、RuntimePolicy和SemanticCodec合成；
2. Harness只允许覆盖官方合同声明为overridable的字段；身份、auth、opaque vendor项和安全owner锁定；
3. 上下文提供`semantic_stable/vendor_default/custom_explicit`；
4. 对上层共享`Execute`语义，同时保留模型级与Agent级明确低层原语；
5. 公共语义逻辑边界已确定，物理模块名与IPC延期到独立实现计划；
6. 审核顺序更新为Codex → Claude → Gemini/Qwen → current Kimi/Copilot → Grok/MiMo → Antigravity；
7. OpenAI自有OAuth直连继续`callable=false`，直到获得书面批准；
8. 官方专属Key/Endpoint仍在Direct Route受可信宿主门禁；只有存在独立官方Harness时才另建Harness Route；
9. Profile作用域按organization/user/workspace/task约束合并，allow取交集、deny取并集、预算取更严；
10. 上下文最小化必须同时有静态配置、preflight Manifest和契约probe；live verified还需真实黑盒；
11. Harness二开逐Route选择最低足够等级，长期fork不作为默认；
12. v1请求、事件、Effect、取消与终态合同已由IME和纸面编译资产固定。

## 15. 设计阶段完成与实现门槛

### 15.1 本计划的设计阶段已完成

- [x] 双执行平面已确认；
- [x] Profile语义路由目的、分层、作用域和合成规则已确认；
- [x] 主要上游正式支持路径、禁止路线和审批研究路线已形成事实卡；
- [x] 首批Harness审核顺序已确认；
- [x] 并集类型体系、五类能力原语、统一事件与结果已收口；
- [x] OpenAI Direct、Codex、Claude、Gemini、current Kimi和Qwen六条Route纸面编译已完成；
- [x] cross-route conformance、Manifest漂移、取消、终态与negative golden已定义；
- [x] design、plan、索引与memory已纳入本轮同步范围。

### 15.2 后续实施状态与仍需授权事项

- [x] 物理边界确认为现有`ExecutionRuntime/model-invoker`内的Go包与受控进程协议；
- [x] 已编写并完成首个执行并集独立落地计划；
- [x] 已创建`union/profile/effect/execution`与Direct/Harness Adapter；本切片不需要Sidecar；
- [ ] 使用真实账号、OAuth、API Key或订阅额度；
- [ ] 执行live Manifest probe、真实黑盒与生产评审。

本设计总计划本身未自动授权实现；首个切片后来已获得独立授权并完成。剩余live验证仍需逐Route单独授权。
