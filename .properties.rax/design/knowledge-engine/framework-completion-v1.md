# Knowledge Engine终局框架设计 V1

> 状态：**design_confirmed_by_business_document / component_framework_implemented_software_test_yes**。Asset继续拥有原始字节，Context继续拥有Frame，Knowledge只拥有Source/Package/Record/Snapshot及其Projection；跨Owner非零Context接线仍等待公共合同。

## 1. Owner与权威对象

Knowledge拥有Source、Package、Record/Claim、Conflict、Withdraw、Snapshot/Pointer、View、Projection Descriptor、Sync Attempt、Inspect/CAS、DomainResult和Cleanup/Residual。Connector、Parser、Indexer、Provider只产生Observation/Receipt；Review只给Verdict；Runtime只给opaque Settlement。

## 2. Sync流水线

```text
reserved -> acquired -> parsed -> normalized -> validated
         -> indexed(partial|complete) -> snapshot_ready -> published
```

`KnowledgeSyncAttemptV1`绑定Source exact ref、Connector/Parser/Normalizer/Indexer版本、Asset/Content digest、Authority/Policy/License、Operation/Attempt/Permit、阶段Observation、Coverage、Residual和TTL。远程Acquire/Parse/Index属于Effect；Begin后Unknown只Inspect原Attempt。

阶段失败不得替换current Snapshot。新Snapshot先比较Source增删、Schema变化、冲突、权限和索引完整性；旧Run继续使用已绑定Snapshot，新Run通过显式Plan/View选择新Pointer。

## 3. Record、冲突与撤回

KnowledgeRecord绑定Package/Snapshot/Source/Evidence、License、Trust、Conflict Group、valid time和transaction time。Correction/Supersede创建新Revision；多个冲突来源并存。Source Withdraw使依赖Record/Graph关系重新计算Trust，新View不得返回withdrawn事实；历史Run只能通过Evidence读取当时exact版本。Purge、remote reindex和撤回传播是Effect并报告Residual。

## 4. Projection、View与Hybrid Retrieval

Skill/Lexical/Vector/Graph Projection均可由Source/Record/Snapshot重建。Vector Entry绑定Record/Snapshot/Package/Source/Chunk/Model/Dimension/Index Version/Content Digest；Graph Node/Edge绑定Source、License、Trust、Conflict和双时间。

Scope闭集：`personal_source`、`project_source`、`team_source`、`domain_source`、`organization_source`、`external_source`。View显式绑定Snapshot、Authority、Scope、Purpose、License、Sensitivity、Disclosure Mode、允许索引和预算。权限/License过滤发生在正文展开前，多路结果经来源聚合、冲突展开、去重、rerank、预算和Cursor形成Retrieval Result；Citation必须回到Source/Asset exact ref。

## 5. 安全与开发者入口

外部文本始终不可信，不能携带系统权限。写入和查询复读Sensitivity、PII/Secret decision、License、Source Trust、Retention和Legal Hold exact refs；Secret明文不得进入Embedding、Index、日志或共享Snapshot。

Go SDK提供RegisterSource、Sync、InspectSource、Build/PublishSnapshot、Submit/Admit/Commit/Correct/Withdraw、Query、Watch和Reindex。CLI/API只调用版本化Port；异步Index任务返回Attempt/Job exact ref，Unknown时Inspect。自定义Connector/Retriever/Indexer/Store必须通过conformance。
