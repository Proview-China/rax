# Execution Fence、Secret与安全替换

## 1. ExecutionFence

唯一Fence/Authority事实所有者发布当前epoch；所有最终效果边界只是强制执行点，必须校验：

```text
agent_identity_id / identity_epoch
instance_lineage_id
instance_id / instance_epoch
sandbox_lease_id / lease_epoch
authority_epoch
capability_grant_digest
effect_intent_id / effect_intent_revision
canonical_payload_digest
action_ref（仅Tool/MCP等Action类Effect可选）
expires_at
```

事件中的instance epoch只能阻止旧事件覆盖状态，不能阻止旧进程或远程Provider继续产生副作用。文件、网络、模型Context外发、Tool/MCP、Secret代理、Cache、Artifact和Memory正式提交都必须绑定Fence。

## 2. RevocationPolicy

```text
risk_class
validation_mode: online_strict | leased_offline
max_revocation_lag
max_clock_skew
token_scope
conflict_effect_domain
fail_closed_behavior
```

- 未显式配置撤销延迟和时钟边界时禁用离线Effect；
- 高风险和正式提交默认要求`online_strict`，具体Risk Policy由用户审核；
- 离线Token只能执行分区前已经线性化的有限Effect Intent；
- 旧epoch在控制事实中立即撤销，但离线执行面的生效只能承诺不超过策略上限；
- 参数值在具体Deployment/Profile/Security Policy审核时确认，Runtime不得写死默认值。

## 3. ConflictEffectDomain（冲突效果域）

至少包括：

- 工作区写入；
- 外部账号/Provider操作；
- Memory正式写命名空间；
- Artifact发布命名空间；
- Tool/MCP资源；
- Budget账户；
- 组织单例职责。

同一冲突域的旧Fence尚未确认失效时，新Instance不能取得该域能力。

## 4. Secret能力分级

| 类型 | 保证 |
|---|---|
| `SecretRef` | Harness不见明文，只保存引用 |
| `BrokeredCapability` | 请求由代理执行或签名，可在线撤销 |
| `ShortLivedCredential` | 可能暴露明文，撤销延迟受TTL策略约束 |
| `ExposedPlaintextSecret` | 无法证明从进程内存或认知中消失 |

V1受治理Harness禁止接收长期、可重用明文Secret。卸载文件只证明访问路径撤销，不证明进程失忆。终止报告分别表达：

```text
secret_access_path_revoked
credential_revoked_at_source
credential_expired
secret_mount_removed
process_memory_erasure_unverifiable
plaintext_exposure_recorded
```

## 5. ReplacementPermit

新Instance可以先进入`provisioning + quarantined`，但取得冲突能力前必须满足：

1. 所有效果边界确认旧Fence，或离线Token已超过策略最大撤销延迟；
2. 旧Lease网络已切断或网络边界按epoch拒绝；
3. Brokered Secret已撤销；
4. 短期Credential已过期或Provider确认撤销；
5. 已暴露长期明文完成Provider侧轮换，否则禁止自动替换；
6. 旧Provider Operation已结算，或相关Effect Domain继续Quarantine；
7. Memory、Artifact和Budget边界已切换到新epoch；
8. Sandbox和效果网关提供隔离Attestation。

不受控直连网络加长期明文凭据的Harness不能自动安全替换，也不满足受治理Harness准入。

## 6. 失败结果与反例

```text
fence_stale
offline_revocation_policy_missing
secret_plaintext_exposure_unbounded
replacement_permit_blocked
conflict_effect_domain_busy
network_isolation_unproven
```

- `FENCE-01`：旧Token尚在策略有效期，新Instance不得取得同一工作区写域；
- `SECRET-01`：Harness已读长期Key，卸载文件不得报告Secret已撤销；
- `REPLACE-01`：旧付款Operation未知，新Instance只能获得不冲突能力；
- `REPLACE-02`：旧Lease失联但网络隔离无法证明，不能签发ReplacementPermit；
- `FENCE-02`：旧Instance重连提交文件写入，最终边界必须因epoch拒绝。
- `FENCE-04`：Invoker与Sandbox看到不同Fence epoch时，不得各自生成权威结论；旧epoch侧拒绝，冲突进入EvidenceConflict。
