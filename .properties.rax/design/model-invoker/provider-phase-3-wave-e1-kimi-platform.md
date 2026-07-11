# 第三阶段波次 E1：Kimi开放平台按量 Provider 设计卡

## 1. 状态与边界

- 模块：`model-invoker`
- Route family：`kimi.platform`
- Provider ID / Runtime Adapter ID：`kimi`
- Offering：`kimi.platform.payg`，`allowed_usage=general_api`
- Deployment：`kimi.platform.cn`，Region=`cn`
- 设计状态：已按 2026-07-11 官方公开合同刷新，可进入离线实施
- 实现范围：`https://api.moonshot.cn/v1` 的 OpenAI Chat Completions兼容路径
- 明确不含：`api.kimi.com/coding` Kimi Code会员、国际站 Key/Endpoint、Partial Mode、官方工具、文件、Batch、多模态输入和真实调用

Kimi开放平台按量 API与 Kimi Code会员是不同 Offering。两者的 Key、Endpoint、允许用途、模型别名和额度不得互换；按量 Adapter不接受 `KIMI_CODE_API_KEY`或 `kimi-for-coding`。

## 2. 官方证据

| Source ID | 官方来源 | 本卡使用结论 |
|---|---|---|
| `kimi.platform.overview.2026-07-11` | <https://platform.kimi.com/docs/api/overview> | OpenAI Chat兼容、`https://api.moonshot.cn/v1`、Bearer Key、端点与错误包络 |
| `kimi.platform.models.2026-07-11` | <https://platform.kimi.com/docs/models> | 当前模型列表、已下线模型、K2.7/K2.6/K2.5与 Moonshot V1边界 |
| `kimi.platform.parameters.2026-07-11` | <https://platform.kimi.com/docs/api/models-overview> | K2.7强制 thinking与 preserved thinking；K2.6 thinking开关 |
| `kimi.platform.reasoning.2026-07-11` | <https://platform.kimi.com/docs/guide/use-kimi-k2-thinking-model> | `reasoning_content`非流/流、保留规则、工具循环与温度限制 |
| `kimi.platform.tools.2026-07-11` | <https://platform.kimi.com/docs/api/tool-use> | 工具名、JSON Schema子集、默认 strict和最多128个函数 |
| `kimi.platform.errors.2026-07-11` | <https://platform.kimi.com/docs/api/errors> | 400/401/403/404/429/499/500/503与具体错误类型 |

证据 TTL为7天。`kimi-k2`预览/思考系列已于 2026-05-25下线，`kimi-latest`与 `kimi-thinking-preview`也已下线，均不得进入 callable集合。

## 3. Route与 Credential

| 维度 | 值 |
|---|---|
| Route ID | `kimi.platform.cn.payg.chat_completions` |
| Model family | `kimi` / `moonshot-v1` |
| 当前文本模型 | `kimi-k2.7-code`、`kimi-k2.7-code-highspeed`、`kimi-k2.6`、`kimi-k2.5`、`moonshot-v1-8k/32k/128k` |
| Protocol | `chat_completions` |
| Endpoint | `https://api.moonshot.cn/v1` |
| Credential | `MOONSHOT_API_KEY`秘密引用 |
| Auth | `Authorization: Bearer` |

只允许固定官方 HTTPS host或 loopback测试服务；禁止 userinfo、query、fragment、重定向和跨 host发送 Key。国际站 Key与中国站 Key独立，国际 Endpoint需未来单独设计 Route，不能通过任意 Base URL绕过。

## 4. 模型方言

### 4.1 K2.7 Code

- 始终开启 thinking，`Reasoning.Effort=none`在 HTTP前拒绝；
- 官方要求无需且不应传 `thinking`，Adapter不会伪造开关；
- 统一 effort没有精确原生档位，除 `none`外只作为请求 reasoning的意图，记录显式映射决定；
- 非流 `reasoning_content`映射为 reasoning output；流式同名 delta映射为 `reasoning_delta`；
- Preserved Thinking始终开启，但当前统一 Chat输入无法回传 assistant `reasoning_content`。包含函数结果的后续 thinking请求必须拒绝，不静默丢失历史推理。

### 4.2 K2.6 / K2.5

- `Reasoning.Effort=none`映射 `thinking.type=disabled`，其他非空 effort映射 `enabled`；
- `reasoning_effort`不是 Kimi原生档位，不发送；
- K2.6的 `thinking.keep=all`与 K2.7的强制 preserved thinking不在当前 slice；K2.5不支持 preserved thinking；
- 多步 thinking工具循环在统一输入可忠实携带 reasoning历史前保持拒绝。

### 4.3 Moonshot V1

- 当前只实现文本生成、流和 usage；
- reasoning请求、视觉预览模型、文件与多模态输入均拒绝；
- 模型 ID精确匹配，不接受已下线别名。

## 5. 能力合同

| 能力 | K2.7/K2.6/K2.5 | Moonshot V1文本 |
|---|---|---|
| 文本、流、usage | compatible | compatible |
| 工具调用 | compatible，最多128个 | unknown，本卡不声明 |
| parallel tools | compatible，具体模型运行时核验 | unknown |
| reasoning | Kimi原生方言 | unsupported |
| reasoning output | compatible | unsupported |
| JSON Object | partial，禁止与 Partial Mode混用 | partial |
| JSON Schema | unsupported | unsupported |
| provider continuation | unsupported；保真入口未实现 | unsupported |
| vision/video/file/Batch | 当前 slice unsupported | unsupported |

工具 `strict`未指定时保留 Kimi默认 true；显式 false原样发送。函数名必须满足 Kimi规则，根 schema必须为 object。Partial Mode需要修改最后一条 assistant消息并改变输出拼接语义，本卡不通过任意 ProviderOptions偷渡。

## 6. 错误、流与安全

- 400 `content_filter`归一化为策略拒绝，其余请求错误为 invalid request；
- 401认证、403权限、404模型/资源、429限流或额度、499取消/连接、500/503服务错误分别归一化；
- `exceeded_current_quota_error`归 Billing且不自动切换其他 Key或 Kimi Code余额；
- `engine_overloaded_error`、rate limit与服务不可用按官方语义设置 retryable；
- 只白名单 request ID、retry-after和限流头；SDK错误与 Key不得进入公开 unwrap链；
- SSE reasoning必须先于 text原样保留，所有事件单调且只有唯一终态；未知事件 fail closed。

## 7. 离线测试与真实烟测

离线黑盒必须覆盖：

1. 官方 OpenAI SDK对本机 HTTP fake的 `/v1/chat/completions`、Bearer Header、body和 SSE；
2. 当前模型正例与已下线/未知/视觉模型预调用拒绝；
3. K2.7强制 thinking、K2.6/K2.5开关、Moonshot reasoning拒绝；
4. 非流/流 `reasoning_content`、工具、usage、JSON Object和唯一终态；
5. thinking工具续接无法保真时拒绝，Kimi Code Endpoint/Key/模型错配拒绝；
6. 400策略、401、403、404、429 quota/rate、499、500、503、重定向、取消、超时和泄密；
7. Catalog、Schema、Markdown、公共 SDK签名和 fuzz门禁。

真实烟测只提供 `integration` build tag入口，要求全局确认、Kimi专属确认、`MOONSHOT_API_KEY`和精确 `KIMI_SMOKE_MODEL`。本阶段只编译入口，不执行真实认证或付费调用。

## 8. 本卡完成产物

- `provider/kimi/`独立 Adapter；
- 一条 callable Catalog Binding，与两条 Kimi Code控制记录并存但不互换；
- `tests/kimi/`黑盒、fuzz和 `tests/integration/`显式烟测入口；
- README、Matrix、module、properties、plan与 memory同步；
- 状态最多为 `implemented_offline`，不标 `live_verified`或生产批准。
