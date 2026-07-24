# Praxis 项目产物索引

## 当前总体状态

Praxis 已建立 `.properties.rax` 的设计 → 计划 → 实现 → 验收 → 同步流程。`model-invoker`前两阶段、第三阶段A-E1、上游调用最终候选A→F、信任闭合、宿主激活再验证，以及Profile/Intent-Mechanism-Effect执行并集Runtime和第二轮合同Review均已完成离线实现、审查、验收与同步；Codex Pro官方CLI与App Server单Route已完成两种真实验证，另有一个用户授权Claude兼容上游完成Messages/Chat Completions基础协议探测；后者尚未建立独立生产Route，其余Route仍需逐项授权。

2026-07-14 Runtime首次、二次及第三次live文件复审发现的问题均已修正；独立线程最终确认仓库Runtime设计资产无可定位P0/P1，6张图、107条唯一反例、34条追溯和Plan保持一致且实现中立。随后用户授权启动组件中立Runtime基座：公共对象、三维状态、Fence/Effect、命令、Activation Journal与原子Commit、Timeline/Checkpoint/Restore、组件Registry、Foundation完整闭环及故障注入均已落地并通过普通/Race/Vet验证。其上的Harness公共合同、单Run Interaction Loop、Action Gateway和Runtime ExecutionPort真实接线也已完成同级验证；生产后端、具体Harness、真实模型/Tool/MCP与其他组件内部实现仍未授权。

## 模块目录

| 模块 | 阶段 | 设计 | 计划 | 模块说明 | 实现 |
|---|---|---|---|---|---|
| `model-invoker` | Factory A/B信任闭合完成；39默认callable Route、16条host-blocked Route与18工厂已有门禁；执行并集Runtime及第二轮P0/P1合同Review、五路生产Adapter离线集成均已验收；Codex Pro单Route已真实验证 | [设计入口](../design/model-invoker/README.md) | [已完成Review v2](../plan/model-invoker/execution-semantic-union-review-hardening-v2.md) | [模块说明](../module/model-invoker/README.md) | [Go module](../../ExecutionRuntime/model-invoker/README.md) |
| `runtime` | 公共合同、Activation容灾、装配门禁与最小完整闭环已通过普通/Race/Vet验证；相邻组件保持Port-only | [设计入口](../design/runtime/README.md) | [Runtime V1 Plan](../plan/runtime/runtime-v1.md) | [模块说明](../module/runtime/README.md) | [Go module](../../ExecutionRuntime/runtime/README.md) |
| `harness` | 公共合同、单Run Kernel、Action Gateway与Runtime ExecutionPort完整闭环已通过普通/Race/Vet验证；具体生产Harness未实现 | [设计入口](../design/harness/README.md) | [Harness V1 Plan](../plan/harness/harness-v1.md) | [模块说明](../module/harness/README.md) | [Go module](../../ExecutionRuntime/harness/README.md) |

## 总体架构与设计域

- [Praxis总体架构索引](./architecture/README.md)
- [定义与装配设计域](./architecture/assembly/README.md)：`agent-definition`、`profile-system`、`agent-assembler`、`harness`；
- [能力依赖设计域](./architecture/capabilities/README.md)：`context-engine`、`tool-engine`、`mcp-gateway`、`memory-engine`、`knowledge-engine`、`asset-manager`；
- [核心执行设计域](./architecture/execution/README.md)：`runtime`与`sandbox`；
- [治理与控制设计域](./architecture/governance/README.md)：`organization-engine`、`review-engine`、`management-engine`与Runtime Control Plane。

除`model-invoker`、Runtime基座和Harness公共最小骨架外，上述新目录当前均为设计入口或设计草案。Runtime/Harness尚未获得生产后端、具体外壳、真实外部集成或相邻组件内部实现授权。

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
- `model-invoker`内的`execution.Runtime`只拥有单次模型/Agent调用的事件、审批、取消、Effect与终态，不替代仍在设计中的全局`runtime`模块；
- 执行并集五顶层原语、三因子Profile、Direct bridge、Codex App Server、Claude、Gemini ACP、current Kimi ACP、Qwen Adapter已完成离线验收；Harness不冒充干净API，真实Effect只由Praxis观察器产生；
- 当前验收使用 fake、`httptest`/TLS server、固定协议样本、官方 SDK，以及 `upstream`/`catalog`/`catalogassets`测试；没有执行真实 API、真实套餐或认证成功模型调用，不能据此声明生产可用；
- Gemini Developer API与 Vertex AI已用不同 Adapter/Credential/Endpoint分离；Prompt Cache创建策略仍未实现；
- 第三阶段波次 A已经分离七维 Route身份；B已抽取四协议并锁定安全边界；C已落地订阅控制面；D已落地四个云 Adapter、两个 Bedrock协议和21条 callable云 Binding；E1已完成 DeepSeek两协议、Kimi/Z.AI按量 Chat、MiniMax三协议、MiMo两协议、Qwen北京/新加坡四条 Binding及 xAI Responses；
- GLM Coding Plan保持`official_client_only + research_only + callable=false`；MiMo与Alibaba订阅Route为`implemented_offline + callable=false + blocked_by_host_trust`，后端与非交互用途仍硬拒绝；
- Kimi Code与MiniMax Token Plan已离线实现，但默认同样受宿主信任门禁阻塞，不作为生产应用后端；
- AWS Bedrock、Vertex AI和 Azure已完成离线实现但未做真实账号核验；其他第三方托管、多模态和 Agent编排仍需独立设计与批准。

详细构建规则见 `.properties.rax/MAIN.md`。已落地模块从`module/<模块名>/README.md`进入；尚处设计阶段的模块从`design/<模块名>/README.md`进入。
