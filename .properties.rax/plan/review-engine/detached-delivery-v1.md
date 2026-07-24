# Review Detached Delivery V1 实施计划

## 状态

- Review-owned design：FROZEN CANDIDATE，等待独立审计；
- Review-owned Go：NO-GO；
- production：Runtime lineage/current、Application detached coordination、Human delivery current、Host root四组P0仍OPEN；
- 不创建第二套Run/Thread状态机，不修改Runtime V3/Harness私有Port。

## DAG

```text
Runtime RunLineage/current -----------┐
Application detached coordination ---┼─> Review DeliveryBinding/Closure ─> Parent Review Gate integration
Human governed delivery/current -----┤
Harness Phase current ---------------┤
Review Request/Case/Verdict ----------┘
                                      └─> Agent Host production root（最后）
```

无SCC：Review domain只import Review自有合同与Runtime public `core/ports`；Application/Harness/platform来源通过具名中立source coordinate映射并由各Owner Reader复读，Review不import其实现/合同；Runtime/Application/Harness不import Review实现；Host composition层持有具体constructor。

## 阶段与文件落点

1. Runtime Owner：冻结并实现lineage/current public Port、repository、SQLite、conformance；不得写Review专用Run。
2. Application Owner：冻结并实现Detached coordination，仅编排`ReviewWaitingV1 + RunCoordinatorV3 + Binding/Closure ref`。
3. Human delivery Owner：冻结governed delivery/current与cleanup/residual；平台评论仍为Observation。
4. Review Owner：在前三者0/0/0且用户实现门通过后，新增：
   - `contract/detached_delivery_v1.go`
   - `ports/detached_delivery_v1.go`
   - `memory/detached_delivery_v1.go`
   - `storage/sqlite/detached_delivery_v1.go`
   - `conformance/detached_delivery_v1.go`
   - `tests/detached_delivery_v1_test.go`
5. Agent Host：最后注入Runtime/Application/Harness/Review公开能力，闭合test-only fixture后再评production root。

## Review Go验收

- canonical ID/revision/digest literal golden；create-once、history、current、deep clone；
- exactly-one endpoint union；Request/Case/Target/Waiting/Phase/endpoint逐层exact；
- S1/S2、min TTL、clock rollback、ABA、lost reply、Unknown Inspect-only；
- Binding/Closure不授Authority/Evidence/Permit/Run Outcome；
- DET-01..40全部覆盖；
- ordinary100、race20、full ordinary/race/vet、gofmt/diff/import、跨Ownerconformance与system root实际PASS。

## 准入证据

每个外部Owner必须提供：公共类型/签名hash、Owner conformance 0/0/0、target100/race20/full/race/vet、lost-reply与TTL反例、SQLite/真实backend证据。最后Host root必须证明无fake/internal import、无runtime registry、唯一composition root和完整cleanup/residual。任何一项缺失继续NO-GO。
