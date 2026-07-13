# Model Invoker上游调用与统一封装候选审核清单

> 2026-07-11独立审核修正已完成：原55条默认callable结论已撤回；订阅可信claim、Secret前模型校验、Alibaba白名单、Close错误和Endpoint入池5项语义偏移均按[信任闭合修正设计](./route-gateway-trust-closure.md)修正并通过全量离线回归。

> 状态演进：本清单最初冻结2026-07-11 Route Gateway候选边界；2026-07-13后续独立计划已完成`union/profile/effect/execution`、Direct与首批Harness Adapter的离线实现。以下分类已据此区分“model-invoker内Semantic Route Profile v1”和仍属未来的全局Profile System。

## 1. 已完成

- Policy/Authorization/Audit层：根包`RouteInvoker`固定RouteID、selector、evidence、model、Offering、entitlement和审计边界；失败时不触达Provider。
- Route Gateway组合层：`routegateway.Gateway`拥有非秘密Runtime Binding、类型化Secret解析、18个明确工厂、并发单飞池/Lease、轮换失效和Resolve/Capabilities/Invoke/Stream/Close。
- 实际Route：Catalog共62条记录，其中39条按量/云Route为`implemented_offline + callable`；16条订阅Route保留已实现Adapter但默认`callable=false + blocked_by_host_trust`；另有7条研究/控制记录。
- 订阅调用面：Kimi Code、MiniMax/MiMo Token Plan、Alibaba Coding/Token Plan只有在宿主注入可信授权Resolver并使用经审核激活的Catalog后，才可按个人、单租户、前台、非生产、真实客户端身份和新鲜Entitlement执行；调用方自证claim无效，不自动回退PAYG。
- 统一语义事实：39 Route × 20能力 = 780行、6协议、14个默认活跃Adapter的机器矩阵与运行态漂移门禁已完成；Registry仍保留18个工厂。
- Provider缓存事实：39条默认callable Route逐条记录控制面、key/TTL/State、read/write usage、错误和Evidence TTL；只有xAI Responses暴露严格Provider命名空间的`prompt_cache_key`。
- 最终离线验收：gofmt、tidy diff、module verify、vet、普通、shuffle、全仓race、integration仅编译、相关fuzz和生成资产门禁全部通过；宿主激活、Endpoint/响应模型身份闭合及两项P1修正后`-coverpkg=./...`合并语句覆盖率重新实测为78.0%。
- 执行语义并集：`model-invoker`内Semantic Route Profile v1、Intent/Mechanism/Effect、Direct与Codex/Claude/Gemini/current Kimi/Qwen Harness Adapter已完成离线实现与验收；真实账号与官方二进制联调仍未运行。

## 2. 候选待联合审核

- `praxis.model-invoker.semantic-matrix/v1candidate`：公共结构目前不需要破坏性变化；ContentBlock、结构化工具结果、Tool分型和新Stream事件必须与Profile、Context和Runtime消费者联合审核后加法演进。
- `praxis.model-invoker.route-gateway/v1candidate`：ActivationPlan、HostConfig/NewHost与Gateway生命周期离线实现完整，但上层Runtime宿主的真实BindingResolver、SecretResolver、原子替换和生产生命周期接线仍需相邻模块审核。
- `CacheIntent`接缝：只允许从上层意图在Route绑定后编译为Provider严格选项；key作用域、TTL、隔离、失效、计费和Route选择尚未联合决定。

## 3. 明确延期

- 全局`profile-system`的持久化、管理、装配与跨模块集成，以及Context Engine、Runtime Kernel、缓存策略、工具执行器、旁路、沙箱、调度、组织和编排；
- 多模态、Hosted/Computer/MCP工具、Batch、Realtime、后台执行；
- xAI gRPC、Qwen DashScope原生、未设计的Sidecar和第三方托管路线；
- 生产容量、性能基准、成本优化和SLA。

## 4. 需用户决定

- Meta Llama应落在哪个精确官方/云/第三方Route、条款和Credential主体；
- 第三方托管首批名单及是否进入新的设计计划；
- 未来每条真实烟测使用哪个账号、Route、模型、预算与单次授权；
- 是否启动全局Profile System、Cache、Context与Runtime联合审核，以及各模块所有权。

## 5. 未运行真实验证

- 没有读取、创建或复用真实Key、登录态、ADC、Entra、AWS默认链或订阅凭据；
- 没有执行认证成功模型调用、真实套餐调用、付费调用、余额/限额消耗或公网Provider烟测；
- 没有真实账号模型可用性、Region容量、条款主体、生产可靠性或性能结论；
- 自动真实套餐smoke仅保留Kimi Code与MiniMax Token；MiMo/Alibaba条款禁止脚本/API测试器，因此只保留离线验证；
- `implemented_offline`只证明离线构造、协议、安全、授权和生命周期合同，不等于`live_verified`或生产可用。
