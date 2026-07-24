# Memory&Knowledge终局文档覆盖矩阵 V1

> 状态：**component_framework_complete / cross_owner_reference_integration_complete / production_activation_pending**。最高业务输入为`tmp.document/Memory&Knowledge.md`。backend-neutral组件框架与必要跨Owner reference链已完成软件实现与质量门；不预选生产DB、向量库、图库、RPC、进程拓扑或SLA。

> 2026-07-17 current truth：领域框架及Harness exact Turn→Application三阶段→Owner S1→Context pending/proof→Owner S2→Context atomic Apply/CAS→exact Frame reference链均已落地。真实远程/物理Effect、生产Backend/SLA与production root继续unsupported。

## 1. 当前覆盖

当前已闭合Memory/Knowledge两个Owner权威事实、V1/V2 Owner-local Current Reader、生命周期、Projection/Index、Hybrid Retrieval、Consolidation、Knowledge Sync、开发者入口与cross-owner reference消费链。剩余是生产部署/Backend选择及真实受治理远程Effect，不属于本文backend-neutral实现口径。

| 文档能力 | live覆盖 | 状态 | 终局退出条件 |
|---|---|---|---|
| Owner隔离 | 独立package、Store、DomainResult/Settlement | 已完成 | Conformance持续证明无跨Owner事实写入 |
| Candidate→Admission→Commit | 两Owner CAS、Review gate、Inspect、lost-reply、DomainResultAssociation/Settlement Apply | 已完成 | 真实Runtime dispatch由公共治理链承载 |
| Memory生命周期 | Create/Correction/Supersede/Pin/Archive/Merge/Decay/Expiry/Forget/Tombstone/Purge Intent | 已完成 | 物理Purge执行仍是外部受治理Effect |
| Knowledge生命周期 | Source/Package/Candidate/Record/Snapshot/Pointer/Correction/Conflict/Deprecate/Withdraw/Purge Intent | 已完成 | 真实Acquire/远程Publish执行仍是外部受治理Effect |
| Scope与继承View | 全部ScopeKind、exact Principal/Authority/Policy/Purpose、Disclosure、预算与TTL | 已完成 | Authority/Policy由其Owner签发，本组件只消费exact refs |
| Skill Index | versioned Entry、UseWhen/DoNotUseWhen、Detail/Source exact refs、确定性reference实现 | 已完成 | 生产index backend未预选 |
| Lexical Index | versioned posting、Coverage、重建状态与确定性reference实现 | 已完成 | 同上 |
| Vector Index | backend-neutral chunk/embedding/model/dimension/index合同与确定性reference实现 | 已完成 | 无真实embedding provider/生产向量库 |
| Graph Index | versioned Node/Edge、来源、置信度、valid/transaction time与确定性reference实现 | 已完成 | 无生产图库 |
| Hybrid Retrieval | Intent/channel预算、四路Observation、早期过滤、RRF、去重/冲突、Cursor、Citation/Coverage | 已完成 | 远程channel gateway unsupported |
| Consolidation | exact settled Timeline输入、Policy/Builder版本、Candidate/Reject、可回放Batch | 已完成 | Continuity真实Adapter属跨Owner后续接线 |
| Knowledge Sync | Acquire→Parse→Normalize→Validate→Index→Snapshot→Publish Journal与两阶段Owner Apply | 已完成 | 真实Connector仅有Observation合同，Provider=0 |
| 安全治理 | Scope/Sensitivity/License/PII/Secret/Prompt Injection/Poison/Retention/LegalHold/Disclosure | 已完成 | 不替代Policy/Review Owner事实 |
| SDK/CLI/API | 公共Go Clients、严格HTTP handler、七个文档CLI命令、Cursor/CAS/Watch/Reindex/Inspect | 已完成 | 部署、认证实现、endpoint与SLA未预选 |
| Context非零注入 | Memory=1、Knowledge=1的real V2 Reader reference fixture，经S1/S2与proof发布exact Frame | 已完成（reference） | production root另行装配与系统验收 |

## 2. 不属于完成的替代品

- 仅增加Vector Top-K或绑定某一家数据库，不算Hybrid Retrieval完成。
- Fake/reference backend通过测试，不代表生产Backend、远程Effect或SLA。
- Provider命中、模型总结、Timeline Event或Context Cache不能直接成为正式事实。
- DTO没有Owner Inspect/CAS、Unknown恢复、canonical tamper和并发反例，不算合同闭合。
- Context接收字符串但不复读Owner exact ref/currentness/TTL，不算非零接线。

## 3. 无SCC实施顺序

```text
Owner合同与Scope/Lifecycle
  -> 可重建Projection合同
  -> Skill/Lexical/Vector/Graph reference实现
  -> Hybrid Retrieval
  -> Memory Consolidation + Knowledge Sync
  -> SDK/CLI/API
  -> Turn/Context/Application非零接线
  -> 集成/系统/Conformance与资产收口
```
