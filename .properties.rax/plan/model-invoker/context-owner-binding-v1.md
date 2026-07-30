# Model Context Authoritative Owner Binding V1 实施计划

## 范围

仅新增：

- `ExecutionRuntime/model-invoker/context_owner_binding_v1.go`；
- `ExecutionRuntime/model-invoker/tests/contextownerbindingv1/**`；
- 同名Model设计、计划、模块说明与memory快照。

禁止修改Invocation Material V1/V2 wire/digest、SQLite schema及Context、Harness、
Provider、Runtime、Tool、Console实现。

## 落地清单

1. 定义原始Context Owner、Material exact lookup、Request和Projection DTO；
2. 实现冻结的完整Owner canonical映射与中立Owner生成；
3. 实现Request/Projection seal、Validate与canonical digest；
4. 定义只读Reader Port，不提供adapter或构造器；
5. 覆盖完整字段漂移、TTL、canonical vector、Kind/Digest角色分离和import边界；
6. 执行focused ordinary×100/race×20及模块full ordinary/race/vet/diff/import；
7. 只提交本切片允许范围。

## 预期产物

完成后，Context adapter可以权威生成带完整Owner provenance的Model中立projection；
Model authorizer/Harness后续只能消费该projection，不能自行发明Context owner映射。
本计划不产生production adapter或dispatch资格。
