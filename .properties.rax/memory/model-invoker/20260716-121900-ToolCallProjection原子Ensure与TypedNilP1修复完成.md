# 2026-07-16 12:19 ToolCall Projection原子Ensure与Typed-nil P1修复完成

## 事件

独立第三审指出上一轮“嵌入Publisher+Reader”的Repository仍可由split Store A/B wrapper伪造一致性，且Direct未拒绝typed-nil Repository。本轮仅在Model Invoker范围完成两个P1修复，并取代上一轮依赖跨方法`AuthoritativeNotFound`的Direct恢复结论。未修改Application、Harness、Tool、Context、Runtime或生产root，未stage、未commit，未调用真实API或订阅。

## 落地产物

- `ToolCallCandidateObservationProjectionRepositoryV1`收口为单方法`EnsureSealedProjectionV1`；
- 公共`EnsureToolCallCandidateObservationProjectionV1(ctx, repository, sealed)`冻结为Model-owned canonical producer合同：只接收完整已sealed Projection，不负责seal，统一typed-nil依赖检查、原子Ensure、同sealed最多一次恢复与exact compare；
- 内存Store在一个线性化点完成同key create-once或return-existing，并返回完整deep clone；
- Direct sync/stream只执行`seal -> Ensure -> exact compare -> authoritative event -> compatibility-only calls`；
- `Indeterminate`只允许以同一sealed Projection重试同一Ensure一次；Direct不再读取Reader的`AuthoritativeNotFound`决定重投；
- split Store A Publisher/Store B Reader wrapper没有Ensure方法，不能满足Direct Repository；
- typed-nil Repository在New、Preflight、Open和producer helper共用安全nil-like判定，于Backend Resolve/Invoke/OpenStream前Fail Closed；
- Direct wrapper只负责seal后委托canonical producer，没有第二套恢复或exact比较逻辑；公开路径反例覆盖Preflight/Open、producer helper，以及Ensure返回Validate通过但非exact Projection的sync与stream N>1拒绝；
- zero-value内存Store安全，N>1仍作为一个完整Projection原子Ensure，不拆分、不升级PendingAction。

## 保留边界

Publisher与Reader继续作为独立兼容/消费Port；Application、Harness与Tool仍只应获得Reader。当前没有生产持久化driver、Continuity Adapter、production composition root、Retention SLA或transport exactly-once承诺。

## 验证

实际通过：

```text
go test ./tests/toolcallobservation ./tests/executiondirect -count=100
go test -race ./tests/toolcallobservation ./tests/executiondirect -count=20
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
git diff --check -- ExecutionRuntime/model-invoker .properties.rax/design/model-invoker .properties.rax/plan/model-invoker .properties.rax/module/model-invoker .properties.rax/memory/model-invoker
```

最终静态收口还通过了`.properties.rax/plan/model-invoker`在内的`git diff --check`、Markdown相对链接检查、陈旧`待评审/Publish→Inspect恢复`语义扫描、测试目录布局检查；暂存区文件数为0。

全量首轮曾因新增白盒测试位于`execution/direct/`而触发仓库“测试必须在tests/目录”布局门；该测试文件已删除，typed-nil通过公共Direct构造/路由反例覆盖，随后full ordinary/race/vet全部通过。

## 索引

- [主设计](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1.md)
- [测试矩阵](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1-test-matrix.md)
- [模块说明](../../module/model-invoker/tool-call-candidate-observation-v1.md)
