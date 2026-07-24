# Model Tool portable方言门与Route兼容Delta

时间：2026-07-18 14:24 Asia/Shanghai

## 粗粒度结果

- Tool Owner在既有`praxis.model/function-calling-v1`中补齐portable expression profile；
  不新增厂商DTO、SDK或业务Tool。
- 名称固定为live OpenAI/Anthropic/Gemini adapter共同接受的
  `[A-Za-z_][A-Za-z0-9_-]{0,63}`。
- 保持既有`Strict=true`设计，并在组装前验证所有object完整`required`、
  `additionalProperties=false`及portable keyword闭集；neutral结构通过但厂商方言不接受的
  名称/Schema现在会在Tool组装期Fail Closed、零输出。
- 定向ordinary×100与race×20通过。
- live Model Invoker的`Request.Validate`、粗粒度Capability与Route Resolve仍不提供
  route-specific Tool dialect compatibility current事实；已登记`PD-TM-05` additive只读
  Projection候选。该公共面闭合前不得把portable owner-local编译升级为production Route兼容。

## 边界

- Tool不导入`model-invoker/internal`、provider实现包或厂商SDK，不复制厂商规则到
  production adapter。
- portable profile是版本化表达合同，不授Authority、Review、Fence或Provider执行权。
- Package Offline Verify仍等待用户对current TTL与legacy admission处置的裁决，本事件不解锁它。
