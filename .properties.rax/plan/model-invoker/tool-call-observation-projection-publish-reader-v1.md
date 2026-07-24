# ToolCall候选观测Projection发布与Exact Reader V1实施计划

## 1. 状态

- 模块：`model-invoker`
- 当前状态：陈旧计划（reference implementation、原子Ensure与typed-nil第三审修复已完成）
- 设计事实源：[主设计](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1.md)
- 测试事实源：[测试矩阵](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1-test-matrix.md)
- 实施目录：仅`ExecutionRuntime/model-invoker/**`

本计划已实施并保留为历史执行依据。该Delta是现有Observation V1的additive Port，不改变Projection Ref，不扩展`N>1`，不创建PendingAction/Action，不拥有Evidence或Settlement。

## 2. 预期产物

1. 公共`ToolCallCandidateObservationProjectionPublisherV1`；
2. 公共`ToolCallCandidateObservationProjectionReaderV1`；
3. 公共单方法原子`ToolCallCandidateObservationProjectionRepositoryV1.EnsureSealedProjectionV1`；
4. Model-owned canonical producer helper `EnsureToolCallCandidateObservationProjectionV1(ctx, repository, sealed)`：输入完整已sealed Projection，执行typed-nil拒绝、原子Ensure、同sealed最多一次恢复与exact compare；helper不负责seal；
5. create-once、双索引Conflict与exact Inspect合同；
6. 线程安全内存Fake及公共Conformance；
7. direct sync/stream ensure-before-emit屏障；
8. lost-reply、clone、并发、typed-nil、split A/B和顺序测试；
9. 只读Consumer import/capability边界检查。

不会产出生产持久化Backend、Continuity Adapter实现、RPC、Retention SLA、production composition root、Application协调器、Harness PendingAction或Tool Action。

Continuity live reference backend已有同步create-once、Evidence/source冲突、clone、typed digest与Retention state基础，但没有本计划要求的publish fence、Projection双索引、正文Exact Reader、Model strict decode、exact retention current或生产RocksDB driver。本计划只把这些事实作为未来Adapter约束，不把Continuity加入本轮候选代码切面。

## 3. 候选代码切面

实际仅在Model Invoker独占目录落地：

```text
ExecutionRuntime/model-invoker/tool_call_observation_repository.go
ExecutionRuntime/model-invoker/execution/direct/config.go
ExecutionRuntime/model-invoker/execution/direct/adapter.go
ExecutionRuntime/model-invoker/execution/direct/events.go
ExecutionRuntime/model-invoker/execution/direct/session.go
ExecutionRuntime/model-invoker/execution/direct/tool_call_projection.go
ExecutionRuntime/model-invoker/tests/toolcallobservation/repository_test.go
ExecutionRuntime/model-invoker/tests/toolcallobservation/repository_conformance_test.go
ExecutionRuntime/model-invoker/tests/executiondirect/direct_test.go
```

实际文件以live最小Delta为准，不创建Application/Harness/Tool/Continuity代码。

## 4. 实施清单

### A. 公共Port

- [x] 固定Publisher/Reader V1签名与错误分类；
- [x] 固定Repository为单方法原子`EnsureSealedProjectionV1(ctx, sealed)`；
- [x] 固定canonical producer helper输入完整sealed Projection；Direct无第二套恢复逻辑；
- [x] Publisher只接受sealed、重新Validate后的Projection；
- [x] ID索引与`Invocation+ResponseID+SourceSequence`source索引原子写入；
- [x] 相同Ref/内容幂等；同ID或source换内容Conflict；
- [x] Reader只接受完整Ref并逐字段exact校验；
- [x] Inspect重算Ref/Observation digest并返回deep clone；
- [x] 不增加Update/Delete/weak lookup/Calls[0]接口。

### B. Fake与Conformance

- [x] 实现线程安全内存Fake；
- [x] Ensure、写入与读取均防御性复制；
- [x] 32+并发same/different content线性化；
- [x] strict codec、篡改、unknown/unavailable/not-found分类；
- [x] Reader兼容面区分`authoritative NotFound`与ordinary/unknown NotFound，但Direct不读取该分类恢复；
- [x] Conformance统计transport Ensure/Publish调用次数与Repository记录数；
- [x] Consumer只读能力与import DAG Conformance；
- [x] Fake/fixture命名不得包含Production/Certified/SLA。

### C. direct屏障

- [x] direct配置只注入单方法原子Repository；Publisher/Reader不参与Direct恢复；
- [x] sync/stream在终态finalize后reserve唯一SourceSequence并seal Projection；
- [x] canonical producer helper接收完整sealed Projection并调用原子Ensure；
- [x] Ensure `Indeterminate`只以同一sealed Projection重试同一Ensure一次；
- [x] Direct不依据Inspect或`AuthoritativeNotFound`重投；
- [x] Ensure返回Validate通过但非exact Projection时Conflict；
- [x] typed-nil在New、Preflight、Open与producer helper于Backend触达前Fail Closed；
- [x] exact compare成功后才发一个权威Observation event；
- [x] 兼容Call只能位于权威事件之后；
- [x] 屏障最终为unavailable、indeterminate或conflict时零权威事件、零兼容Call；
- [x] N>1仍保存完整Observation但零部分升级；
- [x] 不触碰PendingAction、Evidence、Settlement或Runtime终态所有权。

### D. Retention/current边界

- [x] Projection/Ref保持immutable historical truth且不添加TTL；
- [x] 若后续需要Retention，必须定义独立nominal Inspection类型；本轮未伪造该能力；
- [x] 文档明确G6A current上界由Application InputCurrentReader取多Owner最小值；
- [x] Reader/Retention不可用时Fail Closed；本轮不宣称生产Retention实现。

### E. Continuity未来Adapter边界

- [x] 记录Model Ref/source只能opaque映射到Continuity Evidence/Content Object；
- [x] 记录完整wire正文读取后必须由Model strict codec与Reader规则重验；
- [x] 禁止Continuity重封Model identity、改写source或成为Model current Owner；
- [x] 禁止Cursor TTL、RecordedAt或Retention state伪造Projection TTL/current；
- [x] production RocksDB driver、Adapter和composition root登记为G6B/后续残余，不阻塞本轮内存reference实现与test fixture；
- [x] 本阶段没有创建Continuity代码或跨模块wiring。

### F. 测试与收口

- [x] 完成[测试矩阵](../../design/model-invoker/tool-call-observation-projection-publish-reader-v1-test-matrix.md)；
- [x] targeted ordinary 100轮；
- [x] targeted race 20轮；
- [x] full ordinary/race/vet/gofmt/diff；
- [x] Observation/Projection前置实现已获独立Review `YES`；原子Ensure返修以最终门禁与后续独立复审为准；
- [x] 计划已更新为陈旧计划（已完成）并写入memory完成事件。

## 5. 恢复规则

- 调用者先seal完整Projection；canonical producer helper不执行seal，任何失败都不得分配新Ref、sequence或内容；
- Direct只调用Model-owned `EnsureToolCallCandidateObservationProjectionV1`，不持有第二套恢复或exact比较逻辑；
- 首次原子Ensure返回`Indeterminate`时，helper只允许以同一sealed Projection再调用同一Ensure一次；
- Ensure成功后必须逐字段和canonical wire exact比较；合法但非exact的返回仍是`Conflict`；
- 第二次仍失败、Unavailable、Invalid、Conflict或其他分类均Fail Closed，零权威事件、零兼容Call；
- Publisher与Reader只保留兼容写口/下游窄读口，不参与Direct恢复；Reader的`AuthoritativeNotFound`不授权重投；
- 同ID/source changed content永远Conflict；
- 不承诺transport exactly-once；同一canonical Ensure最多调用两次，原子Repository最终只能保存一份；
- 生产crash恢复与持久化等待未来宿主/Continuity Adapter及composition root设计。

## 6. 联合评审与完成条件

前置设计阶段已由中央联合确认Owner、Port方向、ensure-before-emit、G6A只读依赖、TTL/current和生产边界。当前reference implementation已完成公共Port、内存Fake、Conformance、direct屏障、canonical producer单一逻辑与命令门，并保留后续独立复审入口。

G6A reference implementation/test fixture不等待生产Continuity Adapter；但任何真实启用、跨进程恢复或生产持久化声明都必须等待后续Adapter/root验收。
