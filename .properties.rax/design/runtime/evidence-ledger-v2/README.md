# Runtime Evidence Ledger V2设计

## 1. 定位与单主

Evidence Ledger V2是Runtime执行证据的唯一账本主库。只有Ledger Owner分配`ledger sequence`并形成不可变摘要链；Timeline只是对Ledger的只读投影，不能独立分配序号或双写形成第二主库。

旧`EvidencePort`、`TimelinePort`和旧`RunClaimGateway`保持legacy restricted：字段不足时不得补默认值、不得自动升级为V2权威记录，也不得与V2双写。V2调用方直接使用完整公共合同；当前未发布会伪造缺失字段的V1转V2适配器。

## 2. 信任分层

Runtime只理解封闭的`TrustClass`：`observation`、`receipt`、`attestation`、`claim`、`authoritative_fact`和`late_observation`。自定义`EventKind`和`CustomClass`必须由独立、current的`EvidenceSourcePolicyFactV2`映射到封闭TrustClass，不能用字符串自报权威等级。

Source注册成功、Binding有效、具备append capability都不等于获得claim或authoritative资格：

- claim必须由Policy同时授权EventKind、CustomClass与封闭ClaimKind；
- authoritative fact必须由Policy授权FactKind/Owner，并由Gateway调用已注册Owner Fact Inspector复读精确Fact ID、revision、digest、payload schema/content digest、Scope和Authority；
- Observation、Receipt、Attestation和Claim不会自动升级为Authoritative Fact；
- 自定义Source Conformance只证明Envelope可进入独立认证候选，不授予Binding、Trust、Append、Claim、权威Fact或领域Commit资格。

## 3. Source与策略事实

`EvidenceSourceRegistrationFactV2`绑定namespaced Source ID、source epoch、Ledger/Execution Scope、CurrentScope投影水位、Producer Binding、Authority、ActionScope、独立Source Policy、class/kind配置、严格gap策略、TTL与cursor。

`EvidenceSourcePolicyFactV2`由独立Policy Fact Owner提供，包含：

- Policy Owner Binding与Policy Authority；
- 精确Policy Scope与ActionScope；
- 允许的分区、EventKind、class到TrustClass映射；
- claim kind映射与authoritative owner规则；
- late策略、source epoch策略和每次续租窗口上限。

Policy内部集合必须有序且唯一；claim映射必须同时引用allowed kind与claim class，owner规则必须同时引用allowed kind与authoritative class。非法或自相矛盾的Policy不能生成canonical digest。

24×7续租允许同一身份下的治理水位单调前进：CurrentScope、Producer BindingSet revision、Authority revision和Policy revision可在Gateway精确复读后推进；ExecutionScope、LedgerScope、Source ID/epoch、ActionScope、能力、组件/manifest/artifact及class/kind配置不可借续租偷换。Policy的TTL限制作用于每次租期窗口，不限制Source总寿命。

## 4. 原子Append、摘要链与恢复

Source cursor推进与Ledger Record落账属于同一Ledger Fact Owner原子提交：

1. 先按完整source key检查已有Record；
2. 同内容重放返回原Record，即使cursor已前进；
3. 同source sequence换任一Scope、Trust、Kind、因果、payload、Producer或Authority字段都返回EvidenceConflict；
4. 不存在时才检查expected source revision、next sequence和gap policy；
5. 一次提交分配ledger sequence、写`PreviousRecordDigest/RecordDigest`并推进cursor。

Append回包丢失后只按完整source key Inspect。`seq1`已提交但回包丢失、随后`seq2`成功时，`seq1`重放仍返回原Record。高位epoch/sequence使用无歧义十进制键，不使用Unicode rune编码。多Source可并发，但同Ledger序号与摘要链保持单一线性顺序。

Record永不原地改写。Retention使用独立append-only Tombstone Fact，保留Record ref、digest、source key与因果链，不破坏游标和后续PreviousRecordDigest。

## 5. Scope与当前性

Gateway在Register、Renew和每次Append前重新读取Producer Binding、Source Authority、CurrentScope、Source Policy、Policy Owner Binding、Policy Authority，以及对应Run/Effect事实。所有ref、revision、digest、epoch、ProjectionWatermark、capability、state和TTL必须精确匹配。

当前实现的保守能力限制是：所有Evidence分区都要求CurrentScope带有active `running|stopping` Run。因此tenant、identity、lineage和instance分区目前仍是“Run内证据分区”，尚不支持pre-run写入。该限制是live capability caveat，不代表最终领域模型。

上述事实由不同Fact Owner提供，Gateway复读与Ledger append之间不宣称跨Store原子事务。Ledger append是证据落账的线性化点，Record固化当时精确水位；并发撤销依靠每次复读、短TTL和后续reconcile/Tombstone处理，既有Record不会被删除或重写。

## 6. Late与Run Claim V2

旧epoch只能通过独立`AppendLateGoverned`写为`late_observation`。当前ingest Source仍须通过完整治理；Historical Source必须精确引用已有V2 Record，source epoch严格更旧，payload digest、record ref和candidate digest完全一致。Late不能写入Run/Effect分区，不能携带OwnerFact，也不能成为Run Claim。

`RunClaimGatewayV2`按以下顺序工作：

1. 复读running/stopping Run及稳定Run identity；
2. Append V2 claim，丢回包按source key Inspect；
3. 再次复读Run，允许running到stopping的单调推进，拒绝terminal或identity漂移；
4. 创建create-once `RunClaimAssociationFactV2`，精确绑定完整EvidenceRecordRef、candidate/payload digest、source坐标、event、claim kind、Scope和Run revision watermark；
5. Association回包丢失后Inspect侧车，再Inspect exact Ledger Record核验；不同Claim并发时仅一个Association线性化，冲突Claim Evidence继续保留。

Claim只是受治理证据与Run的持久关联，不完成Run，不生成ExecutionOutcome，也不回写旧`AgentRunRecord.CompletionClaim`。P0.5 Settlement必须重新Inspect精确V2 Evidence与Association后才能决定是否请求CompleteRun。

## 7. 当前不做

- 不选择生产数据库、分布式共识、RPC、签名算法、retention后端或SLA；
- 不让Timeline拥有第二写入口或第二sequence；
- 不把旧Evidence/Timeline字段通过默认值伪装成V2；
- 不把Claim、Receipt、Attestation或Tombstone解释成领域成功；
- 不修改Harness或6+1组件内部实现。

## 8. 验收

当前验收覆盖source epoch唯一性、严格gap、同序换内容、丢回包Inspect、并发摘要链、bounded Watch、Tombstone、续租水位、Policy自授信任反例、RequireInstanceEpoch true/false、canonical nil/empty与fuzz，以及RunClaim append/association丢回包、running→stopping、terminal/identity drift、并发冲突、高位source key和逐字段伪造拒绝。
