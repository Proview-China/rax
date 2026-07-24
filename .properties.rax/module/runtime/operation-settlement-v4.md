# Runtime Operation Settlement V4模块说明

## 1. 作用

Operation Settlement V4把Evidence Owner已经原子消费的prepare/execute OperationScope Evidence V3与领域Owner的authoritative DomainResult Fact精确关联，并由Runtime Effect/Settlement Owner形成唯一V4终态。

## 2. 组成

- 公共V4 Evidence Binding、Submission、typed refs与Governance/Fact Port；
- Runtime Settlement Gateway的Evidence、Enforcement、DomainResult与Effect current复读；
- 同一Owner内Settlement、Association、terminal guard、terminal projection四对象原子publish；
- V3/V4共享`(TenantID, EffectID)` terminal guard；
- historical/current分离的只读闭包和公共Conformance。

## 3. 核心语义

- prepare与execute各一份exact Evidence V3消费链，不能交换、复用、遗漏或追加；
- 只接受`consumed_current`，late/observation不能形成Settlement；
- DomainResult必须是领域Owner的typed authoritative Fact，Provider Receipt/Observation不能替代；
- V3-first与V4-first对称互斥，跨Tenant相同Effect ID独立；
- 历史Association/Guard/Projection按Settlement ID读取真实四对象闭包，不借current索引；
- lost reply只Inspect，同canonical幂等，换内容Conflict；Provider不会被Settlement Gateway重派。

## 4. 验证

Owner与中央均完成full ordinary/shuffle、full race、Vet、gofmt与diff-check。中央定向`count=100` PASS（127.334s）、`race count=20` PASS（238.537s）；独立Review最终YES。

## 5. 限制

当前Store为确定性reference fake，只证明合同、锁内线性化、copy-on-write staged publish和恢复语义；不声明生产数据库持久性、进程崩溃耐久性、availability或SLA。G6A Action Matrix/Router已进入隔离fixture实现，但仍无生产composition root。

设计入口：[Operation Settlement V4](../../design/runtime/operation-settlement-v4/README.md)。
