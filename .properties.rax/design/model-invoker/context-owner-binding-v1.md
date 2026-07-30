# Model Context Authoritative Owner Binding V1

## 已冻结裁决

本切片只为Model Invoker提供Context authoritative owner binding的中立消费合同，
不实现Context adapter，不修改`InvocationMaterialV1/V2`或
`InvocationMaterialSourceLineageV2`，不接入Harness、Provider、Runtime、Tool或
Console。

Context原始Owner固定为`{ComponentID, BindingDigest}`。Model中立Owner固定为：

- `Domain = praxis.context`；
- `ID = CanonicalJSONDigest("praxis.context/model-neutral-owner", "v1",
  "ContextOwnerRef", 完整原始Owner)`。

映射必须同时绑定`ComponentID`与`BindingDigest`；Harness或其他消费者不得只复制
ComponentID，也不得另行literal/hash而丢失BindingDigest。

## 合同

- Material lookup为exact `{Kind, ID, Revision, Digest}`；
- Request绑定Material lookup、Checked、NotAfter与canonical Digest；
- Projection同时携带原始Context Owner、Owner identity digest、中立Owner、
  Material/Frame的`Owner + Kind + ID + Revision + Digest`、Context lineage
  digest、Checked、Expires与Projection digest；
- Material与Frame必须保持不同Kind和不同Digest，不允许角色折叠；
- Context Kind的公开语义由Context公共projection/adapter验证，Model不声明Context
  Kind常量；
- Reader Port只有
  `InspectCurrentInvocationContextOwnerBindingV1`，Model不拥有实现。

## 失败关闭

原始Owner、中立Owner、identity digest、Material/Frame任一坐标、Request lookup、
TTL或canonical digest漂移均拒绝。typed nil由后续消费者构造器验证，不在本DTO/
Reader Port切片内增加反射式策略。

Context adapter及后续S1/S2复读未完成前，production保持`NO-GO`。
