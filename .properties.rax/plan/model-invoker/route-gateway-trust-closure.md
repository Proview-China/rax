# Model Invoker Route Gateway信任闭合修正计划

## 1. 状态

- 状态：已完成并转为陈旧计划；
- 创建时间：2026-07-11 16:15 CST；
- 依据：[信任闭合修正设计](../../design/model-invoker/route-gateway-trust-closure.md)；
- 禁止：真实Key/套餐/付费调用、Runtime Kernel/Profile/Context/缓存策略、白皮书、`.gitignore`、`tmp.document`、提交和推送。

## 2. P1授权与模型

- [x] 默认16条订阅Route降为`blocked_by_host_trust`且不可调用；
- [x] 注入可信`SubscriptionAuthorizationResolver`，拒绝普通调用方自证claim；
- [x] fake build-manifest/runtime-observed/active entitlement全部零触达；
- [x] 受限订阅模型改为Catalog逐Route精确静态白名单并在Secret前拒绝；
- [x] Alibaba Coding Plan和Token Plan Team按官方证据拆分模型集合。

## 3. P2生命周期与Endpoint

- [x] 轮换Close错误进入安全生命周期聚合并由Gateway.Close观测；
- [x] stale Lease/Gateway.Close/并发释放错误均有反例；
- [x] Factory Provider身份、Closer和Endpoint在入池前验证；
- [x] 跨host/base-path/编码逃逸/无Closer/Close失败/并发等待者均不污染池。

## 4. 资产与验收

- [x] 重生成Catalog Binding、39×20语义矩阵和39条缓存事实；
- [x] 同步design/plan/memory/module/properties/README；
- [x] 运行gofmt、tidy diff、verify、vet、普通、shuffle、全仓race、integration仅编译、相关fuzz、覆盖率和资产门禁；
- [x] 复核白皮书、`.gitignore`与本轮`tmp.document`在实现写入后未再变化。

## 5. 实际验收

- `./scripts/verify-offline.sh`：通过；包含module verify、gofmt、tidy diff、diff check、vet、普通、shuffle、全仓race、integration仅编译和Catalog资产门禁；
- 相关3项fuzz各3秒：通过；执行12,498、21,444和1,956次；
- `go test -count=1 -coverpkg=./... -coverprofile=... ./...`：通过，合并语句覆盖率77.5%；
- 语义矩阵780行、39 Route、14个默认活跃Adapter、6协议；缓存事实39行；
- 受保护对象收口哈希：白皮书`3b70c876189332a19b383b780eb82882365446b3730786d855251aa7a5152fa3`、`.gitignore` `59b627880465606d96b2bfa781f2858cd46dc1b258d7114a8178259acdfc9353`、`tmp.document`内容聚合`08f89b811c31a9b58b2532e5568877d31a70df730f12b0e0b6c8f857ade54742`；本计划未修改这些对象；
- 未读取真实Key，未执行真实套餐、付费或公网Provider调用，未提交或推送。
