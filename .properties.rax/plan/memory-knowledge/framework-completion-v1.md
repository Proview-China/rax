# Memory + Knowledge终局框架实施计划 V1

> 状态：**W1-W6 implementation_software_test_yes / production_activation_no_go**。用户已确认业务范围、backend-neutral完成口径及必要跨Owner接线。不选择生产DB、Vector DB、Graph DB、RPC、进程拓扑或SLA；reference/fake不得冒充生产能力。

> 2026-07-17 current truth：W1-W4已补齐全部领域框架；W5已由Harness public Turn applicability映射、Application三阶段、Context TransitionProof/`knowledge_reference`、双Owner Adapter与非零fixture闭合；W6 ordinary100/race20/full test/race/vet与边界扫描已执行。production root、真实远程Effect和生产Backend仍未启用。

## 1. 最终产物

1. 两个独立Owner的完整生命周期、Scope/View和治理合同；
2. Skill/Lexical/Vector/Graph可替换Projection及Hybrid Retrieval；
3. Continuity→Memory Consolidation与Source→Knowledge Sync/Snapshot框架；
4. Go SDK、CLI命令层和backend-neutral API handler/Port；
5. V2 Reader到Context pending/proof/S2/atomic publish的非零集成；
6. 单元、白盒、黑盒、故障、Conformance、集成、系统、race、vet和资产收口。

## 2. 分波DAG与文件落点

### W1：合同、Scope与生命周期

只写`ExecutionRuntime/memory-knowledge/**`：

```text
contract/scope.go
contract/index.go
contract/jobs.go
memory/lifecycle.go
memory/view_policy.go
knowledge/sync.go
knowledge/view_policy.go
tests/contracts_*.go
```

退出：strict canonical、closed enums、nested exact refs、CAS、TTL、duplicate semantic key、Retention/LegalHold refs、Unknown Inspect反例通过。

### W2：Projection与Hybrid Retrieval

```text
projection/skill/
projection/lexical/
projection/vector/
projection/graph/
retrieval/planner.go
retrieval/merge.go
retrieval/rerank.go
retrieval/conformance/
```

Vector/Graph只落backend-neutral Port与确定性reference backend；无benchmark不引入Rust。退出：权限过滤早于正文读取；插入顺序不影响结果；Cursor绑定View/Snapshot/Watermark；Coverage诚实；Citation exact。

### W3：Consolidation与Knowledge Sync

```text
consolidation/
knowledge/sync_controller.go
knowledge/sync_store.go
ports/continuity_source.go
ports/connector.go
ports/parser.go
ports/indexer.go
```

退出：Timeline/Provider只产生Candidate/Observation；阶段Journal可回放；Begin后lost reply只Inspect；失败不发布Snapshot；Withdraw/Purge Residual可见。

### W4：SDK、CLI与API合同

```text
sdk/
cmd/praxis-memory-knowledge/
api/
conformance/
```

CLI覆盖memory search/inspect/forget、knowledge source list/snapshot build/query、index status。API支持Cursor、Snapshot/View、权限过滤、引用展开、async Job、CAS和幂等；不得直连内部map。

### W5：Cross-owner reference链与非零Context接线（已完成）

```text
Turn exact Ref/Reader
 -> Context TransitionProof + knowledge_reference
 -> Application prepare/apply/inspect三阶段Port
 -> Memory/Knowledge Adapter
 -> Context nonzero source + G6B/root fixture
```

只在对应Owner目录新增版本化合同/Adapter/测试；不复制Memory/Knowledge DTO，不让Application或Harness创建领域Fact。Harness只消费Context发布并Inspect current的exact Frame。

### W6：系统验收与同步

同步Memory/Knowledge自有design/plan/module/memory；全局索引由集成Owner处理。旧计划保留为historical/stale。

## 3. 测试矩阵

| 级别 | 必测内容 |
|---|---|
| Unit | canonical、状态迁移、Scope/View、排序、预算、Cursor、TTL |
| White-box | CAS线性化、无部分提交、Journal/Unknown、copy isolation |
| Black-box | 公共SDK、CLI/API请求响应、无内部包依赖 |
| Fault | lost reply、clock rollback、binding drift、eviction、partial index、withdraw/purge residual |
| Conformance | Store/Retriever/Indexer/Connector/Consolidator自定义实现 |
| Integration | Continuity Candidate、Knowledge Sync、Context S1/S2 atomic publish |
| System | CLI流程、Snapshot固定查询、Correction/Forget/Withdraw失效传播 |
| Quality | ordinary100、race20、full test/race、vet、gofmt、diff、import/no-network |

## 4. 永久不变量

- Memory与Knowledge不合并Owner/current；模型/Provider不能直接提交正式事实。
- Record/Source/Snapshot是权威事实，Index/Cache/Context是可失效Projection。
- 外部Effect遵循Intent/Reservation→Admission→Permit→Begin→Prepare→Enforcement→Execute/Inspect→Observation→Owner DomainResult→Evidence→Runtime Settlement→Owner Apply。
- ContextReference无法exact物化时Fail Closed或Residual。
- 生产Backend、远程Gateway和SLA在独立选择、实现与验收前保持unsupported。
