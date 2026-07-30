# Application Next Model Turn Eligibility V1 实施计划候选

状态：前半切片代码候选；production Dispatch HARD-BLOCKED。

## 本轮范围

- [x] additive中立 request、derived ref、短期 projection；
- [x] TurnContinuation read-only current reader；
- [x] future Runtime Model actual-point exact request refs/digest坐标；
- [x] pending/expired/splice/cancel/invalid Fence coordinate/Continuation current unavailable/typed nil零下游副作用；
- [x] lost Inspect reply原 exact坐标恢复；
- [x] stable derived ref与64并发只读验证；
- [x] request/Session/Continuation/future ModelBoundary四路最小TTL；
- [x] ModelBoundary先过期、`now == boundary expiry`、seal后跨boundary TTL、projection禁止拓宽boundary expiry；
- [x] TTL crossing、`now == expiry`、clock rollback；
- [x] focused ordinary 100与race 20；
- [x] Application full ordinary/race/vet、diff/import boundary；
- [ ] 独立代码审计与PR合并。

验证补充：`go mod tidy -diff` 仍显示 `origin/main` 已存在的 `golang.org/x/text v0.27.0 -> v0.31.0` checksum漂移；本切片未修改 `go.mod/go.sum`，两者中的 `go.sum` SHA-256仍与 `origin/main` 完全一致。该基线漂移不计为本切片通过项，也不在本分支擅自修复。

## 不做事项

- 不实现 Harness V2 exact adapter；
- 不注入或调用 Runtime actual-point guard，不接收/暴露guard projection；
- 不调用或伪造 Provider；
- 不修改 Harness、Context、Tool、Model、Runtime、Host、Console；
- 不新增 Store、Journal、Owner fact、Capability或production composition root；
- 不把 eligibility projection称为Permit、Authorization、Agent Loop或production GO。
- 不把单次Continuation read称为dispatch current证明；真正dispatch由Harness/Runtime重新fresh-read。

## 解锁条件

下一切片必须由 Harness Owner提供 exact adapter。Runtime actual-point guard只能在Model boundary CAS winner与Model S3之后，于同一call stack紧贴物理Provider调用并现场消费；projection不得缓存或返回Harness/Application。所有 direct/stream/continuation/realtime/raw路径完成no-bypass验证前，production Dispatch保持HARD-BLOCKED。
