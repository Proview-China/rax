# 第三方中转站兼容 Route 实施计划 v1

- 状态：实现与首轮真实集成已完成；Gemini原生Route因中转上游持续429保留外部容量复测项
- 授权：用户明确要求中转兼容、必须通过model-invoker实测并在失败时修复
- 基线提交：`469e69f feat(praxis): add execution semantic union runtime`
- 设计：[第三方中转站兼容 Route v1](../../design/model-invoker/third-party-relay-compat-v1.md)

## 产物

1. 独立`provider/relaycompat`，不修改官方Provider信任边界；
2. 显式`routegateway.NewRelayCompatFactory()`，不默认注册；
3. Chat Completions、Responses、Messages、GenerateContent四协议；
4. 精确模型allowlist、canonical HTTPS、无跳转、秘密脱敏与保守能力合同；
5. 四协议离线文本/Tool Call黑盒测试和Factory白盒测试；
6. 8条真实Route集成探针、统一Usage和错误取证；
7. 模块说明、README和memory同步。

## 清单

- [x] 基线全离线验收并在改动前单独提交；
- [x] 新增独立Relay Provider和可选Factory；
- [x] 扩展内部compatprovider以复用GenerateContent Driver；
- [x] 锁定单Route单协议、精确模型和Endpoint；
- [x] 离线覆盖四协议文本、Tool Call、认证头、路径与负向合同；
- [x] 真实验证Gemini Chat、Grok Chat/Messages、GPT Chat/Responses、Claude Chat/Messages；
- [x] 确认Gemini原生请求经model-invoker到达中转并归一化为Retryable 429；
- [ ] 待中转上游容量恢复后补Gemini原生文本与Tool Call成功证据；
- [ ] 真实流式、结构化输出和工具结果回传按独立低预算批次补证据。

当前实现解决“中转站可作为受控Route进入统一语义”的问题，不证明中转模型纯净、不证明原厂身份，也不把中转站放进默认Catalog。
