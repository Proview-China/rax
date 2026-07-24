# 2026-07-16 08:28 ToolCall Projection发布读取与Direct屏障完成

## 事件

在Model Projection Publish/Exact Reader资产获得中央复核与独立Review `YES`后，Model Invoker独占完成公共Publisher/Reader、线程安全内存reference store和direct publish-before-emit屏障。未修改Application、Harness、Tool、Context、Runtime、根配置或生产root，未调用真实API或订阅。

## 落地产物

- 新增`ToolCallCandidateObservationProjectionPublisherV1`与`ToolCallCandidateObservationProjectionReaderV1`窄接口；
- typed分类区分Invalid、Conflict、AuthoritativeNotFound、UnknownNotFound、RetentionUnreadable、Unavailable与Indeterminate；AuthoritativeNotFound必须携带`linearizable_never_created`证明；
- 内存reference store以Projection ID和`InvocationID + InvocationDigest + ResponseID + SourceSequence`双索引原子create-once，支持相同canonical内容幂等、冲突Fail Closed、strict重验与deep clone；
- direct sync/stream统一执行`seal -> Publish -> exact Inspect -> authoritative union event -> compatibility-only calls`；Publisher/Reader缺失或屏障未闭合时零权威Observation、零兼容Call；
- lost reply先Inspect原Ref；exact命中继续，只有线性化证明从未写入时才重投同一sealed canonical Projection一次；ordinary/unknown NotFound、Retention不可读、暂不可见、Unavailable或Indeterminate均不重投；
- N>1保持一个完整Projection和连续ordinal，Publisher/Reader不创建PendingAction、ActionCandidate、Evidence或Settlement；
- reference store统计transport Publish调用数与Repository记录数，明确不承诺transport exactly-once，只承诺create-once canonical publish。

## 验证

实际通过：

```text
go test ./tests/toolcallobservation ./tests/executiondirect -count=100
go test -race ./tests/toolcallobservation ./tests/executiondirect -count=20
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
git diff --check -- ExecutionRuntime/model-invoker .properties.rax/module/model-invoker .properties.rax/memory/model-invoker
```

测试覆盖unit、外部黑盒、合法同ID/source冲突构造、lost-reply、线性化未写入恢复、普通未知缺失、成功后暂不可见、重复丢包两次上限、sync/stream、N>1、64并发clone/no-alias和Publisher/Reader capability分离。

## 保留边界

当前没有生产持久化driver、Continuity Adapter、production composition root、Retention SLA、真实进程crash恢复或跨进程事件exactly-once。未来生产Adapter只能实现Model公共Port，opaque保存完整Ref/source与wire正文，并在读取后交回Model strict codec重验；Continuity不能成为Model identity/current Owner。

## 索引

- [模块说明](../../module/model-invoker/tool-call-candidate-observation-v1.md)
- [主设计](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1.md)
- [测试矩阵](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1-test-matrix.md)
- [实施计划](../../plan/model-invoker/tool-call-observation-projection-publish-reader-v1.md)
