# Runtime G6A Action Matrix/Router V1模块说明

## 1. 作用

本切面把单Call Tool Action登记为OperationScope Evidence V3唯一Run内封闭矩阵，并提供五维Owner-current路由与Runtime-neutral Provider Boundary只读合同。

## 2. 唯一矩阵

- `OperationScopeKind=run`；
- `EffectKind=praxis.tool/execute`；
- `PolicyProfile=praxis.tool/single-call-action-v1`；
- Generation与Run、Session、Turn、Action、Context全部required。

其他组合、缺维度、forbidden维度、未知Kind或Owner contract version全部Fail Closed。

## 3. Owner与线性化边界

- Runtime只把Owner source的`Kind/ID/Revision/Digest`无损投影为公共Applicability ref，不生成新ID/digest，也不创建Applicability Fact/Store；
- 封闭Router按`dimension + exact Kind + Owner contract version`绑定唯一注入Reader，并验证公共current projection、scope与TTL；
- Runtime-neutral `OperationProviderBoundaryRefV1`与current projection由Tool Owner Adapter提供；Runtime不写Tool Watermark；
- 受控test Provider seam依次复读execute Enforcement 4.1、execute Evidence Handoff与Boundary current proof，全部绑定同一Operation/Attempt后才允许一个逻辑fixture调用；
- boundary已穿越或Provider回包不明时只保留原Attempt恢复语义，不盲目再次调用；本切面不预填Consumption，不创建DomainResult或Settlement。

## 4. 组成

- `runtime/ports/operation_scope_evidence_action_v3.go`：封闭矩阵、source nominal projection、Boundary Ref/Projection/Reader和test seam公共合同；
- `runtime/kernel/operation_scope_evidence_action_router_v3.go`：五维封闭Router与受控fixture Provider seam；
- `runtime/conformance/operation_scope_evidence_action_v3.go`：只依赖公共Port的隔离Conformance；
- `runtime/tests/**/operation_scope_evidence_action_v3_test.go`：canonical、路由、currentness、故障、64并发和零Provider反例。

## 5. 限制

当前只有test fixture注入，没有production composition root、真实Provider backend、持久Boundary backend、availability或SLA声明；不启用Capability、Context Refresh、Continuation、Turn推进或N>1，也不改变Evidence V3、Dispatch V4.0、Enforcement 4.1、Settlement V4的既有字段与digest。

设计入口：[G6A Action Matrix/Router V1](../../design/runtime/g6a-action-matrix-router-v1/README.md)。
