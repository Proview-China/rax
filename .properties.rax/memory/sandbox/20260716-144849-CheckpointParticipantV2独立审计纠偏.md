# Sandbox Checkpoint Participant V2独立审计纠偏

- 时间：2026-07-16 14:48 CST
- 触发：独立代码审计判定前一版测试虽绿但存在caller自造PhaseFact、Participant revision
  未CAS、current presence不完整、历史覆盖与TTL/alias绕过。
- 当前裁决：`implemented-candidate/NO-GO`；`FeatureCheckpointParticipant=false`、
  `FeatureCheckpointRestore=false`。本记录新增纠偏，不改写上一条历史memory。

## 返修真值

- 生产`CheckpointController`删除公开`RecordPhaseFact`与reconcile写入口；
  `CheckpointPhaseStore`不再公开Create/CAS Fact。完整DomainResult current、Evidence-consumed
  current、Runtime Settlement V5 current复读及原子ApplySettlement/PhaseFact/closure未实现，
  因此生产不能产生prepared closure，也不能执行Checkpoint；
- Reservation补齐Previous显式presence、Instance/Epoch、Lease/Epoch、Fence、稳定Participant
  AttemptID、独立expected Runtime Attempt exact ref、Operation/Effect、ChangeSet与source watermarks；
- pre_admission/pre_prepare/pre_execute对全部gate逐项声明present/absent；Reader对present复读
  exact current，对absent按完整坐标确认Owner NotFound，任一early gate或漂移返回零投影；
- stable key=`Tenant+AttemptID+ParticipantID+Phase`，branch key=`Tenant+AttemptID+ParticipantID`；
  Reserve原子CAS Participant current revision，Owner/content/revision/closure漂移均Conflict；
- testkit Store以`ID+Revision+Digest`保存Participant/PhaseFact append-only历史并维护current
  pointer；Unknown追加新revision而不覆盖旧Fact，历史只接受full exact Ref Inspect；
- Seal、Controller入口、Store写读与projection返回均深拷pointer/slice；PhaseFact/testkit
  Participant TTL不得超过传入的Reader最短TTL。testkit append只模拟未来完整Owner Apply链，
  不是生产能力或公共Port。

## 实际验证

在`ExecutionRuntime/sandbox`通过：

```text
go test -count=100 -shuffle=on ./contract ./kernel ./tests -run 'Checkpoint'
go test -count=20 -race -shuffle=on ./contract ./kernel ./tests -run 'Checkpoint'
go test -count=1 -shuffle=on ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
```

新增反例覆盖64种不同Reservation内容并发、跨Effect current、双prepared closure、
required-absent early gate、Participant stale revision、Fact TTL超Reader、历史full exact Ref、
lost-reply/no-ABA与Seal/Store clone。无写入`gofmt -l`、diff、尾空白、依赖与禁止import
扫描均PASS；生产Controller/Port不存在PhaseFact写入口。
