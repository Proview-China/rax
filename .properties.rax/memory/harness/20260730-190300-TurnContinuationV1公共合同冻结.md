# TurnContinuation V1公共合同冻结

- 2026-07-30冻结首条Loop continuation跨Owner公共合同：给定settled ToolResult exact ref与独立sealed Context prepare exact ref后进入`continuation_pending`，Context `applied_current`后由Harness CAS `ActiveContextRef`，完成前禁止下一Model Turn。
- 本切片只co-seal两个independently sealed exact refs，不证明Context prepare内容来自SettledToolResult payload；只证明continuation CAS/recovery，不声称Application/Harness真实接线或完整真实Loop。
- Context prepare的Attempt exact四元组必须在Begin前记录；Commit按Kind/ID/Revision/Digest全等验证，same ID/revision different digest必须拒绝。
- 未采用分布式事务：Tool、Context、Harness各自Owner-local提交；丢回包只Inspect原Attempt。
- V1只有pending/current；Context永久失败时保持fail-closed并Inspect原Attempt，Failure/Run termination与下一Turn出口尚未冻结。
- 当前`Tool=1 / Memory=0 / Knowledge=0`仍被Context candidate拒绝；不在本合同中改写。
- #23已加固Unix socket路径回归；Sandbox `workspace.read`真实Port、production host plane与actual-point read仍留下一PR，不得用catalog或fake冒充。
- 此合同仅属Go Runtime内部，不输出Console/TS ViewModel/Event，Harness真实接线与production root继续NO-GO。
