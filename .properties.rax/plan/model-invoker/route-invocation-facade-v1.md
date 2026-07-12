# Model Invoker Route Policy/Audit阶段计划

## 1. 计划状态

- 模块：`model-invoker`
- 计划版本：`v1`
- 创建时间：2026-07-11 13:29 CST
- 当前状态：阶段性完成并保留为陈旧计划；不代表完整 Route Gateway或最终语义冻结
- 设计依据：
  - `design/model-invoker/semantic-primitives-v1.md`
  - `design/model-invoker/route-invocation-facade-v1.md`
  - `design/model-invoker/provider-cache-transport-boundary-v1.md`
- 实现位置：`ExecutionRuntime/model-invoker/route_invoker.go`
- 测试位置：`ExecutionRuntime/model-invoker/tests/routefacade/`

本计划记录已完成的 Route Policy/Authorization/Audit层。它依赖预构造 Registry/Invoker，不包含 SecretResolver、RuntimeBindingResolver、AdapterFactory、实例生命周期或真实Route构造路径；这些缺口由后续最终候选计划承接。原“冻结统一语义”和“完整门面”的表述已纠正。

## 2. 完成后会产出什么

1. 一个版本化统一语义原语合同，明确 Request/Response/Stream/Error/State/Usage/Raw的稳定含义；
2. 一个以 `RouteID`为唯一上游选择输入的公共 `RouteInvoker`；
3. `Resolve`纯离线预检、`Invoke`非流调用与 `Stream`流调用；
4. 每次成功或失败均可关联 RouteID、七维身份、evidence digest、Adapter、Protocol、Endpoint和 PolicyDecision；
5. caller selector注入、non-callable控制记录、过期 evidence、错误 static model、缺失/失效 entitlement与自动 PAYG的网络前拒绝；
6. 一份明确“传输而非策略”的 Provider缓存边界资产；
7. 覆盖全部39条当前 callable Route的解析漂移门禁，以及允许/禁止订阅正反例；
8. 更新后的设计索引、计划索引、模块说明、实现说明、项目索引和 memory快照。

## 3. 范围与不做事项

### 3.1 本计划范围

- `SemanticPrimitivesVersion`与 `RouteFacadeVersion`；
- `RouteCall`、`RouteSelection`、`RouteResponse`、`RouteError`；
- `RouteInvoker.Resolve/Invoke/Stream`与 `RoutedStream`；
- Catalog callable/status/evidence、协议映射、Endpoint解析、static model和 Adapter注册门禁；
- `InvocationContext + EntitlementState`授权；
- 禁止自动 PAYG与控制记录零 Provider调用；
- Provider缓存字段的传输所有权说明；
- 无公网、无真实 Key的黑白盒、race与全量离线测试。

### 3.2 本计划不授权

- Runtime Kernel、Context Engine或 Model Profile；
- 自动 Route候选选择、评分、负载均衡、故障转移或成本优化；
- Credential秘密解析、Provider Adapter自动构造或账号发现；
- 缓存 key生成、存储、TTL、预热、命中、淘汰或跨 Provider复用；
- 新 Provider、新协议、新模型能力或多模态扩展；
- 消费者登录态、真实订阅、真实 API Key、付费调用和生产批准；
- E2、Sidecar、第三方托管、自托管或 Agent编排。

## 4. 实施清单

### A. 现场审计与资产修复

- [x] 核对 git、现有第三阶段计划、Catalog/Route、基础 Invoker和订阅控制面；
- [x] 运行现有 core/upstream/catalog/catalogassets及全仓普通测试，确认旧基线通过；
- [x] 确认现有缺口是 Catalog与基础 Invoker没有可执行 RouteID组合面；
- [x] 识别白皮书与临时目录为并发外部改动，不回滚、不纳入本模块修改；
- [x] 更新全部 model-invoker状态资产，消除“当前无授权实施”与本计划的漂移。

### B. 设计与冻结

- [x] 冻结 `praxis.model-invoker.semantic/v1`；
- [x] 冻结 `praxis.model-invoker.route-facade/v1`；
- [x] 固定 RouteID对 Provider/Protocol/Endpoint的唯一所有权；
- [x] 固定订阅允许/禁止、entitlement和 no automatic PAYG顺序；
- [x] 固定 Provider缓存只传输、不做策略的边界；
- [x] 明确排除 Runtime Kernel、Context Engine、Model Profile和缓存策略。

### C. 实现

- [x] 实现构造、时钟注入和未初始化反例；
- [x] 实现 `Resolve`零 Provider调用预检；
- [x] 实现按量 `Invoke`与 RouteResponse审计；
- [x] 实现 `RoutedStream`选择、顺序、终态和关闭；
- [x] 实现 selector、callable/status、evidence、protocol、endpoint、model和 Adapter门禁；
- [x] 实现 Offering/entitlement授权和禁止 PAYG；
- [x] 保持统一 Request/Response/Stream/Error不变，不复制第二套语义。

### D. 测试

- [x] 按量 Route绑定且调用方 Request不被修改；
- [x] caller selector与 static model错误为零 Provider调用；
- [x] non-callable订阅控制记录为零 Provider调用；
- [x] callable订阅 fixture覆盖缺 state、有效 state和 quota exhausted；
- [x] evidence在 Catalog构造后过期仍能按调用时钟拒绝；
- [x] Route流保留选择和基础生命周期；
- [x] 当前39条 callable Route全部可经 `Resolve`映射且不触达 Provider；
- [x] 三份版本化设计资产存在、被索引并含关键边界；
- [x] `go vet`、普通、shuffle、race与统一离线入口全部通过。

### E. 同步与收口

- [x] 更新 `ExecutionRuntime/model-invoker/README.md`；
- [x] 更新 `.properties.rax/design/plan/module/properties`索引与状态；
- [x] 写入 `.properties.rax/memory/model-invoker/`完成快照；
- [x] 复核 git diff只包含本轮 model-invoker资产，保留用户并发白皮书与根级忽略文件改动；
- [x] 把本计划标记为完成后保留的陈旧计划。

## 5. 验收标准

1. 所有调用均以显式 RouteID进入，没有自动候选选择或隐式回退；
2. Provider/Protocol/Endpoint只能由活动 Catalog绑定；
3. non-callable、证据过期、策略拒绝和 entitlement失败均在任何 Provider方法之前返回；
4. 有效订阅调用仍保持 `AllowsAutomaticPAYGSwitch=false`；
5. 统一语义原语未被 Route审计字段污染，Provider SDK类型继续不越界；
6. 缓存资产不宣称实现任何缓存策略；
7. 全量离线验证通过且没有读取真实 Key、联网 Provider或付费调用；
8. `.properties.rax`与代码 live state一致，不把离线实现写成生产支持。

## 6. 最终离线验收

2026-07-11 13:35 CST实际执行并通过：

- `go vet ./...`；
- `go test -count=1 ./...`；
- `go test -race -count=1 ./tests/routefacade`；
- `./scripts/verify-offline.sh`。

统一入口实际退出 0，覆盖 module verify、gofmt、tidy diff、git diff check、vet、普通、shuffle、全仓 race、integration仅编译和 Catalog资产门禁。没有读取真实 Provider Key，没有执行公网 Provider、订阅或付费调用。

完成证据记录在[上游调用与统一封装 v1完成快照](../../memory/model-invoker/20260711-133510-上游调用与统一封装v1完成.md)。
