# PreparedProviderInjectionShapeV1 A1实现候选完成

## 事件

基于`origin/main@d96228726042d3910a684bc885db41e3f7ae66a7`，Model Owner Design Delta A的PR A1已在独立分支`agent/prepared-provider-injection-shape-v1`形成未提交实现候选。

## 已完成

- 新增纯canonical公共Shape/Tool/ToolChoice/三态/Options nominal；
- 新增`BuildPreparedProviderInjectionShapeV1`与`ComputeActualProviderInjectionDigestV1`；
- Provider与Protocol、ordered Tools、ToolChoice、Parallel三态及selected ProviderOptions已形成闭集；
- Parameters与ProviderOptions通过`core.DecodeStrictJSON`、`UseNumber`和`json.Marshal`形成strict canonical object；
- Parameters与ProviderOptions在`encoding/json`前逐string token校验Unicode scalar，拒绝未配对UTF-16 surrogate escape，合法pair与literal scalar归一；
- 递归重复键、尾随、非法UTF-8、未配对surrogate、非object、大于1MiB、namespace、重复tool name、Function错名和alias均fail closed；
- 固定golden digest为`sha256:48a493182a43572797ac61ef555d85610bd0e7bd83ee74c11f59ee22b498fba8`；
- import边界证明生产文件未引入Provider、routegateway、internal或相邻Owner实现。

## 验证

以下命令实际退出0：

```text
go test ./tests/preparedproviderinjectionv1 -count=100
go test -race ./tests/preparedproviderinjectionv1 -count=20
go test ./...
go test -race ./...
go vet ./...
gofmt -l ExecutionRuntime/model-invoker/prepared_provider_injection_shape_v1.go ExecutionRuntime/model-invoker/tests/preparedproviderinjectionv1
git diff --check
```

focused import boundary包含在focused ordinary/race中并通过。最终一次全量ordinary/race在新增单Tool明确轴后再次通过；未调用真实Provider、Tool或Sandbox。

## 自审

- P0：0
- P1：0
- 类型映射阻塞：0
- A2、B、C与production Provider链：仍未授权、未实施
- 当前状态：A1未提交候选，等待root独立审计

## P1 surrogate canonical返修

独立审计发现`encoding/json`会把未配对UTF-16 surrogate escape静默替换为U+FFFD，可能使不同raw注入折叠到同一canonical/digest。A1已在标准库解码前增加raw JSON string token Unicode scalar校验：

- `\ud800`、`\ud801`、`\udc00`、high后非low或literal均fail closed；
- 校验覆盖嵌套value与object key；
- 合法paired surrogate允许，并与对应literal Unicode scalar形成同一canonical/digest；
- Parameters与ProviderOptions两轴均覆盖Build拒绝、Compute空digest及positive归一。

返修后以下命令实际退出0：

```text
go test ./tests/preparedproviderinjectionv1 -count=100
go test -race ./tests/preparedproviderinjectionv1 -count=20
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

返修终审：P0=0，P1=0；A2、B、C及真实Provider链仍未授权。
