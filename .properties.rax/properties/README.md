# Praxis 项目产物索引

## 当前总体状态

Praxis 已建立 `.properties.rax` 的设计 → 计划 → 实现 → 验收 → 同步流程。`model-invoker` 已完成前两阶段离线实现，以及第三阶段波次 A上游基础、B协议层、C订阅控制面、D云托管 Provider和 E1全部直连路线；F未触发，G/H按当前授权延期，第三阶段总计划已完成当前授权范围并转为陈旧计划。

## 模块目录

| 模块 | 阶段 | 设计 | 计划 | 模块说明 | 实现 |
|---|---|---|---|---|---|
| `model-invoker` | 第三阶段当前授权范围完成并转为陈旧计划：A-E1离线验收完成，F未触发，第三方首批名单与真实 API联调明确延期 | [设计入口](../design/model-invoker/README.md) | [第三阶段执行计划](../plan/model-invoker/phase-3-upstream-ecosystem.md) | [模块说明](../module/model-invoker/README.md) | [Go module](../../ExecutionRuntime/model-invoker/README.md) |

## 重要边界

- 当前实现 OpenAI Responses/Chat Completions、Anthropic Messages、Gemini GenerateContent、AWS Bedrock Mantle/Runtime、Vertex Gemini/Claude/Chat和 Azure OpenAI v1/legacy；
- 当前 Catalog登记62条记录：39条 callable Binding对应十四个 Runtime Adapter，23条控制记录保持无 Adapter且不可调用；
- 七维 Route、Credential秘密引用与绑定、版本化 Catalog Schema、证据 TTL/状态、Markdown生成块、统一离线脚本和 CI门禁已经落地；
- 当前只有 Go 内核，没有 TypeScript Sidecar 或 Rust；
- 当前验收使用 fake、`httptest`/TLS server、固定协议样本、官方 SDK，以及 `upstream`/`catalog`/`catalogassets`测试；没有执行真实 API、真实套餐或认证成功模型调用，不能据此声明生产可用；
- Gemini Developer API与 Vertex AI已用不同 Adapter/Credential/Endpoint分离；Prompt Cache创建策略仍未实现；
- 第三阶段波次 A已经分离七维 Route身份；B已抽取四协议并锁定安全边界；C已落地订阅控制面；D已落地四个云 Adapter、两个 Bedrock协议和21条 callable云 Binding；E1已完成 DeepSeek两协议、Kimi/Z.AI按量 Chat、MiniMax三协议、MiMo两协议、Qwen北京/新加坡四条 Binding及 xAI Responses；
- GLM Coding Plan、MiMo Token Plan与 Alibaba Coding/Token Plan按官方允许范围标为 `interactive_coding_only + planned + callable=false`，后端与非交互用途仍硬拒绝；
- Kimi Code与 MiniMax Token Plan只计划按官方允许的个人交互式 Agent/Coding场景支持，不作为生产应用后端；
- AWS Bedrock、Vertex AI和 Azure已完成离线实现但未做真实账号核验；其他第三方托管、多模态和 Agent编排仍需独立设计与批准。

详细构建规则见 `.properties.rax/MAIN.md`，模块细节从 `module/<模块名>/README.md` 进入。
