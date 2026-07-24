# H3 Owner Current Readers Port Delta V1

## 原因

冻结的 `HostConfigV1` 只携带opaque stable IDs，无法无损表达Owner exact revision/digest；同时stable source ID不能当作DefinitionID。若无下列current readers，Host只能猜ref或改变V1配置语义，两者均不允许。

## 精确新增签名

```go
type DefinitionSourceCurrentReaderV1 interface {
    InspectDefinitionSourceCurrentV1(context.Context, string) (contract.DefinitionSourceCurrentV1, error)
}

type ResolutionInputsCurrentReaderV1 interface {
    InspectResolutionInputsCurrentV1(context.Context, string, string) (contract.ResolutionInputsCurrentV1, error)
}
```

Reader参数分别为opaque source stable ID，以及opaque catalog/facts stable IDs。Reader内部使用自身时钟产生current projection；Host读取后使用自己的fresh clock验证Checked/Expires。

## 非变更

- `HostConfigV1` 零字段变更；
- 不向Host暴露Owner mutation；
- 不替代Definition Owner current；Definition接线必须同时证明source mapping current与Owner active current；
- 不承担Runtime Binding/Activation/Readiness。
