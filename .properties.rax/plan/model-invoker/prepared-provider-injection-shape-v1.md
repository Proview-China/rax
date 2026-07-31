# Prepared Provider Injection Shape V1 实施计划

## 1. 状态

- Owner：`Model Invoker`
- 切片：Design Delta A / PR A1
- 基线：`origin/main@d96228726042d3910a684bc885db41e3f7ae66a7`
- 当前状态：A1实现候选、完整门禁和P0/P1自审已完成，等待root独立审计
- A2、B、C及真实Provider链：未授权、未实施、继续NO-GO

## 2. 产物

- [x] `ExecutionRuntime/model-invoker/prepared_provider_injection_shape_v1.go`
- [x] `ExecutionRuntime/model-invoker/tests/preparedproviderinjectionv1/**`
- [x] 同名design/plan/module/memory资产
- [x] public Shape/Tool/ToolChoice/三态/Options nominal
- [x] `BuildPreparedProviderInjectionShapeV1`
- [x] `ComputeActualProviderInjectionDigestV1`

## 3. 实施清单

- [x] exact Provider与concrete Protocol入canonical；
- [x] Tools保持原序，nil/empty统一为空有序列表；
- [x] Parameters执行strict object canonical并深复制；
- [x] raw JSON string在`encoding/json`前拒绝未配对surrogate，合法pair与literal scalar归一；
- [x] Strict与Parallel保留unspecified/false/true；
- [x] ToolChoice显式封存auto/none/required/function；
- [x] ProviderOptions只允许selected Provider的0或1个namespace；
- [x] absent与present `{}`通过Present分离；
- [x] 排除Input/Instructions/Model/Endpoint/Credential及其他非注入字段；
- [x] digest domain/version/discriminator按冻结值实现；
- [x] 生产文件保持纯函数且无相邻Owner import；
- [x] focused ordinary `-count=100`；
- [x] focused race `-race -count=20`；
- [x] Model全量ordinary/race/vet；
- [x] gofmt/diff/import boundary；
- [x] P0/P1自审；
- [ ] root独立审计。

## 4. 验收门

1. 固定golden digest逐字节稳定；
2. JSON key order与whitespace规范化，array及number lexeme保持；
3. Provider、Protocol、Tools各轴、ToolChoice、Parallel与Options漂移改变digest；
4. 明确排除字段变化不改变digest；
5. 所有strict JSON、未配对surrogate和alias负例fail closed，合法surrogate pair与literal scalar同canonical；
6. focused与Model全量普通/race/vet通过；
7. production diff仅出现A1获准文件，Provider/routegateway改动为0。

## 5. 完成后的产物边界

A1完成后只提供可复用的实际Provider注入形状canonical和digest计算器。它不接入routegateway、不跨Boundary、不调用Provider，也不解除Credential、durable Attempt/Observation和production dispatch阻塞。
