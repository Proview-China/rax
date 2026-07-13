# 第三方中转站兼容模块说明 v1

## 作用

该模块让第三方API中转站在不冒充OpenAI、Anthropic、Gemini官方直连的前提下，复用Praxis协议Driver并输出统一`Response/FunctionCall/Usage/Error`。

## 组成

| 位置 | 作用 |
|---|---|
| `provider/relaycompat` | 独立Provider、Endpoint/模型门禁、通用方言和保守能力合同 |
| `internal/compatprovider` | 新增GenerateContent组合能力，与Chat/Responses/Messages共享生命周期和脱敏 |
| `routegateway.NewRelayCompatFactory` | 用户显式注册的Route Factory；默认Builtin Registry不包含它 |
| `tests/relaycompat` | 四协议HTTP黑盒、文本、Tool Call及负向合同 |
| `tests/routegateway/relaycompat_test.go` | Factory opt-in、四协议构造和能力白盒 |
| `tests/integration/relaycompat_smoke_test.go` | 环境变量驱动的真实8 Route经济性探针 |

## 调用规则

1. Catalog的`Implementation.AdapterID`使用`third-party-relay`；
2. Runtime显式注册`routegateway.NewRelayCompatFactory()`；
3. Route Endpoint保存协议前缀而不是完整操作URL；
4. Credential只保存Secret Store引用；
5. Provider、Protocol、Endpoint、Model必须与Route一致；
6. 中转URL不能传给官方`provider/openai`、`provider/anthropic`或`provider/gemini`。

## 首轮真实结果

| 模型 | 协议 | 文本 | Tool Call | 结论 |
|---|---|---:|---:|---|
| Gemini 3.5 Flash | Chat Completions | 通过 | 通过 | 兼容 |
| Gemini 3.5 Flash | GenerateContent | 429 | 未执行 | 请求/认证/错误归一化通过，中转容量阻塞 |
| Grok 4.5 | Chat Completions | 通过 | 通过 | 兼容 |
| Grok 4.5 | Messages | 通过 | 通过 | 兼容 |
| GPT 5.6 Luna | Chat Completions | 通过 | 通过 | 兼容 |
| GPT 5.6 Luna | Responses | 通过 | 通过 | 兼容 |
| Claude Sonnet 5 | Chat Completions | 通过 | 通过 | 兼容 |
| Claude Sonnet 5 | Messages | 通过 | 通过 | 兼容 |

真实Usage显示不同中转Route存在显著输入/推理开销差异，不能把它们视为“纯净原厂API”。Profile必须保留Route级HarnessDelta和实际Usage。
