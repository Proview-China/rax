# Catalog Evidence 官方复核刷新完成

- 时间：2026-07-30 18:45 CST
- Owner：Model Invoker
- 基线：`origin/main@a2bfa0fee3de6ad6aba26102eb792eb5ce31d1a5`
- 范围：`catalog.DefaultDocument`的62条记录；不修改Builder、Host、Turn Continuation或公共合同
- 结论：39条默认callable Binding保持`fresh`；16条host-blocked路线中10条保持`fresh`，MiMo Token Plan六条降为`stale`并保持NO-GO；7条研究/控制记录继续遵守既有`unverified`、`terms_blocked`或非callable边界

## Provider复核清单

| Provider/Offering | 主要官方证据 | 复核结论 | 漂移与状态 |
|---|---|---|---|
| OpenAI API | <https://developers.openai.com/api/docs/models>、<https://developers.openai.com/api/reference/resources/responses/methods/create> | `gpt-5.5`及Responses/Chat当前合同可继续证明 | direct路线`fresh` |
| Anthropic API | <https://platform.claude.com/docs/en/about-claude/models/overview>、<https://platform.claude.com/docs/en/api/overview> | `claude-fable-5`和Messages合同可继续证明 | direct路线`fresh` |
| Google Gemini Developer | <https://ai.google.dev/gemini-api/docs/models>、<https://ai.google.dev/api/generate-content> | `gemini-3.5-flash`和GenerateContent当前可继续证明 | direct路线`fresh` |
| xAI API | <https://docs.x.ai/developers/grok-4-5>、<https://docs.x.ai/developers/rest-api-reference/inference/chat> | `grok-4.5`、Responses、函数工具与reasoning当前可继续证明 | direct路线`fresh`；未因品牌页面变化改写Route identity |
| Alibaba Model Studio | <https://www.alibabacloud.com/help/en/model-studio/qwen-api-reference>、<https://help.aliyun.com/en/model-studio/coding-plan>、<https://help.aliyun.com/en/model-studio/token-plan-overview> | 按量、Coding Plan与Token Plan当前页面仍可复核 | direct路线`fresh`；六条受限计划路线`fresh + blocked_by_host_trust` |
| Xiaomi MiMo PAYG | <https://mimo.mi.com/docs/quick-start/summary/model>、<https://mimo.mi.com/docs/api/chat/openai-api>、<https://mimo.mi.com/docs/api/chat/anthropic-api> | PAYG模型、端点与两种协议可继续证明 | 旧`platform.xiaomimimo.com/static/docs`链接已404，迁移到`mimo.mi.com`；两条direct路线`fresh` |
| Xiaomi MiMo Token Plan | <https://mimo.mi.com/docs/tokenplan/subscription>、<https://mimo.mi.com/docs/tokenplan/quick-access> | 当前页面可证明计划、Key与端点，但不能继续证明旧版“仅编程工具、禁止backend”的完整精确约束 | 六条路线`stale + blocked_by_host_trust + NO-GO`；Activation必须返回evidence unavailable |
| MiniMax | <https://platform.minimax.io/docs/api-reference/api-overview>、<https://platform.minimax.io/docs/token-plan/intro> | PAYG与Token Plan当前合同可继续证明 | direct路线`fresh`；两条Token Plan路线`fresh + blocked_by_host_trust` |
| Z.AI | <https://docs.z.ai/api-reference/introduction>、<https://docs.z.ai/api-reference/llm/chat-completion>、<https://docs.z.ai/devpack/usage-policy.md> | `glm-5.2` PAYG可继续证明；Coding Plan仍未授权Praxis身份 | PAYG `fresh`；Coding Plan继续`unverified + official_client_only + NO-GO` |
| Kimi Platform / Kimi Code | <https://platform.kimi.com/docs/models>、<https://www.kimi.com/code/docs/en/kimi-code/models.html> | K2.7/K2.6与`kimi-for-coding`、Coding endpoint当前可继续证明 | 旧third-party URL已404并迁移；direct `fresh`，两条Code路线`fresh + blocked_by_host_trust` |
| DeepSeek | <https://api-docs.deepseek.com/>、<https://api-docs.deepseek.com/guides/anthropic_api/> | V4 Chat/Messages路线可继续证明 | 旧`deepseek-chat/reasoner`已下线但不在当前Catalog；两条direct路线`fresh` |
| AWS Bedrock | <https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-openai.html>、<https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_Converse.html> | Mantle和Runtime两套边界仍可继续证明 | cloud路线`fresh` |
| Google Vertex AI | <https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/use-claude> | Vertex Gemini、Claude与OpenAI兼容入口现有边界可继续证明 | cloud路线`fresh`；自部署/控制记录仍非callable |
| Azure OpenAI / AI Foundry | <https://learn.microsoft.com/en-us/azure/ai-foundry/openai/api-version-lifecycle>、<https://learn.microsoft.com/en-us/azure/ai-foundry/openai/how-to/switching-endpoints> | v1与legacy端点边界可继续证明 | Azure OpenAI路线`fresh`；其他Foundry模型保持`unverified + NO-GO` |
| Anthropic Platform on AWS / consumer plans | <https://platform.claude.com/docs/en/build-with-claude/claude-platform-on-aws>、<https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan> | 旧AWS partner URL已404并迁移；仍不能推出可调用Model Invoker路线 | 继续research/control、non-callable、NO-GO |

## SDK与摘要边界

- 仓库实际编译版本保持：`openai-go v3.41.1`、`anthropic-sdk-go v1.56.0`、`google.golang.org/genai v1.63.0`、`aws-sdk-go-v2/service/bedrockruntime v1.55.0`。
- 复核时观察到上游已有更新版本，但本轮是证据刷新，不升级依赖、不改SDK identity。
- `Evidence.Digest`由Catalog在构造时根据新的Source、CheckedAt、ValidUntil和Status重新计算；没有通过修改Route Protocol/APIVersion伪装证据更新。
- 未使用真实Provider Credential、付费账号或在线推理做live smoke；Production状态仍为既有`requires_review`/`prohibited`，不得因文档复核自升权。
