# Model Invoker 上游调用与统一封装最终候选收口计划

## 1. 计划状态

- 模块：`model-invoker`
- 状态：已完成离线实施与验收，作为陈旧计划保留
- 创建时间：2026-07-11 14:06 CST
- 顺序：A→B→C→D→E→F；每阶段独立验收并同步memory
- 设计依据：`design/model-invoker/route-gateway-final-candidate.md`
- 禁止范围：Model Profile、缓存策略、Context Engine、Runtime Kernel、工具执行器、旁路、沙箱、调度、组织与编排
- 禁止操作：真实Key/登录态/付费调用、修改用户白皮书/`.gitignore`/`tmp.document`、提交或推送

## 2. A 资产纠正

- [x] 保护live未提交和用户并发对象；
- [x] 将RouteInvoker纠正定位为Policy/Authorization/Audit层；
- [x] 修复手工xAI矩阵状态与76.5%最终残留；
- [x] 修正`tmp.document`仍存在且归属不确定；
- [x] 将语义原语/Route合同降为candidate与阶段性完成；
- [x] 运行A阶段资产/代码验收并写memory。

## 3. B Route Gateway组合层

- [x] 定义SecretResolver、RuntimeBindingResolver、AdapterFactoryRegistry、AdapterPool/Lease和Gateway API；
- [x] 实现14个AdapterID工厂和Route一致性门禁；
- [x] 实现轮换/binding/evidence失效、单飞、并发与Close语义；
- [x] 实现Resolve/Capabilities/Invoke/Stream；
- [x] 39条callable Route执行真实离线构造与实际Capabilities合同；
- [x] 安全、竞态、失败、取消、超时、流、泄密与fuzz；
- [x] 独立验收并写memory。

## 4. C 统一语义v1候选

- [x] 生成并校验原语×六协议×39Route/14Adapter×支持级别×映射动作矩阵；
- [x] 逐项判断内容块、结构化工具结果、工具类型、输出、流、State/Options/Usage/Raw扩展；
- [x] 审核确认当前无需立即加公共类型，固定未来只做保留旧义的加法式演进规则；
- [x] 形成xAI gRPC、DashScope、Meta、多模态、Hosted/Batch/Realtime/后台清单；
- [x] 独立验收并写memory。

## 5. D 实际订阅调用面

- [x] 联网刷新Kimi/MiniMax/MiMo/GLM/Alibaba官方一手资料；
- [x] 每个候选Route产出精确设计卡与条款结论；
- [x] 只实现明确允许Praxis自定义交互式客户端的16条callable Route、4个Factory/Adapter身份；
- [x] GLM Coding Plan与xAI消费者等official_client_only/权利不清路线保持不可调用；
- [x] 独立验收并写memory。

## 6. E Provider缓存事实交接

- [x] 建立Route/Adapter/协议精确事实资产或Catalog元数据；
- [x] 覆盖隐式缓存、显式字段、key所有权、TTL/State、usage、错误、限制与证据TTL；
- [x] 列出CacheIntent候选接缝与联合决策问题，不实现策略；
- [x] 独立验收并写memory。

## 7. F 最终验收与同步

- [x] 分清Policy层与Gateway层职责；
- [x] 运行gofmt、tidy diff、verify、vet、普通、shuffle、全仓race、integration仅编译、相关fuzz与覆盖率；
- [x] 确认测试不创建/修改仓库无关资产；
- [x] 同步design/plan/memory/module/properties/README/Catalog/生成矩阵；
- [x] 最终按“已完成/候选待联合审核/明确延期/需用户决定/未运行真实验证”分类。

## 8. 预期产物

完成后，39条按量/云callable Route具备真实离线构造路径和Gateway生命周期；明确获准的订阅Route才增加callable实现；统一语义与缓存只形成可供相邻线程审核的精确候选合同，不越权实现其消费者策略。
