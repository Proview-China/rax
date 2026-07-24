# 2026-07-16 07:55 ToolCall Projection发布读取Delta进入计划

## 事件

基于live代码复核，确认`ToolCallCandidateObservationProjectionV1/RefV1`、strict codec、sync/stream finalizer与direct事件输出已经存在，但Projection Store、Publisher与Exact Reader不存在。G6A文档要求按source coordinate Inspect完整Model Observation，因此新增additive Publish/Exact Reader V1设计与计划，等待中央联合评审。

## 计划边界

- Model Invoker定义create-once Publisher、完整Ref Exact Reader、线程安全内存Fake和Conformance；
- direct必须在Projection已Publish并可exact Inspect后才发权威Observation与兼容Call；
- lost publish reply先Inspect原Ref；只有Owner/Repository完成线性化检查并明确返回非Retention、非eventual unknown的`authoritative NotFound`，且本地Projection/Ref/source/canonical内容仍exact，才允许重投同一Publish；其他NotFound、不可读或未知状态保持unknown；
- Projection保持immutable historical truth，不添加领域TTL；G6A current窗口由Application InputCurrentReader取Retention与其他Owner current上界的最小值；
- Application、Harness、Tool只依赖Reader，不持有Publisher；
- `N>1`、PendingAction/Action、Evidence、Settlement、生产Backend和composition root均不在本Delta。

Continuity Owner只读评估确认：reference backend已有同步create-once、Evidence/source冲突、clone、typed digest和Retention state基础；但没有publish-before-emit fence、Projection ID/source索引、完整正文读取、strict Model decode、exact retention current或生产RocksDB driver。未来Continuity Adapter只能opaque映射Model Ref/source到Evidence/Content Object，读取后由Model codec重验；Cursor、RecordedAt和Retention state不能伪造Projection TTL/current。production Adapter/root登记为G6B/后续残余，不阻塞G6A内存reference implementation/test fixture。

中央审读补充的恢复边界只承诺create-once canonical publish，不承诺transport exactly-once。Publisher成功后Reader暂不可见不能直接算`authoritative NotFound`；Backend必须按自身一致性合同返回ordinary/unknown NotFound、Unavailable或Indeterminate，防止错误重投。

## 当前状态

当前仅完成资产设计，不修改代码，不写完成事件。中央联合`YES`后才能按计划进入Model Invoker独占实现与测试。

## 索引

- [主设计](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1.md)
- [测试矩阵](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1-test-matrix.md)
- [实施计划](../../plan/model-invoker/tool-call-observation-projection-publish-reader-v1.md)
