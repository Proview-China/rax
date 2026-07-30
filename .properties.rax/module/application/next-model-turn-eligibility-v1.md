# Application Next Model Turn Eligibility V1 模块候选

## 作用

该模块在已完成的 TurnContinuation ActiveContext CAS 与未来 Harness V2 Model dispatch adapter之间，提供一次只读、短期、exact-bound的 eligibility检查。

## 组成

- `contract/next_model_turn_eligibility_v1.go`：request、stable derived dispatch ref、短期 projection；
- `ports/next_model_turn_eligibility_v1.go`：TurnContinuation窄只读 Reader与eligibility Inspect Port；
- `next_model_turn_eligibility_v1.go`：仅对Continuation current执行一次advisory读取、fresh clock、exact闭合与四路最小TTL；
- `tests/next_model_turn_eligibility_v1_test.go`：黑盒、故障、并发与边界矩阵。

## 输入与输出

输入只包含 TurnContinuation Attempt/current digest、ActiveContext、Run/Session/TargetTurn、Runtime actual-point exact request、RequestedNotAfter与request digest。

输出包含 stable derived dispatch ref和短期 eligibility projection。Projection NotAfter是request、Session、Continuation与future ModelBoundary expiry的最小值，且不得拓宽ModelBoundary expiry。输出只保留future Runtime actual-point request digest，不包含actual-point guard projection。输出没有 `Allowed` 字段，不授予Permit、Fence、Provider调用或Turn推进。

## 验证与限制

当前实现不持久化、不建第二Store、不写Owner事实、不注入或调用Runtime actual-point guard、不调用Provider。单次Continuation read只是read-only advisory快照，真正dispatch必须由Harness/Runtime重新fresh-read currentness。Runtime guard必须在Model boundary CAS winner与Model S3后，于同一call stack紧贴物理Provider调用；guard projection不得缓存或返回Harness/Application。Harness V2 exact adapter、Model provider boundary真实接线、no-bypass黑盒及production composition root均未完成，因此production Dispatch继续HARD-BLOCKED。
