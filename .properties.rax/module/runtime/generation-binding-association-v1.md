# Runtime Generation-Binding Association V1模块说明

状态：**最终独立代码短审YES（P0/P1/P2=0），additive只读Reader完成**。

## 1. 作用

Runtime继续唯一拥有Generation与BindingSet/Activation之间的Association Fact。新增`GenerationBindingAssociationCurrentReaderV1`只抽取既有current Inspect，使Tool等消费者可以验证Runtime权威Fact，而不获得`AssociateGenerationBindingV1`写权限。

## 2. 组成

- `ports/generation_binding_v1.go`：Reader及兼容嵌入的Governance Port；
- `conformance/generation_binding_v1.go`：public Reader-only Conformance与typed-nil门禁；
- `tests/ports/generation_binding_current_reader_v1_test.go`：方法集、真实Go签名、capability narrowing、wrong fact、typed-nil和import boundary反例。

既有Fact Store、Gateway、对象、digest与行为没有变化，所有旧实现继续满足Governance Port。

## 3. 验证

```text
go test ./kernel ./tests/ports -run 'GenerationBinding' -count=100
go test -race ./kernel ./tests/ports -run 'GenerationBinding' -count=20
go test -count=1 -shuffle=on ./...
go test -race -count=1 ./...
go vet ./...
```

以上均PASS；target100耗时kernel 0.503s、tests/ports 0.102s，race20耗时kernel 2.046s、tests/ports 1.276s，full race中tests/fakes 53.715s。最终独立代码短审结论为YES（P0/P1/P2=0）。

## 4. 限制

本Delta不实现Tool Adapter、production backend/root、持久化、可用性或SLA，不授Binding、Activation、Permit或Provider能力。相邻组件仍须独立验证expected Ref、Fact current state与TTL。

设计入口：[只读Port Delta](../../design/runtime/generation-binding-association-v1/port-delta.md)。
