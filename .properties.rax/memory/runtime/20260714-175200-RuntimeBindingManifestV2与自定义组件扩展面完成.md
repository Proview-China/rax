# Runtime Binding Manifest V2与自定义组件扩展面完成

## 事件

Runtime执行底座P0.1完成：新增domain-separated Canonical Digest、严格ASCII/SemVer/Schema/Opaque Envelope、Governance Catalog、自定义组件注册与Probe、BindingFact/BindingSet CAS状态机、稳定依赖DAG、原子测试Fact Store及Conformance首切面。

## 决定

- 保留`v1alpha1`合同，新能力使用独立V2类型。
- 注册、Probe、认证、绑定和Dispatch资格保持五个不同权威层级。
- 用户自定义组件通过Namespaced Kind和治理Catalog接入，Runtime不硬编码6+1组件。
- 未知required能力/Schema/Extension、TTL边界、Clock回拨、Digest漂移、循环依赖和多主Owner全部Fail Closed。
- v1 Adapter最多产生受限、带Residual的兼容Manifest，不能自动获得V2认证。

## 验证

- Runtime全量普通测试通过。
- Runtime核心、Ports、Control、Fakes定向Race通过。
- `go vet ./...`通过。
- Canonical Digest fuzz约23.8万次执行通过。
- Namespaced Name fuzz约62.6万次执行通过。

## 后续

进入P0.2：Effect Fact Journal、Governance Dispatch Gateway和Budget Binding V2。P0.2完成前，Binding不授予Effect Dispatch资格。
