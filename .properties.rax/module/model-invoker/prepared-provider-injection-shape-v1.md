# Prepared Provider Injection Shape V1 模块说明

## 1. 作用

本模块把已经完成纯映射的`modelinvoker.Request`中、真正影响Provider工具注入解释的字段，转换为一个稳定、闭集、可摘要的canonical Shape。它用于未来在Provider调用前证明“当前实际映射形状”与Prepared事实保存的`ActualProviderInjectionDigest`完全一致。

## 2. 公共入口

```go
BuildPreparedProviderInjectionShapeV1(Request) (PreparedProviderInjectionShapeV1, error)
ComputeActualProviderInjectionDigestV1(Request) (core.Digest, error)
```

Build返回detached Shape，Compute按冻结domain计算digest。两者均为纯函数，没有网络、Provider、Tool、Sandbox或事实写入效果。

## 3. 组成

- `PreparedProviderInjectionShapeV1`：合同版本、Provider/Protocol、ordered Tools、ToolChoice、Parallel和ProviderOptions；
- `PreparedProviderToolV1`：Name、Description、canonical Parameters、Strict三态；
- `PreparedProviderToolChoiceV1`：显式Mode与Function Name；
- `PreparedProviderTriStateV1`：`unspecified/false/true`；
- `PreparedProviderOptionsV1`：Provider、Present、canonical JSON。

## 4. 输入与输出规则

- 输入Protocol必须是映射后的concrete协议；
- Tools严格保留输入顺序；
- Parameters和ProviderOptions必须是UTF-8、递归无重复键、无尾随、顶层object且不超过1MiB；
- raw JSON string必须在标准库解码前拒绝未配对UTF-16 surrogate escape；合法pair与对应literal Unicode scalar归一；
- JSON object key/whitespace被规范化，array顺序和number lexeme保留；
- nil/empty Tools统一，bool pointer三态不合并；
- ProviderOptions absent与present空object不合并；
- 所有Raw JSON在返回前重新分配，不保留caller alias。

## 5. 依赖与边界

模块只依赖Go标准库与Runtime public `core` canonical能力。不依赖Provider实现、routegateway、model-invoker internal、Harness、Context、Tool、Application或Runtime implementation。

本模块不是dispatch Gate或Permit。A2未来负责在`prepareAt`后、Boundary前执行exact compare；Credential current、durable Provider Attempt/Observation仍是独立前置。

## 6. 验证

测试目录：`ExecutionRuntime/model-invoker/tests/preparedproviderinjectionv1/`。

覆盖固定golden、全部字段轴、四态/三态、JSON规范化、number lexeme、surrogate scalar正负例、排除字段、strict JSON负例、namespace负例、function choice负例、alias与import边界。

完整ordinary/race/vet结果以同名memory快照为准。
