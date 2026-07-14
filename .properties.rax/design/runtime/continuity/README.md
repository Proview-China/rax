# Checkpoint、恢复与Cache安全分区

## 1. CheckpointSet

Checkpoint是协调一致性集合，不是多个组件文件的拼接：

```text
barrier_id / checkpoint_epoch
participant manifests
plan/profile/context digests
authority_epoch
effect_intent_accept_watermark
effect_dispatch_watermark
effect_settlement_watermark
remote_continuation_watermark
action watermark（仅Tool/MCP等Effect的可选子维度）
event/source watermarks
consistency_status
```

流程：阻止新Run和新EffectIntent；固定已接受Intent集合；对每个Intent记录`not_dispatched/dispatched/settled/unknown`及Provider Operation；Fence或排空可控在途Effect；发出统一Barrier；收集参与者SnapshotRef和上述四类Effect水位；确认所有不在Checkpoint恢复能力内的Intent已经settled或被显式标记unknown/remote；最后判断一致性。状态为`consistent`、`partial`、`indeterminate`或`rejected`。

一致切面覆盖模型调用、Sandbox资源、网络、Tool/MCP、Hosted Tool、Cache、Credential、Formal Commit与Safety Effect，不能用Action水位代替。Action水位若存在，只是统一Effect集合中的索引。

只有`consistent`允许自动恢复。若V1不实现Checkpoint，则Capability必须明确为unsupported，不能保留模糊承诺。

## 2. 恢复

恢复创建新的Instance ID和更高epoch，并重新验证Identity、Plan/Profile、Route/Model/Harness、Context、Authority、Capability、当前Secret引用、组件版本、Effect Intent/Dispatch/Settlement/Remote与Event Watermark。Secret正文不进入Checkpoint。

恢复采用：

```text
Stage all participants
-> Validate compatibility and Fence
-> Activate together
```

任一Required参与者失败时不得半恢复Ready。Partial Checkpoint只能用于诊断或人工处理。

## 3. Cache Partition Key

至少绑定：

```text
tenant_id
principal_id
authority_epoch / authorization_scope_digest
data_classification / retention_policy_digest
ordered_context_content_digest
context_source_versions / prompt_asset_digests
provider / deployment / region
model_id / model_revision
harness_id / harness_digest
route_fingerprint
visible_tool_schema_digest
provider_cache_namespace/state
cache_schema_version
```

命中时重新校验Authority、用途、数据分级、Route/Harness/Tool面、TTL、来源版本和内容摘要。V1禁止跨租户、跨Principal/Authority和跨Harness隐式复用。Provider隐式缓存无法证明隔离时，不允许敏感Context使用该Route缓存。

## 4. Cache作为Effect

Provider Cache创建、写入、删除、延长Retention或独立远程查询属于数据披露/费用/Provider状态Effect，必须有Intent、Authorization、Fence、Retention和Receipt/UnknownOutcome。模型请求内部的Provider Cache读取由父模型Intent覆盖；无外发无费用的本地查找无需独立Runtime Intent，但必须先校验Partition/Authority并记录CacheAccessEvidence。命中返回前必须重鉴权。Runtime不拥有缓存算法；Context Cache Manager生成CachePlan，Model Profile声明Route能力，Invoker执行原生映射，Runtime只关联Fence、Placement和证据。

## 5. 失败结果与反例

```text
checkpoint_partial
checkpoint_indeterminate
restore_incompatible
restore_authority_invalid
cache_partition_mismatch
cache_authorization_stale
provider_cache_isolation_unproven
cache_retention_unknown
```

- `CKPT-01`：Memory恢复成功但Harness Session失败，Instance不得Ready；
- `CKPT-02`：旧Checkpoint包含已撤销Capability，恢复必须重验并拒绝或收紧；
- `CKPT-03`：模型Effect已dispatched但未settled，Checkpoint未记录该Intent就不得标为consistent；
- `CKPT-04`：Sandbox Effect已settled而Remote Batch仍active，恢复必须保留RemoteContinuation水位和冲突域；
- `CACHE-01`：管理员与普通用户Prompt相同，不得共享权限敏感缓存；
- `CACHE-02`：Harness摘要变化时旧缓存分区失效；
- `CACHE-03`：Provider无法证明隐式缓存隔离，敏感Context禁用缓存；
- `CACHE-04`：为了命中缓存重排有语义差异的Prompt，必须拒绝。
- `CACHE-05`：独立Provider lookup发送Key并计费却没有Intent，必须拒绝；
- `CACHE-06`：命中后Authority已缩小，内容返回前必须重新鉴权并拒绝。
