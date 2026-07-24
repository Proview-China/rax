# Checkpoint Participant governed顺序接线

时间：2026-07-18 10:55 CST

## 结果

- 新增Sandbox自有`GovernedCheckpointParticipantApplicationAdapterV1`；
- 唯一顺序为`prepare/capture -> Owner S1 -> commit -> Owner S2`；
- prepare/commit丢回包只Inspect原Checkpoint Attempt与Participant；
- commit必须保留exact prepared closure，S1/S2任一Fact或Projection drift均Fail Closed；
- live Runtime Attempt/Barrier/EffectCut、Participant phase、Evidence V1、Settlement V5已经足够，
  本阶段Checkpoint capture的最小Runtime schema Delta为0。

## 实际验证

- `go test ./applicationadapter -run 'GovernedCheckpointParticipant' -count=100`：PASS；
- `go test -race ./applicationadapter -run 'GovernedCheckpointParticipant' -count=20`：PASS；
- `go test ./...`：PASS；
- `go vet ./...`：PASS。

## 未解锁

尚未实现Lifecycle对真实Workspace Coverage/Participant Owner Fact、Evidence/Settlement与Manifest的
映射；Provider、production root、Restore、Purge/Delete和外部世界回滚继续NO-GO。
