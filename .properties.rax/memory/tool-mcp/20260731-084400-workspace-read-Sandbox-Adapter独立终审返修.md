# 2026-07-31 workspace.read Sandbox Adapter独立终审返修

- Tool Owner在独立worktree原地修复actual-entry与历史恢复因果缺口，未提交、未重基、未发布。
- S2后新增fresh actual-entry时钟、request/authorization/Command current复验及紧贴Execute的ctx门；
  clock回拨或`now==expiry`时Execute和physical read均为0。
- Execute返回zero、畸形、non-admitted、no-effect或错StableKey Admission时，不消费该返回值，
  仅从原Runtime Attempt执行bounded inspect-only recovery。
- Command Current/Historical Reader返回后立即deep clone唯一指针`ExpectedFileRef`；同步alias与
  并发mutator反例通过race门。
- Reservation StableKey、RequestDigest、AttemptID、TTL可证明轴及Admission/Binding时间关系已
  独立回扣；ExpectedFile与Observation File使用完整Ref比较。
- request schema改为直接调用`runtimeports.SchemaRefV2.Validate()`，非canonical namespace、
  SemVer、media type在Seal阶段Fail Closed。
- 最终focused blackbox/fault ordinary×100与race×20通过；仍等待独立复审。
- 当前仅为owner-local/reference：CorePack `Executable=false`。Runtime Admission production
  Reader、Association/View/Lease独立current、Artifact、production root/backend继续NO-GO。
