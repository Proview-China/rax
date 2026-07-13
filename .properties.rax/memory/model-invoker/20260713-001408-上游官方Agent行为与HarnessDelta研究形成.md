# 上游官方Agent行为与HarnessDelta研究形成

## 时间

- 2026-07-13 00:14 CST

## 当前阶段变化

在既有Intent、Mechanism、Effect与Profile路由草案之上，已完成主要上游官方Agent、CLI、SDK、App Server和官方评测脚手架的行为研究，并形成独立设计资产：

- `design/model-invoker/upstream-official-agent-behavior-and-harness-delta-research-20260713.md`

本轮只研究和修改设计/计划资产，没有新增Runtime代码、没有登录账号、没有读取真实凭据、没有执行真实付费调用。

## 形成的关键结论

1. 不再使用“纯净/不纯净”二元判断。Direct API定义为`request_controlled`，官方Agent定义为`harness_composed`。
2. 官方Agent是高价值行为样本，但其行为由模型、提示词、工具Schema、上下文发现、权限、memory、compaction和运行策略共同产生，不能全部归因给模型。
3. 每个官方样本必须同时产出`ModelBehaviorCandidate`与`HarnessCapabilityProfile + HarnessDelta`。
4. Profile选择键扩展为Provider、model/revision、Deployment、Protocol、Offering、Auth Route、Execution Surface、Harness和Harness Version。
5. Official Harness调用前需要Expected/Actual `InjectionManifest`、preflight比较与`RouteFingerprint`；不可见部分必须标记`opaque`。
6. 上下文模式形成`semantic_stable`、`vendor_default`、`custom_explicit`三档。
7. 稳定并集统一Intent和Verified Effect，不强制统一`apply_patch/Edit/replace/Write`等Mechanism方言。

## 上游研究更新

- Codex：app-server支持覆盖base/developer instructions、关闭项目文档发现和完整typed events，但仍是Agent Harness。
- Claude：订阅最小路线可通过Agent SDK配置空system prompt、`setting_sources=[]`、精确tools与strict MCP；CLI `--bare`不会读取OAuth，不能作为订阅桥。
- Gemini：新增Gemini CLI官方开源样本；支持完整system prompt替换、工具allowlist、headless stream-json与ACP，但memory/GEMINI.md仍需隔离和报告。Antigravity保持独立Harness Profile。
- Grok Build：ACP、结构流和`inspect --json`可用于预检；默认兼容资产发现需要隔离。
- Kimi：官方Agent spec、Wire和ACP提供高可控Harness；默认偏好StrReplace与分块Write。
- MiniMax：当前官方已转向M3与新版MiniMax Code；MiniMax Code基于OpenCode/Pi并使用Producer+Verifier，但当前主要作为行为参考。
- MiMo：MiMo Code已经开源，提供ACP/JSON和模型专属工具投影；默认memory/checkpoint/dream/distill注入较强。
- Qwen：官方TypeScript Agent SDK、ACP/daemon、`--bare`、完整system prompt覆盖和精确coreTools已确认；Harness能力不能再概括为“不能自动化”。
- GLM：ZCode是第一方高编排行为样本；当前未确认可供Praxis嵌入的稳定CLI/SDK/ACP合同，Coding Plan仍受官方支持工具名单限制。
- Copilot：SDK `empty`模式、system section控制、精确工具与丰富事件适合建立稳定Harness Profile。

## 已同步资产

- IME草案加入BehaviorEvidence归因、ProfileSelectionKey、HarnessDelta、InjectionManifest、上下文模式和preflight编译步骤；
- 全量上游计划加入Gemini CLI、Qwen SDK、Kimi/MiMo协议面与新版Official Harness评审顺序；
- 订阅Harness研究纠正“Gemini CLI已整体迁移到Antigravity”的旧归纳，拆分Gemini CLI登录/Code Assist与Antigravity消费者权益；
- design与plan索引已更新。

## 下一步

与用户共同审核新增的六项决定，再为GPT/Codex、Claude SDK、Gemini CLI、Kimi CLI、Qwen SDK和Direct API分别绘制纸面编译示例。未获单独实现授权前，不进入Runtime编码。
