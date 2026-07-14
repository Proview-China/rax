# 事件、事实可信度与Write-ahead Evidence

## 1. 事实层级

| 层级 | 说明 |
|---|---|
| `Provenance` | 证明谁发送了什么，不证明内容真实 |
| `Observation` | 某组件声称观察到了什么 |
| `Attestation` | 某主体用明确方法、范围、摘要和TTL作出保证 |
| `AuthoritativeFact` | 领域所有者确认，或经独立Inspect形成的权威事实 |

Harness签名Ready、Cleaned或EffectFailed仍只是Observation。Ready需要Sandbox/Binding独立Inspect；Cleanup需要资源和远程状态权威事实；Effect结算需要Gateway、Provider Receipt或独立查询。

## 2. Source与Ledger

Provider产生`SourceEvent`并拥有自己的source sequence；Runtime验证后写入`RuntimeLedgerRecord`。只有权威Ledger为指定scope分配ledger sequence。

```text
event_id
ledger_scope / ledger_sequence
source_id / source_epoch
source_event_id / source_sequence
causation_id / correlation_id
observed_at / ingested_at
payload_digest / evidence_ref
```

全序只在单Instance聚合或单Run scope内承诺；跨Instance、跨组件只保证因果偏序。Source到Runtime按至少一次处理，以Source ID、epoch和event ID去重。Source sequence缺口形成显式`EventGap`。

## 3. Source认证

- Source Identity绑定组件、Instance、Lease和epoch；
- 使用认证通道或签名提交；
- Runtime验证身份、epoch、sequence、Schema和Payload摘要；
- 旧epoch事件只能成为迟到/安全证据，不能改变当前状态；
- Payload受大小、深度、速率和Schema限制；
- 第三方Observer只能提交Observation，不能分配权威sequence、修改历史或决定终态。

## 4. Restricted Evidence与Projection

`RestrictedEvidence`保存受策略控制的密文或对象引用；`OperationalProjection`只保存控制和诊断所需的脱敏字段。“保留原生事件”只表示保留来源、类型、摘要和可获准的Restricted内容，不表示永久保存Prompt、Secret、PII、模型输出或工具结果正文。

记录至少包含：

```text
payload_digest
ciphertext_ref
classification
redaction_policy_version
source_identity / signature
retention_until
previous_record_digest
```

Secret高置信正文默认不保存。按政策删除Payload或密钥后，可以保留允许的Digest、分类和Deletion Tombstone。链式摘要或等价Checkpoint用于验证顺序完整性，但不能恢复已删除正文。

## 5. Write-ahead Evidence

以下操作在权威Intent持久化失败时不得派发：模型/网络数据外发、计费、资源分配、Tool/MCP、Provider Continuation、Provider Cache创建/写入/删除/远程独立查询、Credential操作、Artifact/Memory Commit。本地纯Cache查找不产生外部Effect，但仍要求Authority/Partition重验和CacheAccessEvidence；模型请求内的Provider Cache读取由父EffectIntent覆盖。

Command接受、Desired State revision和command intent必须形成一个逻辑提交。Ledger展示投影可以通过持久Outbox延迟生成，但权威Intent不能只存在内存。

证据面不可用时：

- 纯本地无Effect计算可以继续；
- 新Command不能标记Accepted；
- 新外部Effect返回`evidence_unavailable`；
- 已持久化Effect继续结算；
- 本地紧急Fence/Kill可先执行并通过Emergency Journal补证；
- 无法记录紧急动作时标记`evidence_gap`，不声称审计完整。

## 6. 来源冲突

Observation与权威事实冲突产生`EvidenceConflict`。安全相关冲突进入Fenced/Indeterminate；仅Optional且Plan明确允许时可以Degraded。不得因为Source有签名就静默采信。

## 7. 最低反例

- `EVID-01`：Harness签名Ready但Sandbox Inspect无进程，不能Ready；
- `EVID-02`：Harness报告EffectFailed但目标文件存在，必须进入冲突/已发生判断；
- `EVID-03`：权威Intent Store不可用时，模型请求不得外发；
- `EVID-04`：Ledger投影暂不可用但Intent/Outbox已持久化，可以派发并稍后投影；
- `EVID-05`：第三方Observer伪造Source ID，认证或签名必须拒绝；
- `EVID-06`：原生事件含Secret，高置信正文不得进入普通Ledger Payload；
- `EVID-07`：旧epoch的合法签名事件不能改变新Instance状态。
