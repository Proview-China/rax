# Generation-Binding Association V1只读Port Delta

状态：**最终独立代码短审YES（P0/P1/P2=0），additive Reader完成**。

## 1. 目标

向只需验证Generation-Binding Association currentness的相邻组件提供最小只读能力，避免其持有含`AssociateGenerationBindingV1`的治理写面。

本Delta不改变既有Association Candidate/Fact/Ref、canonical、digest、Fact Owner、Gateway、TTL、错误语义或backend；不增加production root。

## 2. 公共合同

```go
type GenerationBindingAssociationCurrentReaderV1 interface {
    InspectCurrentGenerationBindingAssociationV1(
        context.Context,
        string,
    ) (GenerationBindingAssociationFactV1, error)
}

type GenerationBindingAssociationGovernancePortV1 interface {
    GenerationBindingAssociationCurrentReaderV1
    AssociateGenerationBindingV1(
        context.Context,
        GenerationBindingAssociationCandidateV1,
    ) (GenerationBindingAssociationFactV1, error)
}
```

Reader的方法集严格只有一个Inspect方法。Governance通过嵌入保持原有两个方法；所有既有Gateway与实现天然兼容，无迁移或适配写入。

## 3. 能力边界

- Tool等消费者只注入`GenerationBindingAssociationCurrentReaderV1`；静态类型不暴露Associate。
- Reader只返回既有Runtime Fact；调用方仍须执行`Fact.Validate()`、expected Ref/current state/TTL的领域校验。
- Candidate、Handoff或调用方DTO不能冒充Association Fact。
- Reader不授Binding、Activation、Permit、Provider执行权或production资格。
- Adapter导入边界仍只允许`runtime/core`与`runtime/ports`；不得导入`control`、`kernel`、`fakes`或Owner写面。
- nil/typed-nil Reader在公共Conformance中以`ComponentMissing` fail closed。

## 4. 兼容与验证

公共Conformance新增Reader-only case，只调用Inspect，不取得或调用Associate。反射与编译测试冻结：

- Reader恰有一个方法，签名为`(context.Context, string) (GenerationBindingAssociationFactV1, error)`；
- Governance仍恰有Inspect+Associate两个方法；
- reader-only实现可通过Conformance但不能满足Governance Port；
- typed-nil、wrong association与越界import fail closed。

Owner实测：target ordinary `count=100`、target race `count=20`、Runtime full ordinary/race、vet、gofmt与diff-check均通过；最终独立代码短审YES（P0/P1/P2=0）。该结果不声明production backend、root、durability或SLA。
