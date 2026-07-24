# Run Lineage Association V1 设计候选

状态：**已吸收首轮独立审计`NO（P0=1/P1=2/P2=0）`并完成最小资产返修；READY等待不同agent复审，不自标YES；未授权 Go 实现，production root NO-GO**。

## 1. 目标

为需要独立 child Run 的通用编排提供 Runtime-owned、可精确复读的父子 Run 关联。首个消费者可以是 Detached Review，但本合同不包含 Review Case、Target、Verdict、Delivery 或 Harness Phase 对象，也不创建第二套 ReviewRun 生命周期。

本候选复用 live Runtime Run V3：

- child 仍是普通 `core.AgentRunRecord`、`CreatePendingRunRequestV3` 与 `RunLifecycleEnvelopeV3`；
- Runtime 仍唯一拥有 Run、Outcome、Claim、Settlement、Termination 与 Fence；
- Application 仍只持有编排水位；Harness 仍只拥有 Run 内 Session/Phase source；
- 关联只证明两个 Runtime Run 的精确谱系关系，不授予执行、Evidence、Review、Authority、Budget、Scope 或 Provider 权限。

## 2. 关键裁决

1. `RunLineageAssociationV1` 是通用 Runtime sidecar，不是 Review 专用 Run、第二 Run Fact 或 V3 字段扩展。
2. rev1 关联必须与 child pending Run bundle、Plan certification association及空 EffectIndex 在 Runtime Owner 同一原子事务中全有全无发布。
3. 关联稳定 ID 只由 exact parent/child Run identity 与 scope 派生；同一 subject 永不换 ID。
4. immutable history、`highestRevision` 与 current full-ref index 在同一事务发布；revision 从 1 开始并严格 `+1`，禁止 gap、rollback、覆盖历史或 ABA。
5. parent 是创建时 exact anchor；Reader 每次另行复读 parent current。child phase 每次变化时，关联 revision 必须与对应 Runtime lifecycle mutation 同一事务推进。
6. 纯时间流逝不得生成 `expired` revision；过期只由 `ValidateCurrent(now)` 失败关闭。
7. current与subject-bootstrap terminal Resolve使用 baseline→S1→fresh clock→S2→fresh clock，要求association index、exact history、parent lifecycle、child lifecycle完整一致；exact `InspectTerminal`只按full Ref读取immutable terminal history，不借current index，也不新增第二套Run-current/terminal ProjectionRef。
8. Reader constructor只接受具名Store/Lifecycle只读Reader、fresh Clock与`0<timeout<=2s` recovery policy；nil/typed-nil依赖构造即Fail Closed。
9. parent current与anchor同revision时full exact Ref必须相等；只有更高revision才能重算RecordDigest/phase并验证reachable，不接受caller复制digest或自报phase。
10. lost mutation reply只Inspect原child/association exact对象；lost read按Subject新S1或同full Ref exact retry，均受TTL/deadline裁剪；不得再调create、Begin、settle或termination mutation。

## 3. Owner 与非 Owner

| 对象/动作 | 唯一 Owner | 非 Owner 边界 |
|---|---|---|
| parent/child Run lifecycle、Run Outcome | Runtime | Review/Application/Harness只读引用 |
| `RunLineageAssociationV1` history/current/highest | Runtime | Application不能写关联，Review不能造关联 |
| child pending Run + association rev1原子创建 | Runtime trusted assembler/Fact Owner | caller只提交exact候选，不取得create权 |
| child Run record revision/lifecycle phase与association同步推进 | Runtime lifecycle Owner | 不暴露独立publisher给组件 |
| Detached Review等待/Case/Target/Verdict | Application/Review各自Owner | 不进入本合同 |
| Harness Review Phase source | Harness | Runtime不复制Harness DTO或Reader |
| production composition | Agent Host/受信Assembler联合Owner | 当前NO-GO，不以fixture冒充 |

## 4. 生命周期关系

```text
trusted host准备parent exact anchor + ordinary child CreatePendingRunRequestV3
  -> Runtime同事务创建 child pending Run V3 closure + association revision 1
  -> child Run record revision或lifecycle phase变化
       同一Runtime事务追加association revision + highest + current full-ref CAS
  -> child terminal_cleanup
       同事务追加terminal_cleanup association revision
  -> child termination_closed
       同事务追加terminal_closed association revision
  -> current/terminal Resolve按exact subject bootstrap执行S1/S2
  -> exact InspectTerminal按full Ref读immutable history
```

parent current变化不改写association history；Reader必须复读parent current并证明它仍是同一Run identity、同一ExecutionScope且相对创建anchor没有phase/revision回退。这样不会要求一次parent变化同时 fan-out 修改所有child关联。

## 5. 设计入口

- [精确合同](./contracts.md)
- [Port Delta](./port-delta.md)
- [测试矩阵](./test-matrix.md)
- [流程图](./run-lineage-association-v1.drawio)
- [实施计划](../../../plan/runtime/run-lineage-association-v1.md)

## 6. 当前准入结论

- Owner-local asset：READY，等待不同agent复审，不自标YES；
- Go实现：NO-GO；
- Detached Review生产组合：NO-GO；
- production backend/root/durability/SLA：NO-GO。

当前缺口不是Review内部状态机，而是Runtime Owner compound mutation、原子Store、trusted assembler接线、Application与Review的中立关联、以及Agent Host最终composition。任何一项未关闭时都必须Fail Closed。
