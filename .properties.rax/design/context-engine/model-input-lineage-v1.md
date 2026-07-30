# Context Model Input Lineage Current V1

## 当前裁决

本切片只为Context Owner新增模型输入物料的权威lineage读取面，不修改既有
`ContextModelInputMaterialV1`、`ContextFrame`或其摘要语义，也不接入Harness、
Model Invoker或Provider。

公共Kind固定为：

- `praxis.context/model-input-material`
- `praxis.context/frame`

公共exact source由`Owner/Kind/ID/Revision/Digest`组成。Material source的后三项
必须来自`ContextModelInputMaterialV1.Ref`；Frame source的后三项必须来自同一
Material的`FrameRef`。两者Owner相同，Kind和Digest必须不同，禁止拼接或类型冒充。

## Owner与状态

Context Owner拥有：

- Material exact/current读取；
- Frame exact-current读取；
- S1/S2复读和lineage current projection；
- projection的Checked/Expires/Digest。

Reader按以下顺序执行：

```text
request/source校验
  -> S1 Material exact + Material current
  -> 从Material派生FrameRef
  -> S1 Frame exact-current
  -> 重算并验证Material及其FrameRef
  -> S2完整复读
  -> 检查S1/S2一致、clock和TTL
  -> 返回sealed current projection
```

`Expires=min(request not-after, Material TTL, Frame TTL, 30s观察上限)`，不得延长
任何Owner TTL。history-only Material、Owner/Kind/ID/Revision/Digest漂移、
S1/S2漂移、TTL crossing、clock rollback、Unavailable或cancel均返回零projection。

## 边界

- 不产生Evidence、Authorization、Permit、Settlement、Outcome或Continuation；
- 不调用Harness、Model Invoker或Provider；
- 不新增SQLite或生产root；
- Frame exact-current reader若只有fixture，整个跨模块dispatch保持production NO-GO。
