# Memory + Knowledge 实施计划入口

## 终局框架闭环

- [业务文档覆盖矩阵](./document-completion-matrix-v1.md)
- [终局框架实施计划](./framework-completion-v1.md)
- [External P0 live收敛审计](./external-p0-live-closure-audit-20260717.md)

## 当前状态

- 状态：**Memory/Knowledge backend-neutral framework、cross-owner reference integration与non-HA/HA production closure verifier均implementation_software_test_yes**。Verifier复用Runtime公开Resource Reader并对全部生产exact refs执行S1/S2；实际部署资源、外部Owner current、独立Certification与production root注入前仍NO-GO。
- 当前双轴：**Owner-local P0=0/P1=0/P2=0；cross-owner reference P0=0/P1=0/P2=0**。生产装配、真实远程Effect与Backend/SLA另行评审，不以reference/fake冒充。
- 业务基线：`tmp.document/Memory&Knowledge.md`。
- 设计入口：
  - [Memory Owner设计](../../design/memory-engine/README.md#91-memory-owner独立inspect到context物化桥)
  - [Memory Context Source合同](../../design/memory-engine/contract-v1.md#9-per-turn-context-source合同需求)
  - [MemoryContextSourceCurrentReaderV1 Delta](../../design/memory-engine/context-source-port-v1.md)
  - [Memory Delta 10/11 Owner V2冻结与External接线候选](../../design/memory-engine/context-refresh-neutral-delta10-11-v1.md)
  - [P0-2 Context TransitionProof联合审查候选](../../design/memory-engine/context-transition-proof-external-p0-candidate.md)
  - [P0-4 Memory Context Source Adapter候选](../../design/memory-engine/context-source-adapter-external-p0-candidate.md)
  - [Memory Retrieval Domain Gateway Delta](../../design/memory-engine/retrieval-domain-gateway-v1.md)
  - [Memory桥架构图](../../design/memory-engine/architecture.drawio)
  - [Knowledge Owner设计](../../design/knowledge-engine/README.md#91-knowledge-owner独立inspect到context物化桥)
  - [Knowledge Context Source合同](../../design/knowledge-engine/contract-v1.md#9-per-turn-context-source合同需求)
  - [KnowledgeContextSourceCurrentReaderV1 Delta](../../design/knowledge-engine/context-source-port-v1.md)
  - [Knowledge Delta 10/11现有Reader V2与fragment kind候选](../../design/knowledge-engine/context-refresh-neutral-delta10-11-v1.md)
  - [P0-5 knowledge_reference联合审查候选](../../design/knowledge-engine/knowledge-reference-external-p0-candidate.md)
  - [P0-4 Knowledge Context Source Adapter候选](../../design/knowledge-engine/context-source-adapter-external-p0-candidate.md)
  - [Knowledge Retrieval Domain Gateway Delta](../../design/knowledge-engine/retrieval-domain-gateway-v1.md)
  - [Knowledge桥架构图](../../design/knowledge-engine/architecture.drawio)
- 本目录不构成Runtime、Harness、Application、Review、Evidence或Context公共合同的单方变更。

## 文件

| 文件 | 用途 |
|---|---|
| [memory-knowledge-v1.md](./memory-knowledge-v1.md) | 阶段、文件级落点、依赖、准入/退出与迁移 |
| [port-delta.md](./port-delta.md) | 已解决的cross-owner reference Delta与继续unsupported的远程Gateway Delta |
| [delta10-11-live-contract-audit.md](./delta10-11-live-contract-audit.md) | live V1 Reader到同合同族冻结V2的字段差异、唯一current与Turn语义 |
| [context-source-reader-v2-frozen.md](./context-source-reader-v2-frozen.md) | 两Owner各七个V2 struct、StableClosure精确body及Memory六个/Knowledge八个set digest算法；Owner-local冻结合同 |
| [context-transition-knowledge-reference-review-plan.md](./context-transition-knowledge-reference-review-plan.md) | P0-2/P0-5的historical review plan；live实现见当前合同与测试 |
| [owner-context-source-adapter-p0-4-review-plan.md](./owner-context-source-adapter-p0-4-review-plan.md) | P0-4双Owner Adapter的historical review plan |
| [acceptance-test-matrix.md](./acceptance-test-matrix.md) | 单元、白盒、黑盒、故障注入、race、vet、集成和系统验收 |

## 实施门禁

领域Wave 1、两个Owner V1/V2 Reader、Memory/Knowledge Adapter、Application三阶段Port、Context TransitionProof/`knowledge_reference`与Memory=1/Knowledge=1 reference fixture均已YES。V1/V2共享同一Owner Store/current；Session/Turn exact证据只来自Harness committed PendingAction current并经public Adapter无损传递，任何Owner不得补造。顺序固定为`Owner S1 -> pending outputs seal -> proof seal -> Owner S2 -> atomic Apply/CAS -> publish`。production root尚未装配；远程Gateway继续unsupported且Provider/Resolver=0。

Go是v1唯一计划语言。当前没有Rust热点结论，也不预选生产数据库、RPC、进程拓扑或SLA。

## Component Release/readiness

- [x] [Component Release V1](component-release-v1.md) owner-local实现候选：两Owner Capability/Port/Factory、三档支持模式、exact readiness与Catalog恢复；
- [x] non-HA/HA生产证明Bundle、durable Resource exact复读、外部current关联与Release readiness适配的软件侧；
- [ ] 实际durable State Plane资源、Credential/生产current、Purge/Cleanup、Deployment Attestation、独立Certification与production root环境注入；未完成前deployment production NO-GO。
