# TurnContinuation V1公共合同冻结

- 2026-07-30冻结首条Loop continuation跨Owner公共合同：给定settled ToolResult exact ref与独立sealed Context prepare exact ref后进入`continuation_pending`，Context `applied_current`后由Harness CAS `ActiveContextRef`，完成前禁止下一Model Turn。
- 后续同PR已在Harness `applicationadapter`落成single-node `SQLiteTurnContinuationStoreV1`：WAL+FULL synchronous、Attempt create-once、exact pending/ActiveContext CAS、canonical row/index anti-splice、lost-reply Inspect与restart recovery。
- 本切片只co-seal两个independently sealed exact refs，不证明Context prepare内容来自SettledToolResult payload；现在只证明公共合同与Harness Owner-local durable continuation CAS/recovery，不声称Application coordinator、完整真实Loop或下一Model实际调用。
- Context prepare的Attempt exact四元组必须在Begin前记录；Commit按Kind/ID/Revision/Digest全等验证，same ID/revision different digest必须拒绝。
- 未采用分布式事务：Tool、Context、Harness各自Owner-local提交；丢回包只Inspect原Attempt。
- V1只有pending/current；Context永久失败时保持fail-closed并Inspect原Attempt，Failure/Run termination与下一Turn出口尚未冻结。
- `Tool=1 / Memory=0 / Knowledge=0`已由上游PR #32（`origin/main@f6014b93`）闭合Application/Context delta；本PR只消费该基线，不修改Context，也不以durable store代替内容派生证据。
- #23已加固Unix socket路径回归；Sandbox `workspace.read`真实Port、production host plane与actual-point read仍留下一PR，不得用catalog或fake冒充。
- Begin/Commit在writer wait后重新采样时钟，过exclusive TTL不写；schema version集合只允许V1；并发同请求幂等，不同Commit只有一个winner，stale ActiveContext/body splice fail-closed，返回值deep-copy。
- 2026-07-30 Harness相关ordinary/race/vet/diff-check live PASS。此切片仅属Go Runtime内部，不输出Console/TS ViewModel/Event，Application composition-root、真实Model调用与production root继续NO-GO。
