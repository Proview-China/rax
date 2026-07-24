# SnapshotArtifactOwnerV2第三短审纠偏

- 时间：2026-07-16 21:39 CST
- 状态：`third-review-candidate / implementation-NO-GO`
- 范围：仅Sandbox design/plan/memory；未写Go、未stage/commit。

## 本轮纠偏真值

1. canonical固定为stable Subject/ID→payload body digest/FactRef→Entry body digest/EntryRef→
   Envelope body digest/EnvelopeRef。每一层排除own Ref/Digest；TTL属于body，篡改TTL后旧digest
   必须拒绝。StorageArtifactRef是backend-neutral存储对象坐标，SnapshotArtifactFactRef才是跨Owner
   可消费的Owner事实引用，二者禁止互换。
2. `NoActiveLegalHoldCurrentProjectionV2`绑定HoldIndex ID/generation、穷尽hold set digest/count、
   coverage、jurisdiction、hold-kind与query watermark。S1/S2 subject+coverage exact；S2 generation、
   watermark、Owner revision单调且仍NoActive，不要求revision/digest相等。该Projection由Retention
   Owner按natural Checked/Expires seal，Reader/body/digest不含caller RequestedNotAfter；不同caller
   bound复用同一exact ref，Sandbox只在Deletion Attempt/Aggregate最终min应用bound。
3. AggregateCurrent按`reserved|available|deletion_in_progress|deleted|indeterminate`选择active TTL
   闭集。Reservation与历史Fact retention不永久杀current；deleted/indeterminate由不可回退terminal
   tombstone/current index维持，读取TTL过期只表示current unavailable，不表示absent或复活。
4. 首切片严格只有Reserve/Inspect。未来跨Owner写入只能另审typed governed command；seal、Entry/
   Envelope、CurrentIndex CAS committer始终位于Owner实现包内且不可导出/注入，raw Apply/CAS永不公开。
5. Sandbox-owned对象使用Owner clock与显式RequestedNotAfter，Owner只取min；S1 pre-read、S2 post-read、
   CAS时所有active ref仍fresh。clock rollback/uncertain、post drift及`now == expires`全部fail closed。
6. DeletionAttemptState与ArtifactAggregateState分离；aggregate one-active key与Attempt stable key分离。
   unknown保持active，failed关闭后才可用全新Operation/Effect/Attempt key重试；confirmed/
   indeterminate terminal。
7. NotFound恢复分三类：Owner Reservation linearizable absent仅同request replay；CAS lost reply Inspect
   expected successor/current winner；Provider Unknown/NotFound只Inspect原provider Attempt，不重派。

## 当前裁决

- Sandbox第三短审P0/P1语义已在候选资产中闭合，可再次提交短审。
- 仍需Retention/Legal Hold Owner确认negative proof/Reader公共形状，Runtime确认Deletion治理链与
  future governed command边界，管理线确认canonical/clock/current-index DTO。
- 上述联合YES前，`SnapshotArtifactOwnerV2`实现、Provider、Runtime/Continuity写、production
  Backend/root保持NO-GO与调用数0。

## 资产轻门

- 第三短审及caller-bound增量旧语义残留、required语义扫描：PASS。
- `git diff --check`：PASS。
- 本轮live资产尾随空白、冲突标记：均无。
- `xmllint --noout .properties.rax/design/sandbox/architecture.drawio`：PASS。
- staged状态：无。因本轮未写Go，未运行Go测试。
