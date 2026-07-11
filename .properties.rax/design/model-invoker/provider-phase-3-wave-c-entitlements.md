# 第三阶段波次 C：订阅与商业计划控制面设计卡

## 1. 状态

- 模块：`model-invoker`
- 波次：C
- 核验时间：2026-07-11 00:45 CST
- 依据：第三阶段总设计与已审核总计划
- 实施边界：只做离线控制面、Catalog、拒绝行为和测试；不读取真实 Key，不调用真实套餐或按量 API

## 2. 证据刷新后的设计修正

2026-07-11 的官方资料已经推翻“GLM、MiMo、Alibaba订阅只能全面 `terms_blocked`”这一旧判断：

- Kimi Code明确允许会员 Key接入第三方 Coding Agent，并要求保持真实 User-Agent；
- MiniMax Token Plan明确提供独立 Key并允许接入 Coding/Agent工具；
- GLM Coding Plan明确限定官方支持的编程工具和专属 Coding Endpoint；
- Xiaomi MiMo Token Plan明确提供 `tp-*`专属 Key、分区域 Endpoint，并禁止自动脚本、自定义应用后端和非 Coding用途；
- Alibaba Coding Plan与 Token Plan Team Edition明确允许编程工具/OpenClaw类 Agent，同时禁止自动脚本、Dify/n8n、API测试器和应用后端。

因此这些路线的控制面统一建模为 `interactive_coding_only`，而不是伪装成 `general_api`，也不再错误标记为全面 `terms_blocked`。xAI消费者权益仍缺少可供 model-invoker使用的公开 API Key/Base URL合同，保持 `official_client_only + unverified`。

## 3. 控制面决定

1. `CommercialEntitlement`继续拥有静态用途边界：显式交互场景、个人/单租户/前台、生产禁用、真实客户端身份和可选工具白名单。
2. 新增动态 `EntitlementState`，绑定 Offering与 Credential Profile，记录订阅状态、证据观察窗口、套餐到期和剩余额度。
3. 订阅 Route只有静态策略与动态状态同时允许时才可授权；状态过期、额度耗尽、套餐过期、暂停或绑定错位均 fail-closed。
4. 所有订阅的 `allows_automatic_payg_switch`固定为 false；402/403、额度耗尽或到期只返回稳定拒绝，不替换成按量 Key、Endpoint或 Offering。
5. API Key只保存秘密引用；运行时前缀校验接受明文的瞬时输入，但错误不得回显输入值。
6. Catalog登记计划路线和官方来源，但在没有独立 Provider设计、方言测试和 Adapter前一律 `planned + callable=false`。
7. Alibaba Savings Plan不是新 Provider、Route或 Credential，只能作为未来 `alibaba.model-studio`按量 Offering的 BillingPlan引用。

## 4. 首批 Catalog范围

- GLM Coding Plan：中国 Coding Endpoint，OpenAI兼容主路径；
- Kimi Code：OpenAI Chat与 Anthropic Messages；
- MiniMax Token Plan：OpenAI Chat与 Anthropic Messages；
- MiMo Token Plan：中国、新加坡、欧洲三 Region的 OpenAI Chat与 Anthropic Messages；
- Alibaba Coding Plan：中国与国际的 OpenAI Chat与 Anthropic Messages；
- Alibaba Token Plan Team Edition：中国的 OpenAI Chat与 Anthropic Messages；
- xAI消费者/Grok Build：仅外部 Agent占位，不是 HTTP模型 Provider。

## 5. 完成门槛

- 动态 entitlement、Key前缀、到期、额度、错绑定和禁止自动 PAYG均有黑白盒与 fuzz；
- 新 Catalog路线通过严格 Schema、证据 TTL、摘要、来源冲突、Credential audience/Offering/Region绑定和资产门禁；
- 当前 callable生成区块仍只包含已实现的四条直连 Binding；
- 真实凭据、真实认证、真实额度和公网调用全部保持延期。
