# Model Canonical Body Digests V2 模块说明

本切片在Model Invoker现有Invocation Material V2边界上，additive公开两个纯函数：

- `DigestGovernedModelTurnRequestToolSetV2([]Tool)`：直接对实际
  `RouteCall.Request.Tools`等价字节计算Model-owned `RequestToolsDigest`；
- `DigestGovernedModelTurnContextBodyV2([]Instruction, []InputItem)`：直接对实际
  `Instructions/Input`等价字节计算Model-owned Context body digest。

两个函数分别沿用旧完整`RouteCall`入口完全相同的domain、version、
discriminator和canonical body。旧入口仍先完成完整`RouteCall`校验，再委托对应
body函数；body函数深拷输入，执行现有Model合同的合法性、strict JSON、
duplicate/trailing和canonical size校验。

本切片不构造虚假`RouteCall`，不接收或回显caller传入digest，不导入Tool或
Context实现，不调用Provider，不执行Tool，也没有Console合同。Tool Owner可在未来
adapter中把实际`RouteCall.Request.Tools`交给Model算法，但adapter本身不属于本切片。

Context body函数只公开现有Model canonical body算法；它不定义Context Channel、
Frame/Material到`Instructions/Input`的全量lowering。该跨Owner映射仍需独立设计、
实现和审计，当前production继续`NO-GO`。

验证入口：

```text
cd ExecutionRuntime/model-invoker
go test -count=100 ./tests/invocationmaterialv2
go test -race -count=20 ./tests/invocationmaterialv2
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```
