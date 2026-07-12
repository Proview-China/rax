# Model Invoker Route Gateway信任闭合修正设计

## 1. 状态与结论

- 状态：已完成；独立审核发现的5项语义缺口均已修正并通过全量离线回归；
- 范围：`ExecutionRuntime/model-invoker/`及对应`.properties.rax`资产；
- 保留分层：根包`RouteInvoker`继续拥有Policy/Authorization/Audit，`routegateway.Gateway`继续拥有Binding/Secret/Factory/Pool；
- 明确排除：Runtime Kernel、Model Profile、Context Engine、缓存策略和生产宿主实现。

默认Catalog不能把调用方可自行构造的`InvocationContext`、`ClientIdentity.Source`或`EntitlementState`当成可信证明。16条已实现订阅Route必须撤回默认callable，保留Adapter代码和精确Route事实，并标记`blocked_by_host_trust`。只有宿主注入可信订阅授权Resolver、使用经审核激活的Catalog后，订阅调用才允许进入Binding/Secret/Factory。

## 2. 可信订阅授权边界

1. 普通`RouteCall.Invocation`和`RouteCall.EntitlementState`只可用于非订阅Route；订阅Route出现调用方自带claim时直接拒绝。
2. `SubscriptionAuthorizationResolver`由Gateway宿主在构造时注入；输入只含Catalog锚定的Route/Offering/Credential身份，不接收调用方自证claim。
3. Resolver输出完整Invocation与Entitlement快照；现有`Authorize`仍负责结构、绑定、新鲜度、额度、用途和禁止PAYG检查。
4. 没有Resolver时，含可调用订阅Route的Gateway构造失败；默认Catalog中的16条订阅Route本身不可调用。
5. fake build-manifest、fake runtime-observed和fake active entitlement都必须在Binding/Secret/Factory/Provider前零触达；测试专用可信Resolver输出才可通过激活Catalog。

## 3. Secret前模型事实

- 所有受限订阅Route改为`static_catalog + exact_provider_id`，逐Route保存精确模型别名；
- `RouteInvoker`在授权Resolver、Binding和Secret前执行字符级精确校验；
- Provider方言继续用同一Catalog allowlist做纵深防御，不再拥有跨Offering的Alibaba全局集合。

Alibaba Coding Plan依据2026-07-11官方exact-string allowlist，只允许：`qwen3.7-plus`、`qwen3.6-plus`、`kimi-k2.5`、`glm-5`、`MiniMax-M2.5`、`qwen3.5-plus`、`qwen3-max-2026-01-23`、`qwen3-coder-next`、`qwen3-coder-plus`、`glm-4.7`。`GLM-5.1`、`glm-5.1`及其他未列模型明确拒绝。

Alibaba Token Plan Team使用独立官方清单，只纳入文档列出的文本模型；不得与Coding Plan或其他Region共享代码中的隐式并集。

## 4. Pool Close错误合同

- 轮换淘汰的空闲Adapter Close失败进入pool生命周期错误汇聚，不再丢弃；
- `Gateway.Close`返回此前轮换错误与当前关闭错误的安全聚合；
- stale Lease在最后释放时的Close错误由当前Resolve/Invoke/Stream Close返回；
- 所有Close原始错误只保留在unwrap链，公开`Error()`固定使用不含秘密的`adapter_close_failed`消息。

## 5. Factory Endpoint入池门禁

Factory结果必须在`adapterPool.acquire`成功发布前完成Provider身份、Closer、Endpoint非空和Endpoint信任验证。验证覆盖scheme/host、base path、userinfo/query/fragment、点段、反斜线和百分号编码路径逃逸。失败结果必须立即安全Close且不进入池；相同key后续只能重新构造，不能取得污染Adapter。

## 6. 预期默认事实

- Catalog总记录仍为62条；默认callable由55降为39；
- 16条订阅Route为`implemented_offline + callable=false + blocked_by_host_trust`；
- 默认语义矩阵恢复为39 Route × 20能力 = 780行、6协议、14个活跃Adapter；
- Provider缓存事实恢复为39条默认callable Route；
- 内建Factory Registry仍保留18个工厂，其中4个订阅工厂只供经审核激活的Catalog使用。

## 7. 完成证据

- 默认Catalog为62条记录：39条按量/云Route可调用，16条已实现订阅Route为`implemented_offline + callable=false + trusted_subscription_authorization_resolver`，另有7条研究/控制记录；
- 可信Resolver是订阅claim唯一升级路径；调用方自带build-manifest、runtime-observed或active entitlement均在Resolver、Binding、Secret、Factory、Provider前拒绝；
- 所有受限订阅Route使用逐Route `static_catalog + exact_provider_id`，模型字符级预检早于Resolver与Secret；Alibaba Coding Plan与Token Plan Team使用两个独立官方精确集合；
- 轮换Close错误、stale Lease释放错误、Gateway关闭错误均走不泄密错误合同；并发Gateway关闭会等待已登记的轮换Close，不遗漏此前错误；
- Factory的Provider身份、生命周期Closer、Endpoint非空与Endpoint信任均在入池前验证；失败结果不入池，相同key后续必须重新构造；
- 机器资产已重生为39×20=780行语义矩阵和39条Provider缓存事实；统一离线脚本、相关fuzz与`-coverpkg=./...`覆盖率实测均通过。
