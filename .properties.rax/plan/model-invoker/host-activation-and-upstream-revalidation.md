# Model Invoker宿主激活与十家上游再验证计划

## 1. 状态

- 状态：已完成（离线实现与验收）；
- 创建时间：2026-07-11 18:16 CST；
- 设计依据：[宿主激活与十家上游再验证设计](../../design/model-invoker/host-activation-and-upstream-revalidation.md)；
- 禁止：真实Key/套餐/付费调用、生产批准、Runtime Kernel/Profile/Context/缓存策略、白皮书、`.gitignore`、`tmp.document`、提交和推送。

## 2. 官方证据与P0

- [x] 十家逐家核对官方协议、Endpoint、认证、模型、流、工具、错误与商业边界；
- [x] 十家直连P0 Route改为官方exact static model集合；
- [x] OpenAI排除受限预览`gpt-5.6`；Z.AI Catalog与Provider均收窄`glm-5.2`；
- [x] Qwen加入`qwen3.7-plus`，MiMo刷新下线事实与Token Plan来源；
- [x] DeepSeek/Kimi Code/MiniMax Token Messages修正为`x-api-key`并锁定HTTP fixture。
- [x] DeepSeek/Kimi/MiniMax/Z.AI公共构造器拒绝任意远端HTTPS Host；Gateway统一拒绝流/非流响应模型漂移，DeepSeek/Kimi保留Provider层二次门禁。

## 3. P1激活与宿主构造

- [x] 实现immutable `ActivationPlan`、原子Apply、稳定DecisionCode和canonical AuditDigest；
- [x] 实现exact RouteID/evidence/adapter pin、disable恢复、terms/official-client/evidence门禁；
- [x] 实现`HostConfig/NewHost/HostBuildReport`与完整失败报告；
- [x] 覆盖typed-nil、未审计预激活、缺Factory/Resolver、混合计划零部分生效与报告脱敏；
- [x] GLM Coding Plan客户端名单/限制纠正，CN Endpoint未闭合时降为unverified并保持不可调用。

## 4. 真实烟测入口

- [x] 新增十家P0 production Route Gateway smoke；
- [x] 新增Kimi Code与MiniMax Token两类条款允许的P1 subscription Route Gateway smoke；
- [x] MiMo/Alibaba三类保留离线Gateway验证但删除真实自动smoke，避免违反禁止脚本/API测试器边界；
- [x] 双显式开关、显式Key/Route/Model、精确Secret Profile pin、timeout和response marker；
- [x] 离线脚本清除全部新增Key、开关、Route和Model环境变量；
- [x] 真实账号调用未执行；本轮禁止，只有条款允许的入口被保留。

## 5. 资产与验收

- [x] 重生成Provider Matrix、39×20语义矩阵和39条缓存事实；
- [x] 同步design/plan/memory/module/properties/README；
- [x] 运行gofmt、tidy diff、verify、vet、普通、shuffle、全仓race、integration仅编译；
- [x] 运行相关fuzz与`-coverpkg=./...`覆盖率；
- [x] 复核白皮书、`.gitignore`与`tmp.document`哈希未变化。

实测：统一离线入口通过；Catalog、Route Gateway、Qwen与Z.AI相关5项fuzz各3秒通过；全仓合并语句覆盖率78.0%；第二棒审查发现并修复“条款禁止路线仍有自动live smoke”与“Close不等待首次Factory Build”两项P1，修正后的Route Gateway普通/race测试与integration guard通过；兼容Provider任意Host与响应模型漂移反例也已进入全量回归。

## 6. 完成后产物

完成后，默认39条Route保留真实离线Gateway构造路径且十家P0拥有Catalog级exact模型门禁；16条订阅Route继续默认fail-closed，但宿主可用公开原子ActivationPlan和HostConfig按精确Route启用。十家P0及Kimi Code/MiniMax Token有显式Gateway smoke入口；MiMo/Alibaba三类只保留条款安全的离线验证。Anthropic Agent SDK、Gemini Interactions及其他P2 native合同只形成后续决策项，不越权实现。
