# 全上游统一原语层 Code Review（2026-07-14）

## 1. 结论

状态：`review_closed_offline`。

本轮Review前，主LLM链已经具备`Catalog -> Factory -> RouteGateway -> Invoker`闭环，但外围与本地链只能证明“实现存在”，不能机器证明“所有已登记上游都经过公共原语边界”。本轮已补齐统一Surface矩阵、Realtime调用器、外围流生命周期和结果身份校验。

“全部上游”在此严格指：

- 默认Catalog中39条`callable` LLM Route；
- 6个代表性Direct/Harness Profile；
- 14个外围HTTP Surface；
- Gemini resumable upload专用Surface；
- 3个官方Realtime Surface及1个显式本地Realtime Surface；
- 3类本地/企业自建OpenAI-compatible产品；
- 4种显式第三方Relay协议。

研究记录、`official_client_only`、条款阻塞、未激活订阅和未声明的自建能力仍然Fail Closed，不计作可调用。

## 2. Review发现与修正

| 级别 | 发现 | 风险 | 修正 |
|---|---|---|---|
| P1 | Realtime只有`Provider.Open`，没有Registry/Invoker | 业务可直接依赖具体WebSocket实现，绕过统一校验 | 新增`realtime.Registry/Invoker`及Session事件投影 |
| P1 | 外围、Realtime、本地、Relay与Harness没有共同机器清单 | 无法证明新增上游是否进入公共原语 | 新增`semanticmatrix.UnionMatrix`和精确Canonical Row校验 |
| P1 | Operation流的started、sequence、completed和MappingReport由Provider自觉提供 | 不同Provider观测语义不稳定 | `operation.Invoker`统一拥有流开始、重编号、单一终态、自动关闭与MappingReport |
| P1 | LLM非流式、Operation结果和本地LLM结果可返回错误Model/Provider/Kind | 错误路由可穿透公共语义层 | RouteGateway继续拥有LLM最终身份；基础Invoker拒绝非空Model漂移；Operation拒绝Provider/Kind/Model漂移；Localcompat同步与流式均精确校验Model |
| P1 | Operation Artifact只检查非空Kind，URL过期语义未由Invoker兜底 | 方言数据可能伪装成稳定Artifact | 固化Artifact枚举、MIME/标识校验，URL必须有`ExpiresAt`或`ExpiryUnknown` |
| P1 | Operation/Realtime请求和Operation结果仍可能与调用方或Provider共享map/slice/pointer | Provider可越过原语边界修改上层状态，异步修改也会污染已返回结果 | Invoker对请求、结果和流事件做递归防御性复制，并以恶意Provider测试锁定隔离合同 |
| P2 | Operation和Realtime上游测试维护第二份手工清单 | 新增Surface时容易漏测 | `specs.Definitions()`成为唯一Canonical Registry，矩阵与测试共同消费 |

## 3. 最终原语边界

| 上游类型 | 公共调用边界 | 稳定语义所有者 |
|---|---|---|
| 默认Direct/Cloud/Subscription LLM Route | `routegateway.Gateway` | Route、Credential、Endpoint、Model和最终审计身份 |
| Direct/Harness Agent执行 | `profile.Compiler + execution.Runtime` | Intent/Mechanism/Effect、Profile、观测和验证 |
| Embedding/Media/Speech/Files/Stores/Batch/Video | `operation.Invoker` | Capability、Mapping、Result、Artifact、Job/Resource和流终态 |
| File/Store便利API | `resource.Client -> operation.Invoker` | Resource动作到Operation Kind的封闭映射 |
| Batch/Video便利API | `job.Client -> operation.Invoker` | Job状态、结果流与Operation Kind映射 |
| OpenAI Realtime/Gemini Live/xAI Voice/本地WS | `realtime.Invoker` | Provider选择、Request校验、事件序列、错误与Session关闭 |
| Ollama/llama.cpp/企业自建兼容LLM | `modelinvoker.Invoker -> localcompat` | 显式能力、Endpoint、Model allowlist和响应Model证明 |
| 第三方Relay | `routegateway.Gateway -> NewRelayCompatFactory` | 显式Route、协议、端点、Model和Credential绑定 |

这里没有引入巨型万能Request。统一的是选择、能力来源、映射、生命周期、结果身份和审计；不同平面继续保留适合自身生命周期的强类型请求。

## 4. 机器覆盖

`semanticmatrix.BuildUnion`当前生成：

- LLM矩阵：780行，即39条Callable Route × 20项LLM Capability；
- Execution：64行，来自6个Representative Profile的Intent/Mechanism；
- Operation：128行，覆盖34个封闭Operation Kind和全部Canonical HTTP Surface；
- Realtime：4行；
- 本地LLM：6行，即3类产品 × 2种协议；
- Relay：4行，即Chat Completions、Responses、Messages、GenerateContent。

`UnionMatrix.Validate`会按当前Canonical Registry重建完整期望行；缺行、增行、字段漂移、重复、空Plane、遗漏Operation Kind或xAI inference/management混淆均拒绝。

## 5. 保留边界

1. 本轮是离线Code Review和协议黑盒，没有消费真实API或订阅额度。
2. 动态自建实例仍要求宿主显式提供Endpoint、Model和Capability allowlist；家族已支持不等于任意服务器自动获得能力。
3. Provider单元测试可以直接调用Adapter以验证方言；系统/集成调用必须经过对应Invoker、Gateway或Execution Runtime。
4. 安全存储、身份验证器和账号管理不在本轮范围，继续使用引用和临时注入边界。
