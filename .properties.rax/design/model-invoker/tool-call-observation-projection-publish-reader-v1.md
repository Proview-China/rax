# ToolCall候选观测Projection发布与Exact Reader V1设计Delta

## 1. 状态与live缺口

- 模块：`model-invoker`
- 类型：additive前置Delta，不修改既有Observation/Projection Ref
- 当前状态：reference implementation已完成；第三审P1修复已将Direct producer收口为原子Ensure Repository
- 目标：为G6A提供可exact读取的完整Model Projection，并保证Direct只在原子create-or-return-existing成功后发权威Union事件

live已经存在：

- `ToolCallCandidateObservationV1`与sync/stream整批finalizer；
- `ToolCallCandidateObservationProjectionV1`、`ToolCallCandidateObservationRefV1`与strict codec；
- `ToolCallCandidateObservationProjectionPublisherV1`、`ReaderV1`与原子`RepositoryV1`；
- 线程安全内存reference store、ID/source双索引、strict clone与typed错误；
- direct sync/stream在终态后执行原子Ensure屏障并发出`model_tool_call_candidate_observation`；
- 逐Call `model_tool_call`已标记为compatibility-only。

live仍不存在：

- 生产持久化driver或Continuity Adapter；
- production composition root、Retention SLA或跨进程事件exactly-once；
- 把Provider Observation升级为PendingAction、ActionCandidate或执行授权的权限。

Harness `PendingAction`即使带有payload，也不能代替Model Projection的exact ref与完整Calls；Application G6A Request只携带中立Observation coordinate，不携带payload bytes。因此，必须由Model Invoker发布稳定只读合同，让Owner Adapter按exact ref取回完整Projection。

## 2. 所有权与不做事项

Model Invoker拥有：

- Projection identity、Ref、strict codec与Validate；
- Publisher/Reader公共Port语义与producer原子Ensure Repository；
- direct ensure-before-emit顺序；
- 线程安全内存Fake与公共Conformance。

本Delta不拥有或创建：

- `PendingAction`、`ActionCandidate`、Reservation、Permit或执行授权；
- Evidence、Effect、DomainResult、ToolResult或Runtime Settlement；
- G6A Application协调状态；
- Harness Session/Turn currentness；
- Continuity生产Store或宿主production composition root；
- `N>1`拆分、选择首项或批量Action。

生产持久化未来可以由Continuity adapter或宿主实现Model Invoker定义的Port，但具体Backend、SLA、Retention实现、进程拓扑和composition root当前均不存在，不能在本设计中宣称已完成。

### 2.1 Continuity reference backend只读评估

Continuity Owner对live reference backend的裁决分为两部分：

| 裁决 | live事实 | 对本Delta的含义 |
|---|---|---|
| `YES` | 已有同步create-once、Evidence/source冲突、clone、typed digest与Retention state基础 | 可作为未来Adapter和Conformance的参考语义，不需要Model重复发明通用存储概念 |
| `NO` | 无publish-before-emit fence、无Projection ID/source tuple索引、无完整payload正文读取、无strict Model wire decode、无exact retention ref/current window、无生产RocksDB driver | 当前不能把Continuity reference backend直接称为Model Publisher/Reader，更不能宣称生产Adapter存在 |

因此V1先在Model Invoker内提供线程安全内存Store/Fake与Conformance。Continuity的现有能力不阻塞G6A reference implementation/test fixture，但production adapter/root继续是G6B或更后阶段残余。

## 3. 公共Port

### 3.1 写入面

冻结候选名称：

```text
ToolCallCandidateObservationProjectionPublisherV1
  PublishSealedProjectionV1(ctx, projection)
    -> exact ToolCallCandidateObservationRefV1
```

Publisher只接受已经通过`ProjectionV1.Validate`的sealed Projection，不接收Raw Provider响应、逐Call事件或任意JSON bytes。写入前必须重新校验：

- Projection与Observation contract version；
- Invocation ID/digest；
- Observation digest；
- Ref ID/revision/digest；
- Response ID、SourceSequence；
- 全部Calls的ordinal、ID、name、canonical arguments；
- Ref与Observation之间的digest绑定。

create-once规则：

1. 全新Ref ID且全新source key：原子创建并返回原Ref；
2. 相同完整Ref、相同Projection内容：幂等返回原Ref，不创建第二份；
3. 相同Ref ID但revision、digest、Invocation、source或Observation内容变化：`Conflict`；
4. 相同source key但Ref ID或任何内容变化：`Conflict`；
5. 不提供Update、Overwrite、Upsert、Delete或换ID重试。

source key固定由`InvocationID + InvocationDigest + ResponseID + SourceSequence`组成。它只是Repository内部唯一索引，不创建新公共身份，也不改变`ToolCallCandidateObservationRefV1`。

Publisher保留为独立create-once写口和测试兼容面，但Direct producer不再使用它。Publisher与Reader的两个返回不能共同证明一个原子线性化事实；因此Publisher回包丢失时，本V1不允许Direct根据随后Reader的NotFound自动重投。

### 3.2 只读面

冻结候选名称：

```text
ToolCallCandidateObservationProjectionReaderV1
  InspectExactProjectionV1(ctx, fullRef)
    -> complete ToolCallCandidateObservationProjectionV1 clone
     | typed Inspect disposition
```

失败/缺失通道必须使用Model公共nominal分类，至少区分`Conflict`、`AuthoritativeNotFound`、`UnknownNotFound`、`RetentionUnreadable`、`Unavailable`和`Indeterminate`；禁止只返回无来源语义的通用`NotFound`。`AuthoritativeNotFound`还必须携带Owner/Repository一致性分类，证明它来自原Ref与source key的线性化未写入检查。

Reader输入必须是完整`ToolCallCandidateObservationRefV1`，不能只传ID、CallID、ResponseID或digest字符串。Reader行为固定为：

1. 先Validate调用方Ref；
2. 按Ref ID定位候选；
3. 逐字段exact比较ID、revision、digest、Invocation ID/digest、Observation digest、ResponseID与SourceSequence；
4. 对存储值重新执行Projection/Observation Validate并重算两层digest；
5. 返回完整Projection的deep clone；
6. 返回值不得与Store、输入Ref或其他调用者共享可变Calls/JSON bytes。

同ID但exact ref不一致必须返回`Conflict`，不能当作NotFound；不存在、Retention已不可读、Reader不可用和状态未知必须使用不同分类，不能互相伪装。

Reader的NotFound分为两类：

| 分类 | 必要证明 | 恢复资格 |
|---|---|---|
| authoritative NotFound | Reader所属Store对原Ref和source key完成线性化检查；确认从未create；不是Retention删除/不可读；不是eventual consistency可见性窗口 | 仅作为Reader查询事实；不授权Direct跨方法重投 |
| ordinary/unknown NotFound | 无法证明线性化未写入，或可能是Retention不可读、eventual visibility、Reader代理缺证据 | 保持unknown，不重投 |

Publisher已返回成功但Reader暂不可见，不能据此生成authoritative NotFound；Backend若不保证read-after-write线性一致，必须返回ordinary/unknown、Unavailable或Indeterminate。G6A调用方拿到Projection后仍须显式验证`len(Calls)==1`；Reader不替Harness创建PendingAction，也不把N>1裁剪为N=1。

### 3.3 生产侧统一Repository capability

Model Invoker生产侧冻结为一个单方法原子能力：

```text
ToolCallCandidateObservationProjectionRepositoryV1
  EnsureSealedProjectionV1(ctx, sealedProjection)
    -> exact complete Projection clone
     | typed failure
```

`Ensure`必须在一个线性化点完成以下二选一：全新Ref/source时create-once并返回完整clone；已存在同一Ref/source/canonical内容时返回原完整clone。ID、source或内容冲突必须Fail Closed。Direct只调用该方法，不通过公开Publisher/Reader组合推断写入状态，也不使用可伪造DomainID。

Model-owned canonical producer合同冻结为：

```text
EnsureToolCallCandidateObservationProjectionV1(
  ctx,
  repository ToolCallCandidateObservationProjectionRepositoryV1,
  sealed ToolCallCandidateObservationProjectionV1,
) -> exact complete Projection clone | typed failure
```

调用者必须先用Model公共构造器完成Projection seal；该helper只接收并重新Validate完整、已sealed、canonical Projection，不执行seal。它统一拥有Repository nil/typed-nil依赖检查、原子Ensure、同sealed最多一次`Indeterminate`恢复和返回值exact compare。Direct wrapper只负责从终态Observation构造sealed Projection后委托该helper，不得复制第二套恢复或比较逻辑。`IsToolCallCandidateObservationProjectionRepositoryUnavailableV1`只是该框架侧canonical producer的依赖检查，不是provider-controlled能力探测。

若`Ensure`返回`Indeterminate`表示回包可能丢失，canonical producer helper只允许以同一个本地sealed Projection再次调用同一个`Ensure`一次；成功后逐字段exact比较返回Projection。第二次仍失败、返回其他错误或内容漂移时整批Fail Closed。`Unavailable`、Conflict或Reader NotFound都不触发重试。

只实现`PublishSealedProjectionV1 + InspectExactProjectionV1`的split A/B wrapper不满足Repository接口，不能注入Direct。自定义Repository实现必须自行承担单方法原子语义；Direct不再存在可被跨Store伪造的恢复判断。

### 3.4 依赖暴露

- direct执行面只注入`ToolCallCandidateObservationProjectionRepositoryV1`；
- Application、Harness、Tool及其Owner Adapter只允许依赖`ToolCallCandidateObservationProjectionReaderV1`；
- 只读消费者不得获得Publisher接口、组合Repository或具体Fake；
- Model Invoker不得import Application、Harness、Tool、Context或Continuity实现；
- Continuity/宿主若将来提供生产Adapter，只能实现Model公共Port并由未来宿主composition root注入。

未来Continuity adapter的边界固定为：

1. 将Model Projection Ref与source tuple作为opaque identity/source映射到Continuity Evidence/Content Object；
2. Content Object保存完整Projection wire正文，Continuity只保证字节与自身对象引用，不解释Model字段；
3. 读取后必须回到Model `DecodeToolCallCandidateObservationProjectionV1`及exact Reader规则重新严格解码、重算digest并比较原Ref；
4. Continuity不得重新分配Model ID、改写source、选择Calls或成为Model identity/current Owner；
5. Adapter只能实现Model公开Port，不能把Continuity内部Cursor、Record或Retention类型泄漏给Application/Harness/Tool。

## 4. publish-before-emit屏障

direct sync/stream必须使用同一顺序：

```text
整批terminal finalize
-> reserve唯一SourceSequence
-> seal Projection及原Ref
-> Model canonical producer helper
-> Repository.EnsureSealedProjectionV1
-> exact比较返回Projection与本地sealed Projection
-> 发出唯一authoritative observation union event
-> 发出逐Callcompatibility-only事件
```

硬规则：

- Ensure成功但返回Projection与本地sealed内容、Ref或source不exact：整批Conflict；
- Ensure `Indeterminate`：只重试同一个Ensure一次，不换ID、sequence或内容；
- Ensure `Unavailable`、Conflict、Invalid、ordinary/authoritative NotFound或其他异常：不重试并Fail Closed；
- 第二次Ensure仍失败：保持unknown，零权威事件、零兼容Call；
- typed-nil Repository必须在New、Preflight、Open和producer helper中于Backend触达前拒绝；
- Ensure失败时不得降级为逐Callcompatibility事件；
- 权威Observation事件必须使用Ensure返回并再次验证的clone，且Header SourceSequence与Ref完全一致；
- N>1 Projection允许作为完整历史Observation保存；若保留现有逐Call兼容事件，必须在完整Projection可exact Inspect且权威事件已发出后一次性按同一Ref/连续ordinal输出，仍不是Gateway authority。不得取首项、部分升级或创建PendingAction；G6A继续只接受`Calls==1`。

`model_step_started`等非Observation生命周期事件可以保留，但不能暗示Tool Call已经可执行。事件投递本身的全局exactly-once、Union Ledger去重和跨进程恢复不属于本Port；本Delta只保证在一个direct session中，未通过原子Ensure+exact compare屏障就不会输出权威Observation或兼容Call。

## 5. crash与lost-reply

| 窗口 | 恢复规则 |
|---|---|
| seal后、Ensure前crash | 原Ref没有外部事实；恢复必须使用同一已保存坐标，当前无生产恢复声明 |
| Ensure线性化后回包丢失 | 同一调用内只以同一sealed Projection重试同一Ensure一次；返回exact Projection后继续 |
| Ensure结果未知且再次Ensure仍失败 | 保持unknown并Fail Closed；不改用Reader NotFound推断写入状态 |
| Ensure成功、权威事件发出前crash | Projection已经是历史事实；是否重放同一Union事件由未来宿主/Union Ledger合同决定 |
| 权威事件后、兼容Call前crash | 不得从兼容事件重建第二Projection；权威事实仍按原Ref Inspect |

线程安全Fake只证明原子create-or-return-existing、exact Inspect、clone和并发语义，不证明真实进程crash、持久化、SLA或生产恢复。

本合同不承诺transport exactly-once。lost-reply恢复最多发送两次相同Ensure；它只承诺相同Ref/source/canonical内容的原子create-or-return-existing，Repository最终最多保存一份。

## 6. immutable historical truth与current窗口

Projection是不可变历史事实，不携带、推导或伪造领域TTL。`ToolCallCandidateObservationRefV1`保持不变，不能加入`ExpiresAt`或current标志。

G6A的短current观察窗口由Application `SingleCallToolActionInputCurrentReaderV1`计算：

```text
effective current upper bound
  = min(
      Application调用/Request上界,
      Projection Retention可读上界,
      Session/Turn/PendingAction及其他Owner current上界
    )
```

如果生产Backend需要表达Retention可读性，必须使用独立nominal type，例如：

```text
ToolCallCandidateObservationProjectionRetentionInspectionV1
  - exact Projection Ref
  - checked_at
  - readable_until
  - backend/retention binding digest
  - inspection digest
```

Retention Inspection不是Projection Ref、不是领域Fact currentness，也不授Evidence或执行资格。缺少Retention可读证明或任一Owner current Reader不可用时，G6A InputCurrentReader必须Fail Closed。

Continuity的Cursor TTL、`RecordedAt`和Retention state不能直接填入或推导上述current上界：Cursor只约束查询游标，`RecordedAt`只记录时间，Retention state只描述Continuity对象状态。只有未来Adapter提供独立、exact绑定原Projection Ref的可读性Inspection后，Application InputCurrentReader才能把其`readable_until`纳入最小上界。

## 7. Fake与Conformance

首个实现只允许提供线程安全内存Fake，候选名称：

```text
NewInMemoryToolCallCandidateObservationProjectionStoreV1
```

Fake同时实现原子Repository、兼容Publisher与Reader；direct只注入原子Repository。只组合“Store A写、Store B读”的wrapper没有Ensure方法，不能满足Direct Config。Application/Harness/Tool的消费侧合同仍只接收Reader窄接口，不能获得Repository或Publisher。

公共Conformance至少验证：

- create-once和双索引冲突；
- strict Validate/digest重算；
- exact full-ref读取；
- deep clone/no alias；
- lost Ensure reply只重试同一个Ensure一次，并保持同一sealed canonical Projection；
- Direct不读取AuthoritativeNotFound决定重投；
- Reader unavailable/indeterminate分类；
- split Publisher/Reader wrapper不满足原子Repository，跨Store恢复组合不可表达；
- typed-nil Repository在Backend触达前Fail Closed，zero-value Store仍安全；
- 32个以上并发同内容幂等、不同内容只有一个胜者；
- import图与Publisher capability不泄露给只读消费者。

详细用例见[测试矩阵](./tool-call-observation-projection-publish-reader-v1-test-matrix.md)。实施步骤见[对应计划](../../plan/model-invoker/tool-call-observation-projection-publish-reader-v1.md)。

## 8. 已闭合的联合评审门

reference implementation与第三审P1修复必须持续确认：

1. Model Invoker仍是Projection identity/Port Owner；
2. G6A Application DTO继续只携带中立coordinate，不新增payload bytes；
3. Harness PendingAction payload不替代Model Reader；
4. 生产Backend/composition root继续明确不存在；
5. `N>1`、Evidence、Settlement和动作授权没有扩域。
6. Continuity只作为未来opaque存储Adapter候选，不成为Model identity/current Owner；production driver/root保持NO-GO。
7. 不承诺transport exactly-once；只承诺Repository原子create-or-return-existing，Direct不依赖Reader NotFound恢复。
