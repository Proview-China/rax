# Organization Engine 模块说明

## 用途

为 Review Human Enterprise Multi-Sign 提供可独立复读的组织事实：谁是 Reviewer、当前拥有哪些角色、委托是否明确且有效、候选作者/发起者是谁。这样 Review 可以禁止生产自审并验证角色/委托，而不用自行签发 Organization 事实。

## 组成

```text
contract/       immutable facts、exact refs、seal/validate
ports/          StoreV1、ReviewEligibilityCurrentReaderV1
memory/         reference/test backend
storage/sqlite/ single-node WAL State Plane
current/        S1/S2、min TTL、clock/unknown recovery
conformance/    reusable Store/Reader suite
release/        ComponentRelease、local/production readiness、Publisher/Conformance
```

## 输入与输出

- 输入：tenant、Reviewer stable subject、required roles、scope digest、Responsibility subject exact coordinate、可选明确 Delegator/DelegatedRole。
- 输出：sealed `ReviewEligibilityCurrentProjectionV1`，内含当前 Identity、全部 Role、可选 Delegation、Responsibility 与责任 Identity exact facts，以及固定 Checked、真实最短 Expires 和 ProjectionDigest。

## 持久化

Owner-local durable backend 使用单机 SQLite WAL。history 和 current full-ref 分表；projection 对同一 closure create-once，后续 exact Inspect 返回 deep clone。纯时间到期不写新 revision，只在 actual-point current 校验失败。

## Component Release

- ComponentID固定为`components/organization`；唯一Capability为`praxis.organization/review-eligibility-current`。
- memory/reference readiness缺失时只发布`reference_only`；exact SQLite local readiness可发布`standalone`。
- `production`必须同时复读Organization自身Local、ResourceBinding、cleanup、deployment、executable factory与独立certification exact current；当前没有真实production readiness发布。
- Factory只发布`ModuleFactoryDescriptorV1`，不包含constructor、SQLite handle、Review consumer或production root。
- Runtime拥有Runtime Effect/Settlement；Organization只拥有本域事实和cleanup，不签发Review Verdict。

## 验证状态

- targeted ordinary100：PASS；
- targeted race20：PASS；
- full ordinary/race：PASS；
- `go vet ./...`：PASS；
- Release targeted ordinary100/race20：PASS；
- Release Assembler黑盒、lost reply、TTL/clock、64并发、import boundary：PASS；
- conformance 覆盖 memory 与 SQLite；SQLite restart/integrity：PASS。

## 限制

- Owner-local backend 已完成，但未建立 production composition root；
- Component Release代码候选等待独立审计，production readiness/executable factory均未发布；
- 不宣称 HA、跨节点线性一致或 SLA；
- Runtime Authority、Review Verdict、Policy/Binding/Evidence/Scope 仍由各自 Owner 提供；本模块不得被当作它们的替代品。
