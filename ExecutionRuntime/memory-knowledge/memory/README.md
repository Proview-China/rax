# Memory Owner backend-neutral framework

> 状态：**implementation_software_test_yes**。本包拥有Memory Candidate、Admission、Record版本链、生命周期、View/Projection、Commit Attempt、DomainResult和Owner Settlement Apply；不拥有Knowledge、Context、Runtime或其他领域事实。

## 已实现

- Create、Correction/Supersede、Pin、Archive、Merge、Decay/Expiry、Forget/Tombstone；
- Source/Evidence/Content exact refs、Retention/Legal Hold门禁、Authority/Policy/Purpose/Scope/Sensitivity绑定；
- expected-revision CAS、不可变历史、canonical tamper拒绝、同源sequence幂等与64并发单赢家；
- `DomainResultFact -> opaque RuntimeSettlementRef + DomainResultAssociation -> ApplySettlement`，Unknown只Inspect原Attempt；
- Memory View、current Watermark、Skill/Lexical/Vector/Graph Projection、Hybrid Query、Citation/Coverage/Cursor；
- Export、Watch、Reindex、metadata-only Purge Intent和Owner Job Journal；
- Owner-local V1/V2 Context Current Reader，支持ctx取消、exact Run/Session/Turn、稳定/新鲜摘要、bounded content和S1/Get/S2。

## 边界

- Retrieval Result、模型建议、Provider Observation、Context缓存都不是Memory正式事实；
- 内存Store与reference indexes只用于合同、白盒和Conformance，不是生产Backend、持久性或SLA承诺；
- 本包没有网络、Provider、Resolver、真实远程索引或物理Purge执行；
- 不接Context/Application production root；非零Context来源仍等待对应Owner公共合同。

## 验证入口

```text
go test ./memory ./memory/contextsource
go test -race ./memory ./memory/contextsource
```
