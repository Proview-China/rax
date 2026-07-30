# NextModelTurnEligibilityV1前半切片候选

时间：2026-07-31 03:26 CST

## 事件

Application新增只读 next-turn eligibility前半切片，绑定TurnContinuation原Attempt/current digest、ActiveContext、Run/Session/TargetTurn与future Runtime Model actual-point exact request digest，并稳定派生非授权性质的dispatch recovery ref。

## 当前真值

- Application不携带Harness Envelope，不建第二Store，不写任何Owner事实；
- Application不注入或调用Runtime actual-point guard，不接收/暴露guard projection；
- pending、过期、splice、cancel、future Fence坐标无效、Continuation current reader不可用与typed nil均在任何下游执行前Fail Closed；
- lost Inspect reply只允许用原exact request恢复；
- 64并发只读Inspect产生相同derived ref，不产生写入或Provider调用；
- TTL crossing、`now == expiry`与clock rollback为排他失败边界。
- eligibility TTL取request、Session、Continuation与future ModelBoundary四者最小值；boundary已过期、`now == boundary expiry`、seal后跨boundary及projection拓宽boundary均Fail Closed；
- 单次Continuation read仅为read-only advisory快照，真正dispatch必须由Harness/Runtime重新fresh-read，不新增第二Store。

## 验证

- focused `NextModelTurnEligibility` ordinary×100：PASS；
- focused race×20：PASS；
- Application full ordinary与full race：PASS；
- `go vet ./...`：PASS；
- 显式 import boundary与全部新增文件 whitespace/diff check：PASS；
- `go mod tidy -diff`：未通过，原因是 `origin/main` 同样存在的 `golang.org/x/text v0.27.0 -> v0.31.0` checksum漂移；本切片未修改 `go.mod/go.sum`，当前 `go.sum` SHA-256与`origin/main`一致。

当前仍为未提交代码与测试候选。Runtime guard必须在Model boundary CAS winner与Model S3后、同一call stack紧贴物理Provider调用，guard projection不得缓存或返回Harness/Application。Harness V2 exact adapter、Model provider boundary接线、真实Provider dispatch、Agent Loop与production root均未完成，production Dispatch继续HARD-BLOCKED。
