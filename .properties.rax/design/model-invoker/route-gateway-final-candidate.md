# Model Invoker 上游调用最终候选设计

## 1. 状态与边界

- 状态：候选设计已按A→F完成离线实施与复核，等待相邻Runtime宿主联合审核
- 实现范围：`ExecutionRuntime/model-invoker/`同一 Go module
- 组合包：`routegateway/`，允许导入根包、`catalog/upstream`与现有18个 Runtime Adapter工厂
- 保留层：根包 `RouteInvoker`继续作为 Route Policy/Authorization/Audit层
- 明确排除：Model Profile、缓存策略、Context Engine、Runtime Kernel、工具执行器、旁路、沙箱、调度、组织与编排

最终候选不再把“Route能解析到 AdapterID”当作“Route可执行”。完整链路必须由 Catalog Route驱动非秘密绑定、秘密引用解析、正确 Adapter工厂、并发安全生命周期、Route级能力查询和实际 Invoke/Stream。

## 2. 分层职责

```text
routegateway.Gateway
├── Policy/Audit preflight (根包 RouteInvoker等价规则，无Secret/Factory触达)
├── RuntimeBindingResolver (只解析非秘密运行绑定)
├── SecretResolver (只接收CredentialReference)
├── AdapterFactoryRegistry (18 AdapterID明确工厂)
├── AdapterPool (Route/credential-version/binding-version/evidence生命周期)
└── modelinvoker.Invoker (Capabilities/Invoke/Stream语义执行)
```

`RouteInvoker`不是 Gateway的下游实例执行器，因为它构造时需要Registry；Gateway复用其固定的Policy规则或抽取共享预检，但不得为了复用而提前解析Secret/Factory。Policy失败必须发生在所有运行绑定、秘密和Provider触达之前。

## 3. 公共候选接口

### 3.1 SecretResolver

只接收 Route已登记的 `CredentialReference`、Credential类型与Route身份；返回短生命周期秘密材料和不含秘密字节的 `Version`轮换标识。普通测试只能注入 fake resolver。错误不得包含引用对应的值、环境值、header、token或可逆摘要。

### 3.2 RuntimeBindingResolver

输入完整 Catalog Entry，输出类型化非秘密绑定：deployment/project/workspace/resource/region/endpoint及 `Version`。Resolver不能改变 Provider、Offering、Protocol、Endpoint ID或Credential Profile；跨Route、跨Region、跨Offering注入必须拒绝。

### 3.3 AdapterFactoryRegistry

Factory输入只能来自 Catalog Entry、已验证 RuntimeBinding与 SecretMaterial。18个现有 Runtime AdapterID必须各有明确工厂注册；工厂返回 `modelinvoker.Provider`和可选 `io.Closer`。调用方不得传 ProviderID、Protocol或Endpoint覆盖。

### 3.4 AdapterPool与Lease

复用键只包含 Route identity digest、evidence digest、Credential `Version`、Binding `Version`和Factory ID/version，禁止秘密字节、秘密摘要、原生Credential对象或请求内容。并发首次构造使用单飞；Lease防止正在使用的Adapter被提前Close。轮换、binding变化、evidence变化或显式失效后，新调用不得取得陈旧Adapter；旧Lease释放后Close。Close失败可观测但不得泄密。

### 3.5 Gateway

至少提供 `Resolve`、`Capabilities`、`Invoke`、`Stream`和`Close`。`Resolve`返回Route审计与实例准备事实，但不暴露秘密或Provider内部对象。`Capabilities`必须基于该Route实际构造的Adapter、协议、Endpoint和模型执行，不能只返回Catalog静态表。

## 4. 固定调用顺序

```text
Route存在/selector为空/callable/status/evidence/model/policy/entitlement
  -> RuntimeBindingResolver
  -> 绑定与Route七维身份复核
  -> SecretResolver
  -> Secret材料与Credential Profile复核
  -> AdapterPool acquire(singleflight)
  -> AdapterFactory
  -> 实际Provider Capabilities或Invoke/Stream
  -> Lease release / deferred close
```

任何 non-callable、evidence、policy、entitlement或model失败时，BindingResolver、SecretResolver、Factory和Provider调用计数必须全部为0。

## 5. 内建工厂离线合同

每个 callable Catalog Entry必须能找到对应Factory。工厂合同测试必须用 fake SecretResolver和显式本机HTTP transport或纯构造模式，禁止真实环境Key、ADC、Entra、AWS默认链或公网。测试要证明工厂参数来自Route：Provider/Offering/Deployment/Protocol/Endpoint/Credential/Profile/Region/Project/Workspace/Resource不能漂移。

## 6. 语义、订阅与缓存交接

- 统一语义只能标记 `v1candidate`，C阶段生成机器可检查矩阵并逐项判断扩展风险；
- D阶段产出的16条订阅Adapter保留离线实现；信任闭合审核已将默认Catalog撤回为`callable=false + blocked_by_host_trust`，只有可信宿主激活后才可调用；
- E阶段只交接Provider缓存事实，不生成key、不实现策略；
- Candidate输出分别列出稳定候选、待联合审核、明确延期、需用户决定与未运行真实验证。

## 7. 验收

- 外部包黑盒、包内白盒、安全、并发、race、fuzz和资产门禁；
- 39条默认callable Route存在真实工厂构造路径；另有16条订阅Route保留4个真实工厂，但默认受宿主信任门禁阻塞；
- Secret轮换、并发创建、Factory/Close失败、typed-nil、取消、超时和流关闭有反例；
- 每阶段独立memory；最终统一离线入口与相关fuzz/覆盖率实测。
