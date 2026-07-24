# Run Lineage Association V1 实施计划候选

状态：**已吸收首轮独立审计`NO（P0=1/P1=2/P2=0）`并完成最小资产返修；READY等待不同agent复审，不自标YES；Go与production root NO-GO**。

设计入口：[README](../../design/runtime/run-lineage-association-v1/README.md)、[精确合同](../../design/runtime/run-lineage-association-v1/contracts.md)、[Port Delta](../../design/runtime/run-lineage-association-v1/port-delta.md)、[测试矩阵](../../design/runtime/run-lineage-association-v1/test-matrix.md)、[draw.io](../../design/runtime/run-lineage-association-v1/run-lineage-association-v1.drawio)。

## 1. 范围

实现Runtime通用parent/child Run关联、compound pending create、append-only history/current/highest、subject-bootstrap current/terminal Resolve、exact current与historical terminal Inspect、具名Reader dependencies/recovery policy及reusable Conformance。首个联合消费者可以是Detached Review，但Runtime代码与资产不包含Review/Harness/Application实现类型。

不做：第二Run生命周期、Review Case/Target/Verdict、Harness等待状态、V3 Fact字段修改、Provider调用、production DB/RPC/SLA、Agent Host root。

## 2. 依赖DAG

```text
Runtime live Run V3 contracts/store transaction能力
  -> additive ports/canonical
  -> Owner compound mutation/history/current
  -> Kernel current/terminal Reader
  -> public Conformance + fault/race
  -> Application中立adapter（Application Owner）
  -> Review detached consumption（Review Owner）
  -> Agent Host production composition（最后）
```

无环导入见Port Delta；Runtime不得import Application/Review/Harness。

## 3. 文件级候选落点

联合审计YES且用户另行授权后，Runtime Owner最小落点：

```text
ExecutionRuntime/runtime/ports/run_lineage_association_v1.go
ExecutionRuntime/runtime/control/run_lineage_association_v1.go
ExecutionRuntime/runtime/kernel/run_lineage_association_gateway_v1.go
ExecutionRuntime/runtime/fakes/run_lineage_association_store_v1.go
ExecutionRuntime/runtime/conformance/run_lineage_association_v1.go
ExecutionRuntime/runtime/tests/ports/run_lineage_association_v1_test.go
ExecutionRuntime/runtime/tests/control/run_lineage_association_v1_test.go
ExecutionRuntime/runtime/tests/kernel/run_lineage_association_gateway_v1_test.go
ExecutionRuntime/runtime/tests/fakes/run_lineage_association_store_v1_test.go
ExecutionRuntime/runtime/tests/conformance/run_lineage_association_v1_test.go
```

若live production store仍由现有Run bundle/EffectIndex多个独立FactPort组成，必须先由Runtime Owner提供同一事务实现；禁止在Kernel用补偿删除模拟原子性。

## 4. 阶段

### P0 资产冻结

- [x] 完整复读治理、Runtime/Harness/Application live合同与Review detached设计；
- [x] 冻结通用Owner/非Owner和不重复定义原则；
- [x] 冻结exact types/JSON/canonical/stable ID；
- [x] 冻结compound create、history/highest/current原子事务；
- [x] 冻结S1/S2、TTL、clock、lost reply与closed errors；
- [x] 冻结terminal Subject Resolve、exact terminal history、具名Store/Lifecycle/Clock/recovery constructor；
- [x] 冻结same-revision anchor full exact与higher-revision RecordDigest/phase重算；
- [x] 冻结23个硬反例、compat/import DAG与production NO-GO；
- [ ] 两个独立资产审计均P0/P1/P2=0；
- [ ] 用户/管理线明确授权Go。

### P1 ports与canonical（获批后）

- [ ] 新增additive public nominal类型、Validate/Clone/Derive/Digest；
- [ ] literal JSON/digest golden、strict JSON duplicate-key与missing-field反例；
- [ ] Reader/Assembler method-set、Terminal Resolve/Inspect分离和typed-nil constructor compile shape；
- [ ] 确认V3文件hash/方法集/JSON/digest不变。

### P2 Owner compound store（获批后）

- [ ] 与child pending bundle/EffectIndex同事务发布association rev1；
- [ ] history/highest/current/receipt同事务stage/commit；
- [ ] Run record revision或phase变化与child lifecycle同事务revision+1；
- [ ] historical exact不借current index；
- [ ] lost reply只Inspect原canonical closure；
- [ ] memory与目标持久store通过同一Conformance。

### P3 current/terminal Reader（获批后）

- [ ] current/terminal Resolve：Subject→current index→exact history的baseline→S1→fresh→S2→fresh；
- [ ] InspectTerminal：same full Ref exact history双读，不借current index；
- [ ] full index/ref/history/parent/child envelope比较；
- [ ] min TTL、rollback、TTL crossing、terminal exact；
- [ ] `0<timeout<=2s`、TTL/deadline裁剪的bounded detached read recovery与deep clone；
- [ ] zero projection/error closed matrix。

### P4 验收（获批后）

- [ ] 单元：canonical/Validate/phase/TTL/ref/index；
- [ ] 白盒：transaction staging/current index/ABA/lock order；
- [ ] 黑盒：public Assembler→Reader→terminal链，不导入fakes/internal；
- [ ] 故障：每个stage、lost reply、ctx、Unavailable、clock；
- [ ] Conformance：public-only Store/Reader suite；
- [ ] count100、race20、full ordinary/race/vet、gofmt/diff/import；
- [ ] 两次独立代码审计0/0/0。

### P5 联合接线（其他Owner，最后）

- [ ] Application Owner以中立association ref持久关联其detached waiting intent；
- [ ] Review Owner只消费Reader，不创建/修改Run；
- [ ] Harness保持现有Review Phase source，不新增waiting状态；
- [ ] Agent Host trusted assembler注入真实Runtime compound backend；
- [ ] 重启恢复、parent/child终态、Cleanup/Residual系统测试；
- [ ] production root必须另行独立审计与用户GO。

## 5. 验收标准

1. 任意partial create/phase failure均无child/association/history/current/receipt泄露；
2. same subject稳定ID、revision严格+1、历史不可覆盖、ABA可检测；
3. current/terminal Resolve均为parent+child+association/index的单一S1/S2一致快照；InspectTerminal按full Ref返回不借current index的truthful immutable terminal closure；
4. Unknown只Inspect，mutation调用总数不增加；
5. clock rollback、TTL crossing、identity/scope/phase/digest漂移全部zero result；parent same revision必须full exact等于anchor，高revision必须重算RecordDigest/phase；
6. 不改V3语义、不复制相邻Owner类型、不产生Review/Runtime Outcome授权；
7. Fake/reference测试不升级为production声明。

## 6. 当前结论

本Plan只把Runtime Owner所需Delta收敛到可审计、可实现的文件级范围。现阶段不写Go、不接root、不声称Detached Review生产闭环；等待独立资产审计后再决定实现门。
