# Context Owner Binding Adapter V1 设计

## 已冻结裁决

本切片由Context Owner实现
`ContextModelInputLineageCurrentV1 -> modelinvoker.InvocationContextOwnerBindingProjectionV1`
的唯一权威适配。输入只接受Model公开
`InvocationContextOwnerBindingRequestV1`，实现Model公开只读Reader Port；不修改或写入
Context/Model事实，不建立Store，不调用Provider、Harness、Tool、Sandbox、
Application、Host或Runtime实现。

Material Kind只能逐字来自Context公开常量
`ContextInvocationSourceModelInputMaterialV1`，即
`praxis.context/model-input-material`。Model请求中其他任意非空Kind即使通过Model DTO
校验，也必须在调用Context Owner reader前失败关闭。

## 权威流程

```text
Model exact Material request
  -> 校验Model request与Context固定Material Kind
  -> 逐字段转换为Context exact Material source
  -> Context lineage current reader S1
  -> 校验Owner、两个Kind、exact coordinate、current/TTL/digest
  -> Context lineage current reader S2
  -> 校验完整S1 == S2与clock单调
  -> 完整Context Owner映射为Model ContextOwnerRef
  -> 由Model公开映射/sealer派生neutral Owner
  -> Material/Frame逐字段无损转换
  -> fresh clock密封Model projection
```

Context Owner必须完整保留`{ComponentID, BindingDigest}`；不得仅复制
`ComponentID`，不得由Harness生成literal/hash。Model Material/Frame exact source的
`Kind/ID/Revision/Digest`逐字段来自Context projection，Owner使用Model公开合同从完整
Context Owner派生出的neutral Owner。

`ContextLineageDigest`直接绑定通过Context公开合同复算验证的lineage projection
`Digest`。角色由Owner字段位置、公开Kind及`ID/Revision/Digest` exact coordinate证明；
不增加跨domain digest数值全不等语义。`Owner.BindingDigest`可以合法地等于Material或
Lineage digest；仅保留Context公开合同已冻结的Material/Frame Kind不同且
Material/Frame digest不同约束。

## 时间与失败语义

- adapter clock在S1前、S1后和S2后读取，任何回退均拒绝；
- S1和S2分别在新鲜clock下完成`ValidateAgainst`；
- 输出`Expires=min(Model NotAfter, S1 Expires, S2 Expires)`；
- 输出`Checked`使用S2完成后的fresh clock；
- `now == expiry`、TTL crossing、history-only、current=false、S1/S2任一字段漂移、
  reader unavailable/typed nil、cancel与Unknown均返回零Model projection。

## 边界

生产代码只允许导入Context公开`contract/ports`、Model Invoker根公开合同及其公开
Runtime core值类型；禁止导入Model实现子包。Context Frame durable current reader已由
独立PR #57合并到主线，并通过同一个Context lineage窄Reader边界与本adapter共存；
本切片不绕过该窄Reader，也不新增第二套Frame读取语义。

测试fixture只用于owner-local/reference黑盒验证，不代表production composition。
durable Frame reader的合并消除了该依赖缺口，但RouteCall lowering与Harness production
composition仍未进入本切片，production dispatch继续`NO-GO`。
