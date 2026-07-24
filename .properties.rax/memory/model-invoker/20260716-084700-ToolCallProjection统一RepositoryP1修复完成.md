# 2026-07-16 08:47 ToolCall Projection统一Repository P1修复完成

## 事件

独立代码Review发现Direct可分别注入Publisher与Reader，lost-reply恢复可能错误采信另一Store的`AuthoritativeNotFound`。Model Invoker独占完成P1修复：生产侧改为单一`ToolCallCandidateObservationProjectionRepositoryV1` capability，Direct配置、session与publish-before-emit屏障只能使用同一个Repository实例。未修改Application、Harness、Tool、Context、Runtime、根配置或生产root，未stage、未commit，未调用真实API或订阅。

## 落地产物

- 公共Repository capability嵌入Publisher与Reader，但下游消费者仍只获得Reader窄接口；
- Direct `Config`删除可分别配对的Publisher/Reader字段，只保留一个Repository字段；
- sync/stream统一通过同一Repository执行`Publish -> exact Inspect -> 必要时同canonical重投`；
- lost-reply后的`AuthoritativeNotFound`不能再来自另一个可单独注入的Store；
- fault测试改为单Repository包装同一Store，并保留same-store lost-reply、authoritative absence、unknown absence和暂不可见反例；
- 配置Conformance确认跨Store Publisher/Reader组合不可表达；自定义Repository若内部拆分线性化域，属于Port合同违规，不能声称`linearizable_never_created`。

## 边界

Repository是Model生产侧的一致性能力，不是向Application、Harness或Tool授予写权的通用接口；Projection仍不是PendingAction、ActionCandidate、Evidence、Settlement或执行授权。本修复不新增生产持久化driver、Continuity Adapter、production composition root、Retention SLA或transport exactly-once承诺。

## 验证

实际通过：

```text
go test ./tests/toolcallobservation ./tests/executiondirect -count=100
go test -race ./tests/toolcallobservation ./tests/executiondirect -count=20
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
git diff --check -- ExecutionRuntime/model-invoker .properties.rax/design/model-invoker .properties.rax/module/model-invoker .properties.rax/memory/model-invoker
```

## 索引

- [主设计](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1.md)
- [测试矩阵](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1-test-matrix.md)
- [模块说明](../../module/model-invoker/tool-call-candidate-observation-v1.md)
