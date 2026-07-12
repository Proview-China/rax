# Model Invoker Route Policy/Audit Invoker v1候选

## 1. 状态与授权边界

- 契约 ID：`praxis.model-invoker.route-policy/v1candidate`
- Runtime 常量：`modelinvoker.RoutePolicyCandidateVersion`
- 状态：阶段性 Policy/Audit层已实现；不是完整 Route Gateway
- 已完成目标：让 `RouteID`对预构造 Registry/Invoker执行策略、授权、选择器绑定与审计
- 本层明确不拥有：Credential秘密解析、Provider构造、实例生命周期；这些由同 module的 `routegateway`组合层补齐

本层不新增第二套请求语义。它把七维 `UpstreamRoute`、Catalog evidence和 Offering策略绑定到统一语义原语候选，并要求调用方已经提供包含正确 Provider实例的 Registry/Invoker。

## 2. 公共契约

```text
RouteCall
├── RouteID
├── InvocationContext (仅非订阅Route接受调用方claim)
├── EntitlementState (仅非订阅Route接受调用方claim)
└── Request (Provider/Protocol/Endpoint必须为空)

RouteInvoker
├── SubscriptionAuthorizationResolver (订阅Route的宿主可信claim边界)
├── Resolve(RouteCall) -> RouteSelection
├── Invoke(context, RouteCall) -> RouteResponse
└── Stream(context, RouteCall) -> RoutedStream
```

`RouteSelection`固定保留 RouteID、七维身份投影、evidence digest、Runtime AdapterID、Protocol、解析后 Endpoint、Model和 PolicyDecision。`RouteError`在所有失败上保留 RouteID；路由已选定后同时保留完整 `RouteSelection`，并通过 `Unwrap`保留统一 `Error`。

## 3. Policy层选择所有权

| 字段 | 所有者 | v1规则 |
|---|---|---|
| `RouteID` | 调用方显式选择 | 不做自动候选搜索或回退 |
| Runtime `Provider` | Catalog `Implementation.AdapterID` | 调用方非空即 `route_selector_owned` |
| `Protocol` | Catalog `Route.Protocol.ID` | 只接受六个已有协议的精确映射 |
| `Endpoint` | Catalog `Endpoint.ResolveBaseURL(Deployment)` | 调用方非空即拒绝；不能把 Key或任意 map带入模板 |
| `Model` | 调用方语义请求 + Catalog discovery约束 | runtime-selected路线交给 Provider精确校验；static-catalog路线只能使用已登记 ref/alias |
| Credential | 预配置 Provider Adapter | Policy层只验证 Route/Credential身份资产，不读取秘密、不构造 Provider；不能据此宣称Route可执行构造 |

Policy层只接受已注册 Adapter。Catalog Provider身份与 Runtime AdapterID继续分离；例如云托管或协议方身份不能被错误压成模型原厂 Provider。完整 Gateway必须另外证明Factory构造参数与这些身份一致。

## 4. 调用前固定顺序

```text
RouteID存在
  -> caller selectors为空
  -> implementation callable且至少 implemented_offline
  -> evidence在本次调用时仍 fresh
  -> protocol与endpoint可精确绑定
  -> static model约束通过
  -> 订阅Route拒绝调用方claim并从可信Resolver取得Invocation/Entitlement
  -> Offering.Authorize(InvocationContext, EntitlementState, now)
  -> 明确确认 no automatic PAYG switch
  -> Runtime Adapter已注册
  -> 基础 Invoker能力检查与调用
```

`Resolve`执行到“Adapter已注册”为止，不调用 `Provider.Capabilities/Invoke/Stream`，因此可用于纯离线预检。

## 5. 订阅允许/禁止边界

1. 默认Catalog中的16条已实现订阅Route固定`callable=false + blocked_by_host_trust`，不触达 Provider；它们不是“无实现”的普通控制记录。
2. 只有经审核激活为`callable=true`且宿主注入可信`SubscriptionAuthorizationResolver`时，订阅Route才可能进入授权；Gateway构造会拒绝缺Resolver的活动订阅Route。
3. 订阅Route一旦携带调用方自造`InvocationContext`或`EntitlementState`即直接拒绝；Resolver输入只含Catalog锚定的Route/Offering/Credential身份。
4. Resolver输出的Invocation必须满足subject、tenancy、foreground、production和真实client identity限制；Entitlement必须绑定Offering/Credential、未过期、未陈旧且额度可用。
5. missing/stale/expired/suspended/quota-exhausted状态在 Provider能力检查前拒绝。
6. 任何拒绝都保持 `AllowsAutomaticPAYGSwitch=false`；门面不选择另一 Route、Key、账号或按量余额。
7. `official_client_only`和条款阻塞记录永远不可由本门面变成可调用路线。

## 6. 稳定错误码

| code | 含义 |
|---|---|
| `route_id_required` / `route_not_found` | RouteID缺失或不在活动 Catalog |
| `route_selector_owned` | 调用方试图注入 Provider/Protocol/Endpoint |
| `route_not_callable` | 研究、计划、条款阻塞或其他控制记录 |
| `route_evidence_unavailable` | 调用时 evidence非 fresh、未开始或已过期 |
| `route_protocol_unsupported` / `route_endpoint_invalid` | Catalog不能映射到 v1 Runtime选择 |
| `route_model_rejected` | Model缺失或超出 static catalog |
| `upstream.PolicyReasonCode` | Offering/entitlement的具体拒绝原因 |
| 基础 `Error.Code` | Adapter注册、能力、映射、Provider或流生命周期失败 |

## 7. 流与审计

- `RoutedStream.Route()`返回防御性副本；
- 事件仍使用冻结的 `StreamEvent`，不复制一套 Route事件类型；
- terminal `Err`与 `Close`错误包装为 `RouteError`，底层统一错误仍可 `errors.As`；
- `RouteSelection`和 Provider `Response.MappingReport`并列保存：前者解释选了哪条 Catalog Route，后者解释语义如何映射到 Provider。

## 8. 已验收范围与未完成项

- 允许的按量 Route能绑定 Adapter/Protocol/Endpoint且不修改调用方 Request；
- static-catalog错误模型、调用方选择器、过期 evidence均在 Provider前拒绝；
- 默认host-blocked订阅、伪造claim、缺Resolver、失效entitlement与额度耗尽均为零下游调用；
- 可信Resolver和激活Catalog fixture可以调用且不能自动 PAYG；
- Route流保留选择、顺序、终态与关闭语义；
- 全仓离线入口通过，且没有真实 Provider调用；
- Policy层本身仍不拥有SecretResolver、RuntimeBindingResolver、Factory或生命周期；这些已由同module的`routegateway`实现。Policy层不得单独称完整执行门面。
