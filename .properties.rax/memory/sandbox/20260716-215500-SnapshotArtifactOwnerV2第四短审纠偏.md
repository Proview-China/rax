# SnapshotArtifactOwnerV2第四短审纠偏

- 时间：2026-07-16 21:55 CST
- 状态：`fourth-review-candidate / implementation-NO-GO`
- 范围：仅Sandbox design/plan/memory；未回改旧memory，未写Go、未stage/commit。

## 本轮真值

1. `NoActiveLegalHoldCurrentProjectionV2`绑定完整HoldIndex exact ref：type URL、version、ID、
   generation、revision、digest domain、digest与TTL；强制
   `QueryWatermark.IndexGeneration == HoldIndexExactRef.Generation`。
2. S1→S2按`(generation,sequence)`词典序非递减。跨generation必须携连续的
   `CoverageCarryProofRefs`链，每段exact绑定from/to index、水位、coverage/jurisdiction/
   hold-kind/selector、revision/digest/TTL；同generation explicit absent。
3. `SnapshotArtifactAggregateCurrentIndexV2`与`SnapshotArtifactTerminalTombstoneV2`已冻结
   type URL/version/digest domain、stable ID/revision、全部presence discriminator、present exact
   ref TTL、Owner时间字段及canonical body。两者都排除own Ref/Digest；terminal state/lineage不可回退。
4. `SnapshotStorageArtifactRefV2`不再使用“等字段”描述，而是完整exact DTO：固定type URL/
   version/revision/digest algorithm/domain，canonical覆盖tenant/domain/namespace/content/schema/
   length/encryption/residency/Created/Expires并禁止raw locator、Provider Receipt与Credential。
   `SnapshotArtifactFactRefV2`使用独立type URL与digest domain，禁止跨domain替换。
5. P1反例已补：watermark/index generation错配、same revision换digest、跨代无carry/断链/
   coverage漂移；Index/Tombstone presence或TTL篡改、自含Ref、terminal非法组合；Storage错误
   type/version/domain、revision/TTL改动复用digest、raw locator及Fact/Storage互填。

## 当前裁决

- Sandbox第四短审P0/P1候选已闭合，可提交轻门与联合Review。
- Retention/Legal Hold Owner的Index/Carry public DTO与Reader、Runtime Deletion治理链、管理线DTO
  命名/持久化策略仍为外部P0；未全部YES前实现、future command、Provider与production root均NO-GO。

## 轻门结果

- 第四短审旧语义残留：无；required canonical/index/carry/DTO/P1反例扫描：PASS。
- no-hold proof canonical caller-bound隔离：PASS。
- `git diff --check`、尾随空白、冲突标记：PASS。
- `xmllint --noout .properties.rax/design/sandbox/architecture.drawio`：PASS。
- staged：无。文档限定任务未运行Go测试。
