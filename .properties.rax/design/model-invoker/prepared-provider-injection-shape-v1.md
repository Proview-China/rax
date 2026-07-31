# Prepared Provider Injection Shape V1 设计冻结

## 1. 裁决与 Owner

- 唯一 Owner：`Model Invoker`。
- 本切片只定义 `ActualProviderInjectionDigest` 的纯 canonical 合同。
- 本切片不授予 Provider 执行权，不创建 Provider Attempt/Observation，不读取 Credential，不跨 Runtime/Harness/Context/Tool 边界。
- A1 与 A2 分离；本设计不修改 routegateway，也不解除现有 production NO-GO。

## 2. Canonical 闭集

`PreparedProviderInjectionShapeV1` 只覆盖：

1. `ContractVersion`；
2. exact `Provider` 与 concrete `Protocol`；
3. 保留请求原序的 `OrderedTools`；
4. 显式四态 `ToolChoice`；
5. 显式三态 `ParallelToolCalls`；
6. selected Provider namespace 的 `ProviderOptions`。

每个 `PreparedProviderToolV1` 只覆盖：

- `Name`；
- `Description`；
- strict canonical JSON object `Parameters`；
- `unspecified/false/true` 三态 `Strict`。

`PreparedProviderOptionsV1` 固定为：

- `Provider`：exact selected Provider；
- `Present`：区分 absent 与 present `{}`；
- `CanonicalJSON`：present 时为 detached strict canonical JSON object，absent 时为 `null`。

明确排除：

- Instructions、Input、Model；
- Endpoint、Credential；
- Budget、Output、Reasoning、State、Stream、Metadata、AllowDegradation；
- Tool expected digest、Context material；
- Provider response、attempt、observation。

Hosted/builtin/provider tool extensions只能通过 selected ProviderOptions 进入。

## 3. JSON 与 canonical

Tool Parameters和selected ProviderOptions均执行：

```text
UTF-8 valid
→ raw JSON string token Unicode scalar校验
→ core.DecodeStrictJSON(raw, &map[string]json.RawMessage{})
→ 顶层必须是非null object
→ json.Decoder.UseNumber 解码规范树
→ json.Marshal
```

raw JSON string token校验必须在`encoding/json`解码前拒绝所有未配对的UTF-16 high/low surrogate escape；合法paired surrogate允许，并与对应literal Unicode scalar形成同一canonical。不得依赖`encoding/json`对未配对surrogate静默替换为U+FFFD后的值做区分。

该顺序拒绝递归重复键、尾随文档、非object、空输入和大于1MiB的输入。object key由Go JSON编码器确定序，array保序；`json.Number`保留原始number lexeme，因此`1`与`1.0`不合并。输出必须与caller raw slice脱离，不保留alias。

Tools的nil与empty统一为非nil空有序列表；Strict与Parallel均保留nil/false/true。ToolChoice的空Mode只在canonical中显式转换为`auto`；Function保留并校验exact Name。

## 4. Digest

```go
core.CanonicalJSONDigest(
    "praxis.model-invoker.prepared-provider-injection",
    "v1",
    "PreparedProviderInjectionShapeV1",
    shape,
)
```

`ComputeActualProviderInjectionDigestV1(preparedProvider.request)`的返回值必须逐字节等于`PreparedModelInvocationFactV1.ActualProviderInjectionDigest`。它不得与Tool ExpectedInjection、Context injection或Surface digest互比或互推。

## 5. Fail Closed 与边界

- Provider为空、Protocol不是concrete有效值、Tool name空白/重复、Function choice错名均拒绝。
- ProviderOptions只允许0或1个namespace，非exact selected Provider拒绝。
- Build/Compute全过程外部调用、Provider调用、Tool执行和事实写入均为0。
- A2未来只能在`prepareAt`成功之后、任何Boundary/guard/Provider动作之前调用本合同做exact compare；该接线不属于A1。

## 6. 验收

- golden：空/单/多Tools、有序交换、四态ToolChoice、Strict/Parallel三态、options absent与`{}`、JSON key order/whitespace、number lexeme、固定digest；
- negative：递归重复键、尾随、非法UTF-8、未配对UTF-16 surrogate escape、非object、大于1MiB、wrong/multiple namespace、duplicate tool name、Function错名、raw alias；
- Unicode scalar positive：合法paired surrogate与对应literal Unicode在Parameters和ProviderOptions两轴形成同一canonical/digest；
- 边界：生产文件不得导入Provider、routegateway、internal、Harness、Context、Tool、Application或Runtime implementation。
