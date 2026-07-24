# Memory Engine终局框架设计 V1

> 状态：**design_confirmed_by_business_document / component_framework_implemented_software_test_yes**。本设计翻译`tmp.document/Memory&Knowledge.md`中Memory部分，不改变其他领域的唯一Owner；跨Owner非零Context接线仍等待公共合同。

## 1. Owner边界

Memory拥有Candidate、Admission、Record历史、Correction/Supersede/Merge、Pin/Archive/Forget/Tombstone、View/Watermark、Projection Descriptor、Inspect/CAS、DomainResult和Cleanup/Residual。

Memory不拥有Timeline/Checkpoint、Context Frame/Generation、Review Verdict、Runtime Settlement语义、Identity Lease/Authority/Policy、Artifact字节、模型输出或Provider事实。Continuity和模型只能提出Candidate；Context只消费Owner current exact refs。

## 2. Scope与View

`MemoryScopeCoordinateV1`闭集：`run_working`、`identity_private`、`agent_lineage`、`user`、`team`、`domain`、`organization_shared`。它绑定Tenant、ScopeKind/ID、IdentityRef/Epoch、LineageRef、AuthorityRef/Epoch、PolicyRef、Purpose和Sensitivity。

继承由`MemoryViewPolicyV1`显式列出父Scope、Disclosure Mode（`full`、`summary`、`citation_only`、`existence_only`、`denied`）和预算；默认不继承。子Agent、Reviewer和管理Agent均不能凭角色获得隐式全量访问。

## 3. 生命周期

```text
Candidate -> rejected | merged | review_required | commit_ready
          -> active
          -> corrected/superseded | merged | pinned | archived | expired
          -> forget_requested -> tombstoned
          -> purge_pending | purge_complete | purge_residual
```

Correction创建新Revision并保留`Corrects`；Merge创建具名Merge Fact，不静默拼接正文；Pin不能绕过Policy、Legal Hold或明确Forget。物理Purge若涉及外部存储属于Effect。Retention/Legal Hold只以其他Owner exact ref进入门禁。

## 4. Projection与Hybrid Retrieval

Record是权威事实；Skill/Lexical/Vector/Graph都是可重建Projection。`MemoryProjectionDescriptorV1`绑定kind/version、Record exact refs、Builder/Model、Chunk策略、维度或Graph schema、Content digests、Coverage、State、Watermark和TTL。

- Skill Entry保存Title、Description、Keywords、UseWhen、DoNotUseWhen、SourceRef、RecordRef、DetailRef、Revision和Digest。
- Vector Entry绑定Record/Revision/Chunk range/Embedding Model/Dimension/Index Version/Content Digest；相似度只产生Candidate。
- Graph Node/Edge绑定Owner/Scope/Revision/Source/Confidence/Valid Time/Transaction Time；模型推断只能先成为Candidate。

查询顺序固定为：Validate Query/View→权限/Scope/Sensitivity预过滤→路由四类Projection→有界召回→Record currentness与Evidence复读→去重/冲突/diversity/rerank→预算/Cursor→持久Retrieval Result exact refs。无权限候选不得先读取全文再过滤；结果仍不是Context Frame。

## 5. Consolidation

`MemoryConsolidationBatchV1`绑定Continuity Timeline范围、Outcome/Review/Artifact exact refs、Policy、规则或模型Route/Version、Input Digest、候选、拒绝原因和TTL。Consolidator只提交Candidate。未结算Effect、Unknown Outcome、原始Chain of Thought、临时猜测和未经授权私人数据必须拒绝或Residual。

## 6. Effect与开发者入口

本地纯计算Projection rebuild不是Effect；远程Embedding/Graph/Store、Purge、Export和正文外发属于Effect。Begin后丢回包只Inspect原Attempt。

Go SDK提供Submit/Admit/Commit/Correct/Merge/Pin/Archive/Forget/Inspect/Query/Watch/Reindex/Consolidate。CLI/API只封装Owner Port，不直接访问Store map；自定义Store/Retriever/Indexer/Consolidator必须通过conformance。
