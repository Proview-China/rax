# C-01 Timeline Projection Governance设计冻结

时间：2026-07-16 21:11 +08:00
状态：**设计冻结，可交Runtime Evidence Owner联合终审；Go实现NO-GO**。

## 1. live真值

- `ExecutionRuntime/continuity/domain.Timeline.Project`直接消费caller `contract.EvidenceAdmission`，把caller提供的ledger scope/sequence、Record/Payload digest、Trust及`AdmittedByLedger/InspectedByOwner`转换为Event后调用`PutProjection`。
- live Continuity尚无独立Runtime Evidence exact/current Reader、领域Owner Fact current Reader、Projection Attempt create/Inspect/CAS或production current projection。
- 因此Wave 1 Timeline只能作为reference实现；caller candidate绝不能冒充production current。该路径构成明确production P0=1，本文不以资产措辞掩盖，也未修改Go。
- Runtime Checkpoint-first V2参考纵切已经完成第三次独立代码终审YES（P0/P1/P2=0），包含Attempt+Barrier、EffectCut、Checkpoint Evidence V1、Settlement V5、Consistency/Finalization及ManifestSeal Reader；其Conformance仍固定`ProviderCalls=0`、`ProductionClaimEligible=false`。Continuity Manifest/Seal V2也已完成最终独立代码复审YES。跨Owner Checkpoint、production root、Provider与Restore仍NO-GO。

## 2. C-01冻结裁决

1. caller只提交`TimelineProjectionRequestV1`请求坐标与期望值；不能提交可信sequence、Record/candidate/chain/payload digest、Trust、Owner current或production current。
2. Continuity create-once `TimelineProjectionAttemptFactV1`；Attempt只保存Owner exact refs/密封S1/S2 projections、共同`checked_at/not_after`、状态与结果Event ref，不保存caller `EvidenceAdmission`副本。
3. Evidence S1/S2使用同一Runtime Reader binding：按Source Key定位、按Reader返回的exact Record Ref复读；ledger scope/sequence、Record/candidate/chain/payload digest、Trust、scope与时间全部从Reader结果派生。R-CTY-06同时密封source/policy/current、readability、exact Tombstone或Owner-defined absence watermark及subject-current index；空Tombstone ref不等于未删除。Tombstone/已接纳Retention binding/source-policy mutation必须与current index/watermark在Runtime Evidence Owner同一线性化边界原子推进。S1/S2复读同一immutable sealed projection，Checked/Expires/ProjectionDigest逐字段exact一致；fresh now只执行`ValidateCurrent(expected ProjectionRef)`，不能每次读取重封。旧ProjectionRef历史仍可Inspect，但index漂移后即使未过Expires也必须current失败。
4. Runtime六类Trust逐项闭合：`observation|late_observation|receipt|attestation|claim`原样保持Ledger Trust进入Timeline，禁止generic OwnerFact升级；只有`authoritative_fact`允许且必须通过按Fact kind装配的领域Owner Fact current Reader完成immutable sealed S1/S2。领域若要为attestation/claim另做current证明，只能发布独立typed Delta/Fact projection并保持原Ledger TrustClass。
5. `RequestedNotAfter == 0`表示caller不加上限，`<0`非法，`>0`只能缩短。Owner Readers不接收该值，也不得据此重封Expires/ProjectionDigest；Continuity在S1/S2 exact及fresh-now ValidateCurrent通过后，先取Projection Policy、Runtime Evidence source/policy/readability和仅authoritative_fact的领域Owner current/Authority/Scope/Binding全部自然上限的最小值，最后才按正数请求截短。缺失Owner上限、`now >= not_after`、时钟回拨或S1/S2漂移均Fail Closed；Cursor TTL、Event时间、Retention window与默认值不得伪造TTL。
6. Attempt状态为`proposed -> inspecting -> admitted -> visible`，可恢复状态为`reconcile_required`，失败闭集为`rejected | expired | indeterminate`。`visible`必须与Event/current索引原子提交；无法证明原子性时保持不可见并只Inspect原Attempt。
7. Create/CAS/commit回包丢失只Inspect原Attempt exact ref；CAS绑定完整previous Fact digest，current已前进后旧CAS必须Conflict，禁止ABA、换ID或盲重试。
8. closed errors只允许`invalid_argument | not_found | conflict | precondition_failed | unavailable | indeterminate | unsupported`。typed-nil、timeout、无coverage或无法区分absent/unknown不能映射NotFound。

## 3. 最小跨Owner Delta

- 复用Runtime `EvidenceSourceRecordReaderV2`的`InspectRecord/InspectBySource`作为immutable Record exact双读，不新增第二Record DTO或写权。
- 提交`R-CTY-06` additive只读候选给Runtime Evidence Owner：输入exact Record/Source请求坐标、expected Execution Scope、source registration/policy refs与reader binding，明确不接收`RequestedNotAfter`；输出natural immutable sealed Evidence current/readability projection，包含Record/Source、source registration revision/config digest、policy ref/revision/digest、Ledger/Execution Scope、六值Trust、payload、tombstone/readability、Authority/Scope/Binding水位及稳定Checked/Expires/ProjectionDigest。
- R-CTY-06不扩`EvidenceGovernancePortV2`写面，不触发pre-run Evidence，不修改Evidence V2/V3 closed schema；最终名称/version由Runtime Evidence Owner裁决。
- Application/Assembler须按`owner contract/schema/fact kind`绑定领域Owner发布的current Reader；不得提供caller可注入的通用Hook或通用Fact Store。

## 4. 审计结论

| 等级 | 数量 | 结论 |
|---|---:|---|
| P0 | 1 | live caller-driven `Timeline.Project -> PutProjection`可被误作production；须在联合终审后的获批Go波次隔离或替换 |
| P1 | 2 | Runtime Evidence Owner终审R-CTY-06；Assembler/各Fact Owner冻结typed Owner Fact current Reader routing |
| P2 | 0 | 本轮未发现影响设计冻结的文字一致性问题；实际轻门全部PASS |

联合审新增的Trust六值路由P0、RequestedNotAfter/sealed projection/reconcile及Owner current index原子线性化P1已在资产合同、状态机、验收和计划中闭合；它们不消除live caller-driven Project的production P0，也不解除Runtime typed Readers等外部P1依赖。

当前结论：**C-01设计可交联合Review，代码不可实现/不可生产装配**。本轮未写`ExecutionRuntime/continuity`、Runtime、Harness、Application、Restore、production Backend/root，也未stage/commit。

## 5. 资产校验

- Markdown relative links：PASS；
- Markdown fenced blocks：PASS；
- `architecture.drawio` XML：PASS（本轮未修改该图）；
- trailing whitespace：PASS；
- stale词扫描：PASS，未发现旧Runtime候选/P1、旧Timeline Candidate V1、旧caller deadline/expected-not-after或错误Trust聚合词；
- required terms：PASS，六类Trust、RequestedNotAfter、reconcile_required、ProjectionDigest、subject-current、ValidateCurrent、absence watermark均存在于合同/Port/验收闭包；
- untracked `git diff --no-index --check`：8个本轮资产全部PASS；
- 写入范围：仅5个Continuity design文件、1个plan、1个module与本memory；`ExecutionRuntime/continuity`最近写入扫描为空。本轮未运行Go测试，因为未修改Go。
