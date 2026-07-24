# Sandbox Checkpoint Attempt Identity二轮审计纠偏

- 时间：2026-07-16 15:06 CST
- 触发：二轮独立审计确认`CheckpointPhaseReservation.AttemptID`参与phase/branch key，
  但未强制绑定Runtime拥有的global `CheckpointAttemptRef.ID`；caller可替换字符串尝试
  绕过create-once。追加审计同时指出current refresh seam缺少实际测试证据。
- 状态：`implemented-candidate/NO-GO`；`FeatureCheckpointParticipant=false`，Provider=0。

## 本轮真值

- 唯一权威Attempt identity是`Base.CheckpointAttempt.ID`；Reservation、PhaseFact、current
  request/coordinate/projection中的兼容字段`AttemptID`必须与其exact相等；
- phase key=`Tenant+CheckpointAttemptRef.ID+ParticipantID+Phase`，branch key=
  `Tenant+CheckpointAttemptRef.ID+ParticipantID`，不再信任caller提供的重复字符串；
- 反例使用同一global CheckpointAttempt与Participant替换`AttemptID`，即使重算合法摘要
  也在Owner CAS前拒绝，Participant current pointer不移动；既有branch conflict与历史
  no-ABA保持关闭；
- `ReplaceCheckpointCurrent`现有testkit seam已被测试实际调用：exact TTL refresh产生新
  projection/digest，证明Reader重新读取Owner current；Lease epoch、Fence epoch、
  ChangeSet ref或watermark sequence任一漂移均fail closed。
- branch因果性测试先提交commit、复读最新Participant current，再以同一prepared closure
  构造abort及32路并发sibling；全部由branch guard返回Conflict，Participant current不移动，
  不以复用旧revision造成的CAS失败代替sibling互斥证明。

## 当前验证边界

本增量实际通过：

```text
go test -count=1 ./contract ./kernel
go test -count=1 ./contract ./kernel -run 'Checkpoint.*(Attempt|Refresh|Branch|ABA)' -v
go vet ./contract ./kernel
```

Runtime public ports恢复后，本增量进一步实际通过：

```text
go test -count=100 -shuffle=on ./contract ./kernel ./tests -run 'Checkpoint'
go test -count=20 -race -shuffle=on ./contract ./kernel ./tests -run 'Checkpoint'
go test -count=1 -shuffle=on ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
```
