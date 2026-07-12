# Model Invoker Factory双层信任闭合计划

## 1. 状态

- 状态：已完成，计划转为陈旧计划；
- 设计依据：[Factory双层信任矩阵设计](../../design/model-invoker/factory-trust-matrix-v1.md)；
- 范围：现有18 Factory、14默认活跃Adapter、4受限订阅Factory；
- 禁止：真实Key、公网Provider/订阅/付费调用、新业务模块、Runtime Kernel/Profile/Context/缓存/工具/沙箱/编排、提交推送。

## 2. 清单

- [x] 从live Catalog/Registry确认18/14/4全集和两层评分口径；
- [x] 收敛公开Config Endpoint策略，修复任意远端HTTPS、端口、Host/path逃逸及订阅同Host跨路径；
- [x] 以协议层`Base.VerifyResponseModel`统一OpenAI Chat/Responses与Anthropic Messages的actual Model非流/流校验；Azure保留deployment projection，Gemini/Bedrock native明确标为indirect；
- [x] 生成并门禁18行双层Factory信任矩阵，Status只允许`pass/gap/not_applicable`；`indirect`只作为VerificationMode，并列出理由与代码/测试证据；
- [x] 补Endpoint helper反例/fuzz、10家公开Config动态反例、Factory identity/Endpoint回读与生命周期回归；
- [x] 完成定向、全仓、race、shuffle、integration guard/compile、30项fuzz、79.4%覆盖率、资产和diff验收；
- [x] 同步memory/module/properties并停止写入。

Factory精确边界：本计划验收的是18个固定`Version=v1candidate`的builtin值对象；Registry拒绝替换已注册AdapterID，因此不支持Factory实例热替换。Gateway会在每次`prepare`重读自定义`Factory.Version()`并纳入pool key，但这不等同于Factory实例热替换合同已完成。

## 3. 完成后产物

完成后，公开Provider构造器和Gateway生产路径不再混为一个结论；18个Factory均有由live Registry/Catalog核对的A/B层机器合同，Endpoint、Credential audience、响应Model和Build/Close语义可由离线测试重复验证。没有actual Model字段的协议会明确保留`Status=not_applicable, VerificationMode=indirect`，不会制造上游模型已验证的假结论。
