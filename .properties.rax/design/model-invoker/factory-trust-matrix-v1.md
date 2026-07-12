# Model Invoker Factory双层信任矩阵设计

## 1. 目标与边界

- 对象：`ExecutionRuntime/model-invoker`现有18个Builtin Factory、14个默认活跃Adapter、4个受限订阅Factory；不新增业务模块。
- A层：公开`provider/* Config -> Adapter`直用边界，独立评价Endpoint、Credential audience、请求身份和上游实际响应Model。
- B层：`Catalog -> BuiltinFactory -> Gateway`生产路径，独立评价Catalog模板、Binding、Credential audience、FactoryResult Endpoint回读、Gateway身份覆盖、Factory生命周期。
- 状态只能是`pass`、`gap`、`not_applicable`；B层通过不能覆盖A层缺口。
- 机器矩阵必须从live `catalog.DefaultDocument()`、`NewBuiltinFactoryRegistry()`及Builtin合同元数据生成，并由测试核对18/14/4全集、证据文件和零未解释gap。

## 2. Endpoint与Credential audience

公开Config统一遵守：

1. 生产Endpoint仅允许官方精确Host，或由已校验Region/Resource推导出的精确云Host；禁止任意远端HTTPS。
2. 生产Endpoint禁止userinfo、query、fragment、非标准端口、尾点Host、DNS后缀欺骗、路径遍历、编码路径和Catalog基路径逃逸。
3. 测试例外只允许精确`localhost`或`net.IP.IsLoopback()`覆盖的IPv4/IPv6 loopback；`.localhost`、域名解析到loopback及任意远端Host均不是例外。
4. 云Provider不硬钉单一全球Host：AWS按Region推导，Vertex按Location推导，Azure只允许单DNS label的`{resource}.openai.azure.com`。B层还必须由Catalog Endpoint模板、Credential Profile audience/allowed IDs、Binding anchor和FactoryResult回读闭合。
5. 订阅Factory除精确Host外还按Profile+Protocol锁定Catalog基路径；调用方不能用兼容协议把套餐Key送往同Host其他路径。

## 3. 响应Model与身份

- OpenAI Chat/Responses、Anthropic Messages、Z.AI Chat以及已有xAI/Qwen/MiniMax/MiMo/DeepSeek/Kimi路径，若上游返回actual Model，A层非流和流必须与exact请求Model逐字一致；不一致返回Mapping错误，不能静默成功。
- Gemini GenerateContent、Bedrock native等未返回可验证actual Model的协议，`Status=not_applicable`、`VerificationMode=indirect`，以请求URL/部署绑定为身份证据；不得把`request.Model`投影误写成“已验证上游Model”。`indirect`不是第四种Status。
- Azure部署名与底层模型名不是同一身份；A层以部署名请求绑定为主，actual底层Model不与部署名做伪exact比较。
- B层Gateway始终覆盖`Provider/Protocol/Endpoint`为Catalog选择结果，并在协议存在actual Model字段时保留Provider层校验和Gateway二次校验。

## 4. Factory生命周期

- `Build`必须在取消时及时返回；Gateway Close先封闭新acquire，再等待所有首次及轮换中的in-flight Build。
- Build失败、Gateway已关闭或结果迟到时，任何非nil FactoryResult Closer都必须关闭；Closer错误必须脱敏并由当前调用或最终Close可观察。
- 同Route credential/binding/factory/client identity变化触发轮换；旧Lease归零后只关闭一次。并发Close幂等，不允许WaitGroup Add/Wait竞态。
- Factory返回的Provider ID、Closer和Endpoint为必填合同；Endpoint须回读并通过同scheme/host及Catalog基路径内校验后才能入池。

## 5. 机器资产

- `internal/trustmatrix`保存versioned candidate合同元数据，不新增稳定公开SDK能力；其全集、作用域、Route计数和Factory ID必须由live Registry/Catalog核对。
- `cmd/factorytrustgen`只从上述live对象生成CSV/Markdown；检查入库资产漂移。
- 任何新增Factory、默认活跃Adapter、订阅Adapter、Endpoint策略或响应Model语义若未同步合同和测试，资产门禁失败。
- 每个`pass`字段必须声明enforcement mode并绑定可执行门禁；测试逐18行检查字段与共享/专用门禁的对应关系。只有证据路径存在但没有执行断言，不足以标`pass`。
- 每个`Status=not_applicable, VerificationMode=indirect`字段必须有非空理由及协议代码证据，解释为何没有actual Model或为何部署身份不能与底层模型名做伪exact比较。

## 6. 验收

- Endpoint helper黑盒表驱动和fuzz/属性测试覆盖任意HTTPS、userinfo/query/fragment、端口、尾点、后缀欺骗、IPv4/IPv6/localhost和路径逃逸。
- OpenAI、Anthropic、Z.AI补actual Model非流/流反例；无Model协议只验证`not_applicable`理由与请求身份绑定。
- 18 Factory逐项构造、39默认callable Route与16个受限订阅Route继续由live Catalog回归；4订阅Factory不新增真实自动smoke。
- 生命周期定向普通/race，全仓普通/shuffle/race，integration仅编译与无网络guard，相关fuzz、覆盖率、catalogassets和`git diff --check`全部通过。

## 7. 落地结果

- 已生成`praxis.model-invoker.factory-trust-matrix/v1candidate`：CSV严格为header+18个Factory数据行，覆盖14个默认活跃Adapter、4个host-blocked订阅Factory、39条默认callable Route和16条订阅Route；每行记录live `FactoryVersion`。
- A/B合同按protocol/profile展开；exact与indirect不聚合。Markdown先完整输出表格，再集中输出indirect理由。
- 代码证据门禁使用Go AST精确验证完整`path#symbol`，覆盖顶层func/var/const/type与接收者方法；测试证据只允许`tests/**/*_test.go`中的可执行`func Test*(t *testing.T)`，并按VerificationMode限制允许的语义断言。
- 10个direct Adapter逐行、逐protocol固定绑定公开Config动态Endpoint反例；Credential audience、请求身份、Gateway stamp、singleflight、credential/binding/client identity轮换、Lease归零、并发Close、取消和晚到Build均绑定机器断言。
- 本矩阵覆盖的18个builtin Factory是固定`Version=v1candidate`的值对象；Factory Registry拒绝替换已注册AdapterID，因此不支持Factory实例热替换。Gateway每次`prepare`都会读取`Factory.Version()`并纳入pool key，自定义可变Version能够触发缓存Adapter轮换，但本矩阵不把它表述为已验证的可变Factory热替换。
- 统一离线入口、全仓普通/shuffle/race、integration compile/guard、catalogassets、30项fuzz和diff均通过；合并语句覆盖率实测79.4%，只记录现状，不设百分比门禁。
