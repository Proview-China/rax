# Praxis 项目产物索引

## 当前总体状态

Praxis 已建立 `.properties.rax` 的设计 → 计划 → 实现 → 验收 → 同步流程。`model-invoker`前两阶段、第三阶段A-E1、上游调用最终候选A→F、信任闭合及宿主激活再验证均已完成离线实现、两棒审查、验收与同步；后续只剩相邻模块联合审核、用户决策项和逐Route单独授权的真实验证。

## 模块目录

| 模块 | 阶段 | 设计 | 计划 | 模块说明 | 实现 |
|---|---|---|---|---|---|
| `model-invoker` | Factory A/B双层信任矩阵与gap闭合完成；39默认callable Route、16条host-blocked订阅Route、18工厂及780行语义矩阵均有机器门禁 | [设计入口](../design/model-invoker/README.md) | [Factory信任闭合计划](../plan/model-invoker/factory-trust-matrix-v1.md) | [模块说明](../module/model-invoker/README.md) | [Go module](../../ExecutionRuntime/model-invoker/README.md) |

## 重要边界

- 当前实现 OpenAI Responses/Chat Completions、Anthropic Messages、Gemini GenerateContent、AWS Bedrock Mantle/Runtime、Vertex Gemini/Claude/Chat和 Azure OpenAI v1/legacy；
- 当前Catalog登记62条记录：39条默认callable Binding对应14个活跃Runtime Adapter；16条订阅Route保留4个实现工厂但默认host-blocked；另有7条研究/控制记录；
- 七维 Route、Credential秘密引用与绑定、版本化 Catalog Schema、证据 TTL/状态、Markdown生成块、统一离线脚本和 CI门禁已经落地；
- `praxis.model-invoker.semantic/v1candidate`与`praxis.model-invoker.route-policy/v1candidate`仍是候选；Policy层只负责选择与授权，完整构造和执行由Gateway层承担；
- `praxis.model-invoker.route-gateway/v1candidate`保留18个真实内建工厂，并为39条默认callable Route提供具体协议Endpoint、并发复用与Route级Capabilities；秘密值不进入Pool key或审计；
- `praxis.model-invoker.factory-trust-matrix/v1candidate`逐18个Factory记录live Version及A/B层protocol/profile合同；代码证据由Go AST精确校验，测试证据由verification mode白名单和10家direct Adapter逐行门禁约束；
- Kimi Code、MiniMax/MiMo Token Plan、Alibaba Coding/Token Plan共16条Route默认不可调用；只有可信宿主激活Catalog并注入授权Resolver后，才可进入个人、单租户、前台、非生产、有效Entitlement与真实客户端身份门禁；GLM Coding Plan保持official-client-only；
- 自动真实套餐smoke只保留官方范围允许第三方交互式编码工具的Kimi Code与MiniMax Token；MiMo和Alibaba明确禁止脚本/API测试器，因此只做离线Gateway验证；
- non-callable订阅、过期 evidence、错误 static model及缺失/失效 entitlement均在 Provider前拒绝，自动 PAYG固定禁止；
- Provider缓存只拥有传输合同，不拥有 key、存储、TTL、命中、淘汰或跨 Provider策略；
- 当前只有 Go 内核，没有 TypeScript Sidecar 或 Rust；
- 当前验收使用 fake、`httptest`/TLS server、固定协议样本、官方 SDK，以及 `upstream`/`catalog`/`catalogassets`测试；没有执行真实 API、真实套餐或认证成功模型调用，不能据此声明生产可用；
- Gemini Developer API与 Vertex AI已用不同 Adapter/Credential/Endpoint分离；Prompt Cache创建策略仍未实现；
- 第三阶段波次 A已经分离七维 Route身份；B已抽取四协议并锁定安全边界；C已落地订阅控制面；D已落地四个云 Adapter、两个 Bedrock协议和21条 callable云 Binding；E1已完成 DeepSeek两协议、Kimi/Z.AI按量 Chat、MiniMax三协议、MiMo两协议、Qwen北京/新加坡四条 Binding及 xAI Responses；
- GLM Coding Plan保持`official_client_only + research_only + callable=false`；MiMo与Alibaba订阅Route为`implemented_offline + callable=false + blocked_by_host_trust`，后端与非交互用途仍硬拒绝；
- Kimi Code与MiniMax Token Plan已离线实现，但默认同样受宿主信任门禁阻塞，不作为生产应用后端；
- AWS Bedrock、Vertex AI和 Azure已完成离线实现但未做真实账号核验；其他第三方托管、多模态和 Agent编排仍需独立设计与批准。

详细构建规则见 `.properties.rax/MAIN.md`，模块细节从 `module/<模块名>/README.md` 进入。
